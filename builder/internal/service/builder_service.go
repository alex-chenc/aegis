package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"builder/internal/signer"
)

type BuilderService struct {
	signer      *signer.Signer
	workDir     string
	minioClient MinIOClient
}

type MinIOClient interface {
	UploadFile(ctx context.Context, bucket, key, filePath string) error
	DownloadFile(ctx context.Context, bucket, key, destPath string) error
	GetDownloadURL(ctx context.Context, bucket, key string) (string, error)
}

type BuildRequest struct {
	BuildID             string
	PackageID           string
	Version             string
	Title               string
	CVEIDs              []string
	Operator            string
	BuilderProfile      string
	TargetArch          string
	HookPlanYAML        string
	EBPFSource          string
	SigmaRulesYAML      string
	CorrelationYAML     string
	PackageMetadataJSON string
}

type BuildResult struct {
	BuildID                  string
	Status                   string
	ErrorMessage             string
	BuilderImageDigest       string
	ClangVersion             string
	BuildLogObjectKey        string
	BuildLogTail             string
	Artifacts                []BuildArtifact
	HookSummary              []HookSummary
	EventSchemaJSON          string
	UnsignedPackageObjectKey string
	UnsignedPackageSHA256    string
	UnsignedPackageSize      int64
}

type BuildArtifact struct {
	Name      string
	Transport string
	ObjectKey string
	SHA256    string
	Size      int64
}

type HookSummary struct {
	HookType       string
	AttachPoint    string
	ProgramSection string
	RiskLevel      string
}

type SignRequest struct {
	BuildID   string
	PackageID string
	Version   string
	Operator  string
	Confirm   bool
}

type SignResult struct {
	Success               bool
	Message               string
	PackageObjectKey      string
	SignatureObjectKey    string
	PackageSHA256         string
	PackageSize           int64
	SignatureAlgorithm    string
	SigningKeyFingerprint string
	SignedAt              int64
}

type BuilderInfo struct {
	BuilderVersion              string
	BuilderImage                string
	ClangVersion                string
	BPFToolVersion              string
	SupportedArches             []string
	SupportedTransports         []string
	SigningPublicKeyFingerprint string
}

func NewBuilderService(signer *signer.Signer, workDir string, minioClient MinIOClient) *BuilderService {
	return &BuilderService{
		signer:      signer,
		workDir:     workDir,
		minioClient: minioClient,
	}
}

func (s *BuilderService) GetBuilderInfo(ctx context.Context) (*BuilderInfo, error) {
	clangVersion, _ := exec.Command("clang", "--version").Output()
	bpftoolVersion, _ := exec.Command("bpftool", "version").Output()

	return &BuilderInfo{
		BuilderVersion:              "5.8.0",
		BuilderImage:                "aegis-agent-builder-ubi8:5.8.0",
		ClangVersion:                string(clangVersion),
		BPFToolVersion:              string(bpftoolVersion),
		SupportedArches:             []string{"amd64", "arm64"},
		SupportedTransports:         []string{"perf", "ringbuf"},
		SigningPublicKeyFingerprint: s.signer.GetPublicKeyFingerprint(),
	}, nil
}

func (s *BuilderService) StartBuild(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	buildDir := filepath.Join(s.workDir, req.BuildID)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, fmt.Errorf("create build dir: %w", err)
	}

	result := &BuildResult{
		BuildID: req.BuildID,
		Status:  "running",
	}

	sourceFile := filepath.Join(buildDir, "plugin.c")
	if err := os.WriteFile(sourceFile, []byte(req.EBPFSource), 0644); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("write source: %v", err)
		return result, nil
	}

	perfObj := filepath.Join(buildDir, "plugin.perf.bpf.o")
	if err := s.compileBPF(sourceFile, perfObj, "perf", req.TargetArch); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("compile perf: %v", err)
		return result, nil
	}

	ringbufObj := filepath.Join(buildDir, "plugin.ringbuf.bpf.o")
	if err := s.compileBPF(sourceFile, ringbufObj, "ringbuf", req.TargetArch); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("compile ringbuf: %v", err)
		return result, nil
	}

	stagingDir := filepath.Join(buildDir, "staging")
	os.MkdirAll(stagingDir, 0755)
	os.MkdirAll(filepath.Join(stagingDir, "plugin"), 0755)
	os.MkdirAll(filepath.Join(stagingDir, "rules"), 0755)
	os.MkdirAll(filepath.Join(stagingDir, "correlations"), 0755)

	// Copy artifacts to staging
	copyFile(perfObj, filepath.Join(stagingDir, "plugin", "copyfail.perf.bpf.o"))
	copyFile(ringbufObj, filepath.Join(stagingDir, "plugin", "copyfail.ringbuf.bpf.o"))

	os.WriteFile(filepath.Join(stagingDir, "package.yaml"), []byte(req.PackageMetadataJSON), 0644)
	os.WriteFile(filepath.Join(stagingDir, "rules", "atomic_sigma.yml"), []byte(req.SigmaRulesYAML), 0644)
	os.WriteFile(filepath.Join(stagingDir, "correlations", "correlation.yml"), []byte(req.CorrelationYAML), 0644)

	packageFile := filepath.Join(buildDir, "package.tar.gz")
	if err := s.createTarGz(stagingDir, packageFile); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("create tar.gz: %v", err)
		return result, nil
	}

	objectKey := fmt.Sprintf("detection-packages/%s/%s/unsigned/package.tar.gz", req.PackageID, req.Version)
	if err := s.minioClient.UploadFile(ctx, "aegis-builds", objectKey, packageFile); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("upload: %v", err)
		return result, nil
	}

	result.Status = "success"
	result.UnsignedPackageObjectKey = objectKey
	result.BuilderImageDigest = "sha256:placeholder"
	result.ClangVersion = "17.0.0"

	return result, nil
}

func (s *BuilderService) SignPackage(ctx context.Context, req SignRequest) (*SignResult, error) {
	if !req.Confirm {
		return &SignResult{Success: false, Message: "confirm=true required"}, nil
	}

	unsignedKey := fmt.Sprintf("detection-packages/%s/%s/unsigned/package.tar.gz", req.PackageID, req.Version)
	localPath := filepath.Join(s.workDir, req.BuildID, "unsigned.tar.gz")
	if err := s.minioClient.DownloadFile(ctx, "aegis-builds", unsignedKey, localPath); err != nil {
		return &SignResult{Success: false, Message: fmt.Sprintf("download: %v", err)}, nil
	}

	signature, err := s.signer.SignFile(localPath)
	if err != nil {
		return &SignResult{Success: false, Message: fmt.Sprintf("sign: %v", err)}, nil
	}

	signedKey := fmt.Sprintf("detection-packages/%s/%s/signed/package.tar.gz", req.PackageID, req.Version)
	if err := s.minioClient.UploadFile(ctx, "aegis-releases", signedKey, localPath); err != nil {
		return &SignResult{Success: false, Message: fmt.Sprintf("upload signed: %v", err)}, nil
	}

	sigFile := filepath.Join(s.workDir, req.BuildID, "package.tar.gz.sig")
	os.WriteFile(sigFile, signature, 0644)
	sigKey := fmt.Sprintf("detection-packages/%s/%s/signed/package.tar.gz.sig", req.PackageID, req.Version)
	if err := s.minioClient.UploadFile(ctx, "aegis-releases", sigKey, sigFile); err != nil {
		return &SignResult{Success: false, Message: fmt.Sprintf("upload sig: %v", err)}, nil
	}

	stat, _ := os.Stat(localPath)

	return &SignResult{
		Success:               true,
		PackageObjectKey:      signedKey,
		SignatureObjectKey:    sigKey,
		PackageSHA256:         "placeholder",
		PackageSize:           stat.Size(),
		SignatureAlgorithm:    "Ed25519",
		SigningKeyFingerprint: s.signer.GetPublicKeyFingerprint(),
		SignedAt:              time.Now().Unix(),
	}, nil
}

func (s *BuilderService) compileBPF(source, output, transport, arch string) error {
	cmd := exec.Command("clang", "-O2", "-g", "-target", "bpf",
		"-D__TARGET_ARCH_"+arch,
		"-DTRANSPORT_"+transport,
		"-c", source, "-o", output)
	cmd.Dir = s.workDir
	return cmd.Run()
}

func (s *BuilderService) createTarGz(sourceDir, outputFile string) error {
	cmd := exec.Command("tar", "-czf", outputFile, "-C", sourceDir, ".")
	return cmd.Run()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

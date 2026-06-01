package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"builder/internal/signer"

	"gopkg.in/yaml.v3"
)

type BuilderService struct {
	signer        *signer.Signer
	workDir       string
	minioClient   MinIOClient
	buildLogs     map[string]string
	buildStatuses map[string]string
	mu            sync.Mutex
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
	BuilderImageDigest          string
	LlvmVersion                 string
	LibbpfVersion               string
}

type BuildStatusInfo struct {
	BuildID         string
	Status          string
	ErrorMessage    string
	ProgressPercent int32
}

func NewBuilderService(signer *signer.Signer, workDir string, minioClient MinIOClient) *BuilderService {
	return &BuilderService{
		signer:        signer,
		workDir:       workDir,
		minioClient:   minioClient,
		buildLogs:     make(map[string]string),
		buildStatuses: make(map[string]string),
	}
}

func (s *BuilderService) GetBuilderInfo(ctx context.Context) (*BuilderInfo, error) {
	clangVersionRaw, _ := exec.Command("clang", "--version").Output()
	bpftoolVersionRaw, _ := exec.Command("bpftool", "version").Output()

	clangVersion := parseClangVersion(string(clangVersionRaw))
	bpftoolVersion := string(bpftoolVersionRaw)
	libbpfVersion := parseLibbpfVersion(bpftoolVersion)

	return &BuilderInfo{
		BuilderVersion:              "5.8.0",
		BuilderImage:                "aegis-agent-builder-ubi8:5.8.0",
		ClangVersion:                clangVersion,
		BPFToolVersion:              bpftoolVersion,
		SupportedArches:             []string{"amd64", "arm64"},
		SupportedTransports:         []string{"perf", "ringbuf"},
		SigningPublicKeyFingerprint: s.signer.GetPublicKeyFingerprint(),
		BuilderImageDigest:          os.Getenv("BUILDER_IMAGE_DIGEST"),
		LlvmVersion:                 clangVersion,
		LibbpfVersion:               libbpfVersion,
	}, nil
}

func (s *BuilderService) StartBuild(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	buildDir := filepath.Join(s.workDir, req.BuildID)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, fmt.Errorf("create build dir: %w", err)
	}

	// Clean old build artifacts to avoid conflicts from previous builds
	if files, _ := filepath.Glob(filepath.Join(buildDir, "*.bpf.o")); files != nil {
		for _, f := range files {
			os.Remove(f)
		}
	}
	if files, _ := filepath.Glob(filepath.Join(buildDir, "*.tar.gz")); files != nil {
		for _, f := range files {
			os.Remove(f)
		}
	}

	if err := validateBuildInput(req); err != nil {
		return &BuildResult{
			BuildID:      req.BuildID,
			Status:       "failed",
			ErrorMessage: fmt.Sprintf("validation: %v", err),
		}, nil
	}

	result := &BuildResult{
		BuildID: req.BuildID,
		Status:  "running",
	}

	var buildLog strings.Builder

	sourceFile := filepath.Join(buildDir, "plugin.c")
	if err := os.WriteFile(sourceFile, []byte(req.EBPFSource), 0644); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("write source: %v", err)
		s.storeBuildResult(req.BuildID, "failed", buildLog.String())
		return result, nil
	}

	perfObj := filepath.Join(buildDir, "plugin.perf.bpf.o")
	perfLog, err := s.compileBPF(sourceFile, perfObj, "perf", req.TargetArch)
	buildLog.WriteString(perfLog)
	if err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("compile perf: %v", err)
		s.storeBuildResult(req.BuildID, "failed", buildLog.String())
		return result, nil
	}

	ringbufObj := filepath.Join(buildDir, "plugin.ringbuf.bpf.o")
	ringbufLog, err := s.compileBPF(sourceFile, ringbufObj, "ringbuf", req.TargetArch)
	buildLog.WriteString(ringbufLog)
	if err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("compile ringbuf: %v", err)
		s.storeBuildResult(req.BuildID, "failed", buildLog.String())
		return result, nil
	}

	stagingDir := filepath.Join(buildDir, "staging")
	os.MkdirAll(stagingDir, 0755)
	os.MkdirAll(filepath.Join(stagingDir, "plugin"), 0755)
	os.MkdirAll(filepath.Join(stagingDir, "rules"), 0755)
	os.MkdirAll(filepath.Join(stagingDir, "correlations"), 0755)

	copyFile(perfObj, filepath.Join(stagingDir, "plugin", req.PackageID+".perf.bpf.o"))
	copyFile(ringbufObj, filepath.Join(stagingDir, "plugin", req.PackageID+".ringbuf.bpf.o"))

	// Inject sigma_rules and correlation_rules into package.yaml so the agent
	// can discover and load them at runtime.
	pkgYAML := req.PackageMetadataJSON
	if pkgYAML == "" {
		meta := map[string]interface{}{
			"schema_version": "aegis.ebpf_plugin.v1",
			"package_id":     req.PackageID,
			"version":        req.Version,
		}
		if out, err := yaml.Marshal(meta); err == nil {
			pkgYAML = string(out)
		}
	}
	if req.SigmaRulesYAML != "" || req.CorrelationYAML != "" {
		var meta map[string]interface{}
		if err := yaml.Unmarshal([]byte(pkgYAML), &meta); err == nil {
			changed := false
			if req.SigmaRulesYAML != "" {
				if _, ok := meta["sigma_rules"]; !ok {
					meta["sigma_rules"] = []string{"rules/atomic_sigma.yml"}
					changed = true
				}
			}
			if req.CorrelationYAML != "" {
				if _, ok := meta["correlation_rules"]; !ok {
					meta["correlation_rules"] = []string{"correlations/correlation.yml"}
					changed = true
				}
			}
			if changed {
				if out, err := yaml.Marshal(meta); err == nil {
					pkgYAML = string(out)
				}
			}
		}
	}
	os.WriteFile(filepath.Join(stagingDir, "package.yaml"), []byte(pkgYAML), 0644)
	os.WriteFile(filepath.Join(stagingDir, "rules", "atomic_sigma.yml"), []byte(req.SigmaRulesYAML), 0644)
	os.WriteFile(filepath.Join(stagingDir, "correlations", "correlation.yml"), []byte(req.CorrelationYAML), 0644)

	packageFile := filepath.Join(buildDir, "package.tar.gz")
	if err := s.createTarGz(stagingDir, packageFile); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("create tar.gz: %v", err)
		s.storeBuildResult(req.BuildID, "failed", buildLog.String())
		return result, nil
	}

	sha256Hash, err := computeSHA256(packageFile)
	if err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("compute sha256: %v", err)
		s.storeBuildResult(req.BuildID, "failed", buildLog.String())
		return result, nil
	}

	stat, _ := os.Stat(packageFile)

	objectKey := fmt.Sprintf("detection-packages/%s/%s/unsigned/package.tar.gz", req.PackageID, req.Version)
	if err := s.minioClient.UploadFile(ctx, "aegis-builds", objectKey, packageFile); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("upload: %v", err)
		s.storeBuildResult(req.BuildID, "failed", buildLog.String())
		return result, nil
	}

	eventSchemaJSON := extractEventSchema(req.PackageMetadataJSON)
	hookSummaries := generateHookSummaries(req.HookPlanYAML)

	logStr := buildLog.String()
	logObjectKey := fmt.Sprintf("detection-packages/%s/%s/build.log", req.PackageID, req.Version)
	logFile := filepath.Join(buildDir, "build.log")
	os.WriteFile(logFile, []byte(logStr), 0644)
	s.minioClient.UploadFile(ctx, "aegis-builds", logObjectKey, logFile)

	buildLogTail := logStr
	if len(buildLogTail) > 4096 {
		buildLogTail = buildLogTail[len(buildLogTail)-4096:]
	}

	s.storeBuildResult(req.BuildID, "awaiting_review", logStr)

	result.Status = "awaiting_review"
	result.UnsignedPackageObjectKey = objectKey
	result.UnsignedPackageSHA256 = sha256Hash
	result.UnsignedPackageSize = stat.Size()
	result.BuilderImageDigest = os.Getenv("BUILDER_IMAGE_DIGEST")
	result.ClangVersion = parseClangVersion("")
	result.BuildLogObjectKey = logObjectKey
	result.BuildLogTail = buildLogTail
	result.EventSchemaJSON = eventSchemaJSON
	result.HookSummary = hookSummaries

	// Populate artifact summary for perf and ringbuf objects
	perfObjPath := filepath.Join(stagingDir, "plugin", req.PackageID+".perf.bpf.o")
	ringbufObjPath := filepath.Join(stagingDir, "plugin", req.PackageID+".ringbuf.bpf.o")
	perfStat, _ := os.Stat(perfObjPath)
	ringbufStat, _ := os.Stat(ringbufObjPath)
	perfSHA, _ := computeSHA256(perfObjPath)
	ringbufSHA, _ := computeSHA256(ringbufObjPath)

	if perfStat != nil {
		result.Artifacts = append(result.Artifacts, BuildArtifact{
			Name:      req.PackageID + ".perf.bpf.o",
			Transport: "perf",
			ObjectKey: fmt.Sprintf("detection-packages/%s/%s/artifacts/perf.bpf.o", req.PackageID, req.Version),
			SHA256:    perfSHA,
			Size:      perfStat.Size(),
		})
	}
	if ringbufStat != nil {
		result.Artifacts = append(result.Artifacts, BuildArtifact{
			Name:      req.PackageID + ".ringbuf.bpf.o",
			Transport: "ringbuf",
			ObjectKey: fmt.Sprintf("detection-packages/%s/%s/artifacts/ringbuf.bpf.o", req.PackageID, req.Version),
			SHA256:    ringbufSHA,
			Size:      ringbufStat.Size(),
		})
	}

	return result, nil
}

func (s *BuilderService) SignPackage(ctx context.Context, req SignRequest) (*SignResult, error) {
	if !req.Confirm {
		return &SignResult{Success: false, Message: "confirm=true required"}, nil
	}

	s.mu.Lock()
	status := s.buildStatuses[req.BuildID]
	s.mu.Unlock()

	if status != "success" {
		return &SignResult{Success: false, Message: fmt.Sprintf("build status is %q, must be reviewed (success) before signing", status)}, nil
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

	sha256Hash, _ := computeSHA256(localPath)
	stat, _ := os.Stat(localPath)

	return &SignResult{
		Success:               true,
		PackageObjectKey:      signedKey,
		SignatureObjectKey:    sigKey,
		PackageSHA256:         sha256Hash,
		PackageSize:           stat.Size(),
		SignatureAlgorithm:    "Ed25519",
		SigningKeyFingerprint: s.signer.GetPublicKeyFingerprint(),
		SignedAt:              time.Now().Unix(),
	}, nil
}

func (s *BuilderService) ReviewBuild(ctx context.Context, buildID, packageID, version string, approved bool, comment, reviewer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, ok := s.buildStatuses[buildID]
	if !ok {
		return fmt.Errorf("build %s not found", buildID)
	}

	if status != "awaiting_review" {
		return fmt.Errorf("build %s is in status %q, cannot review", buildID, status)
	}

	if approved {
		s.buildStatuses[buildID] = "success"
	} else {
		s.buildStatuses[buildID] = "rejected"
	}

	return nil
}

func (s *BuilderService) GetPackageBuildStatus(ctx context.Context, packageID, version, buildID string) (*BuildStatusInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, ok := s.buildStatuses[buildID]
	if !ok {
		return nil, fmt.Errorf("build %s not found", buildID)
	}

	info := &BuildStatusInfo{
		BuildID: buildID,
		Status:  status,
	}

	if status == "failed" {
		logStr := s.buildLogs[buildID]
		if len(logStr) > 512 {
			info.ErrorMessage = logStr[len(logStr)-512:]
		} else {
			info.ErrorMessage = logStr
		}
	}

	switch status {
	case "running":
		info.ProgressPercent = 50
	case "awaiting_review":
		info.ProgressPercent = 90
	case "success":
		info.ProgressPercent = 100
	case "failed", "rejected":
		info.ProgressPercent = 100
	}

	return info, nil
}

func (s *BuilderService) compileBPF(source, output, transport, arch string) (string, error) {
	cmd := exec.Command("clang", "-O2", "-g", "-target", "bpf",
		"-D__TARGET_ARCH_"+arch,
		"-DTRANSPORT_"+transport,
		"-c", source, "-o", output)
	cmd.Dir = s.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	log := fmt.Sprintf("=== compile %s %s ===\n%s%s", transport, arch, stdout.String(), stderr.String())
	return log, err
}

func (s *BuilderService) createTarGz(sourceDir, outputFile string) error {
	cmd := exec.Command("tar", "-czf", outputFile, "-C", sourceDir, ".")
	return cmd.Run()
}

func (s *BuilderService) storeBuildResult(buildID, status, buildLog string) {
	s.mu.Lock()
	s.buildLogs[buildID] = buildLog
	s.buildStatuses[buildID] = status
	s.mu.Unlock()
}

func computeSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file for sha256: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("compute sha256: %w", err)
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func validateBuildInput(req BuildRequest) error {
	if req.PackageID == "" {
		return fmt.Errorf("package_id is required")
	}
	if req.Version == "" {
		return fmt.Errorf("version is required")
	}
	if req.EBPFSource == "" {
		return fmt.Errorf("ebpf_source is required")
	}

	if req.PackageMetadataJSON != "" {
		var meta map[string]interface{}
		if err := yaml.Unmarshal([]byte(req.PackageMetadataJSON), &meta); err != nil {
			return fmt.Errorf("invalid package_metadata_json: %w", err)
		}
		if _, ok := meta["schema_version"]; !ok {
			return fmt.Errorf("package_metadata_json missing schema_version")
		}
		if pid, ok := meta["package_id"]; !ok {
			return fmt.Errorf("package_metadata_json missing package_id")
		} else if pidStr, ok := pid.(string); !ok || pidStr != req.PackageID {
			return fmt.Errorf("package_metadata_json package_id does not match request package_id")
		}
		if _, ok := meta["version"]; !ok {
			return fmt.Errorf("package_metadata_json missing version")
		}
	}

	if req.HookPlanYAML != "" {
		var hookPlan struct {
			Hooks []map[string]interface{} `yaml:"hooks"`
		}
		if err := yaml.Unmarshal([]byte(req.HookPlanYAML), &hookPlan); err != nil {
			return fmt.Errorf("invalid hook_plan_yaml: %w", err)
		}
		for i, hook := range hookPlan.Hooks {
			if _, ok := hook["attach_type"]; !ok {
				return fmt.Errorf("hook %d missing attach_type", i)
			}
			if _, ok := hook["attach"]; !ok {
				return fmt.Errorf("hook %d missing attach", i)
			}
		}
	}

	forbiddenHelpers := []string{
		"bpf_probe_read_kernel",
		"bpf_override_return",
		"bpf_setsockopt",
		"bpf_sk_redirect",
	}
	for _, helper := range forbiddenHelpers {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(helper) + `\b`)
		if pattern.MatchString(req.EBPFSource) {
			return fmt.Errorf("forbidden BPF helper call: %s", helper)
		}
	}

	return nil
}

func extractEventSchema(metadataJSON string) string {
	if metadataJSON == "" {
		return ""
	}
	var meta map[interface{}]interface{}
	if err := yaml.Unmarshal([]byte(metadataJSON), &meta); err != nil {
		return ""
	}
	eventSchema, ok := meta["event_schema"]
	if !ok {
		return ""
	}
	jsonBytes, err := json.Marshal(normalizeYAMLValue(eventSchema))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(jsonBytes))
}

func normalizeYAMLValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[interface{}]interface{}:
		normalized := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			normalized[fmt.Sprint(key)] = normalizeYAMLValue(val)
		}
		return normalized
	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			normalized[key] = normalizeYAMLValue(val)
		}
		return normalized
	case []interface{}:
		normalized := make([]interface{}, len(typed))
		for idx, val := range typed {
			normalized[idx] = normalizeYAMLValue(val)
		}
		return normalized
	default:
		return typed
	}
}

func generateHookSummaries(hookPlanYAML string) []HookSummary {
	if hookPlanYAML == "" {
		return nil
	}
	var hookPlan struct {
		Hooks []struct {
			Name       string `yaml:"name"`
			AttachType string `yaml:"attach_type"`
			Attach     string `yaml:"attach"`
		} `yaml:"hooks"`
	}
	if err := yaml.Unmarshal([]byte(hookPlanYAML), &hookPlan); err != nil {
		return nil
	}

	summaries := make([]HookSummary, 0, len(hookPlan.Hooks))
	for _, h := range hookPlan.Hooks {
		summaries = append(summaries, HookSummary{
			HookType:       h.Name,
			AttachPoint:    h.Attach,
			ProgramSection: h.AttachType,
			RiskLevel:      assessRiskLevel(h.AttachType),
		})
	}
	return summaries
}

func assessRiskLevel(attachType string) string {
	switch strings.ToLower(attachType) {
	case "kprobe", "kretprobe", "tracepoint":
		return "low"
	case "lsm", "fentry", "fexit":
		return "high"
	case "uprobe", "uretprobe", "usdt":
		return "medium"
	default:
		return "medium"
	}
}

func parseClangVersion(raw string) string {
	re := regexp.MustCompile(`clang version (\S+)`)
	matches := re.FindStringSubmatch(raw)
	if len(matches) >= 2 {
		return matches[1]
	}
	out, err := exec.Command("clang", "--version").Output()
	if err != nil {
		return "unknown"
	}
	matches = re.FindStringSubmatch(string(out))
	if len(matches) >= 2 {
		return matches[1]
	}
	return "unknown"
}

func parseLibbpfVersion(bpftoolOutput string) string {
	re := regexp.MustCompile(`libbpf v?(\S+)`)
	matches := re.FindStringSubmatch(bpftoolOutput)
	if len(matches) >= 2 {
		return matches[1]
	}
	return "unknown"
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/grpc"

	"gopkg.in/yaml.v3"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	goversion "github.com/hashicorp/go-version"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Sentinel errors for detection package operations.
// Handlers use errors.Is to map these to HTTP status codes.
var (
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound = errors.New("resource not found")
	// ErrInvalidState indicates the resource is in a state that disallows the operation.
	ErrInvalidState = errors.New("invalid resource state")
)

type DetectionPackageService struct {
	repo                  *repository.DetectionPackageRepo
	db                    *gorm.DB
	serverClient          GRPCServerClient
	builderClient         BuilderClient
	artifactDownloadBaseURL string // e.g. "http://minio:9000/agent-artifacts"
}

type GRPCServerClient interface {
	InstallDetectionPackageFromService(ctx context.Context, hostID string, command interface{}) (int32, error)
	SyncAgentConfig(ctx context.Context, hostID string, configs []*pb.AgentConfig) (int32, error)
	UninstallDetectionPackage(ctx context.Context, hostID, packageID, version string) (int32, error)
}

// BuilderClient is the interface for calling the builder gRPC service.
type BuilderClient interface {
	StartBuild(ctx context.Context, req *grpc.BuilderStartBuildRequest) (*grpc.BuilderStartBuildResponse, error)
	SignPackage(ctx context.Context, req *grpc.BuilderSignRequest) (*grpc.BuilderSignResponse, error)
	GetPackageBuildStatus(ctx context.Context, packageID, version, buildID string) (*pb.GetPackageBuildStatusResponse, error)
	ReviewBuild(ctx context.Context, buildID, packageID, version string, approved bool, comment, reviewer string) (*pb.ReviewBuildResponse, error)
}

type DetectionPackageCommand struct {
	CommandID    string `json:"command_id"`
	Action       string `json:"action"`
	PackageID    string `json:"package_id"`
	Version      string `json:"version"`
	PackageURL   string `json:"package_url"`
	SignatureURL string `json:"signature_url"`
	PackageSize  int64  `json:"package_size"`
	Rollback     bool   `json:"rollback"`
}

func NewDetectionPackageService(repo *repository.DetectionPackageRepo, db *gorm.DB, serverClient GRPCServerClient, builderClient BuilderClient, artifactDownloadBaseURL string) *DetectionPackageService {
	return &DetectionPackageService{
		repo:                  repo,
		db:                    db,
		serverClient:          serverClient,
		builderClient:         builderClient,
		artifactDownloadBaseURL: artifactDownloadBaseURL,
	}
}

// objectKeyToURL converts a MinIO object key to a full HTTP download URL.
// If the key already starts with "http://" or "https://", it is returned as-is.
func (s *DetectionPackageService) objectKeyToURL(objectKey string) string {
	if objectKey == "" {
		return ""
	}
	if strings.HasPrefix(objectKey, "http://") || strings.HasPrefix(objectKey, "https://") {
		return objectKey
	}
	base := strings.TrimRight(s.artifactDownloadBaseURL, "/")
	return base + "/" + strings.TrimLeft(objectKey, "/")
}

type CreateDraftRequest struct {
	PackageID       string                 `json:"package_id"`
	TargetVersion   string                 `json:"target_version" binding:"required"`
	Title           string                 `json:"title" binding:"required"`
	Description     string                 `json:"description"`
	CVEIDs          []string               `json:"cve_ids"`
	HookPlanYAML    string                 `json:"hook_plan_yaml"`
	EBPFSource      string                 `json:"ebpf_source"`
	SigmaRulesYAML  string                 `json:"sigma_rules_yaml"`
	CorrelationYAML string                 `json:"correlation_yaml"`
	BuildParams     map[string]interface{} `json:"build_params"`
}

type UpdateDraftRequest struct {
	Title           *string `json:"title"`
	Description     *string `json:"description"`
	TargetVersion   *string `json:"target_version"`
	HookPlanYAML    *string `json:"hook_plan_yaml"`
	EBPFSource      *string `json:"ebpf_source"`
	SigmaRulesYAML  *string `json:"sigma_rules_yaml"`
	CorrelationYAML *string `json:"correlation_yaml"`
}

func (s *DetectionPackageService) CreateDraft(ctx context.Context, req CreateDraftRequest, operator string) (*model.DetectionPackageDraft, error) {
	cveIDs, _ := json.Marshal(req.CVEIDs)
	buildParams, _ := json.Marshal(req.BuildParams)
	if buildParams == nil {
		buildParams = []byte("{}")
	}

	draft := &model.DetectionPackageDraft{
		ID:              uuid.New(),
		PackageID:       req.PackageID,
		TargetVersion:   req.TargetVersion,
		Title:           req.Title,
		Description:     req.Description,
		CVEIDs:          datatypes.JSON(cveIDs),
		HookPlanYAML:    req.HookPlanYAML,
		EBPFSource:      req.EBPFSource,
		SigmaRulesYAML:  req.SigmaRulesYAML,
		CorrelationYAML: req.CorrelationYAML,
		BuildParams:     datatypes.JSON(buildParams),
		Status:          "draft",
		CreatedBy:       operator,
		UpdatedBy:       operator,
	}

	// Auto-generate UUID package_id if not provided
	if draft.PackageID == "" {
		draft.PackageID = uuid.New().String()
	}

	if err := s.repo.CreateDraft(draft); err != nil {
		return nil, fmt.Errorf("create draft: %w", err)
	}

	s.recordOperation(draft.PackageID, draft.TargetVersion, "create_draft", operator, nil, true, "")
	return draft, nil
}

func (s *DetectionPackageService) UpdateDraft(ctx context.Context, draftID uuid.UUID, req UpdateDraftRequest, operator string) (*model.DetectionPackageDraft, error) {
	draft, err := s.repo.GetDraftByID(draftID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get draft: %w: %w", ErrNotFound, err)
		}
		return nil, fmt.Errorf("get draft: %w", err)
	}

	if req.Title != nil {
		draft.Title = *req.Title
	}
	if req.Description != nil {
		draft.Description = *req.Description
	}
	if req.TargetVersion != nil {
		draft.TargetVersion = *req.TargetVersion
	}
	if req.HookPlanYAML != nil {
		draft.HookPlanYAML = *req.HookPlanYAML
	}
	if req.EBPFSource != nil {
		draft.EBPFSource = *req.EBPFSource
	}
	if req.SigmaRulesYAML != nil {
		draft.SigmaRulesYAML = *req.SigmaRulesYAML
	}
	if req.CorrelationYAML != nil {
		draft.CorrelationYAML = *req.CorrelationYAML
	}
	draft.UpdatedBy = operator

	if err := s.repo.UpdateDraft(draft); err != nil {
		return nil, fmt.Errorf("update draft: %w", err)
	}

	s.recordOperation(draft.PackageID, draft.TargetVersion, "update_draft", operator, nil, true, "")
	return draft, nil
}

func (s *DetectionPackageService) GetDraft(ctx context.Context, packageID string) (*model.DetectionPackageDraft, error) {
	return s.repo.GetDraftByPackageID(packageID)
}

func (s *DetectionPackageService) DeleteDraftByPackageID(ctx context.Context, packageID string, operator string) error {
	if err := s.repo.DeleteDraftByPackageID(packageID); err != nil {
		return fmt.Errorf("delete draft: %w", err)
	}
	s.recordOperation(packageID, "", "delete_draft", operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) StartBuild(ctx context.Context, packageID string, operator string) (*model.DetectionPackageBuild, error) {
	draft, err := s.repo.GetDraftByPackageID(packageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get draft: %w: %w", ErrNotFound, err)
		}
		return nil, fmt.Errorf("get draft: %w", err)
	}

	build := &model.DetectionPackageBuild{
		ID:           uuid.New(),
		DraftID:      &draft.ID,
		PackageID:    draft.PackageID,
		Version:      draft.TargetVersion,
		Status:       "pending",
		BuilderImage: "aegis-agent-builder-ubi8:5.8.0",
		CreatedBy:    operator,
	}

	if err := s.repo.CreateBuild(build); err != nil {
		return nil, fmt.Errorf("create build: %w", err)
	}

	draft.Status = "build_pending"
	draft.LastBuildID = &build.ID
	s.repo.UpdateDraft(draft)

	// Call builder service if available
	if s.builderClient != nil {
		go s.executeBuild(context.Background(), build, draft, operator)
	}

	s.recordOperation(packageID, draft.TargetVersion, "build", operator, nil, true, "")
	return build, nil
}

func (s *DetectionPackageService) executeBuild(ctx context.Context, build *model.DetectionPackageBuild, draft *model.DetectionPackageDraft, operator string) {
	now := time.Now()
	build.StartedAt = &now
	build.Status = "running"
	s.repo.UpdateBuild(build)
	draft.Status = "build_running"
	s.repo.UpdateDraft(draft)

	result, err := s.builderClient.StartBuild(ctx, &grpc.BuilderStartBuildRequest{
		BuildID:         build.ID.String(),
		PackageID:       build.PackageID,
		Version:         build.Version,
		Title:           draft.Title,
		CVEIDs:          jsonStringSlice(draft.CVEIDs),
		Operator:        operator,
		TargetArch:      "amd64",
		HookPlanYAML:    draft.HookPlanYAML,
		EBPFSource:      draft.EBPFSource,
		SigmaRulesYAML:  draft.SigmaRulesYAML,
		CorrelationYAML: draft.CorrelationYAML,
		// The V5.8 plugin manifest, including event_schema, lives in HookPlanYAML.
		// Ensure required metadata fields are present.
		PackageMetadataJSON: ensurePackageMetadata(draft.HookPlanYAML, build.PackageID, build.Version),
	})

	finished := time.Now()
	build.FinishedAt = &finished
	build.DurationMs = finished.Sub(now).Milliseconds()

	if err != nil {
		build.Status = "failed"
		build.ErrorMessage = err.Error()
		s.repo.UpdateBuild(build)
		draft.Status = "build_failed"
		s.repo.UpdateDraft(draft)
		return
	}

	build.Status = result.Status
	build.ErrorMessage = result.ErrorMessage
	build.BuilderDigest = result.BuilderImageDigest
	build.ClangVersion = result.ClangVersion
	build.BuildLogObjectKey = result.BuildLogObjectKey
	build.UnsignedPackageObjectKey = result.UnsignedPackageObjectKey
	// Strip sha256: prefix if present (column is varchar(64))
	sha256 := result.UnsignedPackageSHA256
	if len(sha256) > 7 && sha256[:7] == "sha256:" {
		sha256 = sha256[7:]
	}
	build.UnsignedPackageSHA256 = sha256
	build.UnsignedPackageSize = result.UnsignedPackageSize

	// Set hook_summary from builder response
	if len(result.HookSummary) > 0 {
		hookSummaryJSON, _ := json.Marshal(result.HookSummary)
		build.HookSummary = datatypes.JSON(hookSummaryJSON)
	}

	// Set event_schema from builder response
	if result.EventSchemaJSON != "" {
		build.EventSchema = datatypes.JSON(result.EventSchemaJSON)
	}

	// Set build_log (tail) from builder response
	if result.BuildLogTail != "" {
		build.BuildLog = result.BuildLogTail
	}

	// Set artifact_summary from builder response
	if len(result.Artifacts) > 0 {
		artifactJSON, _ := json.Marshal(result.Artifacts)
		build.ArtifactSummary = datatypes.JSON(artifactJSON)
	}

	fmt.Printf("[DEBUG] Calling UpdateBuild for build %s, status=%s, hook_summary_len=%d\n", build.ID, build.Status, len(build.HookSummary))
	if err := s.repo.UpdateBuild(build); err != nil {
		fmt.Printf("[ERROR] UpdateBuild failed: %v\n", err)
	} else {
		fmt.Printf("[DEBUG] UpdateBuild succeeded for build %s\n", build.ID)
	}

	switch build.Status {
	case "awaiting_review":
		draft.Status = model.StatusAwaitingReview
	case "success":
		draft.Status = "built"
	case "pending", "running":
		draft.Status = "build_running"
	default:
		draft.Status = "build_failed"
	}
	s.repo.UpdateDraft(draft)
}

func jsonStringSlice(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

func (s *DetectionPackageService) GetBuild(ctx context.Context, buildID uuid.UUID) (*model.DetectionPackageBuild, error) {
	return s.repo.GetBuild(buildID)
}

func (s *DetectionPackageService) GetLatestBuild(ctx context.Context, packageID string) (*model.DetectionPackageBuild, error) {
	return s.repo.GetLatestBuild(packageID)
}

func (s *DetectionPackageService) SignPackage(ctx context.Context, packageID string, operator string) (*model.DetectionPackage, error) {
	build, err := s.repo.GetLatestBuild(packageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get build: %w: %w", ErrNotFound, err)
		}
		return nil, fmt.Errorf("get build: %w", err)
	}
	if build.Status != "success" {
		return nil, fmt.Errorf("build not successful, cannot sign: %w", ErrInvalidState)
	}

	var signResult *grpc.BuilderSignResponse
	if s.builderClient != nil {
		signResult, err = s.builderClient.SignPackage(ctx, &grpc.BuilderSignRequest{
			BuildID:   build.ID.String(),
			PackageID: build.PackageID,
			Version:   build.Version,
			Operator:  operator,
			Confirm:   true,
		})
		if err != nil {
			return nil, fmt.Errorf("builder sign: %w", err)
		}
		if !signResult.Success {
			return nil, fmt.Errorf("builder sign failed: %s", signResult.Message)
		}
	}

	title := build.PackageID
	description := ""
	cveIDs := datatypes.JSON([]byte("[]"))
	if build.DraftID != nil {
		if draft, draftErr := s.repo.GetDraftByID(*build.DraftID); draftErr == nil {
			title = draft.Title
			description = draft.Description
			cveIDs = draft.CVEIDs
		}
	}

	pkg := &model.DetectionPackage{
		ID:            uuid.New(),
		PackageID:     build.PackageID,
		Version:       build.Version,
		Title:         title,
		Description:   description,
		CVEIDs:        cveIDs,
		Status:        "signed",
		BuildID:       &build.ID,
		BuilderImage:  build.BuilderImage,
		BuilderDigest: build.BuilderDigest,
		HookSummary:   build.HookSummary,
		EventSchema:   build.EventSchema,
		SignedBy:      operator,
		SignedAt:      timePtr(time.Now()),
	}

	if signResult != nil {
		pkg.PackageObjectKey = signResult.PackageObjectKey
		pkg.SignatureObjectKey = signResult.SignatureObjectKey
		// Strip sha256: prefix if present (column is varchar(64))
		sha256 := signResult.PackageSHA256
		if len(sha256) > 7 && sha256[:7] == "sha256:" {
			sha256 = sha256[7:]
		}
		pkg.PackageSHA256 = sha256
		pkg.PackageSize = signResult.PackageSize
	}

	if err := s.repo.CreatePackage(pkg); err != nil {
		return nil, fmt.Errorf("create package: %w", err)
	}

	s.recordOperation(packageID, build.Version, "sign", operator, nil, true, "")
	return pkg, nil
}

func (s *DetectionPackageService) EnablePackage(ctx context.Context, packageID string, operator string) error {
	pkg, err := s.repo.GetLatestPackage(packageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get package: %w: %w", ErrNotFound, err)
		}
		return fmt.Errorf("get package: %w", err)
	}
	if pkg.Status != "signed" && pkg.Status != "disabled" {
		return fmt.Errorf("package not signed or disabled, cannot enable: %w", ErrInvalidState)
	}

	currentEnabled, err := s.repo.GetEnabledPackage(packageID)
	if err == nil && currentEnabled != nil {
		currentVer, cerr1 := goversion.NewVersion(currentEnabled.Version)
		newVer, cerr2 := goversion.NewVersion(pkg.Version)
		if cerr1 == nil && cerr2 == nil && newVer.LessThan(currentVer) {
			return fmt.Errorf("version downgrade not allowed: current=%s, requested=%s (use rollback instead)", currentEnabled.Version, pkg.Version)
		}
	}

	if err := s.repo.DisableOtherVersions(packageID, pkg.Version); err != nil {
		return fmt.Errorf("disable other versions: %w", err)
	}

	pkg.Status = "enabled"
	now := time.Now()
	pkg.EnabledAt = &now
	if err := s.repo.UpdatePackage(pkg); err != nil {
		return fmt.Errorf("update package: %w", err)
	}

	// Call server gRPC to install on agents
	if s.serverClient != nil {
		affected, err := s.serverClient.InstallDetectionPackageFromService(ctx, "", &DetectionPackageCommand{
			CommandID:    uuid.New().String(),
			Action:       "install",
			PackageID:    pkg.PackageID,
			Version:      pkg.Version,
			PackageURL:   s.objectKeyToURL(pkg.PackageObjectKey),
			SignatureURL: s.objectKeyToURL(pkg.SignatureObjectKey),
			PackageSize:  pkg.PackageSize,
		})
		if err != nil {
			// Roll back status to signed
			pkg.Status = "signed"
			pkg.EnabledAt = nil
			s.repo.UpdatePackage(pkg)
			s.recordOperation(packageID, pkg.Version, "enable", operator, nil, false, err.Error())
			return fmt.Errorf("install on agents failed (affected=%d): %w", affected, err)
		}
	}

	// Sync sigma rules from draft to sigma_rules table
	if pkg.BuildID != nil {
		if build, berr := s.repo.GetBuild(*pkg.BuildID); berr == nil && build.DraftID != nil {
			if draft, derr := s.repo.GetDraftByID(*build.DraftID); derr == nil && draft.SigmaRulesYAML != "" {
				s.syncSigmaRules(draft.SigmaRulesYAML, pkg.PackageID, pkg.Version)
			}
		}
	}

	s.recordOperation(packageID, pkg.Version, "enable", operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) syncSigmaRules(sigmaYAML, packageID, version string) {
	// Parse sigma rules from YAML and insert into sigma_rules table
	rules := parseSigmaRulesFromYAML(sigmaYAML)
	for _, rule := range rules {
		sr := &model.SigmaRule{
			RuleID:      rule["id"],
			Title:       rule["title"],
			Description: rule["description"],
			Content:     sigmaYAML,
			Status:      "active",
			Severity:    rule["level"],
			GeneratedBy: "detection_package",
			Source:      "detection_package",
			Version:     version,
		}
		// Use ON CONFLICT to avoid duplicate key errors
		s.db.Where("rule_id = ?", sr.RuleID).Assign(sr).FirstOrCreate(sr)
	}
}

func parseSigmaRulesFromYAML(yamlContent string) []map[string]string {
	var rules []map[string]string
	// Simple YAML parsing for sigma rules
	decoder := yaml.NewDecoder(strings.NewReader(yamlContent))
	for {
		var rule map[string]interface{}
		if err := decoder.Decode(&rule); err != nil {
			break
		}
		r := make(map[string]string)
		if id, ok := rule["id"].(string); ok {
			r["id"] = id
		}
		if title, ok := rule["title"].(string); ok {
			r["title"] = title
		}
		if desc, ok := rule["description"].(string); ok {
			r["description"] = desc
		}
		if level, ok := rule["level"].(string); ok {
			r["level"] = level
		}
		if r["id"] != "" {
			rules = append(rules, r)
		}
	}
	return rules
}

func (s *DetectionPackageService) DisablePackage(ctx context.Context, packageID string, operator string) error {
	pkg, err := s.repo.GetLatestPackage(packageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get package: %w: %w", ErrNotFound, err)
		}
		return fmt.Errorf("get package: %w", err)
	}

	// Uninstall from agents first
	if s.serverClient != nil {
		affected, err := s.serverClient.UninstallDetectionPackage(ctx, "", pkg.PackageID, pkg.Version)
		if err != nil {
			fmt.Printf("Warning: failed to uninstall from agents: %v, affected: %d\n", err, affected)
		}
	}

	pkg.Status = "disabled"
	now := time.Now()
	pkg.DisabledAt = &now
	if err := s.repo.UpdatePackage(pkg); err != nil {
		return fmt.Errorf("update package: %w", err)
	}

	s.recordOperation(packageID, pkg.Version, "disable", operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) RollbackPackage(ctx context.Context, packageID, targetVersion, operator string) error {
	currentPkg, err := s.repo.GetEnabledPackage(packageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no enabled version found for package %s: %w: %w", packageID, ErrNotFound, err)
		}
		return fmt.Errorf("no enabled version found for package %s: %w", packageID, err)
	}

	targetPkg, err := s.repo.GetPackageByVersion(packageID, targetVersion)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("target version %s not found: %w: %w", targetVersion, ErrNotFound, err)
		}
		return fmt.Errorf("target version %s not found: %w", targetVersion, err)
	}
	if targetPkg.Status != "signed" && targetPkg.Status != "disabled" {
		return fmt.Errorf("target version %s status is %s, must be signed or disabled: %w", targetVersion, targetPkg.Status, ErrInvalidState)
	}

	if targetPkg.Version == currentPkg.Version {
		return fmt.Errorf("target version %s is the same as current enabled version: %w", targetVersion, ErrInvalidState)
	}

	currentPkg.Status = "disabled"
	now := time.Now()
	currentPkg.DisabledAt = &now
	if err := s.repo.UpdatePackage(currentPkg); err != nil {
		return fmt.Errorf("disable current version: %w", err)
	}

	if s.serverClient != nil {
		s.serverClient.UninstallDetectionPackage(ctx, "", currentPkg.PackageID, currentPkg.Version)
	}

	targetPkg.Status = "enabled"
	targetPkg.EnabledAt = &now
	if err := s.repo.UpdatePackage(targetPkg); err != nil {
		return fmt.Errorf("enable target version: %w", err)
	}

	if s.serverClient != nil {
		_, err := s.serverClient.InstallDetectionPackageFromService(ctx, "", &DetectionPackageCommand{
			CommandID:    uuid.New().String(),
			Action:       "rollback",
			PackageID:    targetPkg.PackageID,
			Version:      targetPkg.Version,
			PackageURL:   s.objectKeyToURL(targetPkg.PackageObjectKey),
			SignatureURL: s.objectKeyToURL(targetPkg.SignatureObjectKey),
			PackageSize:  targetPkg.PackageSize,
			Rollback:     true,
		})
		if err != nil {
			s.recordOperation(packageID, targetVersion, "rollback", operator, nil, false, err.Error())
			return fmt.Errorf("install rollback version on agents failed: %w", err)
		}
	}

	s.recordOperation(packageID, targetVersion, "rollback", operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) UninstallPackage(ctx context.Context, packageID string, operator string) error {
	pkg, err := s.repo.GetLatestPackage(packageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get package: %w: %w", ErrNotFound, err)
		}
		return fmt.Errorf("get package: %w", err)
	}

	// Uninstall from agents
	if s.serverClient != nil {
		affected, err := s.serverClient.UninstallDetectionPackage(ctx, "", pkg.PackageID, pkg.Version)
		if err != nil {
			fmt.Printf("Warning: failed to uninstall from agents: %v, affected: %d\n", err, affected)
		}
	}

	pkg.Status = "uninstalled"
	if err := s.repo.UpdatePackage(pkg); err != nil {
		return fmt.Errorf("update package: %w", err)
	}

	s.recordOperation(packageID, pkg.Version, "uninstall", operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) GetPackage(ctx context.Context, packageID string) (*model.DetectionPackage, error) {
	pkg, err := s.repo.GetLatestPackage(packageID)
	if err == nil {
		return pkg, nil
	}
	// Fall back to draft table (AI-generated drafts only exist in drafts table)
	draft, draftErr := s.repo.GetDraftByPackageID(packageID)
	if draftErr != nil {
		return nil, err // return original error
	}
	return &model.DetectionPackage{
		ID:          draft.ID,
		PackageID:   draft.PackageID,
		Version:     draft.TargetVersion,
		Title:       draft.Title,
		Description: draft.Description,
		CVEIDs:      draft.CVEIDs,
		Status:      draft.Status,
		CreatedAt:   draft.CreatedAt,
		UpdatedAt:   draft.UpdatedAt,
	}, nil
}

func (s *DetectionPackageService) ListPackages(ctx context.Context, page, pageSize int, status, search string) ([]model.DetectionPackage, int64, error) {
	return s.repo.ListPackagesUnified(page, pageSize, status, search)
}

func (s *DetectionPackageService) ListHostStatus(ctx context.Context, packageID, version string, page, pageSize int) ([]model.DetectionPackageHostStatus, int64, error) {
	return s.repo.ListHostStatus(packageID, version, page, pageSize)
}

func (s *DetectionPackageService) GetAllowlist(ctx context.Context) (*model.EBPFHookAllowlistConfig, error) {
	return s.repo.GetActiveAllowlist()
}

func (s *DetectionPackageService) UpdateAllowlist(ctx context.Context, configJSON datatypes.JSON, description, operator string) (*model.EBPFHookAllowlistConfig, error) {
	config := &model.EBPFHookAllowlistConfig{
		ID:          uuid.New(),
		ConfigJSON:  configJSON,
		Description: description,
		UpdatedBy:   operator,
		ActivatedAt: timePtr(time.Now()),
	}

	if err := s.repo.CreateAllowlist(config); err != nil {
		return nil, fmt.Errorf("create allowlist: %w", err)
	}

	// Sync to agents
	var syncErr error
	if s.serverClient != nil {
		configJSONStr := string(config.ConfigJSON)
		configs := []*pb.AgentConfig{
			{ConfigType: "dynamic_ebpf_hook_allowlist", ConfigJson: configJSONStr},
		}
		affected, err := s.serverClient.SyncAgentConfig(ctx, "", configs)
		if err != nil {
			syncErr = fmt.Errorf("sync to agents failed (affected=%d): %w", affected, err)
			s.recordOperation("", "", "allowlist_update", operator, nil, false, syncErr.Error())
		} else {
			s.recordOperation("", "", "allowlist_update", operator, nil, true, "")
		}
	} else {
		s.recordOperation("", "", "allowlist_update", operator, nil, true, "")
	}

	if syncErr != nil {
		return config, syncErr
	}
	return config, nil
}

type HostStatusReport struct {
	HostID         string   `json:"host_id"`
	Hostname       string   `json:"hostname"`
	PackageID      string   `json:"package_id"`
	Version        string   `json:"version"`
	Status         string   `json:"status"`
	ActiveArtifact string   `json:"active_artifact"`
	LoadedHooks    []string `json:"loaded_hooks"`
	ErrorMessage   string   `json:"error_message"`
}

func (s *DetectionPackageService) ReportHostStatus(ctx context.Context, report HostStatusReport) error {
	hostID, err := uuid.Parse(report.HostID)
	if err != nil {
		return fmt.Errorf("invalid host_id: %w: %w", ErrInvalidState, err)
	}

	loadedHooks, _ := json.Marshal(report.LoadedHooks)
	if loadedHooks == nil {
		loadedHooks = []byte("[]")
	}

	now := time.Now()
	status := &model.DetectionPackageHostStatus{
		PackageID:      report.PackageID,
		Version:        report.Version,
		HostID:         hostID,
		Hostname:       report.Hostname,
		Status:         report.Status,
		ActiveArtifact: report.ActiveArtifact,
		LoadedHooks:    datatypes.JSON(loadedHooks),
		ErrorMessage:   report.ErrorMessage,
		LastReportedAt: &now,
	}

	return s.repo.UpsertHostStatus(status)
}

func (s *DetectionPackageService) recordOperation(packageID, version, operation, operator string, req interface{}, success bool, errMsg string) {
	reqJSON, _ := json.Marshal(req)
	if reqJSON == nil {
		reqJSON = []byte("{}")
	}
	op := &model.DetectionPackageOperation{
		ID:           uuid.New(),
		PackageID:    packageID,
		Version:      version,
		Operation:    operation,
		Operator:     operator,
		RequestJSON:  datatypes.JSON(reqJSON),
		ResultJSON:   datatypes.JSON("{}"),
		Success:      success,
		ErrorMessage: errMsg,
	}
	s.repo.CreateOperation(op)
}

func (s *DetectionPackageService) ReviewBuild(ctx context.Context, buildID uuid.UUID, approved bool, comment, operator string) error {
	build, err := s.repo.GetBuild(buildID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get build: %w: %w", ErrNotFound, err)
		}
		return fmt.Errorf("get build: %w", err)
	}
	if build.Status != "awaiting_review" {
		return fmt.Errorf("build status is %s, not awaiting_review: %w", build.Status, ErrInvalidState)
	}
	newStatus := "review_rejected"
	if approved {
		newStatus = "success"
	}
	if s.builderClient != nil {
		resp, err := s.builderClient.ReviewBuild(ctx, build.ID.String(), build.PackageID, build.Version, approved, comment, operator)
		if err != nil {
			s.recordOperation(build.PackageID, build.Version, model.OperationTypeReview, operator, nil, false, err.Error())
			return fmt.Errorf("builder review: %w", err)
		}
		if resp != nil && !resp.Success {
			msg := resp.Message
			if msg == "" {
				msg = "builder review rejected"
			}
			s.recordOperation(build.PackageID, build.Version, model.OperationTypeReview, operator, nil, false, msg)
			return errors.New(msg)
		}
		if resp != nil && (resp.NewStatus == "success" || resp.NewStatus == "review_rejected") {
			newStatus = resp.NewStatus
		}
	}
	build.Status = newStatus
	build.ReviewedBy = &operator
	now := time.Now()
	build.ReviewedAt = &now
	build.ReviewComment = &comment
	if err := s.repo.UpdateBuild(build); err != nil {
		return fmt.Errorf("update build: %w", err)
	}
	if build.DraftID != nil {
		if draft, err := s.repo.GetDraftByID(*build.DraftID); err == nil {
			if build.Status == "success" {
				draft.Status = "built"
			} else {
				draft.Status = model.StatusReviewRejected
			}
			draft.UpdatedBy = operator
			_ = s.repo.UpdateDraft(draft)
		}
	}

	// Sync event_schema from build to the associated package when approved
	if approved && build.EventSchema != nil && string(build.EventSchema) != "{}" {
		if pkg, err := s.repo.GetPackage(build.PackageID, build.Version); err == nil && pkg != nil {
			pkg.EventSchema = build.EventSchema
			_ = s.repo.UpdatePackage(pkg)
		}
	}

	s.recordOperation(build.PackageID, build.Version, model.OperationTypeReview, operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) ListPackageAlerts(ctx context.Context, packageID string, page, pageSize int) ([]model.RuntimeEvent, int64, error) {
	return s.repo.ListAlertsByPackageID(packageID, page, pageSize)
}

func (s *DetectionPackageService) GetBuildLogURL(ctx context.Context, buildID uuid.UUID) (string, error) {
	build, err := s.repo.GetBuild(buildID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("get build: %w: %w", ErrNotFound, err)
		}
		return "", fmt.Errorf("get build: %w", err)
	}
	if build.BuildLogObjectKey == "" {
		return "", fmt.Errorf("build log not available: %w", ErrNotFound)
	}
	return build.BuildLogObjectKey, nil
}

func (s *DetectionPackageService) ListAllowlistHistory(ctx context.Context, page, pageSize int) ([]model.EBPFHookAllowlistConfig, int64, error) {
	return s.repo.ListAllowlistHistory(page, pageSize)
}

func (s *DetectionPackageService) DeletePackage(ctx context.Context, packageID, operator string) error {
	// Try to get the published package
	pkg, err := s.repo.GetLatestPackage(packageID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("get package: %w", err)
	}

	// If published package exists, check status
	if pkg != nil && pkg.ID != uuid.Nil {
		if pkg.Status == "enabled" || pkg.Status == "active" {
			return fmt.Errorf("cannot delete package in '%s' status, disable it first: %w", pkg.Status, ErrInvalidState)
		}
		// Delete the published package
		if err := s.repo.DeletePackage(packageID); err != nil {
			return fmt.Errorf("delete package: %w", err)
		}
		s.recordOperation(packageID, pkg.Version, "delete", operator, nil, true, "")
	}

	// Delete drafts (always try, even if published package exists)
	_ = s.repo.DeleteDraftByPackageID(packageID)

	return nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}


// ensurePackageMetadata ensures the package metadata YAML contains the required
// schema_version, package_id, and version fields. If these fields are missing
// (e.g. from older AI-generated drafts), they are injected by appending lines
// to avoid reformatting the existing YAML content.
func ensurePackageMetadata(yamlStr, packageID, version string) string {
	if yamlStr == "" {
		return buildMinimalMetadata(packageID, version)
	}

	var meta map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &meta); err != nil {
		return buildMinimalMetadata(packageID, version)
	}

	changed := false

	// schema_version: inject if missing
	if _, ok := meta["schema_version"]; !ok {
		meta["schema_version"] = "aegis.ebpf_plugin.v1"
		changed = true
	}

	// package_id: always overwrite with the correct request value
	if meta["package_id"] != packageID {
		meta["package_id"] = packageID
		changed = true
	}

	// version: always overwrite with the correct request value
	if meta["version"] != version {
		meta["version"] = version
		changed = true
	}

	// event_schema: generate from hooks if missing
	if _, ok := meta["event_schema"]; !ok {
		if es := generateEventSchemaFromHooks(meta); es != nil {
			meta["event_schema"] = es
			changed = true
		}
	}

	// hooks: ensure each hook has a "program" field (eBPF function name).
	// Convention: program = "trace_" + hook name if not explicitly set.
	if hooksRaw, ok := meta["hooks"]; ok {
		if hooksSlice, ok := hooksRaw.([]interface{}); ok {
			for _, hookRaw := range hooksSlice {
				if hook, ok := hookRaw.(map[string]interface{}); ok {
					if _, hasProgram := hook["program"]; !hasProgram {
						if name, ok := hook["name"].(string); ok && name != "" {
							hook["program"] = "trace_" + name
							changed = true
						}
					}
				}
			}
		}
	}

	if !changed {
		return yamlStr
	}

	out, err := yaml.Marshal(meta)
	if err != nil {
		return yamlStr
	}
	return string(out)
}


func buildMinimalMetadata(packageID, version string) string {
	meta := map[string]interface{}{
		"schema_version": "aegis.ebpf_plugin.v1",
		"package_id":     packageID,
		"version":        version,
	}
	out, _ := yaml.Marshal(meta)
	return string(out)
}

// generateEventSchemaFromHooks builds a basic event_schema from the hooks
// defined in the plugin manifest. Each hook gets an event entry with a
// sequential ID starting at 1001 and standard eBPF event metadata fields
// (timestamp, pid, tid, comm, uid). If the manifest has no hooks, nil is
// returned so the caller can skip injection.
func generateEventSchemaFromHooks(meta map[string]interface{}) map[string]interface{} {
	hooksRaw, ok := meta["hooks"]
	if !ok {
		return nil
	}
	hooksSlice, ok := hooksRaw.([]interface{})
	if !ok || len(hooksSlice) == 0 {
		return nil
	}

	events := make(map[string]interface{}, len(hooksSlice))
	for i, hookRaw := range hooksSlice {
		hook, ok := hookRaw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := hook["name"].(string)
		if name == "" {
			name = fmt.Sprintf("event_%d", 1001+i)
		}
		eventID := fmt.Sprintf("%d", 1001+i)
		events[eventID] = map[string]interface{}{
			"name": name,
			"fields": map[string]interface{}{
				"1": map[string]interface{}{"name": "timestamp", "type": "uint64"},
				"2": map[string]interface{}{"name": "pid", "type": "uint32"},
				"3": map[string]interface{}{"name": "tid", "type": "uint32"},
				"4": map[string]interface{}{"name": "comm", "type": "string"},
				"5": map[string]interface{}{"name": "uid", "type": "uint32"},
			},
		}
	}
	if len(events) == 0 {
		return nil
	}
	return map[string]interface{}{"events": events}
}

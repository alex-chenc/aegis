package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	goversion "github.com/hashicorp/go-version"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type DetectionPackageService struct {
	repo          *repository.DetectionPackageRepo
	db            *gorm.DB
	serverClient  GRPCServerClient
	builderClient BuilderClient
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
	CommandID    string
	Action       string
	PackageID    string
	Version      string
	PackageURL   string
	SignatureURL string
	PackageSize  int64
	Rollback     bool
}

func NewDetectionPackageService(repo *repository.DetectionPackageRepo, db *gorm.DB, serverClient GRPCServerClient, builderClient BuilderClient) *DetectionPackageService {
	return &DetectionPackageService{
		repo:          repo,
		db:            db,
		serverClient:  serverClient,
		builderClient: builderClient,
	}
}

type CreateDraftRequest struct {
	PackageID       string                 `json:"package_id" binding:"required"`
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

	if err := s.repo.CreateDraft(draft); err != nil {
		return nil, fmt.Errorf("create draft: %w", err)
	}

	s.recordOperation(draft.PackageID, draft.TargetVersion, "create_draft", operator, nil, true, "")
	return draft, nil
}

func (s *DetectionPackageService) UpdateDraft(ctx context.Context, draftID uuid.UUID, req UpdateDraftRequest, operator string) (*model.DetectionPackageDraft, error) {
	draft, err := s.repo.GetDraftByID(draftID)
	if err != nil {
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

	result, err := s.builderClient.StartBuild(ctx, &grpc.BuilderStartBuildRequest{
		BuildID:         build.ID.String(),
		PackageID:       build.PackageID,
		Version:         build.Version,
		Title:           draft.Title,
		Operator:        operator,
		TargetArch:      "amd64",
		HookPlanYAML:    draft.HookPlanYAML,
		EBPFSource:      draft.EBPFSource,
		SigmaRulesYAML:  draft.SigmaRulesYAML,
		CorrelationYAML: draft.CorrelationYAML,
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
	build.UnsignedPackageSHA256 = result.UnsignedPackageSHA256
	build.UnsignedPackageSize = result.UnsignedPackageSize
	s.repo.UpdateBuild(build)

	if build.Status == "success" || build.Status == "awaiting_review" {
		draft.Status = "build_success"
	} else {
		draft.Status = "build_failed"
	}
	s.repo.UpdateDraft(draft)
}

func (s *DetectionPackageService) GetBuild(ctx context.Context, buildID uuid.UUID) (*model.DetectionPackageBuild, error) {
	return s.repo.GetBuild(buildID)
}

func (s *DetectionPackageService) SignPackage(ctx context.Context, packageID string, operator string) (*model.DetectionPackage, error) {
	build, err := s.repo.GetLatestBuild(packageID)
	if err != nil {
		return nil, fmt.Errorf("get build: %w", err)
	}
	if build.Status != "success" {
		return nil, errors.New("build not successful, cannot sign")
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

	pkg := &model.DetectionPackage{
		ID:           uuid.New(),
		PackageID:    build.PackageID,
		Version:      build.Version,
		Title:        build.PackageID,
		Status:       "signed",
		BuildID:      &build.ID,
		BuilderImage: build.BuilderImage,
		HookSummary:  build.HookSummary,
		EventSchema:  build.EventSchema,
		SignedBy:     operator,
		SignedAt:     timePtr(time.Now()),
	}

	if signResult != nil {
		pkg.PackageObjectKey = signResult.PackageObjectKey
		pkg.SignatureObjectKey = signResult.SignatureObjectKey
		pkg.PackageSHA256 = signResult.PackageSHA256
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
		return fmt.Errorf("get package: %w", err)
	}
	if pkg.Status != "signed" && pkg.Status != "disabled" {
		return errors.New("package not signed or disabled, cannot enable")
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
			PackageURL:   pkg.PackageObjectKey,
			SignatureURL: pkg.SignatureObjectKey,
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

	s.recordOperation(packageID, pkg.Version, "enable", operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) DisablePackage(ctx context.Context, packageID string, operator string) error {
	pkg, err := s.repo.GetLatestPackage(packageID)
	if err != nil {
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
		return fmt.Errorf("no enabled version found for package %s: %w", packageID, err)
	}

	targetPkg, err := s.repo.GetPackageByVersion(packageID, targetVersion)
	if err != nil {
		return fmt.Errorf("target version %s not found: %w", targetVersion, err)
	}
	if targetPkg.Status != "signed" && targetPkg.Status != "disabled" {
		return fmt.Errorf("target version %s status is %s, must be signed or disabled", targetVersion, targetPkg.Status)
	}

	if targetPkg.Version == currentPkg.Version {
		return fmt.Errorf("target version %s is the same as current enabled version", targetVersion)
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
			PackageURL:   targetPkg.PackageObjectKey,
			SignatureURL: targetPkg.SignatureObjectKey,
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
	return s.repo.GetLatestPackage(packageID)
}

func (s *DetectionPackageService) ListPackages(ctx context.Context, page, pageSize int, status, search string) ([]model.DetectionPackage, int64, error) {
	packages, pkgTotal, err := s.repo.ListPackages(page, pageSize, status, search)
	if err != nil {
		return nil, 0, err
	}

	if status != "" && status != "draft" {
		return packages, pkgTotal, nil
	}

	drafts, draftTotal, err := s.repo.ListDrafts(page, pageSize, status, search)
	if err != nil {
		return packages, pkgTotal, nil
	}

	var result []model.DetectionPackage
	for _, d := range drafts {
		result = append(result, model.DetectionPackage{
			ID:          d.ID,
			PackageID:   d.PackageID,
			Version:     d.TargetVersion,
			Title:       d.Title,
			Description: d.Description,
			CVEIDs:      d.CVEIDs,
			Status:      d.Status,
			CreatedAt:   d.CreatedAt,
			UpdatedAt:   d.UpdatedAt,
		})
	}
	result = append(result, packages...)

	total := pkgTotal + draftTotal
	return result, total, nil
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
		return fmt.Errorf("invalid host_id: %w", err)
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
		return fmt.Errorf("get build: %w", err)
	}
	if build.Status != "awaiting_review" {
		return fmt.Errorf("build status is %s, not awaiting_review", build.Status)
	}
	if approved {
		build.Status = "success"
	} else {
		build.Status = "review_rejected"
	}
	build.ReviewedBy = &operator
	now := time.Now()
	build.ReviewedAt = &now
	build.ReviewComment = &comment
	if err := s.repo.UpdateBuild(build); err != nil {
		return fmt.Errorf("update build: %w", err)
	}
	s.recordOperation(build.PackageID, build.Version, "review", operator, nil, true, "")
	return nil
}

func (s *DetectionPackageService) ListPackageAlerts(ctx context.Context, packageID string, page, pageSize int) ([]model.RuntimeEvent, int64, error) {
	return s.repo.ListAlertsByPackageID(packageID, page, pageSize)
}

func (s *DetectionPackageService) GetBuildLogURL(ctx context.Context, buildID uuid.UUID) (string, error) {
	build, err := s.repo.GetBuild(buildID)
	if err != nil {
		return "", fmt.Errorf("get build: %w", err)
	}
	if build.BuildLogObjectKey == "" {
		return "", fmt.Errorf("build log not available")
	}
	return build.BuildLogObjectKey, nil
}

func (s *DetectionPackageService) ListAllowlistHistory(ctx context.Context, page, pageSize int) ([]model.EBPFHookAllowlistConfig, int64, error) {
	return s.repo.ListAllowlistHistory(page, pageSize)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

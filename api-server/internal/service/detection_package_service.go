package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"

	"github.com/google/uuid"
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
	SyncAgentConfig(ctx context.Context, hostID, configType, configJSON string) (int32, error)
	UninstallDetectionPackage(ctx context.Context, hostID, packageID, version string) (int32, error)
}

// BuilderClient is the interface for calling the builder gRPC service.
// Uses interface{} for request/response to avoid circular imports with grpc package.
type BuilderClient interface {
	StartBuild(ctx context.Context, req interface{}) (interface{}, error)
	SignPackage(ctx context.Context, req interface{}) (interface{}, error)
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
		ID:               uuid.New(),
		PackageID:        req.PackageID,
		TargetVersion:    req.TargetVersion,
		Title:            req.Title,
		Description:      req.Description,
		CVEIDs:           datatypes.JSON(cveIDs),
		HookPlanYAML:     req.HookPlanYAML,
		EBPFSource:       req.EBPFSource,
		SigmaRulesYAML:   req.SigmaRulesYAML,
		CorrelationYAML:  req.CorrelationYAML,
		BuildParams:      datatypes.JSON(buildParams),
		Status:           "draft",
		CreatedBy:        operator,
		UpdatedBy:        operator,
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

	result, err := s.builderClient.StartBuild(ctx, map[string]interface{}{
		"BuildID":         build.ID.String(),
		"PackageID":       build.PackageID,
		"Version":         build.Version,
		"Title":           draft.Title,
		"Operator":        operator,
		"TargetArch":      "amd64",
		"HookPlanYAML":    draft.HookPlanYAML,
		"EBPFSource":      draft.EBPFSource,
		"SigmaRulesYAML":  draft.SigmaRulesYAML,
		"CorrelationYAML": draft.CorrelationYAML,
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

	resp, ok := result.(map[string]interface{})
	if !ok {
		build.Status = "failed"
		build.ErrorMessage = "invalid builder response type"
		s.repo.UpdateBuild(build)
		draft.Status = "build_failed"
		s.repo.UpdateDraft(draft)
		return
	}

	build.Status = getString(resp, "Status")
	build.ErrorMessage = getString(resp, "ErrorMessage")
	build.BuilderDigest = getString(resp, "BuilderImageDigest")
	build.ClangVersion = getString(resp, "ClangVersion")
	build.BuildLogObjectKey = getString(resp, "BuildLogObjectKey")
	build.UnsignedPackageObjectKey = getString(resp, "UnsignedPackageObjectKey")
	build.UnsignedPackageSHA256 = getString(resp, "UnsignedPackageSHA256")
	build.UnsignedPackageSize = getInt64(resp, "UnsignedPackageSize")
	s.repo.UpdateBuild(build)

	if build.Status == "success" {
		draft.Status = "build_success"
	} else {
		draft.Status = "build_failed"
	}
	s.repo.UpdateDraft(draft)
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case float64:
			return int64(n)
		case int:
			return int64(n)
		}
	}
	return 0
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

	// Call builder to sign the package
	var signData map[string]interface{}
	if s.builderClient != nil {
		result, err := s.builderClient.SignPackage(ctx, map[string]interface{}{
			"BuildID":   build.ID.String(),
			"PackageID": build.PackageID,
			"Version":   build.Version,
			"Operator":  operator,
			"Confirm":   true,
		})
		if err != nil {
			return nil, fmt.Errorf("builder sign: %w", err)
		}
		signData, _ = result.(map[string]interface{})
		if signData != nil {
			if success, ok := signData["Success"].(bool); ok && !success {
				return nil, fmt.Errorf("builder sign failed: %s", getString(signData, "Message"))
			}
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

	if signData != nil {
		pkg.PackageObjectKey = getString(signData, "PackageObjectKey")
		pkg.SignatureObjectKey = getString(signData, "SignatureObjectKey")
		pkg.PackageSHA256 = getString(signData, "PackageSHA256")
		pkg.PackageSize = getInt64(signData, "PackageSize")
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
	return s.repo.ListPackages(page, pageSize, status, search)
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
		affected, err := s.serverClient.SyncAgentConfig(ctx, "", "dynamic_ebpf_hook_allowlist", configJSONStr)
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

func timePtr(t time.Time) *time.Time {
	return &t
}

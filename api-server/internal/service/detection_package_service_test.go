package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Create tables manually for SQLite compatibility (PostgreSQL defaults like gen_random_uuid() don't work in SQLite)
	db.Exec(`CREATE TABLE detection_package_drafts (
		id TEXT PRIMARY KEY,
		package_id TEXT NOT NULL UNIQUE,
		target_version TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		cve_ids TEXT NOT NULL DEFAULT '[]',
		ai_generated INTEGER NOT NULL DEFAULT 0,
		ai_generation_input TEXT,
		hook_plan_yaml TEXT,
		ebpf_source TEXT,
		sigma_rules_yaml TEXT,
		correlation_yaml TEXT,
		build_params TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'draft',
		last_build_id TEXT,
		created_by TEXT,
		updated_by TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE detection_packages (
		id TEXT PRIMARY KEY,
		package_id TEXT NOT NULL,
		version TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		cve_ids TEXT NOT NULL DEFAULT '[]',
		status TEXT NOT NULL DEFAULT 'built',
		package_object_key TEXT,
		signature_object_key TEXT,
		package_size INTEGER NOT NULL DEFAULT 0,
		package_sha256 TEXT,
		signed_by TEXT,
		signed_at DATETIME,
		enabled_at DATETIME,
		disabled_at DATETIME,
		reviewed_by TEXT,
		reviewed_at DATETIME,
		build_id TEXT,
		builder_image TEXT,
		builder_digest TEXT,
		manifest_json TEXT NOT NULL DEFAULT '{}',
		hook_summary TEXT NOT NULL DEFAULT '[]',
		event_schema TEXT NOT NULL DEFAULT '{}',
		limits_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(package_id, version)
	)`)

	db.Exec(`CREATE TABLE detection_package_builds (
		id TEXT PRIMARY KEY,
		draft_id TEXT,
		package_id TEXT NOT NULL,
		version TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		builder_image TEXT NOT NULL,
		builder_digest TEXT,
		clang_version TEXT,
		started_at DATETIME,
		finished_at DATETIME,
		duration_ms INTEGER,
		artifact_summary TEXT NOT NULL DEFAULT '[]',
		hook_summary TEXT NOT NULL DEFAULT '[]',
		event_schema TEXT NOT NULL DEFAULT '{}',
		unsigned_package_object_key TEXT,
		unsigned_package_sha256 TEXT,
		unsigned_package_size INTEGER NOT NULL DEFAULT 0,
		build_log_object_key TEXT,
		build_log TEXT,
		error_message TEXT,
		created_by TEXT,
		reviewed_by TEXT,
		reviewed_at DATETIME,
		review_comment TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE detection_package_host_status (
		id TEXT PRIMARY KEY,
		package_id TEXT NOT NULL,
		version TEXT NOT NULL,
		host_id TEXT NOT NULL,
		hostname TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		plugin_status TEXT,
		sigma_status TEXT,
		correlation_status TEXT,
		active_artifact TEXT,
		loaded_hooks TEXT NOT NULL DEFAULT '[]',
		kernel_release TEXT,
		arch TEXT,
		error_message TEXT,
		metrics_json TEXT NOT NULL DEFAULT '{}',
		installed_at DATETIME,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_reported_at DATETIME,
		UNIQUE(package_id, version, host_id)
	)`)

	db.Exec(`CREATE TABLE detection_package_operations (
		id TEXT PRIMARY KEY,
		package_id TEXT,
		version TEXT,
		operation TEXT NOT NULL,
		operator TEXT,
		request_json TEXT NOT NULL DEFAULT '{}',
		result_json TEXT NOT NULL DEFAULT '{}',
		success INTEGER NOT NULL DEFAULT 1,
		error_message TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	return db
}

func newTestDetectionPackageService(t *testing.T) (*DetectionPackageService, *gorm.DB) {
	t.Helper()

	db := newTestDB(t)
	repo := repository.NewDetectionPackageRepo(db)
	return NewDetectionPackageService(repo, db, nil, nil), db
}

func TestCreateDraft_AutoGeneratesUUIDPackageID(t *testing.T) {
	svc, _ := newTestDetectionPackageService(t)

	req := CreateDraftRequest{
		TargetVersion: "1.0.0",
		Title:         "Test Package",
		Description:   "Test description",
	}

	draft, err := svc.CreateDraft(context.Background(), req, "test-operator")
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}

	// package_id should be a valid UUID
	_, err = uuid.Parse(draft.PackageID)
	if err != nil {
		t.Errorf("expected package_id to be a valid UUID, got %q: %v", draft.PackageID, err)
	}
}

func TestCreateDraft_UsesProvidedPackageID(t *testing.T) {
	svc, _ := newTestDetectionPackageService(t)

	req := CreateDraftRequest{
		PackageID:     "custom-package-id",
		TargetVersion: "1.0.0",
		Title:         "Test Package",
	}

	draft, err := svc.CreateDraft(context.Background(), req, "test-operator")
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}

	if draft.PackageID != "custom-package-id" {
		t.Errorf("expected package_id %q, got %q", "custom-package-id", draft.PackageID)
	}
}

func TestListPackagesUnified_ReturnsMergedResults(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	// Create some drafts
	for i := 0; i < 3; i++ {
		req := CreateDraftRequest{
			TargetVersion: "1.0.0",
			Title:         "Draft Package",
		}
		_, err := svc.CreateDraft(context.Background(), req, "test-operator")
		if err != nil {
			t.Fatalf("CreateDraft %d failed: %v", i, err)
		}
	}

	// Create some published packages
	for i := 0; i < 3; i++ {
		pkg := &model.DetectionPackage{
			PackageID: uuid.New().String(),
			Version:   "1.0.0",
			Title:     "Published Package",
			Status:    "signed",
		}
		if err := db.Create(pkg).Error; err != nil {
			t.Fatalf("CreatePackage %d failed: %v", i, err)
		}
	}

	// List all (no status filter)
	result, total, err := svc.ListPackages(context.Background(), 1, 10, "", "")
	if err != nil {
		t.Fatalf("ListPackages failed: %v", err)
	}

	if total != 6 {
		t.Errorf("expected total 6, got %d", total)
	}
	if len(result) != 6 {
		t.Errorf("expected 6 results, got %d", len(result))
	}
}

func TestListPackagesUnified_Pagination(t *testing.T) {
	svc, _ := newTestDetectionPackageService(t)

	// Create 5 drafts with distinct timestamps
	for i := 0; i < 5; i++ {
		req := CreateDraftRequest{
			TargetVersion: "1.0.0",
			Title:         "Draft Package",
		}
		_, err := svc.CreateDraft(context.Background(), req, "test-operator")
		if err != nil {
			t.Fatalf("CreateDraft %d failed: %v", i, err)
		}
		// Small delay to ensure different updated_at
		time.Sleep(10 * time.Millisecond)
	}

	// Page 1, size 3
	result, total, err := svc.ListPackages(context.Background(), 1, 3, "", "")
	if err != nil {
		t.Fatalf("ListPackages page 1 failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results on page 1, got %d", len(result))
	}

	// Page 2, size 3
	result, total, err = svc.ListPackages(context.Background(), 2, 3, "", "")
	if err != nil {
		t.Fatalf("ListPackages page 2 failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results on page 2, got %d", len(result))
	}
}

func TestListPackagesUnified_FilterByStatus(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	// Create drafts
	for i := 0; i < 2; i++ {
		req := CreateDraftRequest{
			TargetVersion: "1.0.0",
			Title:         "Draft Package",
		}
		_, err := svc.CreateDraft(context.Background(), req, "test-operator")
		if err != nil {
			t.Fatalf("CreateDraft %d failed: %v", i, err)
		}
	}

	// Create published packages
	for i := 0; i < 3; i++ {
		pkg := &model.DetectionPackage{
			PackageID: uuid.New().String(),
			Version:   "1.0.0",
			Title:     "Published Package",
			Status:    "signed",
		}
		if err := db.Create(pkg).Error; err != nil {
			t.Fatalf("CreatePackage %d failed: %v", i, err)
		}
	}

	// Filter by draft status
	result, total, err := svc.ListPackages(context.Background(), 1, 10, "draft", "")
	if err != nil {
		t.Fatalf("ListPackages with draft filter failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 for draft filter, got %d", total)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results for draft filter, got %d", len(result))
	}

	// Filter by signed status
	result, total, err = svc.ListPackages(context.Background(), 1, 10, "signed", "")
	if err != nil {
		t.Fatalf("ListPackages with signed filter failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3 for signed filter, got %d", total)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results for signed filter, got %d", len(result))
	}
}

func TestListPackagesUnified_BuiltStatusIncludesLegacyBuildSuccess(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	legacyDraft, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		PackageID:     "legacy-build-success-draft",
		TargetVersion: "1.0.0",
		Title:         "Legacy Draft",
	}, "test-operator")
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}
	legacyDraft.Status = "build_success"
	if err := db.Save(legacyDraft).Error; err != nil {
		t.Fatalf("update legacy draft failed: %v", err)
	}
	pkg := &model.DetectionPackage{
		PackageID: "canonical-built-package",
		Version:   "1.0.0",
		Title:     "Canonical Built Package",
		Status:    "built",
	}
	if err := db.Create(pkg).Error; err != nil {
		t.Fatalf("CreatePackage failed: %v", err)
	}

	result, total, err := svc.ListPackages(context.Background(), 1, 10, "built", "")
	if err != nil {
		t.Fatalf("ListPackages with built filter failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2 for built aliases, got %d", total)
	}
	statuses := map[string]bool{}
	for _, item := range result {
		statuses[item.Status] = true
	}
	if !statuses["built"] || !statuses["build_success"] {
		t.Fatalf("expected built and build_success statuses, got %#v", statuses)
	}
}

type fakeDetectionPackageServerClient struct {
	installHostID string
	installCmd    *DetectionPackageCommand
	affected      int32
}

func (f *fakeDetectionPackageServerClient) InstallDetectionPackageFromService(ctx context.Context, hostID string, command interface{}) (int32, error) {
	_ = ctx
	f.installHostID = hostID
	if cmd, ok := command.(*DetectionPackageCommand); ok {
		f.installCmd = cmd
	}
	if f.affected == 0 {
		return 1, nil
	}
	return f.affected, nil
}

func (f *fakeDetectionPackageServerClient) SyncAgentConfig(ctx context.Context, hostID string, configs []*pb.AgentConfig) (int32, error) {
	_ = ctx
	_ = hostID
	_ = configs
	return 0, nil
}

func (f *fakeDetectionPackageServerClient) UninstallDetectionPackage(ctx context.Context, hostID, packageID, version string) (int32, error) {
	_ = ctx
	_ = hostID
	_ = packageID
	_ = version
	return 0, nil
}

type fakeDetectionPackageBuilderClient struct {
	reviewCalled   bool
	reviewApproved bool
	reviewResp     *pb.ReviewBuildResponse
	signResp       *grpcclient.BuilderSignResponse
	startReq       *grpcclient.BuilderStartBuildRequest
	startResp      *grpcclient.BuilderStartBuildResponse
	startCalled    chan struct{}
}

func (f *fakeDetectionPackageBuilderClient) StartBuild(ctx context.Context, req *grpcclient.BuilderStartBuildRequest) (*grpcclient.BuilderStartBuildResponse, error) {
	_ = ctx
	f.startReq = req
	if f.startCalled != nil {
		close(f.startCalled)
	}
	if f.startResp != nil {
		return f.startResp, nil
	}
	return &grpcclient.BuilderStartBuildResponse{Status: "awaiting_review"}, nil
}

func (f *fakeDetectionPackageBuilderClient) SignPackage(ctx context.Context, req *grpcclient.BuilderSignRequest) (*grpcclient.BuilderSignResponse, error) {
	_ = ctx
	_ = req
	if f.signResp != nil {
		return f.signResp, nil
	}
	return &grpcclient.BuilderSignResponse{Success: true}, nil
}

func (f *fakeDetectionPackageBuilderClient) GetPackageBuildStatus(ctx context.Context, packageID, version, buildID string) (*pb.GetPackageBuildStatusResponse, error) {
	_ = ctx
	_ = packageID
	_ = version
	_ = buildID
	return nil, nil
}

func (f *fakeDetectionPackageBuilderClient) ReviewBuild(ctx context.Context, buildID, packageID, version string, approved bool, comment, reviewer string) (*pb.ReviewBuildResponse, error) {
	_ = ctx
	_ = buildID
	_ = packageID
	_ = version
	_ = comment
	_ = reviewer
	f.reviewCalled = true
	f.reviewApproved = approved
	if f.reviewResp != nil {
		return f.reviewResp, nil
	}
	return &pb.ReviewBuildResponse{Success: true}, nil
}

func TestStartBuildPassesManifestAndCVEIDsToBuilder(t *testing.T) {
	svc, _ := newTestDetectionPackageService(t)
	startCalled := make(chan struct{})
	builder := &fakeDetectionPackageBuilderClient{
		startCalled: startCalled,
		startResp: &grpcclient.BuilderStartBuildResponse{
			Status:          "awaiting_review",
			EventSchemaJSON: `{"events":{"1001":{"name":"af_alg_socket","fields":{"1":{"name":"family","type":"string"}}}}}`,
		},
	}
	svc.builderClient = builder

	hookPlan := `
schema_version: "aegis.ebpf_plugin.v1"
plugin_id: "copyfail_probe"
package_id: "pkg-build-manifest"
version: "1.0.0"
event_schema:
  events:
    1001:
      name: "af_alg_socket"
      fields:
        1: { name: "family", type: "string" }
`
	if _, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		PackageID:       "pkg-build-manifest",
		TargetVersion:   "1.0.0",
		Title:           "Build Manifest Package",
		CVEIDs:          []string{"CVE-2026-31431"},
		HookPlanYAML:    hookPlan,
		EBPFSource:      "int main(void) { return 0; }",
		SigmaRulesYAML:  "title: test\n",
		CorrelationYAML: "rules: []\n",
	}, "creator"); err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}
	if _, err := svc.StartBuild(context.Background(), "pkg-build-manifest", "builder"); err != nil {
		t.Fatalf("StartBuild failed: %v", err)
	}

	select {
	case <-startCalled:
	case <-time.After(time.Second):
		t.Fatal("builder StartBuild was not called")
	}
	if builder.startReq == nil {
		t.Fatal("expected builder request")
	}
	if builder.startReq.PackageMetadataJSON != hookPlan {
		t.Fatalf("expected package metadata to include HookPlan manifest, got %q", builder.startReq.PackageMetadataJSON)
	}
	if got := builder.startReq.CVEIDs; len(got) != 1 || got[0] != "CVE-2026-31431" {
		t.Fatalf("expected CVE IDs to be passed, got %#v", got)
	}
}

func TestReviewBuild_RejectsAwaitingReviewBuild(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)
	builder := &fakeDetectionPackageBuilderClient{
		reviewResp: &pb.ReviewBuildResponse{Success: true, NewStatus: "review_rejected"},
	}
	svc.builderClient = builder

	draft, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		PackageID:     "pkg-review",
		TargetVersion: "1.0.0",
		Title:         "Review Package",
	}, "creator")
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}

	buildID := uuid.New()
	build := &model.DetectionPackageBuild{
		ID:           buildID,
		DraftID:      &draft.ID,
		PackageID:    draft.PackageID,
		Version:      draft.TargetVersion,
		Status:       "awaiting_review",
		BuilderImage: "aegis-agent-builder-ubi8:5.8.0",
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("Create build failed: %v", err)
	}

	if err := svc.ReviewBuild(context.Background(), buildID, false, "needs changes", "reviewer"); err != nil {
		t.Fatalf("ReviewBuild reject failed: %v", err)
	}
	if !builder.reviewCalled {
		t.Fatal("expected builder review to be called")
	}
	if builder.reviewApproved {
		t.Fatal("expected approved=false to be passed to builder")
	}

	updatedBuild, err := svc.GetBuild(context.Background(), buildID)
	if err != nil {
		t.Fatalf("GetBuild failed: %v", err)
	}
	if updatedBuild.Status != model.StatusReviewRejected {
		t.Fatalf("expected build status %q, got %q", model.StatusReviewRejected, updatedBuild.Status)
	}
	if updatedBuild.ReviewedBy == nil || *updatedBuild.ReviewedBy != "reviewer" {
		t.Fatalf("expected reviewed_by reviewer, got %#v", updatedBuild.ReviewedBy)
	}
	updatedDraft, err := svc.GetDraft(context.Background(), draft.PackageID)
	if err != nil {
		t.Fatalf("GetDraft failed: %v", err)
	}
	if updatedDraft.Status != model.StatusReviewRejected {
		t.Fatalf("expected draft status %q, got %q", model.StatusReviewRejected, updatedDraft.Status)
	}
}

func TestSignPackage_UsesDraftMetadata(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)
	builder := &fakeDetectionPackageBuilderClient{
		signResp: &grpcclient.BuilderSignResponse{
			Success:            true,
			PackageObjectKey:   "packages/pkg/1.0.0.tar.gz",
			SignatureObjectKey: "packages/pkg/1.0.0.tar.gz.sig",
			PackageSHA256:      "abc123",
			PackageSize:        2048,
		},
	}
	svc.builderClient = builder

	draft, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		PackageID:     "pkg-sign",
		TargetVersion: "1.0.0",
		Title:         "Signed Package Title",
		Description:   "Package description",
		CVEIDs:        []string{"CVE-2026-31431"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}
	buildID := uuid.New()
	build := &model.DetectionPackageBuild{
		ID:           buildID,
		DraftID:      &draft.ID,
		PackageID:    draft.PackageID,
		Version:      draft.TargetVersion,
		Status:       "success",
		BuilderImage: "aegis-agent-builder-ubi8:5.8.0",
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("Create build failed: %v", err)
	}

	pkg, err := svc.SignPackage(context.Background(), draft.PackageID, "signer")
	if err != nil {
		t.Fatalf("SignPackage failed: %v", err)
	}
	if pkg.Title != draft.Title {
		t.Fatalf("expected package title %q, got %q", draft.Title, pkg.Title)
	}
	if pkg.Description != draft.Description {
		t.Fatalf("expected package description %q, got %q", draft.Description, pkg.Description)
	}
	var cves []string
	if err := json.Unmarshal(pkg.CVEIDs, &cves); err != nil {
		t.Fatalf("unmarshal cve ids: %v", err)
	}
	if len(cves) != 1 || cves[0] != "CVE-2026-31431" {
		t.Fatalf("expected CVE metadata to be copied, got %#v", cves)
	}
}

func TestEnablePackage_DispatchesInstallCommand(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewDetectionPackageRepo(db)
	serverClient := &fakeDetectionPackageServerClient{affected: 2}
	svc := NewDetectionPackageService(repo, db, serverClient, nil)

	pkg := &model.DetectionPackage{
		ID:                 uuid.New(),
		PackageID:          "pkg-enable",
		Version:            "1.0.0",
		Title:              "Enable Package",
		Status:             "signed",
		PackageObjectKey:   "packages/pkg-enable/1.0.0.tar.gz",
		SignatureObjectKey: "packages/pkg-enable/1.0.0.tar.gz.sig",
		PackageSize:        4096,
	}
	if err := db.Create(pkg).Error; err != nil {
		t.Fatalf("Create package failed: %v", err)
	}

	if err := svc.EnablePackage(context.Background(), pkg.PackageID, "admin"); err != nil {
		t.Fatalf("EnablePackage failed: %v", err)
	}
	if serverClient.installHostID != "" {
		t.Fatalf("expected global rollout host id to be empty, got %q", serverClient.installHostID)
	}
	if serverClient.installCmd == nil {
		t.Fatal("expected install command to be dispatched")
	}
	if serverClient.installCmd.PackageID != pkg.PackageID || serverClient.installCmd.PackageURL != pkg.PackageObjectKey {
		t.Fatalf("unexpected install command: %#v", serverClient.installCmd)
	}

	enabled, err := repo.GetLatestPackage(pkg.PackageID)
	if err != nil {
		t.Fatalf("GetLatestPackage failed: %v", err)
	}
	if enabled.Status != "enabled" {
		t.Fatalf("expected enabled status, got %q", enabled.Status)
	}
}

func TestDetectionPackageCommand_JSONTagsMatchProtoRequest(t *testing.T) {
	cmd := DetectionPackageCommand{
		CommandID:    "cmd-1",
		Action:       "install",
		PackageID:    "pkg-json",
		Version:      "1.0.0",
		PackageURL:   "packages/pkg-json.tar.gz",
		SignatureURL: "packages/pkg-json.tar.gz.sig",
		PackageSize:  1024,
		Rollback:     true,
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	var pbCmd pb.DetectionPackageCommandRequest
	if err := json.Unmarshal(data, &pbCmd); err != nil {
		t.Fatalf("unmarshal into proto request: %v", err)
	}
	if pbCmd.PackageId != cmd.PackageID || pbCmd.PackageUrl != cmd.PackageURL || !pbCmd.Rollback {
		t.Fatalf("json tags did not map to proto request: %#v", pbCmd)
	}
}

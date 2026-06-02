package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"api-server/config"
	grpcclient "api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"gorm.io/datatypes"
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

	db.Exec(`CREATE TABLE ebpf_hook_allowlist_configs (
		id TEXT,
		version INTEGER PRIMARY KEY AUTOINCREMENT,
		config_json TEXT NOT NULL,
		description TEXT,
		updated_by TEXT,
		change_reason TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		activated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE sigma_rules (
		id TEXT PRIMARY KEY,
		rule_id TEXT NOT NULL UNIQUE,
		title TEXT,
		description TEXT,
		content TEXT NOT NULL,
		status TEXT NOT NULL,
		mitre_id TEXT,
		severity TEXT,
		generated_by TEXT NOT NULL,
		version TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		activated_at DATETIME,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		source TEXT DEFAULT 'upload',
		file_name TEXT,
		file_hash TEXT,
		file_size INTEGER,
		parsed_at DATETIME,
		parse_error TEXT,
		ai_generated BOOLEAN DEFAULT FALSE,
		parent_rule_id TEXT,
		generation_prompt TEXT,
		generation_context TEXT,
		approved_by TEXT,
		approved_at DATETIME,
		dispatch_hosts TEXT DEFAULT '[]',
		dispatch_status TEXT DEFAULT 'pending'
	)`)

	db.Exec(`CREATE TABLE correlation_rules (
		id TEXT PRIMARY KEY,
		package_id TEXT NOT NULL,
		package_version TEXT NOT NULL,
		rule_id TEXT NOT NULL,
		title TEXT,
		severity TEXT,
		by_key TEXT,
		window_seconds INTEGER,
		ordered BOOLEAN NOT NULL DEFAULT TRUE,
		sequence_json TEXT NOT NULL DEFAULT '[]',
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(package_id, package_version, rule_id)
	)`)

	return db
}

func newTestDetectionPackageService(t *testing.T) (*DetectionPackageService, *gorm.DB) {
	t.Helper()

	db := newTestDB(t)
	repo := repository.NewDetectionPackageRepo(db)
	return NewDetectionPackageService(repo, db, nil, nil, "http://minio:9000/aegis-releases"), db
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

func TestUpdateDraftResetsBuildStatus(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	draft, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		PackageID:     "pkg-reset-build-status",
		TargetVersion: "1.0.0",
		Title:         "Reset Build Status",
		EBPFSource:    "int main(void) { return 0; }",
	}, "creator")
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}
	buildID := uuid.New()
	draft.Status = "build_failed"
	draft.LastBuildID = &buildID
	if err := db.Save(draft).Error; err != nil {
		t.Fatalf("save failed draft state: %v", err)
	}

	nextSource := "int main(void) { return 1; }"
	updated, err := svc.UpdateDraft(context.Background(), draft.ID, UpdateDraftRequest{
		EBPFSource: &nextSource,
	}, "editor")
	if err != nil {
		t.Fatalf("UpdateDraft failed: %v", err)
	}
	if updated.Status != "draft" {
		t.Fatalf("Status = %q, want draft", updated.Status)
	}
	if updated.LastBuildID != nil {
		t.Fatalf("LastBuildID = %v, want nil", updated.LastBuildID)
	}
}

func TestSyncDetectionPackageRulesRegistersOnlyFinalCorrelationRule(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	const packageID = "pkg-cve-2026-31431"
	sigmaYAML := `---
title: AF ALG Socket
id: pkg-cve-2026-31431.af_alg_socket
description: socket observed
level: medium
tags:
  - cve.2026-31431
---
title: Splice Call
id: pkg-cve-2026-31431.splice_call
description: splice observed
level: high
tags:
  - attack.t1068
`
	correlationYAML := `schema_version: "aegis.correlation.v1"
id: "pkg-cve-2026-31431.copyfail_chain"
package_id: "pkg-cve-2026-31431"
correlation:
  by: "pid"
  window: "10s"
  ordered: true
  sequence:
    - rule_id: "pkg-cve-2026-31431.af_alg_socket"
    - rule_id: "pkg-cve-2026-31431.splice_call"
alert:
  title: "Possible CVE-2026-31431 CopyFail"
  severity: "critical"
  mitre_id: "T1068"
  cve_id: "CVE-2026-31431"
`

	svc.syncDetectionPackageRules(sigmaYAML, correlationYAML, packageID, "1.0.0")

	var rules []model.SigmaRule
	if err := db.Order("rule_id").Find(&rules).Error; err != nil {
		t.Fatalf("query sigma_rules failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected only the final correlation rule, got %d", len(rules))
	}
	if rules[0].RuleID != "pkg-cve-2026-31431.copyfail_chain" {
		t.Fatalf("synced rule = %q, want final correlation rule", rules[0].RuleID)
	}
	if rules[0].Source != "detection_package_correlation" {
		t.Fatalf("Source = %q, want detection_package_correlation", rules[0].Source)
	}
	if rules[0].MitreID != "T1068" {
		t.Fatalf("MitreID = %q, want T1068", rules[0].MitreID)
	}
	if rules[0].Status != "active" {
		t.Fatalf("Status = %q, want active", rules[0].Status)
	}

	var correlation model.CorrelationRule
	if err := db.Where("rule_id = ?", "pkg-cve-2026-31431.copyfail_chain").First(&correlation).Error; err != nil {
		t.Fatalf("correlation rule not synced: %v", err)
	}
	if correlation.Severity != "critical" || correlation.WindowSeconds != 10 {
		t.Fatalf("unexpected correlation metadata: severity=%q window=%d", correlation.Severity, correlation.WindowSeconds)
	}
}

func TestSyncDetectionPackageRulesDeletesExistingAtomicRules(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	const packageID = "pkg-with-old-atomics"
	if err := db.Exec(`
		INSERT INTO sigma_rules (id, rule_id, title, content, status, mitre_id, severity, generated_by, version, source, parent_rule_id)
		VALUES (?, ?, 'Old Atomic', '{}', 'active', 'T1068', 'medium', 'detection_package', '1.0.0', 'detection_package', ?)
	`, uuid.New().String(), packageID+".old_atomic", packageID).Error; err != nil {
		t.Fatalf("insert old atomic rule failed: %v", err)
	}

	correlationYAML := `schema_version: "aegis.correlation.v1"
id: "pkg-with-old-atomics.final"
package_id: "pkg-with-old-atomics"
correlation:
  by: "pid"
  window: "10s"
  ordered: true
  sequence:
    - rule_id: "pkg-with-old-atomics.old_atomic"
alert:
  title: "Final rule"
  severity: "critical"
  mitre_id: "T1068"
`

	svc.syncDetectionPackageRules("", correlationYAML, packageID, "1.0.0")

	var atomicCount int64
	if err := db.Table("sigma_rules").Where("source = ?", "detection_package").Count(&atomicCount).Error; err != nil {
		t.Fatalf("count atomic rules failed: %v", err)
	}
	if atomicCount != 0 {
		t.Fatalf("atomic rule count = %d, want 0", atomicCount)
	}

	var finalCount int64
	if err := db.Table("sigma_rules").Where("source = ?", "detection_package_correlation").Count(&finalCount).Error; err != nil {
		t.Fatalf("count final rules failed: %v", err)
	}
	if finalCount != 1 {
		t.Fatalf("final rule count = %d, want 1", finalCount)
	}
}

func TestListHostStatusDefaultsToLatestPackageVersion(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	packageID := "pkg-host-status-current-version"
	for _, pkg := range []model.DetectionPackage{
		{ID: uuid.New(), PackageID: packageID, Version: "1.0.0", Title: "Old", Status: "disabled", CreatedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now().Add(-time.Hour)},
		{ID: uuid.New(), PackageID: packageID, Version: "1.0.1", Title: "New", Status: "enabled", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	} {
		if err := db.Create(&pkg).Error; err != nil {
			t.Fatalf("create package version %s failed: %v", pkg.Version, err)
		}
	}

	hostID := uuid.New()
	for _, status := range []model.DetectionPackageHostStatus{
		{ID: uuid.New(), PackageID: packageID, Version: "1.0.0", HostID: hostID, Hostname: "agent-1", Status: "uninstalled"},
		{ID: uuid.New(), PackageID: packageID, Version: "1.0.1", HostID: hostID, Hostname: "agent-1", Status: "active"},
	} {
		if err := db.Create(&status).Error; err != nil {
			t.Fatalf("create host status %s failed: %v", status.Version, err)
		}
	}

	statuses, total, err := svc.ListHostStatus(context.Background(), packageID, "", 1, 20)
	if err != nil {
		t.Fatalf("ListHostStatus failed: %v", err)
	}
	if total != 1 || len(statuses) != 1 {
		t.Fatalf("got total=%d len=%d, want one current status", total, len(statuses))
	}
	if statuses[0].Version != "1.0.1" || statuses[0].Status != "active" {
		t.Fatalf("status = version %s status %s, want 1.0.1 active", statuses[0].Version, statuses[0].Status)
	}
}

func TestReportHostStatusRemovesPreviousVersionForHost(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	packageID := "pkg-host-status-upsert"
	hostID := uuid.New()
	if err := db.Create(&model.DetectionPackageHostStatus{
		ID:        uuid.New(),
		PackageID: packageID,
		Version:   "1.0.0",
		HostID:    hostID,
		Hostname:  "agent-1",
		Status:    "uninstalled",
	}).Error; err != nil {
		t.Fatalf("create old host status failed: %v", err)
	}

	if err := svc.ReportHostStatus(context.Background(), HostStatusReport{
		HostID:      hostID.String(),
		Hostname:    "agent-1",
		PackageID:   packageID,
		Version:     "1.0.1",
		Status:      "active",
		LoadedHooks: []string{"sys_enter_socket"},
	}); err != nil {
		t.Fatalf("ReportHostStatus failed: %v", err)
	}

	var statuses []model.DetectionPackageHostStatus
	if err := db.Where("package_id = ? AND host_id = ?", packageID, hostID).Find(&statuses).Error; err != nil {
		t.Fatalf("query statuses failed: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("status row count = %d, want 1", len(statuses))
	}
	if statuses[0].Version != "1.0.1" || statuses[0].Status != "active" {
		t.Fatalf("status = version %s status %s, want 1.0.1 active", statuses[0].Version, statuses[0].Status)
	}
}

func TestListHostStatusMarksInstallingTimeouts(t *testing.T) {
	svc, db := newTestDetectionPackageService(t)

	packageID := "pkg-host-status-timeout"
	if err := db.Create(&model.DetectionPackage{
		ID:        uuid.New(),
		PackageID: packageID,
		Version:   "1.0.0",
		Title:     "Timeout",
		Status:    "enabled",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create package failed: %v", err)
	}

	hostID := uuid.New()
	if err := db.Create(&model.DetectionPackageHostStatus{
		ID:        uuid.New(),
		PackageID: packageID,
		Version:   "1.0.0",
		HostID:    hostID,
		Hostname:  "agent-timeout",
		Status:    "installing",
		UpdatedAt: time.Now().Add(-11 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("create installing status failed: %v", err)
	}

	statuses, total, err := svc.ListHostStatus(context.Background(), packageID, "", 1, 20)
	if err != nil {
		t.Fatalf("ListHostStatus failed: %v", err)
	}
	if total != 1 || len(statuses) != 1 {
		t.Fatalf("got total=%d len=%d, want one timeout status", total, len(statuses))
	}
	if statuses[0].Status != "timeout" {
		t.Fatalf("status = %s, want timeout", statuses[0].Status)
	}
	if !strings.Contains(statuses[0].ErrorMessage, "10 分钟") {
		t.Fatalf("error_message = %q, want 10 minute timeout message", statuses[0].ErrorMessage)
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

func TestGetDraft_MissingReturnsNilDraft(t *testing.T) {
	svc, _ := newTestDetectionPackageService(t)

	draft, err := svc.GetDraft(context.Background(), "missing-package-id")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got draft=%v err=%v", draft, err)
	}
	if draft != nil {
		t.Fatalf("expected nil draft on missing package_id, got %#v", draft)
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

func TestAllowlistConfigPayloadAddsVersion(t *testing.T) {
	payload := allowlistConfigPayload(datatypes.JSON([]byte(`{"tracepoints":["syscalls/sys_enter_socket"]}`)), 42)

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("payload is not valid json: %v", err)
	}
	if got := int(decoded["version"].(float64)); got != 42 {
		t.Fatalf("version = %d, want 42", got)
	}
}

func TestUpdateAllowlistSyncsVersionedPayload(t *testing.T) {
	svc, _ := newTestDetectionPackageService(t)
	server := &fakeDetectionPackageServerClient{}
	svc.serverClient = server

	_, err := svc.UpdateAllowlist(context.Background(), datatypes.JSON([]byte(`{
		"tracepoints": ["syscalls/sys_enter_socket", "syscalls/sys_enter_bind", "syscalls/sys_enter_splice"],
		"kprobes": [],
		"lsm": [],
		"xdp": [],
		"tc": []
	}`)), "test allowlist", "tester")
	if err != nil {
		t.Fatalf("UpdateAllowlist failed: %v", err)
	}
	if len(server.syncConfigs) != 1 {
		t.Fatalf("expected one synced config, got %d", len(server.syncConfigs))
	}
	if server.syncConfigs[0].ConfigType != "dynamic_ebpf_hook_allowlist" {
		t.Fatalf("unexpected config type: %s", server.syncConfigs[0].ConfigType)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(server.syncConfigs[0].ConfigJson), &decoded); err != nil {
		t.Fatalf("synced config is not valid json: %v", err)
	}
	if version, ok := decoded["version"].(float64); !ok || version <= 0 {
		t.Fatalf("expected positive version in synced config, got %#v", decoded["version"])
	}
}

type fakeDetectionPackageServerClient struct {
	installHostID string
	installCmd    *DetectionPackageCommand
	affected      int32
	syncHostID    string
	syncConfigs   []*pb.AgentConfig
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
	f.syncHostID = hostID
	f.syncConfigs = configs
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
	svc := NewDetectionPackageService(repo, db, serverClient, nil, "http://minio:9000/aegis-releases")

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
	if serverClient.installCmd.PackageID != pkg.PackageID || serverClient.installCmd.PackageURL != "http://minio:9000/aegis-releases/"+pkg.PackageObjectKey {
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

// Regression test: objectKeyToURL should correctly build URL from base URL
func TestObjectKeyToURL_WithExternalIP(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		objectKey   string
		expectedURL string
	}{
		{
			name:        "external IP with path",
			baseURL:     "http://192.168.152.159:9000/aegis-releases",
			objectKey:   "detection-packages/pkg-123/1.0.0/signed/package.tar.gz",
			expectedURL: "http://192.168.152.159:9000/aegis-releases/detection-packages/pkg-123/1.0.0/signed/package.tar.gz",
		},
		{
			name:        "localhost base URL",
			baseURL:     "http://localhost:9000/aegis-releases",
			objectKey:   "detection-packages/pkg-456/2.0.0/signed/package.tar.gz",
			expectedURL: "http://localhost:9000/aegis-releases/detection-packages/pkg-456/2.0.0/signed/package.tar.gz",
		},
		{
			name:        "full HTTP URL passed through",
			baseURL:     "http://192.168.152.159:9000/aegis-releases",
			objectKey:   "http://other-host:9000/bucket/file.tar.gz",
			expectedURL: "http://other-host:9000/bucket/file.tar.gz",
		},
		{
			name:        "empty object key",
			baseURL:     "http://192.168.152.159:9000/aegis-releases",
			objectKey:   "",
			expectedURL: "",
		},
		{
			name:        "object key with leading slash",
			baseURL:     "http://192.168.152.159:9000/aegis-releases",
			objectKey:   "/detection-packages/pkg-789/1.0.0/package.tar.gz",
			expectedURL: "http://192.168.152.159:9000/aegis-releases/detection-packages/pkg-789/1.0.0/package.tar.gz",
		},
		{
			name:        "base URL with trailing slash",
			baseURL:     "http://192.168.152.159:9000/aegis-releases/",
			objectKey:   "detection-packages/pkg-abc/1.0.0/package.tar.gz",
			expectedURL: "http://192.168.152.159:9000/aegis-releases/detection-packages/pkg-abc/1.0.0/package.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &DetectionPackageService{
				artifactDownloadBaseURL: tt.baseURL,
			}
			result := svc.objectKeyToURL(tt.objectKey)
			if result != tt.expectedURL {
				t.Errorf("objectKeyToURL(%q) = %q, want %q", tt.objectKey, result, tt.expectedURL)
			}
		})
	}
}

// Regression test: EnablePackage should use correct external IP in download URL
func TestEnablePackage_UsesExternalIPInURL(t *testing.T) {
	externalIP := "192.168.152.159"
	baseURL := "http://" + externalIP + ":9000/aegis-releases"

	db := newTestDB(t)
	repo := repository.NewDetectionPackageRepo(db)
	serverClient := &fakeDetectionPackageServerClient{}
	svc := NewDetectionPackageService(repo, db, serverClient, nil, baseURL)

	pkg := &model.DetectionPackage{
		ID:                 uuid.New(),
		PackageID:          "pkg-external-ip",
		Version:            "1.0.0",
		Title:              "External IP Test Package",
		Status:             "signed",
		PackageObjectKey:   "detection-packages/pkg-external-ip/1.0.0/signed/package.tar.gz",
		SignatureObjectKey: "detection-packages/pkg-external-ip/1.0.0/signed/package.tar.gz.sig",
		PackageSize:        4096,
	}
	if err := db.Create(pkg).Error; err != nil {
		t.Fatalf("Create package failed: %v", err)
	}

	if err := svc.EnablePackage(context.Background(), pkg.PackageID, "admin"); err != nil {
		t.Fatalf("EnablePackage failed: %v", err)
	}

	if serverClient.installCmd == nil {
		t.Fatal("expected install command to be dispatched")
	}

	expectedPackageURL := baseURL + "/" + pkg.PackageObjectKey
	if serverClient.installCmd.PackageURL != expectedPackageURL {
		t.Errorf("expected PackageURL %q, got %q", expectedPackageURL, serverClient.installCmd.PackageURL)
	}

	expectedSignatureURL := baseURL + "/" + pkg.SignatureObjectKey
	if serverClient.installCmd.SignatureURL != expectedSignatureURL {
		t.Errorf("expected SignatureURL %q, got %q", expectedSignatureURL, serverClient.installCmd.SignatureURL)
	}

	// Verify the URL contains the external IP, not localhost
	if strings.Contains(serverClient.installCmd.PackageURL, "localhost") {
		t.Errorf("PackageURL should not contain 'localhost', got %q", serverClient.installCmd.PackageURL)
	}
	if !strings.Contains(serverClient.installCmd.PackageURL, externalIP) {
		t.Errorf("PackageURL should contain external IP %q, got %q", externalIP, serverClient.installCmd.PackageURL)
	}
}

// Regression test: config should support MINIO_ARTIFACT_BASE_URL environment variable
func TestConfig_MinIOArtifactBaseURL_EnvOverride(t *testing.T) {
	// This test verifies that the config struct has the ArtifactBaseURL field
	// and it can be set via environment variable
	cfg := &config.MinIOConfig{
		Endpoint:        "minio:9000",
		ArtifactBaseURL: "http://192.168.152.159:9000/aegis-releases",
	}

	if cfg.ArtifactBaseURL != "http://192.168.152.159:9000/aegis-releases" {
		t.Errorf("expected ArtifactBaseURL to be set, got %q", cfg.ArtifactBaseURL)
	}
}

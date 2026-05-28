package service

import (
	"context"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"

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
		builder_image_digest TEXT,
		clang_version TEXT,
		started_at DATETIME,
		finished_at DATETIME,
		duration_ms INTEGER,
		artifacts TEXT NOT NULL DEFAULT '[]',
		hook_summary TEXT NOT NULL DEFAULT '[]',
		event_schema TEXT NOT NULL DEFAULT '{}',
		unsigned_package_object_key TEXT,
		unsigned_package_sha256 TEXT,
		unsigned_package_size INTEGER NOT NULL DEFAULT 0,
		build_log_object_key TEXT,
		build_log_tail TEXT,
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

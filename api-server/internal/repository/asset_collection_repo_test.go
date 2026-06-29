package repository

import (
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetApplicationAssetsDeduplicatesSameHostName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset_repo_dedupe?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := createHostApplicationAssetsTestTable(db); err != nil {
		t.Fatalf("failed to create host application assets table: %v", err)
	}

	repo := NewAssetCollectionRepository(db)
	hostID := uuid.New()
	now := time.Now()
	olderID := uuid.New()
	keeperID := uuid.New()

	assets := []model.HostApplicationAsset{
		{
			ID:              olderID,
			HostID:          hostID,
			Hostname:        "asset-host",
			OSType:          "linux",
			Category:        "database",
			Name:            "redis",
			DisplayName:     "Redis",
			AIConfidence:    0.6,
			Status:          "active",
			ReviewStatus:    "auto",
			Fingerprint:     "old-redis",
			LastSeenAt:      now.Add(-time.Hour),
			CollectedAt:     now.Add(-time.Hour),
			FirstSeenAt:     now.Add(-time.Hour),
			CreatedAt:       now.Add(-time.Hour),
			UpdatedAt:       now.Add(-time.Hour),
			ConfigPaths:     []byte(`[]`),
			SitePaths:       []byte(`[]`),
			Domains:         []byte(`[]`),
			ListenPorts:     []byte(`[]`),
			RelatedPIDs:     []byte(`[]`),
			RelatedPackages: []byte(`[]`),
			AIEvidence:      []byte(`[]`),
			AIRawOutput:     []byte(`{}`),
			ManualOverrides: []byte(`{}`),
		},
		{
			ID:              keeperID,
			HostID:          hostID,
			Hostname:        "asset-host",
			OSType:          "linux",
			Category:        "database",
			Name:            "redis",
			DisplayName:     "Redis",
			AIConfidence:    0.95,
			Status:          "active",
			ReviewStatus:    "auto",
			Fingerprint:     "new-redis",
			LastSeenAt:      now,
			CollectedAt:     now,
			FirstSeenAt:     now,
			CreatedAt:       now,
			UpdatedAt:       now,
			ConfigPaths:     []byte(`[]`),
			SitePaths:       []byte(`[]`),
			Domains:         []byte(`[]`),
			ListenPorts:     []byte(`[]`),
			RelatedPIDs:     []byte(`[]`),
			RelatedPackages: []byte(`[]`),
			AIEvidence:      []byte(`[]`),
			AIRawOutput:     []byte(`{}`),
			ManualOverrides: []byte(`{}`),
		},
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatalf("failed to create application assets: %v", err)
	}

	items, total, err := repo.GetApplicationAssets(model.ApplicationAssetQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("GetApplicationAssets returned error: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("deduped items = %d total = %d, want 1/1: %#v", len(items), total, items)
	}
	if items[0].ID != keeperID {
		t.Fatalf("kept asset id = %s, want %s", items[0].ID, keeperID)
	}

	summary, err := repo.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
	if summary.ApplicationCount != 1 || summary.DatabaseCount != 1 {
		t.Fatalf("summary counts = %#v, want deduped application/database counts", summary)
	}
}

func TestUpsertApplicationAssetReactivatesDeletedFingerprint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset_repo_reactivate?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := createHostApplicationAssetsTestTable(db); err != nil {
		t.Fatalf("failed to create host application assets table: %v", err)
	}

	repo := NewAssetCollectionRepository(db)
	hostID := uuid.New()
	fp := "reactivate-docker"
	oldAsset := model.HostApplicationAsset{
		ID:              uuid.New(),
		HostID:          hostID,
		Hostname:        "asset-host",
		OSType:          "linux",
		Category:        "other",
		Name:            "docker",
		DisplayName:     "Docker",
		AIConfidence:    0.4,
		Status:          "deleted",
		ReviewStatus:    "auto",
		Fingerprint:     fp,
		ConfigPaths:     []byte(`[]`),
		SitePaths:       []byte(`[]`),
		Domains:         []byte(`[]`),
		ListenPorts:     []byte(`[]`),
		RelatedPIDs:     []byte(`[]`),
		RelatedPackages: []byte(`[]`),
		AIEvidence:      []byte(`[]`),
		AIRawOutput:     []byte(`{}`),
		ManualOverrides: []byte(`{}`),
	}
	if err := db.Create(&oldAsset).Error; err != nil {
		t.Fatalf("failed to create deleted application asset: %v", err)
	}

	newAsset := oldAsset
	newAsset.ID = uuid.New()
	newAsset.DisplayName = "Docker Engine"
	newAsset.AIConfidence = 0.95
	newAsset.Status = "active"
	if err := repo.UpsertApplicationAsset(&newAsset); err != nil {
		t.Fatalf("UpsertApplicationAsset returned error: %v", err)
	}

	var stored model.HostApplicationAsset
	if err := db.Where("host_id = ? AND fingerprint = ?", hostID, fp).First(&stored).Error; err != nil {
		t.Fatalf("failed to load upserted application asset: %v", err)
	}
	if stored.Status != "active" || stored.DisplayName != "Docker Engine" || stored.AIConfidence != 0.95 {
		t.Fatalf("upserted asset = %#v, want active Docker Engine with refreshed confidence", stored)
	}
}

func TestUpsertApplicationAssetKeepsExistingVersionWhenIncomingVersionEmpty(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset_repo_keep_version?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := createHostApplicationAssetsTestTable(db); err != nil {
		t.Fatalf("failed to create host application assets table: %v", err)
	}

	repo := NewAssetCollectionRepository(db)
	hostID := uuid.New()
	fp := "docker-version"
	oldAsset := model.HostApplicationAsset{
		ID:              uuid.New(),
		HostID:          hostID,
		Hostname:        "asset-host",
		OSType:          "linux",
		Category:        "other",
		Name:            "docker",
		DisplayName:     "Docker Engine",
		Version:         "29.5.3",
		VersionSource:   "tool",
		AIConfidence:    0.95,
		Status:          "active",
		ReviewStatus:    "auto",
		Fingerprint:     fp,
		ConfigPaths:     []byte(`[]`),
		SitePaths:       []byte(`[]`),
		Domains:         []byte(`[]`),
		ListenPorts:     []byte(`[]`),
		RelatedPIDs:     []byte(`[]`),
		RelatedPackages: []byte(`[]`),
		AIEvidence:      []byte(`[]`),
		AIRawOutput:     []byte(`{}`),
		ManualOverrides: []byte(`{}`),
	}
	if err := db.Create(&oldAsset).Error; err != nil {
		t.Fatalf("failed to create versioned application asset: %v", err)
	}

	newAsset := oldAsset
	newAsset.ID = uuid.New()
	newAsset.Version = ""
	newAsset.VersionSource = "ai"
	newAsset.AIConfidence = 0.86
	if err := repo.UpsertApplicationAsset(&newAsset); err != nil {
		t.Fatalf("UpsertApplicationAsset returned error: %v", err)
	}

	var stored model.HostApplicationAsset
	if err := db.Where("host_id = ? AND fingerprint = ?", hostID, fp).First(&stored).Error; err != nil {
		t.Fatalf("failed to load upserted application asset: %v", err)
	}
	if stored.Version != "29.5.3" || stored.VersionSource != "tool" {
		t.Fatalf("stored version = %q source = %q, want existing version/source", stored.Version, stored.VersionSource)
	}
	if stored.AIConfidence != 0.86 {
		t.Fatalf("stored confidence = %v, want other fields to refresh", stored.AIConfidence)
	}
}

func createHostApplicationAssetsTestTable(db *gorm.DB) error {
	return db.Exec(`CREATE TABLE host_application_assets (
		id TEXT PRIMARY KEY,
		host_id TEXT,
		hostname TEXT,
		ip_address TEXT,
		group_name TEXT,
		os_type TEXT,
		category TEXT,
		name TEXT,
		display_name TEXT,
		version TEXT,
		version_source TEXT,
		install_path TEXT,
		start_path TEXT,
		config_paths BLOB,
		site_paths BLOB,
		domains BLOB,
		listen_ports BLOB,
		run_user TEXT,
		runtime_name TEXT,
		runtime_version TEXT,
		framework_name TEXT,
		framework_version TEXT,
		related_pids BLOB,
		related_packages BLOB,
		ai_confidence REAL,
		ai_evidence BLOB,
		ai_raw_output BLOB,
		manual_overrides BLOB,
		review_status TEXT,
		status TEXT,
		fingerprint TEXT,
		first_seen_at DATETIME,
		last_seen_at DATETIME,
		collected_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		UNIQUE(host_id, fingerprint)
	)`).Error
}

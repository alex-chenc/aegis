package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentGuardPolicyRepositoryDraftLifecycle(t *testing.T) {
	db := setupAgentGuardPolicyTestDB(t)
	repo := NewAgentGuardPolicyRepository(db)
	ctx := context.Background()

	draft := newAgentGuardPolicyDraft("prod-agent-guard", 1)
	if err := repo.CreateDraft(ctx, draft); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	got, err := repo.GetByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != "draft" || got.PolicyKey != draft.PolicyKey {
		t.Fatalf("unexpected created draft: %#v", got)
	}

	updated, err := repo.UpdateDraft(ctx, draft.ID, model.AgentGuardPolicyDraftUpdate{
		Name:                 "Updated policy",
		Description:          "updated",
		Priority:             200,
		Targets:              datatypes.JSON([]byte(`{"agent_types":["codex"]}`)),
		CollectionPolicy:     datatypes.JSON([]byte(`{"categories":["process"]}`)),
		BuiltinRuleOverrides: datatypes.JSON([]byte(`[]`)),
		AtomicRules:          datatypes.JSON([]byte(`[]`)),
		CorrelationRules:     datatypes.JSON([]byte(`[]`)),
		AnalysisPolicy:       datatypes.JSON([]byte(`{"enabled":false}`)),
		EscapeRules:          datatypes.JSON([]byte(`[]`)),
		FreezeTimeoutSeconds: 120,
		CompiledPreview:      datatypes.JSON([]byte(`{"mode":"monitor_only"}`)),
		Digest:               "sha256:updated",
	})
	if err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	if updated.Name != "Updated policy" || updated.Priority != 200 || updated.FreezeTimeoutSeconds != 120 {
		t.Fatalf("unexpected updated policy: %#v", updated)
	}

	items, total, err := repo.List(ctx, model.AgentGuardPolicyQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		Status:              "draft",
		Keyword:             "updated",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != draft.ID {
		t.Fatalf("unexpected policy list: total=%d items=%#v", total, items)
	}
}

func TestAgentGuardPolicyRepositoryRejectsNonDraftUpdate(t *testing.T) {
	db := setupAgentGuardPolicyTestDB(t)
	repo := NewAgentGuardPolicyRepository(db)
	ctx := context.Background()

	published := newAgentGuardPolicyDraft("published-agent-guard", 1)
	published.Status = "published"
	if err := db.Create(published).Error; err != nil {
		t.Fatalf("seed published policy: %v", err)
	}

	_, err := repo.UpdateDraft(ctx, published.ID, model.AgentGuardPolicyDraftUpdate{Name: "must not change"})
	if !errors.Is(err, ErrAgentGuardPolicyNotDraft) {
		t.Fatalf("UpdateDraft error = %v, want ErrAgentGuardPolicyNotDraft", err)
	}

	got, err := repo.GetByID(ctx, published.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name == "must not change" {
		t.Fatal("published policy was mutated in place")
	}
}

func TestAgentGuardPolicyRepositoryNotFound(t *testing.T) {
	db := setupAgentGuardPolicyTestDB(t)
	repo := NewAgentGuardPolicyRepository(db)
	ctx := context.Background()

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, ErrAgentGuardPolicyNotFound) {
		t.Fatalf("GetByID error = %v, want ErrAgentGuardPolicyNotFound", err)
	}
}

func TestAgentGuardPolicyRepositoryAllocatesVersionsAndPublishesWithDeliveries(t *testing.T) {
	db := setupAgentGuardPolicyTestDB(t)
	repo := NewAgentGuardPolicyRepository(db)
	ctx := context.Background()

	first := newAgentGuardPolicyDraft("versioned-agent-guard", 0)
	if err := repo.CreateDraft(ctx, first); err != nil {
		t.Fatalf("CreateDraft first: %v", err)
	}
	if first.Version != 1 {
		t.Fatalf("first version=%d, want 1", first.Version)
	}

	hostID := uuid.New()
	delivery := model.AgentGuardPolicyDelivery{
		ID:                  uuid.New(),
		HostID:              hostID,
		BundleVersion:       1001,
		BundleDigest:        "sha256:bundle-one",
		PolicyVersions:      datatypes.JSON([]byte(`[{"policy_key":"versioned-agent-guard","version":1}]`)),
		ProfileVersions:     datatypes.JSON([]byte(`[]`)),
		BuiltinRuleVersions: datatypes.JSON([]byte(`{}`)),
		Status:              "pending",
		GeneratedAt:         time.Now().UTC(),
	}
	published, deliveries, err := repo.PublishDraftWithDeliveries(ctx, first.ID, "admin", []model.AgentGuardPolicyDelivery{delivery})
	if err != nil {
		t.Fatalf("PublishDraftWithDeliveries: %v", err)
	}
	if published.Status != "published" || published.PublishedAt == nil || published.PublishedBy != "admin" {
		t.Fatalf("unexpected published policy: %#v", published)
	}
	if len(deliveries) != 1 || deliveries[0].Status != "pending" {
		t.Fatalf("unexpected deliveries: %#v", deliveries)
	}
	if _, _, err := repo.PublishDraftWithDeliveries(ctx, first.ID, "admin", nil); !errors.Is(err, ErrAgentGuardPolicyNotDraft) {
		t.Fatalf("duplicate publish error=%v, want ErrAgentGuardPolicyNotDraft", err)
	}

	second := newAgentGuardPolicyDraft("versioned-agent-guard", 0)
	if err := repo.CreateDraft(ctx, second); err != nil {
		t.Fatalf("CreateDraft second: %v", err)
	}
	if second.Version != 2 {
		t.Fatalf("second version=%d, want 2", second.Version)
	}
}

func newAgentGuardPolicyDraft(key string, version int64) *model.AgentGuardPolicy {
	return &model.AgentGuardPolicy{
		ID:                   uuid.New(),
		PolicyKey:            key,
		Version:              version,
		Name:                 "Agent Guard policy",
		Status:               "draft",
		Priority:             100,
		Targets:              datatypes.JSON([]byte(`{"agent_types":["codex","openclaw","hermes"]}`)),
		CollectionPolicy:     datatypes.JSON([]byte(`{"categories":["process","file","network","identity"]}`)),
		BuiltinRuleOverrides: datatypes.JSON([]byte(`[]`)),
		AtomicRules:          datatypes.JSON([]byte(`[]`)),
		CorrelationRules:     datatypes.JSON([]byte(`[]`)),
		AnalysisPolicy:       datatypes.JSON([]byte(`{"enabled":false}`)),
		EscapeRules:          datatypes.JSON([]byte(`[]`)),
		FreezeTimeoutSeconds: 300,
		CompiledPreview:      datatypes.JSON([]byte(`{}`)),
		CreatedBy:            "admin",
	}
}

func setupAgentGuardPolicyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE agent_guard_policies (
		id TEXT PRIMARY KEY,
		policy_key TEXT NOT NULL,
		version INTEGER NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL,
		priority INTEGER NOT NULL,
		targets TEXT NOT NULL,
		collection_policy TEXT NOT NULL,
		builtin_rule_overrides TEXT NOT NULL,
		atomic_rules TEXT NOT NULL,
		correlation_rules TEXT NOT NULL,
		analysis_policy TEXT NOT NULL,
		escape_rules TEXT NOT NULL,
		freeze_timeout_seconds INTEGER NOT NULL,
		compiled_preview TEXT NOT NULL,
		digest TEXT,
		created_by TEXT NOT NULL,
		published_by TEXT,
		published_at DATETIME,
		disabled_by TEXT,
		disabled_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		UNIQUE(policy_key, version)
	)`).Error; err != nil {
		t.Fatalf("create policy test schema: %v", err)
	}
	if err := db.Exec(`CREATE TABLE agent_guard_policy_deliveries (
		id TEXT PRIMARY KEY,
		host_id TEXT NOT NULL,
		bundle_version INTEGER NOT NULL,
		bundle_digest TEXT NOT NULL,
		policy_versions TEXT NOT NULL,
		profile_versions TEXT NOT NULL,
		builtin_rule_versions TEXT NOT NULL,
		status TEXT NOT NULL,
		capability_snapshot TEXT NOT NULL,
		coverage_level TEXT,
		error_code TEXT,
		error_message TEXT,
		generated_at DATETIME NOT NULL,
		dispatched_at DATETIME,
		received_at DATETIME,
		applied_at DATETIME,
		last_reported_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		UNIQUE(host_id, bundle_version)
	)`).Error; err != nil {
		t.Fatalf("create delivery test schema: %v", err)
	}
	return db
}

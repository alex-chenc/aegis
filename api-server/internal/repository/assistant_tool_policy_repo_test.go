package repository

import (
	"context"
	"testing"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAssistantToolPolicyUpsertFollowsChangedDefaultUntilAdministratorOverride(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE TABLE assistant_tool_policies (
			id TEXT PRIMARY KEY,
			tool_name TEXT NOT NULL UNIQUE,
			domain TEXT NOT NULL,
			operation TEXT NOT NULL,
			risk_level TEXT NOT NULL,
			description TEXT,
			args_summary TEXT,
			default_whitelisted BOOLEAN NOT NULL DEFAULT FALSE,
			whitelisted BOOLEAN NOT NULL DEFAULT FALSE,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			source TEXT NOT NULL DEFAULT 'builtin',
			updated_by TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewAssistantToolPolicyRepository(db)
	ctx := context.Background()

	untouched := &model.AssistantToolPolicy{
		ID:                 uuid.New(),
		ToolName:           "Example.Untouched",
		Domain:             "example",
		Operation:          "create",
		RiskLevel:          "medium",
		DefaultWhitelisted: false,
		Whitelisted:        false,
		Enabled:            true,
		Source:             "builtin",
	}
	if err := db.Create(untouched).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, &model.AssistantToolPolicy{
		ToolName:           untouched.ToolName,
		Domain:             untouched.Domain,
		Operation:          untouched.Operation,
		RiskLevel:          "low",
		DefaultWhitelisted: true,
		Whitelisted:        true,
		Enabled:            true,
		Source:             "builtin",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByToolName(ctx, untouched.ToolName)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DefaultWhitelisted || !got.Whitelisted {
		t.Fatalf("untouched built-in policy did not follow new default: %#v", got)
	}

	overridden := &model.AssistantToolPolicy{
		ID:                 uuid.New(),
		ToolName:           "Example.Overridden",
		Domain:             "example",
		Operation:          "create",
		RiskLevel:          "medium",
		DefaultWhitelisted: false,
		Whitelisted:        false,
		Enabled:            true,
		Source:             "builtin",
		UpdatedBy:          "administrator",
	}
	if err := db.Create(overridden).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, &model.AssistantToolPolicy{
		ToolName:           overridden.ToolName,
		Domain:             overridden.Domain,
		Operation:          overridden.Operation,
		RiskLevel:          "low",
		DefaultWhitelisted: true,
		Whitelisted:        true,
		Enabled:            true,
		Source:             "builtin",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindByToolName(ctx, overridden.ToolName)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DefaultWhitelisted || got.Whitelisted {
		t.Fatalf("administrator whitelist override was not preserved: %#v", got)
	}
}

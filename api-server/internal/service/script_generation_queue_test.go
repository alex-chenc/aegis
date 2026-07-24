package service

import (
	"testing"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestScriptGenerationQueueClaimsRulesAndReleasesFullQueue(t *testing.T) {
	logger.Logger = zap.NewNop()
	db, err := gorm.Open(sqlite.Open("file:script_generation_queue?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE TABLE aegis_rules (
			id TEXT PRIMARY KEY,
			template_id TEXT NOT NULL,
			title TEXT NOT NULL,
			check_content TEXT NOT NULL,
			fix_content TEXT NOT NULL,
			generated_check_script TEXT NULL,
			generated_fix_script TEXT NULL,
			check_script_version INTEGER DEFAULT 0,
			fix_script_version INTEGER DEFAULT 0,
			check_script_status TEXT DEFAULT 'pending',
			fix_script_status TEXT DEFAULT 'pending',
			check_script_error TEXT NULL,
			fix_script_error TEXT NULL,
			script_status TEXT DEFAULT 'pending',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatal(err)
	}
	first := model.AegisRule{ID: uuid.New(), TemplateID: uuid.New(), CheckScriptStatus: "pending", FixScriptStatus: "pending"}
	second := model.AegisRule{ID: uuid.New(), TemplateID: first.TemplateID, CheckScriptStatus: "pending", FixScriptStatus: "pending"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	ruleRepo := repository.NewRuleRepository(db)
	generator := &ScriptGenerationService{
		ruleRepo:      ruleRepo,
		generateQueue: make(chan GenerateTask, 1),
	}

	claimed, err := generator.queueScriptGeneration(first.ID, "CHECK")
	if err != nil || !claimed {
		t.Fatalf("first queue claim = %v, err=%v", claimed, err)
	}
	claimed, err = generator.queueScriptGeneration(first.ID, "CHECK")
	if err != nil || claimed {
		t.Fatalf("duplicate queue claim = %v, err=%v", claimed, err)
	}

	claimed, err = generator.queueScriptGeneration(second.ID, "CHECK")
	if err == nil || claimed {
		t.Fatalf("full queue claim = %v, err=%v", claimed, err)
	}
	firstStored, _ := ruleRepo.FindByID(first.ID)
	secondStored, _ := ruleRepo.FindByID(second.ID)
	if firstStored.CheckScriptStatus != "queued" {
		t.Fatalf("first status = %q, want queued", firstStored.CheckScriptStatus)
	}
	if secondStored.CheckScriptStatus != "pending" {
		t.Fatalf("second status = %q, want pending after queue release", secondStored.CheckScriptStatus)
	}
}

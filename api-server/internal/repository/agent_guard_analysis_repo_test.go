package repository

import (
	"context"
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentGuardAnalysisRepositoryNeverRewritesDeterministicFinding(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE agent_security_findings (
			id TEXT PRIMARY KEY, finding_key TEXT, host_id TEXT, instance_id TEXT, session_id TEXT,
			execution_unit_id TEXT, policy_id TEXT, policy_version INTEGER, title TEXT, severity TEXT,
			verdict TEXT, confidence REAL, status TEXT, decision_sources TEXT, rule_hits TEXT,
			evidence_event_ids TEXT, evidence_graph TEXT, attack_stages TEXT, summary TEXT,
			recommended_action TEXT, latest_analysis_id TEXT, handled_by TEXT, handled_note TEXT,
			handled_at DATETIME, first_observed_at DATETIME, last_observed_at DATETIME,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_security_analysis_runs (
			id TEXT PRIMARY KEY, finding_id TEXT, attempt INTEGER, status TEXT, provider TEXT,
			model TEXT, prompt_version TEXT, input_digest TEXT, evidence_event_ids TEXT,
			evidence_summary TEXT, output TEXT, verdict TEXT, attack_probability REAL,
			confidence REAL, error_code TEXT, error_message TEXT, requested_by TEXT,
			queued_at DATETIME, started_at DATETIME, completed_at DATETIME, created_at DATETIME
		)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	finding := model.AgentSecurityFinding{
		ID:                uuid.New(),
		FindingKey:        "rule-owned-finding",
		HostID:            uuid.New(),
		Title:             "Rule finding",
		Severity:          "high",
		Verdict:           "suspicious",
		Confidence:        0.91,
		Status:            "open",
		DecisionSources:   datatypes.JSON(`["rule"]`),
		RuleHits:          datatypes.JSON(`["AGB-BUILTIN-004"]`),
		EvidenceEventIDs:  datatypes.JSON(`["event-1"]`),
		EvidenceGraph:     datatypes.JSON(`{}`),
		AttackStages:      datatypes.JSON(`[]`),
		Summary:           "rule summary",
		RecommendedAction: "alert",
		FirstObservedAt:   now,
		LastObservedAt:    now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(&finding).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewAgentGuardAnalysisRepository(db)
	run := &model.AgentSecurityAnalysisRun{
		ID:               uuid.New(),
		FindingID:        finding.ID,
		Status:           model.AgentGuardAnalysisStatusPending,
		PromptVersion:    "test",
		InputDigest:      "sha256:test",
		EvidenceEventIDs: datatypes.JSON(`["event-1"]`),
		EvidenceSummary:  datatypes.JSON(`{}`),
		Output:           datatypes.JSON(`{}`),
		QueuedAt:         now,
		CreatedAt:        now,
	}
	if err := repo.CreatePending(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRunning(context.Background(), run.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkFailed(
		context.Background(),
		run.ID,
		model.AgentGuardAnalysisStatusInvalidOutput,
		"provider",
		"model",
		"invalid_output",
		"untrusted raw output must not be persisted",
		now,
	); err != nil {
		t.Fatal(err)
	}
	assertAgentGuardFindingRuleFields(t, db, finding)

	attackProbability, confidence := 0.99, 0.88
	run.Status = model.AgentGuardAnalysisStatusSucceeded
	run.Provider = "provider"
	run.Model = "model"
	run.Output = datatypes.JSON(`{"verdict":"malicious"}`)
	run.Verdict = "malicious"
	run.AttackProbability = &attackProbability
	run.Confidence = &confidence
	if err := repo.MarkSucceeded(context.Background(), run, now); err != nil {
		t.Fatal(err)
	}
	assertAgentGuardFindingRuleFields(t, db, finding)
	var current model.AgentSecurityFinding
	if err := db.First(&current, "id = ?", finding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.LatestAnalysisID == nil || *current.LatestAnalysisID != run.ID {
		t.Fatalf("latest analysis link=%v, want %s", current.LatestAnalysisID, run.ID)
	}
}

func assertAgentGuardFindingRuleFields(
	t *testing.T,
	db *gorm.DB,
	expected model.AgentSecurityFinding,
) {
	t.Helper()
	var current model.AgentSecurityFinding
	if err := db.First(&current, "id = ?", expected.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Verdict != expected.Verdict ||
		current.Severity != expected.Severity ||
		current.Confidence != expected.Confidence ||
		current.Summary != expected.Summary ||
		current.RecommendedAction != expected.RecommendedAction {
		t.Fatalf("analysis rewrote deterministic finding: %#v", current)
	}
}

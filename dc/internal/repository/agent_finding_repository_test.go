package repository

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dc/internal/model"
	"dc/internal/pipeline"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMergeAgentFindingIsIdempotentAndNeverRegressesSeverityOrWorkflow(t *testing.T) {
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	existing := &model.AgentSecurityFinding{
		ID: uuid.New(), FindingKey: "correlation:v1:key:anchor", HostID: uuid.New(),
		Severity: "critical", Verdict: "malicious", Confidence: 0.94, Status: "investigating",
		EvidenceEventIDs: json.RawMessage(`["00000000-0000-4000-8000-000000000001"]`),
		FirstObservedAt:  now, LastObservedAt: now.Add(time.Second),
	}
	incoming := &model.AgentSecurityFinding{
		ID: existing.ID, FindingKey: existing.FindingKey, HostID: existing.HostID,
		Severity: "medium", Verdict: "inconclusive", Confidence: 0.62, Status: "open",
		EvidenceEventIDs: json.RawMessage(`[
			"00000000-0000-4000-8000-000000000001",
			"00000000-0000-4000-8000-000000000002"
		]`),
		FirstObservedAt: now.Add(-time.Second), LastObservedAt: now.Add(2 * time.Second),
	}
	updates, changed, err := mergeAgentFinding(existing, incoming)
	if err != nil {
		t.Fatalf("mergeAgentFinding: %v", err)
	}
	if !changed || updates["severity"] != "critical" || updates["verdict"] != "malicious" ||
		updates["status"] != nil {
		t.Fatalf("updates = %#v", updates)
	}
	var evidence []string
	if err := json.Unmarshal(updates["evidence_event_ids"].(json.RawMessage), &evidence); err != nil ||
		len(evidence) != 2 {
		t.Fatalf("evidence=%v err=%v", evidence, err)
	}

	merged := *existing
	merged.EvidenceEventIDs = updates["evidence_event_ids"].(json.RawMessage)
	merged.FirstObservedAt = updates["first_observed_at"].(time.Time)
	merged.LastObservedAt = updates["last_observed_at"].(time.Time)
	_, changed, err = mergeAgentFinding(&merged, &merged)
	if err != nil || changed {
		t.Fatalf("idempotent merge changed=%v err=%v", changed, err)
	}
}

func TestRemoteEvidenceRepositoryQueryIsExactBoundedAndCrossHost(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	now := time.Now().UTC()
	selector := pipeline.RemoteEvidenceSelector{
		EventID: uuid.NewString(), HostID: uuid.New(), ExecutionUnitID: uuid.New(),
		CorrelationHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ToolOccurredAt:  now,
	}
	query, valid, err := buildRemoteBehaviorEvidenceQuery(db, []pipeline.RemoteEvidenceSelector{selector}, 5*time.Minute)
	if err != nil || len(valid) != 1 {
		t.Fatalf("build query valid=%#v err=%v", valid, err)
	}
	var events []*model.AgentBehaviorEvent
	statement := query.Limit(len(valid)).Find(&events).Statement
	sql := statement.SQL.String()
	for _, required := range []string{
		"raw_event_id =", "host_id =", "execution_unit_id =", "occurred_at BETWEEN",
		"correlation_token_hash", "collection ->> 'source'", "attribution_confidence", "LIMIT",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("remote query missing %q: %s", required, sql)
		}
	}
	if strings.Contains(sql, "agent_official") || strings.Contains(sql, "adapter_hook") {
		t.Fatalf("tool source leaked into OS sensor whitelist: %s", sql)
	}

	bad := selector
	bad.EventID = "not-a-uuid"
	if _, _, err := buildRemoteBehaviorEvidenceQuery(db, []pipeline.RemoteEvidenceSelector{bad}, 5*time.Minute); err == nil {
		t.Fatal("repository accepted an unbounded selector")
	}
	tooMany := make([]pipeline.RemoteEvidenceSelector, 129)
	for index := range tooMany {
		tooMany[index] = selector
		tooMany[index].EventID = uuid.NewString()
	}
	if _, _, err := buildRemoteBehaviorEvidenceQuery(db, tooMany, 5*time.Minute); err == nil {
		t.Fatal("repository accepted selector overflow")
	}
}

func TestFindingEvidenceIDsRejectInvalidOrEmptyEvidence(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`[]`),
		json.RawMessage(`["not-a-uuid"]`),
		json.RawMessage(`["00000000-0000-4000-8000-000000000001","00000000-0000-4000-8000-000000000001"]`),
	} {
		if _, err := findingEvidenceIDs(raw); err == nil {
			t.Fatalf("findingEvidenceIDs(%s) accepted invalid evidence", raw)
		}
	}
}

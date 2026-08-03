package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const testAgentGuardDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestProjectAgentGuardToolEvidenceRequiresTrustedProofAndMatchingSession(t *testing.T) {
	sessionID := uuid.New()
	session := &model.AgentBehaviorSession{
		ID: sessionID, Source: model.AgentGuardSessionSourceAdapterHook,
		Confidence: "confirmed", CorrelationTokenHash: testAgentGuardDigest,
	}
	event := model.AgentBehaviorEvent{
		SessionID: &sessionID, Category: "tool", Operation: "tool_call_started",
		ProcessName: "bash", ResourceType: "tool", ResourceIdentity: "filesystem.read",
		CorrelationID: testAgentGuardDigest, Decision: "audit", Severity: "info",
		OccurredAt: time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC),
		Collection: datatypes.JSON(`{"source":"adapter_hook"}`),
		Evidence: datatypes.JSON(`{
			"correlation_token_hash":"` + testAgentGuardDigest + `",
			"trusted_proof":{"verified":true,"verifier":"ed25519","proof_digest":"` + testAgentGuardDigest + `","issued_at":"2026-08-03T10:00:00Z"}
		}`),
	}

	projection := ProjectAgentGuardToolEvidence(event, session)
	if projection.Trust == nil || projection.Trust.ToolSemantics != AgentGuardToolSemanticsTrusted ||
		projection.Trust.Source != model.AgentGuardSessionSourceAdapterHook ||
		!projection.Trust.ProofVerified || projection.Trust.Correlation != "matched" ||
		projection.Collection != nil {
		t.Fatalf("unexpected trusted projection: %#v", projection)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"correlation_token_hash", "proof_digest", "external_session_id", testAgentGuardDigest, "ed25519"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("safe projection leaks %q: %s", secret, encoded)
		}
	}
}

func TestProjectAgentGuardToolEvidenceFailsClosed(t *testing.T) {
	sessionID := uuid.New()
	baseSession := model.AgentBehaviorSession{
		ID: sessionID, Source: model.AgentGuardSessionSourceAgentOfficial,
		Confidence: "confirmed", CorrelationTokenHash: testAgentGuardDigest,
	}
	baseEvent := model.AgentBehaviorEvent{
		SessionID: &sessionID, Category: "tool", Operation: "tool_call_completed",
		ResourceType: "tool", ResourceIdentity: "filesystem.read", CorrelationID: testAgentGuardDigest,
		Decision: "audit", Severity: "info", OccurredAt: time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC),
		Collection: datatypes.JSON(`{"source":"agent_official"}`),
		Evidence: datatypes.JSON(`{
			"correlation_token_hash":"` + testAgentGuardDigest + `",
			"trusted_proof":{"verified":true,"verifier":"ed25519","proof_digest":"` + testAgentGuardDigest + `","issued_at":"2026-08-03T10:00:00Z"}
		}`),
	}

	tests := []struct {
		name    string
		event   model.AgentBehaviorEvent
		session *model.AgentBehaviorSession
	}{
		{name: "no session", event: baseEvent},
		{name: "process inference forbidden", event: func() model.AgentBehaviorEvent { value := baseEvent; value.Category = "process"; return value }(), session: &baseSession},
		{name: "unknown tool operation", event: func() model.AgentBehaviorEvent {
			value := baseEvent
			value.Operation = "exec"
			return value
		}(), session: &baseSession},
		{name: "derived source", event: func() model.AgentBehaviorEvent {
			value := baseEvent
			value.Collection = datatypes.JSON(`{"source":"execution_unit"}`)
			return value
		}(), session: &baseSession},
		{name: "probable session is not semantic proof", event: baseEvent, session: func() *model.AgentBehaviorSession {
			value := baseSession
			value.Confidence = "probable"
			return &value
		}()},
		{name: "unverified proof", event: func() model.AgentBehaviorEvent {
			value := baseEvent
			value.Evidence = datatypes.JSON(`{"correlation_token_hash":"` + testAgentGuardDigest + `","trusted_proof":{"verified":false}}`)
			return value
		}(), session: &baseSession},
		{name: "token mismatch", event: func() model.AgentBehaviorEvent {
			value := baseEvent
			value.Evidence = datatypes.JSON(`{"correlation_token_hash":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","trusted_proof":{"verified":true,"verifier":"ed25519","proof_digest":"` + testAgentGuardDigest + `","issued_at":"2026-08-03T10:00:00Z"}}`)
			return value
		}(), session: &baseSession},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := ProjectAgentGuardToolEvidence(test.event, test.session)
			if test.event.Category != "tool" {
				if projection.Trust != nil {
					t.Fatalf("non-tool event received trust projection: %#v", projection)
				}
				return
			}
			if projection.Trust == nil || projection.Trust.ToolSemantics != AgentGuardToolSemanticsUnobservable ||
				projection.Trust.Source != "" || projection.Trust.ProofVerified || projection.Trust.Correlation != "unmatched" ||
				projection.Collection == nil || len(projection.Collection.Limitations) != 1 {
				t.Fatalf("event did not fail closed: %#v", projection)
			}
		})
	}
}

func TestProjectAgentGuardRemoteVisibilityRequiresExplicitSensorProof(t *testing.T) {
	unit := model.AgentExecutionUnit{
		UnitType: "remote_sandbox", RemoteBackend: "ssh",
		RemoteExecutionID: "process-name-is-not-proof", CoverageLevel: model.AgentGuardCoverageRemoteUnobservable,
		IsolationActual: datatypes.JSON(`{"remote_sensor_verified":true}`),
	}
	projection := ProjectAgentGuardRemoteVisibility(unit)
	if projection.Trust == nil || projection.Trust.RemoteVisibility != AgentGuardRemoteUnobservable || projection.Collection == nil {
		t.Fatalf("remote unobservable coverage must win: %#v", projection)
	}

	unit.CoverageLevel = model.AgentGuardCoverageMonitorOnly
	projection = ProjectAgentGuardRemoteVisibility(unit)
	if projection.Trust == nil || projection.Trust.RemoteVisibility != AgentGuardRemoteVisibilityTrusted || projection.Collection != nil {
		t.Fatalf("explicit remote sensor proof not projected: %#v", projection)
	}
}

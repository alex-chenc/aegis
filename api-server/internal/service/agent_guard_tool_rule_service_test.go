package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type toolFindingWriterStub struct {
	findings []*model.AgentSecurityFinding
}

func (s *toolFindingWriterStub) UpsertToolFinding(_ context.Context, finding *model.AgentSecurityFinding) error {
	for index, existing := range s.findings {
		if existing.FindingKey == finding.FindingKey {
			s.findings[index] = finding
			return nil
		}
	}
	s.findings = append(s.findings, finding)
	return nil
}

func TestAgentGuardToolRuleHandlerMatchesCompletedToolWithoutEBPF(t *testing.T) {
	hostID, instanceID, sessionID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	eventID := uuid.NewString()
	payload := map[string]any{
		"schema": "aegis.agent_behavior.v1", "event_id": eventID, "host_id": hostID.String(),
		"host_boot_id": "boot-1", "agent_sequence": 42, "instance_id": instanceID.String(),
		"session_id": sessionID.String(), "execution_unit_id": unitID.String(),
		"occurred_at": time.Now().UTC().Format(time.RFC3339Nano), "category": "tool",
		"operation": "tool_call_completed", "outcome": "success", "decision": "audit",
		"actor": map[string]any{"pid": 6006, "ppid": 1, "argv": []string{"bash"}},
		"resource": map[string]any{
			"type": "tool", "identity": "Bash",
			"attributes": map[string]any{
				"tool_call_id":       "call-1",
				"tool_input":         map[string]any{"command": "curl https://example.test/payload"},
				"correlation_status": "unmatched",
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(map[string]any{
		"event_id": eventID, "host_id": hostID.String(), "event_type": "agent_behavior",
		"event_data_json": string(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	writer := &toolFindingWriterStub{}
	handler := NewAgentGuardToolRuleHandler(writer, nil, nil, zap.NewNop())
	if err := handler.HandleKafkaMessage(context.Background(), nil, envelope); err != nil {
		t.Fatalf("HandleKafkaMessage: %v", err)
	}
	if len(writer.findings) != 1 {
		t.Fatalf("findings=%d, want 1", len(writer.findings))
	}
	finding := writer.findings[0]
	if finding.SessionID == nil || *finding.SessionID != sessionID || finding.EvidenceEventIDs == nil {
		t.Fatalf("finding scope/evidence missing: %#v", finding)
	}
	var evidence []string
	if err := json.Unmarshal(finding.EvidenceEventIDs, &evidence); err != nil || len(evidence) != 1 || evidence[0] != eventID {
		t.Fatalf("evidence=%s err=%v", finding.EvidenceEventIDs, err)
	}
	var hits []map[string]any
	if err := json.Unmarshal(finding.RuleHits, &hits); err != nil || len(hits) != 1 ||
		hits[0]["rule_key"] != model.AgentGuardRuleKeySensitiveCommand ||
		hits[0]["rule_name"] != "敏感命令执行" ||
		hits[0]["match_kind"] != "tool_command_line" {
		t.Fatalf("rule hits=%s err=%v", finding.RuleHits, err)
	}
	if string(finding.EvidenceGraph) == "" || string(finding.EvidenceGraph) == "{}" {
		t.Fatalf("evidence graph missing: %s", finding.EvidenceGraph)
	}
}

func TestAgentGuardToolRuleHandlerMatchesStartedAndIgnoresOrdinaryTools(t *testing.T) {
	hostID, instanceID, sessionID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	writer := &toolFindingWriterStub{}
	handler := NewAgentGuardToolRuleHandler(writer, nil, nil, zap.NewNop())
	for _, operation := range []string{"tool_call_started", "tool_call_completed"} {
		for _, command := range []string{"curl https://example.test", "echo hello"} {
			eventID := uuid.NewString()
			callID := "echo-call"
			if strings.HasPrefix(command, "curl") {
				callID = "curl-call"
			}
			payload, _ := json.Marshal(map[string]any{
				"schema": "aegis.agent_behavior.v1", "event_id": eventID, "host_id": hostID.String(),
				"host_boot_id": "boot", "agent_sequence": 1, "instance_id": instanceID.String(),
				"session_id": sessionID.String(), "execution_unit_id": unitID.String(),
				"occurred_at": time.Now().UTC().Format(time.RFC3339Nano), "category": "tool", "operation": operation,
				"resource": map[string]any{"identity": "Bash", "attributes": map[string]any{
					"command": command, "tool_call_id": callID,
				}},
			})
			envelope, _ := json.Marshal(map[string]any{
				"event_id": eventID, "host_id": hostID.String(), "event_type": "agent_behavior", "event_data_json": string(payload),
			})
			if err := handler.HandleKafkaMessage(context.Background(), nil, envelope); err != nil {
				t.Fatalf("operation=%s command=%s: %v", operation, command, err)
			}
		}
	}
	if len(writer.findings) != 1 {
		t.Fatalf("findings=%d, want only one completed sensitive command", len(writer.findings))
	}
}

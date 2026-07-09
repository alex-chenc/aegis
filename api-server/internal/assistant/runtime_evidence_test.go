package assistant

import (
	"context"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

func TestRuntimeEvidenceRejectsOnlineHostContradiction(t *testing.T) {
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{{
				Observation: &agentruntime.Observation{
					CallID:   "call-hosts",
					ToolName: "Vulnerability.AffectedHosts",
					Status:   agentruntime.ToolCallSuccess,
					Content:  `{"total":1,"data":[{"id":"host-1","online":true}]}`,
				},
			}},
		}},
	}
	ledger := buildRuntimeEvidenceLedger(result)
	if ledger.OnlineHostCount != 1 {
		t.Fatalf("online host count = %d", ledger.OnlineHostCount)
	}
	conflicts := validateRuntimeEvidenceConsistency("当前环境没有在线主机。", ledger)
	if !containsDecisionString(conflicts, "online_hosts_contradiction") {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	fallback := buildEvidenceGroundedFallback(ledger)
	if !strings.Contains(fallback, "在线目标主机：1 台") {
		t.Fatalf("fallback = %q", fallback)
	}
}

func TestRuntimeEvidenceRejectsUnprovenDispatch(t *testing.T) {
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{{
				Observation: &agentruntime.Observation{
					CallID:   "call-vuln",
					ToolName: "Vulnerability.List",
					Status:   agentruntime.ToolCallSuccess,
					Content:  `{"total":1,"data":[{"id":"vuln-1"}]}`,
				},
			}},
		}},
	}
	ledger := buildRuntimeEvidenceLedger(result)
	conflicts := validateRuntimeEvidenceConsistency("POC 已下发成功。", ledger)
	if !containsDecisionString(conflicts, "dispatch_without_task_group") {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestRuntimeEvidenceAcceptsTaskGroupBackedDispatch(t *testing.T) {
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{{
				Observation: &agentruntime.Observation{
					CallID:   "call-execute",
					ToolName: "Vulnerability.Script.Execute",
					Status:   agentruntime.ToolCallSuccess,
					Content:  `{"task_group_id":"task-group-1"}`,
				},
			}},
		}},
	}
	ledger := buildRuntimeEvidenceLedger(result)
	if conflicts := validateRuntimeEvidenceConsistency("POC 已下发成功。", ledger); len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %#v", conflicts)
	}
}

func TestRuntimeEvidenceCountsCustomCVECompletion(t *testing.T) {
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{{
				Observation: &agentruntime.Observation{
					CallID:   "call-custom-cve",
					ToolName: "Vulnerability.CustomQuery.Status",
					Status:   agentruntime.ToolCallSuccess,
					Content:  `{"status":"success","result_vulnerability_id":"vuln-1"}`,
				},
			}},
		}},
	}
	ledger := buildRuntimeEvidenceLedger(result)
	if ledger.VulnerabilityCount != 1 {
		t.Fatalf("vulnerability count = %d, want 1", ledger.VulnerabilityCount)
	}
}

func TestPersistRuntimeToolCallRecordsKeepsPreGatewayValidationFailure(t *testing.T) {
	repo := &fakeToolCallRepo{}
	orchestrator := &Orchestrator{toolCallRepo: repo}
	startedAt := time.Now().Add(-25 * time.Millisecond)
	orchestrator.persistRuntimeToolCallRecords(context.Background(), "session-1", "message-1", &agentruntime.TaskResult{
		ToolCalls: []agentruntime.ToolCallRecord{{
			CallID:          "call-validation",
			ToolName:        "Vulnerability.Script.Generate",
			ArgsSummary:     `{"cve_id":"CVE-2021-45340"}`,
			Status:          agentruntime.ToolCallFailed,
			ErrorMessage:    "script_type is required",
			ValidationStage: "arguments",
			StartedAt:       startedAt,
			EndedAt:         startedAt.Add(25 * time.Millisecond),
		}},
	})

	if len(repo.calls) != 1 {
		t.Fatalf("created calls = %d, want 1", len(repo.calls))
	}
	call := repo.calls[0]
	if call.MessageID != "message-1" || call.Status != "failed" {
		t.Fatalf("unexpected persisted call: %#v", call)
	}
	if !strings.Contains(call.ArgsSummary, "validation_stage=arguments") {
		t.Fatalf("args summary = %q", call.ArgsSummary)
	}
	if call.DurationMs != 25 {
		t.Fatalf("duration = %d, want 25", call.DurationMs)
	}

	messageCalls := orchestrator.toolCallsForMessage(context.Background(), "session-1", "message-1")
	if len(messageCalls) == 0 {
		t.Fatal("expected tool calls to be attached to the assistant message")
	}
}

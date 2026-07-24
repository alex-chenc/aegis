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
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "get_vulnerability_affected_hosts",
						OperationStatus: agentruntime.OperationSucceeded,
						Terminal:        true,
						Facts: []map[string]any{{
							"kind": "host_online",
							"id":   "host-1",
						}},
					},
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

func TestBuildFailedGoalFallbackNeverReportsCompleted(t *testing.T) {
	fallback := buildFailedGoalFallback(runtimeEvidenceLedger{
		ActualToolNames: []string{"Host.Resolve"},
		FailedToolNames: []string{"Host.Resolve"},
	})
	if !strings.Contains(fallback, "任务未完成") || strings.Contains(strings.ToLower(fallback), "completed") {
		t.Fatalf("failed-goal fallback = %q", fallback)
	}
	if !strings.Contains(fallback, "未下发任务") {
		t.Fatalf("fallback must state missing task dispatch evidence: %q", fallback)
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
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "list_vulnerabilities",
						OperationStatus: agentruntime.OperationSucceeded,
						Terminal:        true,
						Facts: []map[string]any{{
							"kind": "vulnerability_record",
							"id":   "vuln-1",
						}},
					},
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
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "execute_vulnerability_host_scripts",
						OperationStatus: agentruntime.OperationSucceeded,
						Terminal:        true,
						SideEffects: []map[string]any{{
							"type":          "side_effect",
							"task_group_id": "task-group-1",
						}},
					},
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
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "get_custom_cve_query_status",
						OperationStatus: agentruntime.OperationSucceeded,
						Terminal:        true,
						Artifacts: []map[string]any{{
							"type":                    "artifact",
							"result_vulnerability_id": "vuln-1",
						}},
					},
				},
			}},
		}},
	}
	ledger := buildRuntimeEvidenceLedger(result)
	if ledger.VulnerabilityCount != 1 {
		t.Fatalf("vulnerability count = %d, want 1", ledger.VulnerabilityCount)
	}
}

func TestRuntimeEvidenceRejectsGeneratedClaimForAcceptedOperation(t *testing.T) {
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{{
				Observation: &agentruntime.Observation{
					CallID:   "call-generate",
					ToolName: "Vulnerability.Script.Generate",
					Status:   agentruntime.ToolCallSuccess,
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "generate_vulnerability_script",
						OperationStatus: agentruntime.OperationAccepted,
						Terminal:        false,
					},
				},
			}},
		}},
	}
	ledger := buildRuntimeEvidenceLedger(result)
	conflicts := validateRuntimeEvidenceConsistency("修复脚本生成成功。", ledger)
	if !containsDecisionString(conflicts, "script_generated_without_terminal_evidence") {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestPersistRuntimeToolCallRecordsSkipsPreGatewayValidationFailure(t *testing.T) {
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

	if len(repo.calls) != 0 {
		t.Fatalf("pre-gateway validation attempts must not be durable tool calls: %#v", repo.calls)
	}

	messageCalls := orchestrator.toolCallsForMessage(context.Background(), "session-1", "message-1")
	if len(messageCalls) != 0 {
		t.Fatalf("pre-gateway validation attempts must not be attached to the assistant message: %s", messageCalls)
	}
}

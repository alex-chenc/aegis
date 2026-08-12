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

func TestRuntimeEvidenceCountsResolvedOnlineHostAndExcludesRejectedCandidate(t *testing.T) {
	result := &agentruntime.TaskResult{
		ToolCalls: []agentruntime.ToolCallRecord{
			{CallID: "call-host", ToolName: "Host.Resolve", Status: agentruntime.ToolCallSuccess},
			{CallID: "call-rejected", ToolName: "Baseline.Compliance.Run", Status: agentruntime.ToolCallFailed, ValidationStage: "step_tool_scope"},
		},
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{
				{Observation: &agentruntime.Observation{
					CallID:   "call-host",
					ToolName: "Host.Resolve",
					Status:   agentruntime.ToolCallSuccess,
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "resolve_hosts",
						OperationStatus: agentruntime.OperationSucceeded,
						Terminal:        true,
						Facts: []map[string]any{{
							"kind":  "host_resolved",
							"id":    "host-1",
							"state": "online",
						}},
					},
				}},
				{Observation: &agentruntime.Observation{
					CallID:   "call-rejected",
					ToolName: "Baseline.Compliance.Run",
					Status:   agentruntime.ToolCallFailed,
					Error:    "tool is outside step allowlist",
				}},
			},
		}},
	}

	ledger := buildRuntimeEvidenceLedger(result)
	if ledger.OnlineHostCount != 1 {
		t.Fatalf("resolved online host count = %d, want 1", ledger.OnlineHostCount)
	}
	if len(ledger.ActualToolNames) != 1 || ledger.ActualToolNames[0] != "Host.Resolve" {
		t.Fatalf("pre-gateway candidate leaked into actual tools: %#v", ledger.ActualToolNames)
	}
	if len(ledger.FailedToolNames) != 0 || len(ledger.Calls) != 1 {
		t.Fatalf("pre-gateway candidate leaked into evidence: %#v", ledger)
	}
}

func TestBuildFailedGoalFallbackNeverReportsCompleted(t *testing.T) {
	fallback := buildFailedGoalFallback(runtimeEvidenceLedger{
		ActualToolNames:          []string{"Host.Resolve"},
		FailedToolNames:          []string{"Host.Resolve"},
		VulnerabilityWorkflow:    true,
		VulnerabilityRemediation: true,
	})
	if !strings.Contains(fallback, "任务未完成") || strings.Contains(strings.ToLower(fallback), "completed") {
		t.Fatalf("failed-goal fallback = %q", fallback)
	}
	if !strings.Contains(fallback, "未下发任务") {
		t.Fatalf("fallback must state missing task dispatch evidence: %q", fallback)
	}
}

func TestRuntimeEvidenceTracksMCPOnboardingApproval(t *testing.T) {
	jobID := "8c63f281-4c17-4c13-ba6a-effc44d36c66"
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{{Observation: &agentruntime.Observation{
				CallID:   "call-mcp-status",
				ToolName: "MCP.Aggregation.Server.Onboarding.Get",
				Status:   agentruntime.ToolCallSuccess,
				Content:  `{"job_id":"` + jobID + `","status":"awaiting_approval"}`,
				Outcome: &agentruntime.ToolOutcome{
					Capability:      "get_mcp_onboarding_status",
					OperationStatus: agentruntime.OperationSkipped,
					Terminal:        true,
				},
			}}},
		}},
	}

	ledger := buildRuntimeEvidenceLedger(result)
	if !ledger.MCPOnboardingAwaitingApproval || ledger.MCPOnboardingJobID != jobID || ledger.MCPOnboardingStatus != "awaiting_approval" {
		t.Fatalf("MCP onboarding approval evidence = %#v", ledger)
	}
	answer := buildMCPOnboardingAwaitingApprovalAnswer(ledger, "zh-CN")
	if !strings.Contains(answer, "等待平台审批") || !strings.Contains(answer, jobID) {
		t.Fatalf("approval answer = %q", answer)
	}
}

func TestBuildFailedGoalFallbackForModelOnlyRunIsNotScanSpecific(t *testing.T) {
	fallback := buildFailedGoalFallback(runtimeEvidenceLedger{})
	if !strings.Contains(fallback, "分析未完成") {
		t.Fatalf("model-only fallback = %q", fallback)
	}
	if strings.Contains(fallback, "扫描、修复或验证") {
		t.Fatalf("model-only fallback must not claim a scan workflow: %q", fallback)
	}
}

func TestRuntimeEvidenceReportsRunningVulnerabilityScanWithoutRemediationClaims(t *testing.T) {
	scanID := "92d158f6-e8e8-460d-9e61-43fdd7533933"
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{{
				Observation: &agentruntime.Observation{
					CallID:   "call-scan-status",
					ToolName: "Vulnerability.Scan.Status",
					Status:   agentruntime.ToolCallSuccess,
					Content:  `{"scan":{"scan_id":"` + scanID + `","status":"analyzing","progress":84}}`,
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "get_vulnerability_scan_status",
						OperationStatus: agentruntime.OperationRunning,
						Terminal:        false,
						OperationRef:    map[string]string{"type": "get_vulnerability_scan_status", "scan_id": scanID},
					},
				},
			}},
		}},
	}

	ledger := buildRuntimeEvidenceLedger(result)
	if !ledger.VulnerabilityAssessment || ledger.VulnerabilityRemediation {
		t.Fatalf("vulnerability workflow classification is wrong: %#v", ledger)
	}
	if got := ledger.VulnerabilityScanIDs; len(got) != 1 || got[0] != scanID {
		t.Fatalf("scan IDs = %#v, want %s", got, scanID)
	}
	if ledger.VulnerabilityScanStatus != "analyzing" || ledger.VulnerabilityScanProgress != 84 {
		t.Fatalf("scan status/progress = %s/%d", ledger.VulnerabilityScanStatus, ledger.VulnerabilityScanProgress)
	}

	fallback := buildFailedGoalFallback(ledger)
	for _, forbidden := range []string{"脚本", "任务组", "未下发任务"} {
		if strings.Contains(fallback, forbidden) {
			t.Fatalf("scan-only fallback contains remediation claim %q: %q", forbidden, fallback)
		}
	}
	if !strings.Contains(fallback, scanID) || !strings.Contains(fallback, "84%") || !strings.Contains(fallback, "仍在后台运行") {
		t.Fatalf("scan-only fallback does not preserve running evidence: %q", fallback)
	}
}

func TestRuntimeEvidenceRejectsOmittedCompletedScanAndWeakPasswordResults(t *testing.T) {
	scanID := "19b7a5cd-5360-474e-a3fa-fd1af3745a0b"
	taskID1 := "4dd7615c-b447-498a-ac7a-ac660a3d5e2b"
	taskID2 := "994c18fe-bb55-490b-a140-544db812be32"
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{{
			ReactTurns: []agentruntime.ReactTurn{
				{Observation: &agentruntime.Observation{
					CallID:   "call-vulnerability-status",
					ToolName: "Vulnerability.Scan.Status",
					Status:   agentruntime.ToolCallSuccess,
					Content:  `{"scan":{"scan_id":"` + scanID + `","status":"completed","progress":100,"found_vulns":3}}`,
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "get_vulnerability_scan_status",
						OperationStatus: agentruntime.OperationSucceeded,
						Terminal:        true,
						OperationRef:    map[string]string{"scan_id": scanID},
					},
				}},
				{Observation: &agentruntime.Observation{
					CallID:   "call-weak-scan",
					ToolName: "Credential.WeakPassword.Scan",
					Status:   agentruntime.ToolCallSuccess,
					Content:  `{"status":"pending"}`,
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "weak_password_scan",
						OperationStatus: agentruntime.OperationAccepted,
						Terminal:        false,
						Facts: []map[string]interface{}{
							{"kind": "task_resolved", "id": taskID1},
							{"kind": "task_resolved", "id": taskID2},
						},
					},
				}},
				{Observation: &agentruntime.Observation{
					CallID:   "call-weak-progress",
					ToolName: "Credential.WeakPassword.QueryProgress",
					Status:   agentruntime.ToolCallSuccess,
					Content: `{"status":"partial_failed","matched_findings":2,"tasks":[` +
						`{"task_id":"` + taskID1 + `","task_progress":{"status":"completed","matched_findings":2}},` +
						`{"task_id":"` + taskID2 + `","task_progress":{"status":"failed","matched_findings":0}}]}`,
					Outcome: &agentruntime.ToolOutcome{
						Capability:      "weak_password_progress",
						OperationStatus: agentruntime.OperationSucceeded,
						Terminal:        true,
					},
				}},
			},
		}},
	}

	ledger := buildRuntimeEvidenceLedger(result)
	if !ledger.VulnerabilityScanFoundCountKnown || ledger.VulnerabilityScanFoundCount != 3 {
		t.Fatalf("vulnerability count evidence = known:%v count:%d", ledger.VulnerabilityScanFoundCountKnown, ledger.VulnerabilityScanFoundCount)
	}
	if !ledger.WeakPasswordWorkflow || !ledger.WeakPasswordTerminal {
		t.Fatalf("weak-password evidence was not classified: %#v", ledger)
	}
	if ledger.WeakPasswordTaskTotal != 2 || ledger.WeakPasswordTaskCompleted != 1 || ledger.WeakPasswordTaskFailed != 1 {
		t.Fatalf("weak-password task summary is wrong: %#v", ledger)
	}
	if !ledger.WeakPasswordFindingCountKnown || ledger.WeakPasswordFindingCount != 2 {
		t.Fatalf("weak-password finding evidence = known:%v count:%d", ledger.WeakPasswordFindingCountKnown, ledger.WeakPasswordFindingCount)
	}
	if containsDecisionString(ledger.FailedToolNames, "Credential.WeakPassword.QueryProgress") {
		t.Fatalf("terminal partial failure must not be classified as a query-tool failure: %#v", ledger.FailedToolNames)
	}

	conflicts := validateRuntimeEvidenceConsistency("资产采集已完成。", ledger)
	for _, code := range []string{"vulnerability_scan_evidence_omitted", "weak_password_evidence_omitted"} {
		if !containsDecisionString(conflicts, code) {
			t.Fatalf("missing completeness conflict %s: %#v", code, conflicts)
		}
	}

	fallback := buildEvidenceGroundedFallback(ledger)
	for _, expected := range []string{scanID, "发现漏洞：3 个", taskID1, "完成 1", "失败 1", "弱口令命中：2 条"} {
		if !strings.Contains(fallback, expected) {
			t.Fatalf("evidence fallback missing %q: %q", expected, fallback)
		}
	}
}

func TestBuildFailedGoalFallbackReportsRunningAssetCollection(t *testing.T) {
	fallback := buildFailedGoalFallback(runtimeEvidenceLedger{
		ActualToolNames:         []string{"Asset.Collection.Get", "Asset.Collection.Trigger"},
		AssetCollectionTaskIDs:  []string{"asset-task-1"},
		AssetCollectionTerminal: false,
	})

	if !strings.Contains(fallback, "asset-task-1") {
		t.Fatalf("asset task ID missing from fallback: %q", fallback)
	}
	if !strings.Contains(fallback, "仍在后台运行") {
		t.Fatalf("running asset task must be reported truthfully: %q", fallback)
	}
	if strings.Contains(fallback, "未下发任务") {
		t.Fatalf("created asset task must not be reported as undispatched: %q", fallback)
	}
}

func TestDetectionPackageActivationPauseRequiresNoFailedPackageStage(t *testing.T) {
	authorization := &ToolExecutionPlan{RequiredOutcome: "detection_package_enabled"}
	failedBuild := runtimeEvidenceLedger{
		ActualToolNames:        []string{"Package.Draft.Generate", "Package.Build.Start"},
		FailedToolNames:        []string{"Package.Build.Start"},
		DetectionPackageID:     "package-1",
		DetectionPackageStatus: "draft",
	}
	if shouldPauseDetectionPackageBeforeActivation(authorization, failedBuild, agentruntime.GoalPartiallySucceeded) {
		t.Fatal("failed package build must not be converted into an activation continuation")
	}
	if !hasFailedDetectionPackageStage(failedBuild) {
		t.Fatal("failed package build was not recognized as a terminal lifecycle failure")
	}

	awaitingReview := runtimeEvidenceLedger{
		ActualToolNames:        []string{"Package.Draft.Generate", "Package.Build.Start", "Package.Build.Status"},
		DetectionPackageID:     "package-1",
		DetectionPackageStatus: "awaiting_review",
	}
	if !shouldPauseDetectionPackageBeforeActivation(authorization, awaitingReview, agentruntime.GoalPartiallySucceeded) {
		t.Fatal("successful build review boundary should remain resumable")
	}
}

func TestRuntimeEvidenceMarksDetectionPackageBuildFailure(t *testing.T) {
	result := &agentruntime.TaskResult{
		StepExecutions: []agentruntime.StepExecution{
			{ReactTurns: []agentruntime.ReactTurn{{Observation: &agentruntime.Observation{
				CallID:   "call-draft",
				ToolName: "Package.Draft.Generate",
				Status:   agentruntime.ToolCallSuccess,
				Content:  `{"package_id":"package-1","status":"draft"}`,
			}}}},
			{ReactTurns: []agentruntime.ReactTurn{{Observation: &agentruntime.Observation{
				CallID:   "call-build",
				ToolName: "Package.Build.Start",
				Status:   agentruntime.ToolCallFailed,
				Error:    "hook allowlist validation failed",
			}}}},
		},
	}

	ledger := buildRuntimeEvidenceLedger(result)
	if ledger.DetectionPackageID != "package-1" {
		t.Fatalf("package ID = %q, want package-1", ledger.DetectionPackageID)
	}
	if ledger.DetectionPackageStatus != "build_failed" {
		t.Fatalf("package status = %q, want build_failed", ledger.DetectionPackageStatus)
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

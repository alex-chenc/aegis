package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"api-server/internal/llm"
)

func TestIntentRouterRequiresLLMInsteadOfRuleFallback(t *testing.T) {
	router := NewIntentRouter()
	_, err := router.Classify(context.Background(), IntentInput{Query: "主机列表"})
	if err == nil || !strings.Contains(err.Error(), "client factory is nil") {
		t.Fatalf("expected missing LLM factory error, got %v", err)
	}
}

func TestIntentRouterContractAcceptsOpenActionAndDomain(t *testing.T) {
	err := validateLLMIntentResult(IntentResult{
		Domains:    []string{"custom_domain"},
		Action:     "compare_and_reconcile",
		RiskHint:   ToolRiskReadonly,
		Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("open LLM intent values should be accepted: %v", err)
	}
}

func TestIntentRouterWorkflowContractRequiresBusinessWorkflowButAllowsAnswer(t *testing.T) {
	workflows := NewWorkflowRegistry().List()
	if err := validateLLMIntentResultAgainstWorkflowCatalog(IntentResult{
		Action:     "generate",
		Confidence: 0.9,
	}, workflows); err == nil {
		t.Fatal("expected a business action without workflow_ids to be rejected")
	}
	if err := validateLLMIntentResultAgainstWorkflowCatalog(IntentResult{
		Action:     "answer",
		Confidence: 0.9,
	}, workflows); err != nil {
		t.Fatalf("direct answer should allow empty workflow_ids: %v", err)
	}
}

func TestIntentRouterWorkflowContractPreservesEveryExplicitWorkflow(t *testing.T) {
	workflows := NewWorkflowRegistry().List()
	result := IntentResult{
		Action:      "execute",
		Confidence:  0.9,
		WorkflowIDs: []string{vulnerabilityAssessmentWorkflowID},
	}
	required := []string{
		vulnerabilityAssessmentWorkflowID,
		detectionPackageLifecycleWorkflowID,
	}
	if err := validateLLMIntentResultAgainstWorkflowRequirements(result, workflows, required); err == nil {
		t.Fatal("expected the omitted detection package workflow to be rejected")
	}

	result.WorkflowIDs = required
	if err := validateLLMIntentResultAgainstWorkflowRequirements(result, workflows, required); err != nil {
		t.Fatalf("complete ordered workflow selection should pass: %v", err)
	}
}

func TestExplicitWorkflowRequirementsKeepScanBeforeDetectionPackage(t *testing.T) {
	got := explicitWorkflowRequirements("先对所有在线主机进行漏洞扫描，再针对该漏洞生成动态检测包并进行检测")
	want := []string{
		vulnerabilityAssessmentWorkflowID,
		detectionPackageLifecycleWorkflowID,
	}
	if len(got) != len(want) {
		t.Fatalf("required workflows = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("required workflows = %#v, want %#v", got, want)
		}
	}
}

func TestExplicitWorkflowRequirementsRouteManagedMCPQueries(t *testing.T) {
	got := explicitWorkflowRequirements("查询当前已授权的 MCP 工具目录并调用只读工具")
	if len(got) != 1 || got[0] != MCPAggregationQueryWorkflowID {
		t.Fatalf("required workflows = %#v, want [%s]", got, MCPAggregationQueryWorkflowID)
	}
}

func TestMCPAggregationQueryRequestDoesNotMatchOnboardingOrConceptualQuestions(t *testing.T) {
	if mcpAggregationQueryRequest("把这个接入到远程 MCP") {
		t.Fatal("MCP onboarding must be handled by the control-plane guard")
	}
	if mcpAggregationQueryRequest("解释当前系统内的 MCP 聚合方案") {
		t.Fatal("a conceptual MCP question must not force a query workflow")
	}
}

func TestExplicitWorkflowRequirementsRouteCodexSecurityQuestionsToAgentGuard(t *testing.T) {
	got := explicitWorkflowRequirements("分析一下，目前 Codex 智能体有哪些安全问题")
	if len(got) != 1 || got[0] != agentGuardObservationWorkflowID {
		t.Fatalf("required workflows = %#v, want [%s]", got, agentGuardObservationWorkflowID)
	}
}

func TestExplicitWorkflowRequirementsRouteAgentGuardControlSeparately(t *testing.T) {
	got := explicitWorkflowRequirements("请冻结 Codex 智能体的执行单元")
	if len(got) != 1 || got[0] != agentGuardControlWorkflowID {
		t.Fatalf("required workflows = %#v, want [%s]", got, agentGuardControlWorkflowID)
	}
}

func TestIntentRouterContinuationContractPreservesPendingArtifacts(t *testing.T) {
	input := IntentInput{
		AvailableWorkflows: NewWorkflowRegistry().List(),
		PendingClarification: &PendingClarification{
			OriginalQuery: "生成并启用检测包",
			Question:      "审核后回复继续",
			WorkflowIDs:   []string{detectionPackageLifecycleWorkflowID},
			Artifacts:     map[string]string{"package_id": "pkg-cve-2026-31431"},
		},
	}
	result := IntentResult{
		Action:           "enable",
		Confidence:       0.9,
		WorkflowIDs:      []string{detectionPackageLifecycleWorkflowID},
		ContinuationMode: "resume_pending",
		ResolvedQuery:    "签名并启用检测包",
	}
	if err := validateIntentResultContract(result, input, nil); err == nil {
		t.Fatal("expected resumed query that dropped package_id to be rejected")
	}
	result.ResolvedQuery = "签名并启用检测包 pkg-cve-2026-31431"
	if err := validateIntentResultContract(result, input, nil); err != nil {
		t.Fatalf("artifact-preserving resumed query should pass: %v", err)
	}
}

func TestIntentRouterSelectsWorkflowFromClosedCatalog(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = string(body)
		writeIntentRouterTestResponse(t, w, `{
			"domains":["package"],
			"operations":["generate"],
			"object_types":["detection_package","cve"],
			"object_ids":["CVE-2026-31431"],
			"keywords":["dynamic_detection_package"],
			"workflow_ids":["detection_package_lifecycle"],
			"risk_hint":"medium",
			"need_write":true,
			"need_approval":false,
			"confidence":0.96,
			"reason":"用户明确要求生成动态检测包",
			"action":"generate",
			"object":"detection_package"
		}`)
	}))
	defer server.Close()

	router := NewIntentRouter()
	router.SetLLMClientFactory(func(context.Context) (*llm.LLMClient, error) {
		return llm.NewLLMClient("test-key", server.URL+"/v1", "test-model", 30, 1), nil
	})
	workflows := NewWorkflowRegistry().List()
	result, err := router.Classify(context.Background(), IntentInput{
		Query:              "通过动态检测包检测 CVE-2026-31431 的利用行为",
		AvailableWorkflows: workflows,
	})
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}
	if got, want := result.WorkflowIDs, []string{detectionPackageLifecycleWorkflowID}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("workflow_ids = %#v, want %#v", got, want)
	}
	if !strings.Contains(requestBody, detectionPackageLifecycleWorkflowID) {
		t.Fatalf("intent classifier request omitted the closed workflow catalog: %s", requestBody)
	}
}

func TestIntentRouterCorrectsWorkflowOutsideClosedCatalog(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		workflowID := "invented_detection_workflow"
		if attempt > 1 {
			workflowID = detectionPackageLifecycleWorkflowID
		}
		writeIntentRouterTestResponse(t, w, `{
			"domains":["package"],
			"operations":["generate"],
			"object_types":["detection_package"],
			"workflow_ids":["`+workflowID+`"],
			"risk_hint":"medium",
			"need_write":true,
			"need_approval":false,
			"confidence":0.9,
			"reason":"dynamic detection package request",
			"action":"generate",
			"object":"detection_package"
		}`)
	}))
	defer server.Close()

	router := NewIntentRouter()
	router.SetLLMClientFactory(func(context.Context) (*llm.LLMClient, error) {
		return llm.NewLLMClient("test-key", server.URL+"/v1", "test-model", 30, 1), nil
	})
	result, err := router.Classify(context.Background(), IntentInput{
		Query:              "生成动态检测包",
		AvailableWorkflows: NewWorkflowRegistry().List(),
	})
	if err != nil {
		t.Fatalf("Classify returned error after correction: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("classifier attempts = %d, want 2", attempts.Load())
	}
	if got := result.WorkflowIDs; len(got) != 1 || got[0] != detectionPackageLifecycleWorkflowID {
		t.Fatalf("corrected workflow_ids = %#v", got)
	}
}

func TestIntentRouterCorrectsSemanticallyIncompleteDetectionPackageWorkflow(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		workflowIDs := `["cve_lookup"]`
		if attempt > 1 {
			workflowIDs = `["detection_package_lifecycle"]`
		}
		writeIntentRouterTestResponse(t, w, `{
			"domains":["vulnerability","package"],
			"operations":["generate","detect"],
			"object_types":["cve","detection_package"],
			"object_ids":["CVE-2026-31431"],
			"workflow_ids":`+workflowIDs+`,
			"risk_hint":"medium",
			"need_write":true,
			"need_approval":false,
			"confidence":0.9,
			"reason":"dynamic detection package request",
			"action":"generate",
			"object":"detection_package"
		}`)
	}))
	defer server.Close()

	router := NewIntentRouter()
	router.SetLLMClientFactory(func(context.Context) (*llm.LLMClient, error) {
		return llm.NewLLMClient("test-key", server.URL+"/v1", "test-model", 30, 1), nil
	})
	result, err := router.Classify(context.Background(), IntentInput{
		Query:              "为 CVE-2026-31431 生成动态检测包并进行检测",
		AvailableWorkflows: NewWorkflowRegistry().List(),
	})
	if err != nil {
		t.Fatalf("Classify returned error after semantic correction: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("classifier attempts = %d, want 2", attempts.Load())
	}
	if got := result.WorkflowIDs; len(got) != 1 || got[0] != detectionPackageLifecycleWorkflowID {
		t.Fatalf("corrected workflow_ids = %#v", got)
	}
}

func writeIntentRouterTestResponse(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"choices": []map[string]interface{}{{
			"message": map[string]interface{}{
				"role":    "assistant",
				"content": content,
			},
		}},
	}); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

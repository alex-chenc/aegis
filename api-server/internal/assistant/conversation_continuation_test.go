package assistant

import (
	"reflect"
	"testing"
)

func TestResolveContinuationQueryClientAuthorizationReplacesPendingOnboarding(t *testing.T) {
	query := "已审批，新增一个 Client 授权，接入这个服务 Remote MCP aegis-mcp"
	gotQuery, gotWorkflows, resumed := resolveContinuationQuery(query, IntentResult{
		ContinuationMode: "resume_pending",
		WorkflowIDs:      []string{MCPAggregationClientAuthorizationWorkflowID},
	}, &PendingClarification{
		OriginalQuery: "把这个接入到远程 MCP",
		Question:      "请提供 endpoint_url",
		WorkflowIDs:   []string{MCPAggregationOnboardingWorkflowID},
	})
	if resumed {
		t.Fatal("Client authorization must not resume pending onboarding")
	}
	if gotQuery != query {
		t.Fatalf("query = %q, want %q", gotQuery, query)
	}
	if len(gotWorkflows) != 1 || gotWorkflows[0] != MCPAggregationClientAuthorizationWorkflowID {
		t.Fatalf("workflow_ids = %#v, want [%s]", gotWorkflows, MCPAggregationClientAuthorizationWorkflowID)
	}
}

func TestApplySessionMetadataUpdatesDeletesConsumedPendingClarification(t *testing.T) {
	metadata := map[string]interface{}{
		pendingClarificationMetadataKey: map[string]interface{}{"question": "old"},
		"locale":                        "zh-CN",
	}
	applySessionMetadataUpdates(metadata, map[string]interface{}{
		pendingClarificationMetadataKey: nil,
	})
	if _, exists := metadata[pendingClarificationMetadataKey]; exists {
		t.Fatalf("consumed pending clarification was not deleted: %#v", metadata)
	}
	if metadata["locale"] != "zh-CN" {
		t.Fatalf("unrelated metadata changed: %#v", metadata)
	}
}

func TestResolveContinuationQueryRestoresPendingGoal(t *testing.T) {
	pending := &PendingClarification{
		OriginalQuery: "通过动态检测包检测 CVE-2026-31431 的利用行为",
		Question:      "请提供目标主机",
		WorkflowIDs:   []string{detectionPackageLifecycleWorkflowID},
	}
	intent := IntentResult{
		ContinuationMode: "resume_pending",
		WorkflowIDs:      []string{"host_management"},
	}

	query, workflows, resumed := resolveContinuationQuery("192.168.152.159", intent, pending)
	if !resumed {
		t.Fatal("expected the clarification answer to resume the pending goal")
	}
	if query == "192.168.152.159" || query == pending.OriginalQuery {
		t.Fatalf("resolved query must retain both the original goal and answer: %q", query)
	}
	if !containsExactString(workflows, detectionPackageLifecycleWorkflowID) {
		t.Fatalf("pending workflow was dropped: %#v", workflows)
	}
	if containsExactString(workflows, "host_management") {
		t.Fatalf("standalone host workflow must not replace the resumed goal: %#v", workflows)
	}
}

func TestResolveContinuationQueryAllowsExplicitNewRequest(t *testing.T) {
	pending := &PendingClarification{
		OriginalQuery: "生成动态检测包",
		Question:      "请提供 CVE",
		WorkflowIDs:   []string{detectionPackageLifecycleWorkflowID},
	}
	intent := IntentResult{
		ContinuationMode: "new_request",
		WorkflowIDs:      []string{"host_management"},
	}

	query, workflows, resumed := resolveContinuationQuery("查询所有主机", intent, pending)
	if resumed {
		t.Fatal("an explicit new request must not resume the pending goal")
	}
	if query != "查询所有主机" {
		t.Fatalf("new request query changed: %q", query)
	}
	if !containsExactString(workflows, "host_management") {
		t.Fatalf("new request workflow changed: %#v", workflows)
	}
}

func TestPendingClarificationRoundTripsSessionMetadata(t *testing.T) {
	want := &PendingClarification{
		OriginalQuery: "生成检测包",
		Goal:          "生成并启用检测包",
		Question:      "请提供 CVE",
		WorkflowIDs:   []string{detectionPackageLifecycleWorkflowID},
		Artifacts:     map[string]string{"package_id": "pkg-1"},
	}
	got := pendingClarificationFromMetadata(map[string]interface{}{
		pendingClarificationMetadataKey: want,
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pending clarification round-trip = %#v, want %#v", got, want)
	}
}

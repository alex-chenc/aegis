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

func TestDeduplicateApplicationsUsesRelatedPIDOverlap(t *testing.T) {
	apps := deduplicateApplications([]IdentifiedApplication{
		{Name: "redis", Category: "database", RelatedPIDs: []int{200, 100}, ListenPorts: []int{6379}, Evidence: []string{"first"}, ConfigPaths: []string{"/etc/redis/redis.conf"}},
		{Name: "redis-server", Category: "database", RelatedPIDs: []int{100}, ListenPorts: []int{6380}, Evidence: []string{"duplicate-pid"}},
		{Name: "redis", Category: "database", RelatedPIDs: []int{300}, ListenPorts: []int{6381}, Evidence: []string{"second-instance"}, ConfigPaths: []string{"/data/redis.conf"}},
	})

	if len(apps) != 1 {
		t.Fatalf("applications = %d, want 1: %#v", len(apps), apps)
	}
	if got := apps[0].RelatedPIDs; len(got) != 3 || got[0] != 100 || got[1] != 200 || got[2] != 300 {
		t.Fatalf("related_pids = %#v, want [100 200 300]", got)
	}
	if got := apps[0].ListenPorts; len(got) != 3 || got[0] != 6379 || got[1] != 6380 || got[2] != 6381 {
		t.Fatalf("merged ports = %#v, want [6379 6380 6381]", got)
	}
	if got := apps[0].ConfigPaths; len(got) != 2 || got[0] != "/etc/redis/redis.conf" || got[1] != "/data/redis.conf" {
		t.Fatalf("merged config paths = %#v", got)
	}
}

func TestGenerateAppFingerprintDedupeByHostApplication(t *testing.T) {
	first := generateAppFingerprint("host-1", "database", "redis", "/usr/bin/redis-server", []int{6379}, []int{1234})
	samePIDDifferentShape := generateAppFingerprint("host-1", "web_service", "redis-alt", "/opt/redis", []int{6380}, []int{1234})
	differentPID := generateAppFingerprint("host-1", "database", "redis", "/usr/bin/redis-server", []int{6379}, []int{4321})

	if first != samePIDDifferentShape {
		t.Fatalf("same app fingerprint mismatch: %s != %s", first, samePIDDifferentShape)
	}
	if first != differentPID {
		t.Fatalf("same host application fingerprint should ignore pid: %s != %s", first, differentPID)
	}
}

func TestDetectKnownApplicationsFromProcessesExtractsRedisConfig(t *testing.T) {
	apps := detectKnownApplicationsFromProcesses(HostAssetSnapshot{Processes: []ProcessAsset{{
		PID:         4321,
		Comm:        "redis-server",
		ExePath:     "/usr/local/bin/redis-server",
		Cwd:         "/srv/redis",
		Cmdline:     "redis-server /tmp/aegis-weakpass-test/redis.conf",
		Username:    "redis",
		ListenPorts: []int{6379},
	}}})
	if len(apps) != 1 {
		t.Fatalf("applications = %#v, want one redis app", apps)
	}
	if apps[0].Name != "redis" || len(apps[0].RelatedPIDs) != 1 || apps[0].RelatedPIDs[0] != 4321 {
		t.Fatalf("unexpected app: %#v", apps[0])
	}
	if len(apps[0].ConfigPaths) != 1 || apps[0].ConfigPaths[0] != "/tmp/aegis-weakpass-test/redis.conf" {
		t.Fatalf("config paths = %#v, want redis cmdline config", apps[0].ConfigPaths)
	}
}

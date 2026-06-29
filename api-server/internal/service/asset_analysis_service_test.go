package service

import (
	"testing"
	"time"

	"api-server/internal/model"

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

func TestParseAnalysisResultFiltersNonMarketVisibleApplications(t *testing.T) {
	svc := NewAssetAnalysisService(nil, nil, nil, zap.NewNop())

	result, err := svc.parseAnalysisResult(`{"applications":[
		{"name":"redis-server","display_name":"Redis","category":"database","confidence":0.96,"related_pids":[101],"listen_ports":[6379]},
		{"name":"python3","display_name":"Python Script","category":"web_framework","confidence":0.91,"related_pids":[102]},
		{"name":"systemd","display_name":"systemd","category":"other","confidence":0.99,"related_pids":[1]},
		{"name":"internal-order-service","display_name":"内部订单服务","category":"web_site","confidence":0.88,"related_pids":[103]}
	]}`)
	if err != nil {
		t.Fatalf("parseAnalysisResult returned error: %v", err)
	}
	if len(result.Applications) != 1 {
		t.Fatalf("applications = %#v, want only Redis", result.Applications)
	}
	if result.Applications[0].Name != "redis" || result.Applications[0].DisplayName != "Redis" {
		t.Fatalf("unexpected kept application: %#v", result.Applications[0])
	}
}

func TestParseAnalysisResultNormalizesBroadPublicApplicationsAndDedupes(t *testing.T) {
	svc := NewAssetAnalysisService(nil, nil, nil, zap.NewNop())

	result, err := svc.parseAnalysisResult(`{"applications":[
		{"name":"grafana-server","display_name":"Grafana Server","category":"other","confidence":0.91,"related_pids":[201],"listen_ports":[3000],"evidence":["comm=grafana-server"]},
		{"name":"grafana","display_name":"Grafana","category":"web_site","confidence":0.88,"related_pids":[202],"listen_ports":[3000],"evidence":["port=3000"]},
		{"name":"openresty","display_name":"OpenResty","category":"unknown","confidence":0.93,"related_pids":[301],"evidence":["version=openresty/1.25.3"]},
		{"name":"litellm","display_name":"LiteLLM","category":"other","confidence":0.86,"related_pids":[401],"evidence":["cmdline contains litellm"]}
	]}`)
	if err != nil {
		t.Fatalf("parseAnalysisResult returned error: %v", err)
	}

	byName := map[string]IdentifiedApplication{}
	for _, app := range result.Applications {
		byName[app.Name] = app
	}
	if len(result.Applications) != 3 {
		t.Fatalf("applications = %#v, want grafana/openresty/litellm", result.Applications)
	}
	if app := byName["grafana"]; app.Category != "web_service" || len(app.RelatedPIDs) != 2 {
		t.Fatalf("grafana normalization/dedupe failed: %#v", app)
	}
	if app := byName["openresty"]; app.Category != "web_service" || app.DisplayName != "OpenResty" {
		t.Fatalf("openresty normalization failed: %#v", app)
	}
	if app := byName["litellm"]; app.Category != "llm_service" || app.DisplayName != "LiteLLM" {
		t.Fatalf("litellm normalization failed: %#v", app)
	}
}

func TestParseAnalysisResultRejectsRuntimeButKeepsPublicFramework(t *testing.T) {
	svc := NewAssetAnalysisService(nil, nil, nil, zap.NewNop())

	result, err := svc.parseAnalysisResult(`{"applications":[
		{"name":"python3","display_name":"Python Runtime","category":"web_framework","confidence":0.95,"related_pids":[501]},
		{"name":"fastapi","display_name":"FastAPI","category":"unknown","confidence":0.82,"related_pids":[502],"evidence":["cmdline contains uvicorn app:api"]}
	]}`)
	if err != nil {
		t.Fatalf("parseAnalysisResult returned error: %v", err)
	}
	if len(result.Applications) != 1 {
		t.Fatalf("applications = %#v, want only FastAPI", result.Applications)
	}
	if result.Applications[0].Name != "fastapi" || result.Applications[0].Category != "web_framework" {
		t.Fatalf("unexpected kept application: %#v", result.Applications[0])
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

func TestEnrichIdentifiedApplicationVersionFromSoftwareAssets(t *testing.T) {
	app := enrichIdentifiedApplicationVersion(
		IdentifiedApplication{Name: "docker", DisplayName: "Docker Engine"},
		HostAssetSnapshot{},
		[]model.HostSoftwareAsset{{
			Name:           "docker-ce",
			Version:        "5:29.5.3-1~ubuntu.25.10~questing",
			PackageManager: "dpkg",
		}},
	)

	if app.Version != "29.5.3-1~ubuntu.25.10~questing" || app.VersionSource != "software:dpkg" {
		t.Fatalf("enriched docker version = %q source = %q", app.Version, app.VersionSource)
	}
}

func TestEnrichIdentifiedApplicationVersionUsesNewestFreshSoftwareAsset(t *testing.T) {
	now := time.Now()
	app := enrichIdentifiedApplicationVersion(
		IdentifiedApplication{Name: "docker", DisplayName: "Docker Engine"},
		HostAssetSnapshot{},
		[]model.HostSoftwareAsset{
			{
				Name:           "docker-ce",
				Version:        "5:29.5.2-1~ubuntu.25.10~questing",
				PackageManager: "dpkg",
				LastSeenAt:     now.Add(-48 * time.Hour),
			},
			{
				Name:           "docker-ce",
				Version:        "5:29.5.3-1~ubuntu.25.10~questing",
				PackageManager: "dpkg",
				LastSeenAt:     now,
			},
		},
	)

	if app.Version != "29.5.3-1~ubuntu.25.10~questing" || app.VersionSource != "software:dpkg" {
		t.Fatalf("enriched docker version = %q source = %q", app.Version, app.VersionSource)
	}
}

func TestEnrichIdentifiedApplicationVersionFromProcessPath(t *testing.T) {
	app := enrichIdentifiedApplicationVersion(
		IdentifiedApplication{Name: "postgresql", RelatedPIDs: []int{42}},
		HostAssetSnapshot{Processes: []ProcessAsset{{
			PID:     42,
			Comm:    "postgres",
			ExePath: "/usr/lib/postgresql/16/bin/postgres",
		}}},
		nil,
	)

	if app.Version != "16" || app.VersionSource != "process_path" {
		t.Fatalf("enriched postgres version = %q source = %q", app.Version, app.VersionSource)
	}
}

func TestEnrichIdentifiedApplicationVersionFromEvidence(t *testing.T) {
	app := enrichIdentifiedApplicationVersion(
		IdentifiedApplication{Name: "openssh", Evidence: []string{"version_tool=OpenSSH_10.0p2 Ubuntu-5ubuntu5.4"}},
		HostAssetSnapshot{},
		nil,
	)

	if app.Version != "10.0p2" || app.VersionSource != "evidence" {
		t.Fatalf("enriched openssh version = %q source = %q", app.Version, app.VersionSource)
	}
}

func TestGenerateAppFingerprintDedupeByHostApplication(t *testing.T) {
	first := generateAppFingerprint("host-1", "database", "redis", "/usr/bin/redis-server", []int{6379}, []int{1234})
	samePIDDifferentShape := generateAppFingerprint("host-1", "web_service", "redis-alt", "/opt/redis", []int{6380}, []int{1234})
	differentPID := generateAppFingerprint("host-1", "database", "redis", "/usr/bin/redis-server", []int{6379}, []int{4321})
	kafka := generateAppFingerprint("host-1", "other", "kafka", "/opt/kafka/bin/kafka-server-start.sh", []int{9092}, []int{2345})
	kafkaDifferentPort := generateAppFingerprint("host-1", "web_service", "kafka-server-start", "/tmp/kafka", []int{19092}, []int{9876})

	if first != samePIDDifferentShape {
		t.Fatalf("same app fingerprint mismatch: %s != %s", first, samePIDDifferentShape)
	}
	if first != differentPID {
		t.Fatalf("same host application fingerprint should ignore pid: %s != %s", first, differentPID)
	}
	if kafka != kafkaDifferentPort {
		t.Fatalf("same kafka application fingerprint should ignore shape: %s != %s", kafka, kafkaDifferentPort)
	}
}

func TestApplicationDedupeNamesIncludesKnownAliases(t *testing.T) {
	names := applicationDedupeNames(IdentifiedApplication{Name: "redis", DisplayName: "Redis", Category: "database"})
	seen := map[string]bool{}
	for _, name := range names {
		seen[name] = true
	}
	for _, want := range []string{"redis", "redis-server"} {
		if !seen[want] {
			t.Fatalf("dedupe names = %#v, want alias %q", names, want)
		}
	}

	names = applicationDedupeNames(IdentifiedApplication{Name: "claude_code", DisplayName: "Claude Code", Category: "ai_agent"})
	seen = map[string]bool{}
	for _, name := range names {
		seen[name] = true
	}
	for _, want := range []string{"claude_code", "claude-code", "claude code"} {
		if !seen[want] {
			t.Fatalf("dedupe names = %#v, want alias %q", names, want)
		}
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

func TestDetectKnownApplicationsFromProcessesCoversPublicMiddleware(t *testing.T) {
	apps := detectKnownApplicationsFromProcesses(HostAssetSnapshot{Processes: []ProcessAsset{
		{
			PID:         5001,
			Comm:        "grafana-server",
			ExePath:     "/usr/sbin/grafana-server",
			Cmdline:     "grafana-server -config /etc/grafana/grafana.ini",
			Username:    "grafana",
			ListenPorts: []int{3000},
		},
		{
			PID:         5002,
			Comm:        "python3",
			ExePath:     "/usr/bin/python3",
			Cmdline:     "python3 -m litellm --config /etc/litellm/config.yaml",
			Username:    "litellm",
			ListenPorts: []int{4000},
		},
		{
			PID:         5003,
			Comm:        "cupsd",
			ExePath:     "/usr/sbin/cupsd",
			Cmdline:     "cupsd -f",
			Username:    "root",
			ListenPorts: []int{631},
		},
	}})

	if len(apps) != 3 {
		t.Fatalf("applications = %#v, want grafana, litellm, and cups", apps)
	}
	seen := map[string]bool{}
	for _, app := range apps {
		seen[app.Name] = true
	}
	for _, want := range []string{"grafana", "litellm", "cups"} {
		if !seen[want] {
			t.Fatalf("applications = %#v, missing %s", apps, want)
		}
	}
}

func TestDetectKnownApplicationsFromProcessesCoversHostToolsAndProjectServices(t *testing.T) {
	apps := deduplicateApplications(detectKnownApplicationsFromProcesses(HostAssetSnapshot{Processes: []ProcessAsset{
		{PID: 6001, Comm: "dockerd", ExePath: "/usr/bin/dockerd", Cmdline: "/usr/bin/dockerd -H fd://"},
		{PID: 6002, Comm: "docker-proxy", ExePath: "/usr/bin/docker-proxy", Cmdline: "/usr/bin/docker-proxy -host-port 8082", ListenPorts: []int{8082}},
		{PID: 6003, Comm: "tailscaled", ExePath: "/usr/sbin/tailscaled", Cmdline: "/usr/sbin/tailscaled --state=/var/lib/tailscale/tailscaled.state"},
		{PID: 6004, Comm: "clash-verge", ExePath: "/usr/bin/clash-verge", Cmdline: "/usr/bin/clash-verge", ListenPorts: []int{33331}},
		{PID: 6005, Comm: "api-server", ExePath: "/root/api-server", Cmdline: "./api-server", ListenPorts: []int{8082}},
		{PID: 6006, Comm: "aegis-agent", ExePath: "/opt/aegis-agent/aegis-agent", Cmdline: "/opt/aegis-agent/aegis-agent", ListenPorts: []int{19095}},
		{PID: 6007, Comm: "codex", ExePath: "/usr/lib/node_modules/@openai/codex/bin/codex", Cmdline: "codex app-server proxy"},
	}}))

	seen := map[string]IdentifiedApplication{}
	for _, app := range apps {
		seen[app.Name] = app
	}
	for _, want := range []string{"docker", "tailscale", "clash_verge", "aegis_api_server", "aegis_agent", "codex"} {
		if _, ok := seen[want]; !ok {
			t.Fatalf("applications = %#v, missing %s", apps, want)
		}
	}
	if app := seen["docker"]; len(app.RelatedPIDs) != 2 {
		t.Fatalf("docker processes should be merged, got %#v", app)
	}
}

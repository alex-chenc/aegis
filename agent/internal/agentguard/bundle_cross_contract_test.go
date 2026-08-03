package agentguard

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	pb "aegis-agent/pkg/api/v1"
)

func TestAPIServerBundleContractAndExportConfigStatuses(t *testing.T) {
	fixturePath := os.Getenv("AEGIS_AGENT_GUARD_CONTRACT_BUNDLE")
	if fixturePath == "" {
		t.Skip("cross-module bundle fixture path is not set")
	}
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read api-server bundle fixture: %v", err)
	}
	bundle, err := decodeBundle(payload)
	if err != nil {
		t.Fatalf("decode api-server bundle fixture: %v", err)
	}
	if err := bundle.Validate(bundle.HostID); err != nil {
		t.Fatalf("validate api-server bundle fixture: %v", err)
	}
	if len(bundle.Profiles) != 6 || len(bundle.BuiltinRules) != 5 ||
		len(bundle.Policies) != 1 || bundle.Defaults.Mode != "monitor_only" {
		t.Fatalf("bundle fields were not preserved: %#v", bundle)
	}
	if !bundle.Defaults.BehaviorMonitorEnabled ||
		bundle.Defaults.ToolAdapterEnabled ||
		bundle.Defaults.EnforcementEnabled ||
		bundle.Defaults.FreezeEnabled ||
		bundle.Defaults.FreezeTimeoutSeconds != 300 ||
		bundle.Defaults.ReconcileIntervalSeconds != 30 {
		t.Fatalf("bundle defaults contract changed: %#v", bundle.Defaults)
	}
	for _, profile := range bundle.Profiles {
		if profile.ProfileKey == "" ||
			profile.ProfileVersion < 1 ||
			profile.AgentType == "" ||
			profile.ControllerMatch == nil ||
			profile.WorkerMatch == nil ||
			profile.BackendDetectors == nil ||
			profile.IsolationExpectation == nil ||
			profile.DefaultEscapeRules == nil ||
			profile.Digest == "" {
			t.Fatalf("incomplete profile contract: %#v", profile)
		}
	}
	for _, rule := range bundle.BuiltinRules {
		if rule.RuleKey == "" ||
			rule.RuleVersion < 1 ||
			!rule.Enabled ||
			rule.Severity == "" ||
			rule.Action == "" ||
			rule.CompiledParameters == nil ||
			rule.Digest == "" {
			t.Fatalf("incomplete builtin rule contract: %#v", rule)
		}
	}
	for _, key := range []string{
		"policy_key", "version", "priority", "targets", "collection_policy",
		"builtin_rule_overrides", "atomic_rules", "correlation_rules",
		"analysis_policy", "escape_rules", "compiled_preview", "digest",
	} {
		if _, ok := bundle.Policies[0][key]; !ok {
			t.Fatalf("policy field %q was lost: %#v", key, bundle.Policies[0])
		}
	}

	reporter := &captureReporter{}
	manager := NewManager(ManagerConfig{
		Enabled:                true,
		BehaviorMonitorEnabled: true,
		HostID:                 bundle.HostID,
		StateDir:               t.TempDir(),
		SpoolCapacity:          8,
		ReconcileInterval:      time.Hour,
		FlushInterval:          time.Hour,
	}, &fakeScanner{processes: map[uint32]ProcessSnapshot{}}, reporter)
	if err := manager.ApplyBundle(string(payload)); err != nil {
		t.Fatalf("apply api-server bundle fixture: %v", err)
	}
	manager.flush()

	var rejected map[string]any
	if err := json.Unmarshal(payload, &rejected); err != nil {
		t.Fatalf("decode rejected probe: %v", err)
	}
	defaults, ok := rejected["defaults"].(map[string]any)
	if !ok {
		t.Fatalf("bundle defaults shape = %#v", rejected["defaults"])
	}
	defaults["mode"] = "deny"
	rejectedPayload, err := json.Marshal(rejected)
	if err != nil {
		t.Fatalf("encode rejected probe: %v", err)
	}
	if err := manager.ApplyBundle(string(rejectedPayload)); err == nil {
		t.Fatal("P1-forbidden bundle unexpectedly applied")
	}
	manager.flush()

	applied := findConfigStatus(t, reporter.snapshot(), "applied")
	rejectedStatus := findConfigStatus(t, reporter.snapshot(), "rejected")
	assertConfigStatusContract(t, applied, bundle, false)
	assertConfigStatusContract(t, rejectedStatus, bundle, true)
	writeContractStatus(t, "AEGIS_AGENT_GUARD_APPLIED_STATUS_OUT", applied)
	writeContractStatus(t, "AEGIS_AGENT_GUARD_REJECTED_STATUS_OUT", rejectedStatus)
}

func findConfigStatus(t *testing.T, events []*pb.RuntimeEvent, status string) string {
	t.Helper()
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.EventType != "agent_guard_config_status" {
			continue
		}
		var body map[string]any
		if json.Unmarshal([]byte(event.EventDataJson), &body) == nil && body["status"] == status {
			return event.EventDataJson
		}
	}
	t.Fatalf("missing %s config status", status)
	return ""
}

func assertConfigStatusContract(t *testing.T, raw string, bundle Bundle, wantError bool) {
	t.Helper()
	var body struct {
		Schema        string    `json:"schema"`
		Status        string    `json:"status"`
		BundleVersion int64     `json:"bundle_version"`
		Digest        string    `json:"digest"`
		ErrorCode     string    `json:"error_code"`
		OccurredAt    time.Time `json:"occurred_at"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode config status: %v", err)
	}
	if body.Schema != GuardSchemaV1 ||
		body.BundleVersion != bundle.BundleVersion ||
		body.Digest != bundle.Digest ||
		body.OccurredAt.IsZero() {
		t.Fatalf("invalid config status identity: %#v", body)
	}
	if wantError && body.ErrorCode == "" {
		t.Fatalf("rejected config status has no error code: %#v", body)
	}
	if !wantError && body.ErrorCode != "" {
		t.Fatalf("applied config status has error code: %#v", body)
	}
}

func writeContractStatus(t *testing.T, environmentKey string, raw string) {
	t.Helper()
	if output := os.Getenv(environmentKey); output != "" {
		if err := os.WriteFile(output, []byte(raw), 0o600); err != nil {
			t.Fatalf("write %s: %v", environmentKey, err)
		}
	}
}

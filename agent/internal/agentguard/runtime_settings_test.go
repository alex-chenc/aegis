package agentguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookCommandEnvironmentProvidesHomeWithoutInheritedHome(t *testing.T) {
	original := os.Getenv("HOME")
	if err := os.Unsetenv("HOME"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("HOME")
			return
		}
		_ = os.Setenv("HOME", original)
	})

	var home string
	for _, entry := range hookCommandEnvironment() {
		if strings.HasPrefix(entry, "HOME=") {
			home = strings.TrimPrefix(entry, "HOME=")
			break
		}
	}
	if home == "" {
		t.Fatal("hook command environment must provide HOME")
	}
}

func TestApplyRuntimeSettingsDynamicallyTogglesIngressAndProvisionsSelectedAgents(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	var provisioned []string
	var removed []string
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, ToolAdapterEnabled: true,
		SessionHookEnabled: true, HostID: "host-1", ToolAdapter: fixture.adapter,
		ToolHookSocket: filepath.Join(t.TempDir(), "agent-guard.sock"),
		HookProvisioner: func(agentType string) error {
			provisioned = append(provisioned, agentType)
			return nil
		},
		HookRemover: func(agentType string) error {
			removed = append(removed, agentType)
			return nil
		},
	}, &fakeScanner{processes: map[uint32]ProcessSnapshot{}}, &captureReporter{})
	manager.bundleToolAdapterEnabled.Store(true)

	payload, err := json.Marshal(RuntimeSettings{
		Schema: AgentGuardRuntimeSettingsSchema, Version: 1, HostID: "host-1",
		ToolAdapterEnabled: true, SessionHookEnabled: true,
		Injections: []HookInjection{{AgentType: "claude-code", Enabled: true}, {AgentType: "openclaw", Enabled: false}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyRuntimeSettings(string(payload)); err != nil {
		t.Fatalf("apply runtime settings: %v", err)
	}
	if len(provisioned) != 1 || provisioned[0] != "claude-code" {
		t.Fatalf("unexpected provisioned agents: %#v", provisioned)
	}
	if !manager.toolEnabled.Load() || !manager.sessionEnabled.Load() || manager.toolIngress == nil {
		t.Fatal("runtime settings did not enable trusted Hook ingress")
	}

	payload, _ = json.Marshal(RuntimeSettings{
		Schema: AgentGuardRuntimeSettingsSchema, Version: 2, HostID: "host-1",
		ToolAdapterEnabled: false, SessionHookEnabled: false,
	})
	if err := manager.ApplyRuntimeSettings(string(payload)); err != nil {
		t.Fatalf("disable runtime settings: %v", err)
	}
	if manager.toolEnabled.Load() || manager.sessionEnabled.Load() || manager.toolIngress != nil {
		t.Fatal("runtime settings did not disable trusted Hook ingress")
	}
	if len(removed) != 1 || removed[0] != "claude-code" {
		t.Fatalf("unexpected removed agents: %#v", removed)
	}
}

func TestApplyRuntimeSettingsKeepsBehaviorAndEscapeHooksIndependent(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	var provisioned, removed []string
	manager := NewManager(ManagerConfig{Enabled: true, BehaviorMonitorEnabled: true, ToolAdapterEnabled: true, SessionHookEnabled: true, HostID: "host-1", ToolAdapter: fixture.adapter, ToolHookSocket: filepath.Join(t.TempDir(), "hook.sock"),
		ScopedHookProvisioner: func(agentType, scope string) error {
			provisioned = append(provisioned, agentType+":"+scope)
			return nil
		},
		ScopedHookRemover: func(agentType, scope string) error { removed = append(removed, agentType+":"+scope); return nil },
	}, &fakeScanner{processes: map[uint32]ProcessSnapshot{}}, &captureReporter{})
	first, _ := json.Marshal(RuntimeSettings{Schema: AgentGuardRuntimeSettingsSchema, Version: 1, HostID: "host-1", ToolAdapterEnabled: true, SessionHookEnabled: true, BehaviorPolicyEnabled: true, EscapePolicyEnabled: true, Injections: []HookInjection{{AgentType: "codex", BehaviorEnabled: true, EscapeEnabled: true}}})
	if err := manager.ApplyRuntimeSettings(string(first)); err != nil {
		t.Fatal(err)
	}
	if strings.Join(provisioned, ",") != "codex:behavior,codex:escape" {
		t.Fatalf("hooks did not coexist: %#v", provisioned)
	}
	second, _ := json.Marshal(RuntimeSettings{Schema: AgentGuardRuntimeSettingsSchema, Version: 2, HostID: "host-1", ToolAdapterEnabled: true, SessionHookEnabled: true, BehaviorPolicyEnabled: true, EscapePolicyEnabled: false, Injections: []HookInjection{{AgentType: "codex", BehaviorEnabled: true, EscapeEnabled: true}}})
	if err := manager.ApplyRuntimeSettings(string(second)); err != nil {
		t.Fatal(err)
	}
	if strings.Join(removed, ",") != "codex:escape" {
		t.Fatalf("disabling escape removed wrong hooks: %#v", removed)
	}
	if !manager.hookStates["codex\x00behavior"] || manager.hookStates["codex\x00escape"] {
		t.Fatalf("unexpected scoped state: %#v", manager.hookStates)
	}
}

func TestApplyRuntimeSettingsRejectsWrongHostAndStaleVersion(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, ToolAdapterEnabled: true,
		SessionHookEnabled: true, HostID: "host-1", ToolAdapter: fixture.adapter,
		HookProvisioner: func(string) error { t.Fatal("must not provision rejected settings"); return nil },
	}, &fakeScanner{processes: map[uint32]ProcessSnapshot{}}, &captureReporter{})

	wrongHost, _ := json.Marshal(RuntimeSettings{
		Schema: AgentGuardRuntimeSettingsSchema, Version: 1, HostID: "host-2",
	})
	if err := manager.ApplyRuntimeSettings(string(wrongHost)); err == nil {
		t.Fatal("host mismatch must be rejected")
	}
	valid, _ := json.Marshal(RuntimeSettings{
		Schema: AgentGuardRuntimeSettingsSchema, Version: 3, HostID: "host-1",
		ToolAdapterEnabled: false, SessionHookEnabled: false,
	})
	if err := manager.ApplyRuntimeSettings(string(valid)); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	stale, _ := json.Marshal(RuntimeSettings{
		Schema: AgentGuardRuntimeSettingsSchema, Version: 2, HostID: "host-1",
	})
	if err := manager.ApplyRuntimeSettings(string(stale)); err == nil {
		t.Fatal("stale settings must be rejected")
	}
}

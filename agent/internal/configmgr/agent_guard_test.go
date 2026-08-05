package configmgr

import (
	"testing"

	pb "aegis-agent/pkg/api/v1"
)

func TestApplyConfigSyncAgentGuardBundleRequiresHandlerAndFullSync(t *testing.T) {
	mgr := NewConfigManager()
	if err := mgr.ApplyConfigSync(&pb.ConfigSync{
		ConfigType: "agent_guard_bundle",
		Action:     "full_sync",
		Payload:    `{}`,
	}); err == nil {
		t.Fatal("missing agent guard handler must not report applied")
	}

	var got string
	mgr.SetAgentGuardBundleHandler(func(payload string) error {
		got = payload
		return nil
	})
	if err := mgr.ApplyConfigSync(&pb.ConfigSync{
		ConfigType: "agent_guard_bundle",
		Action:     "incremental",
		Payload:    `{"schema":"aegis.agent_guard.bundle.v1"}`,
	}); err == nil {
		t.Fatal("incremental Agent Guard sync must be rejected")
	}
	payload := `{"schema":"aegis.agent_guard.bundle.v1"}`
	if err := mgr.ApplyConfigSync(&pb.ConfigSync{
		ConfigType: "agent_guard_bundle",
		Action:     "full_sync",
		Payload:    payload,
	}); err != nil {
		t.Fatalf("full sync rejected: %v", err)
	}
	if got != payload {
		t.Fatalf("handler payload mismatch: %q", got)
	}
}

func TestApplyConfigSyncAgentGuardRuntimeSettingsUsesInMemoryHandler(t *testing.T) {
	mgr := NewConfigManager()
	var got string
	mgr.SetAgentGuardRuntimeSettingsHandler(func(payload string) error {
		got = payload
		return nil
	})
	payload := `{"schema":"aegis.agent_guard.runtime_settings.v1","version":1}`
	if err := mgr.ApplyConfigSync(&pb.ConfigSync{
		ConfigType: "agent_guard_runtime_settings", Action: "full_sync", Payload: payload,
	}); err != nil {
		t.Fatalf("runtime settings rejected: %v", err)
	}
	if got != payload {
		t.Fatalf("runtime settings payload mismatch: %q", got)
	}
}

package agentguard

import "testing"

func TestManagerRebindHostIDBeforeStart(t *testing.T) {
	manager := NewManager(ManagerConfig{HostID: "temporary-host"}, NewProcFSScanner("/proc"), nil)
	if err := manager.RebindHostID("canonical-host"); err != nil {
		t.Fatalf("RebindHostID: %v", err)
	}
	if manager.cfg.HostID != "canonical-host" || manager.tracker.hostID != "canonical-host" ||
		manager.bundles.hostID != "canonical-host" {
		t.Fatalf("manager identities were not rebound: cfg=%q tracker=%q bundle=%q",
			manager.cfg.HostID, manager.tracker.hostID, manager.bundles.hostID)
	}
}

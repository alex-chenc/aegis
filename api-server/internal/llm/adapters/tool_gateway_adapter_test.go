package adapters

import "testing"

func TestApplyToolDefaultsAddsAgentPIDDefaults(t *testing.T) {
	processArgs := map[string]any{"host_id": "host-1"}
	applyToolDefaults("GetProcessTree", processArgs)
	if processArgs["pid"] != 1 {
		t.Fatalf("GetProcessTree default pid = %#v, want 1", processArgs["pid"])
	}

	networkArgs := map[string]any{"host_id": "host-1"}
	applyToolDefaults("GetNetworkConnections", networkArgs)
	if networkArgs["pid"] != 0 {
		t.Fatalf("GetNetworkConnections default pid = %#v, want 0", networkArgs["pid"])
	}
}

func TestApplyToolDefaultsPreservesProvidedPID(t *testing.T) {
	args := map[string]any{"host_id": "host-1", "pid": 1234}
	applyToolDefaults("GetProcessTree", args)
	if args["pid"] != 1234 {
		t.Fatalf("provided pid was overwritten: %#v", args["pid"])
	}
}

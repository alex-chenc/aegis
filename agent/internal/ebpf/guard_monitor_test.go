package ebpf

import (
	"encoding/binary"
	"testing"
)

func TestParseGuardMonitorEventAndBuildSemanticEvent(t *testing.T) {
	raw := make([]byte, guardMonitorEventSize)
	binary.LittleEndian.PutUint64(raw[0:8], 123)
	binary.LittleEndian.PutUint32(raw[8:12], 4100)
	binary.LittleEndian.PutUint32(raw[12:16], 4101)
	binary.LittleEndian.PutUint32(raw[16:20], 1000)
	binary.LittleEndian.PutUint32(raw[20:24], 1000)
	binary.LittleEndian.PutUint32(raw[24:28], GuardOperationSetNS)
	binary.LittleEndian.PutUint64(raw[32:40], 7)
	binary.LittleEndian.PutUint64(raw[56:64], ^uint64(0))
	copy(raw[64:80], "bash")
	copy(raw[80:336], "/proc/1/ns/mnt")

	parsed, err := parseGuardMonitorEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	loader := &Loader{hostID: "host-1", hostname: "host"}
	event := loader.buildGuardRuntimeEvent(*parsed)
	if event.EventType != "agent_guard_syscall" ||
		event.SecurityCategory != "isolation" ||
		event.SecurityOperation != "setns" ||
		event.SecurityTarget != "/proc/1/ns/mnt" ||
		event.SyscallReturn != -1 {
		t.Fatalf("semantic monitor event incomplete: %#v", event)
	}
}

func TestDefaultProgramsIncludeOptionalGuardMonitor(t *testing.T) {
	for _, program := range defaultBPFPrograms() {
		if program.name == "guard_monitor" {
			if program.required || program.mapName != "guard_monitor_events" {
				t.Fatalf("guard monitor must degrade independently: %#v", program)
			}
			return
		}
	}
	t.Fatal("guard_monitor not configured")
}

func TestAgentGuardLSMIsLoadedOnlyWithBothLocalGateAndKernelCapability(t *testing.T) {
	for _, test := range []struct {
		name        string
		enforcement bool
		bpfLSM      bool
		want        bool
	}{
		{name: "both", enforcement: true, bpfLSM: true, want: true},
		{name: "local gate off", enforcement: false, bpfLSM: true},
		{name: "kernel unavailable", enforcement: true, bpfLSM: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			programs := configuredBPFPrograms(LoaderOptions{
				AgentGuardEnforcementEnabled: test.enforcement,
				BPFLSMAvailable:              test.bpfLSM,
			})
			found := false
			for _, program := range programs {
				if program.name == "agent_guard_lsm" {
					found = true
					if program.required {
						t.Fatal("LSM rollout must degrade without disabling monitor collection")
					}
				}
			}
			if found != test.want {
				t.Fatalf("agent_guard_lsm present=%v want=%v", found, test.want)
			}
		})
	}
}

func TestForkCollectionPrecedesLSMForSharedGuardMaps(t *testing.T) {
	programs := configuredBPFPrograms(LoaderOptions{
		AgentGuardEnforcementEnabled: true, BPFLSMAvailable: true,
	})
	positions := map[string]int{}
	for index, program := range programs {
		positions[program.name] = index
	}
	if positions["fork"] >= positions["agent_guard_lsm"] {
		t.Fatalf("fork map owner must load before LSM reuse: %v", positions)
	}
}

func TestParseAgentGuardLSMDenyEvent(t *testing.T) {
	raw := make([]byte, agentGuardLSMEventSize)
	binary.LittleEndian.PutUint64(raw[24:32], 42)
	binary.LittleEndian.PutUint64(raw[32:40], 84)
	binary.LittleEndian.PutUint32(raw[40:44], 4100)
	binary.LittleEndian.PutUint32(raw[48:52], 6)
	binary.LittleEndian.PutUint32(raw[52:56], 2)
	copy(raw[56:72], "sandbox-worker")
	copy(raw[72:328], "/run/containerd/containerd.sock")
	parsed, err := parseAgentGuardLSMEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	loader := &Loader{hostID: "host-1", hostname: "host", eventChan: make(chan Event, 1)}
	loader.processAgentGuardLSMEvent(raw)
	event := <-loader.eventChan
	if parsed.PolicySlot != 42 || parsed.RuleSlot != 84 ||
		event.SecurityDecision != "deny_and_freeze" ||
		event.SecurityTarget != "/run/containerd/containerd.sock" {
		t.Fatalf("LSM evidence incomplete: parsed=%+v event=%+v", parsed, event)
	}
}

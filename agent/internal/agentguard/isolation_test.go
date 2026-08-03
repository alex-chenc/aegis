package agentguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIsolationStatusCapturesSecurityStateAndExplicitGaps(t *testing.T) {
	state := newIsolationState()
	parseIsolationStatus([]byte(strings.Join([]string{
		"Uid:\t1000\t1000\t1000\t1000",
		"Gid:\t1000\t1000\t1000\t1000",
		"CapInh:\t0000000000000000",
		"CapPrm:\t0000000000000003",
		"CapEff:\t0000000000000001",
		"CapBnd:\t000001ffffffffff",
		"CapAmb:\t0000000000000000",
		"NoNewPrivs:\t1",
		"Seccomp:\t2",
	}, "\n")), &state)

	if state.Capabilities.Effective != "0x0000000000000001" ||
		state.Capabilities.Permitted != "0x0000000000000003" ||
		state.NoNewPrivileges == nil || !*state.NoNewPrivileges ||
		state.SeccompMode == nil || *state.SeccompMode != 2 {
		t.Fatalf("security state not parsed: %#v", state)
	}
	if !state.Availability["capabilities"].Available ||
		!state.Availability["no_new_privs"].Available ||
		!state.Availability["seccomp"].Available {
		t.Fatalf("visible fields marked unavailable: %#v", state.Availability)
	}

	missing := newIsolationState()
	parseIsolationStatus([]byte("Uid:\t1000\t1000\t1000\t1000\n"), &missing)
	for _, field := range []string{"capabilities", "no_new_privs", "seccomp"} {
		if missing.Availability[field].Available || missing.Availability[field].Reason == "" {
			t.Fatalf("%s gap must be explicit: %#v", field, missing.Availability[field])
		}
	}
}

func TestProcFSIsolationScannerBuildsNamespaceMountAndCgroupBaseline(t *testing.T) {
	root := t.TempDir()
	pidDir := filepath.Join(root, "321")
	if err := os.MkdirAll(filepath.Join(pidDir, "ns"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, inode := range map[string]string{
		"mnt": "4026533001", "pid": "4026533002", "net": "4026533003",
		"user": "4026533004", "uts": "4026533005", "ipc": "4026533006",
		"cgroup": "4026533007", "time": "4026533008",
	} {
		if err := os.Symlink(name+":["+inode+"]", filepath.Join(pidDir, "ns", name)); err != nil {
			t.Fatal(err)
		}
	}
	status := "CapInh:\t0000000000000000\nCapPrm:\t0000000000000003\nCapEff:\t0000000000000001\nCapBnd:\t000001ffffffffff\nCapAmb:\t0000000000000000\nNoNewPrivs:\t1\nSeccomp:\t2\n"
	if err := os.WriteFile(filepath.Join(pidDir, "status"), []byte(status), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "cgroup"), []byte("0::/docker/"+strings.Repeat("a", 64)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "mountinfo"), []byte("10 1 8:1 / / rw,relatime shared:1 - ext4 /dev/root rw\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := NewProcFSScanner(root).ReadIsolation(321)
	if err != nil {
		t.Fatal(err)
	}
	if state.NamespaceInodes["mnt"] != 4026533001 ||
		state.CgroupVersion != 2 || state.ContainerRuntime != "docker" ||
		state.MountInfoDigest == "" || state.MountCount != 1 {
		t.Fatalf("incomplete isolation baseline: %#v", state)
	}
	if got := state.Completeness(); got != "complete" {
		t.Fatalf("completeness=%q, want complete: %#v", got, state.Availability)
	}
}

func TestDiffIsolationStateReportsBeforeAfterAndUnavailableEvidence(t *testing.T) {
	before := newIsolationState()
	before.NamespaceInodes = map[string]uint64{"mnt": 100, "pid": 101}
	before.CgroupPath = "/sandbox/a"
	before.MountInfoDigest = "sha256:before"
	before.Capabilities = CapabilityState{Visible: true, Effective: "0x0000000000000001"}
	before.Availability["namespaces"] = EvidenceAvailability{Available: true}
	before.Availability["cgroup"] = EvidenceAvailability{Available: true}
	before.Availability["mountinfo"] = EvidenceAvailability{Available: true}
	before.Availability["capabilities"] = EvidenceAvailability{Available: true}

	after := before
	after.NamespaceInodes = map[string]uint64{"mnt": 200, "pid": 101}
	after.CgroupPath = "/host"
	after.MountInfoDigest = "sha256:after"
	after.Capabilities.Effective = "0x0000000000000003"
	after.Availability = cloneAvailability(before.Availability)
	after.Availability["seccomp"] = EvidenceAvailability{Available: false, Reason: "proc_status_field_missing"}

	diff := DiffIsolationState(before, after)
	for _, key := range []string{
		"namespace.mnt", "cgroup_path", "mount_info_digest", "capabilities.effective_added",
	} {
		if _, ok := diff.Changes[key]; !ok {
			t.Fatalf("missing %s diff: %#v", key, diff)
		}
	}
	if !diff.StateChanged || diff.Changes["namespace.mnt"].Before != uint64(100) ||
		diff.Changes["namespace.mnt"].After != uint64(200) {
		t.Fatalf("before/after evidence invalid: %#v", diff)
	}
	if !containsString(diff.Unavailable, "seccomp:proc_status_field_missing") {
		t.Fatalf("unavailable evidence missing: %#v", diff.Unavailable)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

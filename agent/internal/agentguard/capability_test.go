package agentguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProbeGuardCapabilitiesReportsObservableFactsAndDegradedReasons(t *testing.T) {
	procRoot := t.TempDir()
	sysRoot := t.TempDir()
	mustWriteTestFile(t, filepath.Join(procRoot, "sys/kernel/osrelease"), "6.8.1-test\n")
	mustWriteTestFile(t, filepath.Join(procRoot, "self/mountinfo"), "1 0 8:1 / / rw - ext4 /dev/root rw\n")
	if err := os.MkdirAll(filepath.Join(procRoot, "self/ns"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("mnt:[4026533001]", filepath.Join(procRoot, "self/ns/mnt")); err != nil {
		t.Fatal(err)
	}
	mustWriteTestFile(t, filepath.Join(sysRoot, "kernel/btf/vmlinux"), "btf")
	mustWriteTestFile(t, filepath.Join(sysRoot, "fs/cgroup/cgroup.controllers"), "cpu memory")
	mustWriteTestFile(t, filepath.Join(sysRoot, "kernel/security/lsm"), "lockdown,yama,integrity,bpf")
	mustWriteTestFile(t, filepath.Join(sysRoot, "fs/cgroup/cgroup.freeze"), "0")

	capabilities := ProbeGuardCapabilities(procRoot, sysRoot)
	if capabilities.KernelRelease != "6.8.1-test" || !capabilities.BTF ||
		!capabilities.BPFLSM || capabilities.CgroupVersion != 2 ||
		!capabilities.CgroupFreeze || !capabilities.NamespaceRead ||
		!capabilities.MountInfoRead {
		t.Fatalf("capability facts incorrect: %#v", capabilities)
	}
	if len(capabilities.DegradedReasons) == 0 {
		t.Fatal("missing monitor hook visibility must be reported as degraded")
	}
}

func mustWriteTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

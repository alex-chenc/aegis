package agentguard

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// ProbeGuardCapabilities performs read-only capability discovery. Values are
// never promoted by configuration; missing kernel surfaces remain explicit
// degraded reasons.
func ProbeGuardCapabilities(procRoot, sysRoot string) GuardCapabilities {
	if procRoot == "" {
		procRoot = "/proc"
	}
	if sysRoot == "" {
		sysRoot = "/sys"
	}
	capabilities := GuardCapabilities{SupportedHooks: []string{}, DegradedReasons: []string{}}
	if data, err := os.ReadFile(filepath.Join(procRoot, "sys/kernel/osrelease")); err == nil {
		capabilities.KernelRelease = strings.TrimSpace(string(data))
	}
	major, minor := kernelMajorMinor(capabilities.KernelRelease)
	capabilities.PerfBuffer = major > 4 || (major == 4 && minor >= 4)
	capabilities.RingBuffer = major > 5 || (major == 5 && minor >= 8)
	_, err := os.Stat(filepath.Join(sysRoot, "kernel/btf/vmlinux"))
	capabilities.BTF = err == nil

	if data, err := os.ReadFile(filepath.Join(sysRoot, "kernel/security/lsm")); err == nil {
		for _, lsm := range strings.Split(strings.TrimSpace(string(data)), ",") {
			if strings.TrimSpace(lsm) == "bpf" {
				capabilities.BPFLSM = true
				break
			}
		}
	}
	switch {
	case fileExists(filepath.Join(sysRoot, "fs/cgroup/cgroup.controllers")):
		capabilities.CgroupVersion = 2
	case fileExists(filepath.Join(sysRoot, "fs/cgroup")):
		capabilities.CgroupVersion = 1
	}
	capabilities.CgroupFreeze = fileExists(filepath.Join(sysRoot, "fs/cgroup/cgroup.freeze"))
	capabilities.NamespaceRead = pathEntryExists(filepath.Join(procRoot, "self/ns/mnt"))
	capabilities.MountInfoRead = fileExists(filepath.Join(procRoot, "self/mountinfo"))

	if fd, err := unix.PidfdOpen(os.Getpid(), 0); err == nil {
		capabilities.Pidfd = true
		_ = unix.Close(fd)
	}
	traceRoots := []string{
		filepath.Join(sysRoot, "kernel/tracing/events/syscalls"),
		filepath.Join(sysRoot, "kernel/debug/tracing/events/syscalls"),
	}
	for _, operation := range []string{
		"setuid", "setgid", "capset", "setns", "unshare", "clone3",
		"mount", "pivot_root", "chroot", "ptrace", "bpf", "perf_event_open",
		"init_module", "finit_module", "delete_module",
	} {
		for _, traceRoot := range traceRoots {
			if fileExists(filepath.Join(traceRoot, "sys_enter_"+operation)) &&
				fileExists(filepath.Join(traceRoot, "sys_exit_"+operation)) {
				capabilities.SupportedHooks = append(capabilities.SupportedHooks, operation)
				break
			}
		}
	}
	sort.Strings(capabilities.SupportedHooks)
	addCapabilityReason := func(condition bool, reason string) {
		if !condition {
			capabilities.DegradedReasons = append(capabilities.DegradedReasons, reason)
		}
	}
	addCapabilityReason(capabilities.BTF, "btf_unavailable")
	addCapabilityReason(capabilities.RingBuffer || capabilities.PerfBuffer, "event_transport_unavailable")
	addCapabilityReason(capabilities.NamespaceRead, "namespace_read_unavailable")
	addCapabilityReason(capabilities.MountInfoRead, "mountinfo_read_unavailable")
	addCapabilityReason(len(capabilities.SupportedHooks) > 0, "agent_guard_monitor_hooks_unobservable")
	// P2 deliberately does not load an LSM. Record host support for rollout,
	// but never derive an active enforcement mode from it.
	if !capabilities.BPFLSM {
		capabilities.DegradedReasons = append(capabilities.DegradedReasons, "bpf_lsm_unavailable")
	}
	sort.Strings(capabilities.DegradedReasons)
	return capabilities
}

func kernelMajorMinor(release string) (int, int) {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return 0, 0
	}
	major, _ := strconv.Atoi(parts[0])
	minorText := parts[1]
	for index, value := range minorText {
		if value < '0' || value > '9' {
			minorText = minorText[:index]
			break
		}
	}
	minor, _ := strconv.Atoi(minorText)
	return major, minor
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pathEntryExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

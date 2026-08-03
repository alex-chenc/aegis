package agentguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var isolationDimensions = []string{
	"namespaces", "cgroup", "mountinfo", "capabilities", "no_new_privs", "seccomp",
}

func newIsolationState() IsolationState {
	availability := make(map[string]EvidenceAvailability, len(isolationDimensions)+1)
	for _, dimension := range isolationDimensions {
		availability[dimension] = EvidenceAvailability{
			Available: false,
			Reason:    "not_observed",
		}
	}
	availability["root_mount"] = EvidenceAvailability{
		Available: false,
		Reason:    "not_observed",
	}
	return IsolationState{
		NamespaceInodes:  make(map[string]uint64),
		MountPropagation: []string{},
		Availability:     availability,
		CapturedAt:       time.Now().UTC(),
	}
}

func cloneAvailability(in map[string]EvidenceAvailability) map[string]EvidenceAvailability {
	out := make(map[string]EvidenceAvailability, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (s IsolationState) Completeness() string {
	available := 0
	partial := false
	for _, dimension := range isolationDimensions {
		status := s.Availability[dimension]
		if status.Available {
			available++
			partial = partial || status.Reason != ""
		}
	}
	switch {
	case available == len(isolationDimensions) && !partial:
		return "complete"
	case available == 0:
		return "unavailable"
	default:
		return "partial"
	}
}

func (s IsolationState) Fingerprint() string {
	normalized := struct {
		NamespaceInodes map[string]uint64
		CgroupPath      string
		MountInfoDigest string
		Capabilities    CapabilityState
		NoNewPrivileges *bool
		SeccompMode     *int
	}{
		NamespaceInodes: s.NamespaceInodes, CgroupPath: s.CgroupPath,
		MountInfoDigest: s.MountInfoDigest, Capabilities: s.Capabilities,
		NoNewPrivileges: s.NoNewPrivileges, SeccompMode: s.SeccompMode,
	}
	data, _ := json.Marshal(normalized)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func parseIsolationStatus(data []byte, state *IsolationState) {
	if state == nil {
		return
	}
	if state.Availability == nil {
		*state = newIsolationState()
	}
	capabilityFields := map[string]*string{
		"CapInh": &state.Capabilities.Inheritable,
		"CapPrm": &state.Capabilities.Permitted,
		"CapEff": &state.Capabilities.Effective,
		"CapBnd": &state.Capabilities.Bounding,
		"CapAmb": &state.Capabilities.Ambient,
	}
	seenCapabilities := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if target, ok := capabilityFields[key]; ok {
			value := strings.ToLower(strings.TrimPrefix(fields[1], "0x"))
			if _, err := strconv.ParseUint(value, 16, 64); err == nil {
				*target = "0x" + leftPadHex(value)
				seenCapabilities++
			}
			continue
		}
		switch key {
		case "NoNewPrivs":
			if value, err := strconv.Atoi(fields[1]); err == nil && (value == 0 || value == 1) {
				enabled := value == 1
				state.NoNewPrivileges = &enabled
				state.Availability["no_new_privs"] = EvidenceAvailability{Available: true}
			}
		case "Seccomp":
			if value, err := strconv.Atoi(fields[1]); err == nil && value >= 0 && value <= 2 {
				state.SeccompMode = &value
				state.Availability["seccomp"] = EvidenceAvailability{Available: true}
			}
		}
	}
	if seenCapabilities == len(capabilityFields) {
		state.Capabilities.Visible = true
		state.Availability["capabilities"] = EvidenceAvailability{Available: true}
	} else {
		state.Capabilities.Visible = false
		state.Availability["capabilities"] = EvidenceAvailability{
			Available: false, Reason: "proc_status_field_missing",
		}
	}
	if state.NoNewPrivileges == nil {
		state.Availability["no_new_privs"] = EvidenceAvailability{
			Available: false, Reason: "proc_status_field_missing",
		}
	}
	if state.SeccompMode == nil {
		state.Availability["seccomp"] = EvidenceAvailability{
			Available: false, Reason: "proc_status_field_missing",
		}
	}
}

func leftPadHex(value string) string {
	if len(value) >= 16 {
		return value
	}
	return strings.Repeat("0", 16-len(value)) + value
}

// ReadIsolation captures security-relevant state from procfs without entering
// or mutating the target process namespaces.
func (s *ProcFSScanner) ReadIsolation(pid uint32) (IsolationState, error) {
	if pid == 0 {
		return IsolationState{}, fmt.Errorf("pid is zero")
	}
	dir := filepath.Join(s.root, strconv.FormatUint(uint64(pid), 10))
	if _, err := os.Stat(dir); err != nil {
		return IsolationState{}, fmt.Errorf("stat proc pid: %w", err)
	}
	state := newIsolationState()
	namespaceFailures := 0
	for _, name := range []string{"cgroup", "ipc", "mnt", "net", "pid", "time", "user", "uts"} {
		target, err := os.Readlink(filepath.Join(dir, "ns", name))
		if err != nil {
			namespaceFailures++
			continue
		}
		if _, inode, ok := parseNamespaceIdentity(target); ok {
			state.NamespaceInodes[name] = inode
		} else {
			namespaceFailures++
		}
	}
	if len(state.NamespaceInodes) > 0 {
		state.Availability["namespaces"] = EvidenceAvailability{Available: true}
		if namespaceFailures > 0 {
			state.Availability["namespaces"] = EvidenceAvailability{
				Available: true, Reason: "some_namespace_types_unavailable",
			}
		}
	} else {
		state.Availability["namespaces"] = EvidenceAvailability{
			Available: false, Reason: "proc_namespace_read_failed",
		}
	}

	status, err := os.ReadFile(filepath.Join(dir, "status"))
	if err == nil {
		parseIsolationStatus(status, &state)
	} else {
		for _, field := range []string{"capabilities", "no_new_privs", "seccomp"} {
			state.Availability[field] = EvidenceAvailability{
				Available: false, Reason: "proc_status_read_failed",
			}
		}
	}

	cgroupData, err := os.ReadFile(filepath.Join(dir, "cgroup"))
	if err == nil {
		state.CgroupPath, state.CgroupVersion = primaryCgroup(string(cgroupData))
		if container, ok := ParseContainerCgroup(string(cgroupData)); ok {
			state.CgroupPath = container.Path
			state.CgroupVersion = container.Version
			state.ContainerID = container.ContainerID
			state.ContainerRuntime = container.Runtime
		}
		state.Availability["cgroup"] = EvidenceAvailability{Available: true}
	} else {
		state.Availability["cgroup"] = EvidenceAvailability{
			Available: false, Reason: "proc_cgroup_read_failed",
		}
	}

	mountInfo, err := os.ReadFile(filepath.Join(dir, "mountinfo"))
	if err == nil {
		sum := sha256.Sum256(mountInfo)
		state.MountInfoDigest = "sha256:" + hex.EncodeToString(sum[:])
		state.MountCount, state.MountPropagation = mountInfoSummary(mountInfo)
		state.Availability["mountinfo"] = EvidenceAvailability{Available: true}
	} else {
		state.Availability["mountinfo"] = EvidenceAvailability{
			Available: false, Reason: "proc_mountinfo_read_failed",
		}
	}

	if info, err := os.Stat(filepath.Join(dir, "root")); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			state.RootMount = fmt.Sprintf("dev:%d:ino:%d", stat.Dev, stat.Ino)
			state.Availability["root_mount"] = EvidenceAvailability{Available: true}
		}
	}
	if state.RootMount == "" {
		state.Availability["root_mount"] = EvidenceAvailability{
			Available: false, Reason: "proc_root_stat_failed",
		}
	}
	state.CapturedAt = time.Now().UTC()
	return state, nil
}

func primaryCgroup(raw string) (string, int) {
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}
		version := 1
		if parts[0] == "0" && parts[1] == "" {
			version = 2
		}
		return normalizeCgroupPath(parts[2]), version
	}
	return "", 0
}

func mountInfoSummary(data []byte) (int, []string) {
	propagation := make(map[string]struct{})
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		count++
		for _, field := range fields[6:] {
			if field == "-" {
				break
			}
			for _, prefix := range []string{"shared:", "master:", "propagate_from:", "unbindable"} {
				if strings.HasPrefix(field, prefix) {
					propagation[field] = struct{}{}
				}
			}
		}
	}
	values := make([]string, 0, len(propagation))
	for value := range propagation {
		values = append(values, value)
	}
	sort.Strings(values)
	return count, values
}

func parseNamespaceIdentity(value string) (string, uint64, bool) {
	open := strings.LastIndex(value, ":[")
	if open <= 0 || !strings.HasSuffix(value, "]") {
		return "", 0, false
	}
	inode, err := strconv.ParseUint(value[open+2:len(value)-1], 10, 64)
	if err != nil || inode == 0 {
		return "", 0, false
	}
	return filepath.Base(value[:open]), inode, true
}

func DiffIsolationState(before, after IsolationState) IsolationDiff {
	diff := IsolationDiff{
		Changes:     make(map[string]StateDifference),
		Unavailable: unavailableIsolationDimensions(after),
	}
	namespaceKeys := make(map[string]struct{}, len(before.NamespaceInodes)+len(after.NamespaceInodes))
	for key := range before.NamespaceInodes {
		namespaceKeys[key] = struct{}{}
	}
	for key := range after.NamespaceInodes {
		namespaceKeys[key] = struct{}{}
	}
	for key := range namespaceKeys {
		if before.NamespaceInodes[key] != after.NamespaceInodes[key] {
			diff.Changes["namespace."+key] = StateDifference{
				Before: before.NamespaceInodes[key], After: after.NamespaceInodes[key],
			}
		}
	}
	addStringDifference(diff.Changes, "cgroup_path", before.CgroupPath, after.CgroupPath)
	addStringDifference(diff.Changes, "root_mount", before.RootMount, after.RootMount)
	addStringDifference(diff.Changes, "mount_info_digest", before.MountInfoDigest, after.MountInfoDigest)
	addPointerDifference(diff.Changes, "no_new_privs", before.NoNewPrivileges, after.NoNewPrivileges)
	addPointerDifference(diff.Changes, "seccomp_mode", before.SeccompMode, after.SeccompMode)
	added := addedCapabilityMask(before.Capabilities.Effective, after.Capabilities.Effective)
	if added != "" && added != "0x0000000000000000" {
		diff.Changes["capabilities.effective_added"] = StateDifference{
			Before: before.Capabilities.Effective, After: after.Capabilities.Effective,
		}
	}
	diff.StateChanged = len(diff.Changes) > 0
	return diff
}

func addStringDifference(changes map[string]StateDifference, key, before, after string) {
	if before != after {
		changes[key] = StateDifference{Before: before, After: after}
	}
}

func addPointerDifference[T comparable](changes map[string]StateDifference, key string, before, after *T) {
	if before == nil && after == nil {
		return
	}
	if before == nil || after == nil || *before != *after {
		var beforeValue, afterValue any
		if before != nil {
			beforeValue = *before
		}
		if after != nil {
			afterValue = *after
		}
		changes[key] = StateDifference{Before: beforeValue, After: afterValue}
	}
}

func addedCapabilityMask(before, after string) string {
	beforeValue, errBefore := strconv.ParseUint(strings.TrimPrefix(before, "0x"), 16, 64)
	afterValue, errAfter := strconv.ParseUint(strings.TrimPrefix(after, "0x"), 16, 64)
	if errBefore != nil || errAfter != nil {
		return ""
	}
	return fmt.Sprintf("0x%016x", afterValue&^beforeValue)
}

func unavailableIsolationDimensions(state IsolationState) []string {
	var out []string
	for key, value := range state.Availability {
		if value.Available {
			continue
		}
		reason := value.Reason
		if reason == "" {
			reason = "unavailable"
		}
		out = append(out, key+":"+reason)
	}
	sort.Strings(out)
	return out
}

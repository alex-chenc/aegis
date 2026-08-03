package ebpf

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

const ExecEventArgsTruncated uint32 = 1 << 0
const execArgSlotLen = 64

const (
	ForkEventTypeFork uint32 = iota + 1
	ForkEventTypeExit
)

const (
	GuardOperationSetUID uint32 = iota + 1
	GuardOperationSetGID
	GuardOperationCapset
	GuardOperationSetNS
	GuardOperationUnshare
	GuardOperationClone3
	GuardOperationMount
	GuardOperationPivotRoot
	GuardOperationChroot
	GuardOperationPtrace
	GuardOperationBPF
	GuardOperationPerfEventOpen
	GuardOperationInitModule
	GuardOperationFinitModule
	GuardOperationDeleteModule
	GuardOperationConnectUnix
)

const guardMonitorEventSize = 592
const agentGuardLSMEventSize = 328

const (
	GuardMonitorTargetTruncated    uint32 = 1 << 0
	GuardMonitorSecondaryTruncated uint32 = 1 << 1
)

// File action codes matching BPF constants
const (
	FileActionOpenWrite uint32 = 1
	FileActionCreate    uint32 = 2
	FileActionTruncate  uint32 = 3
	FileActionDelete    uint32 = 4
	FileActionRename    uint32 = 5
	FileActionChmod     uint32 = 6
	FileActionChown     uint32 = 7
	FileActionOpenRead  uint32 = 8
)

type ExecEvent struct {
	Pid      uint32
	Ppid     uint32
	Uid      uint32
	Gid      uint32
	Flags    uint32
	Comm     [16]byte
	Filename [256]byte
	Args     [512]byte
}

type ForkEvent struct {
	EventType  uint32
	PID        uint32
	PPID       uint32
	UID        uint32
	Comm       [16]byte
	ParentComm [16]byte
}

// FileEvent matches the BPF file_event struct.
type FileEvent struct {
	TimestampNs uint64
	Pid         uint32
	Tid         uint32
	Uid         uint32
	Gid         uint32
	Flags       int32
	Ret         int32
	Action      uint32
	Comm        [16]byte
	Path        [256]byte
	OldPath     [256]byte
}

// ConnEvent matches the BPF conn_event (tcp_connect) and accept_event (accept) structs.
// Layout must be kept in sync with both BPF structs.
type ConnEvent struct {
	TimestampNs uint64
	Pid         uint32
	Tid         uint32
	Uid         uint32
	Gid         uint32
	Family      uint16
	Protocol    uint16
	Sport       uint16
	Dport       uint16
	Ret         int32
	Comm        [16]byte
	Saddr       [16]byte
	Daddr       [16]byte
}

// GuardMonitorEvent matches guard_monitor.bpf.c. It intentionally carries
// bounded syscall metadata and paths only.
type GuardMonitorEvent struct {
	TimestampNS uint64
	PID         uint32
	TID         uint32
	UID         uint32
	GID         uint32
	Operation   uint32
	Flags       uint32
	Arg0        uint64
	Arg1        uint64
	Arg2        uint64
	ReturnCode  int64
	Comm        [16]byte
	Target      [256]byte
	Secondary   [256]byte
}

type AgentGuardLSMEvent struct {
	TimestampNS  uint64
	InstanceSlot uint64
	UnitSlot     uint64
	PolicySlot   uint64
	RuleSlot     uint64
	PID          uint32
	UID          uint32
	Operation    uint32
	Action       uint32
	Comm         [16]byte
	Resource     [256]byte
}

func bytesToString(b []byte) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}

func fileActionName(action uint32) string {
	switch action {
	case FileActionOpenWrite:
		return "open_write"
	case FileActionCreate:
		return "create"
	case FileActionTruncate:
		return "truncate"
	case FileActionDelete:
		return "delete"
	case FileActionRename:
		return "rename"
	case FileActionChmod:
		return "chmod"
	case FileActionChown:
		return "chown"
	case FileActionOpenRead:
		return "open_read"
	default:
		return fmt.Sprintf("unknown_%d", action)
	}
}

func filePathParts(path string) (dir, name string) {
	if path == "" {
		return "", ""
	}
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "", path
	}
	if idx == 0 {
		return "/", path[1:]
	}
	return path[:idx], path[idx+1:]
}

func parseIPv4(addr [16]byte) string {
	return net.IP(addr[:4]).String()
}

func parseIPv6(addr [16]byte) string {
	return net.IP(addr[:]).String()
}

func parseConnAddr(e *ConnEvent) (srcIP, dstIP string, srcPort, dstPort uint16) {
	srcPort = e.Sport
	dstPort = e.Dport
	if e.Family == 2 {
		srcIP = parseIPv4(e.Saddr)
		dstIP = parseIPv4(e.Daddr)
	} else if e.Family == 10 {
		srcIP = parseIPv6(e.Saddr)
		dstIP = parseIPv6(e.Daddr)
	}
	return
}

func connectStatusFromRet(ret int32) string {
	if ret == 0 {
		return "success"
	}
	if ret == -115 { // -EINPROGRESS
		return "in_progress"
	}
	return "failed"
}

func parseFlagsToStrings(flags int32) []string {
	var result []string
	if flags&0x0001 != 0 {
		result = append(result, "O_WRONLY")
	}
	if flags&0x0002 != 0 {
		result = append(result, "O_RDWR")
	}
	if flags&0x0040 != 0 {
		result = append(result, "O_CREAT")
	}
	if flags&0x0200 != 0 {
		result = append(result, "O_TRUNC")
	}
	if flags&0x0400 != 0 {
		result = append(result, "O_APPEND")
	}
	return result
}

func protocolName(proto uint16) string {
	switch proto {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return fmt.Sprintf("%d", proto)
	}
}

func parseFileEvent(data []byte) (*FileEvent, error) {
	if len(data) < 564 {
		return nil, fmt.Errorf("file event data too short: %d", len(data))
	}
	e := &FileEvent{}
	e.TimestampNs = binary.LittleEndian.Uint64(data[0:8])
	e.Pid = binary.LittleEndian.Uint32(data[8:12])
	e.Tid = binary.LittleEndian.Uint32(data[12:16])
	e.Uid = binary.LittleEndian.Uint32(data[16:20])
	e.Gid = binary.LittleEndian.Uint32(data[20:24])
	e.Flags = int32(binary.LittleEndian.Uint32(data[24:28]))
	e.Ret = int32(binary.LittleEndian.Uint32(data[28:32]))
	e.Action = binary.LittleEndian.Uint32(data[32:36])
	copy(e.Comm[:], data[36:52])
	copy(e.Path[:], data[52:308])
	copy(e.OldPath[:], data[308:564])
	return e, nil
}

func parseConnEvent(data []byte) (*ConnEvent, error) {
	if len(data) < 84 {
		return nil, fmt.Errorf("conn event data too short: %d", len(data))
	}
	e := &ConnEvent{}
	e.TimestampNs = binary.LittleEndian.Uint64(data[0:8])
	e.Pid = binary.LittleEndian.Uint32(data[8:12])
	e.Tid = binary.LittleEndian.Uint32(data[12:16])
	e.Uid = binary.LittleEndian.Uint32(data[16:20])
	e.Gid = binary.LittleEndian.Uint32(data[20:24])
	e.Family = binary.LittleEndian.Uint16(data[24:26])
	e.Protocol = binary.LittleEndian.Uint16(data[26:28])
	e.Sport = binary.LittleEndian.Uint16(data[28:30])
	e.Dport = binary.LittleEndian.Uint16(data[30:32])
	e.Ret = int32(binary.LittleEndian.Uint32(data[32:36]))
	copy(e.Comm[:], data[36:52])
	copy(e.Saddr[:], data[52:68])
	copy(e.Daddr[:], data[68:84])
	return e, nil
}

func parseGuardMonitorEvent(data []byte) (*GuardMonitorEvent, error) {
	if len(data) < guardMonitorEventSize {
		return nil, fmt.Errorf("guard monitor event data too short: %d", len(data))
	}
	event := &GuardMonitorEvent{}
	event.TimestampNS = binary.LittleEndian.Uint64(data[0:8])
	event.PID = binary.LittleEndian.Uint32(data[8:12])
	event.TID = binary.LittleEndian.Uint32(data[12:16])
	event.UID = binary.LittleEndian.Uint32(data[16:20])
	event.GID = binary.LittleEndian.Uint32(data[20:24])
	event.Operation = binary.LittleEndian.Uint32(data[24:28])
	event.Flags = binary.LittleEndian.Uint32(data[28:32])
	event.Arg0 = binary.LittleEndian.Uint64(data[32:40])
	event.Arg1 = binary.LittleEndian.Uint64(data[40:48])
	event.Arg2 = binary.LittleEndian.Uint64(data[48:56])
	event.ReturnCode = int64(binary.LittleEndian.Uint64(data[56:64]))
	copy(event.Comm[:], data[64:80])
	copy(event.Target[:], data[80:336])
	copy(event.Secondary[:], data[336:592])
	return event, nil
}

func parseAgentGuardLSMEvent(data []byte) (*AgentGuardLSMEvent, error) {
	if len(data) < agentGuardLSMEventSize {
		return nil, fmt.Errorf("agent guard LSM event data too short: %d", len(data))
	}
	event := &AgentGuardLSMEvent{}
	event.TimestampNS = binary.LittleEndian.Uint64(data[0:8])
	event.InstanceSlot = binary.LittleEndian.Uint64(data[8:16])
	event.UnitSlot = binary.LittleEndian.Uint64(data[16:24])
	event.PolicySlot = binary.LittleEndian.Uint64(data[24:32])
	event.RuleSlot = binary.LittleEndian.Uint64(data[32:40])
	event.PID = binary.LittleEndian.Uint32(data[40:44])
	event.UID = binary.LittleEndian.Uint32(data[44:48])
	event.Operation = binary.LittleEndian.Uint32(data[48:52])
	event.Action = binary.LittleEndian.Uint32(data[52:56])
	copy(event.Comm[:], data[56:72])
	copy(event.Resource[:], data[72:328])
	return event, nil
}

func agentGuardLSMOperation(operation uint32) (category, name string) {
	switch operation {
	case 5:
		return "process", "execute"
	case 6:
		return "network", "connect"
	case 7:
		return "isolation", "setns"
	case 8:
		return "isolation", "mount"
	case 9:
		return "kernel", "ptrace"
	case 10:
		return "kernel", "bpf"
	default:
		return "kernel", fmt.Sprintf("lsm_operation_%d", operation)
	}
}

func guardOperationName(operation uint32) (category, name string) {
	switch operation {
	case GuardOperationSetUID:
		return "identity", "setuid"
	case GuardOperationSetGID:
		return "identity", "setgid"
	case GuardOperationCapset:
		return "identity", "capset"
	case GuardOperationSetNS:
		return "isolation", "setns"
	case GuardOperationUnshare:
		return "isolation", "unshare"
	case GuardOperationClone3:
		return "isolation", "clone3"
	case GuardOperationMount:
		return "isolation", "mount"
	case GuardOperationPivotRoot:
		return "isolation", "pivot_root"
	case GuardOperationChroot:
		return "isolation", "chroot"
	case GuardOperationPtrace:
		return "kernel", "ptrace"
	case GuardOperationBPF:
		return "kernel", "bpf"
	case GuardOperationPerfEventOpen:
		return "kernel", "perf_event_open"
	case GuardOperationInitModule:
		return "kernel", "init_module"
	case GuardOperationFinitModule:
		return "kernel", "finit_module"
	case GuardOperationDeleteModule:
		return "kernel", "delete_module"
	case GuardOperationConnectUnix:
		return "isolation", "connect_unix"
	default:
		return "", ""
	}
}

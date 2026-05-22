package ebpf

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

const ExecEventArgsTruncated uint32 = 1 << 0
const execArgSlotLen = 64

// File action codes matching BPF constants
const (
	FileActionOpenWrite uint32 = 1
	FileActionCreate    uint32 = 2
	FileActionTruncate  uint32 = 3
	FileActionDelete    uint32 = 4
	FileActionRename    uint32 = 5
	FileActionChmod     uint32 = 6
	FileActionChown     uint32 = 7
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
	ParentPid  uint32
	ChildPid   uint32
	Uid        uint32
	ParentComm [16]byte
	ChildComm  [16]byte
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

// ConnEvent matches the BPF conn_event struct.
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

package tools

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Connection struct {
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr"`
	RemotePort int    `json:"remote_port"`
	State      string `json:"state"`
}

type NetworkConnections struct {
	PID         int          `json:"pid"`
	Connections []Connection `json:"connections"`
}

func (m *ToolManager) GetNetworkConnections(pid int) (*NetworkConnections, error) {
	inodes := processSocketInodes(pid)

	data, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	connections := make([]Connection, 0)
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		inode := fields[9]
		if len(inodes) > 0 {
			if _, ok := inodes[inode]; !ok {
				continue
			}
		}

		localAddr, localPort := parseHexAddr(fields[1])
		remoteAddr, remotePort := parseHexAddr(fields[2])
		connections = append(connections, Connection{
			LocalAddr:  localAddr,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
			State:      tcpState(fields[3]),
		})
	}

	return &NetworkConnections{PID: pid, Connections: connections}, nil
}

func processSocketInodes(pid int) map[string]struct{} {
	result := make(map[string]struct{})
	fds, err := filepath.Glob(fmt.Sprintf("/proc/%d/fd/*", pid))
	if err != nil {
		return result
	}

	for _, fd := range fds {
		target, err := os.Readlink(fd)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
			continue
		}
		inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
		if inode != "" {
			result[inode] = struct{}{}
		}
	}

	return result
}

func parseHexAddr(value string) (string, int) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return "", 0
	}

	addrHex := parts[0]
	portHex := parts[1]
	if len(addrHex) != 8 {
		return "", 0
	}

	bytesAddr, err := hex.DecodeString(addrHex)
	if err != nil || len(bytesAddr) != 4 {
		return "", 0
	}

	addr := fmt.Sprintf("%d.%d.%d.%d", bytesAddr[3], bytesAddr[2], bytesAddr[1], bytesAddr[0])
	port64, err := strconv.ParseInt(portHex, 16, 32)
	if err != nil {
		return addr, 0
	}
	return addr, int(port64)
}

func tcpState(code string) string {
	switch strings.ToUpper(code) {
	case "01":
		return "ESTABLISHED"
	case "02":
		return "SYN_SENT"
	case "03":
		return "SYN_RECV"
	case "04":
		return "FIN_WAIT1"
	case "05":
		return "FIN_WAIT2"
	case "06":
		return "TIME_WAIT"
	case "07":
		return "CLOSE"
	case "08":
		return "CLOSE_WAIT"
	case "09":
		return "LAST_ACK"
	case "0A":
		return "LISTEN"
	case "0B":
		return "CLOSING"
	default:
		return code
	}
}

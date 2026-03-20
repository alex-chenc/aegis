package tools

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

type ProcessTree struct {
	PID         int           `json:"pid"`
	PPID        int           `json:"ppid"`
	Name        string        `json:"name"`
	CommandLine string        `json:"command_line"`
	User        string        `json:"user"`
	Children    []ProcessInfo `json:"children"`
}

type ProcessInfo struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

func (m *ToolManager) GetProcessTree(pid int) (*ProcessTree, error) {
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil, err
	}

	cmdlineData, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	cmdline := strings.TrimSpace(strings.ReplaceAll(string(cmdlineData), "\x00", " "))

	commData, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	comm := strings.TrimSpace(string(commData))

	fields := strings.Fields(string(statData))
	ppid := 0
	if len(fields) > 3 {
		ppid, _ = strconv.Atoi(fields[3])
	}

	uid := getProcessUID(pid)
	u, _ := user.LookupId(strconv.Itoa(uid))
	username := ""
	if u != nil {
		username = u.Username
	}

	return &ProcessTree{
		PID:         pid,
		PPID:        ppid,
		Name:        comm,
		CommandLine: cmdline,
		User:        username,
		Children:    findChildren(pid),
	}, nil
}

func findChildren(pid int) []ProcessInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	children := make([]ProcessInfo, 0)
	for _, entry := range entries {
		childPID, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", childPID))
		if err != nil {
			continue
		}
		fields := strings.Fields(string(statData))
		if len(fields) <= 3 {
			continue
		}
		childPPID, err := strconv.Atoi(fields[3])
		if err != nil || childPPID != pid {
			continue
		}

		commData, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", childPID))
		children = append(children, ProcessInfo{
			PID:  childPID,
			Name: strings.TrimSpace(string(commData)),
		})
	}

	return children
}

func getProcessUID(pid int) int {
	statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(statusData), "\n") {
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return 0
		}
		uid, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0
		}
		return uid
	}
	return 0
}

package tools

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RunningProcess represents basic process information for running processes list
type RunningProcess struct {
	PID         int    `json:"pid"`
	Name        string `json:"name"`
	CommandLine string `json:"command_line"`
	User        string `json:"user"`
	State       string `json:"state"`
}

// GetRunningProcesses gets list of running processes
func (m *ToolManager) GetRunningProcesses(args map[string]interface{}) (interface{}, error) {
	filter := ""
	if f, ok := args["filter"].(string); ok {
		filter = strings.ToLower(f)
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}

	processes := []RunningProcess{}
	count := 0
	maxProcesses := 500

	for _, entry := range entries {
		if count >= maxProcesses {
			break
		}

		pidStr := entry.Name()
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Read process info
		commPath := fmt.Sprintf("/proc/%d/comm", pid)
		commData, err := os.ReadFile(commPath)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(commData))

		// Apply filter if specified
		if filter != "" && !strings.Contains(strings.ToLower(name), filter) {
			continue
		}

		// Read cmdline
		cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
		cmdlineData, _ := os.ReadFile(cmdlinePath)
		cmdline := strings.TrimSpace(strings.ReplaceAll(string(cmdlineData), "\x00", " "))

		// Get process state
		statePath := fmt.Sprintf("/proc/%d/status", pid)
		stateData, _ := os.ReadFile(statePath)
		state := "unknown"
		for _, line := range strings.Split(string(stateData), "\n") {
			if strings.HasPrefix(line, "State:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					state = parts[1]
				}
				break
			}
		}

		// Get user
		uid := getProcessUID(pid)
		username, _ := getUserInfo(uid)

		processes = append(processes, RunningProcess{
			PID:         pid,
			Name:        name,
			CommandLine: cmdline,
			User:        username,
			State:       state,
		})
		count++
	}

	return map[string]interface{}{
		"processes": processes,
		"count":     len(processes),
	}, nil
}
package tools

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

type ProcessTree struct {
	PID             int           `json:"pid"`
	PPID            int           `json:"ppid"`
	Name            string        `json:"name"`
	CommandLine     string        `json:"command_line"`
	User            string        `json:"user"`
	UserGroup       string        `json:"user_group"`
	ExePath         string        `json:"exe_path"`
	PPIDCommandLine string        `json:"ppid_command_line"`
	PPIDUser        string        `json:"ppid_user"`
	Children        []ProcessInfo `json:"children"`
}

type ProcessInfo struct {
	PID         int    `json:"pid"`
	Name        string `json:"name"`
	CommandLine string `json:"command_line"`
	User        string `json:"user"`
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

	exePath, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))

	fields := strings.Fields(string(statData))
	ppid := 0
	if len(fields) > 3 {
		ppid, _ = strconv.Atoi(fields[3])
	}

	uid := getProcessUID(pid)
	username, userGroup := getUserInfo(uid)

	tree := &ProcessTree{
		PID:         pid,
		PPID:        ppid,
		Name:        comm,
		CommandLine: cmdline,
		User:        username,
		UserGroup:   userGroup,
		ExePath:     exePath,
		Children:    findChildren(pid),
	}

	if ppid > 0 {
		ppidCmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", ppid))
		tree.PPIDCommandLine = strings.TrimSpace(strings.ReplaceAll(string(ppidCmdline), "\x00", " "))

		ppidUID := getProcessUID(ppid)
		ppidUser, _ := getUserInfo(ppidUID)
		tree.PPIDUser = ppidUser
	}

	return tree, nil
}

func getUserInfo(uid int) (username, userGroup string) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", ""
	}
	username = u.Username

	g, err := user.LookupGroupId(u.Gid)
	if err == nil {
		userGroup = g.Name
	}

	return username, userGroup
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
		cmdlineData, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", childPID))
		cmdline := strings.TrimSpace(strings.ReplaceAll(string(cmdlineData), "\x00", " "))

		uid := getProcessUID(childPID)
		username, _ := getUserInfo(uid)

		children = append(children, ProcessInfo{
			PID:         childPID,
			Name:        strings.TrimSpace(string(commData)),
			CommandLine: cmdline,
			User:        username,
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

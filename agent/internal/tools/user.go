package tools

import (
	"bufio"
	"os"
	"os/user"
	"strconv"
	"strings"
)

type UserInfo struct {
	Username string   `json:"username"`
	UID      int      `json:"uid"`
	GID      int      `json:"gid"`
	Groups   []string `json:"groups"`
	Shell    string   `json:"shell"`
	HomeDir  string   `json:"home_dir"`
	IsLocked bool     `json:"is_locked"`
}

func (m *ToolManager) GetUserInfo(username string) (*UserInfo, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return nil, err
	}

	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	groupIDs, _ := u.GroupIds()
	groupNames := make([]string, 0, len(groupIDs))
	for _, gidStr := range groupIDs {
		g, err := user.LookupGroupId(gidStr)
		if err == nil {
			groupNames = append(groupNames, g.Name)
		}
	}

	shell, locked := userShellAndLock(username)
	return &UserInfo{
		Username: u.Username,
		UID:      uid,
		GID:      gid,
		Groups:   groupNames,
		Shell:    shell,
		HomeDir:  u.HomeDir,
		IsLocked: locked,
	}, nil
}

func userShellAndLock(username string) (string, bool) {
	passwd, err := os.Open("/etc/passwd")
	if err != nil {
		return "", false
	}
	defer passwd.Close()

	shell := ""
	scanner := bufio.NewScanner(passwd)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, username+":") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) >= 7 {
			shell = parts[6]
		}
		break
	}

	shadow, err := os.Open("/etc/shadow")
	if err != nil {
		return shell, false
	}
	defer shadow.Close()

	locked := false
	shadowScan := bufio.NewScanner(shadow)
	for shadowScan.Scan() {
		line := shadowScan.Text()
		if !strings.HasPrefix(line, username+":") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) > 1 {
			passwordField := parts[1]
			locked = strings.HasPrefix(passwordField, "!") || strings.HasPrefix(passwordField, "*")
		}
		break
	}

	return shell, locked
}

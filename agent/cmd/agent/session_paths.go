package main

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// resolveAgentHomeDir keeps static session discovery independent of the
// service manager's environment. systemd commonly starts the Agent without a
// HOME variable; falling back to the current account prevents roots such as
// ".codex/sessions" from being resolved relative to the Agent workdir.
func resolveAgentHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && isAbsoluteDirectory(home) {
		return filepath.Clean(home)
	}
	if current, err := user.Current(); err == nil && isAbsoluteDirectory(current.HomeDir) {
		return filepath.Clean(current.HomeDir)
	}
	return ""
}

func isAbsoluteDirectory(path string) bool {
	return strings.TrimSpace(path) != "" && filepath.IsAbs(path)
}

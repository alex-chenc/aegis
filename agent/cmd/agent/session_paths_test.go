package main

import (
	"os/user"
	"path/filepath"
	"testing"
)

func TestResolveAgentHomeDirFallsBackWhenHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")

	current, err := user.Current()
	if err != nil {
		t.Skipf("current user lookup unavailable: %v", err)
	}
	if got := resolveAgentHomeDir(); got != filepath.Clean(current.HomeDir) {
		t.Fatalf("resolveAgentHomeDir() = %q, want %q", got, filepath.Clean(current.HomeDir))
	}
}

func TestResolveAgentHomeDirPrefersHomeEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := resolveAgentHomeDir(); got != filepath.Clean(home) {
		t.Fatalf("resolveAgentHomeDir() = %q, want %q", got, filepath.Clean(home))
	}
}

func TestResolveAgentHomeDirDoesNotUseRelativeHome(t *testing.T) {
	t.Setenv("HOME", "relative-home")
	current, err := user.Current()
	if err != nil {
		t.Skipf("current user lookup unavailable: %v", err)
	}
	if got := resolveAgentHomeDir(); got != filepath.Clean(current.HomeDir) {
		t.Fatalf("resolveAgentHomeDir() = %q, want current home %q", got, filepath.Clean(current.HomeDir))
	}
}

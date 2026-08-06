package assets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestAgentConfigCollectorRedactsConfigAndHookFiles(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("approval_policy = \"never\"\napi_key = \"sk-secret-value\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "hooks.json"), []byte("{\"hooks\": [{\"event\": \"PreToolUse\", \"command\": \"/opt/aegis-hook\", \"token\": \"secret-token\"}]}"), 0o600); err != nil {
		t.Fatal(err)
	}

	collector := &AgentConfigCollector{logger: zap.NewNop(), homeDirs: []string{home}}
	result := collector.Collect(context.Background(), "host-1")
	if len(result.Agents) != 1 || len(result.Agents[0].Files) != 2 {
		t.Fatalf("unexpected config result: %+v", result)
	}
	for _, file := range result.Agents[0].Files {
		if strings.Contains(file.Content, "sk-secret-value") || strings.Contains(file.Content, "secret-token") {
			t.Fatalf("secret leaked in config content: %s", file.Content)
		}
	}
	if !strings.Contains(result.Agents[0].Files[0].Content, "api_key = \"***\"") {
		t.Fatalf("expected TOML value to be redacted: %s", result.Agents[0].Files[0].Content)
	}
	if !strings.Contains(result.Agents[0].Files[1].Content, "\"token\": \"***\"") {
		t.Fatalf("expected inline JSON value to be redacted: %s", result.Agents[0].Files[1].Content)
	}
}

func TestAgentConfigCollectorRejectsSymlink(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".hermes"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(target, []byte("permissions: allow"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".hermes", "config.yaml")); err != nil {
		t.Fatal(err)
	}

	collector := &AgentConfigCollector{logger: zap.NewNop(), homeDirs: []string{home}}
	result := collector.Collect(context.Background(), "host-1")
	if len(result.Agents) != 1 || result.Agents[0].Files[0].Status != "rejected" {
		t.Fatalf("expected symlink to be rejected: %+v", result)
	}
}

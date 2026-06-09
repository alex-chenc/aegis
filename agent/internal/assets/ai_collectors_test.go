package assets

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestAIAgentCollectorScansMultipleHomeDirs(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	rootHome := filepath.Join(base, "root")
	userHome := filepath.Join(base, "user")
	mustMkdir(t, filepath.Join(rootHome, ".claude"))
	mustMkdir(t, filepath.Join(userHome, ".cursor"))
	mustWriteFile(t, filepath.Join(rootHome, ".claude", "version"), "1.2.3\n")
	mustWriteFile(t, filepath.Join(userHome, ".cursor", "package.json"), `{"name":"cursor","version":"4.5.6"}`)

	collector := &AIAgentCollector{
		logger:   zap.NewNop(),
		homeDirs: []string{rootHome, userHome},
	}

	assets := collector.Collect(context.Background())
	byName := aiAssetsByName(assets)

	claude := byName["claude-code"]
	if claude == nil {
		t.Fatalf("expected claude-code asset, got %#v", assets)
	}
	if claude.Version != "1.2.3" {
		t.Fatalf("expected claude-code version 1.2.3, got %q", claude.Version)
	}
	if claude.Extra["home_dir"] != rootHome {
		t.Fatalf("expected claude-code home_dir %q, got %q", rootHome, claude.Extra["home_dir"])
	}

	cursor := byName["cursor"]
	if cursor == nil {
		t.Fatalf("expected cursor asset, got %#v", assets)
	}
	if cursor.Version != "4.5.6" {
		t.Fatalf("expected cursor package.json version 4.5.6, got %q", cursor.Version)
	}
	if cursor.ConfigPath != filepath.Join(userHome, ".cursor") {
		t.Fatalf("expected cursor config path in second home, got %q", cursor.ConfigPath)
	}
}

func TestMCPCollectorScansHomeAndProjectConfigs(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	project := filepath.Join(base, "project")
	mustMkdir(t, filepath.Join(home, ".claude"))
	mustMkdir(t, project)
	mustWriteFile(t, filepath.Join(home, ".claude", "mcp.json"), `{
		"mcpServers": {
			"filesystem": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem"]}
		}
	}`)
	mustWriteFile(t, filepath.Join(project, ".mcp.json"), `{
		"mcpServers": {
			"browser": {"url": "http://127.0.0.1:3000/sse"}
		}
	}`)

	collector := &MCPCollector{
		logger:      zap.NewNop(),
		homeDirs:    []string{home},
		projectDirs: []string{project},
	}

	assets := collector.Collect(context.Background())
	byName := aiAssetsByName(assets)

	filesystem := byName["filesystem"]
	if filesystem == nil {
		t.Fatalf("expected home MCP server asset, got %#v", assets)
	}
	if filesystem.Extra["agent"] != "claude-code" {
		t.Fatalf("expected claude-code agent, got %q", filesystem.Extra["agent"])
	}
	if filesystem.Extra["home_dir"] != home {
		t.Fatalf("expected home_dir %q, got %q", home, filesystem.Extra["home_dir"])
	}
	if filesystem.Extra["command_line"] == "" {
		t.Fatalf("expected command_line to be captured, got %#v", filesystem.Extra)
	}

	browser := byName["browser"]
	if browser == nil {
		t.Fatalf("expected project MCP server asset, got %#v", assets)
	}
	if browser.Extra["scope"] != "project" {
		t.Fatalf("expected project scope, got %q", browser.Extra["scope"])
	}
	if browser.Extra["transport"] != "sse" {
		t.Fatalf("expected sse transport, got %q", browser.Extra["transport"])
	}
}

func aiAssetsByName(items []AIAsset) map[string]*AIAsset {
	result := make(map[string]*AIAsset)
	for i := range items {
		result[items[i].Name] = &items[i]
	}
	return result
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

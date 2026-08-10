package assets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestMCPCollectorScansCodexConfigTOMLAndRedactsSecrets(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	mustMkdir(t, filepath.Join(home, ".codex"))
	mustWriteFile(t, filepath.Join(home, ".codex", "config.toml"), `[mcp_servers."aegis-hosts"]
command = "python3"
args = ["/opt/aegis-mcp.py", "--token", "super-secret"]
bearer_token_env_var = "AEGIS_MCP_TOKEN"

[mcp_servers."aegis-hosts".env]
AEGIS_API_TOKEN_FILE = "/root/.config/aegis/mcp-token"

[mcp_servers.remote]
url = "https://user:password@example.test/mcp?token=secret-value"
enabled = false
`)

	collector := &MCPCollector{
		logger:      zap.NewNop(),
		homeDirs:    []string{home},
		projectDirs: nil,
	}

	assets := collector.Collect(context.Background())
	byName := aiAssetsByName(assets)

	aegisHosts := byName["aegis-hosts"]
	if aegisHosts == nil {
		t.Fatalf("expected Codex MCP server asset, got %#v", assets)
	}
	if aegisHosts.Extra["agent"] != "codex" {
		t.Fatalf("expected codex agent, got %q", aegisHosts.Extra["agent"])
	}
	if aegisHosts.Extra["transport"] != "stdio" {
		t.Fatalf("expected stdio transport, got %q", aegisHosts.Extra["transport"])
	}
	if aegisHosts.Extra["env_keys"] != "AEGIS_API_TOKEN_FILE" {
		t.Fatalf("expected only environment variable names, got %q", aegisHosts.Extra["env_keys"])
	}
	if aegisHosts.Extra["bearer_token_env_var"] != "AEGIS_MCP_TOKEN" {
		t.Fatalf("expected bearer token environment variable name, got %q", aegisHosts.Extra["bearer_token_env_var"])
	}
	if strings.Contains(aegisHosts.Extra["command_line"], "super-secret") {
		t.Fatalf("command line leaked a token: %q", aegisHosts.Extra["command_line"])
	}

	remote := byName["remote"]
	if remote == nil {
		t.Fatalf("expected Codex remote MCP server asset, got %#v", assets)
	}
	if remote.Extra["transport"] != "streamable_http" {
		t.Fatalf("expected streamable_http transport, got %q", remote.Extra["transport"])
	}
	if remote.Extra["enabled"] != "false" {
		t.Fatalf("expected disabled state to be captured, got %q", remote.Extra["enabled"])
	}
	if strings.Contains(remote.Extra["url"], "password") || strings.Contains(remote.Extra["url"], "secret-value") {
		t.Fatalf("URL leaked credentials: %q", remote.Extra["url"])
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

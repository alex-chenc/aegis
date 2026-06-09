package assets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"
)

// MCPCollector MCP Server 配置解析器
// 扫描 AI Agent 的 MCP 配置文件，提取 MCP Server 定义
// 参考 Snyk Agent Scan 的 MCP 配置发现逻辑
type MCPCollector struct {
	logger      *zap.Logger
	homeDir     string
	homeDirs    []string
	projectDirs []string
}

// MCPConfig mcp.json 配置结构
type MCPConfig struct {
	MCPServers map[string]MCPServerDef `json:"mcpServers"`
}

// MCPServerDef 单个 MCP Server 定义
type MCPServerDef struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	URL       string            `json:"url"`
	Transport string            `json:"transport"`
}

// mcpScanTarget MCP 配置文件扫描目标
type mcpScanTarget struct {
	AgentName string // 关联的 Agent 名称
	Path      string // 配置文件路径（相对于 home）
}

// NewMCPCollector 创建 MCP Server 配置解析器
func NewMCPCollector(logger *zap.Logger) *MCPCollector {
	homeDirs := discoverHomeDirs()
	home := ""
	if len(homeDirs) > 0 {
		home = homeDirs[0]
	}
	return &MCPCollector{
		logger:      logger,
		homeDir:     home,
		homeDirs:    homeDirs,
		projectDirs: discoverProjectDirs(),
	}
}

// Collect 扫描并解析 MCP 配置文件
func (c *MCPCollector) Collect(ctx context.Context) []AIAsset {
	var results []AIAsset
	seen := make(map[string]bool)
	homeDirs := c.homeDirs
	if len(homeDirs) == 0 && c.homeDir != "" {
		homeDirs = []string{c.homeDir}
	}

	for _, homeDir := range homeDirs {
		select {
		case <-ctx.Done():
			c.logger.Warn("MCP config collection cancelled", zap.Error(ctx.Err()))
			return results
		default:
		}

		for _, target := range mcpScanTargets() {
			fullPath := expandPath(target.Path, homeDir)
			if !pathExists(fullPath) {
				continue
			}

			config, err := c.parseMCPConfig(fullPath)
			if err != nil {
				c.logger.Debug("Failed to parse MCP config",
					zap.String("path", fullPath),
					zap.Error(err))
				continue
			}

			for serverName, serverDef := range config.MCPServers {
				key := target.AgentName + ":" + serverName + ":" + fullPath
				if seen[key] {
					continue
				}
				seen[key] = true

				extra := c.buildMCPExtra(target.AgentName, fullPath, serverDef, map[string]string{
					"home_dir": homeDir,
				})

				asset := AIAsset{
					Category:    "mcp_server",
					Name:        serverName,
					DisplayName: serverName,
					Source:      "config",
					ConfigPath:  fullPath,
					Extra:       extra,
				}
				results = append(results, asset)
				c.logger.Info("MCP server detected",
					zap.String("server", serverName),
					zap.String("agent", target.AgentName),
					zap.String("transport", extra["transport"]),
					zap.String("config", fullPath))
			}
		}
	}

	for _, asset := range c.ScanProjectMCPConfigs(ctx, c.projectDirs) {
		key := "project:" + asset.Name + ":" + asset.ConfigPath
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, asset)
	}

	return results
}

// parseMCPConfig 解析 MCP 配置文件
func (c *MCPCollector) parseMCPConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// 如果 mcpServers 为空，尝试解析 Claude Desktop 格式
	if len(config.MCPServers) == 0 {
		var altConfig struct {
			MCPServers map[string]MCPServerDef `json:"mcp_servers"`
		}
		if err := json.Unmarshal(data, &altConfig); err == nil && len(altConfig.MCPServers) > 0 {
			config.MCPServers = altConfig.MCPServers
		}
	}

	return &config, nil
}

func (c *MCPCollector) buildMCPExtra(agentName, configPath string, serverDef MCPServerDef, extra map[string]string) map[string]string {
	transport := serverDef.Transport
	if transport == "" {
		if serverDef.URL != "" {
			transport = "sse"
		} else {
			transport = "stdio"
		}
	}

	if extra == nil {
		extra = make(map[string]string)
	}
	extra["agent"] = agentName
	extra["transport"] = transport
	extra["config"] = configPath
	if serverDef.Command != "" {
		extra["command"] = serverDef.Command
	}
	if len(serverDef.Args) > 0 {
		extra["command_line"] = strings.TrimSpace(serverDef.Command + " " + strings.Join(serverDef.Args, " "))
	}
	if serverDef.URL != "" {
		extra["url"] = serverDef.URL
	}
	return extra
}

// mcpScanTargets 返回 MCP 配置文件扫描目标
func mcpScanTargets() []mcpScanTarget {
	if runtime.GOOS == "darwin" {
		return darwinMCPTargets()
	}
	return linuxMCPTargets()
}

// linuxMCPTargets Linux 平台的 MCP 配置文件路径
func linuxMCPTargets() []mcpScanTarget {
	return []mcpScanTarget{
		{AgentName: "claude-code", Path: "~/.claude/mcp.json"},
		{AgentName: "claude-desktop", Path: "~/.config/claude/mcp.json"},
		{AgentName: "cursor", Path: "~/.cursor/mcp.json"},
		{AgentName: "windsurf", Path: "~/.windsurf/mcp.json"},
		{AgentName: "vscode", Path: "~/.vscode/mcp.json"},
	}
}

// darwinMCPTargets macOS 平台的 MCP 配置文件路径
func darwinMCPTargets() []mcpScanTarget {
	return []mcpScanTarget{
		{AgentName: "claude-code", Path: "~/.claude/mcp.json"},
		{AgentName: "claude-desktop", Path: "~/Library/Application Support/Claude/claude_desktop_config.json"},
		{AgentName: "cursor", Path: "~/.cursor/mcp.json"},
		{AgentName: "windsurf", Path: "~/.windsurf/mcp.json"},
		{AgentName: "vscode", Path: "~/.vscode/mcp.json"},
	}
}

// ScanProjectMCPConfigs 扫描项目目录中的 .mcp.json 文件
func (c *MCPCollector) ScanProjectMCPConfigs(ctx context.Context, projectDirs []string) []AIAsset {
	var results []AIAsset

	for _, dir := range projectDirs {
		select {
		case <-ctx.Done():
			c.logger.Warn("Project MCP config collection cancelled", zap.Error(ctx.Err()))
			return results
		default:
		}

		mcpPath := filepath.Join(dir, ".mcp.json")
		if !pathExists(mcpPath) {
			continue
		}

		config, err := c.parseMCPConfig(mcpPath)
		if err != nil {
			continue
		}

		for serverName, serverDef := range config.MCPServers {
			extra := c.buildMCPExtra("project", mcpPath, serverDef, map[string]string{
				"scope":       "project",
				"project_dir": dir,
			})

			asset := AIAsset{
				Category:    "mcp_server",
				Name:        serverName,
				DisplayName: serverName,
				Source:      "config",
				ConfigPath:  mcpPath,
				Extra:       extra,
			}
			results = append(results, asset)
		}
	}

	return results
}

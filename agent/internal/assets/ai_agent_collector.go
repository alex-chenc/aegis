package assets

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"
)

// AIAgentCollector AI Agent 配置扫描器
// 扫描已知 Agent 配置路径，识别安装的 AI Agent 框架
// 参考 Snyk Agent Scan 的 discoverer 架构
type AIAgentCollector struct {
	logger  *zap.Logger
	homeDir string
}

// AgentProfile Agent 检测配置
type AgentProfile struct {
	Name        string   // 服务标识 (如 "claude-code")
	DisplayName string   // 显示名称
	ConfigPaths []string // 配置目录搜索路径（相对于 home）
}

// NewAIAgentCollector 创建 AI Agent 配置扫描器
func NewAIAgentCollector(logger *zap.Logger) *AIAgentCollector {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/root"
	}
	return &AIAgentCollector{
		logger:  logger,
		homeDir: home,
	}
}

// Collect 扫描已知 Agent 配置路径
func (c *AIAgentCollector) Collect(ctx context.Context) []AIAsset {
	var results []AIAsset

	for _, profile := range agentProfiles() {
		for _, cfgPath := range profile.ConfigPaths {
			fullPath := expandPath(cfgPath, c.homeDir)
			if !pathExists(fullPath) {
				continue
			}

			// 尝试从配置目录读取版本
			version := detectAgentVersion(fullPath)

			asset := AIAsset{
				Category:    "ai_agent",
				Name:        profile.Name,
				DisplayName: profile.DisplayName,
				Version:     version,
				Source:      "config",
				ConfigPath:  fullPath,
				Extra:       map[string]string{"config_dir": fullPath},
			}
			results = append(results, asset)
			c.logger.Info("AI agent detected",
				zap.String("agent", profile.Name),
				zap.String("config_path", fullPath),
				zap.String("version", version))
			break // 一个 Agent 只需匹配一个路径
		}
	}

	return results
}

// agentProfiles 返回已知的 Agent 检测配置
func agentProfiles() []AgentProfile {
	if runtime.GOOS == "darwin" {
		return darwinAgentProfiles()
	}
	return linuxAgentProfiles()
}

// linuxAgentProfiles Linux 平台的 Agent 配置路径
func linuxAgentProfiles() []AgentProfile {
	return []AgentProfile{
		{
			Name:        "claude-code",
			DisplayName: "Claude Code",
			ConfigPaths: []string{"~/.claude"},
		},
		{
			Name:        "claude-desktop",
			DisplayName: "Claude Desktop",
			ConfigPaths: []string{"~/.config/claude"},
		},
		{
			Name:        "cursor",
			DisplayName: "Cursor",
			ConfigPaths: []string{"~/.cursor"},
		},
		{
			Name:        "windsurf",
			DisplayName: "Windsurf",
			ConfigPaths: []string{"~/.windsurf", "~/.codeium/windsurf"},
		},
		{
			Name:        "vscode",
			DisplayName: "VS Code",
			ConfigPaths: []string{"~/.vscode"},
		},
		{
			Name:        "gemini-cli",
			DisplayName: "Gemini CLI",
			ConfigPaths: []string{"~/.gemini"},
		},
		{
			Name:        "amp",
			DisplayName: "Amp",
			ConfigPaths: []string{"~/.amp"},
		},
		{
			Name:        "kiro",
			DisplayName: "Kiro",
			ConfigPaths: []string{"~/.kiro"},
		},
	}
}

// darwinAgentProfiles macOS 平台的 Agent 配置路径
func darwinAgentProfiles() []AgentProfile {
	return []AgentProfile{
		{
			Name:        "claude-code",
			DisplayName: "Claude Code",
			ConfigPaths: []string{"~/.claude"},
		},
		{
			Name:        "claude-desktop",
			DisplayName: "Claude Desktop",
			ConfigPaths: []string{
				"~/Library/Application Support/Claude",
			},
		},
		{
			Name:        "cursor",
			DisplayName: "Cursor",
			ConfigPaths: []string{
				"~/Library/Application Support/Cursor",
			},
		},
		{
			Name:        "windsurf",
			DisplayName: "Windsurf",
			ConfigPaths: []string{
				"~/Library/Application Support/Windsurf",
			},
		},
		{
			Name:        "vscode",
			DisplayName: "VS Code",
			ConfigPaths: []string{
				"~/Library/Application Support/Code",
			},
		},
		{
			Name:        "gemini-cli",
			DisplayName: "Gemini CLI",
			ConfigPaths: []string{"~/.gemini"},
		},
		{
			Name:        "amp",
			DisplayName: "Amp",
			ConfigPaths: []string{"~/.amp"},
		},
		{
			Name:        "kiro",
			DisplayName: "Kiro",
			ConfigPaths: []string{"~/.kiro"},
		},
	}
}

// expandPath 展开 ~ 为 home 目录
func expandPath(path, homeDir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	if path == "~" {
		return homeDir
	}
	return path
}

// pathExists 检查路径是否存在
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// detectAgentVersion 尝试从配置目录读取版本信息
func detectAgentVersion(configDir string) string {
	// 尝试读取常见的版本文件
	versionFiles := []string{"version", "VERSION", ".version"}
	for _, vf := range versionFiles {
		data, err := os.ReadFile(filepath.Join(configDir, vf))
		if err == nil {
			v := strings.TrimSpace(string(data))
			if v != "" {
				return v
			}
		}
	}

	// 尝试读取 package.json
	pkgPath := filepath.Join(configDir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		// 简单提取 version 字段
		content := string(data)
		if idx := strings.Index(content, `"version"`); idx >= 0 {
			rest := content[idx:]
			if start := strings.Index(rest, `"`); start >= 0 {
				rest = rest[start+1:]
				if end := strings.Index(rest, `"`); end > 0 {
					return rest[:end]
				}
			}
		}
	}

	return ""
}

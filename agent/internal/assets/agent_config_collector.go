package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

const (
	maxAgentConfigFileBytes = 256 * 1024
	maxAgentConfigFiles     = 32
)

// AgentConfigSnapshot is the bounded, redacted result returned by the Agent
// ConfigScan tool. It intentionally contains no arbitrary path input.
type AgentConfigSnapshot struct {
	HostID      string             `json:"host_id"`
	CollectedAt time.Time          `json:"collected_at"`
	Agents      []AgentConfigAgent `json:"agents"`
	Errors      []CollectError     `json:"errors,omitempty"`
}

type AgentConfigAgent struct {
	AgentType   string            `json:"agent_type"`
	DisplayName string            `json:"display_name"`
	Files       []AgentConfigFile `json:"files"`
}

type AgentConfigFile struct {
	Path       string    `json:"path"`
	Format     string    `json:"format"`
	Status     string    `json:"status"`
	Size       int64     `json:"size,omitempty"`
	Mode       string    `json:"mode,omitempty"`
	ModifiedAt time.Time `json:"modified_at,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	Content    string    `json:"content,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type agentConfigCandidate struct {
	agentType   string
	displayName string
	path        string
	format      string
}

// AgentConfigCollector reads only well-known configuration files for the
// supported AI agents. The server performs parsing and policy evaluation.
type AgentConfigCollector struct {
	logger   *zap.Logger
	homeDirs []string
}

func NewAgentConfigCollector(logger *zap.Logger) *AgentConfigCollector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentConfigCollector{logger: logger, homeDirs: discoverHomeDirs()}
}

func (c *AgentConfigCollector) Collect(ctx context.Context, hostID string) *AgentConfigSnapshot {
	start := time.Now()
	result := &AgentConfigSnapshot{HostID: hostID, CollectedAt: start}
	result.Agents = make([]AgentConfigAgent, 0)
	result.Errors = make([]CollectError, 0)
	if len(c.homeDirs) == 0 {
		result.Errors = append(result.Errors, CollectError{Stage: "config_discovery", Message: "no user home directory found"})
		return result
	}

	byAgent := make(map[string]*AgentConfigAgent)
	count := 0
	for _, candidate := range c.candidates() {
		if count >= maxAgentConfigFiles {
			result.Errors = append(result.Errors, CollectError{Stage: "config_discovery", Message: "configuration file limit reached"})
			break
		}
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, CollectError{Stage: "config_discovery", Message: ctx.Err().Error()})
			return result
		default:
		}

		file := readAgentConfigFile(candidate)
		if file.Status == "not_found" {
			continue
		}
		count++
		entry := byAgent[candidate.agentType]
		if entry == nil {
			entry = &AgentConfigAgent{AgentType: candidate.agentType, DisplayName: candidate.displayName, Files: make([]AgentConfigFile, 0)}
			byAgent[candidate.agentType] = entry
			result.Agents = append(result.Agents, *entry)
		}
		entry.Files = append(entry.Files, file)
		// The slice above stores a value copy; update the canonical result entry.
		for i := range result.Agents {
			if result.Agents[i].AgentType == candidate.agentType {
				result.Agents[i] = *entry
				break
			}
		}
	}

	c.logger.Info("agent_configuration_collection_completed",
		zap.String("host_id", hostID),
		zap.Int("agent_count", len(result.Agents)),
		zap.Int("file_count", count),
		zap.Duration("duration", time.Since(start)))
	return result
}

func (c *AgentConfigCollector) candidates() []agentConfigCandidate {
	var result []agentConfigCandidate
	for _, home := range c.homeDirs {
		codexHome := resolveCodexHome(home)
		result = append(result,
			agentConfigCandidate{"codex", "Codex", filepath.Join(codexHome, "config.toml"), "toml"},
			agentConfigCandidate{"codex", "Codex", filepath.Join(codexHome, "hooks.json"), "json"},
			agentConfigCandidate{"claude-code", "Claude Code", filepath.Join(home, ".claude", "settings.json"), "json"},
			agentConfigCandidate{"claude-code", "Claude Code", filepath.Join(home, ".claude", "settings.local.json"), "json"},
			agentConfigCandidate{"claude-code", "Claude Code", filepath.Join(home, ".claude.json"), "json"},
			agentConfigCandidate{"openclaw", "OpenClaw", filepath.Join(home, ".openclaw", "openclaw.json"), "json"},
			agentConfigCandidate{"openclaw", "OpenClaw", filepath.Join(home, ".openclaw", "config.json"), "json"},
			agentConfigCandidate{"opencode", "OpenCode", filepath.Join(home, ".config", "opencode", "opencode.json"), "json"},
			agentConfigCandidate{"opencode", "OpenCode", filepath.Join(home, ".config", "opencode", "opencode.jsonc"), "jsonc"},
			agentConfigCandidate{"hermes", "Hermes", filepath.Join(home, ".hermes", "config.yaml"), "yaml"},
			agentConfigCandidate{"hermes", "Hermes", filepath.Join(home, ".hermes", "config.yml"), "yaml"},
			agentConfigCandidate{"hermes", "Hermes", filepath.Join(home, ".hermes", "config.json"), "json"},
		)
	}
	return result
}

func readAgentConfigFile(candidate agentConfigCandidate) AgentConfigFile {
	file := AgentConfigFile{Path: candidate.path, Format: candidate.format, Status: "not_found"}
	info, err := os.Lstat(candidate.path)
	if err != nil {
		if os.IsNotExist(err) {
			return file
		}
		file.Status, file.Error = "unreadable", err.Error()
		return file
	}
	if info.Mode()&os.ModeSymlink != 0 {
		file.Status, file.Error = "rejected", "symbolic links are not allowed"
		return file
	}
	if !info.Mode().IsRegular() {
		file.Status, file.Error = "rejected", "configuration path is not a regular file"
		return file
	}
	file.Size, file.Mode, file.ModifiedAt = info.Size(), info.Mode().String(), info.ModTime()
	if info.Size() > maxAgentConfigFileBytes {
		file.Status, file.Error = "too_large", fmt.Sprintf("file exceeds %d bytes", maxAgentConfigFileBytes)
		return file
	}
	data, err := os.ReadFile(candidate.path)
	if err != nil {
		file.Status, file.Error = "unreadable", err.Error()
		return file
	}
	file.Status = "ok"
	digest := sha256.Sum256(data)
	file.SHA256 = hex.EncodeToString(digest[:])
	file.Content = RedactConfigSummary(string(data))
	return file
}

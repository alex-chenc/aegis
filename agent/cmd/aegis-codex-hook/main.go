package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aegis-agent/internal/agentguard"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const hookBridgeVersion = "1.0.0"

// The OpenClaw plugin is embedded so provisioning works from the released
// helper binary and does not depend on the source repository being present.
//
//go:embed assets/openclaw/*
var openClawPluginAssets embed.FS

type codexHookInput struct {
	SessionID     string          `json:"session_id"`
	HookEventName string          `json:"hook_event_name"`
	EventType     string          `json:"event_type,omitempty"`
	AgentType     string          `json:"agent_type,omitempty"`
	Source        string          `json:"source,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	ToolName      string          `json:"tool_name,omitempty"`
	ToolUseID     string          `json:"tool_use_id,omitempty"`
	ToolCallID    string          `json:"tool_call_id,omitempty"`
	TurnID        string          `json:"turn_id,omitempty"`
	PID           uint32          `json:"pid,omitempty"`
	StartTicks    uint64          `json:"start_ticks,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	ToolResponse  json.RawMessage `json:"tool_response,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	Extra         map[string]any  `json:"extra,omitempty"`
	Error         any             `json:"error,omitempty"`
}

type rootState struct {
	PID        uint32 `json:"pid"`
	StartTicks uint64 `json:"start_ticks"`
	ToolTokens string `json:"tool_tokens,omitempty"`
}

type options struct {
	socketPath string
	privateKey string
	stateDir   string
	sourceID   string
	sourceVer  string
	agentType  string
	scope      string
	parentPID  int
	now        func() time.Time
	stdin      io.Reader
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "provision" {
		if err := provision(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "aegis_codex_hook_provision_failed:", stableError(err))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "remove" {
		if err := remove(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "aegis_codex_hook_remove_failed:", stableError(err))
			os.Exit(1)
		}
		return
	}
	var opts options
	flag.StringVar(&opts.socketPath, "socket", "/run/aegis-agent/agent-guard-hook.sock", "Aegis Agent hook socket")
	flag.StringVar(&opts.privateKey, "private-key", "/etc/aegis-agent/codex-hook.key", "Ed25519 private key")
	flag.StringVar(&opts.stateDir, "state-dir", defaultStateDir(), "per-user lifecycle state directory")
	flag.StringVar(&opts.sourceID, "source-id", "codex-native-hook", "trusted source manifest id")
	flag.StringVar(&opts.sourceVer, "source-version", hookBridgeVersion, "trusted source version")
	flag.StringVar(&opts.agentType, "agent-type", "codex", "agent product type")
	flag.StringVar(&opts.scope, "scope", "behavior", "Hook policy scope")
	flag.Parse()
	opts.agentType = normalizeAgentType(opts.agentType)
	opts.scope = normalizeHookScope(opts.scope)
	if opts.sourceID == "codex-native-hook" && opts.agentType != "codex" {
		opts.sourceID = defaultSourceID(opts.agentType)
	}
	opts.sourceID = scopedSourceID(opts.sourceID, opts.scope)
	if opts.stateDir == defaultStateDir() && opts.agentType != "codex" {
		opts.stateDir = defaultStateDirForAgent(opts.agentType)
	}
	opts.parentPID = os.Getppid()
	opts.now = time.Now
	opts.stdin = os.Stdin
	if err := run(opts); err != nil {
		// Never print Hook input, session ids, paths, or signing material.
		fmt.Fprintln(os.Stderr, "aegis_codex_hook_failed:", stableError(err))
		os.Exit(1)
	}
}

func provision(arguments []string) error {
	flags := flag.NewFlagSet("provision", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaultHome, _ := os.UserHomeDir()
	var privateKeyPath, manifestPath, hooksPath, managedRequirements, socketPath, hookBinary, stateDir, sourceID, sourceVersion, agentType string
	var scope string
	flags.StringVar(&privateKeyPath, "private-key", "/etc/aegis-agent/codex-hook.key", "Ed25519 private key")
	flags.StringVar(&manifestPath, "manifest", "/etc/aegis-agent/codex-hook-sources.json", "trusted source manifest")
	flags.StringVar(&hooksPath, "hooks", "", "product hook configuration file")
	flags.StringVar(&managedRequirements, "managed-requirements", "", "optional managed Codex requirements.toml")
	flags.StringVar(&socketPath, "socket", "/run/aegis-agent/agent-guard-hook.sock", "Aegis Agent hook socket")
	flags.StringVar(&hookBinary, "hook-binary", "/opt/aegis-agent/aegis-codex-hook", "installed hook helper")
	flags.StringVar(&stateDir, "state-dir", defaultStateDir(), "per-user lifecycle state directory")
	flags.StringVar(&sourceID, "source-id", "codex-native-hook", "trusted source id")
	flags.StringVar(&sourceVersion, "source-version", hookBridgeVersion, "trusted source version")
	flags.StringVar(&agentType, "agent-type", "codex", "agent product type")
	flags.StringVar(&scope, "scope", "behavior", "Hook policy scope")
	if flags.Parse(arguments) != nil {
		return errors.New("arguments_invalid")
	}
	agentType = normalizeAgentType(agentType)
	scope = normalizeHookScope(scope)
	if agentType == "" || scope == "" {
		return errors.New("agent_type_invalid")
	}
	if hooksPath == "" {
		hooksPath = defaultHooksPath(defaultHome, agentType)
	}
	if sourceID == "codex-native-hook" && agentType != "codex" {
		sourceID = defaultSourceID(agentType)
	}
	sourceID = scopedSourceID(sourceID, scope)
	for _, path := range []string{privateKeyPath, manifestPath, socketPath, hookBinary, stateDir} {
		if !filepath.IsAbs(path) {
			return errors.New("path_invalid")
		}
	}
	if hooksPath == "" && managedRequirements == "" || hooksPath != "" && !filepath.IsAbs(hooksPath) ||
		managedRequirements != "" && !filepath.IsAbs(managedRequirements) {
		return errors.New("path_invalid")
	}
	artifactDigest, err := fileSHA256(hookBinary)
	if err != nil {
		return errors.New("hook_binary_untrusted")
	}
	privateKey, err := loadOrCreatePrivateKey(privateKeyPath)
	if err != nil {
		return err
	}
	manifest, err := loadOrCreateManifest(manifestPath)
	if err != nil {
		return err
	}
	manifest.Sources = upsertSource(manifest.Sources, agentguard.TrustedToolSource{
		SourceID: sourceID, SourceType: agentguard.ToolSourceAdapterHook,
		Product: agentType, Version: sourceVersion, Verifier: "ed25519",
		PublicKey:    base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
		ArtifactPath: hookBinary, ArtifactDigest: artifactDigest,
		AllowedUIDs: []uint32{uint32(os.Geteuid())},
	})
	manifest.Digest, err = agentguard.TrustedToolManifestDigest(manifest)
	if err != nil {
		return errors.New("manifest_digest_failed")
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil || writeTrustedFile(manifestPath, append(manifestData, '\n')) != nil {
		return errors.New("manifest_write_failed")
	}
	command := managedHookCommand(hookBinary, socketPath, privateKeyPath, stateDir, sourceID, sourceVersion, scope)
	if managedRequirements != "" {
		if err := writeManagedRequirements(managedRequirements, filepath.Dir(hookBinary), command); err != nil {
			return err
		}
	} else if err := mergeProductHooks(hooksPath, command, agentType); err != nil {
		return err
	}
	fmt.Printf("AgentGuardSessionHookEnabled = true\nAgentGuardToolSourceManifest = %q\nAgentGuardToolHookSocket = %q\n", manifestPath, socketPath)
	return nil
}

func remove(arguments []string) error {
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaultHome, _ := os.UserHomeDir()
	var manifestPath, hooksPath, managedRequirements, hookBinary, sourceID, agentType string
	var scope string
	flags.StringVar(&manifestPath, "manifest", "/etc/aegis-agent/codex-hook-sources.json", "trusted source manifest")
	flags.StringVar(&hooksPath, "hooks", "", "product hook configuration file")
	flags.StringVar(&managedRequirements, "managed-requirements", "", "optional managed Codex requirements.toml")
	flags.StringVar(&hookBinary, "hook-binary", "/opt/aegis-agent/aegis-codex-hook", "installed hook helper")
	flags.StringVar(&sourceID, "source-id", "", "trusted source id")
	flags.StringVar(&agentType, "agent-type", "codex", "agent product type")
	flags.StringVar(&scope, "scope", "behavior", "Hook policy scope")
	if flags.Parse(arguments) != nil {
		return errors.New("arguments_invalid")
	}
	agentType = normalizeAgentType(agentType)
	scope = normalizeHookScope(scope)
	if agentType == "" || scope == "" {
		return errors.New("agent_type_invalid")
	}
	if hooksPath == "" {
		hooksPath = defaultHooksPath(defaultHome, agentType)
	}
	if sourceID == "" {
		sourceID = defaultSourceID(agentType)
	}
	sourceID = scopedSourceID(sourceID, scope)
	for _, path := range []string{manifestPath, hookBinary} {
		if !filepath.IsAbs(path) {
			return errors.New("path_invalid")
		}
	}
	if hooksPath == "" && managedRequirements == "" || hooksPath != "" && !filepath.IsAbs(hooksPath) ||
		managedRequirements != "" && !filepath.IsAbs(managedRequirements) {
		return errors.New("path_invalid")
	}

	if managedRequirements != "" {
		if err := removeManagedRequirements(managedRequirements, hookBinary, scope); err != nil {
			return err
		}
	} else if err := removeProductHooks(hooksPath, hookBinary, agentType, scope); err != nil {
		return err
	}
	if agentType == "openclaw" {
		pluginDir := filepath.Join(filepath.Dir(hooksPath), "extensions", "aegis-agent-guard")
		if err := removeOpenClawPlugin(pluginDir); err != nil {
			return err
		}
	}
	return removeManifestSource(manifestPath, sourceID)
}

func managedHookCommand(hookBinary, socketPath, privateKeyPath, stateDir, sourceID, sourceVersion, scope string) string {
	return strings.Join([]string{
		shellQuote(hookBinary), "--socket", shellQuote(socketPath),
		"--private-key", shellQuote(privateKeyPath), "--state-dir", shellQuote(stateDir),
		"--source-id", shellQuote(sourceID), "--source-version", shellQuote(sourceVersion), "--scope", shellQuote(scope),
	}, " ")
}

func normalizeHookScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "behavior", "escape":
		return strings.ToLower(strings.TrimSpace(scope))
	default:
		return ""
	}
}

func scopedSourceID(sourceID, scope string) string {
	if scope == "" || scope == "behavior" || strings.HasSuffix(sourceID, "-"+scope) {
		return sourceID
	}
	return sourceID + "-" + scope
}

func removeProductHooks(path, hookBinary, agentType, scope string) error {
	switch agentType {
	case "codex", "claude-code", "zcode":
		return removeJSONHooks(path, hookBinary, scope)
	case "hermes":
		return removeHermesHooks(path, hookBinary, scope)
	case "openclaw":
		return removeOpenClawConfig(path)
	default:
		return errors.New("agent_type_unsupported")
	}
}

func removeJSONHooks(path, hookBinary, scope string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("hooks_file_untrusted")
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
		return errors.New("hooks_file_untrusted")
	}
	document := map[string]any{}
	if json.Unmarshal(data, &document) != nil {
		return errors.New("hooks_file_untrusted")
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	changed := false
	for event, value := range hooks {
		groups, ok := value.([]any)
		if !ok {
			continue
		}
		filtered, removed := removeManagedJSONGroups(groups, hookBinary, scope)
		if removed {
			hooks[event] = filtered
			changed = true
		}
	}
	if !changed {
		return nil
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil || writeTrustedFile(path, append(encoded, '\n')) != nil {
		return errors.New("hooks_file_write_failed")
	}
	return nil
}

func removeManagedJSONGroups(groups []any, hookBinary, scope string) ([]any, bool) {
	filteredGroups := make([]any, 0, len(groups))
	removed := false
	for _, value := range groups {
		group, ok := value.(map[string]any)
		if !ok {
			filteredGroups = append(filteredGroups, value)
			continue
		}
		handlers, ok := group["hooks"].([]any)
		if !ok {
			filteredGroups = append(filteredGroups, value)
			continue
		}
		filteredHandlers := make([]any, 0, len(handlers))
		groupChanged := false
		for _, handlerValue := range handlers {
			handler, ok := handlerValue.(map[string]any)
			command, _ := handler["command"].(string)
			if ok && isManagedHookCommand(command, hookBinary, scope) {
				removed = true
				groupChanged = true
				continue
			}
			filteredHandlers = append(filteredHandlers, handlerValue)
		}
		if groupChanged && len(filteredHandlers) == 0 {
			continue
		}
		group["hooks"] = filteredHandlers
		filteredGroups = append(filteredGroups, value)
	}
	return filteredGroups, removed
}

func removeHermesHooks(path, hookBinary, scope string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("hooks_file_untrusted")
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
		return errors.New("hooks_file_untrusted")
	}
	document := map[string]any{}
	if yaml.Unmarshal(data, &document) != nil {
		return errors.New("hooks_file_untrusted")
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	changed := false
	for event, value := range hooks {
		entries, ok := value.([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(entries))
		eventChanged := false
		for _, entryValue := range entries {
			entry, ok := entryValue.(map[string]any)
			command, _ := entry["command"].(string)
			if ok && isManagedHookCommand(command, hookBinary, scope) {
				changed = true
				eventChanged = true
				continue
			}
			filtered = append(filtered, entryValue)
		}
		if eventChanged {
			hooks[event] = filtered
		}
	}
	if !changed {
		return nil
	}
	encoded, err := yaml.Marshal(document)
	if err != nil || writeTrustedFile(path, encoded) != nil {
		return errors.New("hooks_file_write_failed")
	}
	return nil
}

func removeOpenClawConfig(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("hooks_file_untrusted")
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
		return errors.New("hooks_file_untrusted")
	}
	document := map[string]any{}
	if json.Unmarshal(data, &document) != nil {
		return errors.New("hooks_file_untrusted")
	}
	plugins, ok := document["plugins"].(map[string]any)
	if !ok {
		return nil
	}
	load, ok := plugins["load"].(map[string]any)
	if !ok {
		return nil
	}
	paths, ok := load["paths"].([]any)
	if !ok {
		return nil
	}
	pluginDir := filepath.Join(filepath.Dir(path), "extensions", "aegis-agent-guard")
	filtered := make([]any, 0, len(paths))
	changed := false
	for _, value := range paths {
		if strings.TrimSpace(fmt.Sprint(value)) == pluginDir {
			changed = true
			continue
		}
		filtered = append(filtered, value)
	}
	if changed {
		load["paths"] = filtered
		encoded, encodeErr := json.MarshalIndent(document, "", "  ")
		if encodeErr != nil || writeTrustedFile(path, append(encoded, '\n')) != nil {
			return errors.New("hooks_file_write_failed")
		}
	}
	return nil
}

func removeOpenClawPlugin(pluginDir string) error {
	if !filepath.IsAbs(pluginDir) || strings.Contains(filepath.Clean(pluginDir), "..") {
		return errors.New("plugin_path_invalid")
	}
	for _, name := range []string{"package.json", "openclaw.plugin.json", "index.js"} {
		path := filepath.Join(pluginDir, name)
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.New("plugin_untrusted")
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
			return errors.New("plugin_untrusted")
		}
		asset, assetErr := openClawPluginAssets.ReadFile("assets/openclaw/" + name)
		if assetErr == nil && string(data) == string(asset) {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return errors.New("plugin_remove_failed")
			}
		}
	}
	_ = os.Remove(pluginDir)
	_ = os.Remove(filepath.Dir(pluginDir))
	return nil
}

func removeManagedRequirements(path, hookBinary, scope string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !strings.Contains(string(data), shellQuote(hookBinary)) || (scope != "behavior" && !strings.Contains(string(data), "--scope "+shellQuote(scope))) {
		return errors.New("managed_requirements_untrusted")
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
		return errors.New("managed_requirements_untrusted")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("managed_requirements_remove_failed")
	}
	return nil
}

func isManagedHookCommand(command, hookBinary, scope string) bool {
	command = strings.TrimSpace(command)
	prefix := shellQuote(hookBinary) + " "
	scopeMatch := strings.Contains(command, " --scope "+shellQuote(scope))
	// Commands created before scoped Hooks are treated as behavior scope so an
	// upgrade can clean them up without touching the new escape source.
	if scope == "behavior" && !strings.Contains(command, " --scope ") {
		scopeMatch = true
	}
	return strings.HasPrefix(command, prefix) && strings.Contains(command, " --socket ") &&
		strings.Contains(command, " --private-key ") && strings.Contains(command, " --state-dir ") &&
		strings.Contains(command, " --source-id ") && scopeMatch
}

func removeManifestSource(path, sourceID string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	manifest, err := loadOrCreateManifest(path)
	if err != nil {
		return err
	}
	filtered := make([]agentguard.TrustedToolSource, 0, len(manifest.Sources))
	removed := false
	for _, source := range manifest.Sources {
		if source.SourceID == sourceID {
			removed = true
			continue
		}
		filtered = append(filtered, source)
	}
	if !removed {
		return nil
	}
	manifest.Sources = filtered
	manifest.Digest, err = agentguard.TrustedToolManifestDigest(manifest)
	if err != nil {
		return errors.New("manifest_digest_failed")
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil || writeTrustedFile(path, append(data, '\n')) != nil {
		return errors.New("manifest_write_failed")
	}
	return nil
}

func writeManagedRequirements(path, managedDir, command string) error {
	if _, err := os.Lstat(path); err == nil {
		return errors.New("managed_requirements_exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("managed_requirements_untrusted")
	}
	content := strings.Join([]string{
		"[features]",
		"hooks = true",
		"",
		"[hooks]",
		"managed_dir = " + strconv.Quote(managedDir),
		"",
		"[[hooks.SessionStart]]",
		`matcher = "startup|resume|clear|compact"`,
		"[[hooks.SessionStart.hooks]]",
		`type = "command"`,
		"command = " + strconv.Quote(command),
		"timeout = 3",
		"",
		"[[hooks.PreToolUse]]",
		`matcher = "*"`,
		"[[hooks.PreToolUse.hooks]]",
		`type = "command"`,
		"command = " + strconv.Quote(command),
		"timeout = 3",
		"",
		"[[hooks.PostToolUse]]",
		`matcher = "*"`,
		"[[hooks.PostToolUse.hooks]]",
		`type = "command"`,
		"command = " + strconv.Quote(command),
		"timeout = 3",
		"",
		"[[hooks.SessionEnd]]",
		"[[hooks.SessionEnd.hooks]]",
		`type = "command"`,
		"command = " + strconv.Quote(command),
		"timeout = 3",
		"",
	}, "\n")
	if err := writeTrustedFile(path, []byte(content)); err != nil {
		return errors.New("managed_requirements_write_failed")
	}
	return nil
}

func loadOrCreatePrivateKey(path string) (ed25519.PrivateKey, error) {
	if _, err := os.Lstat(path); err == nil {
		return readPrivateKey(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("private_key_untrusted")
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, errors.New("private_key_generate_failed")
	}
	data := []byte(base64.StdEncoding.EncodeToString(privateKey) + "\n")
	if err := writeTrustedFile(path, data); err != nil {
		return nil, errors.New("private_key_write_failed")
	}
	return privateKey, nil
}

func mergeCodexHooks(path, command string) error {
	document := map[string]any{"description": "Aegis trusted Codex session lifecycle hooks."}
	if data, err := os.ReadFile(path); err == nil {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) ||
			json.Unmarshal(data, &document) != nil {
			return errors.New("hooks_file_untrusted")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("hooks_file_untrusted")
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		document["hooks"] = hooks
	}
	appendHook := func(event string, matcher string, timeout int) {
		groups, _ := hooks[event].([]any)
		encoded, _ := json.Marshal(groups)
		if strings.Contains(string(encoded), command) {
			return
		}
		handler := map[string]any{"type": "command", "command": command}
		if timeout > 0 {
			handler["timeout"] = timeout
		}
		group := map[string]any{"hooks": []any{handler}}
		if matcher != "" {
			group["matcher"] = matcher
		}
		hooks[event] = append(groups, group)
	}
	appendHook("SessionStart", "startup|resume|clear|compact", 3)
	appendHook("PreToolUse", "*", 3)
	appendHook("PostToolUse", "*", 3)
	appendHook("SessionEnd", "", 3)
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil || writeTrustedFile(path, append(data, '\n')) != nil {
		return errors.New("hooks_file_write_failed")
	}
	return nil
}

func mergeProductHooks(path, command, agentType string) error {
	switch agentType {
	case "codex":
		return mergeCodexHooks(path, command)
	case "claude-code":
		return mergeJSONHooks(path, command, "Aegis trusted Claude Code session lifecycle hooks.",
			[]string{"SessionStart", "PreToolUse", "PostToolUse", "PostToolUseFailure", "SessionEnd"})
	case "zcode":
		// Zcode follows the Claude-compatible hook payload. Its Stop event is
		// intentionally not registered: Stop is a model stop point, not a
		// session lifecycle end signal.
		return mergeJSONHooks(path, command, "Aegis trusted Zcode session and tool hooks.",
			[]string{"SessionStart", "PreToolUse", "PostToolUse", "PostToolUseFailure"})
	case "hermes":
		return mergeHermesHooks(path, command)
	case "openclaw":
		pluginDir := filepath.Join(filepath.Dir(path), "extensions", "aegis-agent-guard")
		if err := writeOpenClawPlugin(pluginDir); err != nil {
			return err
		}
		return mergeOpenClawConfig(path, pluginDir)
	default:
		return errors.New("agent_type_unsupported")
	}
}

func mergeJSONHooks(path, command, description string, events []string) error {
	document := map[string]any{"description": description}
	if data, err := os.ReadFile(path); err == nil {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) ||
			json.Unmarshal(data, &document) != nil {
			return errors.New("hooks_file_untrusted")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("hooks_file_untrusted")
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		document["hooks"] = hooks
	}
	for _, event := range events {
		groups, _ := hooks[event].([]any)
		encoded, _ := json.Marshal(groups)
		if strings.Contains(string(encoded), command) {
			continue
		}
		group := map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": command, "timeout": 3,
		}}}
		if event == "PreToolUse" || event == "PostToolUse" || event == "PostToolUseFailure" {
			group["matcher"] = "*"
		}
		hooks[event] = append(groups, group)
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil || writeTrustedFile(path, append(data, '\n')) != nil {
		return errors.New("hooks_file_write_failed")
	}
	return nil
}

func mergeHermesHooks(path, command string) error {
	document := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
			return errors.New("hooks_file_untrusted")
		}
		if err := yaml.Unmarshal(data, &document); err != nil {
			return errors.New("hooks_file_untrusted")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("hooks_file_untrusted")
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		document["hooks"] = hooks
	}
	for _, event := range []string{"on_session_start", "pre_tool_call", "post_tool_call", "on_session_end"} {
		entries, _ := hooks[event].([]any)
		encoded, _ := json.Marshal(entries)
		if strings.Contains(string(encoded), command) {
			continue
		}
		entry := map[string]any{"command": command, "timeout": 3}
		if event == "pre_tool_call" || event == "post_tool_call" {
			entry["matcher"] = ".*"
		}
		hooks[event] = append(entries, entry)
	}
	data, err := yaml.Marshal(document)
	if err != nil || writeTrustedFile(path, data) != nil {
		return errors.New("hooks_file_write_failed")
	}
	return nil
}

func writeOpenClawPlugin(pluginDir string) error {
	if !filepath.IsAbs(pluginDir) || strings.Contains(filepath.Clean(pluginDir), "..") {
		return errors.New("plugin_path_invalid")
	}
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		return errors.New("plugin_dir_create_failed")
	}
	for _, name := range []string{"package.json", "openclaw.plugin.json", "index.js"} {
		data, err := openClawPluginAssets.ReadFile("assets/openclaw/" + name)
		if err != nil || writeTrustedFile(filepath.Join(pluginDir, name), data) != nil {
			return errors.New("plugin_write_failed")
		}
	}
	return nil
}

func mergeOpenClawConfig(path, pluginDir string) error {
	document := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) ||
			json.Unmarshal(data, &document) != nil {
			return errors.New("hooks_file_untrusted")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("hooks_file_untrusted")
	}
	plugins, ok := document["plugins"].(map[string]any)
	if !ok {
		plugins = make(map[string]any)
		document["plugins"] = plugins
	}
	load, ok := plugins["load"].(map[string]any)
	if !ok {
		load = make(map[string]any)
		plugins["load"] = load
	}
	paths, _ := load["paths"].([]any)
	for _, value := range paths {
		if value == pluginDir {
			paths = nil
			break
		}
	}
	if paths == nil {
		// Preserve the existing list when it already contains the plugin.
		if existing, ok := load["paths"].([]any); ok {
			paths = existing
		}
	}
	found := false
	for _, value := range paths {
		if strings.TrimSpace(fmt.Sprint(value)) == pluginDir {
			found = true
			break
		}
	}
	if !found {
		paths = append(paths, pluginDir)
	}
	load["paths"] = paths
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil || writeTrustedFile(path, append(data, '\n')) != nil {
		return errors.New("hooks_file_write_failed")
	}
	return nil
}

func loadOrCreateManifest(path string) (agentguard.TrustedToolSourceManifest, error) {
	manifest := agentguard.TrustedToolSourceManifest{Schema: agentguard.ToolManifestSchemaV1}
	data, err := os.ReadFile(path)
	if err == nil {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) ||
			json.Unmarshal(data, &manifest) != nil || manifest.Schema != agentguard.ToolManifestSchemaV1 {
			return agentguard.TrustedToolSourceManifest{}, errors.New("manifest_untrusted")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return agentguard.TrustedToolSourceManifest{}, errors.New("manifest_untrusted")
	}
	return manifest, nil
}

func upsertSource(sources []agentguard.TrustedToolSource, source agentguard.TrustedToolSource) []agentguard.TrustedToolSource {
	for index := range sources {
		if sources[index].SourceID == source.SourceID {
			sources[index] = source
			return sources
		}
	}
	return append(sources, source)
}

func normalizeAgentType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "claude", "claude_code", "claude-code":
		return "claude-code"
	case "open-claw", "open_claw", "openclaw":
		return "openclaw"
	case "z-code", "z_code", "zcode":
		return "zcode"
	case "codex", "openai-codex", "openai_codex":
		return "codex"
	case "hermes":
		return "hermes"
	default:
		return value
	}
}

func defaultHooksPath(home, agentType string) string {
	switch agentType {
	case "claude-code":
		return filepath.Join(home, ".claude", "settings.json")
	case "zcode":
		return filepath.Join(home, ".zcode", "cli", "config.json")
	case "hermes":
		return filepath.Join(home, ".hermes", "config.yaml")
	case "openclaw":
		return filepath.Join(home, ".openclaw", "openclaw.json")
	default:
		return filepath.Join(home, ".codex", "hooks.json")
	}
}

func defaultSourceID(agentType string) string {
	switch agentType {
	case "codex":
		return "codex-native-hook"
	case "openclaw":
		return "openclaw-plugin-v1"
	default:
		return agentType + "-hook-v1"
	}
}

func defaultStateDirForAgent(agentType string) string {
	return filepath.Join(os.TempDir(), "aegis-"+strings.ReplaceAll(agentType, "-", "_")+"-hook-"+strconv.Itoa(os.Geteuid()))
}

func writeTrustedFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".aegis-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if temporary.Chmod(0o600) != nil {
		_ = temporary.Close()
		return errors.New("chmod_failed")
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func fileSHA256(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !ownedByEUID(info) {
		return "", errors.New("artifact_untrusted")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func run(opts options) error {
	if !filepath.IsAbs(opts.socketPath) || !filepath.IsAbs(opts.privateKey) ||
		!filepath.IsAbs(opts.stateDir) || opts.parentPID <= 0 || opts.now == nil || opts.stdin == nil ||
		normalizeAgentType(opts.agentType) == "" || normalizeHookScope(opts.scope) == "" {
		return errors.New("configuration_invalid")
	}
	decoder := json.NewDecoder(io.LimitReader(opts.stdin, 64*1024))
	var input codexHookInput
	if err := decoder.Decode(&input); err != nil {
		return errors.New("hook_input_invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("hook_input_invalid")
	}
	if input.SessionID == "" || len(input.SessionID) > 255 {
		return errors.New("session_id_invalid")
	}
	input.HookEventName = normalizeHookEvent(input.HookEventName, input.EventType)
	if input.ToolUseID == "" {
		input.ToolUseID = input.ToolCallID
	}
	if input.ToolUseID == "" {
		input.ToolUseID = extraString(input.Extra, "tool_call_id", "tool_use_id")
	}
	if input.TurnID == "" {
		input.TurnID = extraString(input.Extra, "turn_id", "task_id")
	}
	if len(input.ToolResponse) == 0 {
		input.ToolResponse = input.Result
	}
	if len(input.ToolResponse) == 0 {
		if result := input.Extra["result"]; result != nil {
			input.ToolResponse = jsonValue(result)
		}
	}
	if input.Error == nil {
		input.Error = input.Extra["error"]
	}
	statePath := filepath.Join(opts.stateDir, sessionStateName(input.SessionID))
	var operation, lifecycleReason string
	switch input.HookEventName {
	case "SessionStart":
		if input.Source == "compact" {
			operation = agentguard.SessionEventActivated
		} else {
			operation = agentguard.SessionEventStarted
		}
		lifecycleReason = input.Source
	case "PreToolUse":
		return sendToolEvent(opts, input, statePath, agentguard.ToolEventStarted)
	case "PostToolUse":
		operation := agentguard.ToolEventCompleted
		if toolResponseFailed(input.ToolResponse, input.Error) {
			operation = agentguard.ToolEventFailed
		}
		return sendToolEvent(opts, input, statePath, operation)
	case "PostToolUseFailure":
		return sendToolEvent(opts, input, statePath, agentguard.ToolEventFailed)
	case "SessionEnd":
		operation = agentguard.SessionEventEnded
		lifecycleReason = input.Reason
	case "UserPromptSubmit":
		operation = agentguard.SessionEventActivated
		lifecycleReason = "user_prompt"
	default:
		return errors.New("hook_event_unsupported")
	}
	if lifecycleReason == "" {
		lifecycleReason = "other"
	}
	root, err := sessionRootForInput(operation, statePath, opts.parentPID, input)
	if err != nil {
		return err
	}
	privateKey, err := readPrivateKey(opts.privateKey)
	if err != nil {
		return err
	}
	now := opts.now().UTC()
	event := agentguard.TrustedSessionEvent{
		EventID: uuid.NewString(), SourceID: opts.sourceID, SourceVersion: opts.sourceVer,
		Operation: operation, ExternalSessionID: input.SessionID,
		PID: root.PID, StartTicks: root.StartTicks, LifecycleReason: lifecycleReason,
		OccurredAt: now, IssuedAt: now,
	}
	if err := agentguard.SignTrustedSessionEvent(&event, privateKey); err != nil {
		return errors.New("event_sign_failed")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return errors.New("event_encode_failed")
	}
	if err := sendHookPayload(opts.socketPath, payload); err != nil {
		return err
	}
	if operation == agentguard.SessionEventEnded {
		_ = os.Remove(statePath)
	}
	return nil
}

func sendToolEvent(opts options, input codexHookInput, statePath, operation string) error {
	if strings.TrimSpace(input.ToolName) == "" || strings.TrimSpace(input.ToolUseID) == "" {
		return errors.New("tool_identity_invalid")
	}
	root, err := sessionRootForInput(agentguard.SessionEventActivated, statePath, opts.parentPID, input)
	if err != nil {
		return err
	}
	state, stateErr := readRootState(statePath)
	if stateErr != nil {
		state = root
	}
	tokens := decodeToolTokens(state.ToolTokens)
	token := tokens[input.ToolUseID]
	if token == "" {
		token = uuid.NewString() + uuid.NewString()
	}
	if operation == agentguard.ToolEventStarted {
		tokens[input.ToolUseID] = token
	}
	privateKey, err := readPrivateKey(opts.privateKey)
	if err != nil {
		return err
	}
	now := opts.now().UTC()
	event := agentguard.TrustedToolEvent{
		EventID: uuid.NewString(), SourceID: opts.sourceID, SourceVersion: opts.sourceVer,
		Operation: operation, ToolName: strings.TrimSpace(input.ToolName), ToolCallID: strings.TrimSpace(input.ToolUseID),
		ExternalSessionID: input.SessionID, CorrelationToken: token,
		PID: root.PID, StartTicks: root.StartTicks, TurnID: strings.TrimSpace(input.TurnID),
		ToolInput:    append(json.RawMessage(nil), input.ToolInput...),
		ToolResponse: append(json.RawMessage(nil), input.ToolResponse...),
		OccurredAt:   now, IssuedAt: now,
	}
	if err := agentguard.SignTrustedToolEvent(&event, privateKey); err != nil {
		return errors.New("event_sign_failed")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return errors.New("event_encode_failed")
	}
	if err := sendHookPayload(opts.socketPath, payload); err != nil {
		return err
	}
	if operation == agentguard.ToolEventStarted {
		state.ToolTokens = encodeToolTokens(tokens)
		if err := writeRootState(statePath, state); err != nil {
			return err
		}
	} else {
		delete(tokens, input.ToolUseID)
		state.ToolTokens = encodeToolTokens(tokens)
		if err := writeRootState(statePath, state); err != nil {
			return err
		}
	}
	return nil
}

func sendHookPayload(socketPath string, payload []byte) error {
	connection, err := net.DialTimeout("unix", socketPath, 750*time.Millisecond)
	if err != nil {
		return errors.New("socket_connect_failed")
	}
	defer connection.Close()
	_ = connection.SetWriteDeadline(time.Now().Add(750 * time.Millisecond))
	if _, err := connection.Write(append(payload, '\n')); err != nil {
		return errors.New("socket_write_failed")
	}
	return nil
}

func toolResponseFailed(response json.RawMessage, explicit any) bool {
	if boolValue(explicit) {
		return true
	}
	value := trustedJSONValue(response)
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"is_error", "isError", "failed", "error"} {
			if boolValue(object[key]) {
				return true
			}
		}
	}
	return false
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func trustedJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func sessionRoot(operation, statePath string, parentPID int) (rootState, error) {
	if operation == agentguard.SessionEventEnded || operation == agentguard.SessionEventActivated {
		if state, err := readRootState(statePath); err == nil {
			return state, nil
		}
	}
	startTicks, err := readStartTicks(parentPID)
	if err != nil {
		return rootState{}, errors.New("parent_process_unavailable")
	}
	state := rootState{PID: uint32(parentPID), StartTicks: startTicks}
	if operation != agentguard.SessionEventEnded {
		if err := writeRootState(statePath, state); err != nil {
			return rootState{}, err
		}
	}
	return state, nil
}

func sessionRootForInput(operation, statePath string, parentPID int, input codexHookInput) (rootState, error) {
	if operation == agentguard.SessionEventEnded || operation == agentguard.SessionEventActivated {
		if state, err := readRootState(statePath); err == nil {
			return state, nil
		}
	}
	if input.PID == 0 {
		return sessionRoot(operation, statePath, parentPID)
	}
	pid := int(input.PID)
	startTicks := input.StartTicks
	if startTicks == 0 {
		var err error
		startTicks, err = readStartTicks(pid)
		if err != nil {
			return rootState{}, errors.New("hook_process_unavailable")
		}
	}
	state := rootState{PID: input.PID, StartTicks: startTicks}
	if operation != agentguard.SessionEventEnded {
		if err := writeRootState(statePath, state); err != nil {
			return rootState{}, err
		}
	}
	return state, nil
}

func normalizeHookEvent(eventName, eventType string) string {
	event := strings.TrimSpace(eventName)
	if event == "" {
		event = strings.TrimSpace(eventType)
	}
	switch strings.ToLower(event) {
	case "sessionstart", "session_start", "on_session_start", "session:start":
		return "SessionStart"
	case "sessionend", "session_end", "on_session_end", "session:end":
		return "SessionEnd"
	case "pretooluse", "pre_tool_use", "pre_tool_call":
		return "PreToolUse"
	case "posttooluse", "post_tool_use", "post_tool_call":
		return "PostToolUse"
	case "posttoolusefailure", "post_tool_use_failure", "tool_call_failed":
		return "PostToolUseFailure"
	case "userpromptsubmit", "user_prompt_submit", "pre_llm_call":
		return "UserPromptSubmit"
	default:
		return event
	}
}

func extraString(extra map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := extra[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func jsonValue(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func decodeToolTokens(encoded string) map[string]string {
	result := make(map[string]string)
	if strings.TrimSpace(encoded) == "" || json.Unmarshal([]byte(encoded), &result) != nil {
		return result
	}
	return result
}

func encodeToolTokens(tokens map[string]string) string {
	if len(tokens) == 0 {
		return ""
	}
	data, err := json.Marshal(tokens)
	if err != nil {
		return ""
	}
	return string(data)
}

func readStartTicks(pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return 0, errors.New("stat_invalid")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) <= 19 {
		return 0, errors.New("stat_invalid")
	}
	return strconv.ParseUint(fields[19], 10, 64)
}

func writeRootState(path string, state rootState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("state_dir_create_failed")
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !ownedByEUID(info) {
		return errors.New("state_dir_untrusted")
	}
	temporary, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return errors.New("state_create_failed")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return errors.New("state_chmod_failed")
	}
	if err := json.NewEncoder(temporary).Encode(state); err != nil {
		_ = temporary.Close()
		return errors.New("state_write_failed")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("state_write_failed")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return errors.New("state_commit_failed")
	}
	return nil
}

func readRootState(path string) (rootState, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !ownedByEUID(info) {
		return rootState{}, errors.New("state_untrusted")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return rootState{}, err
	}
	var state rootState
	if json.Unmarshal(data, &state) != nil || state.PID == 0 || state.StartTicks == 0 {
		return rootState{}, errors.New("state_invalid")
	}
	return state, nil
}

func readPrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !ownedByEUID(info) {
		return nil, errors.New("private_key_untrusted")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("private_key_read_failed")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("private_key_invalid")
	}
	return ed25519.PrivateKey(raw), nil
}

func sessionStateName(sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(digest[:]) + ".json"
}

func defaultStateDir() string {
	return filepath.Join(os.TempDir(), "aegis-codex-hook-"+strconv.Itoa(os.Geteuid()))
}

func ownedByEUID(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func stableError(err error) string {
	value := strings.TrimSpace(err.Error())
	if value == "" || len(value) > 80 {
		return "unknown"
	}
	for _, char := range value {
		if !(char == '_' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9') {
			return "unknown"
		}
	}
	return value
}

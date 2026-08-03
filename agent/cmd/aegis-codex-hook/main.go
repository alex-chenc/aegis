package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
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
)

const hookBridgeVersion = "1.0.0"

type codexHookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	Source        string `json:"source,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type rootState struct {
	PID        uint32 `json:"pid"`
	StartTicks uint64 `json:"start_ticks"`
}

type options struct {
	socketPath string
	privateKey string
	stateDir   string
	sourceID   string
	sourceVer  string
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
	var opts options
	flag.StringVar(&opts.socketPath, "socket", "/run/aegis-agent/agent-guard-hook.sock", "Aegis Agent hook socket")
	flag.StringVar(&opts.privateKey, "private-key", "/etc/aegis-agent/codex-hook.key", "Ed25519 private key")
	flag.StringVar(&opts.stateDir, "state-dir", defaultStateDir(), "per-user lifecycle state directory")
	flag.StringVar(&opts.sourceID, "source-id", "codex-native-hook", "trusted source manifest id")
	flag.StringVar(&opts.sourceVer, "source-version", hookBridgeVersion, "trusted source version")
	flag.Parse()
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
	var privateKeyPath, manifestPath, hooksPath, managedRequirements, socketPath, hookBinary, stateDir, sourceID, sourceVersion string
	flags.StringVar(&privateKeyPath, "private-key", "/etc/aegis-agent/codex-hook.key", "Ed25519 private key")
	flags.StringVar(&manifestPath, "manifest", "/etc/aegis-agent/codex-hook-sources.json", "trusted source manifest")
	flags.StringVar(&hooksPath, "hooks", filepath.Join(defaultHome, ".codex", "hooks.json"), "Codex hooks.json")
	flags.StringVar(&managedRequirements, "managed-requirements", "", "optional managed Codex requirements.toml")
	flags.StringVar(&socketPath, "socket", "/run/aegis-agent/agent-guard-hook.sock", "Aegis Agent hook socket")
	flags.StringVar(&hookBinary, "hook-binary", "/opt/aegis-agent/aegis-codex-hook", "installed hook helper")
	flags.StringVar(&stateDir, "state-dir", defaultStateDir(), "per-user lifecycle state directory")
	flags.StringVar(&sourceID, "source-id", "codex-native-hook", "trusted source id")
	flags.StringVar(&sourceVersion, "source-version", hookBridgeVersion, "trusted source version")
	if flags.Parse(arguments) != nil {
		return errors.New("arguments_invalid")
	}
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
	manifest := agentguard.TrustedToolSourceManifest{
		Schema: agentguard.ToolManifestSchemaV1,
		Sources: []agentguard.TrustedToolSource{{
			SourceID: sourceID, SourceType: agentguard.ToolSourceAdapterHook,
			Product: "codex", Version: sourceVersion, Verifier: "ed25519",
			PublicKey:    base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
			ArtifactPath: hookBinary, ArtifactDigest: artifactDigest,
			AllowedUIDs: []uint32{uint32(os.Geteuid())},
		}},
	}
	manifest.Digest, err = agentguard.TrustedToolManifestDigest(manifest)
	if err != nil {
		return errors.New("manifest_digest_failed")
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil || writeTrustedFile(manifestPath, append(manifestData, '\n')) != nil {
		return errors.New("manifest_write_failed")
	}
	command := strings.Join([]string{
		shellQuote(hookBinary), "--socket", shellQuote(socketPath),
		"--private-key", shellQuote(privateKeyPath), "--state-dir", shellQuote(stateDir),
		"--source-id", shellQuote(sourceID), "--source-version", shellQuote(sourceVersion),
	}, " ")
	if managedRequirements != "" {
		if err := writeManagedRequirements(managedRequirements, filepath.Dir(hookBinary), command); err != nil {
			return err
		}
	} else if err := mergeCodexHooks(hooksPath, command); err != nil {
		return err
	}
	fmt.Printf("AgentGuardSessionHookEnabled = true\nAgentGuardToolSourceManifest = %q\nAgentGuardToolHookSocket = %q\n", manifestPath, socketPath)
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
	appendHook("SessionEnd", "", 3)
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil || writeTrustedFile(path, append(data, '\n')) != nil {
		return errors.New("hooks_file_write_failed")
	}
	return nil
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
		!filepath.IsAbs(opts.stateDir) || opts.parentPID <= 0 || opts.now == nil || opts.stdin == nil {
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
		operation = agentguard.SessionEventActivated
		lifecycleReason = "tool"
	case "SessionEnd":
		operation = agentguard.SessionEventEnded
		lifecycleReason = input.Reason
	default:
		return errors.New("hook_event_unsupported")
	}
	if lifecycleReason == "" {
		lifecycleReason = "other"
	}
	statePath := filepath.Join(opts.stateDir, sessionStateName(input.SessionID))
	root, err := sessionRoot(operation, statePath, opts.parentPID)
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
	connection, err := net.DialTimeout("unix", opts.socketPath, 750*time.Millisecond)
	if err != nil {
		return errors.New("socket_connect_failed")
	}
	defer connection.Close()
	_ = connection.SetWriteDeadline(time.Now().Add(750 * time.Millisecond))
	if _, err := connection.Write(append(payload, '\n')); err != nil {
		return errors.New("socket_write_failed")
	}
	if operation == agentguard.SessionEventEnded {
		_ = os.Remove(statePath)
	}
	return nil
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
	if operation == agentguard.SessionEventStarted {
		if err := writeRootState(statePath, state); err != nil {
			return rootState{}, err
		}
	}
	return state, nil
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

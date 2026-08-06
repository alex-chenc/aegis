package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aegis-agent/internal/agentguard"
)

func TestSessionRootPinsFirstHookParentUntilSessionEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	start, err := sessionRoot(agentguard.SessionEventStarted, path, os.Getpid())
	if err != nil || start.PID != uint32(os.Getpid()) || start.StartTicks == 0 {
		t.Fatalf("start root=%#v err=%v", start, err)
	}
	end, err := sessionRoot(agentguard.SessionEventEnded, path, 1)
	if err != nil || end != start {
		t.Fatalf("end must reuse pinned first root: start=%#v end=%#v err=%v", start, end, err)
	}
}

func TestSessionStateNameDoesNotExposeSessionID(t *testing.T) {
	name := sessionStateName("thr_secret_value")
	if len(name) != 64+len(".json") || name == "thr_secret_value.json" {
		t.Fatalf("unsafe state name %q", name)
	}
}

func TestPreToolUseHookInputAllowsOfficialExtensionFields(t *testing.T) {
	input := `{"session_id":"thr_123","hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"call_1","tool_input":{"command":"true"},"cwd":"/workspace","permission_mode":"default","sandbox_mode":"workspace-write","network_access":false,"model":"gpt"}`
	var decoded codexHookInput
	if err := json.Unmarshal([]byte(input), &decoded); err != nil || decoded.SessionID != "thr_123" || decoded.HookEventName != "PreToolUse" {
		t.Fatalf("official extended input rejected: decoded=%#v err=%v", decoded, err)
	}
	if decoded.PermissionMode != "default" || decoded.SandboxMode != "workspace-write" || decoded.NetworkAccess == nil || *decoded.NetworkAccess {
		t.Fatalf("permission snapshot fields lost: %#v", decoded)
	}
}

func TestNativeProductHookEventsNormalizeToSharedContract(t *testing.T) {
	cases := map[string]string{
		"SessionStart":       "SessionStart",
		"session_start":      "SessionStart",
		"on_session_start":   "SessionStart",
		"pre_tool_call":      "PreToolUse",
		"post_tool_call":     "PostToolUse",
		"PostToolUseFailure": "PostToolUseFailure",
		"on_session_end":     "SessionEnd",
	}
	for input, want := range cases {
		if got := normalizeHookEvent(input, ""); got != want {
			t.Fatalf("normalizeHookEvent(%q) = %q, want %q", input, got, want)
		}
	}
	if got := normalizeHookEvent("Stop", ""); got != "Stop" {
		t.Fatalf("Zcode Stop must not become session end: %q", got)
	}
	var hermes codexHookInput
	if err := json.Unmarshal([]byte(`{"session_id":"sess_hermes","hook_event_name":"pre_tool_call","tool_name":"terminal","tool_input":{"command":"id"},"extra":{"tool_call_id":"call_hermes","task_id":"turn_1"}}`), &hermes); err != nil {
		t.Fatal(err)
	}
	if extraString(hermes.Extra, "tool_call_id") != "call_hermes" || extraString(hermes.Extra, "task_id") != "turn_1" {
		t.Fatalf("Hermes hook payload fields lost: %#v", hermes)
	}
}

func TestProvisionKeepsMultipleNativeSources(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "aegis-codex-hook")
	if err := os.WriteFile(binary, []byte("test-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "sources.json")
	common := []string{"--private-key", filepath.Join(dir, "hook.key"), "--manifest", manifest,
		"--socket", filepath.Join(dir, "hook.sock"), "--hook-binary", binary, "--state-dir", filepath.Join(dir, "state")}
	if err := provision(append(common, "--agent-type", "claude-code", "--hooks", filepath.Join(dir, "claude.json"))); err != nil {
		t.Fatal(err)
	}
	if err := provision(append(common, "--agent-type", "hermes", "--hooks", filepath.Join(dir, "hermes.yaml"))); err != nil {
		t.Fatal(err)
	}
	adapter, err := agentguard.LoadTrustedToolAdapter(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, sourceID := range []string{"claude-code-hook-v1", "hermes-hook-v1"} {
		if !adapter.AuthorizePeer(sourceID, uint32(os.Geteuid()), uint32(os.Getegid())) {
			t.Fatalf("source %q missing after second provision", sourceID)
		}
	}
}

func TestProvisionOpenClawWritesNativePluginAndConfigLoadPath(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "aegis-codex-hook")
	if err := os.WriteFile(binary, []byte("test-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "openclaw", "config.json")
	if err := provision([]string{
		"--agent-type", "openclaw", "--private-key", filepath.Join(dir, "hook.key"),
		"--manifest", filepath.Join(dir, "sources.json"), "--hooks", config,
		"--socket", filepath.Join(dir, "hook.sock"), "--hook-binary", binary,
		"--state-dir", filepath.Join(dir, "state"),
	}); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(dir, "openclaw", "extensions", "aegis-agent-guard", "index.js")
	if _, err := os.Stat(plugin); err != nil {
		t.Fatalf("OpenClaw plugin not provisioned: %v", err)
	}
	data, err := os.ReadFile(config)
	if err != nil || !strings.Contains(string(data), "aegis-agent-guard") {
		t.Fatalf("OpenClaw config does not load Aegis plugin: %v %s", err, data)
	}
}

func TestProvisionWritesSignedManifestAndFourNativeHookPoints(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "aegis-codex-hook")
	if err := os.WriteFile(binary, []byte("test-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "codex-hook.key")
	manifest := filepath.Join(dir, "sources.json")
	hooks := filepath.Join(dir, "hooks.json")
	if err := provision([]string{
		"--private-key", key, "--manifest", manifest, "--hooks", hooks,
		"--socket", filepath.Join(dir, "hook.sock"), "--hook-binary", binary,
		"--state-dir", filepath.Join(dir, "state"),
	}); err != nil {
		t.Fatal(err)
	}
	adapter, err := agentguard.LoadTrustedToolAdapter(manifest)
	if err != nil || !adapter.AuthorizePeer("codex-native-hook", uint32(os.Geteuid()), uint32(os.Getegid())) {
		t.Fatalf("provisioned manifest invalid: adapter=%#v err=%v", adapter, err)
	}
	data, err := os.ReadFile(hooks)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "PreToolUse", "PostToolUse", "SessionEnd"} {
		if !strings.Contains(string(data), `"`+event+`"`) {
			t.Fatalf("hook %s missing: %s", event, data)
		}
	}
}

func TestRemovePreservesUserHooksAndRemovesAegisHookAndManifestSource(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "aegis-codex-hook")
	if err := os.WriteFile(binary, []byte("test-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "codex-hook.key")
	manifest := filepath.Join(dir, "codex-hook-sources.json")
	hooks := filepath.Join(dir, "hooks.json")
	if err := os.WriteFile(hooks, []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"user-command"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := provision([]string{
		"--private-key", key, "--manifest", manifest, "--hooks", hooks,
		"--socket", filepath.Join(dir, "hook.sock"), "--hook-binary", binary,
		"--state-dir", filepath.Join(dir, "state"), "--agent-type", "codex",
	}); err != nil {
		t.Fatal(err)
	}
	if err := remove([]string{
		"--manifest", manifest, "--hooks", hooks, "--hook-binary", binary,
		"--agent-type", "codex",
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(hooks)
	if err != nil || !strings.Contains(string(data), "user-command") || strings.Contains(string(data), binary) {
		t.Fatalf("unexpected hooks after removal: err=%v data=%s", err, data)
	}
	manifestData, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var manifestValue agentguard.TrustedToolSourceManifest
	if err := json.Unmarshal(manifestData, &manifestValue); err != nil {
		t.Fatal(err)
	}
	if len(manifestValue.Sources) != 0 {
		t.Fatalf("Aegis source was not removed: %#v", manifestValue.Sources)
	}
}

func TestRemoveOpenClawRemovesManagedPluginAndLoadPath(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "aegis-codex-hook")
	if err := os.WriteFile(binary, []byte("test-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "openclaw", "openclaw.json")
	manifest := filepath.Join(dir, "sources.json")
	if err := provision([]string{
		"--agent-type", "openclaw", "--private-key", filepath.Join(dir, "key"),
		"--manifest", manifest, "--hooks", config, "--socket", filepath.Join(dir, "hook.sock"),
		"--hook-binary", binary, "--state-dir", filepath.Join(dir, "state"),
	}); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(filepath.Dir(config), "extensions", "aegis-agent-guard")
	if _, err := os.Stat(filepath.Join(pluginDir, "index.js")); err != nil {
		t.Fatalf("managed plugin was not created: %v", err)
	}
	if err := remove([]string{
		"--agent-type", "openclaw", "--manifest", manifest, "--hooks", config,
		"--hook-binary", binary,
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config)
	if err != nil || strings.Contains(string(data), "aegis-agent-guard") {
		t.Fatalf("OpenClaw managed load path was not removed: err=%v data=%s", err, data)
	}
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Fatalf("managed plugin directory still exists: %v", err)
	}
}

func TestProvisionCanWriteManagedHooksWithoutUserTrustPrompt(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "aegis-codex-hook")
	if err := os.WriteFile(binary, []byte("test-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	requirements := filepath.Join(dir, "requirements.toml")
	if err := provision([]string{
		"--private-key", filepath.Join(dir, "key"),
		"--manifest", filepath.Join(dir, "manifest.json"), "--hooks", "",
		"--managed-requirements", requirements, "--socket", filepath.Join(dir, "hook.sock"),
		"--hook-binary", binary, "--state-dir", filepath.Join(dir, "state"),
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(requirements)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"[features]", "[[hooks.SessionStart]]", "[[hooks.PreToolUse]]", "[[hooks.PostToolUse]]", "[[hooks.SessionEnd]]"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("managed hook %q missing: %s", expected, data)
		}
	}
}

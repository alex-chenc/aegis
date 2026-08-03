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
	input := `{"session_id":"thr_123","hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"call_1","tool_input":{"command":"true"},"cwd":"/workspace","model":"gpt"}`
	var decoded codexHookInput
	if err := json.Unmarshal([]byte(input), &decoded); err != nil || decoded.SessionID != "thr_123" || decoded.HookEventName != "PreToolUse" {
		t.Fatalf("official extended input rejected: decoded=%#v err=%v", decoded, err)
	}
}

func TestProvisionWritesSignedManifestAndThreeNativeHookPoints(t *testing.T) {
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
	for _, event := range []string{"SessionStart", "PreToolUse", "SessionEnd"} {
		if !strings.Contains(string(data), `"`+event+`"`) {
			t.Fatalf("hook %s missing: %s", event, data)
		}
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
	for _, expected := range []string{"[features]", "[[hooks.SessionStart]]", "[[hooks.PreToolUse]]", "[[hooks.SessionEnd]]"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("managed hook %q missing: %s", expected, data)
		}
	}
}

package agentguard

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestBuiltinProfilesRequireIndependentEvidence(t *testing.T) {
	registry := NewBuiltinProfileRegistry()

	cases := []struct {
		name    string
		process ProcessSnapshot
		want    string
	}{
		{
			name: "codex",
			process: ProcessSnapshot{
				Identity:       ProcessIdentity{PID: 4100, StartTicks: 100},
				Exe:            "/opt/codex/bin/codex",
				Argv:           []string{"codex", "--sandbox", "workspace-write"},
				ConfigEvidence: []string{".codex"},
			},
			want: "codex-linux",
		},
		{
			name: "openclaw",
			process: ProcessSnapshot{
				Identity:       ProcessIdentity{PID: 5200, StartTicks: 200},
				Exe:            "/usr/bin/openclaw",
				Argv:           []string{"openclaw", "gateway"},
				ConfigEvidence: []string{".openclaw"},
			},
			want: "openclaw-linux",
		},
		{
			name: "hermes",
			process: ProcessSnapshot{
				Identity:       ProcessIdentity{PID: 6100, StartTicks: 300},
				Exe:            "/usr/bin/python3",
				Argv:           []string{"python3", "-m", "hermes.agent"},
				ConfigEvidence: []string{".hermes"},
			},
			want: "hermes-linux",
		},
		{
			name: "claude-code",
			process: ProcessSnapshot{
				Identity: ProcessIdentity{PID: 7100, StartTicks: 400}, Exe: "/usr/bin/claude",
				Argv: []string{"claude"}, ConfigEvidence: []string{".claude"},
			},
			want: "claude-code-linux",
		},
		{
			name: "opencode",
			process: ProcessSnapshot{
				Identity: ProcessIdentity{PID: 7200, StartTicks: 500}, Exe: "/usr/bin/opencode",
				Argv: []string{"opencode"}, ConfigEvidence: []string{".config/opencode"},
			},
			want: "opencode-linux",
		},
		{
			name: "gemini-cli",
			process: ProcessSnapshot{
				Identity: ProcessIdentity{PID: 7300, StartTicks: 600}, Exe: "/usr/bin/gemini",
				Argv: []string{"gemini"}, ConfigEvidence: []string{".gemini"},
			},
			want: "gemini-cli-linux",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match := registry.MatchController(tc.process)
			if match.Profile == nil || match.Profile.ProfileKey != tc.want {
				t.Fatalf("expected profile %q, got %#v", tc.want, match)
			}
			if match.Confidence != ConfidenceConfirmed {
				t.Fatalf("expected confirmed match, got %q", match.Confidence)
			}
		})
	}

	candidate := registry.MatchController(ProcessSnapshot{
		Identity: ProcessIdentity{PID: 7000, StartTicks: 400},
		Exe:      "/tmp/codex",
		Argv:     []string{"codex"},
	})
	if candidate.Confidence != ConfidenceCandidate {
		t.Fatalf("a spoofable process name must remain candidate, got %q", candidate.Confidence)
	}
}

func TestBuiltinProfilesUseControlPlaneManifestContract(t *testing.T) {
	data, err := json.Marshal(NewBuiltinProfileRegistry().Profiles())
	if err != nil {
		t.Fatal(err)
	}
	payload := string(data)
	for _, field := range []string{
		`"controller_match"`, `"worker_match"`, `"backend_detectors"`,
		`"isolation_expectation"`, `"default_escape_rules"`, `"sandbox_family"`,
	} {
		if !strings.Contains(payload, field) {
			t.Fatalf("profile contract missing %s: %s", field, payload)
		}
	}
	if strings.Contains(payload, `"executables"`) || strings.Contains(payload, `"required_argv"`) {
		t.Fatalf("second profile contract leaked into bundle: %s", payload)
	}
}

func TestProfileDoesNotClaimOrdinaryShell(t *testing.T) {
	match := NewBuiltinProfileRegistry().MatchController(ProcessSnapshot{
		Identity: ProcessIdentity{PID: 8000, StartTicks: 500},
		Exe:      "/usr/bin/bash",
		Argv:     []string{"bash", "-c", "python3 task.py"},
	})
	if match.Profile != nil || match.Confidence != ConfidenceUnattributed {
		t.Fatalf("ordinary bash must remain unattributed: %#v", match)
	}
}

func TestP4BuiltinProfilesMatchCanonicalControlPlaneDefinitions(t *testing.T) {
	expected := map[string]string{
		"claude-code-linux": "sha256:e4158634ff61db23c9fa930507e5d91bb79840e94508e7ec9d4d5cd76f0e01e1",
		"opencode-linux":    "sha256:c02f7b4117b237dda288bb3eaf5611770f0efa0b42cb5970f916126472ecb7b1",
		"gemini-cli-linux":  "sha256:7038eb7b2a4799747ebd3ec4b29b37f40c0ec44db72b362277915aa7b92141d7",
		"zcode-linux":       "sha256:bcb65be77f138f3f0f5d6de4ac2d017b43876f9cd98a0d0a7c55bd0f8dd5389c",
	}
	registry := NewBuiltinProfileRegistry()
	for key, expectedDigest := range expected {
		profile, ok := registry.Profile(key)
		if !ok {
			t.Fatalf("missing profile %s", key)
		}
		calculated, err := ProfileDefinitionDigest(profile)
		if err != nil {
			t.Fatalf("calculate %s digest: %v", key, err)
		}
		if profile.ProfileVersion != 1 || profile.Digest != expectedDigest || calculated != expectedDigest {
			encoded, _ := json.Marshal(profile)
			t.Fatalf("%s definition digest mismatch: declared=%s calculated=%s profile=%s", key, profile.Digest, calculated, encoded)
		}
		if len(profile.BackendDetectors) == 0 || profile.BackendDetectors[0].Backend != "local" ||
			!slices.Equal(profile.BackendDetectors[0].Signals, []string{"terminal_local"}) {
			t.Fatalf("%s local detector drifted: %#v", key, profile.BackendDetectors)
		}
		if !slices.Equal(profile.DefaultEscapeRules, []string{
			"access_container_runtime_socket", "join_external_namespace", "write_cgroupfs",
			"credential_or_capability_gain", "isolation_baseline_drift",
		}) {
			t.Fatalf("%s escape rules drifted: %#v", key, profile.DefaultEscapeRules)
		}
	}
}

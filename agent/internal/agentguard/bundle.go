package agentguard

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type BundleRule struct {
	RuleKey            string         `json:"rule_key"`
	RuleVersion        int64          `json:"rule_version"`
	Enabled            bool           `json:"enabled"`
	Severity           string         `json:"severity"`
	Action             string         `json:"action"`
	CompiledParameters map[string]any `json:"compiled_parameters"`
	Digest             string         `json:"digest"`
}

type BundlePolicy map[string]any

type BundleDefaults struct {
	Mode                     string `json:"mode"`
	BehaviorMonitorEnabled   bool   `json:"behavior_monitor_enabled"`
	ToolAdapterEnabled       bool   `json:"tool_adapter_enabled"`
	EnforcementEnabled       bool   `json:"enforcement_enabled"`
	FreezeEnabled            bool   `json:"freeze_enabled"`
	FreezeTimeoutSeconds     int    `json:"freeze_timeout_seconds"`
	ReconcileIntervalSeconds int    `json:"reconcile_interval_seconds"`
}

type Bundle struct {
	Schema        string           `json:"schema"`
	BundleVersion int64            `json:"bundle_version"`
	GeneratedAt   time.Time        `json:"generated_at"`
	HostID        string           `json:"host_id"`
	Profiles      []AdapterProfile `json:"profiles"`
	BuiltinRules  []BundleRule     `json:"builtin_rules"`
	Policies      []BundlePolicy   `json:"policies,omitempty"`
	Defaults      BundleDefaults   `json:"defaults"`
	Digest        string           `json:"digest"`
}

type BundleValidationOptions struct {
	EnforcementAllowed bool
	FreezeAllowed      bool
	ToolAdapterAllowed bool
}

func (b Bundle) Validate(expectedHostID string) error {
	return b.ValidateWithOptions(expectedHostID, BundleValidationOptions{})
}

func (b Bundle) ValidateWithOptions(expectedHostID string, options BundleValidationOptions) error {
	if err := b.validateStructure(expectedHostID, options); err != nil {
		return err
	}
	digest, err := BundleDigest(b)
	if err != nil {
		return err
	}
	if !constantDigestEqual(digest, b.Digest) {
		return errors.New("agent_guard_bundle_digest_mismatch")
	}
	return nil
}

func (b Bundle) validateStructure(expectedHostID string, options BundleValidationOptions) error {
	if b.Schema != BundleSchemaV1 {
		return fmt.Errorf("agent_guard_bundle_schema_unsupported: %q", b.Schema)
	}
	if b.BundleVersion <= 0 {
		return errors.New("agent_guard_bundle_version_invalid")
	}
	if b.GeneratedAt.IsZero() {
		return errors.New("agent_guard_bundle_generated_at_missing")
	}
	if b.HostID == "" || (expectedHostID != "" && b.HostID != expectedHostID) {
		return errors.New("agent_guard_bundle_host_scope_mismatch")
	}
	switch b.Defaults.Mode {
	case "monitor_only":
	case "enforce":
		if !b.Defaults.EnforcementEnabled {
			return errors.New("agent_guard_bundle_enforcement_mode_inconsistent")
		}
	default:
		return errors.New("agent_guard_bundle_mode_invalid")
	}
	if b.Defaults.EnforcementEnabled && !options.EnforcementAllowed {
		return errors.New("agent_guard_bundle_local_enforcement_disabled")
	}
	if b.Defaults.FreezeEnabled && (!b.Defaults.EnforcementEnabled || !options.FreezeAllowed) {
		return errors.New("agent_guard_bundle_local_freeze_disabled")
	}
	if b.Defaults.ToolAdapterEnabled && !options.ToolAdapterAllowed {
		return errors.New("agent_guard_bundle_local_tool_adapter_disabled")
	}
	if b.Defaults.ReconcileIntervalSeconds < 5 || b.Defaults.ReconcileIntervalSeconds > 3600 {
		return errors.New("agent_guard_bundle_reconcile_interval_invalid")
	}
	if b.Defaults.FreezeTimeoutSeconds < 30 || b.Defaults.FreezeTimeoutSeconds > 900 {
		return errors.New("agent_guard_bundle_freeze_timeout_invalid")
	}
	for _, policy := range b.Policies {
		if containsForbiddenAction(policy) && (!options.EnforcementAllowed || !b.Defaults.EnforcementEnabled) {
			return errors.New("agent_guard_bundle_policy_enforcement_forbidden")
		}
	}
	if err := validateBundleProfiles(b.Profiles); err != nil {
		return err
	}
	if err := validateBuiltinRules(b.BuiltinRules, options.EnforcementAllowed && b.Defaults.EnforcementEnabled); err != nil {
		return err
	}
	return nil
}

func validateBundleProfiles(profiles []AdapterProfile) error {
	required := map[string]string{
		"codex-linux":       "sha256:ac7f7259e1ea26729377e4535cbdbb2a1e2c17befdeb3965a924388acb0c2384",
		"openclaw-linux":    "sha256:56804a5b02e48827bb944959412ee8f19d46e333257f068e0197d81245e71c4d",
		"hermes-linux":      "sha256:0bf30bb4daff9b86ccf4fd4fad7bc515f3fb3ed760a7b7ce6ca98f5783889524",
		"claude-code-linux": "sha256:e4158634ff61db23c9fa930507e5d91bb79840e94508e7ec9d4d5cd76f0e01e1",
		"opencode-linux":    "sha256:c02f7b4117b237dda288bb3eaf5611770f0efa0b42cb5970f916126472ecb7b1",
		"gemini-cli-linux":  "sha256:7038eb7b2a4799747ebd3ec4b29b37f40c0ec44db72b362277915aa7b92141d7",
	}
	found := make(map[string]bool, len(required))
	for _, profile := range profiles {
		expectedDigest, ok := required[profile.ProfileKey]
		if !ok || profile.ProfileVersion != 1 || profile.AgentType == "" || profile.Digest != expectedDigest {
			return fmt.Errorf("agent_guard_bundle_profile_invalid: %s", profile.ProfileKey)
		}
		if profile.ProfileKey == "claude-code-linux" || profile.ProfileKey == "opencode-linux" ||
			profile.ProfileKey == "gemini-cli-linux" {
			calculatedDigest, err := ProfileDefinitionDigest(profile)
			if err != nil || !constantDigestEqual(calculatedDigest, profile.Digest) {
				return fmt.Errorf("agent_guard_bundle_profile_definition_mismatch: %s", profile.ProfileKey)
			}
		}
		if found[profile.ProfileKey] {
			return fmt.Errorf("agent_guard_bundle_profile_duplicate: %s", profile.ProfileKey)
		}
		found[profile.ProfileKey] = true
	}
	for key := range required {
		if !found[key] {
			return fmt.Errorf("agent_guard_bundle_profile_missing: %s", key)
		}
	}
	return nil
}

// ProfileDefinitionDigest mirrors the control-plane immutable profile digest.
// Source and enabled are part of that cross-component contract even though an
// Agent bundle only carries enabled built-in definitions.
func ProfileDefinitionDigest(profile AdapterProfile) (string, error) {
	controllerMatch, err := canonicalProfileField(profile.ControllerMatch)
	if err != nil {
		return "", err
	}
	workerMatch, err := canonicalProfileField(profile.WorkerMatch)
	if err != nil {
		return "", err
	}
	backendDetectors, err := canonicalProfileField(profile.BackendDetectors)
	if err != nil {
		return "", err
	}
	isolationExpectation, err := canonicalProfileField(profile.IsolationExpectation)
	if err != nil {
		return "", err
	}
	defaultEscapeRules, err := canonicalProfileField(profile.DefaultEscapeRules)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(struct {
		ProfileKey           string        `json:"profile_key"`
		ProfileVersion       int64         `json:"profile_version"`
		AgentType            string        `json:"agent_type"`
		DisplayName          string        `json:"display_name"`
		Source               string        `json:"source"`
		SandboxFamily        IsolationType `json:"sandbox_family"`
		ControllerMatch      any           `json:"controller_match"`
		WorkerMatch          any           `json:"worker_match"`
		BackendDetectors     any           `json:"backend_detectors"`
		IsolationExpectation any           `json:"isolation_expectation"`
		DefaultEscapeRules   any           `json:"default_escape_rules"`
		Enabled              bool          `json:"enabled"`
	}{
		ProfileKey: profile.ProfileKey, ProfileVersion: profile.ProfileVersion,
		AgentType: profile.AgentType, DisplayName: profile.DisplayName, Source: "builtin",
		SandboxFamily: profile.SandboxFamily, ControllerMatch: controllerMatch,
		WorkerMatch: workerMatch, BackendDetectors: backendDetectors,
		IsolationExpectation: isolationExpectation,
		DefaultEscapeRules:   defaultEscapeRules, Enabled: true,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func canonicalProfileField(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func validateBuiltinRules(rules []BundleRule, allowEnforcement bool) error {
	required := map[string]string{
		"AGB-BUILTIN-001": "sha256:e9a7f8b0dda7c742557bbc1a0551ea4caeb0329973ec1c24f7751b4cd2902a82",
		"AGB-BUILTIN-002": "sha256:5852cf43c0be2ddc21e83c8c12fb898ac2aae47bc0d7bff2a5246d4d2436e613",
		"AGB-BUILTIN-003": "sha256:b066e0b452fb7749f9afb49e8a6b918de1285ccef912f50680795a4bc110e03e",
		"AGB-BUILTIN-004": "sha256:43e4e365124e4d895a27f8267e8ab424f0482f121794a812aea946231520e130",
		"AGB-BUILTIN-005": "sha256:63ce19628fc8285ded19f9609ec93770341c14e24e47680e95aff3cec4d775f1",
	}
	found := make(map[string]bool, len(required))
	for _, rule := range rules {
		expectedDigest, ok := required[rule.RuleKey]
		if !ok {
			return fmt.Errorf("agent_guard_bundle_rule_unknown: %s", rule.RuleKey)
		}
		if found[rule.RuleKey] {
			return fmt.Errorf("agent_guard_bundle_rule_duplicate: %s", rule.RuleKey)
		}
		if rule.RuleVersion != 1 || rule.Digest != expectedDigest {
			return fmt.Errorf("agent_guard_bundle_rule_invalid: %s", rule.RuleKey)
		}
		switch strings.ToLower(strings.TrimSpace(rule.Action)) {
		case "", "audit", "alert", "would_deny":
		case "deny", "deny_and_freeze":
			if !allowEnforcement {
				return fmt.Errorf("agent_guard_bundle_rule_action_forbidden: %s", rule.RuleKey)
			}
		default:
			return fmt.Errorf("agent_guard_bundle_rule_action_forbidden: %s", rule.RuleKey)
		}
		found[rule.RuleKey] = true
	}
	for key := range required {
		if !found[key] {
			return fmt.Errorf("agent_guard_bundle_rule_missing: %s", key)
		}
	}
	return nil
}

func BundleDigest(bundle Bundle) (string, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("canonicalize agent guard bundle: %w", err)
	}
	return BundlePayloadDigest(data)
}

func BundlePayloadDigest(data []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var canonical map[string]any
	if err := decoder.Decode(&canonical); err != nil {
		return "", fmt.Errorf("canonicalize agent guard bundle: %w", err)
	}
	delete(canonical, "digest")
	canonicalData, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonicalize agent guard bundle: %w", err)
	}
	sum := sha256.Sum256(canonicalData)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func containsForbiddenAction(value any) bool {
	switch typed := value.(type) {
	case string:
		switch strings.ToLower(typed) {
		case "deny", "deny_and_freeze", "freeze", "freeze_execution_unit",
			"kill", "kill_execution_unit", "kill_agent_instance":
			return true
		}
	case []any:
		for _, item := range typed {
			if containsForbiddenAction(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if containsForbiddenAction(item) {
				return true
			}
		}
	case BundlePolicy:
		for _, item := range typed {
			if containsForbiddenAction(item) {
				return true
			}
		}
	}
	return false
}

func constantDigestEqual(a, b string) bool {
	if !strings.HasPrefix(a, "sha256:") || !strings.HasPrefix(b, "sha256:") {
		return false
	}
	aa, errA := hex.DecodeString(strings.TrimPrefix(a, "sha256:"))
	bb, errB := hex.DecodeString(strings.TrimPrefix(b, "sha256:"))
	if errA != nil || errB != nil || len(aa) != sha256.Size || len(bb) != sha256.Size {
		return false
	}
	return hmac.Equal(aa, bb)
}

type BundleStore struct {
	dir     string
	hostID  string
	options BundleValidationOptions
	mu      sync.RWMutex
	current *Bundle
}

func NewBundleStore(dir, hostID string) *BundleStore {
	return &BundleStore{dir: dir, hostID: hostID}
}

func NewBundleStoreWithOptions(dir, hostID string, options BundleValidationOptions) *BundleStore {
	return &BundleStore{dir: dir, hostID: hostID, options: options}
}

func (s *BundleStore) ApplyFullSync(payload []byte) (Bundle, error) {
	bundle, err := s.ValidatePayload(payload)
	if err != nil {
		return Bundle{}, err
	}
	return s.CommitValidated(bundle)
}

func (s *BundleStore) ValidatePayload(payload []byte) (Bundle, error) {
	bundle, err := decodeBundle(payload)
	if err != nil {
		return Bundle{}, err
	}
	if err := bundle.validateStructure(s.hostID, s.options); err != nil {
		return Bundle{}, err
	}
	payloadDigest, err := BundlePayloadDigest(payload)
	if err != nil || !constantDigestEqual(payloadDigest, bundle.Digest) {
		return Bundle{}, errors.New("agent_guard_bundle_digest_mismatch")
	}
	return bundle, nil
}

func (s *BundleStore) CommitValidated(bundle Bundle) (Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		if bundle.BundleVersion < s.current.BundleVersion {
			return Bundle{}, errors.New("agent_guard_bundle_version_stale")
		}
		if bundle.BundleVersion == s.current.BundleVersion {
			if bundle.Digest != s.current.Digest {
				return Bundle{}, errors.New("agent_guard_bundle_version_digest_conflict")
			}
			return *s.current, nil
		}
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return Bundle{}, fmt.Errorf("marshal agent guard bundle: %w", err)
	}
	if err := atomicWrite(filepath.Join(s.dir, "bundle.json"), data, 0o600); err != nil {
		return Bundle{}, fmt.Errorf("persist agent guard bundle: %w", err)
	}
	if err := atomicWrite(filepath.Join(s.dir, "bundle.digest"), []byte(bundle.Digest+"\n"), 0o600); err != nil {
		return Bundle{}, fmt.Errorf("persist agent guard digest: %w", err)
	}
	s.current = &bundle
	return bundle, nil
}

func (s *BundleStore) Load() (Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		return *s.current, nil
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "bundle.json"))
	if err != nil {
		return Bundle{}, err
	}
	bundle, err := decodeBundle(data)
	if err != nil {
		return Bundle{}, err
	}
	if err := bundle.validateStructure(s.hostID, s.options); err != nil {
		return Bundle{}, err
	}
	payloadDigest, err := BundlePayloadDigest(data)
	if err != nil || !constantDigestEqual(payloadDigest, bundle.Digest) {
		return Bundle{}, errors.New("agent_guard_bundle_digest_mismatch")
	}
	s.current = &bundle
	return bundle, nil
}

func decodeBundle(data []byte) (Bundle, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var bundle Bundle
	if err := decoder.Decode(&bundle); err != nil {
		return Bundle{}, fmt.Errorf("agent_guard_bundle_json_invalid: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Bundle{}, errors.New("agent_guard_bundle_json_trailing_value")
		}
		return Bundle{}, fmt.Errorf("agent_guard_bundle_json_invalid: %w", err)
	}
	return bundle, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-guard-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

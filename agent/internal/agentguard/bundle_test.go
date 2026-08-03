package agentguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validBundle(t *testing.T, version int64) Bundle {
	t.Helper()
	bundle := Bundle{
		Schema:        BundleSchemaV1,
		BundleVersion: version,
		GeneratedAt:   time.Now().UTC(),
		HostID:        "host-1",
		Profiles:      NewBuiltinProfileRegistry().Profiles(),
		BuiltinRules: []BundleRule{
			{RuleKey: "AGB-BUILTIN-001", RuleVersion: 1, Digest: "sha256:e9a7f8b0dda7c742557bbc1a0551ea4caeb0329973ec1c24f7751b4cd2902a82"},
			{RuleKey: "AGB-BUILTIN-002", RuleVersion: 1, Digest: "sha256:5852cf43c0be2ddc21e83c8c12fb898ac2aae47bc0d7bff2a5246d4d2436e613"},
			{RuleKey: "AGB-BUILTIN-003", RuleVersion: 1, Digest: "sha256:b066e0b452fb7749f9afb49e8a6b918de1285ccef912f50680795a4bc110e03e"},
			{RuleKey: "AGB-BUILTIN-004", RuleVersion: 1, Digest: "sha256:43e4e365124e4d895a27f8267e8ab424f0482f121794a812aea946231520e130"},
			{RuleKey: "AGB-BUILTIN-005", RuleVersion: 1, Digest: "sha256:63ce19628fc8285ded19f9609ec93770341c14e24e47680e95aff3cec4d775f1"},
		},
		Defaults: BundleDefaults{
			Mode:                     "monitor_only",
			BehaviorMonitorEnabled:   true,
			FreezeTimeoutSeconds:     300,
			ReconcileIntervalSeconds: 30,
		},
	}
	digest, err := BundleDigest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Digest = digest
	return bundle
}

func TestBundleValidateDigestAndMonitorOnlyBoundary(t *testing.T) {
	bundle := validBundle(t, 7)
	if err := bundle.Validate("host-1"); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	bundle.Defaults.Mode = "deny"
	if err := bundle.Validate("host-1"); err == nil {
		t.Fatal("P1 must reject enforcement bundle")
	}
	bundle = validBundle(t, 7)
	bundle.Defaults.EnforcementEnabled = true
	digest, _ := BundleDigest(bundle)
	bundle.Digest = digest
	if err := bundle.Validate("host-1"); err == nil {
		t.Fatal("P1 must reject enforcement_enabled")
	}
	bundle = validBundle(t, 7)
	bundle.Defaults.ToolAdapterEnabled = true
	digest, _ = BundleDigest(bundle)
	bundle.Digest = digest
	if err := bundle.Validate("host-1"); err == nil {
		t.Fatal("P1 must reject tool_adapter_enabled")
	}
	bundle = validBundle(t, 7)
	bundle.BuiltinRules[0].Action = "deny"
	digest, _ = BundleDigest(bundle)
	bundle.Digest = digest
	if err := bundle.Validate("host-1"); err == nil {
		t.Fatal("P2 monitor-only bundle must reject an active builtin rule action")
	}
	bundle = validBundle(t, 7)
	bundle.HostID = "host-2"
	if err := bundle.Validate("host-1"); err == nil {
		t.Fatal("host scope mismatch accepted")
	}
}

func TestBundleDigestDeletesTopLevelDigestField(t *testing.T) {
	bundle := validBundle(t, 7)
	first, err := BundleDigest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Digest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	second, err := BundleDigest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("top-level digest field participated in canonical digest: %s != %s", first, second)
	}
}

func TestBundleStorePreservesLastKnownGoodOnInvalidApply(t *testing.T) {
	dir := t.TempDir()
	store := NewBundleStore(dir, "host-1")
	first := validBundle(t, 7)
	payload, _ := json.Marshal(first)
	if _, err := store.ApplyFullSync(payload); err != nil {
		t.Fatalf("apply valid bundle: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("bundle permissions = %o", info.Mode().Perm())
	}

	bad := validBundle(t, 8)
	bad.Digest = "sha256:invalid"
	badPayload, _ := json.Marshal(bad)
	if _, err := store.ApplyFullSync(badPayload); err == nil {
		t.Fatal("invalid digest unexpectedly applied")
	}
	current, err := store.Load()
	if err != nil {
		t.Fatalf("load last-known-good: %v", err)
	}
	if current.BundleVersion != 7 {
		t.Fatalf("last-known-good replaced by invalid bundle: %#v", current)
	}
}

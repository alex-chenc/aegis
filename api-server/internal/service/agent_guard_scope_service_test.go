package service

import (
	"strings"
	"testing"
	"time"
)

func TestAgentGuardScopeSignerStableAndTamperEvident(t *testing.T) {
	signer, err := NewAgentGuardScopeSigner(strings.Repeat("k", 32))
	if err != nil {
		t.Fatalf("NewAgentGuardScopeSigner: %v", err)
	}

	first, err := signer.Sign(AgentGuardScope{
		HostID:     "host-1",
		AgentType:  "codex",
		ProfileKey: "codex-linux",
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	second, err := signer.Sign(AgentGuardScope{
		HostID:     "host-1",
		AgentType:  "codex",
		ProfileKey: "codex-linux",
	})
	if err != nil {
		t.Fatalf("Sign second: %v", err)
	}
	if first != second {
		t.Fatalf("scope key must be stable: %q != %q", first, second)
	}

	got, err := signer.Verify(first)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.HostID != "host-1" || got.AgentType != "codex" || got.ProfileKey != "codex-linux" {
		t.Fatalf("unexpected verified scope: %#v", got)
	}

	tampered := first[:len(first)-1] + "A"
	if tampered == first {
		tampered = first[:len(first)-1] + "B"
	}
	if _, err := signer.Verify(tampered); err == nil {
		t.Fatal("tampered scope key must be rejected")
	}
}

func TestAgentGuardScopeSignerRejectsWeakKeyAndInvalidScope(t *testing.T) {
	if _, err := NewAgentGuardScopeSigner("short"); err == nil {
		t.Fatal("weak signing key must be rejected")
	}

	signer, err := NewAgentGuardScopeSigner(strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("NewAgentGuardScopeSigner: %v", err)
	}
	if _, err := signer.Sign(AgentGuardScope{HostID: "host-1"}); err == nil {
		t.Fatal("scope without agent type must be rejected")
	}
	if _, err := signer.Verify("not-a-signed-scope"); err == nil {
		t.Fatal("invalid token must be rejected")
	}
}

func TestAgentGuardScopeSignerBindsLinkedAsset(t *testing.T) {
	signer, err := NewAgentGuardScopeSigner(strings.Repeat("a", 32))
	if err != nil {
		t.Fatalf("NewAgentGuardScopeSigner: %v", err)
	}
	first, err := signer.Sign(AgentGuardScope{
		HostID: "host-1", AgentType: "codex", ProfileKey: "codex-linux", AssetID: "asset-1",
	})
	if err != nil {
		t.Fatalf("Sign first asset: %v", err)
	}
	second, err := signer.Sign(AgentGuardScope{
		HostID: "host-1", AgentType: "codex", ProfileKey: "codex-linux", AssetID: "asset-2",
	})
	if err != nil {
		t.Fatalf("Sign second asset: %v", err)
	}
	if first == second {
		t.Fatal("different linked assets must not share a scope key")
	}
	scope, err := signer.Verify(first)
	if err != nil || scope.AssetID != "asset-1" {
		t.Fatalf("verified asset scope = %#v, err=%v", scope, err)
	}
}

func TestAgentGuardPanoramaNodeIsScopedAndExpires(t *testing.T) {
	signer, err := NewAgentGuardScopeSigner(strings.Repeat("n", 32))
	if err != nil {
		t.Fatalf("NewAgentGuardScopeSigner: %v", err)
	}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	signer.now = func() time.Time { return now }

	token, err := signer.SignPanoramaNode(AgentGuardPanoramaNodeRef{
		NodeType:   "process",
		ObjectID:   "host-1:4100:999",
		HostID:     "host-1",
		AssetID:    "asset-1",
		InstanceID: "instance-1",
	}, time.Minute)
	if err != nil {
		t.Fatalf("SignPanoramaNode: %v", err)
	}
	ref, err := signer.VerifyPanoramaNode(token)
	if err != nil {
		t.Fatalf("VerifyPanoramaNode: %v", err)
	}
	if ref.HostID != "host-1" || ref.InstanceID != "instance-1" || ref.NodeType != "process" {
		t.Fatalf("unexpected node ref: %#v", ref)
	}

	signer.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := signer.VerifyPanoramaNode(token); err == nil {
		t.Fatal("expired panorama node token must be rejected")
	}
}

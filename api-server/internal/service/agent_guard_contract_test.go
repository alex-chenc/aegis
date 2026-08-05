package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const agentGuardContractHostID = "62000000-0000-4000-8000-00000000c001"

func TestExportAgentGuardBundleContract(t *testing.T) {
	hostID := uuid.MustParse(agentGuardContractHostID)
	generatedAt := time.Date(2026, time.July, 30, 10, 0, 0, 123456000, time.UTC)
	policy := model.AgentGuardPolicy{
		PolicyKey:            "contract-monitor-only",
		Version:              1,
		Priority:             100,
		Targets:              datatypes.JSON([]byte(`{"host_ids":["` + hostID.String() + `"]}`)),
		CollectionPolicy:     datatypes.JSON([]byte(`{"behavior_monitor_enabled":true}`)),
		BuiltinRuleOverrides: datatypes.JSON([]byte(`[]`)),
		AtomicRules:          datatypes.JSON([]byte(`[]`)),
		CorrelationRules:     datatypes.JSON([]byte(`[]`)),
		AnalysisPolicy:       datatypes.JSON([]byte(`{}`)),
		EscapeRules:          datatypes.JSON([]byte(`[]`)),
		CompiledPreview:      datatypes.JSON([]byte(`{"mode":"monitor_only"}`)),
		Digest:               "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	bundle, payload, err := buildAgentGuardBundle(
		hostID,
		6201,
		generatedAt,
		model.BuiltinAgentGuardProfileManifest(),
		model.BuiltinAgentBehaviorRuleManifest(),
		[]model.AgentGuardPolicy{policy},
		false,
	)
	if err != nil {
		t.Fatalf("buildAgentGuardBundle: %v", err)
	}
	if bundle.Schema != AgentGuardBundleSchema ||
		bundle.HostID != hostID.String() ||
		bundle.BundleVersion != 6201 ||
		len(bundle.Profiles) != 7 ||
		len(bundle.BuiltinRules) != 5 ||
		len(bundle.Policies) != 1 ||
		bundle.Defaults.Mode != "monitor_only" {
		t.Fatalf("incomplete cross-service bundle: %#v", bundle)
	}
	if got := canonicalBundleDigest(t, payload); got != bundle.Digest {
		t.Fatalf("bundle digest = %s, canonical digest = %s", bundle.Digest, got)
	}

	if output := os.Getenv("AEGIS_AGENT_GUARD_CONTRACT_BUNDLE_OUT"); output != "" {
		if err := os.WriteFile(output, []byte(payload), 0o600); err != nil {
			t.Fatalf("write contract bundle: %v", err)
		}
	}
}

func canonicalBundleDigest(t *testing.T, payload string) string {
	t.Helper()
	var canonical map[string]any
	if err := json.Unmarshal([]byte(payload), &canonical); err != nil {
		t.Fatalf("decode bundle for canonical digest: %v", err)
	}
	delete(canonical, "digest")
	data, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("canonicalize bundle: %v", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

package pipeline

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestAgentConfigStatusCrossContract(t *testing.T) {
	hostID := uuid.MustParse("62000000-0000-4000-8000-00000000c001")
	for _, fixture := range []struct {
		name       string
		key        string
		wantStatus string
		wantError  bool
	}{
		{name: "applied", key: "AEGIS_AGENT_GUARD_APPLIED_STATUS", wantStatus: "applied"},
		{name: "rejected", key: "AEGIS_AGENT_GUARD_REJECTED_STATUS", wantStatus: "failed", wantError: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := os.Getenv(fixture.key)
			if path == "" {
				t.Skipf("%s is not set", fixture.key)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read Agent status fixture: %v", err)
			}
			var identity struct {
				BundleVersion int64  `json:"bundle_version"`
				Digest        string `json:"digest"`
				ErrorCode     string `json:"error_code"`
			}
			if err := json.Unmarshal(raw, &identity); err != nil {
				t.Fatalf("decode Agent status identity: %v", err)
			}
			projection, err := NormalizeAgentGuardState(
				"agent_guard_config_status",
				hostID,
				string(raw),
			)
			if err != nil {
				t.Fatalf("DC rejected Agent status fixture: %v", err)
			}
			if projection.Delivery == nil ||
				projection.Delivery.HostID != hostID ||
				projection.Delivery.BundleVersion != identity.BundleVersion ||
				projection.Delivery.BundleDigest != identity.Digest ||
				projection.Delivery.Status != fixture.wantStatus ||
				projection.Delivery.ErrorCode != identity.ErrorCode {
				t.Fatalf("DC changed Agent status contract: %#v", projection.Delivery)
			}
			if fixture.wantError && projection.Delivery.ErrorCode == "" {
				t.Fatalf("DC lost rejected status error code: %#v", projection.Delivery)
			}
		})
	}
}

package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeAgentGuardLifecycleState(t *testing.T) {
	hostID := uuid.New()
	assetID := uuid.NewString()
	instanceID := uuid.NewString()
	unitID := uuid.NewString()
	sessionID := uuid.NewString()
	tests := []struct {
		eventType string
		raw       string
		wantID    string
	}{
		{
			eventType: "agent_instance_started",
			wantID:    instanceID,
			raw:       `{"schema":"aegis.agent_guard.v1","instance_id":"` + instanceID + `","asset_id":"` + assetID + `","profile_key":"codex-linux","profile_version":1,"agent_type":"codex","display_name":"Codex","controller_pid":10,"controller_start_ticks":99,"controller_exe":"/opt/token=controller-secret/codex","run_uid":1000,"detection_confidence":"confirmed","status":"running","coverage_level":"monitor_only","coverage_reasons":["p1"],"first_seen_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
		},
		{
			eventType: "agent_execution_unit_started",
			wantID:    unitID,
			raw:       `{"schema":"aegis.agent_guard.v1","execution_unit_id":"` + unitID + `","instance_id":"` + instanceID + `","unit_type":"local_process_tree","fingerprint":"unit-fingerprint","root_pid":10,"root_start_ticks":99,"coverage_level":"no_isolation","coverage_reasons":["local_process_tree"],"status":"observed","first_seen_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
		},
		{
			eventType: "agent_behavior_session_started",
			wantID:    sessionID,
			raw:       `{"schema":"aegis.agent_guard.v1","session_id":"` + sessionID + `","instance_id":"` + instanceID + `","execution_unit_id":"` + unitID + `","source":"activity_window","confidence":"inferred","status":"active","completeness":{},"started_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.eventType, func(t *testing.T) {
			got, err := NormalizeAgentGuardState(test.eventType, hostID, test.raw)
			if err != nil {
				t.Fatalf("NormalizeAgentGuardState: %v", err)
			}
			if got.ObjectID.String() != test.wantID {
				t.Fatalf("object ID = %s, want %s", got.ObjectID, test.wantID)
			}
			if got.Instance != nil && strings.Contains(got.Instance.ControllerExe, "controller-secret") {
				t.Fatalf("instance controller path leaked secret: %s", got.Instance.ControllerExe)
			}
			if got.Instance != nil && (got.Instance.AssetID == nil || got.Instance.AssetID.String() != assetID) {
				t.Fatalf("instance asset ID = %v, want %s", got.Instance.AssetID, assetID)
			}
		})
	}
}

func TestNormalizeAgentGuardConfigStatus(t *testing.T) {
	hostID := uuid.New()
	got, err := NormalizeAgentGuardState(
		"agent_guard_config_status",
		hostID,
		`{"schema":"aegis.agent_guard.v1","status":"applied","bundle_version":7,"digest":"sha256:test","occurred_at":"2026-07-30T10:00:00Z"}`,
	)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardState: %v", err)
	}
	if got.Delivery == nil || got.Delivery.Status != "applied" || got.Delivery.BundleVersion != 7 {
		t.Fatalf("delivery projection = %#v", got.Delivery)
	}
}

func TestNormalizeAgentGuardStateRejectsInvalidLifecycle(t *testing.T) {
	_, err := NormalizeAgentGuardState(
		"agent_execution_unit_started",
		uuid.New(),
		`{"schema":"aegis.agent_guard.v1","execution_unit_id":"not-a-uuid"}`,
	)
	if err == nil {
		t.Fatal("expected lifecycle validation error")
	}
}

func TestNormalizeExecutionUnitPreservesIsolationCapabilityAndCompleteness(t *testing.T) {
	instanceID := uuid.NewString()
	unitID := uuid.NewString()
	projection, err := NormalizeAgentGuardState(
		"agent_execution_unit_updated",
		uuid.New(),
		`{"schema":"aegis.agent_guard.v1","execution_unit_id":"`+unitID+
			`","instance_id":"`+instanceID+`","unit_type":"linux_namespace","fingerprint":"unit",`+
			`"coverage_level":"monitor_only","coverage_reasons":[],"status":"healthy",`+
			`"isolation_baseline":{"namespace_inodes":{"mnt":10},"token":"must-redact"},`+
			`"isolation_actual":{"namespace_inodes":{"mnt":11}},`+
			`"isolation_diff":{"state_changed":true,"changes":{"mnt":{"before":10,"after":11}}},`+
			`"capabilities":{"bpf_lsm":false,"supported_hooks":["sys_enter_setns"]},`+
			`"completeness":"partial",`+
			`"first_seen_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
	)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardState: %v", err)
	}
	var baseline, actual, diff map[string]any
	if json.Unmarshal(projection.Unit.IsolationBaseline, &baseline) != nil ||
		json.Unmarshal(projection.Unit.IsolationActual, &actual) != nil ||
		json.Unmarshal(projection.Unit.IsolationDiff, &diff) != nil {
		t.Fatal("invalid isolation projection JSON")
	}
	if baseline["token"] != redactedValue || actual["capabilities"] == nil ||
		actual["completeness"] != "partial" || diff["state_changed"] != true {
		t.Fatalf("baseline=%v actual=%v diff=%v", baseline, actual, diff)
	}
}

func TestNormalizeExecutionUnitPreservesP3EnforcementCoverage(t *testing.T) {
	instanceID, unitID := uuid.NewString(), uuid.NewString()
	projection, err := NormalizeAgentGuardState(
		"agent_execution_unit_updated",
		uuid.New(),
		`{"schema":"aegis.agent_guard.v1","execution_unit_id":"`+unitID+
			`","instance_id":"`+instanceID+`","unit_type":"linux_namespace","fingerprint":"unit",`+
			`"coverage_level":"full_enforcement","coverage_reasons":["bpf_lsm_active"],"status":"healthy",`+
			`"first_seen_at":"2026-07-30T10:00:00Z","last_seen_at":"2026-07-30T10:00:01Z"}`,
	)
	if err != nil {
		t.Fatalf("NormalizeAgentGuardState: %v", err)
	}
	if projection.Unit == nil || projection.Unit.CoverageLevel != "full_enforcement" {
		t.Fatalf("unit=%#v", projection.Unit)
	}
}

func TestNormalizeRemoteUnitAndTrustedSessionStayEvidenceLimited(t *testing.T) {
	hostID := uuid.New()
	instanceID, unitID, sessionID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	unit, err := NormalizeAgentGuardState(
		"agent_execution_unit_started",
		hostID,
		`{"schema":"aegis.agent_guard.v1","execution_unit_id":"`+unitID+
			`","instance_id":"`+instanceID+`","unit_type":"remote_sandbox","fingerprint":"remote-unit",`+
			`"remote_backend":"ssh","remote_execution_id":"job-42","remote_host_ref":"worker.example",`+
			`"coverage_level":"full_enforcement","coverage_reasons":[],"status":"observed",`+
			`"first_seen_at":"2026-08-03T10:00:00Z","last_seen_at":"2026-08-03T10:00:01Z"}`,
	)
	if err != nil {
		t.Fatalf("normalize remote unit: %v", err)
	}
	if unit.Unit.CoverageLevel != remoteUnobservable ||
		!strings.Contains(string(unit.Unit.CoverageReasons), "remote_sensor_unverified") {
		t.Fatalf("remote unit overstated coverage: %#v", unit.Unit)
	}

	session, err := NormalizeAgentGuardState(
		"agent_behavior_session_started",
		hostID,
		`{"schema":"aegis.agent_guard.v1","session_id":"`+sessionID+`","instance_id":"`+instanceID+
			`","execution_unit_id":"`+unitID+`","external_session_id":"official-42",`+
			`"source":"agent_official","confidence":"confirmed","correlation_token_hash":"`+testCorrelationHash+`",`+
			`"status":"active","started_at":"2026-08-03T10:00:00Z","last_seen_at":"2026-08-03T10:00:01Z"}`,
	)
	if err != nil || session.Session == nil || session.Session.ExternalSessionID == nil ||
		session.Session.CorrelationTokenHash == nil || *session.Session.CorrelationTokenHash != testCorrelationHash {
		t.Fatalf("trusted session projection=%#v err=%v", session, err)
	}

	_, err = NormalizeAgentGuardState(
		"agent_behavior_session_started",
		hostID,
		`{"schema":"aegis.agent_guard.v1","session_id":"`+uuid.NewString()+`","instance_id":"`+instanceID+
			`","execution_unit_id":"`+unitID+`","source":"agent_official","confidence":"inferred",`+
			`"status":"active","started_at":"2026-08-03T10:00:00Z","last_seen_at":"2026-08-03T10:00:01Z"}`,
	)
	if err == nil {
		t.Fatal("official source with inferred confidence was accepted")
	}
}

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentGuardActionRepositoryCoalescesConcurrentFreezeAndProtectsTerminalState(t *testing.T) {
	db := setupAgentGuardActionTestDB(t)
	now := time.Now().UTC()
	hostID, instanceID, unitID := uuid.New(), uuid.New(), uuid.New()
	if err := db.Exec(
		`INSERT INTO agent_runtime_instances
		 (id,host_id,profile_key,profile_version,agent_type,controller_pid,controller_start_ticks,
		  detection_confidence,status,coverage_level,first_seen_at,last_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		instanceID, hostID, "codex-linux", 1, "codex", 100, "10", "confirmed", "running",
		model.AgentGuardCoverageFullEnforcement, now, now, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(
		`INSERT INTO agent_execution_units
		 (id,host_id,instance_id,unit_type,fingerprint,coverage_level,status,
		  first_seen_at,last_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		unitID, hostID, instanceID, "linux_cgroup_v2", "unit-1",
		model.AgentGuardCoverageFullEnforcement, "observed", now, now, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewAgentGuardActionRepository(db)
	first := newManualFreezeAction(hostID, instanceID, unitID, now)
	stored, created, err := repo.CreateOrGetActiveFreeze(context.Background(), first)
	if err != nil || !created {
		t.Fatalf("first freeze created=%v err=%v", created, err)
	}
	second := newManualFreezeAction(hostID, instanceID, unitID, now.Add(time.Second))
	second.Action = model.AgentGuardActionHoldExecutionUnit
	second.HoldRequested = true
	second.FreezeTimeoutSeconds = nil
	coalesced, created, err := repo.CreateOrGetActiveFreeze(context.Background(), second)
	if err != nil || created || coalesced.ID != stored.ID {
		t.Fatalf("coalesced=%#v created=%v err=%v, want existing %s", coalesced, created, err, stored.ID)
	}

	completed := now.Add(2 * time.Second)
	result := datatypes.JSON(`{"confirmed":true}`)
	succeeded, err := repo.Transition(
		context.Background(), stored.ID, model.AgentGuardActionStatusSuccess,
		result, "", "", completed,
	)
	if err != nil || succeeded.Status != model.AgentGuardActionStatusSuccess || succeeded.CompletedAt == nil {
		t.Fatalf("success transition=%#v err=%v", succeeded, err)
	}
	if _, err := repo.Transition(
		context.Background(), stored.ID, model.AgentGuardActionStatusRunning,
		nil, "", "", completed.Add(time.Second),
	); !errors.Is(err, ErrAgentGuardActionStateConflict) {
		t.Fatalf("terminal rollback error=%v, want ErrAgentGuardActionStateConflict", err)
	}
	current, err := repo.GetByID(context.Background(), stored.ID)
	if err != nil || current.Status != model.AgentGuardActionStatusSuccess {
		t.Fatalf("terminal action regressed: %#v err=%v", current, err)
	}
}

func newManualFreezeAction(hostID, instanceID, unitID uuid.UUID, now time.Time) *model.AgentGuardAction {
	timeout := 300
	return &model.AgentGuardAction{
		ID: uuid.New(), CommandID: "AG-GUARD-" + uuid.NewString(), HostID: hostID,
		InstanceID: &instanceID, ExecutionUnitID: &unitID,
		Action: model.AgentGuardActionFreezeExecutionUnit, Source: model.AgentGuardActionSourceManual,
		Status: model.AgentGuardActionStatusPending, Reason: "manual containment",
		RequestedBy: "admin", FreezeTimeoutSeconds: &timeout, Result: datatypes.JSON(`{}`),
		RequestedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func setupAgentGuardActionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE agent_runtime_instances (
			id TEXT PRIMARY KEY, host_id TEXT, asset_id TEXT, profile_key TEXT, profile_version INTEGER,
			agent_type TEXT, controller_pid INTEGER, controller_start_ticks TEXT, detection_confidence TEXT,
			status TEXT, coverage_level TEXT, first_seen_at DATETIME, last_seen_at DATETIME,
			stopped_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_execution_units (
			id TEXT PRIMARY KEY, host_id TEXT, instance_id TEXT, unit_type TEXT, fingerprint TEXT,
			remote_backend TEXT, coverage_level TEXT, status TEXT, first_seen_at DATETIME,
			last_seen_at DATETIME, frozen_at DATETIME, stopped_at DATETIME,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_guard_policy_deliveries (
			id TEXT PRIMARY KEY, host_id TEXT, bundle_version INTEGER, capability_snapshot TEXT,
			status TEXT, generated_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_guard_actions (
			id TEXT PRIMARY KEY, command_id TEXT UNIQUE, trigger_behavior_event_id TEXT,
			trigger_finding_id TEXT, host_id TEXT, instance_id TEXT, execution_unit_id TEXT,
			action TEXT, source TEXT, status TEXT, reason TEXT, requested_by TEXT,
			hold_requested BOOLEAN, freeze_timeout_seconds INTEGER, result TEXT,
			error_code TEXT, error_message TEXT, requested_at DATETIME, dispatched_at DATETIME,
			completed_at DATETIME, expires_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentGuardQueryRepositoryFiltersRuntimeData(t *testing.T) {
	db := setupAgentGuardQueryTestDB(t)
	repo := NewAgentGuardQueryRepository(db)
	ctx := context.Background()
	hostID := uuid.New()
	instanceID := uuid.New()
	sessionID := uuid.New()
	unitID := uuid.New()
	eventID := uuid.New()
	findingID := uuid.New()
	analysisID := uuid.New()
	actionID := uuid.New()
	now := time.Now().UTC()

	statements := []string{
		`INSERT INTO agent_runtime_instances
			(id,host_id,profile_key,profile_version,agent_type,display_name,controller_pid,
			 controller_start_ticks,detection_confidence,status,coverage_level,coverage_reasons,
			 first_seen_at,last_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		`INSERT INTO agent_behavior_sessions
			(id,host_id,instance_id,execution_unit_id,source,confidence,status,behavior_count,
			 finding_count,completeness,started_at,last_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		`INSERT INTO agent_execution_units
			(id,host_id,instance_id,unit_type,fingerprint,coverage_level,coverage_reasons,status,
			 first_seen_at,last_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		`INSERT INTO agent_behavior_events
			(id,raw_event_id,host_id,host_boot_id,agent_sequence,instance_id,session_id,
			 execution_unit_id,agent_type,category,operation,outcome,decision,severity,
			 resource_type,resource_identity,occurred_at,received_at,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		`INSERT INTO agent_security_findings
			(id,finding_key,host_id,instance_id,session_id,execution_unit_id,title,severity,
			 verdict,confidence,status,decision_sources,rule_hits,evidence_event_ids,
			 evidence_graph,attack_stages,first_observed_at,last_observed_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		`INSERT INTO agent_security_analysis_runs
			(id,finding_id,attempt,status,prompt_version,input_digest,evidence_event_ids,
			 evidence_summary,output,queued_at,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		`INSERT INTO agent_guard_actions
			(id,host_id,instance_id,execution_unit_id,action,source,status,reason,result,
			 requested_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
	}
	args := [][]any{
		{instanceID, hostID, "codex-linux", 1, "codex", "Codex", 4100, "100",
			"confirmed", "running", "monitor_only", `["bpf_lsm_unavailable"]`,
			now.Add(-time.Hour), now, now, now},
		{sessionID, hostID, instanceID, unitID, "official", "high", "active", 1, 1,
			`{"complete":true}`, now.Add(-time.Minute), now, now, now},
		{unitID, hostID, instanceID, "linux_namespace", "unit-1", "monitor_only", `[]`,
			"observed", now.Add(-time.Minute), now, now, now},
		{eventID, "event-1", hostID, "boot-1", 1, instanceID, sessionID, unitID, "codex",
			"file", "create", "success", "alert", "high", "file", "/tmp/output",
			now, now, now},
		{findingID, "finding-1", hostID, instanceID, sessionID, unitID, "Sensitive write",
			"high", "suspicious", 0.9, "open", `["rule"]`, `["AGB-BUILTIN-001"]`,
			`["event-1"]`, `{}`, `["credential_access"]`, now, now, now, now},
		{analysisID, findingID, 1, model.AgentGuardAnalysisStatusSucceeded, "v1", "sha256:test", `["event-1"]`,
			`{}`, `{"verdict":"suspicious"}`, now, now},
		{actionID, hostID, instanceID, unitID, model.AgentGuardActionFreezeExecutionUnit, "manual", "success", "test", `{}`,
			now, now, now},
	}
	for index, statement := range statements {
		if err := db.Exec(statement, args[index]...).Error; err != nil {
			t.Fatalf("seed query table %d: %v", index, err)
		}
	}
	historicalID := uuid.New()
	if err := db.Exec(`INSERT INTO agent_runtime_instances
		(id,host_id,profile_key,profile_version,agent_type,display_name,controller_pid,
		 controller_start_ticks,detection_confidence,status,coverage_level,coverage_reasons,
		 first_seen_at,last_seen_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, historicalID, hostID, "codex-linux", 1,
		"codex", "Codex", 4090, "90", "confirmed", "running", "monitor_only", `[]`,
		now.Add(-time.Hour), now.Add(-10*time.Minute), now.Add(-time.Hour), now.Add(-10*time.Minute)).Error; err != nil {
		t.Fatalf("seed historical running instance: %v", err)
	}

	instances, total, err := repo.ListInstances(ctx, model.AgentRuntimeInstanceQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		HostID:              hostID.String(),
		AgentTypes:          []string{"codex"},
		Status:              "running",
	})
	if err != nil || total != 1 || len(instances) != 1 || instances[0].ID != instanceID {
		t.Fatalf("ListInstances total=%d items=%#v err=%v", total, instances, err)
	}
	stale, staleTotal, err := repo.ListInstances(ctx, model.AgentRuntimeInstanceQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		HostID:              hostID.String(), AgentTypes: []string{"codex"}, Status: "stale",
	})
	if err != nil || staleTotal != 1 || len(stale) != 1 || stale[0].ID != historicalID {
		t.Fatalf("stale projection total=%d items=%#v err=%v", staleTotal, stale, err)
	}
	if _, err := repo.GetInstance(ctx, instanceID); err != nil {
		t.Fatalf("GetInstance: %v", err)
	}

	behaviors, total, err := repo.ListBehaviors(ctx, model.AgentBehaviorEventQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		InstanceID:          instanceID.String(),
		Category:            "file",
		ResourceKeyword:     "output",
	})
	if err != nil || total != 1 || len(behaviors) != 1 || behaviors[0].ID != eventID {
		t.Fatalf("ListBehaviors total=%d items=%#v err=%v", total, behaviors, err)
	}
	if got, err := repo.GetBehavior(ctx, "event-1"); err != nil || got.ID != eventID {
		t.Fatalf("GetBehavior: got=%#v err=%v", got, err)
	}
	if err := db.Exec(
		`INSERT INTO runtime_events
			(id,event_id,host_id,event_type,event_data,timestamp,created_at,aggregated)
		 VALUES (?,?,?,?,?,?,?,?)`,
		uuid.New(), "event-1", hostID, "agent_behavior", `{"redacted":true}`, now.Unix(), now, false,
	).Error; err != nil {
		t.Fatalf("seed raw runtime event: %v", err)
	}
	if got, err := repo.GetRawBehavior(ctx, "event-1"); err != nil || got.EventID != "event-1" {
		t.Fatalf("GetRawBehavior: got=%#v err=%v", got, err)
	}

	findings, total, err := repo.ListFindings(ctx, model.AgentSecurityFindingQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		InstanceID:          instanceID.String(),
		Severity:            "high",
		Handled:             boolPointer(false),
	})
	if err != nil || total != 1 || len(findings) != 1 || findings[0].ID != findingID {
		t.Fatalf("ListFindings total=%d items=%#v err=%v", total, findings, err)
	}

	analyses, total, err := repo.ListAnalyses(ctx, model.AgentSecurityAnalysisQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		FindingID:           findingID.String(),
		Status:              model.AgentGuardAnalysisStatusSucceeded,
	})
	if err != nil || total != 1 || len(analyses) != 1 || analyses[0].ID != analysisID {
		t.Fatalf("ListAnalyses total=%d items=%#v err=%v", total, analyses, err)
	}

	actions, total, err := repo.ListActions(ctx, model.AgentGuardActionQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		ExecutionUnitID:     unitID.String(),
		Action:              model.AgentGuardActionFreezeExecutionUnit,
	})
	if err != nil || total != 1 || len(actions) != 1 || actions[0].ID != actionID {
		t.Fatalf("ListActions total=%d items=%#v err=%v", total, actions, err)
	}
}

func TestAgentGuardQueryRepositoryTypedNotFound(t *testing.T) {
	repo := NewAgentGuardQueryRepository(setupAgentGuardQueryTestDB(t))
	ctx := context.Background()
	if _, err := repo.GetInstance(ctx, uuid.New()); !errors.Is(err, ErrAgentGuardInstanceNotFound) {
		t.Fatalf("GetInstance error = %v", err)
	}
	if _, err := repo.GetFinding(ctx, uuid.New()); !errors.Is(err, ErrAgentGuardFindingNotFound) {
		t.Fatalf("GetFinding error = %v", err)
	}
	if _, err := repo.GetAction(ctx, uuid.New()); !errors.Is(err, ErrAgentGuardActionNotFound) {
		t.Fatalf("GetAction error = %v", err)
	}
}

func TestAgentGuardQueryRepositoryListsAssetAndConfirmedAssetlessAgents(t *testing.T) {
	db := setupAgentGuardQueryTestDB(t)
	repo := NewAgentGuardQueryRepository(db)
	ctx := context.Background()
	hostID := uuid.New()
	assetID := uuid.New()
	linkedInstanceID := uuid.New()
	orphanInstanceID := uuid.New()
	secondOrphanInstanceID := uuid.New()
	now := time.Now().UTC()

	if err := db.Exec(
		`INSERT INTO hosts (id,ip_address,hostname) VALUES (?,?,?)`,
		hostID, "10.0.0.8", "agent-host",
	).Error; err != nil {
		t.Fatalf("seed host: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO host_application_assets
			(id,host_id,category,name,display_name,runtime_name,status,last_seen_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		assetID, hostID, "ai_agent", "codex", "Codex CLI", "codex", "active", now,
	).Error; err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	for _, row := range []struct {
		id      uuid.UUID
		assetID any
		kind    string
		display string
		pid     int
	}{
		{linkedInstanceID, assetID, "codex", "Codex", 4100},
		{orphanInstanceID, nil, "hermes", "Hermes Worker A", 4200},
		{secondOrphanInstanceID, nil, "hermes", "Hermes Worker B", 4300},
	} {
		if err := db.Exec(
			`INSERT INTO agent_runtime_instances
				(id,host_id,asset_id,profile_key,profile_version,agent_type,display_name,
				 controller_pid,controller_start_ticks,detection_confidence,status,
				 coverage_level,coverage_reasons,first_seen_at,last_seen_at,created_at,updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			row.id, hostID, row.assetID, row.kind+"-linux", 1, row.kind, row.display,
			row.pid, "100", "confirmed", "running", "monitor_only", `[]`,
			now, now, now, now,
		).Error; err != nil {
			t.Fatalf("seed %s instance: %v", row.kind, err)
		}
	}

	items, total, err := repo.ListAgents(ctx, model.AgentGuardAgentQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		HostIDs:             []string{hostID.String()},
	})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("ListAgents total=%d len=%d items=%#v", total, len(items), items)
	}
	var orphan *model.AgentGuardAgentSummary
	for index := range items {
		if items[index].AssetID == nil {
			orphan = &items[index]
		}
	}
	if orphan == nil ||
		orphan.AgentType != "hermes" ||
		orphan.ScopeIdentity == "" ||
		orphan.RunningInstanceCount != 2 {
		t.Fatalf("assetless confirmed Agent missing stable signing identity: %#v", items)
	}
	if orphan.AgentScopeKey != "" {
		t.Fatal("repository must not forge an unsigned agent_scope_key")
	}
}

func TestAgentGuardQueryRepositoryDeduplicatesLogicalAssetsAndMergesAssetlessRuntime(t *testing.T) {
	db := setupAgentGuardQueryTestDB(t)
	repo := NewAgentGuardQueryRepository(db)
	ctx := context.Background()
	hostID := uuid.New()
	activeAssetID := uuid.New()
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO hosts (id,ip_address,hostname) VALUES (?,?,?)`,
		hostID, "10.0.0.9", "logical-agent-host").Error; err != nil {
		t.Fatal(err)
	}
	assets := []struct {
		id, name, status string
		lastSeen         time.Time
	}{
		{uuid.NewString(), "openai_codex", "deleted", now.Add(-time.Hour)},
		{activeAssetID.String(), "codex", "active", now},
		{uuid.NewString(), "openai_codex", "active", now.Add(-time.Minute)},
	}
	for _, asset := range assets {
		if err := db.Exec(`INSERT INTO host_application_assets
			(id,host_id,category,name,display_name,runtime_name,status,last_seen_at)
			VALUES (?,?,?,?,?,?,?,?)`, asset.id, hostID, "ai_agent", asset.name,
			"OpenAI Codex", "", asset.status, asset.lastSeen).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO agent_runtime_instances
		(id,host_id,asset_id,profile_key,profile_version,agent_type,display_name,
		 controller_pid,controller_start_ticks,detection_confidence,status,
		 coverage_level,coverage_reasons,first_seen_at,last_seen_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, uuid.New(), hostID, nil,
		"codex-linux", 1, "codex", "Codex", 6076, "3634", "confirmed", "running",
		"monitor_only", `[]`, now, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO agent_runtime_instances
		(id,host_id,asset_id,profile_key,profile_version,agent_type,display_name,
		 controller_pid,controller_start_ticks,detection_confidence,status,
		 coverage_level,coverage_reasons,first_seen_at,last_seen_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, uuid.New(), hostID, nil,
		"codex-linux", 1, "codex", "Codex", 5076, "2634", "confirmed", "running",
		"monitor_only", `[]`, now.Add(-time.Hour), now.Add(-10*time.Minute), now.Add(-time.Hour), now.Add(-10*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}

	items, total, err := repo.ListAgents(ctx, model.AgentGuardAgentQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		HostIDs:             []string{hostID.String()},
	})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("logical list total=%d len=%d items=%#v", total, len(items), items)
	}
	if items[0].AgentType != "codex" || items[0].RuntimeStatus != "running" ||
		items[0].RunningInstanceCount != 1 || items[0].ProfileKey != "codex-linux" {
		t.Fatalf("logical Codex summary not enriched: %#v", items[0])
	}
	if items[0].AssetID == nil || *items[0].AssetID != activeAssetID {
		t.Fatalf("logical Codex did not select latest active asset: %#v", items[0].AssetID)
	}

	runningItems, runningTotal, err := repo.ListAgents(ctx, model.AgentGuardAgentQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 20},
		RuntimeStatus:       "running",
	})
	if err != nil {
		t.Fatalf("ListAgents running filter: %v", err)
	}
	if runningTotal != 1 || len(runningItems) != 1 {
		t.Fatalf("expected assetless runtime to match one logical running agent, total=%d len=%d", runningTotal, len(runningItems))
	}
}

func TestBuiltinAgentGuardProfileKeyDistinguishesStoppedKnownAndUnsupportedAgents(t *testing.T) {
	for agentType, want := range map[string]string{
		"Claude Code":  "claude-code-linux",
		"openai_codex": "codex-linux",
		"OpenCode":     "opencode-linux",
		"ZCode":        "",
	} {
		if got := builtinAgentGuardProfileKey(agentType); got != want {
			t.Fatalf("builtinAgentGuardProfileKey(%q) = %q, want %q", agentType, got, want)
		}
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func setupAgentGuardQueryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE hosts (
			id TEXT PRIMARY KEY, ip_address TEXT, hostname TEXT
		)`,
		`CREATE TABLE host_application_assets (
			id TEXT PRIMARY KEY, host_id TEXT, category TEXT, name TEXT, display_name TEXT,
			runtime_name TEXT, status TEXT, last_seen_at DATETIME
		)`,
		`CREATE TABLE agent_runtime_instances (
			id TEXT PRIMARY KEY, host_id TEXT, asset_id TEXT, profile_key TEXT, profile_version INTEGER,
			agent_type TEXT, display_name TEXT, controller_pid INTEGER, controller_start_ticks TEXT,
			detection_confidence TEXT, status TEXT, coverage_level TEXT, coverage_reasons TEXT,
			first_seen_at DATETIME, last_seen_at DATETIME, stopped_at DATETIME,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_behavior_sessions (
			id TEXT PRIMARY KEY, host_id TEXT, instance_id TEXT, execution_unit_id TEXT,
			external_session_id TEXT, source TEXT, confidence TEXT, status TEXT,
			behavior_count INTEGER, finding_count INTEGER, completeness TEXT,
			started_at DATETIME, last_seen_at DATETIME, ended_at DATETIME,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_execution_units (
			id TEXT PRIMARY KEY, host_id TEXT, instance_id TEXT, unit_type TEXT, fingerprint TEXT,
			container_id TEXT, coverage_level TEXT, coverage_reasons TEXT, status TEXT,
			first_seen_at DATETIME, last_seen_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_behavior_events (
			id TEXT PRIMARY KEY, raw_event_id TEXT, host_id TEXT, host_boot_id TEXT,
			agent_sequence INTEGER, instance_id TEXT, session_id TEXT, execution_unit_id TEXT,
			policy_id TEXT, rule_id TEXT, agent_type TEXT, category TEXT, operation TEXT,
			outcome TEXT, decision TEXT, severity TEXT, resource_type TEXT,
			resource_identity TEXT, resource_classification TEXT, occurred_at DATETIME,
			received_at DATETIME, created_at DATETIME
		)`,
		`CREATE TABLE agent_security_findings (
			id TEXT PRIMARY KEY, finding_key TEXT, host_id TEXT, instance_id TEXT, session_id TEXT,
			execution_unit_id TEXT, title TEXT, severity TEXT, verdict TEXT, confidence REAL,
			status TEXT, decision_sources TEXT, rule_hits TEXT, evidence_event_ids TEXT,
			evidence_graph TEXT, attack_stages TEXT, latest_analysis_id TEXT, handled_at DATETIME,
			first_observed_at DATETIME, last_observed_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_security_analysis_runs (
			id TEXT PRIMARY KEY, finding_id TEXT, attempt INTEGER, status TEXT, provider TEXT,
			model TEXT, prompt_version TEXT, input_digest TEXT, evidence_event_ids TEXT,
			evidence_summary TEXT, output TEXT, queued_at DATETIME, created_at DATETIME
		)`,
		`CREATE TABLE agent_guard_actions (
			id TEXT PRIMARY KEY, host_id TEXT, instance_id TEXT, execution_unit_id TEXT,
			action TEXT, source TEXT, status TEXT, reason TEXT, result TEXT,
			requested_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE agent_guard_policy_deliveries (
			id TEXT PRIMARY KEY, host_id TEXT, bundle_version INTEGER, status TEXT,
			capability_snapshot TEXT, coverage_level TEXT, error_code TEXT, error_message TEXT,
			generated_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE runtime_events (
			id TEXT PRIMARY KEY, event_id TEXT, host_id TEXT, event_type TEXT, event_data TEXT,
			timestamp INTEGER, created_at DATETIME, aggregated BOOLEAN
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create query test schema: %v", err)
		}
	}
	return db
}

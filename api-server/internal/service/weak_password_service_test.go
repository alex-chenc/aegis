package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockAgentClient is a mock implementation of WeakPasswordAgentClient for testing
type MockAgentClient struct {
	GetAgentStatusFunc func(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error)
	ExecuteToolFunc    func(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error)
}

func (m *MockAgentClient) GetAgentStatus(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
	if m.GetAgentStatusFunc != nil {
		return m.GetAgentStatusFunc(ctx, hostID)
	}
	return &pb.GetAgentStatusResponse{Connected: true}, nil
}

func (m *MockAgentClient) ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error) {
	if m.ExecuteToolFunc != nil {
		return m.ExecuteToolFunc(ctx, callID, hostID, tool, arguments, timeoutSeconds)
	}
	return &pb.ToolExecuteResponse{Success: true}, nil
}

func TestWeakPasswordDefaultDictionarySeeds1000Entries(t *testing.T) {
	svc := newWeakPasswordTestService(t)

	if err := svc.EnsureDefaultDictionary(t.Context()); err != nil {
		t.Fatalf("EnsureDefaultDictionary returned error: %v", err)
	}

	dict, err := svc.repo.GetDefaultDictionary()
	if err != nil {
		t.Fatalf("GetDefaultDictionary returned error: %v", err)
	}
	if dict.EntryCount != 1000 {
		t.Fatalf("default dictionary entry count = %d, want 1000", dict.EntryCount)
	}
}

func TestWeakPasswordPlaintextMatchUsesDefaultDictionary(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	if err := svc.EnsureDefaultDictionary(t.Context()); err != nil {
		t.Fatal(err)
	}

	taskID := uuid.New()
	scanAppID := uuid.New()
	hostID := uuid.New()
	findings, err := svc.MatchCredentialRecords(taskID, scanAppID, hostID, []AgentCredentialRecord{{
		Application:     "redis",
		Account:         "default",
		CredentialType:  "plaintext",
		CredentialValue: "Admin@123",
		SourcePath:      "/etc/redis/redis.conf",
		FieldPath:       "requirepass",
		Parser:          "line_key_value",
	}})
	if err != nil {
		t.Fatalf("MatchCredentialRecords returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].MatchedPasswordMask == "Admin@123" || findings[0].MatchedPasswordMask == "" {
		t.Fatalf("expected masked password, got %q", findings[0].MatchedPasswordMask)
	}
	if findings[0].MatchedPasswordMask != "*********" {
		t.Fatalf("masked password = %q, want all-star mask", findings[0].MatchedPasswordMask)
	}
	if len(findings[0].MatchedPasswordEncrypted) == 0 {
		t.Fatalf("expected encrypted matched password to be stored")
	}
}

func TestWeakPasswordBcryptHashMatchRequiresVerifier(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	if err := svc.EnsureDefaultDictionary(t.Context()); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("Admin@123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	findings, err := svc.MatchCredentialRecords(uuid.New(), uuid.New(), uuid.New(), []AgentCredentialRecord{{
		Application:     "nginx",
		Account:         "admin",
		CredentialType:  "salted_hash",
		CredentialValue: string(hash),
		SourcePath:      "/etc/nginx/.htpasswd",
		FieldPath:       "htpasswd.password",
		Parser:          "htpasswd",
	}})
	if err != nil {
		t.Fatalf("MatchCredentialRecords returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].MatchRule != "server_verifier" {
		t.Fatalf("match rule = %q, want server_verifier", findings[0].MatchRule)
	}
}

func TestWeakPasswordMatchUsesSelectedCustomDictionary(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	if err := svc.EnsureDefaultDictionary(t.Context()); err != nil {
		t.Fatal(err)
	}
	custom, err := svc.CreateDictionary(CreateWeakPasswordDictionaryRequest{
		Name:           "自定义弱密码字典",
		DictionaryType: model.DictTypeUploaded,
		Entries:        []string{"OnlyCustom@123"},
		Source:         "uploaded",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	findings, err := svc.MatchCredentialRecordsWithPolicy(uuid.New(), uuid.New(), uuid.New(), []AgentCredentialRecord{{
		Application:     "redis",
		Account:         "default",
		CredentialType:  model.CredTypePlaintext,
		CredentialValue: "OnlyCustom@123",
		SourcePath:      "/etc/redis/redis.conf",
		FieldPath:       "requirepass",
		Parser:          "line_key_value",
	}}, model.WeakPasswordDictionaryPolicy{
		DictionaryIDs: []string{custom.ID},
	})
	if err != nil {
		t.Fatalf("MatchCredentialRecordsWithPolicy returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].MatchSource != "selected_dictionary" {
		t.Fatalf("match source = %q, want selected_dictionary", findings[0].MatchSource)
	}

	findings, err = svc.MatchCredentialRecordsWithPolicy(uuid.New(), uuid.New(), uuid.New(), []AgentCredentialRecord{{
		Application:     "redis",
		Account:         "default",
		CredentialType:  model.CredTypePlaintext,
		CredentialValue: "Admin@123",
		SourcePath:      "/etc/redis/redis.conf",
		FieldPath:       "requirepass",
		Parser:          "line_key_value",
	}}, model.WeakPasswordDictionaryPolicy{
		DictionaryIDs: []string{custom.ID},
	})
	if err != nil {
		t.Fatalf("MatchCredentialRecordsWithPolicy returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %d, want 0 when default dictionary is not selected", len(findings))
	}
}

func TestGenerateAIDictionaryUsesNaturalLanguageSeeds(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	summary, err := svc.GenerateAIDictionary(model.AIGenerateDictionaryRequest{
		NaturalLanguage: "为 Redis 管理员生成弱密码字典，包含公司名 aegis 和 admin 习惯",
		Count:           20,
	}, nil)
	if err != nil {
		t.Fatalf("GenerateAIDictionary returned error: %v", err)
	}
	if summary.Type != model.DictTypeAIGenerated {
		t.Fatalf("dictionary type = %q, want ai_generated", summary.Type)
	}
	entries, err := svc.repo.ListDictionaryEntries([]uuid.UUID{uuid.MustParse(summary.ID)})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, entry := range entries {
		joined += entry.Candidate + "\n"
	}
	if !strings.Contains(strings.ToLower(joined), "aegis") {
		t.Fatalf("generated entries do not include natural language seed, entries=%q", joined)
	}
}

func TestWeakPasswordRevealRequiresSystemPassword(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	userID := uuid.New()
	hash, err := bcrypt.GenerateFromPassword([]byte("Admin@123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB().Exec(
		`INSERT INTO auth_users (id, username, password_hash, force_password_change, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID.String(), "admin", string(hash), false, time.Now(), time.Now(),
	).Error; err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptWeakPassword("Admin@123")
	if err != nil {
		t.Fatal(err)
	}
	findingID := uuid.New()
	if err := svc.repo.DB().Create(&model.WeakPasswordFinding{
		ID:                       findingID,
		TaskID:                   uuid.New(),
		HostID:                   uuid.New(),
		ApplicationName:          "redis",
		ApplicationType:          "redis",
		Account:                  "default",
		CredentialType:           model.CredTypePlaintext,
		MatchStatus:              model.MatchStatusConfirmed,
		MatchedPasswordMask:      "*********",
		MatchedPasswordEncrypted: encrypted,
		MatchSource:              "default_1000",
		MatchRule:                "dictionary_exact",
		SourcePath:               "/etc/redis/redis.conf",
		FieldPath:                "requirepass",
	}).Error; err != nil {
		t.Fatal(err)
	}

	revealed, err := svc.RevealFinding(findingID, userID, "Admin@123")
	if err != nil {
		t.Fatalf("RevealFinding returned error: %v", err)
	}
	if revealed.MatchedPassword != "Admin@123" {
		t.Fatalf("revealed password = %q, want Admin@123", revealed.MatchedPassword)
	}

	if _, err := svc.RevealFinding(findingID, userID, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
	}
}

func TestCreateTasksByApplicationsSkipsOfflineHosts(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	hostID := uuid.New()
	candidateID := uuid.New()
	if err := svc.repo.DB().Create(&model.WeakPasswordCandidateApplication{
		ID:                 candidateID,
		AnalysisID:         uuid.New(),
		HostID:             hostID,
		ApplicationName:    "redis",
		ApplicationType:    "redis",
		CredentialTypes:    datatypesJSON(t, []string{model.CredTypePlaintext}),
		CandidatePathsJSON: datatypesJSON(t, []string{"/etc/redis/redis.conf"}),
		ExtractorPlanJSON:  datatypesJSON(t, []CredentialExtractor{{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: model.CredTypePlaintext}}),
		Status:             model.AppStatusCandidate,
		CreatedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc.agentClient = &MockAgentClient{
		GetAgentStatusFunc: func(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
			return &pb.GetAgentStatusResponse{Connected: false}, nil
		},
	}

	resp, err := svc.CreateTasksByApplications(t.Context(), model.CreateTasksByApplicationsRequest{
		CandidateApplicationIDs: []string{candidateID.String()},
		DictionaryPolicy:        model.WeakPasswordDictionaryPolicy{UseDefault1000: true},
		AIPolicy:                model.WeakPasswordAIPolicy{RepairCollectionErrors: true, MaxAgentToolCallsPerApp: 10},
	}, nil)
	if err != nil {
		t.Fatalf("CreateTasksByApplications returned error: %v", err)
	}
	if len(resp.Created) != 0 || len(resp.Skipped) != 1 {
		t.Fatalf("created/skipped = %d/%d, want 0/1", len(resp.Created), len(resp.Skipped))
	}
	if resp.Skipped[0].Reason != "host_offline" {
		t.Fatalf("skip reason = %q, want host_offline", resp.Skipped[0].Reason)
	}
}

func TestListCandidateApplicationsIncludesScanStatusAndFindingSummary(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	candidateID := uuid.New()
	hostID := uuid.New()
	scanAppID := uuid.New()
	taskID := uuid.New()
	if err := svc.repo.DB().Create(&model.WeakPasswordCandidateApplication{
		ID:                 candidateID,
		AnalysisID:         uuid.New(),
		HostID:             hostID,
		ApplicationName:    "redis",
		ApplicationType:    "redis",
		CredentialTypes:    datatypesJSON(t, []string{model.CredTypePlaintext}),
		CandidatePathsJSON: datatypesJSON(t, []string{"/etc/redis/redis.conf"}),
		ExtractorPlanJSON:  datatypesJSON(t, []CredentialExtractor{{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: model.CredTypePlaintext}}),
		AssetEvidenceJSON:  datatypesJSON(t, map[string]interface{}{"hostname": "redis-01", "ip_address": "10.0.0.8"}),
		Status:             model.AppStatusCandidate,
		CreatedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB().Create(&model.WeakPasswordScanApplication{
		ID:                     scanAppID,
		TaskID:                 taskID,
		ScanHostID:             uuid.New(),
		HostID:                 hostID,
		CandidateApplicationID: &candidateID,
		ApplicationName:        "redis",
		ApplicationType:        "redis",
		Status:                 model.AppStatusMatched,
		Progress:               100,
		MatchedFindings:        1,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB().Create(&model.WeakPasswordFinding{
		ID:                  uuid.New(),
		TaskID:              taskID,
		ScanApplicationID:   &scanAppID,
		HostID:              hostID,
		ApplicationName:     "redis",
		ApplicationType:     "redis",
		Account:             "default",
		CredentialType:      model.CredTypePlaintext,
		MatchStatus:         model.MatchStatusConfirmed,
		MatchedPasswordMask: "*********",
		MatchSource:         model.DictTypeDefault1000,
		MatchRule:           "dictionary_exact",
		SourcePath:          "/etc/redis/redis.conf",
		FieldPath:           "requirepass",
		EvidenceJSON:        datatypesJSON(t, map[string]interface{}{"process_pid": 1234}),
		CreatedAt:           time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	items, total, err := svc.ListCandidateApplications("", "", "", "", 1, 20)
	if err != nil {
		t.Fatalf("ListCandidateApplications returned error: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("items/total = %d/%d, want 1/1", len(items), total)
	}
	if items[0].ScanStatus != "alert" || items[0].MatchedFindings != 1 {
		t.Fatalf("status/findings = %s/%d, want alert/1", items[0].ScanStatus, items[0].MatchedFindings)
	}
	if len(items[0].Findings) != 1 || items[0].Findings[0].ProcessPID != 1234 {
		t.Fatalf("finding summary = %#v, want process_pid 1234", items[0].Findings)
	}
}

func TestWeakPasswordAnalysisFiltersOfflineHosts(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	onlineHostID := uuid.New()
	offlineHostID := uuid.New()
	now := time.Now()
	for _, host := range []struct {
		id        uuid.UUID
		ip        string
		hostname  string
		heartbeat time.Time
	}{
		{onlineHostID, "10.0.0.10", "online-host", now},
		{offlineHostID, "10.0.0.11", "offline-host", now.Add(-10 * time.Minute)},
	} {
		if err := svc.repo.DB().Exec(
			`INSERT INTO hosts (id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			host.id.String(), host.ip, host.hostname, "linux", "test", host.heartbeat, now, now,
		).Error; err != nil {
			t.Fatal(err)
		}
		if err := svc.repo.DB().Exec(
			`INSERT INTO host_application_assets (id, host_id, hostname, ip_address, category, name, display_name, version, config_paths, listen_ports, ai_confidence, status, collected_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), host.id.String(), host.hostname, host.ip, "redis", "redis", "redis", "7.2", `["/etc/redis/redis.conf"]`, `[]`, 0.96, "active", now, now, now,
		).Error; err != nil {
			t.Fatal(err)
		}
	}

	resp, err := svc.AnalyzeAssetApplications(t.Context(), model.AnalyzeAssetApplicationsRequest{}, nil)
	if err != nil {
		t.Fatalf("AnalyzeAssetApplications returned error: %v", err)
	}
	if resp.ApplicationAssetCount != 1 || resp.CandidateCount != 1 {
		t.Fatalf("counts = assets:%d candidates:%d, want 1/1", resp.ApplicationAssetCount, resp.CandidateCount)
	}
	if resp.Candidates[0].Hostname != "online-host" {
		t.Fatalf("candidate hostname = %q, want online-host", resp.Candidates[0].Hostname)
	}
}

func newWeakPasswordTestService(t *testing.T) *WeakPasswordService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE weak_password_dictionaries (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			dictionary_type TEXT NOT NULL,
			status TEXT NOT NULL,
			entry_count INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL,
			categories JSON,
			generation_policy_json JSON,
			prompt_summary TEXT,
			llm_model TEXT,
			created_by TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE weak_password_dictionary_entries (
			id TEXT PRIMARY KEY,
			dictionary_id TEXT NOT NULL,
			candidate TEXT NOT NULL,
			candidate_hash TEXT NOT NULL,
			category TEXT,
			rule_source TEXT,
			risk_level TEXT,
			created_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_test_wp_entries_hash ON weak_password_dictionary_entries(dictionary_id, candidate_hash)`,
		`CREATE TABLE auth_users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			force_password_change BOOLEAN,
			last_login_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE hosts (
			id TEXT PRIMARY KEY,
			ip_address TEXT NOT NULL,
			hostname TEXT NOT NULL,
			os_type TEXT NOT NULL,
			agent_version TEXT NOT NULL,
			last_heartbeat_at DATETIME NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE host_application_assets (
			id TEXT PRIMARY KEY,
			host_id TEXT,
			hostname TEXT,
			ip_address TEXT,
			group_name TEXT,
			os_type TEXT,
			category TEXT,
			name TEXT,
			display_name TEXT,
			version TEXT,
			version_source TEXT,
			install_path TEXT,
			start_path TEXT,
			config_paths JSON,
			site_paths JSON,
			domains JSON,
			listen_ports JSON,
			run_user TEXT,
			runtime_name TEXT,
			runtime_version TEXT,
			framework_name TEXT,
			framework_version TEXT,
			related_pids JSON,
			related_packages JSON,
			ai_confidence REAL,
			ai_evidence JSON,
			ai_raw_output JSON,
			manual_overrides JSON,
			review_status TEXT,
			status TEXT,
			fingerprint TEXT,
			first_seen_at DATETIME,
			last_seen_at DATETIME,
			collected_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE weak_password_asset_app_analyses (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			scope_json JSON,
			status TEXT,
			application_asset_count INTEGER,
			candidate_count INTEGER,
			error_code TEXT,
			error_message TEXT,
			llm_model TEXT,
			prompt_summary TEXT,
			created_by TEXT,
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME
		)`,
		`CREATE TABLE weak_password_candidate_applications (
			id TEXT PRIMARY KEY,
			analysis_id TEXT,
			host_id TEXT,
			asset_id TEXT,
			application_name TEXT,
			application_type TEXT,
			application_version TEXT,
			profile_id TEXT,
			confidence REAL,
			credential_types JSON,
			candidate_paths_json JSON,
			extractor_plan_json JSON,
			asset_evidence_json JSON,
			ai_reason TEXT,
			status TEXT,
			ignored_by TEXT,
			ignored_at DATETIME,
			created_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_test_wp_candidates_host_asset_type ON weak_password_candidate_applications(host_id, asset_id, application_type)`,
		`CREATE TABLE weak_password_scan_tasks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			trigger_source TEXT,
			status TEXT,
			progress INTEGER,
			current_stage TEXT,
			scope_json JSON,
			dictionary_policy_json JSON,
			ai_policy_json JSON,
			total_hosts INTEGER,
			total_applications INTEGER,
			matched_findings INTEGER,
			failed_applications INTEGER,
			created_by TEXT,
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE weak_password_scan_hosts (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			host_id TEXT,
			status TEXT,
			agent_status TEXT,
			progress INTEGER,
			current_stage TEXT,
			collected_records INTEGER,
			matched_findings INTEGER,
			failed_applications INTEGER,
			error_code TEXT,
			error_message TEXT,
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE weak_password_scan_applications (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			scan_host_id TEXT,
			host_id TEXT,
			asset_id TEXT,
			candidate_application_id TEXT,
			application_name TEXT,
			application_type TEXT,
			profile_id TEXT,
			status TEXT,
			progress INTEGER,
			current_stage TEXT,
			agent_tool_call_count INTEGER,
			max_agent_tool_calls INTEGER,
			collected_records INTEGER,
			matched_findings INTEGER,
			attempted_paths_json JSON,
			error_code TEXT,
			error_message TEXT,
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE weak_password_collection_plans (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			host_id TEXT,
			candidate_application_id TEXT,
			plan_json JSON,
			llm_analysis_json JSON,
			status TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE weak_password_agent_tool_calls (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			scan_application_id TEXT,
			host_id TEXT,
			call_id TEXT,
			tool_name TEXT,
			arguments_summary_json JSON,
			result_summary_json JSON,
			status TEXT,
			error_code TEXT,
			error_message TEXT,
			execution_time_ms INTEGER,
			created_at DATETIME
		)`,
		`CREATE TABLE weak_password_findings (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			scan_application_id TEXT,
			host_id TEXT NOT NULL,
			asset_id TEXT,
			application_name TEXT NOT NULL,
			application_type TEXT NOT NULL,
			account TEXT NOT NULL,
			credential_type TEXT NOT NULL,
			match_status TEXT NOT NULL,
			matched_password_mask TEXT,
			matched_password_encrypted BLOB,
			match_source TEXT NOT NULL,
			match_rule TEXT NOT NULL,
			dictionary_id TEXT,
			confidence REAL,
			source_path TEXT,
			field_path TEXT,
			evidence_json JSON,
			ai_reason TEXT,
			fixed_at DATETIME,
			false_positive_at DATETIME,
			risk_accepted_at DATETIME,
			created_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_test_wp_findings_dedup ON weak_password_findings(task_id, host_id, source_path, field_path, account)`,
		`CREATE TABLE weak_password_collection_errors (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			scan_application_id TEXT,
			host_id TEXT,
			application_name TEXT,
			source_path TEXT,
			error_code TEXT,
			error_message TEXT,
			agent_tool_call_count INTEGER,
			attempted_paths_json JSON,
			repair_trace_json JSON,
			final_status TEXT,
			created_at DATETIME
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	return &WeakPasswordService{repo: repository.NewWeakPasswordRepository(db), logger: zap.NewNop()}
}

func datatypesJSON(t *testing.T, value interface{}) datatypes.JSON {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return datatypes.JSON(data)
}

// ============================================================================
// Regression tests for agent_not_connected bug fix
// ============================================================================

func TestEnsureHostRuntimeOnline_NilAgentClient_ReturnsError(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	// agentClient is nil by default in test service

	hostID := uuid.New()
	err := svc.ensureHostRuntimeOnline(t.Context(), hostID)

	if err == nil {
		t.Fatal("expected error when agentClient is nil, got nil")
	}
	if !errors.Is(err, ErrWeakPasswordHostOffline) {
		t.Fatalf("expected ErrWeakPasswordHostOffline, got: %v", err)
	}
}

func TestEnsureHostRuntimeOnline_AgentOffline_ReturnsError(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.agentClient = &MockAgentClient{
		GetAgentStatusFunc: func(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
			return &pb.GetAgentStatusResponse{Connected: false}, nil
		},
	}

	hostID := uuid.New()
	err := svc.ensureHostRuntimeOnline(t.Context(), hostID)

	if err == nil {
		t.Fatal("expected error when agent is offline, got nil")
	}
	if !errors.Is(err, ErrWeakPasswordHostOffline) {
		t.Fatalf("expected ErrWeakPasswordHostOffline, got: %v", err)
	}
}

func TestEnsureHostRuntimeOnline_AgentOnline_ReturnsNil(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.agentClient = &MockAgentClient{
		GetAgentStatusFunc: func(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
			return &pb.GetAgentStatusResponse{
				Connected:     true,
				HostId:        hostID,
				Hostname:      "test-host",
				LastHeartbeat: time.Now().Unix(),
			}, nil
		},
	}

	hostID := uuid.New()
	err := svc.ensureHostRuntimeOnline(t.Context(), hostID)

	if err != nil {
		t.Fatalf("expected nil error when agent is online, got: %v", err)
	}
}

func TestEnsureHostRuntimeOnline_GetAgentStatusError_ReturnsError(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.agentClient = &MockAgentClient{
		GetAgentStatusFunc: func(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
			return nil, errors.New("connection refused")
		},
	}

	hostID := uuid.New()
	err := svc.ensureHostRuntimeOnline(t.Context(), hostID)

	if err == nil {
		t.Fatal("expected error when GetAgentStatus fails, got nil")
	}
	if !errors.Is(err, ErrWeakPasswordHostOffline) {
		t.Fatalf("expected ErrWeakPasswordHostOffline, got: %v", err)
	}
}

func TestFilterRuntimeOnlineAssets_FiltersOfflineHosts(t *testing.T) {
	svc := newWeakPasswordTestService(t)

	onlineHostID := uuid.New()
	offlineHostID := uuid.New()

	svc.agentClient = &MockAgentClient{
		GetAgentStatusFunc: func(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
			if hostID == onlineHostID.String() {
				return &pb.GetAgentStatusResponse{Connected: true}, nil
			}
			return &pb.GetAgentStatusResponse{Connected: false}, nil
		},
	}

	assets := []model.HostApplicationAsset{
		{HostID: onlineHostID, Name: "redis"},
		{HostID: offlineHostID, Name: "mysql"},
		{HostID: onlineHostID, Name: "nginx"},
	}

	filtered := svc.filterRuntimeOnlineAssets(t.Context(), assets)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 online assets, got %d", len(filtered))
	}
	for _, asset := range filtered {
		if asset.HostID != onlineHostID {
			t.Fatalf("expected only online host assets, got host_id: %s", asset.HostID)
		}
	}
}

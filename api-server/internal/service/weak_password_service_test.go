package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/GehirnInc/crypt/sha512_crypt"
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

type MockWeakPasswordLLMClient struct {
	Response string
	Err      error
	Calls    int
}

func (m *MockWeakPasswordLLMClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error) {
	m.Calls++
	if m.Err != nil {
		return "", m.Err
	}
	return m.Response, nil
}

type cancelAwareWeakPasswordLLMClient struct {
	Calls int
}

func (m *cancelAwareWeakPasswordLLMClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error) {
	m.Calls++
	<-ctx.Done()
	return "", ctx.Err()
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

func TestWeakPasswordShadowSHA512CryptMatchRequiresVerifier(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	if err := svc.EnsureDefaultDictionary(t.Context()); err != nil {
		t.Fatal(err)
	}
	hash, err := sha512_crypt.New().Generate([]byte("Admin@123"), []byte("$6$saltvalue"))
	if err != nil {
		t.Fatal(err)
	}

	findings, err := svc.MatchCredentialRecords(uuid.New(), uuid.New(), uuid.New(), []AgentCredentialRecord{{
		Application:     "openssh",
		Account:         "root",
		CredentialType:  model.CredTypeSaltedHash,
		CredentialValue: hash,
		Salt:            "saltvalue",
		AlgorithmHint:   "sha512-crypt",
		SourcePath:      "/etc/shadow",
		FieldPath:       "shadow.password",
		Parser:          "shadow",
	}})
	if err != nil {
		t.Fatalf("MatchCredentialRecords returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].ApplicationType != "openssh" || findings[0].MatchRule != "server_verifier" {
		t.Fatalf("unexpected finding: %#v", findings[0])
	}
}

func TestWeakPasswordSkillRegistryAddsFixedPaths(t *testing.T) {
	hostID := uuid.New()
	analysisID := uuid.New()
	redisCandidate := buildCandidateFromAsset(analysisID, model.HostApplicationAsset{
		ID:          uuid.New(),
		HostID:      hostID,
		Name:        "redis-server",
		DisplayName: "Redis",
		RelatedPIDs: datatypesJSON(t, []int{4321}),
	})
	paths := weakJSONStrings(redisCandidate.CandidatePathsJSON)
	if !testContainsString(paths, "/etc/redis/redis.conf") {
		t.Fatalf("redis candidate paths = %#v, want fixed redis config path", paths)
	}
	var extractors []CredentialExtractor
	if err := json.Unmarshal(redisCandidate.ExtractorPlanJSON, &extractors); err != nil {
		t.Fatal(err)
	}
	if len(extractors) < 2 || extractors[0].PasswordSelector != "requirepass" {
		t.Fatalf("redis extractors = %#v, want fixed redis extractors", extractors)
	}

	sshCandidate := buildCandidateFromAsset(analysisID, model.HostApplicationAsset{
		ID:     uuid.New(),
		HostID: hostID,
		Name:   "sshd",
	})
	if sshCandidate.ApplicationType != "openssh" || !testContainsString(weakJSONStrings(sshCandidate.CandidatePathsJSON), "/etc/shadow") {
		t.Fatalf("openssh candidate = %#v paths=%#v", sshCandidate, weakJSONStrings(sshCandidate.CandidatePathsJSON))
	}
}

func TestWeakPasswordAnalysisUpsertReturnsPersistedCandidateID(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	hostID := uuid.New()
	assetID := uuid.New()
	firstID := uuid.New()
	firstAnalysisID := uuid.New()
	firstCandidates := []model.WeakPasswordCandidateApplication{{
		ID:                 firstID,
		AnalysisID:         firstAnalysisID,
		HostID:             hostID,
		AssetID:            &assetID,
		ApplicationName:    "Redis",
		ApplicationType:    "redis",
		ProfileID:          "redis_config_v1",
		Confidence:         0.86,
		CredentialTypes:    datatypesJSON(t, []string{"plaintext"}),
		CandidatePathsJSON: datatypesJSON(t, []string{"/etc/redis/redis.conf"}),
		ExtractorPlanJSON:  datatypesJSON(t, []CredentialExtractor{{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: "plaintext"}}),
		AssetEvidenceJSON:  datatypesJSON(t, map[string]string{"source_table": "host_application_assets"}),
		Status:             model.AppStatusCandidate,
	}}
	if err := svc.repo.CreateAnalysisWithCandidates(&model.WeakPasswordAssetAppAnalysis{
		ID: firstAnalysisID, Status: "completed", ApplicationAssetCount: 1, CandidateCount: 1,
	}, firstCandidates); err != nil {
		t.Fatalf("first CreateAnalysisWithCandidates returned error: %v", err)
	}

	secondAnalysisID := uuid.New()
	secondCandidates := []model.WeakPasswordCandidateApplication{{
		ID:                 uuid.New(),
		AnalysisID:         secondAnalysisID,
		HostID:             hostID,
		AssetID:            &assetID,
		ApplicationName:    "Redis",
		ApplicationType:    "redis",
		ProfileID:          "redis_config_v1",
		Confidence:         0.92,
		CredentialTypes:    datatypesJSON(t, []string{"plaintext"}),
		CandidatePathsJSON: datatypesJSON(t, []string{"/etc/redis/redis.conf"}),
		ExtractorPlanJSON:  datatypesJSON(t, []CredentialExtractor{{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: "plaintext"}}),
		AssetEvidenceJSON:  datatypesJSON(t, map[string]string{"source_table": "host_application_assets"}),
		Status:             model.AppStatusCandidate,
	}}
	if err := svc.repo.CreateAnalysisWithCandidates(&model.WeakPasswordAssetAppAnalysis{
		ID: secondAnalysisID, Status: "completed", ApplicationAssetCount: 1, CandidateCount: 1,
	}, secondCandidates); err != nil {
		t.Fatalf("second CreateAnalysisWithCandidates returned error: %v", err)
	}
	if secondCandidates[0].ID != firstID {
		t.Fatalf("upserted candidate ID = %s, want persisted ID %s", secondCandidates[0].ID, firstID)
	}
	if secondCandidates[0].AnalysisID != secondAnalysisID {
		t.Fatalf("upserted candidate analysis_id = %s, want %s", secondCandidates[0].AnalysisID, secondAnalysisID)
	}

	_, err := svc.CreateTaskByApplication(t.Context(), model.CreateTaskByApplicationRequest{
		CandidateApplicationID: secondCandidates[0].ID.String(),
	}, nil)
	if !errors.Is(err, ErrWeakPasswordHostOffline) {
		t.Fatalf("expected persisted candidate lookup to pass and stop at offline runtime check, got %v", err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("candidate lookup used a stale upsert ID: %v", err)
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
	llmClient := &MockWeakPasswordLLMClient{Response: `{"passwords":["Aegis@123","Redis@123","admin123","Aegis@123"]}`}
	svc.SetLLMClient(llmClient)
	summary, err := svc.GenerateAIDictionary(t.Context(), model.AIGenerateDictionaryRequest{
		NaturalLanguage: "为 Redis 管理员生成弱密码字典，包含公司名 aegis 和 admin 习惯",
		Count:           20,
	}, nil)
	if err != nil {
		t.Fatalf("GenerateAIDictionary returned error: %v", err)
	}
	if summary.Type != model.DictTypeAIGenerated {
		t.Fatalf("dictionary type = %q, want ai_generated", summary.Type)
	}
	if llmClient.Calls != 1 {
		t.Fatalf("LLM calls = %d, want 1", llmClient.Calls)
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
	if strings.Contains(summary.LLMModel, "deterministic") {
		t.Fatalf("LLM model = %q, should not use deterministic fallback", summary.LLMModel)
	}
}

func TestGenerateAIDictionaryRequiresLLM(t *testing.T) {
	svc := newWeakPasswordTestService(t)

	_, err := svc.GenerateAIDictionary(t.Context(), model.AIGenerateDictionaryRequest{
		NaturalLanguage: "为 Redis 生成弱密码字典",
		Count:           10,
	}, nil)
	if err == nil {
		t.Fatal("expected GenerateAIDictionary to fail when LLM is unavailable")
	}
	if !strings.Contains(err.Error(), "AI生成密码失败") {
		t.Fatalf("error = %v, want AI generation failure", err)
	}
}

func TestGenerateAIDictionaryDoesNotFallbackWhenLLMFails(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.SetLLMClient(&MockWeakPasswordLLMClient{Err: errors.New("model unavailable")})

	_, err := svc.GenerateAIDictionary(t.Context(), model.AIGenerateDictionaryRequest{
		NaturalLanguage: "为 Redis 生成弱密码字典",
		Count:           10,
	}, nil)
	if err == nil {
		t.Fatal("expected GenerateAIDictionary to fail when LLM call fails")
	}

	var total int64
	if countErr := svc.repo.DB().Model(&model.WeakPasswordDictionary{}).Where("dictionary_type = ?", model.DictTypeAIGenerated).Count(&total).Error; countErr != nil {
		t.Fatal(countErr)
	}
	if total != 0 {
		t.Fatalf("ai_generated dictionaries = %d, want 0 after LLM failure", total)
	}
}

func TestGenerateAIDictionaryHonorsContextCancellation(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	llmClient := &cancelAwareWeakPasswordLLMClient{}
	svc.SetLLMClient(llmClient)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := svc.GenerateAIDictionary(ctx, model.AIGenerateDictionaryRequest{
		NaturalLanguage: "为 Redis 生成弱密码字典",
		Count:           200,
	}, nil)
	if err == nil {
		t.Fatal("expected GenerateAIDictionary to fail when request context is canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if llmClient.Calls != 1 {
		t.Fatalf("LLM calls = %d, want 1", llmClient.Calls)
	}
	var total int64
	if countErr := svc.repo.DB().Model(&model.WeakPasswordDictionary{}).Where("dictionary_type = ?", model.DictTypeAIGenerated).Count(&total).Error; countErr != nil {
		t.Fatal(countErr)
	}
	if total != 0 {
		t.Fatalf("ai_generated dictionaries = %d, want 0 after context cancellation", total)
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

func TestCreateTaskByApplicationNormalizesDetectionRounds(t *testing.T) {
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
			return &pb.GetAgentStatusResponse{Connected: true, HostId: hostID}, nil
		},
		ExecuteToolFunc: func(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error) {
			payload, _ := json.Marshal(AgentCredentialCollectionResult{TaskID: "task", PlanID: "plan", HostID: hostID, Records: []AgentCredentialRecord{}, Errors: []AgentCollectionError{}})
			return &pb.ToolExecuteResponse{Success: true, Result: string(payload)}, nil
		},
	}

	created, err := svc.CreateTaskByApplication(t.Context(), model.CreateTaskByApplicationRequest{
		CandidateApplicationID: candidateID.String(),
		DictionaryPolicy:       model.WeakPasswordDictionaryPolicy{UseDefault1000: true},
		AIPolicy:               model.WeakPasswordAIPolicy{RepairCollectionErrors: true, DetectionRounds: 80},
	}, nil)
	if err != nil {
		t.Fatalf("CreateTaskByApplication returned error: %v", err)
	}
	app, err := svc.repo.GetScanApplicationByTask(uuid.MustParse(created.TaskID))
	if err != nil {
		t.Fatal(err)
	}
	if app.MaxAgentToolCalls != 50 {
		t.Fatalf("max_agent_tool_calls = %d, want clamped 50", app.MaxAgentToolCalls)
	}
	task, err := svc.repo.GetTask(uuid.MustParse(created.TaskID))
	if err != nil {
		t.Fatal(err)
	}
	var policy model.WeakPasswordAIPolicy
	if err := json.Unmarshal(task.AIPolicyJSON, &policy); err != nil {
		t.Fatal(err)
	}
	if policy.DetectionRounds != 50 || policy.MaxAgentToolCallsPerApp != 50 {
		t.Fatalf("ai policy = %#v, want detection rounds 50", policy)
	}
}

func TestListTaskCollectionProgressPaginatesToolCalls(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	taskID := uuid.New()
	scanAppID := uuid.New()
	hostID := uuid.New()
	if err := svc.repo.DB().Create(&model.WeakPasswordScanApplication{
		ID:                 scanAppID,
		TaskID:             taskID,
		ScanHostID:         uuid.New(),
		HostID:             hostID,
		ApplicationName:    "redis",
		ApplicationType:    "redis",
		Status:             model.AppStatusCollecting,
		AgentToolCallCount: 11,
		MaxAgentToolCalls:  50,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 1; i <= 11; i++ {
		call := &model.WeakPasswordAgentToolCall{
			ID:                uuid.New(),
			TaskID:            taskID,
			ScanApplicationID: &scanAppID,
			HostID:            hostID,
			CallID:            "call-" + string(rune('a'+i)),
			ToolName:          "WeakPassword.CollectCredentials",
			Status:            "completed",
			ExecutionTimeMs:   int64(i),
			CreatedAt:         now.Add(time.Duration(i) * time.Second),
		}
		if err := svc.repo.CreateToolCall(call); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := svc.ListTaskCollectionProgress(taskID, 2, 10)
	if err != nil {
		t.Fatalf("ListTaskCollectionProgress returned error: %v", err)
	}
	if total != 11 || len(items) != 1 {
		t.Fatalf("total/len = %d/%d, want 11/1", total, len(items))
	}
	if items[0].Round != 11 || items[0].MaxAgentToolCalls != 50 || items[0].ApplicationName != "redis" {
		t.Fatalf("unexpected progress item: %#v", items[0])
	}
}

func TestListTaskCollectionProgressIncludesFinalFailureSummary(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	taskID := uuid.New()
	scanAppID := uuid.New()
	hostID := uuid.New()
	now := time.Now()
	if err := svc.repo.DB().Create(&model.WeakPasswordScanApplication{
		ID:                 scanAppID,
		TaskID:             taskID,
		ScanHostID:         uuid.New(),
		HostID:             hostID,
		ApplicationName:    "PostgreSQL",
		ApplicationType:    "postgresql",
		Status:             model.AppStatusFailed,
		ErrorCode:          model.ErrCodeConfigDiscoveryFailed,
		ErrorMessage:       "AI 已尝试 10 次受控 Agent 工具调用，仍未定位到有效配置文件",
		AgentToolCallCount: 10,
		MaxAgentToolCalls:  10,
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateToolCall(&model.WeakPasswordAgentToolCall{
		ID:                uuid.New(),
		TaskID:            taskID,
		ScanApplicationID: &scanAppID,
		HostID:            hostID,
		CallID:            "call-collect",
		ToolName:          "WeakPassword.CollectCredentials",
		Status:            "completed",
		ExecutionTimeMs:   20,
		CreatedAt:         now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	items, total, err := svc.ListTaskCollectionProgress(taskID, 1, 10)
	if err != nil {
		t.Fatalf("ListTaskCollectionProgress returned error: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total/len = %d/%d, want 2/2", total, len(items))
	}
	if items[0].ToolName != "WeakPassword.FinalDiagnosis" ||
		items[0].Status != model.AppStatusFailed ||
		items[0].ErrorCode != model.ErrCodeConfigDiscoveryFailed ||
		!strings.Contains(items[0].ErrorMessage, "仍未定位") {
		t.Fatalf("unexpected final summary: %#v", items[0])
	}
	if items[1].Round != 1 || items[1].ToolName != "WeakPassword.CollectCredentials" {
		t.Fatalf("unexpected tool call row: %#v", items[1])
	}
}

func TestAttemptCollectionRepairMergesAIExtractors(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.SetLLMClient(&MockWeakPasswordLLMClient{Response: `{
		"tool":"none",
		"reason":"配置文件可读但字段未命中，补充 Redis masterauth 字段",
		"new_paths":[],
		"new_extractors":[
			{"type":"line_key_value","password_selector":"masterauth","format_hint":"plaintext"},
			{"type":"line_key_value","password_selector":"","format_hint":"plaintext"}
		]
	}`})

	taskID := uuid.New()
	hostID := uuid.New()
	scanHostID := uuid.New()
	scanAppID := uuid.New()
	plan := CredentialCollectionPlan{
		TaskID: taskID.String(),
		PlanID: uuid.New().String(),
		HostID: hostID.String(),
		Applications: []CredentialApplication{{
			Application: "redis",
			ProfileID:   "redis_config_v1",
			Paths:       []string{"/etc/redis/redis.conf"},
			Extractors: []CredentialExtractor{
				{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: model.CredTypePlaintext},
			},
		}},
	}

	repaired, err := svc.attemptCollectionRepair(t.Context(),
		&model.WeakPasswordScanTask{ID: taskID, Name: "weakpass"},
		&model.WeakPasswordScanHost{ID: scanHostID, TaskID: taskID, HostID: hostID},
		&model.WeakPasswordScanApplication{ID: scanAppID, TaskID: taskID, ScanHostID: scanHostID, HostID: hostID, ApplicationName: "redis", ApplicationType: "redis"},
		plan,
		[]AgentCollectionError{{
			Application: "redis",
			SourcePath:  "/etc/redis/redis.conf",
			ErrorCode:   model.ErrCodeFieldNotFound,
			Message:     "password selector requirepass not found",
			Retryable:   true,
		}})
	if err != nil {
		t.Fatalf("attemptCollectionRepair returned error: %v", err)
	}
	if len(repaired.Applications) != 1 {
		t.Fatalf("applications = %d, want 1", len(repaired.Applications))
	}
	extractors := repaired.Applications[0].Extractors
	if len(extractors) != 2 {
		t.Fatalf("extractors = %#v, want original plus valid AI extractor", extractors)
	}
	if extractors[1].PasswordSelector != "masterauth" || extractors[1].Type != "line_key_value" {
		t.Fatalf("unexpected repaired extractors: %#v", extractors)
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

func TestWeakPasswordAnalysisDeduplicatesApplicationsPerHost(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	hostID := uuid.New()
	now := time.Now()
	if err := svc.repo.DB().Exec(
		`INSERT INTO hosts (id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		hostID.String(), "10.0.0.12", "redis-host", "linux", "test", now, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}
	for _, asset := range []struct {
		name       string
		confidence float64
		configJSON string
		pidJSON    string
	}{
		{"redis-server", 0.80, `[]`, `[100]`},
		{"redis", 0.96, `["/etc/redis/redis.conf"]`, `[200]`},
	} {
		if err := svc.repo.DB().Exec(
			`INSERT INTO host_application_assets (id, host_id, hostname, ip_address, category, name, display_name, version, config_paths, listen_ports, related_pids, ai_confidence, status, collected_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), hostID.String(), "redis-host", "10.0.0.12", "database", asset.name, asset.name, "7.2", asset.configJSON, `[6379]`, asset.pidJSON, asset.confidence, "active", now, now, now,
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
	if len(resp.Candidates[0].CandidatePaths) == 0 || resp.Candidates[0].CandidatePaths[0] != "/etc/redis/redis.conf" {
		t.Fatalf("candidate paths = %#v, want preferred config path", resp.Candidates[0].CandidatePaths)
	}
}

func TestWeakPasswordAnalysisFallsBackWhenLLMFails(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.SetLLMClient(&MockWeakPasswordLLMClient{Err: errors.New("model timeout")})
	hostID := uuid.New()
	now := time.Now()
	if err := svc.repo.DB().Exec(
		`INSERT INTO hosts (id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		hostID.String(), "10.0.0.13", "redis-host", "linux", "test", now, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB().Exec(
		`INSERT INTO host_application_assets (id, host_id, hostname, ip_address, category, name, display_name, version, config_paths, listen_ports, ai_confidence, status, collected_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), hostID.String(), "redis-host", "10.0.0.13", "database", "redis", "Redis", "7.2", `["/etc/redis/redis.conf"]`, `[6379]`, 0.96, "active", now, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}

	resp, err := svc.AnalyzeAssetApplications(t.Context(), model.AnalyzeAssetApplicationsRequest{}, nil)
	if err != nil {
		t.Fatalf("AnalyzeAssetApplications returned error: %v", err)
	}
	if resp.CandidateCount != 1 || resp.Candidates[0].ApplicationType != "redis" {
		t.Fatalf("unexpected fallback candidates: %#v", resp.Candidates)
	}
}

func TestAnalyzeApplicationBatchRequiresCompleteCoverage(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.SetLLMClient(&MockWeakPasswordLLMClient{Response: `{"needs":{"redis":"需要"},"skip":{}}`})

	_, err := svc.analyzeApplicationBatch(t.Context(), []string{"redis", "mysql"})
	if err == nil {
		t.Fatal("expected incomplete AI response to fail")
	}
	if !strings.Contains(err.Error(), "missing application types") {
		t.Fatalf("error = %v, want missing application types", err)
	}
}

func TestAnalyzeApplicationBatchNormalizesReturnedTypes(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	svc.SetLLMClient(&MockWeakPasswordLLMClient{Response: `{"needs":{"Redis":"需要"},"skip":{"MySQL":"不需要"}}`})

	results, err := svc.analyzeApplicationBatch(t.Context(), []string{"redis", "mysql"})
	if err != nil {
		t.Fatalf("analyzeApplicationBatch returned error: %v", err)
	}
	if !results["redis"] {
		t.Fatalf("redis result = %v, want true", results["redis"])
	}
	if results["mysql"] {
		t.Fatalf("mysql result = %v, want false", results["mysql"])
	}
}

func TestWeakPasswordTaskUsesProcessHintsAfterFirstConfigMiss(t *testing.T) {
	svc := newWeakPasswordTestService(t)
	if err := svc.EnsureDefaultDictionary(t.Context()); err != nil {
		t.Fatal(err)
	}

	hostID := uuid.New()
	assetID := uuid.New()
	candidateID := uuid.New()
	if err := svc.repo.DB().Create(&model.WeakPasswordCandidateApplication{
		ID:                 candidateID,
		AnalysisID:         uuid.New(),
		HostID:             hostID,
		AssetID:            &assetID,
		ApplicationName:    "redis",
		ApplicationType:    "redis",
		CredentialTypes:    datatypesJSON(t, []string{model.CredTypePlaintext}),
		CandidatePathsJSON: datatypesJSON(t, []string{"/missing/redis.conf"}),
		ExtractorPlanJSON:  datatypesJSON(t, []CredentialExtractor{{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: model.CredTypePlaintext}}),
		AssetEvidenceJSON:  datatypesJSON(t, map[string]interface{}{"hostname": "redis-01", "ip_address": "10.0.0.8", "related_pids": []int{4321}}),
		Status:             model.AppStatusCandidate,
		CreatedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	var collectCalls int32
	var hintCalls int32
	svc.agentClient = &MockAgentClient{
		GetAgentStatusFunc: func(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error) {
			return &pb.GetAgentStatusResponse{Connected: true, HostId: hostID}, nil
		},
		ExecuteToolFunc: func(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error) {
			switch tool {
			case "WeakPassword.CollectCredentials":
				callNo := atomic.AddInt32(&collectCalls, 1)
				if callNo == 1 {
					payload, _ := json.Marshal(AgentCredentialCollectionResult{
						TaskID:  "task",
						PlanID:  "plan",
						HostID:  hostID,
						Records: []AgentCredentialRecord{},
						Errors: []AgentCollectionError{{
							Application: "redis",
							SourcePath:  "/missing/redis.conf",
							ErrorCode:   "file_not_found",
							Message:     "configured file does not exist",
							Retryable:   true,
						}},
					})
					return &pb.ToolExecuteResponse{Success: true, Result: string(payload)}, nil
				}
				if !strings.Contains(arguments, "/etc/redis/redis.conf") {
					return &pb.ToolExecuteResponse{Success: false, Error: "second collection did not include discovered redis config path"}, nil
				}
				payload, _ := json.Marshal(AgentCredentialCollectionResult{
					TaskID: "task",
					PlanID: "plan",
					HostID: hostID,
					Records: []AgentCredentialRecord{{
						Application:     "redis",
						AssetID:         assetID.String(),
						SourcePath:      "/etc/redis/redis.conf",
						SourceKind:      "config_file",
						Account:         "default",
						CredentialType:  model.CredTypePlaintext,
						CredentialValue: "Admin@123",
						FieldPath:       "requirepass",
						Parser:          "line_key_value",
						ProcessPID:      4321,
						Confidence:      0.94,
					}},
					Errors: []AgentCollectionError{},
				})
				return &pb.ToolExecuteResponse{Success: true, Result: string(payload)}, nil
			case "WeakPassword.ProcessConfigHints":
				atomic.AddInt32(&hintCalls, 1)
				var args struct {
					PID         int    `json:"pid"`
					Application string `json:"application"`
				}
				if err := json.Unmarshal([]byte(arguments), &args); err != nil {
					return &pb.ToolExecuteResponse{Success: false, Error: err.Error()}, nil
				}
				if args.PID != 4321 || args.Application != "redis" {
					return &pb.ToolExecuteResponse{Success: false, Error: "unexpected process hint arguments"}, nil
				}
				payload, _ := json.Marshal(AgentProcessConfigHintsResult{
					PID:                  4321,
					ContainerID:          "1234567890ab",
					ContainerRuntime:     "docker",
					ContainerRoot:        "/proc/4321/root",
					ContainerConfigFiles: []string{"/etc/redis/redis.conf"},
					ConfigPathCandidates: []string{"/etc/redis/redis.conf"},
				})
				return &pb.ToolExecuteResponse{Success: true, Result: string(payload)}, nil
			default:
				return &pb.ToolExecuteResponse{Success: false, Error: "unexpected tool " + tool}, nil
			}
		},
	}

	created, err := svc.CreateTaskByApplication(t.Context(), model.CreateTaskByApplicationRequest{
		CandidateApplicationID: candidateID.String(),
		DictionaryPolicy:       model.WeakPasswordDictionaryPolicy{UseDefault1000: true},
		AIPolicy:               model.WeakPasswordAIPolicy{RepairCollectionErrors: true, MaxAgentToolCallsPerApp: 10},
	}, nil)
	if err != nil {
		t.Fatalf("CreateTaskByApplication returned error: %v", err)
	}

	taskID := uuid.MustParse(created.TaskID)
	var task *model.WeakPasswordScanTask
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err = svc.repo.GetTask(taskID)
		if err == nil && (task.Status == model.TaskStatusCompleted || task.Status == model.TaskStatusFailed) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if task == nil {
		t.Fatal("task was not created")
	}
	if task.Status != model.TaskStatusCompleted {
		app, _ := svc.repo.GetScanApplicationByTask(taskID)
		t.Fatalf("task status = %q, want completed; app=%#v", task.Status, app)
	}
	if atomic.LoadInt32(&collectCalls) != 2 || atomic.LoadInt32(&hintCalls) != 1 {
		t.Fatalf("collect/hint calls = %d/%d, want 2/1", collectCalls, hintCalls)
	}

	findings, total, err := svc.ListTaskFindings(taskID, 1, 20)
	if err != nil {
		t.Fatalf("ListTaskFindings returned error: %v", err)
	}
	if total != 1 || len(findings) != 1 {
		t.Fatalf("findings total/len = %d/%d, want 1/1", total, len(findings))
	}
	if findings[0].SourcePath != "/etc/redis/redis.conf" || processPIDFromEvidence(findings[0].EvidenceJSON) != 4321 {
		t.Fatalf("unexpected finding source/evidence: %#v", findings[0])
	}
}

func newWeakPasswordTestService(t *testing.T) *WeakPasswordService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
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

func testContainsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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

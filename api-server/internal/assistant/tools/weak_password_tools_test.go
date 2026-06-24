package tools

import (
	"context"
	"testing"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"

	"github.com/google/uuid"
)

func TestRegisterWeakPasswordToolsExposeAssistantWorkflow(t *testing.T) {
	registry := assistant.NewToolRegistry()
	svc := &fakeWeakPasswordServiceForTools{
		taskID: uuid.New(),
	}
	if err := RegisterWeakPasswordTools(registry, WeakPasswordToolDeps{Service: svc}); err != nil {
		t.Fatalf("RegisterWeakPasswordTools returned error: %v", err)
	}

	for _, name := range []string{
		"Credential.WeakPassword.GenerateDictionary",
		"Credential.WeakPassword.AnalyzeApplications",
		"Credential.WeakPassword.Scan",
		"Credential.WeakPassword.QueryProgress",
		"Credential.WeakPassword.QueryFindings",
		"Credential.WeakPassword.Explain",
	} {
		spec, ok := registry.Get(name)
		if !ok {
			t.Fatalf("expected tool %s to be registered", name)
		}
		if !spec.Enabled {
			t.Fatalf("expected tool %s to be enabled", name)
		}
	}
}

func TestWeakPasswordGenerateDictionaryToolUsesAIService(t *testing.T) {
	registry := assistant.NewToolRegistry()
	svc := &fakeWeakPasswordServiceForTools{}
	if err := RegisterWeakPasswordTools(registry, WeakPasswordToolDeps{Service: svc}); err != nil {
		t.Fatalf("RegisterWeakPasswordTools returned error: %v", err)
	}

	result, err := registry.Execute(context.Background(), "Credential.WeakPassword.GenerateDictionary", map[string]interface{}{
		"natural_language":         "为 Redis 管理员生成弱密码",
		"count":                    5,
		"application_type":         "redis",
		"organization_keywords":    []interface{}{"aegis"},
		"deduplicate_with_default": true,
	})
	if err != nil {
		t.Fatalf("Execute returned dispatcher error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected tool success, got error %s", result.Error)
	}
	if svc.generateReq.NaturalLanguage != "为 Redis 管理员生成弱密码" || svc.generateReq.Count != 5 {
		t.Fatalf("unexpected generate request: %#v", svc.generateReq)
	}
	if svc.generateReq.ApplicationType != "redis" || len(svc.generateReq.OrganizationKeywords) != 1 {
		t.Fatalf("expected AI generation arguments to be forwarded, got %#v", svc.generateReq)
	}
}

func TestWeakPasswordAnalyzeApplicationsAndProgressTools(t *testing.T) {
	taskID := uuid.New()
	registry := assistant.NewToolRegistry()
	svc := &fakeWeakPasswordServiceForTools{taskID: taskID}
	if err := RegisterWeakPasswordTools(registry, WeakPasswordToolDeps{Service: svc}); err != nil {
		t.Fatalf("RegisterWeakPasswordTools returned error: %v", err)
	}

	analysis, err := registry.Execute(context.Background(), "Credential.WeakPassword.AnalyzeApplications", map[string]interface{}{
		"application_types":  []interface{}{"redis", "postgresql"},
		"online_agents_only": false,
	})
	if err != nil {
		t.Fatalf("analysis execute returned dispatcher error: %v", err)
	}
	if !analysis.Success {
		t.Fatalf("expected analysis success, got error %s", analysis.Error)
	}
	if got := svc.analysisReq.Scope.ApplicationTypes; len(got) != 2 || got[0] != "redis" || got[1] != "postgresql" {
		t.Fatalf("unexpected application type filters: %#v", got)
	}

	progress, err := registry.Execute(context.Background(), "Credential.WeakPassword.QueryProgress", map[string]interface{}{
		"task_id": taskID.String(),
	})
	if err != nil {
		t.Fatalf("progress execute returned dispatcher error: %v", err)
	}
	if !progress.Success {
		t.Fatalf("expected progress success, got error %s", progress.Error)
	}
	payload, ok := progress.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected progress payload type: %T", progress.Data)
	}
	if payload["collection_progress_total"] != int64(1) {
		t.Fatalf("unexpected collection progress total: %#v", payload["collection_progress_total"])
	}
}

type fakeWeakPasswordServiceForTools struct {
	taskID      uuid.UUID
	analysisReq model.AnalyzeAssetApplicationsRequest
	generateReq model.AIGenerateDictionaryRequest
}

func (f *fakeWeakPasswordServiceForTools) AnalyzeAssetApplications(ctx context.Context, req model.AnalyzeAssetApplicationsRequest, createdBy *uuid.UUID) (*service.AnalyzeAssetApplicationsResponse, error) {
	_ = ctx
	_ = createdBy
	f.analysisReq = req
	return &service.AnalyzeAssetApplicationsResponse{
		AnalysisID:            uuid.NewString(),
		Status:                "completed",
		ApplicationAssetCount: 1,
		CandidateCount:        1,
		Candidates: []service.WeakPasswordCandidateDTO{{
			CandidateApplicationID: uuid.NewString(),
			ApplicationName:        "Redis",
			ApplicationType:        "redis",
			Confidence:             0.95,
			CandidatePaths:         []string{"/etc/redis/redis.conf"},
			CredentialTypes:        []string{"plaintext"},
		}},
	}, nil
}

func (f *fakeWeakPasswordServiceForTools) CreateTaskByApplication(ctx context.Context, req model.CreateTaskByApplicationRequest, createdBy *uuid.UUID) (*service.CreateTaskByApplicationResponse, error) {
	_ = ctx
	_ = req
	_ = createdBy
	if f.taskID == uuid.Nil {
		f.taskID = uuid.New()
	}
	return &service.CreateTaskByApplicationResponse{
		TaskID:            f.taskID.String(),
		ScanApplicationID: uuid.NewString(),
		Status:            "pending",
	}, nil
}

func (f *fakeWeakPasswordServiceForTools) GenerateAIDictionary(ctx context.Context, req model.AIGenerateDictionaryRequest, createdBy *uuid.UUID) (*service.DictionarySummary, error) {
	_ = ctx
	_ = createdBy
	f.generateReq = req
	return &service.DictionarySummary{
		ID:         uuid.NewString(),
		Name:       "AI 生成弱密码字典",
		Type:       "ai_generated",
		Status:     "enabled",
		EntryCount: req.Count,
		Source:     "ai_generated",
	}, nil
}

func (f *fakeWeakPasswordServiceForTools) GetTaskProgress(taskID uuid.UUID) (*model.TaskProgressResponse, error) {
	return &model.TaskProgressResponse{
		TaskID:       taskID.String(),
		Status:       "collecting_credentials",
		Progress:     40,
		CurrentStage: "dispatch_agent_tool",
		Message:      "dispatch_agent_tool",
	}, nil
}

func (f *fakeWeakPasswordServiceForTools) ListTaskCollectionProgress(taskID uuid.UUID, page, pageSize int) ([]service.WeakPasswordCollectionProgressDTO, int64, error) {
	_ = page
	_ = pageSize
	return []service.WeakPasswordCollectionProgressDTO{{
		ID:              uuid.NewString(),
		TaskID:          taskID.String(),
		HostID:          uuid.NewString(),
		ApplicationName: "Redis",
		ToolName:        "WeakPassword.CollectCredentials",
		Status:          "completed",
		Round:           1,
	}}, 1, nil
}

func (f *fakeWeakPasswordServiceForTools) ListTaskFindings(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordFinding, int64, error) {
	_ = taskID
	_ = page
	_ = pageSize
	return nil, 0, nil
}

package tools

import (
	"context"
	"strings"
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

	analysis, _ := registry.Get("Credential.WeakPassword.AnalyzeApplications")
	if len(analysis.ResultContract.FactBindings) != 1 ||
		analysis.ResultContract.FactBindings[0].IDField != "candidate_application_id" {
		t.Fatalf("analysis tool must expose candidate IDs as bindable facts: %#v", analysis.ResultContract)
	}
	scan, _ := registry.Get("Credential.WeakPassword.Scan")
	if scan.ExecutionContract.Mode != assistant.ToolExecutionAsynchronous ||
		scan.ExecutionContract.CompletionCapability != "weak_password_progress" {
		t.Fatalf("scan tool must declare its async completion contract: %#v", scan.ExecutionContract)
	}
	progress, _ := registry.Get("Credential.WeakPassword.QueryProgress")
	if progress.ResultContract.OperationStatusField != "status" {
		t.Fatalf("progress tool must declare a terminal status contract: %#v", progress.ResultContract)
	}
	for _, terminalStatus := range []string{
		model.TaskStatusCompleted,
		model.TaskStatusPartialFailed,
		model.TaskStatusFailed,
		model.TaskStatusCancelled,
	} {
		if !containsFold(progress.ResultContract.SuccessValues, terminalStatus) {
			t.Fatalf("progress tool must complete observation for terminal status %q: %#v", terminalStatus, progress.ResultContract)
		}
		if containsFold(progress.ResultContract.FailureValues, terminalStatus) {
			t.Fatalf("terminal observed status %q must not be treated as a query failure: %#v", terminalStatus, progress.ResultContract)
		}
	}
	if len(progress.ResultContract.FactBindings) != 1 ||
		progress.ResultContract.FactBindings[0].Kind != "task_resolved" ||
		progress.ResultContract.FactBindings[0].ItemsField != "tasks" {
		t.Fatalf("progress tool must carry task IDs into later automatic polls: %#v", progress.ResultContract)
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
	candidateID := uuid.New()
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

	scan, err := registry.Execute(context.Background(), "Credential.WeakPassword.Scan", map[string]interface{}{
		"candidate_application_ids": []interface{}{candidateID.String()},
	})
	if err != nil {
		t.Fatalf("scan execute returned dispatcher error: %v", err)
	}
	if !scan.Success {
		t.Fatalf("expected batch scan creation success, got error %s", scan.Error)
	}
	if got := svc.batchReq.CandidateApplicationIDs; len(got) != 1 || got[0] != candidateID.String() {
		t.Fatalf("unexpected batch candidate IDs: %#v", got)
	}

	progress, err := registry.Execute(context.Background(), "Credential.WeakPassword.QueryProgress", map[string]interface{}{
		"task_ids": []interface{}{taskID.String()},
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
	if payload["status"] != "running" {
		t.Fatalf("unexpected aggregate progress status: %#v", payload["status"])
	}
	if payload["matched_findings"] != 2 {
		t.Fatalf("aggregate matched findings = %#v, want 2", payload["matched_findings"])
	}
}

func TestWeakPasswordProgressReturnsTerminalBatchSummaryAndFailureDetails(t *testing.T) {
	completedTaskID := uuid.New()
	failedTaskID := uuid.New()
	cancelledTaskID := uuid.New()
	registry := assistant.NewToolRegistry()
	svc := &fakeWeakPasswordServiceForTools{
		progressByTask: map[uuid.UUID]*model.TaskProgressResponse{
			completedTaskID: {
				TaskID:             completedTaskID.String(),
				Status:             model.TaskStatusCompleted,
				Progress:           100,
				CurrentStage:       model.TaskStatusCompleted,
				CurrentApplication: "OpenSSH",
				MatchedFindings:    2,
			},
			failedTaskID: {
				TaskID:             failedTaskID.String(),
				Status:             model.TaskStatusFailed,
				Progress:           100,
				CurrentStage:       model.ErrCodeFieldNotFound,
				CurrentApplication: "Apache Kafka",
				FailedApplications: 1,
				Message:            "未能采集到有效凭据材料",
			},
			cancelledTaskID: {
				TaskID:             cancelledTaskID.String(),
				Status:             model.TaskStatusCancelled,
				Progress:           100,
				CurrentStage:       model.TaskStatusCancelled,
				CurrentApplication: "Redis",
				Message:            "任务已取消",
			},
		},
	}
	if err := RegisterWeakPasswordTools(registry, WeakPasswordToolDeps{Service: svc}); err != nil {
		t.Fatalf("RegisterWeakPasswordTools returned error: %v", err)
	}

	progress, err := registry.Execute(context.Background(), "Credential.WeakPassword.QueryProgress", map[string]interface{}{
		"task_ids": []interface{}{completedTaskID.String(), failedTaskID.String(), cancelledTaskID.String()},
	})
	if err != nil {
		t.Fatalf("progress execute returned dispatcher error: %v", err)
	}
	if !progress.Success {
		t.Fatalf("expected progress query success, got error %s", progress.Error)
	}
	payload, ok := progress.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected progress payload type: %T", progress.Data)
	}
	if payload["status"] != model.TaskStatusPartialFailed {
		t.Fatalf("aggregate status = %#v, want partial_failed", payload["status"])
	}
	for key, want := range map[string]int{
		"task_total":     3,
		"task_completed": 1,
		"task_failed":    2,
		"task_running":   0,
	} {
		if got := payload[key]; got != want {
			t.Fatalf("%s = %#v, want %d", key, got, want)
		}
	}
	if payload["matched_findings"] != 2 {
		t.Fatalf("matched_findings = %#v, want 2", payload["matched_findings"])
	}
	failedTasks, ok := payload["failed_tasks"].([]map[string]interface{})
	if !ok || len(failedTasks) != 2 {
		t.Fatalf("failed_tasks = %#v, want two summaries", payload["failed_tasks"])
	}
	if got := failedTasks[0]["error_code"]; got != model.ErrCodeFieldNotFound {
		t.Fatalf("first failure error_code = %#v", got)
	}
	if got := failedTasks[0]["error_message"]; got != "未能采集到有效凭据材料" {
		t.Fatalf("first failure error_message = %#v", got)
	}
}

type fakeWeakPasswordServiceForTools struct {
	taskID         uuid.UUID
	analysisReq    model.AnalyzeAssetApplicationsRequest
	batchReq       model.CreateTasksByApplicationsRequest
	generateReq    model.AIGenerateDictionaryRequest
	progressByTask map[uuid.UUID]*model.TaskProgressResponse
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

func (f *fakeWeakPasswordServiceForTools) CreateTasksByApplications(ctx context.Context, req model.CreateTasksByApplicationsRequest, createdBy *uuid.UUID) (*service.CreateTasksByApplicationsResponse, error) {
	_ = ctx
	_ = createdBy
	f.batchReq = req
	if f.taskID == uuid.Nil {
		f.taskID = uuid.New()
	}
	created := make([]service.BatchTaskCreatedItem, 0, len(req.CandidateApplicationIDs))
	for _, candidateID := range req.CandidateApplicationIDs {
		created = append(created, service.BatchTaskCreatedItem{
			CandidateApplicationID: candidateID,
			TaskID:                 f.taskID.String(),
			ScanApplicationID:      uuid.NewString(),
			Status:                 model.TaskStatusPending,
		})
	}
	return &service.CreateTasksByApplicationsResponse{Created: created}, nil
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
	if progress := f.progressByTask[taskID]; progress != nil {
		copy := *progress
		return &copy, nil
	}
	return &model.TaskProgressResponse{
		TaskID:          taskID.String(),
		Status:          "collecting_credentials",
		Progress:        40,
		CurrentStage:    "dispatch_agent_tool",
		Message:         "dispatch_agent_tool",
		MatchedFindings: 2,
	}, nil
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
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

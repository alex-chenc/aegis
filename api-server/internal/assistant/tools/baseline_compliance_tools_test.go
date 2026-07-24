package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type baselineTemplateRepoStub struct{ template model.Template }

func (s *baselineTemplateRepoStub) FindAll(int, int) ([]model.Template, error) {
	return []model.Template{s.template}, nil
}
func (s *baselineTemplateRepoStub) FindByID(id uuid.UUID) (*model.Template, error) {
	if id != s.template.ID {
		return nil, gorm.ErrRecordNotFound
	}
	value := s.template
	return &value, nil
}

type baselineRuleRepoStub struct{ rules []model.AegisRule }

func (s *baselineRuleRepoStub) FindByTemplateID(uuid.UUID) ([]model.AegisRule, error) {
	return append([]model.AegisRule(nil), s.rules...), nil
}
func (s *baselineRuleRepoStub) FindByID(id uuid.UUID) (*model.AegisRule, error) {
	for _, rule := range s.rules {
		if rule.ID == id {
			value := rule
			return &value, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

type baselineOperationRepoStub struct{ operation *model.AssistantOperation }

func (s *baselineOperationRepoStub) Create(_ context.Context, operation *model.AssistantOperation) error {
	s.operation = operation
	return nil
}
func (s *baselineOperationRepoStub) FindByID(_ context.Context, id uuid.UUID) (*model.AssistantOperation, error) {
	if s.operation == nil || s.operation.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return s.operation, nil
}
func (s *baselineOperationRepoStub) FindByIdempotencyKey(context.Context, string, string, string) (*model.AssistantOperation, bool, error) {
	return nil, false, nil
}
func (s *baselineOperationRepoStub) ListNonTerminal(context.Context, string, int) ([]model.AssistantOperation, error) {
	if s.operation == nil || s.operation.Terminal {
		return nil, nil
	}
	return []model.AssistantOperation{*s.operation}, nil
}
func (s *baselineOperationRepoStub) Transition(_ context.Context, id uuid.UUID, from []string, to string, result interface{}) (bool, error) {
	if s.operation == nil || s.operation.ID != id || !containsString(from, s.operation.Status) {
		return false, nil
	}
	s.operation.Status = to
	s.operation.Result, _ = json.Marshal(result)
	return true, nil
}
func (s *baselineOperationRepoStub) Update(_ context.Context, id uuid.UUID, status string, result interface{}, errorCode, errorMessage string, terminal bool) error {
	if s.operation == nil || s.operation.ID != id {
		return gorm.ErrRecordNotFound
	}
	s.operation.Status = status
	s.operation.Terminal = terminal
	s.operation.ErrorCode = errorCode
	s.operation.ErrorMessage = errorMessage
	s.operation.Result, _ = json.Marshal(result)
	return nil
}

type baselineTaskGroupRepoStub struct{ tasks []model.TaskLog }

func (s *baselineTaskGroupRepoStub) FindByGroupID(uuid.UUID) ([]model.TaskLog, error) {
	return append([]model.TaskLog(nil), s.tasks...), nil
}

type baselineScriptGenerationStub struct {
	checkCalls int
	fixCalls   int
}

func (s *baselineScriptGenerationStub) BatchGenerateForTemplate(_ context.Context, _ uuid.UUID, scriptType string, _ int) (*service.BatchGenerateResult, error) {
	if scriptType == "FIX" {
		s.fixCalls++
	} else {
		s.checkCalls++
	}
	return &service.BatchGenerateResult{Total: 2, Queued: 2}, nil
}

func (s *baselineScriptGenerationStub) QueueScriptGeneration(uuid.UUID, string) error {
	return nil
}

type baselineTaskServiceStub struct {
	groupRepo *baselineTaskGroupRepoStub
	ruleIDs   []string
	hostIDs   []string
	opts      service.DispatchOptions
}

func (s *baselineTaskServiceStub) CreateAndDispatchTasks(_ context.Context, ruleIDs, hostIDs []string, taskType string, opts *service.DispatchOptions, existingGroupID ...uuid.UUID) (*service.TaskCreateResult, error) {
	s.ruleIDs = append([]string(nil), ruleIDs...)
	s.hostIDs = append([]string(nil), hostIDs...)
	s.opts = *opts
	groupID := existingGroupID[0]
	for _, ruleID := range ruleIDs {
		parsedRuleID := uuid.MustParse(ruleID)
		for _, hostID := range hostIDs {
			s.groupRepo.tasks = append(s.groupRepo.tasks, model.TaskLog{ID: uuid.New(), TaskGroupID: groupID, RuleID: &parsedRuleID, HostID: uuid.MustParse(hostID), TaskType: taskType, Status: "PENDING"})
		}
	}
	expected := len(ruleIDs) * len(hostIDs)
	return &service.TaskCreateResult{TaskGroupID: groupID, ExpectedCount: expected, CreatedCount: expected}, nil
}

func TestBaselineComplianceRunResolvesScopeAndDispatchesEveryRule(t *testing.T) {
	logger.Logger = zap.NewNop()
	db, err := gorm.Open(sqlite.Open("file:baseline_compliance?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createSQLiteHostTable(db); err != nil {
		t.Fatal(err)
	}
	host := model.Host{ID: uuid.New(), IPAddress: "192.168.152.159", Hostname: "baseline-target", OSType: "linux", AgentVersion: "6.1.0", LastHeartbeatAt: time.Now()}
	offlineHost := model.Host{ID: uuid.New(), IPAddress: "192.168.152.160", Hostname: "offline-target", OSType: "linux", AgentVersion: "6.1.0", LastHeartbeatAt: time.Now().Add(-10 * time.Minute)}
	for _, candidate := range []model.Host{host, offlineHost} {
		if err := db.Create(&candidate).Error; err != nil {
			t.Fatal(err)
		}
	}
	template := model.Template{ID: uuid.New(), Name: "cis-ubuntu", DisplayName: "CIS Ubuntu 24.04", Status: "completed", RuleCount: 2}
	checkScript, fixScript := "#!/bin/sh\nexit 0", "#!/bin/sh\nexit 0"
	rules := []model.AegisRule{
		{ID: uuid.New(), TemplateID: template.ID, GeneratedCheckScript: &checkScript, GeneratedFixScript: &fixScript, CheckScriptStatus: "generated", FixScriptStatus: "generated"},
		{ID: uuid.New(), TemplateID: template.ID, GeneratedCheckScript: &checkScript, GeneratedFixScript: &fixScript, CheckScriptStatus: "generated", FixScriptStatus: "generated"},
	}
	operationRepo := &baselineOperationRepoStub{}
	groupRepo := &baselineTaskGroupRepoStub{}
	taskService := &baselineTaskServiceStub{groupRepo: groupRepo}
	deps := BaselineToolDeps{
		TaskService:   taskService,
		TemplateRepo:  &baselineTemplateRepoStub{template: template},
		RuleRepo:      &baselineRuleRepoStub{rules: rules},
		HostRepo:      repository.NewHostRepository(db),
		OperationRepo: operationRepo,
		TaskLogRepo:   groupRepo,
		Logger:        zap.NewNop(),
	}
	registry := assistant.NewToolRegistry()
	if err := registerBaselineComplianceTools(registry, deps); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateModelFacingEnglish(); err != nil {
		t.Fatal(err)
	}

	result, err := registry.Execute(context.Background(), "Baseline.Compliance.Run", map[string]interface{}{
		"target_scope":      "all_online_hosts",
		"template_selector": "CIS Ubuntu 24.04",
		"scope":             "all_rules",
		"remediation":       map[string]interface{}{"enabled": true, "max_rounds": 5.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("workflow failed: %s", result.Error)
	}
	output := result.Data.(map[string]interface{})
	if output["operation_status"] != "running" || output["created_count"] != 2 {
		t.Fatalf("unexpected workflow result: %#v", output)
	}
	if len(taskService.ruleIDs) != 2 || len(taskService.hostIDs) != 1 || taskService.hostIDs[0] != host.ID.String() {
		t.Fatalf("incorrect dispatch scope: rules=%v hosts=%v", taskService.ruleIDs, taskService.hostIDs)
	}
	if !taskService.opts.AutoVerify || taskService.opts.MaxRounds != 5 {
		t.Fatalf("remediation options not propagated: %#v", taskService.opts)
	}
	zero := 0
	for index := range groupRepo.tasks {
		groupRepo.tasks[index].Status = "SUCCESS"
		groupRepo.tasks[index].ExitCode = &zero
	}
	terminalResult, err := registry.Execute(context.Background(), "Operation.Get", map[string]interface{}{
		"operation_id": operationRepo.operation.ID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	terminalOutput := terminalResult.Data.(map[string]interface{})
	if terminalOutput["operation_status"] != "succeeded" || terminalOutput["terminal"] != true || terminalOutput["coverage_complete"] != true {
		t.Fatalf("unexpected terminal workflow result: %#v", terminalOutput)
	}
}

func TestBaselineCompliancePreflightRejectsMissingHostScope(t *testing.T) {
	registry := assistant.NewToolRegistry()
	if err := registerBaselineComplianceTools(registry, BaselineToolDeps{}); err != nil {
		t.Fatal(err)
	}
	filters := assistant.NewToolInvocationFilterChain(registry, zap.NewNop())
	if _, err := filters.Prepare(context.Background(), assistant.ToolInvocationFilterRequest{
		Phase:    assistant.ToolInvocationPhaseCandidate,
		ToolName: "Baseline.Compliance.Run",
		Args: map[string]interface{}{
			"template_selector": "CIS Ubuntu 24.04",
			"scope":             "all_rules",
		},
	}); err == nil {
		t.Fatal("expected missing host scope to be rejected before durable invocation")
	}
}

func TestBaselineOperationWorkerAdvancesWithoutConversationPolling(t *testing.T) {
	template := model.Template{ID: uuid.New(), Name: "cis-ubuntu", Status: "completed", RuleCount: 2}
	checkScript := "#!/bin/sh\nexit 0"
	rules := []model.AegisRule{
		{ID: uuid.New(), TemplateID: template.ID, GeneratedCheckScript: &checkScript, CheckScriptStatus: "generated"},
		{ID: uuid.New(), TemplateID: template.ID, GeneratedCheckScript: &checkScript, CheckScriptStatus: "generated"},
	}
	hostID := uuid.New()
	groupID := uuid.New()
	request, _ := json.Marshal(baselineComplianceRequest{
		HostIDs:     []string{hostID.String()},
		TargetScope: "all_online_hosts",
		TemplateID:  template.ID.String(),
		Scope:       "all_rules",
		MaxRounds:   3,
	})
	operationRepo := &baselineOperationRepoStub{operation: &model.AssistantOperation{
		ID:          uuid.New(),
		Type:        baselineComplianceOperationType,
		Status:      "accepted",
		TaskGroupID: &groupID,
		Request:     request,
	}}
	groupRepo := &baselineTaskGroupRepoStub{}
	taskService := &baselineTaskServiceStub{groupRepo: groupRepo}
	worker := NewBaselineOperationWorker(BaselineToolDeps{
		TaskService:   taskService,
		TemplateRepo:  &baselineTemplateRepoStub{template: template},
		RuleRepo:      &baselineRuleRepoStub{rules: rules},
		OperationRepo: operationRepo,
		TaskLogRepo:   groupRepo,
		Logger:        zap.NewNop(),
	})

	worker.runOnce(context.Background())

	if operationRepo.operation.Status != "running" {
		t.Fatalf("operation status = %q, want running", operationRepo.operation.Status)
	}
	if len(groupRepo.tasks) != 2 {
		t.Fatalf("worker dispatched %d tasks, want 2", len(groupRepo.tasks))
	}
}

func TestBaselineOperationWorkerRefillsScriptQueueWhilePreparing(t *testing.T) {
	template := model.Template{ID: uuid.New(), Name: "cis-ubuntu", Status: "completed", RuleCount: 2}
	rules := []model.AegisRule{
		{ID: uuid.New(), TemplateID: template.ID, CheckScriptStatus: "pending", FixScriptStatus: "pending"},
		{ID: uuid.New(), TemplateID: template.ID, CheckScriptStatus: "pending", FixScriptStatus: "pending"},
	}
	groupID := uuid.New()
	request, _ := json.Marshal(baselineComplianceRequest{
		HostIDs:     []string{uuid.NewString()},
		TargetScope: "all_online_hosts",
		TemplateID:  template.ID.String(),
		Scope:       "all_rules",
		Remediation: true,
		MaxRounds:   5,
	})
	operationRepo := &baselineOperationRepoStub{operation: &model.AssistantOperation{
		ID:          uuid.New(),
		Type:        baselineComplianceOperationType,
		Status:      "preparing_scripts",
		TaskGroupID: &groupID,
		Request:     request,
	}}
	scriptGeneration := &baselineScriptGenerationStub{}
	worker := NewBaselineOperationWorker(BaselineToolDeps{
		TemplateRepo:     &baselineTemplateRepoStub{template: template},
		RuleRepo:         &baselineRuleRepoStub{rules: rules},
		ScriptGenService: scriptGeneration,
		OperationRepo:    operationRepo,
		Logger:           zap.NewNop(),
	})

	worker.runOnce(context.Background())

	if scriptGeneration.checkCalls != 1 || scriptGeneration.fixCalls != 1 {
		t.Fatalf("script queue refill calls: check=%d fix=%d", scriptGeneration.checkCalls, scriptGeneration.fixCalls)
	}
	if operationRepo.operation.Status != "preparing_scripts" {
		t.Fatalf("operation status = %q, want preparing_scripts", operationRepo.operation.Status)
	}
}

func TestBaselineOperationWorkerFailsStaleOperationWithoutDispatch(t *testing.T) {
	groupID := uuid.New()
	operationRepo := &baselineOperationRepoStub{operation: &model.AssistantOperation{
		ID:          uuid.New(),
		Type:        baselineComplianceOperationType,
		Status:      "preparing_scripts",
		TaskGroupID: &groupID,
		Request:     []byte(`{}`),
		UpdatedAt:   time.Now().Add(-25 * time.Hour),
	}}
	groupRepo := &baselineTaskGroupRepoStub{}
	taskService := &baselineTaskServiceStub{groupRepo: groupRepo}
	worker := NewBaselineOperationWorker(BaselineToolDeps{
		TaskService:   taskService,
		OperationRepo: operationRepo,
		Logger:        zap.NewNop(),
	})

	worker.runOnce(context.Background())

	if operationRepo.operation.Status != "failed" || !operationRepo.operation.Terminal {
		t.Fatalf("stale operation was not failed: %#v", operationRepo.operation)
	}
	if operationRepo.operation.ErrorCode != "operation_stale" {
		t.Fatalf("error code = %q, want operation_stale", operationRepo.operation.ErrorCode)
	}
	if len(groupRepo.tasks) != 0 {
		t.Fatalf("stale operation dispatched %d tasks", len(groupRepo.tasks))
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

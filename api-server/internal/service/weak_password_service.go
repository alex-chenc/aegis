package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type WeakPasswordAgentClient interface {
	GetAgentStatus(ctx context.Context, hostID string) (*pb.GetAgentStatusResponse, error)
	ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error)
}

var _ WeakPasswordAgentClient = (*grpcclient.ServerClient)(nil)

var (
	ErrWeakPasswordHostOffline = errors.New("weak password target host agent is offline")
	ErrWeakPasswordTaskRunning = errors.New("weak password task is running")
)

type WeakPasswordService struct {
	repo        *repository.WeakPasswordRepository
	agentClient WeakPasswordAgentClient
	logger      *zap.Logger
}

func NewWeakPasswordService(repo *repository.WeakPasswordRepository, agentClient WeakPasswordAgentClient, logger *zap.Logger) *WeakPasswordService {
	if logger == nil {
		logger = zap.NewNop()
	}
	svc := &WeakPasswordService{
		repo:        repo,
		agentClient: agentClient,
		logger:      logger,
	}
	if repo != nil {
		if err := svc.EnsureDefaultDictionary(context.Background()); err != nil {
			logger.Warn("failed to seed weak password default dictionary", zap.Error(err))
		}
	}
	return svc
}

type WeakPasswordCandidateDTO struct {
	CandidateApplicationID string   `json:"candidate_application_id"`
	HostID                 string   `json:"host_id"`
	AssetID                string   `json:"asset_id,omitempty"`
	Hostname               string   `json:"hostname,omitempty"`
	IPAddress              string   `json:"ip_address,omitempty"`
	ApplicationName        string   `json:"application_name"`
	ApplicationType        string   `json:"application_type"`
	ApplicationVersion     string   `json:"application_version,omitempty"`
	ProfileID              string   `json:"profile_id,omitempty"`
	Confidence             float64  `json:"confidence"`
	CandidatePaths         []string `json:"candidate_paths"`
	CredentialTypes        []string `json:"credential_types"`
	AIReason               string   `json:"ai_reason"`
	Status                 string   `json:"status"`
}

type AnalyzeAssetApplicationsResponse struct {
	AnalysisID            string                     `json:"analysis_id"`
	Status                string                     `json:"status"`
	ApplicationAssetCount int                        `json:"application_asset_count"`
	CandidateCount        int                        `json:"candidate_count"`
	ErrorCode             string                     `json:"error_code,omitempty"`
	Message               string                     `json:"message,omitempty"`
	Candidates            []WeakPasswordCandidateDTO `json:"candidates"`
}

type CreateTaskByApplicationResponse struct {
	TaskID            string `json:"task_id"`
	ScanApplicationID string `json:"scan_application_id"`
	Status            string `json:"status"`
}

type RevealedWeakPasswordFinding struct {
	FindingID       string `json:"finding_id"`
	ApplicationName string `json:"application_name"`
	Account         string `json:"account"`
	CredentialType  string `json:"credential_type"`
	MatchedPassword string `json:"matched_password"`
	SourcePath      string `json:"source_path"`
	FieldPath       string `json:"field_path"`
}

type DictionarySummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"dictionary_type"`
	Status      string   `json:"status"`
	EntryCount  int      `json:"entry_count"`
	Source      string   `json:"source"`
	Categories  []string `json:"categories"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	SampleCount int      `json:"sample_count,omitempty"`
}

type CreateWeakPasswordDictionaryRequest struct {
	Name           string   `json:"name"`
	DictionaryType string   `json:"dictionary_type"`
	Entries        []string `json:"entries"`
	Categories     []string `json:"categories"`
	Source         string   `json:"source"`
}

type CredentialCollectionPlan struct {
	TaskID           string                     `json:"task_id"`
	PlanID           string                     `json:"plan_id"`
	HostID           string                     `json:"host_id"`
	Applications     []CredentialApplication    `json:"applications"`
	CollectionPolicy CredentialCollectionPolicy `json:"collection_policy"`
}

type CredentialApplication struct {
	Application string                `json:"application"`
	AssetID     string                `json:"asset_id"`
	ProfileID   string                `json:"profile_id"`
	Paths       []string              `json:"paths"`
	Extractors  []CredentialExtractor `json:"extractors"`
}

type CredentialExtractor struct {
	Type             string `json:"type"`
	Section          string `json:"section,omitempty"`
	AccountSelector  string `json:"account_selector,omitempty"`
	PasswordSelector string `json:"password_selector,omitempty"`
	FormatHint       string `json:"format_hint,omitempty"`
	SourceKind       string `json:"source_kind,omitempty"`
}

type CredentialCollectionPolicy struct {
	MaxFileBytes          int64 `json:"max_file_bytes"`
	MaxRecords            int   `json:"max_records"`
	RedactContextValues   bool  `json:"redact_context_values"`
	ForbidFindCommand     bool  `json:"forbid_find_command"`
	ForbidRecursiveSearch bool  `json:"forbid_recursive_search"`
}

type AgentCredentialCollectionResult struct {
	TaskID  string                  `json:"task_id"`
	PlanID  string                  `json:"plan_id"`
	HostID  string                  `json:"host_id"`
	Records []AgentCredentialRecord `json:"records"`
	Errors  []AgentCollectionError  `json:"errors"`
}

type AgentCredentialRecord struct {
	RecordID        string  `json:"record_id"`
	Application     string  `json:"application"`
	AssetID         string  `json:"asset_id"`
	SourcePath      string  `json:"source_path"`
	SourceKind      string  `json:"source_kind"`
	Account         string  `json:"account"`
	CredentialType  string  `json:"credential_type"`
	CredentialValue string  `json:"credential_value"`
	Salt            string  `json:"salt"`
	AlgorithmHint   string  `json:"algorithm_hint"`
	FieldPath       string  `json:"field_path"`
	Parser          string  `json:"parser"`
	Confidence      float64 `json:"confidence"`
}

type AgentCollectionError struct {
	Application             string   `json:"application"`
	SourcePath              string   `json:"source_path"`
	ErrorCode               string   `json:"error_code"`
	Message                 string   `json:"message"`
	Retryable               bool     `json:"retryable"`
	SuggestedAuxiliaryTools []string `json:"suggested_auxiliary_tools"`
}

func (s *WeakPasswordService) AnalyzeAssetApplications(ctx context.Context, req model.AnalyzeAssetApplicationsRequest, createdBy *uuid.UUID) (*AnalyzeAssetApplicationsResponse, error) {
	req.Scope.OnlineAgentsOnly = true
	hostIDs := parseUUIDList(req.Scope.HostIDs)
	assets, total, err := s.repo.ListApplicationAssets(repository.WeakPasswordApplicationAssetFilter{
		HostIDs:          hostIDs,
		ApplicationTypes: normalizeApplicationTypeFilters(req.Scope.ApplicationTypes),
		OnlineAgentsOnly: true,
		Page:             1,
		PageSize:         1000,
	})
	if err != nil {
		return nil, err
	}
	assets = s.filterRuntimeOnlineAssets(ctx, assets)
	total = int64(len(assets))
	if total == 0 {
		return &AnalyzeAssetApplicationsResponse{
			Status:    "failed",
			ErrorCode: model.ErrCodeNoApplicationAssets,
			Message:   "当前范围没有在线主机的应用资产，请确认 Agent 在线并完成资产采集",
		}, nil
	}

	analysisID := uuid.New()
	now := time.Now()
	analysis := &model.WeakPasswordAssetAppAnalysis{
		ID:                    analysisID,
		ScopeJSON:             mustJSON(req.Scope),
		Status:                "completed",
		ApplicationAssetCount: int(total),
		CandidateCount:        0,
		PromptSummary:         "deterministic application asset analysis; source=host_application_assets",
		CreatedBy:             createdBy,
		StartedAt:             &now,
		FinishedAt:            &now,
	}

	candidates := make([]model.WeakPasswordCandidateApplication, 0, len(assets))
	dtos := make([]WeakPasswordCandidateDTO, 0, len(assets))
	for _, asset := range assets {
		plan := buildCandidateFromAsset(analysisID, asset)
		if plan.ApplicationType == "unknown" {
			continue
		}
		candidates = append(candidates, plan)
		dtos = append(dtos, candidateDTO(plan, asset.Hostname, asset.IPAddress))
	}
	analysis.CandidateCount = len(candidates)
	if err := s.repo.CreateAnalysisWithCandidates(analysis, candidates); err != nil {
		return nil, err
	}

	s.logger.Info("weak password asset applications analyzed",
		zap.String("analysis_id", analysisID.String()),
		zap.Int64("application_asset_count", total),
		zap.Int("candidate_count", len(candidates)))

	return &AnalyzeAssetApplicationsResponse{
		AnalysisID:            analysisID.String(),
		Status:                "completed",
		ApplicationAssetCount: int(total),
		CandidateCount:        len(candidates),
		Candidates:            dtos,
	}, nil
}

func (s *WeakPasswordService) ListCandidateApplications(analysisID, hostID, applicationType, confidence string, page, pageSize int) ([]WeakPasswordCandidateDTO, int64, error) {
	var analysisUUID *uuid.UUID
	if analysisID != "" {
		parsed, err := uuid.Parse(analysisID)
		if err != nil {
			return nil, 0, err
		}
		analysisUUID = &parsed
	}
	var hostUUID *uuid.UUID
	if hostID != "" {
		parsed, err := uuid.Parse(hostID)
		if err != nil {
			return nil, 0, err
		}
		hostUUID = &parsed
	}
	items, total, err := s.repo.ListCandidateApplications(analysisUUID, hostUUID, applicationType, confidence, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	dtos := make([]WeakPasswordCandidateDTO, 0, len(items))
	for _, item := range items {
		hostname, ip := candidateHostInfo(item)
		dtos = append(dtos, candidateDTO(item, hostname, ip))
	}
	return dtos, total, nil
}

func (s *WeakPasswordService) filterRuntimeOnlineAssets(ctx context.Context, assets []model.HostApplicationAsset) []model.HostApplicationAsset {
	if s.agentClient == nil || len(assets) == 0 {
		return assets
	}
	onlineByHost := make(map[uuid.UUID]bool)
	filtered := make([]model.HostApplicationAsset, 0, len(assets))
	for _, asset := range assets {
		online, ok := onlineByHost[asset.HostID]
		if !ok {
			status, err := s.agentClient.GetAgentStatus(ctx, asset.HostID.String())
			online = err == nil && status != nil && status.GetConnected()
			onlineByHost[asset.HostID] = online
			if err != nil {
				s.logger.Warn("failed to check weak password candidate host runtime status",
					zap.String("host_id", asset.HostID.String()),
					zap.Error(err))
			}
		}
		if online {
			filtered = append(filtered, asset)
		}
	}
	return filtered
}

func (s *WeakPasswordService) ensureHostRuntimeOnline(ctx context.Context, hostID uuid.UUID) error {
	if s.agentClient == nil {
		return fmt.Errorf("%w: agent client not initialized", ErrWeakPasswordHostOffline)
	}
	status, err := s.agentClient.GetAgentStatus(ctx, hostID.String())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWeakPasswordHostOffline, err)
	}
	if status == nil || !status.GetConnected() {
		return ErrWeakPasswordHostOffline
	}
	return nil
}

func (s *WeakPasswordService) CreateTaskByApplication(ctx context.Context, req model.CreateTaskByApplicationRequest, createdBy *uuid.UUID) (*CreateTaskByApplicationResponse, error) {
	candidateID, err := uuid.Parse(req.CandidateApplicationID)
	if err != nil {
		return nil, fmt.Errorf("invalid candidate_application_id: %w", err)
	}
	candidate, err := s.repo.GetCandidateApplication(candidateID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureHostRuntimeOnline(ctx, candidate.HostID); err != nil {
		s.logger.Warn("weak password task rejected because host is offline",
			zap.String("candidate_application_id", candidate.ID.String()),
			zap.String("host_id", candidate.HostID.String()),
			zap.Error(err))
		return nil, err
	}
	maxToolCalls := req.AIPolicy.MaxAgentToolCallsPerApp
	if maxToolCalls <= 0 || maxToolCalls > 10 {
		maxToolCalls = 10
	}

	now := time.Now()
	taskID := uuid.New()
	scanHostID := uuid.New()
	scanAppID := uuid.New()
	planID := uuid.New()
	task := &model.WeakPasswordScanTask{
		ID:                   taskID,
		Name:                 "弱密码检查 - " + candidate.ApplicationName,
		TriggerSource:        "manual",
		Status:               model.TaskStatusPending,
		Progress:             0,
		CurrentStage:         "task_created",
		ScopeJSON:            mustJSON(map[string]string{"candidate_application_id": candidate.ID.String()}),
		DictionaryPolicyJSON: mustJSON(req.DictionaryPolicy),
		AIPolicyJSON:         mustJSON(req.AIPolicy),
		TotalHosts:           1,
		TotalApplications:    1,
		CreatedBy:            createdBy,
		StartedAt:            &now,
	}
	host := &model.WeakPasswordScanHost{
		ID:          scanHostID,
		TaskID:      taskID,
		HostID:      candidate.HostID,
		Status:      model.TaskStatusPending,
		AgentStatus: "unknown",
		Progress:    0,
		StartedAt:   &now,
	}
	app := &model.WeakPasswordScanApplication{
		ID:                     scanAppID,
		TaskID:                 taskID,
		ScanHostID:             scanHostID,
		HostID:                 candidate.HostID,
		AssetID:                candidate.AssetID,
		CandidateApplicationID: &candidate.ID,
		ApplicationName:        candidate.ApplicationName,
		ApplicationType:        candidate.ApplicationType,
		ProfileID:              candidate.ProfileID,
		Status:                 model.AppStatusPlanned,
		Progress:               0,
		CurrentStage:           "planned",
		MaxAgentToolCalls:      maxToolCalls,
		AttemptedPathsJSON:     candidate.CandidatePathsJSON,
		StartedAt:              &now,
	}
	plan := buildCollectionPlan(planID, taskID, *candidate, maxToolCalls)
	planModel := &model.WeakPasswordCollectionPlan{
		ID:                     planID,
		TaskID:                 taskID,
		HostID:                 candidate.HostID,
		CandidateApplicationID: &candidate.ID,
		PlanJSON:               mustJSON(plan),
		LLMAnalysisJSON:        candidate.AssetEvidenceJSON,
		Status:                 "pending",
	}
	if err := s.repo.CreateTaskBundle(task, host, app, planModel); err != nil {
		return nil, err
	}

	go s.executeApplicationTask(context.Background(), taskID, scanHostID, scanAppID, plan)

	return &CreateTaskByApplicationResponse{
		TaskID:            taskID.String(),
		ScanApplicationID: scanAppID.String(),
		Status:            model.TaskStatusPending,
	}, nil
}

func (s *WeakPasswordService) executeApplicationTask(ctx context.Context, taskID, scanHostID, scanAppID uuid.UUID, plan CredentialCollectionPlan) {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		s.logger.Warn("weak password task missing before execution", zap.String("task_id", taskID.String()), zap.Error(err))
		return
	}
	host, app, err := s.loadTaskHostAndApp(scanHostID, scanAppID)
	if err != nil {
		s.logger.Warn("weak password task detail missing before execution", zap.String("task_id", taskID.String()), zap.Error(err))
		return
	}

	task.Status = model.TaskStatusCollectingCredentials
	task.Progress = 20
	task.CurrentStage = "dispatch_agent_tool"
	host.Status = "collecting"
	host.AgentStatus = "checking"
	host.Progress = 20
	app.Status = model.AppStatusCollecting
	app.Progress = 20
	app.CurrentStage = "dispatch_agent_tool"
	_ = s.repo.UpdateTask(task)
	_ = s.repo.UpdateScanHost(host)
	_ = s.repo.UpdateScanApplication(app)

	if s.agentClient == nil {
		s.failApplication(task, host, app, model.ErrCodeAgentNotConnected, "Agent 服务客户端未初始化", 0)
		return
	}

	// 带重试的 Agent 状态检查，处理 Agent 正在重连的情况
	var status *pb.GetAgentStatusResponse
	var checkErr error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		status, checkErr = s.agentClient.GetAgentStatus(ctx, plan.HostID)
		if checkErr == nil && status != nil && status.GetConnected() {
			break
		}
		if i < maxRetries-1 {
			s.logger.Info("agent not ready, retrying...",
				zap.String("host_id", plan.HostID),
				zap.Int("attempt", i+1),
				zap.Error(checkErr))
			time.Sleep(2 * time.Second)
		}
	}

	if checkErr != nil || status == nil || !status.GetConnected() {
		s.failApplication(task, host, app, model.ErrCodeAgentNotConnected, "Agent 不在线，无法下发弱密码采集工具", 0)
		return
	}
	host.AgentStatus = "online"
	callID := "weakpass:" + taskID.String() + ":" + scanAppID.String() + ":collect:0"
	argsJSON, _ := json.Marshal(plan)
	call := &model.WeakPasswordAgentToolCall{
		ID:                   uuid.New(),
		TaskID:               taskID,
		ScanApplicationID:    &scanAppID,
		HostID:               uuid.MustParse(plan.HostID),
		CallID:               callID,
		ToolName:             "WeakPassword.CollectCredentials",
		ArgumentsSummaryJSON: mustJSON(redactCollectionPlanSummary(plan)),
		Status:               "executing",
	}
	_ = s.repo.CreateToolCall(call)
	start := time.Now()
	resp, err := s.agentClient.ExecuteTool(ctx, callID, plan.HostID, "WeakPassword.CollectCredentials", string(argsJSON), 180)
	call.ExecutionTimeMs = time.Since(start).Milliseconds()
	if err != nil {
		call.Status = "failed"
		call.ErrorCode = "agent_execute_failed"
		call.ErrorMessage = err.Error()
		_ = s.repo.UpdateToolCall(call)
		s.failApplication(task, host, app, model.ErrCodeAgentNotConnected, "Agent 工具调用失败", 1)
		return
	}
	if !resp.GetSuccess() {
		call.Status = "failed"
		call.ErrorCode = "agent_execute_failed"
		call.ErrorMessage = resp.GetError()
		_ = s.repo.UpdateToolCall(call)
		s.failApplication(task, host, app, model.ErrCodeAgentNotConnected, "Agent 工具执行失败", 1)
		return
	}

	var result AgentCredentialCollectionResult
	if err := json.Unmarshal([]byte(resp.GetResult()), &result); err != nil {
		call.Status = "failed"
		call.ErrorCode = model.ErrCodeUnsupportedFormat
		call.ErrorMessage = "Agent 返回结果格式不正确"
		_ = s.repo.UpdateToolCall(call)
		s.failApplication(task, host, app, model.ErrCodeUnsupportedFormat, "Agent 返回结果格式不正确", 1)
		return
	}
	call.Status = "completed"
	call.ResultSummaryJSON = mustJSON(map[string]interface{}{
		"record_count": len(result.Records),
		"error_count":  len(result.Errors),
	})
	_ = s.repo.UpdateToolCall(call)

	app.AgentToolCallCount = 1
	app.CollectedRecords = len(result.Records)
	host.CollectedRecords = len(result.Records)
	if len(result.Errors) > 0 && len(result.Records) == 0 {
		s.recordCollectionErrors(taskID, scanAppID, uuid.MustParse(plan.HostID), result.Errors, app.AgentToolCallCount, app.AttemptedPathsJSON)
		if app.AgentToolCallCount >= app.MaxAgentToolCalls {
			s.failApplication(task, host, app, model.ErrCodeConfigDiscoveryFailed, "AI 已尝试 10 次受控 Agent 工具调用，仍未定位到有效配置文件", app.AgentToolCallCount)
			return
		}
		s.failApplication(task, host, app, firstErrorCode(result.Errors), "未能采集到有效凭据材料", app.AgentToolCallCount)
		return
	}

	task.Status = model.TaskStatusMatching
	task.Progress = 72
	task.CurrentStage = "matching"
	app.Status = model.AppStatusMatching
	app.Progress = 72
	app.CurrentStage = "matching"
	host.Status = "matching"
	host.Progress = 72
	_ = s.repo.UpdateTask(task)
	_ = s.repo.UpdateScanHost(host)
	_ = s.repo.UpdateScanApplication(app)

	findings, err := s.MatchCredentialRecords(taskID, scanAppID, host.HostID, result.Records)
	if err != nil {
		s.failApplication(task, host, app, model.ErrCodeLLMMatchVerifyFailed, "弱密码匹配失败", app.AgentToolCallCount)
		return
	}
	if err := s.repo.CreateFindings(findings); err != nil {
		s.failApplication(task, host, app, "finding_persist_failed", "弱密码结果入库失败", app.AgentToolCallCount)
		return
	}
	now := time.Now()
	task.Status = model.TaskStatusCompleted
	task.Progress = 100
	task.CurrentStage = "completed"
	task.MatchedFindings = len(findings)
	task.FinishedAt = &now
	host.Status = "completed"
	host.Progress = 100
	host.MatchedFindings = len(findings)
	host.FinishedAt = &now
	app.Status = model.AppStatusNoMatch
	if len(findings) > 0 {
		app.Status = model.AppStatusMatched
	}
	app.Progress = 100
	app.CurrentStage = "completed"
	app.MatchedFindings = len(findings)
	app.FinishedAt = &now
	_ = s.repo.UpdateTask(task)
	_ = s.repo.UpdateScanHost(host)
	_ = s.repo.UpdateScanApplication(app)
}

func (s *WeakPasswordService) loadTaskHostAndApp(scanHostID, scanAppID uuid.UUID) (*model.WeakPasswordScanHost, *model.WeakPasswordScanApplication, error) {
	var host model.WeakPasswordScanHost
	if err := s.repo.DB().Where("id = ?", scanHostID).First(&host).Error; err != nil {
		return nil, nil, err
	}
	var app model.WeakPasswordScanApplication
	if err := s.repo.DB().Where("id = ?", scanAppID).First(&app).Error; err != nil {
		return nil, nil, err
	}
	return &host, &app, nil
}

func (s *WeakPasswordService) failApplication(task *model.WeakPasswordScanTask, host *model.WeakPasswordScanHost, app *model.WeakPasswordScanApplication, code, message string, callCount int) {
	now := time.Now()
	task.Status = model.TaskStatusFailed
	task.Progress = 100
	task.CurrentStage = code
	task.FailedApplications = 1
	task.FinishedAt = &now
	host.Status = "failed"
	host.Progress = 100
	host.ErrorCode = code
	host.ErrorMessage = message
	host.FailedApplications = 1
	host.FinishedAt = &now
	app.Status = model.AppStatusFailed
	app.Progress = 100
	app.CurrentStage = code
	app.ErrorCode = code
	app.ErrorMessage = message
	if callCount > app.AgentToolCallCount {
		app.AgentToolCallCount = callCount
	}
	app.FinishedAt = &now
	_ = s.repo.UpdateTask(task)
	_ = s.repo.UpdateScanHost(host)
	_ = s.repo.UpdateScanApplication(app)
	_ = s.repo.CreateCollectionError(&model.WeakPasswordCollectionError{
		ID:                 uuid.New(),
		TaskID:             task.ID,
		ScanApplicationID:  &app.ID,
		HostID:             host.HostID,
		ApplicationName:    app.ApplicationName,
		ErrorCode:          code,
		ErrorMessage:       message,
		AgentToolCallCount: app.AgentToolCallCount,
		AttemptedPathsJSON: app.AttemptedPathsJSON,
		FinalStatus:        finalErrorStatus(code),
	})
	s.logger.Warn("weak password application failed",
		zap.String("task_id", task.ID.String()),
		zap.String("scan_application_id", app.ID.String()),
		zap.String("host_id", host.HostID.String()),
		zap.String("error_code", code),
		zap.Int("agent_tool_call_count", app.AgentToolCallCount))
}

func finalErrorStatus(code string) string {
	if code == model.ErrCodeConfigDiscoveryFailed {
		return "config_discovery_failed"
	}
	return "unresolved"
}

func isWeakPasswordTaskRunning(status string) bool {
	switch status {
	case model.TaskStatusPending,
		model.TaskStatusAnalyzingAssets,
		model.TaskStatusCollectingCredentials,
		model.TaskStatusRepairingCollection,
		model.TaskStatusMatching:
		return true
	default:
		return false
	}
}

func (s *WeakPasswordService) recordCollectionErrors(taskID, scanAppID, hostID uuid.UUID, errors []AgentCollectionError, callCount int, attempted datatypes.JSON) {
	for _, item := range errors {
		_ = s.repo.CreateCollectionError(&model.WeakPasswordCollectionError{
			ID:                 uuid.New(),
			TaskID:             taskID,
			ScanApplicationID:  &scanAppID,
			HostID:             hostID,
			ApplicationName:    item.Application,
			SourcePath:         item.SourcePath,
			ErrorCode:          item.ErrorCode,
			ErrorMessage:       item.Message,
			AgentToolCallCount: callCount,
			AttemptedPathsJSON: attempted,
			FinalStatus:        "pending",
		})
	}
}

func (s *WeakPasswordService) MatchCredentialRecords(taskID, scanAppID, hostID uuid.UUID, records []AgentCredentialRecord) ([]model.WeakPasswordFinding, error) {
	defaultDict, err := s.repo.GetDefaultDictionary()
	if err != nil {
		return nil, err
	}
	entries, err := s.repo.ListDictionaryEntries([]uuid.UUID{defaultDict.ID})
	if err != nil {
		return nil, err
	}
	candidates := make([]string, 0, len(entries))
	for _, entry := range entries {
		candidates = append(candidates, entry.Candidate)
	}
	dictionarySet := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		dictionarySet[candidate] = struct{}{}
	}

	findings := []model.WeakPasswordFinding{}
	for _, record := range records {
		switch record.CredentialType {
		case model.CredTypePlaintext:
			if _, ok := dictionarySet[record.CredentialValue]; ok {
				finding, err := findingFromRecord(taskID, scanAppID, hostID, defaultDict.ID, record, record.CredentialValue, model.MatchStatusConfirmed, "default_1000", "dictionary_exact", 1.0)
				if err != nil {
					return nil, err
				}
				findings = append(findings, finding)
			}
		case model.CredTypeHash, model.CredTypeSaltedHash:
			if matched := verifyHashAgainstCandidates(record.CredentialValue, candidates); matched != "" {
				finding, err := findingFromRecord(taskID, scanAppID, hostID, defaultDict.ID, record, matched, model.MatchStatusConfirmed, "default_1000", "server_verifier", 0.98)
				if err != nil {
					return nil, err
				}
				findings = append(findings, finding)
			}
		}
	}
	return findings, nil
}

func findingFromRecord(taskID, scanAppID, hostID, dictionaryID uuid.UUID, record AgentCredentialRecord, matchedPassword, status, source, rule string, confidence float64) (model.WeakPasswordFinding, error) {
	assetID := parseOptionalUUID(record.AssetID)
	encryptedPassword, err := encryptWeakPassword(matchedPassword)
	if err != nil {
		return model.WeakPasswordFinding{}, err
	}
	return model.WeakPasswordFinding{
		ID:                       uuid.New(),
		TaskID:                   taskID,
		ScanApplicationID:        &scanAppID,
		HostID:                   hostID,
		AssetID:                  assetID,
		ApplicationName:          record.Application,
		ApplicationType:          normalizeApplicationType(record.Application),
		Account:                  record.Account,
		CredentialType:           record.CredentialType,
		MatchStatus:              status,
		MatchedPasswordMask:      maskPassword(matchedPassword),
		MatchedPasswordEncrypted: encryptedPassword,
		MatchSource:              source,
		MatchRule:                rule,
		DictionaryID:             &dictionaryID,
		Confidence:               confidence,
		SourcePath:               record.SourcePath,
		FieldPath:                record.FieldPath,
		EvidenceJSON: mustJSON(map[string]interface{}{
			"parser":         record.Parser,
			"source_kind":    record.SourceKind,
			"algorithm_hint": record.AlgorithmHint,
			"salt_present":   record.Salt != "",
		}),
		AIReason: "服务端字典和 verifier 二次校验命中",
	}, nil
}

func (s *WeakPasswordService) GetTaskProgress(taskID uuid.UUID) (*model.TaskProgressResponse, error) {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	app, _ := s.repo.GetScanApplicationByTask(taskID)
	lastTool := ""
	lastErr := ""
	if call, err := s.repo.LastToolCall(taskID); err == nil {
		lastTool = call.ToolName
		lastErr = call.ErrorCode
	}
	resp := &model.TaskProgressResponse{
		TaskID:        task.ID.String(),
		Status:        task.Status,
		Progress:      task.Progress,
		CurrentStage:  task.CurrentStage,
		Message:       task.CurrentStage,
		LastAgentTool: lastTool,
		LastErrorCode: lastErr,
	}
	if app != nil {
		resp.CurrentHostID = app.HostID.String()
		resp.CurrentApplication = app.ApplicationName
		resp.AgentToolCallCount = app.AgentToolCallCount
		resp.MaxAgentToolCalls = app.MaxAgentToolCalls
		if app.ErrorMessage != "" {
			resp.Message = app.ErrorMessage
		}
	}
	return resp, nil
}

func (s *WeakPasswordService) ListTasks(page, pageSize int, status string) ([]model.WeakPasswordScanTask, int64, error) {
	return s.repo.ListTasks(page, pageSize, status)
}

func (s *WeakPasswordService) GetTaskDetail(taskID uuid.UUID) (*model.WeakPasswordScanTask, []model.WeakPasswordCollectionError, error) {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return nil, nil, err
	}
	errors, _ := s.repo.ListCollectionErrors(taskID)
	return task, errors, nil
}

func (s *WeakPasswordService) ListTaskHosts(taskID uuid.UUID) ([]repository.WeakPasswordScanHostWithInfo, error) {
	return s.repo.ListScanHostsWithInfo(taskID)
}

func (s *WeakPasswordService) ListTaskFindings(taskID uuid.UUID) ([]model.WeakPasswordFinding, error) {
	return s.repo.ListFindings(taskID)
}

func (s *WeakPasswordService) RetryFailedTask(ctx context.Context, taskID uuid.UUID) error {
	_ = ctx
	app, err := s.repo.GetScanApplicationByTask(taskID)
	if err != nil {
		return err
	}
	if app.AgentToolCallCount >= app.MaxAgentToolCalls {
		app.ErrorCode = model.ErrCodeConfigDiscoveryFailed
		app.ErrorMessage = "AI 已尝试 10 次受控 Agent 工具调用，仍未定位到有效配置文件"
		app.Status = model.AppStatusFailed
		app.CurrentStage = model.ErrCodeConfigDiscoveryFailed
		return s.repo.UpdateScanApplication(app)
	}
	app.AgentToolCallCount++
	if app.AgentToolCallCount >= app.MaxAgentToolCalls {
		app.ErrorCode = model.ErrCodeConfigDiscoveryFailed
		app.ErrorMessage = "AI 已尝试 10 次受控 Agent 工具调用，仍未定位到有效配置文件"
		app.Status = model.AppStatusFailed
		app.CurrentStage = model.ErrCodeConfigDiscoveryFailed
		return s.repo.UpdateScanApplication(app)
	}
	app.Status = model.AppStatusRepairing
	app.CurrentStage = "repairing_collection"
	return s.repo.UpdateScanApplication(app)
}

func (s *WeakPasswordService) DeleteTask(taskID uuid.UUID) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return err
	}
	if isWeakPasswordTaskRunning(task.Status) {
		return ErrWeakPasswordTaskRunning
	}
	return s.repo.DeleteTask(taskID)
}

func (s *WeakPasswordService) EnsureDefaultDictionary(ctx context.Context) error {
	_ = ctx
	dict, err := s.repo.GetDefaultDictionary()
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return err
		}
		dict = &model.WeakPasswordDictionary{
			ID:             uuid.New(),
			Name:           "默认弱密码字典",
			DictionaryType: model.DictTypeDefault1000,
			Status:         "enabled",
			EntryCount:     0,
			Source:         "built_in",
			Categories:     mustJSON([]string{"通用弱口令", "默认口令", "数据库默认口令", "中间件默认口令", "AI 应用常见弱口令", "企业组合模式"}),
		}
		if err := s.repo.UpsertDictionary(dict); err != nil {
			return err
		}
	}

	count, err := s.repo.CountDictionaryEntries(dict.ID)
	if err != nil {
		return err
	}
	if count >= 1000 {
		if dict.EntryCount != int(count) {
			dict.EntryCount = int(count)
			return s.repo.UpsertDictionary(dict)
		}
		return nil
	}

	entries := buildDictionaryEntries(dict.ID, defaultWeakPasswordCandidates(1000))
	if err := s.repo.UpsertDictionaryEntries(entries); err != nil {
		return err
	}
	count, err = s.repo.CountDictionaryEntries(dict.ID)
	if err != nil {
		return err
	}
	dict.EntryCount = int(count)
	return s.repo.UpsertDictionary(dict)
}

func (s *WeakPasswordService) GetDefaultDictionarySummary() (*DictionarySummary, error) {
	dict, err := s.repo.GetDefaultDictionary()
	if err != nil {
		return nil, err
	}
	return dictionarySummary(*dict), nil
}

func (s *WeakPasswordService) ListDictionaries() ([]DictionarySummary, error) {
	items, err := s.repo.ListDictionaries()
	if err != nil {
		return nil, err
	}
	summaries := make([]DictionarySummary, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, *dictionarySummary(item))
	}
	return summaries, nil
}

func (s *WeakPasswordService) CreateDictionary(req CreateWeakPasswordDictionaryRequest, createdBy *uuid.UUID) (*DictionarySummary, error) {
	if req.Name == "" {
		req.Name = "自定义弱密码字典"
	}
	if req.DictionaryType == "" {
		req.DictionaryType = model.DictTypeUploaded
	}
	if req.Source == "" {
		req.Source = "uploaded"
	}
	entries := uniqueStrings(req.Entries, 1000)
	dict := &model.WeakPasswordDictionary{
		ID:             uuid.New(),
		Name:           req.Name,
		DictionaryType: req.DictionaryType,
		Status:         "enabled",
		EntryCount:     len(entries),
		Source:         req.Source,
		Categories:     mustJSON(req.Categories),
		CreatedBy:      createdBy,
	}
	if err := s.repo.CreateDictionary(dict, buildDictionaryEntries(dict.ID, entries)); err != nil {
		return nil, err
	}
	return dictionarySummary(*dict), nil
}

func (s *WeakPasswordService) GenerateAIDictionary(req model.AIGenerateDictionaryRequest, createdBy *uuid.UUID) (*DictionarySummary, error) {
	if req.Count <= 0 {
		req.Count = 200
	}
	if req.Count > 1000 {
		req.Count = 1000
	}
	seedWords := append([]string{}, req.OrganizationKeywords...)
	seedWords = append(seedWords, req.AccountKeywords...)
	if req.ApplicationType != "" {
		seedWords = append(seedWords, req.ApplicationType)
	}
	if len(seedWords) == 0 {
		seedWords = []string{"admin", "root", "service", "aegis"}
	}
	candidates := generateDictionaryFromSeeds(seedWords, req.Rules, req.Count)
	dict := &model.WeakPasswordDictionary{
		ID:                   uuid.New(),
		Name:                 "AI 生成弱密码字典 - " + time.Now().Format("20060102150405"),
		DictionaryType:       model.DictTypeAIGenerated,
		Status:               "enabled",
		EntryCount:           len(candidates),
		Source:               "ai_generated",
		Categories:           mustJSON([]string{"AI 一键生成字典"}),
		GenerationPolicyJSON: mustJSON(req),
		PromptSummary:        "generate_weak_password_dictionary; count and rule summary only",
		LLMModel:             "deterministic-fallback",
		CreatedBy:            createdBy,
	}
	if err := s.repo.CreateDictionary(dict, buildDictionaryEntries(dict.ID, candidates)); err != nil {
		return nil, err
	}
	return dictionarySummary(*dict), nil
}

func (s *WeakPasswordService) RevealFinding(findingID uuid.UUID, requesterID uuid.UUID, password string) (*RevealedWeakPasswordFinding, error) {
	if strings.TrimSpace(password) == "" {
		return nil, ErrInvalidCredentials
	}
	var user model.AuthUser
	if err := s.repo.DB().Where("id = ?", requesterID).First(&user).Error; err != nil {
		return nil, err
	}
	if user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	finding, err := s.repo.GetFinding(findingID)
	if err != nil {
		return nil, err
	}
	if len(finding.MatchedPasswordEncrypted) == 0 {
		return nil, fmt.Errorf("matched password is unavailable")
	}
	plaintext, err := decryptWeakPassword(finding.MatchedPasswordEncrypted)
	if err != nil {
		return nil, err
	}
	s.logger.Info("weak password finding revealed after system password verification",
		zap.String("finding_id", findingID.String()),
		zap.String("requester_id", requesterID.String()))
	return &RevealedWeakPasswordFinding{
		FindingID:       finding.ID.String(),
		ApplicationName: finding.ApplicationName,
		Account:         finding.Account,
		CredentialType:  finding.CredentialType,
		MatchedPassword: plaintext,
		SourcePath:      finding.SourcePath,
		FieldPath:       finding.FieldPath,
	}, nil
}

func buildCandidateFromAsset(analysisID uuid.UUID, asset model.HostApplicationAsset) model.WeakPasswordCandidateApplication {
	appType := normalizeApplicationType(firstNonEmpty(asset.Name, asset.Category))
	paths := weakJSONStrings(asset.ConfigPaths)
	if len(paths) == 0 && asset.StartPath != "" {
		paths = []string{asset.StartPath}
	}
	profileID, credentialTypes, extractors := profileForApplication(appType)
	reason := "应用资产包含可能承载认证配置的配置路径或启动信息"
	if len(paths) == 0 {
		reason = "应用资产存在，但尚未采集到配置路径；检查时可能需要受控辅助定位"
	}
	assetID := asset.ID
	return model.WeakPasswordCandidateApplication{
		ID:                 uuid.New(),
		AnalysisID:         analysisID,
		HostID:             asset.HostID,
		AssetID:            &assetID,
		ApplicationName:    firstNonEmpty(asset.DisplayName, asset.Name, appType),
		ApplicationType:    appType,
		ApplicationVersion: asset.Version,
		ProfileID:          profileID,
		Confidence:         confidenceOrDefault(asset.AIConfidence),
		CredentialTypes:    mustJSON(credentialTypes),
		CandidatePathsJSON: mustJSON(paths),
		ExtractorPlanJSON:  mustJSON(extractors),
		AssetEvidenceJSON: mustJSON(map[string]interface{}{
			"asset_id":     asset.ID.String(),
			"hostname":     asset.Hostname,
			"ip_address":   asset.IPAddress,
			"category":     asset.Category,
			"start_path":   asset.StartPath,
			"config_paths": paths,
			"listen_ports": weakJSONStrings(asset.ListenPorts),
			"run_user":     asset.RunUser,
			"collected_at": asset.CollectedAt,
			"source_table": "host_application_assets",
		}),
		AIReason:  reason,
		Status:    model.AppStatusCandidate,
		CreatedAt: time.Now(),
	}
}

func buildCollectionPlan(planID, taskID uuid.UUID, candidate model.WeakPasswordCandidateApplication, maxToolCalls int) CredentialCollectionPlan {
	paths := weakJSONStrings(candidate.CandidatePathsJSON)
	extractors := []CredentialExtractor{}
	_ = json.Unmarshal(candidate.ExtractorPlanJSON, &extractors)
	return CredentialCollectionPlan{
		TaskID: taskID.String(),
		PlanID: planID.String(),
		HostID: candidate.HostID.String(),
		Applications: []CredentialApplication{{
			Application: candidate.ApplicationType,
			AssetID:     optionalUUIDString(candidate.AssetID),
			ProfileID:   candidate.ProfileID,
			Paths:       paths,
			Extractors:  extractors,
		}},
		CollectionPolicy: CredentialCollectionPolicy{
			MaxFileBytes:          1024 * 1024,
			MaxRecords:            500,
			RedactContextValues:   true,
			ForbidFindCommand:     true,
			ForbidRecursiveSearch: true,
		},
	}
}

func profileForApplication(appType string) (string, []string, []CredentialExtractor) {
	switch appType {
	case "redis":
		return "redis_config_v1", []string{model.CredTypePlaintext, model.CredTypeAuthString}, []CredentialExtractor{{Type: "line_key_value", PasswordSelector: "requirepass", FormatHint: model.CredTypePlaintext}}
	case "mysql", "mariadb":
		return "mysql_config_v1", []string{model.CredTypePlaintext, model.CredTypeAuthString}, []CredentialExtractor{{Type: "ini", Section: "client", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext}}
	case "postgresql", "postgres":
		return "postgres_config_v1", []string{model.CredTypePlaintext, model.CredTypeAuthString}, []CredentialExtractor{{Type: "properties", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext}}
	case "nginx", "apache", "web_service":
		return "basic_auth_v1", []string{model.CredTypeHash, model.CredTypeSaltedHash}, []CredentialExtractor{{Type: "htpasswd", FormatHint: model.CredTypeHash}}
	case "linux_shadow":
		return "linux_shadow_v1", []string{model.CredTypeSaltedHash}, []CredentialExtractor{{Type: "shadow", SourceKind: "system_account", FormatHint: model.CredTypeSaltedHash}}
	case "ai_agent", "mcp_server", "llm_service":
		return appType + "_config_v1", []string{model.CredTypePlaintext, model.CredTypeAuthString}, []CredentialExtractor{{Type: "yaml", AccountSelector: "auth.user", PasswordSelector: "auth.token", FormatHint: model.CredTypeAuthString}}
	default:
		return appType + "_config_v1", []string{model.CredTypePlaintext, model.CredTypeUnknown}, []CredentialExtractor{{Type: "line_key_value", AccountSelector: "user", PasswordSelector: "password", FormatHint: model.CredTypePlaintext}}
	}
}

func candidateDTO(candidate model.WeakPasswordCandidateApplication, hostname, ip string) WeakPasswordCandidateDTO {
	if hostname == "" || ip == "" {
		evidenceHostname, evidenceIP := candidateHostInfo(candidate)
		if hostname == "" {
			hostname = evidenceHostname
		}
		if ip == "" {
			ip = evidenceIP
		}
	}
	return WeakPasswordCandidateDTO{
		CandidateApplicationID: candidate.ID.String(),
		HostID:                 candidate.HostID.String(),
		AssetID:                optionalUUIDString(candidate.AssetID),
		Hostname:               hostname,
		IPAddress:              ip,
		ApplicationName:        candidate.ApplicationName,
		ApplicationType:        candidate.ApplicationType,
		ApplicationVersion:     candidate.ApplicationVersion,
		ProfileID:              candidate.ProfileID,
		Confidence:             candidate.Confidence,
		CandidatePaths:         weakJSONStrings(candidate.CandidatePathsJSON),
		CredentialTypes:        weakJSONStrings(candidate.CredentialTypes),
		AIReason:               candidate.AIReason,
		Status:                 candidate.Status,
	}
}

func candidateHostInfo(candidate model.WeakPasswordCandidateApplication) (string, string) {
	var evidence map[string]interface{}
	if len(candidate.AssetEvidenceJSON) == 0 || json.Unmarshal(candidate.AssetEvidenceJSON, &evidence) != nil {
		return "", ""
	}
	hostname, _ := evidence["hostname"].(string)
	ip, _ := evidence["ip_address"].(string)
	return hostname, ip
}

func normalizeApplicationType(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "redis"):
		return "redis"
	case strings.Contains(lower, "mariadb"):
		return "mariadb"
	case strings.Contains(lower, "mysql"):
		return "mysql"
	case strings.Contains(lower, "postgres"):
		return "postgresql"
	case strings.Contains(lower, "nginx"):
		return "nginx"
	case strings.Contains(lower, "apache") || strings.Contains(lower, "httpd"):
		return "apache"
	case lower == "database":
		return "mysql"
	case lower == "ai_agent":
		return "ai_agent"
	case lower == "mcp_server":
		return "mcp_server"
	case lower == "llm_service":
		return "llm_service"
	case lower == "web_service":
		return "web_service"
	default:
		if lower == "" {
			return "unknown"
		}
		return lower
	}
}

func normalizeApplicationTypeFilters(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func mustJSON(value interface{}) datatypes.JSON {
	data, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte(`{}`))
	}
	return datatypes.JSON(data)
}

func weakJSONStrings(data datatypes.JSON) []string {
	if len(data) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		return values
	}
	var numbers []int
	if err := json.Unmarshal(data, &numbers); err == nil {
		out := make([]string, 0, len(numbers))
		for _, number := range numbers {
			out = append(out, fmt.Sprint(number))
		}
		return out
	}
	return nil
}

func parseUUIDList(values []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if parsed, err := uuid.Parse(value); err == nil {
			out = append(out, parsed)
		}
	}
	return out
}

func optionalUUIDString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func parseOptionalUUID(value string) *uuid.UUID {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func confidenceOrDefault(value float64) float64 {
	if value <= 0 {
		return 0.72
	}
	return value
}

func firstErrorCode(errors []AgentCollectionError) string {
	if len(errors) == 0 || errors[0].ErrorCode == "" {
		return model.ErrCodeUnsupportedFormat
	}
	return errors[0].ErrorCode
}

func redactCollectionPlanSummary(plan CredentialCollectionPlan) map[string]interface{} {
	apps := make([]map[string]interface{}, 0, len(plan.Applications))
	for _, app := range plan.Applications {
		apps = append(apps, map[string]interface{}{
			"application":     app.Application,
			"asset_id":        app.AssetID,
			"profile_id":      app.ProfileID,
			"path_count":      len(app.Paths),
			"extractor_count": len(app.Extractors),
		})
	}
	return map[string]interface{}{
		"task_id":      plan.TaskID,
		"plan_id":      plan.PlanID,
		"host_id":      plan.HostID,
		"applications": apps,
		"policy": map[string]interface{}{
			"max_file_bytes":          plan.CollectionPolicy.MaxFileBytes,
			"max_records":             plan.CollectionPolicy.MaxRecords,
			"forbid_find_command":     plan.CollectionPolicy.ForbidFindCommand,
			"forbid_recursive_search": plan.CollectionPolicy.ForbidRecursiveSearch,
		},
	}
}

func maskPassword(password string) string {
	runes := []rune(password)
	if len(runes) == 0 {
		return "******"
	}
	return strings.Repeat("*", len(runes))
}

func encryptWeakPassword(plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(weakPasswordCipherKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func decryptWeakPassword(ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(weakPasswordCipherKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, encrypted := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func weakPasswordCipherKey() []byte {
	seed := os.Getenv("AEGIS_WEAK_PASSWORD_ENCRYPTION_KEY")
	if seed == "" {
		seed = "default-encryption-key"
	}
	sum := sha256.Sum256([]byte(seed))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

func verifyHashAgainstCandidates(hash string, candidates []string) string {
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		for _, candidate := range candidates {
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(candidate)) == nil {
				return candidate
			}
		}
	}
	return ""
}

func dictionarySummary(dict model.WeakPasswordDictionary) *DictionarySummary {
	return &DictionarySummary{
		ID:          dict.ID.String(),
		Name:        dict.Name,
		Type:        dict.DictionaryType,
		Status:      dict.Status,
		EntryCount:  dict.EntryCount,
		Source:      dict.Source,
		Categories:  weakJSONStrings(dict.Categories),
		CreatedAt:   dict.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   dict.UpdatedAt.Format(time.RFC3339),
		SampleCount: minInt(dict.EntryCount, 12),
	}
}

func buildDictionaryEntries(dictionaryID uuid.UUID, candidates []string) []model.WeakPasswordDictionaryEntry {
	entries := make([]model.WeakPasswordDictionaryEntry, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		sum := sha256.Sum256([]byte(candidate))
		entries = append(entries, model.WeakPasswordDictionaryEntry{
			ID:            uuid.New(),
			DictionaryID:  dictionaryID,
			Candidate:     candidate,
			CandidateHash: hex.EncodeToString(sum[:]),
			Category:      "default",
			RuleSource:    "built_in",
			RiskLevel:     "high",
		})
	}
	return entries
}

func defaultWeakPasswordCandidates(limit int) []string {
	seeds := []string{
		"admin", "password", "123456", "12345678", "123456789", "qwerty", "abc123", "root", "toor",
		"admin123", "Admin@123", "P@ssw0rd", "Passw0rd", "password1", "123123", "111111", "000000",
		"redis", "Redis@123", "mysql", "Mysql@123", "postgres", "Postgres@123", "oracle", "test",
		"guest", "user", "service", "changeme", "default", "welcome", "Aegis@123",
	}
	return generateDictionaryFromSeeds(seeds, []string{"append_year", "append_special_char", "capitalize", "leet_replace"}, limit)
}

func generateDictionaryFromSeeds(seeds, rules []string, limit int) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || len(out) >= limit {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	years := []string{"2024", "2025", "2026", "123", "1234", "!"}
	specials := []string{"@", "@123", "!", "#", "_"}
	for _, seed := range seeds {
		add(seed)
		add(strings.ToLower(seed))
		add(strings.Title(strings.ToLower(seed)))
		for _, suffix := range years {
			add(seed + suffix)
			add(strings.Title(strings.ToLower(seed)) + suffix)
		}
		for _, special := range specials {
			add(seed + special)
			add(strings.Title(strings.ToLower(seed)) + special)
		}
		add(strings.NewReplacer("a", "@", "A", "@", "o", "0", "O", "0", "i", "1", "I", "1", "e", "3", "E", "3").Replace(seed))
	}
	for len(out) < limit {
		add(fmt.Sprintf("Admin@%03d", len(out)+1))
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func uniqueStrings(values []string, limit int) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/GehirnInc/crypt"
	_ "github.com/GehirnInc/crypt/apr1_crypt"
	_ "github.com/GehirnInc/crypt/md5_crypt"
	_ "github.com/GehirnInc/crypt/sha256_crypt"
	_ "github.com/GehirnInc/crypt/sha512_crypt"
	"github.com/google/uuid"
	yescrypt "github.com/openwall/yescrypt-go"
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

const (
	minWeakPasswordDetectionRounds = 10
	maxWeakPasswordDetectionRounds = 50

	weakPasswordApplicationAnalysisAttemptTimeout = 20 * time.Second
	weakPasswordApplicationAnalysisMaxAttempts    = 1
	weakPasswordAIDictionaryGenerationTimeout     = 90 * time.Second
	weakPasswordAIDictionaryRealtimeMaxCount      = 50
	weakPasswordAIDictionaryLLMMaxCount           = 50
)

// LLMClientInterface defines the interface for LLM client operations
type LLMClientInterface interface {
	ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error)
}

type WeakPasswordService struct {
	repo        *repository.WeakPasswordRepository
	agentClient WeakPasswordAgentClient
	configRepo  ConfigRepositoryInterface
	llmClient   LLMClientInterface
	llmTimeout  int
	llmRetries  int
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

// SetLLMClient sets the LLM client for AI-powered analysis
func (s *WeakPasswordService) SetLLMClient(llmClient LLMClientInterface) {
	s.llmClient = llmClient
}

// SetConfigRepository sets the config repository for LLM configuration
func (s *WeakPasswordService) SetConfigRepository(configRepo ConfigRepositoryInterface, timeout, retries int) {
	s.configRepo = configRepo
	s.llmTimeout = timeout
	s.llmRetries = retries
	s.logger.Info("SetConfigRepository called",
		zap.Bool("configRepo_nil", configRepo == nil),
		zap.Int("timeout", timeout),
		zap.Int("retries", retries))
}

// getLLMClient returns an existing LLM client or creates a new one from config
func (s *WeakPasswordService) getLLMClient(ctx context.Context) (LLMClientInterface, error) {
	if s.llmClient != nil {
		return s.llmClient, nil
	}

	if s.configRepo == nil {
		s.logger.Warn("getLLMClient: config repository not configured")
		return nil, fmt.Errorf("config repository not configured")
	}

	config, err := s.configRepo.GetActive()
	if err != nil {
		s.logger.Warn("getLLMClient: failed to get LLM config", zap.Error(err))
		return nil, fmt.Errorf("failed to get LLM config: %w", err)
	}

	if config.APIKeyEncrypted == "" {
		s.logger.Warn("getLLMClient: LLM API key not configured")
		return nil, fmt.Errorf("LLM API key not configured")
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		s.logger.Warn("getLLMClient: failed to decrypt API key", zap.Error(err))
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	s.logger.Info("getLLMClient: creating new LLM client",
		zap.String("base_url", config.BaseURL),
		zap.String("model", config.ModelName))
	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmRetries)
	s.llmClient = llmClient
	return llmClient, nil
}

type WeakPasswordCandidateDTO struct {
	CandidateApplicationID string                            `json:"candidate_application_id"`
	HostID                 string                            `json:"host_id"`
	AssetID                string                            `json:"asset_id,omitempty"`
	Hostname               string                            `json:"hostname,omitempty"`
	IPAddress              string                            `json:"ip_address,omitempty"`
	ApplicationName        string                            `json:"application_name"`
	ApplicationType        string                            `json:"application_type"`
	ApplicationVersion     string                            `json:"application_version,omitempty"`
	ProfileID              string                            `json:"profile_id,omitempty"`
	Confidence             float64                           `json:"confidence"`
	CandidatePaths         []string                          `json:"candidate_paths"`
	CredentialTypes        []string                          `json:"credential_types"`
	AIReason               string                            `json:"ai_reason"`
	Status                 string                            `json:"status"`
	ScanStatus             string                            `json:"scan_status"`
	LastTaskID             string                            `json:"last_task_id,omitempty"`
	MatchedFindings        int                               `json:"matched_findings"`
	Findings               []WeakPasswordCandidateFindingDTO `json:"findings,omitempty"`
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

type CreateTasksByApplicationsResponse struct {
	Created []BatchTaskCreatedItem `json:"created"`
	Skipped []BatchTaskSkippedItem `json:"skipped"`
}

type DeleteWeakPasswordTasksRequest struct {
	TaskIDs []string `json:"task_ids"`
}

type DeleteWeakPasswordTasksResponse struct {
	Deleted []string               `json:"deleted"`
	Skipped []BatchTaskSkippedItem `json:"skipped"`
	Count   int                    `json:"count"`
}

type BatchTaskCreatedItem struct {
	CandidateApplicationID string `json:"candidate_application_id"`
	TaskID                 string `json:"task_id"`
	ScanApplicationID      string `json:"scan_application_id"`
	Status                 string `json:"status"`
}

type BatchTaskSkippedItem struct {
	CandidateApplicationID string `json:"candidate_application_id"`
	TaskID                 string `json:"task_id,omitempty"`
	Reason                 string `json:"reason"`
	Message                string `json:"message,omitempty"`
}

type WeakPasswordCandidateFindingDTO struct {
	ID                  string `json:"id"`
	TaskID              string `json:"task_id"`
	Account             string `json:"account"`
	MatchedPasswordMask string `json:"matched_password_mask"`
	SourcePath          string `json:"source_path"`
	FieldPath           string `json:"field_path"`
	ProcessPID          int    `json:"process_pid,omitempty"`
	MatchStatus         string `json:"match_status"`
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

type WeakPasswordCollectionProgressDTO struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id"`
	ScanApplicationID  string `json:"scan_application_id,omitempty"`
	HostID             string `json:"host_id"`
	ApplicationName    string `json:"application_name"`
	ToolName           string `json:"tool_name"`
	Status             string `json:"status"`
	Round              int    `json:"round"`
	SourcePath         string `json:"source_path"`
	FieldName          string `json:"field_name"`
	ErrorCode          string `json:"error_code,omitempty"`
	ErrorMessage       string `json:"error_message,omitempty"`
	ExecutionTimeMs    int64  `json:"execution_time_ms"`
	AgentToolCallCount int    `json:"agent_tool_call_count"`
	MaxAgentToolCalls  int    `json:"max_agent_tool_calls"`
	CreatedAt          string `json:"created_at"`
}

type DictionarySummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"dictionary_type"`
	Status      string   `json:"status"`
	EntryCount  int      `json:"entry_count"`
	Source      string   `json:"source"`
	LLMModel    string   `json:"llm_model,omitempty"`
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
	RelatedPIDs []int                 `json:"related_pids,omitempty"`
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
	ProcessPID      int     `json:"process_pid"`
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

type AgentProcessConfigHintsResult struct {
	PID                  int      `json:"pid"`
	Cmdline              []string `json:"cmdline"`
	CWD                  string   `json:"cwd"`
	Cgroup               []string `json:"cgroup"`
	ContainerID          string   `json:"container_id"`
	ContainerRuntime     string   `json:"container_runtime"`
	ContainerRoot        string   `json:"container_root"`
	OpenConfigFiles      []string `json:"open_config_files"`
	ContainerConfigFiles []string `json:"container_config_files"`
	ConfigPathCandidates []string `json:"config_path_candidates"`
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
	assets = deduplicateApplicationAssetsByHostType(assets)
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

	promptSummary := "deterministic application asset analysis; source=host_application_assets"
	aiAnalysisResults, usedAI, err := s.analyzeCandidateApplicationNeeds(ctx, assets)
	if err != nil {
		s.logger.Error("AI weak password application analysis failed after retries", zap.Error(err))
		return nil, fmt.Errorf("AI 分析失败，请检查 LLM 配置后重试: %w", err)
	}
	if usedAI {
		promptSummary = "ai_enhanced application asset analysis; source=host_application_assets+llm; response=needs_skip_complete"
	}

	analysis := &model.WeakPasswordAssetAppAnalysis{
		ID:                    analysisID,
		ScopeJSON:             mustJSON(req.Scope),
		Status:                "completed",
		ApplicationAssetCount: int(total),
		CandidateCount:        0,
		PromptSummary:         promptSummary,
		CreatedBy:             createdBy,
		StartedAt:             &now,
		FinishedAt:            &now,
	}

	candidates := make([]model.WeakPasswordCandidateApplication, 0, len(assets))
	candidateAssets := make([]model.HostApplicationAsset, 0, len(assets))

	// Process results and build candidates
	for _, asset := range assets {
		plan := buildCandidateFromAsset(analysisID, asset)
		if plan.ApplicationType == "unknown" {
			continue
		}

		// Check AI analysis result
		needsAuth, exists := aiAnalysisResults[plan.ApplicationType]
		if !exists {
			s.logger.Warn("Application type not in weak password analysis results, skipping",
				zap.String("application_type", plan.ApplicationType),
				zap.String("application_name", plan.ApplicationName))
			continue
		}

		if !needsAuth {
			s.logger.Info("AI determined application does not need password auth, skipping",
				zap.String("application_type", plan.ApplicationType),
				zap.String("application_name", plan.ApplicationName))
			continue
		}

		plan.AIReason = "AI 分析确认该应用类型需要密码认证"
		candidates = append(candidates, plan)
		candidateAssets = append(candidateAssets, asset)
	}
	analysis.CandidateCount = len(candidates)
	if err := s.repo.CreateAnalysisWithCandidates(analysis, candidates); err != nil {
		return nil, err
	}
	dtos := make([]WeakPasswordCandidateDTO, 0, len(candidates))
	for idx, candidate := range candidates {
		asset := candidateAssets[idx]
		dtos = append(dtos, candidateDTO(candidate, asset.Hostname, asset.IPAddress))
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

func (s *WeakPasswordService) analyzeCandidateApplicationNeeds(ctx context.Context, assets []model.HostApplicationAsset) (map[string]bool, bool, error) {
	if len(assets) == 0 {
		return map[string]bool{}, false, nil
	}
	if _, err := s.getLLMClient(ctx); err != nil {
		s.logger.Info("LLM client not available, using deterministic weak password application filter", zap.Error(err))
		return deterministicWeakPasswordNeedsAuth(assets), false, nil
	}

	var lastErr error
	for attempt := 1; attempt <= weakPasswordApplicationAnalysisMaxAttempts; attempt++ {
		s.logger.Info("AI weak password application analysis attempt",
			zap.Int("attempt", attempt),
			zap.Int("total_assets", len(assets)),
			zap.Duration("timeout", weakPasswordApplicationAnalysisAttemptTimeout))

		attemptCtx, cancel := context.WithTimeout(ctx, weakPasswordApplicationAnalysisAttemptTimeout)
		results, err := s.analyzeApplicationsWithAI(attemptCtx, assets)
		cancel()
		if err == nil {
			return results, true, nil
		}
		lastErr = err
		s.logger.Warn("AI weak password application analysis attempt failed",
			zap.Int("attempt", attempt),
			zap.Error(err))
	}
	s.logger.Warn("AI weak password application analysis unavailable, using deterministic application filter",
		zap.Int("total_assets", len(assets)),
		zap.Error(lastErr))
	return deterministicWeakPasswordNeedsAuth(assets), false, nil
}

// analyzeApplicationsWithAI uses LLM to analyze which applications need password authentication
func (s *WeakPasswordService) analyzeApplicationsWithAI(ctx context.Context, assets []model.HostApplicationAsset) (map[string]bool, error) {
	// Group assets by application type (deduplicated)
	appTypes := make(map[string]bool)
	for _, asset := range assets {
		appType := normalizeApplicationType(firstNonEmpty(asset.Name, asset.Category))
		if appType != "unknown" {
			appTypes[appType] = true
		}
	}

	if len(appTypes) == 0 {
		return nil, fmt.Errorf("no valid application types found")
	}

	// Build deduplicated app type list
	appTypeList := make([]string, 0, len(appTypes))
	for appType := range appTypes {
		appTypeList = append(appTypeList, appType)
	}

	// If more than 1000 types, process in batches
	batchSize := 1000
	if len(appTypeList) > batchSize {
		s.logger.Info("Application types exceed batch size, processing in batches",
			zap.Int("total_types", len(appTypeList)),
			zap.Int("batch_size", batchSize))

		allResults := make(map[string]bool)
		for i := 0; i < len(appTypeList); i += batchSize {
			end := i + batchSize
			if end > len(appTypeList) {
				end = len(appTypeList)
			}
			batch := appTypeList[i:end]

			batchResults, err := s.analyzeApplicationBatch(ctx, batch)
			if err != nil {
				return nil, fmt.Errorf("batch %d failed: %w", i/batchSize+1, err)
			}

			// Merge results
			for k, v := range batchResults {
				allResults[k] = v
			}
		}

		return allResults, nil
	}

	// Process all at once
	return s.analyzeApplicationBatch(ctx, appTypeList)
}

// analyzeApplicationBatch analyzes a batch of application types with AI
func (s *WeakPasswordService) analyzeApplicationBatch(ctx context.Context, appTypeList []string) (map[string]bool, error) {
	systemPrompt := `你是安全专家，筛选需要弱密码检测的应用。

## 核心规则
只返回**同时满足**以下**所有**条件的应用：
1. 市面已知的公开软件（非自研/内部工具/自定义脚本）
2. 有账户密码登录机制（非纯命令行/系统服务/监控代理）
3. 同一应用类型只返回一次（已自动去重）

## 需要检测的应用类型（必须有账号密码机制）
数据库: MySQL, PostgreSQL, Redis, MongoDB, MariaDB, Elasticsearch, Oracle, SQL Server
中间件: Tomcat, WebLogic, JBoss, WildFly, RabbitMQ, Kafka, ZooKeeper
Web服务: Nginx(htpasswd), Apache(htpasswd), IIS
管理工具: phpMyAdmin, Grafana, Kibana, Jenkins, GitLab, Harbor, SonarQube, Nexus
LDAP: OpenLDAP, Active Directory
AI服务: Ollama, LocalAI, vLLM

## 不需要检测的应用类型
系统工具: bash, sh, cron, systemd, docker, containerd, kubelet
监控代理: node_exporter, prometheus, telegraf, collectd, zabbix_agent
日志代理: rsyslog, fluentd, filebeat, logstash
纯代理/负载均衡: haproxy, squid, nginx(无htpasswd), envoy, traefik
自研应用: 业务系统、内部工具、自定义脚本、go程序、python脚本

## 输出格式
严格按JSON格式返回，reason必须简短（10字以内）：
{"needs":{"app_type1":"原因","app_type2":"原因"},"skip":{"app_type3":"原因"}}

重要：
- 必须覆盖输入中的每一个 app_type
- needs中只放需要检测的应用类型
- skip中放不需要检测的应用类型
- 每个应用类型只能出现在needs或skip中的一个
- key 必须使用输入中的标准 app_type，不要翻译、不要新增不存在的类型
- reason必须简短！`

	// Deduplicate app types for prompt
	seenTypes := make(map[string]bool)
	uniqueTypes := make([]string, 0, len(appTypeList))
	for _, appType := range appTypeList {
		if !seenTypes[appType] {
			seenTypes[appType] = true
			uniqueTypes = append(uniqueTypes, appType)
		}
	}

	userPrompt := fmt.Sprintf("筛选以下应用类型中需要弱密码检测的应用（每种类型只分析一次）：\n%s", strings.Join(uniqueTypes, ","))

	// Get LLM client
	llmClient, err := s.getLLMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM client: %w", err)
	}

	// Call LLM
	response, err := llmClient.ChatCompletion(ctx, systemPrompt, userPrompt, 0.3)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	// Parse response - new format: {"needs":{"app1":"reason"}, "skip":{"app2":"reason"}}
	var result struct {
		Needs map[string]string `json:"needs"`
		Skip  map[string]string `json:"skip"`
	}

	// Try to extract JSON from response
	jsonStr := response
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")
	if startIdx >= 0 && endIdx > startIdx {
		jsonStr = response[startIdx : endIdx+1]
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		s.logger.Warn("failed to parse AI analysis response",
			zap.String("response", response),
			zap.Error(err))
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	inputTypes := make(map[string]struct{}, len(uniqueTypes))
	for _, appType := range uniqueTypes {
		normalized := normalizeApplicationType(appType)
		if normalized != "" && normalized != "unknown" {
			inputTypes[normalized] = struct{}{}
		}
	}

	needsAuthMap := make(map[string]bool)
	addResult := func(rawType string, needsAuth bool, reason string) error {
		appType := normalizeApplicationType(rawType)
		if appType == "" || appType == "unknown" {
			return nil
		}
		if _, expected := inputTypes[appType]; !expected {
			s.logger.Warn("AI analysis returned unexpected application type",
				zap.String("raw_type", rawType),
				zap.String("normalized_type", appType),
				zap.String("reason", reason))
			return nil
		}
		if existing, exists := needsAuthMap[appType]; exists && existing != needsAuth {
			return fmt.Errorf("AI response has conflicting decisions for application type %s", appType)
		}
		needsAuthMap[appType] = needsAuth
		return nil
	}

	// Process needs (needs_auth = true)
	for appType, reason := range result.Needs {
		if err := addResult(appType, true, reason); err != nil {
			return nil, err
		}
		s.logger.Info("AI analysis: needs auth",
			zap.String("app_type", appType),
			zap.String("reason", reason))
	}

	// Process skip (needs_auth = false)
	for appType, reason := range result.Skip {
		if err := addResult(appType, false, reason); err != nil {
			return nil, err
		}
		s.logger.Info("AI analysis: skip",
			zap.String("app_type", appType),
			zap.String("reason", reason))
	}

	missingTypes := make([]string, 0)
	for appType := range inputTypes {
		if _, ok := needsAuthMap[appType]; !ok {
			missingTypes = append(missingTypes, appType)
		}
	}
	if len(missingTypes) > 0 {
		sort.Strings(missingTypes)
		return nil, fmt.Errorf("AI response missing application types: %s", strings.Join(missingTypes, ","))
	}

	return needsAuthMap, nil
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
	if len(dtos) > 0 {
		dtos = s.enrichCandidateScanSummaries(dtos)
	}
	return dtos, total, nil
}

func (s *WeakPasswordService) enrichCandidateScanSummaries(dtos []WeakPasswordCandidateDTO) []WeakPasswordCandidateDTO {
	candidateIDs := make([]uuid.UUID, 0, len(dtos))
	indexByID := make(map[uuid.UUID]int, len(dtos))
	for idx, dto := range dtos {
		parsed, err := uuid.Parse(dto.CandidateApplicationID)
		if err != nil {
			continue
		}
		candidateIDs = append(candidateIDs, parsed)
		indexByID[parsed] = idx
		if dtos[idx].ScanStatus == "" {
			dtos[idx].ScanStatus = "unscanned"
		}
	}
	if len(candidateIDs) == 0 {
		return dtos
	}
	apps, err := s.repo.ListScanApplicationsByCandidateIDs(candidateIDs)
	if err != nil {
		s.logger.Warn("failed to load weak password candidate scan summaries", zap.Error(err))
		return dtos
	}
	latestByCandidate := make(map[uuid.UUID]model.WeakPasswordScanApplication)
	for _, app := range apps {
		if app.CandidateApplicationID == nil {
			continue
		}
		if _, ok := latestByCandidate[*app.CandidateApplicationID]; !ok {
			latestByCandidate[*app.CandidateApplicationID] = app
		}
	}
	scanAppIDs := make([]uuid.UUID, 0, len(latestByCandidate))
	scanToCandidate := make(map[uuid.UUID]uuid.UUID, len(latestByCandidate))
	for candidateID, app := range latestByCandidate {
		idx, ok := indexByID[candidateID]
		if !ok {
			continue
		}
		dtos[idx].LastTaskID = app.TaskID.String()
		dtos[idx].MatchedFindings = app.MatchedFindings
		switch {
		case app.MatchedFindings > 0 || app.Status == model.AppStatusMatched:
			dtos[idx].ScanStatus = "alert"
			scanAppIDs = append(scanAppIDs, app.ID)
			scanToCandidate[app.ID] = candidateID
		case app.Status == model.AppStatusNoMatch && app.Progress >= 100:
			dtos[idx].ScanStatus = "safe"
		default:
			dtos[idx].ScanStatus = "unscanned"
		}
	}
	if len(scanAppIDs) == 0 {
		return dtos
	}
	findings, err := s.repo.ListFindingsByScanApplicationIDs(scanAppIDs)
	if err != nil {
		s.logger.Warn("failed to load weak password candidate findings", zap.Error(err))
		return dtos
	}
	for _, finding := range findings {
		if finding.ScanApplicationID == nil {
			continue
		}
		candidateID, ok := scanToCandidate[*finding.ScanApplicationID]
		if !ok {
			continue
		}
		idx, ok := indexByID[candidateID]
		if !ok {
			continue
		}
		dtos[idx].Findings = append(dtos[idx].Findings, WeakPasswordCandidateFindingDTO{
			ID:                  finding.ID.String(),
			TaskID:              finding.TaskID.String(),
			Account:             finding.Account,
			MatchedPasswordMask: finding.MatchedPasswordMask,
			SourcePath:          finding.SourcePath,
			FieldPath:           finding.FieldPath,
			ProcessPID:          processPIDFromEvidence(finding.EvidenceJSON),
			MatchStatus:         finding.MatchStatus,
		})
	}
	return dtos
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

type weakPasswordHostAppKey struct {
	hostID          uuid.UUID
	applicationType string
}

func deduplicateApplicationAssetsByHostType(assets []model.HostApplicationAsset) []model.HostApplicationAsset {
	byKey := make(map[weakPasswordHostAppKey]model.HostApplicationAsset)
	order := make([]weakPasswordHostAppKey, 0, len(assets))
	for _, asset := range assets {
		appType := normalizeApplicationType(firstNonEmpty(asset.Name, asset.Category))
		if appType == "" || appType == "unknown" {
			continue
		}
		key := weakPasswordHostAppKey{hostID: asset.HostID, applicationType: appType}
		if existing, ok := byKey[key]; ok {
			byKey[key] = mergeWeakPasswordApplicationAsset(existing, asset)
			continue
		}
		byKey[key] = asset
		order = append(order, key)
	}
	out := make([]model.HostApplicationAsset, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out
}

func mergeWeakPasswordApplicationAsset(existing, incoming model.HostApplicationAsset) model.HostApplicationAsset {
	winner := existing
	other := incoming
	if weakPasswordAssetRank(incoming) > weakPasswordAssetRank(existing) {
		winner = incoming
		other = existing
	}
	winner.ConfigPaths = mustJSON(mergeStringLists(weakJSONStrings(winner.ConfigPaths), weakJSONStrings(other.ConfigPaths)))
	winner.ListenPorts = mustJSON(mergeStringLists(weakJSONStrings(winner.ListenPorts), weakJSONStrings(other.ListenPorts)))
	winner.RelatedPIDs = mustJSON(uniquePositiveInts(append(weakJSONInts(winner.RelatedPIDs), weakJSONInts(other.RelatedPIDs)...)))
	if strings.TrimSpace(winner.StartPath) == "" {
		winner.StartPath = other.StartPath
	}
	if strings.TrimSpace(winner.RunUser) == "" {
		winner.RunUser = other.RunUser
	}
	if strings.TrimSpace(winner.Version) == "" {
		winner.Version = other.Version
	}
	return winner
}

func weakPasswordAssetRank(asset model.HostApplicationAsset) float64 {
	rank := asset.AIConfidence * 1000
	rank += float64(len(weakJSONStrings(asset.ConfigPaths))) * 20
	rank += float64(len(weakJSONStrings(asset.ListenPorts))) * 5
	rank += float64(len(weakJSONInts(asset.RelatedPIDs))) * 2
	if !asset.CollectedAt.IsZero() {
		rank += float64(asset.CollectedAt.Unix()) / 1_000_000_000
	}
	return rank
}

func deterministicWeakPasswordNeedsAuth(assets []model.HostApplicationAsset) map[string]bool {
	results := make(map[string]bool)
	for _, asset := range assets {
		appType := normalizeApplicationType(firstNonEmpty(asset.Name, asset.Category))
		if appType == "" || appType == "unknown" {
			continue
		}
		results[appType] = isPublicPasswordAuthApplication(appType)
	}
	return results
}

func isPublicPasswordAuthApplication(appType string) bool {
	switch normalizeApplicationType(appType) {
	case "redis", "mysql", "mariadb", "postgresql", "mongodb", "elasticsearch", "oracle", "sqlserver",
		"tomcat", "weblogic", "jboss", "wildfly", "rabbitmq", "kafka", "zookeeper",
		"nginx", "apache", "iis", "phpmyadmin", "grafana", "kibana", "jenkins", "gitlab", "harbor", "sonarqube", "nexus",
		"openldap", "active_directory", "openssh", "ftp", "ollama", "localai", "vllm", "llm_service", "ai_agent", "mcp_server":
		return true
	default:
		return false
	}
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

func (s *WeakPasswordService) hydrateCandidateRuntimeEvidence(candidate *model.WeakPasswordCandidateApplication) {
	if s == nil || s.repo == nil || candidate == nil || candidate.AssetID == nil {
		return
	}
	var asset model.HostApplicationAsset
	if err := s.repo.DB().Where("id = ?", *candidate.AssetID).First(&asset).Error; err != nil {
		return
	}
	relatedPIDs := uniquePositiveInts(append(candidateRelatedPIDs(*candidate), weakJSONInts(asset.RelatedPIDs)...))
	if len(relatedPIDs) == 0 {
		return
	}
	var evidence map[string]interface{}
	if len(candidate.AssetEvidenceJSON) == 0 || json.Unmarshal(candidate.AssetEvidenceJSON, &evidence) != nil {
		evidence = map[string]interface{}{}
	}
	evidence["related_pids"] = relatedPIDs
	if _, ok := evidence["source_table"]; !ok {
		evidence["source_table"] = "host_application_assets"
	}
	if _, ok := evidence["asset_id"]; !ok {
		evidence["asset_id"] = asset.ID.String()
	}
	candidate.AssetEvidenceJSON = mustJSON(evidence)
	assetPaths := weakJSONStrings(asset.ConfigPaths)
	if len(assetPaths) > 0 {
		candidate.CandidatePathsJSON = mustJSON(mergeCredentialPaths(weakJSONStrings(candidate.CandidatePathsJSON), assetPaths))
	}
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
	s.hydrateCandidateRuntimeEvidence(candidate)
	if err := s.ensureHostRuntimeOnline(ctx, candidate.HostID); err != nil {
		s.logger.Warn("weak password task rejected because host is offline",
			zap.String("candidate_application_id", candidate.ID.String()),
			zap.String("host_id", candidate.HostID.String()),
			zap.Error(err))
		return nil, err
	}
	maxToolCalls := normalizeDetectionRounds(req.AIPolicy)
	dictionaryPolicy := normalizeDictionaryPolicy(req.DictionaryPolicy)
	aiPolicy := req.AIPolicy
	aiPolicy.DetectionRounds = maxToolCalls
	aiPolicy.MaxAgentToolCallsPerApp = maxToolCalls

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
		DictionaryPolicyJSON: mustJSON(dictionaryPolicy),
		AIPolicyJSON:         mustJSON(aiPolicy),
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

func (s *WeakPasswordService) CreateTasksByApplications(ctx context.Context, req model.CreateTasksByApplicationsRequest, createdBy *uuid.UUID) (*CreateTasksByApplicationsResponse, error) {
	resp := &CreateTasksByApplicationsResponse{
		Created: []BatchTaskCreatedItem{},
		Skipped: []BatchTaskSkippedItem{},
	}
	seen := map[string]struct{}{}
	for _, candidateID := range req.CandidateApplicationIDs {
		candidateID = strings.TrimSpace(candidateID)
		if candidateID == "" {
			continue
		}
		if _, ok := seen[candidateID]; ok {
			continue
		}
		seen[candidateID] = struct{}{}
		singleReq := model.CreateTaskByApplicationRequest{
			CandidateApplicationID: candidateID,
			DictionaryPolicy:       req.DictionaryPolicy,
			AIPolicy:               req.AIPolicy,
		}
		created, err := s.CreateTaskByApplication(ctx, singleReq, createdBy)
		if err != nil {
			reason := "create_failed"
			if errors.Is(err, ErrWeakPasswordHostOffline) {
				reason = "host_offline"
			}
			resp.Skipped = append(resp.Skipped, BatchTaskSkippedItem{
				CandidateApplicationID: candidateID,
				Reason:                 reason,
				Message:                err.Error(),
			})
			continue
		}
		resp.Created = append(resp.Created, BatchTaskCreatedItem{
			CandidateApplicationID: candidateID,
			TaskID:                 created.TaskID,
			ScanApplicationID:      created.ScanApplicationID,
			Status:                 created.Status,
		})
	}
	s.logger.Info("weak password batch task creation completed",
		zap.Int("created", len(resp.Created)),
		zap.Int("skipped", len(resp.Skipped)))
	return resp, nil
}

func (s *WeakPasswordService) attemptProcessBasedRepair(ctx context.Context, task *model.WeakPasswordScanTask, host *model.WeakPasswordScanHost, app *model.WeakPasswordScanApplication, originalPlan CredentialCollectionPlan) (*CredentialCollectionPlan, error) {
	if len(originalPlan.Applications) == 0 {
		return &originalPlan, nil
	}
	planApp := originalPlan.Applications[0]
	pids := uniquePositiveInts(planApp.RelatedPIDs)
	if len(pids) == 0 {
		return &originalPlan, nil
	}

	suffixes := credentialConfigSuffixAllowlist(planApp.Application)
	discovered := make([]string, 0)
	for _, pid := range pids {
		if app.AgentToolCallCount >= app.MaxAgentToolCalls {
			break
		}

		args := map[string]interface{}{
			"pid":                   pid,
			"application":           planApp.Application,
			"include_open_files":    true,
			"file_suffix_allowlist": suffixes,
			"max_files":             20,
		}
		argsJSON, _ := json.Marshal(args)
		app.AgentToolCallCount++
		callID := fmt.Sprintf("weakpass:%s:%s:process-hints:%d", task.ID.String(), app.ID.String(), app.AgentToolCallCount)
		call := &model.WeakPasswordAgentToolCall{
			ID:                   uuid.New(),
			TaskID:               task.ID,
			ScanApplicationID:    &app.ID,
			HostID:               host.HostID,
			CallID:               callID,
			ToolName:             "WeakPassword.ProcessConfigHints",
			ArgumentsSummaryJSON: mustJSON(map[string]interface{}{"pid": pid, "application": planApp.Application, "include_open_files": true}),
			Status:               "executing",
		}
		_ = s.repo.CreateToolCall(call)

		start := time.Now()
		resp, err := s.agentClient.ExecuteTool(ctx, callID, originalPlan.HostID, "WeakPassword.ProcessConfigHints", string(argsJSON), 60)
		call.ExecutionTimeMs = time.Since(start).Milliseconds()
		if err != nil {
			call.Status = "failed"
			call.ErrorCode = "agent_execute_failed"
			call.ErrorMessage = err.Error()
			_ = s.repo.UpdateToolCall(call)
			s.logger.Warn("process config hints tool call failed",
				zap.String("task_id", task.ID.String()),
				zap.Int("pid", pid),
				zap.Error(err))
			continue
		}
		if resp == nil || !resp.GetSuccess() {
			errMsg := ""
			if resp != nil {
				errMsg = resp.GetError()
			}
			call.Status = "failed"
			call.ErrorCode = "agent_execute_failed"
			call.ErrorMessage = errMsg
			_ = s.repo.UpdateToolCall(call)
			s.logger.Warn("process config hints tool returned failure",
				zap.String("task_id", task.ID.String()),
				zap.Int("pid", pid),
				zap.String("error", errMsg))
			continue
		}

		var hints AgentProcessConfigHintsResult
		if err := json.Unmarshal([]byte(resp.GetResult()), &hints); err != nil {
			call.Status = "failed"
			call.ErrorCode = model.ErrCodeUnsupportedFormat
			call.ErrorMessage = "Agent 返回进程配置提示格式不正确"
			_ = s.repo.UpdateToolCall(call)
			s.logger.Warn("failed to parse process config hints result",
				zap.String("task_id", task.ID.String()),
				zap.Int("pid", pid),
				zap.Error(err))
			continue
		}

		candidates := hintConfigCandidates(hints)
		discovered = append(discovered, candidates...)
		call.Status = "completed"
		call.ResultSummaryJSON = mustJSON(map[string]interface{}{
			"pid":                    pid,
			"container_detected":     hints.ContainerRoot != "",
			"container_runtime":      hints.ContainerRuntime,
			"candidate_path_count":   len(candidates),
			"open_config_file_count": len(hints.OpenConfigFiles),
		})
		_ = s.repo.UpdateToolCall(call)
	}

	newPaths := mergeCredentialPaths(planApp.Paths, discovered)
	repairedPlan := originalPlan
	repairedPlan.Applications[0].Paths = newPaths
	if len(newPaths) > len(planApp.Paths) {
		s.logger.Info("process-based weak password config repair discovered paths",
			zap.String("task_id", task.ID.String()),
			zap.String("scan_application_id", app.ID.String()),
			zap.Int("pid_count", len(pids)),
			zap.Int("new_path_count", len(newPaths)-len(planApp.Paths)))
		app.Status = model.AppStatusRepairing
		app.CurrentStage = "process_based_config_discovery"
		_ = s.repo.UpdateScanApplication(app)
	}
	return &repairedPlan, nil
}

// attemptCollectionRepair uses LLM to analyze collection errors and suggest auxiliary tool calls
// to locate credential config paths and repair account/password field extractors.
func (s *WeakPasswordService) attemptCollectionRepair(ctx context.Context, task *model.WeakPasswordScanTask, host *model.WeakPasswordScanHost, app *model.WeakPasswordScanApplication, originalPlan CredentialCollectionPlan, errors []AgentCollectionError) (*CredentialCollectionPlan, error) {
	llmClient, err := s.getLLMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM client: %w", err)
	}

	// Build repair context
	application := originalPlan.Applications[0].Application

	// Extract error details
	errorDetails := make([]map[string]interface{}, 0, len(errors))
	for _, e := range errors {
		errorDetails = append(errorDetails, map[string]interface{}{
			"source_path":               e.SourcePath,
			"error_code":                e.ErrorCode,
			"message":                   e.Message,
			"suggested_auxiliary_tools": e.SuggestedAuxiliaryTools,
		})
	}

	// Allowed auxiliary tools
	allowedTools := []string{
		"WeakPassword.ProbePath",
		"WeakPassword.ListConfigDir",
		"WeakPassword.ServiceUnitInspect",
		"WeakPassword.ProcessConfigHints",
	}

	currentPlanSummary, _ := json.Marshal(map[string]interface{}{
		"application":  originalPlan.Applications[0].Application,
		"profile_id":   originalPlan.Applications[0].ProfileID,
		"paths":        originalPlan.Applications[0].Paths,
		"extractors":   originalPlan.Applications[0].Extractors,
		"related_pids": originalPlan.Applications[0].RelatedPIDs,
	})

	// Build system prompt for repair
	systemPrompt := `你是弱密码模块的配置定位专家。你的任务不是项目自测，而是根据应用资产、采集错误和受控 Agent 工具，修复 CredentialCollectionPlan 中的配置文件路径和账号/密码字段提取器。

## 重要提示

很多应用运行在 Docker 容器中，没有 systemd service 文件。在这种情况下：
- **不要使用** WeakPassword.ServiceUnitInspect（查找 systemd service 文件）
- **优先使用** WeakPassword.ProcessConfigHints（从进程获取配置路径）
- **或者使用** WeakPassword.ProbePath（检查路径是否存在）

容器/非容器路径规则：
- 当前计划里的 related_pids 是唯一可信 PID 列表；不要使用 pid=1，除非它明确出现在 related_pids。
- 对有 related_pids 的应用，WeakPassword.ProbePath、WeakPassword.ListConfigDir、WeakPassword.ReadConfigSlice 参数必须带 pid。Agent 会先判断该 PID 是否容器进程；容器进程读取 /proc/<pid>/root 下的容器内文件，非容器进程读取宿主机文件。
- new_paths 必须填写应用视角的绝对路径，例如 /etc/redis/redis.conf；不要返回 /proc/<pid>/root/...。
- Redis 若通过 redis-server --requirepass/--masterauth 启动，凭据由受控采集工具从进程参数提取，不要为了这种情况递归搜索宿主机 /etc。

## 可用辅助工具

1. **WeakPassword.ProbePath** - 检查指定路径是否存在、类型、大小、权限
   - 参数: {"path": "/path/to/file"}
   - 用途: 验证文件是否存在

2. **WeakPassword.ListConfigDir** - 非递归列出指定目录下的文件
   - 参数: {"dir": "/path/to/dir", "pid": 1234, "max_entries": 50, "recursive": false}
   - 用途: 发现配置目录中的其他文件

3. **WeakPassword.ServiceUnitInspect** - 读取 systemd service 的 ExecStart、EnvironmentFile
   - 参数: {"service_name": "redis-server"}
   - 用途: 从服务配置中发现实际路径
   - 注意: 仅适用于非容器化部署的应用

4. **WeakPassword.ProcessConfigHints** - 根据 pid 获取启动参数和打开的文件
   - 参数: {"pid": 1234, "include_open_files": true}
   - 用途: 从运行进程中发现配置文件路径
   - **推荐**: 对于容器化应用，优先使用此工具

## 提取器格式

new_extractors 必须使用以下字段：
- type: line_key_value / ini / properties / yaml / json / shadow / htpasswd / tomcat_users_xml
- section: 可选，仅 ini 等格式需要
- account_selector: 可选账号字段
- password_selector: 密码、token 或 hash 字段
- format_hint: plaintext / auth_string / hash / salted_hash / unknown
- source_kind: 可选来源类型，如 system_account

当错误是 field_not_found 或 unsupported_credential_format 时，优先判断是否需要新增 new_extractors。不要把端口、日志、bind、datadir、调优参数当成密码字段。

## 输出格式

严格按JSON格式返回：
{
  "tool": "工具名",
  "arguments": {参数},
  "reason": "选择原因",
  "new_paths": [发现的新路径列表],
  "new_extractors": [
    {"type":"line_key_value","password_selector":"requirepass","format_hint":"plaintext"}
  ]
}

注意：
- 只能选择一个工具
- new_paths 是从证据或工具结果推断出的明确绝对路径，禁止递归搜索、通配符、shell 元字符
- new_extractors 是从错误和应用类型推断出的账号/密码字段，最多5个
- 如果无法修复，返回 {"tool": "none", "reason": "无法修复原因"}`

	// Build user prompt with error context
	userPrompt := fmt.Sprintf(`## 当前采集计划
%s

## 采集失败信息

应用类型: %s
任务ID: %s
已尝试路径和错误:
%s

## 可用辅助工具
%s

请选择一个辅助工具，并返回需要合并到采集计划里的 new_paths 和/或 new_extractors。`, string(currentPlanSummary), application, task.ID.String(), formatErrorDetails(errorDetails), strings.Join(allowedTools, ", "))

	// Call LLM
	response, err := llmClient.ChatCompletion(ctx, systemPrompt, userPrompt, 0.3)
	if err != nil {
		return nil, fmt.Errorf("LLM repair analysis failed: %w", err)
	}

	// Parse LLM response
	var repairResult struct {
		Tool          string                 `json:"tool"`
		Arguments     map[string]interface{} `json:"arguments"`
		Reason        string                 `json:"reason"`
		NewPaths      []string               `json:"new_paths"`
		NewExtractors []CredentialExtractor  `json:"new_extractors"`
	}

	jsonStr := response
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")
	if startIdx >= 0 && endIdx > startIdx {
		jsonStr = response[startIdx : endIdx+1]
	}

	if err := json.Unmarshal([]byte(jsonStr), &repairResult); err != nil {
		return nil, fmt.Errorf("failed to parse LLM repair response: %w", err)
	}

	hasPlanPatch := len(repairResult.NewPaths) > 0 || len(repairResult.NewExtractors) > 0
	if repairResult.Tool == "none" || repairResult.Tool == "" {
		if hasPlanPatch {
			newPlan := originalPlan
			newPlan.Applications[0].Paths = mergeCredentialPaths(originalPlan.Applications[0].Paths, repairResult.NewPaths)
			newPlan.Applications[0].Extractors = mergeCredentialExtractors(originalPlan.Applications[0].Extractors, repairResult.NewExtractors)
			return &newPlan, nil
		}
		return nil, fmt.Errorf("LLM determined repair is not possible: %s", repairResult.Reason)
	}

	// Validate tool is in allowed list
	toolAllowed := false
	for _, t := range allowedTools {
		if t == repairResult.Tool {
			toolAllowed = true
			break
		}
	}
	if !toolAllowed {
		return nil, fmt.Errorf("LLM selected disallowed tool: %s", repairResult.Tool)
	}

	// Execute auxiliary tool on Agent
	normalizedArgs, err := normalizeRepairToolArguments(repairResult.Tool, repairResult.Arguments, originalPlan.Applications[0])
	if err != nil {
		return nil, err
	}
	repairResult.Arguments = normalizedArgs
	s.logger.Info("executing auxiliary tool for repair",
		zap.String("tool", repairResult.Tool),
		zap.String("reason", repairResult.Reason))

	app.AgentToolCallCount++
	callID := fmt.Sprintf("weakpass:%s:%s:repair:%d", task.ID.String(), app.ID.String(), app.AgentToolCallCount)
	argsJSON, _ := json.Marshal(repairResult.Arguments)

	call := &model.WeakPasswordAgentToolCall{
		ID:                   uuid.New(),
		TaskID:               task.ID,
		ScanApplicationID:    &app.ID,
		HostID:               host.HostID,
		CallID:               callID,
		ToolName:             repairResult.Tool,
		ArgumentsSummaryJSON: mustJSON(map[string]interface{}{"tool": repairResult.Tool, "args": repairResult.Arguments}),
		Status:               "executing",
	}
	_ = s.repo.CreateToolCall(call)

	start := time.Now()
	toolResp, err := s.agentClient.ExecuteTool(ctx, callID, originalPlan.HostID, repairResult.Tool, string(argsJSON), 60)
	call.ExecutionTimeMs = time.Since(start).Milliseconds()

	if err != nil {
		call.Status = "failed"
		call.ErrorMessage = err.Error()
		_ = s.repo.UpdateToolCall(call)
		s.logger.Warn("auxiliary tool execution failed, returning empty result",
			zap.String("tool", repairResult.Tool),
			zap.Error(err))
		// Return plan with no new paths instead of error, so the repair loop can continue
		return &originalPlan, nil
	}

	if !toolResp.GetSuccess() {
		call.Status = "failed"
		call.ErrorMessage = toolResp.GetError()
		_ = s.repo.UpdateToolCall(call)
		s.logger.Warn("auxiliary tool failed, returning empty result",
			zap.String("tool", repairResult.Tool),
			zap.String("error", toolResp.GetError()))
		// Return plan with no new paths instead of error, so the repair loop can continue
		return &originalPlan, nil
	}

	call.Status = "completed"
	call.ResultSummaryJSON = mustJSON(map[string]interface{}{"result_length": len(toolResp.GetResult())})
	_ = s.repo.UpdateToolCall(call)

	// Update app status
	app.Status = model.AppStatusRepairing
	app.CurrentStage = "repairing_collection"
	_ = s.repo.UpdateScanApplication(app)

	// Build new plan with discovered paths
	newPaths := originalPlan.Applications[0].Paths
	if len(repairResult.NewPaths) > 0 {
		newPaths = mergeCredentialPaths(newPaths, repairResult.NewPaths)
	}

	// Create new plan with updated paths
	newPlan := originalPlan
	newPlan.Applications[0].Paths = newPaths
	newPlan.Applications[0].Extractors = mergeCredentialExtractors(originalPlan.Applications[0].Extractors, repairResult.NewExtractors)

	s.logger.Info("repair completed, new paths discovered",
		zap.String("task_id", task.ID.String()),
		zap.Strings("new_paths", newPaths),
		zap.Int("extractor_count", len(newPlan.Applications[0].Extractors)),
		zap.Int("total_paths", len(newPaths)))

	return &newPlan, nil
}

// formatErrorDetails formats error details for LLM prompt
func formatErrorDetails(errors []map[string]interface{}) string {
	var sb strings.Builder
	for i, e := range errors {
		sb.WriteString(fmt.Sprintf("%d. 路径: %v\n   错误: %v - %v\n", i+1, e["source_path"], e["error_code"], e["message"]))
		if tools, ok := e["suggested_auxiliary_tools"].([]string); ok && len(tools) > 0 {
			sb.WriteString(fmt.Sprintf("   建议工具: %v\n", strings.Join(tools, ", ")))
		}
	}
	return sb.String()
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
	if app.MaxAgentToolCalls < minWeakPasswordDetectionRounds {
		app.MaxAgentToolCalls = minWeakPasswordDetectionRounds
	}
	if app.MaxAgentToolCalls > maxWeakPasswordDetectionRounds {
		app.MaxAgentToolCalls = maxWeakPasswordDetectionRounds
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

	var result AgentCredentialCollectionResult
	maxRepairAttempts := app.MaxAgentToolCalls - 1
	if maxRepairAttempts < 0 {
		maxRepairAttempts = minWeakPasswordDetectionRounds - 1
	}
	processRepairTried := false
	for attempt := 0; attempt <= maxRepairAttempts; attempt++ {
		callID := fmt.Sprintf("weakpass:%s:%s:collect:%d", taskID.String(), scanAppID.String(), attempt)
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
			s.failApplication(task, host, app, "agent_execute_failed", "Agent 工具调用失败: "+err.Error(), app.AgentToolCallCount)
			return
		}
		if !resp.GetSuccess() {
			call.Status = "failed"
			call.ErrorCode = "agent_execute_failed"
			call.ErrorMessage = resp.GetError()
			_ = s.repo.UpdateToolCall(call)
			s.failApplication(task, host, app, "agent_execute_failed", "Agent 工具执行失败: "+resp.GetError(), app.AgentToolCallCount)
			return
		}

		if err := json.Unmarshal([]byte(resp.GetResult()), &result); err != nil {
			call.Status = "failed"
			call.ErrorCode = model.ErrCodeUnsupportedFormat
			call.ErrorMessage = "Agent 返回结果格式不正确"
			_ = s.repo.UpdateToolCall(call)
			s.failApplication(task, host, app, model.ErrCodeUnsupportedFormat, "Agent 返回结果格式不正确", app.AgentToolCallCount)
			return
		}
		call.Status = "completed"
		call.ResultSummaryJSON = mustJSON(collectionResultSummary(plan, result))
		_ = s.repo.UpdateToolCall(call)

		if attempt+1 > app.AgentToolCallCount {
			app.AgentToolCallCount = attempt + 1
		}
		app.CollectedRecords = len(result.Records)
		host.CollectedRecords = len(result.Records)

		// If we got records, break out of repair loop
		if len(result.Records) > 0 {
			break
		}

		if !processRepairTried && shouldAttemptProcessBasedRepair(plan, result.Errors) {
			processRepairTried = true
			s.logger.Info("first weak password collection round found no records, attempting process-based config discovery",
				zap.String("task_id", taskID.String()),
				zap.String("scan_application_id", scanAppID.String()),
				zap.Int("related_pid_count", len(plan.Applications[0].RelatedPIDs)),
				zap.Int("error_count", len(result.Errors)))
			oldPathCount := len(plan.Applications[0].Paths)
			repairedPlan, repairErr := s.attemptProcessBasedRepair(ctx, task, host, app, plan)
			if repairErr != nil {
				s.logger.Warn("process-based weak password config repair failed",
					zap.String("task_id", taskID.String()),
					zap.Error(repairErr))
			} else if len(repairedPlan.Applications) > 0 && len(repairedPlan.Applications[0].Paths) > oldPathCount {
				plan = *repairedPlan
				s.logger.Info("process-based weak password config repair successful, retrying collection",
					zap.String("task_id", taskID.String()),
					zap.Int("next_attempt", attempt+2),
					zap.Int("total_paths", len(plan.Applications[0].Paths)))
				continue
			}
		}

		// If no errors or non-retryable errors, break
		if len(result.Errors) == 0 {
			break
		}

		// Check if errors are retryable
		hasRetryableErrors := false
		for _, e := range result.Errors {
			if e.Retryable {
				hasRetryableErrors = true
				break
			}
		}

		if !hasRetryableErrors {
			break
		}

		// Record errors
		s.recordCollectionErrors(taskID, scanAppID, uuid.MustParse(plan.HostID), result.Errors, app.AgentToolCallCount, app.AttemptedPathsJSON)

		// If max attempts reached, fail
		if app.AgentToolCallCount >= app.MaxAgentToolCalls {
			s.failApplication(task, host, app, model.ErrCodeConfigDiscoveryFailed, detectionRoundsExhaustedMessage(app.MaxAgentToolCalls), app.AgentToolCallCount)
			return
		}

		// Attempt repair with LLM
		s.logger.Info("retryable errors detected, attempting AI-assisted repair",
			zap.String("task_id", taskID.String()),
			zap.Int("attempt", attempt+1),
			zap.Int("error_count", len(result.Errors)))

		repairedPlan, repairErr := s.attemptCollectionRepair(ctx, task, host, app, plan, result.Errors)
		if repairErr != nil {
			s.logger.Warn("AI repair failed",
				zap.String("task_id", taskID.String()),
				zap.Error(repairErr))
			s.failApplication(task, host, app, firstErrorCode(result.Errors), "未能采集到有效凭据材料，AI 修复失败: "+repairErr.Error(), app.AgentToolCallCount)
			return
		}

		// Check if repair discovered new paths or credential field extractors
		oldPathCount := len(plan.Applications[0].Paths)
		oldExtractorCount := len(plan.Applications[0].Extractors)
		newPathCount := len(repairedPlan.Applications[0].Paths)
		newExtractorCount := len(repairedPlan.Applications[0].Extractors)
		if newPathCount <= oldPathCount && newExtractorCount <= oldExtractorCount {
			s.logger.Info("AI repair did not discover new paths or extractors, continuing with next attempt",
				zap.String("task_id", taskID.String()),
				zap.Int("old_paths", oldPathCount),
				zap.Int("new_paths", newPathCount),
				zap.Int("old_extractors", oldExtractorCount),
				zap.Int("new_extractors", newExtractorCount))
			continue
		}

		// Update plan for next attempt
		plan = *repairedPlan
		s.logger.Info("AI repair successful, retrying collection",
			zap.String("task_id", taskID.String()),
			zap.Int("next_attempt", attempt+2),
			zap.Int("total_paths", newPathCount),
			zap.Int("total_extractors", newExtractorCount))
	}

	// If we still have no records after all attempts
	if len(result.Records) == 0 {
		s.recordCollectionErrors(taskID, scanAppID, uuid.MustParse(plan.HostID), result.Errors, app.AgentToolCallCount, app.AttemptedPathsJSON)
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

	dictionaryPolicy := dictionaryPolicyFromTask(task)
	findings, err := s.MatchCredentialRecordsWithPolicy(taskID, scanAppID, host.HostID, result.Records, dictionaryPolicy)
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

func normalizeDictionaryPolicy(policy model.WeakPasswordDictionaryPolicy) model.WeakPasswordDictionaryPolicy {
	cleanIDs := make([]string, 0, len(policy.DictionaryIDs))
	seen := map[string]struct{}{}
	for _, id := range policy.DictionaryIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleanIDs = append(cleanIDs, id)
	}
	policy.DictionaryIDs = cleanIDs
	if !policy.UseDefault1000 && !policy.UseAIGenerated && len(policy.DictionaryIDs) == 0 {
		policy.UseDefault1000 = true
	}
	policy.Hybrid = false
	policy.Fuzzy = false
	return policy
}

func normalizeDetectionRounds(policy model.WeakPasswordAIPolicy) int {
	rounds := policy.DetectionRounds
	if rounds <= 0 {
		rounds = policy.MaxAgentToolCallsPerApp
	}
	if rounds < minWeakPasswordDetectionRounds {
		return minWeakPasswordDetectionRounds
	}
	if rounds > maxWeakPasswordDetectionRounds {
		return maxWeakPasswordDetectionRounds
	}
	return rounds
}

func detectionRoundsExhaustedMessage(rounds int) string {
	if rounds <= 0 {
		rounds = minWeakPasswordDetectionRounds
	}
	return fmt.Sprintf("AI 已尝试 %d 次受控 Agent 工具调用，仍未定位到有效配置文件", rounds)
}

func dictionaryPolicyFromTask(task *model.WeakPasswordScanTask) model.WeakPasswordDictionaryPolicy {
	policy := model.WeakPasswordDictionaryPolicy{UseDefault1000: true}
	if task == nil || len(task.DictionaryPolicyJSON) == 0 {
		return policy
	}
	if err := json.Unmarshal(task.DictionaryPolicyJSON, &policy); err != nil {
		return model.WeakPasswordDictionaryPolicy{UseDefault1000: true}
	}
	return normalizeDictionaryPolicy(policy)
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
	return s.MatchCredentialRecordsWithPolicy(taskID, scanAppID, hostID, records, model.WeakPasswordDictionaryPolicy{UseDefault1000: true})
}

func (s *WeakPasswordService) MatchCredentialRecordsWithPolicy(taskID, scanAppID, hostID uuid.UUID, records []AgentCredentialRecord, policy model.WeakPasswordDictionaryPolicy) ([]model.WeakPasswordFinding, error) {
	dictionaryIDs, err := s.dictionaryIDsForPolicy(policy)
	if err != nil {
		return nil, err
	}
	defaultDict, err := s.repo.GetDefaultDictionary()
	if err != nil {
		return nil, err
	}
	entries, err := s.repo.ListDictionaryEntries(dictionaryIDs)
	if err != nil {
		return nil, err
	}
	candidates := make([]string, 0, len(entries))
	dictionaryByCandidate := make(map[string]uuid.UUID, len(entries))
	sourceByCandidate := make(map[string]string, len(entries))
	for _, entry := range entries {
		candidates = append(candidates, entry.Candidate)
		if _, ok := dictionaryByCandidate[entry.Candidate]; !ok {
			dictionaryByCandidate[entry.Candidate] = entry.DictionaryID
			sourceByCandidate[entry.Candidate] = dictionaryMatchSource(entry.DictionaryID, defaultDict.ID)
		}
	}
	dictionarySet := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		dictionarySet[candidate] = struct{}{}
	}

	findings := []model.WeakPasswordFinding{}
	for _, record := range records {
		switch record.CredentialType {
		case model.CredTypePlaintext, model.CredTypeAuthString:
			if _, ok := dictionarySet[record.CredentialValue]; ok {
				dictID := dictionaryByCandidate[record.CredentialValue]
				finding, err := findingFromRecord(taskID, scanAppID, hostID, dictID, record, record.CredentialValue, model.MatchStatusConfirmed, sourceByCandidate[record.CredentialValue], "dictionary_exact", 1.0)
				if err != nil {
					return nil, err
				}
				findings = append(findings, finding)
			}
		case model.CredTypeHash, model.CredTypeSaltedHash:
			if matched := verifyHashAgainstCandidates(record.CredentialValue, candidates); matched != "" {
				dictID := dictionaryByCandidate[matched]
				finding, err := findingFromRecord(taskID, scanAppID, hostID, dictID, record, matched, model.MatchStatusConfirmed, sourceByCandidate[matched], "server_verifier", 0.98)
				if err != nil {
					return nil, err
				}
				findings = append(findings, finding)
			}
		}
	}
	return findings, nil
}

func dictionaryMatchSource(dictionaryID, defaultDictionaryID uuid.UUID) string {
	if dictionaryID == defaultDictionaryID {
		return model.DictTypeDefault1000
	}
	return "selected_dictionary"
}

func (s *WeakPasswordService) dictionaryIDsForPolicy(policy model.WeakPasswordDictionaryPolicy) ([]uuid.UUID, error) {
	policy = normalizeDictionaryPolicy(policy)
	defaultDict, err := s.repo.GetDefaultDictionary()
	if err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]struct{}{}
	ids := []uuid.UUID{}
	add := func(id uuid.UUID) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if policy.UseDefault1000 {
		add(defaultDict.ID)
	}
	for _, rawID := range policy.DictionaryIDs {
		parsed, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("invalid dictionary_id %q: %w", rawID, err)
		}
		add(parsed)
	}
	if policy.UseAIGenerated {
		dictionaries, err := s.repo.ListDictionariesByTypes([]string{model.DictTypeAIGenerated})
		if err != nil {
			return nil, err
		}
		for _, dict := range dictionaries {
			add(dict.ID)
		}
	}
	if len(ids) == 0 {
		add(defaultDict.ID)
	}
	return ids, nil
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
			"process_pid":    record.ProcessPID,
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
	errors, _, _ := s.repo.ListCollectionErrors(taskID, 1, 20)
	return task, errors, nil
}

func (s *WeakPasswordService) ListTaskHosts(taskID uuid.UUID, page, pageSize int) ([]repository.WeakPasswordScanHostWithInfo, int64, error) {
	return s.repo.ListScanHostsWithInfo(taskID, page, pageSize)
}

func (s *WeakPasswordService) ListTaskFindings(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordFinding, int64, error) {
	return s.repo.ListFindings(taskID, page, pageSize)
}

func (s *WeakPasswordService) ListTaskCollectionErrors(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordCollectionError, int64, error) {
	return s.repo.ListCollectionErrors(taskID, page, pageSize)
}

func collectionResultSummary(plan CredentialCollectionPlan, result AgentCredentialCollectionResult) map[string]interface{} {
	sourcePaths := []string{}
	fieldNames := []string{}
	recordSources := []map[string]interface{}{}
	for _, record := range result.Records {
		sourcePath := weakPasswordDisplayValue(record.SourcePath, "未记录路径")
		fieldName := weakPasswordDisplayValue(record.FieldPath, "未记录字段")
		sourcePaths = mergeStringLists(sourcePaths, []string{sourcePath})
		fieldNames = mergeStringLists(fieldNames, []string{fieldName})
		recordSources = append(recordSources, map[string]interface{}{
			"source_path": sourcePath,
			"field_name":  fieldName,
			"source_kind": weakPasswordDisplayValue(record.SourceKind, "unknown"),
			"parser":      weakPasswordDisplayValue(record.Parser, "unknown"),
			"account":     record.Account,
			"process_pid": record.ProcessPID,
		})
	}
	for _, item := range result.Errors {
		sourcePaths = mergeStringLists(sourcePaths, []string{item.SourcePath})
	}
	if len(fieldNames) == 0 {
		fieldNames = planExtractorFieldNames(plan)
	}
	if len(sourcePaths) == 0 {
		for _, app := range plan.Applications {
			sourcePaths = mergeStringLists(sourcePaths, app.Paths)
		}
	}
	if len(sourcePaths) == 0 {
		sourcePaths = []string{"未记录路径"}
	}
	if len(fieldNames) == 0 {
		fieldNames = []string{"未记录字段"}
	}
	return map[string]interface{}{
		"record_count":   len(result.Records),
		"error_count":    len(result.Errors),
		"source_path":    strings.Join(sourcePaths, "\n"),
		"field_name":     strings.Join(fieldNames, "\n"),
		"source_paths":   sourcePaths,
		"field_names":    fieldNames,
		"record_sources": recordSources,
	}
}

func planExtractorFieldNames(plan CredentialCollectionPlan) []string {
	fields := []string{}
	for _, app := range plan.Applications {
		for _, extractor := range app.Extractors {
			field := strings.TrimSpace(extractor.PasswordSelector)
			if field == "" {
				field = strings.TrimSpace(extractor.Type)
			}
			if field != "" {
				fields = mergeStringLists(fields, []string{field})
			}
		}
	}
	return fields
}

func collectionProgressSourceAndField(summary datatypes.JSON, fallbackPath, fallbackField string) (string, string) {
	var data map[string]interface{}
	if len(summary) > 0 {
		_ = json.Unmarshal(summary, &data)
	}
	sourcePath := summaryDisplayString(data, "source_path", "source_paths")
	fieldName := summaryDisplayString(data, "field_name", "field_names")
	return weakPasswordDisplayValue(sourcePath, weakPasswordDisplayValue(fallbackPath, "未记录路径")),
		weakPasswordDisplayValue(fieldName, weakPasswordDisplayValue(fallbackField, "未记录字段"))
}

func summaryDisplayString(data map[string]interface{}, scalarKey, listKey string) string {
	if len(data) == 0 {
		return ""
	}
	if value := strings.TrimSpace(fmt.Sprint(data[scalarKey])); value != "" && value != "<nil>" {
		return value
	}
	values := []string{}
	if raw, ok := data[listKey]; ok {
		switch typed := raw.(type) {
		case []interface{}:
			for _, item := range typed {
				if value := strings.TrimSpace(fmt.Sprint(item)); value != "" && value != "<nil>" {
					values = mergeStringLists(values, []string{value})
				}
			}
		case []string:
			values = mergeStringLists(values, typed)
		}
	}
	return strings.Join(values, "\n")
}

func weakPasswordDisplayValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func (s *WeakPasswordService) ListTaskCollectionProgress(taskID uuid.UUID, page, pageSize int) ([]WeakPasswordCollectionProgressDTO, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	app, _ := s.repo.GetScanApplicationByTask(taskID)
	appName := ""
	appToolCalls := 0
	maxToolCalls := 0
	var failedSummary *WeakPasswordCollectionProgressDTO
	if app != nil {
		appName = app.ApplicationName
		appToolCalls = app.AgentToolCallCount
		maxToolCalls = app.MaxAgentToolCalls
		if app.Status == model.AppStatusFailed {
			errorCode := app.ErrorCode
			errorMessage := app.ErrorMessage
			if errorCode == "" || errorMessage == "" {
				if errors, _, err := s.repo.ListCollectionErrors(taskID, 1, 1); err == nil && len(errors) > 0 {
					if errorCode == "" {
						errorCode = errors[0].ErrorCode
					}
					if errorMessage == "" {
						errorMessage = errors[0].ErrorMessage
					}
				}
			}
			if errorCode != "" || errorMessage != "" {
				failedSummary = &WeakPasswordCollectionProgressDTO{
					ID:                 "final-diagnosis-" + app.ID.String(),
					TaskID:             taskID.String(),
					ScanApplicationID:  app.ID.String(),
					HostID:             app.HostID.String(),
					ApplicationName:    appName,
					ToolName:           "WeakPassword.FinalDiagnosis",
					Status:             model.AppStatusFailed,
					Round:              0,
					SourcePath:         weakPasswordDisplayValue(errorMessage, "最终诊断"),
					FieldName:          weakPasswordDisplayValue(errorCode, "失败原因"),
					ErrorCode:          errorCode,
					ErrorMessage:       errorMessage,
					ExecutionTimeMs:    0,
					AgentToolCallCount: appToolCalls,
					MaxAgentToolCalls:  maxToolCalls,
					CreatedAt:          app.UpdatedAt.Format(time.RFC3339),
				}
			}
		}
	}

	totalOffset := (page - 1) * pageSize
	callOffset := totalOffset
	callLimit := pageSize
	items := make([]WeakPasswordCollectionProgressDTO, 0, pageSize)
	if failedSummary != nil {
		if totalOffset == 0 {
			items = append(items, *failedSummary)
			callLimit--
			callOffset = 0
		} else {
			callOffset = totalOffset - 1
		}
	}
	calls := []model.WeakPasswordAgentToolCall{}
	var callTotal int64
	var err error
	if callLimit > 0 {
		calls, callTotal, err = s.repo.ListToolCallsByOffset(taskID, callOffset, callLimit)
		if err != nil {
			return nil, 0, err
		}
	} else {
		_, callTotal, err = s.repo.ListToolCallsByOffset(taskID, 0, 1)
		if err != nil {
			return nil, 0, err
		}
	}
	for idx, call := range calls {
		scanAppID := ""
		if call.ScanApplicationID != nil {
			scanAppID = call.ScanApplicationID.String()
		}
		round := callOffset + idx + 1
		sourcePath, fieldName := collectionProgressSourceAndField(call.ResultSummaryJSON, call.ErrorMessage, call.ErrorCode)
		items = append(items, WeakPasswordCollectionProgressDTO{
			ID:                 call.ID.String(),
			TaskID:             call.TaskID.String(),
			ScanApplicationID:  scanAppID,
			HostID:             call.HostID.String(),
			ApplicationName:    appName,
			ToolName:           call.ToolName,
			Status:             call.Status,
			Round:              round,
			SourcePath:         sourcePath,
			FieldName:          fieldName,
			ErrorCode:          call.ErrorCode,
			ErrorMessage:       call.ErrorMessage,
			ExecutionTimeMs:    call.ExecutionTimeMs,
			AgentToolCallCount: appToolCalls,
			MaxAgentToolCalls:  maxToolCalls,
			CreatedAt:          call.CreatedAt.Format(time.RFC3339),
		})
	}
	total := callTotal
	if failedSummary != nil {
		total++
	}
	return items, total, nil
}

func (s *WeakPasswordService) RetryFailedTask(ctx context.Context, taskID uuid.UUID) error {
	_ = ctx
	app, err := s.repo.GetScanApplicationByTask(taskID)
	if err != nil {
		return err
	}
	if app.AgentToolCallCount >= app.MaxAgentToolCalls {
		app.ErrorCode = model.ErrCodeConfigDiscoveryFailed
		app.ErrorMessage = detectionRoundsExhaustedMessage(app.MaxAgentToolCalls)
		app.Status = model.AppStatusFailed
		app.CurrentStage = model.ErrCodeConfigDiscoveryFailed
		return s.repo.UpdateScanApplication(app)
	}
	app.AgentToolCallCount++
	if app.AgentToolCallCount >= app.MaxAgentToolCalls {
		app.ErrorCode = model.ErrCodeConfigDiscoveryFailed
		app.ErrorMessage = detectionRoundsExhaustedMessage(app.MaxAgentToolCalls)
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

func (s *WeakPasswordService) DeleteTasks(req DeleteWeakPasswordTasksRequest) (*DeleteWeakPasswordTasksResponse, error) {
	resp := &DeleteWeakPasswordTasksResponse{
		Deleted: []string{},
		Skipped: []BatchTaskSkippedItem{},
	}
	seen := map[string]struct{}{}
	for _, rawID := range req.TaskIDs {
		rawID = strings.TrimSpace(rawID)
		if rawID == "" {
			continue
		}
		if _, ok := seen[rawID]; ok {
			continue
		}
		seen[rawID] = struct{}{}
		taskID, err := uuid.Parse(rawID)
		if err != nil {
			resp.Skipped = append(resp.Skipped, BatchTaskSkippedItem{TaskID: rawID, Reason: "invalid_task_id", Message: "任务 ID 格式不正确"})
			continue
		}
		if err := s.DeleteTask(taskID); err != nil {
			reason := "delete_failed"
			message := err.Error()
			if errors.Is(err, ErrWeakPasswordTaskRunning) {
				reason = "task_running"
				message = "运行中的弱密码任务不能删除"
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				reason = "task_not_found"
				message = "任务不存在"
			}
			resp.Skipped = append(resp.Skipped, BatchTaskSkippedItem{TaskID: rawID, Reason: reason, Message: message})
			continue
		}
		resp.Deleted = append(resp.Deleted, rawID)
	}
	resp.Count = len(resp.Deleted)
	return resp, nil
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

func (s *WeakPasswordService) ListDictionaries(page, pageSize int) ([]DictionarySummary, int64, error) {
	items, total, err := s.repo.ListDictionaries(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	summaries := make([]DictionarySummary, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, *dictionarySummary(item))
	}
	return summaries, total, nil
}

func (s *WeakPasswordService) ListDictionaryEntries(dictionaryID uuid.UUID, page, pageSize int) ([]model.WeakPasswordDictionaryEntry, int64, error) {
	return s.repo.ListDictionaryEntriesPaged(dictionaryID, page, pageSize)
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

func (s *WeakPasswordService) GenerateAIDictionary(ctx context.Context, req model.AIGenerateDictionaryRequest, createdBy *uuid.UUID) (*DictionarySummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestedCount := req.Count
	if req.Count <= 0 {
		req.Count = 20
	}
	if req.Count > weakPasswordAIDictionaryRealtimeMaxCount {
		req.Count = weakPasswordAIDictionaryRealtimeMaxCount
	}
	if requestedCount > weakPasswordAIDictionaryRealtimeMaxCount {
		s.logger.Info("AI weak password dictionary count capped for realtime generation",
			zap.Int("requested_count", requestedCount),
			zap.Int("effective_count", req.Count),
			zap.Int("max_count", weakPasswordAIDictionaryRealtimeMaxCount))
	}
	seedWords := extractDictionarySeeds(req.NaturalLanguage)
	seedWords = append(seedWords, req.OrganizationKeywords...)
	seedWords = append(seedWords, req.AccountKeywords...)
	if req.ApplicationType != "" {
		seedWords = append(seedWords, req.ApplicationType)
	}
	if len(seedWords) == 0 {
		seedWords = []string{"admin", "root", "service", "aegis"}
	}

	llmModel := s.activeLLMModelName("configured-llm")
	aiReq := req
	if req.DeduplicateWithDefault && req.Count < weakPasswordAIDictionaryLLMMaxCount {
		aiReq.Count = req.Count * 2
		if aiReq.Count > weakPasswordAIDictionaryLLMMaxCount {
			aiReq.Count = weakPasswordAIDictionaryLLMMaxCount
		}
	}
	generationCtx, cancel := context.WithTimeout(ctx, weakPasswordAIDictionaryGenerationTimeout)
	defer cancel()
	candidates, err := s.generateDictionaryWithAI(generationCtx, aiReq, seedWords)
	if err != nil {
		s.logger.Warn("AI dictionary generation failed", zap.Error(err))
		return nil, fmt.Errorf("AI生成密码失败，未保存字典: %w", err)
	}

	if req.DeduplicateWithDefault {
		candidates = s.deduplicateWithDefaultDictionary(candidates, req.Count)
	}
	candidates = uniqueStrings(candidates, req.Count)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("AI生成密码失败，未返回可用密码候选")
	}
	dict := &model.WeakPasswordDictionary{
		ID:                   uuid.New(),
		Name:                 "AI 生成弱密码字典 - " + time.Now().Format("20060102150405"),
		DictionaryType:       model.DictTypeAIGenerated,
		Status:               "enabled",
		EntryCount:           len(candidates),
		Source:               "ai_generated",
		Categories:           mustJSON([]string{"AI 一键生成字典"}),
		GenerationPolicyJSON: mustJSON(req),
		PromptSummary:        promptSummaryFromNaturalLanguage(req.NaturalLanguage),
		LLMModel:             llmModel,
		CreatedBy:            createdBy,
	}
	if err := s.repo.CreateDictionary(dict, buildDictionaryEntries(dict.ID, candidates)); err != nil {
		return nil, err
	}
	return dictionarySummary(*dict), nil
}

// generateDictionaryWithAI uses LLM to generate weak password candidates based on user input
func (s *WeakPasswordService) generateDictionaryWithAI(ctx context.Context, req model.AIGenerateDictionaryRequest, seedWords []string) ([]string, error) {
	systemPrompt := `你是一个安全专家，负责生成弱密码字典用于安全检测。
你的任务是根据用户提供的信息，生成一组可能被用作弱密码的候选密码。

生成规则：
1. 基于用户提供的关键词、组织名称、账号名称等信息
2. 结合常见的弱密码模式（如：密码+年份、密码+特殊字符等）
3. 考虑应用类型的特点（如数据库默认密码、中间件默认密码等）
4. 生成的密码应该多样化，包含不同长度和复杂度
5. 每个密码应该是一个可能被实际使用的弱密码

重要：只返回密码列表，不要返回其他解释信息。

请以JSON格式返回结果：
{
  "passwords": ["password1", "password2", ...]
}`

	// Build user prompt from request
	var userPromptParts []string
	if req.NaturalLanguage != "" {
		userPromptParts = append(userPromptParts, "用户描述："+req.NaturalLanguage)
	}
	if len(req.OrganizationKeywords) > 0 {
		userPromptParts = append(userPromptParts, "组织关键词："+strings.Join(req.OrganizationKeywords, ", "))
	}
	if len(req.AccountKeywords) > 0 {
		userPromptParts = append(userPromptParts, "账号关键词："+strings.Join(req.AccountKeywords, ", "))
	}
	if req.ApplicationType != "" {
		userPromptParts = append(userPromptParts, "应用类型："+req.ApplicationType)
	}
	if len(seedWords) > 0 {
		userPromptParts = append(userPromptParts, "种子词："+strings.Join(seedWords, ", "))
	}
	userPromptParts = append(userPromptParts, fmt.Sprintf("需要生成 %d 个弱密码候选", req.Count))

	userPrompt := strings.Join(userPromptParts, "\n")

	// Get LLM client
	llmClient, err := s.getLLMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM client: %w", err)
	}

	// Call LLM
	response, err := llmClient.ChatCompletion(ctx, systemPrompt, userPrompt, 0.7)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Parse response
	var result struct {
		Passwords []string `json:"passwords"`
	}

	// Try to extract JSON from response
	jsonStr := response
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")
	if startIdx >= 0 && endIdx > startIdx {
		jsonStr = response[startIdx : endIdx+1]
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		s.logger.Warn("failed to parse AI dictionary generation response",
			zap.String("response", response),
			zap.Error(err))
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	if len(result.Passwords) == 0 {
		return nil, fmt.Errorf("AI generated empty password list")
	}

	s.logger.Info("AI dictionary generation successful",
		zap.Int("requested_count", req.Count),
		zap.Int("generated_count", len(result.Passwords)))

	return result.Passwords, nil
}

func (s *WeakPasswordService) activeLLMModelName(fallback string) string {
	if s.configRepo == nil {
		return fallback
	}
	config, err := s.configRepo.GetActive()
	if err != nil || config == nil || strings.TrimSpace(config.ModelName) == "" {
		return fallback
	}
	return config.ModelName
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
	skill := weakPasswordSkillForApplication(appType)
	paths := weakJSONStrings(asset.ConfigPaths)
	if len(paths) == 0 && asset.StartPath != "" {
		paths = []string{asset.StartPath}
	}
	paths = mergeCredentialPaths(paths, skill.CandidatePaths)
	profileID, credentialTypes, extractors := profileForApplication(appType)
	reason := "应用资产包含可能承载认证配置的配置路径或启动信息"
	if skill.Reason != "" {
		reason = skill.Reason
	}
	if len(paths) == 0 {
		reason = "应用资产存在，但尚未采集到配置路径；检查时可能需要受控辅助定位"
	}
	assetID := asset.ID
	relatedPIDs := weakJSONInts(asset.RelatedPIDs)
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
			"related_pids": relatedPIDs,
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
	relatedPIDs := candidateRelatedPIDs(candidate)
	return CredentialCollectionPlan{
		TaskID: taskID.String(),
		PlanID: planID.String(),
		HostID: candidate.HostID.String(),
		Applications: []CredentialApplication{{
			Application: candidate.ApplicationType,
			AssetID:     optionalUUIDString(candidate.AssetID),
			ProfileID:   candidate.ProfileID,
			Paths:       paths,
			RelatedPIDs: relatedPIDs,
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
	skill := weakPasswordSkillForApplication(appType)
	return skill.ProfileID, skill.CredentialTypes, skill.Extractors
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
		ScanStatus:             "unscanned",
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
	case strings.Contains(lower, "mongo"):
		return "mongodb"
	case strings.Contains(lower, "elastic"):
		return "elasticsearch"
	case strings.Contains(lower, "sql server") || strings.Contains(lower, "mssql"):
		return "sqlserver"
	case strings.Contains(lower, "oracle"):
		return "oracle"
	case strings.Contains(lower, "openssh") || lower == "ssh" || lower == "sshd":
		return "openssh"
	case strings.Contains(lower, "tomcat") || strings.Contains(lower, "catalina"):
		return "tomcat"
	case strings.Contains(lower, "spring boot") || strings.Contains(lower, "spring-boot") || strings.Contains(lower, "springboot"):
		return "spring_boot"
	case strings.Contains(lower, "django"):
		return "django"
	case strings.Contains(lower, "flask"):
		return "flask"
	case strings.Contains(lower, "laravel"):
		return "laravel"
	case strings.Contains(lower, "express"):
		return "express"
	case strings.Contains(lower, "weblogic"):
		return "weblogic"
	case strings.Contains(lower, "wildfly"):
		return "wildfly"
	case strings.Contains(lower, "jboss"):
		return "jboss"
	case strings.Contains(lower, "rabbitmq"):
		return "rabbitmq"
	case strings.Contains(lower, "zookeeper") || strings.Contains(lower, "zoo keeper"):
		return "zookeeper"
	case strings.Contains(lower, "kafka"):
		return "kafka"
	case strings.Contains(lower, "vsftpd") || strings.Contains(lower, "proftpd") || lower == "ftp":
		return "ftp"
	case strings.Contains(lower, "nginx"):
		return "nginx"
	case strings.Contains(lower, "apache") || strings.Contains(lower, "httpd"):
		return "apache"
	case strings.Contains(lower, "phpmyadmin"):
		return "phpmyadmin"
	case strings.Contains(lower, "grafana"):
		return "grafana"
	case strings.Contains(lower, "kibana"):
		return "kibana"
	case strings.Contains(lower, "jenkins"):
		return "jenkins"
	case strings.Contains(lower, "gitlab"):
		return "gitlab"
	case strings.Contains(lower, "harbor"):
		return "harbor"
	case strings.Contains(lower, "sonarqube") || strings.Contains(lower, "sonar"):
		return "sonarqube"
	case strings.Contains(lower, "nexus"):
		return "nexus"
	case strings.Contains(lower, "openldap"):
		return "openldap"
	case strings.Contains(lower, "active directory"):
		return "active_directory"
	case strings.Contains(lower, "ollama"):
		return "ollama"
	case strings.Contains(lower, "localai") || strings.Contains(lower, "local ai"):
		return "localai"
	case strings.Contains(lower, "vllm"):
		return "vllm"
	case lower == "database":
		return "mysql"
	case lower == "ai_agent" || lower == "ai-agent":
		return "ai_agent"
	case lower == "mcp_server" || lower == "mcp-server":
		return "mcp_server"
	case lower == "llm_service" || lower == "llm-service":
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

func weakJSONInts(data datatypes.JSON) []int {
	if len(data) == 0 {
		return nil
	}
	var values []int
	if err := json.Unmarshal(data, &values); err == nil {
		return values
	}
	var floats []float64
	if err := json.Unmarshal(data, &floats); err == nil {
		out := make([]int, 0, len(floats))
		for _, value := range floats {
			if value > 0 {
				out = append(out, int(value))
			}
		}
		return out
	}
	var stringsValue []string
	if err := json.Unmarshal(data, &stringsValue); err == nil {
		out := make([]int, 0, len(stringsValue))
		for _, value := range stringsValue {
			var parsed int
			if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil && parsed > 0 {
				out = append(out, parsed)
			}
		}
		return out
	}
	return nil
}

func candidateRelatedPIDs(candidate model.WeakPasswordCandidateApplication) []int {
	var evidence map[string]interface{}
	if len(candidate.AssetEvidenceJSON) == 0 || json.Unmarshal(candidate.AssetEvidenceJSON, &evidence) != nil {
		return nil
	}
	return intSliceFromInterface(evidence["related_pids"])
}

func processPIDFromEvidence(data datatypes.JSON) int {
	if len(data) == 0 {
		return 0
	}
	var evidence map[string]interface{}
	if err := json.Unmarshal(data, &evidence); err != nil {
		return 0
	}
	values := intSliceFromInterface(evidence["process_pid"])
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func shouldAttemptProcessBasedRepair(plan CredentialCollectionPlan, errors []AgentCollectionError) bool {
	if len(plan.Applications) == 0 || len(plan.Applications[0].RelatedPIDs) == 0 {
		return false
	}
	if len(plan.Applications[0].Paths) == 0 || len(errors) == 0 {
		return true
	}
	for _, item := range errors {
		if !item.Retryable {
			continue
		}
		switch item.ErrorCode {
		case "file_not_found", "field_not_found", "unsupported_credential_format":
			return true
		}
	}
	return false
}

func hintConfigCandidates(hints AgentProcessConfigHintsResult) []string {
	paths := make([]string, 0)
	paths = append(paths, hints.ConfigPathCandidates...)
	paths = append(paths, hints.ContainerConfigFiles...)
	paths = append(paths, hints.OpenConfigFiles...)
	return mergeCredentialPaths(nil, paths)
}

func normalizeRepairToolArguments(tool string, args map[string]interface{}, app CredentialApplication) (map[string]interface{}, error) {
	if args == nil {
		args = map[string]interface{}{}
	}
	normalized := make(map[string]interface{}, len(args)+4)
	for key, value := range args {
		normalized[key] = value
	}
	relatedPIDs := uniquePositiveInts(app.RelatedPIDs)
	switch tool {
	case "WeakPassword.ProcessConfigHints":
		pid := repairToolPID(normalized["pid"], relatedPIDs)
		if pid <= 0 {
			return nil, fmt.Errorf("WeakPassword.ProcessConfigHints requires a related pid")
		}
		normalized["pid"] = pid
		if _, ok := normalized["application"]; !ok || strings.TrimSpace(fmt.Sprint(normalized["application"])) == "" {
			normalized["application"] = app.Application
		}
		if _, ok := normalized["include_open_files"]; !ok {
			normalized["include_open_files"] = true
		}
		if _, ok := normalized["file_suffix_allowlist"]; !ok {
			normalized["file_suffix_allowlist"] = credentialConfigSuffixAllowlist(app.Application)
		}
		if _, ok := normalized["max_files"]; !ok {
			normalized["max_files"] = 20
		}
	case "WeakPassword.ProbePath", "WeakPassword.ListConfigDir", "WeakPassword.ReadConfigSlice":
		if len(relatedPIDs) > 0 {
			normalized["pid"] = repairToolPID(normalized["pid"], relatedPIDs)
		}
		if tool == "WeakPassword.ListConfigDir" {
			if _, ok := normalized["suffix_allowlist"]; !ok {
				normalized["suffix_allowlist"] = credentialConfigSuffixAllowlist(app.Application)
			}
			if _, ok := normalized["max_entries"]; !ok {
				normalized["max_entries"] = 50
			}
			delete(normalized, "max_depth")
			normalized["recursive"] = false
		}
	case "WeakPassword.ServiceUnitInspect":
		if _, ok := normalized["service"]; !ok {
			if serviceName, ok := normalized["service_name"]; ok {
				normalized["service"] = serviceName
				delete(normalized, "service_name")
			}
		}
	}
	return normalized, nil
}

func repairToolPID(value interface{}, relatedPIDs []int) int {
	values := intSliceFromInterface(value)
	if len(relatedPIDs) == 0 {
		if len(values) > 0 {
			return values[0]
		}
		return 0
	}
	allowed := make(map[int]struct{}, len(relatedPIDs))
	for _, pid := range relatedPIDs {
		allowed[pid] = struct{}{}
	}
	for _, pid := range values {
		if _, ok := allowed[pid]; ok {
			return pid
		}
	}
	return relatedPIDs[0]
}

func mergeCredentialPaths(existing []string, discovered []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(existing)+len(discovered))
	add := func(path string) {
		path = strings.TrimSpace(path)
		if !isSafeCredentialPath(path) {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	for _, path := range existing {
		add(path)
	}
	for _, path := range discovered {
		add(path)
	}
	return out
}

func mergeStringLists(existing []string, discovered []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(existing)+len(discovered))
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range existing {
		add(value)
	}
	for _, value := range discovered {
		add(value)
	}
	return out
}

func mergeCredentialExtractors(existing []CredentialExtractor, discovered []CredentialExtractor) []CredentialExtractor {
	seen := map[string]struct{}{}
	out := make([]CredentialExtractor, 0, len(existing)+len(discovered))
	add := func(extractor CredentialExtractor) {
		extractor.Type = strings.TrimSpace(extractor.Type)
		extractor.Section = strings.TrimSpace(extractor.Section)
		extractor.AccountSelector = strings.TrimSpace(extractor.AccountSelector)
		extractor.PasswordSelector = strings.TrimSpace(extractor.PasswordSelector)
		extractor.FormatHint = strings.TrimSpace(extractor.FormatHint)
		extractor.SourceKind = strings.TrimSpace(extractor.SourceKind)
		if !isUsableCredentialExtractor(extractor) {
			return
		}
		key := credentialExtractorKey(extractor)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, extractor)
	}
	for _, extractor := range existing {
		add(extractor)
	}
	for _, extractor := range discovered {
		add(extractor)
	}
	return out
}

func isUsableCredentialExtractor(extractor CredentialExtractor) bool {
	switch extractor.Type {
	case "line_key_value", "ini", "properties", "yaml", "json", "shadow", "htpasswd", "tomcat_users_xml":
	default:
		return false
	}
	if extractor.Type == "shadow" || extractor.Type == "htpasswd" {
		return true
	}
	return extractor.PasswordSelector != ""
}

func credentialExtractorKey(extractor CredentialExtractor) string {
	return strings.Join([]string{
		extractor.Type,
		extractor.Section,
		extractor.AccountSelector,
		extractor.PasswordSelector,
		extractor.FormatHint,
		extractor.SourceKind,
	}, "\x00")
}

func isSafeCredentialPath(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	if filepath.Clean(path) != path || strings.Contains(path, "..") {
		return false
	}
	for _, token := range []string{";", "|", "&", "`", "$(", "\n", "\r"} {
		if strings.Contains(path, token) {
			return false
		}
	}
	return !strings.ContainsAny(path, "*?[]")
}

func credentialConfigSuffixAllowlist(application string) []string {
	common := []string{".conf", ".cnf", ".ini", ".yaml", ".yml", ".json", ".properties", ".toml", ".env", ".xml", ".db", ".passwd"}
	switch normalizeApplicationType(application) {
	case "nginx", "apache", "web_service":
		return append(common, ".htpasswd")
	default:
		return common
	}
}

func uniquePositiveInts(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func intSliceFromInterface(value interface{}) []int {
	switch typed := value.(type) {
	case []int:
		return typed
	case int:
		if typed > 0 {
			return []int{typed}
		}
	case float64:
		if typed > 0 {
			return []int{int(typed)}
		}
	case []interface{}:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			out = append(out, intSliceFromInterface(item)...)
		}
		return out
	case []float64:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			if item > 0 {
				out = append(out, int(item))
			}
		}
		return out
	case []string:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			var parsed int
			if _, err := fmt.Sscanf(item, "%d", &parsed); err == nil && parsed > 0 {
				out = append(out, parsed)
			}
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
	if strings.HasPrefix(hash, "$y$") {
		if matched := verifyYescryptHashAgainstCandidates(hash, candidates); matched != "" {
			return matched
		}
	}
	if crypt.IsHashSupported(hash) {
		crypter := crypt.NewFromHash(hash)
		for _, candidate := range candidates {
			if crypter.Verify(hash, []byte(candidate)) == nil {
				return candidate
			}
		}
	}
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		for _, candidate := range candidates {
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(candidate)) == nil {
				return candidate
			}
		}
	}
	if strings.HasPrefix(hash, "{SHA}") {
		expected := strings.TrimPrefix(hash, "{SHA}")
		for _, candidate := range candidates {
			sum := sha1.Sum([]byte(candidate))
			if base64.StdEncoding.EncodeToString(sum[:]) == expected {
				return candidate
			}
		}
	}
	lowerHash := strings.ToLower(strings.TrimSpace(hash))
	if len(lowerHash) == 32 || len(lowerHash) == 40 || len(lowerHash) == 64 {
		for _, candidate := range candidates {
			switch len(lowerHash) {
			case 32:
				sum := md5.Sum([]byte(candidate))
				if hex.EncodeToString(sum[:]) == lowerHash {
					return candidate
				}
			case 40:
				sum := sha1.Sum([]byte(candidate))
				if hex.EncodeToString(sum[:]) == lowerHash {
					return candidate
				}
			case 64:
				sum := sha256.Sum256([]byte(candidate))
				if hex.EncodeToString(sum[:]) == lowerHash {
					return candidate
				}
			}
		}
	}
	return ""
}

func verifyYescryptHashAgainstCandidates(hash string, candidates []string) string {
	hashBytes := []byte(hash)
	for _, candidate := range candidates {
		encoded, err := yescrypt.Hash([]byte(candidate), hashBytes)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare(encoded, hashBytes) == 1 {
			return candidate
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
		LLMModel:    dict.LLMModel,
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
	}
	for _, seed := range seeds {
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
	// 生成多样化的密码，避免简单的顺序递增
	commonPasswords := []string{
		"Password1!", "Qwerty123", "Letmein1!", "Welcome1!",
		"Changeme1!", "Default1!", "Test1234!", "Service1!",
		"Aegis123!", "Security1!", "Access123!", "System1!",
		"Master1!", "Control1!", "Server123!", "Database1!",
		"Application1!", "Network1!", "Admin1234!", "Root1234!",
		"P@ssw0rd!", "Passw0rd1!", "Adm1n123!", "T3st1234!",
		"S3cur1ty!", "M@nag3r!", "0perat0r!", "Sup3rv1s0r!",
		"Manager1!", "Operator1!", "Supervisor1!", "Administrator1!",
		"SuperUser1!", "RootUser1!", "AdminUser1!", "TestUser1!",
		"GuestUser1!", "ServiceAccount1!", "BackupUser1!", "Monitor1!",
		"Deploy1!", "DevOps1!", "Developer1!", "Tester1!",
		"Production1!", "Staging1!", "Development1!", "Testing1!",
		"MainServer1!", "WebServer1!", "AppServer1!", "DbServer1!",
		"CacheServer1!", "FileServer1!", "MailServer1!", "DnsServer1!",
		"ProxyServer1!", "LoadBalancer1!", "Firewall1!", "Gateway1!",
	}
	for _, pwd := range commonPasswords {
		add(pwd)
		if len(out) >= limit {
			break
		}
	}
	// 基于种子词生成更多变体
	suffixes := []string{
		"123!", "@123", "#2024", "!2025", "@2026",
		"Pass!", "Pwd!", "Key!", "Auth!", "Login!",
		"Admin!", "Root!", "User!", "Test!", "Dev!",
	}
	for _, seed := range seeds {
		for _, suffix := range suffixes {
			if len(out) >= limit {
				break
			}
			add(seed + suffix)
			add(strings.Title(strings.ToLower(seed)) + suffix)
			add(strings.ToUpper(seed) + suffix)
		}
		if len(out) >= limit {
			break
		}
	}
	// Leet speak 变体
	leetReplacer := strings.NewReplacer("a", "@", "A", "@", "o", "0", "O", "0", "i", "1", "I", "1", "e", "3", "E", "3", "s", "5", "S", "5", "t", "7", "T", "7")
	for _, seed := range seeds {
		if len(out) >= limit {
			break
		}
		leet := leetReplacer.Replace(seed)
		if leet != seed {
			add(leet + "123!")
			add(leet + "@2024")
			add(strings.ToUpper(leet) + "!")
		}
	}
	// 年份变体
	allYears := []string{"2020", "2021", "2022", "2023", "2024", "2025", "2026"}
	for _, seed := range seeds {
		for _, year := range allYears {
			if len(out) >= limit {
				break
			}
			add(seed + year)
			add(seed + "@" + year)
			add(seed + "#" + year)
			add(strings.Title(strings.ToLower(seed)) + year)
		}
		if len(out) >= limit {
			break
		}
	}
	shuffleDictionaryCandidates(out, seeds)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func shuffleDictionaryCandidates(values []string, seeds []string) {
	salt := strings.Join(seeds, "|")
	sort.SliceStable(values, func(i, j int) bool {
		left := sha256.Sum256([]byte(salt + "\x00" + values[i]))
		right := sha256.Sum256([]byte(salt + "\x00" + values[j]))
		return hex.EncodeToString(left[:]) < hex.EncodeToString(right[:])
	})
}

func extractDictionarySeeds(naturalLanguage string) []string {
	text := strings.TrimSpace(naturalLanguage)
	if text == "" {
		return nil
	}
	replacer := strings.NewReplacer(
		"，", " ", "。", " ", "、", " ", "；", " ", "：", " ",
		",", " ", ".", " ", ";", " ", ":", " ", "/", " ", "\\", " ",
		"(", " ", ")", " ", "（", " ", "）", " ", "[", " ", "]", " ",
		"{", " ", "}", " ", "\n", " ", "\t", " ",
	)
	words := strings.Fields(replacer.Replace(text))
	stopWords := map[string]struct{}{
		"为": {}, "和": {}, "或": {}, "生成": {}, "弱密码": {}, "字典": {}, "包含": {}, "常见": {}, "密码": {}, "管理员": {}, "生产": {}, "环境": {},
		"the": {}, "and": {}, "or": {}, "for": {}, "with": {}, "password": {}, "dictionary": {}, "weak": {}, "common": {},
	}
	seeds := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.Trim(word, `"'!@#$%^&*_+-=<>?`)
		if len([]rune(word)) < 2 {
			continue
		}
		if _, ok := stopWords[strings.ToLower(word)]; ok {
			continue
		}
		seeds = append(seeds, word)
	}
	return uniqueStrings(seeds, 32)
}

func promptSummaryFromNaturalLanguage(naturalLanguage string) string {
	naturalLanguage = strings.TrimSpace(naturalLanguage)
	if naturalLanguage == "" {
		return "generate_weak_password_dictionary; llm_only"
	}
	runes := []rune(naturalLanguage)
	if len(runes) > 120 {
		naturalLanguage = string(runes[:120])
	}
	return "generate_weak_password_dictionary; natural_language=" + naturalLanguage
}

func (s *WeakPasswordService) deduplicateWithDefaultDictionary(candidates []string, limit int) []string {
	defaultDict, err := s.repo.GetDefaultDictionary()
	if err != nil {
		return uniqueStrings(candidates, limit)
	}
	entries, err := s.repo.ListDictionaryEntries([]uuid.UUID{defaultDict.ID})
	if err != nil {
		return uniqueStrings(candidates, limit)
	}
	defaults := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		defaults[entry.Candidate] = struct{}{}
	}
	out := make([]string, 0, limit)
	for _, candidate := range candidates {
		if _, ok := defaults[candidate]; ok {
			continue
		}
		out = append(out, candidate)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return uniqueStrings(candidates, limit)
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

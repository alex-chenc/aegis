package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	applicationAnalysisProcessBatchSize = 50
	applicationAnalysisBatchTimeout     = 90 * time.Second
)

// AssetAnalysisService 资产分析服务
type AssetAnalysisService struct {
	repo         *repository.AssetCollectionRepository
	configRepo   ConfigRepositoryInterface
	serverClient ServerClientInterface
	logger       *zap.Logger
}

// ConfigRepositoryInterface 配置仓库接口
type ConfigRepositoryInterface interface {
	GetActive() (*model.LLMConfig, error)
	DecryptAPIKey(encrypted string) (string, error)
}

// NewAssetAnalysisService 创建资产分析服务
func NewAssetAnalysisService(
	repo *repository.AssetCollectionRepository,
	configRepo ConfigRepositoryInterface,
	serverClient ServerClientInterface,
	logger *zap.Logger,
) *AssetAnalysisService {
	return &AssetAnalysisService{
		repo:         repo,
		configRepo:   configRepo,
		serverClient: serverClient,
		logger:       logger,
	}
}

// AnalyzeHostApplications 分析主机应用
func (s *AssetAnalysisService) AnalyzeHostApplications(ctx context.Context, taskID uuid.UUID, hostID uuid.UUID, snapshot HostAssetSnapshot) (int, error) {
	s.logger.Info("Starting application analysis",
		zap.String("task_id", taskID.String()),
		zap.String("host_id", hostID.String()),
		zap.Int("process_count", len(snapshot.Processes)),
		zap.Int("batch_size", applicationAnalysisProcessBatchSize))

	if len(snapshot.Processes) == 0 {
		s.logger.Info("Application analysis skipped because process snapshot is empty",
			zap.String("host_id", hostID.String()))
		return 0, nil
	}

	// 获取 LLM 配置
	if s.configRepo == nil {
		return s.saveDeterministicApplicationAnalysisResult(hostID, snapshot, "llm_config_repository_unavailable")
	}
	llmConfig, err := s.configRepo.GetActive()
	if err != nil {
		s.logger.Warn("LLM config unavailable, using deterministic application analysis fallback",
			zap.String("task_id", taskID.String()),
			zap.String("host_id", hostID.String()),
			zap.Error(err))
		return s.saveDeterministicApplicationAnalysisResult(hostID, snapshot, "llm_config_unavailable")
	}

	// 解密 API Key
	apiKey, err := s.configRepo.DecryptAPIKey(llmConfig.APIKeyEncrypted)
	if err != nil {
		s.logger.Warn("LLM API key unavailable, using deterministic application analysis fallback",
			zap.String("task_id", taskID.String()),
			zap.String("host_id", hostID.String()),
			zap.Error(err))
		return s.saveDeterministicApplicationAnalysisResult(hostID, snapshot, "llm_api_key_unavailable")
	}

	// 创建 LLM 客户端
	llmClient := llm.NewLLMClient(
		apiKey,
		llmConfig.BaseURL,
		llmConfig.ModelName,
		60, // timeout seconds
		3,  // max retries
	)

	// 创建工具执行器
	toolExecutor := &assetToolExecutor{
		serverClient: s.serverClient,
		hostID:       hostID.String(),
		logger:       s.logger,
	}

	batches := splitProcessBatches(snapshot.Processes, applicationAnalysisProcessBatchSize)
	totalSaved := 0
	failedBatches := 0
	var firstBatchErr error

	for i, batch := range batches {
		chunkSnapshot := snapshot
		chunkSnapshot.Processes = batch

		prompt := s.buildAnalysisPrompt(chunkSnapshot, i+1, len(batches))
		batchCtx, cancel := context.WithTimeout(ctx, applicationAnalysisBatchTimeout)
		response, err := s.analyzeWithReAct(batchCtx, llmClient, toolExecutor, prompt)
		cancel()
		if err != nil {
			failedBatches++
			if firstBatchErr == nil {
				firstBatchErr = err
			}
			s.logger.Warn("Application analysis batch failed",
				zap.String("task_id", taskID.String()),
				zap.String("host_id", hostID.String()),
				zap.Int("batch", i+1),
				zap.Int("total_batches", len(batches)),
				zap.Duration("timeout", applicationAnalysisBatchTimeout),
				zap.Error(err))
			continue
		}

		result, err := s.parseAnalysisResult(response)
		if err != nil {
			failedBatches++
			if firstBatchErr == nil {
				firstBatchErr = err
			}
			s.logger.Warn("Application analysis batch parse failed",
				zap.String("task_id", taskID.String()),
				zap.String("host_id", hostID.String()),
				zap.Int("batch", i+1),
				zap.Int("total_batches", len(batches)),
				zap.String("response_preview", truncateForLog(response, 500)),
				zap.Error(err))
			continue
		}

		savedCount, err := s.saveApplicationAnalysisResult(hostID, chunkSnapshot, result)
		if err != nil {
			failedBatches++
			if firstBatchErr == nil {
				firstBatchErr = err
			}
			s.logger.Warn("Application analysis batch save failed",
				zap.String("task_id", taskID.String()),
				zap.String("host_id", hostID.String()),
				zap.Int("batch", i+1),
				zap.Int("total_batches", len(batches)),
				zap.Error(err))
			continue
		}

		totalSaved += savedCount
		s.logger.Info("Application analysis batch completed",
			zap.String("task_id", taskID.String()),
			zap.String("host_id", hostID.String()),
			zap.Int("batch", i+1),
			zap.Int("total_batches", len(batches)),
			zap.Int("processes", len(batch)),
			zap.Int("applications", savedCount))
	}

	if failedBatches == len(batches) && firstBatchErr != nil {
		fallbackSaved, fallbackErr := s.saveDeterministicApplicationAnalysisResult(hostID, snapshot, "llm_batches_failed")
		if fallbackErr == nil && fallbackSaved > 0 {
			return fallbackSaved, nil
		}
		return 0, fmt.Errorf("all application analysis batches failed: %w", firstBatchErr)
	}
	if fallbackSaved, err := s.saveDeterministicApplicationAnalysisResult(hostID, snapshot, "deterministic_known_application_merge"); err == nil && fallbackSaved > 0 {
		totalSaved += fallbackSaved
	} else if err != nil {
		s.logger.Warn("deterministic application merge failed",
			zap.String("task_id", taskID.String()),
			zap.String("host_id", hostID.String()),
			zap.Error(err))
	}

	s.logger.Info("Application analysis completed",
		zap.String("host_id", hostID.String()),
		zap.Int("applications", totalSaved),
		zap.Int("failed_batches", failedBatches),
		zap.Int("total_batches", len(batches)))

	return totalSaved, nil
}

// buildAnalysisPrompt 构建分析 prompt
func (s *AssetAnalysisService) buildAnalysisPrompt(snapshot HostAssetSnapshot, batchIndex int, batchTotal int) string {
	var sb strings.Builder

	sb.WriteString("## 主机信息\n")
	sb.WriteString(fmt.Sprintf("- 主机名: %s\n", snapshot.Hostname))
	sb.WriteString(fmt.Sprintf("- IP: %s\n", snapshot.IPAddress))
	sb.WriteString(fmt.Sprintf("- 操作系统: %s %s\n", snapshot.OSType, snapshot.OSVersion))
	sb.WriteString(fmt.Sprintf("- 架构: %s\n", snapshot.Arch))

	sb.WriteString("\n## 进程快照分片\n")
	sb.WriteString(fmt.Sprintf("- 当前分片: %d/%d\n", batchIndex, batchTotal))
	sb.WriteString(fmt.Sprintf("- 本分片进程数: %d\n", len(snapshot.Processes)))

	sb.WriteString("\n## 进程列表\n")
	for _, proc := range snapshot.Processes {
		containerInfo := containerInfoFromProcess(proc)
		sb.WriteString(fmt.Sprintf("- PID: %d, Comm: %s, Exe: %s, Cwd: %s, User: %s, Ports: %v, Container: %t",
			proc.PID, proc.Comm, truncateForPrompt(proc.ExePath, 200), truncateForPrompt(proc.Cwd, 200), proc.Username, proc.ListenPorts, containerInfo.IsContainer))
		if containerInfo.Runtime != "" || containerInfo.ID != "" {
			sb.WriteString(fmt.Sprintf(" runtime=%s id=%s", containerInfo.Runtime, containerInfo.ID))
		}
		sb.WriteString("\n")
		if proc.Cmdline != "" {
			sb.WriteString(fmt.Sprintf("  Cmdline: %s\n", truncateForPrompt(proc.Cmdline, 300)))
		}
		if len(proc.Cgroup) > 0 {
			sb.WriteString(fmt.Sprintf("  Cgroup: %s\n", truncateForPrompt(strings.Join(proc.Cgroup, " | "), 500)))
		}
	}

	return sb.String()
}

func (s *AssetAnalysisService) completeApplicationAnalysis(ctx context.Context, llmClient *llm.LLMClient, prompt string) (string, error) {
	messages := []llm.Message{
		{Role: "system", Content: applicationAnalysisSystemPrompt},
		{Role: "user", Content: prompt},
	}

	responseFormat := &llm.ResponseFormat{Type: "json_object"}
	response, err := llmClient.ChatCompletionWithMessagesFormat(ctx, messages, 0.1, responseFormat)
	if err == nil {
		return response, nil
	}

	s.logger.Warn("LLM JSON response format failed, retrying without response_format",
		zap.Error(err))
	return llmClient.ChatCompletionWithMessages(ctx, messages, 0.1)
}

const maxReActIterations = 10

// analyzeWithReAct 使用 ReAct 循环进行应用分析，支持工具调用
func (s *AssetAnalysisService) analyzeWithReAct(ctx context.Context, llmClient *llm.LLMClient, toolExecutor *assetToolExecutor, prompt string) (string, error) {
	messages := []llm.Message{
		{Role: "system", Content: applicationAnalysisSystemPrompt},
		{Role: "user", Content: prompt},
	}

	// 跟踪已调用的工具，优先按 pid 限频；没有 pid 的工具按目标路径限频。
	calledTools := make(map[string]bool)

	for iteration := 0; iteration < maxReActIterations; iteration++ {
		// 调用 LLM
		response, err := llmClient.ChatCompletionWithMessages(ctx, messages, 0.1)
		if err != nil {
			return "", fmt.Errorf("LLM call failed at iteration %d: %w", iteration, err)
		}

		s.logger.Debug("LLM response received",
			zap.Int("iteration", iteration),
			zap.Int("response_length", len(response)))

		// 检查是否是 Final Answer
		if strings.Contains(response, "Final Answer:") {
			finalAnswer := extractFinalAnswer(response)
			if finalAnswer != "" {
				return finalAnswer, nil
			}
		}

		// 解析工具调用
		toolCall, err := parseToolCall(response)
		if err != nil {
			messages = append(messages, llm.Message{Role: "assistant", Content: response})
			messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("Observation: 工具调用格式错误: %v。请使用指定资产工具和合法 JSON 参数，或输出 Final Answer。", err)})
			continue
		}
		if toolCall == nil {
			// 没有工具调用，尝试从响应中提取 JSON
			jsonStr := extractJSONFromResponse(response)
			if jsonStr != "" {
				return jsonStr, nil
			}
			// 将响应加入历史继续
			messages = append(messages, llm.Message{Role: "assistant", Content: response})
			messages = append(messages, llm.Message{Role: "user", Content: "请继续分析，或者输出 Final Answer。"})
			continue
		}

		// 检查频率限制
		callKey := assetToolCallKey(toolCall.Tool, toolCall.Args)
		if calledTools[callKey] {
			// 已调用过，告诉 LLM
			messages = append(messages, llm.Message{Role: "assistant", Content: response})
			messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("Observation: 工具 %s 对该进程已经调用过，请尝试其他工具或直接输出 Final Answer。", toolCall.Tool)})
			continue
		}
		calledTools[callKey] = true

		// 执行工具
		s.logger.Info("Executing tool",
			zap.String("tool", toolCall.Tool),
			zap.Any("args", toolCall.Args),
			zap.Int("iteration", iteration))

		result, err := toolExecutor.Execute(ctx, toolCall.Tool, toolCall.Args)
		observation := ""
		if err != nil {
			observation = fmt.Sprintf("Error: %v", err)
		} else {
			resultJSON, _ := json.Marshal(result)
			observation = string(resultJSON)
			// 截断过长的结果
			if len(observation) > 4000 {
				observation = observation[:4000] + "...[truncated]"
			}
		}

		s.logger.Debug("Tool execution result",
			zap.String("tool", toolCall.Tool),
			zap.Int("observation_length", len(observation)))

		// 将工具调用和结果加入消息历史
		messages = append(messages, llm.Message{Role: "assistant", Content: response})
		messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("Observation: %s", observation)})
	}

	// 达到最大迭代次数，强制请求 Final Answer
	messages = append(messages, llm.Message{Role: "user", Content: "已达到工具调用次数上限，请立即输出 Final Answer。"})
	response, err := llmClient.ChatCompletionWithMessages(ctx, messages, 0.1)
	if err != nil {
		return "", fmt.Errorf("final LLM call failed: %w", err)
	}

	if strings.Contains(response, "Final Answer:") {
		finalAnswer := extractFinalAnswer(response)
		if finalAnswer != "" {
			return finalAnswer, nil
		}
	}

	jsonStr := extractJSONFromResponse(response)
	if jsonStr != "" {
		return jsonStr, nil
	}

	return response, nil
}

// toolCall represents a parsed tool call from LLM output
type toolCallParsed struct {
	Tool string
	Args map[string]interface{}
}

var assetAnalysisAllowedTools = map[string]bool{
	"AssetGetProcessVersion":    true,
	"AssetResolvePackageByFile": true,
	"AssetReadConfigSummary":    true,
	"AssetListDirectoryHints":   true,
	"AssetReadProcFile":         true,
}

// parseToolCall extracts tool call from ReAct format output
func parseToolCall(content string) (*toolCallParsed, error) {
	lines := strings.Split(content, "\n")
	var action, actionInput string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Action:") {
			action = strings.TrimSpace(strings.TrimPrefix(trimmed, "Action:"))
			continue
		}
		if strings.HasPrefix(trimmed, "Action Input:") {
			actionInput = strings.TrimSpace(strings.TrimPrefix(trimmed, "Action Input:"))
			if actionInput == "" && i+1 < len(lines) {
				actionInput = strings.Join(lines[i+1:], "\n")
			}
			break
		}
	}

	if action == "" {
		return nil, nil
	}
	if !assetAnalysisAllowedTools[action] {
		return nil, fmt.Errorf("tool %q is not allowed for asset analysis", action)
	}

	args := make(map[string]interface{})
	actionInput = strings.TrimSpace(actionInput)
	if actionInput == "" {
		return nil, fmt.Errorf("missing Action Input for tool %s", action)
	}
	jsonInput := extractJSONFromResponse(actionInput)
	if jsonInput == "" {
		return nil, fmt.Errorf("missing JSON Action Input for tool %s", action)
	}
	if err := json.Unmarshal([]byte(jsonInput), &args); err != nil {
		return nil, fmt.Errorf("invalid Action Input JSON for tool %s: %w", action, err)
	}

	return &toolCallParsed{
		Tool: action,
		Args: args,
	}, nil
}

func assetToolCallKey(tool string, args map[string]interface{}) string {
	if pid, ok := args["pid"]; ok {
		return fmt.Sprintf("pid:%v:%s", pid, tool)
	}

	for _, key := range []string{"path", "exe_path", "file_name"} {
		if value, ok := args[key]; ok {
			return fmt.Sprintf("%s:%s:%v", tool, key, value)
		}
	}

	argsJSON, _ := json.Marshal(args)
	return fmt.Sprintf("%s:%s", tool, string(argsJSON))
}

// extractFinalAnswer extracts the JSON after "Final Answer:"
func extractFinalAnswer(content string) string {
	idx := strings.Index(content, "Final Answer:")
	if idx == -1 {
		return ""
	}
	after := content[idx+len("Final Answer:"):]
	return extractJSONFromResponse(after)
}

// assetToolExecutor executes tools via gRPC to Agent
type assetToolExecutor struct {
	serverClient ServerClientInterface
	hostID       string
	logger       *zap.Logger
}

func (e *assetToolExecutor) Execute(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error) {
	if e.serverClient == nil {
		return nil, fmt.Errorf("server client is not configured")
	}
	if !assetAnalysisAllowedTools[tool] {
		return nil, fmt.Errorf("tool %q is not allowed for asset analysis", tool)
	}
	if args == nil {
		args = make(map[string]interface{})
	}

	callArgs := make(map[string]interface{}, len(args)+1)
	for key, value := range args {
		callArgs[key] = value
	}
	callArgs["host_id"] = e.hostID

	argsJSON, err := json.Marshal(callArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool arguments: %w", err)
	}

	if e.logger != nil {
		e.logger.Info("Executing asset tool via gRPC",
			zap.String("tool", tool),
			zap.String("host_id", e.hostID),
			zap.String("args", string(argsJSON)))
	}

	resp, err := e.serverClient.ExecuteTool(ctx, uuid.New().String(), e.hostID, tool, string(argsJSON), 10)
	if err != nil {
		return nil, fmt.Errorf("gRPC ExecuteTool failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("tool execution failed: %s", resp.Error)
	}

	var result interface{}
	if err := json.Unmarshal([]byte(resp.Result), &result); err != nil {
		// 返回原始字符串
		return map[string]interface{}{"raw": resp.Result}, nil
	}

	return result, nil
}

func (s *AssetAnalysisService) saveApplicationAnalysisResult(hostID uuid.UUID, snapshot HostAssetSnapshot, result *ApplicationAnalysisResult) (int, error) {
	savedCount := 0
	var firstSaveErr error
	softwareAssets, err := s.repo.GetSoftwareAssetsByHost(hostID)
	if err != nil {
		s.logger.Debug("failed to load software assets for application version enrichment",
			zap.String("host_id", hostID.String()),
			zap.Error(err))
		softwareAssets = nil
	}
	for _, app := range result.Applications {
		app = enrichIdentifiedApplicationVersion(app, snapshot, softwareAssets)
		asset := s.convertToApplicationAsset(hostID, snapshot, app)
		asset.AIRawOutput = mustMarshalJSON(result)
		if err := s.repo.UpsertApplicationAsset(asset); err != nil {
			if firstSaveErr == nil {
				firstSaveErr = err
			}
			s.logger.Error("Failed to upsert application asset",
				zap.String("host_id", hostID.String()),
				zap.String("app_name", app.Name),
				zap.Error(err))
		} else {
			if removed, err := s.repo.DeactivateDuplicateApplicationAssets(hostID, applicationDedupeNames(app), asset.Fingerprint); err != nil {
				s.logger.Warn("failed to deactivate duplicate application assets",
					zap.String("host_id", hostID.String()),
					zap.String("app_name", app.Name),
					zap.Error(err))
			} else if removed > 0 {
				s.logger.Info("duplicate application assets deactivated",
					zap.String("host_id", hostID.String()),
					zap.String("app_name", app.Name),
					zap.Int64("count", removed))
			}
			savedCount++
		}
	}
	if len(result.Applications) > 0 && savedCount == 0 {
		return 0, fmt.Errorf("failed to save application assets: %w", firstSaveErr)
	}
	return savedCount, nil
}

func (s *AssetAnalysisService) saveDeterministicApplicationAnalysisResult(hostID uuid.UUID, snapshot HostAssetSnapshot, reason string) (int, error) {
	result := &ApplicationAnalysisResult{Applications: deduplicateApplications(detectKnownApplicationsFromProcesses(snapshot))}
	if len(result.Applications) == 0 {
		s.logger.Info("deterministic application analysis found no known applications",
			zap.String("host_id", hostID.String()),
			zap.String("reason", reason))
		return 0, nil
	}
	saved, err := s.saveApplicationAnalysisResult(hostID, snapshot, result)
	if err != nil {
		return 0, err
	}
	s.logger.Info("deterministic application analysis saved known applications",
		zap.String("host_id", hostID.String()),
		zap.String("reason", reason),
		zap.Int("applications", saved))
	return saved, nil
}

func detectKnownApplicationsFromProcesses(snapshot HostAssetSnapshot) []IdentifiedApplication {
	apps := make([]IdentifiedApplication, 0)
	for _, proc := range snapshot.Processes {
		app := identifyKnownApplicationProcess(proc)
		if app.Name == "" {
			continue
		}
		info := containerInfoFromProcess(proc)
		app.IsContainer = info.IsContainer
		app.ContainerID = info.ID
		app.ContainerRuntime = info.Runtime
		app.ConfigPaths = processConfigPaths(proc)
		if len(app.ConfigPaths) == 0 {
			app.ConfigPaths = knownApplicationDefaultConfigPaths(app.Name)
		}
		app.RelatedPIDs = []int{proc.PID}
		app.ListenPorts = append([]int(nil), proc.ListenPorts...)
		app.InstallPath = proc.ExePath
		if len(app.ConfigPaths) > 0 {
			app.StartPath = app.ConfigPaths[0]
		} else {
			app.StartPath = proc.ExePath
		}
		app.RunUser = proc.Username
		app.Confidence = 0.86
		app.Status = "active"
		app.Evidence = []string{fmt.Sprintf("deterministic_process_match pid=%d comm=%s exe=%s", proc.PID, proc.Comm, proc.ExePath)}
		if app.IsContainer {
			app.Evidence = append(app.Evidence, fmt.Sprintf("container_runtime=%s container_id=%s pid=%d", app.ContainerRuntime, app.ContainerID, proc.PID))
		}
		apps = append(apps, app)
	}
	return apps
}

func identifyKnownApplicationProcess(proc ProcessAsset) IdentifiedApplication {
	comm := strings.ToLower(strings.TrimSpace(proc.Comm))
	exeBase := strings.ToLower(filepathBase(proc.ExePath))
	exePath := strings.ToLower(strings.TrimSpace(proc.ExePath))
	cmd := strings.ToLower(strings.ReplaceAll(proc.Cmdline, "\x00", " "))
	match := func(values ...string) bool {
		for _, value := range values {
			if value == "" {
				continue
			}
			if comm == value || exeBase == value || strings.Contains(cmd, value) {
				return true
			}
		}
		return false
	}
	if strings.Contains(comm, "containerd-shim") || strings.Contains(exeBase, "containerd-shim") {
		return IdentifiedApplication{}
	}
	switch {
	case match("redis-server"):
		return IdentifiedApplication{Name: "redis", DisplayName: "Redis", Category: "database"}
	case match("memcached"):
		return IdentifiedApplication{Name: "memcached", DisplayName: "Memcached", Category: "database"}
	case match("mysqld", "mariadbd"):
		return IdentifiedApplication{Name: "mysql", DisplayName: "MySQL/MariaDB", Category: "database"}
	case match("postgres", "postmaster"):
		return IdentifiedApplication{Name: "postgresql", DisplayName: "PostgreSQL", Category: "database"}
	case match("mongod"):
		return IdentifiedApplication{Name: "mongodb", DisplayName: "MongoDB", Category: "database"}
	case match("elasticsearch"):
		return IdentifiedApplication{Name: "elasticsearch", DisplayName: "Elasticsearch", Category: "database"}
	case match("clickhouse-server"):
		return IdentifiedApplication{Name: "clickhouse", DisplayName: "ClickHouse", Category: "database"}
	case match("etcd"):
		return IdentifiedApplication{Name: "etcd", DisplayName: "etcd", Category: "other"}
	case match("dockerd", "docker-proxy") || comm == "containerd" || exeBase == "containerd":
		return IdentifiedApplication{Name: "docker", DisplayName: "Docker Engine", Category: "other"}
	case match("nginx"):
		if strings.Contains(cmd, "openresty") || strings.Contains(proc.ExePath, "openresty") {
			return IdentifiedApplication{Name: "openresty", DisplayName: "OpenResty", Category: "web_service"}
		}
		if strings.Contains(cmd, "tengine") || strings.Contains(proc.ExePath, "tengine") {
			return IdentifiedApplication{Name: "tengine", DisplayName: "Tengine", Category: "web_service"}
		}
		return IdentifiedApplication{Name: "nginx", DisplayName: "Nginx", Category: "web_service"}
	case match("apache2", "httpd"):
		return IdentifiedApplication{Name: "apache", DisplayName: "Apache HTTP Server", Category: "web_service"}
	case match("sshd") || strings.Contains(cmd, "openssh"):
		return IdentifiedApplication{Name: "openssh", DisplayName: "OpenSSH", Category: "other"}
	case strings.Contains(cmd, "tomcat") || strings.Contains(cmd, "catalina") || strings.Contains(exeBase, "tomcat"):
		return IdentifiedApplication{Name: "tomcat", DisplayName: "Tomcat", Category: "web_service"}
	case match("php-fpm"):
		return IdentifiedApplication{Name: "php_fpm", DisplayName: "PHP-FPM", Category: "web_service"}
	case match("cupsd", "cups-browsed"):
		return IdentifiedApplication{Name: "cups", DisplayName: "CUPS", Category: "other"}
	case match("tailscaled", "tailscale"):
		return IdentifiedApplication{Name: "tailscale", DisplayName: "Tailscale", Category: "other"}
	case match("clash-verge", "verge-mihomo") || strings.Contains(exePath, "clash-verge"):
		return IdentifiedApplication{Name: "clash_verge", DisplayName: "Clash Verge", Category: "other"}
	case comm == "codex" || strings.Contains(exePath, "@openai/codex") || strings.Contains(cmd, "/usr/bin/codex"):
		return IdentifiedApplication{Name: "codex", DisplayName: "OpenAI Codex", Category: "ai_agent"}
	case strings.Contains(exePath, "vscode-server") || strings.Contains(cmd, "vscode-server"):
		return IdentifiedApplication{Name: "vscode_server", DisplayName: "VS Code Server", Category: "other"}
	case comm == "aegis-agent" || strings.Contains(exePath, "/aegis-agent"):
		return IdentifiedApplication{Name: "aegis_agent", DisplayName: "Aegis Agent", Category: "other"}
	case comm == "api-server" && (strings.Contains(exePath, "/api-server") || strings.Contains(cmd, "./api-server")):
		return IdentifiedApplication{Name: "aegis_api_server", DisplayName: "Aegis API Server", Category: "web_service"}
	case comm == "server" && (strings.Contains(exePath, "/server") || strings.Contains(cmd, "./server")):
		return IdentifiedApplication{Name: "aegis_server", DisplayName: "Aegis Server", Category: "other"}
	case comm == "dc" && (strings.Contains(exePath, "/dc") || strings.Contains(cmd, "./dc")):
		return IdentifiedApplication{Name: "aegis_dc", DisplayName: "Aegis Data Consumer", Category: "other"}
	case comm == "builder" && (strings.Contains(exePath, "/builder") || strings.Contains(cmd, "./builder")):
		return IdentifiedApplication{Name: "aegis_builder", DisplayName: "Aegis Builder", Category: "other"}
	case match("grafana-server"):
		return IdentifiedApplication{Name: "grafana", DisplayName: "Grafana", Category: "web_service"}
	case match("prometheus"):
		return IdentifiedApplication{Name: "prometheus", DisplayName: "Prometheus", Category: "other"}
	case match("rabbitmq-server"):
		return IdentifiedApplication{Name: "rabbitmq", DisplayName: "RabbitMQ", Category: "other"}
	case strings.Contains(cmd, "kafka.kafka") || match("kafka-server-start"):
		return IdentifiedApplication{Name: "kafka", DisplayName: "Kafka", Category: "other"}
	case strings.Contains(cmd, "zookeeper") || match("zkserver.sh"):
		return IdentifiedApplication{Name: "zookeeper", DisplayName: "ZooKeeper", Category: "other"}
	case strings.Contains(cmd, "jenkins.war") || match("jenkins"):
		return IdentifiedApplication{Name: "jenkins", DisplayName: "Jenkins", Category: "web_service"}
	case strings.Contains(cmd, "sonarqube") || match("sonar"):
		return IdentifiedApplication{Name: "sonarqube", DisplayName: "SonarQube", Category: "web_service"}
	case strings.Contains(cmd, "nexus") || match("nexus"):
		return IdentifiedApplication{Name: "nexus", DisplayName: "Nexus Repository", Category: "web_service"}
	case match("ollama"):
		return IdentifiedApplication{Name: "ollama", DisplayName: "Ollama", Category: "llm_service"}
	case match("vllm") || strings.Contains(cmd, "vllm"):
		return IdentifiedApplication{Name: "vllm", DisplayName: "vLLM", Category: "llm_service"}
	case match("litellm") || strings.Contains(cmd, "litellm"):
		return IdentifiedApplication{Name: "litellm", DisplayName: "LiteLLM", Category: "llm_service"}
	case match("dify") || strings.Contains(cmd, "dify"):
		return IdentifiedApplication{Name: "dify", DisplayName: "Dify", Category: "llm_service"}
	case match("vsftpd", "proftpd"):
		return IdentifiedApplication{Name: "ftp", DisplayName: "FTP Server", Category: "other"}
	default:
		return IdentifiedApplication{}
	}
}

func filepathBase(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func processConfigPaths(proc ProcessAsset) []string {
	values := []string{}
	fields := strings.Fields(strings.ReplaceAll(proc.Cmdline, "\x00", " "))
	valueFlags := map[string]struct{}{
		"--config": {}, "--conf": {}, "--config-file": {}, "--defaults-file": {}, "--defaults-extra-file": {}, "-c": {},
	}
	for i := 0; i < len(fields); i++ {
		part := strings.Trim(fields[i], `"'`)
		if _, ok := valueFlags[part]; ok && i+1 < len(fields) {
			values = append(values, cleanApplicationConfigPath(fields[i+1], proc.Cwd))
			i++
			continue
		}
		if key, value, ok := strings.Cut(part, "="); ok {
			if _, allowed := valueFlags[key]; allowed {
				values = append(values, cleanApplicationConfigPath(value, proc.Cwd))
				continue
			}
		}
		if looksLikeApplicationConfigPath(part) {
			values = append(values, cleanApplicationConfigPath(part, proc.Cwd))
		}
	}
	return uniqueApplicationPaths(values)
}

func cleanApplicationConfigPath(path, cwd string) string {
	path = strings.TrimSpace(strings.Trim(path, `"'`))
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") && cwd != "" {
		path = strings.TrimRight(cwd, "/") + "/" + path
	}
	if !strings.HasPrefix(path, "/") {
		return ""
	}
	for _, token := range []string{";", "|", "&", "`", "$(", "\n", "\r", "*", "?", "["} {
		if strings.Contains(path, token) {
			return ""
		}
	}
	return path
}

func looksLikeApplicationConfigPath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	for _, suffix := range []string{".conf", ".cnf", ".ini", ".yaml", ".yml", ".json", ".properties", ".toml", ".env", ".xml", ".db", ".passwd", ".htpasswd"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return lower == "/etc/shadow"
}

func uniqueApplicationPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func knownApplicationDefaultConfigPaths(name string) []string {
	switch normalizePublicApplicationType(name) {
	case "redis":
		return []string{"/etc/redis/redis.conf", "/etc/redis.conf", "/usr/local/etc/redis/redis.conf", "/data/redis.conf"}
	case "openssh":
		return []string{"/etc/shadow"}
	case "mysql", "mariadb":
		return []string{"/etc/mysql/my.cnf", "/etc/my.cnf", "/root/.my.cnf"}
	case "postgresql":
		return []string{"/var/lib/postgresql/.pgpass", "/root/.pgpass"}
	case "tomcat":
		return []string{"/usr/local/tomcat/conf/tomcat-users.xml", "/opt/tomcat/conf/tomcat-users.xml"}
	case "nginx":
		return []string{"/etc/nginx/.htpasswd", "/usr/local/nginx/conf/.htpasswd"}
	case "openresty", "tengine":
		return []string{"/etc/nginx/nginx.conf", "/usr/local/openresty/nginx/conf/nginx.conf"}
	case "apache":
		return []string{"/etc/apache2/.htpasswd", "/etc/httpd/.htpasswd"}
	case "ftp":
		return []string{"/etc/proftpd/passwd", "/etc/vsftpd/virtual_users.db", "/etc/shadow"}
	case "grafana":
		return []string{"/etc/grafana/grafana.ini", "/usr/local/etc/grafana/grafana.ini"}
	case "prometheus":
		return []string{"/etc/prometheus/prometheus.yml"}
	case "etcd":
		return []string{"/etc/etcd/etcd.conf.yml", "/etc/etcd/etcd.conf"}
	case "kafka":
		return []string{"/etc/kafka/server.properties", "/opt/kafka/config/server.properties"}
	case "zookeeper":
		return []string{"/etc/zookeeper/zoo.cfg", "/opt/zookeeper/conf/zoo.cfg"}
	case "rabbitmq":
		return []string{"/etc/rabbitmq/rabbitmq.conf"}
	default:
		return nil
	}
}

// filterRelatedPackages 过滤相关软件包
func (s *AssetAnalysisService) filterRelatedPackages(snapshot HostAssetSnapshot) []PackageAsset {
	// 提取进程路径中的关键包
	relatedNames := make(map[string]bool)
	for _, proc := range snapshot.Processes {
		if proc.ExePath != "" {
			// 从 exe 路径推断可能的包名
			parts := strings.Split(proc.ExePath, "/")
			for _, p := range parts {
				if p != "" && p != "usr" && p != "bin" && p != "sbin" && p != "lib" {
					relatedNames[strings.ToLower(p)] = true
				}
			}
		}
	}

	var related []PackageAsset
	for _, pkg := range snapshot.Packages {
		nameLower := strings.ToLower(pkg.Name)
		if relatedNames[nameLower] || isCommonServerPackage(nameLower) {
			related = append(related, pkg)
		}
	}

	// 限制数量
	maxPackages := 100
	if len(related) > maxPackages {
		related = related[:maxPackages]
	}

	return related
}

// isCommonServerPackage 判断是否为常见服务器软件包
func isCommonServerPackage(name string) bool {
	commonPackages := []string{
		"nginx", "apache", "httpd", "mysql", "mariadb", "postgres", "redis",
		"mongo", "elasticsearch", "kafka", "rabbitmq", "tomcat", "jetty",
		"spring", "django", "flask", "laravel", "express", "node", "python",
		"java", "php", "ruby", "go", "dotnet",
	}
	for _, p := range commonPackages {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// parseAnalysisResult 解析分析结果
func (s *AssetAnalysisService) parseAnalysisResult(response string) (*ApplicationAnalysisResult, error) {
	// 尝试提取 JSON
	jsonStr := extractJSONFromResponse(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var result ApplicationAnalysisResult
	if err := unmarshalAnalysisJSON(jsonStr, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// 校验输出
	if err := s.validateAnalysisResult(&result); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &result, nil
}

func unmarshalAnalysisJSON(jsonStr string, result *ApplicationAnalysisResult) error {
	trimmed := strings.TrimSpace(jsonStr)
	if strings.HasPrefix(trimmed, "[") {
		var applications []IdentifiedApplication
		if err := json.Unmarshal([]byte(trimmed), &applications); err != nil {
			return err
		}
		result.Applications = applications
		return nil
	}
	return json.Unmarshal([]byte(trimmed), result)
}

// validateAnalysisResult 校验分析结果
func (s *AssetAnalysisService) validateAnalysisResult(result *ApplicationAnalysisResult) error {
	validCategories := map[string]bool{
		"database":      true,
		"web_service":   true,
		"web_framework": true,
		"web_site":      true,
		"llm_service":   true,
		"ai_agent":      true,
		"mcp_server":    true,
		"other":         true,
		"unknown":       true,
	}

	filtered := make([]IdentifiedApplication, 0, len(result.Applications))
	for i, app := range result.Applications {
		app = normalizeIdentifiedApplication(app)
		if strings.TrimSpace(app.Status) == "" {
			app.Status = "active"
		}

		// 校验分类
		if !validCategories[app.Category] {
			app.Category = "unknown"
		}

		// 校验置信度
		if app.Confidence < 0 || app.Confidence > 1 {
			app.Confidence = 0.5
		}

		// 校验置信度阈值
		if app.Confidence < 0.3 {
			app.Status = "needs_review"
		}

		if !isMarketVisibleApplication(app) {
			if s.logger != nil {
				s.logger.Debug("filtered non-market-visible application asset",
					zap.String("name", app.Name),
					zap.String("display_name", app.DisplayName),
					zap.String("category", app.Category))
			}
			continue
		}

		filtered = append(filtered, app)
		result.Applications[i] = app
	}

	// 去重处理
	result.Applications = deduplicateApplications(filtered)

	return nil
}

// deduplicateApplications 去重应用列表
// 同一主机内同类应用合并为一条资产，PID、端口和证据作为列表保留。
func deduplicateApplications(apps []IdentifiedApplication) []IdentifiedApplication {
	seen := make(map[string]*IdentifiedApplication)
	pidOwner := make(map[int]string)
	var order []string

	for _, app := range apps {
		app.RelatedPIDs = normalizePIDList(app.RelatedPIDs)
		key := applicationDedupeKey(app)
		if key == "" {
			continue
		}
		for _, pid := range app.RelatedPIDs {
			if ownerKey, ok := pidOwner[pid]; ok {
				key = ownerKey
				break
			}
		}

		if existing, ok := seen[key]; ok {
			// 合并 related_pids
			existing.RelatedPIDs = mergePIDs(existing.RelatedPIDs, app.RelatedPIDs)

			// 合并 listen_ports
			existing.ListenPorts = mergePorts(existing.ListenPorts, app.ListenPorts)

			// 合并 evidence
			existing.Evidence = mergeEvidence(existing.Evidence, app.Evidence)
			existing.ConfigPaths = mergeEvidence(existing.ConfigPaths, app.ConfigPaths)
			existing.SitePaths = mergeEvidence(existing.SitePaths, app.SitePaths)
			if app.IsContainer {
				existing.IsContainer = true
				if existing.ContainerID == "" {
					existing.ContainerID = app.ContainerID
				}
				if existing.ContainerRuntime == "" {
					existing.ContainerRuntime = app.ContainerRuntime
				}
			}

			// 取更高的置信度
			if app.Confidence > existing.Confidence {
				existing.Confidence = app.Confidence
			}

			// 如果新版本更具体，使用新版本
			if app.Version != "" && existing.Version == "" {
				existing.Version = app.Version
				existing.VersionSource = app.VersionSource
			}
		} else {
			newApp := app
			seen[key] = &newApp
			order = append(order, key)
		}
		for _, pid := range seen[key].RelatedPIDs {
			pidOwner[pid] = key
		}
	}

	result := make([]IdentifiedApplication, 0, len(seen))
	for _, key := range order {
		if app, ok := seen[key]; ok {
			result = append(result, *app)
		}
	}

	return result
}

var (
	applicationVersionTextPattern  = regexp.MustCompile(`(?i)(?:^|[^0-9A-Za-z])v?(\d+(?:\.\d+)+(?:[._~+\-]?[0-9A-Za-z]+)*)`)
	postgresPathVersionPattern     = regexp.MustCompile(`(?i)/postgresql/(\d+(?:\.\d+)?)(?:/|$)`)
	vscodeServerCommitPattern      = regexp.MustCompile(`(?i)(?:stable-|commit[:=/\s-]*)([0-9a-f]{40})`)
	serviceCgroupContainerPatterns = []struct {
		runtime string
		re      *regexp.Regexp
	}{
		{runtime: "docker", re: regexp.MustCompile(`(?:/docker/|docker-)([a-f0-9]{64})(?:\.scope)?`)},
		{runtime: "containerd", re: regexp.MustCompile(`(?:cri-containerd-|containerd/|/containerd/)([a-f0-9]{64})(?:\.scope)?`)},
		{runtime: "cri-o", re: regexp.MustCompile(`(?:crio-|cri-o/|/crio/)([a-f0-9]{64})(?:\.scope)?`)},
		{runtime: "podman", re: regexp.MustCompile(`(?:libpod-|libpod/|podman/|/libpod-)([a-f0-9]{64})(?:\.scope)?`)},
		{runtime: "container", re: regexp.MustCompile(`(?:^|[/:-])([a-f0-9]{64})(?:\.scope)?(?:$|/)`)},
	}
)

type applicationContainerInfo struct {
	IsContainer bool
	ID          string
	Runtime     string
}

func containerInfoForApplication(processes []ProcessAsset, app IdentifiedApplication) applicationContainerInfo {
	info := applicationContainerInfo{
		IsContainer: app.IsContainer,
		ID:          strings.TrimSpace(app.ContainerID),
		Runtime:     strings.TrimSpace(app.ContainerRuntime),
	}
	processInfo := containerInfoForPIDs(processes, app.RelatedPIDs)
	if processInfo.IsContainer {
		return processInfo
	}
	return info
}

func containerInfoForPIDs(processes []ProcessAsset, pids []int) applicationContainerInfo {
	pidSet := map[int]struct{}{}
	for _, pid := range normalizePIDList(pids) {
		pidSet[pid] = struct{}{}
	}
	if len(pidSet) == 0 {
		return applicationContainerInfo{}
	}
	for _, proc := range processes {
		if _, ok := pidSet[proc.PID]; !ok {
			continue
		}
		if info := containerInfoFromProcess(proc); info.IsContainer {
			return info
		}
	}
	return applicationContainerInfo{}
}

func containerInfoFromProcess(proc ProcessAsset) applicationContainerInfo {
	id := strings.TrimSpace(proc.ContainerID)
	runtime := strings.TrimSpace(proc.Runtime)
	if id != "" {
		if runtime == "" {
			runtime = "container"
		}
		return applicationContainerInfo{IsContainer: true, ID: id, Runtime: runtime}
	}
	runtime, id = parseContainerIdentityFromCgroupLines(proc.Cgroup)
	if id != "" {
		return applicationContainerInfo{IsContainer: true, ID: id, Runtime: runtime}
	}
	return applicationContainerInfo{}
}

func parseContainerIdentityFromCgroupLines(lines []string) (string, string) {
	content := strings.ToLower(strings.Join(lines, "\n"))
	for _, pattern := range serviceCgroupContainerPatterns {
		if match := pattern.re.FindStringSubmatch(content); len(match) > 1 {
			return pattern.runtime, match[1]
		}
	}
	return "", ""
}

func enrichIdentifiedApplicationVersion(app IdentifiedApplication, snapshot HostAssetSnapshot, softwareAssets []model.HostSoftwareAsset) IdentifiedApplication {
	if version := normalizeApplicationVersion(app.Version); version != "" {
		app.Version = version
		if strings.TrimSpace(app.VersionSource) == "" {
			app.VersionSource = "ai"
		}
		return app
	}

	if version, source := versionFromApplicationEvidence(app); version != "" {
		return withIdentifiedApplicationVersion(app, version, source)
	}
	if version, source := versionFromProcessMetadata(app, snapshot); version != "" {
		return withIdentifiedApplicationVersion(app, version, source)
	}
	if version, source := versionFromSoftwareAssets(app, softwareAssets); version != "" {
		return withIdentifiedApplicationVersion(app, version, source)
	}
	return app
}

func withIdentifiedApplicationVersion(app IdentifiedApplication, version, source string) IdentifiedApplication {
	version = normalizeApplicationVersion(version)
	if version == "" {
		return app
	}
	app.Version = version
	app.VersionSource = source
	app.Evidence = mergeEvidence(app.Evidence, []string{fmt.Sprintf("version_%s=%s", source, version)})
	return app
}

func versionFromApplicationEvidence(app IdentifiedApplication) (string, string) {
	for _, evidence := range app.Evidence {
		lower := strings.ToLower(evidence)
		if !strings.Contains(lower, "version") && !strings.Contains(lower, "commit") {
			continue
		}
		if version := extractApplicationVersionFromText(evidence); version != "" {
			return version, "evidence"
		}
		if app.Name == "vscode_server" {
			if match := vscodeServerCommitPattern.FindStringSubmatch(evidence); len(match) > 1 {
				return match[1], "evidence"
			}
		}
	}
	return "", ""
}

func versionFromProcessMetadata(app IdentifiedApplication, snapshot HostAssetSnapshot) (string, string) {
	appType := normalizePublicApplicationType(firstNonEmpty(app.Name, app.DisplayName))
	texts := []string{app.InstallPath, app.StartPath}

	pidSet := make(map[int]struct{}, len(app.RelatedPIDs))
	for _, pid := range app.RelatedPIDs {
		pidSet[pid] = struct{}{}
	}
	for _, proc := range snapshot.Processes {
		if _, ok := pidSet[proc.PID]; !ok {
			continue
		}
		texts = append(texts, proc.ExePath, proc.Cmdline, proc.Cwd)
	}

	switch appType {
	case "postgresql":
		for _, text := range texts {
			if match := postgresPathVersionPattern.FindStringSubmatch(text); len(match) > 1 {
				return match[1], "process_path"
			}
		}
	case "vscode_server":
		for _, text := range texts {
			if match := vscodeServerCommitPattern.FindStringSubmatch(text); len(match) > 1 {
				return match[1], "process_path"
			}
		}
	}
	return "", ""
}

func versionFromSoftwareAssets(app IdentifiedApplication, softwareAssets []model.HostSoftwareAsset) (string, string) {
	aliases := applicationSoftwarePackageAliases(firstNonEmpty(app.Name, app.DisplayName))
	if len(aliases) == 0 || len(softwareAssets) == 0 {
		return "", ""
	}

	for _, alias := range aliases {
		if sw := newestMatchingSoftwareAsset(softwareAssets, func(name string) bool {
			return strings.EqualFold(name, alias)
		}); sw != nil {
			if version := normalizePackageVersion(sw.Version); version != "" {
				return version, softwareVersionSource(sw.PackageManager)
			}
		}
	}
	for _, alias := range aliases {
		prefix := strings.ToLower(alias) + "-"
		if sw := newestMatchingSoftwareAsset(softwareAssets, func(name string) bool {
			return strings.HasPrefix(strings.ToLower(name), prefix)
		}); sw != nil {
			if version := normalizePackageVersion(sw.Version); version != "" {
				return version, softwareVersionSource(sw.PackageManager)
			}
		}
	}
	return "", ""
}

func newestMatchingSoftwareAsset(softwareAssets []model.HostSoftwareAsset, match func(string) bool) *model.HostSoftwareAsset {
	var best *model.HostSoftwareAsset
	var bestSeen time.Time
	for i := range softwareAssets {
		sw := &softwareAssets[i]
		if !match(sw.Name) || normalizePackageVersion(sw.Version) == "" || !isSoftwareAssetFresh(*sw) {
			continue
		}
		seenAt := softwareAssetSeenAt(*sw)
		if best == nil || seenAt.After(bestSeen) {
			best = sw
			bestSeen = seenAt
		}
	}
	return best
}

func isSoftwareAssetFresh(sw model.HostSoftwareAsset) bool {
	seenAt := softwareAssetSeenAt(sw)
	if seenAt.IsZero() {
		return true
	}
	return time.Since(seenAt) <= 24*time.Hour
}

func softwareAssetSeenAt(sw model.HostSoftwareAsset) time.Time {
	if !sw.LastSeenAt.IsZero() {
		return sw.LastSeenAt
	}
	return sw.CollectedAt
}

func applicationSoftwarePackageAliases(name string) []string {
	appType := normalizePublicApplicationType(name)
	switch appType {
	case "docker":
		return []string{"docker-ce", "docker.io", "docker", "docker-ce-cli", "containerd.io", "containerd"}
	case "tailscale":
		return []string{"tailscale"}
	case "openssh":
		return []string{"openssh-server", "openssh"}
	case "cups":
		return []string{"cups", "cups-daemon"}
	case "nginx":
		return []string{"nginx", "nginx-core", "nginx-full", "nginx-light", "nginx-common"}
	case "redis":
		return []string{"redis-server", "redis"}
	case "postgresql":
		return []string{"postgresql", "postgresql-16", "postgresql-15", "postgresql-14", "postgresql-client-16", "postgresql-client-15", "postgresql-client-14"}
	case "codex":
		return []string{"codex", "openai-codex"}
	case "clash_verge":
		return []string{"clash-verge", "clash-verge-rev", "verge-mihomo"}
	case "vscode_server":
		return []string{"code", "vscode", "visual-studio-code"}
	case "minio":
		return []string{"minio"}
	case "kafka":
		return []string{"kafka", "confluent-kafka"}
	case "zookeeper":
		return []string{"zookeeper", "cp-zookeeper"}
	case "aegis_agent":
		return []string{"aegis-agent"}
	default:
		normalized := strings.ReplaceAll(appType, "_", "-")
		if normalized == "" {
			return nil
		}
		return []string{normalized, appType}
	}
}

func softwareVersionSource(packageManager string) string {
	packageManager = strings.TrimSpace(packageManager)
	if packageManager == "" {
		return "software"
	}
	return "software:" + packageManager
}

func normalizePackageVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if idx := strings.Index(version, ":"); idx > 0 && isDigits(version[:idx]) {
		version = version[idx+1:]
	}
	return normalizeApplicationVersion(version)
}

func normalizeApplicationVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.Trim(version, "`'\" ,;")
	if version == "" {
		return ""
	}
	if extracted := extractApplicationVersionFromText(version); extracted != "" {
		return extracted
	}
	return version
}

func extractApplicationVersionFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if match := applicationVersionTextPattern.FindStringSubmatch(text); len(match) > 1 {
		return strings.TrimPrefix(strings.TrimPrefix(match[1], "v"), "V")
	}
	return ""
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func applicationDedupeKey(app IdentifiedApplication) string {
	appType := publicApplicationType(firstNonEmpty(app.Name, app.Category))
	if appType == "" {
		appType = publicApplicationType(app.DisplayName)
	}
	if appType != "" && appType != "unknown" {
		return "app:" + appType
	}
	name := strings.ToLower(strings.TrimSpace(firstNonEmpty(app.DisplayName, app.Name, app.Category)))
	if name == "" {
		return ""
	}
	return "name:" + name
}

func applicationDedupeNames(app IdentifiedApplication) []string {
	appType := normalizePublicApplicationType(firstNonEmpty(app.Name, app.DisplayName))
	aliases := map[string][]string{
		"redis":            {"redis", "redis-server"},
		"postgresql":       {"postgresql", "postgres", "postmaster"},
		"mysql":            {"mysql", "mysqld"},
		"mariadb":          {"mariadb", "mariadbd"},
		"nginx":            {"nginx"},
		"openresty":        {"openresty"},
		"tengine":          {"tengine", "tegine"},
		"apache":           {"apache", "apache2", "httpd"},
		"openssh":          {"openssh", "openssh-server", "ssh", "sshd", "sshd-session"},
		"tomcat":           {"tomcat", "catalina"},
		"ftp":              {"ftp", "vsftpd", "proftpd"},
		"grafana":          {"grafana", "grafana-server"},
		"rabbitmq":         {"rabbitmq", "rabbitmq-server"},
		"kafka":            {"kafka", "kafka-server-start"},
		"zookeeper":        {"zookeeper", "zkserver"},
		"php_fpm":          {"php_fpm", "php-fpm"},
		"litellm":          {"litellm", "lite-llm", "lite_llm"},
		"dify":             {"dify"},
		"fastchat":         {"fastchat", "fast-chat"},
		"open_webui":       {"open_webui", "open-webui", "open webui"},
		"claude_code":      {"claude_code", "claude-code", "claude code"},
		"codex":            {"codex", "openai codex", "openai-codex"},
		"minio":            {"minio", "minio-server"},
		"prometheus":       {"prometheus", "prometheus-server"},
		"cups":             {"cups", "cupsd"},
		"docker":           {"docker", "dockerd", "docker-proxy", "containerd"},
		"tailscale":        {"tailscale", "tailscaled"},
		"clash_verge":      {"clash_verge", "clash-verge", "verge-mihomo", "mihomo"},
		"vscode_server":    {"vscode_server", "vscode-server"},
		"aegis_agent":      {"aegis_agent", "aegis-agent"},
		"aegis_api_server": {"aegis_api_server", "aegis-api-server", "api-server"},
		"aegis_server":     {"aegis_server", "aegis-server"},
		"aegis_dc":         {"aegis_dc", "aegis-dc"},
		"aegis_builder":    {"aegis_builder", "aegis-builder"},
	}

	values := []string{app.Name, app.DisplayName, app.Category, appType}
	if names, ok := aliases[appType]; ok {
		values = append(values, names...)
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, normalized := range applicationNameVariants(value) {
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	return result
}

func applicationNameVariants(value string) []string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return nil
	}

	values := []string{normalized}
	underscore := strings.ReplaceAll(strings.ReplaceAll(normalized, "-", "_"), " ", "_")
	hyphen := strings.ReplaceAll(strings.ReplaceAll(normalized, "_", "-"), " ", "-")
	space := strings.ReplaceAll(strings.ReplaceAll(normalized, "_", " "), "-", " ")
	values = append(values, underscore, hyphen, space)

	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, candidate := range values {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

// mergePIDs 合并 PID 列表，去重
func mergePIDs(a, b []int) []int {
	seen := make(map[int]bool)
	var result []int

	for _, pid := range a {
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			result = append(result, pid)
		}
	}
	for _, pid := range b {
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			result = append(result, pid)
		}
	}
	sort.Ints(result)

	return result
}

func normalizePIDList(pids []int) []int {
	return mergePIDs(nil, pids)
}

// mergePorts 合并端口列表，去重
func mergePorts(a, b []int) []int {
	seen := make(map[int]bool)
	var result []int

	for _, port := range a {
		if !seen[port] {
			seen[port] = true
			result = append(result, port)
		}
	}
	for _, port := range b {
		if !seen[port] {
			seen[port] = true
			result = append(result, port)
		}
	}

	return result
}

// mergeEvidence 合并证据列表，去重
func mergeEvidence(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, e := range a {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}
	for _, e := range b {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}

	return result
}

// convertToApplicationAsset 转换为应用资产模型
func (s *AssetAnalysisService) convertToApplicationAsset(hostID uuid.UUID, snapshot HostAssetSnapshot, app IdentifiedApplication) *model.HostApplicationAsset {
	versionSource := strings.TrimSpace(app.VersionSource)
	if versionSource == "" {
		versionSource = "ai"
	}
	containerInfo := containerInfoForApplication(snapshot.Processes, app)
	app.IsContainer = containerInfo.IsContainer
	if app.ContainerID == "" {
		app.ContainerID = containerInfo.ID
	}
	if app.ContainerRuntime == "" {
		app.ContainerRuntime = containerInfo.Runtime
	}
	if app.IsContainer {
		app.Evidence = mergeEvidence(app.Evidence, []string{fmt.Sprintf("container_application runtime=%s id=%s", app.ContainerRuntime, app.ContainerID)})
	}
	return &model.HostApplicationAsset{
		ID:               uuid.New(),
		HostID:           hostID,
		Hostname:         snapshot.Hostname,
		IPAddress:        snapshot.IPAddress,
		OSType:           snapshot.OSType,
		Category:         app.Category,
		Name:             app.Name,
		DisplayName:      app.DisplayName,
		Version:          app.Version,
		VersionSource:    versionSource,
		InstallPath:      app.InstallPath,
		StartPath:        app.StartPath,
		ConfigPaths:      mustMarshalJSON(app.ConfigPaths),
		SitePaths:        mustMarshalJSON(app.SitePaths),
		ListenPorts:      mustMarshalJSON(app.ListenPorts),
		RunUser:          app.RunUser,
		RelatedPIDs:      mustMarshalJSON(app.RelatedPIDs),
		IsContainer:      app.IsContainer,
		ContainerID:      app.ContainerID,
		ContainerRuntime: app.ContainerRuntime,
		AIConfidence:     app.Confidence,
		AIEvidence:       mustMarshalJSON(app.Evidence),
		ReviewStatus:     "auto",
		Status:           app.Status,
		Fingerprint:      generateAppFingerprint(hostID.String(), app.Category, app.Name, app.InstallPath, app.ListenPorts, app.RelatedPIDs),
		LastSeenAt:       time.Now(),
		CollectedAt:      time.Now(),
	}
}

type marketApplicationMeta struct {
	DisplayName string
	Category    string
}

var marketVisibleApplicationCatalog = map[string]marketApplicationMeta{
	"redis":            {DisplayName: "Redis", Category: "database"},
	"memcached":        {DisplayName: "Memcached", Category: "database"},
	"mysql":            {DisplayName: "MySQL", Category: "database"},
	"mariadb":          {DisplayName: "MariaDB", Category: "database"},
	"postgresql":       {DisplayName: "PostgreSQL", Category: "database"},
	"mongodb":          {DisplayName: "MongoDB", Category: "database"},
	"elasticsearch":    {DisplayName: "Elasticsearch", Category: "database"},
	"clickhouse":       {DisplayName: "ClickHouse", Category: "database"},
	"influxdb":         {DisplayName: "InfluxDB", Category: "database"},
	"cassandra":        {DisplayName: "Apache Cassandra", Category: "database"},
	"couchdb":          {DisplayName: "CouchDB", Category: "database"},
	"qdrant":           {DisplayName: "Qdrant", Category: "database"},
	"milvus":           {DisplayName: "Milvus", Category: "database"},
	"oracle":           {DisplayName: "Oracle Database", Category: "database"},
	"sqlserver":        {DisplayName: "SQL Server", Category: "database"},
	"nginx":            {DisplayName: "Nginx", Category: "web_service"},
	"openresty":        {DisplayName: "OpenResty", Category: "web_service"},
	"tengine":          {DisplayName: "Tengine", Category: "web_service"},
	"apache":           {DisplayName: "Apache HTTP Server", Category: "web_service"},
	"caddy":            {DisplayName: "Caddy", Category: "web_service"},
	"envoy":            {DisplayName: "Envoy", Category: "web_service"},
	"traefik":          {DisplayName: "Traefik", Category: "web_service"},
	"haproxy":          {DisplayName: "HAProxy", Category: "web_service"},
	"tomcat":           {DisplayName: "Tomcat", Category: "web_service"},
	"jetty":            {DisplayName: "Jetty", Category: "web_service"},
	"php_fpm":          {DisplayName: "PHP-FPM", Category: "web_service"},
	"spring_boot":      {DisplayName: "Spring Boot", Category: "web_framework"},
	"django":           {DisplayName: "Django", Category: "web_framework"},
	"flask":            {DisplayName: "Flask", Category: "web_framework"},
	"fastapi":          {DisplayName: "FastAPI", Category: "web_framework"},
	"rails":            {DisplayName: "Ruby on Rails", Category: "web_framework"},
	"nextjs":           {DisplayName: "Next.js", Category: "web_framework"},
	"nuxt":             {DisplayName: "Nuxt", Category: "web_framework"},
	"laravel":          {DisplayName: "Laravel", Category: "web_framework"},
	"express":          {DisplayName: "Express", Category: "web_framework"},
	"weblogic":         {DisplayName: "WebLogic", Category: "web_service"},
	"jboss":            {DisplayName: "JBoss", Category: "web_service"},
	"wildfly":          {DisplayName: "WildFly", Category: "web_service"},
	"iis":              {DisplayName: "IIS", Category: "web_service"},
	"phpmyadmin":       {DisplayName: "phpMyAdmin", Category: "web_service"},
	"grafana":          {DisplayName: "Grafana", Category: "web_service"},
	"kibana":           {DisplayName: "Kibana", Category: "web_service"},
	"jenkins":          {DisplayName: "Jenkins", Category: "web_service"},
	"gitlab":           {DisplayName: "GitLab", Category: "web_service"},
	"harbor":           {DisplayName: "Harbor", Category: "web_service"},
	"sonarqube":        {DisplayName: "SonarQube", Category: "web_service"},
	"nexus":            {DisplayName: "Nexus Repository", Category: "web_service"},
	"rabbitmq":         {DisplayName: "RabbitMQ", Category: "other"},
	"kafka":            {DisplayName: "Kafka", Category: "other"},
	"zookeeper":        {DisplayName: "ZooKeeper", Category: "other"},
	"etcd":             {DisplayName: "etcd", Category: "other"},
	"prometheus":       {DisplayName: "Prometheus", Category: "other"},
	"minio":            {DisplayName: "MinIO", Category: "other"},
	"vault":            {DisplayName: "Vault", Category: "other"},
	"consul":           {DisplayName: "Consul", Category: "other"},
	"cups":             {DisplayName: "CUPS", Category: "other"},
	"docker":           {DisplayName: "Docker Engine", Category: "other"},
	"tailscale":        {DisplayName: "Tailscale", Category: "other"},
	"clash_verge":      {DisplayName: "Clash Verge", Category: "other"},
	"vscode_server":    {DisplayName: "VS Code Server", Category: "other"},
	"aegis_agent":      {DisplayName: "Aegis Agent", Category: "other"},
	"aegis_api_server": {DisplayName: "Aegis API Server", Category: "web_service"},
	"aegis_server":     {DisplayName: "Aegis Server", Category: "other"},
	"aegis_dc":         {DisplayName: "Aegis Data Consumer", Category: "other"},
	"aegis_builder":    {DisplayName: "Aegis Builder", Category: "other"},
	"openldap":         {DisplayName: "OpenLDAP", Category: "other"},
	"active_directory": {DisplayName: "Active Directory", Category: "other"},
	"openssh":          {DisplayName: "OpenSSH", Category: "other"},
	"ftp":              {DisplayName: "FTP Server", Category: "other"},
	"ollama":           {DisplayName: "Ollama", Category: "llm_service"},
	"localai":          {DisplayName: "LocalAI", Category: "llm_service"},
	"vllm":             {DisplayName: "vLLM", Category: "llm_service"},
	"litellm":          {DisplayName: "LiteLLM", Category: "llm_service"},
	"dify":             {DisplayName: "Dify", Category: "llm_service"},
	"fastchat":         {DisplayName: "FastChat", Category: "llm_service"},
	"open_webui":       {DisplayName: "Open WebUI", Category: "llm_service"},
	"llm_service":      {DisplayName: "LLM Service", Category: "llm_service"},
	"claude_code":      {DisplayName: "Claude Code", Category: "ai_agent"},
	"codex":            {DisplayName: "OpenAI Codex", Category: "ai_agent"},
	"cursor":           {DisplayName: "Cursor", Category: "ai_agent"},
	"windsurf":         {DisplayName: "Windsurf", Category: "ai_agent"},
	"ai_agent":         {DisplayName: "AI Agent", Category: "ai_agent"},
	"mcp_server":       {DisplayName: "MCP Server", Category: "mcp_server"},
}

var nonApplicationProcessNames = map[string]struct{}{
	"bash": {}, "sh": {}, "zsh": {}, "dash": {}, "fish": {},
	"python": {}, "python2": {}, "python3": {}, "node": {}, "java": {}, "go": {}, "ruby": {}, "php": {},
	"systemd": {}, "systemd-logind": {}, "systemd-journald": {}, "systemd-resolved": {}, "systemd-udevd": {},
	"cron": {}, "crond": {}, "dbus": {}, "dbus-daemon": {}, "networkmanager": {}, "wpa-supplicant": {}, "wpa_supplicant": {},
	"ss": {}, "ps": {}, "top": {}, "sleep": {},
	"containerd-shim": {}, "kubelet": {},
	"accounts-daemon": {}, "power-profiles-daemon": {}, "switcheroo-control": {}, "colord": {}, "rtkit": {},
	"upower": {}, "udisks2": {}, "ibus": {}, "modemmanager": {}, "policykit": {}, "polkit": {},
	"avahi": {}, "avahi-daemon": {}, "chrony": {}, "snapd": {}, "rsyslog": {},
	"node_exporter": {}, "telegraf": {}, "collectd": {}, "zabbix_agent": {}, "fluentd": {}, "filebeat": {}, "logstash": {},
}

func normalizeIdentifiedApplication(app IdentifiedApplication) IdentifiedApplication {
	app.Name = strings.TrimSpace(app.Name)
	app.DisplayName = strings.TrimSpace(app.DisplayName)
	app.Category = strings.TrimSpace(app.Category)
	app.Version = strings.TrimSpace(app.Version)
	app.InstallPath = strings.TrimSpace(app.InstallPath)
	app.StartPath = strings.TrimSpace(app.StartPath)
	app.RunUser = strings.TrimSpace(app.RunUser)
	app.Status = strings.TrimSpace(app.Status)
	app.ContainerID = strings.TrimSpace(app.ContainerID)
	app.ContainerRuntime = strings.TrimSpace(app.ContainerRuntime)

	if appType := publicApplicationType(firstNonEmpty(app.Name, app.DisplayName, app.Category)); appType != "" {
		if meta, ok := marketVisibleApplicationCatalog[appType]; ok {
			app.Name = appType
			if app.DisplayName == "" || isGenericApplicationLabel(app.DisplayName) {
				app.DisplayName = meta.DisplayName
			}
			app.Category = meta.Category
		}
	}
	return app
}

func isMarketVisibleApplication(app IdentifiedApplication) bool {
	rawName := firstNonEmpty(app.Name, app.DisplayName)
	appType := normalizePublicApplicationType(rawName)
	if appType == "" || appType == "unknown" || isGenericApplicationLabel(appType) {
		return false
	}
	if _, excluded := nonApplicationProcessNames[appType]; excluded {
		return false
	}
	if looksLikeInternalApplicationName(app.Name) || looksLikeInternalApplicationName(app.DisplayName) {
		return false
	}
	if _, known := marketVisibleApplicationCatalog[appType]; known {
		return true
	}
	return app.Confidence >= 0.75 && hasApplicationEvidence(app)
}

func hasApplicationEvidence(app IdentifiedApplication) bool {
	if app.InstallPath != "" || app.StartPath != "" || len(app.ConfigPaths) > 0 || len(app.SitePaths) > 0 {
		return true
	}
	if len(app.ListenPorts) > 0 || len(app.RelatedPIDs) > 0 {
		return true
	}
	return len(app.Evidence) > 0
}

func publicApplicationType(value string) string {
	normalized := normalizePublicApplicationType(value)
	if normalized == "" || normalized == "unknown" {
		return ""
	}
	if _, ok := marketVisibleApplicationCatalog[normalized]; ok {
		return normalized
	}
	return ""
}

func normalizePublicApplicationType(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "openresty"):
		return "openresty"
	case strings.Contains(lower, "tengine") || strings.Contains(lower, "tegine"):
		return "tengine"
	case strings.Contains(lower, "php-fpm") || strings.Contains(lower, "php_fpm"):
		return "php_fpm"
	case strings.Contains(lower, "memcached"):
		return "memcached"
	case strings.Contains(lower, "clickhouse"):
		return "clickhouse"
	case strings.Contains(lower, "influxdb") || strings.Contains(lower, "influx db"):
		return "influxdb"
	case strings.Contains(lower, "cassandra"):
		return "cassandra"
	case strings.Contains(lower, "couchdb"):
		return "couchdb"
	case strings.Contains(lower, "qdrant"):
		return "qdrant"
	case strings.Contains(lower, "milvus"):
		return "milvus"
	case strings.Contains(lower, "caddy"):
		return "caddy"
	case strings.Contains(lower, "envoy"):
		return "envoy"
	case strings.Contains(lower, "traefik"):
		return "traefik"
	case strings.Contains(lower, "haproxy") || strings.Contains(lower, "ha proxy"):
		return "haproxy"
	case strings.Contains(lower, "fastapi"):
		return "fastapi"
	case strings.Contains(lower, "ruby on rails") || strings.Contains(lower, "rails"):
		return "rails"
	case strings.Contains(lower, "next.js") || strings.Contains(lower, "nextjs"):
		return "nextjs"
	case strings.Contains(lower, "nuxt"):
		return "nuxt"
	case strings.Contains(lower, "etcd"):
		return "etcd"
	case strings.Contains(lower, "prometheus"):
		return "prometheus"
	case strings.Contains(lower, "minio"):
		return "minio"
	case strings.Contains(lower, "vault"):
		return "vault"
	case strings.Contains(lower, "consul"):
		return "consul"
	case strings.Contains(lower, "cupsd") || strings.Contains(lower, "cups"):
		return "cups"
	case strings.Contains(lower, "docker-proxy") || strings.Contains(lower, "dockerd") || lower == "docker" || strings.Contains(lower, "docker engine"):
		return "docker"
	case strings.Contains(lower, "tailscaled") || strings.Contains(lower, "tailscale"):
		return "tailscale"
	case strings.Contains(lower, "clash-verge") || strings.Contains(lower, "clash verge") || strings.Contains(lower, "verge-mihomo"):
		return "clash_verge"
	case strings.Contains(lower, "vscode-server") || strings.Contains(lower, "vscode_server") || strings.Contains(lower, "vs code server"):
		return "vscode_server"
	case strings.Contains(lower, "aegis-agent") || strings.Contains(lower, "aegis agent"):
		return "aegis_agent"
	case strings.Contains(lower, "aegis-api-server") || strings.Contains(lower, "aegis api server") || lower == "api-server":
		return "aegis_api_server"
	case strings.Contains(lower, "aegis-server") || strings.Contains(lower, "aegis server"):
		return "aegis_server"
	case strings.Contains(lower, "aegis-dc") || strings.Contains(lower, "aegis data consumer"):
		return "aegis_dc"
	case strings.Contains(lower, "aegis-builder") || strings.Contains(lower, "aegis builder"):
		return "aegis_builder"
	case strings.Contains(lower, "litellm") || strings.Contains(lower, "lite llm"):
		return "litellm"
	case strings.Contains(lower, "dify"):
		return "dify"
	case strings.Contains(lower, "fastchat"):
		return "fastchat"
	case strings.Contains(lower, "open webui") || strings.Contains(lower, "open-webui") || strings.Contains(lower, "open_webui"):
		return "open_webui"
	case strings.Contains(lower, "claude code") || strings.Contains(lower, "claude-code") || strings.Contains(lower, "claude_code"):
		return "claude_code"
	case strings.Contains(lower, "openai codex") || lower == "codex":
		return "codex"
	case strings.Contains(lower, "cursor"):
		return "cursor"
	case strings.Contains(lower, "windsurf"):
		return "windsurf"
	default:
		return normalizeApplicationType(value)
	}
}

func isGenericApplicationLabel(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "app", "application", "service", "server", "web", "web_service", "database", "unknown":
		return true
	default:
		return false
	}
}

func looksLikeInternalApplicationName(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, token := range []string{"internal", "自研", "内部", "custom", "script", "业务", "myapp", "demo"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".jar")
}

// ApplicationAnalysisResult 应用分析结果
type ApplicationAnalysisResult struct {
	Applications []IdentifiedApplication `json:"applications"`
}

// IdentifiedApplication 识别出的应用
type IdentifiedApplication struct {
	Name             string   `json:"name"`
	DisplayName      string   `json:"display_name"`
	Category         string   `json:"category"`
	Version          string   `json:"version"`
	VersionSource    string   `json:"version_source,omitempty"`
	Confidence       float64  `json:"confidence"`
	Evidence         []string `json:"evidence"`
	RelatedPIDs      []int    `json:"related_pids"`
	IsContainer      bool     `json:"is_container"`
	ContainerID      string   `json:"container_id,omitempty"`
	ContainerRuntime string   `json:"container_runtime,omitempty"`
	InstallPath      string   `json:"install_path"`
	StartPath        string   `json:"start_path"`
	ConfigPaths      []string `json:"config_paths"`
	SitePaths        []string `json:"site_paths"`
	ListenPorts      []int    `json:"listen_ports"`
	RunUser          string   `json:"run_user"`
	Status           string   `json:"status"`
}

// extractJSONFromResponse 从响应中提取 JSON
func extractJSONFromResponse(response string) string {
	text := strings.TrimSpace(response)
	if strings.HasPrefix(text, "```") {
		firstLineEnd := strings.Index(text, "\n")
		lastFence := strings.LastIndex(text, "```")
		if firstLineEnd != -1 && lastFence > firstLineEnd {
			text = strings.TrimSpace(text[firstLineEnd+1 : lastFence])
		}
	}

	return extractFirstJSONValue(text)
}

func extractFirstJSONValue(text string) string {
	start := -1
	for i, r := range text {
		if r == '{' || r == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}

	stack := make([]rune, 0, 8)
	inString := false
	escaped := false

	for i := start; i < len(text); i++ {
		ch := rune(text[i])
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != ch {
				return ""
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return strings.TrimSpace(text[start : i+1])
			}
		}
	}

	return ""
}

func splitProcessBatches(processes []ProcessAsset, batchSize int) [][]ProcessAsset {
	if batchSize <= 0 || len(processes) == 0 {
		return nil
	}

	batches := make([][]ProcessAsset, 0, (len(processes)+batchSize-1)/batchSize)
	for start := 0; start < len(processes); start += batchSize {
		end := start + batchSize
		if end > len(processes) {
			end = len(processes)
		}
		batches = append(batches, processes[start:end])
	}
	return batches
}

func truncateForPrompt(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func truncateForLog(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

// generateAppFingerprint 生成应用指纹
func generateAppFingerprint(hostID, category, name, installPath string, listenPorts []int, relatedPIDs []int) string {
	appType := ""
	if !isGenericApplicationLabel(name) {
		appType = normalizePublicApplicationType(name)
	}
	if appType != "" && appType != "unknown" && !isGenericApplicationLabel(appType) {
		return fmt.Sprintf("%x", sha256Sum(fmt.Sprintf("%s:app:%s", hostID, appType)))
	}
	portStr := ""
	for _, p := range listenPorts {
		portStr += fmt.Sprintf(":%d", p)
	}
	return fmt.Sprintf("%x", sha256Sum(fmt.Sprintf("%s:%s:%s:%s:%s", hostID, category, name, installPath, portStr)))
}

// applicationAnalysisSystemPrompt 应用分析系统提示
const applicationAnalysisSystemPrompt = `你是 Aegis 主机应用资产识别专家。你需要基于进程快照和受控 Agent 工具证据，识别主机上真实运行的应用、服务、中间件、数据库、Web 服务、Web 框架、AI 服务、AI Agent、MCP Server、运维工具和安全组件。

## 输入上下文
用户消息会提供：
- 主机信息：hostname, ip, os_type, os_version, arch
- 进程快照：pid, comm, exe_path, cwd, username, listen_ports, cmdline, /proc/<pid>/cgroup, container_runtime, container_id
- 后续 Observation 中可能包含工具调用结果

## 可用工具
当快照不足以确认版本、包归属、配置路径或进程细节时，可以调用以下工具：

- AssetGetProcessVersion: 获取进程版本。参数: pid (int), exe_path (string), hint (string)
- AssetResolvePackageByFile: 通过文件路径查找 rpm/dpkg/apk 软件包。参数: path (string)
- AssetReadConfigSummary: 读取配置文件摘要。参数: path (string), max_size (int)
- AssetListDirectoryHints: 列出安装目录或配置目录文件。参数: path (string), max_entries (int)
- AssetReadProcFile: 读取 /proc/{pid}/ 下的安全文件。参数: pid (int), file_name (string)

## 工具调用格式（ReAct）
需要补证据时严格输出：
Thought: [说明候选应用、缺少的证据、为什么调用该工具]
Action: [工具名]
Action Input: [JSON 参数]

## 工具限制
- 每个进程的每个工具只能调用一次。
- 优先用 AssetGetProcessVersion 获取真实版本。
- 版本工具失败时，可用 AssetResolvePackageByFile 补包归属证据。
- 只有拿到具体配置路径后，才调用 AssetReadConfigSummary。
- AssetReadProcFile 最大读取 10KB，禁止请求 environ 和 mem。
- 总工具调用次数最多 10 次，之后必须输出 Final Answer。
- 不允许请求任意 shell、find、递归扫描或写操作。

## 识别策略
请采用“确定性证据优先”的方式：
1. 先从 comm、exe_path、cmdline、cwd、listen_ports 形成候选应用。
2. 再用工具补齐版本、包归属、配置路径或目录线索。
3. 对 Java、Node、Python、PHP、Go、Ruby、dotnet 等运行时进程，不要把运行时本身作为应用；只有能从 jar、classpath、cmdline、路径、配置或包名确认具体应用、服务或框架时才返回。
4. 单独端口不能作为强证据，必须结合进程名、路径、配置或工具结果。
5. 配置路径优先来自 cmdline，其次来自工具结果或明确存在的默认路径。
6. 版本不能猜测。没有工具、包、cmdline 或配置证据时，version 留空字符串。
7. 应该识别 Docker、Tailscale、OpenSSH、CUPS、VS Code Server、Codex、Clash Verge、Aegis Agent/API Server/Server/DC/Builder 这类真实运行的工具或平台服务。
8. 不要返回 shell 命令、语言运行时本身、临时脚本、systemd/dbus/cron/udev/NetworkManager 等系统基础进程、桌面会话辅助进程、containerd-shim 这类容器子进程，或无法从进程/路径/配置确认身份的噪声进程。
9. 容器判断必须优先使用 Agent 逻辑：/proc/<pid>/cgroup 出现 Docker/containerd/cri-o/podman/libpod/kubepods scope 或 64 位容器 ID 时，is_container=true，并返回 container_runtime/container_id；普通 0::/、system.slice、user.slice 且无容器 ID 时为主机应用。
10. 证据不足时不要输出该项，而不是输出 unknown。

## 分类
category 只能使用以下值之一：
- database
- web_service
- web_framework
- web_site
- llm_service
- ai_agent
- mcp_server
- other
- unknown

尽量避免 unknown；不能确认真实应用身份时直接不返回。

## 去重与合并
一个应用可能对应多个相关进程，你必须合并：
- master/worker、父子进程、同 exe_path、同配置路径、同应用端口组。
- 同一个主机上的同一应用只返回一条资产。
- related_pids 不能重复出现在不同应用中。
- 合并多个实例时，把 PID、端口、配置路径、site_paths 和 evidence 合并到一条资产。
- 不要为同一应用分别输出进程名、包名、框架名的重复资产；保留最具体、最能表达真实资产身份的应用名。

## 置信度
- 0.90-1.00: 应用身份明确，并且有版本、配置、包归属、监听端口中至少两类证据。
- 0.75-0.89: 应用身份明确，但只有一类辅助证据。
- 0.50-0.74: 通过路径、cmdline 或包名推断，需要人工复核。
- 低于 0.50: 不要输出。
- 低于 0.30 的结果如确实必须保留，status 设为 needs_review；通常应直接丢弃。

## 最终输出格式
收集到足够信息后，只输出当前项目可解析的 JSON：
Final Answer: {"applications": [...]}

每个应用对象只能使用以下字段：
{
  "name": "标准小写标识，例如 redis/nginx/postgresql/docker/tailscale/aegis_agent/ollama/mcp_server",
  "display_name": "展示名",
  "category": "database|web_service|web_framework|web_site|llm_service|ai_agent|mcp_server|other|unknown",
  "version": "真实版本，未知则为空字符串",
  "confidence": 0.95,
  "evidence": [
    "pid=123 comm=redis-server exe=/usr/bin/redis-server",
    "version_tool=7.0.12",
    "config_from_cmdline=/etc/redis/redis.conf"
  ],
  "related_pids": [123],
  "is_container": true,
  "container_id": "64位容器ID，非容器为空字符串",
  "container_runtime": "docker|containerd|cri-o|podman|container|空字符串",
  "install_path": "/usr/bin/redis-server",
  "start_path": "/var/lib/redis",
  "config_paths": ["/etc/redis/redis.conf"],
  "site_paths": [],
  "listen_ports": [6379],
  "run_user": "redis",
  "status": "active"
}

不要输出 version_source、container_id、container_name、runtime_name、framework_name、related_packages、domains 等当前分析结构不会解析的字段。

如果本分片没有可信应用，输出：
Final Answer: {"applications":[]}`

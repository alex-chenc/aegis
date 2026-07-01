package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const applicationAnalysisProcessBatchSize = 50

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

	// 获取 LLM 配置
	llmConfig, err := s.configRepo.GetActive()
	if err != nil {
		return 0, fmt.Errorf("failed to get LLM config: %w", err)
	}

	// 解密 API Key
	apiKey, err := s.configRepo.DecryptAPIKey(llmConfig.APIKeyEncrypted)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	// 创建 LLM 客户端
	llmClient := llm.NewLLMClient(
		apiKey,
		llmConfig.BaseURL,
		llmConfig.ModelName,
		60, // timeout seconds
		3,  // max retries
	)

	if len(snapshot.Processes) == 0 {
		s.logger.Info("Application analysis skipped because process snapshot is empty",
			zap.String("host_id", hostID.String()))
		return 0, nil
	}

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
		response, err := s.analyzeWithReAct(ctx, llmClient, toolExecutor, prompt)
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
		return 0, fmt.Errorf("all application analysis batches failed: %w", firstBatchErr)
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
		sb.WriteString(fmt.Sprintf("- PID: %d, Comm: %s, Exe: %s, Cwd: %s, User: %s, Ports: %v\n",
			proc.PID, proc.Comm, truncateForPrompt(proc.ExePath, 200), truncateForPrompt(proc.Cwd, 200), proc.Username, proc.ListenPorts))
		if proc.Cmdline != "" {
			sb.WriteString(fmt.Sprintf("  Cmdline: %s\n", truncateForPrompt(proc.Cmdline, 300)))
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
	for _, app := range result.Applications {
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
			savedCount++
		}
	}
	if len(result.Applications) > 0 && savedCount == 0 {
		return 0, fmt.Errorf("failed to save application assets: %w", firstSaveErr)
	}
	return savedCount, nil
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

	for i, app := range result.Applications {
		if strings.TrimSpace(app.Status) == "" {
			result.Applications[i].Status = "active"
		}

		// 校验分类
		if !validCategories[app.Category] {
			result.Applications[i].Category = "unknown"
		}

		// 校验置信度
		if app.Confidence < 0 || app.Confidence > 1 {
			result.Applications[i].Confidence = 0.5
		}

		// 校验置信度阈值
		if app.Confidence < 0.3 {
			result.Applications[i].Status = "needs_review"
		}
	}

	// 去重处理
	result.Applications = deduplicateApplications(result.Applications)

	return nil
}

// deduplicateApplications 去重应用列表
// 如果同一个 name 出现多次，合并 related_pids 和 listen_ports
func deduplicateApplications(apps []IdentifiedApplication) []IdentifiedApplication {
	seen := make(map[string]*IdentifiedApplication)
	var order []string

	for _, app := range apps {
		key := strings.ToLower(strings.TrimSpace(app.Name))
		if key == "" {
			continue
		}

		if existing, ok := seen[key]; ok {
			// 合并 related_pids
			existing.RelatedPIDs = mergePIDs(existing.RelatedPIDs, app.RelatedPIDs)

			// 合并 listen_ports
			existing.ListenPorts = mergePorts(existing.ListenPorts, app.ListenPorts)

			// 合并 evidence
			existing.Evidence = mergeEvidence(existing.Evidence, app.Evidence)

			// 取更高的置信度
			if app.Confidence > existing.Confidence {
				existing.Confidence = app.Confidence
			}

			// 如果新版本更具体，使用新版本
			if app.Version != "" && existing.Version == "" {
				existing.Version = app.Version
			}
		} else {
			newApp := app
			seen[key] = &newApp
			order = append(order, key)
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

// mergePIDs 合并 PID 列表，去重
func mergePIDs(a, b []int) []int {
	seen := make(map[int]bool)
	var result []int

	for _, pid := range a {
		if !seen[pid] {
			seen[pid] = true
			result = append(result, pid)
		}
	}
	for _, pid := range b {
		if !seen[pid] {
			seen[pid] = true
			result = append(result, pid)
		}
	}

	return result
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
	return &model.HostApplicationAsset{
		ID:            uuid.New(),
		HostID:        hostID,
		Hostname:      snapshot.Hostname,
		IPAddress:     snapshot.IPAddress,
		OSType:        snapshot.OSType,
		Category:      app.Category,
		Name:          app.Name,
		DisplayName:   app.DisplayName,
		Version:       app.Version,
		VersionSource: "ai",
		InstallPath:   app.InstallPath,
		StartPath:     app.StartPath,
		ConfigPaths:   mustMarshalJSON(app.ConfigPaths),
		SitePaths:     mustMarshalJSON(app.SitePaths),
		ListenPorts:   mustMarshalJSON(app.ListenPorts),
		RunUser:       app.RunUser,
		RelatedPIDs:   mustMarshalJSON(app.RelatedPIDs),
		AIConfidence:  app.Confidence,
		AIEvidence:    mustMarshalJSON(app.Evidence),
		ReviewStatus:  "auto",
		Status:        app.Status,
		Fingerprint:   generateAppFingerprint(hostID.String(), app.Category, app.Name, app.InstallPath, app.ListenPorts),
		LastSeenAt:    time.Now(),
		CollectedAt:   time.Now(),
	}
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
func generateAppFingerprint(hostID, category, name, installPath string, listenPorts []int) string {
	portStr := ""
	for _, p := range listenPorts {
		portStr += fmt.Sprintf(":%d", p)
	}
	return fmt.Sprintf("%x", sha256Sum(fmt.Sprintf("%s:%s:%s:%s:%s", hostID, category, name, installPath, portStr)))
}

// applicationAnalysisSystemPrompt 应用分析系统提示
const applicationAnalysisSystemPrompt = `你是主机应用识别专家。根据进程快照识别主机上运行的应用程序。

## 任务
1. 识别每个应用的名称、类型和版本
2. 将应用分类为：database, web_service, web_framework, web_site, other, unknown
3. 评估识别置信度（0-1）
4. 提供识别证据

## 可用工具
当无法从进程快照确定版本或需要更多信息时，可以调用以下工具：

- AssetGetProcessVersion: 获取进程版本。参数: pid (int), exe_path (string), hint (string)
- AssetResolvePackageByFile: 通过文件路径查找所属软件包。参数: path (string)
- AssetReadConfigSummary: 读取配置文件摘要。参数: path (string), max_size (int)
- AssetListDirectoryHints: 列出目录文件。参数: path (string), max_entries (int)
- AssetReadProcFile: 读取 /proc/{pid}/ 下的文件。参数: pid (int), file_name (string)

## 工具调用格式（ReAct）
Thought: [你的推理，说明为什么要调用工具]
Action: [工具名]
Action Input: [JSON 格式参数]

## 工具调用限制（重要）
- 每个进程的每个工具只能调用一次，请合理规划调用顺序
- 优先调用 AssetGetProcessVersion 获取版本
- 如果版本工具失败，再尝试 AssetResolvePackageByFile
- AssetReadProcFile 最大读取 10KB，禁止读取 environ 和 mem
- 总工具调用次数最多 10 次，之后必须输出 Final Answer

## 分类规则
- database: MySQL, MariaDB, PostgreSQL, Redis, MongoDB, Elasticsearch 等
- web_service: Nginx, Apache, Tomcat, Jetty 等 Web 服务器
- web_framework: Spring Boot, Django, Flask, Laravel, Express 等框架应用
- web_site: 具体的网站站点，有域名、根目录等
- other: 其他类型应用
- unknown: 无法确定的应用

## 进程关联性分析（重要）
一个应用可能启动多个相关进程，你必须识别并合并它们：

### 常见多进程应用模式
1. **Nginx**: 1个 master 进程 + N个 worker 进程 → 合并为1个应用
2. **PostgreSQL**: postgres, postmaster, pg_dump, postgres stats → 合并为1个应用
3. **Redis**: redis-server, redis-sentinel → 根据实际部署分别识别
4. **Java 应用**: 可能有多个 JVM 进程，通过 classpath/jar 名称判断是否同一应用
5. **Docker**: dockerd, containerd, containerd-shim → 合并为 Docker 1个应用
6. **Systemd**: systemd, systemd-journald, systemd-resolved → 分别识别为不同服务

### 合并规则
- 相同 exe_path 的进程 → 同一应用，PID 列入 related_pids
- 父子进程关系（PPID 关联）→ 同一应用
- 相同 run_user 且相同功能的进程 → 同一应用
- 相同监听端口的进程 → 同一应用

### 去重检查
输出前检查：
- 同一个 name 只能出现一次
- related_pids 不能重复出现在不同应用中
- 相同 install_path 的进程必须合并

## 最终输出格式
当收集到足够信息后，输出 Final Answer：
Final Answer: {"applications": [...]}

每个应用包含：
{
  "name": "nginx",
  "display_name": "Nginx",
  "category": "web_service",
  "version": "1.24.0",
  "confidence": 0.95,
  "evidence": ["comm=nginx", "listen=80,443", "version_tool=1.24.0"],
  "related_pids": [123, 124, 125],
  "install_path": "/usr/sbin/nginx",
  "start_path": "/",
  "config_paths": ["/etc/nginx/nginx.conf"],
  "site_paths": ["/var/www/html"],
  "listen_ports": [80, 443],
  "run_user": "www-data",
  "status": "active"
}

## 约束
- 不要编造不存在的应用
- 版本号优先来自工具调用结果，其次来自进程快照证据
- 置信度低于 0.3 的标记为 needs_review
- 如果本分片没有可识别应用，输出 Final Answer: {"applications":[]}
- **必须合并相关进程，避免重复应用**`

// Container-related helpers for weak password detection

var (
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

func normalizePIDList(pids []int) []int {
	return mergePIDs(nil, pids)
}

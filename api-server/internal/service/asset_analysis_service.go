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

	sb.WriteString("## Host information\n")
	sb.WriteString(fmt.Sprintf("- Hostname: %s\n", snapshot.Hostname))
	sb.WriteString(fmt.Sprintf("- IP: %s\n", snapshot.IPAddress))
	sb.WriteString(fmt.Sprintf("- Operating system: %s %s\n", snapshot.OSType, snapshot.OSVersion))
	sb.WriteString(fmt.Sprintf("- Architecture: %s\n", snapshot.Arch))

	sb.WriteString("\n## Process snapshot chunk\n")
	sb.WriteString(fmt.Sprintf("- Chunk: %d/%d\n", batchIndex, batchTotal))
	sb.WriteString(fmt.Sprintf("- Processes in this chunk: %d\n", len(snapshot.Processes)))

	sb.WriteString("\n## Processes\n")
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
			messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("Observation: invalid tool-call format: %v. Use one listed asset tool with valid JSON arguments, or output Final Answer.", err)})
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
			messages = append(messages, llm.Message{Role: "user", Content: "Continue the analysis or output Final Answer."})
			continue
		}

		// 检查频率限制
		callKey := assetToolCallKey(toolCall.Tool, toolCall.Args)
		if calledTools[callKey] {
			// 已调用过，告诉 LLM
			messages = append(messages, llm.Message{Role: "assistant", Content: response})
			messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("Observation: tool %s was already called for this process. Use another relevant tool or output Final Answer.", toolCall.Tool)})
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
	messages = append(messages, llm.Message{Role: "user", Content: "The tool-call limit has been reached. Output Final Answer now."})
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
const applicationAnalysisSystemPrompt = `You are a host-application identification expert. Identify applications running on the host from the process snapshot.

## Task
1. Identify each application's name, category, and version.
2. Use one category: database, web_service, web_framework, web_site, other, or unknown.
3. Assign confidence from 0 to 1.
4. Provide concrete identification evidence.

## Available tools
Use these tools only when the snapshot does not establish a version or other required detail:
- AssetGetProcessVersion: pid (int), exe_path (string), hint (string)
- AssetResolvePackageByFile: path (string)
- AssetReadConfigSummary: path (string), max_size (int)
- AssetListDirectoryHints: path (string), max_entries (int)
- AssetReadProcFile: pid (int), file_name (string)

## ReAct tool format
Thought: [why additional evidence is needed]
Action: [exact tool name]
Action Input: [JSON arguments]

## Tool limits
- Call a given tool at most once per process.
- Prefer AssetGetProcessVersion for version evidence, then AssetResolvePackageByFile when needed.
- AssetReadProcFile may read at most 10 KB and must never read environ or mem.
- Use at most ten total tool calls, then produce Final Answer.

## Categories
- database: MySQL, MariaDB, PostgreSQL, Redis, MongoDB, Elasticsearch, and similar systems.
- web_service: Nginx, Apache, Tomcat, Jetty, and similar servers.
- web_framework: Spring Boot, Django, Flask, Laravel, Express, and similar frameworks.
- web_site: a concrete site with a domain or document root.
- other: another identifiable application category.
- unknown: insufficient evidence.

## Process correlation and deduplication
One application may own multiple related processes. Merge processes when supported by matching executable path, parent-child relationships, shared function and user, shared listening endpoints, or application-specific evidence.

Typical patterns:
- Merge Nginx master and workers into one application.
- Merge PostgreSQL server-related processes when they belong to the same installation.
- Distinguish Redis server and sentinel according to deployment evidence.
- Use classpath or JAR names to group Java processes.
- Merge dockerd, containerd, and relevant shims into one Docker installation when evidence supports it.
- Treat distinct systemd services separately.

Before output:
- Emit each application name once.
- Do not place one PID in multiple applications.
- Merge processes with the same installation path when they represent the same application.

## Final output
When evidence is sufficient, output:
Final Answer: {"applications": [...]}

Each application has:
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

## Constraints
- Never invent an application.
- Prefer version evidence from tools, then from the process snapshot.
- Mark confidence below 0.3 as needs_review.
- If this chunk has no identifiable application, output Final Answer: {"applications":[]}.
- Merge related processes and avoid duplicate applications.`

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

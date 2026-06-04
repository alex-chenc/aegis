package service

import (
	"context"
	"encoding/json"
	"fmt"
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
	repo       *repository.AssetCollectionRepository
	configRepo ConfigRepositoryInterface
	logger     *zap.Logger
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
	logger *zap.Logger,
) *AssetAnalysisService {
	return &AssetAnalysisService{
		repo:       repo,
		configRepo: configRepo,
		logger:     logger,
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

	batches := splitProcessBatches(snapshot.Processes, applicationAnalysisProcessBatchSize)
	totalSaved := 0
	failedBatches := 0
	var firstBatchErr error

	for i, batch := range batches {
		chunkSnapshot := snapshot
		chunkSnapshot.Processes = batch

		prompt := s.buildAnalysisPrompt(chunkSnapshot, i+1, len(batches))
		response, err := s.completeApplicationAnalysis(ctx, llmClient, prompt)
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

	return nil
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
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Category    string   `json:"category"`
	Version     string   `json:"version"`
	Confidence  float64  `json:"confidence"`
	Evidence    []string `json:"evidence"`
	RelatedPIDs []int    `json:"related_pids"`
	InstallPath string   `json:"install_path"`
	StartPath   string   `json:"start_path"`
	ConfigPaths []string `json:"config_paths"`
	SitePaths   []string `json:"site_paths"`
	ListenPorts []int    `json:"listen_ports"`
	RunUser     string   `json:"run_user"`
	Status      string   `json:"status"`
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

	if strings.HasPrefix(text, "[") {
		return extractJSONEnvelope(text, "[", "]")
	}
	if candidate := extractJSONEnvelope(text, "{", "}"); candidate != "" {
		return candidate
	}
	if candidate := extractJSONEnvelope(text, "[", "]"); candidate != "" {
		return candidate
	}

	return ""
}

func extractJSONEnvelope(text, open, close string) string {
	start := strings.Index(text, open)
	if start == -1 {
		return ""
	}

	end := strings.LastIndex(text, close)
	if end == -1 || end <= start {
		return ""
	}

	return strings.TrimSpace(text[start : end+1])
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
const applicationAnalysisSystemPrompt = `你是主机应用识别专家。只根据进程快照识别主机上运行的应用程序。

## 任务
1. 识别每个应用的名称、类型和版本
2. 将应用分类为：database, web_service, web_framework, web_site, other, unknown
3. 评估识别置信度（0-1）
4. 提供识别证据

## 分类规则
- database: MySQL, MariaDB, PostgreSQL, Redis, MongoDB, Elasticsearch 等
- web_service: Nginx, Apache, Tomcat, Jetty 等 Web 服务器
- web_framework: Spring Boot, Django, Flask, Laravel, Express 等框架应用
- web_site: 具体的网站站点，有域名、根目录等
- other: 其他类型应用
- unknown: 无法确定的应用

## 输出格式
输出 JSON 格式：
{
  "applications": [
    {
      "name": "nginx",
      "display_name": "Nginx",
      "category": "web_service",
      "version": "1.24.0",
      "confidence": 0.95,
      "evidence": ["comm=nginx", "listen=80,443"],
      "related_pids": [123, 124],
      "install_path": "/usr/sbin/nginx",
      "start_path": "/",
      "config_paths": ["/etc/nginx/nginx.conf"],
      "site_paths": ["/var/www/html"],
      "listen_ports": [80, 443],
      "run_user": "www-data",
      "status": "active"
    }
  ]
}

## 约束
- 不要编造不存在的应用
- 版本号必须来自实际证据，不要猜测
- 置信度低于 0.3 的标记为 needs_review
- 如果本分片没有可识别应用，输出 {"applications":[]}
- 只输出 JSON，不要其他解释`

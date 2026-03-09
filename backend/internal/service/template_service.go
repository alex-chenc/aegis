package service

import (
	"context"
	"fmt"
	"io"
	"strings"

	"baseline-system/internal/fileparser"
	"baseline-system/internal/llm"
	"baseline-system/internal/model"
	"baseline-system/internal/repository"
	"baseline-system/internal/storage"
	"baseline-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TemplateService struct {
	templateRepo  *repository.TemplateRepository
	ruleRepo      *repository.RuleRepository
	configRepo    *repository.ConfigRepository
	minioClient   *storage.MinIOClient
	redisClient   *storage.RedisClient
	llmTimeout    int
	llmMaxRetries int
	parseQueue    chan ParseTask
	workerCount   int
}

type ParseTask struct {
	TemplateID uuid.UUID
	FileType   fileparser.FileType
	MinIOPath  string
}

func NewTemplateService(
	templateRepo *repository.TemplateRepository,
	ruleRepo *repository.RuleRepository,
	configRepo *repository.ConfigRepository,
	minioClient *storage.MinIOClient,
	redisClient *storage.RedisClient,
	llmTimeout int,
	llmMaxRetries int,
	workerCount int,
) *TemplateService {
	return &TemplateService{
		templateRepo:  templateRepo,
		ruleRepo:      ruleRepo,
		configRepo:    configRepo,
		minioClient:   minioClient,
		redisClient:   redisClient,
		llmTimeout:    llmTimeout,
		llmMaxRetries: llmMaxRetries,
		parseQueue:    make(chan ParseTask, 100),
		workerCount:   workerCount,
	}
}

// StartWorkers 启动解析 Worker
func (s *TemplateService) StartWorkers(ctx context.Context) {
	logger.Info("starting template parse workers",
		zap.Int("count", s.workerCount),
	)

	for i := 0; i < s.workerCount; i++ {
		go s.parseWorker(ctx, i)
	}
}

func (s *TemplateService) parseWorker(ctx context.Context, workerID int) {
	logger.Info("parse worker started",
		zap.Int("worker_id", workerID),
	)

	for {
		select {
		case <-ctx.Done():
			logger.Info("parse worker stopped",
				zap.Int("worker_id", workerID),
			)
			return
		case task := <-s.parseQueue:
			s.processTemplate(ctx, workerID, task)
		}
	}
}

func (s *TemplateService) processTemplate(ctx context.Context, workerID int, task ParseTask) {
	logger.Info("processing template",
		zap.Int("worker_id", workerID),
		zap.String("template_id", task.TemplateID.String()),
	)

	// 更新 Redis 状态
	s.updateParseStatus(task.TemplateID, "parsing", 20, "下载文件中...")

	// 从 MinIO 下载文件
	reader, err := s.minioClient.DownloadFile("baseline-templates", task.MinIOPath)
	if err != nil {
		logger.Error("failed to download file from MinIO",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, fmt.Sprintf("下载文件失败：%v", err))
		return
	}
	defer reader.Close()

	s.updateParseStatus(task.TemplateID, "parsing", 40, "解析文件内容...")

	// 解析文件内容
	parser, err := fileparser.GetParser(task.FileType)
	if err != nil {
		logger.Error("failed to get parser",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, fmt.Sprintf("不支持的文件类型：%v", err))
		return
	}

	content, err := parser.Parse(reader)
	if err != nil {
		logger.Error("failed to parse file",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, fmt.Sprintf("解析文件失败：%v", err))
		return
	}

	s.updateParseStatus(task.TemplateID, "parsing", 60, "调用 LLM 提取规则...")

	config, err := s.configRepo.GetActive()
	if err != nil {
		logger.Error("failed to get LLM config",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, "LLM配置未设置，请先在设置页面配置API Key")
		return
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		logger.Error("failed to decrypt API key",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, "API Key解密失败")
		return
	}

	llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmMaxRetries)

	prompt := llm.GetRuleExtractionPrompt(content)
	llmResponse, err := llmClient.ChatCompletion(ctx, "你是一位安全基线专家", prompt, 0.1)
	if err != nil {
		logger.Error("failed to call LLM",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, fmt.Sprintf("LLM 调用失败：%v", err))
		return
	}

	s.updateParseStatus(task.TemplateID, "parsing", 80, "解析 LLM 响应...")

	// 解析 LLM 响应
	rules, err := llm.ParseRules(llmResponse)
	if err != nil {
		logger.Error("failed to parse rules",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, fmt.Sprintf("解析规则失败：%v", err))
		return
	}

	// 去重
	rules = llm.ValidateRules(rules)

	// 设置 template_id
	for _, rule := range rules {
		rule.TemplateID = task.TemplateID
	}

	s.updateParseStatus(task.TemplateID, "parsing", 90, "保存规则到数据库...")

	// 批量保存规则
	if err := s.ruleRepo.BatchCreate(rules); err != nil {
		logger.Error("failed to save rules",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, fmt.Sprintf("保存规则失败：%v", err))
		return
	}

	// 更新模板状态
	if err := s.templateRepo.UpdateStatus(task.TemplateID, "completed", nil, len(rules)); err != nil {
		logger.Error("failed to update template status",
			zap.Error(err),
			zap.String("template_id", task.TemplateID.String()),
		)
		s.updateParseStatus(task.TemplateID, "failed", 0, fmt.Sprintf("更新状态失败：%v", err))
		return
	}

	// 更新 Redis 状态
	s.updateParseStatus(task.TemplateID, "completed", 100, fmt.Sprintf("解析完成，共提取 %d 条规则", len(rules)))

	logger.Info("template parsing completed",
		zap.String("template_id", task.TemplateID.String()),
		zap.Int("rule_count", len(rules)),
	)
}

func (s *TemplateService) updateParseStatus(templateID uuid.UUID, status string, progress int, message string) {
	if err := s.redisClient.SetParseStatus(templateID.String(), status, progress, message); err != nil {
		logger.Error("failed to update parse status in Redis",
			zap.Error(err),
			zap.String("template_id", templateID.String()),
		)
	}
}

// QueueTemplate 将模板加入解析队列
func (s *TemplateService) QueueTemplate(templateID uuid.UUID, fileType fileparser.FileType, minIOPath string) error {
	task := ParseTask{
		TemplateID: templateID,
		FileType:   fileType,
		MinIOPath:  minIOPath,
	}

	select {
	case s.parseQueue <- task:
		logger.Info("template queued for parsing",
			zap.String("template_id", templateID.String()),
		)
		return nil
	default:
		logger.Error("parse queue is full",
			zap.String("template_id", templateID.String()),
		)
		return fmt.Errorf("parse queue is full")
	}
}

// UploadTemplate 上传模板文件
func (s *TemplateService) UploadTemplate(ctx context.Context, filename string, reader io.Reader, fileSize int64) (*model.Template, error) {
	// 生成 template_id
	templateID := uuid.New()

	// 确定文件类型
	var fileType fileparser.FileType
	ext := filename[strings.LastIndex(filename, "."):]
	switch strings.ToLower(ext) {
	case ".yaml", ".yml":
		fileType = fileparser.FileTypeYAML
	case ".txt", ".text":
		fileType = fileparser.FileTypeTXT
	case ".md", ".markdown":
		fileType = fileparser.FileTypeMarkdown
	case ".pdf":
		fileType = fileparser.FileTypePDF
	case ".docx", ".doc":
		fileType = fileparser.FileTypeWord
	case ".xlsx", ".xls":
		fileType = fileparser.FileTypeExcel
	default:
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}

	// 生成 MinIO 对象名
	minIOPath := fmt.Sprintf("%s/%s", templateID.String(), filename)
	contentType := storage.GetContentType(filename)

	// 上传到 MinIO
	_, err := s.minioClient.UploadFile("baseline-templates", minIOPath, reader, fileSize, contentType)
	if err != nil {
		logger.Error("failed to upload file to MinIO",
			zap.Error(err),
			zap.String("template_id", templateID.String()),
		)
		return nil, fmt.Errorf("上传文件失败：%w", err)
	}

	// 创建数据库记录
	template := &model.Template{
		ID:              templateID,
		Name:            filename,
		FileType:        string(fileType),
		MinioObjectName: minIOPath,
		Status:          "parsing",
	}

	if err := s.templateRepo.Create(template); err != nil {
		logger.Error("failed to create template record",
			zap.Error(err),
			zap.String("template_id", templateID.String()),
		)
		return nil, fmt.Errorf("创建模板记录失败：%w", err)
	}

	// 初始化 Redis 状态
	if err := s.redisClient.SetParseStatus(templateID.String(), "parsing", 0, "文件已上传，等待解析..."); err != nil {
		logger.Error("failed to initialize Redis status",
			zap.Error(err),
			zap.String("template_id", templateID.String()),
		)
	}

	// 加入解析队列
	if err := s.QueueTemplate(templateID, fileType, minIOPath); err != nil {
		logger.Error("failed to queue template",
			zap.Error(err),
			zap.String("template_id", templateID.String()),
		)
		return nil, err
	}

	logger.Info("template uploaded successfully",
		zap.String("template_id", templateID.String()),
		zap.String("filename", filename),
		zap.String("file_type", string(fileType)),
	)

	return template, nil
}

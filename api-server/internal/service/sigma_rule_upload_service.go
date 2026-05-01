package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"
	"api-server/pkg/logger"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const (
	MaxFileSize = 10 * 1024 * 1024 // 10MB
)

// SigmaRuleUploadService Sigma规则上传服务
type SigmaRuleUploadService struct {
	ruleRepo     *repository.SigmaRuleRepository
	serverClient *grpcclient.ServerClient
	parser       *SigmaRuleUploadParser
}

// SigmaRuleUploadParser Sigma规则上传解析器（内部使用）
type SigmaRuleUploadParser struct{}

// ParsedRule 解析后的规则信息
type ParsedRule struct {
	RuleID     string `json:"rule_id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	MitreID    string `json:"mitre_id,omitempty"`
	Severity   string `json:"severity,omitempty"`
	ParseError string `json:"parse_error,omitempty"`
}

// UploadResult 上传结果
type UploadResult struct {
	Success      bool         `json:"success"`
	ParsedCount  int          `json:"parsed_count"`
	FailedCount  int          `json:"failed_count"`
	SkippedCount int          `json:"skipped_count"`
	Rules        []ParsedRule `json:"rules,omitempty"`
	FailedFiles  []string     `json:"failed_files,omitempty"`
	Error        string       `json:"error,omitempty"`
}

func NewSigmaRuleUploadService(ruleRepo *repository.SigmaRuleRepository, serverClient *grpcclient.ServerClient) *SigmaRuleUploadService {
	return &SigmaRuleUploadService{
		ruleRepo:     ruleRepo,
		serverClient: serverClient,
		parser:       &SigmaRuleUploadParser{},
	}
}

// UploadRules 上传并解析Sigma规则文件
func (s *SigmaRuleUploadService) UploadRules(file io.Reader, fileName string, fileSize int64) (*UploadResult, error) {
	ext := strings.ToLower(filepath.Ext(fileName))

	// 检查文件大小
	if fileSize > MaxFileSize {
		return &UploadResult{
			Success:     false,
			Error:       fmt.Sprintf("file size exceeds maximum limit of %d MB", MaxFileSize/1024/1024),
			ParsedCount: 0,
		}, nil
	}

	switch ext {
	case ".yaml", ".yml":
		return s.parseSingleFile(file, fileName)
	case ".zip":
		return s.parseZipFile(file)
	default:
		return &UploadResult{
			Success:     false,
			Error:       fmt.Sprintf("unsupported file format: %s", ext),
			ParsedCount: 0,
		}, nil
	}
}

// parseSingleFile 解析单个YAML文件
func (s *SigmaRuleUploadService) parseSingleFile(file io.Reader, fileName string) (*UploadResult, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 计算文件hash
	fileHash := computeHash(content)

	// 检查是否重复
	existing, err := s.ruleRepo.FindByFileHash(fileHash)
	if err == nil && existing != nil {
		return &UploadResult{
			Success:      true,
			ParsedCount:  0,
			SkippedCount: 1,
			Rules: []ParsedRule{
				{
					RuleID: existing.RuleID,
					Title:  existing.Title,
					Status: "skipped_duplicate",
				},
			},
		}, nil
	}

	// 解析规则
	parsedRule, err := s.parser.Parse(content)
	if err != nil {
		return &UploadResult{
			Success:     false,
			ParsedCount: 0,
			FailedCount: 1,
			FailedFiles: []string{fileName},
			Error:       err.Error(),
		}, nil
	}

	// 提取MITRE ID用于验证和去重
	mitreID := s.extractMitreID(parsedRule.Tags)

	// 验证MITRE ID不能为空
	if mitreID == "" {
		return &UploadResult{
			Success:     false,
			ParsedCount: 0,
			FailedCount: 1,
			FailedFiles: []string{fileName},
			Error:       "MITRE ID is required, rule cannot be imported without MITRE attribution",
		}, nil
	}

	// 检查MITRE ID是否已存在（去重）
	upperMitreID := strings.ToUpper(mitreID)
	if !strings.HasPrefix(upperMitreID, "T") {
		upperMitreID = "T" + upperMitreID
	}
	exists, err := s.ruleRepo.ExistsByMitreID(upperMitreID)
	if err == nil && exists {
		return &UploadResult{
			Success:      true,
			ParsedCount:  0,
			SkippedCount: 1,
			Rules: []ParsedRule{
				{
					RuleID:  parsedRule.ID,
					Title:   parsedRule.Title,
					Status:  "skipped_duplicate",
					MitreID: upperMitreID,
				},
			},
		}, nil
	}

	// 保存到数据库
	model := s.createSigmaRuleModel(parsedRule, content, fileName, fileHash, len(content), upperMitreID)
	if err := s.ruleRepo.Create(model); err != nil {
		return nil, fmt.Errorf("failed to save rule: %w", err)
	}

	return &UploadResult{
		Success:     true,
		ParsedCount: 1,
		Rules: []ParsedRule{
			{
				RuleID:   model.RuleID,
				Title:    model.Title,
				Status:   model.Status,
				MitreID:  model.MitreID,
				Severity: model.Severity,
			},
		},
	}, nil
}

// parseZipFile 解析ZIP压缩包
func (s *SigmaRuleUploadService) parseZipFile(file io.Reader) (*UploadResult, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read zip file: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return &UploadResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read zip file: %v", err),
		}, nil
	}

	result := &UploadResult{
		Success:      true,
		ParsedCount:  0,
		FailedCount:  0,
		SkippedCount: 0,
		Rules:        []ParsedRule{},
		FailedFiles:  []string{},
	}

	for _, f := range reader.File {
		// 跳过目录和非YAML文件
		if f.FileInfo().IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		// 打开文件
		rc, err := f.Open()
		if err != nil {
			result.FailedCount++
			result.FailedFiles = append(result.FailedFiles, f.Name)
			continue
		}

		fileContent, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			result.FailedCount++
			result.FailedFiles = append(result.FailedFiles, f.Name)
			continue
		}

		// 计算hash检查重复
		fileHash := computeHash(fileContent)
		existing, err := s.ruleRepo.FindByFileHash(fileHash)
		if err == nil && existing != nil {
			result.SkippedCount++
			result.Rules = append(result.Rules, ParsedRule{
				RuleID: existing.RuleID,
				Title:  existing.Title,
				Status: "skipped_duplicate",
			})
			continue
		}

		// 解析规则
		parsedRule, err := s.parser.Parse(fileContent)
		if err != nil {
			result.FailedCount++
			result.FailedFiles = append(result.FailedFiles, f.Name)
			continue
		}

		// 提取MITRE ID用于验证和去重
		mitreID := s.extractMitreID(parsedRule.Tags)

		// 验证MITRE ID不能为空
		if mitreID == "" {
			result.FailedCount++
			result.FailedFiles = append(result.FailedFiles, f.Name+": MITRE ID is required")
			continue
		}

		// 检查MITRE ID是否已存在（去重）
		upperMitreID := strings.ToUpper(mitreID)
		if !strings.HasPrefix(upperMitreID, "T") {
			upperMitreID = "T" + upperMitreID
		}
		exists, err := s.ruleRepo.ExistsByMitreID(upperMitreID)
		if err == nil && exists {
			result.SkippedCount++
			result.Rules = append(result.Rules, ParsedRule{
				RuleID:  parsedRule.ID,
				Title:   parsedRule.Title,
				Status:  "skipped_duplicate",
				MitreID: upperMitreID,
			})
			continue
		}

		// 保存到数据库
		model := s.createSigmaRuleModel(parsedRule, fileContent, f.Name, fileHash, len(fileContent), upperMitreID)
		if err := s.ruleRepo.Create(model); err != nil {
			logger.Warn("failed to save rule from zip",
				zap.String("file", f.Name),
				zap.String("rule_id", parsedRule.ID),
				zap.Error(err))
			result.FailedCount++
			result.FailedFiles = append(result.FailedFiles, f.Name)
			continue
		}

		result.ParsedCount++
		result.Rules = append(result.Rules, ParsedRule{
			RuleID:   model.RuleID,
			Title:    model.Title,
			Status:   model.Status,
			MitreID:  model.MitreID,
			Severity: model.Severity,
		})
	}

	return result, nil
}

// ApproveRule 审批规则并精确下发
func (s *SigmaRuleUploadService) ApproveRule(ruleID string, targetHostIDs []string) error {
	rule, err := s.ruleRepo.FindByID(ruleID)
	if err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}

	nextStatus := "experimental"
	if rule.Status == "experimental" {
		nextStatus = "active"
	} else if rule.Status == "active" {
		nextStatus = "active"
	}

	if err := s.ruleRepo.UpdateStatusWithApproval(ruleID, nextStatus, ""); err != nil {
		return fmt.Errorf("failed to update rule status: %w", err)
	}

	// 精确下发到目标主机
	return s.dispatchRuleToHosts(rule, targetHostIDs)
}

// dispatchRuleToHosts 精确下发规则到指定主机
func (s *SigmaRuleUploadService) dispatchRuleToHosts(rule *model.SigmaRule, hostIDs []string) error {
	if s.serverClient == nil {
		logger.Warn("server client not available for rule dispatch")
		return nil
	}

	if len(hostIDs) == 0 {
		// 空数组表示全量下发，使用广播
		return s.broadcastRuleToAllHosts(rule)
	}

	// 精确下发到指定主机
	for _, hostID := range hostIDs {
		err := s.dispatchToSingleHost(rule, hostID)
		if err != nil {
			logger.Warn("failed to dispatch rule to host",
				zap.String("host_id", hostID),
				zap.String("rule_id", rule.RuleID),
				zap.Error(err))
		}
	}

	// 更新下发状态
	if err := s.ruleRepo.UpdateDispatchStatus(rule.RuleID, hostIDs, "dispatched"); err != nil {
		logger.Warn("failed to update dispatch status",
			zap.String("rule_id", rule.RuleID),
			zap.Error(err))
	}

	return nil
}

// broadcastRuleToAllHosts 广播规则到所有主机
func (s *SigmaRuleUploadService) broadcastRuleToAllHosts(rule *model.SigmaRule) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.serverClient.UpdateAgentRules(ctx, &pb.UpdateAgentRulesRequest{
		Action: "incremental",
		Rules: []*pb.AgentRuleUpdate{
			{
				RuleId:  rule.RuleID,
				Action:  "add",
				Content: rule.Content,
			},
		},
	})

	if err != nil {
		logger.Warn("failed to broadcast rule update",
			zap.String("rule_id", rule.RuleID),
			zap.Error(err))
		return err
	}

	logger.Info("rule broadcasted to all hosts",
		zap.String("rule_id", rule.RuleID))
	return nil
}

// dispatchToSingleHost 下发规则到单个主机
func (s *SigmaRuleUploadService) dispatchToSingleHost(rule *model.SigmaRule, hostID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.serverClient.UpdateAgentRules(ctx, &pb.UpdateAgentRulesRequest{
		HostId: hostID, // 设置host_id实现精确下发
		Action: "incremental",
		Rules: []*pb.AgentRuleUpdate{
			{
				RuleId:  rule.RuleID,
				Action:  "add",
				Content: rule.Content,
			},
		},
	})

	if err != nil {
		return err
	}

	logger.Info("rule dispatched to host",
		zap.String("rule_id", rule.RuleID),
		zap.String("host_id", hostID))
	return nil
}

// createSigmaRuleModel 创建SigmaRule模型
func (s *SigmaRuleUploadService) createSigmaRuleModel(
	parsed *ParsedSigmaRule,
	content []byte,
	fileName string,
	fileHash string,
	fileSize int,
	mitreID string,
) *model.SigmaRule {
	// 规范化severity
	severity := strings.ToLower(parsed.Level)
	if severity == "" {
		severity = "medium"
	}

	now := time.Now()
	return &model.SigmaRule{
		RuleID:         parsed.ID,
		Title:          parsed.Title,
		Description:    parsed.Description,
		Content:        string(content),
		Status:         "pending", // 初始状态为pending
		MitreID:        mitreID,
		Severity:       severity,
		GeneratedBy:    "upload",
		Version:        "1.0",
		CreatedAt:      now,
		UpdatedAt:      now,
		Source:         "upload",
		FileName:       fileName,
		FileHash:       fileHash,
		FileSize:       fileSize,
		ParsedAt:       &now,
		DispatchHosts:  "[]",
		DispatchStatus: "pending",
	}
}

// Parse 解析Sigma规则YAML
func (p *SigmaRuleUploadParser) Parse(content []byte) (*ParsedSigmaRule, error) {
	var rawRule struct {
		Title       string                 `yaml:"title"`
		ID          string                 `yaml:"id"`
		Status      string                 `yaml:"status"`
		Description string                 `yaml:"description"`
		Level       string                 `yaml:"level"`
		Tags        []string               `yaml:"tags"`
		Logsource   map[string]interface{} `yaml:"logsource"`
		Detection   map[string]interface{} `yaml:"detection"`
	}

	if err := yaml.Unmarshal(content, &rawRule); err != nil {
		return nil, fmt.Errorf("invalid YAML format: %w", err)
	}

	// 验证必填字段
	if rawRule.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if len(rawRule.Detection) == 0 {
		return nil, fmt.Errorf("detection is required")
	}

	// 生成规则ID（如果未提供）
	ruleID := rawRule.ID
	if ruleID == "" {
		ruleID = generateRuleID(rawRule.Title)
	}

	// 设置默认状态
	if rawRule.Status == "" {
		rawRule.Status = "experimental"
	}

	return &ParsedSigmaRule{
		Title:       rawRule.Title,
		ID:          ruleID,
		Status:      rawRule.Status,
		Description: rawRule.Description,
		Level:       rawRule.Level,
		Tags:        rawRule.Tags,
		Logsource:   rawRule.Logsource,
		Detection:   rawRule.Detection,
	}, nil
}

// ParsedSigmaRule 解析后的Sigma规则
type ParsedSigmaRule struct {
	Title       string
	ID          string
	Status      string
	Description string
	Level       string
	Tags        []string
	Logsource   map[string]interface{}
	Detection   map[string]interface{}
}

// extractMitreID 从tags中提取MITRE ID
func (s *SigmaRuleUploadService) extractMitreID(tags []string) string {
	mitreRegex := regexp.MustCompile(`T\d{4}(?:\.\d{3})?`)
	for _, tag := range tags {
		if strings.HasPrefix(strings.ToLower(tag), "attack.t") {
			rawMitre := strings.TrimPrefix(strings.ToLower(tag), "attack.")
			upper := strings.ToUpper(rawMitre)
			if match := mitreRegex.FindString(upper); match != "" {
				return match
			}
		}
	}
	return ""
}

// computeHash 计算SHA256哈希
func computeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// generateRuleID 从title生成规则ID
func generateRuleID(title string) string {
	// 转小写，替换空格和特殊字符
	id := strings.ToLower(title)
	id = strings.ReplaceAll(id, " ", "_")
	re := strings.NewReplacer(
		"-", "_", ".", "_", "/", "_", "\\", "_",
		":", "_", "*", "_", "?", "_", "\"", "_",
		"<", "_", ">", "_", "|", "_",
	)
	id = re.Replace(id)
	id = strings.Trim(id, "_")
	// 去除连续的下划线
	for strings.Contains(id, "__") {
		id = strings.ReplaceAll(id, "__", "_")
	}
	// 截断长度
	if len(id) > 50 {
		id = id[:50]
	}
	// 添加随机后缀
	return fmt.Sprintf("%s_%s", id, randomString(6))
}

// randomString 生成随机字符串
func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return string(result)
}

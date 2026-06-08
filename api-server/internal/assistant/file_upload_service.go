package assistant

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	"api-server/internal/fileparser"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	AssistantUploadPurposeAnalysis         = "analysis"
	AssistantUploadPurposeBaselineTemplate = "baseline_template"
	AssistantUploadPurposeSigmaRule        = "sigma_rule"

	assistantUploadMaxBytes         = 10 * 1024 * 1024
	assistantBaselineUploadMaxBytes = 5 * 1024 * 1024
	assistantSummaryMaxRunes        = 12000
	assistantPreviewMaxRunes        = 1200
)

type AssistantTemplateUploadService interface {
	UploadTemplate(ctx context.Context, filename string, reader io.Reader, fileSize int64, fileMD5 string) (*model.Template, error)
}

type AssistantSigmaRuleUploadService interface {
	UploadRules(file io.Reader, fileName string, fileSize int64) (*service.UploadResult, error)
}

type FileUploadService struct {
	contextRepo     repository.AssistantContextRefRepository
	templateService AssistantTemplateUploadService
	sigmaService    AssistantSigmaRuleUploadService
	logger          *zap.Logger
}

type FileUploadServiceDeps struct {
	ContextRepo     repository.AssistantContextRefRepository
	TemplateService AssistantTemplateUploadService
	SigmaService    AssistantSigmaRuleUploadService
	Logger          *zap.Logger
}

type AssistantFileUploadResult struct {
	Purpose    string                    `json:"purpose"`
	Filename   string                    `json:"filename"`
	Size       int64                     `json:"size"`
	ContextRef model.AssistantContextRef `json:"context_ref"`
	Data       map[string]interface{}    `json:"data,omitempty"`
}

func NewFileUploadService(deps FileUploadServiceDeps) *FileUploadService {
	return &FileUploadService{
		contextRepo:     deps.ContextRepo,
		templateService: deps.TemplateService,
		sigmaService:    deps.SigmaService,
		logger:          deps.Logger,
	}
}

func (s *FileUploadService) UploadSessionFile(ctx context.Context, sessionID string, header *multipart.FileHeader, purpose string, operator string) (*AssistantFileUploadResult, error) {
	if s == nil || s.contextRepo == nil {
		return nil, fmt.Errorf("assistant file upload service not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if header == nil {
		return nil, fmt.Errorf("file is required")
	}

	purpose = normalizeUploadPurpose(purpose)
	if purpose == AssistantUploadPurposeBaselineTemplate && header.Size > assistantBaselineUploadMaxBytes {
		return nil, fmt.Errorf("baseline template file size exceeds 5MB")
	}
	if header.Size > assistantUploadMaxBytes {
		return nil, fmt.Errorf("file size exceeds 10MB")
	}

	file, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, assistantUploadMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	if int64(len(content)) > assistantUploadMaxBytes {
		return nil, fmt.Errorf("file size exceeds 10MB")
	}

	md5Hash := md5.Sum(content)
	shaHash := sha256.Sum256(content)
	fileMD5 := hex.EncodeToString(md5Hash[:])
	fileSHA256 := hex.EncodeToString(shaHash[:])

	var ref model.AssistantContextRef
	data := map[string]interface{}{
		"filename": header.Filename,
		"size":     int64(len(content)),
		"purpose":  purpose,
		"md5":      fileMD5,
		"sha256":   fileSHA256,
	}

	switch purpose {
	case AssistantUploadPurposeBaselineTemplate:
		ref, err = s.uploadBaselineTemplate(ctx, sessionID, header.Filename, content, fileMD5, fileSHA256, operator, data)
	case AssistantUploadPurposeSigmaRule:
		ref, err = s.uploadSigmaRule(ctx, sessionID, header.Filename, content, fileMD5, fileSHA256, operator, data)
	default:
		ref, err = s.uploadAnalysisFile(ctx, sessionID, header.Filename, content, fileMD5, fileSHA256, operator, data)
	}
	if err != nil {
		return nil, err
	}

	if err := s.contextRepo.Create(ctx, &ref); err != nil {
		return nil, fmt.Errorf("failed to create assistant context ref: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("assistant file uploaded",
			zap.String("session_id", sessionID),
			zap.String("filename", header.Filename),
			zap.String("purpose", purpose),
			zap.Int64("size", int64(len(content))),
			zap.String("operator", operator),
		)
	}

	return &AssistantFileUploadResult{
		Purpose:    purpose,
		Filename:   header.Filename,
		Size:       int64(len(content)),
		ContextRef: ref,
		Data:       data,
	}, nil
}

func (s *FileUploadService) uploadAnalysisFile(ctx context.Context, sessionID string, filename string, content []byte, fileMD5 string, fileSHA256 string, operator string, data map[string]interface{}) (model.AssistantContextRef, error) {
	_ = ctx
	parser, err := fileparser.GetParserByExtension(filename)
	if err != nil {
		return model.AssistantContextRef{}, err
	}
	parsed, err := parser.Parse(bytes.NewReader(content))
	if err != nil {
		return model.AssistantContextRef{}, fmt.Errorf("failed to parse uploaded file: %w", err)
	}
	summary := truncateRunes(parsed, assistantSummaryMaxRunes)
	preview := truncateRunes(parsed, assistantPreviewMaxRunes)
	data["content_length"] = len([]rune(parsed))
	data["preview"] = preview
	data["extension"] = strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")

	return model.AssistantContextRef{
		ID:         uuid.New(),
		SessionID:  sessionID,
		ObjectType: "file",
		ObjectID:   "file_" + uuid.New().String(),
		Title:      filename,
		Summary:    summary,
		Snapshot: mustMarshalJSON(map[string]interface{}{
			"filename":       filename,
			"purpose":        AssistantUploadPurposeAnalysis,
			"size":           len(content),
			"md5":            fileMD5,
			"sha256":         fileSHA256,
			"content_length": len([]rune(parsed)),
			"preview":        preview,
			"uploaded_by":    operator,
		}),
	}, nil
}

func (s *FileUploadService) uploadBaselineTemplate(ctx context.Context, sessionID string, filename string, content []byte, fileMD5 string, fileSHA256 string, operator string, data map[string]interface{}) (model.AssistantContextRef, error) {
	if s.templateService == nil {
		return model.AssistantContextRef{}, fmt.Errorf("baseline template upload service not configured")
	}
	template, err := s.templateService.UploadTemplate(ctx, filename, bytes.NewReader(content), int64(len(content)), fileMD5)
	if err != nil {
		return model.AssistantContextRef{}, fmt.Errorf("failed to upload baseline template: %w", err)
	}

	templateID := template.ID.String()
	data["template_id"] = templateID
	data["status"] = template.Status
	data["display_name"] = template.DisplayName

	return model.AssistantContextRef{
		ID:         uuid.New(),
		SessionID:  sessionID,
		ObjectType: "baseline_template",
		ObjectID:   templateID,
		Title:      template.DisplayName,
		Summary:    fmt.Sprintf("基线模板已上传，当前状态：%s，规则数量：%d", template.Status, template.RuleCount),
		RoutePath:  "/baseline",
		Snapshot: mustMarshalJSON(map[string]interface{}{
			"filename":     filename,
			"display_name": template.DisplayName,
			"purpose":      AssistantUploadPurposeBaselineTemplate,
			"template_id":  templateID,
			"status":       template.Status,
			"rule_count":   template.RuleCount,
			"size":         len(content),
			"md5":          fileMD5,
			"sha256":       fileSHA256,
			"uploaded_by":  operator,
		}),
	}, nil
}

func (s *FileUploadService) uploadSigmaRule(ctx context.Context, sessionID string, filename string, content []byte, fileMD5 string, fileSHA256 string, operator string, data map[string]interface{}) (model.AssistantContextRef, error) {
	_ = ctx
	if s.sigmaService == nil {
		return model.AssistantContextRef{}, fmt.Errorf("sigma rule upload service not configured")
	}
	result, err := s.sigmaService.UploadRules(bytes.NewReader(content), filename, int64(len(content)))
	if err != nil {
		return model.AssistantContextRef{}, fmt.Errorf("failed to upload sigma rules: %w", err)
	}
	if result == nil || !result.Success {
		if result != nil && result.Error != "" {
			return model.AssistantContextRef{}, errors.New(result.Error)
		}
		return model.AssistantContextRef{}, fmt.Errorf("sigma rule upload failed")
	}

	data["result"] = result
	summary := fmt.Sprintf("Sigma规则上传完成：成功 %d，失败 %d，跳过 %d", result.ParsedCount, result.FailedCount, result.SkippedCount)
	return model.AssistantContextRef{
		ID:         uuid.New(),
		SessionID:  sessionID,
		ObjectType: "sigma_rule_upload",
		ObjectID:   "sigma_upload_" + uuid.New().String(),
		Title:      filename,
		Summary:    summary,
		RoutePath:  "/detection/rules",
		Snapshot: mustMarshalJSON(map[string]interface{}{
			"filename":      filename,
			"purpose":       AssistantUploadPurposeSigmaRule,
			"size":          len(content),
			"md5":           fileMD5,
			"sha256":        fileSHA256,
			"parsed_count":  result.ParsedCount,
			"failed_count":  result.FailedCount,
			"skipped_count": result.SkippedCount,
			"uploaded_by":   operator,
		}),
	}, nil
}

func normalizeUploadPurpose(purpose string) string {
	switch strings.TrimSpace(purpose) {
	case AssistantUploadPurposeBaselineTemplate:
		return AssistantUploadPurposeBaselineTemplate
	case AssistantUploadPurposeSigmaRule:
		return AssistantUploadPurposeSigmaRule
	default:
		return AssistantUploadPurposeAnalysis
	}
}

func truncateRunes(input string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(input))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "\n..."
}

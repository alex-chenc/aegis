package handler

import (
	"net/http"
	"path/filepath"

	"baseline-system/internal/fileparser"
	"baseline-system/internal/model"
	"baseline-system/internal/repository"
	"baseline-system/internal/service"
	"baseline-system/internal/storage"
	"baseline-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TemplateHandler struct {
	templateRepo    *repository.TemplateRepository
	ruleRepo        *repository.RuleRepository
	minioClient     *storage.MinIOClient
	redisClient     *storage.RedisClient
	templateService *service.TemplateService
}

func NewTemplateHandler(
	templateRepo *repository.TemplateRepository,
	ruleRepo *repository.RuleRepository,
	minioClient *storage.MinIOClient,
	redisClient *storage.RedisClient,
	templateService *service.TemplateService,
) *TemplateHandler {
	return &TemplateHandler{
		templateRepo:    templateRepo,
		ruleRepo:        ruleRepo,
		minioClient:     minioClient,
		redisClient:     redisClient,
		templateService: templateService,
	}
}

func (h *TemplateHandler) UploadTemplate(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "missing file"})
		return
	}

	if file.Size > 50*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "file size exceeds 50MB limit"})
		return
	}

	ext := filepath.Ext(file.Filename)
	fileType := ext[1:]
	if fileType == "docx" {
		fileType = "word"
	}

	templateID := uuid.New()
	objectName := templateID.String() + "/" + file.Filename

	src, err := file.Open()
	if err != nil {
		logger.Error("failed to open uploaded file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to process file"})
		return
	}
	defer src.Close()

	_, err = h.minioClient.UploadFile("baseline-templates", objectName, src, file.Size, "application/octet-stream")
	if err != nil {
		logger.Error("failed to upload file to MinIO", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to store file"})
		return
	}

	template := &model.Template{
		ID:              templateID,
		Name:            file.Filename,
		FileType:        fileType,
		MinioObjectName: objectName,
		Status:          "parsing",
		RuleCount:       0,
	}

	err = h.templateRepo.Create(template)
	if err != nil {
		logger.Error("failed to create template record", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to create template"})
		return
	}

	err = h.redisClient.SetParseStatus(templateID.String(), "parsing", 0, "文件已上传，等待解析...")
	if err != nil {
		logger.Warn("failed to set parse status in redis", zap.Error(err))
	}

	err = h.templateService.QueueTemplate(templateID, fileparser.FileType(fileType), objectName)
	if err != nil {
		logger.Error("failed to queue template for parsing", zap.Error(err))
		errMsg := "加入解析队列失败"
		h.templateRepo.UpdateStatus(templateID, "failed", &errMsg, 0)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to queue template"})
		return
	}

	logger.Info("template uploaded successfully",
		zap.String("template_id", templateID.String()),
		zap.String("filename", file.Filename),
		zap.Int64("size", file.Size),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "template uploaded",
		"data": gin.H{
			"id":          templateID.String(),
			"filename":    file.Filename,
			"size":        file.Size,
			"template_id": templateID.String(),
		},
	})
}

func (h *TemplateHandler) ListTemplates(c *gin.Context) {
	templates, err := h.templateRepo.FindAll(1, 100)
	if err != nil {
		logger.Error("failed to list templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to list templates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": templates})
}

func (h *TemplateHandler) GetTemplateStatus(c *gin.Context) {
	id := c.Param("id")
	templateID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	status, progress, message, err := h.redisClient.GetParseStatus(templateID.String())
	if err != nil {
		template, dbErr := h.templateRepo.FindByID(templateID)
		if dbErr != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "template not found"})
			return
		}
		status = template.Status
		progress = 100
		if template.Status == "completed" {
			message = "解析完成"
		} else if template.Status == "failed" && template.ErrorMessage != nil {
			message = *template.ErrorMessage
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"template_id": templateID.String(),
			"status":      status,
			"progress":    progress,
			"message":     message,
		},
	})
}

func (h *TemplateHandler) GetTemplateRules(c *gin.Context) {
	id := c.Param("id")
	templateID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	rules, err := h.ruleRepo.FindByTemplateID(templateID)
	if err != nil {
		logger.Error("failed to get template rules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to get rules"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": rules})
}

func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	id := c.Param("id")
	templateID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	err = h.templateRepo.Delete(templateID)
	if err != nil {
		logger.Error("failed to delete template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to delete template"})
		return
	}

	logger.Info("template deleted", zap.String("template_id", templateID.String()))
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "template deleted"})
}

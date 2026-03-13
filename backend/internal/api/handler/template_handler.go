package handler

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"

	"aegis-system/internal/repository"
	"aegis-system/internal/service"
	"aegis-system/internal/storage"
	"aegis-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TemplateHandler struct {
	templateRepo     *repository.TemplateRepository
	ruleRepo         *repository.RuleRepository
	minioClient      *storage.MinIOClient
	redisClient      *storage.RedisClient
	templateService  *service.TemplateService
	scriptGenService *service.ScriptGenerationService
}

func NewTemplateHandler(
	templateRepo *repository.TemplateRepository,
	ruleRepo *repository.RuleRepository,
	minioClient *storage.MinIOClient,
	redisClient *storage.RedisClient,
	templateService *service.TemplateService,
	scriptGenService *service.ScriptGenerationService,
) *TemplateHandler {
	return &TemplateHandler{
		templateRepo:     templateRepo,
		ruleRepo:         ruleRepo,
		minioClient:      minioClient,
		redisClient:      redisClient,
		templateService:  templateService,
		scriptGenService: scriptGenService,
	}
}

func (h *TemplateHandler) UploadTemplate(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "missing file"})
		return
	}

	fileMD5 := c.PostForm("md5")

	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "文件大小超过 5MB，无法解析"})
		return
	}

	if fileMD5 != "" {
		exists, existingTemplate, err := h.templateRepo.ExistsByMD5(fileMD5)
		if err != nil {
			logger.Error("failed to check md5", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "MD5 校验失败"})
			return
		}
		if exists && existingTemplate != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    0,
				"message": "文件已解析过",
				"data": gin.H{
					"exists":      true,
					"template_id": existingTemplate.ID.String(),
					"filename":    existingTemplate.DisplayName,
				},
			})
			return
		}
	}

	src, err := file.Open()
	if err != nil {
		logger.Error("failed to open uploaded file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to process file"})
		return
	}
	defer src.Close()

	if fileMD5 == "" {
		hash := md5.New()
		if _, err := io.Copy(hash, src); err != nil {
			logger.Error("failed to calculate md5", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "MD5 计算失败"})
			return
		}
		fileMD5 = hex.EncodeToString(hash.Sum(nil))
		src.Seek(0, 0)
	}

	template, err := h.templateService.UploadTemplate(c.Request.Context(), file.Filename, src, file.Size, fileMD5)
	if err != nil {
		logger.Error("failed to upload template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	logger.Info("template uploaded successfully",
		zap.String("template_id", template.ID.String()),
		zap.String("filename", file.Filename),
		zap.Int64("size", file.Size),
	)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "template uploaded",
		"data": gin.H{
			"id":          template.ID.String(),
			"filename":    template.DisplayName,
			"size":        file.Size,
			"template_id": template.ID.String(),
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

	template, err := h.templateRepo.FindByID(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "template not found"})
		return
	}

	if template.MinioObjectName != "" {
		if err := h.minioClient.DeleteFile("aegis-templates", template.MinioObjectName); err != nil {
			logger.Warn("failed to delete file from minio", zap.Error(err), zap.String("object", template.MinioObjectName))
		}
	}

	if err := h.templateRepo.DeleteWithRules(templateID); err != nil {
		logger.Error("failed to delete template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to delete template"})
		return
	}

	h.redisClient.DeleteParseStatus(templateID.String())

	logger.Info("template deleted", zap.String("template_id", templateID.String()))
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "template deleted"})
}

func (h *TemplateHandler) CheckMD5(c *gin.Context) {
	md5 := c.Query("md5")
	if md5 == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "md5 is required"})
		return
	}

	exists, template, err := h.templateRepo.ExistsByMD5(md5)
	if err != nil {
		logger.Error("failed to check md5", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to check md5"})
		return
	}

	response := gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"exists": exists,
		},
	}

	if exists && template != nil {
		response["data"] = gin.H{
			"exists":      true,
			"template_id": template.ID.String(),
			"filename":    template.DisplayName,
		}
	}

	c.JSON(http.StatusOK, response)
}

type BatchGenerateRequest struct {
	ScriptType string `json:"script_type" binding:"required,oneof=CHECK FIX"`
}

func (h *TemplateHandler) BatchGenerateScripts(c *gin.Context) {
	id := c.Param("id")
	templateID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	var req BatchGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}

	template, err := h.templateRepo.FindByID(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "template not found"})
		return
	}

	if template.Status != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "模板未完成解析"})
		return
	}

	result, err := h.scriptGenService.BatchGenerateForTemplate(c.Request.Context(), templateID, req.ScriptType, 2)
	if err != nil {
		logger.Error("failed to batch generate scripts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "批量生成失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

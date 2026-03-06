package handler

import (
	"net/http"

	"baseline-system/internal/repository"
	"baseline-system/internal/storage"
	"baseline-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TemplateHandler 模板 Handler
type TemplateHandler struct {
	templateRepo *repository.TemplateRepository
	ruleRepo     *repository.RuleRepository
	minioClient  *storage.MinIOClient
}

// NewTemplateHandler 创建模板 Handler
func NewTemplateHandler(
	templateRepo *repository.TemplateRepository,
	ruleRepo *repository.RuleRepository,
	minioClient *storage.MinIOClient,
) *TemplateHandler {
	return &TemplateHandler{
		templateRepo: templateRepo,
		ruleRepo:     ruleRepo,
		minioClient:  minioClient,
	}
}

// UploadTemplate 上传模板
func (h *TemplateHandler) UploadTemplate(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "missing file",
		})
		return
	}

	// TODO: 实现文件上传逻辑
	// 1. 保存文件到 MinIO
	// 2. 创建数据库记录
	// 3. 加入解析队列

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "template uploaded",
		"data": gin.H{
			"filename":    file.Filename,
			"size":        file.Size,
			"template_id": uuid.New().String(),
		},
	})
}

// ListTemplates 获取模板列表
func (h *TemplateHandler) ListTemplates(c *gin.Context) {

	// TODO: 查询模板列表
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    []interface{}{},
	})
}

// GetTemplateStatus 获取模板解析状态
func (h *TemplateHandler) GetTemplateStatus(c *gin.Context) {
	id := c.Param("id")
	templateID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid template id",
		})
		return
	}

	// TODO: 查询 Redis 状态
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"template_id": templateID.String(),
			"status":      "completed",
			"progress":    100,
			"message":     "解析完成",
		},
	})
}

// GetTemplateRules 获取模板规则
func (h *TemplateHandler) GetTemplateRules(c *gin.Context) {
	id := c.Param("id")
	templateID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid template id",
		})
		return
	}

	rules, err := h.ruleRepo.FindByTemplateID(templateID)
	if err != nil {
		logger.Error("failed to get template rules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to get rules",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    rules,
	})
}

// DeleteTemplate 删除模板
func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	id := c.Param("id")
	templateID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid template id",
		})
		return
	}

	// TODO: 删除模板和关联规则
	logger.Info("template deleted", zap.String("template_id", templateID.String()))

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "template deleted",
	})
}

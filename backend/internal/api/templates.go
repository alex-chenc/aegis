package api

import (
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ai-benchmark/backend/internal/service"
)

type TemplateHandler struct {
	templateService *service.TemplateService
}

func NewTemplateHandler(templateService *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{
		templateService: templateService,
	}
}

type CreateTemplateRequest struct {
	Name            string `json:"name" binding:"required"`
	FileType        string `json:"file_type" binding:"required"`
	MinioObjectName string `json:"minio_object_name" binding:"required"`
}

func (h *TemplateHandler) GetTemplates(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 10)

	templates, total, err := h.templateService.ListTemplates(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"items": templates,
			"total": total,
		},
	})
}

func (h *TemplateHandler) GetTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	template, err := h.templateService.GetTemplate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "template not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": template})
}

func (h *TemplateHandler) CreateTemplate(c *gin.Context) {
	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	template, err := h.templateService.CreateTemplate(req.Name, req.FileType, req.MinioObjectName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": template})
}

func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	if err := h.templateService.DeleteTemplate(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "template deleted"})
}

type ParseTemplateRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *TemplateHandler) ParseTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	var req ParseTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	if h.templateService.IsParsing(id) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "template is already being parsed"})
		return
	}

	h.templateService.ParseTemplateAsync(id, req.Content, func(parseErr error) {
		if parseErr != nil {
			log.Printf("[API] Failed to parse template %s: %v", id, parseErr)
		} else {
			log.Printf("[API] Successfully parsed template %s", id)
		}
	})

	c.JSON(http.StatusAccepted, gin.H{
		"code":    202,
		"message": "template parsing started",
		"data": gin.H{
			"template_id": id,
			"status":      "parsing",
		},
	})
}

func (h *TemplateHandler) GetRules(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid template id"})
		return
	}

	rules, err := h.templateService.GetRules(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"items": rules,
			"total": len(rules),
		},
	})
}

func (h *TemplateHandler) UploadTemplate(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no file uploaded"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to read file"})
		return
	}

	template, err := h.templateService.CreateTemplate(
		header.Filename,
		getFileType(header.Filename),
		header.Filename,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	h.templateService.ParseTemplateAsync(template.ID, string(content), func(parseErr error) {
		if parseErr != nil {
			log.Printf("[API] Failed to parse uploaded template %s: %v", template.ID, parseErr)
		}
	})

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"template_id": template.ID,
			"status":      "parsing",
		},
	})
}

func getFileType(filename string) string {
	if len(filename) > 4 {
		ext := filename[len(filename)-4:]
		switch ext {
		case ".txt":
			return "txt"
		case ".pdf":
			return "pdf"
		case ".doc":
			return "doc"
		case "docx":
			return "docx"
		}
	}
	return "unknown"
}

func parseIntQuery(c *gin.Context, key string, defaultValue int) int {
	val := c.Query(key)
	if val == "" {
		return defaultValue
	}
	var result int
	if _, err := uuid.Parse(val); err == nil {
		return defaultValue
	}
	for _, ch := range val {
		if ch < '0' || ch > '9' {
			return defaultValue
		}
		result = result*10 + int(ch-'0')
	}
	if result <= 0 {
		return defaultValue
	}
	return result
}

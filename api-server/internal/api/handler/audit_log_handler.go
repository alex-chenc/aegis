package handler

import (
	"net/http"
	"strconv"

	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AuditLogHandler struct {
	auditLogRepo *repository.AuditLogRepo
}

func NewAuditLogHandler(auditLogRepo *repository.AuditLogRepo) *AuditLogHandler {
	return &AuditLogHandler{auditLogRepo: auditLogRepo}
}

// ListLogs GET /api/v1/settings/audit-logs
func (h *AuditLogHandler) ListLogs(c *gin.Context) {
	scriptType := c.Query("script_type")
	auditSource := c.Query("audit_source")
	passed := c.Query("passed")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	logs, total, err := h.auditLogRepo.List(scriptType, auditSource, passed, page, pageSize)
	if err != nil {
		logger.Error("failed to list audit logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to list audit logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items":     logs,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetLog GET /api/v1/settings/audit-logs/:id
func (h *AuditLogHandler) GetLog(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid log id"})
		return
	}

	log, err := h.auditLogRepo.FindByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "audit log not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": log})
}

// GetStats GET /api/v1/settings/audit-logs/stats
func (h *AuditLogHandler) GetStats(c *gin.Context) {
	stats, err := h.auditLogRepo.GetStats()
	if err != nil {
		logger.Error("failed to get audit stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to get stats"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": stats})
}

package handler

import (
	"net/http"
	"strconv"

	"api-server/internal/model"
	"api-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AssetHandler 资产处理器
type AssetHandler struct {
	collectionService *service.AssetCollectionService
	queryService      *service.AssetQueryService
	analysisService   *service.AssetAnalysisService
	logger            *zap.Logger
}

// NewAssetHandler 创建资产处理器
func NewAssetHandler(
	collectionService *service.AssetCollectionService,
	queryService *service.AssetQueryService,
	analysisService *service.AssetAnalysisService,
	logger *zap.Logger,
) *AssetHandler {
	return &AssetHandler{
		collectionService: collectionService,
		queryService:      queryService,
		analysisService:   analysisService,
		logger:            logger,
	}
}

// RegisterRoutes 注册路由
func (h *AssetHandler) RegisterRoutes(api *gin.RouterGroup) {
	assets := api.Group("/host-assets")
	{
		// 概览
		assets.GET("/summary", h.GetSummary)

		// 软件资产
		assets.GET("/software", h.ListSoftware)

		// 应用资产
		assets.GET("/applications", h.ListApplications)
		assets.GET("/applications/:id", h.GetApplication)
		assets.PUT("/applications/:id/review", h.ReviewApplication)

		// 采集任务
		assets.POST("/collections", h.TriggerCollection)
		assets.GET("/collections", h.ListCollections)
		assets.GET("/collections/:id", h.GetCollection)
		assets.POST("/collections/:id/retry", h.RetryCollection)
		assets.POST("/collections/:id/cancel", h.CancelCollection)

		// 周期配置
		assets.GET("/collection-config", h.GetCollectionConfig)
		assets.PUT("/collection-config", h.UpdateCollectionConfig)
	}
}

// GetSummary 获取资产概览
func (h *AssetHandler) GetSummary(c *gin.Context) {
	summary, err := h.queryService.GetSummary()
	if err != nil {
		h.logger.Error("Failed to get summary", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get summary",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    summary,
	})
}

// ListSoftware 列出软件资产
func (h *AssetHandler) ListSoftware(c *gin.Context) {
	var query model.SoftwareAssetQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid query parameters",
		})
		return
	}

	// 设置默认分页
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}

	items, total, err := h.queryService.ListSoftwareAssets(query)
	if err != nil {
		h.logger.Error("Failed to list software assets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to list software assets",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": items,
			"total": total,
		},
	})
}

// ListApplications 列出应用资产
func (h *AssetHandler) ListApplications(c *gin.Context) {
	var query model.ApplicationAssetQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid query parameters",
		})
		return
	}

	// 设置默认分页
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}

	items, total, err := h.queryService.ListApplicationAssets(query)
	if err != nil {
		h.logger.Error("Failed to list application assets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to list application assets",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": items,
			"total": total,
		},
	})
}

// GetApplication 获取应用详情
func (h *AssetHandler) GetApplication(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid application ID",
		})
		return
	}

	app, toolCalls, err := h.queryService.GetApplicationDetail(id)
	if err != nil {
		h.logger.Error("Failed to get application", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get application",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"application": app,
			"tool_calls":  toolCalls,
		},
	})
}

// ReviewApplication 人工复核
func (h *AssetHandler) ReviewApplication(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid application ID",
		})
		return
	}

	var payload model.ApplicationReviewPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body",
		})
		return
	}

	if err := h.queryService.ReviewApplication(id, payload); err != nil {
		h.logger.Error("Failed to review application", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to review application",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

// TriggerCollection 触发采集
func (h *AssetHandler) TriggerCollection(c *gin.Context) {
	var req model.TriggerAssetCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body",
		})
		return
	}

	// 获取当前用户
	requestedBy := c.GetString("username")
	if requestedBy == "" {
		requestedBy = "system"
	}

	task, err := h.collectionService.TriggerAssetCollection(c.Request.Context(), req, requestedBy)
	if err != nil {
		h.logger.Error("Failed to trigger collection", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to trigger collection",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task_id": task.ID.String(),
			"status":  task.Status,
		},
	})
}

// ListCollections 列出采集任务
func (h *AssetHandler) ListCollections(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	repo := h.collectionService.GetRepo()
	tasks, total, err := repo.ListTasks(page, pageSize, status)
	if err != nil {
		h.logger.Error("Failed to list tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to list tasks",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": tasks,
			"total": total,
		},
	})
}

// GetCollection 获取采集任务详情
func (h *AssetHandler) GetCollection(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid task ID",
		})
		return
	}

	repo := h.collectionService.GetRepo()
	task, err := repo.GetTask(id)
	if err != nil {
		h.logger.Error("Failed to get task", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get task",
		})
		return
	}

	hosts, _ := repo.GetTaskHosts(id)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task":  task,
			"hosts": hosts,
		},
	})
}

// RetryCollection 重试采集
func (h *AssetHandler) RetryCollection(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid task ID",
		})
		return
	}

	if err := h.collectionService.RetryFailed(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to retry collection", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

// CancelCollection 取消采集
func (h *AssetHandler) CancelCollection(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid task ID",
		})
		return
	}

	if err := h.collectionService.Cancel(c.Request.Context(), id); err != nil {
		h.logger.Error("Failed to cancel collection", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

// GetCollectionConfig 获取采集配置
func (h *AssetHandler) GetCollectionConfig(c *gin.Context) {
	config, err := h.collectionService.GetConfig()
	if err != nil {
		h.logger.Error("Failed to get config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get config",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    config,
	})
}

// UpdateCollectionConfig 更新采集配置
func (h *AssetHandler) UpdateCollectionConfig(c *gin.Context) {
	var config model.AssetCollectionConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body",
		})
		return
	}

	if err := h.collectionService.UpdateConfig(&config); err != nil {
		h.logger.Error("Failed to update config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    config,
	})
}

package handler

import (
	"fmt"
	"net/http"

	"api-server/internal/repository"
	"api-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// NotificationHandler 通知处理器
type NotificationHandler struct {
	svc *service.NotificationService
}

// NewNotificationHandler 构造函数
func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// GetNotifications 获取通知列表
func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	// 解析分页和过滤参数
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if ps := c.Query("pageSize"); ps != "" {
		fmt.Sscanf(ps, "%d", &pageSize)
	}
	if pageSize > 100 {
		pageSize = 100
	}

	filter := repository.ListFilter{
		Page:     page,
		PageSize: pageSize,
	}

	// 解析 is_read 过滤
	if isRead := c.Query("is_read"); isRead != "" {
		val := isRead == "true"
		filter.IsRead = &val
	}

	// 解析 type 过滤
	if typ := c.Query("type"); typ != "" {
		filter.Type = typ
	}

	result, err := h.svc.List(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    1,
			"message": "failed to get notifications",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

// MarkAsRead 将单条通知标为已读
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	idStr := c.Param("id")

	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "invalid notification id",
		})
		return
	}

	if err := h.svc.MarkRead(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    1,
			"message": "notification not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

// MarkAllAsRead 将所有通知标为已读
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	count, err := h.svc.MarkAllRead()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    1,
			"message": "failed to mark all as read",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"updated_count": count,
		},
	})
}
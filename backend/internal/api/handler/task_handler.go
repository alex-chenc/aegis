package handler

import (
	"baseline-system/internal/grpc_server"
	"baseline-system/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TaskHandler 任务 Handler
type TaskHandler struct {
	taskService *service.TaskService
	grpcServer  *grpc_server.GRPCServer
}

// NewTaskHandler 创建任务 Handler
func NewTaskHandler(taskService *service.TaskService, grpcServer *grpc_server.GRPCServer) *TaskHandler {
	return &TaskHandler{
		taskService: taskService,
		grpcServer:  grpcServer,
	}
}

// RunCheck 执行检查任务
func (h *TaskHandler) RunCheck(c *gin.Context) {
	var req struct {
		RuleIDs []string `json:"rule_ids"`
		HostIDs []string `json:"host_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body",
		})
		return
	}

	// TODO: 实现任务下发逻辑
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "check task queued",
		"data": gin.H{
			"task_group_id": "mock-task-group-id",
			"task_count":    len(req.RuleIDs) * len(req.HostIDs),
		},
	})
}

// RunFix 执行修复任务
func (h *TaskHandler) RunFix(c *gin.Context) {
	var req struct {
		RuleIDs []string `json:"rule_ids"`
		HostIDs []string `json:"host_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body",
		})
		return
	}

	// TODO: 实现任务下发逻辑
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "fix task queued",
		"data": gin.H{
			"task_group_id": "mock-task-group-id",
			"task_count":    len(req.RuleIDs) * len(req.HostIDs),
		},
	})
}

// GetTaskStatus 获取任务状态
func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	taskGroupID := c.Param("id")

	// TODO: 查询任务状态
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task_group_id": taskGroupID,
			"status":        "running",
			"total":         10,
			"success":       5,
			"failed":        2,
			"pending":       3,
		},
	})
}

// GetTaskLogs 获取任务日志
func (h *TaskHandler) GetTaskLogs(c *gin.Context) {
	taskID := c.Param("id")
	offset := c.DefaultQuery("offset", "0")

	// TODO: 查询任务日志
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task_id": taskID,
			"offset":  offset,
			"logs":    []string{},
		},
	})
}

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ai-benchmark/backend/internal/service"
)

type TaskHandler struct {
	taskService *service.TaskService
}

func NewTaskHandler(taskService *service.TaskService) *TaskHandler {
	return &TaskHandler{
		taskService: taskService,
	}
}

type ExecuteCheckRequest struct {
	RuleID  string   `json:"rule_id" binding:"required"`
	HostIDs []string `json:"host_ids" binding:"required"`
}

type ExecuteFixRequest struct {
	RuleID  string   `json:"rule_id" binding:"required"`
	HostIDs []string `json:"host_ids" binding:"required"`
}

func (h *TaskHandler) GetTasks(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 10)

	var hostID, ruleID *uuid.UUID

	if hostIDStr := c.Query("host_id"); hostIDStr != "" {
		if id, err := uuid.Parse(hostIDStr); err == nil {
			hostID = &id
		}
	}

	if ruleIDStr := c.Query("rule_id"); ruleIDStr != "" {
		if id, err := uuid.Parse(ruleIDStr); err == nil {
			ruleID = &id
		}
	}

	tasks, total, err := h.taskService.ListTasks(page, pageSize, hostID, ruleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"items": tasks,
			"total": total,
		},
	})
}

func (h *TaskHandler) GetTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid task id"})
		return
	}

	task, err := h.taskService.GetTask(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "task not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": task})
}

func (h *TaskHandler) ExecuteCheck(c *gin.Context) {
	var req ExecuteCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	ruleID, err := uuid.Parse(req.RuleID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	var tasks []gin.H
	var errors []string

	for _, hostIDStr := range req.HostIDs {
		hostID, err := uuid.Parse(hostIDStr)
		if err != nil {
			errors = append(errors, "invalid host id: "+hostIDStr)
			continue
		}

		task, err := h.taskService.ExecuteCheck(ruleID, hostID)
		if err != nil {
			errors = append(errors, "failed to execute on host "+hostIDStr+": "+err.Error())
			continue
		}

		tasks = append(tasks, gin.H{
			"task_id": task.ID,
			"host_id": task.HostID,
			"status":  task.Status,
		})
	}

	c.JSON(http.StatusAccepted, gin.H{
		"code": 202,
		"data": gin.H{
			"tasks":  tasks,
			"errors": errors,
		},
	})
}

func (h *TaskHandler) ExecuteFix(c *gin.Context) {
	var req ExecuteFixRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	ruleID, err := uuid.Parse(req.RuleID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	var tasks []gin.H
	var errors []string

	for _, hostIDStr := range req.HostIDs {
		hostID, err := uuid.Parse(hostIDStr)
		if err != nil {
			errors = append(errors, "invalid host id: "+hostIDStr)
			continue
		}

		task, err := h.taskService.ExecuteFix(ruleID, hostID)
		if err != nil {
			errors = append(errors, "failed to execute on host "+hostIDStr+": "+err.Error())
			continue
		}

		tasks = append(tasks, gin.H{
			"task_id": task.ID,
			"host_id": task.HostID,
			"status":  task.Status,
		})
	}

	c.JSON(http.StatusAccepted, gin.H{
		"code": 202,
		"data": gin.H{
			"tasks":  tasks,
			"errors": errors,
		},
	})
}

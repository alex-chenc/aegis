package handler

import (
	"baseline-system/internal/grpc_server"
	"baseline-system/internal/repository"
	"baseline-system/internal/service"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TaskHandler struct {
	taskService      *service.TaskService
	taskLogRepo      *repository.TaskLogRepository
	scriptGenService *service.ScriptGenerationService
	grpcServer       *grpc_server.GRPCServer
}

func NewTaskHandler(
	taskService *service.TaskService,
	taskLogRepo *repository.TaskLogRepository,
	scriptGenService *service.ScriptGenerationService,
	grpcServer *grpc_server.GRPCServer,
) *TaskHandler {
	return &TaskHandler{
		taskService:      taskService,
		taskLogRepo:      taskLogRepo,
		scriptGenService: scriptGenService,
		grpcServer:       grpcServer,
	}
}

type RunCheckRequest struct {
	RuleIDs []string `json:"rule_ids"`
	HostIDs []string `json:"host_ids"`
}

type RunFixRequest struct {
	RuleIDs []string `json:"rule_ids"`
	HostIDs []string `json:"host_ids"`
}

type TaskResponse struct {
	TaskGroupID string   `json:"task_group_id"`
	TaskIDs     []string `json:"task_ids"`
	TaskCount   int      `json:"task_count"`
}

type TaskGroupStatus struct {
	TaskGroupID string `json:"task_group_id"`
	Status      string `json:"status"`
	Total       int    `json:"total"`
	Pending     int    `json:"pending"`
	Running     int    `json:"running"`
	Success     int    `json:"success"`
	Failed      int    `json:"failed"`
}

type TaskLogResponse struct {
	ID            string  `json:"id"`
	TaskGroupID   string  `json:"task_group_id"`
	RuleID        string  `json:"rule_id"`
	HostID        string  `json:"host_id"`
	RuleTitle     string  `json:"rule_title"`
	Hostname      string  `json:"hostname"`
	TaskType      string  `json:"task_type"`
	Status        string  `json:"status"`
	ScriptContent *string `json:"script_content"`
	Stdout        *string `json:"stdout"`
	Stderr        *string `json:"stderr"`
	ExitCode      *int    `json:"exit_code"`
	StartedAt     *string `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
}

func (h *TaskHandler) RunCheck(c *gin.Context) {
	var req RunCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body: " + err.Error(),
		})
		return
	}

	if len(req.RuleIDs) == 0 || len(req.HostIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "rule_ids and host_ids are required",
		})
		return
	}

	result, err := h.taskService.CreateAndDispatchTasks(c.Request.Context(), req.RuleIDs, req.HostIDs, "check")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "check tasks created",
		"data": TaskResponse{
			TaskGroupID: result.TaskGroupID.String(),
			TaskIDs:     result.TaskIDs,
			TaskCount:   len(result.TaskIDs),
		},
	})
}

func (h *TaskHandler) RunFix(c *gin.Context) {
	var req RunFixRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body: " + err.Error(),
		})
		return
	}

	if len(req.RuleIDs) == 0 || len(req.HostIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "rule_ids and host_ids are required",
		})
		return
	}

	result, err := h.taskService.CreateAndDispatchTasks(c.Request.Context(), req.RuleIDs, req.HostIDs, "fix")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "fix tasks created",
		"data": TaskResponse{
			TaskGroupID: result.TaskGroupID.String(),
			TaskIDs:     result.TaskIDs,
			TaskCount:   len(result.TaskIDs),
		},
	})
}

func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	taskGroupIDStr := c.Param("id")
	taskGroupID, err := uuid.Parse(taskGroupIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid task_group_id",
		})
		return
	}

	logs, err := h.taskLogRepo.FindByGroupID(taskGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to query tasks",
		})
		return
	}

	status := TaskGroupStatus{
		TaskGroupID: taskGroupIDStr,
		Status:      "pending",
		Total:       len(logs),
	}

	for _, log := range logs {
		switch log.Status {
		case "pending":
			status.Pending++
		case "running":
			status.Running++
		case "success":
			status.Success++
		case "failed":
			status.Failed++
		}
	}

	if status.Running > 0 {
		status.Status = "running"
	} else if status.Pending > 0 {
		status.Status = "pending"
	} else if status.Failed > 0 {
		status.Status = "partial_success"
		if status.Success == 0 {
			status.Status = "failed"
		}
	} else {
		status.Status = "success"
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    status,
	})
}

func (h *TaskHandler) GetTaskLogs(c *gin.Context) {
	taskGroupIDStr := c.Param("id")
	taskGroupID, err := uuid.Parse(taskGroupIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid task_group_id",
		})
		return
	}

	logs, err := h.taskLogRepo.FindByGroupID(taskGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to query task logs",
		})
		return
	}

	responses := make([]TaskLogResponse, len(logs))
	for i, log := range logs {
		responses[i] = TaskLogResponse{
			ID:            log.ID.String(),
			TaskGroupID:   log.TaskGroupID.String(),
			RuleID:        log.RuleID.String(),
			HostID:        log.HostID.String(),
			TaskType:      log.TaskType,
			Status:        log.Status,
			ScriptContent: log.ScriptContent,
			Stdout:        log.Stdout,
			Stderr:        log.Stderr,
			ExitCode:      log.ExitCode,
		}

		if log.StartedAt != nil {
			t := log.StartedAt.Format(time.RFC3339)
			responses[i].StartedAt = &t
		}
		if log.FinishedAt != nil {
			t := log.FinishedAt.Format(time.RFC3339)
			responses[i].FinishedAt = &t
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    responses,
	})
}

func (h *TaskHandler) GetTaskDetail(c *gin.Context) {
	taskIDStr := c.Param("id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid task_id",
		})
		return
	}

	log, err := h.taskLogRepo.FindByID(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "task not found",
		})
		return
	}

	response := TaskLogResponse{
		ID:            log.ID.String(),
		TaskGroupID:   log.TaskGroupID.String(),
		RuleID:        log.RuleID.String(),
		HostID:        log.HostID.String(),
		TaskType:      log.TaskType,
		Status:        log.Status,
		ScriptContent: log.ScriptContent,
		Stdout:        log.Stdout,
		Stderr:        log.Stderr,
		ExitCode:      log.ExitCode,
	}

	if log.StartedAt != nil {
		t := log.StartedAt.Format(time.RFC3339)
		response.StartedAt = &t
	}
	if log.FinishedAt != nil {
		t := log.FinishedAt.Format(time.RFC3339)
		response.FinishedAt = &t
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    response,
	})
}

type ListTasksRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Status   string `form:"status"`
	TaskType string `form:"task_type"`
	Search   string `form:"search"`
}

type TaskGroupResponse struct {
	TaskGroupID  string  `json:"task_group_id"`
	TaskCount    int     `json:"task_count"`
	TaskType     string  `json:"task_type"`
	Status       string  `json:"status"`
	SuccessCount int     `json:"success_count"`
	FailedCount  int     `json:"failed_count"`
	PendingCount int     `json:"pending_count"`
	RunningCount int     `json:"running_count"`
	CreatedAt    string  `json:"created_at"`
	FinishedAt   *string `json:"finished_at"`
}

type ListTasksResponse struct {
	Items      []TaskGroupResponse `json:"items"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalPages int                 `json:"total_pages"`
}

func (h *TaskHandler) ListTasks(c *gin.Context) {
	var req ListTasksRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid query parameters: " + err.Error(),
		})
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	params := repository.ListTaskGroupsParams{
		Page:     req.Page,
		PageSize: req.PageSize,
		Status:   req.Status,
		TaskType: req.TaskType,
		Search:   req.Search,
	}

	summaries, err := h.taskLogRepo.ListTaskGroups(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to list tasks",
		})
		return
	}

	total, err := h.taskLogRepo.CountTaskGroups(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to count tasks",
		})
		return
	}

	items := make([]TaskGroupResponse, len(summaries))
	for i, s := range summaries {
		items[i] = TaskGroupResponse{
			TaskGroupID:  s.TaskGroupID.String(),
			TaskCount:    s.TaskCount,
			TaskType:     s.TaskType,
			Status:       s.Status,
			SuccessCount: s.SuccessCount,
			FailedCount:  s.FailedCount,
			PendingCount: s.PendingCount,
			RunningCount: s.RunningCount,
			CreatedAt:    s.CreatedAt.Format(time.RFC3339),
		}
		if s.FinishedAt != nil {
			t := s.FinishedAt.Format(time.RFC3339)
			items[i].FinishedAt = &t
		}
	}

	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": ListTasksResponse{
			Items:      items,
			Total:      total,
			Page:       req.Page,
			PageSize:   req.PageSize,
			TotalPages: totalPages,
		},
	})
}

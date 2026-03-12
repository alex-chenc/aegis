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
	healingLogRepo   *repository.HealingLogRepository
	scriptGenService *service.ScriptGenerationService
	grpcServer       *grpc_server.GRPCServer
	ruleRepo         *repository.RuleRepository
}

func NewTaskHandler(
	taskService *service.TaskService,
	taskLogRepo *repository.TaskLogRepository,
	healingLogRepo *repository.HealingLogRepository,
	scriptGenService *service.ScriptGenerationService,
	grpcServer *grpc_server.GRPCServer,
	ruleRepo *repository.RuleRepository,
) *TaskHandler {
	return &TaskHandler{
		taskService:      taskService,
		taskLogRepo:      taskLogRepo,
		healingLogRepo:   healingLogRepo,
		scriptGenService: scriptGenService,
		grpcServer:       grpcServer,
		ruleRepo:         ruleRepo,
	}
}

type RunCheckRequest struct {
	RuleIDs []string `json:"rule_ids"`
	HostIDs []string `json:"host_ids"`
}

type RunFixRequest struct {
	RuleIDs     []string `json:"rule_ids"`
	HostIDs     []string `json:"host_ids"`
	TaskGroupID string   `json:"task_group_id"`
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
	Timeout     int    `json:"timeout"`
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

type RedispatchTaskResponse struct {
	TaskID      string `json:"task_id"`
	TaskGroupID string `json:"task_group_id"`
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

	unreadyRules := h.checkScriptsReady(req.RuleIDs, "CHECK")
	if len(unreadyRules) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "检测脚本未生成完成，请等待脚本生成后再下发",
			"data": gin.H{
				"unready_count": len(unreadyRules),
			},
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

	unreadyRules := h.checkScriptsReady(req.RuleIDs, "FIX")
	if len(unreadyRules) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "修复脚本未生成完成，请等待脚本生成后再下发",
			"data": gin.H{
				"unready_count": len(unreadyRules),
			},
		})
		return
	}

	var result *service.TaskCreateResult
	var err error
	if req.TaskGroupID != "" {
		groupID, parseErr := uuid.Parse(req.TaskGroupID)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "invalid task_group_id: " + parseErr.Error(),
			})
			return
		}
		result, err = h.taskService.CreateAndDispatchTasks(c.Request.Context(), req.RuleIDs, req.HostIDs, "fix", groupID)
	} else {
		result, err = h.taskService.CreateAndDispatchTasks(c.Request.Context(), req.RuleIDs, req.HostIDs, "fix")
	}
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

func (h *TaskHandler) checkScriptsReady(ruleIDs []string, scriptType string) []string {
	var unreadyRules []string
	for _, ruleIDStr := range ruleIDs {
		ruleID, err := uuid.Parse(ruleIDStr)
		if err != nil {
			unreadyRules = append(unreadyRules, ruleIDStr)
			continue
		}

		rule, err := h.ruleRepo.FindByID(ruleID)
		if err != nil {
			unreadyRules = append(unreadyRules, ruleIDStr)
			continue
		}

		if scriptType == "CHECK" {
			if rule.CheckScriptStatus != "generated" {
				unreadyRules = append(unreadyRules, ruleIDStr)
			}
		} else {
			if rule.FixScriptStatus != "generated" {
				unreadyRules = append(unreadyRules, ruleIDStr)
			}
		}
	}
	return unreadyRules
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
		case "timeout":
			status.Timeout++
		}
	}

	if status.Running > 0 {
		status.Status = "running"
	} else if status.Pending > 0 {
		status.Status = "pending"
	} else if status.Failed > 0 || status.Timeout > 0 {
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
	ruleTitleCache := make(map[string]string)
	hostnameCache := make(map[string]string)
	for i, log := range logs {
		ruleID := log.RuleID.String()
		hostID := log.HostID.String()

		ruleTitle, ok := ruleTitleCache[ruleID]
		if !ok {
			ruleTitle = ruleID
			rule, findErr := h.ruleRepo.FindByID(log.RuleID)
			if findErr == nil {
				ruleTitle = rule.Title
			}
			ruleTitleCache[ruleID] = ruleTitle
		}

		hostname, ok := hostnameCache[hostID]
		if !ok {
			hostname = hostID
			host, findErr := h.taskService.GetHostByID(log.HostID)
			if findErr == nil {
				hostname = host.Hostname
			}
			hostnameCache[hostID] = hostname
		}

		responses[i] = TaskLogResponse{
			ID:            log.ID.String(),
			TaskGroupID:   log.TaskGroupID.String(),
			RuleID:        log.RuleID.String(),
			HostID:        log.HostID.String(),
			RuleTitle:     ruleTitle,
			Hostname:      hostname,
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

func (h *TaskHandler) RedispatchTask(c *gin.Context) {
	taskIDStr := c.Param("id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid task_id",
		})
		return
	}

	newTask, err := h.taskService.RedispatchTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to redispatch task: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "task redispatched successfully",
		"data": RedispatchTaskResponse{
			TaskID:      newTask.ID.String(),
			TaskGroupID: newTask.TaskGroupID.String(),
		},
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

type TaskHandlerWithHealing struct {
	*TaskHandler
	healingService *service.SelfHealingService
	ruleRepo       *repository.RuleRepository
	taskLogRepo    *repository.TaskLogRepository
}

func NewTaskHandlerWithHealing(
	taskService *service.TaskService,
	taskLogRepo *repository.TaskLogRepository,
	healingLogRepo *repository.HealingLogRepository,
	scriptGenService *service.ScriptGenerationService,
	grpcServer *grpc_server.GRPCServer,
	healingService *service.SelfHealingService,
	ruleRepo *repository.RuleRepository,
) *TaskHandlerWithHealing {
	return &TaskHandlerWithHealing{
		TaskHandler:    NewTaskHandler(taskService, taskLogRepo, healingLogRepo, scriptGenService, grpcServer, ruleRepo),
		healingService: healingService,
		ruleRepo:       ruleRepo,
		taskLogRepo:    taskLogRepo,
	}
}

type TriggerHealingRequest struct {
	UserSuggestion string `json:"user_suggestion"`
}

func (h *TaskHandlerWithHealing) TriggerSelfHealing(c *gin.Context) {
	taskIDStr := c.Param("id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid task_id",
		})
		return
	}

	var req TriggerHealingRequest
	c.ShouldBindJSON(&req)

	taskLog, err := h.taskLogRepo.FindByID(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "task not found",
		})
		return
	}

	if taskLog.Status != "failed" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "task is not in failed state, self-healing is only available for failed tasks",
		})
		return
	}

	scriptType := "CHECK"
	if taskLog.TaskType == "fix" {
		scriptType = "FIX"
	}

	errMsg := ""
	if taskLog.Stderr != nil {
		errMsg = *taskLog.Stderr
	} else if taskLog.Stdout != nil {
		errMsg = *taskLog.Stdout
	}

	exitCode := -1
	if taskLog.ExitCode != nil {
		exitCode = *taskLog.ExitCode
	}

	scriptContent := ""
	if taskLog.ScriptContent != nil {
		scriptContent = *taskLog.ScriptContent
	}

	healingTask := service.HealingTask{
		OriginalTaskID: taskID,
		RuleID:         taskLog.RuleID,
		HostID:         taskLog.HostID,
		ScriptType:     scriptType,
		ScriptContent:  scriptContent,
		ErrorMessage:   errMsg,
		ExitCode:       exitCode,
		UserSuggestion: req.UserSuggestion,
	}

	if err := h.healingService.TriggerHealing(healingTask); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to trigger self-healing: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "self-healing triggered successfully",
		"data": gin.H{
			"task_id":     taskIDStr,
			"rule_id":     taskLog.RuleID.String(),
			"script_type": scriptType,
			"status":      "healing",
		},
	})
}

func (h *TaskHandlerWithHealing) GetHealingStatus(c *gin.Context) {
	taskIDStr := c.Param("id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid task_id",
		})
		return
	}

	healingLog, err := h.healingService.GetHealingLogByTaskID(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to get healing status",
		})
		return
	}

	if healingLog == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "no healing record found",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"id":                   healingLog.ID.String(),
			"original_task_id":     healingLog.OriginalTaskID.String(),
			"rule_id":              healingLog.RuleID.String(),
			"script_type":          healingLog.ScriptType,
			"status":               healingLog.Status,
			"total_attempts":       healingLog.TotalAttempts,
			"max_attempts":         healingLog.MaxAttempts,
			"final_script_version": healingLog.FinalScriptVersionID,
			"last_error":           healingLog.LastError,
			"user_suggestion":      healingLog.UserSuggestion,
		},
	})
}

func (h *TaskHandler) DeleteTask(c *gin.Context) {
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
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "task group not found",
		})
		return
	}

	if len(logs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "task group not found",
		})
		return
	}

	for _, log := range logs {
		if log.Status == "running" || log.Status == "pending" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "cannot delete running or pending tasks",
			})
			return
		}
	}

	taskIDs := make([]uuid.UUID, len(logs))
	for i, log := range logs {
		taskIDs[i] = log.ID
	}

	if h.healingLogRepo != nil {
		if err := h.healingLogRepo.DeleteByOriginalTaskIDs(taskIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "failed to delete healing logs",
			})
			return
		}
	}

	deletedCount, err := h.taskLogRepo.DeleteByGroupID(taskGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to delete task group",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "task group deleted successfully",
		"data": gin.H{
			"deleted_count": deletedCount,
		},
	})
}

func (h *TaskHandler) DeleteSingleTask(c *gin.Context) {
	taskIDStr := c.Param("id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid task_id",
		})
		return
	}

	taskLog, err := h.taskLogRepo.FindByID(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "task not found",
		})
		return
	}

	if taskLog.Status == "running" || taskLog.Status == "pending" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "cannot delete running or pending task",
		})
		return
	}

	if h.healingLogRepo != nil {
		if err := h.healingLogRepo.DeleteByOriginalTaskIDs([]uuid.UUID{taskID}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "failed to delete healing logs",
			})
			return
		}
	}

	if err := h.taskLogRepo.Delete(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to delete task",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "task deleted successfully",
	})
}

type BatchDeleteRequest struct {
	TaskGroupIDs []string `json:"task_ids" binding:"required,min=1"`
}

func (h *TaskHandler) BatchDeleteTasks(c *gin.Context) {
	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request: " + err.Error(),
		})
		return
	}

	groupIDs := make([]uuid.UUID, 0, len(req.TaskGroupIDs))
	for _, idStr := range req.TaskGroupIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		groupIDs = append(groupIDs, id)
	}

	if len(groupIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "no valid task group IDs provided",
		})
		return
	}

	var allTaskIDs []uuid.UUID
	for _, groupID := range groupIDs {
		logs, err := h.taskLogRepo.FindByGroupID(groupID)
		if err != nil {
			continue
		}
		for _, log := range logs {
			if log.Status != "running" && log.Status != "pending" {
				allTaskIDs = append(allTaskIDs, log.ID)
			}
		}
	}

	if h.healingLogRepo != nil && len(allTaskIDs) > 0 {
		h.healingLogRepo.DeleteByOriginalTaskIDs(allTaskIDs)
	}

	deletedCount, skippedCount, err := h.taskLogRepo.BatchDeleteByGroupIDs(groupIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to batch delete task groups",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "batch delete completed",
		"data": gin.H{
			"deleted_count": deletedCount,
			"skipped_count": skippedCount,
		},
	})
}

# Task Execution Pipeline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement complete task execution pipeline with async script generation, gRPC dispatch, and result processing.

**Architecture:** Orchestrated pipeline where TaskService coordinates script generation, persistence, and agent communication. Async queue-based script generation with status tracking through pending → generating → running → success/failed states.

**Tech Stack:** Go 1.20+, Gin, gRPC, PostgreSQL, Redis, existing LLM client

---

## Task 1: Update TaskLog Model with Status Constants

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/model/task_log.go`

**Step 1: Add status constants**

Add after the TaskLog struct definition:

```go
// Task status constants
const (
	TaskStatusPending    = "pending"
	TaskStatusGenerating = "generating"
	TaskStatusRunning    = "running"
	TaskStatusSuccess    = "success"
	TaskStatusFailed     = "failed"
)
```

**Step 2: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./...`
Expected: Success, no errors

**Step 3: Commit**

```bash
git add backend/internal/model/task_log.go
git commit -m "feat(task): add task status constants"
```

---

## Task 2: Add TaskService Dependencies

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/service/task_service.go`

**Step 1: Add gRPC server dependency**

Update the TaskService struct to include grpcServer:

```go
type TaskService struct {
	taskLogRepo        *repository.TaskLogRepository
	hostRepo           *repository.HostRepository
	ruleRepo           *repository.RuleRepository
	healingLogRepo     *repository.HealingLogRepository
	redisClient        *storage.RedisClient
	selfHealingService *SelfHealingService
	grpcServer         *grpc_server.GRPCServer  // NEW
}
```

**Step 2: Update constructor**

Modify NewTaskService function signature:

```go
func NewTaskService(
	taskLogRepo *repository.TaskLogRepository,
	hostRepo *repository.HostRepository,
	ruleRepo *repository.RuleRepository,
	healingLogRepo *repository.HealingLogRepository,
	redisClient *storage.RedisClient,
	selfHealingService *SelfHealingService,
	grpcServer *grpc_server.GRPCServer,  // NEW
) *TaskService {
	return &TaskService{
		taskLogRepo:        taskLogRepo,
		hostRepo:           hostRepo,
		ruleRepo:           ruleRepo,
		healingLogRepo:     healingLogRepo,
		redisClient:        redisClient,
		selfHealingService: selfHealingService,
		grpcServer:         grpcServer,  // NEW
	}
}
```

**Step 3: Add import**

Add to imports section:

```go
import (
	"context"
	"time"

	"aegis-system/internal/grpc_server"  // ADD THIS
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/internal/storage"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)
```

**Step 4: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./internal/service`
Expected: Success

**Step 5: Commit**

```bash
git add backend/internal/service/task_service.go
git commit -m "feat(task): add grpc server dependency to task service"
```

---

## Task 3: Implement Script-Ready Check Method

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/service/task_service.go`

**Step 1: Add helper method to check script availability**

Add after the DispatchTask method (around line 172):

```go
// isScriptReady checks if the script is ready for the given rule and script type
func (s *TaskService) isScriptReady(rule *model.BaselineRule, scriptType string) bool {
	if scriptType == "CHECK" {
		return rule.GeneratedCheckScript != nil && *rule.GeneratedCheckScript != ""
	}
	return rule.GeneratedFixScript != nil && *rule.GeneratedFixScript != ""
}

// getScriptContent retrieves the script content for the given rule and script type
func (s *TaskService) getScriptContent(rule *model.BaselineRule, scriptType string) *string {
	if scriptType == "CHECK" {
		return rule.GeneratedCheckScript
	}
	return rule.GeneratedFixScript
}
```

**Step 2: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./internal/service`
Expected: Success

**Step 3: Commit**

```bash
git add backend/internal/service/task_service.go
git commit -m "feat(task): add script ready check helpers"
```

---

## Task 4: Implement gRPC Dispatch Method

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/service/task_service.go`

**Step 1: Add dispatchTaskToAgent method**

Add after the helper methods from Task 3:

```go
// dispatchTaskToAgent sends a task command to an agent via gRPC
func (s *TaskService) dispatchTaskToAgent(ctx context.Context, taskLog *model.TaskLog, hostID uuid.UUID) error {
	if s.grpcServer == nil {
		logger.Error("grpc server not initialized")
		return fmt.Errorf("grpc server not initialized")
	}

	cmd := &pb.CommandExecute{
		TaskId:        taskLog.ID.String(),
		ScriptType:    taskLog.TaskType,
		ScriptContent: *taskLog.ScriptContent,
	}

	if err := s.grpcServer.SendCommand(hostID, cmd); err != nil {
		logger.Error("failed to send command to agent",
			zap.Error(err),
			zap.String("task_id", taskLog.ID.String()),
			zap.String("host_id", hostID.String()),
		)
		return err
	}

	logger.Info("task dispatched to agent",
		zap.String("task_id", taskLog.ID.String()),
		zap.String("host_id", hostID.String()),
	)

	return nil
}
```

**Step 2: Add required imports**

Add to imports:

```go
import (
	"context"
	"fmt"  // ADD THIS
	"time"

	"aegis-system/internal/grpc_server"
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/internal/storage"
	"aegis-system/pkg/api/v1"  // ADD THIS (for pb)
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)
```

**Step 3: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./internal/service`
Expected: Success

**Step 4: Commit**

```bash
git add backend/internal/service/task_service.go
git commit -m "feat(task): implement grpc dispatch method"
```

---

## Task 5: Update DispatchTask to Support Async Flow

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/service/task_service.go`

**Step 1: Add response type with more details**

Update the TaskDispatchResponse struct (around line 35):

```go
// TaskDispatchResponse 任务下发响应
type TaskDispatchResponse struct {
	TaskGroupID  uuid.UUID
	TotalTasks   int
	Dispatched   int
	Failed       int
	FailedHosts  []string
}
```

**Step 2: Rewrite DispatchTask method**

Replace the entire DispatchTask method (lines 59-172) with:

```go
// DispatchTask 下发任务
func (s *TaskService) DispatchTask(ctx context.Context, req TaskDispatchRequest) (*TaskDispatchResponse, error) {
	logger.Info("dispatching task",
		zap.Int("rule_count", len(req.RuleIDs)),
		zap.Int("host_count", len(req.HostIDs)),
		zap.String("script_type", req.ScriptType),
	)

	// 生成任务组 ID
	taskGroupID := uuid.New()

	response := &TaskDispatchResponse{
		TaskGroupID: taskGroupID,
		FailedHosts: []string{},
	}

	// 为每个规则 - 主机对创建子任务
	for _, ruleID := range req.RuleIDs {
		// 获取规则
		rule, err := s.ruleRepo.FindByID(ruleID)
		if err != nil {
			logger.Error("failed to find rule",
				zap.Error(err),
				zap.String("rule_id", ruleID.String()),
			)
			continue
		}

		// 检查脚本是否已生成
		scriptReady := s.isScriptReady(rule, req.ScriptType)
		var scriptContent *string
		var scriptVersion *int

		if scriptReady {
			scriptContent = s.getScriptContent(rule, req.ScriptType)
			if req.ScriptType == "CHECK" {
				v := rule.CheckScriptVersion
				scriptVersion = &v
			} else {
				v := rule.FixScriptVersion
				scriptVersion = &v
			}
		}

		// 为每个主机创建任务
		for _, hostID := range req.HostIDs {
			response.TotalTasks++

			// 检查主机是否在线
			online, err := s.redisClient.IsOnline(hostID.String())
			if err != nil {
				logger.Error("failed to check host online status",
					zap.Error(err),
					zap.String("host_id", hostID.String()),
				)
				response.Failed++
				response.FailedHosts = append(response.FailedHosts, hostID.String())
				continue
			}

			if !online {
				logger.Warn("host is offline, skipping",
					zap.String("host_id", hostID.String()),
				)
				// 离线主机直接标记为失败
				s.markTaskAsFailed(taskGroupID, ruleID, hostID, req.ScriptType, "主机离线")
				response.Failed++
				response.FailedHosts = append(response.FailedHosts, hostID.String())
				continue
			}

			// 确定任务状态
			status := model.TaskStatusPending
			if !scriptReady {
				status = model.TaskStatusGenerating
			}

			// 创建任务日志记录
			now := time.Now()
			taskLog := &model.TaskLog{
				TaskGroupID:   taskGroupID,
				RuleID:        ruleID,
				HostID:        hostID,
				TaskType:      req.ScriptType,
				Status:        status,
				ScriptContent: scriptContent,
				ScriptVersion: scriptVersion,
				StartedAt:     &now,
			}

			if err := s.taskLogRepo.Create(taskLog); err != nil {
				logger.Error("failed to create task log",
					zap.Error(err),
					zap.String("rule_id", ruleID.String()),
					zap.String("host_id", hostID.String()),
				)
				response.Failed++
				response.FailedHosts = append(response.FailedHosts, hostID.String())
				continue
			}

			logger.Info("task created",
				zap.String("task_id", taskLog.ID.String()),
				zap.String("task_group_id", taskGroupID.String()),
				zap.String("rule_id", ruleID.String()),
				zap.String("host_id", hostID.String()),
				zap.String("status", status),
			)

			// 如果脚本已准备好，立即下发
			if scriptReady && scriptContent != nil {
				if err := s.dispatchTaskToAgent(ctx, taskLog, hostID); err != nil {
					// 下发失败，标记任务为失败
					s.updateTaskStatus(taskLog.ID, model.TaskStatusFailed, "gRPC dispatch error: "+err.Error())
					response.Failed++
					response.FailedHosts = append(response.FailedHosts, hostID.String())
					continue
				}
				// 更新状态为 running
				s.updateTaskStatus(taskLog.ID, model.TaskStatusRunning, "")
			}

			response.Dispatched++
		}
	}

	logger.Info("task dispatch completed",
		zap.String("task_group_id", taskGroupID.String()),
		zap.Int("total_tasks", response.TotalTasks),
		zap.Int("dispatched", response.Dispatched),
		zap.Int("failed", response.Failed),
	)

	return response, nil
}
```

**Step 3: Add updateTaskStatus helper method**

Add after dispatchTaskToAgent:

```go
// updateTaskStatus updates task status and optionally sets an error message
func (s *TaskService) updateTaskStatus(taskID uuid.UUID, status string, errorMsg string) {
	updates := map[string]interface{}{
		"status": status,
	}

	if errorMsg != "" {
		updates["stderr"] = errorMsg
	}

	if status == model.TaskStatusFailed || status == model.TaskStatusSuccess {
		now := time.Now()
		updates["finished_at"] = &now
	}

	if err := s.taskLogRepo.DB().Model(&model.TaskLog{}).
		Where("id = ?", taskID).
		Updates(updates).Error; err != nil {
		logger.Error("failed to update task status",
			zap.Error(err),
			zap.String("task_id", taskID.String()),
		)
	}
}
```

**Step 4: Add DB() method to TaskLogRepository**

We need to expose the DB instance. Modify `/code/ai-benchmark/backend/internal/repository/task_log_repo.go`:

Add after the FindByID method:

```go
// DB returns the underlying database connection for advanced queries
func (r *TaskLogRepository) DB() *gorm.DB {
	return r.db
}
```

**Step 5: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./...`
Expected: Success

**Step 6: Commit**

```bash
git add backend/internal/service/task_service.go backend/internal/repository/task_log_repo.go
git commit -m "feat(task): implement async dispatch with status tracking"
```

---

## Task 6: Update TaskHandler with Real Implementation

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/api/handler/task_handler.go`

**Step 1: Implement RunCheck endpoint**

Replace the RunCheck method (lines 26-49) with:

```go
// RunCheck 执行检查任务
func (h *TaskHandler) RunCheck(c *gin.Context) {
	var req struct {
		RuleIDs []string `json:"rule_ids" binding:"required"`
		HostIDs []string `json:"host_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body: " + err.Error(),
		})
		return
	}

	// Validate arrays are not empty
	if len(req.RuleIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "rule_ids cannot be empty",
		})
		return
	}

	if len(req.HostIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "host_ids cannot be empty",
		})
		return
	}

	// Parse UUIDs
	ruleIDs := make([]uuid.UUID, 0, len(req.RuleIDs))
	for _, idStr := range req.RuleIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "invalid rule_id: " + idStr,
			})
			return
		}
		ruleIDs = append(ruleIDs, id)
	}

	hostIDs := make([]uuid.UUID, 0, len(req.HostIDs))
	for _, idStr := range req.HostIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "invalid host_id: " + idStr,
			})
			return
		}
		hostIDs = append(hostIDs, id)
	}

	// Dispatch task
	dispatchReq := service.TaskDispatchRequest{
		RuleIDs:    ruleIDs,
		HostIDs:    hostIDs,
		ScriptType: "CHECK",
	}

	resp, err := h.taskService.DispatchTask(c.Request.Context(), dispatchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to dispatch tasks: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"code":    0,
		"message": "check tasks queued",
		"data": gin.H{
			"task_group_id": resp.TaskGroupID.String(),
			"total_tasks":   resp.TotalTasks,
			"dispatched":    resp.Dispatched,
			"failed":        resp.Failed,
			"failed_hosts":  resp.FailedHosts,
		},
	})
}
```

**Step 2: Implement RunFix endpoint**

Replace the RunFix method (lines 51-75) with:

```go
// RunFix 执行修复任务
func (h *TaskHandler) RunFix(c *gin.Context) {
	var req struct {
		RuleIDs []string `json:"rule_ids" binding:"required"`
		HostIDs []string `json:"host_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid request body: " + err.Error(),
		})
		return
	}

	// Validate arrays are not empty
	if len(req.RuleIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "rule_ids cannot be empty",
		})
		return
	}

	if len(req.HostIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "host_ids cannot be empty",
		})
		return
	}

	// Parse UUIDs
	ruleIDs := make([]uuid.UUID, 0, len(req.RuleIDs))
	for _, idStr := range req.RuleIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "invalid rule_id: " + idStr,
			})
			return
		}
		ruleIDs = append(ruleIDs, id)
	}

	hostIDs := make([]uuid.UUID, 0, len(req.HostIDs))
	for _, idStr := range req.HostIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "invalid host_id: " + idStr,
			})
			return
		}
		hostIDs = append(hostIDs, id)
	}

	// Dispatch task
	dispatchReq := service.TaskDispatchRequest{
		RuleIDs:    ruleIDs,
		HostIDs:    hostIDs,
		ScriptType: "FIX",
	}

	resp, err := h.taskService.DispatchTask(c.Request.Context(), dispatchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to dispatch tasks: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"code":    0,
		"message": "fix tasks queued",
		"data": gin.H{
			"task_group_id": resp.TaskGroupID.String(),
			"total_tasks":   resp.TotalTasks,
			"dispatched":    resp.Dispatched,
			"failed":        resp.Failed,
			"failed_hosts":  resp.FailedHosts,
		},
	})
}
```

**Step 3: Add required imports**

Update imports section:

```go
import (
	"aegis-system/internal/grpc_server"
	"aegis-system/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"  // ADD THIS
)
```

**Step 4: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./internal/api/handler`
Expected: Success

**Step 5: Commit**

```bash
git add backend/internal/api/handler/task_handler.go
git commit -m "feat(task): implement run-check and run-fix endpoints"
```

---

## Task 7: Implement GetTaskStatus Endpoint

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/api/handler/task_handler.go`

**Step 1: Add GetTaskStatus implementation**

Replace the GetTaskStatus method (lines 77-94) with:

```go
// GetTaskStatus 获取任务状态
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

	status, err := h.taskService.GetTaskGroupStatus(taskGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to get task status: " + err.Error(),
		})
		return
	}

	total := 0
	for _, count := range status {
		total += count
	}

	progressPercent := 0
	if total > 0 {
		completed := status[model.TaskStatusSuccess] + status[model.TaskStatusFailed]
		progressPercent = (completed * 100) / total
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task_group_id":    taskGroupIDStr,
			"total":            total,
			"pending":          status[model.TaskStatusPending],
			"generating":       status[model.TaskStatusGenerating],
			"running":          status[model.TaskStatusRunning],
			"success":          status[model.TaskStatusSuccess],
			"failed":           status[model.TaskStatusFailed],
			"progress_percent": progressPercent,
		},
	})
}
```

**Step 2: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./internal/api/handler`
Expected: Success

**Step 3: Commit**

```bash
git add backend/internal/api/handler/task_handler.go
git commit -m "feat(task): implement get task status endpoint"
```

---

## Task 8: Implement GetTaskLogs Endpoint

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/api/handler/task_handler.go`

**Step 1: Implement GetTaskLogs**

Replace the GetTaskLogs method (lines 96-111) with:

```go
// GetTaskLogs 获取任务日志
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

	// Get query parameters
	statusFilter := c.Query("status")
	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Get task logs with details
	tasks, err := h.taskService.GetTaskLogsWithDetails(taskGroupID, statusFilter, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to get task logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task_group_id": taskGroupIDStr,
			"tasks":         tasks,
			"total":         len(tasks),
		},
	})
}
```

**Step 2: Add strconv import**

Update imports:

```go
import (
	"aegis-system/internal/grpc_server"
	"aegis-system/internal/service"
	"net/http"
	"strconv"  // ADD THIS

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)
```

**Step 3: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./internal/api/handler`
Expected: Error - GetTaskLogsWithDetails not implemented yet (this is expected)

**Step 4: Commit**

```bash
git add backend/internal/api/handler/task_handler.go
git commit -m "feat(task): implement get task logs endpoint"
```

---

## Task 9: Implement GetTaskLogsWithDetails in TaskService

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/service/task_service.go`

**Step 1: Add response struct**

Add after the TaskDispatchResponse struct:

```go
// TaskLogDetail 任务日志详情
type TaskLogDetail struct {
	TaskID        string  `json:"task_id"`
	RuleID        string  `json:"rule_id"`
	RuleTitle     string  `json:"rule_title"`
	HostID        string  `json:"host_id"`
	Hostname      string  `json:"hostname"`
	Status        string  `json:"status"`
	ScriptContent *string `json:"script_content"`
	Stdout        *string `json:"stdout"`
	Stderr        *string `json:"stderr"`
	ExitCode      *int    `json:"exit_code"`
	ScriptVersion *int    `json:"script_version"`
	StartedAt     *string `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
}
```

**Step 2: Implement GetTaskLogsWithDetails**

Add after GetTaskGroupStatus method (around line 254):

```go
// GetTaskLogsWithDetails 获取任务日志详情（包含规则和主机信息）
func (s *TaskService) GetTaskLogsWithDetails(taskGroupID uuid.UUID, statusFilter string, limit int) ([]TaskLogDetail, error) {
	var logs []model.TaskLog
	query := s.taskLogRepo.DB().Where("task_group_id = ?", taskGroupID)

	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	if err := query.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		logger.Error("failed to find task logs",
			zap.Error(err),
			zap.String("task_group_id", taskGroupID.String()),
		)
		return nil, err
	}

	details := make([]TaskLogDetail, 0, len(logs))
	for _, log := range logs {
		// Get rule title
		rule, err := s.ruleRepo.FindByID(log.RuleID)
		ruleTitle := ""
		if err == nil {
			ruleTitle = rule.Title
		}

		// Get hostname
		host, err := s.hostRepo.FindByID(log.HostID)
		hostname := ""
		if err == nil {
			hostname = host.Hostname
		}

		var startedAt, finishedAt *string
		if log.StartedAt != nil {
			t := log.StartedAt.Format("2006-01-02T15:04:05Z")
			startedAt = &t
		}
		if log.FinishedAt != nil {
			t := log.FinishedAt.Format("2006-01-02T15:04:05Z")
			finishedAt = &t
		}

		details = append(details, TaskLogDetail{
			TaskID:        log.ID.String(),
			RuleID:        log.RuleID.String(),
			RuleTitle:     ruleTitle,
			HostID:        log.HostID.String(),
			Hostname:      hostname,
			Status:        log.Status,
			ScriptContent: log.ScriptContent,
			Stdout:        log.Stdout,
			Stderr:        log.Stderr,
			ExitCode:      log.ExitCode,
			ScriptVersion: log.ScriptVersion,
			StartedAt:     startedAt,
			FinishedAt:    finishedAt,
		})
	}

	return details, nil
}
```

**Step 3: Add FindByID to HostRepository**

Check if FindByID exists in `/code/ai-benchmark/backend/internal/repository/host_repo.go`. If not, add:

```go
func (r *HostRepository) FindByID(id uuid.UUID) (*model.Host, error) {
	var host model.Host
	result := r.db.First(&host, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to find host by id",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return nil, result.Error
	}
	return &host, nil
}
```

**Step 4: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./...`
Expected: Success

**Step 5: Commit**

```bash
git add backend/internal/service/task_service.go backend/internal/repository/host_repo.go
git commit -m "feat(task): implement get task logs with details"
```

---

## Task 10: Connect GRPCServer Result Handler to TaskService

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/grpc_server/server.go`

**Step 1: Add TaskService to GRPCServer**

Update GRPCServer struct (around line 21):

```go
type GRPCServer struct {
	pb.UnimplementedAgentServiceServer
	server           *grpc.Server
	hostRepo         *repository.HostRepository
	redisClient      *storage.RedisClient
	agentConnections sync.Map
	port             int
	taskService      TaskServiceInterface  // NEW
}
```

**Step 2: Add TaskServiceInterface**

Add before GRPCServer struct:

```go
// TaskServiceInterface defines the interface for task service (to avoid circular dependency)
type TaskServiceInterface interface {
	ProcessTaskResult(ctx context.Context, taskID uuid.UUID, exitCode int, stdout, stderr string) error
}
```

**Step 3: Update NewGRPCServer**

Modify the constructor (around line 40):

```go
func NewGRPCServer(
	hostRepo *repository.HostRepository,
	redisClient *storage.RedisClient,
	port int,
	taskService TaskServiceInterface,  // ADD THIS
) *GRPCServer {
	return &GRPCServer{
		hostRepo:    hostRepo,
		redisClient: redisClient,
		port:        port,
		taskService: taskService,  // ADD THIS
	}
}
```

**Step 4: Update ExecuteCommand result handler**

Update the case for CommandRequest_Result (around line 265):

```go
case *pb.CommandRequest_Result:
	// Agent 返回命令执行结果
	result := r.Result
	logger.Info("command result received",
		zap.String("task_id", result.TaskId),
		zap.Stringer("host_id", hostID),
		zap.Int32("exit_code", result.ExitCode),
		zap.Bool("is_final", result.IsFinal),
	)

	// Process the result via TaskService
	taskID, err := uuid.Parse(result.TaskId)
	if err != nil {
		logger.Error("invalid task_id in result",
			zap.Error(err),
			zap.String("task_id", result.TaskId),
		)
		continue
	}

	if s.taskService != nil {
		if err := s.taskService.ProcessTaskResult(
			ctx,
			taskID,
			int(result.ExitCode),
			result.Stdout,
			result.Stderr,
		); err != nil {
			logger.Error("failed to process task result",
				zap.Error(err),
				zap.String("task_id", result.TaskId),
			)
		}
	} else {
		logger.Warn("task service not set, cannot process result")
	}
```

**Step 5: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./internal/grpc_server`
Expected: Success

**Step 6: Commit**

```bash
git add backend/internal/grpc_server/server.go
git commit -m "feat(task): connect grpc result handler to task service"
```

---

## Task 11: Wire Up Dependencies in main.go

**Files:**
- Modify: `/code/ai-benchmark/backend/cmd/server/main.go`

**Step 1: Read current main.go**

Check the dependency injection flow to understand how to add the new dependencies.

**Step 2: Update initialization order**

This depends on the current structure. The key changes needed:
1. Create TaskService with GRPCServer reference
2. Pass TaskService to GRPCServer constructor

Look for the section where repositories and services are initialized and update accordingly.

**Step 3: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./cmd/server`
Expected: Success

**Step 4: Commit**

```bash
git add backend/cmd/server/main.go
git commit -m "feat(task): wire up task service dependencies"
```

---

## Task 12: Add Helper Function to task_service.go

**Files:**
- Modify: `/code/ai-benchmark/backend/internal/service/task_service.go`

**Step 1: Add missing pointerToTime helper**

Find the helper functions section (around line 286) and ensure this exists:

```go
// pointerToTime returns a pointer to a time.Time value
func pointerToTime(t time.Time) *time.Time {
	return &t
}
```

**Step 2: Verify compilation**

Run: `cd /code/ai-benchmark/backend && go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add backend/internal/service/task_service.go
git commit -m "fix(task): add missing helper function"
```

---

## Task 13: Manual Testing

**Step 1: Start backend**

Run: `cd /code/ai-benchmark/backend && go run ./cmd/server`

**Step 2: Test POST /api/v1/tasks/run-check with valid request**

```bash
curl -X POST http://localhost:8080/api/v1/tasks/run-check \
  -H "Content-Type: application/json" \
  -d '{
    "rule_ids": ["<valid-uuid>"],
    "host_ids": ["<valid-uuid>"]
  }'
```

Expected: 202 Accepted with task_group_id

**Step 3: Test GET /api/v1/tasks/:id/status**

```bash
curl http://localhost:8080/api/v1/tasks/<task_group_id>/status
```

Expected: 200 OK with status breakdown

**Step 4: Test GET /api/v1/tasks/:id/logs**

```bash
curl http://localhost:8080/api/v1/tasks/<task_group_id>/logs
```

Expected: 200 OK with task list

**Step 5: Test error cases**
- Empty arrays → 400 Bad Request
- Invalid UUIDs → 400 Bad Request
- Non-existent task_group_id → Empty results

---

## Verification Checklist

After completing all tasks, verify:

- [ ] `go build ./...` compiles successfully
- [ ] POST /api/v1/tasks/run-check returns 202 with task_group_id
- [ ] POST /api/v1/tasks/run-fix returns 202 with task_group_id
- [ ] GET /api/v1/tasks/:id/status returns correct status breakdown
- [ ] GET /api/v1/tasks/:id/logs returns task details
- [ ] TaskLog records created in database
- [ ] Status transitions work: pending → generating → running → success/failed
- [ ] gRPC commands sent to online agents
- [ ] Results processed and stored correctly

---

## Notes

- This plan assumes the existing infrastructure (database, Redis, MinIO, LLM client) is already configured
- The gRPC integration requires agents to be connected for end-to-end testing
- Script generation integration will be enhanced in future phases
- Self-healing is automatically triggered on failure (existing service)
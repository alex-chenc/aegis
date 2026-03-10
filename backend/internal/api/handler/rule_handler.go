package handler

import (
	"net/http"

	"baseline-system/internal/repository"
	"baseline-system/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RuleHandler struct {
	ruleRepo         *repository.RuleRepository
	taskLogRepo      *repository.TaskLogRepository
	scriptGenService *service.ScriptGenerationService
}

func NewRuleHandler(
	ruleRepo *repository.RuleRepository,
	taskLogRepo *repository.TaskLogRepository,
	scriptGenService *service.ScriptGenerationService,
) *RuleHandler {
	return &RuleHandler{
		ruleRepo:         ruleRepo,
		taskLogRepo:      taskLogRepo,
		scriptGenService: scriptGenService,
	}
}

type GenerateScriptRequest struct {
	ScriptType string `json:"script_type" binding:"required,oneof=CHECK FIX"`
}

type UpdateScriptRequest struct {
	ScriptType    string `json:"script_type" binding:"required,oneof=CHECK FIX"`
	ScriptContent string `json:"script_content" binding:"required"`
}

type HasTasksResponse struct {
	HasTasks  bool  `json:"has_tasks"`
	TaskCount int64 `json:"task_count"`
}

func (h *RuleHandler) GenerateScript(c *gin.Context) {
	id := c.Param("id")
	ruleID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	var req GenerateScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}

	rule, err := h.ruleRepo.FindByID(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "rule not found"})
		return
	}

	if req.ScriptType == "CHECK" && rule.GeneratedCheckScript != nil && *rule.GeneratedCheckScript != "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "script already exists",
			"data": gin.H{
				"rule_id":        ruleID.String(),
				"script_type":    req.ScriptType,
				"script_content": *rule.GeneratedCheckScript,
				"version":        rule.CheckScriptVersion,
			},
		})
		return
	}

	if req.ScriptType == "FIX" && rule.GeneratedFixScript != nil && *rule.GeneratedFixScript != "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "script already exists",
			"data": gin.H{
				"rule_id":        ruleID.String(),
				"script_type":    req.ScriptType,
				"script_content": *rule.GeneratedFixScript,
				"version":        rule.FixScriptVersion,
			},
		})
		return
	}

	if err := h.scriptGenService.QueueScriptGeneration(ruleID, req.ScriptType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to queue script generation"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"code":    0,
		"message": "script generation queued",
		"data": gin.H{
			"rule_id":     ruleID.String(),
			"script_type": req.ScriptType,
			"status":      "generating",
		},
	})
}

func (h *RuleHandler) UpdateScript(c *gin.Context) {
	id := c.Param("id")
	ruleID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	var req UpdateScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}

	_, err = h.ruleRepo.FindByID(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "rule not found"})
		return
	}

	if err := h.ruleRepo.UpdateScriptContent(ruleID, req.ScriptType, req.ScriptContent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to update script"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "script updated successfully",
	})
}

func (h *RuleHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	ruleID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	hasTasks, taskCount, err := h.ruleRepo.HasTasks(ruleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to check rule tasks"})
		return
	}

	if hasTasks {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "该规则有关联任务，无法删除",
			"data": gin.H{
				"task_count": taskCount,
			},
		})
		return
	}

	if err := h.ruleRepo.Delete(ruleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to delete rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "rule deleted successfully",
	})
}

func (h *RuleHandler) HasTasks(c *gin.Context) {
	id := c.Param("id")
	ruleID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	hasTasks, taskCount, err := h.ruleRepo.HasTasks(ruleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to check rule tasks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": HasTasksResponse{
			HasTasks:  hasTasks,
			TaskCount: taskCount,
		},
	})
}

func (h *RuleHandler) GetScript(c *gin.Context) {
	id := c.Param("id")
	ruleID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid rule id"})
		return
	}

	scriptType := c.Query("script_type")
	if scriptType != "CHECK" && scriptType != "FIX" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "script_type must be CHECK or FIX"})
		return
	}

	rule, err := h.ruleRepo.FindByID(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "rule not found"})
		return
	}

	var scriptContent string
	var version int
	if scriptType == "CHECK" {
		if rule.GeneratedCheckScript != nil {
			scriptContent = *rule.GeneratedCheckScript
		}
		version = rule.CheckScriptVersion
	} else {
		if rule.GeneratedFixScript != nil {
			scriptContent = *rule.GeneratedFixScript
		}
		version = rule.FixScriptVersion
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"rule_id":        ruleID.String(),
			"script_type":    scriptType,
			"script_content": scriptContent,
			"version":        version,
		},
	})
}

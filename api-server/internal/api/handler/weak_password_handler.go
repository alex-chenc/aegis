package handler

import (
	"errors"
	"net/http"
	"strconv"

	"api-server/internal/model"
	"api-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type WeakPasswordHandler struct {
	service *service.WeakPasswordService
	logger  *zap.Logger
}

func NewWeakPasswordHandler(service *service.WeakPasswordService, logger *zap.Logger) *WeakPasswordHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WeakPasswordHandler{service: service, logger: logger}
}

func (h *WeakPasswordHandler) RegisterRoutes(api *gin.RouterGroup) {
	wp := api.Group("/weak-password")
	{
		wp.POST("/asset-applications/analyze", h.AnalyzeAssetApplications)
		wp.GET("/asset-applications", h.ListAssetApplications)
		wp.POST("/tasks", h.CreateTask)
		wp.POST("/tasks/by-application", h.CreateTaskByApplication)
		wp.GET("/tasks", h.ListTasks)
		wp.GET("/tasks/:id", h.GetTask)
		wp.GET("/tasks/:id/progress", h.GetTaskProgress)
		wp.GET("/tasks/:id/hosts", h.GetTaskHosts)
		wp.GET("/tasks/:id/findings", h.GetTaskFindings)
		wp.POST("/tasks/:id/retry-failed", h.RetryFailedTask)
		wp.DELETE("/tasks/:id", h.DeleteTask)
		wp.GET("/dictionaries/default", h.GetDefaultDictionary)
		wp.GET("/dictionaries", h.ListDictionaries)
		wp.POST("/dictionaries", h.CreateDictionary)
		wp.POST("/dictionaries/ai-generate", h.GenerateDictionary)
		wp.POST("/findings/:id/reveal", h.RevealFinding)
	}

	api.POST("/assistant/tools/weak-password.scan", h.AssistantScan)
	api.POST("/assistant/tools/weak-password.explain", h.AssistantExplain)
}

func (h *WeakPasswordHandler) AnalyzeAssetApplications(c *gin.Context) {
	var req model.AnalyzeAssetApplicationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.service.AnalyzeAssetApplications(c.Request.Context(), req, currentUserID(c))
	if err != nil {
		h.logger.Error("weak password asset analysis failed", zap.Error(err))
		errorJSON(c, http.StatusInternalServerError, "Failed to analyze asset applications")
		return
	}
	if resp.Status == "failed" {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": resp.Message, "data": resp})
		return
	}
	successJSON(c, resp)
}

func (h *WeakPasswordHandler) ListAssetApplications(c *gin.Context) {
	page, pageSize := pageParams(c)
	items, total, err := h.service.ListCandidateApplications(
		c.Query("analysis_id"),
		c.Query("host_id"),
		c.Query("application_type"),
		c.Query("confidence"),
		page,
		pageSize,
	)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, err.Error())
		return
	}
	successJSON(c, gin.H{"items": items, "total": total})
}

func (h *WeakPasswordHandler) CreateTask(c *gin.Context) {
	var req model.CreateTaskByApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.CandidateApplicationID == "" {
		errorJSON(c, http.StatusBadRequest, "candidate_application_id is required")
		return
	}
	h.createTaskByApplication(c, req)
}

func (h *WeakPasswordHandler) CreateTaskByApplication(c *gin.Context) {
	var req model.CreateTaskByApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	h.createTaskByApplication(c, req)
}

func (h *WeakPasswordHandler) createTaskByApplication(c *gin.Context, req model.CreateTaskByApplicationRequest) {
	resp, err := h.service.CreateTaskByApplication(c.Request.Context(), req, currentUserID(c))
	if err != nil {
		h.logger.Error("weak password task creation failed", zap.Error(err))
		if errors.Is(err, service.ErrWeakPasswordHostOffline) {
			errorJSON(c, http.StatusBadRequest, "目标主机 Agent 不在线，无法执行弱密码检测")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "Failed to create weak password task")
		return
	}
	successJSON(c, resp)
}

func (h *WeakPasswordHandler) ListTasks(c *gin.Context) {
	page, pageSize := pageParams(c)
	tasks, total, err := h.service.ListTasks(page, pageSize, c.Query("status"))
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to list tasks")
		return
	}
	successJSON(c, gin.H{"items": tasks, "total": total})
}

func (h *WeakPasswordHandler) GetTask(c *gin.Context) {
	taskID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	task, errors, err := h.service.GetTaskDetail(taskID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "Task not found")
		return
	}
	successJSON(c, gin.H{"task": task, "errors": errors})
}

func (h *WeakPasswordHandler) GetTaskProgress(c *gin.Context) {
	taskID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	progress, err := h.service.GetTaskProgress(taskID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "Task not found")
		return
	}
	successJSON(c, progress)
}

func (h *WeakPasswordHandler) GetTaskHosts(c *gin.Context) {
	taskID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	hosts, err := h.service.ListTaskHosts(taskID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to list task hosts")
		return
	}
	successJSON(c, gin.H{"items": hosts, "total": len(hosts)})
}

func (h *WeakPasswordHandler) GetTaskFindings(c *gin.Context) {
	taskID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	findings, err := h.service.ListTaskFindings(taskID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to list findings")
		return
	}
	successJSON(c, gin.H{"items": findings, "total": len(findings)})
}

func (h *WeakPasswordHandler) RetryFailedTask(c *gin.Context) {
	taskID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.service.RetryFailedTask(c.Request.Context(), taskID); err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to retry task")
		return
	}
	successJSON(c, gin.H{"status": "retry_scheduled"})
}

func (h *WeakPasswordHandler) DeleteTask(c *gin.Context) {
	taskID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteTask(taskID); err != nil {
		if errors.Is(err, service.ErrWeakPasswordTaskRunning) {
			errorJSON(c, http.StatusBadRequest, "运行中的弱密码任务不能删除")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "Failed to delete task")
		return
	}
	successJSON(c, gin.H{"deleted": 1})
}

func (h *WeakPasswordHandler) GetDefaultDictionary(c *gin.Context) {
	summary, err := h.service.GetDefaultDictionarySummary()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to get default dictionary")
		return
	}
	successJSON(c, summary)
}

func (h *WeakPasswordHandler) ListDictionaries(c *gin.Context) {
	items, err := h.service.ListDictionaries()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to list dictionaries")
		return
	}
	successJSON(c, gin.H{"items": items, "total": len(items)})
}

func (h *WeakPasswordHandler) CreateDictionary(c *gin.Context) {
	var req service.CreateWeakPasswordDictionaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	summary, err := h.service.CreateDictionary(req, currentUserID(c))
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to create dictionary")
		return
	}
	successJSON(c, summary)
}

func (h *WeakPasswordHandler) GenerateDictionary(c *gin.Context) {
	var req model.AIGenerateDictionaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	summary, err := h.service.GenerateAIDictionary(req, currentUserID(c))
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "Failed to generate dictionary")
		return
	}
	successJSON(c, summary)
}

func (h *WeakPasswordHandler) RevealFinding(c *gin.Context) {
	findingID, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var req model.RevealFindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	requester := currentUserID(c)
	if requester == nil {
		errorJSON(c, http.StatusBadRequest, "requester is required")
		return
	}
	revealed, err := h.service.RevealFinding(findingID, *requester, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			errorJSON(c, http.StatusUnauthorized, "系统密码错误")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "Failed to reveal finding")
		return
	}
	successJSON(c, revealed)
}

func (h *WeakPasswordHandler) AssistantScan(c *gin.Context) {
	var req model.CreateTaskByApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	h.createTaskByApplication(c, req)
}

func (h *WeakPasswordHandler) AssistantExplain(c *gin.Context) {
	successJSON(c, gin.H{
		"summary":        "弱密码结果解释基于命中方式、凭据类型、配置来源和服务端 verifier 状态生成。",
		"recommendation": "请优先修改命中账号密码，禁用默认口令，收敛配置文件权限，并复测确认。",
	})
}

func successJSON(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": data})
}

func errorJSON(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"code": status, "message": message})
}

func pageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func pathUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "Invalid ID")
		return uuid.Nil, false
	}
	return id, true
}

func currentUserID(c *gin.Context) *uuid.UUID {
	raw := c.GetString("auth_user_id")
	if raw == "" {
		return nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &parsed
}

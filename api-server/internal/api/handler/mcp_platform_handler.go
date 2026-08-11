package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"api-server/internal/repository"
	"api-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MCPPlatformHandler struct {
	service          *service.MCPPlatformService
	logger           *zap.Logger
	runtimeSecret    string
	publicGatewayURL string
}

func NewMCPPlatformHandler(svc *service.MCPPlatformService, logger *zap.Logger) *MCPPlatformHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MCPPlatformHandler{service: svc, logger: logger.Named("mcp_platform_handler")}
}

func (h *MCPPlatformHandler) SetRuntimeSecret(secret string) {
	h.runtimeSecret = strings.TrimSpace(secret)
}

func (h *MCPPlatformHandler) SetPublicGatewayBaseURL(baseURL string) {
	h.publicGatewayURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func (h *MCPPlatformHandler) RegisterRoutes(api *gin.RouterGroup, permissions func(string) gin.HandlerFunc) {
	group := api.Group("/mcp-platform")
	group.GET("/overview", permissions(repository.PermissionMCPServerRead), h.GetOverview)
	group.GET("/onboarding-jobs", permissions(repository.PermissionMCPOnboardingRead), h.ListOnboardingJobs)
	group.POST("/onboarding-jobs", permissions(repository.PermissionMCPOnboardingCreate), h.CreateOnboardingJob)
	group.GET("/onboarding-jobs/:id", permissions(repository.PermissionMCPOnboardingRead), h.GetOnboardingJob)
	group.POST("/onboarding-jobs/:id/retry", permissions(repository.PermissionMCPOnboardingOperate), h.RetryOnboardingJob)
	group.POST("/onboarding-jobs/:id/cancel", permissions(repository.PermissionMCPOnboardingOperate), h.CancelOnboardingJob)
	group.GET("/servers", permissions(repository.PermissionMCPServerRead), h.ListServers)
	group.GET("/servers/:id", permissions(repository.PermissionMCPServerRead), h.GetServer)
	group.DELETE("/servers/:id", permissions(repository.PermissionMCPServerWrite), h.DeleteServer)
	group.GET("/tools", permissions(repository.PermissionMCPCatalogRead), h.ListTools)
	group.GET("/catalogs", permissions(repository.PermissionMCPCatalogRead), h.ListCatalogs)
	group.POST("/catalogs", permissions(repository.PermissionMCPCatalogWrite), h.CreateCatalog)
	group.GET("/catalogs/:id/snapshot", permissions(repository.PermissionMCPCatalogRead), h.GetCatalogSnapshot)
	group.GET("/clients", permissions(repository.PermissionMCPClientRead), h.ListClients)
	group.POST("/clients", permissions(repository.PermissionMCPClientWrite), h.CreateClient)
	group.POST("/grants", permissions(repository.PermissionMCPGrantWrite), h.CreateGrant)
	group.GET("/client-endpoints", permissions(repository.PermissionMCPClientRead), h.ListClientEndpoints)
	group.POST("/client-endpoints", permissions(repository.PermissionMCPClientWrite), h.CreateClientEndpoint)
	group.DELETE("/client-endpoints/:client_id", permissions(repository.PermissionMCPClientWrite), h.DeleteClientEndpoint)
	group.PUT("/client-endpoints/:grant_id/tools", permissions(repository.PermissionMCPGrantWrite), h.UpdateClientEndpointTools)
	group.GET("/approvals", permissions(repository.PermissionMCPApprovalRead), h.ListApprovals)
	group.POST("/approvals/:id/approve", permissions(repository.PermissionMCPApprovalDecide), h.Approve)
	group.POST("/approvals/:id/reject", permissions(repository.PermissionMCPApprovalDecide), h.Reject)
	group.GET("/invocations", permissions(repository.PermissionMCPInvocationRead), h.ListInvocations)
	group.POST("/invocations/:id/disable-tool", permissions(repository.PermissionMCPGrantWrite), h.DisableInvocationTool)
	group.GET("/security-verdicts", permissions(repository.PermissionMCPSecurityRead), h.ListSecurityVerdicts)
	group.GET("/security-rules", permissions(repository.PermissionMCPPolicyRead), h.ListSecurityRules)
	group.PUT("/security-rules/:id/enabled", permissions(repository.PermissionMCPPolicyWrite), h.SetSecurityRuleEnabled)
}

// RegisterRuntimeRoutes exposes only the gateway-to-api-server data plane.
// The route is excluded from user-session auth and protected by a separate
// shared secret plus the per-client MCP bearer token.
func (h *MCPPlatformHandler) RegisterRuntimeRoutes(engine *gin.Engine) {
	engine.POST("/internal/mcp-runtime/tools", h.RuntimeTools)
	engine.POST("/internal/mcp-runtime/call", h.RuntimeCall)
}

func (h *MCPPlatformHandler) GetOverview(c *gin.Context) {
	data, err := h.service.Overview(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "overview_failed", err)
		return
	}
	h.success(c, data)
}

func (h *MCPPlatformHandler) CreateOnboardingJob(c *gin.Context) {
	var req service.MCPOnboardingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_request", err)
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	operator := authOperator(c)
	job, err := h.service.CreateOnboardingJob(c.Request.Context(), req, key, operator)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrMCPPlatformJobConflict) {
			status = http.StatusConflict
		}
		h.writeError(c, status, "onboarding_create_failed", err)
		return
	}
	h.successStatus(c, http.StatusAccepted, job)
}

func (h *MCPPlatformHandler) ListOnboardingJobs(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListJobs(c.Request.Context(), repository.MCPOnboardingJobQuery{Status: c.Query("status"), Page: page, PageSize: size})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "onboarding_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func (h *MCPPlatformHandler) GetOnboardingJob(c *gin.Context) {
	id, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_job_id", err)
		return
	}
	job, err := h.service.GetJob(c.Request.Context(), id)
	if err != nil {
		h.notFoundOrError(c, "job_not_found", err)
		return
	}
	h.success(c, job)
}

func (h *MCPPlatformHandler) RetryOnboardingJob(c *gin.Context) {
	id, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_job_id", err)
		return
	}
	job, err := h.service.RetryOnboardingJob(c.Request.Context(), id, authOperator(c))
	if err != nil {
		h.writeError(c, http.StatusConflict, "onboarding_retry_failed", err)
		return
	}
	h.successStatus(c, http.StatusAccepted, job)
}

func (h *MCPPlatformHandler) CancelOnboardingJob(c *gin.Context) {
	id, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_job_id", err)
		return
	}
	job, err := h.service.CancelOnboardingJob(c.Request.Context(), id, authOperator(c))
	if err != nil {
		h.writeError(c, http.StatusConflict, "onboarding_cancel_failed", err)
		return
	}
	h.success(c, job)
}

func (h *MCPPlatformHandler) ListServers(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListServers(c.Request.Context(), repository.MCPServerQuery{Keyword: c.Query("keyword"), Environment: c.Query("environment"), Status: c.Query("status"), RiskTier: c.Query("risk_tier"), Page: page, PageSize: size})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "server_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func (h *MCPPlatformHandler) GetServer(c *gin.Context) {
	id, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_server_id", err)
		return
	}
	server, err := h.service.GetServer(c.Request.Context(), id)
	if err != nil {
		h.notFoundOrError(c, "server_not_found", err)
		return
	}
	h.success(c, server)
}

func (h *MCPPlatformHandler) DeleteServer(c *gin.Context) {
	id, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_server_id", err)
		return
	}
	server, err := h.service.RetireServer(c.Request.Context(), id, authOperator(c))
	if err != nil {
		h.notFoundOrError(c, "server_delete_failed", err)
		return
	}
	h.success(c, server)
}

func (h *MCPPlatformHandler) ListTools(c *gin.Context) {
	page, size := mcpPageParams(c)
	var revisionID *uuid.UUID
	if raw := strings.TrimSpace(c.Query("server_revision_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			h.writeError(c, http.StatusBadRequest, "invalid_revision_id", err)
			return
		}
		revisionID = &id
	}
	items, total, err := h.service.ListTools(c.Request.Context(), revisionID, page, size)
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "tool_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func (h *MCPPlatformHandler) ListCatalogs(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListCatalogs(c.Request.Context(), page, size)
	if err != nil {
		h.writeError(c, 500, "catalog_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func (h *MCPPlatformHandler) CreateCatalog(c *gin.Context) {
	var req service.MCPCatalogCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_catalog_request", err)
		return
	}
	item, err := h.service.CreateCatalog(c.Request.Context(), req, authOperator(c))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "catalog_create_failed", err)
		return
	}
	h.successStatus(c, http.StatusCreated, item)
}

func (h *MCPPlatformHandler) GetCatalogSnapshot(c *gin.Context) {
	id, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_catalog_id", err)
		return
	}
	snapshot, err := h.service.BuildCatalogSnapshot(c.Request.Context(), id)
	if err != nil {
		h.notFoundOrError(c, "catalog_snapshot_failed", err)
		return
	}
	h.success(c, snapshot)
}
func (h *MCPPlatformHandler) ListClients(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListClients(c.Request.Context(), page, size)
	if err != nil {
		h.writeError(c, 500, "client_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func (h *MCPPlatformHandler) CreateClient(c *gin.Context) {
	var req service.MCPClientCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_client_request", err)
		return
	}
	item, err := h.service.CreateClient(c.Request.Context(), req, authOperator(c))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "client_create_failed", err)
		return
	}
	h.successStatus(c, http.StatusCreated, item)
}

func (h *MCPPlatformHandler) CreateGrant(c *gin.Context) {
	var req service.MCPGrantCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_grant_request", err)
		return
	}
	item, err := h.service.CreateGrant(c.Request.Context(), req, authOperator(c))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "grant_create_failed", err)
		return
	}
	h.successStatus(c, http.StatusCreated, item)
}

func (h *MCPPlatformHandler) ListClientEndpoints(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListClientEndpoints(c.Request.Context(), h.publicGatewayBaseURL(c), page, size)
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "client_endpoint_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func (h *MCPPlatformHandler) CreateClientEndpoint(c *gin.Context) {
	var req service.MCPClientEndpointCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_client_endpoint_request", err)
		return
	}
	item, err := h.service.CreateClientEndpoint(c.Request.Context(), req, authOperator(c), h.publicGatewayBaseURL(c))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "client_endpoint_create_failed", err)
		return
	}
	h.successStatus(c, http.StatusCreated, item)
}

func (h *MCPPlatformHandler) DeleteClientEndpoint(c *gin.Context) {
	clientID, err := parseMCPID(c.Param("client_id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_client_id", err)
		return
	}
	item, err := h.service.RevokeClientEndpoint(c.Request.Context(), clientID, authOperator(c))
	if err != nil {
		h.notFoundOrError(c, "client_endpoint_delete_failed", err)
		return
	}
	h.success(c, item)
}

type mcpClientEndpointToolsRequest struct {
	ToolAllowlist *[]string `json:"tool_allowlist" binding:"required"`
}

func (h *MCPPlatformHandler) UpdateClientEndpointTools(c *gin.Context) {
	grantID, err := parseMCPID(c.Param("grant_id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_grant_id", err)
		return
	}
	var req mcpClientEndpointToolsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_tool_allowlist", err)
		return
	}
	item, err := h.service.UpdateClientEndpointTools(c.Request.Context(), grantID, *req.ToolAllowlist, authOperator(c), h.publicGatewayBaseURL(c))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrMCPPlatformClientEndpointDenied) {
			status = http.StatusForbidden
		}
		h.writeError(c, status, "client_endpoint_tools_update_failed", err)
		return
	}
	h.success(c, item)
}

func (h *MCPPlatformHandler) RuntimeTools(c *gin.Context) {
	if !h.validRuntimeRequest(c) {
		return
	}
	items, err := h.service.RuntimeTools(c.Request.Context(), runtimeBearer(c), c.GetHeader("X-MCP-Client-Key"))
	if err != nil {
		h.runtimeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tools": items})
}

type mcpRuntimeCallRequest struct {
	ToolAlias string          `json:"tool_alias"`
	Arguments json.RawMessage `json:"arguments"`
}

func (h *MCPPlatformHandler) RuntimeCall(c *gin.Context) {
	if !h.validRuntimeRequest(c) {
		return
	}
	var req mcpRuntimeCallRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ToolAlias) == "" {
		h.runtimeJSONError(c, http.StatusBadRequest, "invalid runtime call")
		return
	}
	result, err := h.service.RuntimeCallAs(c.Request.Context(), runtimeBearer(c), c.GetHeader("X-MCP-Client-Key"), strings.TrimSpace(req.ToolAlias), req.Arguments, h.validRuntimeActor(c))
	if err != nil {
		h.runtimeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

// validRuntimeActor accepts only the Assistant context signed with the same
// secret that authenticates the Gateway-to-api-server hop. Client credentials
// remain the authorization boundary; this context is audit metadata only.
func (h *MCPPlatformHandler) validRuntimeActor(c *gin.Context) string {
	payload := strings.TrimSpace(c.GetHeader("X-Aegis-MCP-Assistant-Context"))
	signature := strings.TrimSpace(c.GetHeader("X-Aegis-MCP-Assistant-Signature"))
	if payload == "" || signature == "" || h.runtimeSecret == "" {
		return ""
	}
	digest := hmac.New(sha256.New, []byte(h.runtimeSecret))
	_, _ = digest.Write([]byte(payload))
	if !hmac.Equal([]byte(strings.ToLower(signature)), []byte(fmtHex(digest.Sum(nil)))) {
		return ""
	}
	var context struct {
		Operator string `json:"operator"`
	}
	if err := json.Unmarshal([]byte(payload), &context); err != nil {
		return ""
	}
	return strings.TrimSpace(context.Operator)
}

func fmtHex(value []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for i, item := range value {
		result[i*2] = hexChars[item>>4]
		result[i*2+1] = hexChars[item&0x0f]
	}
	return string(result)
}

func (h *MCPPlatformHandler) validRuntimeRequest(c *gin.Context) bool {
	if h.runtimeSecret == "" || c.GetHeader("X-Aegis-MCP-Gateway-Secret") != h.runtimeSecret || runtimeBearer(c) == "" {
		h.runtimeJSONError(c, http.StatusUnauthorized, "runtime authentication failed")
		return false
	}
	return true
}

func (h *MCPPlatformHandler) runtimeError(c *gin.Context, err error) {
	status := http.StatusForbidden
	if !errors.Is(err, service.ErrMCPPlatformClientEndpointDenied) && !errors.Is(err, service.ErrMCPPlatformToolNotAllowed) && !errors.Is(err, service.ErrMCPPlatformSecurityBlocked) {
		status = http.StatusBadGateway
	}
	h.runtimeJSONError(c, status, safeHandlerError(err))
}

func (h *MCPPlatformHandler) runtimeJSONError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func runtimeBearer(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func (h *MCPPlatformHandler) publicGatewayBaseURL(c *gin.Context) string {
	return h.publicGatewayURL
}
func (h *MCPPlatformHandler) ListApprovals(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListApprovals(c.Request.Context(), c.Query("status"), page, size)
	if err != nil {
		h.writeError(c, 500, "approval_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

type mcpApprovalDecisionRequest struct {
	ExpectedRequestDigest string `json:"expected_request_digest" binding:"required"`
	Reason                string `json:"reason" binding:"required"`
}

func (h *MCPPlatformHandler) Approve(c *gin.Context) { h.decideApproval(c, modelMCPApprovalApproved) }
func (h *MCPPlatformHandler) Reject(c *gin.Context)  { h.decideApproval(c, modelMCPApprovalRejected) }

const (
	modelMCPApprovalApproved = "approved"
	modelMCPApprovalRejected = "rejected"
)

func (h *MCPPlatformHandler) decideApproval(c *gin.Context, status string) {
	id, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_approval_id", err)
		return
	}
	var req mcpApprovalDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_approval_decision", err)
		return
	}
	item, err := h.service.DecideApproval(c.Request.Context(), id, req.ExpectedRequestDigest, status, req.Reason, authOperator(c), c.GetString("role"))
	if err != nil {
		statusCode := http.StatusConflict
		errorCode := "approval_decision_failed"
		if errors.Is(err, repository.ErrMCPPlatformNotFound) {
			statusCode = http.StatusNotFound
		}
		if errors.Is(err, service.ErrMCPPlatformSelfApproval) {
			statusCode = http.StatusForbidden
			errorCode = "MCP_APPROVAL_SELF_DECISION"
		}
		h.writeError(c, statusCode, errorCode, err)
		return
	}
	h.success(c, item)
}
func (h *MCPPlatformHandler) ListInvocations(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListInvocations(c.Request.Context(), page, size)
	if err != nil {
		h.writeError(c, 500, "invocation_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}
func (h *MCPPlatformHandler) DisableInvocationTool(c *gin.Context) {
	invocationID, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_invocation_id", err)
		return
	}
	item, err := h.service.DisableInvocationTool(c.Request.Context(), invocationID, authOperator(c))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, repository.ErrMCPPlatformNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrMCPPlatformClientEndpointDenied) {
			status = http.StatusForbidden
		}
		h.writeError(c, status, "invocation_tool_disable_failed", err)
		return
	}
	h.success(c, item)
}
func (h *MCPPlatformHandler) ListSecurityVerdicts(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListSecurityVerdicts(c.Request.Context(), page, size)
	if err != nil {
		h.writeError(c, 500, "security_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}
func (h *MCPPlatformHandler) ListSecurityRules(c *gin.Context) {
	page, size := mcpPageParams(c)
	items, total, err := h.service.ListSecurityRules(c.Request.Context(), page, size)
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "security_rule_list_failed", err)
		return
	}
	h.success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

type mcpSecurityRuleEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

func (h *MCPPlatformHandler) SetSecurityRuleEnabled(c *gin.Context) {
	ruleID, err := parseMCPID(c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_security_rule_id", err)
		return
	}
	var req mcpSecurityRuleEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.writeError(c, http.StatusBadRequest, "invalid_security_rule_state", err)
		return
	}
	item, err := h.service.SetSecurityRuleEnabled(c.Request.Context(), ruleID, *req.Enabled, authOperator(c))
	if err != nil {
		h.notFoundOrError(c, "security_rule_update_failed", err)
		return
	}
	h.success(c, item)
}

func (h *MCPPlatformHandler) success(c *gin.Context, data interface{}) {
	h.successStatus(c, http.StatusOK, data)
}
func (h *MCPPlatformHandler) successStatus(c *gin.Context, status int, data interface{}) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"code": 0, "message": "success", "data": data})
}
func (h *MCPPlatformHandler) writeError(c *gin.Context, status int, code string, err error) {
	h.logger.Warn("mcp_platform_request_failed", zap.String("code", code), zap.Error(err))
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"code": code, "error_code": code, "message": safeHandlerError(err), "request_id": c.GetString("request_id")})
}

func (h *MCPPlatformHandler) notFoundOrError(c *gin.Context, code string, err error) {
	if errors.Is(err, repository.ErrMCPPlatformNotFound) {
		h.writeError(c, http.StatusNotFound, code, err)
		return
	}
	h.writeError(c, http.StatusInternalServerError, code, err)
}
func parseMCPID(raw string) (uuid.UUID, error) { return uuid.Parse(strings.TrimSpace(raw)) }
func mcpPageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
func authOperator(c *gin.Context) string {
	if value, ok := c.Get("auth_username"); ok {
		if username, ok := value.(string); ok && strings.TrimSpace(username) != "" {
			return username
		}
	}
	return "unknown"
}
func safeHandlerError(err error) string {
	if err == nil {
		return "request failed"
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return msg
}

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"api-server/internal/llm"
	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

type DetectionPackageHandler struct {
	pkgService *service.DetectionPackageService
	llmClient  LLMCaller
}

type LLMCaller interface {
	ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error)
}

func NewDetectionPackageHandler(pkgService *service.DetectionPackageService, llmClient LLMCaller) *DetectionPackageHandler {
	return &DetectionPackageHandler{pkgService: pkgService, llmClient: llmClient}
}

func (h *DetectionPackageHandler) ListPackages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	search := c.Query("search")

	packages, total, err := h.pkgService.ListPackages(c.Request.Context(), page, pageSize, status, search)
	if err != nil {
		logger.Error("list packages failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"data": packages, "total": total}})
}

func (h *DetectionPackageHandler) GetPackage(c *gin.Context) {
	packageID := c.Param("package_id")
	pkg, err := h.pkgService.GetPackage(c.Request.Context(), packageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "package not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": pkg})
}

func (h *DetectionPackageHandler) CreateDraft(c *gin.Context) {
	var req service.CreateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	operator := getOperator(c)
	draft, err := h.pkgService.CreateDraft(c.Request.Context(), req, operator)
	if err != nil {
		logger.Error("create draft failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": draft})
}

func (h *DetectionPackageHandler) AIGenerateDraft(c *gin.Context) {
	var req struct {
		CVEID                      string `json:"cve_id" binding:"required"`
		VulnerabilityDescription   string `json:"vulnerability_description" binding:"required"`
		AttackPrerequisites        string `json:"attack_prerequisites"`
		ExploitationChain          string `json:"exploitation_chain"`
		FalsePositiveConstraints   string `json:"false_positive_constraints"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// Generate package_id from CVE ID
	packageID := strings.ToLower(strings.ReplaceAll(req.CVEID, "-", "_"))
	packageID = strings.ReplaceAll(packageID, "cve_", "cve-")

	// Call LLM to generate detection package content
	hookPlanYAML := "# AI generated HookPlan - to be implemented"
	ebpfSource := "// AI generated eBPF source - to be implemented"
	sigmaRulesYAML := "# AI generated Sigma rules - to be implemented"
	correlationYAML := "# AI generated Correlation - to be implemented"

	if h.llmClient != nil {
		prompt := llm.GetDetectionPackageGenerationPrompt(
			req.CVEID,
			req.VulnerabilityDescription,
			req.ExploitationChain,
			req.FalsePositiveConstraints,
		)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 120)
		defer cancel()
		response, err := h.llmClient.ChatCompletion(ctx, "", prompt, 0.7)
		if err != nil {
			logger.Warn("LLM generation failed, using placeholders", zap.Error(err))
		} else {
			// Parse the LLM response to extract sections
			hookPlanYAML, ebpfSource, sigmaRulesYAML, correlationYAML = parseLLMResponse(response)
		}
	}

	// Create draft with AI generation input
	draftReq := service.CreateDraftRequest{
		PackageID:       packageID,
		TargetVersion:   "1.0.0",
		Title:           fmt.Sprintf("%s Runtime Detector", req.CVEID),
		Description:     req.VulnerabilityDescription,
		CVEIDs:          []string{req.CVEID},
		HookPlanYAML:    hookPlanYAML,
		EBPFSource:      ebpfSource,
		SigmaRulesYAML:  sigmaRulesYAML,
		CorrelationYAML: correlationYAML,
	}

	operator := getOperator(c)
	draft, err := h.pkgService.CreateDraft(c.Request.Context(), draftReq, operator)
	if err != nil {
		logger.Error("ai generate draft failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": draft})
}

// parseLLMResponse extracts the four sections from the LLM response
func parseLLMResponse(response string) (hookPlan, ebpfSource, sigmaRules, correlation string) {
	hookPlan = extractCodeBlock(response, "yaml", "HookPlan")
	ebpfSource = extractCodeBlock(response, "c", "eBPF Source")
	sigmaRules = extractCodeBlock(response, "yaml", "Sigma")
	correlation = extractCodeBlock(response, "yaml", "Correlation")

	// Fallback to defaults if parsing fails
	if hookPlan == "" {
		hookPlan = "# AI generated HookPlan - parsing failed"
	}
	if ebpfSource == "" {
		ebpfSource = "// AI generated eBPF source - parsing failed"
	}
	if sigmaRules == "" {
		sigmaRules = "# AI generated Sigma rules - parsing failed"
	}
	if correlation == "" {
		correlation = "# AI generated Correlation - parsing failed"
	}
	return
}

func extractCodeBlock(response, lang, sectionHint string) string {
	// Find section header containing the hint
	lines := strings.Split(response, "\n")
	inSection := false
	var blockLines []string
	inBlock := false

	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, strings.ToLower(sectionHint)) && strings.HasPrefix(strings.TrimSpace(line), "#") {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inBlock {
				break // End of block
			}
			inBlock = true
			continue
		}
		if inSection && inBlock {
			blockLines = append(blockLines, line)
		}
		// If we hit another section header while looking, stop
		if inSection && !inBlock && strings.HasPrefix(strings.TrimSpace(line), "## ") && !strings.Contains(lower, strings.ToLower(sectionHint)) {
			inSection = false
		}
	}

	if len(blockLines) > 0 {
		return strings.Join(blockLines, "\n")
	}
	return ""
}

func (h *DetectionPackageHandler) UpdateDraft(c *gin.Context) {
	draftID, err := uuid.Parse(c.Param("draft_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid draft_id"})
		return
	}

	var req service.UpdateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	operator := getOperator(c)
	draft, err := h.pkgService.UpdateDraft(c.Request.Context(), draftID, req, operator)
	if err != nil {
		logger.Error("update draft failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": draft})
}

func (h *DetectionPackageHandler) GetDraft(c *gin.Context) {
	packageID := c.Param("package_id")
	draft, err := h.pkgService.GetDraft(c.Request.Context(), packageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "draft not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": draft})
}

func (h *DetectionPackageHandler) StartBuild(c *gin.Context) {
	packageID := c.Param("package_id")
	operator := getOperator(c)

	build, err := h.pkgService.StartBuild(c.Request.Context(), packageID, operator)
	if err != nil {
		logger.Error("start build failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": build})
}

func (h *DetectionPackageHandler) GetBuild(c *gin.Context) {
	buildID, err := uuid.Parse(c.Param("build_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid build_id"})
		return
	}

	build, err := h.pkgService.GetBuild(c.Request.Context(), buildID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "build not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": build})
}

func (h *DetectionPackageHandler) SignPackage(c *gin.Context) {
	packageID := c.Param("package_id")
	operator := getOperator(c)

	pkg, err := h.pkgService.SignPackage(c.Request.Context(), packageID, operator)
	if err != nil {
		logger.Error("sign package failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": pkg})
}

func (h *DetectionPackageHandler) EnablePackage(c *gin.Context) {
	packageID := c.Param("package_id")
	operator := getOperator(c)

	if err := h.pkgService.EnablePackage(c.Request.Context(), packageID, operator); err != nil {
		logger.Error("enable package failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionPackageHandler) DisablePackage(c *gin.Context) {
	packageID := c.Param("package_id")
	operator := getOperator(c)

	if err := h.pkgService.DisablePackage(c.Request.Context(), packageID, operator); err != nil {
		logger.Error("disable package failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionPackageHandler) UninstallPackage(c *gin.Context) {
	packageID := c.Param("package_id")
	operator := getOperator(c)

	if err := h.pkgService.UninstallPackage(c.Request.Context(), packageID, operator); err != nil {
		logger.Error("uninstall package failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionPackageHandler) ListHostStatus(c *gin.Context) {
	packageID := c.Param("package_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	statuses, total, err := h.pkgService.ListHostStatus(c.Request.Context(), packageID, "", page, pageSize)
	if err != nil {
		logger.Error("list host status failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"data": statuses, "total": total}})
}

func (h *DetectionPackageHandler) ReportHostStatus(c *gin.Context) {
	var report service.HostStatusReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	if err := h.pkgService.ReportHostStatus(c.Request.Context(), report); err != nil {
		logger.Error("report host status failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

func (h *DetectionPackageHandler) GetAllowlist(c *gin.Context) {
	config, err := h.pkgService.GetAllowlist(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "allowlist not found"})
		return
	}

	// Parse config_json to return in frontend expected format
	var configData map[string][]string
	if config.ConfigJSON != nil {
		json.Unmarshal(config.ConfigJSON, &configData)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"version":     config.Version,
			"tracepoints": configData["tracepoints"],
			"kprobes":     configData["kprobes"],
			"lsm":         configData["lsm"],
			"xdp":         configData["xdp"],
			"tc":          configData["tc"],
		},
	})
}

func (h *DetectionPackageHandler) UpdateAllowlist(c *gin.Context) {
	// Accept both formats: {"config": {...}} or direct {"tracepoints": [...], ...}
	var rawReq map[string]interface{}
	if err := c.ShouldBindJSON(&rawReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// Extract config from wrapper or use directly
	var configData map[string]interface{}
	if config, ok := rawReq["config"]; ok {
		if configMap, ok := config.(map[string]interface{}); ok {
			configData = configMap
		}
	}
	if configData == nil {
		// Use the request body directly as config
		delete(rawReq, "description")
		configData = rawReq
	}

	configBytes, _ := json.Marshal(configData)
	description, _ := rawReq["description"].(string)
	operator := getOperator(c)
	config, err := h.pkgService.UpdateAllowlist(c.Request.Context(), datatypes.JSON(configBytes), description, operator)
	if err != nil {
		logger.Error("update allowlist failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	// Parse config_json to return in frontend expected format
	var resultConfig map[string][]string
	if config.ConfigJSON != nil {
		json.Unmarshal(config.ConfigJSON, &resultConfig)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"version":     config.Version,
			"tracepoints": resultConfig["tracepoints"],
			"kprobes":     resultConfig["kprobes"],
			"lsm":         resultConfig["lsm"],
			"xdp":         resultConfig["xdp"],
			"tc":          resultConfig["tc"],
		},
	})
}

func getOperator(c *gin.Context) string {
	if username, exists := c.Get("username"); exists {
		return username.(string)
	}
	return "unknown"
}

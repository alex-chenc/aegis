package api

import (
	"api-server/internal/api/handler"
	"api-server/internal/api/middleware"

	"github.com/gin-gonic/gin"
)

type Router struct {
	engine                 *gin.Engine
	configHandler          *handler.ConfigHandler
	hostHandler            *handler.HostHandler
	templateHandler        *handler.TemplateHandler
	taskHandler            *handler.TaskHandler
	taskHandlerWithHealing *handler.TaskHandlerWithHealing
	agentHandler           *handler.AgentHandler
	ruleHandler            *handler.RuleHandler
	vulnerabilityHandler   *handler.VulnerabilityHandler
	detectionHandler       *handler.DetectionHandler
	websocketHandler       *handler.WebSocketHandler
}

func NewRouter(
	configHandler *handler.ConfigHandler,
	hostHandler *handler.HostHandler,
	templateHandler *handler.TemplateHandler,
	taskHandler *handler.TaskHandler,
	taskHandlerWithHealing *handler.TaskHandlerWithHealing,
	agentHandler *handler.AgentHandler,
	ruleHandler *handler.RuleHandler,
	vulnerabilityHandler *handler.VulnerabilityHandler,
	detectionHandler *handler.DetectionHandler,
	websocketHandler *handler.WebSocketHandler,
) *Router {
	return &Router{
		configHandler:          configHandler,
		hostHandler:            hostHandler,
		templateHandler:        templateHandler,
		taskHandler:            taskHandler,
		taskHandlerWithHealing: taskHandlerWithHealing,
		agentHandler:           agentHandler,
		ruleHandler:            ruleHandler,
		vulnerabilityHandler:   vulnerabilityHandler,
		detectionHandler:       detectionHandler,
		websocketHandler:       websocketHandler,
	}
}

// Setup 设置路由和中间件
func (r *Router) Setup() {
	r.engine = gin.Default()

	// 全局中间件
	r.engine.Use(middleware.CORS())
	r.engine.Use(middleware.RequestLogger())
	r.engine.Use(middleware.Recovery())

	// 健康检查端点
	r.engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// API v1 路由组
	v1 := r.engine.Group("/api/v1")
	{
		// 配置接口
		config := v1.Group("/config")
		{
			config.GET("/llm", r.configHandler.GetLLMConfig)
			config.POST("/llm", r.configHandler.SaveLLMConfig)
			config.POST("/llm/test", r.configHandler.TestLLMConnection)
			config.GET("/llm/full-key", r.configHandler.GetFullAPIKey)
		}

		// 主机接口
		hosts := v1.Group("/hosts")
		{
			hosts.GET("", r.hostHandler.ListHosts)
			hosts.GET("/:id", r.hostHandler.GetHost)
		}

		// 模板接口
		templates := v1.Group("/templates")
		{
			templates.POST("/upload", r.templateHandler.UploadTemplate)
			templates.GET("", r.templateHandler.ListTemplates)
			templates.GET("/check-md5", r.templateHandler.CheckMD5)
			templates.GET("/:id/status", r.templateHandler.GetTemplateStatus)
			templates.GET("/:id/rules", r.templateHandler.GetTemplateRules)
			templates.POST("/:id/generate-scripts", r.templateHandler.BatchGenerateScripts)
			templates.DELETE("/:id", r.templateHandler.DeleteTemplate)
		}

		// 规则接口
		rules := v1.Group("/rules")
		{
			rules.GET("/:id", r.ruleHandler.GetScript)
			rules.GET("/:id/has-tasks", r.ruleHandler.HasTasks)
			rules.POST("/:id/scripts/generate", r.ruleHandler.GenerateScript)
			rules.PUT("/:id/scripts", r.ruleHandler.UpdateScript)
			rules.DELETE("/:id", r.ruleHandler.DeleteRule)
		}

		// 任务接口
		tasks := v1.Group("/tasks")
		{
			tasks.GET("", r.taskHandler.ListTasks)
			tasks.POST("/run-check", r.taskHandler.RunCheck)
			tasks.POST("/run-fix", r.taskHandler.RunFix)
			tasks.GET("/:id/status", r.taskHandler.GetTaskStatus)
			tasks.GET("/:id/logs", r.taskHandler.GetTaskLogs)
			tasks.GET("/:id", r.taskHandler.GetTaskDetail)
			tasks.POST("/:id/redispatch", r.taskHandler.RedispatchTask)
			tasks.DELETE("/:id", r.taskHandler.DeleteSingleTask)
			tasks.DELETE("/group/:id", r.taskHandler.DeleteTask)
			tasks.DELETE("/batch", r.taskHandler.BatchDeleteTasks)
			tasks.POST("/:id/heal", r.taskHandlerWithHealing.TriggerSelfHealing)
			tasks.GET("/:id/healing-status", r.taskHandlerWithHealing.GetHealingStatus)
		}

		// Agent 接口
		agent := v1.Group("/agent")
		{
			agent.GET("/install-command", r.agentHandler.GetInstallCommand)
			agent.GET("/install.sh", r.agentHandler.GetInstallScript)
			agent.GET("/uninstall.sh", r.agentHandler.GetUninstallScript)
			agent.GET("/download", r.agentHandler.DownloadAgent)
		}

		// 漏洞管理接口
		vulnerability := v1.Group("/vulnerability")
		{
			vulnerability.POST("/scan", r.vulnerabilityHandler.StartScan)
			vulnerability.POST("/scan/stop", r.vulnerabilityHandler.StopScan)
			vulnerability.GET("/scan/:scan_id/status", r.vulnerabilityHandler.GetScanStatus)
			vulnerability.GET("", r.vulnerabilityHandler.ListVulnerabilities)
			vulnerability.GET("/:cve_id/affected-hosts", r.vulnerabilityHandler.GetAffectedHosts)
			vulnerability.POST("/custom-query", r.vulnerabilityHandler.StartCustomQuery)
			vulnerability.GET("/custom-query/:query_id/status", r.vulnerabilityHandler.GetCustomQueryStatus)
			vulnerability.GET("/custom-query/current", r.vulnerabilityHandler.GetCurrentQuery)
			vulnerability.POST("/:cve_id/scripts/generate", r.vulnerabilityHandler.GenerateHostScripts)
			vulnerability.GET("/:cve_id/host-scripts", r.vulnerabilityHandler.GetHostScriptsStatus)
			vulnerability.POST("/:cve_id/scripts/execute", r.vulnerabilityHandler.ExecuteScripts)
			vulnerability.POST("/:cve_id/fix", r.vulnerabilityHandler.InitiateFix)
			vulnerability.POST("/:cve_id/poc", r.vulnerabilityHandler.InitiatePoc)
			vulnerability.GET("/:cve_id/generation-status", r.vulnerabilityHandler.GetGenerationStatus)
			vulnerability.GET("/:cve_id/task-status", r.vulnerabilityHandler.GetCveTaskStatus)
			vulnerability.GET("/scripts/:script_id/status", r.vulnerabilityHandler.GetScriptStatus)
		}

		detection := v1.Group("/detection")
		{
			detection.GET("/alerts", r.detectionHandler.ListAlerts)
			detection.GET("/alerts/:id", r.detectionHandler.GetAlert)
			detection.GET("/alerts/:id/process-tree", r.detectionHandler.GetProcessTree)
			detection.POST("/alerts/:id/resolve", r.detectionHandler.ResolveAlert)
			detection.POST("/alerts/:id/block", r.detectionHandler.BlockAlert)
			detection.DELETE("/alerts", r.detectionHandler.DeleteAlerts)

			detection.GET("/blocks", r.detectionHandler.ListBlockRecords)

			detection.GET("/block-policies", r.detectionHandler.ListBlockPolicies)
			detection.POST("/block-policies/sync", r.detectionHandler.SyncBlockPolicies)
			detection.POST("/block-policies/normalize", r.detectionHandler.NormalizeMitreIDs)
			detection.PUT("/block-policies/:mitre_id", r.detectionHandler.UpdateBlockPolicy)

			detection.GET("/attack-matrix", r.detectionHandler.GetAttackMatrix)

			detection.POST("/llm/aggregate", r.detectionHandler.StartLLMAggregation)
			detection.GET("/llm/aggregate/:id", r.detectionHandler.GetLLMAggregationStatus)

			detectionRules := detection.Group("/rules")
			{
				detectionRules.POST("/upload", r.detectionHandler.UploadRules)
				detectionRules.POST("/import", r.detectionHandler.ImportRules)
				detectionRules.POST("/generate", r.detectionHandler.GenerateSigmaRule)
				detectionRules.POST("/check-delete", r.detectionHandler.CheckRulesBeforeDelete)
				detectionRules.DELETE("", r.detectionHandler.DeleteRules)
				detectionRules.GET("", r.detectionHandler.ListRules)

				// AI规则配置 - 使用 /ai-rule-config 路径避免与 /:id 冲突
				detectionRules.GET("/ai-rule-config", r.detectionHandler.GetAIConfig)
				detectionRules.PUT("/ai-rule-config", r.detectionHandler.UpdateAIConfig)
				detectionRules.POST("/generate-test", r.detectionHandler.GenerateTestRule)

				detectionRules.GET("/:id", r.detectionHandler.GetRule)
				detectionRules.PUT("/:id/status", r.detectionHandler.UpdateRuleStatus)
			}

			detection.GET("/tool-calls", r.detectionHandler.ListToolCalls)

			detection.GET("/statistics/threats", r.detectionHandler.GetThreatStatistics)
			detection.GET("/statistics/alert-trend", r.detectionHandler.GetAlertTrend)

			detection.GET("/runtime/ws", r.websocketHandler.HandleConnection)
		}
	}
}

// GetEngine 返回 Gin 引擎
func (r *Router) GetEngine() *gin.Engine {
	return r.engine
}

// Run 启动 HTTP 服务器
func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}

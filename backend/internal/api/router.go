package api

import (
	"aegis-system/internal/api/handler"
	"aegis-system/internal/api/middleware"
	"aegis-system/internal/grpc_server"

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
}

func NewRouter(
	configHandler *handler.ConfigHandler,
	hostHandler *handler.HostHandler,
	templateHandler *handler.TemplateHandler,
	taskHandler *handler.TaskHandler,
	taskHandlerWithHealing *handler.TaskHandlerWithHealing,
	agentHandler *handler.AgentHandler,
	ruleHandler *handler.RuleHandler,
) *Router {
	return &Router{
		configHandler:          configHandler,
		hostHandler:            hostHandler,
		templateHandler:        templateHandler,
		taskHandler:            taskHandler,
		taskHandlerWithHealing: taskHandlerWithHealing,
		agentHandler:           agentHandler,
		ruleHandler:            ruleHandler,
	}
}

// Setup 设置路由和中间件
func (r *Router) Setup(grpcServer *grpc_server.GRPCServer) {
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
			agent.GET("/download", r.agentHandler.DownloadAgent)
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

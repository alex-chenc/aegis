package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"api-server/config"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Server struct {
	cfg          *config.Config
	logger       *zap.Logger
	db           *gorm.DB
	redisClient  *redis.Client
	minioClient  interface{}
	grpcServer   *grpc.Server
	httpEngine   *gin.Engine
	grpcListener net.Listener
	httpListener net.Listener
}

func NewServer(cfg *config.Config, logger *zap.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
	}
}

func (s *Server) Start() error {
	s.logger.Info("Initializing API Server...")

	var err error
	s.db, err = s.initDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	s.logger.Info("Database connected")

	s.redisClient, err = s.initRedis()
	if err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}
	s.logger.Info("Redis connected")

	s.minioClient, err = s.initMinIO()
	if err != nil {
		s.logger.Warn("MinIO initialization failed", zap.Error(err))
	}
	s.logger.Info("MinIO connected")

	s.grpcServer = grpc.NewServer()

	grpcAddr := fmt.Sprintf(":%d", s.cfg.Server.GRPCPort)
	s.grpcListener, err = net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port %d: %w", s.cfg.Server.GRPCPort, err)
	}
	s.logger.Info("gRPC listener started", zap.String("address", grpcAddr))

	go func() {
		s.logger.Info("Starting gRPC server", zap.String("address", grpcAddr))
		if err := s.grpcServer.Serve(s.grpcListener); err != nil {
			s.logger.Error("gRPC server failed", zap.Error(err))
		}
	}()

	s.httpEngine = gin.New()
	s.httpEngine.Use(gin.Recovery())
	s.httpEngine.Use(corsMiddleware())
	s.httpEngine.Use(s.requestLoggerMiddleware())

	s.setupRoutes()

	httpAddr := fmt.Sprintf(":%d", s.cfg.Server.HTTPPort)
	s.httpListener, err = net.Listen("tcp", httpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on HTTP port %d: %w", s.cfg.Server.HTTPPort, err)
	}
	s.logger.Info("HTTP listener started", zap.String("address", httpAddr))

	go func() {
		s.logger.Info("Starting HTTP server", zap.String("address", httpAddr))
		if err := s.httpEngine.RunListener(s.httpListener); err != nil {
			s.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()

	s.waitForShutdown()
	return nil
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func (s *Server) requestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.logger.Info("HTTP request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("ip", c.ClientIP()),
		)
		c.Next()
	}
}

func (s *Server) setupRoutes() {
	// Health check
	s.httpEngine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// API v1 routes
	v1 := s.httpEngine.Group("/api/v1")
	{
		// Config endpoints
		config := v1.Group("/config")
		{
			config.GET("/llm", s.handleGetLLMConfig)
			config.POST("/llm", s.handleSaveLLMConfig)
			config.POST("/llm/test", s.handleTestLLMConnection)
		}

		// Host endpoints
		hosts := v1.Group("/hosts")
		{
			hosts.GET("", s.handleListHosts)
			hosts.GET("/:id", s.handleGetHost)
		}

		// Template endpoints
		templates := v1.Group("/templates")
		{
			templates.POST("/upload", s.handleUploadTemplate)
			templates.GET("", s.handleListTemplates)
			templates.GET("/check-md5", s.handleCheckMD5)
			templates.GET("/:id/status", s.handleGetTemplateStatus)
			templates.GET("/:id/rules", s.handleGetTemplateRules)
			templates.POST("/:id/generate-scripts", s.handleBatchGenerateScripts)
			templates.DELETE("/:id", s.handleDeleteTemplate)
		}

		// Rules endpoints
		rules := v1.Group("/rules")
		{
			rules.GET("/:id", s.handleGetScript)
			rules.GET("/:id/has-tasks", s.handleHasTasks)
			rules.POST("/:id/scripts/generate", s.handleGenerateScript)
			rules.PUT("/:id/scripts", s.handleUpdateScript)
			rules.DELETE("/:id", s.handleDeleteRule)
		}

		// Task endpoints
		tasks := v1.Group("/tasks")
		{
			tasks.GET("", s.handleListTasks)
			tasks.POST("/run-check", s.handleRunCheck)
			tasks.POST("/run-fix", s.handleRunFix)
			tasks.GET("/:id/status", s.handleGetTaskStatus)
			tasks.GET("/:id/logs", s.handleGetTaskLogs)
			tasks.GET("/:id", s.handleGetTaskDetail)
			tasks.POST("/:id/redispatch", s.handleRedispatchTask)
			tasks.DELETE("/:id", s.handleDeleteSingleTask)
			tasks.DELETE("/group/:id", s.handleDeleteTask)
			tasks.DELETE("/batch", s.handleBatchDeleteTasks)
			tasks.POST("/:id/heal", s.handleTriggerSelfHealing)
			tasks.GET("/:id/healing-status", s.handleGetHealingStatus)
		}

		// Agent endpoints
		agent := v1.Group("/agent")
		{
			agent.GET("/install-command", s.handleGetInstallCommand)
			agent.GET("/install.sh", s.handleGetInstallScript)
			agent.GET("/uninstall.sh", s.handleGetUninstallScript)
			agent.GET("/download", s.handleDownloadAgent)
		}

		// Vulnerability endpoints
		vulnerability := v1.Group("/vulnerability")
		{
			vulnerability.POST("/scan", s.handleStartScan)
			vulnerability.GET("/scan/:scan_id/status", s.handleGetScanStatus)
			vulnerability.GET("", s.handleListVulnerabilities)
			vulnerability.GET("/:cve_id/affected-hosts", s.handleGetAffectedHosts)
			vulnerability.POST("/custom-query", s.handleStartCustomQuery)
			vulnerability.GET("/custom-query/:query_id/status", s.handleGetCustomQueryStatus)
			vulnerability.GET("/custom-query/current", s.handleGetCurrentQuery)
			vulnerability.POST("/:cve_id/scripts/generate", s.handleGenerateHostScripts)
			vulnerability.GET("/:cve_id/host-scripts", s.handleGetHostScriptsStatus)
			vulnerability.POST("/:cve_id/scripts/execute", s.handleExecuteScripts)
			vulnerability.POST("/:cve_id/fix", s.handleInitiateFix)
			vulnerability.POST("/:cve_id/poc", s.handleInitiatePoc)
			vulnerability.GET("/:cve_id/generation-status", s.handleGetGenerationStatus)
			vulnerability.GET("/:cve_id/task-status", s.handleGetCveTaskStatus)
			vulnerability.GET("/scripts/:script_id/status", s.handleGetScriptStatus)
		}

		// Detection endpoints
		detection := v1.Group("/detection")
		{
			detection.GET("/alerts", s.handleListAlerts)
			detection.GET("/alerts/:id", s.handleGetAlert)
			detection.GET("/alerts/:id/process-tree", s.handleGetProcessTree)
			detection.POST("/alerts/:id/resolve", s.handleResolveAlert)
			detection.POST("/alerts/:id/block", s.handleBlockAlert)
			detection.DELETE("/alerts", s.handleDeleteAlerts)

			detection.GET("/blocks", s.handleListBlockRecords)

			detection.GET("/block-policies", s.handleListBlockPolicies)
			detection.POST("/block-policies/sync", s.handleSyncBlockPolicies)
			detection.POST("/block-policies/normalize", s.handleNormalizeMitreIDs)
			detection.PUT("/block-policies/:mitre_id", s.handleUpdateBlockPolicy)

			detection.GET("/attack-matrix", s.handleGetAttackMatrix)

			detection.POST("/llm/aggregate", s.handleStartLLMAggregation)
			detection.GET("/llm/aggregate/:id", s.handleGetLLMAggregationStatus)

			detectionRules := detection.Group("/rules")
			{
				detectionRules.POST("/import", s.handleImportRules)
				detectionRules.POST("/generate", s.handleGenerateSigmaRule)
				detectionRules.POST("/check-delete", s.handleCheckRulesBeforeDelete)
				detectionRules.DELETE("", s.handleDeleteRules)
				detectionRules.GET("", s.handleListRules)
				detectionRules.GET("/:id", s.handleGetRule)
				detectionRules.PUT("/:id/status", s.handleUpdateRuleStatus)
			}

			detection.GET("/tool-calls", s.handleListToolCalls)

			detection.GET("/statistics/threats", s.handleGetThreatStatistics)
			detection.GET("/statistics/alert-trend", s.handleGetAlertTrend)

			detection.GET("/runtime/ws", s.handleRuntimeWS)
		}
	}
}

// Handler functions - Placeholder implementations

func (s *Server) handleGetLLMConfig(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"api_key_masked": "****",
			"base_url":       "https://api.openai.com/v1",
			"model_name":     "gpt-4",
			"is_active":      true,
		},
	})
}

func (s *Server) handleSaveLLMConfig(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "config saved successfully",
	})
}

func (s *Server) handleTestLLMConnection(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "connection successful",
		"data": gin.H{
			"status": "ok",
		},
	})
}

func (s *Server) handleListHosts(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleGetHost(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    nil,
	})
}

func (s *Server) handleUploadTemplate(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "template uploaded successfully",
	})
}

func (s *Server) handleCheckMD5(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"exists": false,
		},
	})
}

func (s *Server) handleListTemplates(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleGetTemplateStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"status": "completed",
		},
	})
}

func (s *Server) handleGetTemplateRules(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleBatchGenerateScripts(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task_id": "batch-task-001",
		},
	})
}

func (s *Server) handleDeleteTemplate(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleGetScript(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    nil,
	})
}

func (s *Server) handleHasTasks(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"has_tasks": false,
		},
	})
}

func (s *Server) handleGenerateScript(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleUpdateScript(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleDeleteRule(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleListTasks(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleRunCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "check started",
		"data": gin.H{
			"task_id": "task-001",
		},
	})
}

func (s *Server) handleRunFix(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "fix started",
		"data": gin.H{
			"task_id": "task-002",
		},
	})
}

func (s *Server) handleGetTaskStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"status": "running",
		},
	})
}

func (s *Server) handleGetTaskLogs(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"logs": "",
		},
	})
}

func (s *Server) handleGetTaskDetail(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    nil,
	})
}

func (s *Server) handleRedispatchTask(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleDeleteSingleTask(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleDeleteTask(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleBatchDeleteTasks(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleTriggerSelfHealing(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "self-healing triggered",
	})
}

func (s *Server) handleGetHealingStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"status": "not_needed",
		},
	})
}

func (s *Server) handleGetInstallCommand(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"command": "curl -sSL http://<SERVER_IP>:19090/api/v1/agent/install.sh | sudo bash",
		},
	})
}

func (s *Server) handleGetInstallScript(c *gin.Context) {
	c.Header("Content-Type", "text/plain")
	c.String(200, "#!/bin/bash\necho 'Install script placeholder'\n")
}

func (s *Server) handleGetUninstallScript(c *gin.Context) {
	c.Header("Content-Type", "text/plain")
	c.String(200, "#!/bin/bash\necho 'Uninstall script placeholder'\n")
}

func (s *Server) handleDownloadAgent(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

// Vulnerability handlers
func (s *Server) handleStartScan(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "scan started",
		"data": gin.H{
			"scan_id": "scan-001",
		},
	})
}

func (s *Server) handleGetScanStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"scan_id":        c.Param("scan_id"),
			"status":         "completed",
			"progress":       100,
			"found_vulns":    0,
			"scanned_hosts":  0,
			"total_hosts":    0,
		},
	})
}

func (s *Server) handleListVulnerabilities(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleGetAffectedHosts(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleStartCustomQuery(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "custom query started",
		"data": gin.H{
			"query_id": "query-001",
		},
	})
}

func (s *Server) handleGetCustomQueryStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"query_id": c.Param("query_id"),
			"status":   "completed",
		},
	})
}

func (s *Server) handleGetCurrentQuery(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    nil,
	})
}

func (s *Server) handleGenerateHostScripts(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "scripts generation started",
	})
}

func (s *Server) handleGetHostScriptsStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    gin.H{},
	})
}

func (s *Server) handleExecuteScripts(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "scripts execution started",
	})
}

func (s *Server) handleInitiateFix(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "fix initiated",
	})
}

func (s *Server) handleInitiatePoc(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "poc initiated",
	})
}

func (s *Server) handleGetGenerationStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    gin.H{},
	})
}

func (s *Server) handleGetCveTaskStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"poc": gin.H{"has_running": false},
			"fix": gin.H{"has_running": false},
		},
	})
}

func (s *Server) handleGetScriptStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"script_id": c.Param("script_id"),
			"status":    "completed",
		},
	})
}

// Detection handlers
func (s *Server) handleListAlerts(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"data":  []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleGetAlert(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    nil,
	})
}

func (s *Server) handleGetProcessTree(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"process_tree": "",
		},
	})
}

func (s *Server) handleResolveAlert(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleBlockAlert(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    gin.H{},
	})
}

func (s *Server) handleDeleteAlerts(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"deleted_count": 0,
		},
	})
}

func (s *Server) handleListBlockRecords(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"data":  []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleListBlockPolicies(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"data":       []interface{}{},
			"total":      0,
			"page":       1,
			"page_size":  10,
			"total_page": 0,
		},
	})
}

func (s *Server) handleSyncBlockPolicies(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"created":     0,
			"total_rules": 0,
		},
	})
}

func (s *Server) handleNormalizeMitreIDs(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"rules_updated":    0,
			"policies_updated": 0,
			"alerts_updated":   0,
		},
	})
}

func (s *Server) handleUpdateBlockPolicy(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleGetAttackMatrix(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"tactics": []interface{}{},
		},
	})
}

func (s *Server) handleStartLLMAggregation(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"aggregation_id": "agg-001",
			"status":        "processing",
		},
	})
}

func (s *Server) handleGetLLMAggregationStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"aggregation_id": c.Param("id"),
			"status":         "completed",
		},
	})
}

func (s *Server) handleImportRules(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"total":    0,
			"imported": 0,
			"skipped":  0,
		},
	})
}

func (s *Server) handleGenerateSigmaRule(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    gin.H{},
	})
}

func (s *Server) handleCheckRulesBeforeDelete(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"has_alerts":        false,
			"rules_with_alerts": []interface{}{},
			"total_alerts":      0,
		},
	})
}

func (s *Server) handleDeleteRules(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"deleted_rules":    0,
			"deleted_alerts":   0,
			"deleted_policies": 0,
		},
	})
}

func (s *Server) handleListRules(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"data":  []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleGetRule(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    nil,
	})
}

func (s *Server) handleUpdateRuleStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (s *Server) handleListToolCalls(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"data":  []interface{}{},
			"total": 0,
		},
	})
}

func (s *Server) handleGetThreatStatistics(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"today_alerts":   0,
			"today_blocks":   0,
			"affected_hosts": 0,
			"active_rules":   0,
		},
	})
}

func (s *Server) handleGetAlertTrend(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": []interface{}{},
	})
}

func (s *Server) handleRuntimeWS(c *gin.Context) {
	c.JSON(200, gin.H{
		"code":    0,
		"message": "websocket endpoint",
	})
}

func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Shutting down API Server...")

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.logger.Info("gRPC server stopped")
	}

	if s.httpEngine != nil {
		if err := s.httpListener.Close(); err != nil {
			s.logger.Error("Error closing HTTP listener", zap.Error(err))
		}
		s.logger.Info("HTTP server stopped")
	}

	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err != nil {
			return fmt.Errorf("failed to get database instance: %w", err)
		}
		if err := sqlDB.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
		s.logger.Info("Database connection closed")
	}

	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			s.logger.Error("failed to close Redis connection", zap.Error(err))
		}
		s.logger.Info("Redis connection closed")
	}

	s.logger.Info("API Server shutdown complete")
	return nil
}

func (s *Server) initDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		s.cfg.Database.Host,
		s.cfg.Database.Port,
		s.cfg.Database.User,
		s.cfg.Database.Password,
		s.cfg.Database.DBName,
		s.cfg.Database.SSLMode,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	s.logger.Info("Database connected successfully")
	return db, nil
}

func (s *Server) initRedis() (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", s.cfg.Redis.Host, s.cfg.Redis.Port),
		Password: s.cfg.Redis.Password,
		DB:       s.cfg.Redis.DB,
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	s.logger.Info("Redis connected successfully")
	return rdb, nil
}

func (s *Server) initMinIO() (interface{}, error) {
	s.logger.Info("MinIO initialization skipped (placeholder)")
	return nil, nil
}

func (s *Server) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	s.logger.Info("Shutdown signal received")
}
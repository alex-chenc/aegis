package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ai-benchmark/backend/internal/api"
	grpcHandler "ai-benchmark/backend/internal/grpc/handler"
	"ai-benchmark/backend/internal/models"
	"ai-benchmark/backend/internal/repository"
	"ai-benchmark/backend/internal/service"
	"ai-benchmark/backend/pkg/config"
	"ai-benchmark/backend/pkg/llm"
	pb "ai-benchmark/backend/proto/agent_comm"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "host=localhost user=postgres password=postgres dbname=baseline_system port=5432 sslmode=disable"
	}

	httpPort := os.Getenv("BACKEND_HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}

	grpcPort := os.Getenv("BACKEND_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "9090"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to database: %v. Continuing without DB.", err)
		db = nil
	} else {
		err = db.AutoMigrate(&models.Host{}, &models.Template{}, &models.BaselineRule{}, &models.TaskLog{})
		if err != nil {
			log.Printf("Failed to auto migrate models: %v", err)
		} else {
			log.Println("Database migration completed successfully")
		}
	}

	repo := repository.NewRepository(db)

	var redisClient *config.RedisClient
	redisCfg := config.RedisConfig{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
	}
	redisClient = config.NewRedisClient(redisCfg)
	if err := redisClient.Ping(); err != nil {
		log.Printf("Failed to connect to Redis: %v. Continuing without Redis.", err)
		redisClient = nil
	} else {
		log.Println("Redis connection established")
	}

	llmClient := llm.NewClientFromEnv()

	templateService := service.NewTemplateService(repo, llmClient)

	agentServiceHandler := grpcHandler.NewAgentServiceHandler(db, redisClient)
	taskService := service.NewTaskService(repo, agentServiceHandler)

	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("Failed to listen on gRPC port %s: %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAgentServiceServer(grpcServer, agentServiceHandler)

	go func() {
		log.Printf("gRPC server listening on port %s", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	r := gin.Default()

	r.Use(corsMiddleware())

	wsHandler := api.NewWebSocketHandler()

	hostHandler := api.NewHostHandler(db, agentServiceHandler)
	templateHandler := api.NewTemplateHandler(templateService)
	taskHandler := api.NewTaskHandler(taskService)
	settingsHandler := api.NewSettingsHandler(llmClient)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/hosts", hostHandler.GetHosts)
		v1.GET("/hosts/:id", hostHandler.GetHostDetails)
		v1.POST("/hosts/:id/command", hostHandler.SendCommand)

		v1.GET("/templates", templateHandler.GetTemplates)
		v1.POST("/templates", templateHandler.CreateTemplate)
		v1.GET("/templates/:id", templateHandler.GetTemplate)
		v1.DELETE("/templates/:id", templateHandler.DeleteTemplate)
		v1.POST("/templates/:id/parse", templateHandler.ParseTemplate)
		v1.GET("/templates/:id/rules", templateHandler.GetRules)
		v1.POST("/templates/upload", templateHandler.UploadTemplate)

		v1.GET("/tasks", taskHandler.GetTasks)
		v1.GET("/tasks/:id", taskHandler.GetTask)
		v1.POST("/tasks/check", taskHandler.ExecuteCheck)
		v1.POST("/tasks/fix", taskHandler.ExecuteFix)

		v1.GET("/settings", settingsHandler.GetSettings)
		v1.PUT("/settings/llm", settingsHandler.UpdateLLMConfig)
		v1.POST("/settings/llm/test", settingsHandler.TestLLMConnection)
		v1.GET("/settings/server-info", settingsHandler.GetServerInfo)
		v1.GET("/settings/install-command", settingsHandler.GetInstallCommand)

		v1.GET("/ws", wsHandler.HandleWebSocket)
		v1.GET("/ws/hosts", wsHandler.HandleHostStatusWS)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
		})
	})

	srv := &http.Server{
		Addr:    ":" + httpPort,
		Handler: r,
	}

	go func() {
		log.Printf("HTTP server listening on port %s", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	log.Println("========================================")
	log.Println("AI Benchmark Baseline System Started")
	log.Printf("HTTP API: http://0.0.0.0:%s", httpPort)
	log.Printf("gRPC API: 0.0.0.0:%s", grpcPort)
	log.Println("========================================")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	grpcServer.GracefulStop()

	log.Println("Servers stopped.")
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

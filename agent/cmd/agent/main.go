package main

import (
	"os"
	"os/signal"
	"syscall"

	"aegis-agent/internal/asset"
	"aegis-agent/internal/client"
	"aegis-agent/internal/config"
	"aegis-agent/internal/executor"
	"aegis-agent/internal/logger"

	_ "aegis-agent/pkg/api/v1"

	"go.uber.org/zap"
)

func main() {
	if err := logger.Init("/opt/aegis-agent/logs"); err != nil {
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Aegis Agent v3.0.0 starting...")

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	logger.Info("Config loaded",
		zap.String("server_addr", cfg.ServerAddr),
		zap.String("host_id", cfg.HostID),
	)

	assetInfo, err := asset.Collect()
	if err != nil {
		logger.Fatal("Failed to collect asset info", zap.Error(err))
	}
	logger.Info("Asset info collected",
		zap.String("ip", assetInfo.IPAddress),
		zap.String("hostname", assetInfo.Hostname),
		zap.String("os", assetInfo.OSType),
	)

	exec := executor.NewExecutor(2)
	logger.Info("Executor created", zap.Int("max_concurrency", 2))

	c := client.NewClient(cfg, exec)

	go func() {
		if err := c.Run(); err != nil {
			logger.Error("Client error", zap.Error(err))
		}
	}()
	logger.Info("Client started")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")
	c.Close()
	logger.Info("Agent stopped")
}

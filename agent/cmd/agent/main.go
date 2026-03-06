package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"baseline-agent/internal/asset"
	"baseline-agent/internal/client"
	"baseline-agent/internal/config"
	"baseline-agent/internal/logger"

	"go.uber.org/zap"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "/etc/baseline-agent/config.toml", "config file path")
	flag.Parse()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(cfg.LogFile, cfg.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("agent starting",
		zap.String("version", asset.AgentVersion),
		zap.String("config_path", *configPath),
	)

	// Create client
	agentClient := client.NewClient(cfg.GRPCAddr, cfg.HostID, 2)

	// Connect with retry
	if err := agentClient.Connect(); err != nil {
		logger.Error("failed to connect", zap.Error(err))
		os.Exit(1)
	}
	defer agentClient.Close()

	logger.Info("agent connected successfully",
		zap.String("host_id", cfg.HostID),
		zap.String("grpc_addr", cfg.GRPCAddr),
	)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("agent shutting down")
}

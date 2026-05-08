package main

import (
	"os"
	"os/signal"
	"syscall"

	"aegis-agent/internal/asset"
	"aegis-agent/internal/blocker"
	"aegis-agent/internal/checker"
	"aegis-agent/internal/client"
	"aegis-agent/internal/config"
	"aegis-agent/internal/ebpf"
	"aegis-agent/internal/executor"
	"aegis-agent/internal/logger"
	"aegis-agent/internal/monitor"
	"aegis-agent/internal/sigma"
	"aegis-agent/internal/tools"

	_ "aegis-agent/pkg/api/v1"

	"go.uber.org/zap"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		os.Exit(1)
	}

	if err := logger.Init("/opt/aegis-agent/logs", cfg.LogLevel); err != nil {
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Aegis Agent v3.0.0 starting...",
		zap.String("log_level", cfg.LogLevel))

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

	// V5.7: Load agent-side blacklist checker
	var blacklistChecker *checker.BlacklistChecker
	rulesPath := "/etc/aegis-agent/audit_rules.json"
	if bc, err := checker.NewBlacklistChecker(rulesPath); err != nil {
		logger.Warn("Failed to load blacklist rules, agent-side check disabled", zap.Error(err))
	} else {
		blacklistChecker = bc
		logger.Info("Blacklist checker loaded", zap.Int("rules", bc.RuleCount()))
	}

	exec := executor.NewExecutor(2, blacklistChecker)
	logger.Info("Executor created", zap.Int("max_concurrency", 2))

	ruleLoader := sigma.NewLoader(cfg.RuleDir)
	logger.Info("Creating rule loader", zap.String("rule_dir", cfg.RuleDir))
	// Load rules from disk for local operation
	if err := ruleLoader.LoadFromDisk(); err != nil {
		logger.Info("Loading rules from disk...")
		logger.Warn("Failed to load rules from disk", zap.Error(err))
	}
	blockerInst := blocker.NewBlocker(cfg.QuarantineDir)
	toolManager := tools.NewToolManager()
	metrics := monitor.NewMetrics()
	logger.Info("V5 modules initialized",
		zap.String("rule_dir", cfg.RuleDir),
		zap.String("quarantine_dir", cfg.QuarantineDir),
		zap.Int("event_buffer_size", cfg.EventBufferSize),
	)

	c := client.NewClient(cfg, exec, toolManager, ruleLoader, blockerInst)

	collector := ebpf.NewCollector(cfg.HostID, cfg.EventBufferSize)
	if err := collector.Start(); err != nil {
		logger.Fatal("Failed to start event collector", zap.Error(err))
	}
	logger.Info("Event collector started")

	pipeline := ebpf.NewPipeline(collector, ruleLoader, c, cfg.HostID, metrics)
	pipelineDone := make(chan struct{})
	go pipeline.Run(pipelineDone)
	logger.Info("Event pipeline started")

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
	close(pipelineDone)
	collector.Stop()
	c.Close()
	logger.Info("Agent stopped")
}

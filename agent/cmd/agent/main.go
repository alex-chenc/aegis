package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aegis-agent/internal/asset"
	"aegis-agent/internal/blocker"
	"aegis-agent/internal/checker"
	"aegis-agent/internal/client"
	"aegis-agent/internal/config"
	"aegis-agent/internal/correlation"
	"aegis-agent/internal/dynpkg"
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
	debug := flag.Bool("debug", false, "Enable debug logging (overrides config LogLevel)")
	logStdout := flag.Bool("log-stdout", false, "Also log to stdout (for systemd journal or terminal)")
	signingPubKeyHex := flag.String("signing-public-key", "", "Ed25519 public key (hex) for package signature verification")
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		os.Exit(1)
	}

	if *debug {
		cfg.LogLevel = "debug"
	}

	if err := logger.Init("/opt/aegis-agent/logs", cfg.LogLevel, *logStdout); err != nil {
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

	// V5.8: Initialize correlation engine
	corrEngine := correlation.NewEngine(correlation.CorrelationLimits{
		Window:          60 * time.Second,
		EventsPerKey:    128,
		GlobalCacheSize: 10000,
	})
	logger.Info("Correlation engine initialized")

	// V5.8: Initialize dynamic package manager
	var signingPubKey ed25519.PublicKey
	if *signingPubKeyHex != "" {
		keyBytes, err := hex.DecodeString(*signingPubKeyHex)
		if err != nil {
			logger.Fatal("invalid signing public key hex", zap.Error(err))
		}
		signingPubKey = ed25519.PublicKey(keyBytes)
		logger.Info("signing public key loaded", zap.Int("key_len", len(signingPubKey)))
	}
	corrAdapter := dynpkg.NewCorrelationEngineAdapter(corrEngine)
	dynpkgManager := dynpkg.NewManager(signingPubKey, "", nil, corrAdapter)
	dynpkgManager.SetAlertCallback(func(alert interface{}) {
		logger.Info("Correlation alert triggered", zap.Any("alert", alert))
		// TODO: Report alert to server via gRPC
	})
	dynpkgManager.SetStatusChangeCallback(func(packageID, version, status string) {
		logger.Info("Detection package status changed",
			zap.String("package_id", packageID),
			zap.String("version", version),
			zap.String("status", status))
	})
	logger.Info("Dynamic package manager initialized")

	c := client.NewClient(cfg, exec, toolManager, ruleLoader, blockerInst)

	// Set up detection package handler - parse payload and call dynpkgManager
	c.ConfigManager().SetDetectionPackageHandler(func(action, payload string) error {
		logger.Info("Detection package command received",
			zap.String("action", action),
			zap.Int("payload_len", len(payload)))

		var cmd dynpkg.DetectionPackageCommand
		if err := json.Unmarshal([]byte(payload), &cmd); err != nil {
			logger.Error("failed to parse detection package command", zap.Error(err))
			return fmt.Errorf("parse command: %w", err)
		}
		cmd.Action = action

		ctx := context.Background()
		switch action {
		case "install":
			if err := dynpkgManager.Install(ctx, cmd); err != nil {
				logger.Error("failed to install detection package", zap.Error(err), zap.String("package_id", cmd.PackageID))
				return fmt.Errorf("install: %w", err)
			}
		case "uninstall":
			if err := dynpkgManager.Uninstall(ctx, cmd.PackageID, cmd.Version); err != nil {
				logger.Error("failed to uninstall detection package", zap.Error(err), zap.String("package_id", cmd.PackageID))
				return fmt.Errorf("uninstall: %w", err)
			}
		default:
			logger.Warn("unknown detection package action", zap.String("action", action))
			return fmt.Errorf("unknown action: %s", action)
		}
		return nil
	})

	// Set up allowlist update handler - parse payload and call dynpkgManager
	c.ConfigManager().SetAllowlistUpdateHandler(func(payload string) error {
		logger.Info("Allowlist update received",
			zap.Int("payload_len", len(payload)))

		var allowlist dynpkg.HookAllowlist
		if err := json.Unmarshal([]byte(payload), &allowlist); err != nil {
			logger.Error("failed to parse allowlist", zap.Error(err))
			return fmt.Errorf("parse allowlist: %w", err)
		}

		ctx := context.Background()
		if err := dynpkgManager.ApplyAllowlist(ctx, allowlist); err != nil {
			logger.Error("failed to apply allowlist", zap.Error(err))
			return fmt.Errorf("apply allowlist: %w", err)
		}
		return nil
	})

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

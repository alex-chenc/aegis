package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aegis-agent/internal/asset"

	"github.com/google/uuid"
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

	pb "aegis-agent/pkg/api/v1"

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
	sigmaAdapter := dynpkg.NewSigmaMatcherAdapter(ruleLoader)
	dynpkgManager := dynpkg.NewManager(signingPubKey, "", sigmaAdapter, corrAdapter)
	dynpkgManager.SetStatusChangeCallback(func(packageID, version, status string) {
		logger.Info("Detection package status changed",
			zap.String("package_id", packageID),
			zap.String("version", version),
			zap.String("status", status))
		// Report host status to API server
		go func() {
			reportURL := fmt.Sprintf("http://%s/api/v1/detection/packages/hosts/report", "127.0.0.1:8082")
			reportData := map[string]string{
				"host_id":    cfg.HostID,
					"hostname":   assetInfo.Hostname,
				"package_id": packageID,
				"version":    version,
				"status":     status,
			}
			body, _ := json.Marshal(reportData)
			req, err := http.NewRequest("POST", reportURL, bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			// Use a simple HTTP client without auth (internal API)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				logger.Debug("failed to report host status", zap.Error(err))
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				logger.Debug("host status report returned non-200", zap.Int("status", resp.StatusCode))
			}
		}()
	})
	logger.Info("Dynamic package manager initialized")

	// Restore previously installed packages from disk
	if err := dynpkgManager.RestorePackages(context.Background()); err != nil {
		logger.Error("failed to restore detection packages", zap.Error(err))
	} else {
		logger.Info("Detection packages restored from disk")
		// Report host status for all restored packages
		go func() {
			for _, pkg := range dynpkgManager.Status() {
				reportURL := fmt.Sprintf("http://%s/api/v1/detection/packages/hosts/report", "127.0.0.1:8082")
				reportData := map[string]string{
					"host_id":    cfg.HostID,
					"hostname":   assetInfo.Hostname,
					"package_id": pkg.PackageID,
					"version":    pkg.Version,
					"status":     pkg.Status,
				}
				body, _ := json.Marshal(reportData)
				req, err := http.NewRequest("POST", reportURL, bytes.NewReader(body))
				if err != nil {
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					logger.Debug("failed to report restored package status", zap.String("package_id", pkg.PackageID), zap.Error(err))
					continue
				}
				resp.Body.Close()
				logger.Info("restored package status reported", zap.String("package_id", pkg.PackageID), zap.String("status", pkg.Status))
			}
		}()
	}

	c := client.NewClient(cfg, exec, toolManager, ruleLoader, blockerInst)

	dynpkgManager.SetAlertCallback(func(alert interface{}) {
		logger.Info("Correlation alert triggered", zap.Any("alert", alert))
		corrAlert, ok := alert.(correlation.CorrelationAlert)
		if !ok {
			logger.Error("unexpected alert type in callback")
			return
		}
		evidenceJSON, _ := json.Marshal(corrAlert.Evidence)
		// Extract PID from first evidence item
		var pid int32
		if corrAlert.Evidence != nil && len(corrAlert.Evidence) > 0 {
			pid = int32(corrAlert.Evidence[0].PID)
		}
		// Collect process tree for the triggering PID
		var processTreeJSON string
		if pid > 0 {
			if tree, err := toolManager.GetProcessTree(int(pid)); err == nil {
				if treeJSON, err := json.Marshal(tree); err == nil {
					processTreeJSON = string(treeJSON)
				}
			}
		}
		// Build matched_rule_id to include package_id for frontend correlation
		matchedRuleID := corrAlert.SpecID
		if corrAlert.PackageID != "" {
			matchedRuleID = corrAlert.SpecID
		}
		req := &pb.ReportEventRequest{
			HostId: cfg.HostID,
			Events: []*pb.RuntimeEvent{
				{
					EventId:          "EVT-" + uuid.New().String()[:8],
					HostId:           cfg.HostID,
					EventType:        "correlation_alert",
					Timestamp:        time.Now().UnixMilli(),
					Pid:              pid,
					MatchedRuleId:    matchedRuleID,
					MitreId:          corrAlert.MitreID,
					Severity:         corrAlert.Severity,
					EventDataJson:    string(evidenceJSON),
					MatchedRuleTitle: corrAlert.Title,
					ProcessTree:      processTreeJSON,
				},
			},
		}
		if _, err := c.ReportEvent(context.Background(), req); err != nil {
			logger.Error("failed to report correlation alert", zap.Error(err))
		}
	})

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
	// Feed built-in eBPF events to the correlation engine so package-specific
	// sigma rules (e.g. suspicious_root_exec) can be evaluated and findings
	// fed to the correlation engine for 4-step chains.
	pipeline.SetEventCallback(func(eventMap map[string]interface{}) {
		dynpkgManager.ProcessEventForAll(eventMap)
	})
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

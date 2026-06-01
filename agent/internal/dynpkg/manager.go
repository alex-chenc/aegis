package dynpkg

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"aegis-agent/internal/logger"
	"aegis-agent/internal/sigma"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

const defaultStoragePath = "/var/lib/aegis/detection-packages"

func NewManager(publicKey ed25519.PublicKey, storagePath string, sigmaMatcher SigmaMatcher, corrEngine CorrelationEngine) *Manager {
	if storagePath == "" {
		storagePath = defaultStoragePath
	}
	m := &Manager{
		storagePath:      storagePath,
		publicKey:        publicKey,
		allowlistChecker: &AllowlistChecker{},
		storage:          NewPackageStorage(filepath.Join(storagePath, "state")),
		packages:         make(map[string]*InstalledPackage),
		sigmaMatcher:     sigmaMatcher,
		corrEngine:       corrEngine,
	}
	m.rateLimiter = NewRateLimiter(m.disableByRateLimit)
	return m
}

// RestorePackages loads previously installed packages from disk on startup.
// It reads state files and re-installs packages that were in "active" state.
func (m *Manager) RestorePackages(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	records, err := m.storage.ListInstalled()
	if err != nil {
		return fmt.Errorf("list installed packages: %w", err)
	}

	restoredCount := 0
	for _, record := range records {
		if record.State != StateActive {
			logger.Info("skipping non-active package during restore",
				zap.String("package_id", record.PackageID),
				zap.String("version", record.Version),
				zap.String("state", string(record.State)),
			)
			continue
		}

		extractDir := filepath.Join(m.storagePath, record.PackageID, record.Version)
		if _, err := os.Stat(extractDir); os.IsNotExist(err) {
			logger.Warn("package directory not found, skipping restore",
				zap.String("package_id", record.PackageID),
				zap.String("version", record.Version),
				zap.String("path", extractDir),
			)
			continue
		}

		// Parse manifests
		pkgManifest, pluginManifest, err := ParseManifests(extractDir)
		if err != nil {
			logger.Error("failed to parse manifests during restore",
				zap.String("package_id", record.PackageID),
				zap.Error(err),
			)
			continue
		}

		pkg := &InstalledPackage{
			PackageID:      record.PackageID,
			Version:        record.Version,
			Manifest:       pkgManifest,
			PluginManifest: pluginManifest,
			stateMachine:   NewStateMachine(StateActive),
			Status:         string(StateActive),
		}

		// Load plugin
		if err := m.loadPlugin(ctx, pkg, extractDir); err != nil {
			logger.Error("failed to load plugin during restore",
				zap.String("package_id", record.PackageID),
				zap.Error(err),
			)
			continue
		}

		// Load sigma rules
		if len(pkgManifest.SigmaRules) > 0 {
			for _, rulePath := range pkgManifest.SigmaRules {
				fullPath := filepath.Join(extractDir, rulePath)
				content, err := os.ReadFile(fullPath)
				if err != nil {
					logger.Error("failed to read sigma rule file during restore",
						zap.String("package_id", record.PackageID),
						zap.String("file", rulePath), zap.Error(err))
					continue
				}
				rules, err := sigma.ParseRules(content)
				if err != nil {
					logger.Error("failed to parse sigma rules during restore",
						zap.String("package_id", record.PackageID),
						zap.String("file", rulePath), zap.Error(err))
					continue
				}
				for _, r := range rules {
					pkg.LoadedSigmaRuleIDs = append(pkg.LoadedSigmaRuleIDs, r.ID)
				}
				if m.sigmaMatcher != nil {
					if err := m.sigmaMatcher.AddRules(content); err != nil {
						logger.Error("failed to add sigma rules during restore",
							zap.String("package_id", record.PackageID),
							zap.String("file", rulePath), zap.Error(err))
					} else {
						logger.Info("sigma rules restored from package",
							zap.String("package_id", record.PackageID),
							zap.String("file", rulePath),
							zap.Int("rule_count", len(rules)))
					}
				}
			}
		}

		// Load correlation rules
		if len(pkgManifest.CorrelationRules) > 0 && m.corrEngine != nil {
			for _, rulePath := range pkgManifest.CorrelationRules {
				fullPath := filepath.Join(extractDir, rulePath)
				content, err := os.ReadFile(fullPath)
				if err != nil {
					logger.Error("failed to read correlation rule file during restore",
						zap.String("package_id", record.PackageID),
						zap.String("file", rulePath), zap.Error(err))
					continue
				}
				if err := m.corrEngine.AddSpec(content); err != nil {
					logger.Error("failed to add correlation rule during restore",
						zap.String("package_id", record.PackageID),
						zap.String("file", rulePath), zap.Error(err))
				} else {
					pkg.LoadedCorrelationFiles = append(pkg.LoadedCorrelationFiles, rulePath)
					logger.Info("correlation rules restored from package",
						zap.String("package_id", record.PackageID),
						zap.String("file", rulePath))
				}
			}
		}

		// Set rate limits
		if pkgManifest.Limits.MaxEventsPerSecond > 0 {
			m.rateLimiter.UpdateLimits(record.PackageID,
				rate.Limit(float64(pkgManifest.Limits.MaxEventsPerSecond)),
				rate.Limit(float64(pkgManifest.Limits.MaxEventsPerPidPerSecond)),
				rate.Limit(float64(pkgManifest.Limits.MaxEventsPerPidPerSecond)),
			)
		}

		m.packages[record.PackageID] = pkg
		restoredCount++

		logger.Info("detection package restored from disk",
			zap.String("package_id", record.PackageID),
			zap.String("version", record.Version),
			zap.String("artifact", pkg.ActiveArtifact),
			zap.Int("hooks", len(pkg.LoadedHooks)),
			zap.Int("sigma_rules", len(pkg.LoadedSigmaRuleIDs)),
		)
	}

	logger.Info("package restore complete",
		zap.Int("total_records", len(records)),
		zap.Int("restored", restoredCount),
	)
	return nil
}

func (m *Manager) SetAlertCallback(fn func(alert interface{})) {
	m.onAlert = fn
}

func (m *Manager) SetStatusChangeCallback(fn func(packageID, version, status string)) {
	m.onStatusChange = fn
}

func (m *Manager) ApplyAllowlist(ctx context.Context, allowlist HookAllowlist) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.allowlistChecker.Update(allowlist)
	logger.Info("hook allowlist updated",
		zap.Int64("version", allowlist.Version),
		zap.Int("tracepoints", len(allowlist.Tracepoints)),
	)

	for _, pkg := range m.packages {
		if pkg.stateMachine.Current == StateActive {
			if err := checkHooksAgainstAllowlist(pkg.PluginManifest.Hooks, &allowlist); err != nil {
				m.failPkg(pkg, StateBlockedByAllowlist, err.Error())
			}
		}
	}
	return nil
}

func (m *Manager) Install(ctx context.Context, cmd DetectionPackageCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.allowlistChecker.Get().Version == 0 {
		return fmt.Errorf("hook allowlist not received, cannot install dynamic packages")
	}

	// If a package with the same ID is already installed, unload it first.
	if existing, exists := m.packages[cmd.PackageID]; exists {
		logger.Warn("package already installed, unloading before reinstall",
			zap.String("package_id", cmd.PackageID),
			zap.String("old_version", existing.Version),
		)
		if err := m.unloadPlugin(existing); err != nil {
			logger.Error("failed to unload previous plugin instance",
				zap.String("package_id", cmd.PackageID), zap.Error(err))
		}
		// Remove sigma rules from the previous installation
		if len(existing.LoadedSigmaRuleIDs) > 0 && m.sigmaMatcher != nil {
			m.sigmaMatcher.RemoveRules(existing.LoadedSigmaRuleIDs)
		}
		if m.corrEngine != nil {
			m.corrEngine.RemovePackage(cmd.PackageID)
		}
		m.rateLimiter.RemovePackage(cmd.PackageID)
		delete(m.packages, cmd.PackageID)
	}

	pkg := &InstalledPackage{
		PackageID:    cmd.PackageID,
		Version:      cmd.Version,
		stateMachine: NewStateMachine(StatePending),
	}
	m.transitionPkg(pkg, StateDownloading, "download started")

	packagePath, sigPath, err := m.downloadPackage(ctx, cmd.PackageURL, cmd.SignatureURL)
	if err != nil {
		m.failPkg(pkg, StateLoadFailed, err.Error())
		return fmt.Errorf("download package: %w", err)
	}
	defer os.Remove(packagePath)
	defer os.Remove(sigPath)

	m.transitionPkg(pkg, StateVerifying, "signature verification started")

	if err := VerifySignature(m.publicKey, packagePath, sigPath); err != nil {
		m.failPkg(pkg, StateSignatureFailed, err.Error())
		return fmt.Errorf("verify signature: %w", err)
	}

	extractDir := filepath.Join(m.storagePath, cmd.PackageID, cmd.Version)
	if err := ExtractPackage(packagePath, extractDir); err != nil {
		m.failPkg(pkg, StateLoadFailed, err.Error())
		return fmt.Errorf("extract package: %w", err)
	}

	pkgManifest, pluginManifest, err := ParseManifests(extractDir)
	if err != nil {
		m.failPkg(pkg, StateLoadFailed, err.Error())
		return fmt.Errorf("parse manifests: %w", err)
	}
	pkg.Manifest = pkgManifest
	pkg.PluginManifest = pluginManifest

	m.transitionPkg(pkg, StateCheckingAllowlist, "allowlist check started")

	if err := checkHooksAgainstAllowlist(pluginManifest.Hooks, m.allowlistChecker.GetPtr()); err != nil {
		m.failPkg(pkg, StateBlockedByAllowlist, err.Error())
		return fmt.Errorf("allowlist check: %w", err)
	}

	m.transitionPkg(pkg, StateInstalling, "loading plugin")

	if err := m.loadPlugin(ctx, pkg, extractDir); err != nil {
		m.failPkg(pkg, StateLoadFailed, err.Error())
		return fmt.Errorf("load plugin: %w", err)
	}

	// Load sigma rules from the package
	if len(pkgManifest.SigmaRules) > 0 {
		for _, rulePath := range pkgManifest.SigmaRules {
			fullPath := filepath.Join(extractDir, rulePath)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				logger.Error("failed to read sigma rule file",
					zap.String("package_id", cmd.PackageID),
					zap.String("file", rulePath), zap.Error(err))
				continue
			}
			// Parse rules to extract IDs for tracking
			rules, err := sigma.ParseRules(content)
			if err != nil {
				logger.Error("failed to parse sigma rules from file",
					zap.String("package_id", cmd.PackageID),
					zap.String("file", rulePath), zap.Error(err))
				continue
			}
			for _, r := range rules {
				pkg.LoadedSigmaRuleIDs = append(pkg.LoadedSigmaRuleIDs, r.ID)
			}
			if m.sigmaMatcher != nil {
				if err := m.sigmaMatcher.AddRules(content); err != nil {
					logger.Error("failed to add sigma rules from package",
						zap.String("package_id", cmd.PackageID),
						zap.String("file", rulePath), zap.Error(err))
				} else {
					logger.Info("sigma rules loaded from package",
						zap.String("package_id", cmd.PackageID),
						zap.String("file", rulePath),
						zap.Int("rule_count", len(rules)))
				}
			}
		}
	}

	// Load correlation rules from the package
	if len(pkgManifest.CorrelationRules) > 0 && m.corrEngine != nil {
		for _, rulePath := range pkgManifest.CorrelationRules {
			fullPath := filepath.Join(extractDir, rulePath)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				logger.Error("failed to read correlation rule file",
					zap.String("package_id", cmd.PackageID),
					zap.String("file", rulePath), zap.Error(err))
				continue
			}
			// The correlation engine accepts raw content as interface{}
			if err := m.corrEngine.AddSpec(content); err != nil {
				logger.Error("failed to add correlation rule from package",
					zap.String("package_id", cmd.PackageID),
					zap.String("file", rulePath), zap.Error(err))
			} else {
				pkg.LoadedCorrelationFiles = append(pkg.LoadedCorrelationFiles, rulePath)
				logger.Info("correlation rules loaded from package",
					zap.String("package_id", cmd.PackageID),
					zap.String("file", rulePath))
			}
		}
	}

	if pkgManifest.Limits.MaxEventsPerSecond > 0 {
		m.rateLimiter.UpdateLimits(cmd.PackageID,
			rate.Limit(float64(pkgManifest.Limits.MaxEventsPerSecond)),
			rate.Limit(float64(pkgManifest.Limits.MaxEventsPerPidPerSecond)),
			rate.Limit(float64(pkgManifest.Limits.MaxEventsPerPidPerSecond)),
		)
	}

	m.transitionPkg(pkg, StateActive, "package active")
	logger.Info("detection package installed",
		zap.String("package_id", cmd.PackageID),
		zap.String("version", cmd.Version),
		zap.String("artifact", pkg.ActiveArtifact),
	)
	return nil
}

func (m *Manager) Uninstall(ctx context.Context, packageID, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pkg, exists := m.packages[packageID]
	if !exists {
		return fmt.Errorf("package %s not found", packageID)
	}

	if err := m.unloadPlugin(pkg); err != nil {
		logger.Error("failed to unload plugin", zap.String("package_id", packageID), zap.Error(err))
	}

	// Remove sigma rules loaded by this package
	if len(pkg.LoadedSigmaRuleIDs) > 0 && m.sigmaMatcher != nil {
		m.sigmaMatcher.RemoveRules(pkg.LoadedSigmaRuleIDs)
		logger.Info("sigma rules removed for package",
			zap.String("package_id", packageID),
			zap.Int("rule_count", len(pkg.LoadedSigmaRuleIDs)))
	}

	// Remove correlation rules loaded by this package
	if len(pkg.LoadedCorrelationFiles) > 0 && m.corrEngine != nil {
		m.corrEngine.RemovePackage(packageID)
		logger.Info("correlation rules removed for package",
			zap.String("package_id", packageID))
	}

	extractDir := filepath.Join(m.storagePath, packageID, version)
	if err := os.RemoveAll(extractDir); err != nil {
		logger.Error("failed to remove package files", zap.String("path", extractDir), zap.Error(err))
	}

	_ = pkg.stateMachine.Transition(StateUninstalled, "uninstalled")
	pkg.Status = string(StateUninstalled)
	delete(m.packages, packageID)
	m.rateLimiter.RemovePackage(packageID)
	if m.onStatusChange != nil {
		m.onStatusChange(packageID, version, string(StateUninstalled))
	}

	logger.Info("detection package uninstalled",
		zap.String("package_id", packageID),
		zap.String("version", version),
	)
	return nil
}

func (m *Manager) DisableByPolicy(packageID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pkg, exists := m.packages[packageID]
	if !exists {
		return fmt.Errorf("package %s not found", packageID)
	}

	if m.corrEngine != nil {
		m.corrEngine.RemovePackage(packageID)
	}

	if err := m.unloadPlugin(pkg); err != nil {
		logger.Error("failed to unload plugin in DisableByPolicy", zap.String("package_id", packageID), zap.Error(err))
	}

	m.rateLimiter.RemovePackage(packageID)
	m.failPkg(pkg, StateDisabledByPolicy, reason)

	logger.Info("detection package disabled by policy",
		zap.String("package_id", packageID),
		zap.String("reason", reason),
	)
	return nil
}

func (m *Manager) failPkg(pkg *InstalledPackage, state PackageState, reason string) {
	m.transitionPkg(pkg, state, reason)
	pkg.ErrorMessage = reason
}

func (m *Manager) transitionPkg(pkg *InstalledPackage, state PackageState, reason string) {
	_ = pkg.stateMachine.Transition(state, reason)
	pkg.Status = string(state)
	m.packages[pkg.PackageID] = pkg
	_ = m.storage.SaveState(PackageStateRecord{
		PackageID: pkg.PackageID,
		Version:   pkg.Version,
		State:     pkg.stateMachine.Current,
		UpdatedAt: time.Now(),
		Reason:    reason,
	})
	if m.onStatusChange != nil {
		m.onStatusChange(pkg.PackageID, pkg.Version, pkg.Status)
	}
}

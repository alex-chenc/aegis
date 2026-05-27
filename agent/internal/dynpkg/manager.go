package dynpkg

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"aegis-agent/internal/logger"

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

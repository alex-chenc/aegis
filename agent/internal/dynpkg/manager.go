package dynpkg

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"aegis-agent/internal/ebpf/plugin"
	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

const defaultStoragePath = "/var/lib/aegis/detection-packages"

type Manager struct {
	mu             sync.RWMutex
	storagePath    string
	publicKey      ed25519.PublicKey
	allowlist      *HookAllowlist
	packages       map[string]*InstalledPackage
	onStatusChange func(packageID, version, status string)
	onAlert        func(alert interface{})

	// V5.8: Sigma and Correlation integration
	sigmaMatcher SigmaMatcher
	corrEngine   CorrelationEngine
}

type SigmaMatcher interface {
	Match(event map[string]interface{}) []SigmaMatch
}

type SigmaMatch struct {
	RuleID    string
	Title     string
	Severity  string
	MitreID   string
	EventType string
}

type CorrelationEngine interface {
	AddSpec(spec interface{}) error
	RemovePackage(packageID string)
	AddFinding(finding interface{}) []interface{}
}

type InstalledPackage struct {
	PackageID      string
	Version        string
	Manifest       *PackageManifest
	PluginManifest *PluginManifest
	ActiveArtifact string
	Status         string
	LoadedHooks    []string
	ErrorMessage   string
}

type DetectionPackageCommand struct {
	CommandID    string
	Action       string
	PackageID    string
	Version      string
	PackageURL   string
	SignatureURL string
	PackageSize  int64
	Rollback     bool
}

func NewManager(publicKey ed25519.PublicKey, storagePath string, sigmaMatcher SigmaMatcher, corrEngine CorrelationEngine) *Manager {
	if storagePath == "" {
		storagePath = defaultStoragePath
	}
	return &Manager{
		storagePath:  storagePath,
		publicKey:    publicKey,
		packages:     make(map[string]*InstalledPackage),
		sigmaMatcher: sigmaMatcher,
		corrEngine:   corrEngine,
	}
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

	m.allowlist = &allowlist
	logger.Info("hook allowlist updated",
		zap.Int64("version", allowlist.Version),
		zap.Int("tracepoints", len(allowlist.Tracepoints)),
	)

	for _, pkg := range m.packages {
		if pkg.Status == "active" {
			if err := m.checkAllowlist(pkg); err != nil {
				pkg.Status = "blocked_by_hook_allowlist"
				pkg.ErrorMessage = err.Error()
				m.notifyStatusChange(pkg.PackageID, pkg.Version, pkg.Status)
			}
		}
	}
	return nil
}

func (m *Manager) Install(ctx context.Context, cmd DetectionPackageCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.allowlist == nil {
		return fmt.Errorf("hook allowlist not received, cannot install dynamic packages")
	}

	packagePath, sigPath, err := m.downloadPackage(ctx, cmd.PackageURL, cmd.SignatureURL)
	if err != nil {
		return fmt.Errorf("download package: %w", err)
	}
	defer os.Remove(packagePath)
	defer os.Remove(sigPath)

	if err := VerifySignature(m.publicKey, packagePath, sigPath); err != nil {
		m.updateStatus(cmd.PackageID, cmd.Version, "signature_failed", err.Error())
		return fmt.Errorf("verify signature: %w", err)
	}

	extractDir := filepath.Join(m.storagePath, cmd.PackageID, cmd.Version)
	if err := ExtractPackage(packagePath, extractDir); err != nil {
		return fmt.Errorf("extract package: %w", err)
	}

	pkgManifest, pluginManifest, err := ParseManifests(extractDir)
	if err != nil {
		m.updateStatus(cmd.PackageID, cmd.Version, "load_failed", err.Error())
		return fmt.Errorf("parse manifests: %w", err)
	}

	pkg := &InstalledPackage{
		PackageID:      cmd.PackageID,
		Version:        cmd.Version,
		Manifest:       pkgManifest,
		PluginManifest: pluginManifest,
		Status:         "installing",
	}

	if err := m.checkAllowlist(pkg); err != nil {
		pkg.Status = "blocked_by_hook_allowlist"
		pkg.ErrorMessage = err.Error()
		m.packages[cmd.PackageID] = pkg
		m.notifyStatusChange(cmd.PackageID, cmd.Version, pkg.Status)
		return fmt.Errorf("allowlist check: %w", err)
	}

	if err := m.loadPlugin(ctx, pkg, extractDir); err != nil {
		pkg.Status = "load_failed"
		pkg.ErrorMessage = err.Error()
		m.packages[cmd.PackageID] = pkg
		m.notifyStatusChange(cmd.PackageID, cmd.Version, pkg.Status)
		return fmt.Errorf("load plugin: %w", err)
	}

	pkg.Status = "active"
	m.packages[cmd.PackageID] = pkg
	m.notifyStatusChange(cmd.PackageID, cmd.Version, pkg.Status)

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

	delete(m.packages, packageID)
	m.notifyStatusChange(packageID, version, "uninstalled")

	logger.Info("detection package uninstalled",
		zap.String("package_id", packageID),
		zap.String("version", version),
	)
	return nil
}

func (m *Manager) Status() []InstalledPackage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []InstalledPackage
	for _, pkg := range m.packages {
		statuses = append(statuses, *pkg)
	}
	return statuses
}

func (m *Manager) checkAllowlist(pkg *InstalledPackage) error {
	if m.allowlist == nil {
		return fmt.Errorf("hook allowlist not available")
	}

	for _, hook := range pkg.PluginManifest.Hooks {
		allowed := false
		switch hook.AttachType {
		case "tracepoint":
			allowed = contains(m.allowlist.Tracepoints, hook.Attach)
		case "kprobe":
			allowed = contains(m.allowlist.Kprobes, hook.Attach)
		case "lsm":
			allowed = contains(m.allowlist.LSM, hook.Attach)
		case "xdp":
			allowed = contains(m.allowlist.XDP, hook.Attach)
		case "tc":
			allowed = contains(m.allowlist.TC, hook.Attach)
		}
		if !allowed {
			return fmt.Errorf("hook %s (%s) not in allowlist", hook.Name, hook.Attach)
		}
	}
	return nil
}

func (m *Manager) loadPlugin(ctx context.Context, pkg *InstalledPackage, extractDir string) error {
	pkg.ActiveArtifact = "ringbuf"

	// Convert to plugin package types (avoid import cycle)
	pluginInfo := &plugin.PackageInfo{
		PackageID:      pkg.PackageID,
		ActiveArtifact: pkg.ActiveArtifact,
		Manifest:       convertManifest(pkg.PluginManifest),
	}

	err := plugin.LoadPlugin(pluginInfo, extractDir, func(pkgID string, event map[string]interface{}) {
		m.onPluginEvent(pkgID, event)
	})
	if err != nil {
		return fmt.Errorf("load plugin: %w", err)
	}

	for _, hook := range pkg.PluginManifest.Hooks {
		pkg.LoadedHooks = append(pkg.LoadedHooks, hook.Attach)
	}
	return nil
}

func (m *Manager) unloadPlugin(pkg *InstalledPackage) error {
	return plugin.UnloadPlugin(pkg.PackageID)
}

func convertManifest(pm *PluginManifest) *plugin.PluginManifest {
	if pm == nil {
		return nil
	}
	hooks := make([]plugin.PluginHook, len(pm.Hooks))
	for i, h := range pm.Hooks {
		hooks[i] = plugin.PluginHook{
			Name:       h.Name,
			AttachType: h.AttachType,
			Attach:     h.Attach,
			Program:    h.Program,
		}
	}
	events := make(map[int]plugin.EventDef)
	for k, v := range pm.EventSchema.Events {
		fields := make(map[int]plugin.FieldDef)
		for fk, fv := range v.Fields {
			fields[fk] = plugin.FieldDef{Name: fv.Name, Type: fv.Type}
		}
		events[k] = plugin.EventDef{Name: v.Name, Fields: fields}
	}
	return &plugin.PluginManifest{
		Hooks:       hooks,
		EventSchema: plugin.EventSchema{Events: events},
	}
}

func (m *Manager) onPluginEvent(pkgID string, event map[string]interface{}) {
	m.ProcessEvent(pkgID, event)
}

// ProcessEvent processes a plugin event through Sigma and Correlation
func (m *Manager) ProcessEvent(packageID string, event map[string]interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pkg, exists := m.packages[packageID]
	if !exists || pkg.Status != "active" {
		return
	}

	// Run through Sigma matcher
	if m.sigmaMatcher != nil {
		matches := m.sigmaMatcher.Match(event)
		for _, match := range matches {
			// Create AtomicFinding
			finding := map[string]interface{}{
				"package_id": packageID,
				"version":    pkg.Version,
				"rule_id":    match.RuleID,
				"event_type": match.EventType,
				"timestamp":  event["timestamp"],
				"host_id":    event["host_id"],
				"pid":        event["pid"],
				"uid":        event["uid"],
				"event_map":  event,
			}

			// Pass to correlation engine
			if m.corrEngine != nil {
				alerts := m.corrEngine.AddFinding(finding)
				for _, alert := range alerts {
					if m.onAlert != nil {
						m.onAlert(alert)
					}
				}
			}
		}
	}
}

func (m *Manager) downloadPackage(ctx context.Context, packageURL, signatureURL string) (string, string, error) {
	tmpDir := filepath.Join(m.storagePath, "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", "", fmt.Errorf("create tmp dir: %w", err)
	}

	packagePath := filepath.Join(tmpDir, "package.tar.gz")
	if err := downloadFile(ctx, packageURL, packagePath); err != nil {
		return "", "", fmt.Errorf("download package: %w", err)
	}

	sigPath := filepath.Join(tmpDir, "package.tar.gz.sig")
	if err := downloadFile(ctx, signatureURL, sigPath); err != nil {
		os.Remove(packagePath)
		return "", "", fmt.Errorf("download signature: %w", err)
	}

	return packagePath, sigPath, nil
}

func downloadFile(ctx context.Context, url, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") {
		data, err := os.ReadFile(url)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}

func (m *Manager) updateStatus(packageID, version, status, errMsg string) {
	pkg, exists := m.packages[packageID]
	if !exists {
		pkg = &InstalledPackage{
			PackageID: packageID,
			Version:   version,
		}
		m.packages[packageID] = pkg
	}
	pkg.Status = status
	pkg.ErrorMessage = errMsg
	m.notifyStatusChange(packageID, version, status)
}

func (m *Manager) notifyStatusChange(packageID, version, status string) {
	if m.onStatusChange != nil {
		m.onStatusChange(packageID, version, status)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

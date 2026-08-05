package agentguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aegis-agent/internal/logger"
	pb "aegis-agent/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type RuntimeReporter interface {
	ReportEvents(events []*pb.RuntimeEvent) error
}

type ManagerConfig struct {
	Enabled                bool
	BehaviorMonitorEnabled bool
	ToolAdapterEnabled     bool
	SessionHookEnabled     bool
	ToolSourceManifest     string
	ToolHookSocket         string
	HookBinary             string
	HookProvisioner        HookProvisioner
	HookRemover            HookRemover
	ScopedHookProvisioner  ScopedHookProvisioner
	ScopedHookRemover      ScopedHookProvisioner
	ToolAdapter            *TrustedToolAdapter
	EnforcementEnabled     bool
	FreezeEnabled          bool
	HostID                 string
	StateDir               string
	SpoolCapacity          int
	ReconcileInterval      time.Duration
	FlushInterval          time.Duration
	Capabilities           *GuardCapabilities
	ActionFS               ActionFileSystem
	ProcessSignaler        ProcessSignaler
	ActionScheduler        ActionScheduler
	CgroupRoot             string
	SelfPID                uint32
	ParentPID              uint32
	FreezeTimeout          time.Duration
}

type isolationStateScanner interface {
	ReadIsolation(pid uint32) (IsolationState, error)
}

type Manager struct {
	cfg            ManagerConfig
	scanner        ProcessScanner
	reporter       RuntimeReporter
	tracker        *IdentityTracker
	reconciler     *Reconciler
	normalizer     *BehaviorNormalizer
	aggregator     *Aggregator
	spool          *PrioritySpool
	bundles        *BundleStore
	monitorAllowed bool
	monitorEnabled atomic.Bool
	escapeEnabled  atomic.Bool
	toolEnabled    atomic.Bool
	sessionEnabled atomic.Bool
	reconcileReset chan time.Duration
	isolationScan  isolationStateScanner
	capabilities   GuardCapabilities
	actions        *ActionExecutor
	toolAdapter    *TrustedToolAdapter
	toolAdapterMu  sync.RWMutex
	toolIngress    *ToolHookReceiver
	toolIngressMu  sync.Mutex
	kernelPolicyMu sync.RWMutex
	kernelPolicy   CompiledKernelPolicy
	kernelApply    func(CompiledKernelPolicy, []KernelSubject) error

	statusMu                  sync.Mutex
	lifecycleMu               sync.Mutex
	isolationMu               sync.Mutex
	pendingStatus             []*pb.RuntimeEvent
	started                   map[string]bool
	startedUnits              map[string]bool
	startedSessions           map[string]bool
	lastHeartbeat             map[string]time.Time
	lastDrift                 map[string]string
	runtimeSettingsSet        atomic.Bool
	runtimeToolAdapterEnabled atomic.Bool
	runtimeSessionHookEnabled atomic.Bool
	runtimeSettingsVersion    atomic.Int64
	bundleToolAdapterEnabled  atomic.Bool
	hookProvisioner           HookProvisioner
	hookRemover               HookRemover
	scopedHookProvisioner     ScopedHookProvisioner
	scopedHookRemover         ScopedHookProvisioner
	hookStateMu               sync.Mutex
	hookStates                map[string]bool
	cancel                    context.CancelFunc
	wg                        sync.WaitGroup
}

func NewManager(cfg ManagerConfig, scanner ProcessScanner, reporter RuntimeReporter) *Manager {
	if scanner == nil {
		scanner = NewProcFSScanner("/proc")
	}
	if cfg.StateDir == "" {
		cfg.StateDir = "/var/lib/aegis/agent-guard"
	}
	if cfg.ReconcileInterval <= 0 {
		cfg.ReconcileInterval = 30 * time.Second
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	profiles := NewBuiltinProfileRegistry()
	tracker := NewIdentityTracker(cfg.HostID, profiles)
	capabilities := ProbeGuardCapabilities("/proc", "/sys")
	if cfg.Capabilities != nil {
		capabilities = *cfg.Capabilities
	}
	manager := &Manager{
		cfg:        cfg,
		scanner:    scanner,
		reporter:   reporter,
		tracker:    tracker,
		reconciler: NewReconciler(tracker),
		normalizer: NewBehaviorNormalizer(cfg.HostID, scanner.BootID(), tracker),
		aggregator: NewAggregator(2 * time.Second),
		spool:      NewPrioritySpool(cfg.SpoolCapacity),
		bundles: NewBundleStoreWithOptions(cfg.StateDir, cfg.HostID, BundleValidationOptions{
			EnforcementAllowed: cfg.EnforcementEnabled,
			FreezeAllowed:      cfg.FreezeEnabled,
			ToolAdapterAllowed: cfg.ToolAdapterEnabled,
		}),
		monitorAllowed:        cfg.BehaviorMonitorEnabled,
		started:               make(map[string]bool),
		startedUnits:          make(map[string]bool),
		startedSessions:       make(map[string]bool),
		lastHeartbeat:         make(map[string]time.Time),
		reconcileReset:        make(chan time.Duration, 1),
		capabilities:          capabilities,
		lastDrift:             make(map[string]string),
		toolAdapter:           cfg.ToolAdapter,
		hookProvisioner:       cfg.HookProvisioner,
		hookRemover:           cfg.HookRemover,
		hookStates:            make(map[string]bool),
		scopedHookProvisioner: cfg.ScopedHookProvisioner,
		scopedHookRemover:     cfg.ScopedHookRemover,
	}
	hookBinary := cfg.HookBinary
	if hookBinary == "" {
		hookBinary = "/opt/aegis-agent/aegis-codex-hook"
	}
	legacyProvisionerProvided := cfg.HookProvisioner != nil
	legacyRemoverProvided := cfg.HookRemover != nil
	if manager.hookProvisioner == nil {
		manager.hookProvisioner = DefaultHookProvisioner(hookBinary)
	}
	if manager.hookRemover == nil {
		manager.hookRemover = DefaultHookRemover(hookBinary)
	}
	if manager.scopedHookProvisioner == nil && !legacyProvisionerProvided {
		manager.scopedHookProvisioner = DefaultScopedHookProvisioner(hookBinary)
	}
	if manager.scopedHookRemover == nil && !legacyRemoverProvided {
		manager.scopedHookRemover = DefaultScopedHookRemover(hookBinary)
	}
	manager.isolationScan, _ = scanner.(isolationStateScanner)
	manager.actions = newActionExecutor(cfg, tracker, scanner, capabilities)
	manager.actions.report = manager.reportActionResult
	manager.monitorEnabled.Store(cfg.BehaviorMonitorEnabled)
	manager.escapeEnabled.Store(true)
	manager.toolEnabled.Store(false)
	manager.sessionEnabled.Store(false)
	return manager
}

func (m *Manager) currentToolAdapter() *TrustedToolAdapter {
	m.toolAdapterMu.RLock()
	defer m.toolAdapterMu.RUnlock()
	return m.toolAdapter
}

func (m *Manager) replaceToolAdapter(adapter *TrustedToolAdapter) {
	m.toolAdapterMu.Lock()
	m.toolAdapter = adapter
	m.toolAdapterMu.Unlock()
}

func (m *Manager) effectiveToolAdapterEnabled() bool {
	if m.runtimeSettingsSet.Load() {
		return m.runtimeToolAdapterEnabled.Load()
	}
	return m.cfg.ToolAdapterEnabled
}

func (m *Manager) effectiveSessionHookEnabled() bool {
	if m.runtimeSettingsSet.Load() {
		return m.runtimeSessionHookEnabled.Load()
	}
	return m.cfg.SessionHookEnabled
}

// ApplyRuntimeSettings applies the control-plane settings in memory and
// provisions only the explicitly selected native Hook integrations. The local
// Agent TOML is intentionally not changed.
func (m *Manager) ApplyRuntimeSettings(payload string) error {
	if !m.cfg.Enabled {
		return errors.New("agent_guard_disabled")
	}
	settings, err := decodeRuntimeSettings(payload)
	if err != nil {
		m.queueRuntimeSettingsStatus("rejected", settings.Version, errorCode(err), nil)
		return err
	}
	if settings.HostID != m.cfg.HostID {
		m.queueRuntimeSettingsStatus("rejected", settings.Version, "agent_guard_runtime_settings_host_mismatch", nil)
		return errors.New("agent_guard_runtime_settings_host_mismatch")
	}
	if !settings.BehaviorPolicyEnabled && !settings.EscapePolicyEnabled {
		// Compatibility for v1 senders that predate policy scopes.
		settings.BehaviorPolicyEnabled = settings.ToolAdapterEnabled || settings.SessionHookEnabled
		for _, injection := range settings.Injections {
			settings.BehaviorPolicyEnabled = settings.BehaviorPolicyEnabled || injection.BehaviorEnabled
		}
		hasEscapeScope := false
		for _, injection := range settings.Injections {
			hasEscapeScope = hasEscapeScope || injection.EscapeEnabled
		}
		settings.EscapePolicyEnabled = hasEscapeScope || !hasScopedPolicyFields(settings)
	}
	currentVersion := m.runtimeSettingsVersion.Load()
	if m.runtimeSettingsSet.Load() && settings.Version < currentVersion {
		m.queueRuntimeSettingsStatus("rejected", settings.Version, "agent_guard_runtime_settings_stale", nil)
		return errors.New("agent_guard_runtime_settings_stale")
	}

	enabledInjections, hookErr := m.applyHookInjections(settings)
	if hookErr != nil {
		m.queueRuntimeSettingsStatus("failed", settings.Version, errorCode(hookErr), enabledInjections)
		return hookErr
	}

	if m.cfg.ToolSourceManifest != "" {
		adapter, loadErr := LoadTrustedToolAdapter(m.cfg.ToolSourceManifest)
		if loadErr == nil {
			m.replaceToolAdapter(adapter)
		} else if len(enabledInjections) > 0 && (settings.ToolAdapterEnabled || settings.SessionHookEnabled) {
			m.queueRuntimeSettingsStatus("failed", settings.Version, "agent_guard_tool_manifest_reload_failed", enabledInjections)
			return loadErr
		} else if len(enabledInjections) == 0 {
			m.replaceToolAdapter(nil)
		}
	}
	if len(enabledInjections) > 0 && (settings.ToolAdapterEnabled || settings.SessionHookEnabled) && m.currentToolAdapter() == nil {
		m.queueRuntimeSettingsStatus("failed", settings.Version, "agent_guard_tool_adapter_unavailable", enabledInjections)
		return errors.New("agent_guard_tool_adapter_unavailable")
	}

	m.runtimeToolAdapterEnabled.Store(settings.ToolAdapterEnabled)
	m.runtimeSessionHookEnabled.Store(settings.SessionHookEnabled)
	m.escapeEnabled.Store(settings.EscapePolicyEnabled)
	m.runtimeSettingsVersion.Store(settings.Version)
	m.runtimeSettingsSet.Store(true)
	if err := m.persistRuntimeSettings(settings); err != nil {
		m.queueRuntimeSettingsStatus("failed", settings.Version, "agent_guard_runtime_settings_persist_failed", enabledInjections)
		return err
	}
	adapter := m.currentToolAdapter()
	// Runtime settings are the control-plane switch for the Hook adapter. The
	// bundle default is a provisioning hint and must not disable an explicitly
	// enabled runtime adapter.
	m.toolEnabled.Store(m.effectiveToolAdapterEnabled() && adapter != nil && m.cfg.ToolHookSocket != "")
	m.sessionEnabled.Store(m.effectiveSessionHookEnabled() && adapter != nil && m.cfg.ToolHookSocket != "")
	if err := m.setToolIngressDesired(m.toolEnabled.Load() || m.sessionEnabled.Load()); err != nil {
		m.queueRuntimeSettingsStatus("failed", settings.Version, errorCode(err), enabledInjections)
		return err
	}
	m.queueRuntimeSettingsStatus("applied", settings.Version, "", enabledInjections)
	logger.Info("agent_guard_runtime_settings_applied",
		zap.String("host_id", m.cfg.HostID), zap.Int64("settings_version", settings.Version),
		zap.Bool("tool_adapter_enabled", settings.ToolAdapterEnabled),
		zap.Bool("session_hook_enabled", settings.SessionHookEnabled),
		zap.Bool("behavior_policy_enabled", settings.BehaviorPolicyEnabled),
		zap.Bool("escape_policy_enabled", settings.EscapePolicyEnabled),
		zap.Int("injection_count", len(enabledInjections)))
	return nil
}

func hasScopedPolicyFields(settings RuntimeSettings) bool {
	for _, injection := range settings.Injections {
		if injection.BehaviorEnabled || injection.EscapeEnabled {
			return true
		}
	}
	return false
}

var runtimeHookAgentTypes = []string{"codex", "claude-code", "openclaw", "hermes", "zcode"}

func (m *Manager) applyHookInjections(settings RuntimeSettings) ([]string, error) {
	desired := make(map[string]bool, len(settings.Injections)*2)
	for _, injection := range settings.Injections {
		behavior := injection.BehaviorEnabled
		escape := injection.EscapeEnabled
		if injection.Enabled && !behavior && !escape {
			behavior = true
		}
		desired[injection.AgentType+"\x00behavior"] = settings.BehaviorPolicyEnabled && behavior
		desired[injection.AgentType+"\x00escape"] = settings.EscapePolicyEnabled && escape
	}

	m.hookStateMu.Lock()
	defer m.hookStateMu.Unlock()

	enabled := make([]string, 0, len(runtimeHookAgentTypes))
	for _, agentType := range runtimeHookAgentTypes {
		for _, scope := range []string{"behavior", "escape"} {
			key := agentType + "\x00" + scope
			if !desired[key] {
				continue
			}
			if !m.hookStates[key] {
				if m.scopedHookProvisioner != nil {
					if err := m.scopedHookProvisioner(agentType, scope); err != nil {
						logger.Warn("agent_guard_hook_provision_failed", zap.String("host_id", m.cfg.HostID), zap.String("agent_type", agentType), zap.String("scope", scope), zap.String("error_code", errorCode(err)))
						return enabled, err
					}
				} else if scope == "behavior" && m.hookProvisioner != nil {
					if err := m.hookProvisioner(agentType); err != nil {
						return enabled, err
					}
				} else {
					return enabled, errors.New("agent_guard_hook_provisioner_unavailable")
				}
				m.hookStates[key] = true
			}
			if scope == "behavior" {
				enabled = append(enabled, agentType)
			} else {
				enabled = append(enabled, agentType+":escape")
			}
		}
	}

	for _, agentType := range runtimeHookAgentTypes {
		for _, scope := range []string{"behavior", "escape"} {
			key := agentType + "\x00" + scope
			if desired[key] || !m.hookStates[key] {
				continue
			}
			if m.scopedHookRemover != nil {
				if err := m.scopedHookRemover(agentType, scope); err != nil {
					logger.Warn("agent_guard_hook_remove_failed", zap.String("host_id", m.cfg.HostID), zap.String("agent_type", agentType), zap.String("scope", scope), zap.String("error_code", errorCode(err)))
					return enabled, err
				}
			} else if scope == "behavior" && m.hookRemover != nil {
				if err := m.hookRemover(agentType); err != nil {
					return enabled, err
				}
			} else {
				return enabled, errors.New("agent_guard_hook_remover_unavailable")
			}
			delete(m.hookStates, key)
		}
	}
	return enabled, nil
}

func (m *Manager) loadPersistedRuntimeSettings() error {
	data, err := os.ReadFile(filepath.Join(m.cfg.StateDir, "runtime-settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("agent_guard_runtime_settings_read_failed")
	}
	settings, err := decodeRuntimeSettings(string(data))
	if err != nil || settings.HostID != m.cfg.HostID {
		return errors.New("agent_guard_runtime_settings_persisted_invalid")
	}
	if !settings.BehaviorPolicyEnabled && !settings.EscapePolicyEnabled {
		settings.BehaviorPolicyEnabled = settings.ToolAdapterEnabled || settings.SessionHookEnabled
		for _, injection := range settings.Injections {
			settings.BehaviorPolicyEnabled = settings.BehaviorPolicyEnabled || injection.Enabled || injection.BehaviorEnabled
		}
		settings.EscapePolicyEnabled = true
	}
	m.runtimeToolAdapterEnabled.Store(settings.ToolAdapterEnabled)
	m.runtimeSessionHookEnabled.Store(settings.SessionHookEnabled)
	m.runtimeSettingsVersion.Store(settings.Version)
	m.runtimeSettingsSet.Store(true)
	m.hookStateMu.Lock()
	for _, injection := range settings.Injections {
		behavior := injection.BehaviorEnabled || (injection.Enabled && !injection.EscapeEnabled)
		if settings.BehaviorPolicyEnabled && behavior {
			m.hookStates[injection.AgentType+"\x00behavior"] = true
		} else {
			delete(m.hookStates, injection.AgentType+"\x00behavior")
		}
		if settings.EscapePolicyEnabled && injection.EscapeEnabled {
			m.hookStates[injection.AgentType+"\x00escape"] = true
		} else {
			delete(m.hookStates, injection.AgentType+"\x00escape")
		}
	}
	m.hookStateMu.Unlock()
	return nil
}

func (m *Manager) persistRuntimeSettings(settings RuntimeSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return errors.New("agent_guard_runtime_settings_encode_failed")
	}
	if err := atomicWrite(filepath.Join(m.cfg.StateDir, "runtime-settings.json"), append(data, '\n'), 0o600); err != nil {
		return errors.New("agent_guard_runtime_settings_write_failed")
	}
	return nil
}

// RebindHostID applies the canonical identity returned by the server before
// discovery starts. Runtime IDs and bundle scope validation must never use the
// temporary ID generated by a fresh local installation.
func (m *Manager) RebindHostID(hostID string) error {
	hostID = strings.TrimSpace(hostID)
	if hostID == "" {
		return errors.New("agent_guard_host_id_invalid")
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.cancel != nil {
		return errors.New("agent_guard_host_id_rebind_after_start")
	}
	if m.cfg.HostID == hostID {
		return nil
	}
	previous := m.cfg.HostID
	m.cfg.HostID = hostID
	profiles := NewBuiltinProfileRegistry()
	m.tracker = NewIdentityTracker(hostID, profiles)
	m.reconciler = NewReconciler(m.tracker)
	m.normalizer = NewBehaviorNormalizer(hostID, m.scanner.BootID(), m.tracker)
	m.bundles = NewBundleStoreWithOptions(m.cfg.StateDir, hostID, BundleValidationOptions{
		EnforcementAllowed: m.cfg.EnforcementEnabled,
		FreezeAllowed:      m.cfg.FreezeEnabled,
		ToolAdapterAllowed: m.cfg.ToolAdapterEnabled,
	})
	logger.Info("agent_guard_host_identity_rebound",
		zap.String("previous_host_id", previous),
		zap.String("host_id", hostID))
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	if !m.cfg.Enabled {
		logger.Info("agent_guard_disabled", zap.String("host_id", m.cfg.HostID))
		return nil
	}
	if err := m.loadPersistedRuntimeSettings(); err != nil {
		logger.Warn("agent_guard_runtime_settings_restore_failed",
			zap.String("host_id", m.cfg.HostID), zap.String("error_code", errorCode(err)))
	}
	if (m.effectiveToolAdapterEnabled() || m.effectiveSessionHookEnabled()) && m.currentToolAdapter() == nil && m.cfg.ToolSourceManifest != "" {
		adapter, err := LoadTrustedToolAdapter(m.cfg.ToolSourceManifest)
		if err != nil {
			return err
		}
		m.replaceToolAdapter(adapter)
	}
	var loadedBundle *Bundle
	if bundle, err := m.bundles.Load(); err == nil {
		loadedBundle = &bundle
		m.cfg.ReconcileInterval = time.Duration(bundle.Defaults.ReconcileIntervalSeconds) * time.Second
		m.monitorEnabled.Store(m.monitorAllowed && bundle.Defaults.BehaviorMonitorEnabled)
		if !m.runtimeSettingsSet.Load() {
			m.escapeEnabled.Store(bundle.Defaults.EscapePolicyEnabled || len(bundle.EscapeRules) > 0)
		}
		m.bundleToolAdapterEnabled.Store(bundle.Defaults.ToolAdapterEnabled)
		m.toolEnabled.Store(m.effectiveToolAdapterEnabled() && bundle.Defaults.ToolAdapterEnabled &&
			m.currentToolAdapter() != nil && m.cfg.ToolHookSocket != "")
		logger.Info("agent_guard_last_known_good_loaded",
			zap.String("host_id", m.cfg.HostID),
			zap.Int64("bundle_version", bundle.BundleVersion),
			zap.String("bundle_digest", bundle.Digest))
	} else if !errors.Is(err, os.ErrNotExist) {
		logger.Warn("agent_guard_last_known_good_load_failed",
			zap.String("host_id", m.cfg.HostID),
			zap.String("error_code", errorCode(err)))
	}
	m.sessionEnabled.Store(m.effectiveSessionHookEnabled() && m.currentToolAdapter() != nil && m.cfg.ToolHookSocket != "")
	if err := m.reconcileOnce(); err != nil {
		return err
	}
	toolStatus, toolReason := "ready", ""
	if !m.toolEnabled.Load() {
		toolStatus, toolReason = "tool_semantics_unobservable", "trusted_tool_source_not_enabled"
	}
	m.queueStatusEvent("agent_guard_tool_adapter_status", map[string]any{
		"schema": GuardSchemaV1, "status": toolStatus, "reason": toolReason,
		"occurred_at": time.Now().UTC(),
	}, "info")
	if loadedBundle != nil {
		compiled, err := CompileKernelPolicy(*loadedBundle)
		if err != nil {
			return err
		}
		if err := m.applyCompiledKernelPolicy(*loadedBundle, compiled); err != nil {
			return err
		}
	}
	if err := m.setToolIngressDesired(m.toolEnabled.Load() || m.sessionEnabled.Load()); err != nil {
		return err
	}
	logger.Info("agent_guard_started",
		zap.String("host_id", m.cfg.HostID),
		zap.Bool("behavior_monitor_enabled", m.monitorEnabled.Load()),
		zap.Bool("session_hook_enabled", m.sessionEnabled.Load()),
		zap.String("coverage", string(m.currentEnforcementCoverage())),
		zap.Bool("namespace_read", m.capabilities.NamespaceRead),
		zap.Bool("mountinfo_read", m.capabilities.MountInfoRead),
		zap.Bool("bpf_lsm", m.capabilities.BPFLSM),
		zap.Bool("enforcement_enabled", m.cfg.EnforcementEnabled),
		zap.Bool("freeze_enabled", m.cfg.FreezeEnabled),
		zap.Int("supported_monitor_hooks", len(m.capabilities.SupportedHooks)),
		zap.Int("degraded_reason_count", len(m.capabilities.DegradedReasons)),
		zap.Int("reconcile_interval_seconds", int(m.cfg.ReconcileInterval.Seconds())))
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(runCtx)
	}()
	return nil
}

func (m *Manager) Stop() {
	m.monitorEnabled.Store(false)
	m.escapeEnabled.Store(false)
	m.toolEnabled.Store(false)
	_ = m.setToolIngressDesired(false)
	if m.cancel != nil {
		m.cancel()
		m.wg.Wait()
	}
}

func (m *Manager) run(ctx context.Context) {
	reconcileTicker := time.NewTicker(m.cfg.ReconcileInterval)
	flushTicker := time.NewTicker(m.cfg.FlushInterval)
	defer reconcileTicker.Stop()
	defer flushTicker.Stop()
	defer logger.Info("agent_guard_stopped", zap.String("host_id", m.cfg.HostID))
	for {
		select {
		case <-ctx.Done():
			m.enqueueAggregates(m.aggregator.Flush(time.Now().Add(m.aggregator.window)))
			m.flush()
			return
		case <-reconcileTicker.C:
			if err := m.reconcileOnce(); err != nil {
				logger.Warn("agent_guard_reconcile_failed",
					zap.String("host_id", m.cfg.HostID),
					zap.String("error_code", errorCode(err)))
			}
		case interval := <-m.reconcileReset:
			reconcileTicker.Reset(interval)
		case now := <-flushTicker.C:
			m.enqueueAggregates(m.aggregator.Flush(now))
			m.flush()
		}
	}
}

func (m *Manager) ApplyBundle(payload string) error {
	if !m.cfg.Enabled {
		return errors.New("agent_guard_disabled")
	}
	candidateVersion, candidateDigest := bundleIdentity(payload)
	if candidateVersion > 0 {
		m.queueConfigStatus("received", candidateVersion, candidateDigest, "")
		m.queueConfigStatus("validating", candidateVersion, candidateDigest, "")
	}
	bundle, err := m.bundles.ValidatePayload([]byte(payload))
	if err != nil {
		code := errorCode(err)
		if candidateVersion > 0 {
			m.queueConfigStatus("rejected", candidateVersion, candidateDigest, code)
		}
		logger.Warn("agent_guard_bundle_rejected",
			zap.String("host_id", m.cfg.HostID),
			zap.String("error_code", code))
		return err
	}
	compiled, err := CompileKernelPolicy(bundle)
	if err != nil {
		code := errorCode(err)
		m.queueConfigStatus("rejected", bundle.BundleVersion, bundle.Digest, code)
		return err
	}
	previous, previousErr := m.bundles.Load()
	if err := m.applyCompiledKernelPolicy(bundle, compiled); err != nil {
		code := errorCode(err)
		m.queueConfigStatus("rejected", bundle.BundleVersion, bundle.Digest, code)
		logger.Warn("agent_guard_kernel_policy_apply_failed",
			zap.String("host_id", m.cfg.HostID),
			zap.Int64("bundle_version", bundle.BundleVersion),
			zap.String("error_code", code))
		return err
	}
	previousToolEnabled := m.toolEnabled.Load()
	// Tool rule matching is performed by api-server. A newly applied policy
	// bundle may still explicitly disable the adapter and must clean up its
	// ingress; the runtime settings path above is the immediate UI switch.
	desiredToolEnabled := m.effectiveToolAdapterEnabled() && bundle.Defaults.ToolAdapterEnabled &&
		m.currentToolAdapter() != nil && m.cfg.ToolHookSocket != ""
	if desiredToolEnabled || m.sessionEnabled.Load() {
		if err := m.setToolIngressDesired(true); err != nil {
			if previousErr == nil {
				if rollback, compileErr := CompileKernelPolicy(previous); compileErr == nil {
					_ = m.applyCompiledKernelPolicy(previous, rollback)
				}
			}
			return err
		}
	}
	bundle, err = m.bundles.CommitValidated(bundle)
	if err != nil {
		_ = m.setToolIngressDesired(previousToolEnabled || m.sessionEnabled.Load())
		if previousErr == nil {
			if rollback, compileErr := CompileKernelPolicy(previous); compileErr == nil {
				_ = m.applyCompiledKernelPolicy(previous, rollback)
			}
		}
		code := errorCode(err)
		m.queueConfigStatus("rejected", candidateVersion, candidateDigest, code)
		return err
	}
	m.bundleToolAdapterEnabled.Store(bundle.Defaults.ToolAdapterEnabled)
	if bundle.Defaults.ReconcileIntervalSeconds > 0 {
		interval := time.Duration(bundle.Defaults.ReconcileIntervalSeconds) * time.Second
		select {
		case m.reconcileReset <- interval:
		default:
			select {
			case <-m.reconcileReset:
			default:
			}
			m.reconcileReset <- interval
		}
	}
	m.monitorEnabled.Store(m.monitorAllowed && bundle.Defaults.BehaviorMonitorEnabled)
	if !m.runtimeSettingsSet.Load() {
		m.escapeEnabled.Store(bundle.Defaults.EscapePolicyEnabled || len(bundle.EscapeRules) > 0)
	}
	m.toolEnabled.Store(desiredToolEnabled)
	m.sessionEnabled.Store(m.effectiveSessionHookEnabled() && m.currentToolAdapter() != nil && m.cfg.ToolHookSocket != "")
	if err := m.setToolIngressDesired(desiredToolEnabled || m.sessionEnabled.Load()); err != nil {
		return err
	}
	m.actions.SetFreezeTimeout(time.Duration(bundle.Defaults.FreezeTimeoutSeconds) * time.Second)
	m.queueConfigStatus("applied", bundle.BundleVersion, bundle.Digest, "")
	logger.Info("agent_guard_bundle_applied",
		zap.String("host_id", m.cfg.HostID),
		zap.Int64("bundle_version", bundle.BundleVersion),
		zap.String("bundle_digest", bundle.Digest),
		zap.Int("profile_count", len(bundle.Profiles)),
		zap.Int("policy_count", len(bundle.Policies)),
		zap.Int("escape_rule_count", len(bundle.EscapeRules)),
		zap.Bool("escape_policy_enabled", m.escapeEnabled.Load()))
	return nil
}

func bundleIdentity(payload string) (int64, string) {
	var identity struct {
		BundleVersion int64  `json:"bundle_version"`
		Digest        string `json:"digest"`
	}
	if err := json.Unmarshal([]byte(payload), &identity); err != nil {
		return 0, ""
	}
	if len(identity.Digest) > 80 {
		identity.Digest = ""
	}
	return identity.BundleVersion, identity.Digest
}

func (m *Manager) CurrentBundle() (Bundle, error) {
	return m.bundles.Load()
}

func (m *Manager) Tracker() *IdentityTracker {
	return m.tracker
}

func (m *Manager) SetKernelPolicyApplier(
	applier func(CompiledKernelPolicy, []KernelSubject) error,
) {
	m.kernelPolicyMu.Lock()
	m.kernelApply = applier
	m.kernelPolicyMu.Unlock()
}

func (m *Manager) applyCompiledKernelPolicy(bundle Bundle, compiled CompiledKernelPolicy) error {
	m.kernelPolicyMu.RLock()
	applier := m.kernelApply
	m.kernelPolicyMu.RUnlock()
	if !bundle.Defaults.EnforcementEnabled {
		compiled = CompiledKernelPolicy{BundleVersion: bundle.BundleVersion, BundleDigest: bundle.Digest}
	}
	if !m.cfg.EnforcementEnabled {
		m.kernelPolicyMu.Lock()
		m.kernelPolicy = compiled
		m.kernelPolicyMu.Unlock()
		return nil
	}
	if bundle.Defaults.EnforcementEnabled && !m.capabilities.BPFLSM {
		logger.Warn("agent_guard_enforcement_unavailable",
			zap.String("host_id", m.cfg.HostID),
			zap.Int64("bundle_version", bundle.BundleVersion),
			zap.String("error_code", "bpf_lsm_unavailable"))
		m.kernelPolicyMu.Lock()
		m.kernelPolicy = compiled
		m.kernelPolicyMu.Unlock()
		return nil
	}
	if applier == nil {
		if bundle.Defaults.EnforcementEnabled {
			return errors.New("agent_guard_lsm_applier_unavailable")
		}
		return nil
	}
	if err := applier(compiled, m.tracker.KernelSubjects(compiled)); err != nil {
		return err
	}
	m.kernelPolicyMu.Lock()
	m.kernelPolicy = compiled
	m.kernelPolicyMu.Unlock()
	return nil
}

func (m *Manager) syncKernelSubjects() {
	m.kernelPolicyMu.RLock()
	compiled := m.kernelPolicy
	applier := m.kernelApply
	m.kernelPolicyMu.RUnlock()
	if applier == nil || compiled.BundleVersion == 0 || !m.capabilities.BPFLSM {
		return
	}
	if err := applier(compiled, m.tracker.KernelSubjects(compiled)); err != nil {
		logger.Warn("agent_guard_lsm_subject_sync_failed",
			zap.String("host_id", m.cfg.HostID),
			zap.Int64("bundle_version", compiled.BundleVersion),
			zap.String("error_code", errorCode(err)))
	}
}

func (m *Manager) Capabilities() GuardCapabilities {
	return m.capabilities
}

func (m *Manager) ObserveTrustedToolPayload(payload []byte) (BehaviorEvent, error) {
	adapter := m.currentToolAdapter()
	if !m.cfg.Enabled || !m.monitorEnabled.Load() || !m.toolEnabled.Load() || adapter == nil {
		return BehaviorEvent{}, errors.New("tool_semantics_unobservable")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var input TrustedToolEvent
	if err := decoder.Decode(&input); err != nil {
		return BehaviorEvent{}, errors.New("agent_guard_tool_event_json_invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return BehaviorEvent{}, errors.New("agent_guard_tool_event_json_invalid")
	}
	verified, err := adapter.Verify(input)
	if err != nil {
		return BehaviorEvent{}, err
	}
	process, err := m.scanner.ReadPID(input.PID)
	if err != nil || process.Identity != (ProcessIdentity{PID: input.PID, StartTicks: input.StartTicks}) {
		return BehaviorEvent{}, errors.New("agent_guard_tool_process_identity_stale")
	}
	subject, ok := m.tracker.Attribute(process)
	if !ok || subject.Confidence != ConfidenceConfirmed {
		return BehaviorEvent{}, errors.New("agent_guard_tool_process_not_confirmed")
	}
	session, sessionChanged := m.tracker.ObserveTrustedSession(
		subject, verified.Source.SourceType, verified.CorrelationHash, input.ExternalSessionID,
	)
	if session.SessionID == "" {
		if input.Operation == ToolEventStarted {
			recoveredSession, recoveredUnit, recovered, startErr := m.tracker.StartTrustedSession(
				process, subject, verified.Source.SourceType, input.ExternalSessionID, input.OccurredAt,
			)
			if startErr == nil {
				session = recoveredSession
				sessionChanged = recovered
				m.queueTrustedSessionStarted(recoveredSession, recoveredUnit, recovered)
				m.syncKernelSubjects()
				logger.Info("agent_guard_trusted_session_recovered_from_tool",
					zap.String("host_id", m.cfg.HostID),
					zap.String("instance_id", recoveredSession.InstanceID),
					zap.String("session_id", recoveredSession.SessionID),
					zap.String("execution_unit_id", recoveredUnit.UnitID),
					zap.String("external_session_ref", stableID("external_session", input.ExternalSessionID)),
					zap.Uint32("root_pid", input.PID),
					zap.String("source", string(verified.Source.SourceType)),
				)
			} else {
				logger.Warn("agent_guard_tool_session_recovery_failed",
					zap.String("host_id", m.cfg.HostID),
					zap.String("external_session_ref", stableID("external_session", input.ExternalSessionID)),
					zap.String("error_code", errorCode(startErr)),
				)
			}
		}
	}
	if session.SessionID == "" {
		logger.Warn("agent_guard_tool_session_unavailable",
			zap.String("host_id", m.cfg.HostID),
			zap.String("external_session_ref", stableID("external_session", input.ExternalSessionID)),
			zap.String("subject_session_id", subject.SessionID),
			zap.String("operation", input.Operation),
			zap.String("tool_name", input.ToolName),
			zap.Uint32("hook_pid", input.PID),
			zap.Uint64("hook_start_ticks", input.StartTicks),
		)
		return BehaviorEvent{}, errors.New("agent_guard_tool_session_unavailable")
	}
	m.queueTrustedSessionStatus(session, sessionChanged)
	parentEventID := ""
	evidence, evidenceMatched := adapter.Evidence(input.ToolCallID)
	if evidenceMatched && evidence.Link.CorrelationHash != verified.CorrelationHash {
		evidenceMatched = false
	}
	if evidenceMatched {
		parentEventID = evidence.Link.ToolEventID
	}
	if evidenceMatched && evidence.Representative.Identity.Valid() {
		if candidate, readErr := m.scanner.ReadPID(evidence.Representative.Identity.PID); readErr == nil &&
			candidate.Identity == evidence.Representative.Identity {
			process = candidate
		}
	}
	attributes := map[string]any{"tool_call_id": input.ToolCallID}
	if input.TurnID != "" {
		attributes["turn_id"] = input.TurnID
	}
	if value := trustedJSONValue(input.ToolInput); value != nil {
		attributes["tool_input"] = value
	}
	if value := trustedJSONValue(input.ToolResponse); value != nil {
		attributes["tool_response"] = value
	}
	commandText := toolCommandText(input.ToolName, input.ToolInput)
	if commandText != "" {
		attributes["command"] = commandText
	}
	if evidenceMatched && evidence.Representative.Identity.Valid() {
		attributes["correlation_status"] = "matched"
		attributes["correlation_method"] = evidence.Method
		attributes["correlated_process_pid"] = evidence.Representative.Identity.PID
		attributes["correlated_process_start_ticks"] = evidence.Representative.Identity.StartTicks
	} else {
		attributes["correlation_status"] = "unmatched"
		attributes["correlation_method"] = "ebpf_unresolved"
	}
	processEventID := input.ProcessEventID
	resourceEventIDs := append([]string(nil), input.ResourceEventIDs...)
	if evidenceMatched {
		if evidence.ProcessEventID != "" {
			processEventID = evidence.ProcessEventID
		}
		if len(evidence.ResourceEventIDs) > 0 {
			resourceEventIDs = append([]string(nil), evidence.ResourceEventIDs...)
		}
	}
	if processEventID != "" {
		attributes["process_event_id"] = processEventID
	}
	if len(resourceEventIDs) > 0 {
		attributes["resource_event_ids"] = resourceEventIDs
	}
	remoteCoverage := string(CoverageRemoteUnobservable)
	if verified.Remote != nil {
		attributes["remote_host_id"] = verified.Remote.RemoteHostID
		attributes["remote_execution_unit_id"] = verified.Remote.RemoteExecutionUnitID
		attributes["remote_sensor_event_id"] = verified.Remote.EventID
	}
	outcome := OutcomeUnknown
	if input.Operation == ToolEventCompleted {
		outcome = OutcomeSuccess
	} else if input.Operation == ToolEventFailed {
		outcome = OutcomeFailed
	}
	// Tool calls are behavior facts, not lifecycle/state events. Keep the
	// canonical Agent Guard event type so the server/DC projection path stores
	// them in agent_behavior_events; category=tool carries the tool semantic.
	event, accepted := m.observeRawEvent(RawBehavior{
		EventID: input.EventID, EventType: "agent_behavior", SessionID: session.SessionID,
		CorrelationID: verified.CorrelationHash, ParentEventID: parentEventID,
		OccurredAt: input.OccurredAt, Category: CategoryTool, Operation: input.Operation,
		Outcome: outcome, Process: process, Argv: process.Argv,
		Resource: Resource{Type: "tool", Identity: input.ToolName, Attributes: attributes},
		Source:   verified.Source.SourceType, Sensor: verified.Source.SourceID, Visibility: "complete",
		Decision: DecisionAudit, Severity: "info",
		Evidence: map[string]any{
			"trusted_proof":          verified.Proof,
			"correlation_token_hash": verified.CorrelationHash,
			"remote_coverage":        remoteCoverage,
		},
	})
	if !accepted {
		return BehaviorEvent{}, errors.New("agent_guard_tool_event_unattributed")
	}
	if input.Operation == ToolEventStarted {
		adapter.Bind(process.Identity, toolCorrelationLink{
			CorrelationHash: verified.CorrelationHash, ToolEventID: event.EventID,
			ToolCallID: input.ToolCallID, SessionID: session.SessionID,
			CommandText: commandText,
			ExpiresAt:   input.OccurredAt.Add(30 * time.Minute),
		})
	} else {
		adapter.CompleteToolCall(input.ToolCallID)
	}
	return event, nil
}

func trustedJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func toolCommandText(toolName string, raw json.RawMessage) string {
	value := trustedJSONValue(raw)
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"command", "cmdline", "command_line", "script"} {
			if command, ok := object[key].(string); ok && strings.TrimSpace(command) != "" {
				return strings.TrimSpace(command)
			}
		}
	}
	if value == nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(toolName)
	}
	return string(encoded)
}

// ObserveTrustedPayload routes the shared signed hook socket without inferring
// lifecycle or tool semantics from process names.
func (m *Manager) ObserveTrustedPayload(payload []byte) (BehaviorEvent, error) {
	var header struct {
		Operation string `json:"operation"`
	}
	if json.Unmarshal(payload, &header) != nil {
		return BehaviorEvent{}, errors.New("agent_guard_hook_event_json_invalid")
	}
	switch header.Operation {
	case SessionEventStarted, SessionEventActivated, SessionEventEnded:
		return m.ObserveTrustedSessionPayload(payload)
	default:
		return m.ObserveTrustedToolPayload(payload)
	}
}

func (m *Manager) ObserveTrustedSessionPayload(payload []byte) (BehaviorEvent, error) {
	adapter := m.currentToolAdapter()
	if !m.cfg.Enabled || !m.monitorEnabled.Load() || !m.sessionEnabled.Load() || adapter == nil {
		return BehaviorEvent{}, errors.New("session_lifecycle_unobservable")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var input TrustedSessionEvent
	if err := decoder.Decode(&input); err != nil {
		return BehaviorEvent{}, errors.New("agent_guard_session_event_json_invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return BehaviorEvent{}, errors.New("agent_guard_session_event_json_invalid")
	}
	verified, err := adapter.VerifySession(input)
	if err != nil {
		return BehaviorEvent{}, err
	}

	root := ProcessIdentity{PID: input.PID, StartTicks: input.StartTicks}
	if input.Operation == SessionEventEnded {
		session, unit, changed, endErr := m.tracker.EndTrustedSession(
			verified.Source.SourceType, input.ExternalSessionID, root, input.OccurredAt,
		)
		if endErr != nil {
			return BehaviorEvent{}, endErr
		}
		if changed {
			m.queueExecutionUnitStatus(unit, "agent_execution_unit_stopped")
			m.queueSessionStatus(session, "agent_behavior_session_stopped")
			m.syncKernelSubjects()
			logger.Info("agent_guard_trusted_session_ended",
				zap.String("host_id", m.cfg.HostID),
				zap.String("instance_id", session.InstanceID),
				zap.String("session_id", session.SessionID),
				zap.String("external_session_ref", stableID("external_session", input.ExternalSessionID)),
				zap.Uint32("root_pid", input.PID),
				zap.String("source", verified.Source.SourceType))
		}
		return BehaviorEvent{}, nil
	}

	process, err := m.scanner.ReadPID(input.PID)
	if err != nil || process.Identity != root {
		return BehaviorEvent{}, errors.New("agent_guard_session_process_identity_stale")
	}
	subject, ok := m.tracker.Attribute(process)
	if !ok {
		subject = m.tracker.OnExec(process)
		ok = subject.InstanceID != ""
	}
	if !ok || subject.Confidence != ConfidenceConfirmed {
		return BehaviorEvent{}, errors.New("agent_guard_session_process_not_confirmed")
	}
	session, unit, changed, err := m.tracker.StartTrustedSession(
		process, subject, verified.Source.SourceType, input.ExternalSessionID, input.OccurredAt,
	)
	if err != nil {
		return BehaviorEvent{}, err
	}
	m.queueTrustedSessionStarted(session, unit, changed)
	m.syncKernelSubjects()
	if input.Operation == SessionEventActivated && !changed {
		return BehaviorEvent{}, nil
	}
	logger.Info("agent_guard_trusted_session_started",
		zap.String("host_id", m.cfg.HostID),
		zap.String("instance_id", session.InstanceID),
		zap.String("session_id", session.SessionID),
		zap.String("execution_unit_id", unit.UnitID),
		zap.String("external_session_ref", stableID("external_session", input.ExternalSessionID)),
		zap.Uint32("root_pid", input.PID),
		zap.String("source", verified.Source.SourceType))

	event, accepted := m.observeRawEvent(RawBehavior{
		EventID: input.EventID, EventType: "agent_behavior", OccurredAt: input.OccurredAt,
		Category: CategoryProcess, Operation: "session_root", Outcome: OutcomeSuccess,
		Process: process, Argv: process.Argv,
		Resource: Resource{Type: "process", Identity: process.Exe},
		Source:   verified.Source.SourceType, Sensor: lifecycleSensor(input.Operation), Visibility: "complete",
		Decision: DecisionAudit, Severity: "info",
		Evidence: map[string]any{"trusted_proof": verified.Proof},
	})
	if !accepted {
		return BehaviorEvent{}, errors.New("agent_guard_session_root_unattributed")
	}
	return event, nil
}

func lifecycleSensor(operation string) string {
	if operation == SessionEventActivated {
		return "PreToolUse"
	}
	return "SessionStart"
}

func (m *Manager) queueTrustedSessionStarted(session BehaviorSession, unit ExecutionUnit, changed bool) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	wasSessionStarted := m.startedSessions[session.SessionID]
	wasUnitStarted := m.startedUnits[unit.UnitID]
	m.queuePendingLifecyclesLocked()
	if changed && wasUnitStarted {
		m.queueExecutionUnitStatus(unit, "agent_execution_unit_updated")
	}
	if changed && wasSessionStarted {
		m.queueSessionStatus(session, "agent_behavior_session_updated")
	}
}

func (m *Manager) setToolIngressDesired(enabled bool) error {
	m.toolIngressMu.Lock()
	if !enabled {
		ingress := m.toolIngress
		m.toolIngress = nil
		m.toolIngressMu.Unlock()
		if ingress != nil {
			ingress.Stop()
		}
		return nil
	}
	defer m.toolIngressMu.Unlock()
	if m.toolIngress != nil {
		return nil
	}
	adapter := m.currentToolAdapter()
	if adapter == nil {
		return errors.New("agent_guard_tool_adapter_unavailable")
	}
	ingress, err := StartToolHookReceiver(
		m.cfg.ToolHookSocket, adapter.SocketPolicy(),
		adapter.AuthorizePeer, m.ObserveTrustedPayload,
	)
	if err != nil {
		return err
	}
	m.toolIngress = ingress
	logger.Info("agent_guard_tool_hook_started",
		zap.String("host_id", m.cfg.HostID),
		zap.String("socket_path", m.cfg.ToolHookSocket))
	return nil
}

// ExecuteAgentGuardAction handles only the V6.2 UUID-scoped Agent Guard action
// family. Real cgroup paths and PID/start_ticks identities are always resolved
// from the local registry by ActionExecutor.
func (m *Manager) ExecuteAgentGuardAction(
	ctx context.Context,
	commandID, action, target, _ string,
) (ActionResult, error) {
	if !m.cfg.Enabled {
		result := ActionResult{CommandID: commandID, Action: action, Status: ActionStatusFailed, ErrorCode: "agent_guard_disabled"}
		m.reportActionResult(result)
		return result, errors.New(result.ErrorCode)
	}
	result, err := m.actions.Execute(ctx, commandID, action, target)
	m.reportActionResult(result)
	return result, err
}

func (m *Manager) reportActionResult(result ActionResult) {
	if result.ActionID == "" {
		result.ActionID = actionID(result.CommandID, result.Action, result.ExecutionUnitID, result.InstanceID)
	}
	body := map[string]any{
		"schema":            GuardSchemaV1,
		"action_id":         result.ActionID,
		"command_id":        result.CommandID,
		"action":            result.Action,
		"instance_id":       result.InstanceID,
		"execution_unit_id": result.ExecutionUnitID,
		"status":            result.Status,
		"method":            result.Method,
		"degraded":          result.Degraded,
		"auto_resume":       result.AutoResume,
		"executed":          result.Executed,
		"state_changed":     result.StateChanged,
		"error_code":        result.ErrorCode,
		"occurred_at":       time.Now().UTC(),
	}
	severity := "info"
	if result.Status == ActionStatusFailed {
		severity = "high"
	}
	m.queueStatusEvent("agent_guard_action_status", body, severity)
	logger.Info("agent_guard_action_result",
		zap.String("host_id", m.cfg.HostID),
		zap.String("action_id", result.ActionID),
		zap.String("command_id", result.CommandID),
		zap.String("action", result.Action),
		zap.String("instance_id", result.InstanceID),
		zap.String("execution_unit_id", result.ExecutionUnitID),
		zap.String("status", result.Status),
		zap.String("method", result.Method),
		zap.Bool("degraded", result.Degraded),
		zap.Bool("executed", result.Executed),
		zap.Bool("state_changed", result.StateChanged),
		zap.String("error_code", result.ErrorCode))
}

func actionID(commandID, action, unitID, instanceID string) string {
	trimmed := strings.TrimSpace(commandID)
	for _, candidate := range []string{
		trimmed,
		strings.TrimPrefix(trimmed, "AG-GUARD-"),
	} {
		if parsed, err := uuid.Parse(candidate); err == nil && parsed != uuid.Nil {
			return parsed.String()
		}
	}
	if trimmed == "" {
		return uuid.NewString()
	}
	return stableID("action", trimmed, action, unitID, instanceID)
}

func (m *Manager) AttachContainer(instanceID string, info ContainerCgroup) (ExecutionUnit, error) {
	unit, err := m.tracker.AttachContainer(instanceID, info)
	if err != nil {
		return ExecutionUnit{}, err
	}
	m.queuePendingLifecycles()
	return unit, nil
}

func (m *Manager) ObserveRaw(raw RawBehavior) bool {
	_, ok := m.observeRawEvent(raw)
	return ok
}

func (m *Manager) observeRawEvent(raw RawBehavior) (BehaviorEvent, bool) {
	if !m.cfg.Enabled || !m.monitorEnabled.Load() {
		return BehaviorEvent{}, false
	}
	adapter := m.currentToolAdapter()
	if raw.CorrelationID == "" && m.toolEnabled.Load() && adapter != nil {
		if link, ok := adapter.LookupForProcess(raw.Process.Identity, raw.Process); ok {
			raw.SessionID = link.SessionID
			raw.CorrelationID = link.CorrelationHash
			raw.ParentEventID = link.ToolEventID
			if raw.Evidence == nil {
				raw.Evidence = make(map[string]any)
			}
			raw.Evidence["correlation_token_hash"] = link.CorrelationHash
			raw.Evidence["tool_call_id"] = link.ToolCallID
		}
	}
	event, ok := m.normalizer.Normalize(raw)
	if !ok {
		return BehaviorEvent{}, false
	}
	if raw.CorrelationID != "" && raw.Category != CategoryTool && m.toolEnabled.Load() && adapter != nil {
		if link, linked := adapter.LookupForProcess(raw.Process.Identity, raw.Process); linked && link.CorrelationHash == raw.CorrelationID {
			adapter.RecordEvidence(raw.Process.Identity, event.EventID, string(raw.Category), raw.Operation, raw.Process)
		}
	}
	m.enqueueAggregates(m.aggregator.Add(event))
	return event, true
}

func (m *Manager) ObserveEventMap(eventMap map[string]any) bool {
	if !m.cfg.Enabled || !m.monitorEnabled.Load() {
		return false
	}
	pid := uint32(numberValue(eventMap["pid"]))
	if pid == 0 {
		return false
	}
	eventType := stringValue(eventMap["event_type"])
	if eventType == "process_exit" {
		return m.observeProcessExit(eventMap, pid)
	}
	process, err := m.scanner.ReadPID(pid)
	if err != nil {
		return false
	}
	switch eventType {
	case "process_fork":
		parent, err := m.scanner.ReadPID(process.PPID)
		if err == nil {
			m.tracker.OnFork(parent.Identity, process)
			if adapter := m.currentToolAdapter(); adapter != nil {
				adapter.OnForkProcess(parent.Identity, process)
			}
		}
	case "process_exec":
		m.tracker.OnExec(process)
	}
	if eventType == "process_fork" || eventType == "process_exec" {
		m.syncKernelSubjects()
		// A controller can be confirmed during exec attribution, before the
		// periodic reconcile loop. Queue its dependency chain now so Kafka
		// observes lifecycle state before the first behavior for that scope.
		m.queuePendingLifecycles()
	}
	if eventType == "agent_guard_syscall" {
		return m.observeGuardSemanticEvent(eventMap, process)
	}
	category, operation := mapBehaviorOperation(eventType, eventMap)
	if category == "" {
		return false
	}
	outcome := OutcomeSuccess
	if status := stringValue(eventMap["connect_status"]); status == "failed" {
		outcome = OutcomeFailed
	}
	resource := Resource{Type: string(category)}
	switch category {
	case CategoryFile:
		resource.Type = "file"
		resource.Identity = stringValue(eventMap["file_path"])
	case CategoryNetwork:
		resource.Type = "network"
		resource.Identity = stringValue(eventMap["remote_addr"])
		resource.Attributes = map[string]any{
			"destination_ip":   stringValue(eventMap["dst_ip"]),
			"destination_port": numberValue(eventMap["dst_port"]),
			"protocol":         stringValue(eventMap["network_transport"]),
			"content_observed": false,
		}
	case CategoryProcess:
		resource.Type = "process"
		resource.Identity = process.Exe
	}
	normalized, accepted := m.observeRawEvent(RawBehavior{
		EventID:    stringValue(eventMap["event_id"]),
		OccurredAt: time.Now().UTC(),
		Category:   category,
		Operation:  operation,
		Outcome:    outcome,
		Process:    process,
		Argv:       processCommandArgv(process, stringValue(eventMap["commandline"])),
		Resource:   resource,
		Source:     "ebpf",
		Sensor:     eventType,
		Visibility: visibilityForEvent(eventMap),
	})
	if !accepted {
		return false
	}
	if category == CategoryFile {
		m.observeEscapeViolation(GuardAttempt{
			EventID: normalized.EventID, Category: category, Operation: operation,
			Target: resource.Identity, ReturnCode: numberValue(eventMap["return_code"]),
		}, process, normalized)
	}
	return true
}

func processCommandArgv(process ProcessSnapshot, fallback string) []string {
	if len(process.Argv) > 0 {
		return append([]string(nil), process.Argv...)
	}
	return strings.Fields(fallback)
}

func (m *Manager) observeProcessExit(eventMap map[string]any, pid uint32) bool {
	process, _, ok := m.tracker.ProcessByPID(pid)
	if !ok {
		return false
	}
	if ppid := uint32(numberValue(eventMap["ppid"])); ppid > 0 {
		process.PPID = ppid
	}
	resource := Resource{Type: "process", Identity: process.Exe}
	_, accepted := m.observeRawEvent(RawBehavior{
		EventID: stringValue(eventMap["event_id"]), OccurredAt: time.Now().UTC(),
		OccurredMonotonicNS: uint64(numberValue(eventMap["timestamp_ns"])),
		Category:            CategoryProcess, Operation: "exit", Outcome: OutcomeSuccess,
		Process: process, Resource: resource, Source: "ebpf", Sensor: "process_exit",
		Visibility: visibilityForEvent(eventMap),
	})
	exit, exited := m.tracker.ExitPID(pid, time.Now().UTC())
	if exited && exit.InstanceStopped {
		m.queueStoppedLifecycle(exit, "ebpf_exit")
	}
	return accepted
}

func (m *Manager) queueStoppedLifecycle(exit ProcessExitResult, reason string) {
	if !exit.InstanceStopped {
		return
	}
	m.queueInstanceStatus(exit.Instance, "agent_instance_stopped")
	if exit.Unit.UnitID != "" {
		m.queueExecutionUnitStatus(exit.Unit, "agent_execution_unit_stopped")
	}
	if exit.Session.SessionID != "" {
		m.queueSessionStatus(exit.Session, "agent_behavior_session_stopped")
	}
	logger.Info("agent_guard_instance_stopped",
		zap.String("host_id", m.cfg.HostID),
		zap.String("instance_id", exit.Instance.InstanceID),
		zap.Uint32("controller_pid", exit.Process.Identity.PID),
		zap.String("reason", reason))
}

func (m *Manager) observeGuardSemanticEvent(eventMap map[string]any, process ProcessSnapshot) bool {
	category := Category(stringValue(eventMap["security_category"]))
	switch category {
	case CategoryProcess, CategoryNetwork, CategoryIdentity, CategoryKernel, CategoryIsolation:
	default:
		return false
	}
	operation := stringValue(eventMap["security_operation"])
	if operation == "" {
		return false
	}
	subject, ok := m.tracker.Attribute(process)
	if !ok {
		return false
	}
	unit, ok := m.tracker.Unit(subject.UnitID)
	if !ok {
		return false
	}
	baseline := unit.IsolationBaseline
	actual := unit.IsolationActual
	if m.isolationScan != nil {
		if observed, err := m.isolationScan.ReadIsolation(process.Identity.PID); err == nil {
			actual = observed
			if baseline.CapturedAt.IsZero() {
				baseline = observed
			}
			unit, _ = m.tracker.UpdateUnitIsolation(unit.UnitID, observed, m.capabilities)
		}
	}
	returnCode := numberValue(eventMap["return_code"])
	outcome := OutcomeSuccess
	errno := int32(0)
	if returnCode < 0 {
		outcome = OutcomeFailed
		if returnCode >= -1<<31 {
			errno = int32(-returnCode)
		}
	}
	target := stringValue(eventMap["security_target"])
	secondary := stringValue(eventMap["security_secondary"])
	diff := DiffIsolationState(baseline, actual)
	evidence := map[string]any{
		"syscall":       operation,
		"target":        target,
		"secondary":     secondary,
		"return_code":   returnCode,
		"baseline":      baseline,
		"actual":        actual,
		"diff":          diff,
		"state_changed": diff.StateChanged,
		"identity_before": map[string]any{
			"uid": numberValue(eventMap["uid"]),
			"gid": numberValue(eventMap["gid"]),
		},
		"identity_after": map[string]any{
			"uid": process.UID,
			"gid": process.GID,
		},
	}
	decision := DecisionAudit
	switch stringValue(eventMap["security_decision"]) {
	case string(DecisionDeny):
		decision = DecisionDeny
	case string(DecisionDenyAndFreeze):
		decision = DecisionDenyAndFreeze
	}
	severity := "info"
	if decision == DecisionDeny || decision == DecisionDenyAndFreeze {
		severity = "critical"
	}
	normalized, accepted := m.observeRawEvent(RawBehavior{
		EventID:             stringValue(eventMap["event_id"]),
		OccurredAt:          time.Now().UTC(),
		OccurredMonotonicNS: uint64(max(numberValue(eventMap["timestamp_ns"]), 0)),
		Category:            category,
		Operation:           operation + "_attempt",
		Outcome:             outcome,
		Errno:               errno,
		Process:             process,
		Argv:                process.Argv,
		Resource: Resource{
			Type: string(category), Identity: target,
			Attributes: map[string]any{
				"syscall": operation, "secondary_target": secondary,
				"return_code": returnCode, "content_observed": false,
			},
		},
		Source: "ebpf", Sensor: "agent_guard_syscall",
		Visibility: visibilityForEvent(eventMap), Decision: decision,
		Severity: severity,
		Isolation: map[string]any{
			"unit_type": unit.Type, "baseline_fingerprint": baseline.Fingerprint(),
			"actual_fingerprint": actual.Fingerprint(),
			"completeness":       actual.Completeness(),
		},
		Evidence: evidence,
	})
	if !accepted {
		return false
	}
	if decision == DecisionDenyAndFreeze &&
		normalized.AttributionConfidence == ConfidenceConfirmed &&
		normalized.ExecutionUnitID != "" {
		commandID := "AG-GUARD-" + uuid.NewString()
		unitID := normalized.ExecutionUnitID
		go func() {
			result, err := m.actions.Execute(context.Background(), commandID, ActionFreezeExecutionUnit, unitID)
			m.reportActionResult(result)
			if err != nil {
				logger.Warn("agent_guard_local_freeze_failed",
					zap.String("host_id", m.cfg.HostID),
					zap.String("execution_unit_id", unitID),
					zap.String("command_id", commandID),
					zap.String("error_code", errorCode(err)))
			}
		}()
	}
	attempt := GuardAttempt{
		EventID: normalized.EventID, Category: category, Operation: operation,
		Target: target, SecondaryTarget: secondary,
		TargetPID:  uint32(numberValue(eventMap["security_arg1"])),
		ReturnCode: returnCode, Baseline: baseline, Actual: actual,
	}
	if _, exists := eventMap["uid"]; exists {
		attempt.BeforeUID = uint32(numberValue(eventMap["uid"]))
		attempt.BeforeUIDVisible = true
	}
	if kind, _, ok := parseNamespaceIdentity(target); ok {
		attempt.TargetNamespace = kind
	}
	m.observeEscapeViolation(attempt, process, normalized)
	return true
}

func (m *Manager) observeEscapeViolation(
	attempt GuardAttempt,
	process ProcessSnapshot,
	parent BehaviorEvent,
) bool {
	if !m.cfg.Enabled || !m.monitorEnabled.Load() || !m.escapeEnabled.Load() {
		return false
	}
	subject, ok := m.tracker.Attribute(process)
	if !ok {
		return false
	}
	unit, ok := m.tracker.Unit(subject.UnitID)
	if !ok {
		return false
	}
	if attempt.Baseline.CapturedAt.IsZero() {
		attempt.Baseline = unit.IsolationBaseline
	}
	if attempt.Actual.CapturedAt.IsZero() {
		attempt.Actual = unit.IsolationActual
	}
	if attempt.Operation == "ptrace" && attempt.TargetPID > 0 {
		target, err := m.scanner.ReadPID(attempt.TargetPID)
		if err == nil {
			if targetSubject, attributed := m.tracker.Attribute(target); attributed &&
				targetSubject.UnitID == subject.UnitID {
				attempt.TargetPID = 0
			}
		}
	}
	attempt.EvidenceEventIDs = []string{parent.EventID}
	violation, detected := DetectEscapeAttempt(attempt)
	if !detected {
		return false
	}
	evidence := map[string]any{
		"rule":               violation.Rule,
		"operation":          violation.Operation,
		"target":             violation.Target,
		"baseline":           violation.Baseline,
		"actual":             violation.Actual,
		"diff":               violation.Diff,
		"state_changed":      violation.StateChanged,
		"return_code":        violation.ReturnCode,
		"evidence_event_ids": violation.EvidenceEventIDs,
	}
	_, accepted := m.observeRawEvent(RawBehavior{
		OccurredAt: time.Now().UTC(), Category: attempt.Category,
		Operation: attempt.Operation + "_violation", Outcome: parent.Outcome,
		Process: process, Argv: process.Argv,
		Resource: Resource{
			Type: "escape_target", Identity: violation.Target,
			Classification: violation.Rule,
		},
		Source: "agent_guard", Sensor: "escape_evaluator",
		Visibility: parent.Collection.Visibility,
		EventType:  "agent_sandbox_violation", Decision: violation.Decision,
		Severity: violation.Severity, RuleID: violation.Rule,
		Isolation: map[string]any{
			"unit_type": unit.Type, "baseline": violation.Baseline,
			"actual": violation.Actual, "diff": violation.Diff,
			"completeness": violation.Actual.Completeness(),
		},
		Evidence: evidence,
	})
	if accepted {
		logger.Warn("agent_guard_escape_would_deny",
			zap.String("host_id", m.cfg.HostID),
			zap.String("instance_id", subject.InstanceID),
			zap.String("execution_unit_id", subject.UnitID),
			zap.String("rule_id", violation.Rule),
			zap.String("decision", string(violation.Decision)),
			zap.Bool("state_changed", violation.StateChanged))
	}
	return accepted
}

func (m *Manager) reconcileIsolation(processes []ProcessSnapshot) {
	if m.isolationScan == nil {
		return
	}
	states := make(map[ProcessIdentity]IsolationState)
	for _, process := range processes {
		if _, ok := m.tracker.LookupProcess(process.Identity); !ok {
			continue
		}
		state, err := m.isolationScan.ReadIsolation(process.Identity.PID)
		if err != nil {
			state = unavailableIsolationState("proc_isolation_read_failed")
		}
		states[process.Identity] = state
	}

	controllerStates := make(map[string]IsolationState)
	for _, instance := range m.tracker.Instances() {
		if state, ok := states[instance.Controller]; ok {
			controllerStates[instance.InstanceID] = state
		}
	}
	for _, process := range processes {
		subject, ok := m.tracker.LookupProcess(process.Identity)
		if !ok {
			continue
		}
		instance, ok := m.tracker.Instance(subject.InstanceID)
		if !ok {
			continue
		}
		profile, ok := m.tracker.profiles.Profile(instance.ProfileKey)
		if !ok || profile.SandboxFamily != IsolationLinuxNamespace ||
			!m.tracker.profiles.MatchWorker(instance.ProfileKey, process) {
			continue
		}
		state := states[process.Identity]
		controllerState := controllerStates[instance.InstanceID]
		if namespaceTupleEqual(state, controllerState) {
			continue
		}
		if _, err := m.tracker.AttachNamespace(instance.InstanceID, process, state); err != nil {
			logger.Debug("agent_guard_namespace_unit_discovery_skipped",
				zap.String("instance_id", instance.InstanceID),
				zap.String("error_code", errorCode(err)))
		}
	}

	namespaceUnits := make(map[string]string)
	for _, unit := range m.tracker.Units() {
		if unit.Type == IsolationLinuxNamespace {
			namespaceUnits[unit.InstanceID+"\x00"+namespaceFingerprint(unit.IsolationBaseline)] = unit.UnitID
		}
	}
	for _, process := range processes {
		subject, ok := m.tracker.LookupProcess(process.Identity)
		if !ok {
			continue
		}
		state, ok := states[process.Identity]
		if !ok || len(state.NamespaceInodes) == 0 {
			continue
		}
		if unitID := namespaceUnits[subject.InstanceID+"\x00"+namespaceFingerprint(state)]; unitID != "" {
			m.tracker.AssignProcessToUnit(process, unitID)
		}
	}

	representatives := make(map[string]ProcessSnapshot)
	for _, unit := range m.tracker.Units() {
		if unit.RootProcess.Valid() {
			for _, process := range processes {
				if process.Identity == unit.RootProcess {
					representatives[unit.UnitID] = process
					break
				}
			}
		}
	}
	for _, process := range processes {
		subject, ok := m.tracker.Attribute(process)
		if !ok {
			continue
		}
		if _, exists := representatives[subject.UnitID]; !exists {
			representatives[subject.UnitID] = process
		}
	}
	for unitID, process := range representatives {
		actual, ok := states[process.Identity]
		if !ok {
			actual = unavailableIsolationState("representative_state_missing")
		}
		previous, _ := m.tracker.Unit(unitID)
		updated, hadBaseline := m.tracker.UpdateUnitIsolation(unitID, actual, m.capabilities)
		stateChanged := previous.IsolationActual.Fingerprint() != updated.IsolationActual.Fingerprint()
		if hadBaseline && stateChanged {
			m.lifecycleMu.Lock()
			if m.startedUnits[unitID] {
				m.queueExecutionUnitStatus(updated, "agent_execution_unit_updated")
			}
			m.lifecycleMu.Unlock()
		}
		if hadBaseline && updated.IsolationDiff.StateChanged {
			m.emitIsolationDrift(process, updated)
		} else if !updated.IsolationDiff.StateChanged {
			m.isolationMu.Lock()
			delete(m.lastDrift, unitID)
			m.isolationMu.Unlock()
		}
	}
}

func (m *Manager) emitIsolationDrift(process ProcessSnapshot, unit ExecutionUnit) {
	data, _ := json.Marshal(unit.IsolationDiff)
	digest := stableID("drift", unit.UnitID, string(data))
	m.isolationMu.Lock()
	if m.lastDrift[unit.UnitID] == digest {
		m.isolationMu.Unlock()
		return
	}
	m.lastDrift[unit.UnitID] = digest
	m.isolationMu.Unlock()
	decision := DecisionWouldDeny
	severity := "high"
	if unit.Coverage == CoverageNoIsolation || !dangerousIsolationDrift(unit) {
		decision = DecisionAudit
		severity = "medium"
	}
	_, accepted := m.observeRawEvent(RawBehavior{
		OccurredAt: time.Now().UTC(), Category: CategoryIsolation,
		Operation: "isolation_drift", Outcome: OutcomeUnknown,
		Process: process, Argv: process.Argv,
		Resource: Resource{
			Type: "execution_unit", Identity: unit.UnitID,
			Classification: EscapeRuleIsolationDrift,
		},
		Source: "procfs", Sensor: "isolation_reconciler",
		Visibility: visibilityFromCompleteness(unit.Completeness), EventType: "agent_isolation_drift",
		Decision: decision, Severity: severity, RuleID: EscapeRuleIsolationDrift,
		Isolation: map[string]any{
			"unit_type": unit.Type, "baseline": unit.IsolationBaseline,
			"actual": unit.IsolationActual, "diff": unit.IsolationDiff,
			"completeness": unit.Completeness,
		},
		Evidence: map[string]any{
			"rule": EscapeRuleIsolationDrift, "baseline": unit.IsolationBaseline,
			"actual": unit.IsolationActual, "diff": unit.IsolationDiff,
			"state_changed": unit.IsolationDiff.StateChanged,
		},
	})
	if accepted {
		logger.Warn("agent_guard_isolation_drift_observed",
			zap.String("host_id", m.cfg.HostID),
			zap.String("instance_id", unit.InstanceID),
			zap.String("execution_unit_id", unit.UnitID),
			zap.String("decision", string(decision)),
			zap.Int("changed_dimension_count", len(unit.IsolationDiff.Changes)))
	}
}

func visibilityFromCompleteness(completeness string) string {
	switch completeness {
	case "complete":
		return "complete"
	case "unavailable":
		return "unobservable"
	default:
		return "partial"
	}
}

func dangerousIsolationDrift(unit ExecutionUnit) bool {
	for key := range unit.IsolationDiff.Changes {
		if strings.HasPrefix(key, "namespace.") || key == "cgroup_path" ||
			key == "root_mount" || key == "capabilities.effective_added" {
			return true
		}
	}
	if change, ok := unit.IsolationDiff.Changes["no_new_privs"]; ok &&
		change.Before == true && change.After == false {
		return true
	}
	if change, ok := unit.IsolationDiff.Changes["seccomp_mode"]; ok {
		before, beforeOK := change.Before.(int)
		after, afterOK := change.After.(int)
		return beforeOK && afterOK && after < before
	}
	return false
}

func unavailableIsolationState(reason string) IsolationState {
	state := newIsolationState()
	for _, key := range isolationDimensions {
		state.Availability[key] = EvidenceAvailability{Available: false, Reason: reason}
	}
	return state
}

func namespaceTupleEqual(left, right IsolationState) bool {
	if len(left.NamespaceInodes) == 0 || len(right.NamespaceInodes) == 0 {
		return true
	}
	for _, key := range []string{"mnt", "pid", "user", "net"} {
		if left.NamespaceInodes[key] != right.NamespaceInodes[key] {
			return false
		}
	}
	return true
}

func (m *Manager) reconcileOnce() error {
	processes, err := m.scanner.Scan()
	if err != nil {
		return fmt.Errorf("scan process identities: %w", err)
	}
	stats := m.reconciler.Reconcile(processes)
	for _, exit := range stats.Exits {
		if exit.InstanceStopped {
			m.queueStoppedLifecycle(exit, "reconcile_missing_pid")
		}
	}
	m.reconcileIsolation(processes)
	m.lifecycleMu.Lock()
	m.queuePendingLifecyclesLocked()
	m.queueInstanceHeartbeatsLocked(time.Now().UTC())
	m.lifecycleMu.Unlock()
	m.syncKernelSubjects()
	if stats.ProcessLabelsRepaired > 0 || stats.ExpiredLabelsRemoved > 0 || stats.ControllersDiscovered > 0 {
		logger.Info("agent_guard_reconcile_completed",
			zap.String("host_id", m.cfg.HostID),
			zap.Uint64("controllers_discovered", stats.ControllersDiscovered),
			zap.Uint64("process_labels_repaired", stats.ProcessLabelsRepaired),
			zap.Uint64("expired_labels_removed", stats.ExpiredLabelsRemoved))
	}
	return nil
}

func (m *Manager) queuePendingLifecycles() {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.queuePendingLifecyclesLocked()
}

func (m *Manager) queueTrustedSessionStatus(session BehaviorSession, changed bool) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if !m.startedSessions[session.SessionID] {
		m.queuePendingLifecyclesLocked()
		return
	}
	if changed {
		m.queueSessionStatus(session, "agent_behavior_session_updated")
	}
}

func (m *Manager) queuePendingLifecyclesLocked() {
	for _, instance := range m.tracker.Instances() {
		if m.started[instance.InstanceID] {
			continue
		}
		m.started[instance.InstanceID] = true
		m.queueInstanceStatus(instance, "agent_instance_started")
		m.lastHeartbeat[instance.InstanceID] = time.Now().UTC()
		logger.Info("agent_guard_instance_started",
			zap.String("host_id", m.cfg.HostID),
			zap.String("instance_id", instance.InstanceID),
			zap.String("profile_key", instance.ProfileKey),
			zap.Uint32("controller_pid", instance.Controller.PID))
	}
	for _, unit := range m.tracker.Units() {
		session, sessionExists := m.tracker.Session(unit.SessionID)
		if m.startedUnits[unit.UnitID] || !m.started[unit.InstanceID] ||
			!sessionExists || !isTrustedSession(session) {
			continue
		}
		m.startedUnits[unit.UnitID] = true
		m.queueExecutionUnitStatus(unit, "agent_execution_unit_started")
		logger.Info("agent_guard_execution_unit_started",
			zap.String("host_id", m.cfg.HostID),
			zap.String("instance_id", unit.InstanceID),
			zap.String("execution_unit_id", unit.UnitID),
			zap.String("unit_type", string(unit.Type)),
			zap.String("coverage", string(unit.Coverage)))
	}
	for _, session := range m.tracker.Sessions() {
		if m.startedSessions[session.SessionID] || !m.started[session.InstanceID] ||
			!isTrustedSession(session) {
			continue
		}
		m.startedSessions[session.SessionID] = true
		m.queueSessionStatus(session, "agent_behavior_session_started")
		logger.Info("agent_guard_behavior_session_started",
			zap.String("host_id", m.cfg.HostID),
			zap.String("instance_id", session.InstanceID),
			zap.String("session_id", session.SessionID),
			zap.String("session_source", session.Source),
			zap.String("session_confidence", string(session.Confidence)))
	}
}

func (m *Manager) queueInstanceHeartbeatsLocked(now time.Time) {
	interval := m.cfg.ReconcileInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	for _, instance := range m.tracker.Instances() {
		if instance.Status != "running" || !m.started[instance.InstanceID] ||
			now.Sub(m.lastHeartbeat[instance.InstanceID]) < interval/2 {
			continue
		}
		m.queueInstanceStatus(instance, "agent_instance_updated")
		m.lastHeartbeat[instance.InstanceID] = now
	}
}

func (m *Manager) enqueueAggregates(events []BehaviorEvent) {
	for _, event := range events {
		priority := priorityForEvent(event)
		if !m.spool.Push(event, priority) {
			stats := m.spool.Stats()
			logger.Warn("agent_guard_spool_event_dropped",
				zap.String("host_id", m.cfg.HostID),
				zap.String("category", string(event.Category)),
				zap.Int("queued", stats.Queued))
		}
	}
}

func (m *Manager) flush() {
	if m.reporter == nil {
		return
	}
	m.statusMu.Lock()
	status := append([]*pb.RuntimeEvent(nil), m.pendingStatus...)
	m.pendingStatus = nil
	m.statusMu.Unlock()
	behaviors := m.spool.PopBatch(128)
	events := make([]*pb.RuntimeEvent, 0, len(status)+len(behaviors))
	events = append(events, status...)
	for _, event := range behaviors {
		events = append(events, behaviorRuntimeEvent(event))
	}
	if len(events) == 0 {
		return
	}
	if err := m.reporter.ReportEvents(events); err != nil {
		m.statusMu.Lock()
		m.pendingStatus = append(status, m.pendingStatus...)
		m.statusMu.Unlock()
		for _, event := range behaviors {
			m.spool.Push(event, priorityForEvent(event))
		}
		logger.Warn("agent_guard_report_deferred",
			zap.String("host_id", m.cfg.HostID),
			zap.Int("event_count", len(events)),
			zap.String("error_code", errorCode(err)))
	}
}

func (m *Manager) queueConfigStatus(status string, version int64, digest, code string) {
	body := map[string]any{
		"schema": GuardSchemaV1, "status": status, "bundle_version": version,
		"digest": digest, "error_code": code, "occurred_at": time.Now().UTC(),
		"coverage_level": m.currentEnforcementCoverage(),
		"capabilities":   m.capabilities,
	}
	m.queueStatusEvent("agent_guard_config_status", body, "info")
}

func (m *Manager) queueRuntimeSettingsStatus(status string, version int64, code string, injections []string) {
	body := map[string]any{
		"schema": GuardSchemaV1, "settings_schema": AgentGuardRuntimeSettingsSchema,
		"status": status, "settings_version": version, "error_code": code,
		"injections": injections, "occurred_at": time.Now().UTC(),
		"coverage_level": m.currentEnforcementCoverage(),
	}
	m.queueStatusEvent("agent_guard_config_status", body, "info")
}

func (m *Manager) currentEnforcementCoverage() CoverageLevel {
	m.kernelPolicyMu.RLock()
	policy := m.kernelPolicy
	m.kernelPolicyMu.RUnlock()
	if m.cfg.EnforcementEnabled && m.capabilities.BPFLSM &&
		(len(policy.PathRules) > 0 || len(policy.EscapeRules) > 0) {
		return CoverageFullEnforcement
	}
	return CoverageMonitorOnly
}

func (m *Manager) queueInstanceStatus(instance RuntimeInstance, eventType string) {
	body := map[string]any{
		"schema":                 GuardSchemaV1,
		"instance_id":            instance.InstanceID,
		"asset_id":               instance.AssetID,
		"profile_key":            instance.ProfileKey,
		"profile_version":        instance.ProfileVersion,
		"agent_type":             instance.AgentType,
		"display_name":           instance.DisplayName,
		"controller_pid":         instance.Controller.PID,
		"controller_start_ticks": instance.Controller.StartTicks,
		"controller_exe":         RedactString(instance.ControllerExe),
		"run_uid":                instance.RunUID,
		"detection_confidence":   instance.Confidence,
		"status":                 instance.Status,
		"coverage_level":         instance.Coverage,
		"coverage_reasons":       coverageReasons(instance.Coverage),
		"first_seen_at":          instance.FirstSeenAt,
		"last_seen_at":           instance.LastSeenAt,
		"occurred_at":            time.Now().UTC(),
	}
	m.queueStatusEvent(eventType, body, "info")
}

func (m *Manager) queueExecutionUnitStatus(unit ExecutionUnit, eventType string) {
	session, ok := m.tracker.Session(unit.SessionID)
	if !ok || !isTrustedSession(session) {
		return
	}
	baseline := redactIsolationState(unit.IsolationBaseline)
	actual := redactIsolationState(unit.IsolationActual)
	diff := redactIsolationDiff(unit.IsolationDiff, 0)
	body := map[string]any{
		"schema":             GuardSchemaV1,
		"execution_unit_id":  unit.UnitID,
		"instance_id":        unit.InstanceID,
		"unit_type":          unit.Type,
		"fingerprint":        stableID("fingerprint", m.cfg.HostID, unit.InstanceID, unit.UnitID),
		"cgroup_path":        RedactString(unit.CgroupPath),
		"container_id":       unit.ContainerID,
		"container_runtime":  unit.ContainerRuntime,
		"coverage_level":     unit.Coverage,
		"coverage_reasons":   appendCoverageReasons(unit.Coverage, unit.IsolationActual),
		"capabilities":       unit.Capabilities,
		"isolation_baseline": baseline,
		"isolation_actual":   actual,
		"isolation_diff":     diff,
		"completeness":       unit.Completeness,
		"status":             unit.Status,
		"first_seen_at":      unit.FirstSeenAt,
		"last_seen_at":       unit.LastSeenAt,
		"occurred_at":        time.Now().UTC(),
	}
	if unit.RootProcess.Valid() {
		body["root_pid"] = unit.RootProcess.PID
		body["root_start_ticks"] = unit.RootProcess.StartTicks
	}
	m.queueStatusEvent(eventType, body, "info")
}

func (m *Manager) queueSessionStatus(session BehaviorSession, eventType string) {
	if !isTrustedSession(session) {
		return
	}
	body := map[string]any{
		"schema":                 GuardSchemaV1,
		"session_id":             session.SessionID,
		"instance_id":            session.InstanceID,
		"external_session_id":    session.ExternalSessionID,
		"execution_unit_id":      unitIDForSession(m.tracker.Units(), session.SessionID),
		"source":                 session.Source,
		"confidence":             session.Confidence,
		"correlation_token_hash": session.CorrelationTokenHash,
		"status":                 session.Status,
		"completeness":           map[string]any{},
		"started_at":             session.FirstSeenAt,
		"last_seen_at":           session.LastSeenAt,
		"occurred_at":            time.Now().UTC(),
	}
	m.queueStatusEvent(eventType, body, "info")
}

func unitIDForSession(units []ExecutionUnit, sessionID string) string {
	for _, unit := range units {
		if unit.SessionID == sessionID {
			return unit.UnitID
		}
	}
	return ""
}

func coverageReasons(level CoverageLevel) []string {
	switch level {
	case CoverageNoIsolation:
		return []string{"backend_no_isolation"}
	case CoverageRemoteUnobservable:
		return []string{"remote_sensor_unavailable"}
	case CoverageDegraded:
		return []string{"p1_monitor_only", "isolation_evidence_partial"}
	default:
		return []string{"p1_monitor_only"}
	}
}

func behaviorCoverage(level CoverageLevel) CoverageLevel {
	if level == CoverageDegraded {
		return CoverageMonitorOnly
	}
	return level
}

func (m *Manager) queueStatusEvent(eventType string, body map[string]any, severity string) {
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	event := &pb.RuntimeEvent{
		EventId: stableID(eventType, m.cfg.HostID, time.Now().UnixNano()),
		HostId:  m.cfg.HostID, Timestamp: time.Now().UnixMilli(),
		EventType: eventType, Severity: severity, EventDataJson: string(data),
	}
	m.statusMu.Lock()
	m.pendingStatus = append(m.pendingStatus, event)
	m.statusMu.Unlock()
}

func behaviorRuntimeEvent(event BehaviorEvent) *pb.RuntimeEvent {
	return &pb.RuntimeEvent{
		EventId: validProtoString(event.EventID), HostId: validProtoString(event.HostID),
		Timestamp: event.OccurredAt.UnixMilli(), EventType: validProtoString(event.EventType),
		ProcessName: validProtoString(filepath.Base(event.Actor.Exe)), Pid: int32(event.Actor.PID),
		Ppid: int32(event.Actor.PPID), Uid: int32(event.Actor.UID),
		CommandLine: validProtoString(strings.Join(event.Actor.Argv, " ")),
		FilePath:    validProtoString(behaviorFilePath(event)), RemoteAddr: validProtoString(behaviorRemoteAddr(event)),
		Severity: validProtoString(event.Severity), EventDataJson: validProtoString(event.MustJSON()),
	}
}

func validProtoString(value string) string {
	return strings.ToValidUTF8(value, "\uFFFD")
}

func behaviorFilePath(event BehaviorEvent) string {
	if event.Category == CategoryFile {
		return event.Resource.Identity
	}
	return ""
}

func behaviorRemoteAddr(event BehaviorEvent) string {
	if event.Category == CategoryNetwork {
		return event.Resource.Identity
	}
	return ""
}

func mapBehaviorOperation(eventType string, eventMap map[string]any) (Category, string) {
	switch eventType {
	case "process_exec":
		return CategoryProcess, "exec"
	case "process_fork":
		return CategoryProcess, "fork"
	case "process_exit":
		return CategoryProcess, "exit"
	case "file_access":
		return CategoryFile, stringValue(eventMap["file_action"])
	case "network_connect":
		return CategoryNetwork, "connect"
	case "network_accept":
		return CategoryNetwork, "accept"
	case "privilege_change":
		return CategoryIdentity, "credential_change"
	}
	return "", ""
}

func visibilityForEvent(eventMap map[string]any) string {
	if value, ok := eventMap["args_truncated"].(bool); ok && value {
		return "partial"
	}
	return "complete"
}

func priorityForEvent(event BehaviorEvent) EventPriority {
	if event.Category == CategoryIsolation || event.Operation == "exit" ||
		event.Operation == "create" || event.Operation == "delete" ||
		event.Operation == "rename" || event.Operation == "chmod" ||
		event.Operation == "credential_change" {
		return PriorityStateChange
	}
	if isAggregatable(event) {
		return PriorityRepetitiveIO
	}
	return PriorityProcessNetwork
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func numberValue(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if index := strings.IndexByte(value, ':'); index >= 0 {
		value = value[:index]
	}
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	if len(value) > 96 {
		value = value[:96]
	}
	return value
}

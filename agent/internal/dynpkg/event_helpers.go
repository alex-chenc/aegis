package dynpkg

import (
	"fmt"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

func (m *Manager) Status() []InstalledPackage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []InstalledPackage
	for _, pkg := range m.packages {
		statuses = append(statuses, *pkg)
	}
	return statuses
}

func (m *Manager) ProcessEvent(packageID string, event map[string]interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pkg, exists := m.packages[packageID]
	if !exists || pkg.stateMachine.Current != StateActive {
		return
	}

	if !m.rateLimiter.Allow(packageID, eventTypeFromEvent(event), pidFromEvent(event)) {
		return
	}

	if m.sigmaMatcher != nil {
		for _, match := range m.sigmaMatcher.Match(event) {
			finding := buildFinding(packageID, pkg, match, event)
			if m.corrEngine != nil {
				for _, alert := range m.corrEngine.AddFinding(finding) {
					if m.onAlert != nil {
						m.onAlert(alert)
					}
				}
			}
		}
	}
}

func (m *Manager) disableByRateLimit(packageID, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pkg, exists := m.packages[packageID]
	if !exists {
		return
	}

	if err := m.unloadPlugin(pkg); err != nil {
		logger.Error("failed to unload plugin in disableByRateLimit", zap.String("package_id", packageID), zap.Error(err))
	}

	m.failPkg(pkg, StateDisabledByRate, reason)

	logger.Warn("detection package disabled by rate limit",
		zap.String("package_id", packageID),
		zap.String("reason", reason),
	)
}

func pidFromEvent(event map[string]interface{}) int {
	if pid, ok := event["pid"].(int); ok {
		return pid
	}
	if pid, ok := event["pid"].(float64); ok {
		return int(pid)
	}
	return 0
}

func buildFinding(packageID string, pkg *InstalledPackage, match SigmaMatch, event map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
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
}

func eventTypeFromEvent(event map[string]interface{}) string {
	return fmt.Sprintf("%v", event["event_type"])
}

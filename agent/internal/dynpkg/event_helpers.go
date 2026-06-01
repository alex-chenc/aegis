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

// ProcessEventForAll feeds a built-in eBPF event (process_exec, file_access, etc.)
// to all active detection packages. This is called from the Pipeline event callback
// so that package-specific sigma rules (e.g. suspicious_root_exec) are evaluated
// against built-in events and findings are fed to the correlation engine.
func (m *Manager) ProcessEventForAll(event map[string]interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	evtType := eventTypeFromEvent(event)
	pid := pidFromEvent(event)
	uid := 0
	if v, ok := event["uid"].(int); ok {
		uid = v
	} else if v, ok := event["uid"].(float64); ok {
		uid = int(v)
	}
	image, _ := event["image"].(string)

	for packageID, pkg := range m.packages {
		if pkg.stateMachine.Current != StateActive {
			continue
		}
		if !m.rateLimiter.Allow(packageID, evtType, pid) {
			continue
		}

		if m.sigmaMatcher != nil {
			matches := m.sigmaMatcher.Match(event)
			if len(matches) > 0 {
				logger.Info("ProcessEventForAll matched",
					zap.String("package_id", packageID),
					zap.String("event_type", evtType),
					zap.Int("pid", pid),
					zap.Int("uid", uid),
					zap.String("image", image),
					zap.Int("match_count", len(matches)),
				)
			}
			for _, match := range matches {
				finding := buildFinding(packageID, pkg, match, event)
				logger.Debug("ProcessEventForAll finding",
					zap.String("package_id", packageID),
					zap.String("rule_id", match.RuleID),
				)
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
}

func (m *Manager) ProcessEvent(packageID string, event map[string]interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pkg, exists := m.packages[packageID]
	if !exists || pkg.stateMachine.Current != StateActive {
		logger.Debug("ProcessEvent: package not active",
			zap.String("package_id", packageID),
			zap.Bool("exists", exists),
		)
		return
	}

	evtType := eventTypeFromEvent(event)
	pid := pidFromEvent(event)
	if !m.rateLimiter.Allow(packageID, evtType, pid) {
		logger.Debug("ProcessEvent: rate limited",
			zap.String("package_id", packageID),
			zap.String("event_type", evtType),
			zap.Int("pid", pid),
		)
		return
	}

	if m.sigmaMatcher != nil {
		logger.Debug("ProcessEvent event", zap.String("package_id", packageID), zap.Any("event_keys", event), zap.Int("pid_from_event", pidFromEvent(event)))
	logger.Debug("sigma matching plugin event",
			zap.String("package_id", packageID),
			zap.String("event_type", fmt.Sprintf("%v", event["event_type"])),
			zap.String("category", fmt.Sprintf("%v", event["category"])),
		)
		matches := m.sigmaMatcher.Match(event)
		if len(matches) > 0 {
			logger.Debug("sigma matched",
				zap.String("package_id", packageID),
				zap.Int("match_count", len(matches)),
			)
		}
		for _, match := range matches {
			finding := buildFinding(packageID, pkg, match, event)
			logger.Debug("correlation finding",
				zap.String("package_id", packageID),
				zap.String("rule_id", match.RuleID),
				zap.String("finding_rule_id", fmt.Sprintf("%v", finding["rule_id"])),
			)
			if m.corrEngine != nil {
				alerts := m.corrEngine.AddFinding(finding)
				logger.Debug("correlation result",
					zap.String("package_id", packageID),
					zap.String("rule_id", match.RuleID),
					zap.Int("alert_count", len(alerts)),
				)
				for _, alert := range alerts {
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

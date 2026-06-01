package correlation

import (
	"fmt"
	"sync"
	"time"

	"aegis-agent/internal/logger"
	"go.uber.org/zap"
)

type Engine struct {
	mu      sync.RWMutex
	specs   map[string]CorrelationSpec
	cache   *FindingCache
	limits  CorrelationLimits
	onAlert func(alert CorrelationAlert)
}

type CorrelationLimits struct {
	Window          time.Duration
	EventsPerKey    int
	GlobalCacheSize int
}

func NewEngine(limits CorrelationLimits) *Engine {
	if limits.Window == 0 {
		limits.Window = 60 * time.Second
	}
	if limits.EventsPerKey == 0 {
		limits.EventsPerKey = 128
	}
	if limits.GlobalCacheSize == 0 {
		limits.GlobalCacheSize = 10000
	}

	return &Engine{
		specs:  make(map[string]CorrelationSpec),
		cache:  NewFindingCache(limits.GlobalCacheSize),
		limits: limits,
	}
}

func (e *Engine) SetAlertCallback(fn func(alert CorrelationAlert)) {
	e.onAlert = fn
}

func (e *Engine) AddSpec(spec CorrelationSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.specs[spec.ID] = spec
	logger.Info("correlation spec added",
		zap.String("id", spec.ID),
		zap.String("package_id", spec.PackageID),
		zap.Int("sequence_length", len(spec.Correlation.Sequence)),
	)
	return nil
}

func (e *Engine) RemovePackage(packageID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for id, spec := range e.specs {
		if spec.PackageID == packageID {
			delete(e.specs, id)
		}
	}
	e.cache.RemoveByPackage(packageID)
}

func (e *Engine) AddFinding(finding AtomicFinding) []CorrelationAlert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	e.cache.Add(finding)
	logger.Debug("AddFinding called", zap.String("package_id", finding.PackageID), zap.String("rule_id", finding.RuleID), zap.Int("pid", finding.PID))

	var alerts []CorrelationAlert
	for _, spec := range e.specs {
		logger.Debug("AddFinding checking spec", zap.String("spec_id", spec.ID), zap.String("spec_package_id", spec.PackageID), zap.String("finding_package_id", finding.PackageID), zap.Bool("match", spec.PackageID == finding.PackageID))
		if spec.PackageID != finding.PackageID {
			continue
		}

		alert := e.checkSpec(spec, finding)
		if alert != nil {
			alerts = append(alerts, *alert)
			if e.onAlert != nil {
				e.onAlert(*alert)
			}
		}
	}

	return alerts
}

func (e *Engine) checkSpec(spec CorrelationSpec, finding AtomicFinding) *CorrelationAlert {
	if len(spec.Correlation.Sequence) == 0 {
		return nil
	}

	lastStep := spec.Correlation.Sequence[len(spec.Correlation.Sequence)-1]
	logger.Debug("checkSpec", zap.String("spec_id", spec.ID), zap.String("finding_rule_id", finding.RuleID), zap.String("last_step_rule_id", lastStep.RuleID), zap.Bool("match", finding.RuleID == lastStep.RuleID))
	if finding.RuleID != lastStep.RuleID {
		return nil
	}

	key := e.getCorrelationKey(spec.Correlation.By, finding)
	windowNs := spec.Correlation.Window.Nanoseconds()

	matchedFindings := make([]AtomicFinding, len(spec.Correlation.Sequence))
	matchedFindings[len(spec.Correlation.Sequence)-1] = finding
	now := finding.Timestamp

	for i := len(spec.Correlation.Sequence) - 2; i >= 0; i-- {
		step := spec.Correlation.Sequence[i]
		found := false

		e.cache.ForEach(func(f AtomicFinding) bool {
			if f.PackageID != finding.PackageID {
				return true
			}
			if f.RuleID != step.RuleID {
				return true
			}

			if spec.Correlation.Ordered {
				if f.Timestamp >= now || f.Timestamp < now-windowNs {
					return true
				}
			} else {
				if f.Timestamp < now-windowNs || f.Timestamp > now+windowNs {
					return true
				}
			}

			fKey := e.getCorrelationKey(spec.Correlation.By, f)
			if fKey != key {
				return true
			}

			matchedFindings[i] = f
			now = f.Timestamp
			found = true
			return false
		})

		if !found {
			return nil
		}
	}

	evidence := make([]AtomicFinding, 0, len(matchedFindings))
	for _, f := range matchedFindings {
		evidence = append(evidence, f)
	}

	return &CorrelationAlert{
		SpecID:      spec.ID,
		PackageID:   spec.PackageID,
		Title:       spec.Alert.Title,
		Severity:    spec.Alert.Severity,
		MitreID:     spec.Alert.MitreID,
		CVEID:       spec.Alert.CVEID,
		Evidence:    evidence,
		TriggeredAt: time.Now(),
	}
}

func (e *Engine) getCorrelationKey(by string, finding AtomicFinding) string {
	switch by {
	case "pid":
		return fmt.Sprintf("%d", finding.PID)
	case "pid_tree":
		return ComputeTreeKey(finding.HostID, finding.Process)
	case "host":
		return finding.HostID
	default:
		return finding.HostID
	}
}

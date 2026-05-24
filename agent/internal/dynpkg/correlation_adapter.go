package dynpkg

import (
	"aegis-agent/internal/correlation"
	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

type CorrelationEngineAdapter struct {
	engine *correlation.Engine
}

func NewCorrelationEngineAdapter(engine *correlation.Engine) *CorrelationEngineAdapter {
	return &CorrelationEngineAdapter{engine: engine}
}

func (a *CorrelationEngineAdapter) AddSpec(spec interface{}) error {
	if corrSpec, ok := spec.(correlation.CorrelationSpec); ok {
		return a.engine.AddSpec(corrSpec)
	}
	logger.Warn("AddSpec: unsupported type", zap.Any("spec", spec))
	return nil
}

func (a *CorrelationEngineAdapter) RemovePackage(packageID string) {
	a.engine.RemovePackage(packageID)
}

func (a *CorrelationEngineAdapter) AddFinding(finding interface{}) []interface{} {
	var atomicFinding correlation.AtomicFinding

	switch f := finding.(type) {
	case correlation.AtomicFinding:
		atomicFinding = f
	case map[string]interface{}:
		atomicFinding = mapToAtomicFinding(f)
	default:
		logger.Warn("AddFinding: unsupported type", zap.Any("finding", finding))
		return nil
	}

	alerts := a.engine.AddFinding(atomicFinding)
	result := make([]interface{}, len(alerts))
	for i, alert := range alerts {
		result[i] = alert
	}
	return result
}

func mapToAtomicFinding(m map[string]interface{}) correlation.AtomicFinding {
	f := correlation.AtomicFinding{
		EventMap: m,
	}
	if v, ok := m["package_id"].(string); ok {
		f.PackageID = v
	}
	if v, ok := m["version"].(string); ok {
		f.Version = v
	}
	if v, ok := m["rule_id"].(string); ok {
		f.RuleID = v
	}
	if v, ok := m["event_type"].(string); ok {
		f.EventType = v
	}
	if v, ok := m["timestamp"].(int64); ok {
		f.Timestamp = v
	} else if v, ok := m["timestamp"].(float64); ok {
		f.Timestamp = int64(v)
	}
	if v, ok := m["host_id"].(string); ok {
		f.HostID = v
	}
	if v, ok := m["hostname"].(string); ok {
		f.Hostname = v
	}
	if v, ok := m["pid"].(int); ok {
		f.PID = v
	} else if v, ok := m["pid"].(float64); ok {
		f.PID = int(v)
	}
	if v, ok := m["ppid"].(int); ok {
		f.PPID = v
	} else if v, ok := m["ppid"].(float64); ok {
		f.PPID = int(v)
	}
	if v, ok := m["uid"].(int); ok {
		f.UID = v
	} else if v, ok := m["uid"].(float64); ok {
		f.UID = int(v)
	}
	if proc, ok := m["process"].(map[string]interface{}); ok {
		if n, ok := proc["name"].(string); ok {
			f.Process.Name = n
		}
		if c, ok := proc["command_line"].(string); ok {
			f.Process.CommandLine = c
		}
		if e, ok := proc["exe_path"].(string); ok {
			f.Process.ExePath = e
		}
	}
	return f
}

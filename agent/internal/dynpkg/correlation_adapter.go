package dynpkg

import (
	"encoding/base64"
	"fmt"
	"time"

	"aegis-agent/internal/correlation"
	"aegis-agent/internal/logger"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type CorrelationEngineAdapter struct {
	engine *correlation.Engine
}

func NewCorrelationEngineAdapter(engine *correlation.Engine) *CorrelationEngineAdapter {
	return &CorrelationEngineAdapter{engine: engine}
}

// yamlCorrelationSpec is a YAML-friendly struct for parsing correlation spec files.
type yamlCorrelationSpec struct {
	SchemaVersion string   `yaml:"schema_version"`
	ID            string   `yaml:"id"`
	PackageID     string   `yaml:"package_id"`
	Requires      []string `yaml:"requires"`
	Correlation   struct {
		By       string `yaml:"by"`
		Window   string `yaml:"window"`
		Ordered  bool   `yaml:"ordered"`
		Sequence []struct {
			RuleID string `yaml:"rule_id"`
		} `yaml:"sequence"`
	} `yaml:"correlation"`
	Alert struct {
		Title    string `yaml:"title"`
		Severity string `yaml:"severity"`
		MitreID  string `yaml:"mitre_id"`
		CVEID    string `yaml:"cve_id"`
	} `yaml:"alert"`
}

func (a *CorrelationEngineAdapter) AddSpec(spec interface{}) error {
	switch v := spec.(type) {
	case correlation.CorrelationSpec:
		return a.engine.AddSpec(v)
	case []byte:
		return a.addSpecFromBytes(v)
	case string:
		if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
			return a.addSpecFromBytes(decoded)
		}
		return a.addSpecFromBytes([]byte(v))
	default:
		logger.Warn("AddSpec: unsupported type", zap.Any("spec", spec))
		return fmt.Errorf("unsupported spec type: %T", spec)
	}
}

func (a *CorrelationEngineAdapter) addSpecFromBytes(data []byte) error {
	var raw yamlCorrelationSpec
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse correlation spec yaml: %w", err)
	}

	if raw.ID == "" {
		return fmt.Errorf("correlation spec missing id")
	}

	window, err := time.ParseDuration(raw.Correlation.Window)
	if err != nil {
		window = 10 * time.Second
	}

	sequence := make([]correlation.SequenceStep, len(raw.Correlation.Sequence))
	for i, s := range raw.Correlation.Sequence {
		sequence[i] = correlation.SequenceStep{RuleID: s.RuleID}
	}

	corrSpec := correlation.CorrelationSpec{
		ID:        raw.ID,
		PackageID: raw.PackageID,
		Requires:  raw.Requires,
		Correlation: correlation.CorrelationClause{
			By:       raw.Correlation.By,
			Window:   window,
			Ordered:  raw.Correlation.Ordered,
			Sequence: sequence,
		},
		Alert: correlation.AlertSpec{
			Title:    raw.Alert.Title,
			Severity: raw.Alert.Severity,
			MitreID:  raw.Alert.MitreID,
			CVEID:    raw.Alert.CVEID,
		},
	}

	logger.Info("correlation spec parsed",
		zap.String("id", corrSpec.ID),
		zap.String("package_id", corrSpec.PackageID),
		zap.Int("requires", len(corrSpec.Requires)),
		zap.Int("sequence", len(corrSpec.Correlation.Sequence)),
	)

	return a.engine.AddSpec(corrSpec)
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
	// Extract host_id and hostname — check top level first, then fall back
	// to nested event_map (where buildFinding puts the original event data).
	if v, ok := m["host_id"].(string); ok && v != "" {
		f.HostID = v
	} else if evtMap, ok := m["event_map"].(map[string]interface{}); ok {
		if v, ok := evtMap["host_id"].(string); ok {
			f.HostID = v
		}
	}
	if v, ok := m["hostname"].(string); ok && v != "" {
		f.Hostname = v
	} else if evtMap, ok := m["event_map"].(map[string]interface{}); ok {
		if v, ok := evtMap["hostname"].(string); ok {
			f.Hostname = v
		}
	}
	// Extract pid/ppid/uid — check top level first, then nested event_map.
	if v, ok := m["pid"].(int); ok {
		f.PID = v
	} else if v, ok := m["pid"].(float64); ok {
		f.PID = int(v)
	} else if evtMap, ok := m["event_map"].(map[string]interface{}); ok {
		if v, ok := evtMap["pid"].(int); ok {
			f.PID = v
		} else if v, ok := evtMap["pid"].(float64); ok {
			f.PID = int(v)
		}
	}
	if v, ok := m["ppid"].(int); ok {
		f.PPID = v
	} else if v, ok := m["ppid"].(float64); ok {
		f.PPID = int(v)
	} else if evtMap, ok := m["event_map"].(map[string]interface{}); ok {
		if v, ok := evtMap["ppid"].(int); ok {
			f.PPID = v
		} else if v, ok := evtMap["ppid"].(float64); ok {
			f.PPID = int(v)
		}
	}
	if v, ok := m["uid"].(int); ok {
		f.UID = v
	} else if v, ok := m["uid"].(float64); ok {
		f.UID = int(v)
	} else if evtMap, ok := m["event_map"].(map[string]interface{}); ok {
		if v, ok := evtMap["uid"].(int); ok {
			f.UID = v
		} else if v, ok := evtMap["uid"].(float64); ok {
			f.UID = int(v)
		}
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

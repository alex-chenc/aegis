package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/model"
)

// AlertServiceForTools 告警服务接口（写操作）
type AlertServiceForTools interface {
	Resolve(alertID string) error
	ManualBlock(alertID string, action string) (*model.BlockRecord, error)
}

// DetectionWriteToolDeps 检测写操作工具依赖
type DetectionWriteToolDeps struct {
	AlertService AlertServiceForTools
}

// RegisterDetectionWriteTools 注册检测域写操作工具
func RegisterDetectionWriteTools(registry *assistant.ToolRegistry, deps DetectionWriteToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Alert.Resolve",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpUpdate,
		Capability:         "resolve_detection_alert",
		Description:        "Mark an anomaly-detection alert as resolved.",
		Aliases:            []string{"处置告警", "解决告警", "关闭异常事件", "标记已解决"},
		Tags:               []string{"detection", "alert", "resolve", "operation"},
		ObjectTypes:        []string{"alert", "detection", "event"},
		PageRoutes:         []string{"/detection", "/detection/alerts", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskLow,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"alert_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact alert ID.",
				},
			},
			"required": []string{"alert_id"},
		},
		Handler: makeAlertResolveHandler(deps.AlertService),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Alert.Block",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpExecute,
		Capability:         "block_detection_alert",
		Description:        "Execute an approved high-risk block action against a process, file, or connection associated with an alert.",
		Aliases:            []string{"阻断告警", "阻断异常事件", "kill 进程", "隔离文件", "阻断连接"},
		Tags:               []string{"detection", "alert", "block", "response", "critical"},
		ObjectTypes:        []string{"alert", "detection", "event", "block"},
		PageRoutes:         []string{"/detection", "/detection/alerts", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskCritical,
		AutoCallable:       false,
		Idempotent:         false,
		Enabled:            true,
		DefaultWhitelisted: false,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"alert_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact alert ID.",
				},
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Block action: kill_process, quarantine_file, or block_connection; defaults to kill_process.",
				},
			},
			"required": []string{"alert_id"},
		},
		Handler: makeAlertBlockHandler(deps.AlertService),
	}); err != nil {
		return err
	}

	return nil
}

func makeAlertResolveHandler(svc AlertServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		alertID := getStringArg(args, "alert_id", "")
		if alertID == "" {
			return nil, fmt.Errorf("alert_id is required")
		}

		if err := svc.Resolve(alertID); err != nil {
			return nil, fmt.Errorf("failed to resolve alert: %w", err)
		}

		return map[string]interface{}{
			"alert_id": alertID,
			"status":   "resolved",
		}, nil
	}
}

func makeAlertBlockHandler(svc AlertServiceForTools) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		alertID := getStringArg(args, "alert_id", "")
		if alertID == "" {
			return nil, fmt.Errorf("alert_id is required")
		}
		action := getStringArg(args, "action", "kill_process")

		result, err := svc.ManualBlock(alertID, action)
		if err != nil {
			return nil, fmt.Errorf("failed to block alert: %w", err)
		}

		return result, nil
	}
}

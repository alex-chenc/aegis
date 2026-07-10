package tools

import (
	"context"

	"api-server/internal/assistant"
)

// NotificationToolDeps 通知工具依赖
type NotificationToolDeps struct {
	// 通知服务（待实现）
}

// RegisterNotificationTools 注册通知域工具
func RegisterNotificationTools(registry *assistant.ToolRegistry, deps NotificationToolDeps) error {
	// Notification.List — 获取通知列表
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Notification.List",
		Domain:             assistant.DomainNotification,
		Operation:          assistant.OpList,
		Capability:         "list_notifications",
		Description:        "获取系统通知列表，包括告警通知、任务完成通知、审批通知等",
		Aliases:            []string{"通知列表", "消息列表", "list notifications"},
		Tags:               []string{"v6.0", "notification"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":      map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size": map[string]interface{}{"type": "integer", "description": "每页数量"},
				"type":      map[string]interface{}{"type": "string", "description": "通知类型过滤"},
			},
		},
		Handler: makeNotificationListHandler(),
	}); err != nil {
		return err
	}

	return nil
}

// makeNotificationListHandler 创建通知列表 handler
func makeNotificationListHandler() assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		// 通知服务待实现，返回空列表
		return map[string]interface{}{
			"data":  []interface{}{},
			"total": 0,
		}, nil
	}
}

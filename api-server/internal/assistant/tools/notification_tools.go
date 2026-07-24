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
		Description:        "List system notifications, including alerts, task completion, and approval notifications.",
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
				"page":      map[string]interface{}{"type": "integer", "description": "One-based page number."},
				"page_size": map[string]interface{}{"type": "integer", "description": "Items per page."},
				"type":      map[string]interface{}{"type": "string", "description": "Notification type filter."},
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

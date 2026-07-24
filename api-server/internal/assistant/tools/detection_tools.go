package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
	"api-server/internal/repository"
)

// DetectionToolDeps 检测工具依赖
type DetectionToolDeps struct {
	AlertRepo     *repository.AlertRepository
	BlockRepo     *repository.BlockRepository
	SigmaRuleRepo *repository.SigmaRuleRepository
}

// RegisterDetectionTools 注册检测域工具
func RegisterDetectionTools(registry *assistant.ToolRegistry, deps DetectionToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Alert.List",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpList,
		Capability:         "list_detection_alerts",
		Description:        "List anomaly-detection alerts and security events with hostname, time-range, and disposition-status filters.",
		Aliases:            []string{"异常事件", "异常检测", "告警列表", "安全告警", "威胁事件"},
		Tags:               []string{"detection", "alert", "event", "abnormal", "ai-analysis"},
		ObjectTypes:        []string{"alert", "detection", "event"},
		PageRoutes:         []string{"/detection", "/detection/alerts", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":       map[string]interface{}{"type": "integer", "description": "One-based page number."},
				"page_size":  map[string]interface{}{"type": "integer", "description": "Items per page."},
				"hostnames":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Exact hostnames to include."},
				"status":     map[string]interface{}{"type": "string", "enum": []interface{}{"pending", "resolved"}, "description": "Alert status filter."},
				"start_time": map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive start time."},
				"end_time":   map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive end time."},
			},
		},
		Handler: makeAlertListHandler(deps.AlertRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Alert.Get",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGet,
		Capability:         "get_detection_alert",
		Description:        "Get anomaly-event details, matched-rule data, and context by alert ID.",
		Aliases:            []string{"告警详情", "异常事件详情", "规则命中详情", "安全事件详情"},
		Tags:               []string{"detection", "alert", "detail", "rule", "event"},
		ObjectTypes:        []string{"alert", "detection", "event"},
		PageRoutes:         []string{"/detection", "/detection/alerts", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"alert_id": map[string]interface{}{"type": "string", "description": "Alert reference from list results, preferably alert_id such as ALT-xxxx; database UUID is also accepted."},
			},
			"required": []string{"alert_id"},
		},
		Handler: makeAlertGetHandler(deps.AlertRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Statistics.Get",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGet,
		Capability:         "get_detection_statistics",
		Description:        "Get anomaly-detection and threat statistics, including today's alerts, blocks, affected hosts, and active rules.",
		Aliases:            []string{"威胁统计", "异常检测统计", "检测概览", "AI 分析概览"},
		Tags:               []string{"detection", "statistics", "alert", "sigma", "ai-analysis"},
		ObjectTypes:        []string{"detection", "alert", "sigma_rule"},
		PageRoutes:         []string{"/detection", "/detection/dashboard", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: makeThreatStatisticsHandler(deps.AlertRepo, deps.BlockRepo, deps.SigmaRuleRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Trend.Get",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGet,
		Capability:         "get_detection_trend",
		Description:        "Get hourly aggregated anomaly-detection alert trends.",
		Aliases:            []string{"告警趋势", "异常趋势", "威胁趋势", "检测趋势"},
		Tags:               []string{"detection", "trend", "alert", "event"},
		ObjectTypes:        []string{"detection", "alert", "event"},
		PageRoutes:         []string{"/detection", "/detection/dashboard", "/detection/ai-analysis"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hours": map[string]interface{}{"type": "integer", "minimum": 1, "description": "Lookback window in hours; defaults to 24."},
			},
		},
		Handler: makeAlertTrendHandler(deps.AlertRepo),
	}); err != nil {
		return err
	}

	return nil
}

func makeAlertListHandler(repo *repository.AlertRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		page := getIntArg(args, "page", 1)
		pageSize := getIntArg(args, "page_size", 20)

		filters := make(map[string]interface{})
		if hostnames, ok := args["hostnames"]; ok {
			filters["hostnames"] = hostnames
		}
		if status := getStringArg(args, "status", ""); status != "" {
			filters["status"] = status
		}
		if startTime := getStringArg(args, "start_time", ""); startTime != "" {
			filters["start_time"] = startTime
		}
		if endTime := getStringArg(args, "end_time", ""); endTime != "" {
			filters["end_time"] = endTime
		}

		alerts, total, err := repo.List(page, pageSize, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to list alerts: %w", err)
		}

		return map[string]interface{}{
			"data":  alerts,
			"total": total,
		}, nil
	}
}

func makeAlertGetHandler(repo *repository.AlertRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		alertID := getStringArg(args, "alert_id", "")
		if alertID == "" {
			return nil, fmt.Errorf("alert_id is required")
		}

		alert, err := repo.FindByID(alertID)
		if err != nil {
			return nil, fmt.Errorf("failed to find alert: %w", err)
		}

		return alert, nil
	}
}

func makeThreatStatisticsHandler(
	alertRepo *repository.AlertRepository,
	blockRepo *repository.BlockRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		todayAlerts, err1 := alertRepo.GetTodayCount()
		todayBlocks, err2 := blockRepo.GetTodayCount()
		affectedHosts, err3 := alertRepo.GetAffectedHostCount()
		activeRules, err4 := sigmaRuleRepo.GetActiveCount()

		// 收集错误但不阻断返回，部分数据仍可用
		var warnings []string
		if err1 != nil {
			warnings = append(warnings, fmt.Sprintf("today_alerts query failed: %v", err1))
		}
		if err2 != nil {
			warnings = append(warnings, fmt.Sprintf("today_blocks query failed: %v", err2))
		}
		if err3 != nil {
			warnings = append(warnings, fmt.Sprintf("affected_hosts query failed: %v", err3))
		}
		if err4 != nil {
			warnings = append(warnings, fmt.Sprintf("active_rules query failed: %v", err4))
		}

		result := map[string]interface{}{
			"today_alerts":   todayAlerts,
			"today_blocks":   todayBlocks,
			"affected_hosts": affectedHosts,
			"active_rules":   activeRules,
		}
		if len(warnings) > 0 {
			result["warnings"] = warnings
		}
		return result, nil
	}
}

func makeAlertTrendHandler(repo *repository.AlertRepository) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		hours := getIntArg(args, "hours", 24)
		if hours <= 0 {
			hours = 24
		}

		trend, err := repo.GetTrend(hours)
		if err != nil {
			return nil, fmt.Errorf("failed to get alert trend: %w", err)
		}

		return trend, nil
	}
}

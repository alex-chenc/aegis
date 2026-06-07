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
		Domain:             "detection",
		Operation:          "alert_list",
		Description:        "列出告警，支持按主机名、时间范围等条件筛选",
		Risk:               assistant.ToolRiskLow,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"page":       map[string]interface{}{"type": "integer", "description": "页码"},
				"page_size":  map[string]interface{}{"type": "integer", "description": "每页数量"},
				"hostnames":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "主机名列表"},
				"status":     map[string]interface{}{"type": "string", "description": "告警状态（pending/resolved）"},
				"start_time": map[string]interface{}{"type": "string", "description": "开始时间"},
				"end_time":   map[string]interface{}{"type": "string", "description": "结束时间"},
			},
		},
		Handler: makeAlertListHandler(deps.AlertRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Alert.Get",
		Domain:             "detection",
		Operation:          "alert_get",
		Description:        "根据告警ID获取告警详情",
		Risk:               assistant.ToolRiskLow,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"alert_id": map[string]interface{}{"type": "string", "description": "告警ID。优先传列表结果中的 alert_id（如 ALT-xxxx），也兼容列表结果中的数据库 UUID id"},
			},
			"required": []string{"alert_id"},
		},
		Handler: makeAlertGetHandler(deps.AlertRepo),
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Detection.Statistics.Get",
		Domain:             "detection",
		Operation:          "statistics_get",
		Description:        "获取威胁统计概览，包含今日告警数、拦截数、受影响主机数和活跃规则数",
		Risk:               assistant.ToolRiskLow,
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
		Domain:             "detection",
		Operation:          "trend_get",
		Description:        "获取告警趋势数据，按小时聚合",
		Risk:               assistant.ToolRiskLow,
		Enabled:            true,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hours": map[string]interface{}{"type": "integer", "description": "查询时间范围（小时），默认24"},
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

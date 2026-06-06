package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"go.uber.org/zap"
)

// ExternalMCPQueryPlanner 外部 MCP 查询规划器
// 负责根据用户意图和可用数据源生成查询计划
type ExternalMCPQueryPlanner struct {
	sourceService  *ExternalMCPSourceService
	promptProvider *ExternalMCPPromptProvider
	logger         *zap.Logger
}

// NewExternalMCPQueryPlanner 创建查询规划器
func NewExternalMCPQueryPlanner(
	sourceService *ExternalMCPSourceService,
	promptProvider *ExternalMCPPromptProvider,
	logger *zap.Logger,
) *ExternalMCPQueryPlanner {
	return &ExternalMCPQueryPlanner{
		sourceService:  sourceService,
		promptProvider: promptProvider,
		logger:         logger,
	}
}

// QueryPlan 查询计划
type QueryPlan struct {
	NeedExternalData bool             `json:"need_external_data"`
	Reason           string           `json:"reason"`
	SelectedSources  []SelectedSource `json:"selected_sources"`
	QueryItems       []QueryPlanItem  `json:"query_plan"`
	SafetyNotes      []string         `json:"safety_notes"`
}

// SelectedSource 选中的数据源
type SelectedSource struct {
	SourceID   string `json:"source_id"`
	SourceType string `json:"source_type"`
	Why        string `json:"why"`
}

// QueryPlanItem 查询计划项
type QueryPlanItem struct {
	SourceID       string            `json:"source_id"`
	QueryGoal      string            `json:"query_goal"`
	TimeRange      TimeRange         `json:"time_range"`
	Filters        map[string]string `json:"filters"`
	MaxRows        int               `json:"max_rows"`
	ExpectedFields []string          `json:"expected_fields"`
}

// TimeRange 时间范围
type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// PlanFromIntent 从意图生成查询计划
func (p *ExternalMCPQueryPlanner) PlanFromIntent(ctx context.Context, intent IntentResult, userMessage string) (*QueryPlan, error) {
	// 获取可用数据源
	query := repository.MCPSourceQuery{
		Enabled:  boolPtr(true),
		Page:     1,
		PageSize: 100,
	}
	sources, _, err := p.sourceService.ListSources(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP sources: %w", err)
	}

	// 转换为视图
	sourceViews := convertSourcesToViews(sources)

	// 如果没有可用数据源，直接返回
	if len(sourceViews) == 0 {
		return &QueryPlan{
			NeedExternalData: false,
			Reason:           "没有可用的外部 MCP 数据源",
		}, nil
	}

	// 检查意图是否需要外部数据
	needExternal := p.checkNeedExternalData(intent, sourceViews)
	if !needExternal {
		return &QueryPlan{
			NeedExternalData: false,
			Reason:           "当前意图不需要外部数据",
		}, nil
	}

	// 选择相关数据源
	selected := p.selectRelevantSources(intent, sourceViews)

	// 生成查询计划
	plan := &QueryPlan{
		NeedExternalData: true,
		Reason:           fmt.Sprintf("需要查询 %d 个外部数据源", len(selected)),
		SelectedSources:  selected,
		QueryItems:       p.generateQueryItems(selected, intent, userMessage),
		SafetyNotes:      p.generateSafetyNotes(selected),
	}

	p.logger.Info("query plan generated",
		zap.Bool("need_external", plan.NeedExternalData),
		zap.Int("sources_count", len(plan.SelectedSources)),
		zap.Int("queries_count", len(plan.QueryItems)),
	)

	return plan, nil
}

// checkNeedExternalData 检查是否需要外部数据
func (p *ExternalMCPQueryPlanner) checkNeedExternalData(intent IntentResult, sources []MCPSourceView) bool {
	// 检查关键词是否与外部数据相关
	externalKeywords := []string{"SIEM", "CMDB", "EDR", "威胁情报", "工单", "外部", "关联"}

	// 将所有关键词合并为一个字符串进行检查
	allKeywords := strings.Join(intent.Keywords, " ")

	for _, kw := range externalKeywords {
		if strings.Contains(strings.ToLower(allKeywords), strings.ToLower(kw)) {
			return true
		}
	}

	// 检查域是否包含 external_mcp
	for _, domain := range intent.Domains {
		if domain == "external_mcp" {
			return true
		}
	}

	return false
}

// selectRelevantSources 选择相关数据源
func (p *ExternalMCPQueryPlanner) selectRelevantSources(intent IntentResult, sources []MCPSourceView) []SelectedSource {
	var selected []SelectedSource

	// 将所有关键词合并为一个字符串进行检查
	allKeywords := strings.Join(intent.Keywords, " ")

	// 根据意图选择数据源
	for _, source := range sources {
		relevant := false
		reason := ""

		switch source.SourceType {
		case "siem":
			if containsAnyKeyword(allKeywords, []string{"日志", "事件", "登录", "告警", "SIEM"}) {
				relevant = true
				reason = "需要查询日志和事件"
			}
		case "cmdb":
			if containsAnyKeyword(allKeywords, []string{"资产", "主机", "业务", "归属", "CMDB"}) {
				relevant = true
				reason = "需要查询资产信息"
			}
		case "edr":
			if containsAnyKeyword(allKeywords, []string{"终端", "进程", "EDR", "隔离"}) {
				relevant = true
				reason = "需要查询终端信息"
			}
		case "ticket":
			if containsAnyKeyword(allKeywords, []string{"工单", "处置", "变更", "记录"}) {
				relevant = true
				reason = "需要查询工单记录"
			}
		case "threat_intel":
			if containsAnyKeyword(allKeywords, []string{"IOC", "情报", "威胁", "IP", "域名"}) {
				relevant = true
				reason = "需要查询威胁情报"
			}
		}

		if relevant {
			selected = append(selected, SelectedSource{
				SourceID:   source.SourceID,
				SourceType: source.SourceType,
				Why:        reason,
			})
		}
	}

	// 最多选择 3 个数据源
	if len(selected) > 3 {
		selected = selected[:3]
	}

	return selected
}

// generateQueryItems 生成查询项
func (p *ExternalMCPQueryPlanner) generateQueryItems(selected []SelectedSource, intent IntentResult, userMessage string) []QueryPlanItem {
	var items []QueryPlanItem

	now := time.Now()
	defaultFrom := now.Add(-24 * time.Hour).Format(time.RFC3339)
	defaultTo := now.Format(time.RFC3339)

	for _, s := range selected {
		item := QueryPlanItem{
			SourceID:  s.SourceID,
			QueryGoal: fmt.Sprintf("查询 %s 相关数据", s.SourceType),
			TimeRange: TimeRange{
				From: defaultFrom,
				To:   defaultTo,
			},
			Filters: make(map[string]string),
			MaxRows: 50,
		}

		// 从意图中提取过滤条件
		for _, objID := range intent.ObjectIDs {
			item.Filters["host_id"] = objID
		}

		items = append(items, item)
	}

	return items
}

// generateSafetyNotes 生成安全提示
func (p *ExternalMCPQueryPlanner) generateSafetyNotes(selected []SelectedSource) []string {
	notes := []string{
		"限制查询时间范围为最近 24 小时",
		"限制返回行数不超过 50 行",
		"不查询与问题无关的数据源",
	}

	if len(selected) > 1 {
		notes = append(notes, fmt.Sprintf("同时查询 %d 个数据源，注意结果关联", len(selected)))
	}

	return notes
}

// containsAnyKeyword 检查字符串是否包含任一关键词
func containsAnyKeyword(s string, keywords []string) bool {
	sLower := strings.ToLower(s)
	for _, kw := range keywords {
		if strings.Contains(sLower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// boolPtr 返回布尔指针
func boolPtr(b bool) *bool {
	return &b
}

// convertSourcesToViews 将 model.ExternalMCPSource 转换为 MCPSourceView
func convertSourcesToViews(sources []model.ExternalMCPSource) []MCPSourceView {
	views := make([]MCPSourceView, len(sources))
	for i, s := range sources {
		// 解析 query_limits
		maxRows := 50
		timeout := 20
		if s.QueryLimits != nil {
			var limits map[string]interface{}
			if err := json.Unmarshal(s.QueryLimits, &limits); err == nil {
				if mr, ok := limits["max_rows"].(float64); ok {
					maxRows = int(mr)
				}
				if t, ok := limits["timeout_seconds"].(float64); ok {
					timeout = int(t)
				}
			}
		}

		views[i] = MCPSourceView{
			SourceID:   s.SourceID,
			Name:       s.Name,
			SourceType: s.SourceType,
			Transport:  s.Transport,
			Enabled:    s.Enabled,
			MaxRows:    maxRows,
			Timeout:    timeout,
		}
	}
	return views
}

// MarshalQueryPlan 序列化查询计划
func MarshalQueryPlan(plan *QueryPlan) (string, error) {
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

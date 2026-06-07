package assistant

import (
	"strings"
)

// IntentResult 意图识别结果
type IntentResult struct {
	Domains    []string `json:"domains"`
	Action     string   `json:"action"`
	Object     string   `json:"object"`
	Confidence float64  `json:"confidence"`
}

// IntentRouter 意图路由器
type IntentRouter struct {
	keywords map[string][]string // domain -> keywords
}

// NewIntentRouter 创建意图路由器
func NewIntentRouter() *IntentRouter {
	return &IntentRouter{
		keywords: map[string][]string{
			"host":          {"主机", "资产", "agent", "离线", "在线", "主机列表", "资产态势"},
			"task":          {"任务", "基线", "检查", "修复", "扫描任务", "执行"},
			"vulnerability": {"漏洞", "CVE", "补丁", "修复脚本", "POC", "受影响"},
			"detection":     {"告警", "威胁", "检测", "阻断", "告警趋势", "威胁统计", "攻击矩阵"},
			"sigma_rule":    {"sigma", "规则", "检测规则", "规则生成", "规则激活"},
			"package":       {"检测包", "包", "签名", "构建", "启用", "禁用", "回滚", "hook", "allowlist"},
			"block":         {"阻断", "策略", "封禁", "白名单"},
			"config":        {"配置", "设置", "LLM", "系统配置"},
			"audit":         {"审计", "日志", "审计日志", "操作记录"},
			"investigation": {"研判", "攻击研判", "攻击路径", "入口", "溯源", "攻击时间线", "置信度"},
			"external_mcp":  {"MCP", "外部数据", "SIEM", "CMDB", "EDR", "威胁情报", "工单"},
		},
	}
}

// Classify 意图分类
func (r *IntentRouter) Classify(input IntentInput) IntentResult {
	query := strings.ToLower(input.Query)

	// Score each domain
	scores := make(map[string]float64)
	for domain, keywords := range r.keywords {
		score := 0.0
		for _, kw := range keywords {
			if strings.Contains(query, strings.ToLower(kw)) {
				score += 1.0
			}
		}
		if score > 0 {
			scores[domain] = score / float64(len(keywords))
		}
	}

	// Boost from page route
	if input.PageRoute != "" {
		pageRoute := strings.ToLower(input.PageRoute)
		for domain, keywords := range r.keywords {
			for _, kw := range keywords {
				if strings.Contains(pageRoute, strings.ToLower(kw)) {
					scores[domain] += 0.3
					break
				}
			}
		}
		// Direct route mapping
		routeDomainMap := map[string]string{
			"/hosts":        "host",
			"/baseline":     "task",
			"/vulnerability": "vulnerability",
			"/detection":    "detection",
			"/settings":     "config",
		}
		for route, domain := range routeDomainMap {
			if strings.Contains(pageRoute, route) {
				scores[domain] += 0.5
			}
		}
	}

	// Boost from context objects
	for _, ref := range input.ContextRefs {
		ctxDomain := r.mapContextType(ref.ObjectType)
		if ctxDomain != "" {
			scores[ctxDomain] += 0.4
		}
	}

	// Collect domains with score > 0
	var domains []string
	maxScore := 0.0
	for domain, score := range scores {
		if score > 0.1 {
			domains = append(domains, domain)
			if score > maxScore {
				maxScore = score
			}
		}
	}

	if len(domains) == 0 {
		// Default to host for general queries
		domains = []string{"host"}
		maxScore = 0.3
	}

	return IntentResult{
		Domains:    domains,
		Action:     r.inferAction(query),
		Object:     r.inferObject(query),
		Confidence: maxScore,
	}
}

func (r *IntentRouter) mapContextType(objectType string) string {
	mapping := map[string]string{
		"host":             "host",
		"alert":            "detection",
		"task":             "task",
		"vulnerability":    "vulnerability",
		"package":          "package",
		"detection_package": "package",
		"sigma_rule":       "sigma_rule",
	}
	return mapping[objectType]
}

func (r *IntentRouter) inferAction(query string) string {
	actionKeywords := map[string][]string{
		"query":    {"查询", "列出", "查看", "获取", "显示", "列表"},
		"analyze":  {"分析", "研判", "溯源", "调查"},
		"create":   {"创建", "新建", "生成", "添加"},
		"update":   {"更新", "修改", "编辑"},
		"delete":   {"删除", "移除"},
		"execute":  {"执行", "运行", "触发", "启动"},
		"approve":  {"批准", "审批", "通过"},
		"block":    {"阻断", "封禁", "拦截"},
	}
	for action, keywords := range actionKeywords {
		for _, kw := range keywords {
			if strings.Contains(query, kw) {
				return action
			}
		}
	}
	return "query"
}

func (r *IntentRouter) inferObject(query string) string {
	objectKeywords := map[string]string{
		"主机": "host", "资产": "host",
		"告警": "alert", "威胁": "alert",
		"任务": "task", "基线": "task",
		"漏洞": "vulnerability", "CVE": "vulnerability",
		"规则": "sigma_rule", "sigma": "sigma_rule",
		"检测包": "package", "包": "package",
		"阻断": "block", "策略": "block",
		"配置": "config",
		"审计": "audit",
	}
	for kw, obj := range objectKeywords {
		if strings.Contains(strings.ToLower(query), strings.ToLower(kw)) {
			return obj
		}
	}
	return ""
}

// IntentInput 意图输入
type IntentInput struct {
	Query       string             `json:"query"`
	PageRoute   string             `json:"page_route,omitempty"`
	ContextRefs []ContextRefInput  `json:"context_refs,omitempty"`
}

package assistant

import (
	"context"
	"encoding/json"
	"strings"

	"api-server/internal/llm"
)

// IntentResult 意图识别结果（对齐设计文档 6.1 节）
type IntentResult struct {
	Domains          []string   `json:"domains"`
	Operations       []string   `json:"operations,omitempty"`
	ObjectTypes      []string   `json:"object_types,omitempty"`
	ObjectIDs        []string   `json:"object_ids,omitempty"`
	Keywords         []string   `json:"keywords,omitempty"`
	ExplicitToolName string     `json:"explicit_tool_name,omitempty"`
	RiskHint         ToolRisk   `json:"risk_hint,omitempty"`
	NeedWrite        bool       `json:"need_write"`
	NeedApproval     bool       `json:"need_approval"`
	Confidence       float64    `json:"confidence"`
	Reason           string     `json:"reason,omitempty"`
	Action           string     `json:"action"`
	Object           string     `json:"object"`
}

// IntentRouter 意图路由器
type IntentRouter struct {
	keywords    map[string][]string // domain -> keywords
	llmClientFn func(ctx context.Context) (*llm.LLMClient, error)
}

// NewIntentRouter 创建意图路由器
func NewIntentRouter() *IntentRouter {
	return &IntentRouter{
		keywords: map[string][]string{
			"host":          {"主机", "资产", "agent", "离线", "在线", "主机列表", "资产态势", "软件", "安装"},
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
			"notification":  {"通知", "消息", "告警通知"},
		},
	}
}

// SetLLMClientFactory 设置 LLM 客户端工厂（用于低置信度时 LLM 分类降级）
func (r *IntentRouter) SetLLMClientFactory(fn func(ctx context.Context) (*llm.LLMClient, error)) {
	r.llmClientFn = fn
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
			"/hosts":         "host",
			"/baseline":      "task",
			"/vulnerability": "vulnerability",
			"/detection":     "detection",
			"/settings":      "config",
			"/packages":      "package",
			"/sigma":         "sigma_rule",
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

	action := r.inferAction(query)
	object := r.inferObject(query)

	result := IntentResult{
		Domains:    domains,
		Action:     action,
		Object:     object,
		Confidence: maxScore,
		NeedWrite:  r.inferNeedWrite(action),
		Keywords:   r.extractKeywords(query),
		RiskHint:   r.inferRiskHint(action),
		Reason:     r.buildReason(domains, action, maxScore),
	}

	// 检查是否有明确的工具名（如用户说 "调用 Package.Sign"）
	result.ExplicitToolName = r.extractExplicitToolName(query)

	// 推断操作类型
	result.Operations = r.inferOperations(action)

	return result
}

// ClassifyWithLLM 低置信度时使用 LLM 分类（设计文档 6.1 节：规则置信度低时调用 LLM）
func (r *IntentRouter) ClassifyWithLLM(ctx context.Context, input IntentInput) (IntentResult, error) {
	// 先用规则分类
	result := r.Classify(input)

	// 置信度足够高时直接返回
	if result.Confidence >= 0.5 && r.llmClientFn == nil {
		return result, nil
	}

	// 没有 LLM 客户端工厂时降级为规则结果
	if r.llmClientFn == nil {
		return result, nil
	}

	llmClient, err := r.llmClientFn(ctx)
	if err != nil || llmClient == nil {
		return result, nil
	}

	// 构建轻量 LLM 分类 prompt
	prompt := `你是意图分类器。根据用户消息，返回 JSON 格式的意图分析结果。
只返回 JSON，不要其他内容。

用户消息: ` + input.Query + `

返回格式:
{"domains":["域"],"action":"动作","object":"对象","risk_hint":"风险级别","need_write":false,"confidence":0.8,"reason":"分类依据"}

域可选值: host, task, vulnerability, detection, sigma_rule, package, block, config, audit, investigation, external_mcp, notification
动作可选值: query, analyze, create, update, delete, execute, approve, block
风险级别可选值: readonly, low, medium, high, critical`

	messages := []llm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: input.Query},
	}

	resp, err := llmClient.ChatCompletionWithMessages(ctx, messages, 0.1)
	if err != nil {
		return result, nil // 降级为规则结果
	}

	// 解析 LLM 返回的 JSON
	var llmResult struct {
		Domains    []string `json:"domains"`
		Action     string   `json:"action"`
		Object     string   `json:"object"`
		RiskHint   string   `json:"risk_hint"`
		NeedWrite  bool     `json:"need_write"`
		Confidence float64  `json:"confidence"`
		Reason     string   `json:"reason"`
	}

	if err := json.Unmarshal([]byte(resp), &llmResult); err != nil {
		return result, nil // 解析失败，降级为规则结果
	}

	// 用 LLM 结果增强规则结果
	if llmResult.Confidence > result.Confidence {
		if len(llmResult.Domains) > 0 {
			result.Domains = llmResult.Domains
		}
		if llmResult.Action != "" {
			result.Action = llmResult.Action
		}
		if llmResult.Object != "" {
			result.Object = llmResult.Object
		}
		result.Confidence = llmResult.Confidence
		result.Reason = llmResult.Reason
		if llmResult.RiskHint != "" {
			result.RiskHint = ToolRisk(llmResult.RiskHint)
		}
		result.NeedWrite = llmResult.NeedWrite
	}

	return result, nil
}

func (r *IntentRouter) mapContextType(objectType string) string {
	mapping := map[string]string{
		"host":              "host",
		"alert":             "detection",
		"task":              "task",
		"vulnerability":     "vulnerability",
		"package":           "package",
		"detection_package": "package",
		"sigma_rule":        "sigma_rule",
	}
	return mapping[objectType]
}

func (r *IntentRouter) inferAction(query string) string {
	actionKeywords := map[string][]string{
		"query":   {"查询", "列出", "查看", "获取", "显示", "列表", "有哪些", "多少"},
		"analyze": {"分析", "研判", "溯源", "调查", "检测", "扫描"},
		"create":  {"创建", "新建", "生成", "添加", "注册"},
		"update":  {"更新", "修改", "编辑", "调整"},
		"delete":  {"删除", "移除", "清理"},
		"execute": {"执行", "运行", "触发", "启动", "部署"},
		"approve": {"批准", "审批", "通过", "签名"},
		"block":   {"阻断", "封禁", "拦截", "禁止"},
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
		"通知": "notification",
	}
	for kw, obj := range objectKeywords {
		if strings.Contains(strings.ToLower(query), strings.ToLower(kw)) {
			return obj
		}
	}
	return ""
}

func (r *IntentRouter) inferNeedWrite(action string) bool {
	writeActions := map[string]bool{
		"create": true, "update": true, "delete": true,
		"execute": true, "approve": true, "block": true,
	}
	return writeActions[action]
}

func (r *IntentRouter) inferRiskHint(action string) ToolRisk {
	switch action {
	case "delete", "block", "approve":
		return ToolRiskCritical
	case "execute", "update":
		return ToolRiskHigh
	case "create":
		return ToolRiskMedium
	default:
		return ToolRiskReadonly
	}
}

func (r *IntentRouter) extractKeywords(query string) []string {
	var keywords []string
	for _, domainKws := range r.keywords {
		for _, kw := range domainKws {
			if strings.Contains(query, strings.ToLower(kw)) {
				keywords = append(keywords, kw)
			}
		}
	}
	return keywords
}

func (r *IntentRouter) extractExplicitToolName(query string) string {
	// 检查用户是否明确提到了工具名（如 "调用 Package.Sign"）
	toolIndicators := []string{"调用", "使用", "执行", "运行"}
	for _, indicator := range toolIndicators {
		if idx := strings.Index(query, indicator); idx >= 0 {
			rest := query[idx+len(indicator):]
			rest = strings.TrimSpace(rest)
			// 工具名格式: Domain.Operation
			if dotIdx := strings.Index(rest, "."); dotIdx > 0 {
				candidate := rest[:dotIdx]
				// 检查是否是已知域名
				for _, domain := range r.keywords {
					for _, kw := range domain {
						if strings.EqualFold(candidate, kw) || strings.Contains(strings.ToLower(candidate), strings.ToLower(kw)) {
							// 找到域名，尝试提取完整工具名
							parts := strings.Fields(rest)
							if len(parts) > 0 {
								return parts[0]
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func (r *IntentRouter) inferOperations(action string) []string {
	switch action {
	case "query":
		return []string{"list", "get", "search"}
	case "analyze":
		return []string{"get", "search", "generate"}
	case "create":
		return []string{"create", "generate"}
	case "update":
		return []string{"update"}
	case "delete":
		return []string{"delete"}
	case "execute":
		return []string{"execute", "dispatch"}
	case "approve":
		return []string{"approve"}
	case "block":
		return []string{"execute", "block"}
	default:
		return []string{"list", "get"}
	}
}

func (r *IntentRouter) buildReason(domains []string, action string, confidence float64) string {
	if confidence >= 0.7 {
		return "关键词匹配明确"
	}
	if confidence >= 0.4 {
		return "部分关键词匹配"
	}
	return "低置信度，可能需要 LLM 辅助分类"
}

// IntentInput 意图输入
type IntentInput struct {
	Query       string             `json:"query"`
	PageRoute   string             `json:"page_route,omitempty"`
	ContextRefs []ContextRefInput  `json:"context_refs,omitempty"`
}

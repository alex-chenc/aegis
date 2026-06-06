package assistant

import (
	"sort"
	"strings"
)

// ToolSelector 工具选择器
type ToolSelector struct {
	catalog  *ToolCatalog
	registry *ToolRegistry
}

// NewToolSelector 创建工具选择器
func NewToolSelector(catalog *ToolCatalog, registry *ToolRegistry) *ToolSelector {
	return &ToolSelector{
		catalog:  catalog,
		registry: registry,
	}
}

// Select 根据意图选择工具
func (s *ToolSelector) Select(input ToolSelectionInput) *ToolSelectionResult {
	allTools := s.registry.List()

	// Score each tool
	type scoredTool struct {
		tool  *ToolSpec
		score float64
	}
	var scored []scoredTool

	for _, tool := range allTools {
		if !tool.Enabled {
			continue
		}

		score := s.scoreTool(tool, input)

		// Filter: critical tools excluded unless explicit intent
		if tool.Risk == ToolRiskCritical && !input.ExplicitHighRisk {
			continue
		}

		// Filter: high risk write tools excluded unless explicit intent
		if s.isWriteTool(tool) && !input.ExplicitWrite {
			if tool.Risk == ToolRiskHigh || tool.Risk == ToolRiskCritical {
				continue
			}
		}

		// Filter: agent tools only for security analysis intents
		// Agent 工具（进程采集、网络采集等）通过 gRPC 调用目标主机 Agent，
		// 只应在安全分析场景（事件分析、攻击研判、威胁检测）下注入。
		// 普通资源查询（主机列表、资产统计等）应直接使用服务端数据库查询工具。
		if tool.Domain == DomainAgent && !s.IntentRequiresAgentTools(input.Intent) {
			continue
		}

		if score > 0 {
			scored = append(scored, scoredTool{tool: tool, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Apply limits
	maxTools := input.MaxTools
	if maxTools <= 0 {
		maxTools = 24
	}
	maxWriteTools := input.MaxWriteTools
	if maxWriteTools <= 0 {
		maxWriteTools = 6
	}

	var selected []*ToolSpec
	writeCount := 0
	for _, st := range scored {
		if len(selected) >= maxTools {
			break
		}
		if s.isWriteTool(st.tool) {
			if writeCount >= maxWriteTools {
				continue
			}
			writeCount++
		}
		selected = append(selected, st.tool)
	}

	// Always include resident tools
	residentTools := []string{"Tool.Search", "Context.Get", "Session.Summarize"}
	for _, name := range residentTools {
		found := false
		for _, t := range selected {
			if t.Name == name {
				found = true
				break
			}
		}
		if !found {
			if tool, ok := s.registry.Get(name); ok {
				selected = append(selected, tool)
			}
		}
	}

	// Build result
	var selectedNames []string
	var candidateNames []string
	for _, t := range selected {
		selectedNames = append(selectedNames, t.Name)
	}
	for _, st := range scored {
		candidateNames = append(candidateNames, st.tool.Name)
	}

	return &ToolSelectionResult{
		SelectedTools:  selectedNames,
		CandidateTools: candidateNames,
		Query:          input.Query,
		Intent:         input.Intent,
		MaxTools:       maxTools,
	}
}

// Expand 扩展工具集（用于 Tool.Search 两阶段扩展）
func (s *ToolSelector) Expand(currentTools []string, expansionQuery string, maxAdd int) []string {
	if maxAdd <= 0 {
		maxAdd = 10
	}

	currentSet := make(map[string]bool)
	for _, name := range currentTools {
		currentSet[name] = true
	}

	// Search for matching tools
	searchResults := s.catalog.Search(expansionQuery, SearchOptions{MaxResults: maxAdd * 2})

	var expanded []string
	for _, tool := range searchResults {
		if len(expanded) >= maxAdd {
			break
		}
		if !currentSet[tool.Name] {
			expanded = append(expanded, tool.Name)
		}
	}

	return expanded
}

// scoreTool 评分工具（对齐设计文档 6.3 节评分规则）
//
// score =
//
//	0.35 * domain_match
//	+ 0.20 * operation_match
//	+ 0.15 * keyword_match
//	+ 0.10 * page_route_match
//	+ 0.10 * context_object_match
//	+ 0.05 * recent_usage_match
//	+ 0.05 * risk_fit
func (s *ToolSelector) scoreTool(tool *ToolSpec, input ToolSelectionInput) float64 {
	score := 0.0

	// Domain match (0.35)
	// 1. 直接域匹配
	domainMatched := false
	for _, domain := range input.Intent.Domains {
		if string(tool.Domain) == domain {
			domainMatched = true
			break
		}
	}
	// 2. ObjectTypes 匹配（工具的 ObjectTypes 包含意图的域）
	if !domainMatched {
		for _, domain := range input.Intent.Domains {
			for _, ot := range tool.ObjectTypes {
				if ot == domain {
					domainMatched = true
					break
				}
			}
			if domainMatched {
				break
			}
		}
	}
	if domainMatched {
		score += 0.35
	}

	// Operation match (0.20)
	if input.Intent.Action != "" {
		action := strings.ToLower(input.Intent.Action)
		op := strings.ToLower(string(tool.Operation))
		if strings.Contains(op, action) || action == "query" && (op == "list" || op == "get" || op == "search") {
			score += 0.20
		}
	}

	// Keyword match (0.15) — 匹配 name, description, aliases, tags
	// 支持双向匹配：查询包含关键词，或关键词包含查询
	query := strings.ToLower(input.Query)
	if query != "" {
		matched := false
		name := strings.ToLower(tool.Name)
		desc := strings.ToLower(tool.Description)

		// 1. 工具名或描述包含查询
		if strings.Contains(name, query) || strings.Contains(desc, query) {
			matched = true
		}

		// 2. 查询包含工具名（去掉点号后匹配）
		if !matched {
			cleanName := strings.ReplaceAll(name, ".", " ")
			if strings.Contains(query, cleanName) {
				matched = true
			}
		}

		// 3. 别名匹配（双向）
		if !matched {
			for _, alias := range tool.Aliases {
				aliasLower := strings.ToLower(alias)
				if strings.Contains(aliasLower, query) || strings.Contains(query, aliasLower) {
					matched = true
					break
				}
			}
		}

		// 4. 标签匹配（双向）
		if !matched {
			for _, tag := range tool.Tags {
				tagLower := strings.ToLower(tag)
				if strings.Contains(tagLower, query) || strings.Contains(query, tagLower) {
					matched = true
					break
				}
			}
		}

		// 5. 查询分词匹配（将查询按空格/标点分词，任一词匹配即算匹配）
		if !matched {
			// 提取查询中的关键词（简单分词）
			words := tokenizeQuery(query)
			for _, word := range words {
				if len(word) < 2 {
					continue
				}
				if strings.Contains(name, word) || strings.Contains(desc, word) {
					matched = true
					break
				}
				for _, alias := range tool.Aliases {
					if strings.Contains(strings.ToLower(alias), word) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
				for _, tag := range tool.Tags {
					if strings.Contains(strings.ToLower(tag), word) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}

		if matched {
			score += 0.15
		}
	}

	// Page route match (0.10)
	if input.PageRoute != "" {
		pageRoute := strings.ToLower(input.PageRoute)
		for _, pr := range tool.PageRoutes {
			if strings.Contains(pageRoute, strings.ToLower(pr)) {
				score += 0.10
				break
			}
		}
		// 降级：按域名匹配
		if score < 0.10 {
			domain := strings.ToLower(string(tool.Domain))
			if strings.Contains(pageRoute, domain) {
				score += 0.05
			}
		}
	}

	// Context object match (0.10)
	for _, ref := range input.ContextRefs {
		if string(tool.Domain) == s.mapContextToDomain(ref.ObjectType) {
			score += 0.10
			break
		}
		// 检查 ObjectTypes
		for _, ot := range tool.ObjectTypes {
			if ot == ref.ObjectType {
				score += 0.10
				break
			}
		}
	}

	// Risk fit (0.05) - prefer readonly for general queries
	if tool.Risk == ToolRiskReadonly {
		score += 0.05
	}

	// Query tools always prioritized
	if tool.Risk == ToolRiskReadonly && input.Intent.Action == "query" {
		score += 0.05
	}

	return score
}

func (s *ToolSelector) isWriteTool(tool *ToolSpec) bool {
	return tool.Risk != ToolRiskReadonly && tool.Risk != ToolRiskLow
}

func (s *ToolSelector) mapContextToDomain(objectType string) string {
	mapping := map[string]string{
		"host":              "host",
		"alert":             "detection",
		"task":              "task",
		"vulnerability":     "vulnerability",
		"detection_package": "package",
		"package":           "package",
		"sigma_rule":        "sigma_rule",
	}
	return mapping[objectType]
}

// IntentRequiresAgentTools 判断意图是否需要 agent 工具
// agent 工具（进程采集、网络采集等）通过 gRPC 调用目标主机 Agent，
// 只应在安全分析场景下使用。普通资源查询应直接查数据库。
func (s *ToolSelector) IntentRequiresAgentTools(intent IntentResult) bool {
	// 1. 分析类动作 → 需要 agent 工具
	if intent.Action == "analyze" || intent.Action == "investigate" {
		return true
	}

	// 2. 安全分析相关领域 → 需要 agent 工具
	securityDomains := map[string]bool{
		"detection":     true,
		"investigation": true,
		"sigma_rule":    true,
		"block":         true,
	}
	for _, d := range intent.Domains {
		if securityDomains[d] {
			return true
		}
	}

	// 3. 普通资源查询（host/query、task/query 等）→ 不需要 agent 工具
	return false
}

// ToolSelectionInput 工具选择输入
type ToolSelectionInput struct {
	Query            string           `json:"query"`
	PageRoute        string           `json:"page_route,omitempty"`
	ContextRefs      []ContextRefInput `json:"context_refs,omitempty"`
	Intent           IntentResult     `json:"intent"`
	MaxTools         int              `json:"max_tools,omitempty"`
	MaxWriteTools    int              `json:"max_write_tools,omitempty"`
	ExplicitHighRisk bool             `json:"explicit_high_risk,omitempty"`
	ExplicitWrite    bool             `json:"explicit_write,omitempty"`
}

// ToolSelectionResult 工具选择结果
type ToolSelectionResult struct {
	SelectedTools  []string     `json:"selected_tools"`
	CandidateTools []string     `json:"candidate_tools"`
	Query          string       `json:"query"`
	Intent         IntentResult `json:"intent"`
	MaxTools       int          `json:"max_tools"`
}

// ToolNames 返回选中工具名列表
func (r ToolSelectionResult) ToolNames() []string {
	return r.SelectedTools
}

// tokenizeQuery 将查询分词（简单实现：按空格和中文标点分词）
func tokenizeQuery(query string) []string {
	// 替换中文标点为空格
	replacer := strings.NewReplacer(
		"，", " ",
		"。", " ",
		"？", " ",
		"！", " ",
		"、", " ",
		"；", " ",
		"：", " ",
		"（", " ",
		"）", " ",
		"【", " ",
		"】", " ",
		"《", " ",
		"》", " ",
		"“", " ",
		"”", " ",
		"‘", " ",
		"’", " ",
		"「", " ",
		"」", " ",
		"『", " ",
		"』", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"<", " ",
		">", " ",
		",", " ",
		".", " ",
		"?", " ",
		"!", " ",
		";", " ",
		":", " ",
		"'", " ",
		"\"", " ",
		"/", " ",
		"\\", " ",
		"|", " ",
		"~", " ",
		"`", " ",
		"@", " ",
		"#", " ",
		"$", " ",
		"%", " ",
		"^", " ",
		"&", " ",
		"*", " ",
		"+", " ",
		"=", " ",
		"_", " ",
		"-", " ",
	)

	cleaned := replacer.Replace(query)
	words := strings.Fields(cleaned)

	// 过滤空字符串
	var result []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w != "" {
			result = append(result, w)
		}
	}
	return result
}

package assistant

import (
	agentruntime "github.com/alex-chenc/agent-runtime"
	"sort"
	"strings"
	"time"
)

// ToolCatalog 工具目录
type ToolCatalog struct {
	registry *ToolRegistry
}

// NewToolCatalog 创建工具目录
func NewToolCatalog(registry *ToolRegistry) *ToolCatalog {
	return &ToolCatalog{registry: registry}
}

// Resolve 按名称解析工具
func (c *ToolCatalog) Resolve(name string) (ToolSpec, bool) {
	tool, ok := c.registry.Get(name)
	if !ok {
		return ToolSpec{}, false
	}
	return *tool, true
}

// Search 搜索工具（支持名称、别名、描述、标签、域、操作匹配）
func (c *ToolCatalog) Search(query string, opts SearchOptions) []*ToolSpec {
	tools := c.registry.List()

	query = strings.ToLower(strings.TrimSpace(query))

	type scoredTool struct {
		tool  *ToolSpec
		score int
	}
	var scored []scoredTool

	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		if opts.Domain != "" && string(tool.Domain) != opts.Domain {
			continue
		}
		if opts.RiskLevel != "" && string(tool.Risk) != opts.RiskLevel {
			continue
		}

		// Score based on keyword match
		score := 0
		if query != "" {
			name := strings.ToLower(tool.Name)
			desc := strings.ToLower(tool.Description)
			domain := strings.ToLower(string(tool.Domain))
			operation := strings.ToLower(string(tool.Operation))
			capability := strings.ToLower(tool.Capability)

			if strings.Contains(name, query) {
				score += 10
			}
			if strings.Contains(domain, query) {
				score += 8
			}
			if strings.Contains(operation, query) {
				score += 6
			}
			if capability != "" && strings.Contains(capability, query) {
				score += 5
			}
			if strings.Contains(desc, query) {
				score += 4
			}

			// Check aliases
			for _, alias := range tool.Aliases {
				if strings.Contains(strings.ToLower(alias), query) {
					score += 7
					break
				}
			}

			// Check tags
			for _, tag := range tool.Tags {
				if strings.Contains(strings.ToLower(tag), query) {
					score += 3
					break
				}
			}

			if score == 0 {
				continue
			}
		} else {
			score = 1
		}

		scored = append(scored, scoredTool{tool: tool, score: score})
	}

	// Sort by relevance (higher score first, then by name)
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].tool.Name < scored[j].tool.Name
	})

	results := make([]*ToolSpec, 0, len(scored))
	for _, st := range scored {
		results = append(results, st.tool)
	}

	if opts.MaxResults > 0 && len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results
}

// ListByDomain 按域列出工具
func (c *ToolCatalog) ListByDomain(domain ToolDomain) []*ToolSpec {
	return c.registry.ListByDomain(string(domain))
}

// ListForPageRoute 按页面路由列出工具
func (c *ToolCatalog) ListForPageRoute(route string) []*ToolSpec {
	tools := c.registry.List()
	var result []*ToolSpec
	routeLower := strings.ToLower(route)
	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		for _, pr := range tool.PageRoutes {
			if strings.Contains(routeLower, strings.ToLower(pr)) {
				result = append(result, tool)
				break
			}
		}
	}
	return result
}

// BuildDescriptors 构建 agent-runtime ToolDescriptor 列表
func (c *ToolCatalog) BuildDescriptors(toolNames []string) []agentruntime.ToolDescriptor {
	var descriptors []agentruntime.ToolDescriptor
	for _, name := range toolNames {
		tool, ok := c.registry.Get(name)
		if !ok || !tool.Enabled {
			continue
		}
		descriptors = append(descriptors, tool.Descriptor())
	}
	return descriptors
}

// GetDomains 获取所有域
func (c *ToolCatalog) GetDomains() []string {
	tools := c.registry.List()
	domainSet := make(map[string]bool)
	for _, t := range tools {
		domainSet[string(t.Domain)] = true
	}
	domains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	return domains
}

// SearchOptions 搜索选项
type SearchOptions struct {
	Domain     string
	RiskLevel  string
	MaxResults int
}

// ToolSearchArgs Tool.Search 元工具的参数
type ToolSearchArgs struct {
	Query      string   `json:"query"`
	Domains    []string `json:"domains,omitempty"`
	Operations []string `json:"operations,omitempty"`
	MaxResults int      `json:"max_results,omitempty"`
}

// ToolSearchResult Tool.Search 元工具的返回结果
type ToolSearchResult struct {
	Matches []ToolSearchItem `json:"matches"`
}

// ToolSearchItem Tool.Search 返回的单个工具信息
type ToolSearchItem struct {
	Name        string   `json:"name"`
	Domain      string   `json:"domain"`
	Operation   string   `json:"operation"`
	Risk        string   `json:"risk"`
	Description string   `json:"description"`
	ArgsSummary string   `json:"args_summary"`
	Tags        []string `json:"tags"`
}

// toRuntimeRisk 将 ToolRisk 映射到 agent-runtime RiskLevel
func toRuntimeRisk(risk ToolRisk) agentruntime.RiskLevel {
	switch risk {
	case ToolRiskReadonly:
		return agentruntime.RiskReadOnly
	case ToolRiskLow, ToolRiskMedium:
		return agentruntime.RiskLow
	case ToolRiskHigh:
		return agentruntime.RiskHigh
	case ToolRiskCritical:
		return agentruntime.RiskDangerous
	default:
		return agentruntime.RiskLow
	}
}

// Descriptor 将 ToolSpec 转换为 agentruntime.ToolDescriptor
func (s ToolSpec) Descriptor() agentruntime.ToolDescriptor {
	return agentruntime.ToolDescriptor{
		Name:             s.Name,
		Description:      modelFacingToolDescription(&s),
		ArgsSchema:       normalizeRuntimeArgsSchema(s.ArgsSchema),
		ResultSchema:     s.ResultSchema,
		Prerequisites:    toRuntimePrerequisites(s.ExecutionContract.Prerequisites),
		RiskLevel:        toRuntimeRisk(s.Risk),
		AutoCallable:     s.AutoCallable,
		RequiresApproval: s.RequiresApproval,
		DefaultTimeout:   s.DefaultTimeout,
		Idempotent:       s.Idempotent,
		Tags:             append([]string{string(s.Domain), string(s.Operation)}, s.Tags...),
	}
}

func toRuntimePrerequisites(prerequisites []ToolPrerequisite) []agentruntime.ToolPrerequisite {
	if len(prerequisites) == 0 {
		return nil
	}
	result := make([]agentruntime.ToolPrerequisite, 0, len(prerequisites))
	for _, prerequisite := range prerequisites {
		if strings.TrimSpace(prerequisite.Capability) == "" || strings.TrimSpace(prerequisite.Condition) == "" {
			continue
		}
		result = append(result, agentruntime.ToolPrerequisite{
			Capability: prerequisite.Capability,
			Condition:  prerequisite.Condition,
		})
	}
	return result
}

func modelFacingToolDescription(tool *ToolSpec) string {
	if tool == nil {
		return ""
	}
	contract := BuildToolUseContract(tool)
	parts := make([]string, 0, 8)
	if description := strings.TrimSpace(tool.ModelDescription); description != "" && !containsHan(description) {
		parts = append(parts, description)
	}
	parts = append(parts,
		"Capability: "+contract.Capability+".",
		"Domain: "+contract.Domain+".",
		"Operation: "+string(tool.Operation)+".",
	)
	if len(contract.ObjectTypes) > 0 {
		parts = append(parts, "Objects: "+strings.Join(contract.ObjectTypes, ", ")+".")
	}
	if len(contract.Preconditions) > 0 {
		parts = append(parts, "Preconditions: "+strings.Join(contract.Preconditions, ", ")+".")
	}
	if len(contract.Postconditions) > 0 {
		parts = append(parts, "Postconditions: "+strings.Join(contract.Postconditions, ", ")+".")
	}
	if tool.ExecutionContract.Mode != "" {
		parts = append(parts, "Execution mode: "+tool.ExecutionContract.Mode+".")
	}
	if tool.ExecutionContract.CompletionCapability != "" {
		parts = append(parts, "Completion capability: "+tool.ExecutionContract.CompletionCapability+".")
	}
	return strings.Join(parts, " ")
}

// normalizeRuntimeArgsSchema 深拷贝参数 schema，并将所有 "integer" 类型放宽为
// "number"。LLM 返回的 JSON 数字经 encoding/json 解析后一律是 float64，而
// agent-runtime 的参数校验器对 "integer" 类型拒绝 float64，会报出诸如
// `$.page has type number, want integer` 的错误，导致模型给出的任何整数参数都无法通过。
// 只放宽提交给 runtime 的 schema 副本，源工具定义与提示词仍保留 integer 语义；
// 下游工具处理器自身会把浮点数转换为 int。
func normalizeRuntimeArgsSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema)+1)
	for k, v := range schema {
		if k == "description" {
			continue
		}
		out[k] = normalizeRuntimeSchemaValue(k, v)
	}
	// A schema with declared properties is a closed model-facing contract.
	// Otherwise the model may emit plausible aliases that pass validation but
	// are silently ignored by the handler, causing successful calls with wrong
	// business defaults.
	if out["type"] == "object" && out["properties"] != nil {
		if _, declared := out["additionalProperties"]; !declared {
			out["additionalProperties"] = false
		}
	}
	return out
}

func normalizeRuntimeSchemaValue(key string, v any) any {
	switch val := v.(type) {
	case map[string]any:
		return normalizeRuntimeArgsSchema(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeRuntimeSchemaValue(key, item)
		}
		return out
	case string:
		if key == "type" && val == "integer" {
			return "number"
		}
		return val
	default:
		return v
	}
}

// defaultTimeout 默认工具超时
func defaultTimeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 60 * time.Second
}

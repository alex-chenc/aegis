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
		Description:      s.Description,
		ArgsSchema:       s.ArgsSchema,
		ResultSchema:     s.ResultSchema,
		RiskLevel:        toRuntimeRisk(s.Risk),
		AutoCallable:     s.AutoCallable,
		RequiresApproval: s.RequiresApproval,
		DefaultTimeout:   s.DefaultTimeout,
		Idempotent:       s.Idempotent,
		Tags:             append([]string{string(s.Domain), string(s.Operation)}, s.Tags...),
	}
}

// defaultTimeout 默认工具超时
func defaultTimeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 60 * time.Second
}

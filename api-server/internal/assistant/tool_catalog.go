package assistant

import (
	"sort"
	"strings"
)

// ToolCatalog 工具目录
type ToolCatalog struct {
	registry *ToolRegistry
}

// NewToolCatalog 创建工具目录
func NewToolCatalog(registry *ToolRegistry) *ToolCatalog {
	return &ToolCatalog{registry: registry}
}

// Search 搜索工具
func (c *ToolCatalog) Search(query string, opts SearchOptions) []*ToolSpec {
	tools := c.registry.List()
	var results []*ToolSpec

	query = strings.ToLower(strings.TrimSpace(query))

	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		if opts.Domain != "" && tool.Domain != opts.Domain {
			continue
		}
		if opts.RiskLevel != "" && tool.RiskLevel != opts.RiskLevel {
			continue
		}

		// Score based on keyword match
		score := 0
		if query != "" {
			name := strings.ToLower(tool.Name)
			desc := strings.ToLower(tool.Description)
			domain := strings.ToLower(tool.Domain)
			operation := strings.ToLower(tool.Operation)

			if strings.Contains(name, query) {
				score += 10
			}
			if strings.Contains(domain, query) {
				score += 8
			}
			if strings.Contains(operation, query) {
				score += 6
			}
			if strings.Contains(desc, query) {
				score += 4
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

		results = append(results, tool)
	}

	// Sort by relevance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	if opts.MaxResults > 0 && len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results
}

// BuildDescriptors 构建工具描述符列表（用于 agent-runtime）
func (c *ToolCatalog) BuildDescriptors(toolNames []string) []map[string]interface{} {
	var descriptors []map[string]interface{}
	for _, name := range toolNames {
		tool, ok := c.registry.Get(name)
		if !ok || !tool.Enabled {
			continue
		}
		descriptors = append(descriptors, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.ArgsSchema,
		})
	}
	return descriptors
}

// GetDomains 获取所有域
func (c *ToolCatalog) GetDomains() []string {
	tools := c.registry.List()
	domainSet := make(map[string]bool)
	for _, t := range tools {
		domainSet[t.Domain] = true
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

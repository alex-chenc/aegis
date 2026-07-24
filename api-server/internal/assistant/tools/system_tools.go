package tools

import (
	"context"
	"fmt"

	"api-server/internal/assistant"
)

// RegisterSystemTools 注册系统域工具（Tool.Search, Context.Get, Session.Summarize）
func RegisterSystemTools(registry *assistant.ToolRegistry, catalog *assistant.ToolCatalog, sessionRepo interface{}, contextLoader interface{}) error {
	// Tool.Search 元工具 —— 常驻工具，用于模型发现未注入的工具
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Tool.Search",
		Domain:             assistant.DomainSystem,
		Operation:          assistant.OpSearch,
		Capability:         "search_available_tools",
		Description:        "Search discoverable primary and contextual tools when the current tool set is insufficient; results are not injected into the current run automatically.",
		Aliases:            []string{"搜索工具", "查找工具", "search tools", "find tools"},
		Tags:               []string{"meta", "system", "tool-search"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       false,
		RequiresApproval:   false,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Tool name, domain, operation, capability, alias, tag, or functional keyword.",
				},
				"domains": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional domain filters such as host, detection, or package.",
				},
				"operations": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional operation filters such as list, get, create, or update.",
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum result count; defaults to 10.",
				},
			},
			"required": []string{"query"},
		},
		Handler: makeToolSearchHandler(catalog),
	}); err != nil {
		return err
	}

	// Context.Get 工具
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Context.Get",
		Domain:             assistant.DomainSystem,
		Operation:          assistant.OpGet,
		Capability:         "get_session_context",
		Description:        "Get current session context, including referenced hosts, alerts, tasks, and other objects.",
		Aliases:            []string{"获取上下文", "上下文信息"},
		Tags:               []string{"meta", "system", "context"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: makeContextGetHandler(contextLoader),
	}); err != nil {
		return err
	}

	// Session.Summarize 工具
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Session.Summarize",
		Domain:             assistant.DomainSystem,
		Operation:          assistant.OpGet,
		Capability:         "summarize_session",
		Description:        "Summarize the current conversation history while preserving object and operation references.",
		Aliases:            []string{"会话总结", "对话摘要"},
		Tags:               []string{"meta", "system", "session"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: makeSessionSummarizeHandler(),
	}); err != nil {
		return err
	}

	return nil
}

// makeToolSearchHandler 创建 Tool.Search 工具的 handler
func makeToolSearchHandler(catalog *assistant.ToolCatalog) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		query, _ := args["query"].(string)
		if query == "" {
			return map[string]interface{}{
				"matches": []interface{}{},
				"message": "Provide a search keyword.",
			}, nil
		}

		// 解析可选参数
		maxResults := 10
		if mr, ok := args["max_results"].(float64); ok && mr > 0 {
			maxResults = int(mr)
		}

		// 构建搜索选项
		opts := assistant.SearchOptions{
			MaxResults: maxResults,
		}

		// 按域过滤
		if domains, ok := args["domains"].([]interface{}); ok && len(domains) > 0 {
			if len(domains) == 1 {
				if d, ok := domains[0].(string); ok {
					opts.Domain = d
				}
			}
		}

		// 执行搜索
		results := catalog.Search(query, opts)

		// 构建返回结果
		var matches []assistant.ToolSearchItem
		for _, tool := range results {
			matches = append(matches, assistant.ToolSearchItem{
				Name:        tool.Name,
				Domain:      string(tool.Domain),
				Operation:   string(tool.Operation),
				Risk:        string(tool.Risk),
				Description: tool.Description,
				ArgsSummary: summarizeArgsSchema(tool.ArgsSchema),
				Tags:        tool.Tags,
			})
		}

		return assistant.ToolSearchResult{
			Matches: matches,
		}, nil
	}
}

// summarizeArgsSchema 生成参数摘要
func summarizeArgsSchema(schema map[string]interface{}) string {
	if schema == nil {
		return ""
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return ""
	}
	required, _ := schema["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		if s, ok := r.(string); ok {
			requiredSet[s] = true
		}
	}

	var parts []string
	for name := range props {
		if requiredSet[name] {
			parts = append(parts, name+" (required)")
		} else {
			parts = append(parts, name+" (optional)")
		}
	}
	return fmt.Sprintf("%v", parts)
}

// makeContextGetHandler 创建 Context.Get 工具的 handler
func makeContextGetHandler(contextLoader interface{}) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		// Context.Get 返回当前上下文信息
		// 实际实现需要从 session 中获取 context refs
		return map[string]interface{}{
			"message": "Session context is already injected through the system prompt.",
		}, nil
	}
}

// makeSessionSummarizeHandler 创建 Session.Summarize 工具的 handler
func makeSessionSummarizeHandler() assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"message": "Conversation summarization is handled by the agent runtime.",
		}, nil
	}
}

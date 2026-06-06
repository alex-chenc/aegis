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
		Name:        "Tool.Search",
		Domain:      assistant.DomainSystem,
		Operation:   assistant.OpSearch,
		Capability:  "search_available_tools",
		Description: "搜索可用工具。当当前工具不足以完成任务时，使用此工具发现其他可用工具。返回匹配的工具列表，但不直接注入到当前运行中。",
		Aliases:     []string{"搜索工具", "查找工具", "search tools", "find tools"},
		Tags:        []string{"meta", "system", "tool-search"},
		Risk:        assistant.ToolRiskReadonly,
		AutoCallable: false,
		RequiresApproval: false,
		Idempotent:   true,
		DefaultWhitelisted: true,
		Enabled:      true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "搜索关键词，可以是工具名、域名、操作类型或功能描述",
				},
				"domains": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "按域过滤，如 host, detection, package 等",
				},
				"operations": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "按操作类型过滤，如 list, get, create, update 等",
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "最大返回结果数，默认 10",
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
		Name:        "Context.Get",
		Domain:      assistant.DomainSystem,
		Operation:   assistant.OpGet,
		Capability:  "get_session_context",
		Description: "获取当前会话的上下文信息，包括关联的主机、告警、任务等对象",
		Aliases:     []string{"获取上下文", "上下文信息"},
		Tags:        []string{"meta", "system", "context"},
		Risk:        assistant.ToolRiskReadonly,
		AutoCallable: true,
		RequiresApproval: false,
		Idempotent:   true,
		DefaultWhitelisted: true,
		Enabled:      true,
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
		Name:        "Session.Summarize",
		Domain:      assistant.DomainSystem,
		Operation:   assistant.OpGet,
		Capability:  "summarize_session",
		Description: "总结当前会话的对话历史，生成摘要",
		Aliases:     []string{"会话总结", "对话摘要"},
		Tags:        []string{"meta", "system", "session"},
		Risk:        assistant.ToolRiskReadonly,
		AutoCallable: true,
		RequiresApproval: false,
		Idempotent:   true,
		DefaultWhitelisted: true,
		Enabled:      true,
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
				"message": "请提供搜索关键词",
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
			parts = append(parts, name+"(必填)")
		} else {
			parts = append(parts, name+"(可选)")
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
			"message": "上下文信息已通过系统提示注入",
		}, nil
	}
}

// makeSessionSummarizeHandler 创建 Session.Summarize 工具的 handler
func makeSessionSummarizeHandler() assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"message": "会话总结功能由 agent-runtime 内置处理",
		}, nil
	}
}

package tools

import (
	"context"
	"fmt"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/repository"
	"go.uber.org/zap"
)

// ExternalMCPToolDeps MCP 工具依赖
type ExternalMCPToolDeps struct {
	SourceService  *assistant.ExternalMCPSourceService
	QueryPlanner   *assistant.ExternalMCPQueryPlanner
	Normalizer     *assistant.ExternalMCPNormalizer
	Redactor       *assistant.ExternalMCPRedactor
	PromptProvider *assistant.ExternalMCPPromptProvider
	Logger         *zap.Logger
}

// RegisterExternalMCPTools 注册外部 MCP 域工具
func RegisterExternalMCPTools(registry *assistant.ToolRegistry, deps ExternalMCPToolDeps) error {
	// ExternalMCP.Source.List — 查询已配置的数据源
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "ExternalMCP.Source.List",
		Domain:             assistant.DomainExternalMCP,
		Operation:          assistant.OpList,
		Capability:         "list_mcp_sources",
		Description:        "List external MCP data sources available to the current user.",
		Aliases:            []string{"外部数据源", "MCP数据源", "list mcp sources"},
		Tags:               []string{"v6.0", "external_mcp", "source"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"source_type": map[string]interface{}{
					"type":        "string",
					"description": "Optional data-source type filter.",
					"enum":        []string{"siem", "cmdb", "edr", "ticket", "threat_intel", "log_warehouse", "custom"},
				},
			},
		},
		Handler: makeExternalMCPSourceListHandler(deps.SourceService),
		ServiceBinding: assistant.ServiceBinding{
			Component: "api-server",
			File:      "api-server/internal/assistant/external_mcp_service.go",
			Function:  "ExternalMCPSourceService.ListSources",
		},
	}); err != nil {
		return err
	}

	// ExternalMCP.Source.GetSchema — 获取数据源 schema
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "ExternalMCP.Source.GetSchema",
		Domain:             assistant.DomainExternalMCP,
		Operation:          assistant.OpGet,
		Capability:         "get_mcp_source_schema",
		Description:        "Get the schema and available-tool summary for one external MCP data source.",
		Aliases:            []string{"MCP schema", "数据源详情"},
		Tags:               []string{"v6.0", "external_mcp", "schema"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"source_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact external data-source UUID.",
				},
			},
			"required": []string{"source_id"},
		},
		Handler: makeExternalMCPSourceGetSchemaHandler(deps.SourceService),
	}); err != nil {
		return err
	}

	// ExternalMCP.Source.TestConnection — 测试连接
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "ExternalMCP.Source.TestConnection",
		Domain:             assistant.DomainExternalMCP,
		Operation:          assistant.OpExecute,
		Capability:         "test_mcp_connection",
		Description:        "Test connectivity to one external MCP data source.",
		Aliases:            []string{"测试连接", "test connection"},
		Tags:               []string{"v6.0", "external_mcp", "test"},
		Risk:               assistant.ToolRiskLow,
		AutoCallable:       false,
		Idempotent:         true,
		DefaultWhitelisted: false,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"source_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact external data-source UUID.",
				},
			},
			"required": []string{"source_id"},
		},
		Handler: makeExternalMCPTestConnectionHandler(deps.SourceService),
	}); err != nil {
		return err
	}

	// ExternalMCP.Query — 查询外部数据源
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "ExternalMCP.Query",
		Domain:             assistant.DomainExternalMCP,
		Operation:          assistant.OpSearch,
		Capability:         "query_mcp_source",
		Description:        "Query one external MCP data source and return normalized, redacted results.",
		Aliases:            []string{"查询外部数据", "MCP查询"},
		Tags:               []string{"v6.0", "external_mcp", "query"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: false,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"source_id": map[string]interface{}{
					"type":        "string",
					"description": "Exact external data-source UUID.",
				},
				"query_goal": map[string]interface{}{
					"type":        "string",
					"description": "Natural-language description of the query objective.",
				},
				"time_range": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"from": map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive RFC3339 start time."},
						"to":   map[string]interface{}{"type": "string", "format": "date-time", "description": "Inclusive RFC3339 end time."},
					},
				},
				"filters": map[string]interface{}{
					"type":        "object",
					"description": "Source-specific query filters allowed by the source schema.",
				},
				"max_rows": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum rows to return; defaults to 50.",
				},
			},
			"required": []string{"source_id", "query_goal"},
		},
		Handler: makeExternalMCPQueryHandler(deps.SourceService, deps.Normalizer, deps.Redactor, deps.Logger),
	}); err != nil {
		return err
	}

	// ExternalMCP.MultiQuery — 多数据源并发查询
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "ExternalMCP.MultiQuery",
		Domain:             assistant.DomainExternalMCP,
		Operation:          assistant.OpSearch,
		Capability:         "multi_query_mcp_sources",
		Description:        "Query multiple external MCP data sources concurrently and return normalized, redacted results.",
		Aliases:            []string{"多源查询", "关联查询"},
		Tags:               []string{"v6.0", "external_mcp", "multi_query"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: false,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"queries": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"source_id":  map[string]interface{}{"type": "string"},
							"query_goal": map[string]interface{}{"type": "string"},
							"filters":    map[string]interface{}{"type": "object"},
						},
						"required": []string{"source_id", "query_goal"},
					},
					"description": "Per-source query requests.",
				},
				"max_rows_per_source": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum rows per data source; defaults to 50.",
				},
			},
			"required": []string{"queries"},
		},
		Handler: makeExternalMCPMultiQueryHandler(deps.SourceService, deps.Normalizer, deps.Redactor, deps.Logger),
	}); err != nil {
		return err
	}

	// ExternalMCP.Analyze — 证据融合分析
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "ExternalMCP.Analyze",
		Domain:             assistant.DomainExternalMCP,
		Operation:          assistant.OpGenerate,
		Capability:         "analyze_mcp_evidence",
		Description:        "Analyze and correlate previously queried external MCP results without accessing external sources again.",
		Aliases:            []string{"证据融合", "MCP分析"},
		Tags:               []string{"v6.0", "external_mcp", "analyze"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Query IDs whose persisted results should be analyzed.",
				},
				"analysis_goal": map[string]interface{}{
					"type":        "string",
					"description": "Analysis objective.",
				},
			},
			"required": []string{"query_ids"},
		},
		Handler: makeExternalMCPAnalyzeHandler(deps.SourceService, deps.PromptProvider),
	}); err != nil {
		return err
	}

	return nil
}

// makeExternalMCPSourceListHandler 创建列出数据源处理器
func makeExternalMCPSourceListHandler(svc *assistant.ExternalMCPSourceService) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		sourceType, _ := args["source_type"].(string)

		query := repository.MCPSourceQuery{
			SourceType: sourceType,
			Enabled:    boolPtr(true),
			Page:       1,
			PageSize:   100,
		}

		sources, total, err := svc.ListSources(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to list MCP sources: %w", err)
		}

		// 转换为安全的视图
		views := make([]assistant.MCPSourceView, len(sources))
		for i, s := range sources {
			views[i] = assistant.MCPSourceView{
				SourceID:   s.SourceID,
				Name:       s.Name,
				SourceType: s.SourceType,
				Transport:  s.Transport,
				Enabled:    s.Enabled,
			}
		}

		return map[string]interface{}{
			"sources": views,
			"total":   total,
		}, nil
	}
}

// makeExternalMCPSourceGetSchemaHandler 创建获取 schema 处理器
func makeExternalMCPSourceGetSchemaHandler(svc *assistant.ExternalMCPSourceService) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		sourceID, _ := args["source_id"].(string)
		if sourceID == "" {
			return nil, fmt.Errorf("source_id is required")
		}

		source, err := svc.GetSource(ctx, sourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get MCP source: %w", err)
		}

		return map[string]interface{}{
			"source_id":   source.SourceID,
			"name":        source.Name,
			"source_type": source.SourceType,
			"transport":   source.Transport,
			"enabled":     source.Enabled,
			"schema":      string(source.SchemaCache),
		}, nil
	}
}

// makeExternalMCPTestConnectionHandler 创建测试连接处理器
func makeExternalMCPTestConnectionHandler(svc *assistant.ExternalMCPSourceService) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		sourceID, _ := args["source_id"].(string)
		if sourceID == "" {
			return nil, fmt.Errorf("source_id is required")
		}

		result, err := svc.TestConnection(ctx, sourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to test connection: %w", err)
		}

		return result, nil
	}
}

// makeExternalMCPQueryHandler 创建查询处理器
func makeExternalMCPQueryHandler(
	svc *assistant.ExternalMCPSourceService,
	normalizer *assistant.ExternalMCPNormalizer,
	redactor *assistant.ExternalMCPRedactor,
	logger *zap.Logger,
) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		start := time.Now()

		sourceID, _ := args["source_id"].(string)
		if sourceID == "" {
			return nil, fmt.Errorf("source_id is required")
		}

		queryGoal, _ := args["query_goal"].(string)
		if queryGoal == "" {
			return nil, fmt.Errorf("query_goal is required")
		}

		logger.Info("ExternalMCP.Query started",
			zap.String("source_id", sourceID),
			zap.String("query_goal", queryGoal),
		)

		// 获取数据源
		source, err := svc.GetSource(ctx, sourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get MCP source: %w", err)
		}

		// 查询外部 MCP
		queryResult, err := svc.Query(ctx, assistant.ExternalMCPQueryRequest{
			SourceID:  sourceID,
			QueryGoal: queryGoal,
			Filters:   extractMap(args, "filters"),
			MaxRows:   extractInt(args, "max_rows", 50),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query MCP source: %w", err)
		}

		// 归一化结果
		normalizedResult, err := normalizer.NormalizeResponse(ctx, &assistant.MCPSourceView{
			SourceID:   source.SourceID,
			Name:       source.Name,
			SourceType: source.SourceType,
		}, &assistant.MCPClientQueryResponse{
			Rows: queryResult.Rows,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to normalize result: %w", err)
		}

		// 脱敏结果
		redactedResult, err := redactor.RedactResult(ctx, normalizedResult)
		if err != nil {
			logger.Error("ExternalMCP.Query failed during redaction",
				zap.String("source_id", sourceID),
				zap.Error(err),
				zap.Duration("duration", time.Since(start)),
			)
			return nil, fmt.Errorf("failed to redact result: %w", err)
		}

		logger.Info("ExternalMCP.Query completed",
			zap.String("source_id", sourceID),
			zap.String("query_goal", queryGoal),
			zap.Int("result_count", redactedResult.ResultCount),
			zap.Duration("duration", time.Since(start)),
		)

		return map[string]interface{}{
			"success": true,
			"summary": redactedResult.Summary,
			"data":    redactedResult,
		}, nil
	}
}

// makeExternalMCPMultiQueryHandler 创建多源查询处理器
func makeExternalMCPMultiQueryHandler(
	svc *assistant.ExternalMCPSourceService,
	normalizer *assistant.ExternalMCPNormalizer,
	redactor *assistant.ExternalMCPRedactor,
	logger *zap.Logger,
) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		start := time.Now()

		queries, ok := args["queries"].([]interface{})
		if !ok || len(queries) == 0 {
			return nil, fmt.Errorf("queries is required and must be non-empty")
		}

		maxRowsPerSource := extractInt(args, "max_rows_per_source", 50)

		logger.Info("ExternalMCP.MultiQuery started",
			zap.Int("query_count", len(queries)),
			zap.Int("max_rows_per_source", maxRowsPerSource),
		)

		var results []interface{}
		for _, q := range queries {
			queryMap, ok := q.(map[string]interface{})
			if !ok {
				continue
			}

			sourceID, _ := queryMap["source_id"].(string)
			queryGoal, _ := queryMap["query_goal"].(string)

			if sourceID == "" || queryGoal == "" {
				continue
			}

			// 获取数据源
			source, err := svc.GetSource(ctx, sourceID)
			if err != nil {
				results = append(results, map[string]interface{}{
					"source_id": sourceID,
					"success":   false,
					"error":     err.Error(),
				})
				continue
			}

			// 查询
			queryResult, err := svc.Query(ctx, assistant.ExternalMCPQueryRequest{
				SourceID:  sourceID,
				QueryGoal: queryGoal,
				Filters:   extractMap(queryMap, "filters"),
				MaxRows:   maxRowsPerSource,
			})
			if err != nil {
				results = append(results, map[string]interface{}{
					"source_id": sourceID,
					"success":   false,
					"error":     err.Error(),
				})
				continue
			}

			// 归一化
			normalizedResult, err := normalizer.NormalizeResponse(ctx, &assistant.MCPSourceView{
				SourceID:   source.SourceID,
				Name:       source.Name,
				SourceType: source.SourceType,
			}, &assistant.MCPClientQueryResponse{
				Rows: queryResult.Rows,
			})
			if err != nil {
				results = append(results, map[string]interface{}{
					"source_id": sourceID,
					"success":   false,
					"error":     err.Error(),
				})
				continue
			}

			// 脱敏
			redactedResult, err := redactor.RedactResult(ctx, normalizedResult)
			if err != nil {
				results = append(results, map[string]interface{}{
					"source_id": sourceID,
					"success":   false,
					"error":     err.Error(),
				})
				continue
			}

			results = append(results, map[string]interface{}{
				"source_id": sourceID,
				"success":   true,
				"data":      redactedResult,
			})
		}

		logger.Info("ExternalMCP.MultiQuery completed",
			zap.Int("query_count", len(queries)),
			zap.Int("result_count", len(results)),
			zap.Duration("duration", time.Since(start)),
		)

		return map[string]interface{}{
			"results": results,
			"total":   len(results),
		}, nil
	}
}

// makeExternalMCPAnalyzeHandler 创建分析处理器
func makeExternalMCPAnalyzeHandler(
	svc *assistant.ExternalMCPSourceService,
	promptProvider *assistant.ExternalMCPPromptProvider,
) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		queryIDs, ok := args["query_ids"].([]interface{})
		if !ok || len(queryIDs) == 0 {
			return nil, fmt.Errorf("query_ids is required")
		}

		analysisGoal, _ := args["analysis_goal"].(string)
		if analysisGoal == "" {
			analysisGoal = "Correlate and analyze external evidence."
		}

		// 获取查询日志
		var queryLogs []interface{}
		for _, qid := range queryIDs {
			queryID, ok := qid.(string)
			if !ok {
				continue
			}

			// 这里简化实现，实际应该从 queryLogRepo 查询
			queryLogs = append(queryLogs, map[string]interface{}{
				"query_id": queryID,
			})
		}

		// 构建分析提示词
		analysisPrompt := promptProvider.BuildMCPResultAnalysisPrompt(assistant.MCPResultAnalysisInput{
			UserMessage:             analysisGoal,
			AegisEvidenceJSON:       "[]",
			ExternalMCPEvidenceJSON: fmt.Sprintf("%v", queryLogs),
			QueryLimitationsJSON:    "[]",
		})

		return map[string]interface{}{
			"success":   true,
			"prompt":    analysisPrompt,
			"query_ids": queryIDs,
			"message":   "The analysis prompt is ready for LLM evidence synthesis.",
		}, nil
	}
}

// extractMap 从参数中提取 map
func extractMap(args map[string]interface{}, key string) map[string]string {
	result := make(map[string]string)
	if m, ok := args[key].(map[string]interface{}); ok {
		for k, v := range m {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// extractInt 从参数中提取 int
func extractInt(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	if v, ok := args[key].(int); ok {
		return v
	}
	return defaultVal
}

// boolPtr 返回布尔指针
func boolPtr(b bool) *bool {
	return &b
}

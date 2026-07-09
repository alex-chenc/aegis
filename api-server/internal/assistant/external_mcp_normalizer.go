package assistant

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ExternalMCPNormalizer 外部 MCP 数据归一化器
// 负责将外部 MCP 返回的数据转换为统一格式
type ExternalMCPNormalizer struct {
	logger *zap.Logger
}

// NewExternalMCPNormalizer 创建归一化器
func NewExternalMCPNormalizer(logger *zap.Logger) *ExternalMCPNormalizer {
	return &ExternalMCPNormalizer{
		logger: logger,
	}
}

// NormalizeResponse 归一化外部 MCP 响应
func (n *ExternalMCPNormalizer) NormalizeResponse(ctx context.Context, source *MCPSourceView, rawResp *MCPClientQueryResponse) (*ExternalMCPQueryResult, error) {
	if rawResp == nil {
		return nil, fmt.Errorf("empty response from MCP source")
	}

	queryID := "mcpq_" + uuid.New().String()[:8]

	result := &ExternalMCPQueryResult{
		QueryID:    queryID,
		SourceID:   source.SourceID,
		SourceName: source.Name,
		SourceType: source.SourceType,
		Status:     "success",
		Metadata:   make(map[string]interface{}),
	}

	// 归一化字段
	if rawResp.Fields != nil {
		result.Fields = make([]ExternalMCPField, len(rawResp.Fields))
		for i, f := range rawResp.Fields {
			result.Fields[i] = ExternalMCPField{
				Name:        f.Name,
				Type:        f.Type,
				Description: f.Description,
			}
		}
	}

	// 归一化行数据
	if rawResp.Rows != nil {
		result.Rows = rawResp.Rows
		result.ResultCount = len(rawResp.Rows)

		// 检查是否超过限制
		if source.MaxRows > 0 && result.ResultCount > source.MaxRows {
			result.Rows = result.Rows[:source.MaxRows]
			result.ResultCount = source.MaxRows
			result.Truncated = true
		}
	}

	// 生成摘要
	result.Summary = n.generateSummary(source, result)

	// 提取证据
	result.Evidence = n.extractEvidence(source, result)

	// 保存原始元数据
	if rawResp.Metadata != nil {
		for k, v := range rawResp.Metadata {
			result.Metadata[k] = v
		}
	}
	result.Metadata["normalized_at"] = time.Now().UTC()
	result.Metadata["source_version"] = rawResp.Version

	n.logger.Debug("external MCP response normalized",
		zap.String("query_id", queryID),
		zap.String("source_id", source.SourceID),
		zap.Int("result_count", result.ResultCount),
		zap.Bool("truncated", result.Truncated),
	)

	return result, nil
}

// generateSummary 生成查询摘要
func (n *ExternalMCPNormalizer) generateSummary(source *MCPSourceView, result *ExternalMCPQueryResult) string {
	if result.ResultCount == 0 {
		return fmt.Sprintf("从 %s 查询无结果", source.Name)
	}

	truncatedMsg := ""
	if result.Truncated {
		truncatedMsg = "（结果已截断）"
	}

	return fmt.Sprintf("从 %s 查询到 %d 条记录%s", source.Name, result.ResultCount, truncatedMsg)
}

// extractEvidence 从结果中提取证据
func (n *ExternalMCPNormalizer) extractEvidence(source *MCPSourceView, result *ExternalMCPQueryResult) []ExternalMCPEvidence {
	var evidence []ExternalMCPEvidence

	// 最多提取 10 条证据
	maxEvidence := 10
	if len(result.Rows) < maxEvidence {
		maxEvidence = len(result.Rows)
	}

	for i := 0; i < maxEvidence; i++ {
		row := result.Rows[i]
		ev := ExternalMCPEvidence{
			EvidenceID: "ev_" + uuid.New().String()[:8],
			SourceName: source.Name,
			ObjectType: source.SourceType,
			Fields:     make(map[string]string),
		}

		// 尝试提取常见字段
		if id, ok := row["id"]; ok {
			ev.ObjectID = fmt.Sprintf("%v", id)
		}
		if ts, ok := row["timestamp"]; ok {
			if t, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", ts)); err == nil {
				ev.Time = &t
			}
		}
		if title, ok := row["title"]; ok {
			ev.Title = fmt.Sprintf("%v", title)
		}
		if summary, ok := row["summary"]; ok {
			ev.Summary = fmt.Sprintf("%v", summary)
		}

		// 保存所有字段
		for k, v := range row {
			ev.Fields[k] = fmt.Sprintf("%v", v)
		}

		evidence = append(evidence, ev)
	}

	return evidence
}

// MCPSourceView MCP 数据源视图
type MCPSourceView struct {
	SourceID   string `json:"source_id"`
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
	Transport  string `json:"transport"`
	Enabled    bool   `json:"enabled"`
	MaxRows    int    `json:"max_rows"`
	Timeout    int    `json:"timeout"`
	Schema     string `json:"schema_summary,omitempty"`
}

// ExternalMCPField 字段描述
type ExternalMCPField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// ExternalMCPQueryResult 查询结果
type ExternalMCPQueryResult struct {
	QueryID     string                 `json:"query_id"`
	SourceID    string                 `json:"source_id"`
	SourceName  string                 `json:"source_name"`
	SourceType  string                 `json:"source_type"`
	Status      string                 `json:"status"`
	ResultCount int                    `json:"result_count"`
	Fields      []ExternalMCPField     `json:"fields"`
	Rows        []map[string]any       `json:"rows"`
	Summary     string                 `json:"summary"`
	Evidence    []ExternalMCPEvidence  `json:"evidence"`
	Truncated   bool                   `json:"truncated"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ExternalMCPEvidence 证据项
type ExternalMCPEvidence struct {
	EvidenceID string            `json:"evidence_id"`
	SourceName string            `json:"source_name"`
	ObjectType string            `json:"object_type"`
	ObjectID   string            `json:"object_id"`
	Time       *time.Time        `json:"time,omitempty"`
	Title      string            `json:"title"`
	Summary    string            `json:"summary"`
	Fields     map[string]string `json:"fields"`
}

// MCPClientQueryResponse MCP 客户端查询响应
type MCPClientQueryResponse struct {
	Fields   []ExternalMCPField     `json:"fields,omitempty"`
	Rows     []map[string]any       `json:"rows"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Version  string                 `json:"version,omitempty"`
}

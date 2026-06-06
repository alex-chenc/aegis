package assistant

import (
	"context"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// ExternalMCPRedactor 外部 MCP 数据脱敏器
// 负责移除或掩码处理敏感数据，防止泄露给 LLM
type ExternalMCPRedactor struct {
	logger *zap.Logger

	// 敏感数据正则表达式
	tokenPatterns      []*regexp.Regexp
	passwordPatterns   []*regexp.Regexp
	emailPattern       *regexp.Regexp
	phonePattern       *regexp.Regexp
	idCardPattern      *regexp.Regexp
	accessKeyPattern   *regexp.Regexp
	privateKeyPattern  *regexp.Regexp
}

// NewExternalMCPRedactor 创建脱敏器
func NewExternalMCPRedactor(logger *zap.Logger) *ExternalMCPRedactor {
	return &ExternalMCPRedactor{
		logger: logger,
		tokenPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(token|api[_-]?key|apikey|access[_-]?token|auth[_-]?token)\s*[:=]\s*["']?([a-zA-Z0-9_\-\.]{20,})["']?`),
			regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_\-\.]{20,}`),
			regexp.MustCompile(`(?i)sk-[a-zA-Z0-9]{20,}`),
		},
		passwordPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(password|passwd|pwd|secret)\s*[:=]\s*["']?([^\s"']{8,})["']?`),
			regexp.MustCompile(`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
		},
		emailPattern:      regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
		phonePattern:      regexp.MustCompile(`\b1[3-9]\d{9}\b`),
		idCardPattern:     regexp.MustCompile(`\b\d{17}[\dXx]\b`),
		accessKeyPattern:  regexp.MustCompile(`(?i)(access[_-]?key|ak|sk)\s*[:=]\s*["']?([a-zA-Z0-9]{16,})["']?`),
		privateKeyPattern: regexp.MustCompile(`-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----[\s\S]*?-----END\s+(RSA\s+)?PRIVATE\s+KEY-----`),
	}
}

// RedactResult 对查询结果进行脱敏
func (r *ExternalMCPRedactor) RedactResult(ctx context.Context, result *ExternalMCPQueryResult) (*ExternalMCPQueryResult, error) {
	if result == nil {
		return nil, nil
	}

	// 脱敏行数据
	redactedRows := make([]map[string]any, len(result.Rows))
	for i, row := range result.Rows {
		redactedRow := make(map[string]any)
		for k, v := range row {
			if str, ok := v.(string); ok {
				redactedRow[k] = r.redactString(str)
			} else {
				redactedRow[k] = v
			}
		}
		redactedRows[i] = redactedRow
	}
	result.Rows = redactedRows

	// 脱敏摘要
	if result.Summary != "" {
		result.Summary = r.redactString(result.Summary)
	}

	// 脱敏证据
	for i := range result.Evidence {
		result.Evidence[i].Summary = r.redactString(result.Evidence[i].Summary)
		result.Evidence[i].Title = r.redactString(result.Evidence[i].Title)
		for k, v := range result.Evidence[i].Fields {
			result.Evidence[i].Fields[k] = r.redactString(v)
		}
	}

	r.logger.Debug("external MCP result redacted",
		zap.String("query_id", result.QueryID),
		zap.Int("row_count", len(result.Rows)),
	)

	return result, nil
}

// RedactPromptContext 对 Prompt 上下文进行脱敏
func (r *ExternalMCPRedactor) RedactPromptContext(ctx context.Context, promptCtx ExternalMCPPromptContext) (ExternalMCPPromptContext, error) {
	// 脱敏查询结果
	for i := range promptCtx.QueryResults {
		for j := range promptCtx.QueryResults[i].Evidence {
			promptCtx.QueryResults[i].Evidence[j].Summary = r.redactString(promptCtx.QueryResults[i].Evidence[j].Summary)
		}
	}

	return promptCtx, nil
}

// redactString 对字符串进行脱敏
func (r *ExternalMCPRedactor) redactString(s string) string {
	if s == "" {
		return s
	}

	result := s

	// 移除 token/api key
	for _, pattern := range r.tokenPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED_TOKEN]")
	}

	// 移除 password/private key
	for _, pattern := range r.passwordPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED_SECRET]")
	}

	// 移除 private key 块
	result = r.privateKeyPattern.ReplaceAllString(result, "[REDACTED_PRIVATE_KEY]")

	// 掩码邮箱
	result = r.emailPattern.ReplaceAllStringFunc(result, func(email string) string {
		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			return "[REDACTED_EMAIL]"
		}
		return parts[0][:1] + "***@" + parts[1]
	})

	// 掩码手机号
	result = r.phonePattern.ReplaceAllStringFunc(result, func(phone string) string {
		if len(phone) >= 7 {
			return phone[:3] + "****" + phone[7:]
		}
		return "[REDACTED_PHONE]"
	})

	// 掩码身份证号
	result = r.idCardPattern.ReplaceAllStringFunc(result, func(id string) string {
		if len(id) >= 10 {
			return id[:6] + "********" + id[10:]
		}
		return "[REDACTED_ID]"
	})

	// 掩码 access key
	result = r.accessKeyPattern.ReplaceAllString(result, "[REDACTED_ACCESS_KEY]")

	return result
}

// ExternalMCPPromptContext Prompt 上下文结构
type ExternalMCPPromptContext struct {
	SourcesUsed  []MCPSourceView         `json:"sources_used"`
	QueryResults []ExternalMCPQueryResult `json:"query_results"`
	Limitations  []string                `json:"limitations"`
}

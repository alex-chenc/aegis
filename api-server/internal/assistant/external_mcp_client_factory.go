package assistant

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ExternalMCPClientFactory MCP 客户端工厂
// 根据 transport 类型创建对应的 MCP 客户端
type ExternalMCPClientFactory struct {
	httpClient *http.Client
	logger     *zap.Logger
}

// NewExternalMCPClientFactory 创建客户端工厂
func NewExternalMCPClientFactory(logger *zap.Logger) *ExternalMCPClientFactory {
	return &ExternalMCPClientFactory{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// NewClient 创建 MCP 客户端
func (f *ExternalMCPClientFactory) NewClient(ctx context.Context, source *MCPSourceConfig) (ExternalMCPClient, error) {
	if source == nil {
		f.logger.Error("failed to create MCP client: source config is nil")
		return nil, fmt.Errorf("source config is nil")
	}

	f.logger.Info("creating MCP client",
		zap.String("source_id", source.SourceID),
		zap.String("transport", source.Transport),
		zap.String("endpoint", source.EndpointURL),
	)

	switch source.Transport {
	case "streamable_http":
		client := NewStreamableHTTPClient(source, f.httpClient, f.logger)
		f.logger.Info("streamable HTTP client created",
			zap.String("source_id", source.SourceID),
		)
		return client, nil
	case "sse":
		client := NewSSEClient(source, f.httpClient, f.logger)
		f.logger.Info("SSE client created",
			zap.String("source_id", source.SourceID),
		)
		return client, nil
	default:
		f.logger.Error("unsupported MCP transport",
			zap.String("transport", source.Transport),
			zap.String("source_id", source.SourceID),
		)
		return nil, fmt.Errorf("unsupported transport: %s", source.Transport)
	}
}

// MCPSourceConfig MCP 数据源配置
type MCPSourceConfig struct {
	SourceID    string            `json:"source_id"`
	Name        string            `json:"name"`
	SourceType  string            `json:"source_type"`
	Transport   string            `json:"transport"`
	EndpointURL string            `json:"endpoint_url"`
	AuthType    string            `json:"auth_type"`
	Credential  string            `json:"credential"`
	Enabled     bool              `json:"enabled"`
	MaxRows     int               `json:"max_rows"`
	Timeout     int               `json:"timeout"`
	ToolNames   []string          `json:"tool_names"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ExternalMCPClient MCP 客户端接口
type ExternalMCPClient interface {
	// Ping 测试连接
	Ping(ctx context.Context) error

	// ListTools 列出可用工具
	ListTools(ctx context.Context) ([]ExternalMCPToolDescriptor, error)

	// Query 执行查询
	Query(ctx context.Context, req MCPClientQueryRequest) (*MCPClientQueryResponse, error)

	// Close 关闭连接
	Close() error
}

// ExternalMCPToolDescriptor MCP 工具描述
type ExternalMCPToolDescriptor struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ArgsSchema  map[string]interface{} `json:"args_schema,omitempty"`
}

// MCPClientQueryRequest MCP 客户端查询请求
type MCPClientQueryRequest struct {
	ToolName string                 `json:"tool_name"`
	Args     map[string]interface{} `json:"args"`
	Timeout  int                    `json:"timeout,omitempty"`
}

// StreamableHTTPClient streamable_http 传输客户端
type StreamableHTTPClient struct {
	source     *MCPSourceConfig
	httpClient *http.Client
	logger     *zap.Logger
}

// NewStreamableHTTPClient 创建 streamable_http 客户端
func NewStreamableHTTPClient(source *MCPSourceConfig, httpClient *http.Client, logger *zap.Logger) *StreamableHTTPClient {
	return &StreamableHTTPClient{
		source:     source,
		httpClient: httpClient,
		logger:     logger,
	}
}

// Ping 测试连接
func (c *StreamableHTTPClient) Ping(ctx context.Context) error {
	c.logger.Info("pinging MCP server",
		zap.String("source_id", c.source.SourceID),
		zap.String("endpoint", c.source.EndpointURL),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", c.source.EndpointURL+"/ping", nil)
	if err != nil {
		c.logger.Error("failed to create ping request",
			zap.String("source_id", c.source.SourceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	c.addAuthHeaders(req)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("ping failed",
			zap.String("source_id", c.source.SourceID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return fmt.Errorf("ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("ping returned non-OK status",
			zap.String("source_id", c.source.SourceID),
			zap.Int("status_code", resp.StatusCode),
			zap.Duration("duration", duration),
		)
		return fmt.Errorf("ping returned status %d", resp.StatusCode)
	}

	c.logger.Info("ping successful",
		zap.String("source_id", c.source.SourceID),
		zap.Duration("duration", duration),
	)
	return nil
}

// ListTools 列出可用工具
func (c *StreamableHTTPClient) ListTools(ctx context.Context) ([]ExternalMCPToolDescriptor, error) {
	// 简化实现：返回空列表，实际应调用 MCP Server 的 tools/list 接口
	return []ExternalMCPToolDescriptor{}, nil
}

// Query 执行查询
func (c *StreamableHTTPClient) Query(ctx context.Context, req MCPClientQueryRequest) (*MCPClientQueryResponse, error) {
	// 简化实现：返回空结果，实际应调用 MCP Server 的 tools/call 接口
	return &MCPClientQueryResponse{
		Rows: []map[string]any{},
	}, nil
}

// Close 关闭连接
func (c *StreamableHTTPClient) Close() error {
	return nil
}

// addAuthHeaders 添加认证头
func (c *StreamableHTTPClient) addAuthHeaders(req *http.Request) {
	switch c.source.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.source.Credential)
	case "api_key":
		req.Header.Set("X-API-Key", c.source.Credential)
	case "basic":
		req.Header.Set("Authorization", "Basic "+c.source.Credential)
	}
}

// SSEClient SSE 传输客户端
type SSEClient struct {
	source     *MCPSourceConfig
	httpClient *http.Client
	logger     *zap.Logger
}

// NewSSEClient 创建 SSE 客户端
func NewSSEClient(source *MCPSourceConfig, httpClient *http.Client, logger *zap.Logger) *SSEClient {
	return &SSEClient{
		source:     source,
		httpClient: httpClient,
		logger:     logger,
	}
}

// Ping 测试连接
func (c *SSEClient) Ping(ctx context.Context) error {
	c.logger.Info("pinging MCP server via SSE",
		zap.String("source_id", c.source.SourceID),
		zap.String("endpoint", c.source.EndpointURL),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", c.source.EndpointURL+"/ping", nil)
	if err != nil {
		c.logger.Error("failed to create ping request",
			zap.String("source_id", c.source.SourceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	c.addAuthHeaders(req)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("ping failed",
			zap.String("source_id", c.source.SourceID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return fmt.Errorf("ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("ping returned non-OK status",
			zap.String("source_id", c.source.SourceID),
			zap.Int("status_code", resp.StatusCode),
			zap.Duration("duration", duration),
		)
		return fmt.Errorf("ping returned status %d", resp.StatusCode)
	}

	c.logger.Info("ping successful",
		zap.String("source_id", c.source.SourceID),
		zap.Duration("duration", duration),
	)
	return nil
}

// ListTools 列出可用工具
func (c *SSEClient) ListTools(ctx context.Context) ([]ExternalMCPToolDescriptor, error) {
	// 简化实现：返回空列表
	return []ExternalMCPToolDescriptor{}, nil
}

// Query 执行查询
func (c *SSEClient) Query(ctx context.Context, req MCPClientQueryRequest) (*MCPClientQueryResponse, error) {
	// 简化实现：返回空结果
	return &MCPClientQueryResponse{
		Rows: []map[string]any{},
	}, nil
}

// Close 关闭连接
func (c *SSEClient) Close() error {
	return nil
}

// addAuthHeaders 添加认证头
func (c *SSEClient) addAuthHeaders(req *http.Request) {
	switch c.source.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.source.Credential)
	case "api_key":
		req.Header.Set("X-API-Key", c.source.Credential)
	case "basic":
		req.Header.Set("Authorization", "Basic "+c.source.Credential)
	}
}

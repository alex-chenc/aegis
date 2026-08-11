package assistant

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MCPGatewayClient is the Assistant-side data-plane client. It uses one
// pre-authorized Aegis Client endpoint and never accepts an upstream endpoint
// or credential from model-supplied arguments.
type MCPGatewayClient struct {
	baseURL    string
	clientKey  string
	token      string
	signSecret string
	httpClient *http.Client
	logger     *zap.Logger
}

type MCPGatewayClientConfig struct {
	BaseURL              string
	ClientKey            string
	Token                string
	ContextSigningSecret string
	Timeout              time.Duration
	Logger               *zap.Logger
}

type MCPGatewayTool struct {
	Name         string
	Title        string
	Description  string
	RiskTier     string
	InputSchema  map[string]interface{}
	OutputSchema map[string]interface{}
}

func NewMCPGatewayClient(cfg MCPGatewayClientConfig) (*MCPGatewayClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid MCP Gateway base URL")
	}
	if strings.TrimSpace(cfg.ClientKey) == "" || strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("MCP Assistant Client authorization is incomplete")
	}
	if strings.TrimSpace(cfg.ContextSigningSecret) == "" {
		return nil, fmt.Errorf("MCP Gateway runtime shared secret is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MCPGatewayClient{
		baseURL:    baseURL,
		clientKey:  strings.TrimSpace(cfg.ClientKey),
		token:      strings.TrimSpace(cfg.Token),
		signSecret: strings.TrimSpace(cfg.ContextSigningSecret),
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger.Named("mcp_gateway_client"),
	}, nil
}

func (c *MCPGatewayClient) ListTools(ctx context.Context) ([]MCPGatewayTool, error) {
	var response struct {
		Tools []struct {
			Name         string          `json:"name"`
			Title        string          `json:"title"`
			Description  string          `json:"description"`
			RiskTier     string          `json:"risk_tier"`
			InputSchema  json.RawMessage `json:"inputSchema"`
			OutputSchema json.RawMessage `json:"outputSchema"`
		} `json:"tools"`
	}
	if err := c.rpc(ctx, "tools/list", map[string]interface{}{}, &response); err != nil {
		return nil, err
	}
	result := make([]MCPGatewayTool, 0, len(response.Tools))
	for _, item := range response.Tools {
		input := map[string]interface{}{}
		output := map[string]interface{}{}
		if len(item.InputSchema) > 0 && json.Unmarshal(item.InputSchema, &input) != nil {
			continue
		}
		if len(item.OutputSchema) > 0 {
			_ = json.Unmarshal(item.OutputSchema, &output)
		}
		result = append(result, MCPGatewayTool{Name: item.Name, Title: item.Title, Description: item.Description, RiskTier: item.RiskTier, InputSchema: input, OutputSchema: output})
	}
	c.logger.Debug("mcp_assistant_catalog_listed", zap.String("client_key_hash", digestMCPClientKey(c.clientKey)), zap.Int("tool_count", len(result)))
	return result, nil
}

func (c *MCPGatewayClient) Call(ctx context.Context, name string, arguments map[string]interface{}) (map[string]interface{}, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("MCP tool name is required")
	}
	var response map[string]interface{}
	if err := c.rpc(ctx, "tools/call", map[string]interface{}{"name": name, "arguments": arguments}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *MCPGatewayClient) rpc(ctx context.Context, method string, params interface{}, result interface{}) error {
	callID := uuid.NewString()
	body, err := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": callID, "method": method, "params": params})
	if err != nil {
		return err
	}
	endpoint := c.baseURL + "/mcp/v1/clients/" + url.PathEscape(c.clientKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.attachSignedContext(req, ctx)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("MCP Gateway request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("MCP Gateway rejected request with status %d", resp.StatusCode)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("invalid MCP Gateway response: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("MCP Gateway tool call rejected (%d): %s", envelope.Error.Code, envelope.Error.Message)
	}
	if len(envelope.Result) == 0 {
		return fmt.Errorf("MCP Gateway response has no result")
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("invalid MCP Gateway result: %w", err)
	}
	return nil
}

func (c *MCPGatewayClient) attachSignedContext(req *http.Request, ctx context.Context) {
	if c.signSecret == "" {
		return
	}
	metadata, ok := ToolInvocationFromContext(ctx)
	if !ok || strings.TrimSpace(metadata.Operator) == "" {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"operator":   metadata.Operator,
		"session_id": metadata.SessionID,
		"message_id": metadata.MessageID,
		"run_id":     metadata.RunID,
		"call_id":    metadata.CallID,
	})
	if err != nil {
		return
	}
	mac := hmac.New(sha256.New, []byte(c.signSecret))
	_, _ = mac.Write(payload)
	req.Header.Set("X-Aegis-MCP-Assistant-Context", string(payload))
	req.Header.Set("X-Aegis-MCP-Assistant-Signature", hex.EncodeToString(mac.Sum(nil)))
}

func digestMCPClientKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

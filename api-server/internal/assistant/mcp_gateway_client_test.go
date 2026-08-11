package assistant

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPGatewayClientUsesAuthorizedEndpointAndSignsAssistantContext(t *testing.T) {
	const secret = "runtime-secret"
	var gotContext bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp/v1/clients/assistant-client" || r.Header.Get("Authorization") != "Bearer one-time-token" {
			t.Fatalf("unexpected Gateway request: path=%s authorization=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		payload := r.Header.Get("X-Aegis-MCP-Assistant-Context")
		signature := r.Header.Get("X-Aegis-MCP-Assistant-Signature")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(payload))
		gotContext = payload != "" && hmac.Equal([]byte(strings.ToLower(signature)), []byte(hex.EncodeToString(mac.Sum(nil))))
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Method == "tools/list" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"tools":[{"name":"list_hosts","title":"Hosts","description":"List hosts","risk_tier":"l2","inputSchema":{"type":"object"}}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"items":[{"id":"host-1"}]}}`))
	}))
	defer server.Close()

	client, err := NewMCPGatewayClient(MCPGatewayClientConfig{BaseURL: server.URL, ClientKey: "assistant-client", Token: "one-time-token", ContextSigningSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithToolInvocationContext(context.Background(), ToolInvocationContext{SessionID: "session-1", MessageID: "message-1", RunID: "run-1", CallID: "call-1", Operator: "alice"})
	tools, err := client.ListTools(ctx)
	if err != nil || len(tools) != 1 || tools[0].RiskTier != "l2" {
		t.Fatalf("unexpected tool list: %#v err=%v", tools, err)
	}
	result, err := client.Call(ctx, "list_hosts", map[string]interface{}{})
	if err != nil || result["items"] == nil {
		t.Fatalf("unexpected call result: %#v err=%v", result, err)
	}
	if !gotContext {
		t.Fatal("expected Assistant context signature to be forwarded")
	}
}

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"io"
	"log/slog"
)

func TestGatewayHealthAndFailClosedMCP(t *testing.T) {
	g := &gateway{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	server := httptest.NewServer(httpHandler(g))
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	request := httptest.NewRequest("POST", "/mcp/v1/catalogs/test", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	recorder := httptest.NewRecorder()
	g.handleMCP(recorder, request)
	if recorder.Code != 503 {
		t.Fatalf("expected fail-closed status, got %d", recorder.Code)
	}
}

func TestSignedSnapshotServesOnlyPublishedTools(t *testing.T) {
	key := []byte("test-signing-key")
	payload, err := json.Marshal(map[string]interface{}{
		"version": "2026.08.11.1", "catalog_key": "internal", "release_id": "release-1",
		"expires_at": time.Now().UTC().Add(time.Hour), "tools": []map[string]interface{}{{"exposed_name": "search", "input_schema": map[string]interface{}{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	envelope, _ := json.Marshal(signedSnapshot{Payload: payload, Signature: hex.EncodeToString(mac.Sum(nil))})
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, envelope, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadSnapshot(path, key)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateway{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), snapshot: snapshot}
	req := httptest.NewRequest("POST", "/mcp/v1/catalogs/internal", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	recorder := httptest.NewRecorder()
	g.handleMCP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "search") {
		t.Fatalf("expected published tool list, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestClientEndpointFiltersAndDispatchesThroughRuntimePolicy(t *testing.T) {
	var gotPath, gotClientKey, gotAuth string
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotClientKey = r.Header.Get("X-MCP-Client-Key")
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("X-Aegis-MCP-Gateway-Secret") != "shared" {
			http.Error(w, "bad secret", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/internal/mcp-runtime/tools" {
			writeJSON(w, http.StatusOK, map[string]interface{}{"tools": []map[string]interface{}{{"name": "list_hosts", "description": "List hosts", "inputSchema": map[string]interface{}{"type": "object"}}}})
			return
		}
		if r.URL.Path == "/internal/mcp-runtime/call" {
			writeJSON(w, http.StatusOK, map[string]interface{}{"result": map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "ok"}}}})
			return
		}
		http.NotFound(w, r)
	}))
	defer runtime.Close()
	g := &gateway{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), runtimeBaseURL: runtime.URL, runtimeSecret: "shared", runtimeHTTP: runtime.Client()}

	request := httptest.NewRequest(http.MethodPost, "/mcp/v1/clients/codex-aegis", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Authorization", "Bearer secret-token")
	recorder := httptest.NewRecorder()
	g.handleMCP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "list_hosts") || strings.Contains(recorder.Body.String(), "get_host") {
		t.Fatalf("unexpected tools/list response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if gotPath != "/internal/mcp-runtime/tools" || gotClientKey != "codex-aegis" || gotAuth != "Bearer secret-token" {
		t.Fatalf("runtime identity not forwarded path=%s client=%s auth=%s", gotPath, gotClientKey, gotAuth)
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp/v1/clients/codex-aegis", bytes.NewBufferString(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_hosts","arguments":{}}}`))
	request.Header.Set("Authorization", "Bearer secret-token")
	recorder = httptest.NewRecorder()
	g.handleMCP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ok") {
		t.Fatalf("unexpected tools/call response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestClientEndpointRequiresBearerToken(t *testing.T) {
	g := &gateway{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), runtimeBaseURL: "http://runtime", runtimeSecret: "shared", runtimeHTTP: &http.Client{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mcp/v1/clients/codex-aegis", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	g.handleMCP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status, got %d", recorder.Code)
	}
}

func httpHandler(g *gateway) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", g.health)
	mux.HandleFunc("/mcp/", g.handleMCP)
	return mux
}

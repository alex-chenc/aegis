package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultPort = 8084

type gateway struct {
	logger         *slog.Logger
	snapshot       *catalogSnapshot
	runtimeBaseURL string
	runtimeSecret  string
	runtimeHTTP    *http.Client
}

type catalogSnapshot struct {
	Version    string         `json:"version"`
	CatalogKey string         `json:"catalog_key"`
	ReleaseID  string         `json:"release_id"`
	ExpiresAt  time.Time      `json:"expires_at"`
	Tools      []snapshotTool `json:"tools"`
}

type snapshotTool struct {
	ExposedName  string          `json:"exposed_name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

type signedSnapshot struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		os.Exit(runHealthcheck())
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	g := &gateway{
		logger:         logger,
		runtimeBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("MCP_GATEWAY_RUNTIME_BASE_URL")), "/"),
		runtimeSecret:  strings.TrimSpace(os.Getenv("MCP_GATEWAY_RUNTIME_SECRET")),
		runtimeHTTP:    &http.Client{Timeout: 35 * time.Second},
	}
	if snapshotPath := strings.TrimSpace(os.Getenv("MCP_GATEWAY_SNAPSHOT_FILE")); snapshotPath != "" {
		key := []byte(os.Getenv("MCP_GATEWAY_SIGNING_KEY"))
		if loaded, err := loadSnapshot(snapshotPath, key); err != nil {
			logger.Error("mcp_gateway_snapshot_rejected", "error_code", "snapshot_invalid", "error", err)
		} else {
			g.snapshot = loaded
			logger.Info("mcp_gateway_snapshot_loaded", "catalog_key_hash", digestString(loaded.CatalogKey), "release_id", loaded.ReleaseID, "version", loaded.Version)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", g.health)
	mux.HandleFunc("/ready", g.ready)
	mux.HandleFunc("/mcp/", g.handleMCP)
	port := gatewayPort()
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	logger.Info("mcp_gateway_started", "port", port, "protocols", []string{"2025-11-25", "2026-07-28"})
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("mcp_gateway_stopped", "error_code", "listen_failed", "error", err)
		os.Exit(1)
	}
}

func gatewayPort() int {
	port := defaultPort
	if raw := strings.TrimSpace(os.Getenv("MCP_GATEWAY_PORT")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed < 65536 {
			port = parsed
		}
	}
	return port
}

func runHealthcheck() int {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(gatewayPort()) + "/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func (g *gateway) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (g *gateway) ready(w http.ResponseWriter, _ *http.Request) {
	if g.runtimeBaseURL != "" && g.runtimeSecret != "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ready", "mode": "runtime_policy"})
		return
	}
	if g.snapshot == nil || g.snapshot.ExpiresAt.Before(time.Now().UTC()) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"status": "degraded", "error_code": "catalog_snapshot_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ready"})
}

func (g *gateway) handleMCP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/mcp/v1/clients/") {
		g.handleClientMCP(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/mcp/v1/catalogs/") {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "catalog not found"})
		return
	}
	if g.snapshot == nil || g.snapshot.ExpiresAt.Before(time.Now().UTC()) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"jsonrpc": "2.0", "id": nil, "error": map[string]interface{}{"code": -32001, "message": "catalog snapshot unavailable"}})
		return
	}
	catalogKey := strings.Trim(strings.TrimPrefix(r.URL.Path, "/mcp/v1/catalogs/"), "/")
	if catalogKey == "" || catalogKey != g.snapshot.CatalogKey || strings.Contains(catalogKey, "/") {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "catalog not found"})
		return
	}
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"jsonrpc": "2.0", "id": nil, "error": map[string]interface{}{"code": -32700, "message": "invalid JSON-RPC request"}})
		return
	}
	if request.JSONRPC != "2.0" || strings.TrimSpace(request.Method) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "error": map[string]interface{}{"code": -32600, "message": "invalid JSON-RPC request"}})
		return
	}
	switch request.Method {
	case "initialize":
		writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": map[string]interface{}{"protocolVersion": "2025-11-25", "capabilities": map[string]interface{}{"tools": map[string]interface{}{}}, "serverInfo": map[string]string{"name": "aegis-mcp-gateway", "version": g.snapshot.Version}}})
	case "tools/list":
		writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": map[string]interface{}{"tools": g.snapshotTools()}})
	case "tools/call":
		// Read-only discovery is safe to serve from the signed snapshot. Calls
		// remain fail-closed until the Credential Broker/upstream dispatcher is
		// configured; never pretend an upstream call succeeded.
		g.logger.Warn("mcp_gateway_request_rejected", "method", request.Method, "reason", "upstream_dispatch_unavailable")
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "error": map[string]interface{}{"code": -32002, "message": "upstream dispatch unavailable"}})
	default:
		writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "error": map[string]interface{}{"code": -32601, "message": "method not found"}})
	}
}

func (g *gateway) handleClientMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	const prefix = "/mcp/v1/clients/"
	clientKey := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if clientKey == "" || strings.Contains(clientKey, "/") {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "client endpoint not found"})
		return
	}
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authorization, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "missing MCP bearer token"})
		return
	}
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"jsonrpc": "2.0", "id": nil, "error": map[string]interface{}{"code": -32700, "message": "invalid JSON-RPC request"}})
		return
	}
	if request.JSONRPC != "2.0" || strings.TrimSpace(request.Method) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "error": map[string]interface{}{"code": -32600, "message": "invalid JSON-RPC request"}})
		return
	}
	if request.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch request.Method {
	case "initialize":
		var tools struct {
			Tools []map[string]interface{} `json:"tools"`
		}
		if err := g.runtimeRequest(r, clientKey, authorization, "/internal/mcp-runtime/tools", map[string]interface{}{}, &tools); err != nil {
			g.writeRuntimeRPCError(w, request.ID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": map[string]interface{}{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]string{"name": "aegis-mcp-gateway", "version": "6.3"},
			"instructions":    "Only tools returned by Aegis policy are available to this client.",
		}})
	case "tools/list":
		var result struct {
			Tools []map[string]interface{} `json:"tools"`
		}
		if err := g.runtimeRequest(r, clientKey, authorization, "/internal/mcp-runtime/tools", map[string]interface{}{}, &result); err != nil {
			g.writeRuntimeRPCError(w, request.ID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": result})
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "error": map[string]interface{}{"code": -32602, "message": "tool name is required"}})
			return
		}
		var result struct {
			Result map[string]interface{} `json:"result"`
		}
		if err := g.runtimeRequest(r, clientKey, authorization, "/internal/mcp-runtime/call", map[string]interface{}{"tool_alias": params.Name, "arguments": params.Arguments}, &result); err != nil {
			g.writeRuntimeRPCError(w, request.ID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": result.Result})
	default:
		writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "error": map[string]interface{}{"code": -32601, "message": "method not found"}})
	}
}

func (g *gateway) runtimeRequest(incoming *http.Request, clientKey, authorization, path string, payload interface{}, output interface{}) error {
	if g.runtimeBaseURL == "" || g.runtimeSecret == "" || g.runtimeHTTP == nil {
		return errors.New("runtime gateway is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(incoming.Context(), http.MethodPost, g.runtimeBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authorization)
	req.Header.Set("X-Aegis-MCP-Gateway-Secret", g.runtimeSecret)
	req.Header.Set("X-MCP-Client-Key", clientKey)
	resp, err := g.runtimeHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var remote struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &remote)
		if remote.Error != "" {
			return fmt.Errorf("runtime request rejected (%d): %s", resp.StatusCode, remote.Error)
		}
		return fmt.Errorf("runtime request rejected (%d)", resp.StatusCode)
	}
	if err := json.Unmarshal(data, output); err != nil {
		return err
	}
	return nil
}

func (g *gateway) writeRuntimeRPCError(w http.ResponseWriter, id interface{}, err error) {
	message := "MCP runtime request failed"
	code := -32002
	if strings.Contains(err.Error(), "not allowed") || strings.Contains(err.Error(), "access denied") || strings.Contains(err.Error(), "rejected (403)") {
		code = -32003
		message = "tool is not allowed for this client"
	}
	g.logger.Warn("mcp_gateway_runtime_rejected", "error_code", code, "error", err)
	writeJSON(w, http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": map[string]interface{}{"code": code, "message": message}})
}

func (g *gateway) snapshotTools() []map[string]interface{} {
	tools := make([]map[string]interface{}, 0, len(g.snapshot.Tools))
	for _, tool := range g.snapshot.Tools {
		item := map[string]interface{}{"name": tool.ExposedName, "description": tool.Description, "inputSchema": json.RawMessage(tool.InputSchema)}
		if len(tool.OutputSchema) > 0 {
			item["outputSchema"] = json.RawMessage(tool.OutputSchema)
		}
		if tool.Title != "" {
			item["title"] = tool.Title
		}
		tools = append(tools, item)
	}
	return tools
}

func loadSnapshot(path string, key []byte) (*catalogSnapshot, error) {
	if len(key) == 0 {
		return nil, errors.New("snapshot signing key is not configured")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var envelope signedSnapshot
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Payload) == 0 || envelope.Signature == "" {
		return nil, errors.New("invalid signed snapshot envelope")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(envelope.Payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(expected)), []byte(strings.ToLower(envelope.Signature))) {
		return nil, errors.New("snapshot signature mismatch")
	}
	var snapshot catalogSnapshot
	if err := json.Unmarshal(envelope.Payload, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.CatalogKey == "" || snapshot.ReleaseID == "" || snapshot.Version == "" || snapshot.ExpiresAt.IsZero() || !snapshot.ExpiresAt.After(time.Now().UTC()) {
		return nil, errors.New("snapshot is missing identity or is expired")
	}
	if len(snapshot.Tools) > 1000 {
		return nil, errors.New("snapshot contains too many tools")
	}
	for _, tool := range snapshot.Tools {
		if tool.ExposedName == "" || len(tool.ExposedName) > 255 || len(tool.InputSchema) > 512*1024 {
			return nil, errors.New("snapshot contains invalid tool schema")
		}
	}
	return &snapshot, nil
}

func digestString(value string) string {
	mac := sha256.Sum256([]byte(value))
	return hex.EncodeToString(mac[:])[:16]
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("mcp_gateway_request_completed", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

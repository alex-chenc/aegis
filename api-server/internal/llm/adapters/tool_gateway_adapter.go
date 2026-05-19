package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	agentruntime "github.com/alex-chenc/agent-runtime"

	"api-server/internal/grpc"
)

// ToolGatewayAdapter implements agent-runtime's ToolGateway interface by
// wrapping the existing gRPC ServerClient used for agent communication.
type ToolGatewayAdapter struct {
	serverClient   *grpc.ServerClient
	defaultHostIDs []string
}

// NewToolGatewayAdapter creates a ToolGatewayAdapter that forwards tool calls
// to agents via the gRPC ServerClient. defaultHostIDs provides fallback host
// routing when a tool request does not specify a host_id.
func NewToolGatewayAdapter(serverClient *grpc.ServerClient, defaultHostIDs []string) *ToolGatewayAdapter {
	return &ToolGatewayAdapter{
		serverClient:   serverClient,
		defaultHostIDs: defaultHostIDs,
	}
}

// Call implements agentruntime.ToolGateway. It normalises the incoming request
// arguments, resolves a target host, and executes the tool synchronously via gRPC.
func (a *ToolGatewayAdapter) Call(ctx context.Context, req agentruntime.ToolRequest) (agentruntime.ToolResponse, error) {
	startedAt := time.Now()

	// 1. Normalise argument keys: camelCase -> snake_case
	normalizedArgs := normalizeArgs(req.Args)

	// 2. Resolve host_id
	hostID := resolveHostID(normalizedArgs, a.defaultHostIDs)
	if hostID == "" {
		return toolErrorResponse(req, startedAt,
			"tool %s requires host_id parameter but none was provided and no default is available", req.ToolName), nil
	}
	normalizedArgs["host_id"] = hostID

	// 3. Apply tool-specific defaults
	applyToolDefaults(req.ToolName, normalizedArgs)

	// 4. Serialise arguments to JSON
	argsJSON, err := json.Marshal(normalizedArgs)
	if err != nil {
		return toolErrorResponse(req, startedAt,
			"failed to marshal tool arguments: %v", err), nil
	}

	// 5. Compute timeout
	timeoutSeconds := int32(60)
	if req.Timeout > 0 {
		timeoutSeconds = int32(req.Timeout.Seconds())
		if timeoutSeconds < 1 {
			timeoutSeconds = 1
		}
	}

	// 6. Execute via gRPC
	resp, err := a.serverClient.ExecuteTool(ctx, req.CallID, hostID, req.ToolName, string(argsJSON), timeoutSeconds)
	if err != nil {
		return toolErrorResponse(req, startedAt, "gRPC ExecuteTool failed: %v", err), nil
	}

	endedAt := time.Now()

	// 7. Build ToolResponse from gRPC result
	if resp.Success {
		return agentruntime.ToolResponse{
			CallID:    req.CallID,
			ToolName:  req.ToolName,
			Status:    agentruntime.ToolCallSuccess,
			Content:   resp.Result,
			Summary:   fmt.Sprintf("tool %s executed successfully", req.ToolName),
			StartedAt: startedAt,
			EndedAt:   endedAt,
		}, nil
	}

	return agentruntime.ToolResponse{
		CallID:       req.CallID,
		ToolName:     req.ToolName,
		Status:       agentruntime.ToolCallFailed,
		Content:      resp.Result,
		Summary:      fmt.Sprintf("tool %s failed", req.ToolName),
		ErrorMessage: resp.Error,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
	}, nil
}

// Cancel implements agentruntime.ToolGateway. It is a no-op because gRPC
// ExecuteTool is synchronous and is automatically terminated when the context
// is cancelled.
func (a *ToolGatewayAdapter) Cancel(_ context.Context, _ string, _ string) error {
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveHostID extracts host_id from the normalised arguments, falling back
// to the first default host ID when the argument is missing or a placeholder.
func resolveHostID(args map[string]any, defaultHostIDs []string) string {
	raw, ok := args["host_id"]
	if ok {
		if s, ok := raw.(string); ok && s != "" && !isPlaceholderToolValue(s) {
			return s
		}
	}
	if len(defaultHostIDs) > 0 {
		return defaultHostIDs[0]
	}
	return ""
}

// applyToolDefaults fills in sensible defaults for specific tools when the
// caller did not supply required parameters.
func applyToolDefaults(toolName string, args map[string]any) {
	if toolName != "QueryHistoricalLogs" {
		return
	}

	now := time.Now()

	if _, ok := args["end_time"]; !ok {
		args["end_time"] = now.Format(time.RFC3339)
	}
	if _, ok := args["start_time"]; !ok {
		args["start_time"] = now.Add(-24 * time.Hour).Format(time.RFC3339)
	}
}

// toolErrorResponse is a convenience builder for error ToolResponses.
func toolErrorResponse(req agentruntime.ToolRequest, startedAt time.Time, format string, a ...any) agentruntime.ToolResponse {
	return agentruntime.ToolResponse{
		CallID:       req.CallID,
		ToolName:     req.ToolName,
		Status:       agentruntime.ToolCallFailed,
		ErrorMessage: fmt.Sprintf(format, a...),
		Summary:      fmt.Sprintf("tool %s failed", req.ToolName),
		StartedAt:    startedAt,
		EndedAt:      time.Now(),
	}
}

// normalizeArgs converts every key in args from camelCase to snake_case.
// Values are preserved unchanged.
func normalizeArgs(args map[string]any) map[string]any {
	if args == nil {
		return make(map[string]any)
	}
	result := make(map[string]any, len(args))
	for k, v := range args {
		result[camelToSnake(k)] = v
	}
	return result
}

// camelToSnake converts a camelCase or PascalCase string to snake_case.
//
//	"hostId"      -> "host_id"
//	"startTime"   -> "start_time"
//	"PID"         -> "pid"
//	"hostID"      -> "host_id"
//	"processName" -> "process_name"
func camelToSnake(s string) string {
	if s == "" {
		return s
	}

	var buf strings.Builder
	buf.Grow(len(s) + 4) // rough upper-bound for inserted underscores

	runes := []rune(s)
	n := len(runes)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			// Insert underscore when transitioning from lowercase/digit to uppercase,
			// or from uppercase to uppercase-followed-by-lowercase (e.g. "hostID" -> "host_id",
			// but "PID" stays "pid").
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					buf.WriteRune('_')
				} else if unicode.IsUpper(prev) && i+1 < n && unicode.IsLower(runes[i+1]) {
					buf.WriteRune('_')
				}
			}
			buf.WriteRune(unicode.ToLower(r))
		} else {
			buf.WriteRune(r)
		}
	}

	return buf.String()
}

// isPlaceholderToolValue returns true when val looks like an unresolved
// placeholder that an LLM might emit instead of a real value.
func isPlaceholderToolValue(val string) bool {
	val = strings.TrimSpace(val)
	if val == "" {
		return true
	}
	lower := strings.ToLower(val)
	switch lower {
	case "host_id", "hostid", "hostname", "<host_id>", "{{host_id}}",
		"[host_id]", "[the host id]", "...", "your_host_id":
		return true
	default:
		return strings.HasPrefix(val, "[") || strings.HasPrefix(val, "<") || strings.HasPrefix(val, "{{")
	}
}

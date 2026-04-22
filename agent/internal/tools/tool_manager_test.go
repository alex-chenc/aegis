package tools

import (
	"testing"
)

func TestToolManager_QueryHistoricalLogs(t *testing.T) {
	m := NewToolManager()

	// Test with valid time range parameters
	params := map[string]interface{}{
		"start_time": "2026-04-01T00:00:00Z",
		"end_time":   "2026-04-22T23:59:59Z",
		"filter":     "ssh",
	}

	result, err := m.Execute("QueryHistoricalLogs", params)
	if err != nil {
		t.Errorf("QueryHistoricalLogs returned error: %v", err)
		return
	}

	if result == nil {
		t.Error("QueryHistoricalLogs returned nil result")
		return
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Error("QueryHistoricalLogs result is not a map")
		return
	}

	if _, ok := resultMap["logs"]; !ok {
		t.Error("QueryHistoricalLogs result missing 'logs' field")
	}

	if _, ok := resultMap["count"]; !ok {
		t.Error("QueryHistoricalLogs result missing 'count' field")
	}

	t.Logf("QueryHistoricalLogs result: %+v", resultMap)
}

func TestToolManager_UnknownTool(t *testing.T) {
	m := NewToolManager()

	params := map[string]interface{}{}
	_, err := m.Execute("NonExistentTool", params)
	if err == nil {
		t.Error("Expected error for unknown tool, got nil")
		return
	}

	expectedErr := "unknown tool: NonExistentTool"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestToolManager_GetProcessTree(t *testing.T) {
	m := NewToolManager()

	params := map[string]interface{}{
		"pid": float64(1), // systemd process
	}

	result, err := m.Execute("GetProcessTree", params)
	if err != nil {
		t.Errorf("GetProcessTree returned error: %v", err)
		return
	}

	if result == nil {
		t.Error("GetProcessTree returned nil result")
		return
	}

	t.Logf("GetProcessTree result: %+v", result)
}

func TestToolManager_GetNetworkConnections(t *testing.T) {
	m := NewToolManager()

	params := map[string]interface{}{
		"pid": float64(1),
	}

	result, err := m.Execute("GetNetworkConnections", params)
	if err != nil {
		t.Errorf("GetNetworkConnections returned error: %v", err)
		return
	}

	if result == nil {
		t.Error("GetNetworkConnections returned nil result")
		return
	}

	t.Logf("GetNetworkConnections result: %+v", result)
}
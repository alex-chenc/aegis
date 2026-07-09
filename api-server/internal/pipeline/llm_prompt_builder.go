package pipeline

import (
	"encoding/json"
	"fmt"
	"time"
)

// LLMAnalysisInput represents the input structure for LLM analysis
type LLMAnalysisInput struct {
	HostID      string         `json:"host_id"`
	WindowStart string         `json:"window_start"`
	WindowEnd   string         `json:"window_end"`
	Events      []RuntimeEvent `json:"events"`
}

// ToolDefinition represents a tool available to the LLM
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// BuildAnalysisPrompt builds the complete prompt for LLM analysis
func BuildAnalysisPrompt(window *HostWindow) (string, error) {
	input := LLMAnalysisInput{
		HostID:      window.HostID,
		WindowStart: window.WindowStart.Format(time.RFC3339),
		WindowEnd:   window.WindowEnd.Format(time.RFC3339),
		Events:      window.Events,
	}

	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal input: %w", err)
	}

	systemPrompt := getSystemPrompt()
	return fmt.Sprintf("%s\n\nEvent data:\n%s", systemPrompt, string(inputJSON)), nil
}

// getSystemPrompt returns the system prompt for LLM analysis
func getSystemPrompt() string {
	return `You are a host-security analyst. Analyze the security events in the following two-minute window and determine whether they indicate a real threat.

Available tools:
1. get_process_tree(pid): retrieve parent-child process relationships.
2. get_network_connections(pid): retrieve external and local network connections.
3. get_file_info(path): retrieve file attributes.
4. get_user_info(username): retrieve user and privilege information.

Return JSON with this schema:
{
  "alerts": [
    {
      "rule_id": "rule ID, for example reverse_shell_001",
      "rule_title": "rule title in the user's language",
      "mitre_id": "T1059.004",
      "mitre_name": "Command and Scripting Interpreter",
      "severity": "critical|high|medium|low",
      "pid": 12345,
      "description": "detailed threat description",
      "llm_summary": "one-sentence conclusion",
      "disposal_strategy": "recommended response",
      "block_action": "kill_process|quarantine_file|block_connection",
      "block_target": "process PID or file path",
      "judgment_source": "ai"
    }
  ],
  "tool_calls": [
    {
      "tool": "get_process_tree",
      "params": {"pid": 12345},
      "reason": "reason for the call"
    }
  ],
  "rule_adjustments": [
    {
      "rule_id": "rule_id",
      "action": "tighten|loosen",
      "reason": "reason for the adjustment"
    }
  ]
}

Analysis rules:
1. Analyze each command line and its context.
2. Distinguish normal administration from malicious behavior.
3. Consider parent-child process relationships.
4. Request tool data when evidence is insufficient.
5. Use at most ten tool calls.
6. alerts may be empty for false positives.
7. Derive severity from evidence and MITRE ATT&CK impact.
8. Prioritize reverse shells, privilege escalation, and exfiltration.
9. description and llm_summary are required.
10. Write user-facing text fields in the user's language.`
}

// BuildToolResultPrompt builds a prompt to include tool call results
func BuildToolResultPrompt(originalPrompt string, toolName string, result string) string {
	return fmt.Sprintf(`%s

Tool result:
Tool: %s
Result: %s

Continue the analysis using this result and return the updated JSON analysis object.`, originalPrompt, toolName, result)
}

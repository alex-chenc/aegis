package pipeline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// LLMAnalysisOutput represents the parsed output from LLM analysis
type LLMAnalysisOutput struct {
	Alerts          []AlertPayload    `json:"alerts"`
	ToolCalls       []ToolCallPayload `json:"tool_calls"`
	RuleAdjustments []RuleAdjustment  `json:"rule_adjustments"`
}

// AlertPayload represents an alert from LLM analysis
type AlertPayload struct {
	RuleID           string `json:"rule_id"`
	RuleTitle        string `json:"rule_title"`
	MitreID          string `json:"mitre_id"`
	MitreName        string `json:"mitre_name"`
	Severity         string `json:"severity"`
	PID              int    `json:"pid"`
	Description      string `json:"description"`
	LLMSummary       string `json:"llm_summary"`
	DisposalStrategy string `json:"disposal_strategy"`
	BlockAction      string `json:"block_action"`
	BlockTarget      string `json:"block_target"`
	JudgmentSource   string `json:"judgment_source"`
}

// ToolCallPayload represents a tool call request from LLM
type ToolCallPayload struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
	Reason string                 `json:"reason"`
}

// RuleAdjustment represents a rule adjustment suggestion from LLM
type RuleAdjustment struct {
	RuleID string `json:"rule_id"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// ParseLLMResponse parses and validates LLM response
func ParseLLMResponse(response string) (*LLMAnalysisOutput, error) {
	cleaned := extractJSON(response)
	if cleaned == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var output LLMAnalysisOutput
	if err := json.Unmarshal([]byte(cleaned), &output); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if err := validateOutput(&output); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if len(output.ToolCalls) > 10 {
		output.ToolCalls = output.ToolCalls[:10]
	}

	return &output, nil
}

// extractJSON extracts JSON from LLM response, handling markdown code blocks
func extractJSON(text string) string {
	re := regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}

	return strings.TrimSpace(text)
}

// validateOutput validates the parsed output
func validateOutput(output *LLMAnalysisOutput) error {
	for i, alert := range output.Alerts {
		if alert.MitreID == "" {
			return fmt.Errorf("alert[%d]: mitre_id is required", i)
		}
		if alert.Severity == "" {
			return fmt.Errorf("alert[%d]: severity is required", i)
		}
		validSeverities := map[string]bool{
			"critical": true,
			"high":     true,
			"medium":   true,
			"low":      true,
		}
		if !validSeverities[alert.Severity] {
			return fmt.Errorf("alert[%d]: invalid severity '%s'", i, alert.Severity)
		}
	}

	for i, call := range output.ToolCalls {
		if call.Tool == "" {
			return fmt.Errorf("tool_call[%d]: tool name is required", i)
		}
	}

	for i, adj := range output.RuleAdjustments {
		if adj.RuleID == "" {
			return fmt.Errorf("rule_adjustment[%d]: rule_id is required", i)
		}
		if adj.Action == "" {
			return fmt.Errorf("rule_adjustment[%d]: action is required", i)
		}
		validActions := map[string]bool{
			"tighten": true,
			"loosen":  true,
		}
		if !validActions[adj.Action] {
			return fmt.Errorf("rule_adjustment[%d]: invalid action '%s'", i, adj.Action)
		}
	}

	return nil
}

// HasToolCalls returns true if the output contains tool calls
func (o *LLMAnalysisOutput) HasToolCalls() bool {
	return len(o.ToolCalls) > 0
}

// HasAlerts returns true if the output contains alerts
func (o *LLMAnalysisOutput) HasAlerts() bool {
	return len(o.Alerts) > 0
}

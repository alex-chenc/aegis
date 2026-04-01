package pipeline

import (
	"dc/internal/model"
	"fmt"
	"strings"
)

type LLMPromptBuilder struct{}

func NewLLMPromptBuilder() *LLMPromptBuilder {
	return &LLMPromptBuilder{}
}

// Build builds an LLM prompt from events
func (b *LLMPromptBuilder) Build(events []*model.RuntimeEvent) string {
	if len(events) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("You are a security analyst analyzing host security events.\n\n")
	sb.WriteString("## Events to Analyze\n\n")

	for i, event := range events {
		sb.WriteString(fmt.Sprintf("### Event %d\n", i+1))
		sb.WriteString(fmt.Sprintf("- Event Type: %s\n", event.EventType))
		sb.WriteString(fmt.Sprintf("- Process: %s\n", event.ProcessName))
		sb.WriteString(fmt.Sprintf("- Command: %s\n", event.CommandLine))
		sb.WriteString(fmt.Sprintf("- MITRE ID: %s\n", event.MitreID))
		sb.WriteString(fmt.Sprintf("- Severity: %s\n", event.Severity))
		sb.WriteString(fmt.Sprintf("- Rule ID: %s\n", event.MatchedRuleID))
		sb.WriteString("\n")
	}

	sb.WriteString("## Analysis Request\n")
	sb.WriteString("Please analyze these events and respond with:\n")
	sb.WriteString("1. A brief summary of what happened\n")
	sb.WriteString("2. Severity assessment (critical/high/medium/low)\n")
	sb.WriteString("3. MITRE technique if applicable\n")
	sb.WriteString("4. Recommended actions\n\n")
	sb.WriteString("Format your response as JSON with fields: summary, severity, mitre_technique, analysis_details\n")

	return sb.String()
}

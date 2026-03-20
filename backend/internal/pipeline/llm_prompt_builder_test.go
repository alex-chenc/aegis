package pipeline

import (
	"strings"
	"testing"
	"time"
)

func TestBuildAnalysisPrompt(t *testing.T) {
	window := &HostWindow{
		HostID:      "test-host-001",
		WindowStart: time.Now().Add(-2 * time.Minute),
		WindowEnd:   time.Now(),
		Events: []RuntimeEvent{
			{
				EventType:   "process_exec",
				PID:         12345,
				CommandLine: "/bin/bash -i",
				MitreID:     "T1059.004",
				Severity:    "critical",
				Timestamp:   time.Now(),
			},
		},
	}

	prompt, err := BuildAnalysisPrompt(window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify prompt contains key elements
	if !strings.Contains(prompt, "主机安全分析专家") {
		t.Error("prompt should contain system role description")
	}
	if !strings.Contains(prompt, "test-host-001") {
		t.Error("prompt should contain host ID")
	}
	if !strings.Contains(prompt, "T1059.004") {
		t.Error("prompt should contain MITRE ID")
	}
	if !strings.Contains(prompt, "get_process_tree") {
		t.Error("prompt should contain tool definitions")
	}
}

func TestBuildToolResultPrompt(t *testing.T) {
	originalPrompt := "分析以下事件"
	result := "进程树: bash -> python -> nc"

	prompt := BuildToolResultPrompt(originalPrompt, "get_process_tree", result)

	if !strings.Contains(prompt, originalPrompt) {
		t.Error("prompt should contain original prompt")
	}
	if !strings.Contains(prompt, "get_process_tree") {
		t.Error("prompt should contain tool name")
	}
	if !strings.Contains(prompt, result) {
		t.Error("prompt should contain tool result")
	}
}

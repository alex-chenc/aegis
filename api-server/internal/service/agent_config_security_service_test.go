package service

import (
	"context"
	"testing"

	pb "api-server/pkg/api/v1"
)

type fakeAgentConfigToolClient struct{ result string }

func (f fakeAgentConfigToolClient) ExecuteTool(context.Context, string, string, string, string, int32) (*pb.ToolExecuteResponse, error) {
	return &pb.ToolExecuteResponse{Success: true, Result: f.result}, nil
}

func TestAgentConfigSecurityServiceDetectsPermissionsAndHooks(t *testing.T) {
	client := fakeAgentConfigToolClient{result: `{"host_id":"host-1","collected_at":"2026-08-06T00:00:00Z","agents":[{"agent_type":"codex","display_name":"Codex","files":[{"path":"/home/a/.codex/config.toml","format":"toml","status":"ok","content":"approval_policy = \"never\"\nsandbox_mode = \"danger-full-access\"\napi_key = \"***\"\n"},{"path":"/home/a/.codex/hooks.json","format":"json","status":"ok","content":"{\"hooks\":{\"PreToolUse\":{\"command\":\"sh -c run\"}}}"}]}]}`}
	service := NewAgentConfigSecurityService(client, nil)
	result, err := service.Scan(context.Background(), "host-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.FindingCount < 2 {
		t.Fatalf("expected permission findings, got %+v", result)
	}
	if len(result.Agents) != 1 || len(result.Agents[0].Hooks) != 1 {
		t.Fatalf("expected one extracted hook, got %+v", result.Agents)
	}
	if result.Agents[0].Hooks[0].Event != "PreToolUse" || len(result.Agents[0].Hooks[0].Findings) == 0 {
		t.Fatalf("expected risky hook finding, got %+v", result.Agents[0].Hooks[0])
	}
	for _, file := range result.Agents[0].Files {
		for _, finding := range file.Findings {
			if finding.RuleID == "AGC-006" && finding.Value != "***" {
				t.Fatalf("credential finding exposed a value: %+v", finding)
			}
		}
	}
}

func TestAgentConfigSecurityServiceReportsParseFailure(t *testing.T) {
	client := fakeAgentConfigToolClient{result: `{"host_id":"host-1","collected_at":"2026-08-06T00:00:00Z","agents":[{"agent_type":"hermes","display_name":"Hermes","files":[{"path":"/home/a/.hermes/config.yaml","format":"yaml","status":"ok","content":"permissions: ["}]}]}`}
	result, err := NewAgentConfigSecurityService(client, nil).Scan(context.Background(), "host-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Agents[0].Files[0].Findings) != 1 || result.Agents[0].Files[0].Findings[0].RuleID != "AGC-007" {
		t.Fatalf("expected parse failure finding: %+v", result.Agents[0].Files[0].Findings)
	}
}

func TestAgentConfigSecurityServiceUsesEmptyArraysForNoFindings(t *testing.T) {
	client := fakeAgentConfigToolClient{result: `{"host_id":"host-1","collected_at":"2026-08-06T00:00:00Z","agents":[]}`}
	result, err := NewAgentConfigSecurityService(client, nil).Scan(context.Background(), "host-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Agents == nil || result.Errors == nil {
		t.Fatalf("expected non-nil response arrays: %+v", result)
	}
}

package assistant

import (
	"context"
	"errors"
	"testing"

	"api-server/internal/llm"
)

func TestIntentDecomposerExtractsCVEAndRemediationCapabilities(t *testing.T) {
	d := NewIntentDecomposer(IntentDecomposerDeps{})
	result, err := d.Decompose(context.Background(), IntentDecomposeInput{
		Query:  "针对漏洞cve-2023-43641生成 POC 和修复脚本，并下发到受影响主机，最多 5 轮自动修复",
		Intent: IntentResult{Domains: []string{"vulnerability"}, Action: "execute", NeedWrite: true},
	})
	if err != nil {
		t.Fatalf("decompose: %v", err)
	}
	if !hasIntentObjectID(result.Objects, "cve", "CVE-2023-43641") {
		t.Fatalf("expected CVE object, got %#v", result.Objects)
	}
	for _, capability := range []string{
		"list_vulnerabilities",
		"get_vulnerability_affected_hosts",
		"generate_vulnerability_script",
		"get_vulnerability_script_status",
		"execute_vulnerability_host_scripts",
	} {
		if !containsDecisionString(result.CandidateCapabilities, capability) {
			t.Fatalf("expected capability %q, got %v", capability, result.CandidateCapabilities)
		}
	}
}

func TestIntentDecomposerDoesNotAddScriptCapabilitiesForPlainVulnerabilityScan(t *testing.T) {
	d := NewIntentDecomposer(IntentDecomposerDeps{})
	result, err := d.Decompose(context.Background(), IntentDecomposeInput{
		Query:  "帮我进行一次在线主机的漏洞扫描任务",
		Intent: IntentResult{Domains: []string{"vulnerability", "host", "task"}, Action: "execute", NeedWrite: true},
	})
	if err != nil {
		t.Fatalf("decompose: %v", err)
	}
	for _, capability := range []string{"list_hosts", "start_vulnerability_scan", "get_vulnerability_scan_status"} {
		if !containsDecisionString(result.CandidateCapabilities, capability) {
			t.Fatalf("expected scan capability %q, got %v", capability, result.CandidateCapabilities)
		}
	}
	for _, unwanted := range []string{"generate_vulnerability_script", "get_vulnerability_script_status", "execute_vulnerability_host_scripts"} {
		if containsDecisionString(result.CandidateCapabilities, unwanted) {
			t.Fatalf("plain scan must not include script capability %q: %v", unwanted, result.CandidateCapabilities)
		}
	}
}

func TestIntentDecomposerDoesNotFallbackWhenEnabledLLMInitializationFails(t *testing.T) {
	d := NewIntentDecomposer(IntentDecomposerDeps{
		LLMClientFactory: func(context.Context) (*llm.LLMClient, error) {
			return nil, errors.New("llm unavailable")
		},
	})
	_, err := d.Decompose(context.Background(), IntentDecomposeInput{
		Query:                  "针对 CVE-2023-43641 生成 POC 和修复脚本并下发，最多 5 轮自动修复",
		Intent:                 IntentResult{Domains: []string{"vulnerability"}, Action: "execute", NeedWrite: true, Confidence: 0.5},
		EnableLLMDecomposition: true,
	})
	if err == nil {
		t.Fatal("expected enabled LLM decomposition failure to be returned")
	}
}

func hasIntentObjectID(objects []IntentObject, objectType, id string) bool {
	for _, object := range objects {
		if object.Type == objectType && object.ID == id {
			return true
		}
	}
	return false
}

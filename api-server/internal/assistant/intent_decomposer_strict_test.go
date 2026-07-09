package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"api-server/internal/llm"
)

func TestIntentObjectBareStringRemainsOpenBusinessType(t *testing.T) {
	var object IntentObject
	if err := json.Unmarshal([]byte(`"release_candidate"`), &object); err != nil {
		t.Fatal(err)
	}
	if object.Type != "release_candidate" || object.ID != "" {
		t.Fatalf("bare string must remain a generic object type, got %#v", object)
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

func TestValidateIntentBreakdownAcceptsOpenScopeAndParameters(t *testing.T) {
	var breakdown IntentBreakdown
	if err := json.Unmarshal([]byte(`{
		"goal":"compare two arbitrary resources",
		"domains":["custom_domain"],
		"actions":["compare"],
		"objects":[{"type":"release_candidate","id":"rc-42"}],
		"scope":{"kind":"production_cluster"},
		"parameters":{"region":"cn-north","threshold":0.75,"modes":["fast","safe"]},
		"requires_write":false,
		"risk_hint":"readonly",
		"candidate_capabilities":["compare_resources"],
		"need_clarification":false,
		"reason":"explicit request",
		"confidence":0.9
	}`), &breakdown); err != nil {
		t.Fatal(err)
	}
	if err := validateIntentBreakdown(&breakdown); err != nil {
		t.Fatalf("open business intent should be valid: %v", err)
	}
	encoded, err := json.Marshal(breakdown.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"region":"cn-north"`) || !strings.Contains(string(encoded), `"threshold":0.75`) {
		t.Fatalf("arbitrary parameters were not preserved: %s", encoded)
	}
}

func TestValidateIntentBreakdownRequiresEnglishCapabilityIdentifiers(t *testing.T) {
	cases := []string{
		"生成漏洞脚本",
		"generate vulnerability script",
		"1_generate_script",
		"generate_脚本",
		"generate_script_🔧",
	}
	for _, capability := range cases {
		t.Run(capability, func(t *testing.T) {
			breakdown := &IntentBreakdown{
				Goal:                  "generate a vulnerability script",
				Scope:                 IntentScope{Kind: "vulnerability"},
				Confidence:            0.9,
				CandidateCapabilities: []string{capability},
			}
			err := validateIntentBreakdown(breakdown)
			if err == nil || !strings.Contains(err.Error(), "candidate_capabilities") {
				t.Fatalf("expected invalid capability %q to be rejected, got %v", capability, err)
			}
		})
	}
}

func TestNormalizeIntentBreakdownCanonicalizesEnglishCapabilities(t *testing.T) {
	breakdown := &IntentBreakdown{
		Scope:                 IntentScope{Kind: "vulnerability"},
		CandidateCapabilities: []string{" Generate_Vulnerability_Script ", "generate_vulnerability_script"},
	}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{
		Query:                 "生成漏洞脚本",
		CandidateCapabilities: []string{"List_Vulnerabilities"},
	})

	want := []string{"generate_vulnerability_script", "list_vulnerabilities"}
	if len(breakdown.CandidateCapabilities) != len(want) {
		t.Fatalf("capabilities = %#v, want %#v", breakdown.CandidateCapabilities, want)
	}
	for i := range want {
		if breakdown.CandidateCapabilities[i] != want[i] {
			t.Fatalf("capabilities = %#v, want %#v", breakdown.CandidateCapabilities, want)
		}
	}
	if err := validateIntentBreakdown(breakdown); err != nil {
		t.Fatalf("normalized English capabilities should be valid: %v", err)
	}
}

func TestNormalizeIntentBreakdownDoesNotInjectScenarioCapabilities(t *testing.T) {
	breakdown := &IntentBreakdown{Scope: IntentScope{Kind: "online_hosts"}}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{
		Query:  "检查在线目标",
		Intent: IntentResult{Domains: []string{"host"}, Action: "query"},
	})
	if len(breakdown.CandidateCapabilities) != 0 {
		t.Fatalf("normalization must not inject a named scenario capability: %#v", breakdown)
	}
	if err := validateIntentBreakdown(breakdown); err != nil {
		t.Fatalf("normalized breakdown should be valid: %v", err)
	}
}

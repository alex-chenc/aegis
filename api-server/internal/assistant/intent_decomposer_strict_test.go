package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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

func TestNormalizeIntentBreakdownPreservesFirstLayerWorkflowContract(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:       "Generate a dynamic detection package",
		Actions:    []string{"generate"},
		Scope:      IntentScope{Kind: "unspecified"},
		Confidence: 0.9,
	}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{
		Query: "生成动态检测包",
		Intent: IntentResult{
			Action:      "generate",
			WorkflowIDs: []string{detectionPackageLifecycleWorkflowID},
		},
	})
	if got := breakdown.WorkflowIDs; len(got) != 1 || got[0] != detectionPackageLifecycleWorkflowID {
		t.Fatalf("workflow_ids = %#v, want first-layer workflow contract to be preserved", got)
	}
}

func TestNormalizeIntentBreakdownCompletesVulnerabilityRemediationCapabilities(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:          "针对主机192.168.152.159进行CVE-2023-29484的POC验证，如果存在漏洞则进行修复",
		Actions:       []string{"poc_verification", "remediation"},
		Objects:       []IntentObject{{Type: "host", ID: "192.168.152.159"}, {Type: "vulnerability", ID: "CVE-2023-29484"}},
		Scope:         IntentScope{Kind: "specific", ObjectIDs: []string{"192.168.152.159", "CVE-2023-29484"}},
		RequiresWrite: true,
		WorkflowIDs:   []string{vulnerabilityRemediationWorkflowID},
		CandidateCapabilities: []string{
			"resolve_hosts",
			"list_vulnerabilities",
			"execute_vulnerability_host_scripts",
		},
	}

	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: breakdown.Goal})

	for _, required := range []string{
		"resolve_hosts",
		"generate_vulnerability_script",
		"execute_vulnerability_host_scripts",
	} {
		if !containsExactString(breakdown.CandidateCapabilities, required) {
			t.Fatalf("required workflow capability %q missing from %#v", required, breakdown.CandidateCapabilities)
		}
	}
	if containsExactString(breakdown.CandidateCapabilities, "start_vulnerability_scan") {
		t.Fatalf("vulnerability remediation normalization must not inject scan capability: %#v", breakdown.CandidateCapabilities)
	}
}

func TestNormalizeIntentBreakdownRoutesDynamicDetectionPackageAwayFromVulnerabilityScripts(t *testing.T) {
	query := "样本通过 AF_ALG、pipe 与 splice 利用 CVE-2026-31431 本地提权，请通过动态检测包的方式进行检测"
	breakdown := &IntentBreakdown{
		Goal:    "Detect CVE-2026-31431 exploitation using a dynamic detection package",
		Domains: []string{"vulnerability", "cybersecurity"},
		Actions: []string{"lookup", "generate", "build", "enable"},
		Objects: []IntentObject{
			{Type: "cve", ID: "CVE-2026-31431"},
			{Type: "dynamic_detection"},
		},
		Scope:         IntentScope{Kind: "unspecified"},
		Parameters:    IntentParameters{"cve_id": "CVE-2026-31431", "detection_method": "dynamic_detection_package"},
		RequiresWrite: false,
		RiskHint:      "readonly",
		WorkflowIDs:   []string{"cve_lookup", detectionPackageLifecycleWorkflowID},
		CandidateCapabilities: []string{
			"list_detection_packages",
			"get_detection_package",
			"generate_detection_package_draft",
			"start_detection_package_build",
			"enable_detection_package",
		},
	}

	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: query})

	if !breakdown.RequiresWrite {
		t.Fatal("dynamic detection package generation must be treated as a write operation")
	}
	if breakdown.RiskHint != string(ToolRiskMedium) {
		t.Fatalf("risk_hint = %q, want %q", breakdown.RiskHint, ToolRiskMedium)
	}
	if got, want := breakdown.CandidateCapabilities, []string{"generate_detection_package_draft", "start_detection_package_build"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %#v, want %#v", got, want)
	}
	for _, object := range breakdown.Objects {
		if object.Type == "dynamic_detection" {
			t.Fatalf("dynamic_detection object was not canonicalized: %#v", breakdown.Objects)
		}
	}
	if got := breakdown.Parameters["vulnerability_description"]; got != query {
		t.Fatalf("vulnerability_description = %#v, want original user query", got)
	}
}

func TestNormalizeIntentBreakdownCanonicalizesCapturedDetectionPackageContract(t *testing.T) {
	query := "样本利用 CVE-2026-31431 实现本地提权，你通过动态检测包的方式来进行检测"
	breakdown := &IntentBreakdown{
		Goal:       "Generate a dynamic detection package to detect CVE-2026-31431 exploitation.",
		Domains:    []string{"vulnerability", "package"},
		Actions:    []string{"generate_detection_package"},
		Objects:    []IntentObject{{Type: "cve", ID: "CVE-2026-31431"}, {Type: "detection_package"}},
		Scope:      IntentScope{Kind: "unspecified"},
		Parameters: IntentParameters{"detection_method": "dynamic", "vulnerability_id": "CVE-2026-31431"},
		WorkflowIDs: []string{
			detectionPackageLifecycleWorkflowID,
		},
		CandidateCapabilities: []string{
			"generate_detection_package_draft",
			"start_detection_package_build",
			"get_detection_package",
			"sign_detection_package",
			"enable_detection_package",
		},
		RequiresWrite: true,
		RiskHint:      "medium",
		Confidence:    0.95,
	}

	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: query})

	if got, want := breakdown.Actions, []string{"generate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %#v, want canonical package action %#v", got, want)
	}
	if got, want := breakdown.CandidateCapabilities, []string{
		"generate_detection_package_draft",
		"start_detection_package_build",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %#v, want lifecycle boundary %#v", got, want)
	}
	if got := breakdown.Parameters["cve_id"]; got != "CVE-2026-31431" {
		t.Fatalf("cve_id = %#v, want captured CVE", got)
	}
	if got := breakdown.Parameters["vulnerability_description"]; got != query {
		t.Fatalf("vulnerability_description = %#v, want original query", got)
	}
}

func TestNormalizeDetectionPackageIntentDoesNotAskUserForGeneratedArtifacts(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:       "Create a dynamic detection package for CVE-2026-31431",
		Actions:    []string{"generate", "build", "sign", "enable"},
		Objects:    []IntentObject{{Type: "detection_package", Selector: "CVE-2026-31431"}},
		Scope:      IntentScope{Kind: "unspecified"},
		Parameters: IntentParameters{"cve_id": "CVE-2026-31431", "vulnerability_description": "AF_ALG, pipe and splice exploitation chain"},
		WorkflowIDs: []string{
			detectionPackageLifecycleWorkflowID,
		},
		CandidateCapabilities: []string{
			"generate_detection_package_draft",
			"start_detection_package_build",
		},
		MissingInfo: []MissingInfo{
			{Field: "eBPF hook plan for detecting AF_ALG socket creation"},
			{Field: "Sigma rules for CVE-2026-31431"},
			{Field: "Target deployment environment"},
		},
		NeedClarification:  true,
		ClarifyingQuestion: "请提供 eBPF hook 计划和 Sigma 规则",
		RequiresWrite:      true,
		Confidence:         0.8,
	}

	normalizeDetectionPackageIntent(breakdown, "通过动态检测包检测 CVE-2026-31431")

	if breakdown.NeedClarification {
		t.Fatalf("generated package artifacts must not block execution: %#v", breakdown)
	}
	if breakdown.ClarifyingQuestion != "" {
		t.Fatalf("clarifying_question = %q, want empty", breakdown.ClarifyingQuestion)
	}
	if len(breakdown.MissingInfo) != 0 {
		t.Fatalf("generated artifact missing_info was not removed: %#v", breakdown.MissingInfo)
	}
}

func TestNormalizeDetectionPackageIntentPreservesEarlierScanCapabilities(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:          "先进行漏洞扫描，再为 CVE-2026-31431 生成动态检测包",
		Actions:       []string{"scan", "generate", "detect"},
		Objects:       []IntentObject{{Type: "cve", ID: "CVE-2026-31431"}, {Type: "detection_package"}},
		Parameters:    IntentParameters{"cve_id": "CVE-2026-31431"},
		RequiresWrite: true,
		WorkflowIDs: []string{
			vulnerabilityAssessmentWorkflowID,
			detectionPackageLifecycleWorkflowID,
		},
		CandidateCapabilities: []string{
			"resolve_hosts",
			"start_vulnerability_scan",
			"get_vulnerability_scan_status",
			"generate_detection_package_draft",
			"start_detection_package_build",
			"enable_detection_package",
		},
	}

	normalizeDetectionPackageIntent(breakdown, breakdown.Goal)

	for _, capability := range []string{
		"resolve_hosts",
		"start_vulnerability_scan",
		"get_vulnerability_scan_status",
		"generate_detection_package_draft",
		"start_detection_package_build",
	} {
		if !containsExactString(breakdown.CandidateCapabilities, capability) {
			t.Fatalf("required capability %q was dropped: %#v", capability, breakdown.CandidateCapabilities)
		}
	}
	for _, capability := range []string{"enable_detection_package"} {
		if containsExactString(breakdown.CandidateCapabilities, capability) {
			t.Fatalf("future package stage %q must be deferred: %#v", capability, breakdown.CandidateCapabilities)
		}
	}
}

func TestNormalizeDetectionPackageGenerateAndBuildIncludesBuildCapability(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:          "Generate and build a dynamic detection package for CVE-2026-31431",
		Actions:       []string{"generate", "build"},
		Objects:       []IntentObject{{Type: "cve", ID: "CVE-2026-31431"}, {Type: "detection_package"}},
		Parameters:    IntentParameters{"cve_id": "CVE-2026-31431"},
		RequiresWrite: true,
		WorkflowIDs:   []string{detectionPackageLifecycleWorkflowID},
		CandidateCapabilities: []string{
			"generate_detection_package_draft",
		},
	}

	normalizeDetectionPackageIntent(breakdown, "通过动态检测包方式进行检测")

	if got, want := breakdown.CandidateCapabilities, []string{
		"generate_detection_package_draft",
		"start_detection_package_build",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("generate+build capabilities = %#v, want %#v", got, want)
	}
}

func TestNormalizeIntentBreakdownPreservesExplicitExistingPackageBuildStage(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:       "Build existing dynamic detection package",
		Actions:    []string{"build"},
		Objects:    []IntentObject{{Type: "detection_package", ID: "pkg-cve-2026-31431"}},
		Parameters: IntentParameters{"package_id": "pkg-cve-2026-31431", "detection_method": "dynamic_detection_package"},
		WorkflowIDs: []string{
			detectionPackageLifecycleWorkflowID,
		},
		CandidateCapabilities: []string{
			"generate_detection_package_draft",
			"start_detection_package_build",
			"enable_detection_package",
		},
	}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: "构建检测包 pkg-cve-2026-31431"})
	if got, want := breakdown.CandidateCapabilities, []string{"start_detection_package_build"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit existing-package build stage changed: got %#v want %#v", got, want)
	}
}

func TestNormalizeIntentBreakdownCanonicalizesCapturedBaselineAliases(t *testing.T) {
	breakdown := &IntentBreakdown{
		Goal:                  "run baseline checks and repair every live machine",
		Domains:               []string{"security_baseline"},
		Actions:               []string{"apply_baseline", "enable_auto_repair"},
		Objects:               []IntentObject{{Type: "machine", Selector: "live"}, {Type: "baseline", ID: "CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf"}},
		Scope:                 IntentScope{Kind: "unspecified"},
		Parameters:            IntentParameters{"auto_repair": true, "repair_rounds": float64(5)},
		WorkflowIDs:           []string{"baseline_compliance"},
		CandidateCapabilities: []string{"resolve_hosts", "run_baseline_compliance"},
		RequiresWrite:         true,
		RiskHint:              "critical",
		Confidence:            0.95,
	}

	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: "给存活的机器下发基线，开启自动修复5轮"})

	if breakdown.Scope.Kind != "all_online_hosts" {
		t.Fatalf("scope.kind = %q, want all_online_hosts", breakdown.Scope.Kind)
	}
	if len(breakdown.Objects) != 2 || breakdown.Objects[0].Type != "host" || breakdown.Objects[1].Type != "baseline_template" {
		t.Fatalf("objects were not canonicalized: %#v", breakdown.Objects)
	}
	if breakdown.Parameters["auto_remediate"] != true || breakdown.Parameters["remediation_rounds"] != float64(5) {
		t.Fatalf("parameters were not canonicalized: %#v", breakdown.Parameters)
	}
}

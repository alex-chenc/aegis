package assistant

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestVulnerabilityAssessmentCompilerResolvesIPBeforeStartingScan(t *testing.T) {
	registry := newWorkflowCompilerTestRegistry(t)
	mapper := NewToolCapabilityMapper(registry)
	breakdown := &IntentBreakdown{
		Goal:          "更新主机192.168.152.159 的漏洞",
		Domains:       []string{"asset_management", "vulnerability_management"},
		Actions:       []string{"update", "scan"},
		Objects:       []IntentObject{{Type: "host", ID: "192.168.152.159", Selector: "ip_address"}},
		Scope:         IntentScope{Kind: "unspecified"},
		RequiresWrite: true,
		CandidateCapabilities: []string{
			"resolve_hosts",
			"start_vulnerability_scan",
		},
		WorkflowIDs: []string{vulnerabilityAssessmentWorkflowID},
	}

	result, compiled, err := NewWorkflowPlanCompilerRegistry().CompileForBreakdown(WorkflowCompileInput{
		Breakdown:     breakdown,
		AcceptedTools: []string{"Host.Resolve", "Vulnerability.Scan.Start", "Host.List", "Vulnerability.Scan.Status"},
		Registry:      registry,
		Mapper:        mapper,
	})
	if err != nil {
		t.Fatalf("CompileForBreakdown returned error: %v", err)
	}
	if !compiled {
		t.Fatal("expected vulnerability_assessment workflow to use a registered compiler")
	}
	if result == nil || result.Clarification != "" {
		t.Fatalf("expected executable plan, got %#v", result)
	}

	steps := assignPlanStepIDs(result.Steps)
	if got, want := toolNamesFromSteps(steps), []string{"Host.Resolve", "Vulnerability.Scan.Start", "Vulnerability.Scan.Status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected compiled tools: got %v want %v", got, want)
	}
	if got := steps[0].Args["host_selectors"]; !reflect.DeepEqual(got, []string{"192.168.152.159"}) {
		t.Fatalf("Host.Resolve selectors mismatch: %#v", got)
	}
	if got := steps[0].Args["require_online"]; got != true {
		t.Fatalf("Host.Resolve require_online mismatch: %#v", got)
	}
	if _, exists := steps[1].Args["host_ids"]; exists {
		t.Fatalf("scan compiler must not bind the IP as host_ids: %#v", steps[1].Args)
	}
	if source := steps[1].ArgSources["host_ids"]; source.SourceType != "previous_step" {
		t.Fatalf("scan host_ids must come from Host.Resolve facts: %#v", source)
	}
	if source := steps[2].ArgSources["scan_id"]; source.SourceType != "previous_step" {
		t.Fatalf("status scan_id must come from scan start result: %#v", source)
	}

	plan := &ToolExecutionPlan{Goal: breakdown.Goal, Steps: steps}
	if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
		t.Fatalf("compiled vulnerability plan failed validation: %v", err)
	}
}

func TestVulnerabilityRemediationCompilerDoesNotStartScanForExactCVEPOC(t *testing.T) {
	registry := newWorkflowCompilerTestRegistry(t)
	mapper := NewToolCapabilityMapper(registry)
	breakdown := &IntentBreakdown{
		Goal:          "针对主机192.168.152.159进行 POC 验证CVE-2023-29484 此漏洞，若是存在，则修复此漏洞",
		Domains:       []string{"vulnerability_management"},
		Actions:       []string{"poc_verification", "remediation"},
		Objects:       []IntentObject{{Type: "host", ID: "192.168.152.159", Selector: "ip_address"}, {Type: "vulnerability", ID: "CVE-2023-29484"}},
		Scope:         IntentScope{Kind: "specific", ObjectIDs: []string{"192.168.152.159", "CVE-2023-29484"}},
		Parameters:    IntentParameters{"cve_id": "CVE-2023-29484"},
		RequiresWrite: true,
		CandidateCapabilities: []string{
			"resolve_hosts",
			"generate_vulnerability_script",
			"get_vulnerability_script_status",
			"execute_vulnerability_host_scripts",
			"start_vulnerability_scan",
			"get_vulnerability_scan_status",
		},
		WorkflowIDs: []string{
			"cve_lookup",
			vulnerabilityAssessmentWorkflowID,
			vulnerabilityRemediationWorkflowID,
		},
	}

	result, compiled, err := NewWorkflowPlanCompilerRegistry().CompileForBreakdown(WorkflowCompileInput{
		Breakdown: breakdown,
		AcceptedTools: []string{
			"Host.Resolve",
			"Vulnerability.Script.Generate",
			"Vulnerability.Script.Status",
			"Vulnerability.Script.Execute",
			"Vulnerability.Scan.Start",
			"Vulnerability.Scan.Status",
		},
		Registry: registry,
		Mapper:   mapper,
	})
	if err != nil {
		t.Fatalf("CompileForBreakdown returned error: %v", err)
	}
	if !compiled {
		t.Fatal("expected vulnerability_remediation workflow to use a registered compiler")
	}
	if result == nil || result.Clarification != "" {
		t.Fatalf("expected executable plan, got %#v", result)
	}

	steps := assignPlanStepIDs(result.Steps)
	if got, want := toolNamesFromSteps(steps), []string{
		"Host.Resolve",
		"Vulnerability.Script.Generate",
		"Vulnerability.Script.Status",
		"Vulnerability.Script.Execute",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected compiled tools: got %v want %v", got, want)
	}
	for _, step := range steps {
		if step.ToolName == "Vulnerability.Scan.Start" || step.ToolName == "Vulnerability.Scan.Status" {
			t.Fatalf("POC/remediation plan must not contain vulnerability scan tools: %#v", steps)
		}
	}
	if got, want := steps[0].Args["host_selectors"], []string{"192.168.152.159"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Host.Resolve selectors: got %#v want %#v", got, want)
	}
	if got := steps[1].Args["cve_id"]; got != "CVE-2023-29484" {
		t.Fatalf("generate cve_id mismatch: %#v", got)
	}
	if got := steps[1].Args["script_type"]; got != "poc" {
		t.Fatalf("generate script_type mismatch: %#v", got)
	}
	if got := steps[2].Args["script_type"]; got != "poc" {
		t.Fatalf("status script_type mismatch: %#v", got)
	}
	if got := steps[3].Args["auto_verify"]; got != true {
		t.Fatalf("execute auto_verify mismatch: %#v", got)
	}
	if _, exists := steps[3].Args["host_ids"]; exists {
		t.Fatalf("execute step must not bind the IP as host_ids: %#v", steps[3].Args)
	}
	if source := steps[3].ArgSources["host_ids"]; source.SourceType != "previous_step" {
		t.Fatalf("execute host_ids must come from Host.Resolve facts: %#v", source)
	}

	plan := &ToolExecutionPlan{Goal: breakdown.Goal, Steps: steps}
	if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
		t.Fatalf("compiled vulnerability remediation plan failed validation: %v", err)
	}

	descriptors := (&Orchestrator{toolRegistry: registry}).buildAgentToolDescriptors(plan.ToolNames())
	runtimePlan := runtimePlanFromToolExecutionPlanWithDescriptors(plan, descriptors)
	if runtimePlan == nil || len(runtimePlan.Steps) != 3 {
		t.Fatalf("expected resolve, generate/status, and execute runtime steps, got %#v", runtimePlan)
	}
	if got, want := runtimePlan.Steps[1].AllowedTools, []string{"Vulnerability.Script.Generate", "Vulnerability.Script.Status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("script generation runtime tools: got %v want %v", got, want)
	}
	if got, want := runtimePlan.Steps[2].AllowedTools, []string{"Vulnerability.Script.Execute"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("script execution runtime tools: got %v want %v", got, want)
	}
}

type orderedWorkflowCompiler struct {
	workflowID    string
	toolName      string
	clarification string
}

func (c orderedWorkflowCompiler) WorkflowID() string { return c.workflowID }

func (c orderedWorkflowCompiler) Compile(WorkflowCompileInput) (*WorkflowCompileResult, error) {
	if c.clarification != "" {
		return &WorkflowCompileResult{Clarification: c.clarification}, nil
	}
	return &WorkflowCompileResult{Steps: []ToolPlanStep{{
		ToolName:   c.toolName,
		Capability: strings.ToLower(c.toolName),
	}}}, nil
}

func TestWorkflowCompilerRegistryDefersLaterClarificationUntilReadyStepsComplete(t *testing.T) {
	registry := &WorkflowPlanCompilerRegistry{compilers: make(map[string]WorkflowPlanCompiler)}
	registry.Register(orderedWorkflowCompiler{workflowID: vulnerabilityAssessmentWorkflowID, toolName: "Vulnerability.Scan.Start"})
	registry.Register(orderedWorkflowCompiler{
		workflowID:    detectionPackageLifecycleWorkflowID,
		clarification: "请选择扫描结果中的 CVE",
	})

	result, compiled, err := registry.CompileForBreakdown(WorkflowCompileInput{
		Breakdown: &IntentBreakdown{WorkflowIDs: []string{
			vulnerabilityAssessmentWorkflowID,
			detectionPackageLifecycleWorkflowID,
		}},
	})
	if err != nil || !compiled || result == nil {
		t.Fatalf("expected deferred composed plan, result=%#v compiled=%v err=%v", result, compiled, err)
	}
	if got := toolNamesFromSteps(result.Steps); !reflect.DeepEqual(got, []string{"Vulnerability.Scan.Start"}) {
		t.Fatalf("ready workflow steps = %v", got)
	}
	if result.DeferredClarification == "" {
		t.Fatal("later workflow clarification must be deferred until ready steps finish")
	}
	if got, want := result.RemainingWorkflowIDs, []string{detectionPackageLifecycleWorkflowID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining workflows = %v, want %v", got, want)
	}
}

func TestWorkflowCompilerRegistryComposesSelectedWorkflowsInRequestOrder(t *testing.T) {
	registry := &WorkflowPlanCompilerRegistry{compilers: make(map[string]WorkflowPlanCompiler)}
	registry.Register(orderedWorkflowCompiler{workflowID: vulnerabilityAssessmentWorkflowID, toolName: "Vulnerability.Scan.Start"})
	registry.Register(orderedWorkflowCompiler{workflowID: detectionPackageLifecycleWorkflowID, toolName: "Package.Draft.Generate"})

	result, compiled, err := registry.CompileForBreakdown(WorkflowCompileInput{
		Breakdown: &IntentBreakdown{WorkflowIDs: []string{
			vulnerabilityAssessmentWorkflowID,
			detectionPackageLifecycleWorkflowID,
		}},
	})
	if err != nil {
		t.Fatalf("CompileForBreakdown returned error: %v", err)
	}
	if !compiled || result == nil {
		t.Fatalf("expected a composed plan, compiled=%v result=%#v", compiled, result)
	}
	if got, want := toolNamesFromSteps(result.Steps), []string{"Vulnerability.Scan.Start", "Package.Draft.Generate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("composed workflow order = %v, want %v", got, want)
	}
}

func TestVulnerabilityRemediationCompilerDoesNotEnableFixForPOCOnly(t *testing.T) {
	registry := newWorkflowCompilerTestRegistry(t)
	mapper := NewToolCapabilityMapper(registry)
	breakdown := &IntentBreakdown{
		Goal:       "验证 CVE-2023-29484",
		Actions:    []string{"poc_verification"},
		Objects:    []IntentObject{{Type: "host", ID: "cf18f7f7-5b45-46e2-9889-160dddc4ee30"}, {Type: "vulnerability", ID: "CVE-2023-29484"}},
		Parameters: IntentParameters{"cve_id": "CVE-2023-29484"},
		WorkflowIDs: []string{
			vulnerabilityAssessmentWorkflowID,
			vulnerabilityRemediationWorkflowID,
		},
	}
	result, compiled, err := NewWorkflowPlanCompilerRegistry().CompileForBreakdown(WorkflowCompileInput{
		Breakdown: breakdown,
		AcceptedTools: []string{
			"Vulnerability.Script.Generate",
			"Vulnerability.Script.Status",
			"Vulnerability.Script.Execute",
			"Vulnerability.Scan.Start",
			"Vulnerability.Scan.Status",
		},
		Registry: registry,
		Mapper:   mapper,
	})
	if err != nil || !compiled || result == nil {
		t.Fatalf("expected compiled POC-only plan, result=%#v compiled=%v err=%v", result, compiled, err)
	}
	steps := result.Steps
	execute := steps[len(steps)-1]
	if got := execute.Args["auto_verify"]; got != false {
		t.Fatalf("POC-only request must not enable remediation: %#v", got)
	}
}

func TestDetectionPackageLifecycleCompilerBuildsGeneratedPackageBeforeReviewBoundary(t *testing.T) {
	registry := newDetectionPackageCompilerTestRegistry(t)
	breakdown := &IntentBreakdown{
		Goal:          "Detect exploitation of CVE-2026-31431 using a dynamic detection package",
		Domains:       []string{"vulnerability", "detection", "package"},
		Actions:       []string{"lookup", "generate", "build"},
		Objects:       []IntentObject{{Type: "cve", ID: "CVE-2026-31431"}, {Type: "detection_package", Selector: "dynamic detection package for CVE-2026-31431"}},
		Scope:         IntentScope{Kind: "unspecified"},
		Parameters:    IntentParameters{"cve_id": "CVE-2026-31431", "vulnerability_description": "AF_ALG, pipe and splice exploitation chain", "exploitation_chain": "AF_ALG -> pipe -> splice"},
		RequiresWrite: true,
		RiskHint:      "high",
		WorkflowIDs:   []string{"cve_lookup", detectionPackageLifecycleWorkflowID},
		CandidateCapabilities: []string{
			"list_detection_packages",
			"get_detection_package",
			"generate_detection_package_draft",
			"start_detection_package_build",
			"enable_detection_package",
		},
		Confidence: 0.9,
	}

	result, compiled, err := NewWorkflowPlanCompilerRegistry().CompileForBreakdown(WorkflowCompileInput{
		Breakdown: breakdown,
		AcceptedTools: []string{
			"Package.Enable",
			"Package.Get",
			"Package.List",
			"Package.Build.Start",
			"Package.Build.Status",
			"Package.Draft.Generate",
		},
		Registry: registry,
		Mapper:   NewToolCapabilityMapper(registry),
	})
	if err != nil {
		t.Fatalf("CompileForBreakdown returned error: %v", err)
	}
	if !compiled || result == nil || result.Clarification != "" {
		t.Fatalf("expected detection package workflow compiler, result=%#v compiled=%v", result, compiled)
	}
	steps := assignPlanStepIDs(result.Steps)
	if got, want := toolNamesFromSteps(steps), []string{"Package.Draft.Generate", "Package.Build.Start", "Package.Build.Status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("detection request must stop at the build review boundary: got %v want %v", got, want)
	}
	if got := steps[0].Args["cve_id"]; got != "CVE-2026-31431" {
		t.Fatalf("draft cve_id mismatch: %#v", got)
	}
	for _, step := range steps[1:] {
		if source := step.ArgSources["package_id"]; source.SourceType != "previous_step" {
			t.Fatalf("%s package_id must come from the generated package: %#v", step.ToolName, source)
		}
	}
	if err := NewCompiledPlanValidator(registry, nil).Validate(&ToolExecutionPlan{Goal: breakdown.Goal, Steps: steps}, nil); err != nil {
		t.Fatalf("compiled detection package draft plan failed validation: %v", err)
	}
}

func TestToolDecisionEngineCompilesCapturedDetectionPackageContract(t *testing.T) {
	registry := newDetectionPackageCompilerTestRegistry(t)
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

	engine := NewToolDecisionEngine(ToolDecisionEngineDeps{
		Registry: registry,
		Mapper:   NewToolCapabilityMapper(registry),
		Config: ToolDecisionConfig{
			Enabled:                      true,
			ClarificationRequiredWrite:   true,
			PostconditionCheckEnabled:    true,
			AssetWorkflowCompilerEnabled: true,
		},
	})
	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     breakdown.Goal,
		Intent:    IntentResult{Action: "generate", NeedWrite: true, Confidence: 0.95},
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if plan == nil || plan.NeedClarification {
		t.Fatalf("expected executable detection package plan: %#v", plan)
	}
	if got, want := plan.ToolNames(), []string{
		"Package.Draft.Generate",
		"Package.Build.Start",
		"Package.Build.Status",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("captured detection package plan = %v, want %v", got, want)
	}
	if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
		t.Fatalf("captured detection package plan failed validation: %v", err)
	}
}

func TestDetectionPackageLifecycleCompilerOrdersExplicitSignBeforeEnable(t *testing.T) {
	registry := newDetectionPackageCompilerTestRegistry(t)
	breakdown := &IntentBreakdown{
		Goal:                  "Sign and enable package pkg-cve-2026-31431",
		Actions:               []string{"sign", "enable"},
		Objects:               []IntentObject{{Type: "detection_package", ID: "pkg-cve-2026-31431"}},
		Scope:                 IntentScope{Kind: "specific", ObjectIDs: []string{"pkg-cve-2026-31431"}},
		Parameters:            IntentParameters{"package_id": "pkg-cve-2026-31431"},
		RequiresWrite:         true,
		WorkflowIDs:           []string{detectionPackageLifecycleWorkflowID},
		CandidateCapabilities: []string{"enable_detection_package", "sign_detection_package"},
		Confidence:            0.9,
	}
	result, compiled, err := NewWorkflowPlanCompilerRegistry().CompileForBreakdown(WorkflowCompileInput{
		Breakdown:     breakdown,
		AcceptedTools: []string{"Package.Enable", "Package.Sign"},
		Registry:      registry,
		Mapper:        NewToolCapabilityMapper(registry),
	})
	if err != nil || !compiled || result == nil {
		t.Fatalf("expected compiled sign/enable plan, result=%#v compiled=%v err=%v", result, compiled, err)
	}
	steps := assignPlanStepIDs(result.Steps)
	if got, want := toolNamesFromSteps(steps), []string{"Package.Sign", "Package.Enable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sign/enable order mismatch: got %v want %v", got, want)
	}
	for _, step := range steps {
		if got := step.Args["package_id"]; got != "pkg-cve-2026-31431" {
			t.Fatalf("%s package_id mismatch: %#v", step.ToolName, got)
		}
	}
	if err := NewCompiledPlanValidator(registry, nil).Validate(&ToolExecutionPlan{Goal: breakdown.Goal, Steps: steps}, nil); err != nil {
		t.Fatalf("compiled sign/enable plan failed validation: %v", err)
	}
}

func TestDetectionPackageLifecycleCompilerBuildStageWaitsForTerminalStatus(t *testing.T) {
	registry := newDetectionPackageCompilerTestRegistry(t)
	breakdown := &IntentBreakdown{
		Goal:                  "Build package pkg-cve-2026-31431",
		Actions:               []string{"build"},
		Objects:               []IntentObject{{Type: "detection_package", ID: "pkg-cve-2026-31431"}},
		Scope:                 IntentScope{Kind: "specific", ObjectIDs: []string{"pkg-cve-2026-31431"}},
		Parameters:            IntentParameters{"package_id": "pkg-cve-2026-31431"},
		RequiresWrite:         true,
		WorkflowIDs:           []string{detectionPackageLifecycleWorkflowID},
		CandidateCapabilities: []string{"start_detection_package_build"},
		Confidence:            0.9,
	}
	result, compiled, err := NewWorkflowPlanCompilerRegistry().CompileForBreakdown(WorkflowCompileInput{
		Breakdown:     breakdown,
		AcceptedTools: []string{"Package.Build.Start", "Package.Build.Status"},
		Registry:      registry,
		Mapper:        NewToolCapabilityMapper(registry),
	})
	if err != nil || !compiled || result == nil {
		t.Fatalf("expected compiled build plan, result=%#v compiled=%v err=%v", result, compiled, err)
	}
	steps := assignPlanStepIDs(result.Steps)
	if got, want := toolNamesFromSteps(steps), []string{"Package.Build.Start", "Package.Build.Status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("build stage must include terminal status: got %v want %v", got, want)
	}
	if err := NewCompiledPlanValidator(registry, nil).Validate(&ToolExecutionPlan{Goal: breakdown.Goal, Steps: steps}, nil); err != nil {
		t.Fatalf("compiled package build plan failed validation: %v", err)
	}
}

func TestToolDecisionEngineAddsDetectionPackageBuildStatusCompanion(t *testing.T) {
	registry := newDetectionPackageCompilerTestRegistry(t)
	breakdown := &IntentBreakdown{
		Goal:                  "Build package pkg-cve-2026-31431",
		Actions:               []string{"build"},
		Objects:               []IntentObject{{Type: "detection_package", ID: "pkg-cve-2026-31431"}},
		Scope:                 IntentScope{Kind: "specific", ObjectIDs: []string{"pkg-cve-2026-31431"}},
		Parameters:            IntentParameters{"package_id": "pkg-cve-2026-31431"},
		RequiresWrite:         true,
		WorkflowIDs:           []string{detectionPackageLifecycleWorkflowID},
		CandidateCapabilities: []string{"start_detection_package_build"},
		Confidence:            0.9,
	}
	engine := NewToolDecisionEngine(ToolDecisionEngineDeps{
		Registry: registry,
		Mapper:   NewToolCapabilityMapper(registry),
		Config: ToolDecisionConfig{
			Enabled:                      true,
			ClarificationRequiredWrite:   true,
			PostconditionCheckEnabled:    true,
			AssetWorkflowCompilerEnabled: true,
		},
	})
	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     breakdown.Goal,
		Intent:    IntentResult{Action: "build", NeedWrite: true, Confidence: 0.9},
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if plan == nil || plan.NeedClarification {
		t.Fatalf("expected executable build plan: %#v", plan)
	}
	if got, want := plan.ToolNames(), []string{"Package.Build.Start", "Package.Build.Status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("build status companion was not mapped: got %v want %v", got, want)
	}
	if err := NewCompiledPlanValidator(registry, nil).Validate(plan, nil); err != nil {
		t.Fatalf("authorized package build plan failed validation: %v", err)
	}
}

func TestToolDecisionEngineCompletesLatestVulnerabilityRemediationPlan(t *testing.T) {
	registry := newWorkflowCompilerTestRegistry(t)
	breakdown := &IntentBreakdown{
		Goal:          "针对主机192.168.152.159进行CVE-2023-29484的POC验证，如果存在漏洞则进行修复",
		Domains:       []string{"vulnerability"},
		Actions:       []string{"poc_verification", "remediation"},
		Objects:       []IntentObject{{Type: "host", ID: "192.168.152.159", Selector: "ip_address"}, {Type: "vulnerability", ID: "CVE-2023-29484", Selector: "cve_id"}},
		Scope:         IntentScope{Kind: "specific", ObjectIDs: []string{"192.168.152.159", "CVE-2023-29484"}},
		Parameters:    IntentParameters{"cve_id": "CVE-2023-29484", "host_ip": "192.168.152.159"},
		RequiresWrite: true,
		RiskHint:      "high",
		WorkflowIDs:   []string{vulnerabilityRemediationWorkflowID},
		CandidateCapabilities: []string{
			"resolve_hosts",
			"execute_vulnerability_host_scripts",
		},
		Confidence: 0.95,
	}
	normalizeIntentBreakdown(breakdown, IntentDecomposeInput{Query: breakdown.Goal})

	intent := IntentResult{
		Domains:     []string{"security", "vulnerability_management"},
		Action:      "execute",
		Operations:  []string{"verify", "remediate"},
		ObjectTypes: []string{"host", "cve"},
		NeedWrite:   true,
		RiskHint:    ToolRiskHigh,
		Confidence:  0.95,
	}
	orchestrator := &Orchestrator{toolRegistry: registry}
	workflowCards := NewWorkflowRegistry().Match(intent)
	catalog := orchestrator.buildCapabilityCatalog(intent, nil, workflowCards)
	if err := validateIntentBreakdownAgainstCatalog(breakdown, catalog, workflowCards); err != nil {
		t.Fatalf("latest remediation intent must remain valid against its capability catalog: %v", err)
	}

	engine := NewToolDecisionEngine(ToolDecisionEngineDeps{
		Registry: registry,
		Mapper:   NewToolCapabilityMapper(registry),
		Config: ToolDecisionConfig{
			Enabled:                      true,
			ClarificationRequiredWrite:   true,
			PostconditionCheckEnabled:    true,
			AssetWorkflowCompilerEnabled: true,
		},
	})
	plan, err := engine.Decide(context.Background(), ToolDecisionInput{
		Query:     breakdown.Goal,
		Intent:    intent,
		Breakdown: breakdown,
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if plan == nil || plan.NeedClarification {
		t.Fatalf("latest remediation intent must compile without clarification: %#v", plan)
	}
	if got, want := plan.ToolNames(), []string{
		"Host.Resolve",
		"Vulnerability.Script.Generate",
		"Vulnerability.Script.Status",
		"Vulnerability.Script.Execute",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected authorized plan: got %v want %v", got, want)
	}
	if got, want := plan.Steps[0].Args["host_selectors"], []string{"192.168.152.159"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Host.Resolve selectors: got %#v want %#v", got, want)
	}
}

func newWorkflowCompilerTestRegistry(t *testing.T) *ToolRegistry {
	t.Helper()
	registry := NewToolRegistry()
	register := func(spec *ToolSpec) {
		t.Helper()
		spec.Enabled = true
		spec.Handler = func(context.Context, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		}
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}
	register(&ToolSpec{
		Name:               "Host.Resolve",
		Domain:             DomainHost,
		Operation:          OpGet,
		Capability:         "resolve_hosts",
		Description:        "Resolve host selectors.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ResultContract: ToolResultContract{
			FactBindings: []ToolFactBinding{{Kind: "host_resolved", IDField: "host_id"}},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_selectors": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"target_scope": map[string]interface{}{
					"type": "string",
					"enum": []interface{}{"all_online_hosts"},
				},
				"require_online": map[string]interface{}{"type": "boolean"},
			},
			"additionalProperties": false,
		},
	})
	register(&ToolSpec{
		Name:               "Host.List",
		Domain:             DomainHost,
		Operation:          OpList,
		Capability:         "list_hosts",
		Description:        "List hosts.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
	})
	register(&ToolSpec{
		Name:               "Vulnerability.Scan.Start",
		Domain:             DomainVulnerability,
		Operation:          OpExecute,
		Capability:         "start_vulnerability_scan",
		Description:        "Start vulnerability scan.",
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: false,
		ExecutionContract: ToolExecutionContract{
			Mode:                 ToolExecutionAsynchronous,
			CompletionCapability: "get_vulnerability_scan_status",
		},
		ResultContract: ToolResultContract{OperationRefFields: []string{"scan_id"}},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_ids": map[string]interface{}{
					"type":     "array",
					"minItems": 1,
					"items":    map[string]interface{}{"type": "string", "format": "uuid"},
				},
			},
			"required":             []string{"host_ids"},
			"additionalProperties": false,
		},
	})
	register(&ToolSpec{
		Name:               "Vulnerability.Scan.Status",
		Domain:             DomainVulnerability,
		Operation:          OpGet,
		Capability:         "get_vulnerability_scan_status",
		Description:        "Get vulnerability scan status.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"scan_id": map[string]interface{}{"type": "string", "format": "uuid"},
			},
			"required":             []string{"scan_id"},
			"additionalProperties": false,
		},
	})
	register(&ToolSpec{
		Name:               "Vulnerability.Script.Generate",
		Domain:             DomainVulnerability,
		Operation:          OpGenerate,
		Capability:         "generate_vulnerability_script",
		Description:        "Generate a vulnerability script.",
		Risk:               ToolRiskMedium,
		DefaultWhitelisted: false,
		ExecutionContract: ToolExecutionContract{
			Mode:                 ToolExecutionAsynchronous,
			CompletionCapability: "get_vulnerability_script_status",
		},
		ResultContract: ToolResultContract{OperationRefFields: []string{"cve_id", "script_type"}},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cve_id":      map[string]interface{}{"type": "string"},
				"script_type": map[string]interface{}{"type": "string", "enum": []interface{}{"poc", "fix"}},
			},
			"required":             []string{"cve_id", "script_type"},
			"additionalProperties": false,
		},
	})
	register(&ToolSpec{
		Name:               "Vulnerability.Script.Status",
		Domain:             DomainVulnerability,
		Operation:          OpGet,
		Capability:         "get_vulnerability_script_status",
		Description:        "Get vulnerability script generation status.",
		Risk:               ToolRiskReadonly,
		DefaultWhitelisted: true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cve_id":      map[string]interface{}{"type": "string"},
				"script_type": map[string]interface{}{"type": "string", "enum": []interface{}{"poc", "fix"}},
			},
			"required":             []string{"cve_id", "script_type"},
			"additionalProperties": false,
		},
	})
	register(&ToolSpec{
		Name:               "Vulnerability.Script.Execute",
		Domain:             DomainVulnerability,
		Operation:          OpExecute,
		Capability:         "execute_vulnerability_host_scripts",
		Description:        "Execute a generated vulnerability script.",
		Risk:               ToolRiskHigh,
		DefaultWhitelisted: false,
		RequiresApproval:   true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cve_id":      map[string]interface{}{"type": "string"},
				"script_type": map[string]interface{}{"type": "string", "enum": []interface{}{"poc", "fix"}},
				"host_ids": map[string]interface{}{
					"type":     "array",
					"minItems": 1,
					"items":    map[string]interface{}{"type": "string", "format": "uuid"},
				},
				"auto_verify": map[string]interface{}{"type": "boolean"},
				"max_rounds":  map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 10},
			},
			"required":             []string{"cve_id", "script_type", "host_ids"},
			"additionalProperties": false,
		},
	})
	return registry
}

func newDetectionPackageCompilerTestRegistry(t *testing.T) *ToolRegistry {
	t.Helper()
	registry := NewToolRegistry()
	register := func(spec *ToolSpec) {
		t.Helper()
		spec.Enabled = true
		spec.Handler = func(context.Context, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		}
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}
	register(&ToolSpec{
		Name: "Package.Draft.Generate", Domain: DomainPackage, Operation: OpGenerate,
		Capability: "generate_detection_package_draft", Description: "Generate package draft.", Risk: ToolRiskMedium,
		ResultContract: ToolResultContract{OperationRefFields: []string{"package_id"}},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cve_id":                    map[string]interface{}{"type": "string"},
				"vulnerability_description": map[string]interface{}{"type": "string"},
				"exploitation_chain":        map[string]interface{}{"type": "string"},
			},
			"required":             []string{"cve_id", "vulnerability_description"},
			"additionalProperties": false,
		},
	})
	for _, spec := range []*ToolSpec{
		{Name: "Package.List", Domain: DomainPackage, Operation: OpList, Capability: "list_detection_packages", Description: "List packages.", Risk: ToolRiskReadonly},
		{Name: "Package.Get", Domain: DomainPackage, Operation: OpGet, Capability: "get_detection_package", Description: "Get package.", Risk: ToolRiskReadonly},
		{
			Name: "Package.Build.Start", Domain: DomainPackage, Operation: OpExecute, Capability: "start_detection_package_build", Description: "Build package.", Risk: ToolRiskMedium,
			ExecutionContract: ToolExecutionContract{Mode: ToolExecutionAsynchronous, CompletionCapability: "get_detection_package_build_status"},
			ResultContract:    ToolResultContract{OperationRefFields: []string{"id", "package_id"}},
		},
		{Name: "Package.Build.Status", Domain: DomainPackage, Operation: OpGet, Capability: "get_detection_package_build_status", Description: "Get build status.", Risk: ToolRiskReadonly, DefaultWhitelisted: true},
		{Name: "Package.Sign", Domain: DomainPackage, Operation: OpApprove, Capability: "sign_detection_package", Description: "Sign package.", Risk: ToolRiskCritical},
		{Name: "Package.Enable", Domain: DomainPackage, Operation: OpDispatch, Capability: "enable_detection_package", Description: "Enable package.", Risk: ToolRiskCritical},
	} {
		if spec.Name != "Package.List" {
			spec.ArgsSchema = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"package_id": map[string]interface{}{"type": "string"}},
				"required":   []string{"package_id"},
			}
		}
		register(spec)
	}
	return registry
}

func toolNamesFromSteps(steps []ToolPlanStep) []string {
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		names = append(names, step.ToolName)
	}
	return names
}

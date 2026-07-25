package assistant

import (
	"context"
	"testing"
)

func exposureTestTool(name string, domain ToolDomain, exposure ToolExposure) *ToolSpec {
	return &ToolSpec{
		Name:        name,
		Domain:      domain,
		Operation:   OpGet,
		Capability:  "get_" + normalizeExposureIdentifier(name),
		Description: "Get test data.",
		Risk:        ToolRiskReadonly,
		Enabled:     true,
		ExposurePolicy: ToolExposurePolicy{
			Exposure:       exposure,
			Discoverable:   exposure == ToolExposurePrimary || exposure == ToolExposureContextual,
			DirectCallable: exposure == ToolExposurePrimary || exposure == ToolExposureContextual,
		},
		ArgsSchema: map[string]interface{}{"type": "object"},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return nil, nil
		},
	}
}

func TestToolExposureResolverFiltersIntentCatalog(t *testing.T) {
	registry := NewToolRegistry()
	for _, tool := range []*ToolSpec{
		exposureTestTool("Workflow.Run", DomainSystem, ToolExposurePrimary),
		exposureTestTool("Host.List", DomainHost, ToolExposureContextual),
		exposureTestTool("Detection.Alert.List", DomainDetection, ToolExposureContextual),
		exposureTestTool("Task.Status", DomainTask, ToolExposureCompanion),
		exposureTestTool("Task.Run", DomainTask, ToolExposureInternal),
	} {
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	resolver := NewToolExposureResolver(registry)
	visible := resolver.IntentCatalog(ToolExposureContext{Domains: []string{"host"}})
	got := make(map[string]bool, len(visible))
	for _, tool := range visible {
		got[tool.Name] = true
	}

	for _, name := range []string{"Workflow.Run", "Host.List"} {
		if !got[name] {
			t.Fatalf("expected %s in intent catalog, got %#v", name, got)
		}
	}
	for _, name := range []string{"Detection.Alert.List", "Task.Status", "Task.Run"} {
		if got[name] {
			t.Fatalf("did not expect %s in intent catalog, got %#v", name, got)
		}
	}
}

func TestToolExposureResolverIncludesOnlyDeclaredLowRiskCompanions(t *testing.T) {
	registry := NewToolRegistry()
	primary := exposureTestTool("Workflow.Run", DomainSystem, ToolExposurePrimary)
	primary.Operation = OpExecute
	primary.Risk = ToolRiskMedium
	primary.ExecutionContract = ToolExecutionContract{
		Mode:                 ToolExecutionAsynchronous,
		CompletionCapability: "get_task_status",
		DiscoveryCapabilities: []string{
			"get_task_lookup",
			"execute_unsafe_helper",
		},
	}
	status := exposureTestTool("Task.Status", DomainTask, ToolExposureCompanion)
	status.Capability = "get_task_status"
	lookup := exposureTestTool("Task.Lookup", DomainTask, ToolExposureCompanion)
	lookup.Capability = "get_task_lookup"
	unsafe := exposureTestTool("Task.Unsafe", DomainTask, ToolExposureCompanion)
	unsafe.Capability = "execute_unsafe_helper"
	unsafe.Operation = OpExecute
	unsafe.Risk = ToolRiskHigh

	for _, tool := range []*ToolSpec{primary, status, lookup, unsafe} {
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	visible := NewToolExposureResolver(registry).IntentCatalog(ToolExposureContext{})
	got := make(map[string]bool, len(visible))
	for _, tool := range visible {
		got[tool.Name] = true
	}
	for _, name := range []string{"Workflow.Run", "Task.Status", "Task.Lookup"} {
		if !got[name] {
			t.Fatalf("declared catalog tool %s missing from %#v", name, got)
		}
	}
	if got["Task.Unsafe"] {
		t.Fatalf("high-risk companion leaked into intent catalog: %#v", got)
	}
}

func TestToolExposureResolverIncludesVulnerabilityRemediationCompilerTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, tool := range []*ToolSpec{
		{
			Name:        "Vulnerability.Script.Generate",
			Domain:      DomainVulnerability,
			Operation:   OpGenerate,
			Capability:  "generate_vulnerability_script",
			Description: "Generate a POC or remediation script.",
			ObjectTypes: []string{"vulnerability"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Vulnerability.Script.Execute",
			Domain:      DomainVulnerability,
			Operation:   OpExecute,
			Capability:  "execute_vulnerability_host_scripts",
			Description: "Execute a generated POC or remediation script.",
			ObjectTypes: []string{"vulnerability", "host"},
			Risk:        ToolRiskHigh,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
	} {
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	visible := NewToolExposureResolver(registry).IntentCatalog(ToolExposureContext{
		Domains:     []string{"security", "vulnerability_management"},
		ObjectTypes: []string{"host", "cve"},
		WorkflowIDs: []string{vulnerabilityRemediationWorkflowID},
	})
	got := make(map[string]bool, len(visible))
	for _, tool := range visible {
		got[tool.Name] = true
	}
	for _, name := range []string{"Vulnerability.Script.Generate", "Vulnerability.Script.Execute"} {
		if !got[name] {
			t.Fatalf("workflow compiler tool %s missing from remediation capability catalog: %#v", name, got)
		}
	}
}

func TestToolExposureResolverIncludesDetectionPackageLifecycleTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, tool := range []*ToolSpec{
		{
			Name:        "Package.Draft.Generate",
			Domain:      DomainPackage,
			Operation:   OpGenerate,
			Capability:  "generate_detection_package_draft",
			Description: "Generate a dynamic detection package draft.",
			ObjectTypes: []string{"detection_package", "package"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
		{
			Name:        "Package.Build.Start",
			Domain:      DomainPackage,
			Operation:   OpExecute,
			Capability:  "start_detection_package_build",
			Description: "Start a dynamic detection package build.",
			ObjectTypes: []string{"detection_package", "package"},
			Risk:        ToolRiskMedium,
			Enabled:     true,
			ArgsSchema:  map[string]interface{}{"type": "object"},
			Handler:     func(context.Context, map[string]interface{}) (interface{}, error) { return nil, nil },
		},
	} {
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	visible := NewToolExposureResolver(registry).IntentCatalog(ToolExposureContext{
		Domains:     []string{"cybersecurity", "vulnerability_detection"},
		ObjectTypes: []string{"cve", "exploit", "dynamic_detection"},
		WorkflowIDs: []string{detectionPackageLifecycleWorkflowID},
	})
	got := make(map[string]bool, len(visible))
	for _, tool := range visible {
		got[tool.Name] = true
	}
	for _, name := range []string{"Package.Draft.Generate", "Package.Build.Start"} {
		if !got[name] {
			t.Fatalf("detection package workflow tool %s missing from capability catalog: %#v", name, got)
		}
	}
}

func TestToolCatalogSearchHidesCompanionAndInternalTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, tool := range []*ToolSpec{
		exposureTestTool("Workflow.Run", DomainSystem, ToolExposurePrimary),
		exposureTestTool("Host.List", DomainHost, ToolExposureContextual),
		exposureTestTool("Task.Status", DomainTask, ToolExposureCompanion),
		exposureTestTool("Task.Run", DomainTask, ToolExposureInternal),
	} {
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	results := NewToolCatalog(registry).Search("", SearchOptions{})
	if len(results) != 2 {
		t.Fatalf("search returned %d tools, want only primary and contextual tools: %#v", len(results), results)
	}
}

func TestToolRegistryRejectsNonEnglishDescriptions(t *testing.T) {
	tool := exposureTestTool("Host.List", DomainHost, ToolExposureContextual)
	tool.Description = "查询主机"
	if err := ValidateToolEnglishContract(tool); err == nil {
		t.Fatal("expected a non-English tool description to be rejected")
	}
}

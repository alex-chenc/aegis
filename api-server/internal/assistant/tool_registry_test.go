package assistant

import (
	"strings"
	"testing"
)

// TestValidateCapabilityUniquenessAcceptsDistinctCapabilities confirms the
// startup check passes when every tool resolves to a distinct capability.
func TestValidateCapabilityUniquenessAcceptsDistinctCapabilities(t *testing.T) {
	registry := NewToolRegistry()
	specs := []*ToolSpec{
		{Name: "A.List", Domain: DomainHost, Operation: OpList, Capability: "list_hosts", Risk: ToolRiskReadonly, DefaultWhitelisted: true, Enabled: true, Handler: noopToolHandler},
		{Name: "A.Get", Domain: DomainHost, Operation: OpGet, Capability: "get_host", Risk: ToolRiskReadonly, DefaultWhitelisted: true, Enabled: true, Handler: noopToolHandler},
		{Name: "A.Run", Domain: DomainBaseline, Operation: OpExecute, Capability: "run_baseline", Risk: ToolRiskHigh, DefaultWhitelisted: false, Enabled: true, Handler: noopToolHandler},
	}
	for _, spec := range specs {
		if err := registry.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.ValidateCapabilityUniqueness(); err != nil {
		t.Fatalf("expected no uniqueness error, got %v", err)
	}
}

// TestValidateCapabilityUniquenessRejectsExplicitDuplicate confirms the startup
// check catches two tools that explicitly declare the same capability.
func TestValidateCapabilityUniquenessRejectsExplicitDuplicate(t *testing.T) {
	registry := NewToolRegistry()
	// Register the first tool with an explicit capability.
	if err := registry.Register(&ToolSpec{
		Name: "First.Scan", Domain: DomainVulnerability, Operation: OpExecute,
		Capability: "start_scan", Risk: ToolRiskMedium, DefaultWhitelisted: true,
		Enabled: true, Handler: noopToolHandler,
	}); err != nil {
		t.Fatal(err)
	}
	// Bypass the per-Register guard by constructing the second tool with a
	// different name but the same capability, then inject it directly to
	// simulate a registration path that skipped the guard.
	second := &ToolSpec{
		Name: "Second.Scan", Domain: DomainVulnerability, Operation: OpExecute,
		Capability: "start_scan", Risk: ToolRiskMedium, DefaultWhitelisted: true,
		Enabled: true, Handler: noopToolHandler,
	}
	registry.mu.Lock()
	registry.tools[second.Name] = second
	registry.mu.Unlock()

	err := registry.ValidateCapabilityUniqueness()
	if err == nil {
		t.Fatalf("expected uniqueness error for duplicate explicit capability")
	}
	if !strings.Contains(err.Error(), "start_scan") {
		t.Fatalf("expected error to name the colliding capability, got %v", err)
	}
}

// TestValidateCapabilityUniquenessRejectsSyntheticDuplicate confirms the startup
// check catches two tools with an EMPTY Capability that synthesize the same
// capability from identical domain+operation. This is the gap the per-Register
// guard (which only compares the raw Capability field) misses.
func TestValidateCapabilityUniquenessRejectsSyntheticDuplicate(t *testing.T) {
	registry := NewToolRegistry()
	// Two tools with empty Capability but identical domain+operation. The
	// per-Register guard sees two empty strings and registers both; they
	// synthesize the same capability "execute_system" via BuildToolUseContract.
	if err := registry.Register(&ToolSpec{
		Name: "Legacy.Execute", Domain: DomainSystem, Operation: OpExecute,
		Risk: ToolRiskMedium, DefaultWhitelisted: true, Enabled: true, Handler: noopToolHandler,
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&ToolSpec{
		Name: "Other.Execute", Domain: DomainSystem, Operation: OpExecute,
		Risk: ToolRiskMedium, DefaultWhitelisted: true, Enabled: true, Handler: noopToolHandler,
	}); err != nil {
		t.Fatal(err)
	}
	err := registry.ValidateCapabilityUniqueness()
	if err == nil {
		t.Fatalf("expected uniqueness error for synthetic capability collision")
	}
	if !strings.Contains(err.Error(), "execute_system") {
		t.Fatalf("expected error to name the synthesized capability, got %v", err)
	}
}

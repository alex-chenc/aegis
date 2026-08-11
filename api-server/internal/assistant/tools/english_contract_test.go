package tools

import (
	"testing"

	"api-server/internal/assistant"
)

func TestAllBuiltInToolModelContractsAreEnglish(t *testing.T) {
	registry := assistant.NewToolRegistry()
	registrations := []struct {
		name string
		call func() error
	}{
		{"host", func() error { return RegisterHostTools(registry, HostToolDeps{}) }},
		{"agent", func() error { return RegisterAgentTools(registry, AgentToolDeps{}) }},
		{"asset", func() error { return RegisterAssetTools(registry, AssetToolDeps{}) }},
		{"audit", func() error { return RegisterAuditTools(registry, AuditToolDeps{}) }},
		{"baseline", func() error { return RegisterBaselineTools(registry, BaselineToolDeps{}) }},
		{"block", func() error { return RegisterBlockTools(registry, BlockToolDeps{}) }},
		{"config", func() error { return RegisterConfigTools(registry, ConfigToolDeps{}) }},
		{"detection", func() error { return RegisterDetectionTools(registry, DetectionToolDeps{}) }},
		{"detection_write", func() error { return RegisterDetectionWriteTools(registry, DetectionWriteToolDeps{}) }},
		{"external_mcp", func() error { return RegisterExternalMCPTools(registry, ExternalMCPToolDeps{}) }},
		{"investigation", func() error { return RegisterInvestigationTools(registry, InvestigationToolDeps{}) }},
		{"notification", func() error { return RegisterNotificationTools(registry, NotificationToolDeps{}) }},
		{"package", func() error { return RegisterPackageTools(registry, PackageToolDeps{}) }},
		{"package_write", func() error { return RegisterPackageWriteTools(registry, PackageWriteToolDeps{}) }},
		{"sigma", func() error { return RegisterSigmaRuleTools(registry, SigmaRuleToolDeps{}) }},
		{"task", func() error { return RegisterTaskTools(registry, TaskToolDeps{}) }},
		{"vulnerability", func() error { return RegisterVulnerabilityTools(registry, VulnerabilityToolDeps{}) }},
		{"weak_password", func() error { return RegisterWeakPasswordTools(registry, WeakPasswordToolDeps{}) }},
		{"agent_guard", func() error { return RegisterAgentGuardTools(registry, AgentGuardToolDeps{}) }},
	}
	for _, registration := range registrations {
		if err := registration.call(); err != nil {
			t.Fatalf("register %s tools: %v", registration.name, err)
		}
	}
	catalog := assistant.NewToolCatalog(registry)
	if err := RegisterSystemTools(registry, catalog, nil, nil); err != nil {
		t.Fatalf("register system tools: %v", err)
	}
	if err := registry.ValidateModelFacingEnglish(); err != nil {
		t.Fatal(err)
	}
}

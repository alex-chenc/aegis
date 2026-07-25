package assistant

import (
	"sort"
	"strings"
	"unicode"
)

// ToolExposureContext contains deterministic routing facts available before
// intent decomposition. It never authorizes a write operation.
type ToolExposureContext struct {
	Domains       []string
	ObjectTypes   []string
	AssistantMode string
	WorkflowIDs   []string
}

// ToolExposureResolver builds the small model-facing catalog for one request.
type ToolExposureResolver struct {
	registry *ToolRegistry
}

func NewToolExposureResolver(registry *ToolRegistry) *ToolExposureResolver {
	return &ToolExposureResolver{registry: registry}
}

func (r *ToolExposureResolver) IntentCatalog(ctx ToolExposureContext) []*ToolSpec {
	if r == nil || r.registry == nil {
		return nil
	}
	var result []*ToolSpec
	for _, tool := range r.registry.List() {
		if tool == nil || !tool.Enabled || isResidentTool(tool.Name) {
			continue
		}
		if !exposureModeAllows(tool.ExposurePolicy, ctx.AssistantMode) {
			continue
		}
		switch tool.ExposurePolicy.Exposure {
		case ToolExposurePrimary:
			if len(tool.ExposurePolicy.WorkflowIDs) == 0 || stringSliceIntersectsFold(ctx.WorkflowIDs, tool.ExposurePolicy.WorkflowIDs) {
				result = append(result, tool)
			}
		case ToolExposureContextual:
			if exposureContextMatches(tool, ctx) {
				result = append(result, tool)
			}
		}
	}
	result = r.withDeclaredLowRiskCompanions(result, ctx.AssistantMode)
	sortIntentCatalog(result)
	return result
}

// intentCatalogForCapabilities resolves a closed workflow capability contract
// directly. Contextual domain matching is deliberately bypassed here because a
// workflow may span tools owned by another domain (for example asset-scoped
// weak-password assessment tools implemented by the detection service).
func (r *ToolExposureResolver) intentCatalogForCapabilities(capabilities map[string]bool, assistantMode string) []*ToolSpec {
	if r == nil || r.registry == nil || len(capabilities) == 0 {
		return nil
	}
	var result []*ToolSpec
	for _, tool := range r.registry.List() {
		if !intentCatalogToolEligible(tool, assistantMode) ||
			tool.ExposurePolicy.Exposure == ToolExposureInternal {
			continue
		}
		capability := strings.ToLower(strings.TrimSpace(BuildToolUseContract(tool).Capability))
		if capabilities[capability] {
			result = append(result, tool)
		}
	}
	result = r.withDeclaredLowRiskCompanions(result, assistantMode)
	sortIntentCatalog(result)
	return result
}

// withDeclaredLowRiskCompanions computes the bounded dependency closure of the
// selected tools. Only completion/discovery capabilities explicitly declared
// by a tool contract and implemented by read-only or low-risk tools are added.
func (r *ToolExposureResolver) withDeclaredLowRiskCompanions(tools []*ToolSpec, assistantMode string) []*ToolSpec {
	if r == nil || r.registry == nil || len(tools) == 0 {
		return tools
	}
	result := append([]*ToolSpec{}, tools...)
	seen := make(map[string]bool, len(result))
	primaryNames := make([]string, 0, len(result))
	for _, tool := range result {
		if tool == nil {
			continue
		}
		seen[tool.Name] = true
		primaryNames = append(primaryNames, tool.Name)
	}
	for _, name := range NewToolCapabilityMapper(r.registry).ReadonlyCompanionToolNames(primaryNames) {
		if seen[name] {
			continue
		}
		tool, ok := r.registry.Get(name)
		if !ok || !intentCatalogToolEligible(tool, assistantMode) ||
			tool.ExposurePolicy.Exposure == ToolExposureInternal {
			continue
		}
		seen[name] = true
		result = append(result, tool)
	}
	return result
}

func intentCatalogToolEligible(tool *ToolSpec, assistantMode string) bool {
	return tool != nil &&
		tool.Enabled &&
		!isResidentTool(tool.Name) &&
		exposureModeAllows(tool.ExposurePolicy, assistantMode)
}

func sortIntentCatalog(result []*ToolSpec) {
	sort.Slice(result, func(i, j int) bool {
		if result[i].ExposurePolicy.CatalogPriority != result[j].ExposurePolicy.CatalogPriority {
			return result[i].ExposurePolicy.CatalogPriority > result[j].ExposurePolicy.CatalogPriority
		}
		return result[i].Name < result[j].Name
	})
}

func exposureContextMatches(tool *ToolSpec, ctx ToolExposureContext) bool {
	if tool == nil {
		return false
	}
	if stringSliceIntersectsFold(ctx.WorkflowIDs, tool.ExposurePolicy.WorkflowIDs) {
		return true
	}
	if stringSliceContainsFold(ctx.Domains, string(tool.Domain)) {
		return true
	}
	if stringSliceIntersectsFold(ctx.ObjectTypes, tool.ObjectTypes) {
		return true
	}
	return len(ctx.Domains) == 0 && len(ctx.ObjectTypes) == 0 && len(ctx.WorkflowIDs) == 0 && tool.Risk == ToolRiskReadonly
}

func exposureModeAllows(policy ToolExposurePolicy, mode string) bool {
	if len(policy.AssistantModes) == 0 || strings.TrimSpace(mode) == "" {
		return true
	}
	return stringSliceContainsFold(policy.AssistantModes, mode)
}

func stringSliceContainsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(wanted)) {
			return true
		}
	}
	return false
}

func stringSliceIntersectsFold(left, right []string) bool {
	for _, value := range left {
		if stringSliceContainsFold(right, value) {
			return true
		}
	}
	return false
}

func validToolExposure(value ToolExposure) bool {
	switch value {
	case ToolExposurePrimary, ToolExposureContextual, ToolExposureCompanion, ToolExposureInternal:
		return true
	default:
		return false
	}
}

func normalizeExposureIdentifier(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func builtinToolExposurePolicy(name string) ToolExposurePolicy {
	exposure := ToolExposureContextual
	var workflowIDs []string
	switch name {
	case "Context.Get", "Session.Summarize",
		"Asset.Collection.Get", "Task.GetDetail",
		"Vulnerability.CustomQuery.Status", "Vulnerability.Scan.Status", "Vulnerability.Script.Status",
		"Package.Build.Status",
		"Credential.WeakPassword.QueryProgress", "ExternalMCP.Source.GetSchema", "Operation.Get":
		exposure = ToolExposureCompanion
	case "Baseline.Template.Rules.List", "Baseline.Script.Generate", "Task.RunCheck", "Task.RunFix":
		exposure = ToolExposureInternal
	case "Host.Resolve", "Baseline.Compliance.Run":
		exposure = ToolExposurePrimary
	case "Vulnerability.Script.Generate", "Vulnerability.Script.Execute":
		workflowIDs = []string{vulnerabilityRemediationWorkflowID}
	case "Package.List", "Package.Get", "Package.Draft.Generate",
		"Package.Build.Start", "Package.Sign", "Package.Enable":
		workflowIDs = []string{detectionPackageLifecycleWorkflowID}
	case "":
		exposure = ToolExposureInternal
	}
	return ToolExposurePolicy{
		Exposure:       exposure,
		WorkflowIDs:    workflowIDs,
		Discoverable:   exposure == ToolExposurePrimary || exposure == ToolExposureContextual,
		DirectCallable: exposure != ToolExposureInternal,
		CatalogPriority: func() int {
			if exposure == ToolExposurePrimary {
				return 100
			}
			return 0
		}(),
	}
}

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
	sort.Slice(result, func(i, j int) bool {
		if result[i].ExposurePolicy.CatalogPriority != result[j].ExposurePolicy.CatalogPriority {
			return result[i].ExposurePolicy.CatalogPriority > result[j].ExposurePolicy.CatalogPriority
		}
		return result[i].Name < result[j].Name
	})
	return result
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
	switch name {
	case "Context.Get", "Session.Summarize",
		"Asset.Collection.Get", "Task.GetDetail",
		"Vulnerability.CustomQuery.Status", "Vulnerability.Scan.Status", "Vulnerability.Script.Status",
		"Credential.WeakPassword.QueryProgress", "ExternalMCP.Source.GetSchema", "Operation.Get":
		exposure = ToolExposureCompanion
	case "Baseline.Template.Rules.List", "Baseline.Script.Generate", "Task.RunCheck", "Task.RunFix":
		exposure = ToolExposureInternal
	case "Host.Resolve", "Baseline.Compliance.Run":
		exposure = ToolExposurePrimary
	case "":
		exposure = ToolExposureInternal
	}
	return ToolExposurePolicy{
		Exposure:       exposure,
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

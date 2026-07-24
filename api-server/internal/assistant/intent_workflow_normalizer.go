package assistant

import "strings"

const baselineComplianceWorkflowID = "baseline_compliance"

// normalizeWorkflowIntentBreakdown converts model-selected synonyms into the
// canonical identifiers owned by a selected workflow. The generic intent
// contract remains open; normalization is intentionally scoped to workflows
// whose backend compiler has a closed input contract.
func normalizeWorkflowIntentBreakdown(value *IntentBreakdown) {
	if value == nil {
		return
	}
	if isBaselineComplianceIntent(value) {
		normalizeBaselineComplianceIntent(value)
	}
	normalizeVulnerabilityRemediationIntent(value)
}

func normalizeBaselineComplianceIntent(value *IntentBreakdown) {
	for index := range value.Objects {
		objectType := strings.ToLower(strings.TrimSpace(value.Objects[index].Type))
		switch objectType {
		case "machine", "server", "endpoint":
			value.Objects[index].Type = "host"
		case "baseline", "benchmark", "compliance_baseline":
			value.Objects[index].Type = "baseline_template"
		default:
			value.Objects[index].Type = objectType
		}
	}

	switch strings.ToLower(strings.TrimSpace(value.Scope.Kind)) {
	case "all_live_hosts", "live_hosts", "all_alive_hosts", "alive_hosts", "online_hosts", "all_online_hosts":
		value.Scope.Kind = "all_online_hosts"
	case "", "unspecified":
		if hasAliveHostSelector(value.Objects) {
			value.Scope.Kind = "all_online_hosts"
			value.Scope.Source = "backend_workflow_normalizer"
		}
	}

	if value.Parameters == nil {
		value.Parameters = IntentParameters{}
	}
	copyIntentParameterAlias(value.Parameters, "auto_remediate",
		"auto_repair",
		"automatic_repair",
		"enable_auto_repair",
		"enable_automatic_repair",
		"remediation_enabled",
		"enable_remediation",
		"automatic_remediation",
	)
	copyIntentParameterAlias(value.Parameters, "remediation_rounds",
		"repair_rounds",
		"retry_rounds",
		"auto_repair_rounds",
		"max_repair_rounds",
	)
}

// normalizeVulnerabilityRemediationIntent completes the closed capability
// contract for an explicitly selected POC/remediation workflow. The model may
// identify the final execution capability while omitting the prerequisite
// script-generation capability. Completing that dependency before Mapping
// keeps every tool inside the normal authorization hard gates and lets Mapping
// derive the read-only asynchronous status companion from the Generate tool.
func normalizeVulnerabilityRemediationIntent(value *IntentBreakdown) {
	if value == nil ||
		!value.RequiresWrite ||
		!workflowSelected(value, vulnerabilityRemediationWorkflowID) ||
		!hasVulnerabilityPOCOrRemediationAction(value) {
		return
	}
	value.CandidateCapabilities = dedupeStrings(append(
		value.CandidateCapabilities,
		"resolve_hosts",
		"generate_vulnerability_script",
		"execute_vulnerability_host_scripts",
	))
}

func workflowSelected(value *IntentBreakdown, workflowID string) bool {
	if value == nil {
		return false
	}
	for _, candidate := range value.WorkflowIDs {
		if strings.EqualFold(strings.TrimSpace(candidate), workflowID) {
			return true
		}
	}
	return false
}

func isBaselineComplianceIntent(value *IntentBreakdown) bool {
	for _, workflowID := range value.WorkflowIDs {
		if strings.EqualFold(strings.TrimSpace(workflowID), baselineComplianceWorkflowID) {
			return true
		}
	}
	for _, capability := range value.CandidateCapabilities {
		if strings.EqualFold(strings.TrimSpace(capability), "run_baseline_compliance") {
			return true
		}
	}
	return false
}

func hasAliveHostSelector(objects []IntentObject) bool {
	for _, object := range objects {
		switch strings.ToLower(strings.TrimSpace(object.Type)) {
		case "host", "machine", "server", "endpoint":
		default:
			continue
		}
		if isOnlineHostSelectorAlias(object.Selector) {
			return true
		}
	}
	return false
}

func copyIntentParameterAlias(parameters IntentParameters, canonical string, aliases ...string) {
	if _, exists := parameters[canonical]; exists {
		for _, alias := range aliases {
			delete(parameters, alias)
		}
		return
	}
	for _, alias := range aliases {
		if value, exists := parameters[alias]; exists {
			parameters[canonical] = value
			break
		}
	}
	for _, alias := range aliases {
		delete(parameters, alias)
	}
}

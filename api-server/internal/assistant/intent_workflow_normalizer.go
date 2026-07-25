package assistant

import "strings"

const baselineComplianceWorkflowID = "baseline_compliance"

// normalizeWorkflowIntentBreakdown converts model-selected synonyms into the
// canonical identifiers owned by a selected workflow. Descriptive intent
// fields remain open, while workflow IDs come from the first-layer closed
// registry contract. Normalization stays scoped to workflows whose backend
// compiler has a closed input contract.
func normalizeWorkflowIntentBreakdown(value *IntentBreakdown, query string) {
	if value == nil {
		return
	}
	if isBaselineComplianceIntent(value) {
		normalizeBaselineComplianceIntent(value)
	}
	normalizeVulnerabilityRemediationIntent(value)
	normalizeDetectionPackageIntent(value, query)
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

func normalizeDetectionPackageIntent(value *IntentBreakdown, query string) {
	if value == nil || !workflowSelected(value, detectionPackageLifecycleWorkflowID) || !isDetectionPackageMutationIntent(value) {
		return
	}

	for index := range value.Actions {
		if action, ok := canonicalDetectionPackageAction(value.Actions[index]); ok {
			value.Actions[index] = action
		}
	}
	value.Actions = dedupeStrings(value.Actions)

	for index := range value.Objects {
		switch normalizeExposureIdentifier(value.Objects[index].Type) {
		case "dynamic_detection", "dynamic_detection_package", "runtime_detection_package":
			value.Objects[index].Type = "detection_package"
		}
	}
	if !containsExactString(value.Domains, string(DomainPackage)) {
		value.Domains = append(value.Domains, string(DomainPackage))
	}

	value.RequiresWrite = true
	if value.RiskHint == "" || strings.EqualFold(value.RiskHint, string(ToolRiskReadonly)) ||
		strings.EqualFold(value.RiskHint, string(ToolRiskLow)) {
		value.RiskHint = string(ToolRiskMedium)
	}
	if value.Parameters == nil {
		value.Parameters = IntentParameters{}
	}
	if strings.TrimSpace(query) != "" {
		value.Parameters["vulnerability_description"] = strings.TrimSpace(query)
		value.Parameters["exploitation_chain"] = strings.TrimSpace(query)
	}
	if _, ok := value.Parameters["cve_id"]; !ok {
		if cveIDs := exactCVEIDsFromBreakdown(value); len(cveIDs) == 1 {
			value.Parameters["cve_id"] = cveIDs[0]
		}
	}
	filteredMissingInfo := make([]MissingInfo, 0, len(value.MissingInfo))
	for _, missing := range value.MissingInfo {
		if isGeneratedDetectionPackageArtifactMissingInfo(missing) {
			continue
		}
		filteredMissingInfo = append(filteredMissingInfo, missing)
	}
	value.MissingInfo = filteredMissingInfo
	if detectionPackageGenerationInputsComplete(value) && len(value.MissingInfo) == 0 {
		value.NeedClarification = false
		value.ClarifyingQuestion = ""
	}

	// Creating a package is the first durable lifecycle stage. Build, sign and
	// enable require the generated package ID plus later build/review state, so
	// model-inferred future PACKAGE actions must not be authorized in the
	// creation run. Capabilities owned by earlier requested workflows (for
	// example a vulnerability scan before package generation) must remain.
	if requiresDetectionPackageDraft(value) {
		filtered := make([]string, 0, len(value.CandidateCapabilities)+1)
		for _, capability := range value.CandidateCapabilities {
			capability = strings.ToLower(strings.TrimSpace(capability))
			if isDetectionPackageLifecycleCapability(capability) {
				continue
			}
			filtered = append(filtered, capability)
		}
		filtered = append(filtered, "generate_detection_package_draft")
		if requiresDetectionPackageActivation(value) {
			// Build.Status is derived as the read-only completion companion of
			// Build.Start by Mapping. Do not inject a tool name here.
			filtered = append(filtered, "start_detection_package_build")
		}
		value.CandidateCapabilities = dedupeStrings(filtered)
		return
	}

	filtered := make([]string, 0, len(value.CandidateCapabilities))
	for _, capability := range value.CandidateCapabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		if detectionPackageCapabilityAllowed(value, capability) {
			filtered = append(filtered, capability)
		}
	}
	value.CandidateCapabilities = dedupeStrings(filtered)
}

func isGeneratedDetectionPackageArtifactMissingInfo(missing MissingInfo) bool {
	text := strings.ToLower(strings.Join([]string{
		missing.Field,
		missing.Reason,
		missing.Question,
	}, " "))
	for _, generatedOutput := range []string{
		"hookplan",
		"hook plan",
		"hook 计划",
		"钩子计划",
		"ebpf",
		"e-bpf",
		"sigma",
		"correlation rule",
		"correlation detection",
		"关联规则",
		"检测规则",
		"target deployment environment",
		"deployment environment",
		"target environment",
		"platform environment",
		"目标部署环境",
	} {
		if strings.Contains(text, generatedOutput) {
			return true
		}
	}
	return false
}

func detectionPackageGenerationInputsComplete(value *IntentBreakdown) bool {
	if value == nil || value.Parameters == nil || len(exactCVEIDsFromBreakdown(value)) != 1 {
		return false
	}
	for _, field := range []string{"vulnerability_description", "exploitation_chain"} {
		if raw, ok := value.Parameters[field]; ok {
			if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

func requiresDetectionPackageActivation(value *IntentBreakdown) bool {
	if value == nil {
		return false
	}
	for _, action := range value.Actions {
		normalized, _ := canonicalDetectionPackageAction(action)
		switch normalized {
		case "build", "compile", "detect", "execute", "enable", "publish", "dispatch", "activate":
			return true
		}
	}
	// A closed first-layer capability is stronger evidence than a model-specific
	// action spelling. For a newly generated package, Build.Start means the user
	// requested detection through the package rather than draft creation only.
	if exactDetectionPackageID(value) == "" &&
		containsExactString(value.CandidateCapabilities, "start_detection_package_build") {
		return true
	}
	return false
}

func isDetectionPackageLifecycleCapability(capability string) bool {
	switch strings.ToLower(strings.TrimSpace(capability)) {
	case "list_detection_packages",
		"get_detection_package",
		"generate_detection_package_draft",
		"start_detection_package_build",
		"get_detection_package_build_status",
		"sign_detection_package",
		"enable_detection_package":
		return true
	default:
		return false
	}
}

func isDetectionPackageMutationIntent(value *IntentBreakdown) bool {
	if value == nil {
		return false
	}
	if method, ok := value.Parameters["detection_method"].(string); ok {
		switch normalizeExposureIdentifier(method) {
		case "dynamic", "runtime", "dynamic_detection_package", "detection_package", "runtime_detection_package":
			return true
		}
	}
	for _, action := range value.Actions {
		normalized, _ := canonicalDetectionPackageAction(action)
		switch normalized {
		case "detect", "generate", "create", "build", "publish", "sign", "enable", "dispatch":
			return true
		}
	}
	return hasDetectionPackageMutationCapability(value)
}

func requiresDetectionPackageDraft(value *IntentBreakdown) bool {
	if value == nil {
		return false
	}
	if hasDetectionPackageAction(value, "build", "compile", "sign", "approve", "enable", "publish", "dispatch") &&
		exactDetectionPackageID(value) != "" {
		return false
	}
	if hasDetectionPackageAction(value, "detect", "generate", "create") {
		return true
	}
	if exactDetectionPackageID(value) == "" &&
		containsExactString(value.CandidateCapabilities, "generate_detection_package_draft") {
		return true
	}
	if method, ok := value.Parameters["detection_method"].(string); ok {
		switch normalizeExposureIdentifier(method) {
		case "dynamic", "runtime", "dynamic_detection_package", "detection_package", "runtime_detection_package":
			return true
		}
	}
	return false
}

func detectionPackageCapabilityAllowed(value *IntentBreakdown, capability string) bool {
	switch capability {
	case "list_detection_packages", "get_detection_package":
		return true
	case "generate_detection_package_draft":
		return requiresDetectionPackageDraft(value)
	case "start_detection_package_build":
		return hasDetectionPackageAction(value, "build", "compile")
	case "sign_detection_package":
		return hasDetectionPackageAction(value, "sign", "approve", "publish")
	case "enable_detection_package":
		return hasDetectionPackageAction(value, "enable", "publish", "dispatch")
	default:
		return false
	}
}

func hasDetectionPackageAction(value *IntentBreakdown, wanted ...string) bool {
	if value == nil {
		return false
	}
	for _, action := range value.Actions {
		normalized, _ := canonicalDetectionPackageAction(action)
		for _, candidate := range wanted {
			if normalized == candidate {
				return true
			}
		}
	}
	return false
}

func hasDetectionPackageMutationCapability(value *IntentBreakdown) bool {
	if value == nil {
		return false
	}
	for _, capability := range value.CandidateCapabilities {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "generate_detection_package_draft",
			"start_detection_package_build",
			"sign_detection_package",
			"enable_detection_package":
			return true
		}
	}
	return false
}

// canonicalDetectionPackageAction converts only aliases owned by the closed
// detection-package workflow. Unrelated actions (for example a preceding scan)
// remain untouched so multi-workflow request order is preserved.
func canonicalDetectionPackageAction(action string) (string, bool) {
	switch normalizeExposureIdentifier(action) {
	case "detect", "dynamic_detection", "detect_with_dynamic_package", "detect_with_dynamic_detection_package":
		return "detect", true
	case "generate", "create",
		"generate_detection_package", "generate_dynamic_detection_package",
		"create_detection_package", "create_dynamic_detection_package",
		"generate_detection_package_draft":
		return "generate", true
	case "build", "compile",
		"build_detection_package", "compile_detection_package",
		"start_detection_package_build":
		return "build", true
	case "sign", "approve", "sign_detection_package":
		return "sign", true
	case "enable", "activate", "dispatch",
		"enable_detection_package", "activate_detection_package", "dispatch_detection_package":
		return "enable", true
	case "publish", "publish_detection_package":
		return "publish", true
	case "execute", "execute_detection_package":
		return "execute", true
	default:
		return normalizeExposureIdentifier(action), false
	}
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

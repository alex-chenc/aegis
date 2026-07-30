package assistant

import (
	"fmt"
	"sort"
	"strings"
)

// WorkflowSpec is the compact, versioned routing card used before the model
// sees individual tools. Detailed execution remains in high-level services.
type WorkflowSpec struct {
	ID                  string     `json:"id"`
	Version             string     `json:"version"`
	Domain              ToolDomain `json:"domain"`
	Goal                string     `json:"goal"`
	TriggerIntents      []string   `json:"trigger_intents,omitempty"`
	ObjectTypes         []string   `json:"object_types,omitempty"`
	Risk                ToolRisk   `json:"risk"`
	ExposedCapabilities []string   `json:"exposed_capabilities,omitempty"`
}

type WorkflowRegistry struct {
	specs []WorkflowSpec
}

func NewWorkflowRegistry() *WorkflowRegistry {
	return &WorkflowRegistry{specs: builtinWorkflowSpecs()}
}

// List returns a stable copy of every workflow card available to the first
// intent-classification contract.
func (r *WorkflowRegistry) List() []WorkflowSpec {
	if r == nil {
		return nil
	}
	specs := append([]WorkflowSpec{}, r.specs...)
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	return specs
}

// Resolve maps the model-selected closed workflow IDs back to registered
// cards. Unknown IDs are rejected instead of being guessed from free text.
func (r *WorkflowRegistry) Resolve(ids []string) ([]WorkflowSpec, error) {
	if r == nil {
		return nil, fmt.Errorf("workflow registry is nil")
	}
	registered := make(map[string]WorkflowSpec, len(r.specs))
	for _, spec := range r.specs {
		registered[spec.ID] = spec
	}
	resolved := make([]WorkflowSpec, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		spec, ok := registered[id]
		if !ok {
			return nil, fmt.Errorf("workflow ID %q is not registered", id)
		}
		seen[id] = struct{}{}
		resolved = append(resolved, spec)
	}
	return resolved, nil
}

// Match deterministically retrieves only workflow cards relevant to the
// classified domain, object type, operation, or normalized keyword.
func (r *WorkflowRegistry) Match(intent IntentResult) []WorkflowSpec {
	if r == nil {
		return nil
	}
	terms := append([]string{}, intent.Operations...)
	terms = append(terms, intent.Keywords...)
	terms = append(terms, intent.Action, intent.Object)
	var matches []WorkflowSpec
	for _, spec := range r.specs {
		if stringSliceContainsFold(intent.Domains, string(spec.Domain)) ||
			stringSliceIntersectsFold(intent.ObjectTypes, spec.ObjectTypes) ||
			workflowTermsMatch(terms, spec.TriggerIntents) {
			matches = append(matches, spec)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	if len(matches) > 15 {
		matches = matches[:15]
	}
	return matches
}

func workflowTermsMatch(terms, triggers []string) bool {
	for _, term := range terms {
		normalizedTerm := normalizeExposureIdentifier(term)
		if normalizedTerm == "" {
			continue
		}
		for _, trigger := range triggers {
			normalizedTrigger := normalizeExposureIdentifier(trigger)
			if normalizedTrigger != "" && (normalizedTerm == normalizedTrigger || strings.Contains(normalizedTerm, normalizedTrigger)) {
				return true
			}
		}
	}
	return false
}

func builtinWorkflowSpecs() []WorkflowSpec {
	return []WorkflowSpec{
		{ID: "host_management", Version: "6.1", Domain: DomainHost, Goal: "Resolve hosts and inspect identity and agent availability.", TriggerIntents: []string{"resolve_host", "list_host", "host_status"}, ObjectTypes: []string{"host"}, Risk: ToolRiskReadonly, ExposedCapabilities: []string{"resolve_hosts", "list_hosts", "get_host_detail", "get_agent_status"}},
		{ID: "host_forensics", Version: "6.1", Domain: DomainAgent, Goal: "Collect bounded live evidence from an online host.", TriggerIntents: []string{"forensics", "process", "network", "log"}, ObjectTypes: []string{"host", "process", "connection", "file", "log"}, Risk: ToolRiskReadonly},
		{ID: "asset_inventory", Version: "6.1", Domain: DomainAsset, Goal: "Refresh and query software, application, and AI asset inventory.", TriggerIntents: []string{"asset", "inventory", "collect"}, ObjectTypes: []string{"asset", "software", "application"}, Risk: ToolRiskMedium, ExposedCapabilities: []string{"asset_summary", "list_software_assets", "list_application_assets", "trigger_asset_collection", "list_asset_collection_tasks", "get_asset_collection_task"}},
		{ID: "vulnerability_assessment", Version: "6.1", Domain: DomainVulnerability, Goal: "Run and monitor vulnerability assessment with complete host coverage.", TriggerIntents: []string{"scan", "assessment"}, ObjectTypes: []string{"vulnerability", "host"}, Risk: ToolRiskMedium, ExposedCapabilities: []string{"resolve_hosts", "start_vulnerability_scan", "get_vulnerability_scan_status"}},
		{ID: "cve_lookup", Version: "6.1", Domain: DomainVulnerability, Goal: "Find an exact CVE and enrich it only when the catalog is empty.", TriggerIntents: []string{"cve", "lookup"}, ObjectTypes: []string{"cve", "vulnerability"}, Risk: ToolRiskReadonly, ExposedCapabilities: []string{"list_vulnerabilities", "start_custom_cve_query", "get_custom_cve_query_status"}},
		{ID: "vulnerability_remediation", Version: "6.1", Domain: DomainVulnerability, Goal: "Generate, execute, and verify CVE proof or remediation scripts.", TriggerIntents: []string{"verify", "remediate", "fix"}, ObjectTypes: []string{"cve", "vulnerability", "host"}, Risk: ToolRiskHigh, ExposedCapabilities: []string{"resolve_hosts", "list_vulnerabilities", "generate_vulnerability_script", "execute_vulnerability_host_scripts"}},
		{ID: "baseline_compliance", Version: "6.1", Domain: DomainBaseline, Goal: "Run complete baseline checks and optional verified remediation.", TriggerIntents: []string{"baseline", "compliance", "check", "remediate"}, ObjectTypes: []string{"baseline_template", "baseline_rule", "host"}, Risk: ToolRiskHigh, ExposedCapabilities: []string{"run_baseline_compliance"}},
		{ID: "task_operations", Version: "6.1", Domain: DomainTask, Goal: "Monitor task groups and explain every terminal or incomplete task.", TriggerIntents: []string{"task", "progress", "retry"}, ObjectTypes: []string{"task", "task_group"}, Risk: ToolRiskLow},
		{ID: "detection_response", Version: "6.1", Domain: DomainDetection, Goal: "Investigate alerts and perform approved response actions.", TriggerIntents: []string{"alert", "detection", "respond", "block"}, ObjectTypes: []string{"alert", "detection"}, Risk: ToolRiskCritical},
		{ID: "host_investigation", Version: "6.1", Domain: DomainInvestigation, Goal: "Correlate host evidence into an attack timeline with explicit gaps.", TriggerIntents: []string{"investigate", "attack", "timeline"}, ObjectTypes: []string{"investigation", "host", "alert"}, Risk: ToolRiskReadonly},
		{ID: "weak_password_assessment", Version: "6.1", Domain: DomainAsset, Goal: "Assess password-authenticated applications and verify remediation safely.", TriggerIntents: []string{"weak_password", "credential"}, ObjectTypes: []string{"credential", "application", "host"}, Risk: ToolRiskHigh, ExposedCapabilities: []string{"resolve_hosts", "weak_password_dictionary_generation", "weak_password_asset_analysis", "weak_password_scan", "weak_password_progress", "weak_password_findings", "weak_password_explain"}},
		{ID: "sigma_rule_lifecycle", Version: "6.1", Domain: DomainSigmaRule, Goal: "Import, generate, enable, and validate Sigma detection rules.", TriggerIntents: []string{"sigma", "detection_rule", "enable_rule", "import_rule"}, ObjectTypes: []string{"sigma_rule", "sigma_rule_upload", "rule"}, Risk: ToolRiskMedium, ExposedCapabilities: []string{"list_sigma_rules", "import_sigma_rule", "enable_sigma_rule", "generate_sigma_rule"}},
		{ID: detectionPackageLifecycleWorkflowID, Version: "6.1", Domain: DomainPackage, Goal: "Generate, build, validate, sign, enable, and distribute detection packages.", TriggerIntents: []string{"package", "publish", "build"}, ObjectTypes: []string{"detection_package"}, Risk: ToolRiskHigh, ExposedCapabilities: []string{"list_detection_packages", "get_detection_package", "generate_detection_package_draft", "start_detection_package_build", "get_detection_package_build_status", "sign_detection_package", "enable_detection_package"}},
		{ID: "block_policy_change", Version: "6.1", Domain: DomainBlock, Goal: "Preview, approve, persist, and audit block policy changes.", TriggerIntents: []string{"block_policy", "mitre"}, ObjectTypes: []string{"block_policy"}, Risk: ToolRiskCritical},
		{ID: "external_evidence", Version: "6.1", Domain: DomainExternalMCP, Goal: "Query authorized external sources and analyze normalized redacted evidence.", TriggerIntents: []string{"external", "mcp", "evidence"}, ObjectTypes: []string{"external_source", "evidence"}, Risk: ToolRiskReadonly},
		{ID: "administrative_queries", Version: "6.1", Domain: DomainConfig, Goal: "Query configuration, audit, and notification records with explicit scope.", TriggerIntents: []string{"config", "audit", "notification"}, ObjectTypes: []string{"config", "audit_log", "notification"}, Risk: ToolRiskReadonly},
		{ID: "assistant_context", Version: "6.1", Domain: DomainSystem, Goal: "Preserve context, object references, approvals, and incomplete operations.", TriggerIntents: []string{"context", "session", "tool"}, ObjectTypes: []string{"session", "context", "operation"}, Risk: ToolRiskReadonly},
	}
}

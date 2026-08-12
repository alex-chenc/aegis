package assistant

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	assetInventoryWorkflowID            = "asset_inventory"
	vulnerabilityAssessmentWorkflowID   = "vulnerability_assessment"
	vulnerabilityRemediationWorkflowID  = "vulnerability_remediation"
	weakPasswordAssessmentWorkflowID    = "weak_password_assessment"
	detectionPackageLifecycleWorkflowID = "detection_package_lifecycle"
	agentGuardObservationWorkflowID     = "agent_guard_observation"
	agentGuardControlWorkflowID         = "agent_guard_control"
)

// MCPAggregationQueryWorkflowID is the V6.3 managed MCP workflow. It is
// exported because the fixed MCP facade tools live in the tools subpackage and
// must bind their exposure policy to the same closed workflow contract.
const MCPAggregationQueryWorkflowID = "mcp_aggregation_query"

// MCPAggregationOnboardingWorkflowID is the Assistant workflow for creating
// a remote MCP onboarding job through the governed control plane.
const MCPAggregationOnboardingWorkflowID = "mcp_aggregation_onboarding"

// MCPAggregationClientAuthorizationWorkflowID is the Assistant workflow for
// authorizing a new Client against an already published MCP service.
const MCPAggregationClientAuthorizationWorkflowID = "mcp_aggregation_client_authorization"

var exactCVEIDPattern = regexp.MustCompile(`(?i)\bCVE-[0-9]{4}-[0-9]{4,}\b`)

// ArgValueKind classifies how a bound argument value must be typed so the
// generic binder does not coerce a business scope into an entity ID.
type ArgValueKind string

const (
	ArgValueBusinessScope ArgValueKind = "business_scope"
	ArgValueEntityID      ArgValueKind = "entity_id"
	ArgValueEntityIDs     ArgValueKind = "entity_ids"
	ArgValuePreviousFacts ArgValueKind = "previous_facts"
)

// WorkflowPlanCompiler compiles an accepted capability Mapping set into a
// deterministic, typed business execution plan. A compiler may reorder, prune
// and rebind arguments, but it must never introduce a tool that was not
// accepted by Mapping. When the intent is too ambiguous to compile safely the
// compiler returns a non-empty Clarification instead of guessing.
type WorkflowPlanCompiler interface {
	WorkflowID() string
	Compile(input WorkflowCompileInput) (*WorkflowCompileResult, error)
}

// WorkflowCompileInput carries everything a compiler needs to transform the
// accepted Mapping set into an ordered plan.
type WorkflowCompileInput struct {
	Breakdown       *IntentBreakdown
	AcceptedTools   []string
	DecisionRecords []ToolDecisionRecord
	Registry        *ToolRegistry
	Mapper          *ToolCapabilityMapper
}

// WorkflowCompileResult is the output of a workflow compiler. When
// Clarification is non-empty the caller must surface it to the user instead of
// building a runtime plan.
type WorkflowCompileResult struct {
	Steps                 []ToolPlanStep
	Clarification         string
	DeferredClarification string
	RemainingWorkflowIDs  []string
}

// WorkflowPlanCompilerRegistry selects the unique compiler for a workflow ID.
type WorkflowPlanCompilerRegistry struct {
	compilers map[string]WorkflowPlanCompiler
}

func NewWorkflowPlanCompilerRegistry() *WorkflowPlanCompilerRegistry {
	registry := &WorkflowPlanCompilerRegistry{compilers: make(map[string]WorkflowPlanCompiler)}
	registry.Register(&AssetInventoryCompiler{})
	registry.Register(&VulnerabilityAssessmentCompiler{})
	registry.Register(&VulnerabilityRemediationCompiler{})
	registry.Register(&WeakPasswordAssessmentCompiler{})
	registry.Register(&DetectionPackageLifecycleCompiler{})
	return registry
}

func (r *WorkflowPlanCompilerRegistry) Register(compiler WorkflowPlanCompiler) {
	if r == nil || compiler == nil {
		return
	}
	r.compilers[strings.ToLower(strings.TrimSpace(compiler.WorkflowID()))] = compiler
}

// Get returns the compiler registered for the given workflow ID.
func (r *WorkflowPlanCompilerRegistry) Get(workflowID string) WorkflowPlanCompiler {
	if r == nil {
		return nil
	}
	return r.compilers[strings.ToLower(strings.TrimSpace(workflowID))]
}

// CompileForBreakdown invokes every selected compiler in declared order and
// composes their steps. A user request may contain an ordered workflow chain
// (for example vulnerability assessment followed by package generation);
// returning after the first compiler would silently drop the later goal.
// When no compiler produces a result the caller falls back to the generic
// capability-driven path.
func (r *WorkflowPlanCompilerRegistry) CompileForBreakdown(input WorkflowCompileInput) (*WorkflowCompileResult, bool, error) {
	if r == nil || input.Breakdown == nil {
		return nil, false, nil
	}
	combined := &WorkflowCompileResult{}
	compiled := false
	seen := make(map[string]bool)
	for index, workflowID := range input.Breakdown.WorkflowIDs {
		workflowID = strings.ToLower(strings.TrimSpace(workflowID))
		if workflowID == "" || seen[workflowID] {
			continue
		}
		seen[workflowID] = true
		compiler := r.Get(workflowID)
		if compiler == nil {
			continue
		}
		result, err := compiler.Compile(input)
		if err != nil {
			return nil, true, err
		}
		if result == nil {
			continue
		}
		compiled = true
		if result.Clarification != "" {
			if len(combined.Steps) > 0 {
				combined.DeferredClarification = result.Clarification
				combined.RemainingWorkflowIDs = dedupeStrings(
					append([]string{}, input.Breakdown.WorkflowIDs[index:]...),
				)
				return combined, true, nil
			}
			return result, true, nil
		}
		combined.Steps = append(combined.Steps, result.Steps...)
	}
	if !compiled {
		return nil, false, nil
	}
	return combined, true, nil
}

// ---------------------------------------------------------------------------
// Asset inventory compiler
// ---------------------------------------------------------------------------

// AssetInventoryCompiler compiles asset re-collection requests into a
// deterministic business DAG. It replaces the registration-order-dependent
// generic path for the asset_inventory workflow.
//
// Scenario A – all hosts: scope.kind=all compiles to a single
// Asset.Collection.Trigger step with scope=all_hosts. Host.List and
// Host.Resolve are pruned because the asset domain service already owns the
// all_hosts backend semantic.
//
// Scenario B – exact UUIDs: compiles to a single Trigger step with
// scope=hosts and a de-duplicated host_ids array.
//
// Scenario C – selector (hostname/IP): compiles Host.Resolve followed by
// Asset.Collection.Trigger. The Trigger's host_ids are bound from the
// real host_resolved facts produced by the resolve step; the model may not
// regenerate them.
//
// Scenario D – ambiguous: returns a clarification; no runtime tool descriptor
// is produced.
type AssetInventoryCompiler struct{}

func (c *AssetInventoryCompiler) WorkflowID() string { return assetInventoryWorkflowID }

func (c *AssetInventoryCompiler) Compile(input WorkflowCompileInput) (*WorkflowCompileResult, error) {
	breakdown := input.Breakdown
	if breakdown == nil {
		return nil, nil
	}
	if !containsExactString(input.AcceptedTools, "Asset.Collection.Trigger") {
		// Not an asset re-collection write request; let the generic path handle it.
		return nil, nil
	}

	scenario, hostIDs, selectors := classifyAssetScope(breakdown)

	var primarySteps []ToolPlanStep
	var clarification string

	switch scenario {
	case "all_hosts":
		primarySteps = []ToolPlanStep{
			c.buildTriggerStep(input, map[string]interface{}{
				"scope": "all_hosts",
				"force": true,
			}, map[string]ArgSource{
				"scope": {SourceType: "intent_scope", SourceRef: "all_hosts", Confidence: 1},
				"force": {SourceType: "policy_default", SourceRef: "force_refresh", Confidence: 1},
			}),
		}

	case "exact_uuids":
		primarySteps = []ToolPlanStep{
			c.buildTriggerStep(input, map[string]interface{}{
				"scope":    "hosts",
				"host_ids": dedupeStrings(hostIDs),
				"force":    true,
			}, map[string]ArgSource{
				"scope":    {SourceType: "intent_scope", SourceRef: "hosts", Confidence: 1},
				"host_ids": {SourceType: "intent_scope", SourceRef: "scope:object_ids", Confidence: 0.95},
				"force":    {SourceType: "policy_default", SourceRef: "force_refresh", Confidence: 1},
			}),
		}

	case "selector":
		if !containsExactString(input.AcceptedTools, "Host.Resolve") {
			clarification = localizedClarification(input.Breakdown,
				"无法解析目标主机。请提供精确的主机 UUID，或明确选择“所有主机”进行资产重采集。",
				"Unable to resolve target hosts. Provide exact host UUIDs or explicitly choose all hosts for asset re-collection.",
			)
			break
		}
		resolveStep := c.buildResolveStep(input, selectors)
		triggerStep := c.buildTriggerStep(input, map[string]interface{}{
			"scope": "hosts",
			"force": true,
		}, map[string]ArgSource{
			"scope":    {SourceType: "intent_scope", SourceRef: "hosts", Confidence: 1},
			"host_ids": {SourceType: "previous_step", SourceRef: "host", Confidence: 0.5},
			"force":    {SourceType: "policy_default", SourceRef: "force_refresh", Confidence: 1},
		})
		triggerStep.Condition = "requires previous step to produce: host_ids"
		primarySteps = []ToolPlanStep{resolveStep, triggerStep}

	default: // ambiguous
		clarification = localizedClarification(input.Breakdown,
			"资产采集需要明确的目标范围：所有主机、指定主机 UUID 或可解析的主机名/IP。请补充范围信息后再执行。",
			"Asset collection requires an explicit target scope: all hosts, exact host UUIDs, or a resolvable hostname/IP. Please specify the scope before continuing.",
		)
	}

	if clarification != "" {
		return &WorkflowCompileResult{Clarification: clarification}, nil
	}

	// Append the mapped completion tool as a plan step so the runtime can
	// attach it to the Trigger's business step. Asset.Collection.Get polls
	// the real task_id from the Trigger outcome until a terminal status is
	// reached; the model may not generate or modify it.
	steps := primarySteps
	if containsExactString(input.AcceptedTools, "Asset.Collection.Get") {
		steps = append(steps, c.buildCompletionStep(input))
	}
	return &WorkflowCompileResult{Steps: steps}, nil
}

// buildTriggerStep creates an Asset.Collection.Trigger plan step with the
// compiler-bound arguments and the tool's declarative contract metadata.
func (c *AssetInventoryCompiler) buildTriggerStep(input WorkflowCompileInput, args map[string]interface{}, argSources map[string]ArgSource) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Asset.Collection.Trigger")
	return ToolPlanStep{
		ToolName:         "Asset.Collection.Trigger",
		Capability:       "trigger_asset_collection",
		Args:             args,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Asset re-collection compiled by the asset_inventory workflow compiler.",
		ArgSources:       argSources,
		Preconditions:    contract.Preconditions,
		Postconditions:   contract.Postconditions,
	}
}

// buildResolveStep creates a Host.Resolve plan step for scenario C.
func (c *AssetInventoryCompiler) buildResolveStep(input WorkflowCompileInput, selectors []string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Host.Resolve")
	return ToolPlanStep{
		ToolName:   "Host.Resolve",
		Capability: "resolve_hosts",
		Args: map[string]interface{}{
			"host_selectors": dedupeStrings(selectors),
			"require_online": true,
		},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Resolve host selectors to exact UUIDs before asset re-collection.",
		ArgSources: map[string]ArgSource{
			"host_selectors": {SourceType: "user_message", SourceRef: "host_selector", Confidence: 0.8},
			"require_online": {SourceType: "policy_default", SourceRef: "asset_collection_targets", Confidence: 1},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
	}
}

// buildCompletionStep creates an Asset.Collection.Get plan step. Its task_id
// argument is bound from the Trigger step's real outcome at runtime; the
// compiler does not set it. The runtime attaches this step to the Trigger's
// business step as a mapped completion tool.
func (c *AssetInventoryCompiler) buildCompletionStep(input WorkflowCompileInput) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Asset.Collection.Get")
	return ToolPlanStep{
		ToolName:         "Asset.Collection.Get",
		Capability:       "get_asset_collection_task",
		Args:             map[string]interface{}{},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Poll the real asset collection task_id until a terminal status is reached.",
		ArgSources: map[string]ArgSource{
			"task_id": {SourceType: "previous_step", SourceRef: "asset_collection_task", Confidence: 0.5},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
		Condition:      "requires previous step to produce: task_id",
	}
}

// ---------------------------------------------------------------------------
// Vulnerability assessment compiler
// ---------------------------------------------------------------------------

// VulnerabilityAssessmentCompiler compiles a vulnerability scan into a typed
// host-resolution, scan-start, and status-polling workflow. A hostname or IP is
// a selector, not a host ID, so it must be resolved before the write tool is
// allowed to receive host_ids.
type VulnerabilityAssessmentCompiler struct{}

func (c *VulnerabilityAssessmentCompiler) WorkflowID() string {
	return vulnerabilityAssessmentWorkflowID
}

func (c *VulnerabilityAssessmentCompiler) Compile(input WorkflowCompileInput) (*WorkflowCompileResult, error) {
	breakdown := input.Breakdown
	if breakdown == nil {
		return nil, nil
	}
	if hasVulnerabilityPOCOrRemediationAction(breakdown) &&
		(containsExactString(input.AcceptedTools, "Vulnerability.Script.Generate") ||
			containsExactString(input.AcceptedTools, "Vulnerability.Script.Execute")) {
		// A broad vulnerability workflow match may expose scan capabilities for
		// a request whose explicit action is POC verification/remediation. Let
		// the remediation compiler own that request instead of starting a second
		// full vulnerability assessment.
		return nil, nil
	}
	if !containsExactString(input.AcceptedTools, "Vulnerability.Scan.Start") {
		return nil, nil
	}

	if !containsExactString(input.AcceptedTools, "Vulnerability.Scan.Status") {
		return nil, newCompilePlanError(
			"completion_contract",
			"Vulnerability.Scan.Start",
			"completion_capability",
			"vulnerability scan requires the mapped Vulnerability.Scan.Status completion tool",
		)
	}

	scenario, hostIDs, selectors := classifyAssetScope(breakdown)
	steps := make([]ToolPlanStep, 0, 3)

	switch scenario {
	case "all_hosts":
		if !containsExactString(input.AcceptedTools, "Host.Resolve") {
			return &WorkflowCompileResult{Clarification: localizedClarification(
				breakdown,
				"无法安全解析扫描目标。请提供精确的主机 UUID，或允许先解析在线主机。",
				"Unable to resolve scan targets safely. Provide exact host UUIDs or allow online-host resolution first.",
			)}, nil
		}
		steps = append(steps, c.buildResolveStep(input, nil, true))
		steps = append(steps, c.buildStartStep(input, nil, true))

	case "exact_uuids":
		steps = append(steps, c.buildStartStep(input, dedupeStrings(hostIDs), false))

	case "selector":
		if !containsExactString(input.AcceptedTools, "Host.Resolve") {
			return &WorkflowCompileResult{Clarification: localizedClarification(
				breakdown,
				"无法解析目标主机。请提供精确的主机 UUID，或先提供可解析的主机名/IP。",
				"Unable to resolve target hosts. Provide exact host UUIDs or a resolvable hostname/IP.",
			)}, nil
		}
		steps = append(steps, c.buildResolveStep(input, selectors, false))
		steps = append(steps, c.buildStartStep(input, nil, true))

	default:
		return &WorkflowCompileResult{Clarification: localizedClarification(
			breakdown,
			"漏洞扫描需要明确的目标：主机 UUID、主机名/IP，或所有在线主机。请补充扫描范围。",
			"Vulnerability scanning requires exact host UUIDs, hostnames/IPs, or all online hosts. Please specify the target scope.",
		)}, nil
	}

	steps = append(steps, c.buildStatusStep(input))
	return &WorkflowCompileResult{Steps: steps}, nil
}

func (c *VulnerabilityAssessmentCompiler) buildResolveStep(input WorkflowCompileInput, selectors []string, allOnline bool) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Host.Resolve")
	args := map[string]interface{}{
		"require_online": true,
	}
	argSources := map[string]ArgSource{
		"require_online": {SourceType: "policy_default", SourceRef: "vulnerability_scan_targets", Confidence: 1},
	}
	if allOnline {
		args["target_scope"] = "all_online_hosts"
		argSources["target_scope"] = ArgSource{SourceType: "intent_scope", SourceRef: "all_online_hosts", Confidence: 1}
	} else {
		args["host_selectors"] = dedupeStrings(selectors)
		argSources["host_selectors"] = ArgSource{SourceType: "user_message", SourceRef: "host_selector", Confidence: 0.8}
	}
	return ToolPlanStep{
		ToolName:         "Host.Resolve",
		Capability:       "resolve_hosts",
		Args:             args,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Resolve vulnerability scan targets to exact online host UUIDs.",
		ArgSources:       argSources,
		Preconditions:    contract.Preconditions,
		Postconditions:   contract.Postconditions,
	}
}

func (c *VulnerabilityAssessmentCompiler) buildStartStep(input WorkflowCompileInput, hostIDs []string, fromPreviousStep bool) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Vulnerability.Scan.Start")
	args := make(map[string]interface{})
	argSources := make(map[string]ArgSource)
	condition := ""
	if fromPreviousStep {
		argSources["host_ids"] = ArgSource{SourceType: "previous_step", SourceRef: "host", Confidence: 0.5}
		condition = "requires previous step to produce: host_ids"
	} else {
		args["host_ids"] = dedupeStrings(hostIDs)
		argSources["host_ids"] = ArgSource{SourceType: "intent_scope", SourceRef: "host_uuid", Confidence: 0.95}
	}
	return ToolPlanStep{
		ToolName:         "Vulnerability.Scan.Start",
		Capability:       "start_vulnerability_scan",
		Args:             args,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Start vulnerability scanning for compiler-validated host UUIDs.",
		ArgSources:       argSources,
		Preconditions:    contract.Preconditions,
		Postconditions:   contract.Postconditions,
		Condition:        condition,
	}
}

func (c *VulnerabilityAssessmentCompiler) buildStatusStep(input WorkflowCompileInput) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Vulnerability.Scan.Status")
	return ToolPlanStep{
		ToolName:         "Vulnerability.Scan.Status",
		Capability:       "get_vulnerability_scan_status",
		Args:             map[string]interface{}{},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Poll the real vulnerability scan_id until a terminal status is reached.",
		ArgSources: map[string]ArgSource{
			"scan_id": {SourceType: "previous_step", SourceRef: "vulnerability_scan", Confidence: 0.5},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
		Condition:      "requires previous step to produce: scan_id",
	}
}

// ---------------------------------------------------------------------------
// Weak-password assessment compiler
// ---------------------------------------------------------------------------

// WeakPasswordAssessmentCompiler turns a weak-password request into a typed
// host-resolution, candidate-analysis, batch-task, and aggregate-progress
// workflow. Candidate and task IDs always come from prior tool evidence; the
// model is never asked to regenerate those references.
type WeakPasswordAssessmentCompiler struct{}

func (c *WeakPasswordAssessmentCompiler) WorkflowID() string {
	return weakPasswordAssessmentWorkflowID
}

func (c *WeakPasswordAssessmentCompiler) Compile(input WorkflowCompileInput) (*WorkflowCompileResult, error) {
	breakdown := input.Breakdown
	if breakdown == nil || !containsExactString(input.AcceptedTools, "Credential.WeakPassword.Scan") {
		return nil, nil
	}
	for _, toolName := range []string{
		"Credential.WeakPassword.AnalyzeApplications",
		"Credential.WeakPassword.QueryProgress",
	} {
		if !containsExactString(input.AcceptedTools, toolName) {
			return nil, newCompilePlanError(
				"completion_contract",
				"Credential.WeakPassword.Scan",
				"completion_capability",
				fmt.Sprintf("weak-password assessment requires the mapped %s tool", toolName),
			)
		}
	}

	scenario, hostIDs, selectors := classifyAssetScope(breakdown)
	steps := make([]ToolPlanStep, 0, 4)
	switch scenario {
	case "all_hosts":
		// AnalyzeAssetApplications owns the all-online-hosts selection and
		// deduplication semantic, so a separate Host.Resolve step is unnecessary.
		steps = append(steps, c.buildAnalyzeStep(input, nil, false))
	case "exact_uuids":
		steps = append(steps, c.buildAnalyzeStep(input, dedupeStrings(hostIDs), false))
	case "selector":
		if !containsExactString(input.AcceptedTools, "Host.Resolve") {
			return &WorkflowCompileResult{Clarification: localizedClarification(
				breakdown,
				"无法解析弱口令检查目标。请提供精确的主机 UUID，或先提供可解析的主机名/IP。",
				"Unable to resolve weak-password targets. Provide exact host UUIDs or a resolvable hostname/IP.",
			)}, nil
		}
		steps = append(steps, c.buildResolveStep(input, selectors))
		steps = append(steps, c.buildAnalyzeStep(input, nil, true))
	default:
		return &WorkflowCompileResult{Clarification: localizedClarification(
			breakdown,
			"弱口令检查需要明确的目标：主机 UUID、主机名/IP，或所有在线主机。请补充检查范围。",
			"Weak-password assessment requires exact host UUIDs, hostnames/IPs, or all online hosts. Please specify the target scope.",
		)}, nil
	}

	steps = append(steps, c.buildScanStep(input), c.buildProgressStep(input))
	return &WorkflowCompileResult{Steps: steps}, nil
}

func (c *WeakPasswordAssessmentCompiler) buildResolveStep(input WorkflowCompileInput, selectors []string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Host.Resolve")
	return ToolPlanStep{
		ToolName:         "Host.Resolve",
		Capability:       "resolve_hosts",
		Args:             map[string]interface{}{"host_selectors": dedupeStrings(selectors), "require_online": true},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Resolve weak-password assessment targets to exact online host UUIDs.",
		ArgSources: map[string]ArgSource{
			"host_selectors": {SourceType: "user_message", SourceRef: "host_selector", Confidence: 0.8},
			"require_online": {SourceType: "policy_default", SourceRef: "weak_password_targets", Confidence: 1},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
	}
}

func (c *WeakPasswordAssessmentCompiler) buildAnalyzeStep(input WorkflowCompileInput, hostIDs []string, fromPreviousStep bool) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Credential.WeakPassword.AnalyzeApplications")
	args := map[string]interface{}{"online_agents_only": true}
	argSources := map[string]ArgSource{
		"online_agents_only": {SourceType: "policy_default", SourceRef: "online_weak_password_targets", Confidence: 1},
	}
	condition := ""
	if fromPreviousStep {
		argSources["host_ids"] = ArgSource{SourceType: "previous_step", SourceRef: "host", Confidence: 0.5}
		condition = "requires previous step to produce: host_ids"
	} else if len(hostIDs) > 0 {
		args["host_ids"] = dedupeStrings(hostIDs)
		argSources["host_ids"] = ArgSource{SourceType: "intent_scope", SourceRef: "host_uuid", Confidence: 0.95}
	}
	if applicationTypes := weakPasswordApplicationTypes(input.Breakdown); len(applicationTypes) > 0 {
		args["application_types"] = applicationTypes
		argSources["application_types"] = ArgSource{SourceType: "intent_parameter", SourceRef: "application_types", Confidence: 0.9}
	}
	return ToolPlanStep{
		ToolName:         "Credential.WeakPassword.AnalyzeApplications",
		Capability:       "weak_password_asset_analysis",
		Args:             args,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Analyze password-authenticated application candidates on validated online hosts.",
		ArgSources:       argSources,
		Preconditions:    contract.Preconditions,
		Postconditions:   contract.Postconditions,
		Condition:        condition,
	}
}

func (c *WeakPasswordAssessmentCompiler) buildScanStep(input WorkflowCompileInput) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Credential.WeakPassword.Scan")
	return ToolPlanStep{
		ToolName:         "Credential.WeakPassword.Scan",
		Capability:       "weak_password_scan",
		Args:             map[string]interface{}{},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Create weak-password assessment tasks for every candidate produced by analysis.",
		ArgSources: map[string]ArgSource{
			"candidate_application_ids": {SourceType: "previous_step", SourceRef: "candidate_application", Confidence: 0.5},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
		Condition:      "requires previous step to produce: candidate_application_ids",
	}
}

func (c *WeakPasswordAssessmentCompiler) buildProgressStep(input WorkflowCompileInput) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Credential.WeakPassword.QueryProgress")
	return ToolPlanStep{
		ToolName:         "Credential.WeakPassword.QueryProgress",
		Capability:       "weak_password_progress",
		Args:             map[string]interface{}{},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Poll every created weak-password task until the aggregate workflow reaches a terminal state.",
		ArgSources: map[string]ArgSource{
			"task_ids": {SourceType: "previous_step", SourceRef: "task", Confidence: 0.5},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
		Condition:      "requires previous step to produce: task_ids",
	}
}

func weakPasswordApplicationTypes(breakdown *IntentBreakdown) []string {
	if breakdown == nil {
		return nil
	}
	for _, key := range []string{"application_types", "application_type"} {
		if value, ok := breakdown.Parameters[key]; ok {
			if values := dedupeStrings(toStringSlice(value)); len(values) > 0 {
				return values
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Vulnerability remediation compiler
// ---------------------------------------------------------------------------

// VulnerabilityRemediationCompiler compiles an exact-CVE POC request into:
// host resolution -> POC generation/status -> POC task dispatch. The dispatch
// enables the existing backend verify-remediate-reverify loop, which generates
// or reuses a FIX script only when the POC reports the host is vulnerable.
type VulnerabilityRemediationCompiler struct{}

func (c *VulnerabilityRemediationCompiler) WorkflowID() string {
	return vulnerabilityRemediationWorkflowID
}

func (c *VulnerabilityRemediationCompiler) Compile(input WorkflowCompileInput) (*WorkflowCompileResult, error) {
	breakdown := input.Breakdown
	if breakdown == nil || !hasVulnerabilityPOCOrRemediationAction(breakdown) {
		return nil, nil
	}

	requiredTools := []string{
		"Vulnerability.Script.Generate",
		"Vulnerability.Script.Status",
		"Vulnerability.Script.Execute",
	}
	for _, toolName := range requiredTools {
		if !containsExactString(input.AcceptedTools, toolName) {
			return nil, newCompilePlanError(
				"mapping_compile",
				toolName,
				"accepted_tools",
				"vulnerability POC/remediation requires script generation, completion status, and execution tools",
			)
		}
	}

	cveIDs := exactCVEIDsFromBreakdown(breakdown)
	if len(cveIDs) != 1 {
		return &WorkflowCompileResult{Clarification: localizedClarification(
			breakdown,
			"POC 验证和漏洞修复需要一个明确的 CVE 编号，请提供单个 CVE（例如 CVE-2023-29484）。",
			"POC verification and remediation require one exact CVE ID.",
		)}, nil
	}
	cveID := cveIDs[0]

	scenario, hostIDs, selectors := classifyAssetScope(breakdown)
	steps := make([]ToolPlanStep, 0, 4)
	executeFromPreviousStep := false
	switch scenario {
	case "exact_uuids":
		hostIDs = dedupeStrings(hostIDs)
	case "selector":
		if !containsExactString(input.AcceptedTools, "Host.Resolve") {
			return &WorkflowCompileResult{Clarification: localizedClarification(
				breakdown,
				"无法解析目标主机。请提供精确的主机 UUID，或允许先解析主机名/IP。",
				"Unable to resolve the target host. Provide an exact host UUID or allow hostname/IP resolution.",
			)}, nil
		}
		steps = append(steps, c.buildResolveStep(input, selectors))
		executeFromPreviousStep = true
	default:
		return &WorkflowCompileResult{Clarification: localizedClarification(
			breakdown,
			"POC 验证和漏洞修复需要明确的目标主机 UUID、主机名或 IP。",
			"POC verification and remediation require an exact host UUID, hostname, or IP.",
		)}, nil
	}

	maxRounds, err := vulnerabilityRemediationMaxRounds(breakdown)
	if err != nil {
		return nil, newCompilePlanError(
			"arguments",
			"Vulnerability.Script.Execute",
			"max_rounds",
			err.Error(),
		)
	}
	steps = append(steps,
		c.buildGenerateStep(input, cveID),
		c.buildStatusStep(input, cveID),
		c.buildExecuteStep(
			input,
			cveID,
			hostIDs,
			executeFromPreviousStep,
			hasVulnerabilityRemediationAction(breakdown),
			maxRounds,
		),
	)
	return &WorkflowCompileResult{Steps: steps}, nil
}

func (c *VulnerabilityRemediationCompiler) buildResolveStep(input WorkflowCompileInput, selectors []string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Host.Resolve")
	return ToolPlanStep{
		ToolName:   "Host.Resolve",
		Capability: "resolve_hosts",
		Args: map[string]interface{}{
			"host_selectors": dedupeStrings(selectors),
			"require_online": true,
		},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Resolve POC/remediation targets to exact online host UUIDs.",
		ArgSources: map[string]ArgSource{
			"host_selectors": {SourceType: "user_message", SourceRef: "host_selector", Confidence: 0.95},
			"require_online": {SourceType: "policy_default", SourceRef: "vulnerability_script_targets", Confidence: 1},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
	}
}

func (c *VulnerabilityRemediationCompiler) buildGenerateStep(input WorkflowCompileInput, cveID string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Vulnerability.Script.Generate")
	return ToolPlanStep{
		ToolName:         "Vulnerability.Script.Generate",
		Capability:       "generate_vulnerability_script",
		Args:             map[string]interface{}{"cve_id": cveID, "script_type": "poc"},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Generate the exact-CVE POC script before dispatching any host task.",
		ArgSources: map[string]ArgSource{
			"cve_id":      {SourceType: "user_message", SourceRef: "cve_id", Confidence: 1},
			"script_type": {SourceType: "intent_action", SourceRef: "poc_verification", Confidence: 1},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
	}
}

func (c *VulnerabilityRemediationCompiler) buildStatusStep(input WorkflowCompileInput, cveID string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Vulnerability.Script.Status")
	return ToolPlanStep{
		ToolName:         "Vulnerability.Script.Status",
		Capability:       "get_vulnerability_script_status",
		Args:             map[string]interface{}{"cve_id": cveID, "script_type": "poc"},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Wait until the POC script is generated before dispatching it.",
		ArgSources: map[string]ArgSource{
			"cve_id":      {SourceType: "user_message", SourceRef: "cve_id", Confidence: 1},
			"script_type": {SourceType: "intent_action", SourceRef: "poc_verification", Confidence: 1},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
	}
}

func (c *VulnerabilityRemediationCompiler) buildExecuteStep(input WorkflowCompileInput, cveID string, hostIDs []string, fromPreviousStep, autoVerify bool, maxRounds int) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Vulnerability.Script.Execute")
	args := map[string]interface{}{
		"cve_id":      cveID,
		"script_type": "poc",
		"auto_verify": autoVerify,
		"max_rounds":  maxRounds,
	}
	autoVerifySource := "poc_verification"
	if autoVerify {
		autoVerifySource = "remediation"
	}
	argSources := map[string]ArgSource{
		"cve_id":      {SourceType: "user_message", SourceRef: "cve_id", Confidence: 1},
		"script_type": {SourceType: "intent_action", SourceRef: "poc_verification", Confidence: 1},
		"auto_verify": {SourceType: "intent_action", SourceRef: autoVerifySource, Confidence: 1},
		"max_rounds":  {SourceType: "policy_default", SourceRef: "vulnerability_auto_verify", Confidence: 1},
	}
	condition := ""
	if fromPreviousStep {
		argSources["host_ids"] = ArgSource{SourceType: "previous_step", SourceRef: "host", Confidence: 0.5}
		condition = "requires previous step to produce: host_ids"
	} else {
		args["host_ids"] = dedupeStrings(hostIDs)
		argSources["host_ids"] = ArgSource{SourceType: "intent_scope", SourceRef: "host_uuid", Confidence: 0.95}
	}
	return ToolPlanStep{
		ToolName:         "Vulnerability.Script.Execute",
		Capability:       "execute_vulnerability_host_scripts",
		Args:             args,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Dispatch the generated POC and let the backend fix and re-verify only when the POC confirms the vulnerability.",
		ArgSources:       argSources,
		Preconditions:    contract.Preconditions,
		Postconditions:   contract.Postconditions,
		Condition:        condition,
	}
}

func hasVulnerabilityPOCOrRemediationAction(breakdown *IntentBreakdown) bool {
	return hasVulnerabilityPOCAction(breakdown) || hasVulnerabilityRemediationAction(breakdown)
}

func hasVulnerabilityPOCAction(breakdown *IntentBreakdown) bool {
	if breakdown == nil {
		return false
	}
	for _, action := range breakdown.Actions {
		switch normalizeExposureIdentifier(action) {
		case "poc", "poc_verification", "poc_verify", "verification", "verify":
			return true
		}
	}
	return false
}

func hasVulnerabilityRemediationAction(breakdown *IntentBreakdown) bool {
	if breakdown == nil {
		return false
	}
	for _, action := range breakdown.Actions {
		switch normalizeExposureIdentifier(action) {
		case "remediation", "remediate", "repair", "fix":
			return true
		}
	}
	return false
}

func exactCVEIDsFromBreakdown(breakdown *IntentBreakdown) []string {
	if breakdown == nil {
		return nil
	}
	var candidates []string
	if value, ok := breakdown.Parameters["cve_id"]; ok {
		candidates = append(candidates, toStringSlice(value)...)
	}
	if value, ok := breakdown.Parameters["cve_ids"]; ok {
		candidates = append(candidates, toStringSlice(value)...)
	}
	for _, object := range breakdown.Objects {
		candidates = append(candidates, object.ID, object.Selector, object.Type)
	}
	candidates = append(candidates, breakdown.Goal)

	var cveIDs []string
	for _, candidate := range candidates {
		for _, match := range exactCVEIDPattern.FindAllString(candidate, -1) {
			cveIDs = append(cveIDs, strings.ToUpper(match))
		}
	}
	return dedupeStrings(cveIDs)
}

func vulnerabilityRemediationMaxRounds(breakdown *IntentBreakdown) (int, error) {
	const defaultMaxRounds = 3
	if breakdown == nil {
		return defaultMaxRounds, nil
	}
	value, ok := firstIntentParameter(breakdown.Parameters, "max_rounds", "remediation_rounds", "repair_rounds")
	if !ok {
		return defaultMaxRounds, nil
	}
	maxRounds := 0
	switch typed := value.(type) {
	case float64:
		maxRounds = int(typed)
	case float32:
		maxRounds = int(typed)
	case int:
		maxRounds = typed
	case int64:
		maxRounds = int(typed)
	}
	if maxRounds < 1 || maxRounds > 10 {
		return 0, fmt.Errorf("max_rounds must be between 1 and 10")
	}
	return maxRounds, nil
}

// ---------------------------------------------------------------------------
// Detection package lifecycle compiler
// ---------------------------------------------------------------------------

// DetectionPackageLifecycleCompiler keeps each durable package lifecycle
// stage explicit. A draft-only request stops after generation; a request to
// activate detection also builds and waits at the review boundary. Signing and
// enabling remain ordered independent steps for an already identified package.
type DetectionPackageLifecycleCompiler struct{}

func (c *DetectionPackageLifecycleCompiler) WorkflowID() string {
	return detectionPackageLifecycleWorkflowID
}

func (c *DetectionPackageLifecycleCompiler) Compile(input WorkflowCompileInput) (*WorkflowCompileResult, error) {
	breakdown := input.Breakdown
	if breakdown == nil || !isDetectionPackageMutationIntent(breakdown) {
		return nil, nil
	}

	if requiresDetectionPackageDraft(breakdown) {
		requiredTools := []string{"Package.Draft.Generate"}
		if requiresDetectionPackageActivation(breakdown) {
			requiredTools = append(requiredTools, "Package.Build.Start", "Package.Build.Status")
		}
		if err := requireAcceptedTools(input.AcceptedTools, requiredTools...); err != nil {
			return nil, err
		}
		cveIDs := exactCVEIDsFromBreakdown(breakdown)
		if len(cveIDs) != 1 {
			return &WorkflowCompileResult{Clarification: localizedClarification(
				breakdown,
				"生成动态检测包需要一个明确的 CVE 编号，请提供单个 CVE（例如 CVE-2026-31431）。",
				"Generating a dynamic detection package requires one exact CVE ID.",
			)}, nil
		}
		description := detectionPackageParameterString(breakdown, "vulnerability_description")
		if description == "" {
			description = strings.TrimSpace(breakdown.Goal)
		}
		if description == "" {
			return &WorkflowCompileResult{Clarification: localizedClarification(
				breakdown,
				"请提供漏洞原理或利用链描述，以便生成动态检测包。",
				"Provide the vulnerability mechanism or exploitation chain.",
			)}, nil
		}
		steps := []ToolPlanStep{c.buildDraftStep(input, cveIDs[0], description)}
		if requiresDetectionPackageActivation(breakdown) {
			steps = append(steps,
				c.buildPreviousPackageIDStep(input, "Package.Build.Start", "Build the generated detection package draft."),
				c.buildPreviousPackageIDStep(input, "Package.Build.Status", "Wait for the generated detection package build to reach review or a terminal state."),
			)
		}
		return &WorkflowCompileResult{Steps: steps}, nil
	}

	wantsBuild := hasDetectionPackageAction(breakdown, "build", "compile")
	wantsSign := hasDetectionPackageAction(breakdown, "sign", "approve", "publish")
	wantsEnable := hasDetectionPackageAction(breakdown, "enable", "publish", "dispatch")
	if !wantsBuild && !wantsSign && !wantsEnable {
		return nil, nil
	}

	packageID := exactDetectionPackageID(breakdown)
	if packageID == "" {
		return &WorkflowCompileResult{Clarification: localizedClarification(
			breakdown,
			"构建、签名或启用检测包需要明确的 package_id。",
			"Building, signing, or enabling a detection package requires an exact package_id.",
		)}, nil
	}

	// Build is asynchronous and may enter a manual-review state. Never compile
	// sign/enable into the same run even if the model inferred those future
	// actions; a later explicit request must observe the terminal build state.
	if wantsBuild {
		if err := requireAcceptedTools(input.AcceptedTools, "Package.Build.Start", "Package.Build.Status"); err != nil {
			return nil, err
		}
		return &WorkflowCompileResult{Steps: []ToolPlanStep{
			c.buildPackageIDStep(input, "Package.Build.Start", packageID, "Start the requested detection package build."),
			c.buildPackageIDStep(input, "Package.Build.Status", packageID, "Wait for the detection package build to reach a terminal state."),
		}}, nil
	}

	steps := make([]ToolPlanStep, 0, 2)
	if wantsSign {
		if err := requireAcceptedTools(input.AcceptedTools, "Package.Sign"); err != nil {
			return nil, err
		}
		steps = append(steps, c.buildPackageIDStep(input, "Package.Sign", packageID, "Sign the successfully built detection package."))
	}
	if wantsEnable {
		if err := requireAcceptedTools(input.AcceptedTools, "Package.Enable"); err != nil {
			return nil, err
		}
		steps = append(steps, c.buildPackageIDStep(input, "Package.Enable", packageID, "Enable and distribute the signed detection package."))
	}
	return &WorkflowCompileResult{Steps: steps}, nil
}

func (c *DetectionPackageLifecycleCompiler) buildDraftStep(input WorkflowCompileInput, cveID, description string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName("Package.Draft.Generate")
	args := map[string]interface{}{
		"cve_id":                    cveID,
		"vulnerability_description": description,
	}
	sources := map[string]ArgSource{
		"cve_id":                    {SourceType: "user_message", SourceRef: "cve_id", Confidence: 1},
		"vulnerability_description": {SourceType: "user_message", SourceRef: "vulnerability_description", Confidence: 1},
	}
	for _, key := range []string{"attack_prerequisites", "exploitation_chain", "false_positive_constraints"} {
		if value := detectionPackageParameterString(input.Breakdown, key); value != "" {
			args[key] = value
			sources[key] = ArgSource{SourceType: "intent_parameters", SourceRef: key, Confidence: 0.9}
		}
	}
	return ToolPlanStep{
		ToolName:         "Package.Draft.Generate",
		Capability:       "generate_detection_package_draft",
		Args:             args,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           "Generate the package draft before any build, signing, or distribution stage.",
		ArgSources:       sources,
		Preconditions:    contract.Preconditions,
		Postconditions:   contract.Postconditions,
	}
}

func (c *DetectionPackageLifecycleCompiler) buildPackageIDStep(input WorkflowCompileInput, toolName, packageID, reason string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName(toolName)
	return ToolPlanStep{
		ToolName:         toolName,
		Capability:       contract.Capability,
		Args:             map[string]interface{}{"package_id": packageID},
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           reason,
		ArgSources: map[string]ArgSource{
			"package_id": {SourceType: "intent_object", SourceRef: "package_id", Confidence: 1},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
	}
}

func (c *DetectionPackageLifecycleCompiler) buildPreviousPackageIDStep(input WorkflowCompileInput, toolName, reason string) ToolPlanStep {
	contract, _ := input.Mapper.ContractForToolName(toolName)
	return ToolPlanStep{
		ToolName:         toolName,
		Capability:       contract.Capability,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           reason,
		ArgSources: map[string]ArgSource{
			"package_id": {SourceType: "previous_step", SourceRef: "detection_package", Confidence: 1},
		},
		Preconditions:  contract.Preconditions,
		Postconditions: contract.Postconditions,
		Condition:      "requires previous step to produce: package_id",
	}
}

func requireAcceptedTools(accepted []string, toolNames ...string) error {
	for _, toolName := range toolNames {
		if !containsExactString(accepted, toolName) {
			return newCompilePlanError(
				"mapping_compile",
				toolName,
				"accepted_tools",
				fmt.Sprintf("detection package lifecycle requires authorized tool %s", toolName),
			)
		}
	}
	return nil
}

func exactDetectionPackageID(breakdown *IntentBreakdown) string {
	if breakdown == nil {
		return ""
	}
	if value := detectionPackageParameterString(breakdown, "package_id"); value != "" && !isExactCVEIdentifier(value) {
		return value
	}
	for _, object := range breakdown.Objects {
		switch normalizeExposureIdentifier(object.Type) {
		case "detection_package", "dynamic_detection_package", "runtime_detection_package", "package":
		default:
			continue
		}
		if value := strings.TrimSpace(object.ID); value != "" && !isExactCVEIdentifier(value) {
			return value
		}
	}
	for _, value := range breakdown.Scope.ObjectIDs {
		value = strings.TrimSpace(value)
		if value != "" && !isExactCVEIdentifier(value) {
			return value
		}
	}
	return ""
}

func detectionPackageParameterString(breakdown *IntentBreakdown, key string) string {
	if breakdown == nil {
		return ""
	}
	value, ok := breakdown.Parameters[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

// classifyAssetScope determines which compilation scenario applies from the
// intent breakdown. It returns the scenario name, any exact host UUIDs, and
// any non-UUID host selectors.
func classifyAssetScope(breakdown *IntentBreakdown) (scenario string, hostIDs []string, selectors []string) {
	if breakdown == nil {
		return "ambiguous", nil, nil
	}
	scopeKind := strings.ToLower(strings.TrimSpace(breakdown.Scope.Kind))

	// Scenario A: explicit all-hosts business scope.
	if isAllHostsScopeKind(scopeKind) {
		// If exact UUIDs are also present they take precedence (Scenario B).
		if uuids := filterUUIDs(breakdown.Scope.ObjectIDs); len(uuids) > 0 && len(uuids) == len(breakdown.Scope.ObjectIDs) {
			return "exact_uuids", uuids, nil
		}
		return "all_hosts", nil, nil
	}

	// Scenario B: exact host UUIDs in scope.object_ids.
	if uuids := filterUUIDs(breakdown.Scope.ObjectIDs); len(uuids) > 0 && len(uuids) == len(breakdown.Scope.ObjectIDs) {
		return "exact_uuids", uuids, nil
	}

	// Scenario C: non-UUID selectors (hostname/IP) in scope.object_ids.
	if nonUUIDSelectors := filterNonUUIDs(breakdown.Scope.ObjectIDs); len(nonUUIDSelectors) > 0 {
		return "selector", nil, nonUUIDSelectors
	}

	// Scenario C: host objects with selectors.
	for _, obj := range breakdown.Objects {
		objType := strings.ToLower(strings.TrimSpace(obj.Type))
		switch objType {
		case "host", "machine", "server", "endpoint":
		default:
			continue
		}
		if obj.ID != "" {
			if _, err := uuid.Parse(obj.ID); err == nil {
				hostIDs = append(hostIDs, obj.ID)
			} else {
				// Intent decomposition may use Selector to describe the ID kind
				// (for example selector="ip_address"). The non-UUID ID itself
				// is the concrete hostname/IP that Host.Resolve must receive.
				selectors = append(selectors, obj.ID)
			}
		} else if strings.TrimSpace(obj.Selector) != "" {
			selectors = append(selectors, obj.Selector)
		}
	}
	if len(hostIDs) > 0 && len(selectors) == 0 {
		return "exact_uuids", hostIDs, nil
	}
	if len(selectors) > 0 {
		return "selector", nil, selectors
	}

	// Fallback: check parameters for host_ids or host_selectors.
	if value, ok := breakdown.Parameters["host_ids"]; ok {
		if ids := toStringSlice(value); len(ids) > 0 {
			uuids := filterUUIDs(ids)
			if len(uuids) == len(ids) {
				return "exact_uuids", uuids, nil
			}
			return "selector", nil, filterNonUUIDs(ids)
		}
	}
	if value, ok := breakdown.Parameters["host_selectors"]; ok {
		if sels := toStringSlice(value); len(sels) > 0 {
			return "selector", nil, sels
		}
	}

	return "ambiguous", nil, nil
}

func isAllHostsScopeKind(kind string) bool {
	switch kind {
	case "all", "all_hosts", "all_online_hosts", "all_live_hosts", "all_alive_hosts",
		"online_hosts", "live_hosts", "alive_hosts":
		return true
	default:
		return isOnlineHostSelectorAlias(kind)
	}
}

func filterUUIDs(values []string) []string {
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, err := uuid.Parse(value); err == nil {
			result = append(result, value)
		}
	}
	return dedupeStrings(result)
}

func filterNonUUIDs(values []string) []string {
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, err := uuid.Parse(value); err != nil && !isExactCVEIdentifier(value) {
			result = append(result, value)
		}
	}
	return dedupeStrings(result)
}

func isExactCVEIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	match := exactCVEIDPattern.FindString(value)
	return match != "" && strings.EqualFold(match, value)
}

func toStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{typed}
	default:
		return nil
	}
}

func localizedClarification(breakdown *IntentBreakdown, zh, en string) string {
	if breakdown != nil && containsChineseBreakdown(breakdown) {
		return zh
	}
	// Default to Chinese for user-facing clarification; callers may override.
	return zh + "\n" + en
}

func containsChineseBreakdown(breakdown *IntentBreakdown) bool {
	if breakdown == nil {
		return false
	}
	if containsHan(breakdown.Goal) {
		return true
	}
	for _, obj := range breakdown.Objects {
		if containsHan(obj.Type) || containsHan(obj.Selector) {
			return true
		}
	}
	return false
}

// CompilePlanError is a structured compile-time failure. It is produced when a
// compiled plan violates schema, dependency, or completion-contract rules
// before the plan reaches the runtime.
type CompilePlanError struct {
	Code       string `json:"code"`
	Stage      string `json:"stage"`
	WorkflowID string `json:"workflow_id,omitempty"`
	StepID     string `json:"step_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	Field      string `json:"field,omitempty"`
	Reason     string `json:"reason"`
}

func (e *CompilePlanError) Error() string {
	return fmt.Sprintf("compiled plan invalid: code=%s stage=%s tool=%s field=%s reason=%s",
		e.Code, e.Stage, e.ToolName, e.Field, e.Reason)
}

func newCompilePlanError(stage, toolName, field, reason string) *CompilePlanError {
	return &CompilePlanError{
		Code:     "compiled_plan_invalid",
		Stage:    stage,
		ToolName: toolName,
		Field:    field,
		Reason:   reason,
	}
}

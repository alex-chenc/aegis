package assistant

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	toolDecisionAccepted              = "accepted"
	toolDecisionRejected              = "rejected"
	toolDecisionClarificationRequired = "clarification_required"
)

type ToolDecisionConfig struct {
	Enabled                    bool
	TraceEnabled               bool
	ClarificationRequiredWrite bool
	PostconditionCheckEnabled  bool
	DryRunForWrite             bool
}

func DefaultToolDecisionConfigFromEnv() ToolDecisionConfig {
	return ToolDecisionConfig{
		Enabled:                    envBool("ASSISTANT_TOOL_DECISION_ENGINE_ENABLED", true),
		TraceEnabled:               envBool("ASSISTANT_TOOL_DECISION_TRACE", false),
		ClarificationRequiredWrite: envBool("ASSISTANT_CLARIFICATION_REQUIRED_FOR_WRITE", true),
		PostconditionCheckEnabled:  envBool("ASSISTANT_TOOL_POSTCONDITION_CHECK_ENABLED", true),
		DryRunForWrite:             envBool("ASSISTANT_TOOL_DRY_RUN_FOR_WRITE", false),
	}
}

type ToolDecisionEngineDeps struct {
	Registry *ToolRegistry
	Mapper   *ToolCapabilityMapper
	Config   ToolDecisionConfig
	Logger   *zap.Logger
}

type ToolDecisionEngine struct {
	registry          *ToolRegistry
	mapper            *ToolCapabilityMapper
	config            ToolDecisionConfig
	clarificationGate *ClarificationGate
	logger            *zap.Logger
}

type ToolDecisionInput struct {
	Query                string               `json:"query"`
	Intent               IntentResult         `json:"intent"`
	Breakdown            *IntentBreakdown     `json:"intent_breakdown,omitempty"`
	ContextRefs          []ContextRefInput    `json:"context_refs,omitempty"`
	PreliminarySelection *ToolSelectionResult `json:"preliminary_selection,omitempty"`
	UseAIAnalysisFlow    bool                 `json:"use_ai_analysis_flow,omitempty"`
}

func NewToolDecisionEngine(deps ToolDecisionEngineDeps) *ToolDecisionEngine {
	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	config := deps.Config
	if deps.Mapper == nil {
		deps.Mapper = NewToolCapabilityMapper(deps.Registry)
	}
	return &ToolDecisionEngine{
		registry:          deps.Registry,
		mapper:            deps.Mapper,
		config:            config,
		clarificationGate: NewClarificationGate(config, logger),
		logger:            logger,
	}
}

func (e *ToolDecisionEngine) Decide(ctx context.Context, input ToolDecisionInput) (*ToolExecutionPlan, error) {
	_ = ctx
	if e == nil || e.registry == nil || e.mapper == nil {
		return nil, fmt.Errorf("tool decision engine dependencies unavailable")
	}
	breakdown := input.Breakdown
	if breakdown == nil {
		return nil, fmt.Errorf("llm intent breakdown is required")
	}
	if err := validateIntentBreakdown(breakdown); err != nil {
		return nil, fmt.Errorf("invalid llm intent breakdown: %w", err)
	}

	traceID := "td_" + strings.ReplaceAll(uuid.New().String()[:13], "-", "")
	candidateNames, unmatchedCapabilities := e.recallCandidateToolNames(input, breakdown)
	records := make([]ToolDecisionRecord, 0, len(candidateNames)+len(unmatchedCapabilities))
	rejectedRecords := make([]ToolDecisionRecord, 0)

	for _, capability := range unmatchedCapabilities {
		record := ToolDecisionRecord{
			TraceID:    traceID,
			Capability: capability,
			Decision:   toolDecisionRejected,
			Reason:     "candidate capability is not mapped to any enabled tool",
			HardGateResults: []HardGateResult{{
				Name:   "capability_mapped",
				Passed: false,
				Reason: "no enabled tool exposes this capability",
			}},
		}
		records = append(records, record)
		rejectedRecords = append(rejectedRecords, record)
	}

	accepted := make([]string, 0, len(candidateNames))
	clarificationQuestion := ""
	if breakdown.NeedClarification {
		clarificationQuestion = breakdown.ClarifyingQuestion
	}
	for _, name := range candidateNames {
		record, accept := e.evaluateCandidate(name, input, breakdown)
		record.TraceID = traceID
		records = append(records, record)
		if accept {
			accepted = append(accepted, name)
			continue
		}
		rejectedRecords = append(rejectedRecords, record)
		if record.Decision == toolDecisionClarificationRequired && clarificationQuestion == "" {
			clarificationQuestion = record.Reason
		}
	}

	if breakdown.NeedClarification && clarificationQuestion == "" {
		clarificationQuestion = breakdown.ClarifyingQuestion
	}
	// 使用 ClarificationGate 评估是否需要追问
	clarification := e.clarificationGate.Evaluate(breakdown, accepted, records)
	if clarification.Required || (clarificationQuestion != "" && shouldStopForClarification(breakdown, accepted, e.registry)) {
		question := clarificationQuestion
		if question == "" {
			question = clarification.Question
		}
		e.logger.Info("assistant tool authorization requires clarification",
			zap.String("trace_id", traceID),
			zap.String("action", input.Intent.Action),
			zap.Strings("domains", input.Intent.Domains),
			zap.String("reason", question),
			zap.String("source", clarification.Source),
		)
		return &ToolExecutionPlan{
			Goal:                breakdown.Goal,
			NeedClarification:   true,
			ClarifyingQuestion:  question,
			EvidencePolicy:      defaultEvidencePolicy(e.config.PostconditionCheckEnabled),
			DecisionTraceID:     traceID,
			DecisionRecords:     records,
			RejectedToolRecords: rejectedRecords,
		}, nil
	}

	// This layer is an authorization boundary, not a workflow planner. Preserve
	// the LLM-selected set exactly; agent-runtime is the only component allowed
	// to order, repeat or conditionally execute these tools.
	steps := e.buildPlanSteps(dedupeStrings(accepted), input, breakdown)

	e.logger.Info("assistant tool authorization completed",
		zap.String("trace_id", traceID),
		zap.String("authorization_mode", "mapping_hard_gates"),
		zap.Strings("selected_capabilities", breakdown.CandidateCapabilities),
		zap.Int("candidate_count", len(candidateNames)),
		zap.Int("authorized_count", len(steps)),
		zap.Bool("need_clarification", false),
	)
	return &ToolExecutionPlan{
		Goal:                breakdown.Goal,
		NeedClarification:   false,
		Steps:               steps,
		EvidencePolicy:      defaultEvidencePolicy(e.config.PostconditionCheckEnabled),
		DecisionTraceID:     traceID,
		DecisionRecords:     records,
		RejectedToolRecords: rejectedRecords,
	}, nil
}

func (e *ToolDecisionEngine) buildPlanSteps(names []string, input ToolDecisionInput, breakdown *IntentBreakdown) []ToolPlanStep {
	steps := make([]ToolPlanStep, 0, len(names))
	for _, name := range names {
		if step, ok := e.newToolPlanStep(name, input, breakdown); ok {
			step.StepID = fmt.Sprintf("authorized_%02d", len(steps)+1)
			steps = append(steps, step)
		}
	}
	return steps
}

func (e *ToolDecisionEngine) newToolPlanStep(name string, input ToolDecisionInput, breakdown *IntentBreakdown) (ToolPlanStep, bool) {
	contract, ok := e.mapper.ContractForToolName(name)
	if !ok {
		return ToolPlanStep{}, false
	}
	tool, _ := e.registry.Get(name)
	args, argSources := bindPlanArgs(contract, input, breakdown)
	reason := decisionStepReason(tool, contract, input, breakdown)
	return ToolPlanStep{
		ToolName:         name,
		Capability:       contract.Capability,
		Args:             args,
		Risk:             contract.Risk,
		RequiresApproval: contract.RequiresApproval,
		Reason:           reason,
		ArgSources:       argSources,
		Preconditions:    contract.Preconditions,
		Postconditions:   contract.Postconditions,
	}, true
}

func (e *ToolDecisionEngine) recallCandidateToolNames(input ToolDecisionInput, breakdown *IntentBreakdown) ([]string, []string) {
	names := make([]string, 0, 32)
	if input.Intent.ExplicitToolName != "" {
		names = append(names, input.Intent.ExplicitToolName)
	}
	mapped := e.mapper.ToolNamesForCapabilities(breakdown.CandidateCapabilities)
	names = append(names, mapped...)
	names = append(names, e.mapper.ReadonlyCompanionToolNames(mapped)...)

	capabilityMatched := make(map[string]bool, len(breakdown.CandidateCapabilities))
	for _, name := range mapped {
		if contract, ok := e.mapper.ContractForToolName(name); ok {
			capabilityMatched[strings.ToLower(contract.Capability)] = true
		}
	}
	unmatched := make([]string, 0)
	for _, capability := range breakdown.CandidateCapabilities {
		key := strings.ToLower(strings.TrimSpace(capability))
		if key != "" && !capabilityMatched[key] && len(e.mapper.ToolNamesForCapabilities([]string{capability})) == 0 {
			unmatched = append(unmatched, capability)
		}
	}

	return dedupeStrings(names), dedupeStrings(unmatched)
}

func (e *ToolDecisionEngine) evaluateCandidate(name string, input ToolDecisionInput, breakdown *IntentBreakdown) (ToolDecisionRecord, bool) {
	tool, ok := e.registry.Get(name)
	if !ok || tool == nil {
		return ToolDecisionRecord{
			ToolName: name,
			Decision: toolDecisionRejected,
			Reason:   "tool is not registered",
			HardGateResults: []HardGateResult{{
				Name:   "tool_registered",
				Passed: false,
				Reason: "tool does not exist in registry",
			}},
		}, false
	}
	contract := BuildToolUseContract(tool)
	record := ToolDecisionRecord{
		ToolName:      name,
		Capability:    contract.Capability,
		RequiresWrite: contract.RequiresExplicitUserIntent || tool.Risk != ToolRiskReadonly,
		ArgSources:    map[string]ArgSource{},
	}
	gates := []HardGateResult{
		{Name: "tool_registered", Passed: true},
		{Name: "tool_enabled", Passed: tool.Enabled},
	}
	if !tool.Enabled {
		record.Decision = toolDecisionRejected
		record.Reason = "tool is disabled"
		record.HardGateResults = gates
		return record, false
	}
	// 检查 denied_intents：LLM 候选能力命中 denied_intents 时拒绝
	if denied, reason := e.checkDeniedIntents(contract, breakdown); denied {
		gates = append(gates, HardGateResult{Name: "denied_capability", Passed: false, Reason: reason})
		record.Decision = toolDecisionRejected
		record.Reason = reason
		record.HardGateResults = gates
		return record, false
	}
	gates = append(gates, HardGateResult{Name: "denied_capability", Passed: true})

	if contract.RequiresExplicitUserIntent && !breakdown.RequiresWrite {
		gates = append(gates, HardGateResult{Name: "explicit_write_intent", Passed: false, Reason: "write tool requires explicit user intent"})
		record.Decision = toolDecisionRejected
		record.Reason = "write tool requires explicit user intent"
		record.HardGateResults = gates
		return record, false
	}
	gates = append(gates, HardGateResult{Name: "explicit_write_intent", Passed: true})

	if missing, question := e.missingRequiredEntity(contract, input, breakdown); missing {
		gates = append(gates, HardGateResult{Name: "required_entities", Passed: false, Reason: question})
		record.Decision = toolDecisionClarificationRequired
		record.Reason = question
		record.HardGateResults = gates
		return record, false
	}
	gates = append(gates, HardGateResult{Name: "required_entities", Passed: true})

	record.Decision = toolDecisionAccepted
	record.Reason = "exact capability mapping passed authorization hard gates"
	record.HardGateResults = gates
	record.ApprovalState = approvalStateForContract(contract)
	_, sources := bindPlanArgs(contract, input, breakdown)
	record.ArgSources = sources
	return record, true
}

// checkDeniedIntents 检查 LLM 候选能力是否命中工具的 denied_intents 或 negative_cases。
// 返回 (是否拒绝, 拒绝原因)。
func (e *ToolDecisionEngine) checkDeniedIntents(contract ToolUseContract, breakdown *IntentBreakdown) (bool, string) {
	if breakdown == nil {
		return false, ""
	}
	// 检查 denied_intents：候选能力与 denied_intents 匹配时拒绝
	for _, capability := range breakdown.CandidateCapabilities {
		for _, denied := range contract.DeniedIntents {
			if strings.EqualFold(strings.TrimSpace(capability), strings.TrimSpace(denied)) {
				return true, fmt.Sprintf("candidate capability %q matches denied intent %q", capability, denied)
			}
		}
	}
	// 检查 denied_intents 与 allowed_intents 的交叉：如果 breakdown 的 action 对应的意图在 denied 列表中
	action := firstAction(breakdown, IntentResult{})
	if action != "" {
		intentKey := action + "_" + contract.Domain
		for _, denied := range contract.DeniedIntents {
			if strings.EqualFold(intentKey, denied) {
				return true, fmt.Sprintf("action %q on domain %q matches denied intent %q", action, contract.Domain, denied)
			}
		}
	}
	return false, ""
}

func (e *ToolDecisionEngine) missingRequiredEntity(contract ToolUseContract, input ToolDecisionInput, breakdown *IntentBreakdown) (bool, string) {
	if len(contract.RequiredEntities) == 0 {
		return false, ""
	}
	for _, entity := range contract.RequiredEntities {
		satisfied := false
		for _, alternative := range strings.Split(entity, "|") {
			alternative = strings.TrimSpace(alternative)
			binding, ok := bindingForEntity(contract.ArgBindings, alternative)
			if !ok {
				binding = ArgBindingRule{
					ArgName:     alternative,
					Entity:      inferEntityFromArgName(alternative),
					SourceOrder: []string{"user_message", "page_context", "session_context", "previous_step"},
					Required:    true,
				}
			}
			if value, _ := resolveArgBySourceOrderWithoutPreviousStep(binding, input, breakdown); value != nil ||
				canResolveDuringRuntime(binding, input, breakdown) {
				satisfied = true
				break
			}
		}
		if satisfied || isResidentTool(contract.ToolName) {
			continue
		}
		return true, fmt.Sprintf("请补充执行所需参数 %s，或提供可用于查询该参数的上下文。", entity)
	}
	return false, ""
}

func bindingForEntity(bindings []ArgBindingRule, entity string) (ArgBindingRule, bool) {
	for _, binding := range bindings {
		if strings.EqualFold(binding.ArgName, entity) || strings.EqualFold(binding.Entity, entity) {
			return binding, true
		}
	}
	return ArgBindingRule{}, false
}

func canResolveDuringRuntime(binding ArgBindingRule, input ToolDecisionInput, breakdown *IntentBreakdown) bool {
	hasPreviousStepSource := false
	hasSessionSource := false
	for _, source := range binding.SourceOrder {
		switch strings.ToLower(strings.TrimSpace(source)) {
		case "previous_step":
			hasPreviousStepSource = true
		case "session_context":
			hasSessionSource = true
		}
	}
	if hasSessionSource && len(input.ContextRefs) > 0 {
		return true
	}
	if !hasPreviousStepSource {
		return false
	}
	if breakdown != nil {
		if len(dedupeStrings(breakdown.CandidateCapabilities)) > 1 {
			return true
		}
		if breakdown.Scope.Kind != "" && breakdown.Scope.Kind != "unspecified" {
			return true
		}
	}
	return len(input.ContextRefs) > 0
}

func hasBreakdownEntity(breakdown *IntentBreakdown, entity string) bool {
	if breakdown == nil {
		return false
	}
	objectType := strings.TrimSuffix(strings.TrimSpace(entity), "_id")
	for _, object := range breakdown.Objects {
		if strings.EqualFold(object.Type, objectType) && strings.TrimSpace(object.ID) != "" {
			return true
		}
	}
	return false
}

func defaultEvidencePolicy(enabled bool) EvidencePolicy {
	return EvidencePolicy{
		RequireToolEvidence:     true,
		RequirePostcondition:    enabled,
		MissingEvidenceBehavior: "report_gap",
	}
}

func bindPlanArgs(contract ToolUseContract, input ToolDecisionInput, breakdown *IntentBreakdown) (map[string]interface{}, map[string]ArgSource) {
	args := make(map[string]interface{})
	sources := make(map[string]ArgSource)

	// Parameter binding is driven only by the tool's generic contract.
	for _, binding := range contract.ArgBindings {
		value, source := resolveArgBySourceOrder(binding, input, breakdown)
		if value != nil {
			args[binding.ArgName] = value
		}
		if source.SourceType != "" {
			sources[binding.ArgName] = source
		}
	}
	return args, sources
}

// resolveArgBySourceOrder 按 ArgBindingRule.SourceOrder 优先级依次尝试解析参数值。
func resolveArgBySourceOrder(binding ArgBindingRule, input ToolDecisionInput, breakdown *IntentBreakdown) (interface{}, ArgSource) {
	for _, sourceType := range binding.SourceOrder {
		switch sourceType {
		case "page_context":
			if ids := contextRefIDs(input.ContextRefs, binding.Entity); len(ids) > 0 {
				if len(ids) == 1 {
					return ids[0], ArgSource{SourceType: "page_context", SourceRef: binding.Entity, Confidence: 0.95}
				}
				return ids, ArgSource{SourceType: "page_context", SourceRef: binding.Entity, Confidence: 0.95}
			}
		case "user_message":
			if value, ref := extractArgFromBreakdown(binding, breakdown); value != nil {
				return value, ArgSource{SourceType: "user_message", SourceRef: ref, Confidence: 0.8}
			}
		case "policy_default":
			if binding.DefaultPolicy != "" {
				return binding.DefaultPolicy, ArgSource{SourceType: "policy_default", SourceRef: binding.DefaultPolicy, Confidence: 0.7}
			}
		case "previous_step":
			// previous_step 由调用方在执行时动态注入，此处记录来源意图
			return nil, ArgSource{SourceType: "previous_step", SourceRef: binding.Entity, Confidence: 0.5}
		}
	}
	return nil, ArgSource{}
}

func resolveArgBySourceOrderWithoutPreviousStep(binding ArgBindingRule, input ToolDecisionInput, breakdown *IntentBreakdown) (interface{}, ArgSource) {
	filtered := binding
	filtered.SourceOrder = make([]string, 0, len(binding.SourceOrder))
	for _, source := range binding.SourceOrder {
		if strings.EqualFold(strings.TrimSpace(source), "previous_step") {
			continue
		}
		filtered.SourceOrder = append(filtered.SourceOrder, source)
	}
	return resolveArgBySourceOrder(filtered, input, breakdown)
}

// extractArgFromBreakdown 从 IntentBreakdown 中提取与 binding 匹配的参数值。
func extractArgFromBreakdown(binding ArgBindingRule, breakdown *IntentBreakdown) (interface{}, string) {
	if breakdown == nil {
		return nil, ""
	}
	entity := strings.ToLower(binding.Entity)
	for _, key := range []string{binding.ArgName, binding.Entity} {
		if value, ok := breakdown.Parameters[key]; ok && value != nil {
			return value, "parameter:" + key
		}
	}
	// 从 Objects 中匹配
	for _, obj := range breakdown.Objects {
		if strings.EqualFold(obj.Type, entity) || strings.Contains(strings.ToLower(obj.Type), entity) {
			if obj.ID != "" {
				return obj.ID, "object:" + obj.Type
			}
		}
	}
	// 从 Scope 中推断
	if breakdown.Scope.Kind != "" && breakdown.Scope.Kind != "unspecified" {
		if entity == "host" || entity == "scope" {
			if len(breakdown.Scope.ObjectIDs) == 1 {
				return breakdown.Scope.ObjectIDs[0], "scope:object_ids"
			}
			if len(breakdown.Scope.ObjectIDs) > 1 {
				return append([]string{}, breakdown.Scope.ObjectIDs...), "scope:object_ids"
			}
			return breakdown.Scope.Kind, "scope:" + breakdown.Scope.Kind
		}
	}
	return nil, ""
}

func decisionStepReason(tool *ToolSpec, contract ToolUseContract, input ToolDecisionInput, breakdown *IntentBreakdown) string {
	_ = input
	if tool == nil {
		return "The tool passed deterministic authorization hard gates."
	}
	if hasCapability(breakdown, contract.Capability) {
		return "The exact intent capability mapped to this tool and passed authorization hard gates."
	}
	return "The tool is a declared read-only companion or an explicitly requested tool and passed authorization hard gates."
}

func shouldStopForClarification(breakdown *IntentBreakdown, accepted []string, registry *ToolRegistry) bool {
	_ = registry
	if breakdown == nil {
		return false
	}
	if breakdown.NeedClarification && breakdown.RequiresWrite {
		return true
	}
	if len(accepted) == 0 {
		return true
	}
	return breakdown.NeedClarification
}

func approvalStateForContract(contract ToolUseContract) string {
	if contract.RequiresApproval {
		return "required"
	}
	return "not_required"
}

func hasCapability(breakdown *IntentBreakdown, capability string) bool {
	if breakdown == nil {
		return false
	}
	for _, candidate := range breakdown.CandidateCapabilities {
		if strings.EqualFold(candidate, capability) {
			return true
		}
	}
	return false
}

func firstAction(breakdown *IntentBreakdown, intent IntentResult) string {
	if breakdown != nil && len(breakdown.Actions) > 0 && breakdown.Actions[0] != "" {
		return breakdown.Actions[0]
	}
	return intent.Action
}

func actionMatchesContract(action string, actions []string, op ToolOperation) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return false
	}
	for _, candidate := range actions {
		if strings.EqualFold(action, candidate) {
			return true
		}
	}
	switch action {
	case "query":
		return op == OpList || op == OpGet || op == OpSearch
	case "analyze", "investigate":
		return op == OpList || op == OpGet || op == OpSearch || op == OpGenerate
	case "execute", "collect", "scan":
		return op == OpExecute || op == OpDispatch
	case "create", "generate":
		return op == OpCreate || op == OpGenerate
	default:
		return strings.EqualFold(action, string(op))
	}
}

func objectMatchesTool(objects []IntentObject, tool *ToolSpec) bool {
	if tool == nil || len(objects) == 0 {
		return false
	}
	for _, object := range objects {
		if object.Type == "" {
			continue
		}
		if strings.EqualFold(object.Type, string(tool.Domain)) {
			return true
		}
		for _, objectType := range tool.ObjectTypes {
			if strings.EqualFold(object.Type, objectType) {
				return true
			}
		}
	}
	return false
}

func contextMatchesTool(refs []ContextRefInput, tool *ToolSpec) bool {
	if tool == nil {
		return false
	}
	for _, ref := range refs {
		if strings.EqualFold(ref.ObjectType, string(tool.Domain)) {
			return true
		}
		for _, objectType := range tool.ObjectTypes {
			if strings.EqualFold(ref.ObjectType, objectType) {
				return true
			}
		}
	}
	return false
}

func hasContextRef(refs []ContextRefInput, objectType string) bool {
	for _, ref := range refs {
		if strings.EqualFold(ref.ObjectType, objectType) && ref.ObjectID != "" {
			return true
		}
	}
	return false
}

func hasContextForEntity(refs []ContextRefInput, entity string) bool {
	entity = strings.TrimSuffix(strings.ToLower(entity), "_id")
	for _, ref := range refs {
		if strings.Contains(strings.ToLower(ref.ObjectType), entity) && ref.ObjectID != "" {
			return true
		}
	}
	return false
}

func contextRefIDs(refs []ContextRefInput, objectType string) []string {
	ids := make([]string, 0)
	for _, ref := range refs {
		if strings.EqualFold(ref.ObjectType, objectType) && ref.ObjectID != "" {
			ids = append(ids, ref.ObjectID)
		}
	}
	return ids
}

func containsDecisionString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

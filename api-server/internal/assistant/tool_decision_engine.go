package assistant

import (
	"context"
	"fmt"
	"os"
	"sort"
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
	MinScore                   float64
	ReadonlyMinScore           float64
	PostconditionCheckEnabled  bool
	DryRunForWrite             bool
}

func DefaultToolDecisionConfigFromEnv() ToolDecisionConfig {
	return ToolDecisionConfig{
		Enabled:                    envBool("ASSISTANT_TOOL_DECISION_ENGINE_ENABLED", true),
		TraceEnabled:               envBool("ASSISTANT_TOOL_DECISION_TRACE", false),
		ClarificationRequiredWrite: envBool("ASSISTANT_CLARIFICATION_REQUIRED_FOR_WRITE", true),
		MinScore:                   envFloat("ASSISTANT_TOOL_DECISION_MIN_SCORE", 0.75),
		ReadonlyMinScore:           envFloat("ASSISTANT_TOOL_READONLY_MIN_SCORE", 0.60),
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
	if config.MinScore <= 0 {
		config.MinScore = 0.75
	}
	if config.ReadonlyMinScore <= 0 {
		config.ReadonlyMinScore = 0.60
	}
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
	if _, _, err := parseMaxRepairRounds(input.Query); err != nil {
		return nil, err
	}
	breakdown := input.Breakdown
	if breakdown == nil {
		breakdown = (&IntentDecomposer{}).decomposeByRules(IntentDecomposeInput{
			Query:       input.Query,
			Intent:      input.Intent,
			ContextRefs: input.ContextRefs,
		})
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
	acceptedSet := make(map[string]bool)
	clarificationQuestion := ""
	if breakdown.NeedClarification {
		clarificationQuestion = breakdown.ClarifyingQuestion
	}
	for _, name := range candidateNames {
		record, accept := e.evaluateCandidate(name, input, breakdown, false)
		record.TraceID = traceID
		records = append(records, record)
		if accept {
			accepted = append(accepted, name)
			acceptedSet[name] = true
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
		e.logger.Info("assistant tool decision requires clarification",
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

	finalNames := e.expandWorkflowTools(accepted, input, breakdown)
	for _, name := range finalNames {
		if acceptedSet[name] {
			continue
		}
		record, accept := e.evaluateCandidate(name, input, breakdown, true)
		record.TraceID = traceID
		records = append(records, record)
		if accept {
			acceptedSet[name] = true
			continue
		}
		rejectedRecords = append(rejectedRecords, record)
	}
	filteredNames := make([]string, 0, len(finalNames))
	for _, name := range finalNames {
		if acceptedSet[name] {
			filteredNames = append(filteredNames, name)
		}
	}
	finalNames = e.orderToolPlanNames(filteredNames)

	steps := make([]ToolPlanStep, 0, len(finalNames))
	for _, name := range finalNames {
		contract, ok := e.mapper.ContractForToolName(name)
		if !ok {
			continue
		}
		tool, _ := e.registry.Get(name)
		args, argSources := bindPlanArgs(contract, input, breakdown)
		steps = append(steps, ToolPlanStep{
			StepID:           fmt.Sprintf("step_%02d", len(steps)+1),
			ToolName:         name,
			Capability:       contract.Capability,
			Args:             args,
			Risk:             contract.Risk,
			RequiresApproval: contract.RequiresApproval,
			Reason:           decisionStepReason(tool, contract, input, breakdown),
			ArgSources:       argSources,
			Preconditions:    contract.Preconditions,
			Postconditions:   contract.Postconditions,
			OnSuccess:        contract.NextCapabilities,
		})
	}

	e.logger.Info("assistant tool decision completed",
		zap.String("trace_id", traceID),
		zap.Int("candidate_count", len(candidateNames)),
		zap.Int("selected_count", len(steps)),
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

func (e *ToolDecisionEngine) recallCandidateToolNames(input ToolDecisionInput, breakdown *IntentBreakdown) ([]string, []string) {
	names := make([]string, 0, 32)
	if input.Intent.ExplicitToolName != "" {
		names = append(names, input.Intent.ExplicitToolName)
	}
	mapped := e.mapper.ToolNamesForCapabilities(breakdown.CandidateCapabilities)
	names = append(names, mapped...)
	if sel := input.PreliminarySelection; sel != nil {
		for _, name := range sel.SelectedTools {
			contract, ok := e.mapper.ContractForToolName(name)
			if len(mapped) == 0 || (ok && hasCapability(breakdown, contract.Capability)) || strings.EqualFold(name, input.Intent.ExplicitToolName) {
				names = append(names, name)
			}
		}
	}

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

	if hasCapability(breakdown, "trigger_asset_collection") {
		names = append(names, "Host.List", "Asset.Collection.Trigger", "Asset.Collection.Get")
	}
	if shouldIncludeAssetAnalysisTools(input.Query, input.Intent, breakdown) {
		names = append(names,
			"Asset.Application.List",
			"Asset.Summary.Get",
			"Software.Installed.Search",
			"Vulnerability.List",
			"Vulnerability.AffectedHosts",
		)
	}
	if input.Intent.Action == "block" || hasCapability(breakdown, "block_detection_alert") {
		names = append(names, "Detection.Alert.Get", "Detection.Alert.Block")
	}
	return dedupeStrings(names), dedupeStrings(unmatched)
}

func (e *ToolDecisionEngine) evaluateCandidate(name string, input ToolDecisionInput, breakdown *IntentBreakdown, workflowForced bool) (ToolDecisionRecord, bool) {
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
		ToolName:   name,
		Capability: contract.Capability,
		Score:      e.scoreToolDecision(tool, contract, input, breakdown, workflowForced),
		ArgSources: map[string]ArgSource{},
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
	if strings.HasPrefix(contract.ToolName, "Vulnerability.Script.") && !hasVulnerabilityScriptOperationIntent(input.Query) {
		gates = append(gates, HardGateResult{Name: "explicit_script_intent", Passed: false, Reason: "vulnerability script tools require explicit POC or remediation intent"})
		record.Decision = toolDecisionRejected
		record.Reason = "vulnerability script tools require explicit POC or remediation intent"
		record.HardGateResults = gates
		return record, false
	}

	if isConceptQuestion(input.Query) && contract.RequiresExplicitUserIntent {
		gates = append(gates, HardGateResult{Name: "denied_intent", Passed: false, Reason: "concept explanation request must not trigger write tools"})
		record.Decision = toolDecisionRejected
		record.Reason = "concept explanation request must not trigger write tools"
		record.HardGateResults = gates
		return record, false
	}
	gates = append(gates, HardGateResult{Name: "denied_intent", Passed: true})

	// 检查 denied_intents：LLM 候选能力命中 denied_intents 时拒绝
	if denied, reason := e.checkDeniedIntents(contract, breakdown, input.Query); denied {
		gates = append(gates, HardGateResult{Name: "denied_capability", Passed: false, Reason: reason})
		record.Decision = toolDecisionRejected
		record.Reason = reason
		record.HardGateResults = gates
		return record, false
	}
	gates = append(gates, HardGateResult{Name: "denied_capability", Passed: true})

	if contract.RequiresExplicitUserIntent && !breakdown.RequiresWrite && !hasExplicitWriteIntent(input.Query) {
		gates = append(gates, HardGateResult{Name: "explicit_write_intent", Passed: false, Reason: "write tool requires explicit user intent"})
		record.Decision = toolDecisionRejected
		record.Reason = "write tool requires explicit user intent"
		record.HardGateResults = gates
		return record, false
	}
	gates = append(gates, HardGateResult{Name: "explicit_write_intent", Passed: true})

	if missing, question := e.missingRequiredEntity(contract, input, breakdown, workflowForced); missing {
		gates = append(gates, HardGateResult{Name: "required_entities", Passed: false, Reason: question})
		record.Decision = toolDecisionClarificationRequired
		record.Reason = question
		record.HardGateResults = gates
		return record, false
	}
	gates = append(gates, HardGateResult{Name: "required_entities", Passed: true})

	threshold := e.config.MinScore
	if tool.Risk == ToolRiskReadonly || tool.Risk == ToolRiskLow {
		threshold = e.config.ReadonlyMinScore
	}
	if isResidentTool(name) || workflowForced || record.Score >= threshold || preliminarySelected(input.PreliminarySelection, name) {
		record.Decision = toolDecisionAccepted
		record.Reason = "tool contract matched intent and passed hard gates"
		record.HardGateResults = gates
		record.ApprovalState = approvalStateForContract(contract)
		_, sources := bindPlanArgs(contract, input, breakdown)
		record.ArgSources = sources
		return record, true
	}

	gates = append(gates, HardGateResult{Name: "score_threshold", Passed: false, Reason: fmt.Sprintf("score %.2f below threshold %.2f", record.Score, threshold)})
	record.Decision = toolDecisionRejected
	record.Reason = fmt.Sprintf("tool score %.2f below threshold %.2f", record.Score, threshold)
	record.HardGateResults = gates
	return record, false
}

// checkDeniedIntents 检查 LLM 候选能力是否命中工具的 denied_intents 或 negative_cases。
// 返回 (是否拒绝, 拒绝原因)。
func (e *ToolDecisionEngine) checkDeniedIntents(contract ToolUseContract, breakdown *IntentBreakdown, query string) (bool, string) {
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
	// 检查 negative_cases：概念解释类查询不应触发写工具
	if len(contract.NegativeCases) > 0 && isConceptQuestion(query) && contract.RequiresExplicitUserIntent {
		return true, "概念解释类查询不应触发写操作工具（命中 negative_cases）"
	}
	return false, ""
}

func (e *ToolDecisionEngine) missingRequiredEntity(contract ToolUseContract, input ToolDecisionInput, breakdown *IntentBreakdown, workflowForced bool) (bool, string) {
	if len(contract.RequiredEntities) == 0 {
		return false, ""
	}
	for _, entity := range contract.RequiredEntities {
		switch entity {
		case "scope|host_ids":
			if breakdown.Scope.Kind != "" && breakdown.Scope.Kind != "unspecified" {
				continue
			}
			if hasContextRef(input.ContextRefs, "host") {
				continue
			}
			return true, "请确认资产采集范围，例如全部主机、在线主机或指定主机。"
		case "alert_id":
			if hasContextRef(input.ContextRefs, "alert") || looksLikeIDInText(input.Query) {
				continue
			}
			return true, "请确认要操作的告警 ID，或先在告警详情页引用这个告警。"
		case "host_ids":
			if hasContextRef(input.ContextRefs, "host") || breakdown.Scope.Kind == "online_hosts" || breakdown.Scope.Kind == "all" || workflowForced || isCVERemediationIntent(input.Query) {
				continue
			}
			return true, "请确认要操作的主机范围或主机 ID。"
		case "script_type":
			if containsAnyFold(input.Query, "poc", "修复脚本", "检测脚本", "script_type") {
				continue
			}
			return true, "请确认脚本类型，例如 POC 检测脚本或修复脚本。"
		case "vulnerability_id":
			if hasContextRef(input.ContextRefs, "vulnerability") || cveIDPattern.MatchString(input.Query) || workflowForced {
				continue
			}
			return true, "请确认要操作的漏洞或 CVE。"
		case "rule_ids":
			if hasContextRef(input.ContextRefs, "baseline_rule") || hasContextRef(input.ContextRefs, "baseline_template") || workflowForced {
				continue
			}
			return true, "请确认要执行的基线规则或模板。"
		case "task_id":
			if workflowForced || hasContextRef(input.ContextRefs, "task") || hasBreakdownEntity(breakdown, "task_id") || strings.Contains(strings.ToLower(input.Query), "task_id=") {
				continue
			}
			return true, "请确认要查询的任务 ID。"
		default:
			if workflowForced || hasContextForEntity(input.ContextRefs, entity) || hasBreakdownEntity(breakdown, entity) {
				continue
			}
			if isResidentTool(contract.ToolName) || contract.Risk == string(ToolRiskReadonly) {
				continue
			}
			return true, fmt.Sprintf("请补充参数 %s 后再执行。", entity)
		}
	}
	return false, ""
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

func (e *ToolDecisionEngine) scoreToolDecision(tool *ToolSpec, contract ToolUseContract, input ToolDecisionInput, breakdown *IntentBreakdown, workflowForced bool) float64 {
	if tool == nil {
		return 0
	}
	if isResidentTool(tool.Name) {
		return 1
	}
	if workflowForced {
		return 0.9
	}
	score := 0.0
	if containsDecisionString(input.Intent.Domains, string(tool.Domain)) || containsDecisionString(breakdown.Domains, string(tool.Domain)) {
		score += 0.20
	}
	action := firstAction(breakdown, input.Intent)
	if actionMatchesContract(action, contract.Actions, tool.Operation) {
		score += 0.20
	}
	if objectMatchesTool(breakdown.Objects, tool) || input.Intent.Object == string(tool.Domain) {
		score += 0.20
	}
	if breakdown.Scope.Kind != "" && breakdown.Scope.Kind != "unspecified" {
		score += 0.15
	}
	if contextMatchesTool(input.ContextRefs, tool) {
		score += 0.10
	}
	if hasCapability(breakdown, contract.Capability) || preliminarySelected(input.PreliminarySelection, tool.Name) {
		score += 0.10
	}
	if (tool.Risk == ToolRiskReadonly && !breakdown.RequiresWrite) || (tool.Risk != ToolRiskReadonly && breakdown.RequiresWrite) {
		score += 0.05
	}
	return score
}

func (e *ToolDecisionEngine) expandWorkflowTools(accepted []string, input ToolDecisionInput, breakdown *IntentBreakdown) []string {
	names := append([]string{}, accepted...)
	if containsDecisionString(names, "Asset.Collection.Trigger") {
		names = append(names, "Host.List", "Asset.Collection.Get")
		if shouldIncludeAssetAnalysisTools(input.Query, input.Intent, breakdown) {
			names = append(names,
				"Asset.Application.List",
				"Asset.Summary.Get",
				"Software.Installed.Search",
				"Vulnerability.List",
				"Vulnerability.AffectedHosts",
			)
		}
	}
	if containsDecisionString(names, "Vulnerability.Scan.Start") || hasCapability(breakdown, "start_vulnerability_scan") {
		names = append(names, "Host.List", "Vulnerability.Scan.Start", "Vulnerability.Scan.Status")
	}
	if isCVERemediationIntent(input.Query) {
		names = append(names,
			"Vulnerability.List",
			"Vulnerability.AffectedHosts",
			"Vulnerability.Script.Generate",
			"Vulnerability.Script.Status",
			"Vulnerability.Script.Execute",
		)
	}
	return dedupeStrings(names)
}

func (e *ToolDecisionEngine) orderToolPlanNames(names []string) []string {
	preferred := []string{
		"Host.List",
		"Host.Get",
		"Host.AgentStatus.Get",
		"Asset.Collection.Trigger",
		"Asset.Collection.Get",
		"Asset.Application.List",
		"Asset.Summary.Get",
		"Software.Installed.Search",
		"Vulnerability.List",
		"Vulnerability.AffectedHosts",
		"Vulnerability.Scan.Start",
		"Vulnerability.Scan.Status",
		"Vulnerability.Script.Generate",
		"Vulnerability.Script.Status",
		"Vulnerability.Script.Execute",
		"Detection.Alert.Get",
		"Detection.Alert.Block",
		"Baseline.Template.List",
		"Baseline.Template.Status.Get",
		"Baseline.Template.Rules.List",
		"Baseline.Script.Generate",
		"Task.RunCheck",
		"Task.RunFix",
	}
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	ordered := make([]string, 0, len(names))
	for _, name := range preferred {
		if set[name] {
			ordered = append(ordered, name)
			delete(set, name)
		}
	}
	rest := make([]string, 0, len(set))
	for name := range set {
		rest = append(rest, name)
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)
	return ordered
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

	// 特定工具的参数绑定（高优先级，处理无法用通用规则表达的逻辑）
	switch contract.ToolName {
	case "Host.List":
		if breakdown.Scope.Kind == "online_hosts" {
			args["status"] = "online"
			sources["status"] = ArgSource{SourceType: "user_message", SourceRef: "online", Confidence: 0.9}
		}
	case "Asset.Collection.Trigger":
		if ids := contextRefIDs(input.ContextRefs, "host"); len(ids) > 0 {
			args["host_ids"] = ids
			sources["host_ids"] = ArgSource{SourceType: "page_context", SourceRef: "host", Confidence: 0.95}
		} else if breakdown.Scope.Kind == "all" {
			args["scope"] = "all_hosts"
			sources["scope"] = ArgSource{SourceType: "user_message", SourceRef: "all", Confidence: 0.85}
		} else if breakdown.Scope.Kind == "online_hosts" {
			args["scope"] = "online_hosts"
			sources["scope"] = ArgSource{SourceType: "user_message", SourceRef: "online_hosts", Confidence: 0.85}
		}
	case "Detection.Alert.Block", "Detection.Alert.Resolve", "Detection.Alert.Get":
		if ids := contextRefIDs(input.ContextRefs, "alert"); len(ids) > 0 {
			args["alert_id"] = ids[0]
			sources["alert_id"] = ArgSource{SourceType: "page_context", SourceRef: ids[0], Confidence: 0.95}
		}
	case "Asset.Collection.Get":
		sources["task_id"] = ArgSource{SourceType: "previous_step", SourceRef: "Asset.Collection.Trigger", Confidence: 0.8}
	case "Vulnerability.Scan.Start":
		sources["host_ids"] = ArgSource{SourceType: "previous_step", SourceRef: "Host.List", Confidence: 0.8}
	case "Vulnerability.Scan.Status":
		sources["scan_id"] = ArgSource{SourceType: "previous_step", SourceRef: "Vulnerability.Scan.Start", Confidence: 0.8}
	case "Vulnerability.AffectedHosts":
		sources["vulnerability_id"] = ArgSource{SourceType: "previous_step", SourceRef: "Vulnerability.List", Confidence: 0.8}
	case "Vulnerability.Script.Execute":
		sources["host_ids"] = ArgSource{SourceType: "previous_step", SourceRef: "Vulnerability.AffectedHosts", Confidence: 0.8}
		if rounds, specified, _ := parseMaxRepairRounds(input.Query); specified {
			args["max_rounds"] = rounds
			sources["max_rounds"] = ArgSource{SourceType: "user_message", SourceRef: fmt.Sprintf("%d rounds", rounds), Confidence: 0.95}
		}
	}

	// 通用 ArgBindingRule 驱动的参数绑定（补充特定工具未覆盖的参数）
	for _, binding := range contract.ArgBindings {
		if _, alreadyBound := args[binding.ArgName]; alreadyBound {
			continue
		}
		if _, dynamicallyBound := sources[binding.ArgName]; dynamicallyBound {
			continue
		}
		value, source := resolveArgBySourceOrder(binding, input, breakdown)
		if value != nil {
			args[binding.ArgName] = value
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

// extractArgFromBreakdown 从 IntentBreakdown 中提取与 binding 匹配的参数值。
func extractArgFromBreakdown(binding ArgBindingRule, breakdown *IntentBreakdown) (interface{}, string) {
	if breakdown == nil {
		return nil, ""
	}
	entity := strings.ToLower(binding.Entity)
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
			return breakdown.Scope.Kind, "scope:" + breakdown.Scope.Kind
		}
	}
	return nil, ""
}

func decisionStepReason(tool *ToolSpec, contract ToolUseContract, input ToolDecisionInput, breakdown *IntentBreakdown) string {
	if tool == nil {
		return "工具已通过后端裁决"
	}
	if containsDecisionString(contract.WorkflowHints, "asset_collection_then_analysis") || containsDecisionString(contract.NextCapabilities, "get_asset_collection_task") {
		return "用户要求资产采集或资产分析，后端按契约生成采集及后续查询计划"
	}
	if preliminarySelected(input.PreliminarySelection, tool.Name) {
		return "候选工具通过后端 mapping、风险和参数裁决"
	}
	if hasCapability(breakdown, contract.Capability) {
		return "意图拆解候选能力映射到该工具并通过后端裁决"
	}
	return "后端根据工作流契约补充的工具"
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

func shouldIncludeAssetAnalysisTools(query string, intent IntentResult, breakdown *IntentBreakdown) bool {
	normalized := strings.ToLower(query)
	if containsAnyFold(normalized, "mysql", "软件", "应用", "漏洞", "cve", "ai agent", "ai资产", "llm", "mcp") {
		return true
	}
	if containsDecisionString(intent.Domains, "vulnerability") {
		return true
	}
	return hasCapability(breakdown, "list_vulnerabilities") || hasCapability(breakdown, "search_installed_software")
}

func preliminarySelected(selection *ToolSelectionResult, name string) bool {
	if selection == nil {
		return false
	}
	return containsDecisionString(selection.SelectedTools, name)
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

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}

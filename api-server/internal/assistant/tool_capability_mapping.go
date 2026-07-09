package assistant

import (
	"fmt"
	"strings"
)

type ToolCapabilityMapper struct {
	registry *ToolRegistry
}

func NewToolCapabilityMapper(registry *ToolRegistry) *ToolCapabilityMapper {
	return &ToolCapabilityMapper{registry: registry}
}

func (m *ToolCapabilityMapper) ContractForToolName(toolName string) (ToolUseContract, bool) {
	if m == nil || m.registry == nil {
		return ToolUseContract{}, false
	}
	tool, ok := m.registry.Get(toolName)
	if !ok || tool == nil || !tool.Enabled {
		return ToolUseContract{}, false
	}
	return BuildToolUseContract(tool), true
}

func (m *ToolCapabilityMapper) ToolNamesForCapabilities(capabilities []string) []string {
	if m == nil || m.registry == nil || len(capabilities) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		if capability != "" {
			wanted[capability] = true
		}
	}
	var names []string
	for _, tool := range m.registry.List() {
		if tool == nil || !tool.Enabled {
			continue
		}
		contract := BuildToolUseContract(tool)
		if wanted[strings.ToLower(contract.Capability)] {
			names = append(names, tool.Name)
		}
	}
	return dedupeStrings(names)
}

func BuildToolUseContract(tool *ToolSpec) ToolUseContract {
	if tool == nil {
		return ToolUseContract{}
	}
	capability := strings.TrimSpace(tool.Capability)
	if capability == "" {
		capability = syntheticToolCapability(tool)
	}
	contract := ToolUseContract{
		ToolName:                   tool.Name,
		Capability:                 capability,
		Domain:                     string(tool.Domain),
		AllowedIntents:             defaultAllowedIntents(tool),
		Actions:                    defaultContractActions(tool),
		ObjectTypes:                append([]string{}, tool.ObjectTypes...),
		RequiredEntities:           requiredArgsFromToolSchema(tool.ArgsSchema),
		ArgBindings:                defaultArgBindings(tool),
		Risk:                       string(tool.Risk),
		RequiresExplicitUserIntent: requiresExplicitUserIntent(tool),
		RequiresApproval:           tool.RequiresApproval || !tool.DefaultWhitelisted || tool.Risk == ToolRiskHigh || tool.Risk == ToolRiskCritical,
	}
	applyBuiltinToolContractOverrides(&contract)
	return contract
}

func syntheticToolCapability(tool *ToolSpec) string {
	if tool == nil {
		return ""
	}
	domain := strings.ReplaceAll(strings.ToLower(string(tool.Domain)), ".", "_")
	operation := strings.ReplaceAll(strings.ToLower(string(tool.Operation)), ".", "_")
	name := strings.ReplaceAll(strings.ToLower(tool.Name), ".", "_")
	if domain != "" && operation != "" {
		return fmt.Sprintf("%s_%s", operation, domain)
	}
	return name
}

func defaultAllowedIntents(tool *ToolSpec) []string {
	if tool == nil {
		return nil
	}
	domain := string(tool.Domain)
	switch tool.Operation {
	case OpList, OpGet, OpSearch:
		return []string{"query_" + domain, "analyze_" + domain}
	case OpCreate, OpGenerate:
		return []string{"create_" + domain, "generate_" + domain}
	case OpExecute, OpDispatch:
		return []string{"execute_" + domain}
	case OpUpdate:
		return []string{"update_" + domain}
	case OpDelete:
		return []string{"delete_" + domain}
	case OpApprove:
		return []string{"approve_" + domain}
	default:
		if tool.Risk == ToolRiskReadonly {
			return []string{"query_" + domain}
		}
		return []string{"execute_" + domain}
	}
}

func defaultContractActions(tool *ToolSpec) []string {
	if tool == nil {
		return nil
	}
	switch tool.Operation {
	case OpList, OpGet, OpSearch:
		return []string{"query", "analyze"}
	case OpCreate, OpGenerate:
		return []string{"create", "generate"}
	case OpExecute, OpDispatch:
		return []string{"execute", "collect", "scan"}
	case OpUpdate:
		return []string{"update"}
	case OpDelete:
		return []string{"delete"}
	case OpApprove:
		return []string{"approve"}
	default:
		return []string{strings.ToLower(string(tool.Operation))}
	}
}

func requiresExplicitUserIntent(tool *ToolSpec) bool {
	if tool == nil {
		return false
	}
	if tool.Risk == ToolRiskHigh || tool.Risk == ToolRiskCritical {
		return true
	}
	switch tool.Operation {
	case OpCreate, OpGenerate, OpUpdate, OpDelete, OpExecute, OpDispatch, OpApprove, OpRollback:
		return tool.Risk != ToolRiskReadonly
	default:
		return false
	}
}

func defaultArgBindings(tool *ToolSpec) []ArgBindingRule {
	required := requiredArgsFromToolSchema(tool.ArgsSchema)
	bindings := make([]ArgBindingRule, 0, len(required))
	for _, argName := range required {
		bindings = append(bindings, ArgBindingRule{
			ArgName:     argName,
			Entity:      inferEntityFromArgName(argName),
			SourceOrder: []string{"user_message", "page_context", "session_context", "previous_step"},
			Required:    true,
		})
	}
	return bindings
}

func requiredArgsFromToolSchema(schema map[string]interface{}) []string {
	if schema == nil {
		return nil
	}
	var result []string
	switch typed := schema["required"].(type) {
	case []string:
		result = append(result, typed...)
	case []interface{}:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	}
	return dedupeStrings(result)
}

func inferEntityFromArgName(argName string) string {
	lower := strings.ToLower(strings.TrimSpace(argName))
	lower = strings.TrimSuffix(lower, "_ids")
	lower = strings.TrimSuffix(lower, "_id")
	return lower
}

func applyBuiltinToolContractOverrides(contract *ToolUseContract) {
	switch contract.ToolName {
	case "Asset.Collection.Trigger":
		contract.AllowedIntents = []string{"execute_asset_collection", "refresh_asset_inventory"}
		contract.DeniedIntents = []string{"explain_asset_collection", "query_asset_collection_history"}
		contract.RequiredEntities = []string{"scope|host_ids"}
		contract.Preconditions = []string{"user_explicit_execute_intent", "scope_resolved"}
		contract.ArgBindings = []ArgBindingRule{{
			ArgName:     "host_ids",
			Entity:      "host",
			SourceOrder: []string{"user_message", "page_context", "previous_step"},
			Required:    false,
		}}
		contract.NegativeCases = []string{
			"用户只询问资产采集概念时不要调用",
			"用户只查看采集历史时不要调用",
		}
		contract.Postconditions = []string{"task_id_created"}
		contract.ResultValidators = []string{"asset_collection_task_id_present"}
		contract.RequiresExplicitUserIntent = true
		contract.RequiresApproval = true
	case "Asset.Collection.Get":
		contract.Preconditions = append(contract.Preconditions, "asset_collection_task_created")
		contract.ArgBindings = []ArgBindingRule{{
			ArgName:     "task_id",
			Entity:      "asset_collection_task",
			SourceOrder: []string{"previous_step", "user_message", "page_context"},
			Required:    true,
		}}
	case "Detection.Alert.Block":
		contract.AllowedIntents = []string{"block_alert", "respond_detection_alert"}
		contract.DeniedIntents = []string{"explain_alert_block", "query_detection_alert"}
		contract.RequiredEntities = []string{"alert_id"}
		contract.Preconditions = []string{"user_explicit_block_intent", "alert_id_resolved", "approval_required"}
		contract.NegativeCases = []string{
			"用户只询问告警详情时不要调用阻断",
			"用户只查看告警趋势时不要调用阻断",
		}
		contract.Postconditions = []string{"block_action_created"}
		contract.ResultValidators = []string{"block_action_result_present"}
		contract.RequiresExplicitUserIntent = true
		contract.RequiresApproval = true
	case "Detection.Alert.Resolve":
		contract.RequiredEntities = []string{"alert_id"}
		contract.Preconditions = []string{"alert_id_resolved"}
	case "Task.RunFix":
		contract.AllowedIntents = []string{"execute_baseline_fix", "repair_baseline"}
		contract.DeniedIntents = []string{"explain_baseline_fix", "query_baseline_fix_history"}
		contract.RequiredEntities = []string{"rule_ids", "host_ids"}
		contract.Preconditions = []string{"user_explicit_fix_intent", "rule_ids_resolved", "host_ids_resolved", "approval_required"}
		contract.NegativeCases = []string{
			"用户只询问基线修复概念时不要调用",
			"用户只查看修复历史时不要调用",
		}
		contract.Postconditions = []string{"task_id_created"}
		contract.RequiresExplicitUserIntent = true
		contract.RequiresApproval = true
	case "Task.RunCheck":
		contract.RequiredEntities = []string{"rule_ids", "host_ids"}
		contract.Postconditions = []string{"task_id_created"}
		contract.RequiresExplicitUserIntent = true
	case "Baseline.Script.Generate":
		contract.Postconditions = []string{"script_generated"}
		contract.RequiresExplicitUserIntent = true
	case "Vulnerability.CustomQuery.Start":
		contract.Preconditions = []string{"exact_cve_list_result_empty"}
		contract.RequiresExplicitUserIntent = true
	case "Vulnerability.CustomQuery.Status":
		contract.Preconditions = []string{"custom_cve_query_started"}
	case "Vulnerability.Script.Execute":
		contract.RequiresExplicitUserIntent = true
		contract.RequiresApproval = true
	}
}

func isResidentTool(name string) bool {
	switch name {
	case "Tool.Search", "Context.Get", "Session.Summarize":
		return true
	default:
		return false
	}
}

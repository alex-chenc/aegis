package assistant

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

type runtimeToolEvidence struct {
	CallID          string      `json:"call_id"`
	ToolName        string      `json:"tool_name"`
	Status          string      `json:"status"`
	Content         interface{} `json:"content,omitempty"`
	Error           string      `json:"error,omitempty"`
	ValidationStage string      `json:"validation_stage,omitempty"`
}

type runtimeEvidenceLedger struct {
	Calls                 []runtimeToolEvidence `json:"calls"`
	ActualToolNames       []string              `json:"actual_tool_names"`
	FailedToolNames       []string              `json:"failed_tool_names,omitempty"`
	VulnerabilityCount    int                   `json:"vulnerability_count"`
	OnlineHostCount       int                   `json:"online_host_count"`
	GeneratedScriptTypes  []string              `json:"generated_script_types,omitempty"`
	TaskGroupIDs          []string              `json:"task_group_ids,omitempty"`
	VulnerabilityWorkflow bool                  `json:"vulnerability_workflow"`
}

func buildRuntimeEvidenceLedger(result *agentruntime.TaskResult) runtimeEvidenceLedger {
	ledger := runtimeEvidenceLedger{}
	if result == nil {
		return ledger
	}
	toolNames := make(map[string]bool)
	failedNames := make(map[string]bool)
	generatedTypes := make(map[string]bool)
	taskGroups := make(map[string]bool)
	onlineHostIDs := make(map[string]bool)

	validationByCall := make(map[string]agentruntime.ToolCallRecord, len(result.ToolCalls))
	for _, call := range result.ToolCalls {
		validationByCall[call.CallID] = call
		toolNames[call.ToolName] = true
		if call.Status != agentruntime.ToolCallSuccess {
			failedNames[call.ToolName] = true
		}
	}

	for _, step := range result.StepExecutions {
		for _, turn := range step.ReactTurns {
			observation := turn.Observation
			if observation == nil {
				continue
			}
			toolNames[observation.ToolName] = true
			evidence := runtimeToolEvidence{
				CallID:   observation.CallID,
				ToolName: observation.ToolName,
				Status:   string(observation.Status),
				Error:    observation.Error,
			}
			if record, ok := validationByCall[observation.CallID]; ok {
				evidence.ValidationStage = record.ValidationStage
			}
			var content interface{}
			if strings.TrimSpace(observation.Content) != "" {
				if err := json.Unmarshal([]byte(observation.Content), &content); err != nil {
					content = observation.Content
				}
			}
			evidence.Content = content
			ledger.Calls = append(ledger.Calls, evidence)

			if observation.Status != agentruntime.ToolCallSuccess {
				failedNames[observation.ToolName] = true
				continue
			}
			payload, _ := content.(map[string]interface{})
			switch observation.ToolName {
			case "Vulnerability.List":
				ledger.VulnerabilityWorkflow = true
				if total, ok := numericValue(payload["total"]); ok && int(total) > ledger.VulnerabilityCount {
					ledger.VulnerabilityCount = int(total)
				}
				if count := len(resultDataItems(payload)); count > ledger.VulnerabilityCount {
					ledger.VulnerabilityCount = count
				}
			case "Vulnerability.CustomQuery.Status":
				ledger.VulnerabilityWorkflow = true
				if stringValue(payload["result_vulnerability_id"]) != "" && ledger.VulnerabilityCount == 0 {
					ledger.VulnerabilityCount = 1
				}
			case "Vulnerability.AffectedHosts":
				ledger.VulnerabilityWorkflow = true
				for _, item := range resultDataItems(payload) {
					if online, exists := item["online"].(bool); exists && online {
						if id := stringValue(item["id"]); id != "" {
							onlineHostIDs[id] = true
						}
					}
				}
			case "Host.List":
				status := strings.ToLower(stringValue(payload["status"]))
				if status == "online" {
					for _, item := range resultDataItems(payload) {
						if id := stringValue(item["id"]); id != "" {
							onlineHostIDs[id] = true
						}
					}
					if len(onlineHostIDs) == 0 {
						if total, ok := numericValue(payload["total"]); ok {
							ledger.OnlineHostCount = int(total)
						}
					}
				}
			case "Vulnerability.Script.Status":
				ledger.VulnerabilityWorkflow = true
				if strings.EqualFold(stringValue(payload["generation_status"]), "generated") {
					if scriptType := stringValue(payload["script_type"]); scriptType != "" {
						generatedTypes[scriptType] = true
					}
				}
			case "Vulnerability.Script.Execute":
				ledger.VulnerabilityWorkflow = true
				if taskGroupID := stringValue(payload["task_group_id"]); taskGroupID != "" {
					taskGroups[taskGroupID] = true
				}
			}
		}
	}

	if len(onlineHostIDs) > ledger.OnlineHostCount {
		ledger.OnlineHostCount = len(onlineHostIDs)
	}
	ledger.ActualToolNames = sortedStringSet(toolNames)
	ledger.FailedToolNames = sortedStringSet(failedNames)
	ledger.GeneratedScriptTypes = sortedStringSet(generatedTypes)
	ledger.TaskGroupIDs = sortedStringSet(taskGroups)
	return ledger
}

func validateRuntimeEvidenceConsistency(answer string, ledger runtimeEvidenceLedger) []string {
	normalized := strings.ToLower(strings.TrimSpace(answer))
	conflicts := make([]string, 0, 4)
	if ledger.OnlineHostCount > 0 && containsAnyFold(normalized,
		"没有在线主机", "无在线主机", "没有处于在线状态的主机", "当前环境没有在线主机") {
		conflicts = append(conflicts, "online_hosts_contradiction")
	}
	if ledger.VulnerabilityCount > 0 && containsAnyFold(normalized,
		"漏洞不存在", "漏洞未收录", "未查询到该漏洞", "未查到 cve", "未查到cve") {
		conflicts = append(conflicts, "vulnerability_contradiction")
	}
	if ledger.VulnerabilityWorkflow && len(ledger.TaskGroupIDs) == 0 &&
		containsAnyFold(normalized, "已下发", "下发成功", "任务已下发") {
		conflicts = append(conflicts, "dispatch_without_task_group")
	}
	if ledger.VulnerabilityWorkflow && len(ledger.GeneratedScriptTypes) == 0 &&
		containsAnyFold(normalized, "脚本已生成", "脚本生成成功", "poc生成成功", "修复脚本生成成功") {
		conflicts = append(conflicts, "script_generated_without_terminal_evidence")
	}
	return dedupeStrings(conflicts)
}

func buildEvidenceGroundedFallback(ledger runtimeEvidenceLedger) string {
	var b strings.Builder
	b.WriteString("任务结论已根据真实工具记录重新校正。")
	if ledger.VulnerabilityCount > 0 {
		b.WriteString(fmt.Sprintf("\n\n- 已查询到目标漏洞记录：%d 条。", ledger.VulnerabilityCount))
	}
	if ledger.OnlineHostCount > 0 {
		b.WriteString(fmt.Sprintf("\n- 已确认在线目标主机：%d 台。", ledger.OnlineHostCount))
	}
	if len(ledger.GeneratedScriptTypes) > 0 {
		b.WriteString("\n- 已生成脚本类型：" + strings.Join(ledger.GeneratedScriptTypes, "、") + "。")
	} else if ledger.VulnerabilityWorkflow {
		b.WriteString("\n- 尚未取得脚本状态为 generated 的终态证据。")
	}
	if len(ledger.TaskGroupIDs) > 0 {
		b.WriteString("\n- 已创建下发任务组：" + strings.Join(ledger.TaskGroupIDs, "、") + "。")
	} else if ledger.VulnerabilityWorkflow {
		b.WriteString("\n- 尚未取得任务组 ID，不能认定任务已经下发。")
	}
	if len(ledger.FailedToolNames) > 0 {
		b.WriteString("\n- 实际失败的工具：" + strings.Join(ledger.FailedToolNames, "、") + "。")
	}
	if len(ledger.ActualToolNames) > 0 {
		b.WriteString("\n- 本轮实际调用：" + strings.Join(ledger.ActualToolNames, "、") + "。")
	}
	return b.String()
}

func sortedStringSet(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

package assistant

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

type runtimeToolEvidence struct {
	CallID          string                    `json:"call_id"`
	ToolName        string                    `json:"tool_name"`
	Status          string                    `json:"status"`
	Content         interface{}               `json:"content,omitempty"`
	Error           string                    `json:"error,omitempty"`
	ValidationStage string                    `json:"validation_stage,omitempty"`
	Outcome         *agentruntime.ToolOutcome `json:"outcome,omitempty"`
}

type runtimeEvidenceLedger struct {
	Calls                   []runtimeToolEvidence `json:"calls"`
	ActualToolNames         []string              `json:"actual_tool_names"`
	FailedToolNames         []string              `json:"failed_tool_names,omitempty"`
	VulnerabilityCount      int                   `json:"vulnerability_count"`
	OnlineHostCount         int                   `json:"online_host_count"`
	GeneratedScriptTypes    []string              `json:"generated_script_types,omitempty"`
	TaskGroupIDs            []string              `json:"task_group_ids,omitempty"`
	VulnerabilityWorkflow   bool                  `json:"vulnerability_workflow"`
	AssetCollectionTaskIDs  []string              `json:"asset_collection_task_ids,omitempty"`
	AssetCollectionTerminal bool                  `json:"asset_collection_terminal,omitempty"`
	AssetCollectionCoverage map[string]int        `json:"asset_collection_coverage,omitempty"`
	DetectionPackageID      string                `json:"detection_package_id,omitempty"`
	DetectionPackageStatus  string                `json:"detection_package_status,omitempty"`
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
	assetTaskIDs := make(map[string]bool)
	assetCoverage := make(map[string]int)
	assetTerminal := false

	validationByCall := make(map[string]agentruntime.ToolCallRecord, len(result.ToolCalls))
	for _, call := range result.ToolCalls {
		validationByCall[call.CallID] = call
		if strings.TrimSpace(call.ValidationStage) != "" {
			// A validation-stage record is a rejected model candidate, not a
			// backend tool execution. Keep it in runtime diagnostics only.
			continue
		}
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
			if record, ok := validationByCall[observation.CallID]; ok &&
				strings.TrimSpace(record.ValidationStage) != "" {
				continue
			}
			toolNames[observation.ToolName] = true
			evidence := runtimeToolEvidence{
				CallID:   observation.CallID,
				ToolName: observation.ToolName,
				Status:   string(observation.Status),
				Error:    observation.Error,
				Outcome:  observation.Outcome,
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
			if strings.HasPrefix(observation.ToolName, "Package.") {
				if contentMap, ok := content.(map[string]interface{}); ok {
					if packageID := stringValue(contentMap["package_id"]); packageID != "" {
						ledger.DetectionPackageID = packageID
					}
					if status := stringValue(contentMap["status"]); status != "" {
						ledger.DetectionPackageStatus = status
					}
				}
				if observation.Outcome != nil {
					if packageID := observation.Outcome.OperationRef["package_id"]; packageID != "" {
						ledger.DetectionPackageID = packageID
					}
				}
			}

			if observation.Status != agentruntime.ToolCallSuccess {
				failedNames[observation.ToolName] = true
				if ledger.DetectionPackageID != "" {
					switch observation.ToolName {
					case "Package.Build.Start", "Package.Build.Status":
						ledger.DetectionPackageStatus = "build_failed"
					case "Package.Sign":
						ledger.DetectionPackageStatus = "sign_failed"
					case "Package.Enable":
						ledger.DetectionPackageStatus = "enable_failed"
					}
				}
				continue
			}
			outcome := observation.Outcome
			if outcome == nil {
				continue
			}
			if strings.Contains(strings.ToLower(outcome.Capability), "vulnerability") {
				ledger.VulnerabilityWorkflow = true
			}
			if outcome.OperationStatus == agentruntime.OperationFailed {
				failedNames[observation.ToolName] = true
			}
			for _, fact := range outcome.Facts {
				switch strings.ToLower(stringValue(fact["kind"])) {
				case "host_online":
					if id := stringValue(fact["id"]); id != "" {
						onlineHostIDs[id] = true
					}
				case "host_resolved":
					if strings.EqualFold(stringValue(fact["state"]), "online") {
						if id := stringValue(fact["id"]); id != "" {
							onlineHostIDs[id] = true
						}
					}
				case "vulnerability_record":
					ledger.VulnerabilityWorkflow = true
					ledger.VulnerabilityCount++
				}
			}
			for _, artifact := range outcome.Artifacts {
				if scriptType := stringValue(artifact["script_type"]); scriptType != "" {
					generatedTypes[scriptType] = true
				}
				if stringValue(artifact["result_vulnerability_id"]) != "" && ledger.VulnerabilityCount == 0 {
					ledger.VulnerabilityWorkflow = true
					ledger.VulnerabilityCount = 1
				}
			}
			for _, sideEffect := range outcome.SideEffects {
				for _, field := range []string{"task_group_id", "task_id", "action_id", "block_id"} {
					if id := stringValue(sideEffect[field]); id != "" {
						taskGroups[id] = true
						break
					}
				}
			}
			// Track asset collection task references, terminal status, and
			// coverage so the final summary cannot claim completion without a
			// real task_id and terminal evidence.
			if strings.Contains(strings.ToLower(outcome.Capability), "asset_collection") {
				if taskID := outcome.OperationRef["task_id"]; taskID != "" {
					assetTaskIDs[taskID] = true
				}
				if outcome.Terminal {
					assetTerminal = true
				}
				if contentMap, ok := content.(map[string]interface{}); ok {
					if progress, ok := contentMap["progress"].(map[string]interface{}); ok {
						if v, ok := numericValue(progress["total_hosts"]); ok && v > 0 {
							assetCoverage["total_hosts"] = int(v)
						}
						if v, ok := numericValue(progress["success_hosts"]); ok && v > 0 {
							assetCoverage["success_hosts"] = int(v)
						}
						if v, ok := numericValue(progress["failed_hosts"]); ok && v > 0 {
							assetCoverage["failed_hosts"] = int(v)
						}
					}
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
	ledger.AssetCollectionTaskIDs = sortedStringSet(assetTaskIDs)
	ledger.AssetCollectionTerminal = assetTerminal
	if len(assetCoverage) > 0 {
		ledger.AssetCollectionCoverage = assetCoverage
	}
	return ledger
}

func validateRuntimeEvidenceConsistency(answer string, ledger runtimeEvidenceLedger) []string {
	normalized := strings.ToLower(strings.TrimSpace(answer))
	conflicts := make([]string, 0, 6)
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
	// Asset collection: the answer must not claim completion without a real
	// task_id and terminal evidence from Asset.Collection.Get.
	if len(ledger.AssetCollectionTaskIDs) > 0 && !ledger.AssetCollectionTerminal &&
		containsAnyFold(normalized, "资产采集完成", "采集已完成", "资产重采集完成", "asset collection completed") {
		conflicts = append(conflicts, "asset_collection_completed_without_terminal")
	}
	if len(ledger.AssetCollectionTaskIDs) == 0 &&
		containsAnyFold(normalized, "资产采集完成", "采集已完成", "资产重采集完成", "已创建采集任务") {
		conflicts = append(conflicts, "asset_collection_claimed_without_task_id")
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
	if len(ledger.AssetCollectionTaskIDs) > 0 {
		b.WriteString("\n- 资产采集任务：" + strings.Join(ledger.AssetCollectionTaskIDs, "、") + "。")
		if !ledger.AssetCollectionTerminal {
			b.WriteString("\n- 资产采集任务尚未达到终态，不能认定采集完成。")
		} else if len(ledger.AssetCollectionCoverage) > 0 {
			total := ledger.AssetCollectionCoverage["total_hosts"]
			success := ledger.AssetCollectionCoverage["success_hosts"]
			failed := ledger.AssetCollectionCoverage["failed_hosts"]
			b.WriteString(fmt.Sprintf("\n- 采集覆盖：目标 %d 台 / 成功 %d 台 / 失败 %d 台。", total, success, failed))
		}
	}
	if len(ledger.FailedToolNames) > 0 {
		b.WriteString("\n- 实际失败的工具：" + strings.Join(ledger.FailedToolNames, "、") + "。")
	}
	if len(ledger.ActualToolNames) > 0 {
		b.WriteString("\n- 本轮实际调用：" + strings.Join(ledger.ActualToolNames, "、") + "。")
	}
	return b.String()
}

// buildFailedGoalFallback prevents a normal runtime-loop completion from being
// presented as a successful business task. It intentionally reports only
// durable evidence summaries and never echoes model reasoning or raw payloads.
func buildFailedGoalFallback(ledger runtimeEvidenceLedger) string {
	var b strings.Builder
	b.WriteString("任务未完成，不能认定扫描、修复或验证已经执行成功。")
	if len(ledger.FailedToolNames) > 0 {
		b.WriteString("\n\n- 失败工具：" + strings.Join(ledger.FailedToolNames, "、") + "。")
	}
	if len(ledger.AssetCollectionTaskIDs) > 0 {
		b.WriteString("\n- 已创建资产采集任务：" + strings.Join(ledger.AssetCollectionTaskIDs, "、") + "。")
		if !ledger.AssetCollectionTerminal {
			b.WriteString("\n- 资产采集任务仍在后台运行，但本轮监控提前结束，暂时不能认定采集完成。")
		} else if len(ledger.AssetCollectionCoverage) > 0 {
			total := ledger.AssetCollectionCoverage["total_hosts"]
			success := ledger.AssetCollectionCoverage["success_hosts"]
			failed := ledger.AssetCollectionCoverage["failed_hosts"]
			b.WriteString(fmt.Sprintf("\n- 采集覆盖：目标 %d 台 / 成功 %d 台 / 失败 %d 台。", total, success, failed))
		}
	} else if ledger.VulnerabilityWorkflow {
		if len(ledger.TaskGroupIDs) == 0 {
			b.WriteString("\n- 未取得任务组证据，未下发任务。")
		} else {
			b.WriteString("\n- 已创建任务组，但本轮目标未达到成功终态。")
		}
	}
	if len(ledger.ActualToolNames) > 0 {
		b.WriteString("\n- 实际调用：" + strings.Join(ledger.ActualToolNames, "、") + "。")
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

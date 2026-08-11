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
	Calls                            []runtimeToolEvidence `json:"calls"`
	ActualToolNames                  []string              `json:"actual_tool_names"`
	FailedToolNames                  []string              `json:"failed_tool_names,omitempty"`
	VulnerabilityCount               int                   `json:"vulnerability_count"`
	OnlineHostCount                  int                   `json:"online_host_count"`
	GeneratedScriptTypes             []string              `json:"generated_script_types,omitempty"`
	TaskGroupIDs                     []string              `json:"task_group_ids,omitempty"`
	VulnerabilityWorkflow            bool                  `json:"vulnerability_workflow"`
	VulnerabilityAssessment          bool                  `json:"vulnerability_assessment,omitempty"`
	VulnerabilityRemediation         bool                  `json:"vulnerability_remediation,omitempty"`
	VulnerabilityScanIDs             []string              `json:"vulnerability_scan_ids,omitempty"`
	VulnerabilityScanStatus          string                `json:"vulnerability_scan_status,omitempty"`
	VulnerabilityScanProgress        int                   `json:"vulnerability_scan_progress,omitempty"`
	VulnerabilityScanTerminal        bool                  `json:"vulnerability_scan_terminal,omitempty"`
	VulnerabilityScanFoundCount      int                   `json:"vulnerability_scan_found_count,omitempty"`
	VulnerabilityScanFoundCountKnown bool                  `json:"vulnerability_scan_found_count_known,omitempty"`
	WeakPasswordWorkflow             bool                  `json:"weak_password_workflow,omitempty"`
	WeakPasswordTaskIDs              []string              `json:"weak_password_task_ids,omitempty"`
	WeakPasswordStatus               string                `json:"weak_password_status,omitempty"`
	WeakPasswordTerminal             bool                  `json:"weak_password_terminal,omitempty"`
	WeakPasswordTaskTotal            int                   `json:"weak_password_task_total,omitempty"`
	WeakPasswordTaskCompleted        int                   `json:"weak_password_task_completed,omitempty"`
	WeakPasswordTaskFailed           int                   `json:"weak_password_task_failed,omitempty"`
	WeakPasswordTaskRunning          int                   `json:"weak_password_task_running,omitempty"`
	WeakPasswordFindingCount         int                   `json:"weak_password_finding_count,omitempty"`
	WeakPasswordFindingCountKnown    bool                  `json:"weak_password_finding_count_known,omitempty"`
	AssetCollectionTaskIDs           []string              `json:"asset_collection_task_ids,omitempty"`
	AssetCollectionTerminal          bool                  `json:"asset_collection_terminal,omitempty"`
	AssetCollectionCoverage          map[string]int        `json:"asset_collection_coverage,omitempty"`
	DetectionPackageID               string                `json:"detection_package_id,omitempty"`
	DetectionPackageStatus           string                `json:"detection_package_status,omitempty"`
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
	vulnerabilityScanIDs := make(map[string]bool)
	weakPasswordTaskIDs := make(map[string]bool)
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
			capability := strings.ToLower(strings.TrimSpace(outcome.Capability))
			if strings.Contains(capability, "vulnerability") {
				ledger.VulnerabilityWorkflow = true
			}
			if isVulnerabilityAssessmentCapability(capability) {
				ledger.VulnerabilityAssessment = true
				if scanID := outcome.OperationRef["scan_id"]; scanID != "" {
					vulnerabilityScanIDs[scanID] = true
				}
				if scanID, status, progress, foundCount, foundKnown, ok := vulnerabilityScanSnapshot(content); ok {
					if scanID != "" {
						vulnerabilityScanIDs[scanID] = true
					}
					if status != "" {
						ledger.VulnerabilityScanStatus = status
					}
					ledger.VulnerabilityScanProgress = progress
					if foundKnown {
						ledger.VulnerabilityScanFoundCount = foundCount
						ledger.VulnerabilityScanFoundCountKnown = true
					}
				}
				if outcome.Terminal {
					ledger.VulnerabilityScanTerminal = true
				}
			}
			if isVulnerabilityRemediationCapability(capability) {
				ledger.VulnerabilityRemediation = true
			}
			if strings.HasPrefix(capability, "weak_password_") {
				ledger.WeakPasswordWorkflow = true
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
				case "task_resolved":
					if strings.HasPrefix(capability, "weak_password_") {
						if id := stringValue(fact["id"]); id != "" {
							weakPasswordTaskIDs[id] = true
						}
					}
				}
			}
			if capability == "weak_password_progress" {
				snapshot := weakPasswordProgressEvidence(content)
				if snapshot.status != "" {
					ledger.WeakPasswordStatus = snapshot.status
				}
				ledger.WeakPasswordTerminal = outcome.Terminal
				ledger.WeakPasswordTaskTotal = snapshot.total
				ledger.WeakPasswordTaskCompleted = snapshot.completed
				ledger.WeakPasswordTaskFailed = snapshot.failed
				ledger.WeakPasswordTaskRunning = snapshot.running
				if snapshot.findingCountKnown {
					ledger.WeakPasswordFindingCount = snapshot.findingCount
					ledger.WeakPasswordFindingCountKnown = true
				}
				for _, taskID := range snapshot.taskIDs {
					weakPasswordTaskIDs[taskID] = true
				}
			}
			for _, artifact := range outcome.Artifacts {
				if scriptType := stringValue(artifact["script_type"]); scriptType != "" {
					ledger.VulnerabilityRemediation = true
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
	ledger.VulnerabilityScanIDs = sortedStringSet(vulnerabilityScanIDs)
	ledger.WeakPasswordTaskIDs = sortedStringSet(weakPasswordTaskIDs)
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
	if len(ledger.VulnerabilityScanIDs) > 0 && !answerContainsAnyReference(answer, ledger.VulnerabilityScanIDs) {
		conflicts = append(conflicts, "vulnerability_scan_evidence_omitted")
	}
	if len(ledger.WeakPasswordTaskIDs) > 0 && !answerContainsAnyReference(answer, ledger.WeakPasswordTaskIDs) {
		conflicts = append(conflicts, "weak_password_evidence_omitted")
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
	writeVulnerabilityScanEvidence(&b, ledger)
	if len(ledger.GeneratedScriptTypes) > 0 {
		b.WriteString("\n- 已生成脚本类型：" + strings.Join(ledger.GeneratedScriptTypes, "、") + "。")
	} else if ledger.VulnerabilityRemediation {
		b.WriteString("\n- 尚未取得脚本状态为 generated 的终态证据。")
	}
	if len(ledger.TaskGroupIDs) > 0 {
		b.WriteString("\n- 已创建下发任务组：" + strings.Join(ledger.TaskGroupIDs, "、") + "。")
	} else if ledger.VulnerabilityRemediation {
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
	writeWeakPasswordEvidence(&b, ledger)
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
	if len(ledger.ActualToolNames) == 0 &&
		!ledger.VulnerabilityWorkflow &&
		!ledger.VulnerabilityRemediation &&
		!ledger.WeakPasswordWorkflow &&
		len(ledger.AssetCollectionTaskIDs) == 0 {
		return "分析未完成，本轮未执行任何工具操作，也未取得可验证结果。请稍后重试。"
	}
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
	}
	if ledger.VulnerabilityAssessment {
		writeVulnerabilityScanEvidence(&b, ledger)
	}
	if ledger.VulnerabilityRemediation {
		if len(ledger.TaskGroupIDs) == 0 {
			b.WriteString("\n- 未取得任务组证据，未下发任务。")
		} else {
			b.WriteString("\n- 已创建任务组，但本轮目标未达到成功终态。")
		}
	}
	writeWeakPasswordEvidence(&b, ledger)
	if len(ledger.ActualToolNames) > 0 {
		b.WriteString("\n- 实际调用：" + strings.Join(ledger.ActualToolNames, "、") + "。")
	}
	return b.String()
}

func writeVulnerabilityScanEvidence(b *strings.Builder, ledger runtimeEvidenceLedger) {
	if b == nil || !ledger.VulnerabilityAssessment {
		return
	}
	if len(ledger.VulnerabilityScanIDs) > 0 {
		b.WriteString("\n- 漏洞扫描任务：" + strings.Join(ledger.VulnerabilityScanIDs, "、") + "。")
	}
	if ledger.VulnerabilityScanStatus != "" {
		b.WriteString(fmt.Sprintf("\n- 漏洞扫描状态：%s（%d%%）。", ledger.VulnerabilityScanStatus, ledger.VulnerabilityScanProgress))
	} else if ledger.VulnerabilityScanProgress > 0 {
		b.WriteString(fmt.Sprintf("\n- 漏洞扫描进度：%d%%。", ledger.VulnerabilityScanProgress))
	}
	if ledger.VulnerabilityScanFoundCountKnown {
		b.WriteString(fmt.Sprintf("\n- 漏洞扫描结果：发现漏洞：%d 个。", ledger.VulnerabilityScanFoundCount))
	}
	if !ledger.VulnerabilityScanTerminal {
		b.WriteString("\n- 漏洞扫描仍在后台运行，尚未达到终态。")
	}
}

func writeWeakPasswordEvidence(b *strings.Builder, ledger runtimeEvidenceLedger) {
	if b == nil || !ledger.WeakPasswordWorkflow {
		return
	}
	if len(ledger.WeakPasswordTaskIDs) > 0 {
		b.WriteString("\n- 弱口令扫描任务：" + strings.Join(ledger.WeakPasswordTaskIDs, "、") + "。")
	}
	if ledger.WeakPasswordStatus != "" {
		b.WriteString("\n- 弱口令扫描状态：" + ledger.WeakPasswordStatus + "。")
	}
	if ledger.WeakPasswordTaskTotal > 0 {
		b.WriteString(fmt.Sprintf(
			"\n- 弱口令任务汇总：总计 %d / 完成 %d / 失败 %d / 运行中 %d。",
			ledger.WeakPasswordTaskTotal,
			ledger.WeakPasswordTaskCompleted,
			ledger.WeakPasswordTaskFailed,
			ledger.WeakPasswordTaskRunning,
		))
	}
	if ledger.WeakPasswordFindingCountKnown {
		b.WriteString(fmt.Sprintf("\n- 弱口令命中：%d 条。", ledger.WeakPasswordFindingCount))
	}
	if !ledger.WeakPasswordTerminal {
		b.WriteString("\n- 弱口令扫描仍在后台运行，尚未达到聚合终态。")
	}
}

func isVulnerabilityAssessmentCapability(capability string) bool {
	switch strings.ToLower(strings.TrimSpace(capability)) {
	case "start_vulnerability_scan", "get_vulnerability_scan_status", "stop_vulnerability_scan":
		return true
	default:
		return false
	}
}

func isVulnerabilityRemediationCapability(capability string) bool {
	switch strings.ToLower(strings.TrimSpace(capability)) {
	case "generate_vulnerability_script", "get_vulnerability_script_status", "execute_vulnerability_host_scripts":
		return true
	default:
		return false
	}
}

func vulnerabilityScanSnapshot(content interface{}) (scanID, status string, progress, foundCount int, foundCountKnown, ok bool) {
	payload, _ := content.(map[string]interface{})
	if payload == nil {
		return "", "", 0, 0, false, false
	}
	scan := payload
	if nested, nestedOK := payload["scan"].(map[string]interface{}); nestedOK {
		scan = nested
	}
	scanID = stringValue(scan["scan_id"])
	status = stringValue(scan["status"])
	if value, found := numericValue(scan["progress"]); found {
		progress = int(value)
	}
	if value, found := numericValue(scan["found_vulns"]); found {
		foundCount = int(value)
		foundCountKnown = true
	}
	return scanID, status, progress, foundCount, foundCountKnown, scanID != "" || status != "" || progress > 0 || foundCountKnown
}

type weakPasswordProgressSnapshot struct {
	status            string
	taskIDs           []string
	total             int
	completed         int
	failed            int
	running           int
	findingCount      int
	findingCountKnown bool
}

func weakPasswordProgressEvidence(content interface{}) weakPasswordProgressSnapshot {
	payload, _ := content.(map[string]interface{})
	if payload == nil {
		return weakPasswordProgressSnapshot{}
	}
	snapshot := weakPasswordProgressSnapshot{status: stringValue(payload["status"])}
	if value, ok := numericValue(payload["matched_findings"]); ok {
		snapshot.findingCount = int(value)
		snapshot.findingCountKnown = true
	}
	tasks, _ := payload["tasks"].([]interface{})
	snapshot.total = len(tasks)
	nestedFindingCount := 0
	nestedFindingKnown := false
	for _, rawTask := range tasks {
		task, _ := rawTask.(map[string]interface{})
		if task == nil {
			continue
		}
		if taskID := stringValue(task["task_id"]); taskID != "" {
			snapshot.taskIDs = append(snapshot.taskIDs, taskID)
		}
		progress, _ := task["task_progress"].(map[string]interface{})
		switch strings.ToLower(stringValue(progress["status"])) {
		case "completed":
			snapshot.completed++
		case "partial_failed", "failed", "cancelled":
			snapshot.failed++
		default:
			snapshot.running++
		}
		if value, ok := numericValue(progress["matched_findings"]); ok {
			nestedFindingCount += int(value)
			nestedFindingKnown = true
		}
	}
	if !snapshot.findingCountKnown && nestedFindingKnown {
		snapshot.findingCount = nestedFindingCount
		snapshot.findingCountKnown = true
	}
	snapshot.taskIDs = dedupeStrings(snapshot.taskIDs)
	return snapshot
}

func answerContainsAnyReference(answer string, references []string) bool {
	for _, reference := range references {
		if reference != "" && strings.Contains(answer, reference) {
			return true
		}
	}
	return false
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

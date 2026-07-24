package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const baselineComplianceOperationType = "baseline_compliance"

type AssistantOperationRepositoryForTools interface {
	Create(ctx context.Context, operation *model.AssistantOperation) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.AssistantOperation, error)
	FindByIdempotencyKey(ctx context.Context, sessionID, workflowID, key string) (*model.AssistantOperation, bool, error)
	ListNonTerminal(ctx context.Context, operationType string, limit int) ([]model.AssistantOperation, error)
	Transition(ctx context.Context, id uuid.UUID, from []string, to string, result interface{}) (bool, error)
	Update(ctx context.Context, id uuid.UUID, status string, result interface{}, errorCode, errorMessage string, terminal bool) error
}

type TaskGroupRepositoryForTools interface {
	FindByGroupID(groupID uuid.UUID) ([]model.TaskLog, error)
}

type baselineComplianceRequest struct {
	HostIDs        []string `json:"host_ids"`
	TargetScope    string   `json:"target_scope,omitempty"`
	TemplateID     string   `json:"template_id"`
	Scope          string   `json:"scope"`
	Remediation    bool     `json:"remediation"`
	MaxRounds      int      `json:"max_rounds"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
}

func registerBaselineComplianceTools(registry *assistant.ToolRegistry, deps BaselineToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Baseline.Compliance.Run",
		Domain:             assistant.DomainBaseline,
		Operation:          assistant.OpExecute,
		Capability:         "run_baseline_compliance",
		Description:        "Resolve hosts and a baseline template, enumerate every rule server-side, prepare scripts, dispatch checks, and monitor optional verified remediation.",
		Aliases:            []string{"运行基线合规", "全量基线检查", "基线检查并修复"},
		Tags:               []string{"v6.1", "baseline", "compliance", "workflow"},
		ObjectTypes:        []string{"host", "baseline_template", "baseline_rule", "operation"},
		PageRoutes:         []string{"/baseline"},
		Risk:               assistant.ToolRiskHigh,
		AutoCallable:       false,
		RequiresApproval:   true,
		Idempotent:         true,
		DefaultWhitelisted: false,
		Enabled:            true,
		DefaultTimeout:     30 * time.Second,
		ExposurePolicy: assistant.ToolExposurePolicy{
			Exposure:        assistant.ToolExposurePrimary,
			WorkflowIDs:     []string{"baseline_compliance"},
			Discoverable:    true,
			DirectCallable:  true,
			CatalogPriority: 200,
		},
		ExecutionContract: assistant.ToolExecutionContract{
			Mode:                 assistant.ToolExecutionAsynchronous,
			CompletionCapability: "get_operation_status",
		},
		ResultContract: assistant.ToolResultContract{
			OperationStatusField: "operation_status",
			SuccessValues:        []string{"succeeded", "partially_succeeded"},
			PendingValues:        []string{"accepted", "preparing_template", "preparing_scripts", "dispatching", "running"},
			FailureValues:        []string{"failed", "cancelled"},
			OperationRefFields:   []string{"operation_id"},
			SideEffectRefFields:  []string{"task_group_id"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_selectors": map[string]interface{}{
					"type":        "array",
					"minItems":    1,
					"items":       map[string]interface{}{"type": "string"},
					"description": "Explicit host UUIDs, IP addresses, hostnames, or short labels. Do not use natural-language groups here.",
					"examples":    []interface{}{[]interface{}{"159IP"}},
				},
				"target_scope": map[string]interface{}{
					"type":        "string",
					"enum":        []interface{}{hostTargetScopeAllOnline},
					"description": "Deterministic server-side target scope. Use all_online_hosts to operate on every currently online host. Mutually exclusive with host_selectors.",
				},
				"template_selector": map[string]interface{}{
					"type":        "string",
					"minLength":   1,
					"description": "Exact template UUID, name, or display name supplied by the user.",
				},
				"scope": map[string]interface{}{
					"type":        "string",
					"enum":        []interface{}{"all_rules"},
					"description": "Rule scope. Version 6.1 requires all_rules so the backend owns complete enumeration.",
				},
				"remediation": map[string]interface{}{
					"type":        "object",
					"description": "Optional automatic remediation and recheck policy.",
					"properties": map[string]interface{}{
						"enabled":    map[string]interface{}{"type": "boolean", "description": "Enable approved automatic remediation for noncompliant checks."},
						"max_rounds": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 10, "description": "Maximum remediation and recheck rounds; defaults to 3."},
					},
					"additionalProperties": false,
				},
				"idempotency_key": map[string]interface{}{
					"type":        "string",
					"description": "Optional caller-provided key for a logically identical request.",
				},
			},
			"required":             []string{"template_selector"},
			"additionalProperties": false,
		},
		Preflight: validateBaselineComplianceArgs,
		Handler:   makeBaselineComplianceRunHandler(deps),
	}); err != nil {
		return err
	}

	return registry.Register(&assistant.ToolSpec{
		Name:               "Operation.Get",
		Domain:             assistant.DomainSystem,
		Operation:          assistant.OpGet,
		Capability:         "get_operation_status",
		Description:        "Get and advance the durable status, stage, coverage, references, and terminal outcome of a high-level assistant operation.",
		Aliases:            []string{"操作状态", "流程进度", "operation status"},
		Tags:               []string{"v6.1", "operation", "status"},
		ObjectTypes:        []string{"operation"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ExposurePolicy: assistant.ToolExposurePolicy{
			Exposure:       assistant.ToolExposureCompanion,
			Discoverable:   false,
			DirectCallable: true,
		},
		ResultContract: assistant.ToolResultContract{
			OperationStatusField:  "operation_status",
			SuccessValues:         []string{"succeeded", "partially_succeeded"},
			PendingValues:         []string{"accepted", "preparing_template", "preparing_scripts", "dispatching", "running"},
			FailureValues:         []string{"failed", "cancelled"},
			OperationRefFields:    []string{"operation_id"},
			SideEffectRefFields:   []string{"task_group_id"},
			SatisfiesCapabilities: []string{"run_baseline_compliance"},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"operation_id": map[string]interface{}{"type": "string", "format": "uuid", "description": "Operation UUID returned by a high-level assistant tool."},
			},
			"required":             []string{"operation_id"},
			"additionalProperties": false,
		},
		Handler: makeOperationGetHandler(deps),
	})
}

func validateBaselineComplianceArgs(ctx context.Context, args map[string]interface{}) error {
	if err := validateHostResolveArgs(ctx, args); err != nil {
		return fmt.Errorf("host scope: %w", err)
	}
	scope := strings.ToLower(strings.TrimSpace(getStringArg(args, "scope", "all_rules")))
	if scope != "all_rules" {
		return fmt.Errorf("scope must be all_rules")
	}
	if _, _, err := parseRemediationPolicy(args); err != nil {
		return err
	}
	return nil
}

func makeBaselineComplianceRunHandler(deps BaselineToolDeps) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		if deps.OperationRepo == nil || deps.TemplateRepo == nil || deps.RuleRepo == nil || deps.TaskService == nil || deps.TaskLogRepo == nil {
			return nil, fmt.Errorf("baseline compliance dependencies are unavailable")
		}
		resolved, err := resolveHostTargetInput(ctx, deps.HostRepo, deps.ServerClient, args, true)
		if err != nil {
			return nil, err
		}
		if resolved.OperationStatus != "succeeded" || len(resolved.Resolved) == 0 {
			return nil, fmt.Errorf("host scope is not executable: target_scope=%s resolved=%d requested=%d ambiguous=%d unresolved=%d offline=%d", resolved.TargetScope, len(resolved.Resolved), len(resolved.Requested), len(resolved.Ambiguous), len(resolved.Unresolved), len(resolved.Offline))
		}

		template, err := resolveBaselineTemplate(deps.TemplateRepo, getStringArg(args, "template_selector", ""))
		if err != nil {
			return nil, err
		}
		scope := strings.ToLower(strings.TrimSpace(getStringArg(args, "scope", "all_rules")))
		if scope != "all_rules" {
			return nil, fmt.Errorf("scope must be all_rules")
		}
		remediation, maxRounds, err := parseRemediationPolicy(args)
		if err != nil {
			return nil, err
		}
		hostIDs := make([]string, 0, len(resolved.Resolved))
		for _, host := range resolved.Resolved {
			hostIDs = append(hostIDs, fmt.Sprint(host["host_id"]))
		}
		sort.Strings(hostIDs)
		request := baselineComplianceRequest{
			HostIDs:        hostIDs,
			TargetScope:    resolved.TargetScope,
			TemplateID:     template.ID.String(),
			Scope:          scope,
			Remediation:    remediation,
			MaxRounds:      maxRounds,
			IdempotencyKey: strings.TrimSpace(getStringArg(args, "idempotency_key", "")),
		}
		requestJSON, _ := json.Marshal(request)
		resolvedScopeJSON, _ := json.Marshal(map[string]interface{}{
			"host_ids":     hostIDs,
			"host_count":   len(hostIDs),
			"target_scope": resolved.TargetScope,
			"template_id":  template.ID.String(),
			"scope":        scope,
		})
		invocation, _ := assistant.ToolInvocationFromContext(ctx)
		if request.IdempotencyKey != "" {
			existing, found, findErr := deps.OperationRepo.FindByIdempotencyKey(ctx, invocation.SessionID, "baseline_compliance", request.IdempotencyKey)
			if findErr != nil {
				return nil, fmt.Errorf("check baseline operation idempotency: %w", findErr)
			}
			if found && existing != nil {
				if string(existing.Request) != string(requestJSON) {
					return nil, fmt.Errorf("idempotency_key is already bound to a different baseline request")
				}
				return advanceBaselineOperation(ctx, deps, existing)
			}
		}
		taskGroupID := uuid.New()
		operation := &model.AssistantOperation{
			ID:              uuid.New(),
			Type:            baselineComplianceOperationType,
			SessionID:       invocation.SessionID,
			RunID:           invocation.RunID,
			WorkflowID:      "baseline_compliance",
			WorkflowVersion: "6.1",
			Status:          "accepted",
			CurrentStage:    "resolve_scope",
			Request:         requestJSON,
			ResolvedScope:   resolvedScopeJSON,
			Result:          []byte("{}"),
			Counts:          []byte("{}"),
			References:      []byte("{}"),
			Violations:      []byte("[]"),
			TaskGroupID:     &taskGroupID,
			IdempotencyKey:  request.IdempotencyKey,
			CreatedBy:       invocation.Operator,
		}
		if err := deps.OperationRepo.Create(ctx, operation); err != nil {
			return nil, fmt.Errorf("create baseline operation: %w", err)
		}
		if deps.Logger != nil {
			deps.Logger.Info("assistant baseline compliance operation accepted",
				zap.String("operation_id", operation.ID.String()),
				zap.String("template_id", template.ID.String()),
				zap.String("target_scope", resolved.TargetScope),
				zap.Int("host_count", len(hostIDs)),
				zap.Bool("remediation", remediation),
				zap.Int("max_rounds", maxRounds),
			)
		}
		return advanceBaselineOperation(ctx, deps, operation)
	}
}

func uniqueSelectors(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		key := strings.ToLower(trimmed)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func makeOperationGetHandler(deps BaselineToolDeps) assistant.ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		operationID, err := parseUUID(args, "operation_id")
		if err != nil {
			return nil, err
		}
		if deps.OperationRepo == nil {
			return nil, fmt.Errorf("operation repository not configured")
		}
		operation, err := deps.OperationRepo.FindByID(ctx, operationID)
		if err != nil {
			return nil, fmt.Errorf("find operation: %w", err)
		}
		switch operation.Type {
		case baselineComplianceOperationType:
			return advanceBaselineOperation(ctx, deps, operation)
		default:
			return operationResult(operation, map[string]interface{}{}), nil
		}
	}
}

func advanceBaselineOperation(ctx context.Context, deps BaselineToolDeps, operation *model.AssistantOperation) (interface{}, error) {
	if operation == nil {
		return nil, fmt.Errorf("operation is nil")
	}
	var request baselineComplianceRequest
	if err := json.Unmarshal(operation.Request, &request); err != nil {
		return failBaselineOperation(ctx, deps, operation, "invalid_operation_request", err)
	}
	if operation.Status == "succeeded" || operation.Status == "partially_succeeded" || operation.Status == "failed" || operation.Status == "cancelled" {
		return operationResult(operation, decodeOperationResult(operation.Result)), nil
	}

	templateID, err := uuid.Parse(request.TemplateID)
	if err != nil {
		return failBaselineOperation(ctx, deps, operation, "invalid_template_id", err)
	}
	template, err := deps.TemplateRepo.FindByID(templateID)
	if err != nil {
		return failBaselineOperation(ctx, deps, operation, "template_not_found", err)
	}
	if strings.EqualFold(template.Status, "failed") {
		return failBaselineOperation(ctx, deps, operation, "template_parse_failed", fmt.Errorf("template parsing failed"))
	}
	if !strings.EqualFold(template.Status, "completed") {
		result := map[string]interface{}{"stage": "wait_template_parsed", "template_id": templateID.String(), "template_status": template.Status}
		_, _ = deps.OperationRepo.Transition(ctx, operation.ID, []string{"accepted", "preparing_template"}, "preparing_template", result)
		operation.Status = "preparing_template"
		return operationResult(operation, result), nil
	}

	rules, err := deps.RuleRepo.FindByTemplateID(templateID)
	if err != nil {
		return failBaselineOperation(ctx, deps, operation, "rule_enumeration_failed", err)
	}
	if len(rules) == 0 {
		return failBaselineOperation(ctx, deps, operation, "empty_rule_scope", fmt.Errorf("template contains no rules"))
	}
	readiness := inspectBaselineScriptReadiness(rules, request.Remediation)
	if readiness.failed > 0 {
		return failBaselineOperation(ctx, deps, operation, "script_generation_failed", fmt.Errorf("%d required scripts failed generation", readiness.failed))
	}
	if readiness.missing > 0 {
		result := map[string]interface{}{
			"stage":            "ensure_scripts_ready",
			"template_id":      templateID.String(),
			"rule_count":       len(rules),
			"required_scripts": readiness.required,
			"ready_scripts":    readiness.ready,
			"missing_scripts":  readiness.missing,
		}
		if deps.ScriptGenService == nil {
			return failBaselineOperation(ctx, deps, operation, "script_service_unavailable", fmt.Errorf("script generation service not configured"))
		}
		checkBatch, batchErr := deps.ScriptGenService.BatchGenerateForTemplate(ctx, templateID, "CHECK", 2)
		if batchErr != nil {
			return failBaselineOperation(ctx, deps, operation, "check_script_queue_failed", batchErr)
		}
		result["check_generation"] = checkBatch
		if request.Remediation {
			fixBatch, fixErr := deps.ScriptGenService.BatchGenerateForTemplate(ctx, templateID, "FIX", 2)
			if fixErr != nil {
				return failBaselineOperation(ctx, deps, operation, "fix_script_queue_failed", fixErr)
			}
			result["fix_generation"] = fixBatch
		}
		_, _ = deps.OperationRepo.Transition(ctx, operation.ID, []string{"accepted", "preparing_template", "preparing_scripts"}, "preparing_scripts", result)
		operation.Status = "preparing_scripts"
		return operationResult(operation, result), nil
	}
	if operation.TaskGroupID == nil {
		return failBaselineOperation(ctx, deps, operation, "missing_task_group", fmt.Errorf("task group reference is missing"))
	}

	if operation.Status != "running" {
		claimed, err := deps.OperationRepo.Transition(ctx, operation.ID, []string{"accepted", "preparing_template", "preparing_scripts"}, "dispatching", map[string]interface{}{"stage": "dispatch_check"})
		if err != nil {
			return nil, fmt.Errorf("claim baseline dispatch: %w", err)
		}
		if claimed {
			ruleIDs := make([]string, 0, len(rules))
			for _, rule := range rules {
				ruleIDs = append(ruleIDs, rule.ID.String())
			}
			dispatchResult, dispatchErr := deps.TaskService.CreateAndDispatchTasks(ctx, ruleIDs, request.HostIDs, "CHECK", &service.DispatchOptions{AutoVerify: request.Remediation, MaxRounds: request.MaxRounds}, *operation.TaskGroupID)
			if dispatchErr != nil {
				return failBaselineOperation(ctx, deps, operation, "task_dispatch_failed", dispatchErr)
			}
			result := map[string]interface{}{
				"stage":          "monitor_check",
				"rule_count":     len(rules),
				"host_count":     len(request.HostIDs),
				"expected_count": dispatchResult.ExpectedCount,
				"created_count":  dispatchResult.CreatedCount,
				"task_group_id":  dispatchResult.TaskGroupID.String(),
			}
			if err := deps.OperationRepo.Update(ctx, operation.ID, "running", result, "", "", false); err != nil {
				return nil, fmt.Errorf("persist running baseline operation: %w", err)
			}
			operation.Status = "running"
			operation.Result, _ = json.Marshal(result)
		}
	}

	return monitorBaselineTaskGroup(ctx, deps, operation, request, len(rules))
}

type baselineScriptReadiness struct{ required, ready, missing, failed int }

func inspectBaselineScriptReadiness(rules []model.AegisRule, remediation bool) baselineScriptReadiness {
	result := baselineScriptReadiness{}
	for _, rule := range rules {
		result.required++
		if strings.EqualFold(rule.CheckScriptStatus, "failed") {
			result.failed++
		} else if rule.GeneratedCheckScript != nil && strings.TrimSpace(*rule.GeneratedCheckScript) != "" && strings.EqualFold(rule.CheckScriptStatus, "generated") {
			result.ready++
		} else {
			result.missing++
		}
		if remediation {
			result.required++
			if strings.EqualFold(rule.FixScriptStatus, "failed") {
				result.failed++
			} else if rule.GeneratedFixScript != nil && strings.TrimSpace(*rule.GeneratedFixScript) != "" && strings.EqualFold(rule.FixScriptStatus, "generated") {
				result.ready++
			} else {
				result.missing++
			}
		}
	}
	return result
}

func monitorBaselineTaskGroup(ctx context.Context, deps BaselineToolDeps, operation *model.AssistantOperation, request baselineComplianceRequest, ruleCount int) (interface{}, error) {
	if operation.TaskGroupID == nil {
		return failBaselineOperation(ctx, deps, operation, "missing_task_group", fmt.Errorf("task group reference is missing"))
	}
	tasks, err := deps.TaskLogRepo.FindByGroupID(*operation.TaskGroupID)
	if err != nil {
		return nil, fmt.Errorf("get baseline task group: %w", err)
	}
	expected := ruleCount * len(request.HostIDs)
	counts := map[string]int{"expected": expected, "created": len(tasks)}
	terminal := len(tasks) >= expected && expected > 0
	latestChecks := make(map[string]model.TaskLog)
	for _, task := range tasks {
		status := strings.ToUpper(strings.TrimSpace(task.Status))
		counts[strings.ToLower(status)]++
		if status == "PENDING" || status == "RUNNING" || status == "" {
			terminal = false
		}
		if strings.EqualFold(task.TaskType, "CHECK") && task.RuleID != nil {
			key := task.RuleID.String() + ":" + task.HostID.String()
			if current, exists := latestChecks[key]; !exists || task.VerifyRound >= current.VerifyRound {
				latestChecks[key] = task
			}
		}
	}
	result := map[string]interface{}{
		"stage":          "monitor_check",
		"task_group_id":  operation.TaskGroupID.String(),
		"rule_count":     ruleCount,
		"host_count":     len(request.HostIDs),
		"expected_count": expected,
		"created_count":  len(tasks),
		"counts":         counts,
	}
	if !terminal {
		operation.Status = "running"
		_ = deps.OperationRepo.Update(ctx, operation.ID, "running", result, "", "", false)
		return operationResult(operation, result), nil
	}

	noncompliant := 0
	for _, task := range latestChecks {
		if task.ExitCode == nil || *task.ExitCode != 0 {
			noncompliant++
		}
	}
	result["noncompliant_count"] = noncompliant
	result["coverage_complete"] = len(latestChecks) == expected
	status := "succeeded"
	if len(latestChecks) != expected || counts["failed"] > 0 || counts["timeout"] > 0 || counts["audit_blocked"] > 0 || (request.Remediation && noncompliant > 0) {
		status = "partially_succeeded"
	}
	if len(tasks) == 0 {
		status = "failed"
	}
	result["stage"] = "verify_coverage"
	if err := deps.OperationRepo.Update(ctx, operation.ID, status, result, "", "", true); err != nil {
		return nil, fmt.Errorf("persist terminal baseline operation: %w", err)
	}
	operation.Status = status
	operation.Result, _ = json.Marshal(result)
	if deps.Logger != nil {
		deps.Logger.Info("assistant baseline compliance operation completed",
			zap.String("operation_id", operation.ID.String()),
			zap.String("task_group_id", operation.TaskGroupID.String()),
			zap.String("status", status),
			zap.Int("expected_count", expected),
			zap.Int("created_count", len(tasks)),
			zap.Int("noncompliant_count", noncompliant),
		)
	}
	return operationResult(operation, result), nil
}

func failBaselineOperation(ctx context.Context, deps BaselineToolDeps, operation *model.AssistantOperation, code string, cause error) (interface{}, error) {
	result := map[string]interface{}{"stage": "failed", "error_code": code, "error_message": cause.Error()}
	if deps.OperationRepo != nil && operation != nil {
		_ = deps.OperationRepo.Update(ctx, operation.ID, "failed", result, code, cause.Error(), true)
		operation.Status = "failed"
		operation.ErrorCode = code
		operation.ErrorMessage = cause.Error()
		operation.Result, _ = json.Marshal(result)
	}
	if deps.Logger != nil && operation != nil {
		deps.Logger.Warn("assistant baseline compliance operation failed", zap.String("operation_id", operation.ID.String()), zap.String("error_code", code), zap.Error(cause))
	}
	return operationResult(operation, result), nil
}

func operationResult(operation *model.AssistantOperation, result map[string]interface{}) map[string]interface{} {
	output := make(map[string]interface{}, len(result)+6)
	for key, value := range result {
		output[key] = value
	}
	if operation == nil {
		output["operation_status"] = "failed"
		return output
	}
	output["operation_id"] = operation.ID.String()
	output["operation_type"] = operation.Type
	output["operation_status"] = operation.Status
	output["terminal"] = operation.Status == "succeeded" || operation.Status == "partially_succeeded" || operation.Status == "failed" || operation.Status == "cancelled"
	if operation.TaskGroupID != nil {
		output["task_group_id"] = operation.TaskGroupID.String()
	}
	if operation.ErrorCode != "" {
		output["error_code"] = operation.ErrorCode
		output["error_message"] = operation.ErrorMessage
	}
	return output
}

func decodeOperationResult(value []byte) map[string]interface{} {
	result := make(map[string]interface{})
	_ = json.Unmarshal(value, &result)
	return result
}

func resolveBaselineTemplate(repo TemplateRepositoryForTools, selector string) (*model.Template, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, fmt.Errorf("template_selector is required")
	}
	if templateID, err := uuid.Parse(selector); err == nil {
		template, findErr := repo.FindByID(templateID)
		if findErr != nil {
			return nil, fmt.Errorf("find baseline template %s: %w", templateID, findErr)
		}
		return template, nil
	}
	templates, err := repo.FindAll(1, 1000)
	if err != nil {
		return nil, fmt.Errorf("list baseline templates: %w", err)
	}
	var exact, partial []model.Template
	for _, template := range templates {
		if strings.EqualFold(template.Name, selector) || strings.EqualFold(template.DisplayName, selector) {
			exact = append(exact, template)
		} else if strings.Contains(strings.ToLower(template.Name), strings.ToLower(selector)) || strings.Contains(strings.ToLower(template.DisplayName), strings.ToLower(selector)) {
			partial = append(partial, template)
		}
	}
	if len(exact) == 1 {
		return &exact[0], nil
	}
	if len(exact) > 1 || len(partial) > 1 {
		return nil, fmt.Errorf("template_selector %q is ambiguous", selector)
	}
	if len(partial) == 1 {
		return &partial[0], nil
	}
	return nil, fmt.Errorf("template_selector %q did not match a template", selector)
}

func parseRemediationPolicy(args map[string]interface{}) (bool, int, error) {
	maxRounds := 3
	value, exists := args["remediation"]
	if !exists || value == nil {
		return false, maxRounds, nil
	}
	policy, ok := value.(map[string]interface{})
	if !ok {
		return false, 0, fmt.Errorf("remediation must be an object")
	}
	enabled := getBoolArg(policy, "enabled", false)
	maxRounds = getIntArg(policy, "max_rounds", 3)
	if maxRounds < 1 || maxRounds > 10 {
		return false, 0, fmt.Errorf("remediation.max_rounds must be between 1 and 10")
	}
	return enabled, maxRounds, nil
}

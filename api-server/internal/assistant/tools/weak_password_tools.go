package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"

	"github.com/google/uuid"
)

type WeakPasswordServiceForTools interface {
	AnalyzeAssetApplications(ctx context.Context, req model.AnalyzeAssetApplicationsRequest, createdBy *uuid.UUID) (*service.AnalyzeAssetApplicationsResponse, error)
	CreateTaskByApplication(ctx context.Context, req model.CreateTaskByApplicationRequest, createdBy *uuid.UUID) (*service.CreateTaskByApplicationResponse, error)
	CreateTasksByApplications(ctx context.Context, req model.CreateTasksByApplicationsRequest, createdBy *uuid.UUID) (*service.CreateTasksByApplicationsResponse, error)
	GenerateAIDictionary(ctx context.Context, req model.AIGenerateDictionaryRequest, createdBy *uuid.UUID) (*service.DictionarySummary, error)
	GetTaskProgress(taskID uuid.UUID) (*model.TaskProgressResponse, error)
	ListTaskCollectionProgress(taskID uuid.UUID, page, pageSize int) ([]service.WeakPasswordCollectionProgressDTO, int64, error)
	ListTaskFindings(taskID uuid.UUID, page, pageSize int) ([]model.WeakPasswordFinding, int64, error)
}

type WeakPasswordToolDeps struct {
	Service WeakPasswordServiceForTools
}

func RegisterWeakPasswordTools(registry *assistant.ToolRegistry, deps WeakPasswordToolDeps) error {
	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.GenerateDictionary",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGenerate,
		Capability:         "weak_password_dictionary_generation",
		Description:        "Generate and save a weak-password dictionary from natural-language requirements using the weak-password AI generation service.",
		Aliases:            []string{"AI生成弱密码", "生成弱密码字典", "弱密码字典生成", "AI 一键生成字典"},
		Tags:               []string{"weak_password", "credential", "dictionary", "ai", "v6.1"},
		ObjectTypes:        []string{"dictionary"},
		PageRoutes:         []string{"/risk/weak-password/dictionaries"},
		Risk:               assistant.ToolRiskLow,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         false,
		DefaultWhitelisted: true,
		Enabled:            true,
		DefaultTimeout:     120 * time.Second,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "GenerateAIDictionary",
			Notes:     "Uses the LLM generation and persistence path without a random hard-coded fallback.",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"natural_language": map[string]interface{}{"type": "string", "description": "Natural-language requirements for the weak-password dictionary."},
				"count":            map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20},
				"application_type": map[string]interface{}{"type": "string"},
				"organization_keywords": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"account_keywords": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"deduplicate_with_default": map[string]interface{}{"type": "boolean", "default": true},
			},
			"required": []string{"natural_language"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			req := model.AIGenerateDictionaryRequest{
				NaturalLanguage:        getStringArg(args, "natural_language", ""),
				ApplicationType:        getStringArg(args, "application_type", ""),
				Count:                  getIntArg(args, "count", 20),
				DeduplicateWithDefault: getBoolArg(args, "deduplicate_with_default", true),
			}
			if req.NaturalLanguage == "" {
				return nil, fmt.Errorf("natural_language is required")
			}
			req.OrganizationKeywords = getOptionalStringSliceArg(args, "organization_keywords")
			req.AccountKeywords = getOptionalStringSliceArg(args, "account_keywords")
			summary, err := deps.Service.GenerateAIDictionary(ctx, req, nil)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"dictionary":  summary,
				"next_action": "The dictionary was saved. Review it or analyze candidate applications before starting an assessment.",
				"route_path":  "/risk/weak-password/dictionaries",
			}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.AnalyzeApplications",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGenerate,
		Capability:         "weak_password_asset_analysis",
		Description:        "Analyze application assets on online hosts, select externally recognizable password-authenticated applications, and deduplicate by host and application type.",
		Aliases:            []string{"弱密码应用分析", "分析弱密码资产", "应用资产分析", "弱口令应用识别"},
		Tags:               []string{"weak_password", "asset", "application", "ai", "v6.1"},
		ObjectTypes:        []string{"candidate_application", "host_application_asset"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskLow,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         false,
		DefaultWhitelisted: true,
		Enabled:            true,
		DefaultTimeout:     60 * time.Second,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "AnalyzeAssetApplications",
			Notes:     "The backend enforces online_agents_only=true and deduplicates by host and application_type.",
		},
		ResultContract: assistant.ToolResultContract{
			FactBindings: []assistant.ToolFactBinding{{
				Kind:       "candidate_application_resolved",
				ItemsField: "analysis.candidates",
				IDField:    "candidate_application_id",
			}},
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"application_types": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"online_agents_only": map[string]interface{}{"type": "boolean", "default": true},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			req := model.AnalyzeAssetApplicationsRequest{}
			req.Scope.HostIDs = getOptionalStringSliceArg(args, "host_ids")
			req.Scope.ApplicationTypes = getOptionalStringSliceArg(args, "application_types")
			req.Scope.OnlineAgentsOnly = getBoolArg(args, "online_agents_only", true)
			resp, err := deps.Service.AnalyzeAssetApplications(ctx, req, nil)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"analysis":    resp,
				"next_action": "Select candidate_application_id values, create the assessment task, and monitor it with the progress companion tool.",
				"route_path":  "/risk/weak-password",
			}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.Scan",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpExecute,
		Capability:         "weak_password_scan",
		Description:        "Create a weak-password assessment task for selected candidate applications through the V6.1 workflow service.",
		Aliases:            []string{"弱密码检查", "弱口令扫描", "检查弱密码"},
		Tags:               []string{"weak_password", "credential", "v6.1"},
		ObjectTypes:        []string{"candidate_application", "task"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskMedium,
		AutoCallable:       false,
		RequiresApproval:   true,
		Idempotent:         false,
		DefaultWhitelisted: false,
		Enabled:            true,
		DefaultTimeout:     120 * time.Second,
		ExecutionContract: assistant.ToolExecutionContract{
			Mode:                 assistant.ToolExecutionAsynchronous,
			CompletionCapability: "weak_password_progress",
		},
		ResultContract: assistant.ToolResultContract{
			AcceptedOnSuccess: true,
			FactBindings: []assistant.ToolFactBinding{{
				Kind:       "task_resolved",
				ItemsField: "created",
				IDField:    "task_id",
			}},
		},
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "CreateTasksByApplications",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"candidate_application_ids": map[string]interface{}{
					"type":     "array",
					"minItems": 1,
					"items":    map[string]interface{}{"type": "string", "format": "uuid"},
				},
			},
			"required":             []string{"candidate_application_ids"},
			"additionalProperties": false,
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			candidateIDs, err := getStringSliceArg(args, "candidate_application_ids")
			if err != nil || len(candidateIDs) == 0 {
				return nil, fmt.Errorf("candidate_application_ids is required")
			}
			req := model.CreateTasksByApplicationsRequest{CandidateApplicationIDs: candidateIDs}
			req.DictionaryPolicy.UseDefault1000 = true
			req.AIPolicy.RepairCollectionErrors = true
			req.AIPolicy.DetectionRounds = 10
			req.AIPolicy.MaxAgentToolCallsPerApp = 10
			resp, err := deps.Service.CreateTasksByApplications(ctx, req, nil)
			if err != nil {
				return nil, err
			}
			if resp == nil || len(resp.Created) == 0 {
				return nil, fmt.Errorf("no weak-password assessment tasks were created")
			}
			return map[string]interface{}{
				"status":      model.TaskStatusPending,
				"created":     resp.Created,
				"skipped":     resp.Skipped,
				"next_action": "Monitor every created task_id with the progress companion tool until the aggregate status is terminal.",
				"route_path":  "/risk/weak-password",
			}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.QueryFindings",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpList,
		Capability:         "weak_password_findings",
		Description:        "Get weak-password assessment findings with passwords redacted by default.",
		Aliases:            []string{"弱密码结果", "弱口令命中"},
		Tags:               []string{"weak_password", "finding", "v6.1"},
		ObjectTypes:        []string{"finding", "task"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "ListTaskFindings",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{"type": "string"},
			},
			"required": []string{"task_id"},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			_ = ctx
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			taskID, err := uuid.Parse(getStringArg(args, "task_id", ""))
			if err != nil {
				return nil, fmt.Errorf("invalid task_id: %w", err)
			}
			findings, total, err := deps.Service.ListTaskFindings(taskID, 1, 20)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"items": findings, "total": total}, nil
		},
	}); err != nil {
		return err
	}

	if err := registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.QueryProgress",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGet,
		Capability:         "weak_password_progress",
		Description:        "Get overall and collection progress for a weak-password assessment, including agent tool rounds, status, error code, and error message.",
		Aliases:            []string{"弱密码进度", "弱口令进度", "采集进度", "弱密码任务进度"},
		Tags:               []string{"weak_password", "progress", "task", "v6.1"},
		ObjectTypes:        []string{"task", "collection_progress"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		RequiresApproval:   false,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ResultContract: assistant.ToolResultContract{
			OperationStatusField: "status",
			// QueryProgress is a read-only observer. Every terminal workflow
			// state is successful terminal evidence for the observation,
			// while the raw status and failed-task details continue to
			// describe the scan result.
			SuccessValues: []string{
				model.TaskStatusCompleted,
				model.TaskStatusPartialFailed,
				model.TaskStatusFailed,
				model.TaskStatusCancelled,
			},
			PendingValues: []string{
				model.TaskStatusPending,
				model.TaskStatusAnalyzingAssets,
				model.TaskStatusCollectingCredentials,
				model.TaskStatusRepairingCollection,
				model.TaskStatusMatching,
				"running",
			},
			SatisfiesCapabilities: []string{"weak_password_scan"},
			FactBindings: []assistant.ToolFactBinding{{
				Kind:       "task_resolved",
				ItemsField: "tasks",
				IDField:    "task_id",
			}},
		},
		ServiceBinding: assistant.ServiceBinding{
			Component: "weak_password_service",
			File:      "api-server/internal/service/weak_password_service.go",
			Function:  "GetTaskProgress/ListTaskCollectionProgress",
		},
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_ids": map[string]interface{}{
					"type":     "array",
					"minItems": 1,
					"items":    map[string]interface{}{"type": "string", "format": "uuid"},
				},
				"page_size": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20},
			},
			"required":             []string{"task_ids"},
			"additionalProperties": false,
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			_ = ctx
			if deps.Service == nil {
				return nil, fmt.Errorf("weak password service not configured")
			}
			rawTaskIDs, err := getStringSliceArg(args, "task_ids")
			if err != nil || len(rawTaskIDs) == 0 {
				return nil, fmt.Errorf("task_ids is required")
			}
			pageSize := getIntArg(args, "page_size", 20)
			if pageSize <= 0 || pageSize > 50 {
				pageSize = 20
			}
			tasks := make([]map[string]interface{}, 0, len(rawTaskIDs))
			progresses := make([]*model.TaskProgressResponse, 0, len(rawTaskIDs))
			var collectionTotal int64
			matchedFindings := 0
			for _, rawTaskID := range rawTaskIDs {
				taskID, err := uuid.Parse(rawTaskID)
				if err != nil {
					return nil, fmt.Errorf("invalid task_id %q: %w", rawTaskID, err)
				}
				progress, err := deps.Service.GetTaskProgress(taskID)
				if err != nil {
					return nil, err
				}
				items, total, err := deps.Service.ListTaskCollectionProgress(taskID, 1, pageSize)
				if err != nil {
					return nil, err
				}
				progresses = append(progresses, progress)
				matchedFindings += progress.MatchedFindings
				collectionTotal += total
				tasks = append(tasks, map[string]interface{}{
					"task_id":                   taskID.String(),
					"task_progress":             progress,
					"collection_progress":       items,
					"collection_progress_total": total,
				})
			}
			summary := summarizeWeakPasswordTaskProgresses(progresses)
			result := map[string]interface{}{
				"status":                    summary.status,
				"task_ids":                  rawTaskIDs,
				"tasks":                     tasks,
				"task_total":                summary.total,
				"task_completed":            summary.completed,
				"task_failed":               summary.failed,
				"task_running":              summary.running,
				"failed_tasks":              summary.failedTasks,
				"collection_progress_total": collectionTotal,
				"matched_findings":          matchedFindings,
				"next_action":               "Poll again while any task is active. On terminal failure, inspect error_code and error_message in the per-task collection progress.",
				"route_path":                "/risk/weak-password",
			}
			// Preserve the single-task projection for callers that display the
			// original progress payload while the assistant uses batch status.
			if len(tasks) == 1 {
				result["task_progress"] = tasks[0]["task_progress"]
				result["collection_progress"] = tasks[0]["collection_progress"]
				result["route_path"] = "/risk/weak-password/tasks/" + rawTaskIDs[0]
			}
			return result, nil
		},
	}); err != nil {
		return err
	}

	return registry.Register(&assistant.ToolSpec{
		Name:               "Credential.WeakPassword.Explain",
		Domain:             assistant.DomainDetection,
		Operation:          assistant.OpGet,
		Capability:         "weak_password_explain",
		Description:        "Explain weak-password evidence and remediation guidance without exposing full plaintext passwords.",
		Aliases:            []string{"解释弱密码", "弱密码建议"},
		Tags:               []string{"weak_password", "explain", "v6.1"},
		ObjectTypes:        []string{"finding"},
		PageRoutes:         []string{"/risk/weak-password"},
		Risk:               assistant.ToolRiskReadonly,
		AutoCallable:       true,
		Idempotent:         true,
		DefaultWhitelisted: true,
		Enabled:            true,
		ArgsSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"finding_id": map[string]interface{}{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			_ = ctx
			return map[string]interface{}{
				"summary":        "The backend dictionary or verifier confirmed the weak-password finding; passwords remain redacted.",
				"recommendation": "Change affected credentials, disable default passwords, restrict configuration-file permissions, and run a post-remediation assessment.",
			}, nil
		},
	})
}

func getOptionalStringSliceArg(args map[string]interface{}, key string) []string {
	values, err := getStringSliceArg(args, key)
	if err != nil {
		return nil
	}
	return values
}

func aggregateWeakPasswordTaskStatus(progresses []*model.TaskProgressResponse) string {
	return summarizeWeakPasswordTaskProgresses(progresses).status
}

type weakPasswordTaskProgressSummary struct {
	status      string
	total       int
	completed   int
	failed      int
	running     int
	failedTasks []map[string]interface{}
}

func summarizeWeakPasswordTaskProgresses(progresses []*model.TaskProgressResponse) weakPasswordTaskProgressSummary {
	summary := weakPasswordTaskProgressSummary{
		total:       len(progresses),
		failedTasks: make([]map[string]interface{}, 0),
	}
	if len(progresses) == 0 {
		summary.status = model.TaskStatusFailed
		return summary
	}
	for _, progress := range progresses {
		if progress == nil {
			summary.failed++
			continue
		}
		switch progress.Status {
		case model.TaskStatusCompleted:
			summary.completed++
		case model.TaskStatusPartialFailed, model.TaskStatusFailed, model.TaskStatusCancelled:
			summary.failed++
			errorCode := strings.TrimSpace(progress.LastErrorCode)
			if errorCode == "" {
				errorCode = strings.TrimSpace(progress.CurrentStage)
			}
			summary.failedTasks = append(summary.failedTasks, map[string]interface{}{
				"task_id":          progress.TaskID,
				"application_name": progress.CurrentApplication,
				"status":           progress.Status,
				"error_code":       errorCode,
				"error_message":    strings.TrimSpace(progress.Message),
			})
		default:
			summary.running++
		}
	}
	switch {
	case summary.running > 0:
		summary.status = "running"
	case summary.failed == 0:
		summary.status = model.TaskStatusCompleted
	case summary.completed == 0:
		summary.status = model.TaskStatusFailed
	default:
		summary.status = model.TaskStatusPartialFailed
	}
	return summary
}

package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"api-server/internal/model"
	"api-server/internal/recovery"
	"api-server/internal/service"
	"go.uber.org/zap"
)

const hookAllowlistRecoveryExecutor = "hook_allowlist"

type HookAllowlistRecoveryExecutor struct {
	service *service.DetectionPackageService
	logger  *zap.Logger
}

func NewHookAllowlistRecoveryExecutor(svc *service.DetectionPackageService, logger *zap.Logger) *HookAllowlistRecoveryExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HookAllowlistRecoveryExecutor{service: svc, logger: logger}
}

func (e *HookAllowlistRecoveryExecutor) ExecuteRecoveryAction(
	ctx context.Context,
	request *model.AssistantRecoveryRequest,
	action recovery.Action,
	_ map[string]interface{},
	operator string,
) (map[string]interface{}, error) {
	if e == nil || e.service == nil {
		return nil, fmt.Errorf("detection package service unavailable")
	}
	required, err := requiredHooksFromRecovery(request)
	if err != nil {
		return nil, err
	}
	current, err := e.service.GetActiveHookAllowlist(ctx)
	if err != nil {
		return nil, fmt.Errorf("load active hook allowlist: %w", err)
	}
	if current == nil {
		current = &service.AllowlistConfig{}
	}
	updated := cloneAllowlist(current)
	added, err := mergeRequiredHooks(updated, required)
	proposal := map[string]interface{}{
		"required_hooks": required,
		"added_hooks":    added,
		"current":        current,
		"proposed":       updated,
	}
	if action.ID == "prepare_hook_allowlist_change" {
		if err != nil || len(required) == 0 {
			proposal["manual_review_required"] = true
			if err != nil {
				proposal["validation_error"] = err.Error()
			} else {
				proposal["validation_error"] = "the generator did not provide exact required hooks"
			}
		}
		return proposal, nil
	}
	if action.ID != "extend_hook_allowlist" {
		return nil, fmt.Errorf("unsupported hook allowlist recovery action %q", action.ID)
	}
	if err != nil {
		return nil, err
	}
	if len(required) == 0 {
		return nil, fmt.Errorf("cannot extend hook allowlist without validated required hooks")
	}
	proposal["changed"] = len(added) > 0
	configJSON, err := json.Marshal(updated)
	if err != nil {
		return nil, fmt.Errorf("marshal updated hook allowlist: %w", err)
	}
	row, err := e.service.UpdateAllowlist(
		ctx,
		configJSON,
		fmt.Sprintf("Assistant recovery %s: add required hooks for %s", request.RecoveryID, recoveryContextString(request, "cve_id")),
		operator,
	)
	if err != nil {
		return nil, fmt.Errorf("apply and sync hook allowlist recovery: %w", err)
	}
	proposal["allowlist_version"] = row.Version
	e.logger.Info("assistant recovery extended active hook allowlist",
		zap.String("recovery_id", request.RecoveryID),
		zap.String("session_id", request.SessionID),
		zap.Int64("allowlist_version", row.Version),
		zap.Strings("added_hooks", added),
	)
	return proposal, nil
}

type recoveryHookRequirement struct {
	AttachType string `json:"attach_type"`
	Attach     string `json:"attach"`
}

func requiredHooksFromRecovery(request *model.AssistantRecoveryRequest) ([]recoveryHookRequirement, error) {
	var contextData struct {
		RequiredHooks []recoveryHookRequirement `json:"required_hooks"`
	}
	if request == nil {
		return nil, fmt.Errorf("recovery request is required")
	}
	if err := json.Unmarshal(request.Context, &contextData); err != nil {
		return nil, fmt.Errorf("recovery request context is invalid: %w", err)
	}
	return contextData.RequiredHooks, nil
}

func recoveryContextString(request *model.AssistantRecoveryRequest, key string) string {
	var contextData map[string]interface{}
	if request == nil || json.Unmarshal(request.Context, &contextData) != nil {
		return ""
	}
	return strings.TrimSpace(recoveryStringValue(contextData[key]))
}

func cloneAllowlist(input *service.AllowlistConfig) *service.AllowlistConfig {
	return &service.AllowlistConfig{
		Tracepoints: append([]string{}, input.Tracepoints...),
		Kprobes:     append([]string{}, input.Kprobes...),
		LSM:         append([]string{}, input.LSM...),
		XDP:         append([]string{}, input.XDP...),
		TC:          append([]string{}, input.TC...),
	}
}

func mergeRequiredHooks(config *service.AllowlistConfig, required []recoveryHookRequirement) ([]string, error) {
	var added []string
	for _, hook := range required {
		attachType := strings.TrimSpace(hook.AttachType)
		attach := strings.TrimSpace(hook.Attach)
		if !validRecoveryHook(attachType, attach) {
			return nil, fmt.Errorf("required hook %s/%s is outside the recovery safety contract", attachType, attach)
		}
		var target *[]string
		switch attachType {
		case "tracepoint":
			target = &config.Tracepoints
		case "kprobe":
			target = &config.Kprobes
		case "lsm":
			target = &config.LSM
		case "xdp":
			target = &config.XDP
		case "tc":
			target = &config.TC
		}
		if !containsStringValue(*target, attach) {
			*target = append(*target, attach)
			added = append(added, attachType+":"+attach)
		}
	}
	sort.Strings(config.Tracepoints)
	sort.Strings(config.Kprobes)
	sort.Strings(config.LSM)
	sort.Strings(config.XDP)
	sort.Strings(config.TC)
	sort.Strings(added)
	return added, nil
}

func validRecoveryHook(attachType, attach string) bool {
	if attach == "" || strings.ContainsAny(attach, " \t\r\n;") {
		return false
	}
	switch attachType {
	case "tracepoint":
		return strings.Contains(attach, "/")
	case "kprobe", "lsm", "xdp", "tc":
		return !strings.Contains(attach, "/")
	default:
		return false
	}
}

func containsStringValue(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

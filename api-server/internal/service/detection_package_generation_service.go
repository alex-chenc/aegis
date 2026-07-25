package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/recovery"
	applogger "api-server/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var detectionPackageForbiddenBPFHelpers = []string{
	"bpf_probe_read_kernel",
	"bpf_override_return",
	"bpf_setsockopt",
	"bpf_sk_redirect",
	"bpf_get_current_task",
}

var detectionPackageForbiddenKernelAnnotations = []string{
	"__user",
	"__kernel",
	"__iomem",
	"__force",
}

type detectionPackageCoverageDecision struct {
	Status                 string                         `yaml:"status"`
	Reason                 string                         `yaml:"reason"`
	CoveredBehaviors       []string                       `yaml:"covered_behaviors"`
	UncoveredCoreBehaviors []string                       `yaml:"uncovered_core_behaviors"`
	RequiredHooks          []detectionPackageRequiredHook `yaml:"required_hooks"`
}

type detectionPackageRequiredHook struct {
	AttachType string `yaml:"attach_type" json:"attach_type"`
	Attach     string `yaml:"attach" json:"attach"`
}

type unsupportedDetectionPackageCoverageError struct {
	reason             string
	cveID              string
	uncoveredBehaviors []string
	requiredHooks      []detectionPackageRequiredHook
	activeAllowlist    *AllowlistConfig
}

func (e *unsupportedDetectionPackageCoverageError) Error() string {
	return "active hook allowlist cannot faithfully observe requested exploit chain: " + e.reason
}

func (e *unsupportedDetectionPackageCoverageError) RecoveryDescriptor() recovery.Descriptor {
	contextData := map[string]interface{}{
		"cve_id":                e.cveID,
		"uncovered_behaviors":   append([]string{}, e.uncoveredBehaviors...),
		"required_hooks":        append([]detectionPackageRequiredHook{}, e.requiredHooks...),
		"active_hook_allowlist": e.activeAllowlist,
	}
	actions := []recovery.Action{{
		ID:          "prepare_hook_allowlist_change",
		Label:       "仅查看 Hook 白名单变更建议",
		Description: "生成当前白名单与所需 Hook 的精确差异，不修改配置。",
		RiskLevel:   model.RiskReadonly,
		Executor:    "hook_allowlist",
		KeepsOpen:   true,
	}}
	if validDetectionPackageRequiredHooks(e.requiredHooks) {
		actions = append([]recovery.Action{{
			ID:                   "extend_hook_allowlist",
			Label:                "将所需 Hook 加入白名单并继续",
			Description:          "创建新的 Hook 白名单版本、同步 Agent，并在成功后重新执行原任务。",
			RiskLevel:            model.RiskHigh,
			Executor:             "hook_allowlist",
			ConfirmationRequired: true,
			ResumesRun:           true,
			RetrySafe:            true,
		}}, actions...)
	}
	actions = append(actions,
		recovery.Action{
			ID:          "pause",
			Label:       "暂停当前操作",
			Description: "保留恢复上下文，稍后再处理。",
			RiskLevel:   model.RiskReadonly,
		},
		recovery.Action{
			ID:          "cancel",
			Label:       "取消当前操作",
			Description: "取消本次目标，不修改 Hook 白名单。",
			RiskLevel:   model.RiskReadonly,
		},
		recovery.Action{
			ID:            "provide_other",
			Label:         "提供其他处理说明",
			Description:   "将补充说明作为恢复上下文交给智能体，并重新执行原任务。",
			RiskLevel:     model.RiskReadonly,
			Executor:      "assistant_resume",
			ResumesRun:    true,
			InputRequired: true,
		},
	)
	return recovery.Descriptor{
		Code:      "detection_package_hook_coverage_blocked",
		Category:  recovery.CategoryRecoverableBusinessBlocker,
		Summary:   "当前 Hook 白名单无法完整观测该漏洞利用链。",
		Detail:    e.reason,
		RiskLevel: model.RiskHigh,
		Context:   contextData,
		Actions:   actions,
	}
}

type DetectionPackageGenerationConfigRepository interface {
	GetActive() (*model.LLMConfig, error)
	DecryptAPIKey(encrypted string) (string, error)
}

type DetectionPackageDraftCreator interface {
	CreateDraft(ctx context.Context, req CreateDraftRequest, operator string) (*model.DetectionPackageDraft, error)
	GetActiveHookAllowlist(ctx context.Context) (*AllowlistConfig, error)
}

type detectionPackageLLMCaller interface {
	ChatCompletion(ctx context.Context, systemPrompt, userPrompt string, temperature float64) (string, error)
}

type detectionPackageLLMFactory func(apiKey, baseURL, modelName string, timeoutSeconds, maxRetries int) detectionPackageLLMCaller

// DetectionPackageGenerationService 检测包生成服务（对齐设计文档第 10.2 节）
// 从 handler 层下沉，供工具 handler 和页面 handler 共同调用
type DetectionPackageGenerationService struct {
	configRepo   DetectionPackageGenerationConfigRepository
	pkgService   DetectionPackageDraftCreator
	llmTimeout   int
	llmRetries   int
	newLLMClient detectionPackageLLMFactory
	logger       *zap.Logger
}

// NewDetectionPackageGenerationService 创建检测包生成服务
func NewDetectionPackageGenerationService(
	configRepo DetectionPackageGenerationConfigRepository,
	pkgService DetectionPackageDraftCreator,
	llmTimeout,
	llmRetries int,
) *DetectionPackageGenerationService {
	serviceLogger := applogger.Get()
	if serviceLogger == nil {
		serviceLogger = zap.NewNop()
	}
	return &DetectionPackageGenerationService{
		configRepo: configRepo,
		pkgService: pkgService,
		llmTimeout: llmTimeout,
		llmRetries: llmRetries,
		logger:     serviceLogger,
		newLLMClient: func(apiKey, baseURL, modelName string, timeoutSeconds, maxRetries int) detectionPackageLLMCaller {
			return llm.NewLLMClient(apiKey, baseURL, modelName, timeoutSeconds, maxRetries)
		},
	}
}

// GenerateDetectionPackageDraftRequest 生成检测包草稿请求
type GenerateDetectionPackageDraftRequest struct {
	CVEID                    string `json:"cve_id"`
	VulnerabilityDescription string `json:"vulnerability_description"`
	AttackPrerequisites      string `json:"attack_prerequisites"`
	ExploitationChain        string `json:"exploitation_chain"`
	FalsePositiveConstraints string `json:"false_positive_constraints"`
	Operator                 string `json:"operator"`
}

// GenerateDraft 生成检测包草稿（对齐 Package.Draft.Generate 工具）
func (s *DetectionPackageGenerationService) GenerateDraft(ctx context.Context, req GenerateDetectionPackageDraftRequest) (*model.DetectionPackageDraft, error) {
	req.CVEID = strings.ToUpper(strings.TrimSpace(req.CVEID))
	req.VulnerabilityDescription = strings.TrimSpace(req.VulnerabilityDescription)
	if req.CVEID == "" {
		return nil, fmt.Errorf("cve_id is required")
	}
	if req.VulnerabilityDescription == "" {
		return nil, fmt.Errorf("vulnerability_description is required")
	}
	if s == nil || s.configRepo == nil || s.pkgService == nil || s.newLLMClient == nil {
		return nil, fmt.Errorf("detection package generation dependencies unavailable")
	}
	operator := strings.TrimSpace(req.Operator)
	if operator == "" {
		operator = "assistant"
	}
	packageID := uuid.NewString()
	s.logger.Info("detection package draft generation started",
		zap.String("package_id", packageID),
		zap.String("cve_id", req.CVEID),
	)

	config, err := s.configRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("get active LLM config: %w", err)
	}
	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt LLM API key: %w", err)
	}
	client := s.newLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmRetries)
	if client == nil {
		return nil, fmt.Errorf("initialize detection package LLM client")
	}
	allowlist, err := s.pkgService.GetActiveHookAllowlist(ctx)
	if err != nil {
		return nil, fmt.Errorf("get active hook allowlist for generation: %w", err)
	}
	baseGenerationConstraints := joinDetectionPackageGenerationConstraints(
		req.FalsePositiveConstraints,
		detectionPackageHookAllowlistConstraints(allowlist),
		detectionPackageBPFHelperConstraints(),
	)
	timeout := 120 * time.Second
	if s.llmTimeout > 120 {
		timeout = time.Duration(s.llmTimeout) * time.Second
	}

	generationConstraints := baseGenerationConstraints
	var hookPlan, ebpfSource, sigmaRules, correlation string
	var accumulatedUnsupported *unsupportedDetectionPackageCoverageError
	for attempt := 1; attempt <= 2; attempt++ {
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, timeout)
		response, generationErr := client.ChatCompletion(attemptCtx, "", llm.GetDetectionPackageGenerationPrompt(
			packageID,
			req.CVEID,
			req.VulnerabilityDescription,
			req.AttackPrerequisites,
			req.ExploitationChain,
			generationConstraints,
		), 0.3)
		cancelAttempt()
		if generationErr != nil {
			s.logger.Warn("detection package draft LLM generation failed",
				zap.String("package_id", packageID),
				zap.String("cve_id", req.CVEID),
				zap.Int("generation_attempt", attempt),
				zap.Error(generationErr),
			)
			return nil, fmt.Errorf("generate detection package content on attempt %d: %w", attempt, generationErr)
		}

		validationErr := validateDetectionPackageCoverage(response)
		var unsupportedCoverage *unsupportedDetectionPackageCoverageError
		if errors.As(validationErr, &unsupportedCoverage) {
			unsupportedCoverage.cveID = req.CVEID
			unsupportedCoverage.activeAllowlist = allowlist
			accumulatedUnsupported = mergeUnsupportedDetectionPackageCoverage(
				accumulatedUnsupported,
				unsupportedCoverage,
			)
			if attempt < 2 {
				s.logger.Warn("detection package coverage incomplete; requesting bounded completeness confirmation",
					zap.String("package_id", packageID),
					zap.String("cve_id", req.CVEID),
					zap.Int("generation_attempt", attempt),
					zap.Int("required_hook_count", len(accumulatedUnsupported.requiredHooks)),
				)
				generationConstraints = joinDetectionPackageGenerationConstraints(
					baseGenerationConstraints,
					detectionPackageCoverageConfirmationConstraint(accumulatedUnsupported),
				)
				continue
			}
			s.logger.Warn("detection package generation stopped because requested behavior is not observable",
				zap.String("package_id", packageID),
				zap.String("cve_id", req.CVEID),
				zap.Int("generation_attempt", attempt),
				zap.Int("required_hook_count", len(accumulatedUnsupported.requiredHooks)),
				zap.Error(accumulatedUnsupported),
			)
			return nil, accumulatedUnsupported
		}
		if accumulatedUnsupported != nil {
			// The active allowlist did not change between completeness passes.
			// A later "supported" answer cannot invalidate missing hooks already
			// reported by the same bounded analysis, so fail closed with the
			// accumulated backend-visible evidence.
			s.logger.Warn("detection package coverage confirmation contradicted prior missing-hook evidence",
				zap.String("package_id", packageID),
				zap.String("cve_id", req.CVEID),
				zap.Int("generation_attempt", attempt),
				zap.Int("required_hook_count", len(accumulatedUnsupported.requiredHooks)),
			)
			return nil, accumulatedUnsupported
		}
		if validationErr == nil {
			hookPlan, ebpfSource, sigmaRules, correlation, validationErr =
				parseCompleteDetectionPackageLLMResponse(response)
		}
		if validationErr == nil {
			validationErr = validateGeneratedHookPlan(hookPlan, allowlist)
		}
		if validationErr == nil {
			validationErr = validateGeneratedEBPFSource(ebpfSource)
		}
		if validationErr == nil {
			break
		}

		if attempt == 2 {
			s.logger.Warn("detection package draft rejected after bounded generation correction",
				zap.String("package_id", packageID),
				zap.String("cve_id", req.CVEID),
				zap.Int("generation_attempt", attempt),
				zap.Error(validationErr),
			)
			return nil, fmt.Errorf("generated detection package rejected after one correction: %w", validationErr)
		}

		s.logger.Warn("detection package draft rejected by generation preflight; requesting bounded correction",
			zap.String("package_id", packageID),
			zap.String("cve_id", req.CVEID),
			zap.Int("generation_attempt", attempt),
			zap.Error(validationErr),
		)
		generationConstraints = joinDetectionPackageGenerationConstraints(
			baseGenerationConstraints,
			"Previous draft rejected by generation preflight: "+validationErr.Error()+
				". Regenerate the complete package from scratch. Use only the active Hook allowlist and comply with the BPF helper policy. Do not invent observability.",
		)
	}

	draft, err := s.pkgService.CreateDraft(ctx, CreateDraftRequest{
		PackageID:       packageID,
		TargetVersion:   "1.0.0",
		Title:           fmt.Sprintf("%s Runtime Detector", req.CVEID),
		Description:     req.VulnerabilityDescription,
		CVEIDs:          []string{req.CVEID},
		HookPlanYAML:    hookPlan,
		EBPFSource:      ebpfSource,
		SigmaRulesYAML:  sigmaRules,
		CorrelationYAML: correlation,
		AIGenerated:     true,
		AIGenerationInput: map[string]interface{}{
			"cve_id":                     req.CVEID,
			"vulnerability_description":  req.VulnerabilityDescription,
			"attack_prerequisites":       req.AttackPrerequisites,
			"exploitation_chain":         req.ExploitationChain,
			"false_positive_constraints": req.FalsePositiveConstraints,
			"model_name":                 config.ModelName,
		},
	}, operator)
	if err != nil {
		return nil, fmt.Errorf("create generated detection package draft: %w", err)
	}
	s.logger.Info("detection package draft generation completed",
		zap.String("package_id", draft.PackageID),
		zap.String("cve_id", req.CVEID),
		zap.String("status", draft.Status),
	)
	return draft, nil
}

func parseCompleteDetectionPackageLLMResponse(response string) (hookPlan, ebpfSource, sigmaRules, correlation string, err error) {
	hookPlan, ebpfSource, sigmaRules, correlation = parseDetectionPackageLLMResponse(response)
	if strings.TrimSpace(hookPlan) == "" || strings.TrimSpace(ebpfSource) == "" {
		err = fmt.Errorf("generated detection package is missing hook plan or eBPF source")
	}
	return
}

func validateGeneratedHookPlan(hookPlan string, allowlist *AllowlistConfig) error {
	hooks, err := parseHookPlan(hookPlan)
	if err != nil {
		return err
	}
	if len(hooks) == 0 {
		return fmt.Errorf("generated HookPlan contains no hooks")
	}
	if allowlist == nil {
		return nil
	}
	return ValidateHooksAgainstAllowlist(hooks, allowlist)
}

func validateDetectionPackageCoverage(response string) error {
	rawDecision := extractDetectionPackageCodeBlock(response, "Coverage Decision")
	if strings.TrimSpace(rawDecision) == "" {
		return fmt.Errorf("generated detection package is missing Coverage Decision")
	}
	var decision detectionPackageCoverageDecision
	if err := yaml.Unmarshal([]byte(rawDecision), &decision); err != nil {
		return fmt.Errorf("invalid Coverage Decision: %w", err)
	}
	status := strings.ToLower(strings.TrimSpace(decision.Status))
	if status == "unsupported" || len(decision.UncoveredCoreBehaviors) > 0 {
		reason := strings.TrimSpace(decision.Reason)
		if reason == "" {
			reason = strings.Join(decision.UncoveredCoreBehaviors, ", ")
		}
		if reason == "" {
			reason = "the generator reported unsupported coverage"
		}
		return &unsupportedDetectionPackageCoverageError{
			reason:             reason,
			uncoveredBehaviors: append([]string{}, decision.UncoveredCoreBehaviors...),
			requiredHooks:      normalizeDetectionPackageRequiredHooks(decision.RequiredHooks),
		}
	}
	if status != "supported" {
		return fmt.Errorf("Coverage Decision status must be supported or unsupported")
	}
	return nil
}

func normalizeDetectionPackageRequiredHooks(hooks []detectionPackageRequiredHook) []detectionPackageRequiredHook {
	result := make([]detectionPackageRequiredHook, 0, len(hooks))
	seen := make(map[string]bool)
	for _, hook := range hooks {
		hook.AttachType = strings.ToLower(strings.TrimSpace(hook.AttachType))
		hook.Attach = strings.TrimSpace(hook.Attach)
		key := hook.AttachType + "\x00" + hook.Attach
		if hook.AttachType == "" || hook.Attach == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, hook)
	}
	return result
}

func validDetectionPackageRequiredHooks(hooks []detectionPackageRequiredHook) bool {
	if len(hooks) == 0 {
		return false
	}
	for _, hook := range hooks {
		if strings.ContainsAny(hook.Attach, " \t\r\n;") {
			return false
		}
		switch hook.AttachType {
		case "tracepoint":
			if !strings.Contains(hook.Attach, "/") {
				return false
			}
		case "kprobe", "lsm", "xdp", "tc":
			if strings.Contains(hook.Attach, "/") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validateGeneratedEBPFSource(source string) error {
	for _, annotation := range detectionPackageForbiddenKernelAnnotations {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(annotation) + `\b`)
		if pattern.MatchString(source) {
			return fmt.Errorf(
				"unsupported kernel source annotation: %s; declare user pointers as const void * and read them with bpf_probe_read_user",
				annotation,
			)
		}
	}
	for _, helper := range detectionPackageForbiddenBPFHelpers {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(helper) + `\b`)
		if pattern.MatchString(source) {
			return fmt.Errorf("forbidden BPF helper call: %s", helper)
		}
	}
	return nil
}

func mergeUnsupportedDetectionPackageCoverage(
	current,
	next *unsupportedDetectionPackageCoverageError,
) *unsupportedDetectionPackageCoverageError {
	if current == nil {
		current = &unsupportedDetectionPackageCoverageError{}
	}
	if next == nil {
		return current
	}
	current.cveID = firstNonEmptyDetectionPackageString(current.cveID, next.cveID)
	current.activeAllowlist = next.activeAllowlist
	current.uncoveredBehaviors = dedupeDetectionPackageStrings(append(
		current.uncoveredBehaviors,
		next.uncoveredBehaviors...,
	))
	current.requiredHooks = normalizeDetectionPackageRequiredHooks(append(
		current.requiredHooks,
		next.requiredHooks...,
	))
	reasons := dedupeDetectionPackageStrings([]string{current.reason, next.reason})
	current.reason = strings.Join(reasons, " ")
	if strings.TrimSpace(current.reason) == "" {
		current.reason = "the active hook allowlist cannot observe every core behavior in the complete exploit chain"
	}
	return current
}

func detectionPackageCoverageConfirmationConstraint(unsupported *unsupportedDetectionPackageCoverageError) string {
	payload := map[string]interface{}{
		"previously_uncovered_behaviors": unsupported.uncoveredBehaviors,
		"previously_required_hooks":      unsupported.requiredHooks,
	}
	encoded, _ := json.Marshal(payload)
	return "Previous coverage analysis reported the following missing evidence: " + string(encoded) +
		". Perform one final completeness pass over the complete exploit chain from start to impact. " +
		"If coverage is still unsupported, return the complete union of every exact minimally required hook, " +
		"including all hooks already identified above. The active allowlist has not changed, so do not change " +
		"an unsupported decision to supported."
}

func dedupeDetectionPackageStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func firstNonEmptyDetectionPackageString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func detectionPackageHookAllowlistConstraints(allowlist *AllowlistConfig) string {
	if allowlist == nil {
		return ""
	}
	payload, err := json.Marshal(allowlist)
	if err != nil {
		return ""
	}
	return "ACTIVE HOOK ALLOWLIST (mandatory security contract): " + string(payload) +
		". Every HookPlan attach_type and attach value must exactly match this list. " +
		"Never substitute an unrelated allowed hook merely to pass validation. " +
		"If this allowlist cannot faithfully observe the requested exploit chain, omit HookPlan/eBPF output and explain the limitation instead of inventing support."
}

func detectionPackageBPFHelperConstraints() string {
	return "BUILDER BPF HELPER POLICY (mandatory security contract): never call " +
		strings.Join(detectionPackageForbiddenBPFHelpers, ", ") +
		". The builder rejects source containing any of these helper calls."
}

func joinDetectionPackageGenerationConstraints(values ...string) string {
	nonEmpty := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			nonEmpty = append(nonEmpty, value)
		}
	}
	return strings.Join(nonEmpty, "\n")
}

func parseDetectionPackageLLMResponse(response string) (hookPlan, ebpfSource, sigmaRules, correlation string) {
	hookPlan = extractDetectionPackageCodeBlock(response, "HookPlan")
	ebpfSource = extractDetectionPackageCodeBlock(response, "eBPF Source")
	sigmaRules = extractDetectionPackageCodeBlock(response, "Sigma")
	correlation = extractDetectionPackageCodeBlock(response, "Correlation")
	if hookPlan == "" {
		hookPlan = extractDetectionPackageCodeBlockByLanguage(response, "yaml", 1)
	}
	if ebpfSource == "" {
		ebpfSource = extractDetectionPackageCodeBlockByLanguage(response, "c", 1)
	}
	if sigmaRules == "" {
		sigmaRules = extractDetectionPackageCodeBlockByLanguage(response, "yaml", 2)
	}
	if correlation == "" {
		correlation = extractDetectionPackageCodeBlockByLanguage(response, "yaml", 3)
	}
	return
}

func extractDetectionPackageCodeBlock(response, sectionHint string) string {
	lines := strings.Split(response, "\n")
	inSection := false
	inBlock := false
	var blockLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.Contains(lower, strings.ToLower(sectionHint)) && strings.HasPrefix(trimmed, "#") {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "```") {
			if inBlock {
				break
			}
			inBlock = true
			continue
		}
		if inSection && inBlock {
			blockLines = append(blockLines, line)
		}
		if inSection && !inBlock && strings.HasPrefix(trimmed, "## ") && !strings.Contains(lower, strings.ToLower(sectionHint)) {
			inSection = false
		}
	}
	return strings.TrimSpace(strings.Join(blockLines, "\n"))
}

func extractDetectionPackageCodeBlockByLanguage(response, language string, occurrence int) string {
	marker := "```" + language
	lines := strings.Split(response, "\n")
	inBlock := false
	count := 0
	var blockLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, marker) {
			count++
			if count == occurrence {
				inBlock = true
				continue
			}
			if inBlock {
				break
			}
			continue
		}
		if inBlock && strings.HasPrefix(trimmed, "```") {
			break
		}
		if inBlock {
			blockLines = append(blockLines, line)
		}
	}
	return strings.TrimSpace(strings.Join(blockLines, "\n"))
}

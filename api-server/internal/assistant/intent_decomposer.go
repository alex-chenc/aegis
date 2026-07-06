package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api-server/internal/llm"
	"go.uber.org/zap"
)

const intentDecomposeTimeout = 45 * time.Second

type LLMClientFactory func(ctx context.Context) (*llm.LLMClient, error)

type IntentDecomposeInput struct {
	Query                  string            `json:"query"`
	Intent                 IntentResult      `json:"intent"`
	ContextRefs            []ContextRefInput `json:"context_refs,omitempty"`
	CandidateCapabilities  []string          `json:"candidate_capabilities,omitempty"`
	EnableLLMDecomposition bool              `json:"enable_llm_decomposition,omitempty"`
}

type IntentDecomposerDeps struct {
	LLMClientFactory LLMClientFactory
	Logger           *zap.Logger
}

type IntentDecomposer struct {
	clientFactory LLMClientFactory
	logger        *zap.Logger
}

func NewIntentDecomposer(deps IntentDecomposerDeps) *IntentDecomposer {
	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &IntentDecomposer{
		clientFactory: deps.LLMClientFactory,
		logger:        logger,
	}
}

func (d *IntentDecomposer) Decompose(ctx context.Context, input IntentDecomposeInput) (*IntentBreakdown, error) {
	base := d.decomposeByRules(input)
	if !input.EnableLLMDecomposition || d.clientFactory == nil || !shouldUseLLMIntentBreakdown(input.Query, input.Intent) {
		return base, nil
	}

	client, err := d.clientFactory(ctx)
	if err != nil || client == nil {
		d.logger.Warn("assistant intent decomposition llm unavailable, using rule breakdown",
			zap.Error(err),
			zap.String("action", input.Intent.Action),
			zap.Strings("domains", input.Intent.Domains),
		)
		return base, nil
	}

	decomposeCtx, cancel := context.WithTimeout(ctx, intentDecomposeTimeout)
	defer cancel()
	enhanced, err := requestLLMIntentBreakdown(decomposeCtx, client, input)
	if err != nil {
		d.logger.Warn("assistant intent decomposition llm failed, using rule breakdown",
			zap.Error(err),
			zap.String("action", input.Intent.Action),
			zap.Strings("domains", input.Intent.Domains),
		)
		return base, nil
	}
	normalizeIntentBreakdown(enhanced, base)
	return enhanced, nil
}

func (d *IntentDecomposer) decomposeByRules(input IntentDecomposeInput) *IntentBreakdown {
	query := strings.TrimSpace(input.Query)
	intent := input.Intent
	action := intent.Action
	if action == "" {
		action = "query"
	}

	objects := make([]IntentObject, 0, 1+len(input.ContextRefs))
	if intent.Object != "" {
		objects = append(objects, IntentObject{Type: intent.Object, Source: "intent_router"})
	}
	for _, ref := range input.ContextRefs {
		if ref.ObjectType == "" {
			continue
		}
		objects = append(objects, IntentObject{
			Type:   ref.ObjectType,
			ID:     ref.ObjectID,
			Source: "context_ref",
		})
	}

	scope := inferIntentScope(query, input.ContextRefs)
	requiresWrite := intent.NeedWrite || hasExplicitWriteIntent(query)
	needClarification, missingInfo, question := inferMissingInfo(query, action, objects, scope, requiresWrite)

	capabilities := append([]string{}, input.CandidateCapabilities...)
	capabilities = append(capabilities, inferCandidateCapabilities(intent, query)...)
	capabilities = dedupeStrings(capabilities)

	return &IntentBreakdown{
		Goal:                  query,
		Domains:               append([]string{}, intent.Domains...),
		Actions:               []string{action},
		Objects:               objects,
		Scope:                 scope,
		MissingInfo:           missingInfo,
		RequiresWrite:         requiresWrite,
		RiskHint:              string(intent.RiskHint),
		CandidateCapabilities: capabilities,
		NeedClarification:     needClarification,
		ClarifyingQuestion:    question,
		Reason:                intent.Reason,
		Confidence:            intent.Confidence,
	}
}

func requestLLMIntentBreakdown(ctx context.Context, client *llm.LLMClient, input IntentDecomposeInput) (*IntentBreakdown, error) {
	systemPrompt := `你是 Aegis 智能助手的问题拆解器。你只负责把用户问题拆成业务意图，不负责授权工具执行。

要求：
1. 只能输出业务能力 candidate_capabilities，不要输出最终 tool_name。
2. 写操作必须从用户原文中找到明确执行、采集、扫描、修复、阻断、生成、下发等意图。
3. 对象、动作、范围不清楚时 need_clarification=true，并给出一个简短追问。
4. 页面引用如“这个告警”“当前主机”要保留为对象引用，不要编造真实 ID。
5. 只输出 JSON，不要 Markdown。

返回格式：
{"goal":"","domains":[],"actions":[],"objects":[],"scope":{"kind":""},"constraints":[],"missing_info":[],"requires_write":false,"risk_hint":"readonly","candidate_capabilities":[],"need_clarification":false,"clarifying_question":"","reason":"","confidence":0.8}`

	userPrompt := fmt.Sprintf("用户消息：%s\n\n规则意图：%s\n\n上下文引用：%s\n\n已有候选能力：%s",
		input.Query,
		encodeJSON(input.Intent),
		encodeJSON(input.ContextRefs),
		encodeJSON(input.CandidateCapabilities),
	)
	resp, err := client.ChatCompletionWithMessages(ctx, []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.1)
	if err != nil {
		return nil, err
	}
	var breakdown IntentBreakdown
	if err := unmarshalFirstJSONObject(resp, &breakdown); err != nil {
		return nil, err
	}
	return &breakdown, nil
}

func normalizeIntentBreakdown(value, fallback *IntentBreakdown) {
	if value == nil || fallback == nil {
		return
	}
	if strings.TrimSpace(value.Goal) == "" {
		value.Goal = fallback.Goal
	}
	if len(value.Domains) == 0 {
		value.Domains = fallback.Domains
	}
	if len(value.Actions) == 0 {
		value.Actions = fallback.Actions
	}
	if value.RiskHint == "" {
		value.RiskHint = fallback.RiskHint
	}
	if value.Confidence <= 0 {
		value.Confidence = fallback.Confidence
	}
	value.CandidateCapabilities = dedupeStrings(append(value.CandidateCapabilities, fallback.CandidateCapabilities...))
	if value.NeedClarification && value.ClarifyingQuestion == "" {
		value.ClarifyingQuestion = fallback.ClarifyingQuestion
	}
}

func shouldUseLLMIntentBreakdown(query string, intent IntentResult) bool {
	if strings.TrimSpace(query) == "" || intent.Action == "answer" {
		return false
	}
	if intent.Confidence < 0.65 {
		return true
	}
	return estimateQueryComplexity(query) >= 3
}

func inferIntentScope(query string, refs []ContextRefInput) IntentScope {
	normalized := strings.ToLower(strings.TrimSpace(query))
	switch {
	case strings.Contains(normalized, "在线"):
		return IntentScope{Kind: "online_hosts", Source: "user_message"}
	case containsAnyFold(normalized, "全部", "所有", "全量", "all"):
		return IntentScope{Kind: "all", Source: "user_message"}
	case len(refs) > 0:
		ids := make([]string, 0, len(refs))
		for _, ref := range refs {
			if ref.ObjectID != "" {
				ids = append(ids, ref.ObjectID)
			}
		}
		return IntentScope{Kind: "context_refs", ObjectIDs: ids, Source: "context_ref"}
	default:
		return IntentScope{Kind: "unspecified"}
	}
}

func inferMissingInfo(query, action string, objects []IntentObject, scope IntentScope, requiresWrite bool) (bool, []MissingInfo, string) {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if isVagueRepairRequest(normalized) {
		question := "请确认要修复的对象，是基线规则、漏洞 CVE、弱密码任务还是检测包？"
		return true, []MissingInfo{{
			Field:    "object",
			Reason:   "修复请求缺少明确对象和范围",
			Question: question,
		}}, question
	}
	if action == "block" && !hasContextObject(objects, "alert") && !looksLikeIDInText(normalized) {
		question := "请确认要阻断的告警 ID，或先在告警详情页引用这个告警。"
		return true, []MissingInfo{{
			Field:    "alert_id",
			Reason:   "阻断操作缺少明确告警对象",
			Question: question,
		}}, question
	}
	if requiresWrite && len(objects) == 0 && scope.Kind == "unspecified" {
		question := "请补充要操作的对象和范围后再执行。"
		return true, []MissingInfo{{
			Field:    "object_scope",
			Reason:   "写操作缺少对象或范围",
			Question: question,
		}}, question
	}
	return false, nil, ""
}

func inferCandidateCapabilities(intent IntentResult, query string) []string {
	normalized := strings.ToLower(query)
	capabilities := make([]string, 0, 8)
	for _, domain := range intent.Domains {
		switch domain {
		case "host":
			capabilities = append(capabilities, "list_hosts", "get_agent_status")
		case "asset":
			if hasAssetCollectionIntent(normalized) || intent.Action == "execute" {
				capabilities = append(capabilities, "trigger_asset_collection", "get_asset_collection_task")
			}
			capabilities = append(capabilities, "asset_summary", "list_application_assets", "list_software_assets")
		case "vulnerability":
			capabilities = append(capabilities, "list_vulnerabilities", "get_vulnerability_affected_hosts", "search_installed_software")
		case "detection":
			capabilities = append(capabilities, "list_detection_alerts", "get_detection_alert", "get_detection_statistics")
			if intent.Action == "block" || strings.Contains(normalized, "阻断") {
				capabilities = append(capabilities, "block_detection_alert")
			}
		case "baseline":
			capabilities = append(capabilities, "list_baseline_templates", "get_baseline_template_status", "list_baseline_template_rules")
			if intent.NeedWrite {
				capabilities = append(capabilities, "generate_baseline_scripts")
			}
		}
	}
	return capabilities
}

func hasExplicitWriteIntent(query string) bool {
	normalized := strings.ToLower(query)
	return containsAnyFold(normalized,
		"执行", "运行", "触发", "启动", "采集", "扫描", "修复", "整改", "阻断", "封禁",
		"生成", "下发", "部署", "创建", "新建", "更新", "删除", "批准", "审批",
		"execute", "run", "trigger", "collect", "scan", "fix", "block", "generate", "deploy",
	)
}

func isConceptQuestion(query string) bool {
	return containsAnyFold(query, "是什么", "什么意思", "介绍", "解释", "说明一下", "如何理解", "why", "what is")
}

func isVagueRepairRequest(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}
	return containsAnyFold(trimmed, "帮我修复一下", "修复一下", "处理一下", "整改一下") &&
		!containsAnyFold(trimmed, "基线", "漏洞", "cve", "弱密码", "检测包", "规则", "告警", "主机")
}

func hasContextObject(objects []IntentObject, objectType string) bool {
	for _, object := range objects {
		if object.Type == objectType && object.ID != "" {
			return true
		}
	}
	return false
}

func looksLikeIDInText(text string) bool {
	return strings.Contains(text, "id") || strings.Contains(text, "uuid") || strings.Contains(text, "-")
}

func containsAnyFold(text string, keywords ...string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
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

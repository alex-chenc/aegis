package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"api-server/internal/llm"
	"go.uber.org/zap"
)

const (
	// 推理模型会把 max_tokens 预算分给“思考”与最终文本，预算过小会导致思考耗尽预算、
	// 正文（JSON）被截断为空，客户端回退返回纯思考正文而没有 JSON。放宽到 8192 留足空间。
	llmToolSelectionMaxTokens = 8192
	llmToolSelectionMax       = 24
	// 结构化 JSON 调用在解析失败时的最大重试次数（推理模型偶发只返回没有 JSON 的思考正文）。
	llmJSONParseMaxAttempts = 3
)

// jsonOnlyRetryReminder 在重试时追加，强制模型只输出可解析的 JSON。
const jsonOnlyRetryReminder = "The previous response was not valid parseable JSON. Return exactly one JSON object this time. " +
	"Do not output reasoning, explanations, or Markdown fences. Start with { and end with }."

const toolSelectionDraftSystemPrompt = `You are the tool selector for the Aegis agent. Select potentially useful tools from the short catalog based on the user intent. Do not execute the task.

Requirements:
1. Understand the user's goal, objects, constraints, and missing information before selecting tools.
2. Select semantically. Do not rely on keyword matching alone. A task may span multiple open domains.
3. If the short catalog is insufficient, put tool names or English capability identifiers in detail_requests so the system can provide details for the final selection.
4. selected_tools may contain only exact tool names present in the short catalog. Never invent a tool.
5. Select write or high-risk tools only when the user explicitly asks to execute, collect, scan, repair, generate, dispatch, or otherwise change state.
6. If required information is unavailable and cannot be safely defaulted or discovered, set need_clarification=true and provide one concise question. Read-only context tools may still be selected.
7. For multi-stage, asynchronous, conditional, or verification goals, select relevant trigger, status, result, validation, or alternative tools based on their contracts. Do not determine a fixed execution order; Runtime will plan from actual results.
8. Natural-language fields may follow the user's language. Tool names and capability identifiers must remain exact English catalog values.

Return JSON only:
{"intent_summary":"","need_clarification":false,"clarifying_question":"","selected_tools":[],"detail_requests":[],"reason":""}`

const toolSelectionFinalSystemPrompt = `You are the final tool selector for the Aegis agent. Use the detailed tool contracts to choose the bounded tool set that the planner and executor may need.

Requirements:
1. Keep only tools that may be needed for the user's goal. Do not fill the set with unrelated tools.
2. Include relevant read, detail, and status tools as well as write tools explicitly requested by the user.
3. For an asynchronous task, include both a relevant trigger tool and relevant status or result tools when their contracts support the goal.
4. If required arguments cannot be supplied by user input, context, or a preceding tool result, select a read-only discovery tool or set need_clarification=true.
5. selected_tools may contain only exact names present in the detailed or short catalog.
6. Select conditional and asynchronous tools only when their contracts are relevant. Runtime decides the actual order and calls from real results.
7. Natural-language fields may follow the user's language. Tool names and capability identifiers must remain exact English catalog values.

Return JSON only:
{"intent_summary":"","need_clarification":false,"clarifying_question":"","selected_tools":[],"reason":""}`

// jsonObjectResponseFormat 返回 OpenAI 兼容的 JSON 模式（response_format={"type":"json_object"}），
// 让小米 MiMo 等模型强制输出合法 JSON，从根本上减少“只返回思考正文、没有 JSON”的情况；
// 若端点走 Anthropic 协议（会忽略 response_format），则返回 nil 交由提示词与重试兜底。
func jsonObjectResponseFormat(client *llm.LLMClient) *llm.ResponseFormat {
	if client != nil && client.SupportsJSONObjectResponseFormat() {
		return &llm.ResponseFormat{Type: "json_object"}
	}
	return nil
}

// requestLLMJSONWithRetry 调用 LLM 并解析首个 JSON 对象；推理模型偶尔会把 token 预算耗在思考上、
// 只返回没有 JSON 的推理正文，导致 "no json object found"。这里在解析失败时有限次重试，
// 并逐步加强“只输出 JSON”的提示与温度，提升结构化输出的鲁棒性。网络/超时类错误由
// llm.LLMClient 内部统一重试，这里遇到即直接返回。
func requestLLMJSONWithRetry(ctx context.Context, target interface{}, baseMessages []llm.Message, call func(ctx context.Context, messages []llm.Message, temperature float64) (string, error)) error {
	var lastErr error
	for attempt := 0; attempt < llmJSONParseMaxAttempts; attempt++ {
		messages := baseMessages
		temperature := 0.1
		if attempt > 0 {
			messages = append(append([]llm.Message{}, baseMessages...), llm.Message{
				Role:    "user",
				Content: jsonOnlyRetryReminder,
			})
			temperature = 0.35
		}
		resp, err := call(ctx, messages, temperature)
		if err != nil {
			return err
		}
		if perr := unmarshalFirstJSONObject(resp, target); perr != nil {
			lastErr = perr
			continue
		}
		return nil
	}
	return lastErr
}

type llmToolSelectionDraft struct {
	IntentSummary      string   `json:"intent_summary"`
	NeedClarification  bool     `json:"need_clarification"`
	ClarifyingQuestion string   `json:"clarifying_question"`
	SelectedTools      []string `json:"selected_tools"`
	DetailRequests     []string `json:"detail_requests"`
	Reason             string   `json:"reason"`
}

type llmToolSelectionFinal struct {
	IntentSummary      string   `json:"intent_summary"`
	NeedClarification  bool     `json:"need_clarification"`
	ClarifyingQuestion string   `json:"clarifying_question"`
	SelectedTools      []string `json:"selected_tools"`
	Reason             string   `json:"reason"`
}

func (o *Orchestrator) selectToolsWithLLM(ctx context.Context, userMessage string, intent IntentResult, contextRefs []ContextRefInput) (*ToolSelectionResult, error) {
	if o.runtimeFactory == nil || o.toolRegistry == nil {
		return nil, fmt.Errorf("llm tool selection dependencies unavailable")
	}
	client, err := o.runtimeFactory.BuildLLMClient(ctx)
	if err != nil {
		return nil, err
	}

	briefCatalog := o.buildLLMToolBriefCatalog()
	if len(briefCatalog) == 0 {
		return nil, fmt.Errorf("tool catalog is empty")
	}

	draftStartedAt := time.Now()
	// 每次 LLM 调用的超时与重试由 llm.LLMClient 统一控制（平台标准 1200s / 5 次重试），
	// 此处不再叠加更短的阶段级超时，避免在慢速推理模型下过早触发 context deadline exceeded。
	draft, err := requestLLMToolSelectionDraft(ctx, client, userMessage, intent, contextRefs, briefCatalog)
	if err != nil {
		return nil, fmt.Errorf("draft tool selection: %w", err)
	}
	if o.logger != nil {
		o.logger.Info("assistant llm draft tool selection completed",
			zap.Duration("duration", time.Since(draftStartedAt)),
			zap.Int("catalog_bytes", len(briefCatalog)),
			zap.Int("selected_count", len(draft.SelectedTools)),
		)
	}

	details := o.buildLLMToolDetailCatalog(draft.SelectedTools, draft.DetailRequests)
	finalStartedAt := time.Now()
	final, err := requestLLMToolSelectionFinal(ctx, client, userMessage, intent, contextRefs, briefCatalog, details, draft)
	if err != nil {
		return nil, fmt.Errorf("final tool selection: %w", err)
	}
	if o.logger != nil {
		o.logger.Info("assistant llm final tool selection completed",
			zap.Duration("duration", time.Since(finalStartedAt)),
			zap.Int("details_bytes", len(details)),
			zap.Int("selected_count", len(final.SelectedTools)),
		)
	}

	selected := o.normalizeLLMSelectedTools(final.SelectedTools)
	selected = filterResidentToolsForIntent(selected, userMessage, intent.ExplicitToolName)
	if len(selected) == 0 && !final.NeedClarification {
		selected = o.normalizeLLMSelectedTools(draft.SelectedTools)
		selected = filterResidentToolsForIntent(selected, userMessage, intent.ExplicitToolName)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("llm selected no executable tools")
	}

	return &ToolSelectionResult{
		SelectedTools:  selected,
		CandidateTools: selected,
		Query:          userMessage,
		Intent:         intent,
		MaxTools:       llmToolSelectionMax,
	}, nil
}

func filterResidentToolsForIntent(names []string, query, explicitToolName string) []string {
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if isResidentTool(name) && !residentToolExplicitlyRequested(name, query, explicitToolName) {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

func requestLLMToolSelectionDraft(ctx context.Context, client *llm.LLMClient, userMessage string, intent IntentResult, contextRefs []ContextRefInput, briefCatalog string) (llmToolSelectionDraft, error) {
	userPrompt := fmt.Sprintf("User message:\n%s\n\nUpstream LLM intent:\n%s\n\nContext references:\n%s\n\nShort tool catalog:\n%s", userMessage, encodeJSON(intent), encodeJSON(contextRefs), briefCatalog)
	baseMessages := []llm.Message{
		{Role: "system", Content: toolSelectionDraftSystemPrompt},
		{Role: "user", Content: userPrompt},
	}
	var draft llmToolSelectionDraft
	respFormat := jsonObjectResponseFormat(client)
	if err := requestLLMJSONWithRetry(ctx, &draft, baseMessages, func(ctx context.Context, messages []llm.Message, temperature float64) (string, error) {
		return client.ChatCompletionWithMessagesMaxTokensFormat(ctx, messages, temperature, llmToolSelectionMaxTokens, respFormat)
	}); err != nil {
		return llmToolSelectionDraft{}, err
	}
	return draft, nil
}

func requestLLMToolSelectionFinal(ctx context.Context, client *llm.LLMClient, userMessage string, intent IntentResult, contextRefs []ContextRefInput, briefCatalog, details string, draft llmToolSelectionDraft) (llmToolSelectionFinal, error) {
	userPrompt := fmt.Sprintf("User message:\n%s\n\nUpstream LLM intent:\n%s\n\nContext references:\n%s\n\nDraft selection:\n%s\n\nDetailed tool contracts:\n%s\n\nShort catalog fallback:\n%s", userMessage, encodeJSON(intent), encodeJSON(contextRefs), encodeJSON(draft), details, briefCatalog)
	baseMessages := []llm.Message{
		{Role: "system", Content: toolSelectionFinalSystemPrompt},
		{Role: "user", Content: userPrompt},
	}
	var final llmToolSelectionFinal
	respFormat := jsonObjectResponseFormat(client)
	if err := requestLLMJSONWithRetry(ctx, &final, baseMessages, func(ctx context.Context, messages []llm.Message, temperature float64) (string, error) {
		return client.ChatCompletionWithMessagesMaxTokensFormat(ctx, messages, temperature, llmToolSelectionMaxTokens, respFormat)
	}); err != nil {
		return llmToolSelectionFinal{}, err
	}
	return final, nil
}

func (o *Orchestrator) buildLLMToolBriefCatalog() string {
	tools := o.toolRegistry.List()
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].Domain != tools[j].Domain {
			return tools[i].Domain < tools[j].Domain
		}
		return tools[i].Name < tools[j].Name
	})

	var buf strings.Builder
	for _, tool := range tools {
		if tool == nil || !tool.Enabled {
			continue
		}
		buf.WriteString(fmt.Sprintf("- %s | domain=%s | op=%s | risk=%s | %s\n",
			tool.Name,
			tool.Domain,
			tool.Operation,
			tool.Risk,
			shortToolBrief(tool),
		))
	}
	return strings.TrimSpace(buf.String())
}

func (o *Orchestrator) buildLLMToolDetailCatalog(selectedTools, detailRequests []string) string {
	names := map[string]bool{}
	for _, name := range selectedTools {
		if _, ok := o.toolRegistry.Get(name); ok {
			names[name] = true
		}
	}
	for _, query := range detailRequests {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		if _, ok := o.toolRegistry.Get(query); ok {
			names[query] = true
			continue
		}
		for _, tool := range o.toolRegistry.List() {
			if tool == nil || !tool.Enabled {
				continue
			}
			if toolMatchesDetailQuery(tool, query) {
				names[tool.Name] = true
			}
		}
	}

	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	if len(ordered) > 36 {
		ordered = ordered[:36]
	}

	var buf strings.Builder
	for _, name := range ordered {
		tool, ok := o.toolRegistry.Get(name)
		if !ok || tool == nil || !tool.Enabled {
			continue
		}
		buf.WriteString(formatToolDetailForLLM(tool))
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func (o *Orchestrator) normalizeLLMSelectedTools(names []string) []string {
	result := make([]string, 0, len(names))
	seen := map[string]bool{}
	writeCount := 0
	for _, name := range names {
		name = strings.TrimSpace(name)
		tool, ok := o.toolRegistry.Get(name)
		if !ok || tool == nil || !tool.Enabled || seen[name] {
			continue
		}
		if tool.Risk == ToolRiskCritical {
			continue
		}
		if tool.Risk != ToolRiskReadonly && tool.Risk != ToolRiskLow {
			if writeCount >= 6 {
				continue
			}
			writeCount++
		}
		seen[name] = true
		result = append(result, name)
		if len(result) >= llmToolSelectionMax {
			break
		}
	}
	return result
}

func shouldUseLLMToolSelection(message string, intent IntentResult) bool {
	if strings.TrimSpace(message) == "" || intent.Action == "answer" {
		return false
	}
	return true
}

func shortToolBrief(tool *ToolSpec) string {
	if tool == nil {
		return ""
	}
	contract := BuildToolUseContract(tool)
	parts := []string{"capability=" + contract.Capability}
	if len(contract.ObjectTypes) > 0 {
		parts = append(parts, "objects="+strings.Join(contract.ObjectTypes, ","))
	}
	return strings.Join(parts, " ")
}

func formatToolDetailForLLM(tool *ToolSpec) string {
	contract := BuildToolUseContract(tool)
	return fmt.Sprintf("- %s\n  capability=%s domain=%s operation=%s risk=%s approval=%v\n  objects=%s\n  allowed_actions=%s\n  preconditions=%s\n  postconditions=%s\n  args=%s",
		tool.Name,
		contract.Capability,
		tool.Domain,
		tool.Operation,
		tool.Risk,
		tool.RequiresApproval || !tool.DefaultWhitelisted,
		strings.Join(contract.ObjectTypes, ","),
		strings.Join(contract.Actions, ","),
		strings.Join(contract.Preconditions, ","),
		strings.Join(contract.Postconditions, ","),
		summarizeToolArgsForLLM(tool.ArgsSchema),
	)
}

func summarizeToolArgsForLLM(schema map[string]interface{}) string {
	if schema == nil {
		return "{}"
	}
	props, _ := schema["properties"].(map[string]interface{})
	if len(props) == 0 {
		return "{}"
	}
	required := stringSetFromSchemaRequired(schema["required"])
	keys := make([]string, 0, len(props))
	for key := range props {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		prop, _ := props[key].(map[string]interface{})
		flag := "optional"
		if required[key] {
			flag = "required"
		}
		dataType, _ := prop["type"].(string)
		if dataType == "" {
			dataType = "any"
		}
		item := fmt.Sprintf("%s(%s,type=%s", key, flag, dataType)
		if values, ok := prop["enum"]; ok {
			if encoded, err := json.Marshal(values); err == nil {
				item += ",enum=" + string(encoded)
			}
		}
		parts = append(parts, item+")")
	}
	return strings.Join(parts, "; ")
}

func stringSetFromSchemaRequired(raw interface{}) map[string]bool {
	set := map[string]bool{}
	switch typed := raw.(type) {
	case []interface{}:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				set[s] = true
			}
		}
	case []string:
		for _, item := range typed {
			set[item] = true
		}
	}
	return set
}

func toolMatchesDetailQuery(tool *ToolSpec, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}
	candidates := []string{
		tool.Name,
		string(tool.Domain),
		string(tool.Operation),
		tool.Capability,
		tool.Description,
		strings.Join(tool.Aliases, " "),
		strings.Join(tool.Tags, " "),
	}
	for _, candidate := range candidates {
		candidate = strings.ToLower(candidate)
		if strings.Contains(candidate, query) || strings.Contains(query, candidate) {
			return true
		}
	}
	return false
}

func unmarshalFirstJSONObject(raw string, target interface{}) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return fmt.Errorf("no json object found in llm response")
	}
	return json.Unmarshal([]byte(raw[start:end+1]), target)
}

func encodeJSON(value interface{}) string {
	b, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func limitRunes(s string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit])
}

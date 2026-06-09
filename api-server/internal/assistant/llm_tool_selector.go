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
)

const (
	llmToolSelectionTimeout = 75 * time.Second
	llmToolSelectionMax     = 24
)

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

	selectCtx, cancel := context.WithTimeout(ctx, llmToolSelectionTimeout)
	defer cancel()

	briefCatalog := o.buildLLMToolBriefCatalog()
	if len(briefCatalog) == 0 {
		return nil, fmt.Errorf("tool catalog is empty")
	}

	draft, err := requestLLMToolSelectionDraft(selectCtx, client, userMessage, intent, contextRefs, briefCatalog)
	if err != nil {
		return nil, err
	}

	details := o.buildLLMToolDetailCatalog(draft.SelectedTools, draft.DetailRequests)
	final, err := requestLLMToolSelectionFinal(selectCtx, client, userMessage, intent, contextRefs, briefCatalog, details, draft)
	if err != nil {
		return nil, err
	}

	selected := o.normalizeLLMSelectedTools(final.SelectedTools)
	if len(selected) == 0 && !final.NeedClarification {
		selected = o.normalizeLLMSelectedTools(draft.SelectedTools)
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

func requestLLMToolSelectionDraft(ctx context.Context, client *llm.LLMClient, userMessage string, intent IntentResult, contextRefs []ContextRefInput, briefCatalog string) (llmToolSelectionDraft, error) {
	systemPrompt := `你是 Aegis 智能体的工具选择器。你只负责根据用户意图从工具短目录中挑选可能需要的工具，不执行任务。

要求：
1. 先理解用户目标、对象、约束和缺失信息，再选择工具。
2. 不要只按关键词机械匹配；同一任务可跨资产、主机、漏洞、基线、告警、任务等域。
3. 如果短目录信息不足，可以在 detail_requests 中写工具名或能力关键词，系统会提供详情后再让你最终选择。
4. selected_tools 只能填写短目录中存在的工具名，不得发明工具。
5. 高风险/写操作只有用户明确要求执行、采集、扫描、修复、生成、下发时才选择。
6. 如果用户信息不足且无法安全默认，need_clarification=true 并给出一个简短追问；仍可选择用于澄清前查询上下文的只读工具。

只输出 JSON：{"intent_summary":"","need_clarification":false,"clarifying_question":"","selected_tools":[],"detail_requests":[],"reason":""}`

	userPrompt := fmt.Sprintf("用户消息：%s\n\n规则意图：%s\n\n上下文引用：%s\n\n工具短目录：\n%s", userMessage, encodeJSON(intent), encodeJSON(contextRefs), briefCatalog)
	resp, err := client.ChatCompletionWithMessages(ctx, []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.1)
	if err != nil {
		return llmToolSelectionDraft{}, err
	}
	var draft llmToolSelectionDraft
	if err := unmarshalFirstJSONObject(resp, &draft); err != nil {
		return llmToolSelectionDraft{}, err
	}
	return draft, nil
}

func requestLLMToolSelectionFinal(ctx context.Context, client *llm.LLMClient, userMessage string, intent IntentResult, contextRefs []ContextRefInput, briefCatalog, details string, draft llmToolSelectionDraft) (llmToolSelectionFinal, error) {
	systemPrompt := `你是 Aegis 智能体的最终工具选择器。现在你拿到了工具详情，请选出计划阶段和执行阶段大概需要注入给模型的工具集合。

要求：
1. selected_tools 只保留完成用户目标可能需要的工具，不要塞满无关工具。
2. 选择查询/详情/状态工具，也要选择用户明确要求的执行类工具。
3. 如果任务需要先执行任务再查看结果，同时选择触发工具和状态/结果查询工具。
4. 如果工具详情显示参数无法满足，改为选择可查询上下文的工具或设置 need_clarification=true。
5. selected_tools 只能填写工具详情或短目录中存在的工具名。

只输出 JSON：{"intent_summary":"","need_clarification":false,"clarifying_question":"","selected_tools":[],"reason":""}`

	userPrompt := fmt.Sprintf("用户消息：%s\n\n规则意图：%s\n\n上下文引用：%s\n\n第一轮选择：%s\n\n工具详情：\n%s\n\n工具短目录备用：\n%s", userMessage, encodeJSON(intent), encodeJSON(contextRefs), encodeJSON(draft), details, briefCatalog)
	resp, err := client.ChatCompletionWithMessages(ctx, []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.1)
	if err != nil {
		return llmToolSelectionFinal{}, err
	}
	var final llmToolSelectionFinal
	if err := unmarshalFirstJSONObject(resp, &final); err != nil {
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
	result := make([]string, 0, len(names)+3)
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
	for _, name := range []string{"Tool.Search", "Context.Get", "Session.Summarize"} {
		if len(result) >= llmToolSelectionMax {
			break
		}
		if seen[name] {
			continue
		}
		if tool, ok := o.toolRegistry.Get(name); ok && tool != nil && tool.Enabled {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}

func shouldUseLLMToolSelection(message string, intent IntentResult) bool {
	if strings.TrimSpace(message) == "" || intent.Action == "answer" || shouldBypassLLMToolSelection(message) {
		return false
	}
	return true
}

func shouldBypassLLMToolSelection(message string) bool {
	return detectNaturalOperationShortcut(message).Kind != naturalOperationNone
}

func shortToolBrief(tool *ToolSpec) string {
	if tool == nil {
		return ""
	}
	text := strings.TrimSpace(tool.Description)
	if text == "" {
		text = strings.TrimSpace(tool.Capability)
	}
	if text == "" && len(tool.Aliases) > 0 {
		text = strings.TrimSpace(tool.Aliases[0])
	}
	if utf8.RuneCountInString(text) < 20 {
		extras := make([]string, 0, 3)
		if len(tool.Aliases) > 0 {
			limit := len(tool.Aliases)
			if limit > 2 {
				limit = 2
			}
			extras = append(extras, "用于"+strings.Join(tool.Aliases[:limit], "、"))
		}
		if tool.Capability != "" {
			extras = append(extras, "能力"+strings.ReplaceAll(tool.Capability, "_", " "))
		}
		if len(tool.Tags) > 0 {
			extras = append(extras, "标签"+tool.Tags[0])
		}
		if len(extras) > 0 {
			text = text + "，" + strings.Join(extras, "，")
		}
	}
	return limitRunes(strings.Join(strings.Fields(text), " "), 30)
}

func formatToolDetailForLLM(tool *ToolSpec) string {
	return fmt.Sprintf("- %s\n  domain=%s operation=%s risk=%s approval=%v\n  description=%s\n  aliases=%s\n  tags=%s\n  args=%s",
		tool.Name,
		tool.Domain,
		tool.Operation,
		tool.Risk,
		tool.RequiresApproval || !tool.DefaultWhitelisted,
		tool.Description,
		strings.Join(tool.Aliases, ","),
		strings.Join(tool.Tags, ","),
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
		desc, _ := prop["description"].(string)
		flag := "optional"
		if required[key] {
			flag = "required"
		}
		if desc != "" {
			parts = append(parts, fmt.Sprintf("%s(%s:%s)", key, flag, desc))
		} else {
			parts = append(parts, fmt.Sprintf("%s(%s)", key, flag))
		}
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

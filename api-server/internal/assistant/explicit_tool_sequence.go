package assistant

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"go.uber.org/zap"

	"api-server/internal/model"
)

type explicitToolStep struct {
	ToolName string
	Args     map[string]interface{}
}

func (o *Orchestrator) executeExplicitToolSequenceFallback(ctx context.Context, input RunInput, messageID string) (int, string, error) {
	if o.toolDispatcher == nil || o.toolRegistry == nil || !shouldForceExplicitToolSequence(input.UserMessage) {
		return 0, "", nil
	}

	steps := parseExplicitToolSequence(input.UserMessage, o.toolRegistry.List())
	if len(steps) == 0 {
		return 0, "", nil
	}
	successCounts := o.successfulToolCountsForMessage(ctx, input.SessionID, messageID)

	sequenceCtx := context.WithValue(ctx, skipBaselineSequenceKey{}, true)
	sequenceCtx = context.WithValue(sequenceCtx, skipAssetCollectionSequenceKey{}, true)
	sequenceCtx = context.WithValue(sequenceCtx, skipVulnerabilityScriptSequenceKey{}, true)
	sequenceCtx = context.WithValue(sequenceCtx, skipDetectionSequenceKey{}, true)
	sequenceCtx = context.WithValue(sequenceCtx, skipPackageSequenceKey{}, true)

	gateway := NewAssistantToolGatewayAdapter(AssistantToolGatewayConfig{
		Dispatcher:  o.toolDispatcher,
		SessionID:   input.SessionID,
		MessageID:   messageID,
		RunID:       input.RunID,
		Operator:    input.UserID,
		Logger:      o.logger,
		RunManager:  o.runManager,
		UserInput:   input.UserMessage,
		ContextRefs: convertModelContextRefs(input.ContextRefs),
	})

	executed := 0
	seen := map[string]int{}
	for _, step := range steps {
		seen[step.ToolName]++
		if successCounts[step.ToolName] >= seen[step.ToolName] {
			continue
		}
		resp, err := gateway.Call(sequenceCtx, agentruntime.ToolRequest{
			CallID:   fmt.Sprintf("fallback_%s_%d", strings.ToLower(strings.ReplaceAll(step.ToolName, ".", "_")), time.Now().UnixNano()),
			ToolName: step.ToolName,
			Args:     step.Args,
		})
		if err != nil {
			return executed, "", err
		}
		if resp.Status != agentruntime.ToolCallSuccess {
			return executed, "", fmt.Errorf("%s failed: %s", step.ToolName, resp.ErrorMessage)
		}
		executed++
	}
	if executed == 0 {
		return 0, "", nil
	}

	summary := fmt.Sprintf("已按用户明确给出的工具序列兜底执行 %d 个工具调用。请以工具调用面板和任务状态为准查看执行结果。", executed)
	if o.logger != nil {
		o.logger.Info("explicit tool sequence fallback executed",
			zap.String("session_id", input.SessionID),
			zap.String("message_id", messageID),
			zap.Int("tool_count", executed),
		)
	}
	return executed, summary, nil
}

func (o *Orchestrator) successfulToolCountsForMessage(ctx context.Context, sessionID, messageID string) map[string]int {
	counts := map[string]int{}
	if o.toolCallRepo == nil {
		return counts
	}
	calls, _, err := o.toolCallRepo.ListBySession(ctx, sessionID, 1, 100)
	if err != nil {
		return counts
	}
	for _, call := range calls {
		if call.MessageID == messageID && call.Status == model.ToolCallStatusSuccess {
			counts[call.ToolName]++
		}
	}
	return counts
}

func shouldForceExplicitToolSequence(message string) bool {
	lower := strings.ToLower(message)
	if !strings.Contains(message, "参数") {
		return false
	}
	for _, keyword := range []string{"按顺序调用", "严格按顺序", "不要只文字说明", "必须使用工具"} {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func parseExplicitToolSequence(message string, specs []*ToolSpec) []explicitToolStep {
	if strings.TrimSpace(message) == "" || len(specs) == 0 {
		return nil
	}
	toolNames := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec != nil && spec.Enabled && strings.TrimSpace(spec.Name) != "" {
			toolNames = append(toolNames, strings.TrimSpace(spec.Name))
		}
	}
	sort.Slice(toolNames, func(i, j int) bool {
		return len(toolNames[i]) > len(toolNames[j])
	})

	var steps []explicitToolStep
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, toolName := range toolNames {
			if !strings.Contains(line, toolName) {
				continue
			}
			argsText := line
			if idx := strings.Index(line, "参数"); idx >= 0 {
				argsText = line[idx+len("参数"):]
			}
			steps = append(steps, explicitToolStep{ToolName: toolName, Args: parseExplicitToolArgs(argsText)})
			break
		}
	}
	return steps
}

func parseExplicitToolArgs(input string) map[string]interface{} {
	args := map[string]interface{}{}
	for _, part := range splitExplicitArgs(input) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.Trim(key, " ，,。；;"))
		if key == "" {
			continue
		}
		args[key] = parseExplicitToolValue(value)
	}
	return args
}

func splitExplicitArgs(input string) []string {
	input = strings.TrimSpace(strings.Trim(input, " ，,。；;"))
	var parts []string
	var current strings.Builder
	inQuote := rune(0)
	bracketDepth := 0
	for _, r := range input {
		switch r {
		case '\'', '"':
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			}
			current.WriteRune(r)
		case '[':
			if inQuote == 0 {
				bracketDepth++
			}
			current.WriteRune(r)
		case ']':
			if inQuote == 0 && bracketDepth > 0 {
				bracketDepth--
			}
			current.WriteRune(r)
		case ',', '，':
			if inQuote == 0 && bracketDepth == 0 {
				if part := strings.TrimSpace(current.String()); part != "" {
					parts = append(parts, part)
				}
				current.Reset()
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	if part := strings.TrimSpace(current.String()); part != "" {
		parts = append(parts, part)
	}
	return parts
}

func parseExplicitToolValue(value string) interface{} {
	value = strings.TrimSpace(strings.Trim(value, " ，,。；;"))
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
		if inner == "" {
			return []string{}
		}
		items := splitExplicitArgs(inner)
		values := make([]string, 0, len(items))
		for _, item := range items {
			item = strings.TrimSpace(strings.Trim(item, `"'`))
			if item != "" {
				values = append(values, item)
			}
		}
		return values
	}
	if strings.EqualFold(value, "true") {
		return true
	}
	if strings.EqualFold(value, "false") {
		return false
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	return strings.Trim(value, `"'`)
}

func convertModelContextRefs(refs []model.AssistantContextRef) []ContextRefResult {
	result := make([]ContextRefResult, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ContextRefResult{
			ObjectType: ref.ObjectType,
			ObjectID:   ref.ObjectID,
			Title:      ref.Title,
			Summary:    ref.Summary,
		})
	}
	return result
}

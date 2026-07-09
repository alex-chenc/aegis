package assistant

import (
	"strings"

	"go.uber.org/zap"
)

// ClarificationDecision describes whether the system should pause and ask
// the user for more information before proceeding.
type ClarificationDecision struct {
	Required bool   `json:"required"`
	Question string `json:"question,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Source   string `json:"source,omitempty"` // "intent_breakdown", "missing_entity", "no_accepted_tools", "config"
}

// ClarificationGate consolidates all clarification checks into a single
// evaluation point. It replaces scattered checks in intent_decomposer,
// tool_decision_engine, and orchestrator.
type ClarificationGate struct {
	config ToolDecisionConfig
	logger *zap.Logger
}

// NewClarificationGate creates a gate with the given config.
func NewClarificationGate(config ToolDecisionConfig, logger *zap.Logger) *ClarificationGate {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ClarificationGate{config: config, logger: logger}
}

// Evaluate determines whether clarification is required based on the intent
// breakdown, accepted tool names, and decision records. It returns a decision
// with the question to ask and the reason.
func (g *ClarificationGate) Evaluate(breakdown *IntentBreakdown, accepted []string, records []ToolDecisionRecord) ClarificationDecision {
	if breakdown == nil {
		return ClarificationDecision{}
	}

	// 1. 写操作目标缺失时强制追问（来自 inferMissingInfo）
	if breakdown.NeedClarification && breakdown.RequiresWrite {
		question := breakdown.ClarifyingQuestion
		if question == "" {
			question = "请补充要操作的对象和范围后再执行。"
		}
		return ClarificationDecision{
			Required: true,
			Question: question,
			Reason:   "写操作缺少明确对象或范围",
			Source:   "intent_breakdown",
		}
	}

	// 2. 无已接受工具时追问
	if len(accepted) == 0 && breakdown.NeedClarification {
		question := breakdown.ClarifyingQuestion
		if question == "" {
			question = "请补充更多信息后再执行。"
		}
		return ClarificationDecision{
			Required: true,
			Question: question,
			Reason:   "没有匹配到可执行的工具",
			Source:   "no_accepted_tools",
		}
	}

	// 3. 写操作中的必需参数缺失不能被任意已接受的只读工具掩盖。
	// 前置步骤可动态提供的参数应在裁决阶段标记为 workflowForced，而不会进入此分支。
	if breakdown.RequiresWrite || len(accepted) == 0 {
		for _, record := range records {
			if record.Decision == toolDecisionClarificationRequired && (len(accepted) == 0 || isWriteDecisionTool(record.ToolName)) {
				return ClarificationDecision{
					Required: true,
					Question: record.Reason,
					Reason:   "工具裁决要求追问: " + record.ToolName,
					Source:   "missing_entity",
				}
			}
		}
	}

	// 4. IntentBreakdown 标记需要追问（非写操作场景）
	if breakdown.NeedClarification && breakdown.ClarifyingQuestion != "" {
		// 只有在配置允许时才对非写操作追问
		if g.config.ClarificationRequiredWrite {
			return ClarificationDecision{
				Required: true,
				Question: breakdown.ClarifyingQuestion,
				Reason:   "意图拆解标记需要追问",
				Source:   "intent_breakdown",
			}
		}
	}

	return ClarificationDecision{}
}

func isWriteDecisionTool(toolName string) bool {
	for _, marker := range []string{".Trigger", ".Start", ".Stop", ".Execute", ".Generate", ".Run", ".Fix", ".Block", ".Resolve", ".Delete", ".Create", ".Update"} {
		if strings.Contains(toolName, marker) {
			return true
		}
	}
	return false
}

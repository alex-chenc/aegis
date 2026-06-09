package assistant

import (
	"context"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
	"github.com/alex-chenc/agent-runtime/router"
)

// TaskRouterAdapter 包装 agent-runtime 的 Router 为 TaskRouter 接口
type TaskRouterAdapter struct {
	inner *router.Router
}

// NewTaskRouterAdapter 创建任务路由器适配器
func NewTaskRouterAdapter(llmClient agentruntime.LLMClient, fragments []agentruntime.PromptFragment, cfg router.Config) *TaskRouterAdapter {
	return &TaskRouterAdapter{
		inner: router.New(llmClient, fragments, cfg),
	}
}

// Route 实现 TaskRouter 接口
func (a *TaskRouterAdapter) Route(ctx context.Context, input agentruntime.RouteInput) (*agentruntime.RouteResult, error) {
	if shouldForceSimpleToolRoute(input.UserMessage, input.Tools) {
		return &agentruntime.RouteResult{
			Action:            agentruntime.ActionSimpleCall,
			SelectedFragments: []string{"base_assistant", "react_format"},
		}, nil
	}

	return a.inner.Route(ctx, router.RouteInput{
		TaskID:      input.TaskID,
		UserMessage: input.UserMessage,
		Tools:       input.Tools,
		MaxSteps:    input.MaxSteps,
	})
}

func shouldForceSimpleToolRoute(message string, tools []agentruntime.ToolDescriptor) bool {
	message = strings.TrimSpace(message)
	if message == "" || len(tools) == 0 {
		return false
	}

	lower := strings.ToLower(message)
	hasDirective := false
	for _, keyword := range []string{
		"按顺序调用",
		"严格按顺序",
		"必须使用工具",
		"不要只文字说明",
		"只调用",
		"请调用",
		"调用工具",
		"工具：",
		"工具:",
		"call tool",
		"use tool",
	} {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			hasDirective = true
			break
		}
	}
	if !hasDirective {
		return false
	}

	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name != "" && strings.Contains(message, name) {
			return true
		}
	}
	return false
}

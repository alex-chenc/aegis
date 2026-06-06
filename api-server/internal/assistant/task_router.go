package assistant

import (
	"context"

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
	return a.inner.Route(ctx, router.RouteInput{
		TaskID:      input.TaskID,
		UserMessage: input.UserMessage,
		Tools:       input.Tools,
		MaxSteps:    input.MaxSteps,
	})
}

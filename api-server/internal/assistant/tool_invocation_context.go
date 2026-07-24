package assistant

import "context"

type toolInvocationContextKey struct{}

// ToolInvocationContext carries trusted dispatcher metadata to high-level
// handlers without exposing it as model-supplied tool arguments.
type ToolInvocationContext struct {
	SessionID string
	MessageID string
	RunID     string
	CallID    string
	Operator  string
	Approved  bool
}

func WithToolInvocationContext(ctx context.Context, value ToolInvocationContext) context.Context {
	return context.WithValue(ctx, toolInvocationContextKey{}, value)
}

func ToolInvocationFromContext(ctx context.Context) (ToolInvocationContext, bool) {
	value, ok := ctx.Value(toolInvocationContextKey{}).(ToolInvocationContext)
	return value, ok
}

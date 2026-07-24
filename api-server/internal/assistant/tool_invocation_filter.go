package assistant

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// ToolInvocationPhase identifies how trusted a tool request is and whether
// semantic-preserving argument normalization is still allowed.
type ToolInvocationPhase string

const (
	ToolInvocationPhaseCandidate      ToolInvocationPhase = "candidate"
	ToolInvocationPhaseDispatch       ToolInvocationPhase = "dispatch"
	ToolInvocationPhaseApprovalResume ToolInvocationPhase = "approval_resume"
)

// ToolInvocationFilterRequest is a candidate invocation before a durable tool
// call, approval, or handler execution exists.
type ToolInvocationFilterRequest struct {
	SessionID string
	MessageID string
	RunID     string
	StepID    string
	Phase     ToolInvocationPhase
	ToolName  string
	Args      map[string]interface{}
}

// ToolInvocationFilterResult contains the final validated arguments and an
// audit-safe list of field names changed by allowlisted normalization.
type ToolInvocationFilterResult struct {
	Args          map[string]interface{}
	Modified      bool
	ChangedFields []string
}

// ToolInvocationFilter applies one ordered, side-effect-free preparation rule.
// Filters may mutate Args only through SetArg/DeleteArg so mutations remain
// observable. Returning an error prevents the invocation from continuing.
type ToolInvocationFilter interface {
	Name() string
	Priority() int
	Apply(ctx context.Context, invocation *ToolInvocationFilterContext) error
}

// ToolInvocationFilterContext is shared by one ordered filter pass.
type ToolInvocationFilterContext struct {
	Request ToolInvocationFilterRequest
	Tool    *ToolSpec
	Args    map[string]interface{}

	changedFields map[string]struct{}
}

func (c *ToolInvocationFilterContext) CanModify() bool {
	return c != nil && c.Request.Phase != ToolInvocationPhaseApprovalResume
}

func (c *ToolInvocationFilterContext) SetArg(name string, value interface{}) {
	if c == nil {
		return
	}
	if current, exists := c.Args[name]; exists &&
		canonicalToolArgs(map[string]interface{}{"value": current}) == canonicalToolArgs(map[string]interface{}{"value": value}) {
		return
	}
	c.Args[name] = value
	c.changedFields[name] = struct{}{}
}

func (c *ToolInvocationFilterContext) DeleteArg(name string) {
	if c == nil {
		return
	}
	if _, exists := c.Args[name]; !exists {
		return
	}
	delete(c.Args, name)
	c.changedFields[name] = struct{}{}
}

// ToolInvocationFilterChain provides an explicit, deterministic order. This
// avoids security behavior changing with dependency-injection registration
// order.
type ToolInvocationFilterChain struct {
	registry *ToolRegistry
	logger   *zap.Logger
	filters  []ToolInvocationFilter
}

func NewToolInvocationFilterChain(registry *ToolRegistry, logger *zap.Logger) *ToolInvocationFilterChain {
	if logger == nil {
		logger = zap.NewNop()
	}
	chain := &ToolInvocationFilterChain{
		registry: registry,
		logger:   logger,
	}
	chain.register(hostResolveSelectorFilter{})
	chain.register(toolSchemaValidationFilter{})
	chain.register(toolBusinessPreflightFilter{})
	return chain
}

func (c *ToolInvocationFilterChain) register(filter ToolInvocationFilter) {
	if c == nil || filter == nil {
		return
	}
	c.filters = append(c.filters, filter)
	sort.SliceStable(c.filters, func(i, j int) bool {
		return c.filters[i].Priority() < c.filters[j].Priority()
	})
}

// Prepare normalizes allowlisted aliases and validates the resulting request.
// It never creates durable state or invokes the tool handler.
func (c *ToolInvocationFilterChain) Prepare(ctx context.Context, req ToolInvocationFilterRequest) (ToolInvocationFilterResult, error) {
	if c == nil || c.registry == nil {
		return ToolInvocationFilterResult{}, fmt.Errorf("tool invocation filter registry is not configured")
	}
	tool, ok := c.registry.Get(req.ToolName)
	if !ok {
		return ToolInvocationFilterResult{}, fmt.Errorf("tool %s not found", req.ToolName)
	}
	if req.Phase == "" {
		req.Phase = ToolInvocationPhaseDispatch
	}
	invocation := &ToolInvocationFilterContext{
		Request:       req,
		Tool:          tool,
		Args:          cloneInvocationArgs(req.Args),
		changedFields: make(map[string]struct{}),
	}
	for _, filter := range c.filters {
		if err := filter.Apply(ctx, invocation); err != nil {
			fields := []zap.Field{
				zap.String("session_id", req.SessionID),
				zap.String("run_id", req.RunID),
				zap.String("step_id", req.StepID),
				zap.String("tool_name", req.ToolName),
				zap.String("phase", string(req.Phase)),
				zap.String("filter", filter.Name()),
				zap.Error(err),
			}
			if req.Phase == ToolInvocationPhaseCandidate {
				c.logger.Debug("assistant tool candidate rejected by invocation filter", fields...)
			} else {
				c.logger.Warn("assistant tool invocation rejected before durable execution", fields...)
			}
			return ToolInvocationFilterResult{}, fmt.Errorf("%s: %w", filter.Name(), err)
		}
	}

	changedFields := make([]string, 0, len(invocation.changedFields))
	for field := range invocation.changedFields {
		changedFields = append(changedFields, field)
	}
	sort.Strings(changedFields)
	if len(changedFields) > 0 {
		c.logger.Debug("assistant tool arguments normalized before invocation",
			zap.String("session_id", req.SessionID),
			zap.String("run_id", req.RunID),
			zap.String("step_id", req.StepID),
			zap.String("tool_name", req.ToolName),
			zap.String("phase", string(req.Phase)),
			zap.Strings("changed_fields", changedFields),
		)
	}
	return ToolInvocationFilterResult{
		Args:          invocation.Args,
		Modified:      len(changedFields) > 0,
		ChangedFields: changedFields,
	}, nil
}

type hostResolveSelectorFilter struct{}

func (hostResolveSelectorFilter) Name() string  { return "host_resolve_selector_canonicalization" }
func (hostResolveSelectorFilter) Priority() int { return 200 }

func (hostResolveSelectorFilter) Apply(_ context.Context, invocation *ToolInvocationFilterContext) error {
	if invocation == nil || invocation.Tool == nil || invocation.Tool.Name != "Host.Resolve" || !invocation.CanModify() {
		return nil
	}
	rawSelector, exists := invocation.Args["selector"]
	if !exists {
		return nil
	}
	selector, ok := rawSelector.(string)
	if !ok || !isOnlineHostSelectorAlias(selector) {
		return nil
	}
	if _, exists := invocation.Args["target_scope"]; exists {
		return nil
	}
	if _, exists := invocation.Args["host_selectors"]; exists {
		return nil
	}
	if requireOnline, exists := invocation.Args["require_online"]; exists {
		online, ok := requireOnline.(bool)
		if !ok || !online {
			return nil
		}
	}

	invocation.DeleteArg("selector")
	invocation.SetArg("target_scope", "all_online_hosts")
	invocation.SetArg("require_online", true)
	return nil
}

type toolSchemaValidationFilter struct{}

func (toolSchemaValidationFilter) Name() string  { return "tool_schema_validation" }
func (toolSchemaValidationFilter) Priority() int { return 900 }

func (toolSchemaValidationFilter) Apply(_ context.Context, invocation *ToolInvocationFilterContext) error {
	if invocation == nil || invocation.Tool == nil {
		return fmt.Errorf("tool specification is unavailable")
	}
	// Apply the same closed model-facing contract at the trusted backend
	// boundary. Tool definitions that declare properties but omit
	// additionalProperties are intentionally treated as closed.
	if err := ValidateToolArgs(normalizeRuntimeArgsSchema(invocation.Tool.ArgsSchema), invocation.Args); err != nil {
		return fmt.Errorf("arguments for %s are invalid: %w", invocation.Tool.Name, err)
	}
	return nil
}

type toolBusinessPreflightFilter struct{}

func (toolBusinessPreflightFilter) Name() string  { return "tool_business_preflight" }
func (toolBusinessPreflightFilter) Priority() int { return 1000 }

func (toolBusinessPreflightFilter) Apply(ctx context.Context, invocation *ToolInvocationFilterContext) error {
	if invocation == nil || invocation.Tool == nil || invocation.Tool.Preflight == nil {
		return nil
	}
	return invocation.Tool.Preflight(ctx, invocation.Args)
}

func cloneInvocationArgs(args map[string]interface{}) map[string]interface{} {
	if len(args) == 0 {
		return make(map[string]interface{})
	}
	cloned := make(map[string]interface{}, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func isOnlineHostSelectorAlias(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "live", "alive", "online", "all_live", "all_alive", "all_online", "all_live_hosts", "all_alive_hosts", "all_online_hosts":
		return true
	default:
		return false
	}
}

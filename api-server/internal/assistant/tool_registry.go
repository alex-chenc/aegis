package assistant

import (
	"context"
	"fmt"
	"sync"
)

// ToolSpec 工具规格定义
type ToolSpec struct {
	Name               string                 `json:"name"`
	Domain             string                 `json:"domain"`
	Operation          string                 `json:"operation"`
	Description        string                 `json:"description"`
	RiskLevel          string                 `json:"risk_level"`
	RequiredPermission string                 `json:"required_permission,omitempty"`
	DefaultWhitelisted bool                   `json:"default_whitelisted"`
	Enabled            bool                   `json:"enabled"`
	ArgsSchema         map[string]interface{} `json:"args_schema"`
	OutputSchema       map[string]interface{} `json:"output_schema,omitempty"`
	Tags               []string               `json:"tags,omitempty"`
	Handler            ToolHandler            `json:"-"`
}

// ToolHandler 工具执行函数
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// ToolExecutionResult 工具执行结果
type ToolExecutionResult struct {
	Success    bool        `json:"success"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms"`
}

// ToolRegistry 工具注册中心
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolSpec
}

// NewToolRegistry 创建工具注册中心
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*ToolSpec),
	}
}

// Register 注册工具
func (r *ToolRegistry) Register(spec *ToolSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if spec.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if spec.Handler == nil {
		return fmt.Errorf("tool handler is required for %s", spec.Name)
	}
	if _, exists := r.tools[spec.Name]; exists {
		return fmt.Errorf("tool %s already registered", spec.Name)
	}

	r.tools[spec.Name] = spec
	return nil
}

// Get 获取工具
func (r *ToolRegistry) Get(name string) (*ToolSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// List 列出所有工具
func (r *ToolRegistry) List() []*ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ListByDomain 按域列出工具
func (r *ToolRegistry) ListByDomain(domain string) []*ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*ToolSpec
	for _, t := range r.tools {
		if t.Domain == domain {
			result = append(result, t)
		}
	}
	return result
}

// Execute 执行工具
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (*ToolExecutionResult, error) {
	tool, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool %s not found", name)
	}
	if !tool.Enabled {
		return nil, fmt.Errorf("tool %s is disabled", name)
	}

	startTime := timeNow()
	data, err := tool.Handler(ctx, args)
	duration := timeSince(startTime)

	if err != nil {
		return &ToolExecutionResult{
			Success:    false,
			Error:      err.Error(),
			DurationMs: duration,
		}, nil
	}

	return &ToolExecutionResult{
		Success:    true,
		Data:       data,
		DurationMs: duration,
	}, nil
}

// Count 返回工具数量
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

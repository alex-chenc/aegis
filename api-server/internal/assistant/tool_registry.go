package assistant

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ToolRisk 工具风险等级枚举
type ToolRisk string

const (
	ToolRiskReadonly ToolRisk = "readonly"
	ToolRiskLow      ToolRisk = "low"
	ToolRiskMedium   ToolRisk = "medium"
	ToolRiskHigh     ToolRisk = "high"
	ToolRiskCritical ToolRisk = "critical"
)

// ToolDomain 工具领域枚举
type ToolDomain string

const (
	DomainSystem        ToolDomain = "system"
	DomainHost          ToolDomain = "host"
	DomainBaseline      ToolDomain = "baseline"
	DomainTask          ToolDomain = "task"
	DomainVulnerability ToolDomain = "vulnerability"
	DomainDetection     ToolDomain = "detection"
	DomainSigmaRule     ToolDomain = "sigma_rule"
	DomainBlock         ToolDomain = "block"
	DomainPackage       ToolDomain = "package"
	DomainConfig        ToolDomain = "config"
	DomainAudit         ToolDomain = "audit"
	DomainAgent         ToolDomain = "agent"
	DomainInvestigation ToolDomain = "investigation"
	DomainExternalMCP   ToolDomain = "external_mcp"
	DomainNotification  ToolDomain = "notification"
)

// ToolOperation 工具操作类型枚举
type ToolOperation string

const (
	OpList     ToolOperation = "list"
	OpGet      ToolOperation = "get"
	OpSearch   ToolOperation = "search"
	OpCreate   ToolOperation = "create"
	OpUpdate   ToolOperation = "update"
	OpDelete   ToolOperation = "delete"
	OpGenerate ToolOperation = "generate"
	OpExecute  ToolOperation = "execute"
	OpDispatch ToolOperation = "dispatch"
	OpApprove  ToolOperation = "approve"
	OpRollback ToolOperation = "rollback"
)

// ServiceBinding 工具与 service 函数的绑定关系
type ServiceBinding struct {
	Component string `json:"component"`
	File      string `json:"file"`
	Function  string `json:"function"`
	Notes     string `json:"notes,omitempty"`
}

// ToolSpec 工具规格定义（完整版，对齐设计文档）
type ToolSpec struct {
	Name              string                 `json:"name"`
	Domain            ToolDomain             `json:"domain"`
	Operation         ToolOperation          `json:"operation"`
	Capability        string                 `json:"capability,omitempty"`
	Description       string                 `json:"description"`
	Aliases           []string               `json:"aliases,omitempty"`
	Tags              []string               `json:"tags,omitempty"`
	ObjectTypes       []string               `json:"object_types,omitempty"`
	PageRoutes        []string               `json:"page_routes,omitempty"`
	Risk              ToolRisk               `json:"risk"`
	AutoCallable      bool                   `json:"auto_callable"`
	RequiresApproval  bool                   `json:"requires_approval"`
	Idempotent        bool                   `json:"idempotent"`
	DefaultTimeout    time.Duration          `json:"default_timeout"`
	ArgsSchema        map[string]interface{} `json:"args_schema"`
	ResultSchema      map[string]interface{} `json:"result_schema,omitempty"`
	Handler           ToolHandler            `json:"-"`
	ServiceBinding    ServiceBinding         `json:"service_binding,omitempty"`
	DefaultWhitelisted bool                  `json:"default_whitelisted"`
	Enabled           bool                   `json:"enabled"`

	// 兼容旧字段（已废弃，使用 Risk + DefaultWhitelisted 替代）
	// Deprecated: use Risk instead
	RiskLevel string `json:"risk_level,omitempty"`
	// Deprecated: use DefaultWhitelisted instead
	RequiredPermission string `json:"required_permission,omitempty"`
	// Deprecated: use ResultSchema instead
	OutputSchema map[string]interface{} `json:"output_schema,omitempty"`
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
		if string(t.Domain) == domain {
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

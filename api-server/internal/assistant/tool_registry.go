package assistant

import (
	"context"
	"fmt"
	"strings"
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
	DomainAsset         ToolDomain = "asset"
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

const (
	ToolExecutionSynchronous  = "synchronous"
	ToolExecutionAsynchronous = "asynchronous"
)

// ToolExecutionContract describes domain-neutral capabilities that Runtime may
// need in addition to the primary tool. Companion capabilities are availability
// hints only; they never create execution steps or authorize write operations.
type ToolExecutionContract struct {
	Mode                  string             `json:"mode,omitempty"`
	CompletionCapability  string             `json:"completion_capability,omitempty"`
	DiscoveryCapabilities []string           `json:"discovery_capabilities,omitempty"`
	Prerequisites         []ToolPrerequisite `json:"prerequisites,omitempty"`
}

// ToolPrerequisite is declarative evidence gating for a companion operation.
// Runtime evaluates it without inferring a business workflow from a tool name.
type ToolPrerequisite struct {
	Capability string `json:"capability"`
	Condition  string `json:"condition"`
}

// ToolResultContract declares how a raw tool result becomes terminal business
// evidence. Result normalization remains generic and driven by these fields.
type ToolResultContract struct {
	AcceptedOnSuccess     bool              `json:"accepted_on_success,omitempty"`
	OperationStatusField  string            `json:"operation_status_field,omitempty"`
	SuccessValues         []string          `json:"success_values,omitempty"`
	PendingValues         []string          `json:"pending_values,omitempty"`
	FailureValues         []string          `json:"failure_values,omitempty"`
	OperationRefFields    []string          `json:"operation_ref_fields,omitempty"`
	ArtifactRefFields     []string          `json:"artifact_ref_fields,omitempty"`
	SideEffectRefFields   []string          `json:"side_effect_ref_fields,omitempty"`
	SatisfiesCapabilities []string          `json:"satisfies_capabilities,omitempty"`
	FactBindings          []ToolFactBinding `json:"fact_bindings,omitempty"`
}

// ToolFactBinding extracts repeatable facts from a result collection without
// coupling the evidence ledger to a concrete tool name.
type ToolFactBinding struct {
	Kind       string `json:"kind"`
	ItemsField string `json:"items_field"`
	IDField    string `json:"id_field,omitempty"`
	StateField string `json:"state_field,omitempty"`
	StateValue string `json:"state_value,omitempty"`
}

// ToolSpec 工具规格定义（完整版，对齐设计文档）
type ToolSpec struct {
	Name               string                 `json:"name"`
	Domain             ToolDomain             `json:"domain"`
	Operation          ToolOperation          `json:"operation"`
	Capability         string                 `json:"capability,omitempty"`
	Description        string                 `json:"description"`
	ModelDescription   string                 `json:"model_description,omitempty"`
	Aliases            []string               `json:"aliases,omitempty"`
	Tags               []string               `json:"tags,omitempty"`
	ObjectTypes        []string               `json:"object_types,omitempty"`
	PageRoutes         []string               `json:"page_routes,omitempty"`
	Risk               ToolRisk               `json:"risk"`
	AutoCallable       bool                   `json:"auto_callable"`
	RequiresApproval   bool                   `json:"requires_approval"`
	Idempotent         bool                   `json:"idempotent"`
	DefaultTimeout     time.Duration          `json:"default_timeout"`
	ArgsSchema         map[string]interface{} `json:"args_schema"`
	ResultSchema       map[string]interface{} `json:"result_schema,omitempty"`
	ExecutionContract  ToolExecutionContract  `json:"execution_contract,omitempty"`
	ResultContract     ToolResultContract     `json:"result_contract,omitempty"`
	Handler            ToolHandler            `json:"-"`
	ServiceBinding     ServiceBinding         `json:"service_binding,omitempty"`
	DefaultWhitelisted bool                   `json:"default_whitelisted"`
	Enabled            bool                   `json:"enabled"`

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
	if capability := strings.TrimSpace(spec.Capability); capability != "" && !capabilityIdentifierPattern.MatchString(capability) {
		return fmt.Errorf("tool %s capability %q must be a lowercase English capability identifier", spec.Name, capability)
	}
	for _, capability := range append([]string{spec.ExecutionContract.CompletionCapability}, spec.ExecutionContract.DiscoveryCapabilities...) {
		capability = strings.TrimSpace(capability)
		if capability != "" && !capabilityIdentifierPattern.MatchString(capability) {
			return fmt.Errorf("tool %s companion capability %q must be a lowercase English capability identifier", spec.Name, capability)
		}
	}
	if spec.ExecutionContract.Mode == "" {
		spec.ExecutionContract.Mode = ToolExecutionSynchronous
	}
	if spec.ExecutionContract.Mode != ToolExecutionSynchronous && spec.ExecutionContract.Mode != ToolExecutionAsynchronous {
		return fmt.Errorf("tool %s execution mode %q is invalid", spec.Name, spec.ExecutionContract.Mode)
	}
	if _, exists := r.tools[spec.Name]; exists {
		return fmt.Errorf("tool %s already registered", spec.Name)
	}
	for _, existing := range r.tools {
		if spec.Capability != "" && existing != nil && strings.EqualFold(existing.Capability, spec.Capability) {
			return fmt.Errorf("tool capability %q is already registered by %s", spec.Capability, existing.Name)
		}
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

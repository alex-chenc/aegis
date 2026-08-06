package model

import "time"

// AgentEscapeRuleDefinition is the immutable permission-boundary catalog. It
// reuses the trusted Hook/eBPF execution stream used by behavior detection,
// but has its own match rules and applies to a product's effective boundary.
type AgentEscapeRuleDefinition struct {
	RuleKey           string    `json:"rule_key"`
	RuleVersion       int64     `json:"rule_version"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	HookPoints        []string  `json:"hook_points"`
	RequiredEvidence  []string  `json:"required_evidence"`
	AgentTypes        []string  `json:"agent_types,omitempty"`
	Backends          []string  `json:"backends,omitempty"`
	BoundarySemantics []string  `json:"boundary_semantics,omitempty"`
	DefaultEnabled    bool      `json:"default_enabled"`
	DefaultSeverity   string    `json:"default_severity"`
	DefaultAction     string    `json:"default_action"`
	Source            string    `json:"source"`
	Immutable         bool      `json:"immutable"`
	Digest            string    `json:"digest"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
}

var builtinAgentEscapeRules = []AgentEscapeRuleDefinition{
	{
		RuleKey: "AGE-BUILTIN-101", RuleVersion: 1, Name: "访问工作区外路径",
		Description:       "受限会话访问当前智能体声明的工作区、临时目录或安全写入根之外的文件或目录。",
		AgentTypes:        []string{"codex", "claude-code", "openclaw", "hermes"},
		BoundarySemantics: []string{"workspace_roots", "temp_roots", "safe_write_root"},
		HookPoints:        []string{"PreToolUse", "PostToolUse", "ebpf:file_access"},
		RequiredEvidence:  []string{"permission.class", "permission.workspace_roots", "hook.tool", "hook.command", "process.pid", "process.start_ticks", "ebpf.operation", "ebpf.outcome"},
		DefaultEnabled:    true, DefaultSeverity: "high", DefaultAction: "alert", Source: "builtin", Immutable: true,
		Digest: "sha256:escape-permission-101",
	},
	{
		RuleKey: "AGE-BUILTIN-102", RuleVersion: 1, Name: "受限网络访问",
		Description:       "网络关闭、域名不在 allowlist 或命中 denylist 的受限会话通过 curl、socket 或其他网络工具访问目标。",
		AgentTypes:        []string{"codex", "claude-code", "openclaw", "hermes"},
		BoundarySemantics: []string{"network_access", "allowed_domains", "denied_domains"},
		HookPoints:        []string{"PreToolUse", "PostToolUse", "ebpf:network_connect"},
		RequiredEvidence:  []string{"permission.network_access", "hook.tool", "hook.command", "process.pid", "process.start_ticks", "ebpf.destination", "ebpf.outcome"},
		DefaultEnabled:    true, DefaultSeverity: "high", DefaultAction: "alert", Source: "builtin", Immutable: true,
		Digest: "sha256:escape-permission-102",
	},
	{
		RuleKey: "AGE-BUILTIN-103", RuleVersion: 1, Name: "访问容器运行时控制接口",
		Description:       "受限会话访问 Docker、containerd、CRI-O 或 Podman 控制 socket，试图从工作区沙箱控制宿主容器。",
		AgentTypes:        []string{"codex", "openclaw", "hermes", "claude-code"},
		BoundarySemantics: []string{"runtime_socket_denied"},
		HookPoints:        []string{"PreToolUse", "PostToolUse", "ebpf:unix_connect"},
		RequiredEvidence:  []string{"permission.class", "hook.tool", "hook.command", "process.pid", "process.start_ticks", "ebpf.socket", "ebpf.outcome"},
		DefaultEnabled:    true, DefaultSeverity: "critical", DefaultAction: "alert", Source: "builtin", Immutable: true,
		Digest: "sha256:escape-permission-103",
	},
	{
		RuleKey: "AGE-BUILTIN-104", RuleVersion: 1, Name: "执行进程边界操作",
		Description:       "受限会话执行 setns、mount、ptrace、内核加载或 capability 变更等进程边界操作。",
		AgentTypes:        []string{"codex", "openclaw", "hermes", "claude-code"},
		BoundarySemantics: []string{"namespace", "mount", "ptrace", "capability"},
		HookPoints:        []string{"PreToolUse", "PostToolUse", "ebpf:guarded_syscall"},
		RequiredEvidence:  []string{"permission.class", "hook.tool", "hook.command", "process.pid", "process.start_ticks", "ebpf.operation", "ebpf.outcome"},
		DefaultEnabled:    true, DefaultSeverity: "critical", DefaultAction: "alert", Source: "builtin", Immutable: true,
		Digest: "sha256:escape-permission-104",
	},
	{
		RuleKey: "AGE-BUILTIN-105", RuleVersion: 1, Name: "绕过操作确认边界",
		Description: "Zcode 等确认型模式要求确认的命令，在未获得确认时已经由 Hook 关联的进程实际执行。",
		AgentTypes:  []string{"zcode", "claude-code"}, BoundarySemantics: []string{"approval_required", "approval_status"},
		HookPoints:       []string{"PreToolUse", "PostToolUse", "ebpf:process_exec", "ebpf:file_access", "ebpf:network_connect"},
		RequiredEvidence: []string{"permission.approval_required", "permission.approval_status", "hook.tool", "hook.command", "process.pid", "process.start_ticks", "ebpf.outcome"},
		DefaultEnabled:   true, DefaultSeverity: "high", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-permission-105",
	},
	{
		RuleKey: "AGE-BUILTIN-106", RuleVersion: 1, Name: "写入 Hermes 受保护路径",
		Description: "Hermes 配置了 HERMES_WRITE_SAFE_ROOT 时，写入安全根之外的路径。",
		AgentTypes:  []string{"hermes"}, BoundarySemantics: []string{"safe_write_root"},
		HookPoints:       []string{"pre_tool_call", "post_tool_call", "ebpf:file_access"},
		RequiredEvidence: []string{"permission.safe_write_root", "hook.tool", "hook.command", "process.pid", "process.start_ticks", "ebpf.path", "ebpf.outcome"},
		DefaultEnabled:   true, DefaultSeverity: "medium", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-permission-106",
	},
	{
		RuleKey: "AGE-BUILTIN-107", RuleVersion: 1, Name: "OpenClaw 提权绕过沙箱",
		Description: "OpenClaw tools.elevated 使工具绕过 Docker/Podman 沙箱并在宿主执行。",
		AgentTypes:  []string{"openclaw"}, Backends: []string{"docker", "podman", "openshell"}, BoundarySemantics: []string{"elevated", "workspace_access", "network_access"},
		HookPoints:       []string{"before_tool_call", "after_tool_call", "ebpf:process_exec"},
		RequiredEvidence: []string{"permission.elevated", "hook.tool", "hook.command", "process.pid", "process.start_ticks", "ebpf.outcome"},
		DefaultEnabled:   true, DefaultSeverity: "critical", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-permission-107",
	},
}

// BuiltinAgentEscapeRuleManifest returns a copy so callers cannot mutate the
// process-wide immutable catalog.
func BuiltinAgentEscapeRuleManifest() []AgentEscapeRuleDefinition {
	result := make([]AgentEscapeRuleDefinition, 0, len(builtinAgentEscapeRules))
	for _, rule := range builtinAgentEscapeRules {
		rule.HookPoints = append([]string(nil), rule.HookPoints...)
		rule.RequiredEvidence = append([]string(nil), rule.RequiredEvidence...)
		rule.AgentTypes = append([]string(nil), rule.AgentTypes...)
		rule.Backends = append([]string(nil), rule.Backends...)
		rule.BoundarySemantics = append([]string(nil), rule.BoundarySemantics...)
		result = append(result, rule)
	}
	return result
}

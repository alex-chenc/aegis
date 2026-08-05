package model

import "time"

// AgentEscapeRuleDefinition is deliberately separate from
// AgentBehaviorRuleDefinition. Escape rules describe isolation-boundary Hook
// points and require a process plus /proc and cgroup re-verification chain;
// they must never be enabled or disabled through the behavior-rule catalog.
type AgentEscapeRuleDefinition struct {
	RuleKey          string    `json:"rule_key"`
	RuleVersion      int64     `json:"rule_version"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	HookPoints       []string  `json:"hook_points"`
	RequiredEvidence []string  `json:"required_evidence"`
	DefaultEnabled   bool      `json:"default_enabled"`
	DefaultSeverity  string    `json:"default_severity"`
	DefaultAction    string    `json:"default_action"`
	Source           string    `json:"source"`
	Immutable        bool      `json:"immutable"`
	Digest           string    `json:"digest"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
}

var builtinAgentEscapeRules = []AgentEscapeRuleDefinition{
	{RuleKey: "AGE-BUILTIN-001", RuleVersion: 1, Name: "跨命名空间加入", Description: "检测 setns/unshare 等导致进程加入预期外 namespace 的尝试。", HookPoints: []string{"tracepoint/syscalls/sys_enter_setns", "tracepoint/syscalls/sys_enter_unshare", "lsm/task_setnice"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "proc.namespace", "cgroup.before", "cgroup.after", "outcome"}, DefaultEnabled: true, DefaultSeverity: "high", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-001"},
	{RuleKey: "AGE-BUILTIN-002", RuleVersion: 1, Name: "脱离预期 cgroup", Description: "检测进程写入 cgroupfs 或迁移到不在执行单元基线中的 cgroup。", HookPoints: []string{"tracepoint/syscalls/sys_enter_write", "tracepoint/syscalls/sys_enter_openat"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "proc.cgroup", "cgroup.before", "cgroup.after", "outcome"}, DefaultEnabled: true, DefaultSeverity: "high", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-002"},
	{RuleKey: "AGE-BUILTIN-003", RuleVersion: 1, Name: "访问宿主 proc 根", Description: "检测进程通过 /proc、nsfs 或 procfd 访问宿主命名空间和进程信息。", HookPoints: []string{"tracepoint/syscalls/sys_enter_openat", "tracepoint/syscalls/sys_enter_openat2"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "proc.mountinfo", "proc.ns", "cgroup.path", "outcome"}, DefaultEnabled: true, DefaultSeverity: "high", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-003"},
	{RuleKey: "AGE-BUILTIN-004", RuleVersion: 1, Name: "访问容器运行时 socket", Description: "检测访问 Docker、containerd、CRI-O 等容器运行时控制 socket。", HookPoints: []string{"lsm/socket_connect", "tracepoint/syscalls/sys_enter_connect"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "proc.exe", "proc.cgroup", "resource.socket", "outcome"}, DefaultEnabled: true, DefaultSeverity: "critical", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-004"},
	{RuleKey: "AGE-BUILTIN-005", RuleVersion: 1, Name: "宿主敏感路径挂载", Description: "检测 mount、pivot_root、chroot 等把宿主敏感路径带入执行单元的操作。", HookPoints: []string{"tracepoint/syscalls/sys_enter_mount", "tracepoint/syscalls/sys_enter_pivot_root", "tracepoint/syscalls/sys_enter_chroot"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "proc.mountinfo", "proc.root", "cgroup.path", "outcome"}, DefaultEnabled: true, DefaultSeverity: "critical", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-005"},
	{RuleKey: "AGE-BUILTIN-006", RuleVersion: 1, Name: "外部进程 ptrace", Description: "检测跨执行单元 ptrace 或进程注入行为。", HookPoints: []string{"tracepoint/syscalls/sys_enter_ptrace", "tracepoint/syscalls/sys_enter_process_vm_writev"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "target.pid", "target.start_ticks", "target.cgroup", "outcome"}, DefaultEnabled: true, DefaultSeverity: "high", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-006"},
	{RuleKey: "AGE-BUILTIN-007", RuleVersion: 1, Name: "加载 BPF 或内核模块", Description: "检测 BPF 程序、BTF 或内核模块加载，复核执行进程的身份和 cgroup。", HookPoints: []string{"lsm/bpf", "tracepoint/syscalls/sys_enter_bpf", "tracepoint/syscalls/sys_enter_init_module"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "proc.status", "proc.cgroup", "capabilities.before", "capabilities.after", "outcome"}, DefaultEnabled: true, DefaultSeverity: "critical", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-007"},
	{RuleKey: "AGE-BUILTIN-008", RuleVersion: 1, Name: "身份或 capability 越界", Description: "检测 UID/GID、capability、no_new_privs 和 user namespace 相对基线的变化。", HookPoints: []string{"tracepoint/syscalls/sys_enter_setuid", "tracepoint/syscalls/sys_enter_setgid", "tracepoint/syscalls/sys_enter_capset"}, RequiredEvidence: []string{"hook.event", "process.pid", "process.start_ticks", "proc.status", "proc.uid_map", "proc.gid_map", "capabilities.after", "cgroup.path", "outcome"}, DefaultEnabled: true, DefaultSeverity: "critical", DefaultAction: "alert", Source: "builtin", Immutable: true, Digest: "sha256:escape-008"},
}

// BuiltinAgentEscapeRuleManifest returns a copy so callers cannot mutate the
// process-wide immutable catalog.
func BuiltinAgentEscapeRuleManifest() []AgentEscapeRuleDefinition {
	result := make([]AgentEscapeRuleDefinition, 0, len(builtinAgentEscapeRules))
	for _, rule := range builtinAgentEscapeRules {
		rule.HookPoints = append([]string(nil), rule.HookPoints...)
		rule.RequiredEvidence = append([]string(nil), rule.RequiredEvidence...)
		result = append(result, rule)
	}
	return result
}

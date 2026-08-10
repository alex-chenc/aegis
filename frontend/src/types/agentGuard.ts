export type AgentGuardCoverage =
  | 'full_enforcement'
  | 'behavior_monitor_escape_enforce'
  | 'monitor_only'
  | 'no_isolation'
  | 'remote_unobservable'
  | 'unsupported'
  | 'unsupported_profile'
  | 'degraded'
  | 'unknown'

export type AgentGuardMode = 'behavior' | 'escape'
export type AgentGuardDetailTab = 'panorama' | 'analysis'

export type AgentConfigFindingSeverity = 'critical' | 'high' | 'medium' | 'low' | string

export interface AgentConfigFinding {
  rule_id: string
  severity: AgentConfigFindingSeverity
  field_path: string
  value?: string
  title: string
  reason: string
  remediation: string
}

export interface AgentConfigFile {
  path: string
  format: string
  status: string
  size?: number
  mode?: string
  modified_at?: string
  sha256?: string
  content?: string
  error?: string
  findings: AgentConfigFinding[]
}

export interface AgentConfigHook {
  file_path: string
  field_path: string
  event: string
  command: string
  executor?: string
  findings: AgentConfigFinding[]
}

export interface AgentConfigAgent {
  agent_type: string
  display_name: string
  files: AgentConfigFile[]
  hooks: AgentConfigHook[]
  finding_count: number
}

export interface AgentConfigScanError {
  stage: string
  message: string
}

export interface AgentConfigScanResult {
  host_id: string
  hostname?: string
  scanned_at: string
  agents: AgentConfigAgent[]
  errors?: AgentConfigScanError[]
  finding_count: number
}

export type AgentRuntimeStatus = 'running' | 'stale' | 'stopped' | 'unknown'

export type AgentGuardRuntimeDispatchStatus =
  | 'not_dispatched'
  | 'pending'
  | 'pending_reconnect'
  | 'dispatched'
  | 'failed'
  | string

export interface AgentGuardHookInjection {
  agent_type: string
  enabled: boolean
  behavior_enabled?: boolean
  escape_enabled?: boolean
  status: string
  error_code?: string
  updated_at?: string
}

export interface AgentGuardRuntimeSettings {
  schema: string
  version: number
  host_id: string
  tool_adapter_enabled: boolean
  session_hook_enabled: boolean
  behavior_policy_enabled: boolean
  escape_policy_enabled: boolean
  injections: AgentGuardHookInjection[]
  dispatch_status: AgentGuardRuntimeDispatchStatus
  dispatch_error_code?: string
  updated_at?: string
}

export type ExecutionUnitType =
  | 'local_process_tree'
  | 'linux_namespace'
  | 'oci_container'
  | 'remote_sandbox'
  | 'whole_process_container'
  | string

export interface PageResult<T> {
  items: T[]
  total: number
}

export interface AgentGuardOverview {
  agent_assets?: number
  running_instances?: number
  monitored_instances?: number
  high_risk_findings?: number
  successful_blocks?: number
  escape_attempts?: number
  frozen_units?: number
  execution_units?: number
  coverage?: Partial<Record<AgentGuardCoverage, number>>
  updated_at?: string
  stale?: boolean
}

export interface AgentGuardAgentSummary {
  agent_scope_key: string
  asset_id?: string
  host: {
    id: string
    hostname: string
    ip: string
  }
  agent_type: string
  display_name: string
  profile_key?: string
  running_instance_count: number
  /** Number of statically collected Claude/Codex sessions for this host/type. */
  session_count?: number
  controller_pids: number[]
  runtime_status: AgentRuntimeStatus
  asset_status?: 'running' | 'stopped' | string
  asset_collected_at?: string
  isolation_types: ExecutionUnitType[]
  coverage_level: AgentGuardCoverage
  coverage_reasons: string[]
  high_risk_finding_count: number
  escape_finding_count: number
  action_status?: string
  last_seen_at?: string
}

export interface AgentRuntimeInstance {
  id: string
  host_id: string
  asset_id?: string
  agent_type: string
  display_name?: string
  profile_key?: string
  profile_version?: number
  controller_pid: number
  controller_start_ticks: string
  status: AgentRuntimeStatus
  coverage_level: AgentGuardCoverage
  coverage_reasons: string[]
  high_risk_finding_count?: number
  last_seen_at?: string
}

export interface AgentBehaviorSession {
  id: string
  host_id: string
  instance_id: string
  execution_unit_id?: string
  external_session_id?: string
  source: string
  confidence: string
  status: string
  behavior_count?: number
  finding_count?: number
  started_at: string
  last_seen_at: string
  permission?: {
    agent_type?: string
    backend?: string
    boundary?: 'enforced' | 'none' | 'no_isolation' | 'remote_unobservable' | string
    class?: 'full_access' | 'restricted' | 'unknown' | string
    permission_mode?: string
    sandbox_mode?: string
    approval_policy?: string
    approval_status?: string
    cwd?: string
    workspace_roots?: string[]
    temp_roots?: string[]
    network_access?: boolean
    sandbox_enabled?: boolean
    workspace_access?: 'none' | 'ro' | 'rw' | string
    allowed_domains?: string[]
    denied_domains?: string[]
    elevated?: boolean
    approval_required?: boolean
    safe_write_root?: string
    remote_execution_id?: string
    complete?: boolean
  }
}

export interface AgentExecutionUnit {
  id: string
  host_id?: string
  instance_id: string
  unit_type: ExecutionUnitType
  fingerprint?: string
  root_pid?: number
  process_count?: number
  root_start_ticks?: string
  cgroup_id?: string
  cgroup_path?: string
  container_id?: string
  container_runtime?: string
  remote_backend?: string
  remote_execution_id?: string
  coverage_level: AgentGuardCoverage
  coverage_reasons: string[]
  status: string
  isolation_baseline: Record<string, unknown>
  isolation_actual: Record<string, unknown>
  isolation_diff: Record<string, unknown>
  first_seen_at: string
  last_seen_at: string
}

export type AgentGuardActionName =
  | 'freeze_execution_unit'
  | 'resume_execution_unit'
  | 'hold_execution_unit'
  | 'kill_execution_unit'
  | 'kill_agent_instance'
  | 'auto_resume'

export type AgentGuardActionStatus =
  | 'pending'
  | 'dispatching'
  | 'running'
  | 'success'
  | 'failed'
  | 'expired'
  | 'cancelled'
  | string

export interface AgentGuardActionRequest {
  reason: string
  hold?: boolean
}

export interface AgentGuardActionAccepted {
  action_id: string
  command_id: string
  status: AgentGuardActionStatus
}

export interface AgentGuardAction {
  id: string
  command_id?: string
  host_id?: string
  instance_id?: string
  execution_unit_id?: string
  action: AgentGuardActionName
  source?: 'local_policy' | 'correlation_policy' | 'manual' | 'timeout' | 'system' | string
  status: AgentGuardActionStatus
  reason: string
  requested_by?: string
  hold_requested?: boolean
  freeze_timeout_seconds?: number
  result?: Record<string, unknown>
  error_code?: string
  error_message?: string
  requested_at?: string
  dispatched_at?: string
  completed_at?: string
  expires_at?: string
}

export interface PanoramaTreeNode {
  id: string
  parent_id?: string
  node_type: string
  label: string
  severity?: 'info' | 'low' | 'medium' | 'high' | 'critical'
  has_children?: boolean
  child_count?: number
  occurred_at?: string
  pid?: number
  ppid?: number
  process_start_ticks?: string
  process_status?: 'running' | 'stopped' | 'unknown' | string
  cmdline?: string
  external_session_id?: string
  session_source?: 'agent_official' | 'adapter_hook' | 'aegis_wrapper' | 'activity_window' | 'execution_unit' | string
  session_confidence?: 'confirmed' | 'probable' | 'inferred' | string
	tool_name?: string
	tool_call_id?: string
	turn_id?: string
	command?: string
	tool_input?: unknown
	tool_response?: unknown
	correlation_status?: 'matched' | 'unmatched' | string
	correlation_method?: string
  event_id?: string
  object_id?: string
  execution_unit_id?: string
  collection?: {
    visibility?: 'complete' | 'partial' | 'unobservable'
    truncated_fields?: string[]
    lost_events_since_last?: number
    limitations?: Array<'tool_semantics_unobservable' | 'remote_unobservable' | string>
  }
  trust?: {
    tool_semantics?: 'trusted' | 'tool_semantics_unobservable'
    source?: 'agent_official' | 'adapter_hook' | 'aegis_wrapper'
    proof_verified?: boolean
    remote_visibility?: 'trusted_sensor' | 'remote_unobservable'
    correlation?: 'matched' | 'unmatched'
  }
  children?: PanoramaTreeNode[]
}

export interface AgentPanoramaResponse {
  root?: PanoramaTreeNode | null
  items?: PanoramaTreeNode[]
  nodes?: PanoramaTreeNode[]
  total?: number
  next_cursor?: string
}

export interface AgentSecurityFindingSummary {
  id: string
  asset_id?: string
  agent_scope_key?: string
  instance_id?: string
  session_id?: string
  agent_type?: string
  agent_display_name?: string
  host?: {
    id?: string
    hostname?: string
    ip?: string
  }
  title: string
  severity: 'info' | 'low' | 'medium' | 'high' | 'critical'
  verdict?: 'benign' | 'suspicious' | 'malicious' | 'inconclusive'
  confidence?: number
  decision_sources?: Array<'rule' | 'ai' | 'combined'>
  rule_hits?: Array<{
    rule_key?: string
    rule_version?: number
    rule_name?: string
    severity?: string
    match_kind?: string
    event_id?: string
    event_ids?: string[]
    evidence_event_ids?: string[]
  }>
  matched_rules?: AgentSecurityFindingRuleDetail[]
  evidence_event_ids?: string[]
  evidence_event_count?: number
  evidence_graph?: {
    nodes?: Array<Record<string, unknown>>
    edges?: Array<Record<string, unknown>>
  }
  evidence_completeness?: {
    visibility?: 'complete' | 'partial' | 'unobservable'
    reasons?: string[]
    lost_events?: number
    truncated_fields?: string[]
  }
  escape_chain?: {
    hook_event_ids?: string[]
    hook_events?: Array<{
      event_id: string
      event_type?: string
      tool_name?: string
      command?: string
      command_line?: string
      pid?: number
      ppid?: number
      process_start_ticks?: string
      process_name?: string
      process_exe?: string
      cwd?: string
      outcome?: string
      decision?: string
      target?: string
    }>
    process_evidence?: Array<Record<string, unknown>>
    execution_evidence?: Array<Record<string, unknown>>
    permission?: {
      agent_type?: string
      backend?: string
      boundary?: string
      class?: 'full_access' | 'restricted' | 'unknown' | string
      permission_mode?: string
      sandbox_mode?: string
      approval_policy?: string
      approval_status?: string
      cwd?: string
      workspace_roots?: string[]
      temp_roots?: string[]
      network_access?: boolean
      sandbox_enabled?: boolean
      workspace_access?: string
      allowed_domains?: string[]
      denied_domains?: string[]
      elevated?: boolean
      approval_required?: boolean
      safe_write_root?: string
      remote_execution_id?: string
      complete?: boolean
    }
    classification?: 'policy_violation_attempt' | 'confirmed_escape' | 'authorized_boundary_expansion' | 'not_applicable' | 'evidence_insufficient' | string
    gaps?: string[]
  }
  attack_stages?: Array<string | Record<string, unknown>>
  summary?: string
  counter_evidence?: string[]
  uncertainties?: string[]
  recommended_action?: string
  analysis_status?: string
  status: 'open' | 'investigating' | 'contained' | 'resolved' | 'dismissed' | string
  last_observed_at?: string
}

export interface AgentSecurityFindingRuleDetail {
  rule_key: string
  rule_version?: number
  name: string
  severity?: string
  match_kind?: string
  event_ids: string[]
  process_tree?: AgentSecurityFindingProcessNode[]
  tool_calls?: AgentSecurityFindingToolCall[]
}

export interface AgentSecurityFindingToolCall {
  event_id: string
  tool_name: string
  tool_call_id?: string
  turn_id?: string
  command?: string
  tool_input?: unknown
  tool_response?: unknown
  outcome?: string
  occurred_at?: string
  pid?: number
  ppid?: number
  process_start_ticks?: string
  command_line?: string
  correlation_status?: 'matched' | 'unmatched' | string
  correlation_method?: string
}

export interface AgentSecurityFindingProcessNode {
  id: string
  parent_id?: string
  pid: number
  ppid: number
  process_start_ticks?: string
  process_name?: string
  process_exe?: string
  cmdline?: string
  command_cwd?: string
  command_visibility?: string
  process_status?: string
  first_seen_at?: string
  last_seen_at?: string
  event_count?: number
  matched?: boolean
  matched_event_ids?: string[]
  children?: AgentSecurityFindingProcessNode[]
}

export interface AgentBehaviorIndex {
  id: string
  event_id?: string
  asset_id?: string
  agent_scope_key?: string
  instance_id?: string
  agent_type?: string
  agent_display_name?: string
  host?: {
    id?: string
    hostname?: string
    ip?: string
  }
  category?: string
  operation?: string
  outcome?: string
  pid?: number
  ppid?: number
  process_start_ticks?: string
  process_name?: string
  process_exe?: string
  command_argv?: string[]
  command_cwd?: string
  command_visibility?: string
  actor?: Record<string, unknown>
  resource?: Record<string, unknown>
  collection?: Record<string, unknown>
  occurred_at?: string
}

export interface BuiltinAgentBehaviorRuleSummary {
  id?: string
  rule_key: string
  rule_version: number
  name: string
  description?: string
  source?: string
  categories?: string[]
  default_enabled?: boolean
  enabled?: boolean
  engine?: string
  default_severity?: string
  severity?: string
  default_action?: string
  action?: string
  recommended_action?: string
  parameters_schema?: Record<string, unknown>
  default_parameters?: Record<string, unknown>
  required_evidence?: string[]
  allow_conditions?: string[]
  mitre?: string[]
  immutable?: boolean
  hits_24h?: number
  findings_24h?: number
  digest?: string
}

export interface BuiltinAgentEscapeRuleSummary {
  rule_key: string
  rule_version: number
  name: string
  description?: string
  hook_points?: string[]
  required_evidence?: string[]
  agent_types?: string[]
  backends?: string[]
  boundary_semantics?: string[]
  default_enabled?: boolean
  enabled?: boolean
  default_severity?: string
  severity?: string
  default_action?: string
  action?: string
  source?: string
  immutable?: boolean
  digest?: string
}

export interface AgentSecurityAnalysisRun {
  id: string
  finding_id: string
  attempt: number
  status: 'pending' | 'running' | 'completed' | 'failed' | 'invalid_output' | 'inconclusive' | string
  provider?: string
  model?: string
  prompt_version: string
  input_digest: string
  evidence_event_ids: string[]
  verdict?: 'benign' | 'suspicious' | 'malicious' | 'inconclusive'
  attack_probability?: number
  confidence?: number
  output?: {
    summary?: string
    intent_hypotheses?: Array<Record<string, unknown>>
    attack_chain?: Array<Record<string, unknown>>
    counter_evidence?: string[]
    uncertainties?: string[]
    recommended_action?: string
  }
  error_code?: string
  started_at?: string
  completed_at?: string
}

export interface AgentGuardListFilters {
  host_id: string
  agent_types: string[]
  runtime_status: string
  coverage: string
  isolation_type: string
  keyword: string
}

export interface AgentGuardAgentQuery {
  host_ids?: string[]
  agent_types?: string[]
  runtime_status?: string
  coverage?: string
  isolation_type?: string
  keyword?: string
  page: number
  page_size: number
}

export interface AgentGuardInstanceQuery {
  host_id?: string
  asset_ids?: string[]
  agent_scope_key?: string
  agent_types?: string[]
  instance_ids?: string[]
  status?: string
  coverage?: string
  page?: number
  page_size?: number
}

export interface AgentGuardPanoramaQuery {
  asset_id?: string
  agent_scope_key?: string
  instance_ids?: string[]
  session_id?: string
  page?: number
  page_size?: number
  cursor?: string
}

export interface AgentGuardExecutionUnitQuery {
  host_id?: string
  instance_id?: string
  unit_type?: string
  coverage?: string
  status?: string
  page?: number
  page_size?: number
}

export interface AgentGuardFindingQuery {
	host_id?: string
	asset_id?: string
	agent_scope_key?: string
	instance_id?: string
	session_id?: string
	finding_domain?: 'tool' | 'escape'
	severity?: string
  status?: string
  page: number
  page_size: number
}

export interface AgentGuardRuleQuery {
  enabled?: boolean
  keyword?: string
  page?: number
  page_size?: number
}

export interface AgentGuardPolicyTargets {
  host_ids: string[]
  host_group_ids: string[]
  agent_types: string[]
  profile_keys?: string[]
}

export interface AgentGuardPolicyDraftRequest {
  policy_key: string
  name: string
  description?: string
  priority: number
  targets: AgentGuardPolicyTargets
  collection: {
    categories: string[]
    tool_adapter_enabled?: boolean
    command_argv: 'redacted' | 'disabled'
    file_content: 'disabled'
    network_content: 'disabled'
    aggregation: Record<string, number>
  }
  builtin_rule_overrides: Array<Record<string, unknown>>
  atomic_rules: Array<Record<string, unknown>>
  correlation_rules: Array<Record<string, unknown>>
  analysis: {
    enabled: boolean
    trigger_severities: string[]
    ai_only_action_ceiling: 'audit' | 'alert'
    evidence_window_seconds: number
  }
  escape_rules: Array<Record<string, unknown>>
  freeze_timeout_seconds: number
}

export interface AgentGuardPolicy {
  id: string
  policy_key: string
  version: number
  name: string
  description?: string
  status: 'draft' | 'published' | 'superseded' | 'disabled'
  priority: number
  digest?: string
  published_by?: string
  published_at?: string
  updated_at?: string
}

export interface AgentGuardPolicyValidation {
  valid: boolean
  digest?: string
  errors: Array<{ field: string; code: string; message: string }>
  warnings: Array<{ field: string; code: string; message: string }>
}

export interface AgentGuardPolicyMutationResult {
  policy: AgentGuardPolicy
  validation: AgentGuardPolicyValidation
}

export interface AgentGuardPolicyDelivery {
  id: string
  host_id: string
  bundle_version: number
  bundle_digest: string
  status: 'pending' | 'dispatching' | 'received' | 'applied' | 'degraded' | 'failed' | string
  coverage_level?: AgentGuardCoverage
  error_code?: string
  error_message?: string
  generated_at: string
  applied_at?: string
}

export interface AgentGuardPolicyPublishResult {
  policy: AgentGuardPolicy
  deliveries: AgentGuardPolicyDelivery[]
}

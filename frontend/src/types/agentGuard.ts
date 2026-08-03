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

export type AgentRuntimeStatus = 'running' | 'stale' | 'stopped' | 'unknown'

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
  controller_pids: number[]
  runtime_status: AgentRuntimeStatus
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
  session_source?: 'agent_official' | 'adapter_hook' | 'aegis_wrapper' | 'activity_window' | 'execution_unit' | string
  session_confidence?: 'confirmed' | 'probable' | 'inferred' | string
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
    severity?: string
    event_ids?: string[]
    evidence_event_ids?: string[]
  }>
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
  attack_stages?: Array<string | Record<string, unknown>>
  summary?: string
  counter_evidence?: string[]
  uncertainties?: string[]
  recommended_action?: string
  analysis_status?: string
  status: 'open' | 'investigating' | 'contained' | 'resolved' | 'dismissed' | string
  last_observed_at?: string
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
  default_enabled?: boolean
  enabled?: boolean
  engine?: string
  default_severity?: string
  severity?: string
  default_action?: string
  action?: string
  hits_24h?: number
  findings_24h?: number
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

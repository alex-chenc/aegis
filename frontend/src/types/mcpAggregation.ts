export type MCPJobStatus =
  | 'created'
  | 'validating_endpoint'
  | 'awaiting_auth'
  | 'authenticating'
  | 'discovering'
  | 'validating_tools'
  | 'security_scanning'
  | 'classifying'
  | 'building_release'
  | 'awaiting_approval'
  | 'publishing'
  | 'active'
  | 'failed'
  | 'cancelled'

export interface MCPOverview {
  remote_servers: number
  published_tools: number
  active_clients: number
  pending_approvals: number
  high_risk_calls_24h: number
  updated_at: string
}

export interface MCPOnboardingJob {
  id: string
  server_id?: string
  display_name: string
  endpoint_display: string
  auth_type: string
  environment: string
  publish_policy: string
  status: MCPJobStatus
  step: MCPJobStatus
  attempt: number
  error_code?: string
  error_message?: string
  revision_id?: string
  created_at: string
  updated_at: string
  completed_at?: string
}

export interface MCPServer {
  id: string
  server_key: string
  display_name: string
  owner_user_id: string
  environment: string
  transport: string
  endpoint_display: string
  auth_type: string
  protocol_version?: string
  risk_tier: string
  lifecycle_status: string
  active_revision_id?: string
  tool_count: number
  published_tool_count: number
  last_health_status?: string
  last_error_code?: string
  last_error_message?: string
  last_synced_at?: string
  created_at: string
  updated_at: string
}

export interface MCPToolRevision {
  id: string
  server_id: string
  server_name: string
  server_revision_id: string
  upstream_name: string
  alias: string
  title?: string
  description?: string
  input_schema: Record<string, unknown>
  output_schema: Record<string, unknown>
  verified_metadata: Record<string, unknown>
  risk_tier: string
  status: string
  created_at: string
  release_tool_id?: string
}

export interface MCPClient {
  id: string
  client_key: string
  display_name: string
  client_type: string
  status: string
  created_by: string
  created_at: string
}

export interface MCPClientEndpointTool {
  alias: string
  title?: string
  description?: string
  risk_tier: string
  enabled: boolean
}

export interface MCPClientEndpoint {
  client_id: string
  client_key: string
  display_name: string
  client_type: string
  status: string
  grant_id: string
  server_id: string
  server_name: string
  endpoint: string
  expires_at?: string
  tools: MCPClientEndpointTool[]
}

export interface MCPClientEndpointCreated extends MCPClientEndpoint {
  token: string
}

export interface MCPClientEndpointRevokeResult {
  client_id: string
  client_key: string
  grant_id?: string
  status: string
  revoked: boolean
  changed: boolean
}

export interface MCPCatalog {
  id: string
  catalog_key: string
  display_name: string
  status: string
  created_by: string
  created_at: string
}

export interface MCPApprovalRequest {
  id: string
  approval_type: string
  subject_type: string
  subject_id: string
  requested_by: string
  status: string
  request_digest?: string
  reason?: string
  created_at: string
}

export interface MCPInvocation {
  id: string
  client_id?: string
  client_key: string
  client_name: string
  server_id: string
  server_name: string
  tool_revision_id?: string
  tool_alias: string
  tool_enabled: boolean
  status: string
  policy_decision?: string
  authorization_outcome?: string
  authorization_reason_code?: string
  authorization_policy_revision?: string
  authorization_deny_rule_ids?: string[]
  authorization_audit_rule_ids?: string[]
  created_at: string
  completed_at?: string
}

export interface MCPInvocationToolDisableResult {
  invocation_id: string
  client_id: string
  grant_id: string
  server_id: string
  tool_alias: string
  disabled: boolean
  changed: boolean
}

export interface MCPSecurityVerdict {
  id: string
  invocation_id: string
  client_id?: string
  client_key: string
  client_name: string
  server_id: string
  server_name: string
  tool_alias: string
  invocation_status: string
  invocation_created_at: string
  deterministic_severity: string
  engine: string
  source: string
  phase: string
  action: string
  policy_revision?: string
  rule_ids: string[]
  audit_rule_ids: string[]
  matched_rules: string[]
  overall_risk: string
  evidence: unknown[]
  updated_at: string
}

export interface MCPAuthorizationPermission {
  catalog_release_id: string
  tool_revision_id: string
  require_user: boolean
  user_ids: string[]
  project_ids: string[]
  max_limit: number
}

export interface MCPAuthorizationPolicy {
  revision: string
  permissions: Record<string, Record<string, MCPAuthorizationPermission>>
  block_risk_at_least?: string
  rego_source?: string
}

export interface MCPAuthorizationPolicyStatus {
  policy_key: string
  status: 'empty' | 'active' | string
  version: number
  language_version?: string
  digest?: string
  created_by?: string
  created_at?: string
  policy?: MCPAuthorizationPolicy
}

export interface MCPAuthorizationPolicyValidation {
  valid: boolean
  revision: string
  language_version: string
  digest: string
}

export interface MCPPage<T> {
  items: T[]
  total: number
  page?: number
  page_size?: number
}

export interface MCPOnboardingPayload {
  display_name: string
  endpoint_url: string
  auth_type: string
  credential_ref?: string
  environment: string
  publish_policy: string
  owner_team_id?: string
  target_catalog_id?: string
}

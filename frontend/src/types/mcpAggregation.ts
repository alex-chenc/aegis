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
  tool_alias: string
  status: string
  policy_decision?: string
  rule_status?: string
  ai_status?: string
  created_at: string
}

export interface MCPSecurityVerdict {
  id: string
  invocation_id: string
  deterministic_severity: string
  ai_verdict?: string
  overall_risk: string
  evidence: unknown[]
  updated_at: string
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

export interface Host {
  id: string
  ip_address: string
  hostname: string
  os_type: string
  agent_version: string
  last_heartbeat_at: string
  online: boolean
  created_at: string
}

export interface Template {
  id: string
  name: string
  display_name: string
  file_type: string
  file_md5?: string
  status: 'parsing' | 'completed' | 'failed'
  error_message?: string
  rule_count: number
  created_at: string
  updated_at: string
}

export interface BaselineRule {
  id: string
  template_id: string
  title: string
  check_content: string
  fix_content: string
  generated_check_script?: string
  generated_fix_script?: string
  check_script_version: number
  fix_script_version: number
  check_script_status: 'pending' | 'generating' | 'generated' | 'failed'
  fix_script_status: 'pending' | 'generating' | 'generated' | 'failed'
  check_script_error?: string
  fix_script_error?: string
}

export interface ParseStatus {
  status: 'parsing' | 'completed' | 'failed'
  progress: number
  message: string
}

export interface InstallCommand {
  command: string
  server_ip: string
  http_port: number
  grpc_port: number
}

export interface LLMConfig {
  api_key_masked: string
  base_url: string
  model_name: string
  is_active: boolean
}

export interface Alert {
  id: string
  alert_id: string
  host_id: string
  hostname?: string
  pid: number
  mitre_id: string
  mitre_name?: string
  severity: 'Critical' | 'High' | 'Medium' | 'Low'
  description?: string
  llm_summary?: string
  hit_count: number
  auto_blocked: boolean
  manual_blocked: boolean
  status: 'active' | 'resolved'
  first_seen_at: string
  last_seen_at: string
  created_at: string
}

export interface BlockPolicy {
  id: string
  mitre_id: string
  mitre_name?: string
  enabled: boolean
  auto_block: boolean
  action: string
  updated_at: string
}

export interface SigmaRule {
  id: string
  rule_id: string
  title?: string
  description?: string
  content?: string
  status: 'pending' | 'experimental' | 'active' | 'disabled'
  mitre_id?: string
  severity?: string
  generated_by: string
  version: string
  created_at: string
  activated_at?: string
}

export interface ThreatStatistics {
  today_alerts: number
  today_blocks: number
  affected_hosts: number
  active_rules: number
}

export interface AlertTrendPoint {
  time_bucket: string
  count: number
}

export interface BlockRecord {
  id: string
  block_id: string
  alert_id?: string
  host_id: string
  action: string
  target?: string
  success: boolean
  message?: string
  issued_by: string
  created_at: string
}

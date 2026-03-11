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
  file_type: string
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
  script_status: 'pending' | 'generating' | 'generated' | 'failed'
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
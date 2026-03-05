export interface Host {
  id: string;
  ip_address: string;
  hostname: string;
  os_type: string;
  os_version: string;
  kernel_version: string;
  agent_version: string;
  architecture: string;
  cpu_info: CpuInfo | null;
  total_memory_mb: number;
  total_disk_gb: number;
  network_interfaces: NetworkInterface[] | null;
  cpu_load_1min: number;
  mem_usage_percent: number;
  last_heartbeat_at: string;
  is_online: boolean;
  created_at: string;
  updated_at: string;
}

export interface CpuInfo {
  model_name: string;
  cores: number;
  threads: number;
  frequency: number;
}

export interface NetworkInterface {
  name: string;
  mac_address: string;
  ip_addresses: string[];
  is_up: boolean;
}

export interface Template {
  id: string;
  name: string;
  file_type: string;
  minio_object_name: string;
  llm_prompt_template: string;
  baseline_rules: BaselineRule[];
  created_at: string;
  updated_at: string;
}

export interface BaselineRule {
  id: string;
  template_id: string;
  title: string;
  check_content: string;
  fix_content: string;
  generated_check_script: string;
  generated_fix_script: string;
  created_at: string;
  updated_at: string;
}

export interface TaskLog {
  id: string;
  rule_id: string;
  host_id: string;
  task_type: 'CHECK' | 'FIX';
  status: 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'TIMEOUT';
  stdout: string;
  stderr: string;
  exit_code: number;
  started_at: string;
  finished_at: string;
  created_at: string;
}

export interface Settings {
  llm_configured: boolean;
  llm_base_url: string;
  llm_model: string;
}

export interface ServerInfo {
  server_ip: string;
  server_address: string;
  grpc_port: string;
  http_port: string;
}

export interface PaginatedResponse<T> {
  total: number;
  items: T[];
}

export interface ApiResponse<T = any> {
  code: number;
  message: string;
  data: T;
}
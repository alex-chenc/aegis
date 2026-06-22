export interface WeakPasswordScope {
  host_ids: string[]
  host_group_ids: string[]
  application_types: string[]
  online_agents_only: boolean
}

export interface AnalyzeAssetApplicationsRequest {
  scope: WeakPasswordScope
}

export interface WeakPasswordCandidateApplication {
  candidate_application_id: string
  host_id: string
  asset_id?: string
  hostname?: string
  ip_address?: string
  application_name: string
  application_type: string
  application_version?: string
  profile_id?: string
  confidence: number
  candidate_paths: string[]
  credential_types: string[]
  ai_reason: string
  status: string
}

export interface AnalyzeAssetApplicationsResponse {
  analysis_id: string
  status: string
  application_asset_count: number
  candidate_count: number
  error_code?: string
  message?: string
  candidates: WeakPasswordCandidateApplication[]
}

export interface WeakPasswordDictionaryPolicy {
  use_default_1000: boolean
  dictionary_ids: string[]
  use_ai_generated: boolean
  hybrid: boolean
  fuzzy: boolean
}

export interface WeakPasswordAIPolicy {
  repair_collection_errors: boolean
  encrypted_password_llm_match: boolean
  max_agent_tool_calls_per_app: number
}

export interface CreateWeakPasswordTaskRequest {
  candidate_application_id: string
  dictionary_policy: WeakPasswordDictionaryPolicy
  ai_policy: WeakPasswordAIPolicy
}

export interface CreateWeakPasswordTaskResponse {
  task_id: string
  scan_application_id: string
  status: string
}

export interface WeakPasswordTask {
  id: string
  name: string
  trigger_source: string
  status: string
  progress: number
  current_stage: string
  total_hosts: number
  total_applications: number
  matched_findings: number
  failed_applications: number
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface WeakPasswordTaskProgress {
  task_id: string
  status: string
  progress: number
  current_stage: string
  current_host_id: string
  current_application: string
  agent_tool_call_count: number
  max_agent_tool_calls: number
  last_agent_tool: string
  last_error_code: string
  message: string
}

export interface WeakPasswordScanHost {
  id: string
  task_id: string
  host_id: string
  hostname?: string
  ip_address?: string
  status: string
  agent_status: string
  progress: number
  current_stage: string
  collected_records: number
  matched_findings: number
  failed_applications: number
  error_code: string
  error_message: string
}

export interface WeakPasswordFinding {
  id: string
  task_id: string
  scan_application_id?: string
  host_id: string
  asset_id?: string
  application_name: string
  application_type: string
  account: string
  credential_type: string
  match_status: string
  matched_password_mask: string
  match_source: string
  match_rule: string
  confidence: number
  source_path: string
  field_path: string
  ai_reason: string
  created_at: string
}

export interface RevealedWeakPasswordFinding {
  finding_id: string
  application_name: string
  account: string
  credential_type: string
  matched_password: string
  source_path: string
  field_path: string
}

export interface WeakPasswordCollectionError {
  id: string
  task_id: string
  scan_application_id?: string
  host_id: string
  application_name: string
  source_path: string
  error_code: string
  error_message: string
  agent_tool_call_count: number
  final_status: string
  created_at: string
}

export interface WeakPasswordDictionary {
  id: string
  name: string
  dictionary_type: string
  status: string
  entry_count: number
  source: string
  categories: string[]
  created_at: string
  updated_at: string
  sample_count?: number
}

export interface CreateWeakPasswordDictionaryRequest {
  name: string
  dictionary_type?: string
  entries: string[]
  categories: string[]
  source?: string
}

export interface AIGenerateDictionaryRequest {
  target: string
  application_type: string
  organization_keywords: string[]
  account_keywords: string[]
  count: number
  rules: string[]
  deduplicate_with_default: boolean
}

export interface PageResult<T> {
  items: T[]
  total: number
}

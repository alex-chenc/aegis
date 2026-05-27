import request from './index'

export interface DetectionPackageDraft {
  id: string
  package_id: string
  target_version: string
  title: string
  description?: string
  cve_ids: string[]
  ai_generated: boolean
  ai_generation_input?: Record<string, unknown>
  hook_plan_yaml: string
  ebpf_source: string
  sigma_rules_yaml: string
  correlation_yaml: string
  build_params: Record<string, unknown>
  status: 'draft' | 'build_pending' | 'build_running' | 'built' | 'signed'
}

export interface DetectionPackage {
  id: string
  package_id: string
  version: string
  title: string
  description?: string
  cve_ids: string[]
  status: 'draft' | 'build_failed' | 'built' | 'signed' | 'enabled' | 'active' | 'degraded' | 'disabled'
  package_object_key?: string
  signature_object_key?: string
  package_size?: number
  package_sha256?: string
  manifest_json: Record<string, unknown>
  hook_summary: PackageHook[]
  event_schema: Record<string, unknown>
  limits_json: Record<string, unknown>
  host_total?: number
  host_active?: number
  host_failed?: number
  created_at: string
  updated_at: string
}

export interface PackageHook {
  name: string
  attach_type: 'tracepoint' | 'kprobe' | 'lsm' | 'xdp' | 'tc'
  attach: string
  program: string
  allowed?: boolean
}

export interface PackageHostStatus {
  host_id: string
  hostname: string
  kernel_release?: string
  arch?: string
  status: 'pending' | 'downloading' | 'signature_failed' | 'blocked_by_hook_allowlist' | 'installing' | 'active' | 'degraded' | 'load_failed' | 'disabled_by_policy' | 'disabled_by_rate' | 'rolled_back' | 'uninstalled'
  active_artifact?: 'ringbuf' | 'perf'
  loaded_hooks: string[]
  error_message?: string
  last_reported_at?: string
}

export interface DetectionPackageBuild {
  id: string
  package_id: string
  version: string
  status: 'build_pending' | 'build_running' | 'build_failed' | 'built'
  error_message?: string
  builder_image_digest?: string
  clang_version?: string
  build_log_object_key?: string
  build_log_tail?: string
  artifacts: BuildArtifact[]
  hook_summary: PackageHook[]
  event_schema_json?: string
  unsigned_package_object_key?: string
  unsigned_package_sha256?: string
  unsigned_package_size?: number
  created_at: string
  updated_at: string
}

export interface BuildArtifact {
  name: string
  transport: 'perf' | 'ringbuf'
  object_key: string
  sha256: string
  size: number
}

export interface AIGenerateRequest {
  cve_id: string
  vulnerability_description: string
  attack_prerequisites?: string
  exploitation_chain?: string
  observable_syscalls?: string[]
  false_positive_constraints?: string
}

export interface CreateDraftRequest {
  package_id: string
  target_version: string
  title: string
  description?: string
  cve_ids: string[]
  hook_plan_yaml: string
  ebpf_source: string
  sigma_rules_yaml: string
  correlation_yaml: string
  build_params?: Record<string, unknown>
}

export interface UpdateDraftRequest {
  title?: string
  description?: string
  target_version?: string
  hook_plan_yaml?: string
  ebpf_source?: string
  sigma_rules_yaml?: string
  correlation_yaml?: string
  build_params?: Record<string, unknown>
}

export interface PageQuery {
  page?: number
  page_size?: number
  status?: string
  cve_id?: string
  search?: string
}

export const detectionPackageApi = {
  list: (params?: PageQuery): Promise<{ data: DetectionPackage[]; total: number }> =>
    request.get('/detection/packages', { params }),

  get: (packageId: string): Promise<DetectionPackage> =>
    request.get(`/detection/packages/${packageId}`),

  generateDraft: (data: AIGenerateRequest): Promise<DetectionPackageDraft> =>
    request.post('/detection/packages/ai-generate', data),

  createDraft: (data: CreateDraftRequest): Promise<DetectionPackageDraft> =>
    request.post('/detection/packages/drafts', data),

  updateDraft: (draftId: string, data: UpdateDraftRequest): Promise<DetectionPackageDraft> =>
    request.put(`/detection/packages/drafts/${draftId}`, data),

  build: (packageId: string): Promise<DetectionPackageBuild> =>
    request.post(`/detection/packages/${packageId}/build`),

  getBuild: (buildId: string): Promise<DetectionPackageBuild> =>
    request.get(`/detection/packages/builds/${buildId}`),

  sign: (packageId: string): Promise<DetectionPackage> =>
    request.post(`/detection/packages/${packageId}/sign`),

  enable: (packageId: string): Promise<void> =>
    request.post(`/detection/packages/${packageId}/enable`),

  disable: (packageId: string): Promise<void> =>
    request.post(`/detection/packages/${packageId}/disable`),

  uninstall: (packageId: string): Promise<void> =>
    request.post(`/detection/packages/${packageId}/uninstall`),

  hostStatus: (packageId: string, params?: PageQuery): Promise<{ data: PackageHostStatus[]; total: number }> =>
    request.get(`/detection/packages/${packageId}/hosts`, { params }),

  reviewBuild: (buildId: string, data: { approved: boolean; comment: string }): Promise<void> =>
    request.post(`/detection/packages/builds/${buildId}/review`, data),

  rollbackPackage: (packageId: string, targetVersion: string): Promise<void> =>
    request.post(`/detection/packages/${packageId}/rollback`, { target_version: targetVersion }),

  getPackageAlerts: (packageId: string): Promise<any[]> =>
    request.get(`/detection/packages/${packageId}/alerts`),

  getBuildLog: (buildId: string): Promise<{ log_url: string }> =>
    request.get(`/detection/packages/builds/${buildId}/log`),
}

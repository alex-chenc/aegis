import request from './index'

// 类型定义
export interface AssetSummary {
  software_count: number
  application_count: number
  database_count: number
  web_service_count: number
  web_framework_count: number
  web_site_count: number
  needs_review_count: number
  last_collection_at: string | null
}

export interface SoftwareAsset {
  id: string
  host_id: string
  hostname: string
  ip_address: string
  group_name: string
  os_type: string
  name: string
  version: string
  package_manager: string
  architecture: string
  install_paths: string[]
  last_modified_at: string | null
  collected_at: string
}

export interface ApplicationAsset {
  id: string
  host_id: string
  hostname: string
  ip_address: string
  group_name: string
  os_type: string
  name: string
  display_name: string
  category: string
  version: string
  listen_ports: number[]
  run_user: string
  start_path: string
  config_paths: string[]
  confidence: number
  ai_confidence?: number
  review_status: string
  status: string
  collected_at: string
}

export interface CollectionTask {
  id: string
  task_type: string
  trigger_source: string
  scope: string
  status: string
  total_hosts: number
  success_hosts: number
  failed_hosts: number
  current_stage: string
  error_message: string
  requested_by: string
  started_at: string | null
  finished_at: string | null
  created_at: string
}

export interface AssetCollectionConfig {
  id: string
  enabled: boolean
  interval_hours: number
  collect_types: string[]
  scope: string
  next_run_at: string | null
  last_run_at: string | null
}

export interface SoftwareAssetQuery {
  page?: number
  page_size?: number
  keyword?: string
  host_id?: string
  group_id?: string
  os_type?: string
  package_manager?: string
  status?: string
  start_time?: string
  end_time?: string
}

export interface ApplicationAssetQuery {
  page?: number
  page_size?: number
  keyword?: string
  host_id?: string
  group_id?: string
  category?: string
  min_confidence?: number
  review_status?: string
  status?: string
}

export interface CollectionTaskQuery {
  page?: number
  page_size?: number
  status?: string
}

export interface TriggerCollectionPayload {
  scope: string
  host_ids?: string[]
  types: string[]
  force?: boolean
}

export interface ApplicationReviewPayload {
  name?: string
  category?: string
  version?: string
  install_path?: string
  config_paths?: string[]
  review_status: string
}

export interface PageResult<T> {
  items: T[]
  total: number
}

// API 函数

/**
 * 获取资产概览
 */
export function getAssetSummary(): Promise<AssetSummary> {
  return request.get('/host-assets/summary')
}

/**
 * 列出软件资产
 */
export function listSoftwareAssets(params: SoftwareAssetQuery): Promise<PageResult<SoftwareAsset>> {
  return request.get('/host-assets/software', { params })
}

/**
 * 列出应用资产
 */
export function listApplicationAssets(params: ApplicationAssetQuery): Promise<PageResult<ApplicationAsset>> {
  return request.get('/host-assets/applications', { params })
}

/**
 * 获取应用详情
 */
export function getApplicationDetail(id: string): Promise<{ application: ApplicationAsset; tool_calls: any[] }> {
  return request.get(`/host-assets/applications/${id}`)
}

/**
 * 人工复核应用
 */
export function reviewApplication(id: string, payload: ApplicationReviewPayload): Promise<void> {
  return request.put(`/host-assets/applications/${id}/review`, payload)
}

/**
 * 触发资产采集
 */
export function triggerAssetCollection(payload: TriggerCollectionPayload): Promise<{ task_id: string; status: string }> {
  return request.post('/host-assets/collections', payload)
}

/**
 * 列出采集任务
 */
export function listCollectionTasks(params: CollectionTaskQuery): Promise<PageResult<CollectionTask>> {
  return request.get('/host-assets/collections', { params })
}

/**
 * 获取采集任务详情
 */
export function getCollectionTask(id: string): Promise<{ task: CollectionTask; hosts: any[] }> {
  return request.get(`/host-assets/collections/${id}`)
}

/**
 * 重试采集任务
 */
export function retryCollectionTask(id: string): Promise<void> {
  return request.post(`/host-assets/collections/${id}/retry`)
}

/**
 * 取消采集任务
 */
export function cancelCollectionTask(id: string): Promise<void> {
  return request.post(`/host-assets/collections/${id}/cancel`)
}

/**
 * 获取采集配置
 */
export function getCollectionConfig(): Promise<AssetCollectionConfig> {
  return request.get('/host-assets/collection-config')
}

/**
 * 更新采集配置
 */
export function updateCollectionConfig(payload: Partial<AssetCollectionConfig>): Promise<AssetCollectionConfig> {
  return request.put('/host-assets/collection-config', payload)
}

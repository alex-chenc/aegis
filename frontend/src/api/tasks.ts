import request from './index'

// ============================================
// 类型定义 - 支持基线检查和漏洞管理两种模式
// ============================================

// 任务类型（大写 - 后端返回格式）
export type TaskType = 'CHECK' | 'FIX' | 'POC_VERIFY' | 'VULNERABILITY_FIX'
// 任务类型（小写 - 兼容旧格式）
export type LegacyTaskType = 'check' | 'fix' | 'poc_verify' | 'vulnerability_fix'
// 任务状态（大写）
export type TaskStatus = 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'TIMEOUT'
// 任务状态（小写 - 兼容旧格式）
export type LegacyTaskStatus = 'pending' | 'running' | 'success' | 'failed' | 'timeout'

// 类型标准化辅助函数
export function normalizeType(type: string | undefined): string {
  return (type || '').toUpperCase()
}

export function normalizeStatus(status: string | undefined): string {
  return (status || '').toLowerCase()
}

// ============================================
// 接口定义
// ============================================

export interface RunCheckRequest {
  rule_ids: string[]
  host_ids: string[]
}

export interface RunFixRequest {
  rule_ids: string[]
  host_ids: string[]
  task_group_id?: string
}

export interface RunFixInGroupRequest {
  rule_ids: string[]
  host_ids: string[]
  task_group_id: string
}

export interface HealingStatus {
  task_id: string
  status: 'healing' | 'healed' | 'failed' | 'timeout'
  started_at?: string
  total_attempts: number
  max_attempts: number
  last_error?: string
  user_suggestion?: string
  script_type?: string
}

export interface TriggerHealingRequest {
  user_suggestion?: string
}

export interface RunTaskResponse {
  task_group_id: string
  task_ids: string[]
  task_count: number
}

export interface RedispatchTaskResponse {
  task_id: string
  task_group_id: string
}

export interface TaskGroupStatus {
  task_group_id: string
  status: string
  total: number
  pending: number
  running: number
  success: number
  failed: number
  timeout?: number
}

export interface TaskLog {
  id: string
  task_group_id: string
  rule_id: string
  host_id: string
  vulnerability_id?: string
  rule_title?: string
  hostname?: string
  task_type: TaskType | LegacyTaskType | string
  status: TaskStatus | LegacyTaskStatus | string
  script_content?: string
  stdout?: string
  stderr?: string
  exit_code?: number
  started_at?: string
  finished_at?: string
  healing_status?: HealingStatus
  created_at?: string
  audit_info?: {
    hit_rules: { rule_name: string; severity: string; line_number: number }[]
    error_message?: string
    audit_log_id?: string
  }
}

export interface TaskGroupSummary {
  task_group_id: string
  task_count: number
  task_type: TaskType | LegacyTaskType | string
  has_check?: number
  has_fix?: number
  status: TaskStatus | LegacyTaskStatus | string
  success_count: number
  failed_count: number
  pending_count: number
  running_count: number
  timeout_count?: number
  created_at: string
  finished_at?: string
}

export interface ListTasksParams {
  page?: number
  page_size?: number
  status?: string
  task_type?: string
  search?: string
}

export interface ListTasksResponse {
  items: TaskGroupSummary[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

// ============================================
// API 函数
// ============================================

export function runCheck(data: RunCheckRequest) {
  return request<any, RunTaskResponse>({
    url: '/tasks/run-check',
    method: 'post',
    data
  })
}

export function runFix(data: RunFixRequest) {
  return request<any, RunTaskResponse>({
    url: '/tasks/run-fix',
    method: 'post',
    data
  })
}

export function runFixInGroup(data: RunFixInGroupRequest) {
  return request<any, RunTaskResponse>({
    url: '/tasks/run-fix',
    method: 'post',
    data
  })
}

export function getTaskLogs(taskGroupId: string) {
  return request<any, TaskLog[]>({
    url: `/tasks/${taskGroupId}/logs`,
    method: 'get'
  })
}

export function getTaskStatus(taskGroupId: string) {
  return request<any, TaskGroupStatus>({
    url: `/tasks/${taskGroupId}/status`,
    method: 'get'
  })
}

export function getTaskDetail(taskId: string) {
  return request<any, TaskLog>({
    url: `/tasks/${taskId}`,
    method: 'get'
  })
}

export function redispatchTask(taskId: string) {
  return request<any, RedispatchTaskResponse>({
    url: `/tasks/${taskId}/redispatch`,
    method: 'post'
  })
}

export function triggerSelfHealing(taskId: string, userSuggestion?: string) {
  return request<any, { code: number; message: string; data: { task_id: string; rule_id: string; script_type: string; status: string } }>({
    url: `/tasks/${taskId}/heal`,
    method: 'post',
    data: { user_suggestion: userSuggestion }
  })
}

export function deleteTask(taskId: string) {
  return request<any, { code: number; message: string }>({
    url: `/tasks/${taskId}`,
    method: 'delete'
  })
}

export function deleteTaskGroup(taskGroupId: string) {
  return request<any, { code: number; message: string; data?: { deleted_count: number } }>({
    url: `/tasks/group/${taskGroupId}`,
    method: 'delete'
  })
}

export function getHealingStatus(taskId: string) {
  return request<any, HealingStatus | null>({
    url: `/tasks/${taskId}/healing-status`,
    method: 'get'
  })
}

export function listTasks(params: ListTasksParams) {
  return request<any, ListTasksResponse>({
    url: '/tasks',
    method: 'get',
    params
  })
}

export function batchDeleteTasks(taskGroupIds: string[]) {
  return request<any, { deleted_count: number; skipped_count: number }>({
    url: '/tasks/batch',
    method: 'delete',
    data: { task_ids: taskGroupIds }
  })
}

// ============================================
// 兼容性别名（用于迁移）
// ============================================

export const getTasks = listTasks
export const getTaskGroupLogs = getTaskLogs
export const getTaskById = getTaskDetail
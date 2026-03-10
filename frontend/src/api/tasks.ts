import request from './index'

export interface RunCheckRequest {
  rule_ids: string[]
  host_ids: string[]
}

export interface RunFixRequest {
  rule_ids: string[]
  host_ids: string[]
}

export interface TaskLog {
  id: string
  task_group_id: string
  rule_id: string
  host_id: string
  rule_title?: string
  hostname?: string
  task_type: 'check' | 'fix'
  status: 'pending' | 'running' | 'success' | 'failed' | 'healing'
  script_content?: string
  stdout?: string
  stderr?: string
  exit_code?: number
  started_at?: string
  finished_at?: string
  healing_status?: HealingStatus
}

export interface HealingStatus {
  id: string
  original_task_id: string
  rule_id: string
  script_type: string
  status: 'healing' | 'healed' | 'failed'
  total_attempts: number
  max_attempts: number
  final_script_version?: string
  last_error?: string
  user_suggestion?: string
}

export interface TriggerHealingRequest {
  user_suggestion?: string
}

export interface RunTaskResponse {
  task_group_id: string
  task_ids: string[]
  task_count: number
}

export interface TaskGroupStatus {
  task_group_id: string
  status: string
  total: number
  pending: number
  running: number
  success: number
  failed: number
}

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

export function getHealingStatus(taskId: string) {
  return request<any, HealingStatus | null>({
    url: `/tasks/${taskId}/healing-status`,
    method: 'get'
  })
}

export interface TaskGroupSummary {
  task_group_id: string
  task_count: number
  task_type: 'check' | 'fix'
  status: 'pending' | 'running' | 'success' | 'failed' | 'partial'
  success_count: number
  failed_count: number
  pending_count: number
  running_count: number
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

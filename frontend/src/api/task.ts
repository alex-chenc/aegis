import request from './index'

export interface Task {
  id: string
  task_group_id: string
  rule_id: string | null
  host_id: string
  vulnerability_id: string | null
  task_type: string
  status: 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'TIMEOUT'
  script_content: string | null
  script_version: number | null
  stdout: string | null
  stderr: string | null
  exit_code: number | null
  started_at: string | null
  finished_at: string | null
  created_at: string
}

export interface TaskListParams {
  page?: number
  page_size?: number
  status?: string
  task_type?: string
  host_id?: string
}

export interface TaskListResponse {
  items: TaskGroup[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface TaskGroup {
  task_group_id: string
  task_count: number
  task_type: string
  has_check: number
  has_fix: number
  status: string
  success_count: number
  failed_count: number
  pending_count: number
  running_count: number
  created_at: string
  finished_at: string | null
}

export function getTasks(params: TaskListParams): Promise<TaskListResponse> {
  return request.get('/tasks', { params })
}

export function getTaskGroupLogs(taskGroupId: string): Promise<Task[]> {
  return request.get(`/tasks/${taskGroupId}/logs`)
}

export function getTaskById(taskId: string): Promise<Task> {
  return request.get(`/tasks/${taskId}`)
}

export function retryTask(taskId: string): Promise<{ task_id: string }> {
  return request.post(`/tasks/${taskId}/retry`)
}
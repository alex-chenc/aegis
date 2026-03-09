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
  task_type: 'check' | 'fix'
  status: 'pending' | 'running' | 'success' | 'failed'
  script_content: string
  stdout: string
  stderr: string
  exit_code: number
  started_at: string
  finished_at: string
}

export interface RunTaskResponse {
  task_group_id: string
  task_ids: string[]
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
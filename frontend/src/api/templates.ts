import request from './index'
import type { Template, BaselineRule, ParseStatus } from '@/types'

export { type Template, type BaselineRule, type ParseStatus }

export function uploadTemplate(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return request<any, { template_id: string }>({
    url: '/templates/upload',
    method: 'post',
    data: formData,
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}

export function getTemplates(params?: { page?: number; pageSize?: number }) {
  return request<any, Template[]>({
    url: '/templates',
    method: 'get',
    params
  })
}

export function getTemplateStatus(id: string): Promise<ParseStatus> {
  return request<any, ParseStatus>({
    url: `/templates/${id}/status`,
    method: 'get'
  })
}

export function getTemplateRules(id: string): Promise<BaselineRule[]> {
  return request<any, BaselineRule[]>({
    url: `/templates/${id}/rules`,
    method: 'get'
  })
}

export function deleteTemplate(id: string) {
  return request<any, void>({
    url: `/templates/${id}`,
    method: 'delete'
  })
}

export interface GenerateScriptResponse {
  rule_id: string
  script_type: string
  script_content?: string
  version?: number
  status?: string
}

export interface HasTasksResponse {
  has_tasks: boolean
  task_count: number
}

export function generateScript(ruleId: string, scriptType: 'CHECK' | 'FIX') {
  return request<any, GenerateScriptResponse>({
    url: `/rules/${ruleId}/scripts/generate`,
    method: 'post',
    data: { script_type: scriptType }
  })
}

export function updateScript(ruleId: string, scriptType: 'CHECK' | 'FIX', scriptContent: string) {
  return request<any, { code: number; message: string }>({
    url: `/rules/${ruleId}/scripts`,
    method: 'put',
    data: { script_type: scriptType, script_content: scriptContent }
  })
}

export function getRuleScript(ruleId: string, scriptType: 'CHECK' | 'FIX') {
  return request<any, { rule_id: string; script_type: string; script_content: string; version: number }>({
    url: `/rules/${ruleId}`,
    method: 'get',
    params: { script_type: scriptType }
  })
}

export function hasRuleTasks(ruleId: string) {
  return request<any, HasTasksResponse>({
    url: `/rules/${ruleId}/has-tasks`,
    method: 'get'
  })
}

export function deleteRule(ruleId: string) {
  return request<any, { code: number; message: string }>({
    url: `/rules/${ruleId}`,
    method: 'delete'
  })
}
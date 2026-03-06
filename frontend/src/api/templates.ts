import request from './index'

export interface Template {
  id: string
  name: string
  file_type: string
  status: string
  rule_count: number
  created_at: string
}

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

export function getTemplateStatus(id: string) {
  return request<any, { status: string; progress: number; message: string }>({
    url: `/templates/${id}/status`,
    method: 'get'
  })
}

export function getTemplateRules(id: string) {
  return request<any, any[]>({
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
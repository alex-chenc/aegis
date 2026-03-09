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
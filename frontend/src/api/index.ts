import request from './request';
import type { Template, BaselineRule, PaginatedResponse, TaskLog } from '../types';

export function getTemplates(page: number = 1, pageSize: number = 10): Promise<PaginatedResponse<Template>> {
  return request({
    url: '/templates',
    method: 'get',
    params: { page, page_size: pageSize },
  });
}

export function getTemplate(id: string): Promise<Template> {
  return request({
    url: `/templates/${id}`,
    method: 'get',
  });
}

export function createTemplate(data: { name: string; file_type: string; minio_object_name: string }): Promise<Template> {
  return request({
    url: '/templates',
    method: 'post',
    data,
  });
}

export function deleteTemplate(id: string): Promise<void> {
  return request({
    url: `/templates/${id}`,
    method: 'delete',
  });
}

export function parseTemplate(id: string, content: string): Promise<{ template_id: string; status: string }> {
  return request({
    url: `/templates/${id}/parse`,
    method: 'post',
    data: { content },
  });
}

export function getTemplateRules(id: string): Promise<PaginatedResponse<BaselineRule>> {
  return request({
    url: `/templates/${id}/rules`,
    method: 'get',
  });
}

export function uploadTemplate(file: File): Promise<{ template_id: string; status: string }> {
  const formData = new FormData();
  formData.append('file', file);
  return request({
    url: '/templates/upload',
    method: 'post',
    data: formData,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
}

export function getTasks(params?: { page?: number; page_size?: number; host_id?: string; rule_id?: string }): Promise<PaginatedResponse<TaskLog>> {
  return request({
    url: '/tasks',
    method: 'get',
    params,
  });
}

export function getTask(id: string): Promise<TaskLog> {
  return request({
    url: `/tasks/${id}`,
    method: 'get',
  });
}

export function executeCheck(ruleId: string, hostIds: string[]): Promise<{ tasks: any[]; errors: string[] }> {
  return request({
    url: '/tasks/check',
    method: 'post',
    data: { rule_id: ruleId, host_ids: hostIds },
  });
}

export function executeFix(ruleId: string, hostIds: string[]): Promise<{ tasks: any[]; errors: string[] }> {
  return request({
    url: '/tasks/fix',
    method: 'post',
    data: { rule_id: ruleId, host_ids: hostIds },
  });
}
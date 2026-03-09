import request from './index'
import type { Host } from '@/types'

export { type Host }

export function getHosts(params?: { page?: number; pageSize?: number; query?: string }) {
  return request<any, Host[]>({
    url: '/hosts',
    method: 'get',
    params
  })
}

export function getHost(id: string) {
  return request<any, Host>({
    url: `/hosts/${id}`,
    method: 'get'
  })
}
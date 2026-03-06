import request from './index'

export interface Host {
  id: string
  ip_address: string
  hostname: string
  os_type: string
  agent_version: string
  last_heartbeat_at: string
  online: boolean
}

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
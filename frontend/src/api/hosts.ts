import request from './request';
import type { Host, PaginatedResponse, Settings, ServerInfo } from '../types';

export function getHosts(params: { page: number; pageSize: number; search?: string }): Promise<PaginatedResponse<Host>> {
  return request({
    url: '/hosts',
    method: 'get',
    params,
  });
}

export function getHostDetails(id: string): Promise<Host> {
  return request({
    url: `/hosts/${id}`,
    method: 'get',
  });
}

export function sendCommand(id: string, command: string, timeout?: number): Promise<{ command_id: string; status: string }> {
  return request({
    url: `/hosts/${id}/command`,
    method: 'post',
    data: { command, timeout },
  });
}

export function getSettings(): Promise<Settings> {
  return request({
    url: '/settings',
    method: 'get',
  });
}

export function getServerInfo(): Promise<ServerInfo> {
  return request({
    url: '/settings/server-info',
    method: 'get',
  });
}

export function testLLMConnection(): Promise<{ connected: boolean; error?: string }> {
  return request({
    url: '/settings/llm/test',
    method: 'post',
  });
}

export function getInstallCommand(serverIP?: string): Promise<{ install_command: string; server_address: string }> {
  return request({
    url: '/settings/install-command',
    method: 'get',
    params: serverIP ? { server_ip: serverIP } : undefined,
  });
}
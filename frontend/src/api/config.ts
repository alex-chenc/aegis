import request from './index'

export interface LLMConfig {
  api_key_masked: string
  base_url: string
  model_name: string
  is_active: boolean
}

export function getLLMConfig() {
  return request<any, LLMConfig>({
    url: '/config/llm',
    method: 'get'
  })
}

export function saveLLMConfig(data: { api_key: string; base_url: string; model_name: string }) {
  return request<any, void>({
    url: '/config/llm',
    method: 'post',
    data
  })
}

export function testLLMConnection(data: { api_key: string; base_url: string; model_name: string }) {
  return request<any, void>({
    url: '/config/llm/test',
    method: 'post',
    data
  })
}

export function getInstallCommand() {
  return request<any, { command: string; server_ip: string; http_port: number; grpc_port: number }>({
    url: '/agent/install-command',
    method: 'get'
  })
}
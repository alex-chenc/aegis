import request from './index'
import type { LLMConfig, InstallCommand } from '@/types'

export function getLLMConfig(): Promise<LLMConfig> {
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

export function getInstallCommand(): Promise<InstallCommand> {
  return request<any, InstallCommand>({
    url: '/agent/install-command',
    method: 'get'
  })
}
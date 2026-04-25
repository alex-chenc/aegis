import { defineStore } from 'pinia'
import { getLLMConfig, saveLLMConfig, testLLMConnection, getInstallCommand, getFullAPIKey, getImageModelConfig, saveImageModelConfig, testImageModelConnection, getFullImageModelAPIKey } from '@/api/config'

interface ConfigState {
  llmConfig: {
    api_key_masked: string
    provider: string
    base_url: string
    model_name: string
    is_active: boolean
  } | null
  imageModelConfig: {
    api_key_masked: string
    provider: string
    base_url: string
    model_name: string
    is_active: boolean
  } | null
  installCommand: {
    command: string
    server_ip: string
    http_port: number
    grpc_port: number
  } | null
  loading: boolean
}

export const useConfigStore = defineStore('config', {
  state: (): ConfigState => ({
    llmConfig: null,
    imageModelConfig: null,
    installCommand: null,
    loading: false
  }),

  actions: {
    async fetchLLMConfig() {
      this.loading = true
      try {
        this.llmConfig = await getLLMConfig()
      } finally {
        this.loading = false
      }
    },

    async saveLLMConfig(apiKey: string, provider: string, baseURL: string, modelName: string) {
      await saveLLMConfig({ api_key: apiKey, provider, base_url: baseURL, model_name: modelName })
      await this.fetchLLMConfig()
    },

    async testConnection(apiKey: string, provider: string, baseURL: string, modelName: string) {
      await testLLMConnection({ api_key: apiKey, provider, base_url: baseURL, model_name: modelName })
    },

    async fetchImageModelConfig() {
      this.loading = true
      try {
        this.imageModelConfig = await getImageModelConfig()
      } finally {
        this.loading = false
      }
    },

    async saveImageModelConfig(apiKey: string, provider: string, baseURL: string, modelName: string) {
      await saveImageModelConfig({ api_key: apiKey, provider, base_url: baseURL, model_name: modelName })
      await this.fetchImageModelConfig()
    },

    async testImageModelConnection(apiKey: string, provider: string, baseURL: string, modelName: string) {
      await testImageModelConnection({ api_key: apiKey, provider, base_url: baseURL, model_name: modelName })
    },

    async fetchInstallCommand() {
      this.installCommand = await getInstallCommand()
    },

    async fetchFullAPIKey(): Promise<string> {
      const response = await getFullAPIKey()
      return response.api_key
    },

    async fetchFullImageModelAPIKey(): Promise<string> {
      const response = await getFullImageModelAPIKey()
      return response.api_key
    }
  }
})

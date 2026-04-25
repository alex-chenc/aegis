<template>
  <div class="settings">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>LLM 配置</span>
          <el-tag type="info" size="small">仅通过页面配置，不读取配置文件</el-tag>
        </div>
      </template>
      
      <el-alert
        title="配置说明"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 20px"
      >
        <p>LLM 配置仅通过本页面进行设置，配置文件中的相关配置已禁用。</p>
        <p>选择厂商后会自动填入推荐 Base URL 和模型名称；仍可按实际服务手动调整。</p>
      </el-alert>
      
      <el-form :model="form" label-width="120px">
        <el-form-item label="模型厂商">
          <el-radio-group v-model="form.provider" class="provider-selector" @change="handleProviderChange">
            <el-radio-button
              v-for="provider in providerOptions"
              :key="provider.value"
              :label="provider.value"
            >
              {{ provider.label }}
            </el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="API Key">
          <el-input 
            v-model="form.api_key" 
            :type="apiKeyVisible ? 'text' : 'password'" 
            placeholder="请输入 API Key（如显示 **** 表示已保存，可直接输入新值）"
          >
            <template #suffix>
              <el-icon @click="toggleApiKeyVisibility" class="cursor-pointer">
                <View v-if="apiKeyVisible" />
                <Hide v-else />
              </el-icon>
            </template>
          </el-input>
        </el-form-item>
        <el-form-item label="Base URL">
          <el-input v-model="form.base_url" :placeholder="selectedProvider?.baseURL || 'https://api.example.com/v1'" />
        </el-form-item>
        <el-form-item label="模型名称">
          <el-input v-model="form.model_name" :placeholder="selectedProvider?.modelName || 'model-name'" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveConfig" :loading="saving">保存配置</el-button>
          <el-button @click="testConnection" :loading="testing">测试连接</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card style="margin-top: 20px">
      <template #header>
        <div class="card-header">
          <span>图片模型配置</span>
          <el-tag type="success" size="small">用于流程图与报告图生成</el-tag>
        </div>
      </template>

      <el-alert
        title="图片模型说明"
        type="success"
        show-icon
        :closable="false"
        style="margin-bottom: 20px"
      >
        <p>图片模型配置独立于文本 LLM。MiniMax Token Plan 可使用 image-01 接口。</p>
      </el-alert>

      <el-form :model="imageForm" label-width="120px">
        <el-form-item label="图片厂商">
          <el-radio-group v-model="imageForm.provider" class="provider-selector" @change="handleImageProviderChange">
            <el-radio-button
              v-for="provider in imageProviderOptions"
              :key="provider.value"
              :label="provider.value"
            >
              {{ provider.label }}
            </el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="API Key">
          <el-input
            v-model="imageForm.api_key"
            :type="imageApiKeyVisible ? 'text' : 'password'"
            placeholder="请输入图片模型 API Key"
          >
            <template #suffix>
              <el-icon @click="toggleImageApiKeyVisibility" class="cursor-pointer">
                <View v-if="imageApiKeyVisible" />
                <Hide v-else />
              </el-icon>
            </template>
          </el-input>
        </el-form-item>
        <el-form-item label="Base URL">
          <el-input v-model="imageForm.base_url" :placeholder="selectedImageProvider?.baseURL || 'https://api.minimax.io/v1'" />
        </el-form-item>
        <el-form-item label="图片模型">
          <el-input v-model="imageForm.model_name" :placeholder="selectedImageProvider?.modelName || 'image-01'" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveImageConfig" :loading="imageSaving">保存图片配置</el-button>
          <el-button @click="testImageConnection" :loading="imageTesting">测试图片连接</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card style="margin-top: 20px">
      <template #header>
        <div class="card-header">
          <span>Agent 安装命令</span>
          <el-button link @click="fetchInstallCmd">刷新</el-button>
        </div>
      </template>
      
      <el-input v-model="installCommand" readonly>
        <template #append>
          <el-button @click="copyCommand" :disabled="!installCommand">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
        </template>
      </el-input>
      
      <div style="margin-top: 12px; color: #666; font-size: 13px">
        <p>服务器地址: {{ installInfo?.server_ip }}:{{ installInfo?.http_port }}</p>
        <p>gRPC端口: {{ installInfo?.grpc_port }}</p>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, View, Hide } from '@element-plus/icons-vue'
import { useConfigStore } from '@/store/config'
import { getInstallCommand } from '@/api/config'
import { getFullAPIKey, getFullImageModelAPIKey } from '@/api/config'
import type { InstallCommand } from '@/types'

const configStore = useConfigStore()
const providerOptions = [
  {
    value: 'deepseek',
    label: 'DeepSeek',
    baseURL: 'https://api.deepseek.com/v1',
    modelName: 'deepseek-chat'
  },
  {
    value: 'dashscope',
    label: '阿里云百炼',
    baseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    modelName: 'qwen-plus'
  },
  {
    value: 'minimax',
    label: 'MiniMax',
    baseURL: 'https://api.minimaxi.com/anthropic',
    modelName: 'MiniMax-M2.7'
  },
  {
    value: 'openai',
    label: 'OpenAI',
    baseURL: 'https://api.openai.com/v1',
    modelName: 'gpt-4o-mini'
  },
  {
    value: 'custom',
    label: '自定义',
    baseURL: '',
    modelName: ''
  }
]

const imageProviderOptions = [
  {
    value: 'minimax',
    label: 'MiniMax',
    baseURL: 'https://api.minimax.io/v1',
    modelName: 'image-01'
  },
  {
    value: 'zhipu',
    label: '智谱',
    baseURL: 'https://open.bigmodel.cn/api/paas/v4',
    modelName: 'cogview-3-flash'
  },
  {
    value: 'custom',
    label: '自定义',
    baseURL: '',
    modelName: ''
  }
]

const form = ref({
  api_key: '',
  provider: 'deepseek',
  base_url: 'https://api.deepseek.com/v1',
  model_name: 'deepseek-chat'
})
const imageForm = ref({
  api_key: '',
  provider: 'zhipu',
  base_url: 'https://open.bigmodel.cn/api/paas/v4',
  model_name: 'cogview-3-flash'
})
const selectedProvider = computed(() => providerOptions.find((item) => item.value === form.value.provider))
const selectedImageProvider = computed(() => imageProviderOptions.find((item) => item.value === imageForm.value.provider))
const installCommand = ref('')
const installInfo = ref<InstallCommand | null>(null)
const saving = ref(false)
const testing = ref(false)
const imageSaving = ref(false)
const imageTesting = ref(false)
const apiKeyVisible = ref(false)
const imageApiKeyVisible = ref(false)
const originalApiKey = ref('')
const originalImageApiKey = ref('')

const inferProvider = (baseURL: string) => {
  const url = baseURL.toLowerCase()
  if (url.includes('deepseek')) return 'deepseek'
  if (url.includes('dashscope') || url.includes('aliyuncs')) return 'dashscope'
  if (url.includes('minimaxi') || url.includes('minimax')) return 'minimax'
  if (url.includes('openai')) return 'openai'
  return 'custom'
}

const inferImageProvider = (baseURL: string) => {
  const url = baseURL.toLowerCase()
  if (url.includes('minimaxi') || url.includes('minimax')) return 'minimax'
  if (url.includes('bigmodel') || url.includes('zhipu')) return 'zhipu'
  return 'custom'
}

const handleProviderChange = () => {
  const provider = selectedProvider.value
  if (!provider || provider.value === 'custom') return

  form.value.base_url = provider.baseURL
  form.value.model_name = provider.modelName
}

const handleImageProviderChange = () => {
  const provider = selectedImageProvider.value
  if (!provider || provider.value === 'custom') return

  imageForm.value.base_url = provider.baseURL
  imageForm.value.model_name = provider.modelName
}

const saveConfig = async () => {
  // 检查是否为 masked API Key
  if (form.value.api_key.includes('****')) {
    ElMessage.warning('请输入完整的 API Key（当前显示的是脱敏后的值）')
    return
  }

  if (!form.value.api_key) {
    ElMessage.warning('请输入 API Key')
    return
  }

  saving.value = true
  try {
    await configStore.saveLLMConfig(form.value.api_key, form.value.provider, form.value.base_url, form.value.model_name)
    originalApiKey.value = form.value.api_key
    ElMessage.success('配置保存成功')
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const testConnection = async () => {
  if (!form.value.api_key) {
    ElMessage.warning('请输入 API Key')
    return
  }

  testing.value = true
  try {
    let apiKeyToUse = form.value.api_key
    
    // 如果是 masked key，从后端获取真正的 API Key
    if (apiKeyToUse.includes('****')) {
      const data = await getFullAPIKey()
      apiKeyToUse = data.api_key
      originalApiKey.value = apiKeyToUse  // 缓存真正的 key
    }
    
    await configStore.testConnection(apiKeyToUse, form.value.provider, form.value.base_url, form.value.model_name)
    ElMessage.success('连接测试成功')
  } catch (e: any) {
    ElMessage.error(e.message || '连接测试失败')
  } finally {
    testing.value = false
  }
}

const saveImageConfig = async () => {
  if (imageForm.value.api_key.includes('****')) {
    ElMessage.warning('请输入完整的图片模型 API Key')
    return
  }

  if (!imageForm.value.api_key) {
    ElMessage.warning('请输入图片模型 API Key')
    return
  }

  imageSaving.value = true
  try {
    await configStore.saveImageModelConfig(
      imageForm.value.api_key,
      imageForm.value.provider,
      imageForm.value.base_url,
      imageForm.value.model_name
    )
    originalImageApiKey.value = imageForm.value.api_key
    ElMessage.success('图片模型配置保存成功')
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    imageSaving.value = false
  }
}

const testImageConnection = async () => {
  if (!imageForm.value.api_key) {
    ElMessage.warning('请输入图片模型 API Key')
    return
  }

  imageTesting.value = true
  try {
    let apiKeyToUse = imageForm.value.api_key
    if (apiKeyToUse.includes('****')) {
      const data = await getFullImageModelAPIKey()
      apiKeyToUse = data.api_key
      originalImageApiKey.value = apiKeyToUse
    }

    await configStore.testImageModelConnection(
      apiKeyToUse,
      imageForm.value.provider,
      imageForm.value.base_url,
      imageForm.value.model_name
    )
    ElMessage.success('图片模型连接测试成功')
  } catch (e: any) {
    ElMessage.error(e.message || '图片模型连接测试失败')
  } finally {
    imageTesting.value = false
  }
}

const toggleApiKeyVisibility = async () => {
  if (!apiKeyVisible.value && !originalApiKey.value) {
    try {
      const data = await getFullAPIKey()
      originalApiKey.value = data.api_key
      form.value.api_key = originalApiKey.value
    } catch (e: any) {
      ElMessage.error('获取API Key失败')
      return
    }
  } else if (!apiKeyVisible.value) {
    form.value.api_key = originalApiKey.value
  } else {
    form.value.api_key = configStore.llmConfig?.api_key_masked || ''
  }
  apiKeyVisible.value = !apiKeyVisible.value
}

const toggleImageApiKeyVisibility = async () => {
  if (!imageApiKeyVisible.value && !originalImageApiKey.value) {
    try {
      const data = await getFullImageModelAPIKey()
      originalImageApiKey.value = data.api_key
      imageForm.value.api_key = originalImageApiKey.value
    } catch (e: any) {
      ElMessage.error('获取图片模型 API Key 失败')
      return
    }
  } else if (!imageApiKeyVisible.value) {
    imageForm.value.api_key = originalImageApiKey.value
  } else {
    imageForm.value.api_key = configStore.imageModelConfig?.api_key_masked || ''
  }
  imageApiKeyVisible.value = !imageApiKeyVisible.value
}

const copyCommand = async () => {
  if (!installCommand.value) {
    ElMessage.warning('没有可复制的内容')
    return
  }
  
  const success = await copyToClipboard(installCommand.value)
  if (success) {
    ElMessage.success('已复制到剪贴板')
  } else {
    ElMessage.error('复制失败，请手动复制')
  }
}

const copyToClipboard = async (text: string): Promise<boolean> => {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch (err) {
      console.warn('Clipboard API failed:', err)
    }
  }
  
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  
  try {
    const success = document.execCommand('copy')
    document.body.removeChild(textarea)
    return success
  } catch {
    document.body.removeChild(textarea)
    return false
  }
}

const fetchInstallCmd = async () => {
  try {
    const data = await getInstallCommand()
    installInfo.value = data
    installCommand.value = data.command
  } catch (e: any) {
    ElMessage.error(e.message || '获取安装命令失败')
  }
}

onMounted(async () => {
  try {
    await configStore.fetchLLMConfig()
    if (configStore.llmConfig) {
      form.value.provider = configStore.llmConfig.provider || inferProvider(configStore.llmConfig.base_url)
      form.value.base_url = configStore.llmConfig.base_url
      form.value.model_name = configStore.llmConfig.model_name
      form.value.api_key = configStore.llmConfig.api_key_masked || ''
    }
  } catch {
    // ignore
  }

  try {
    await configStore.fetchImageModelConfig()
    if (configStore.imageModelConfig) {
      imageForm.value.provider = configStore.imageModelConfig.provider || inferImageProvider(configStore.imageModelConfig.base_url)
      imageForm.value.base_url = configStore.imageModelConfig.base_url
      imageForm.value.model_name = configStore.imageModelConfig.model_name
      imageForm.value.api_key = configStore.imageModelConfig.api_key_masked || ''
    }
  } catch {
    // ignore
  }
  
  await fetchInstallCmd()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.cursor-pointer {
  cursor: pointer;
}

.provider-selector {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.provider-selector :deep(.el-radio-button__inner) {
  border-radius: 6px;
  min-width: 92px;
}
</style>

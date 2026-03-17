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
        <p>推荐使用 DeepSeek：Base URL 为 <code>https://api.deepseek.com/v1</code>，模型为 <code>deepseek-chat</code></p>
        <p>或使用阿里云百炼：Base URL 为 <code>https://dashscope.aliyuncs.com/compatible-mode/v1</code></p>
      </el-alert>
      
      <el-form :model="form" label-width="120px">
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
          <el-input v-model="form.base_url" placeholder="https://dashscope.aliyuncs.com/compatible-mode/v1" />
        </el-form-item>
        <el-form-item label="模型名称">
          <el-input v-model="form.model_name" placeholder="qwen-plus" />
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
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, View, Hide } from '@element-plus/icons-vue'
import { useConfigStore } from '@/store/config'
import { getInstallCommand } from '@/api/config'
import { getFullAPIKey } from '@/api/config'
import type { InstallCommand } from '@/types'

const configStore = useConfigStore()
const form = ref({
  api_key: '',
  base_url: 'https://api.deepseek.com/v1',
  model_name: 'deepseek-chat'
})
const installCommand = ref('')
const installInfo = ref<InstallCommand | null>(null)
const saving = ref(false)
const testing = ref(false)
const apiKeyVisible = ref(false)
const originalApiKey = ref('')

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
    await configStore.saveLLMConfig(form.value.api_key, form.value.base_url, form.value.model_name)
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
    
    await configStore.testConnection(apiKeyToUse, form.value.base_url, form.value.model_name)
    ElMessage.success('连接测试成功')
  } catch (e: any) {
    ElMessage.error(e.message || '连接测试失败')
  } finally {
    testing.value = false
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
      form.value.base_url = configStore.llmConfig.base_url
      form.value.model_name = configStore.llmConfig.model_name
      form.value.api_key = configStore.llmConfig.api_key_masked || ''
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
</style>

<template>
  <div class="settings">
    <el-card>
      <template #header>
        <span>LLM 配置</span>
      </template>
      
      <el-form :model="form" label-width="120px">
        <el-form-item label="API Key">
          <el-input v-model="form.api_key" type="password" placeholder="请输入 API Key" show-password />
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
import { CopyDocument } from '@element-plus/icons-vue'
import { useConfigStore } from '@/store/config'
import { getInstallCommand } from '@/api/config'
import type { InstallCommand } from '@/types'

const configStore = useConfigStore()
const form = ref({
  api_key: '',
  base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  model_name: 'qwen-plus'
})
const installCommand = ref('')
const installInfo = ref<InstallCommand | null>(null)
const saving = ref(false)
const testing = ref(false)

const saveConfig = async () => {
  saving.value = true
  try {
    await configStore.saveLLMConfig(form.value.api_key, form.value.base_url, form.value.model_name)
    ElMessage.success('配置保存成功')
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const testConnection = async () => {
  testing.value = true
  try {
    await configStore.testConnection(form.value.api_key, form.value.base_url, form.value.model_name)
    ElMessage.success('连接测试成功')
  } catch (e: any) {
    ElMessage.error(e.message || '连接测试失败')
  } finally {
    testing.value = false
  }
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
</style>
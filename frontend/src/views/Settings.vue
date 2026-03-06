<template>
  <div class="settings">
    <el-card>
      <template #header>
        <span>LLM 配置</span>
      </template>
      
      <el-form :model="form" label-width="120px">
        <el-form-item label="API Key">
          <el-input v-model="form.api_key" type="password" placeholder="请输入 API Key" />
        </el-form-item>
        <el-form-item label="Base URL">
          <el-input v-model="form.base_url" placeholder="https://dashscope.aliyuncs.com/compatible-mode/v1" />
        </el-form-item>
        <el-form-item label="模型名称">
          <el-input v-model="form.model_name" placeholder="qwen-plus" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveConfig">保存配置</el-button>
          <el-button @click="testConnection">测试连接</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card style="margin-top: 20px">
      <template #header>
        <span>Agent 安装命令</span>
      </template>
      
      <el-input v-model="installCommand" readonly>
        <template #append>
          <el-button @click="copyCommand">复制</el-button>
        </template>
      </el-input>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useConfigStore } from '@/store/config'

const configStore = useConfigStore()
const form = ref({
  api_key: '',
  base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  model_name: 'qwen-plus'
})
const installCommand = ref('')

const saveConfig = async () => {
  await configStore.saveLLMConfig(form.value.api_key, form.value.base_url, form.value.model_name)
  ElMessage.success('配置保存成功')
}

const testConnection = async () => {
  await configStore.testConnection(form.value.api_key, form.value.base_url, form.value.model_name)
  ElMessage.success('连接测试成功')
}

const copyCommand = () => {
  navigator.clipboard.writeText(installCommand.value)
  ElMessage.success('已复制到剪贴板')
}

onMounted(async () => {
  await configStore.fetchLLMConfig()
  await configStore.fetchInstallCommand()
  if (configStore.llmConfig) {
    form.value.base_url = configStore.llmConfig.base_url
    form.value.model_name = configStore.llmConfig.model_name
  }
  if (configStore.installCommand) {
    installCommand.value = configStore.installCommand.command
  }
})
</script>
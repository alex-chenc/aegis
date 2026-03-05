<template>
  <div class="settings-page">
    <el-card class="box-card">
      <template #header>
        <div class="card-header">
          <span>大模型配置</span>
        </div>
      </template>
      <el-form :model="configForm" :rules="rules" ref="formRef" label-width="120px">
        <el-form-item label="Base URL" prop="baseUrl">
<el-input 
            v-model="configForm.baseUrl" 
            placeholder="请输入 Base URL, 如 https://api.openai.com/v1"
            :disabled="isValidated || isLoading"
          />
        </el-form-item>
        <el-form-item label="API Key" prop="apiKey">
          <el-input 
            v-model="configForm.apiKey" 
            type="password"
            placeholder="请输入 API Key"
            show-password
            :disabled="isValidated || isLoading"
          />
        </el-form-item>

        <!-- Test result messages -->
        <el-alert
          v-if="testResult === 'failed'"
          title="后端服务连接失败，请检查网络或联系管理员"
          type="error"
          show-icon
          class="status-alert"
          :closable="false"
        />
        <el-alert
          v-if="isValidated"
          title="已验证：连接成功"
          type="success"
          show-icon
          class="status-alert"
          :closable="false"
        />

        <el-form-item>
          <el-button 
            type="primary" 
            v-if="!isValidated"
            :loading="isLoading"
            :disabled="!configForm.apiKey || !configForm.baseUrl"
            @click="handleTestConnection"
          >
            测试连通性
          </el-button>
          <el-button 
            type="success" 
            v-if="isValidated"
            :loading="isSaving"
            @click="handleSaveConfig"
          >
            保存配置
          </el-button>
          <el-button 
            v-if="isValidated"
            @click="handleEditConfig"
          >
            编辑
          </el-button>
        </el-form-item>
        <el-form-item label="API Key" prop="apiKey">
          <el-input 
            v-model="configForm.apiKey" 
            type="password"
            placeholder="请输入 API Key"
            show-password
            :disabled="isValidated || isLoading"
          />
        </el-form-item>

        <!-- Test result messages -->
        <el-alert
          v-if="testResult === 'failed'"
          title="后端服务连接失败，请检查网络或联系管理员"
          type="error"
          show-icon
          class="status-alert"
          :closable="false"
        />
        <el-alert
          v-if="isValidated"
          title="已验证：连接成功"
          type="success"
          show-icon
          class="status-alert"
          :closable="false"
        />
        <el-alert
          v-if="isValidated"
          title="已验证：连接成功"
          type="success"
          show-icon
          class="status-alert"
          :closable="false"
        />

        <el-form-item>
          <el-button 
            type="primary" 
            v-if="!isValidated"
            :loading="isLoading"
            :disabled="!configForm.apiKey || !configForm.baseUrl"
            @click="handleTestConnection"
          >
            测试连通性
          </el-button>
          <el-button 
            type="success" 
            v-if="isValidated"
            :loading="isSaving"
            @click="handleSaveConfig"
          >
            保存配置
          </el-button>
          <el-button 
            v-if="isValidated"
            @click="handleEditConfig"
          >
            编辑
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="box-card mt-20">
      <template #header>
        <div class="card-header">
          <span>Agent 安装指南</span>
        </div>
      </template>
      <div class="agent-install">
        <p>请在目标主机上执行以下命令以安装并启动 Agent：</p>
        <div class="code-block">
          <pre><code>curl -sSL http://&lt;your-server-ip&gt;/install.sh | bash</code></pre>
          <el-button size="small" icon="CopyDocument" @click="copyCommand">复制</el-button>
        </div>
        <p class="text-sm mt-10">注意：请将 &lt;your-server-ip&gt; 替换为当前平台的实际 IP 或域名。</p>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import type { FormInstance, FormRules } from 'element-plus';
import { CopyDocument } from '@element-plus/icons-vue';
import { getServerInfo, testLLMConnection } from '@/api/hosts';

const formRef = ref<FormInstance>();

const configForm = reactive({
  baseUrl: '',
  apiKey: '',
});

const isValidated = ref(false);
const isLoading = ref(false);
const isSaving = ref(false);
const testResult = ref<'none' | 'failed' | 'success'>('none');
const serverInfo = ref({ server_ip: '', server_address: '', grpc_port: '', http_port: '' });

const rules = reactive<FormRules>({
  baseUrl: [
    { required: true, message: '请输入有效的 URL 格式', trigger: 'blur' },
    { type: 'url', message: '请输入有效的 URL 格式', trigger: ['blur', 'change'] }
  ],
  apiKey: [
    { required: true, message: '请输入有效的 API Key 格式', trigger: 'blur' }
  ]
});

const handleTestConnection = async () => {
  if (!formRef.value) return;
  
  await formRef.value.validate(async (valid) => {
    if (valid) {
      testResult.value = 'none';
      isLoading.value = true;
      try {
        await testLLMConnection();
        isValidated.value = true;
        testResult.value = 'success';
        ElMessage.success('连接测试成功');
      } catch (error) {
        testResult.value = 'failed';
        ElMessage.error('测试连通性失败');
      } finally {
        isLoading.value = false;
      }
    }
  });
};

const handleSaveConfig = async () => {
  isSaving.value = true;
  try {
    ElMessage.success('保存成功');
    isValidated.value = true;
  } catch (error) {
    ElMessage.error('配置保存失败');
  } finally {
    isSaving.value = false;
  }
};

const handleEditConfig = () => {
  isValidated.value = false;
  testResult.value = 'none';
};

const copyCommand = () => {
  const command = `curl -sSL http://${serverInfo.value.server_ip}/agent/install.sh | bash -s -- --server=${serverInfo.value.server_address}`;
  navigator.clipboard.writeText(command);
  ElMessage.success('复制成功');
};

onMounted(async () => {
  try {
    const info = await getServerInfo();
    serverInfo.value = info;
  } catch (error) {
    console.error('Failed to get server info', error);
  }
});
</script>

<style scoped>
.mt-20 {
  margin-top: 20px;
}
.mt-10 {
  margin-top: 10px;
}
.status-alert {
  margin-bottom: 20px;
}
.agent-install p {
  margin-bottom: 10px;
  color: #606266;
}
.text-sm {
  font-size: 13px;
  color: #909399;
}
.code-block {
  background-color: #282c34;
  color: #abb2bf;
  padding: 10px 15px;
  border-radius: 4px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.code-block pre {
  margin: 0;
  font-family: Consolas, Monaco, 'Andale Mono', 'Ubuntu Mono', monospace;
  font-size: 14px;
}
</style>

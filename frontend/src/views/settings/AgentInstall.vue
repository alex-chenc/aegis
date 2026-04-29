<template>
  <div class="settings page-shell">
    <section class="page-hero agent-hero">
      <div>
        <span class="hero-kicker">Agent Enrollment</span>
        <h1>Agent 安装</h1>
        <p>获取当前控制面的 Agent 安装命令，在目标主机执行后即可接入采集、检测与响应链路。</p>
      </div>
      <el-button type="primary" :loading="loading" @click="fetchInstallCmd">刷新安装命令</el-button>
    </section>

    <el-card class="aegis-card install-card">
      <template #header>
        <div class="card-header">
          <span>安装命令</span>
          <el-tag type="success" size="small">在线生成</el-tag>
        </div>
      </template>

      <div class="command-panel">
        <div class="command-label">在目标 Linux 主机执行</div>
        <el-input v-model="installCommand" readonly class="command-input">
          <template #append>
            <el-button :disabled="!installCommand" @click="copyCommand">
              <el-icon><CopyDocument /></el-icon>
              复制
            </el-button>
          </template>
        </el-input>
      </div>

      <div class="install-meta">
        <div class="meta-card">
          <span class="meta-label">服务器地址</span>
          <strong>{{ installInfo?.server_ip || '-' }}:{{ installInfo?.http_port || '-' }}</strong>
        </div>
        <div class="meta-card">
          <span class="meta-label">gRPC 端口</span>
          <strong>{{ installInfo?.grpc_port || '-' }}</strong>
        </div>
        <div class="meta-card">
          <span class="meta-label">安装状态</span>
          <strong>{{ installCommand ? '命令已就绪' : '等待生成' }}</strong>
        </div>
      </div>

      <el-alert
        title="部署提示"
        type="info"
        show-icon
        :closable="false"
        class="install-alert"
      >
        <p>如果目标主机无法连接控制面，请先确认防火墙、安全组和 gRPC 端口连通性。</p>
      </el-alert>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import { getInstallCommand } from '@/api/config'
import type { InstallCommand } from '@/types'

const installCommand = ref('')
const installInfo = ref<InstallCommand | null>(null)
const loading = ref(false)

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
  loading.value = true
  try {
    const data = await getInstallCommand()
    installInfo.value = data
    installCommand.value = data.command
  } catch (e: any) {
    ElMessage.error(e.message || '获取安装命令失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchInstallCmd()
})
</script>

<style scoped>
.agent-hero {
  margin-bottom: 0;
}

.hero-kicker {
  display: inline-flex;
  margin-bottom: 8px;
  color: #0891b2;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.install-card {
  overflow: hidden;
}

.command-panel {
  padding: 18px;
  border: 1px solid rgba(37, 99, 235, 0.14);
  border-radius: 16px;
  background:
    linear-gradient(135deg, rgba(37, 99, 235, 0.08), rgba(34, 211, 238, 0.08)),
    #ffffff;
}

.command-label {
  margin-bottom: 10px;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.command-input :deep(.el-input__wrapper) {
  font-family: var(--aegis-font-mono);
}

.install-meta {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
  margin-top: 18px;
}

.meta-card {
  padding: 16px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.72);
}

.meta-label {
  display: block;
  margin-bottom: 8px;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.meta-card strong {
  color: #0f172a;
  font-size: 18px;
}

.install-alert {
  margin-top: 18px;
}
</style>

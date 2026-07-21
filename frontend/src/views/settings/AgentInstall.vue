<template>
  <div class="settings page-shell">
    <section class="page-hero agent-hero">
      <div>
        <span class="hero-kicker">Agent Enrollment</span>
        <h1>{{ $t('generated.settingsAgentInstall_agent_installation_c8dc62') }}</h1>
        <p>{{ $t('generated.settingsAgentInstall_obtain_the_agent_installation_command_of_e0113f') }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="fetchInstallCmd">{{ $t('generated.settingsAgentInstall_refresh_installation_command_b2c31b') }}</el-button>
    </section>

    <el-card class="aegis-card install-card">
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.settingsAgentInstall_installation_command_5ff833') }}</span>
          <el-tag type="success" size="small">{{ $t('generated.settingsAgentInstall_online_generation_d6e96a') }}</el-tag>
        </div>
      </template>

      <div class="command-panel">
        <div class="command-label">{{ $t('generated.settingsAgentInstall_execute_on_the_target_linux_host_f56864') }}</div>
        <el-input v-model="installCommand" readonly class="command-input">
          <template #append>
            <el-button :disabled="!installCommand" @click="copyCommand">
              <el-icon><CopyDocument /></el-icon>
              {{ $t('generated.common_copy_4edd1d') }}
            </el-button>
          </template>
        </el-input>
      </div>

      <div class="install-meta">
        <div class="meta-card">
          <span class="meta-label">{{ $t('generated.settingsAgentInstall_server_address_2eb291') }}</span>
          <strong>{{ installInfo?.server_ip || '-' }}:{{ installInfo?.http_port || '-' }}</strong>
        </div>
        <div class="meta-card">
          <span class="meta-label">{{ $t('generated.settingsAgentInstall_grpc_port_df9c9a') }}</span>
          <strong>{{ installInfo?.grpc_port || '-' }}</strong>
        </div>
        <div class="meta-card">
          <span class="meta-label">{{ $t('generated.settingsAgentInstall_installation_status_9c7821') }}</span>
          <strong>{{ installCommand ? $t('dynamic.commandReady') : $t('dynamic.waitingGeneration') }}</strong>
        </div>
      </div>

      <el-alert
        :title="$t('generated.settingsAgentInstall_deployment_tips_2f7a3c')"
        type="info"
        show-icon
        :closable="false"
        class="install-alert"
      >
        <p>{{ $t('generated.settingsAgentInstall_if_the_target_host_cannot_connect_40b97d') }}</p>
      </el-alert>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

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
    ElMessage.warning(translate('generatedScript.settingsAgentInstall_no_content_to_copy_2241b3'))
    return
  }

  const success = await copyToClipboard(installCommand.value)
  if (success) {
    ElMessage.success(translate('generatedScript.common_copied_to_clipboard_c2bb6d'))
  } else {
    ElMessage.error(translate('generatedScript.settingsAgentInstall_copy_failed_please_copy_manually_2b1cae'))
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
    ElMessage.error(e.message || translate('generatedScript.settingsAgentInstall_failed_to_get_installation_command_b6fbf1'))
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

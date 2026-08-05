<template>
  <el-dialog
    :model-value="visible"
    :title="t(mode === 'escape' ? 'agentGuard.settings.escapeTitle' : 'agentGuard.settings.title')"
    width="720px"
    destroy-on-close
    @close="emit('close')"
  >
    <el-alert
      type="info"
      :title="t(mode === 'escape' ? 'agentGuard.settings.escapeNotice' : 'agentGuard.settings.notice')"
      :closable="false"
      show-icon
      class="settings-notice"
    />

    <el-form label-position="top" class="settings-form">
      <el-form-item :label="t('agentGuard.settings.host')" required>
        <el-select
          v-model="selectedHostId"
          filterable
          class="full-width"
          :disabled="loading || saving"
          @change="loadSettings"
        >
          <el-option
            v-for="host in hosts"
            :key="host.id"
            :value="host.id"
            :label="`${host.hostname} · ${host.ip}`"
          />
        </el-select>
      </el-form-item>

      <div v-if="settings" class="settings-switches">
        <div v-if="mode === 'behavior'" class="settings-switch-row">
          <div><strong>{{ t('agentGuard.settings.behaviorPolicyLabel') }}</strong><p>{{ t('agentGuard.settings.behaviorPolicyHint') }}</p></div>
          <el-switch :model-value="settings.behavior_policy_enabled ?? true" :disabled="loading || saving" @change="toggleRuntimeSetting('behavior_policy_enabled', $event)" />
        </div>
        <div v-if="mode === 'escape'" class="settings-switch-row">
          <div><strong>{{ t('agentGuard.settings.escapePolicyLabel') }}</strong><p>{{ t('agentGuard.settings.escapePolicyHint') }}</p></div>
          <el-switch :model-value="settings.escape_policy_enabled ?? true" :disabled="loading || saving" @change="toggleRuntimeSetting('escape_policy_enabled', $event)" />
        </div>
        <div v-if="mode === 'behavior'" class="settings-switch-row">
          <div>
            <strong>{{ t('agentGuard.settings.toolAdapterLabel') }}</strong>
            <p>{{ t('agentGuard.settings.toolAdapterHint') }}</p>
          </div>
          <el-switch
            :model-value="settings.tool_adapter_enabled"
            :active-text="t('agentGuard.settings.on')"
            :inactive-text="t('agentGuard.settings.off')"
            :disabled="loading || saving"
            @change="toggleRuntimeSetting('tool_adapter_enabled', $event)"
          />
        </div>
        <div v-if="mode === 'behavior'" class="settings-switch-row">
          <div>
            <strong>{{ t('agentGuard.settings.sessionHookLabel') }}</strong>
            <p>{{ t('agentGuard.settings.sessionHookHint') }}</p>
          </div>
          <el-switch
            :model-value="settings.session_hook_enabled"
            :active-text="t('agentGuard.settings.on')"
            :inactive-text="t('agentGuard.settings.off')"
            :disabled="loading || saving"
            @change="toggleRuntimeSetting('session_hook_enabled', $event)"
          />
        </div>
      </div>

      <el-form-item v-if="settings" :label="t(mode === 'escape' ? 'agentGuard.settings.escapeInjections' : 'agentGuard.settings.injections')">
        <div class="injection-switches">
          <div v-for="injection in settings.injections" :key="injection.agent_type" class="settings-switch-row">
            <div>
              <strong>{{ t(`agentGuard.agentTypes.${injection.agent_type}`) }}</strong>
              <p>{{ t('agentGuard.settings.injectionHint') }}</p>
            </div>
            <div class="scope-switches">
            <el-switch
              v-if="mode === 'behavior'"
              :model-value="injection.behavior_enabled ?? injection.enabled"
              :active-text="t('agentGuard.settings.on')"
              :inactive-text="t('agentGuard.settings.off')"
              :disabled="loading || saving"
              @change="toggleInjection(injection.agent_type, 'behavior_enabled', $event)"
            />
            <el-switch v-if="mode === 'escape'" :model-value="injection.escape_enabled ?? false" :active-text="t('agentGuard.settings.escapeHookShort')" :inactive-text="t('agentGuard.settings.off')" :disabled="loading || saving" @change="toggleInjection(injection.agent_type, 'escape_enabled', $event)" />
            </div>
          </div>
        </div>
      </el-form-item>

      <el-alert
        v-if="errorMessage"
        type="error"
        :title="errorMessage"
        :closable="false"
        show-icon
      />
    </el-form>

    <template #footer>
      <el-button @click="emit('close')">{{ t('common.actions.close') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { getAgentGuardRuntimeSettings, updateAgentGuardRuntimeSettings } from '@/api/agentGuard'
import type { AgentGuardRuntimeSettings } from '@/types/agentGuard'

interface HostOption {
  id: string
  hostname: string
  ip: string
}

const props = withDefaults(defineProps<{
  visible: boolean
  hosts: HostOption[]
  mode?: 'behavior' | 'escape'
}>(), { mode: 'behavior' })

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saved', settings: AgentGuardRuntimeSettings): void
}>()

const { t } = useI18n()
const selectedHostId = ref('')
const settings = ref<AgentGuardRuntimeSettings | null>(null)
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')

type RuntimeToggleField = 'tool_adapter_enabled' | 'session_hook_enabled' | 'behavior_policy_enabled' | 'escape_policy_enabled'

watch(() => props.visible, visible => {
  if (!visible) return
  selectedHostId.value = selectedHostId.value || props.hosts[0]?.id || ''
  if (selectedHostId.value) loadSettings()
})

watch(() => props.hosts, hosts => {
  if (props.visible && !selectedHostId.value && hosts[0]) {
    selectedHostId.value = hosts[0].id
    loadSettings()
  }
}, { deep: true })

async function loadSettings() {
  if (!selectedHostId.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    settings.value = await getAgentGuardRuntimeSettings(selectedHostId.value)
  } catch {
    settings.value = null
    errorMessage.value = t('agentGuard.settings.loadFailed')
  } finally {
    loading.value = false
  }
}

async function save(): Promise<boolean> {
  if (!settings.value) return false
  saving.value = true
  errorMessage.value = ''
  try {
    const result = await updateAgentGuardRuntimeSettings({
      host_id: settings.value.host_id,
      tool_adapter_enabled: settings.value.tool_adapter_enabled,
      session_hook_enabled: settings.value.session_hook_enabled,
      behavior_policy_enabled: settings.value.behavior_policy_enabled ?? true,
      escape_policy_enabled: settings.value.escape_policy_enabled ?? true,
      injections: settings.value.injections,
    })
    settings.value = result
    emit('saved', result)
    ElMessage.success(t('agentGuard.settings.saved'))
    return true
  } catch (error: any) {
    errorMessage.value = error?.response?.data?.message || t('agentGuard.settings.saveFailed')
    return false
  } finally {
    saving.value = false
  }
}

async function toggleRuntimeSetting(field: RuntimeToggleField, value: boolean | string | number) {
  if (!settings.value) return
  const previous = settings.value[field]
  settings.value[field] = Boolean(value)
  if (!(await save()) && settings.value) {
    settings.value[field] = previous
  }
}

async function toggleInjection(agentType: string, field: 'behavior_enabled' | 'escape_enabled', value: boolean | string | number) {
  if (!settings.value) return
  const injection = settings.value.injections.find(item => item.agent_type === agentType)
  if (!injection) return
  const previous = injection[field]
  injection[field] = Boolean(value)
  injection.enabled = Boolean(injection.behavior_enabled || injection.escape_enabled)
  if (!(await save()) && settings.value) {
    const current = settings.value.injections.find(item => item.agent_type === agentType)
    if (current) { current[field] = previous; current.enabled = Boolean(current.behavior_enabled || current.escape_enabled) }
  }
}

</script>

<style scoped>
.settings-notice {
  margin-bottom: 18px;
}

.full-width {
  width: 100%;
}

.settings-switches {
  display: grid;
  gap: 12px;
  margin-bottom: 18px;
}

.settings-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 14px 16px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
}

.settings-switch-row p {
  margin: 5px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.injection-switches {
  width: 100%;
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--el-border-color-lighter);
  display: grid;
  gap: 12px;
}

.scope-switches {
  display: flex;
  gap: 12px;
  align-items: center;
}
</style>

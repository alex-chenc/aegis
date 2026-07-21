<template>
  <el-card class="audit-policy-card">
    <template #header>
      <div class="card-header">
        <div class="card-header-left">
          <span class="card-title">{{ $t('generated.settingsCommandAuditAuditPolicyCard_audit_strategy_c75707') }}</span>
          <span class="card-subtitle">{{ $t('generated.settingsCommandAuditAuditPolicyCard_configuring_inspection_levels_and_behavior_for_4bf1da') }}</span>
        </div>
      </div>
    </template>
    <div class="policy-grid">
      <div
        v-for="item in policyItems"
        :key="item.key"
        class="policy-item"
        :class="{ 'policy-item--active': localSettings[item.key] }"
      >
        <div class="policy-item__icon" :style="{ background: item.bgColor }">
          <span class="policy-item__icon-text">{{ item.icon }}</span>
        </div>
        <div class="policy-item__content">
          <div class="policy-item__header">
            <span class="policy-item__name">{{ item.label }}</span>
            <el-switch
              v-model="localSettings[item.key]"
              size="small"
              @change="emitUpdate"
            />
          </div>
          <span class="policy-item__desc">{{ item.desc }}</span>
          <el-tag v-if="item.key === 'ai_enabled' && !llmAvailable" type="warning" size="small" effect="plain" style="margin-top: 4px">
            {{ $t('generated.settingsCommandAuditAuditPolicyCard_llm_not_configured_3abb4e') }}
          </el-tag>
        </div>
      </div>
    </div>
    <div class="retry-section">
      <span class="retry-label">{{ $t('generated.settingsCommandAuditAuditPolicyCard_maximum_number_of_retries_11b03b') }}</span>
      <el-input-number v-model="localSettings.max_retry" :min="1" :max="5" size="small" @change="emitUpdate" />
      <span class="retry-hint">{{ $t('generated.settingsCommandAuditAuditPolicyCard_maximum_number_of_attempts_to_regenerate_b08e81') }}</span>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, reactive, watch } from 'vue'
import type { CommandAuditSettings } from '@/api/command-audit'

const props = defineProps<{
  settings: CommandAuditSettings | null
  llmAvailable?: boolean
}>()

const emit = defineEmits<{
  (e: 'update', data: Partial<CommandAuditSettings>): void
}>()

const policyItems = computed(() => [
  { key: 'blacklist_enabled' as const, label: translate('generatedScript.settingsCommandAuditAuditPolicyCard_blacklist_audit_673edd'), icon: 'B', desc: translate('generatedScript.settingsCommandAuditAuditPolicyCard_deterministic_inspection_based_on_preset_rules_a1acb4'), bgColor: 'rgba(239, 68, 68, 0.1)' },
  { key: 'ai_enabled' as const, label: translate('generatedScript.settingsCommandAuditAuditPolicyCard_ai_audit_efb648'), icon: 'AI', desc: translate('generatedScript.settingsCommandAuditAuditPolicyCard_contextual_risk_analysis_based_on_large_7cf8e0'), bgColor: 'rgba(99, 102, 241, 0.1)' },
  { key: 'dispatch_check' as const, label: translate('generatedScript.settingsCommandAuditAuditPolicyCard_verification_before_delivery_f5a60f'), icon: 'P', desc: translate('generatedScript.settingsCommandAuditAuditPolicyCard_check_the_blacklist_again_before_sending_928eea'), bgColor: 'rgba(245, 158, 11, 0.1)' },
  { key: 'agent_check' as const, label: translate('generatedScript.settingsCommandAuditAuditPolicyCard_agent_side_verification_570901'), icon: 'A', desc: translate('generatedScript.settingsCommandAuditAuditPolicyCard_the_last_line_of_defense_before_dea346'), bgColor: 'rgba(16, 185, 129, 0.1)' }
])

const localSettings = reactive<CommandAuditSettings>({
  blacklist_enabled: true,
  ai_enabled: true,
  dispatch_check: true,
  agent_check: true,
  max_retry: 3
})

watch(() => props.settings, (val) => {
  if (val) Object.assign(localSettings, val)
}, { immediate: true })

function emitUpdate() {
  emit('update', { ...localSettings })
}
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.card-header-left {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
}
.card-subtitle {
  font-size: 13px;
  color: #9ca3af;
}
.policy-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}
.policy-item {
  display: flex;
  gap: 12px;
  padding: 14px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  background: #fafafa;
  transition: border-color 0.2s, background 0.2s;
}
.policy-item--active {
  border-color: #c7d2fe;
  background: #f5f3ff;
}
.policy-item__icon {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.policy-item__icon-text {
  font-size: 14px;
  font-weight: 700;
  color: #4b5563;
}
.policy-item__content {
  flex: 1;
  min-width: 0;
}
.policy-item__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}
.policy-item__name {
  font-size: 14px;
  font-weight: 600;
  color: #374151;
}
.policy-item__desc {
  font-size: 12px;
  color: #9ca3af;
  line-height: 1.4;
}
.retry-section {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #f3f4f6;
}
.retry-label {
  font-size: 14px;
  font-weight: 600;
  color: #374151;
  white-space: nowrap;
}
.retry-hint {
  font-size: 12px;
  color: #9ca3af;
}
</style>

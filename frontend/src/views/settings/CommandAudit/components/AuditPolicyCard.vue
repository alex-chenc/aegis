<template>
  <el-card class="audit-policy-card">
    <template #header>
      <div class="card-header">
        <div class="card-header-left">
          <span class="card-title">审计策略</span>
          <span class="card-subtitle">配置脚本安全审计的检查级别和行为</span>
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
            未配置LLM
          </el-tag>
        </div>
      </div>
    </div>
    <div class="retry-section">
      <span class="retry-label">最大重试次数</span>
      <el-input-number v-model="localSettings.max_retry" :min="1" :max="5" size="small" @change="emitUpdate" />
      <span class="retry-hint">AI审计失败后重新生成脚本的最大尝试次数</span>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { CommandAuditSettings } from '@/api/command-audit'

const props = defineProps<{
  settings: CommandAuditSettings | null
  llmAvailable?: boolean
}>()

const emit = defineEmits<{
  (e: 'update', data: Partial<CommandAuditSettings>): void
}>()

const policyItems = [
  { key: 'blacklist_enabled' as const, label: '黑名单审计', icon: 'B', desc: '基于预置规则的确定性检查，命中即拦截', bgColor: 'rgba(239, 68, 68, 0.1)' },
  { key: 'ai_enabled' as const, label: 'AI 审计', icon: 'AI', desc: '基于大模型的上下文风险分析，检测隐蔽威胁', bgColor: 'rgba(99, 102, 241, 0.1)' },
  { key: 'dispatch_check' as const, label: '下发前校验', icon: 'P', desc: '脚本从 API Server 下发前再次校验黑名单', bgColor: 'rgba(245, 158, 11, 0.1)' },
  { key: 'agent_check' as const, label: 'Agent 侧校验', icon: 'A', desc: 'Agent 执行前的最后一道防线', bgColor: 'rgba(16, 185, 129, 0.1)' }
]

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

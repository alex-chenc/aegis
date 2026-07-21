<template>
  <div class="tool-call-card" :class="call.status">
    <div class="card-header">
      <div class="tool-info">
        <el-tag :type="getRiskTag(call.risk_level)" size="small">
          {{ call.risk_level }}
        </el-tag>
        <span class="tool-name">{{ call.tool_name }}</span>
      </div>
      <el-tag :type="getStatusTag(call.status)" size="small">
        {{ getStatusLabel(call.status) }}
      </el-tag>
    </div>

    <div v-if="call.args_summary" class="card-args">
      <span class="label">{{ $t('generated.assistantAssistantToolCallCard_parameter_0b94fc') }}</span>
      <span class="value">{{ call.args_summary }}</span>
    </div>

    <div v-if="call.result_summary" class="card-result">
      <span class="label">{{ $t('generated.assistantAssistantToolCallCard_result_b2957d') }}</span>
      <span class="value">{{ call.result_summary }}</span>
    </div>

    <div v-if="call.error_message" class="card-error">
      <el-icon><Warning /></el-icon>
      <span>{{ call.error_message }}</span>
    </div>

    <div v-if="call.duration_ms" class="card-duration">
      {{ $t('generated.assistantAssistantToolCallCard_time_consuming_1bb779') }} {{ formatDuration(call.duration_ms) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { Warning } from '@element-plus/icons-vue'
import type { AssistantToolCall } from '@/api/assistant'

defineProps<{
  call: AssistantToolCall
}>()

function getRiskTag(level: string): string {
  const map: Record<string, string> = {
    readonly: 'info',
    low: 'success',
    medium: 'warning',
    high: 'danger',
    critical: 'danger',
  }
  return map[level] || 'info'
}

function getStatusTag(status: string): string {
  const map: Record<string, string> = {
    pending: 'info',
    running: 'warning',
    accepted: 'warning',
    completed: 'success',
    success: 'success',
    failed: 'danger',
    approval_required: 'warning',
    rejected: 'info',
    cancelled: 'info',
  }
  return map[status] || 'info'
}

function getStatusLabel(status: string): string {
  const map: Record<string, string> = {
    pending: translate('generatedScript.common_waiting_bd3488'),
    running: translate('generatedScript.common_executing_1f425b'),
    accepted: translate('generatedScript.common_accepted_926b04'),
    completed: translate('generatedScript.common_success_51991a'),
    success: translate('generatedScript.common_success_51991a'),
    failed: translate('generatedScript.common_fail_3e3c80'),
    approval_required: translate('generatedScript.common_pending_approval_57fce0'),
    rejected: translate('generatedScript.common_rejected_4c7c52'),
    cancelled: translate('generatedScript.common_canceled_a5ffdc'),
  }
  return map[status] || status
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}
</script>

<style scoped>
.tool-call-card {
  background: #f5f7fa;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
}

.tool-call-card.success {
  border-left: 3px solid #67c23a;
}

.tool-call-card.failed {
  border-left: 3px solid #f56c6c;
}

.tool-call-card.running {
  border-left: 3px solid #e6a23c;
}

.tool-call-card.approval_required {
  border-left: 3px solid #e6a23c;
  background: #fdf6ec;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.tool-info {
  display: flex;
  align-items: center;
  gap: 6px;
}

.tool-name {
  font-weight: 500;
  color: #303133;
}

.card-args,
.card-result {
  margin-top: 4px;
  color: #606266;
}

.card-args .label,
.card-result .label {
  color: #909399;
  margin-right: 4px;
}

.card-error {
  margin-top: 4px;
  color: #f56c6c;
  display: flex;
  align-items: center;
  gap: 4px;
}

.card-duration {
  margin-top: 4px;
  color: #909399;
  font-size: 12px;
}
</style>

<template>
  <aside class="context-rail">
    <div v-if="plan" class="rail-section">
      <ExecutionPlan :plan="plan" title-only />
    </div>
    <div v-else class="rail-section">
      <div class="section-header">
        <el-icon><List /></el-icon>
        <span>{{ $t('generated.common_execution_plan_822376') }}</span>
      </div>
      <el-empty :description="$t('generated.assistantAssistantContextRail_no_execution_plan_yet_f2c588')" :image-size="48" />
    </div>

    <div v-if="approvals.length" class="rail-section">
      <div class="section-header">
        <el-icon><Bell /></el-icon>
        <span>{{ $t('generated.assistantAssistantContextRail_action_pending_approval_80adeb') }}</span>
        <el-badge :value="approvals.length" type="warning" />
      </div>
      <div class="approval-list">
        <div
          v-for="approval in approvals"
          :key="approval.approval_id"
          class="approval-item"
        >
          <div class="approval-info">
            <el-tag :type="getRiskTag(approval.risk_level)" size="small">
              {{ approval.risk_level }}
            </el-tag>
            <span class="approval-tool">{{ approval.tool_name }}</span>
          </div>
          <div class="approval-title">{{ approval.title }}</div>
        </div>
      </div>
    </div>

    <div class="rail-section">
      <div class="section-header">
        <el-icon><Tools /></el-icon>
        <span>{{ $t('generated.assistantAssistantContextRail_tool_call_record_99f656') }}</span>
      </div>
      <div v-if="toolCalls.length" class="tool-call-list">
        <div
          v-for="call in toolCalls"
          :key="call.call_id"
          class="tool-call-item"
        >
          <el-tag :type="getRiskTag(call.risk_level)" size="small">
            {{ call.risk_level }}
          </el-tag>
          <span class="tool-name">{{ call.tool_name }}</span>
          <el-tag :type="getStatusTag(call.status)" size="small">
            {{ getStatusLabel(call.status) }}
          </el-tag>
        </div>
      </div>
      <el-empty v-else :description="$t('generated.assistantAssistantContextRail_no_tool_calls_yet_c7c541')" :image-size="48" />
    </div>
  </aside>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { Bell, List, Tools } from '@element-plus/icons-vue'
import ExecutionPlan from '@/components/ExecutionPlan.vue'
import type { PlanEvent } from '@/api/aiAnalysis'
import type { AssistantToolCall, AssistantApproval } from '@/api/assistant'

defineProps<{
  plan: PlanEvent | null
  approvals: AssistantApproval[]
  toolCalls: AssistantToolCall[]
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
    completed: translate('generatedScript.common_success_51991a'),
    success: translate('generatedScript.common_success_51991a'),
    failed: translate('generatedScript.common_fail_3e3c80'),
    approval_required: translate('generatedScript.common_pending_approval_57fce0'),
    rejected: translate('generatedScript.common_rejected_4c7c52'),
    cancelled: translate('generatedScript.common_canceled_a5ffdc'),
  }
  return map[status] || status
}
</script>

<style scoped>
.context-rail {
  width: 360px;
  background: #fff;
  border-left: 1px solid #e4e7ed;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  will-change: transform, opacity;
}

.rail-section {
  padding: 16px;
  border-bottom: 1px solid #e4e7ed;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.approval-list,
.tool-call-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.approval-item {
  padding: 10px;
  background: #f5f7fa;
  border-radius: 6px;
}

.approval-info,
.tool-call-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.approval-tool,
.tool-name {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.approval-title {
  margin-top: 6px;
  font-size: 12px;
  color: #606266;
  line-height: 1.4;
}

.tool-call-item {
  padding: 8px;
  background: #f5f7fa;
  border-radius: 6px;
}

@media (max-width: 1200px) {
  .context-rail {
    display: none;
  }
}
</style>

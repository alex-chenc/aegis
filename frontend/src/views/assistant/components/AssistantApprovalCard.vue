<template>
  <div class="approval-card" :class="approval.status">
    <div class="card-header">
      <div class="approval-info">
        <el-icon class="warning-icon"><WarningFilled /></el-icon>
        <span class="approval-title">{{ approval.title || $t('dynamic.approvalRequired') }}</span>
      </div>
      <el-tag :type="getRiskTag(approval.risk_level)" size="small">
        {{ approval.risk_level }}
      </el-tag>
    </div>

    <div class="card-body">
      <div class="detail-row">
        <span class="label">{{ $t('generated.assistantAssistantApprovalCard_tool_f7bb5a') }}</span>
        <span class="value">{{ approval.tool_name }}</span>
      </div>
      <div v-if="approval.impact_summary" class="detail-row">
        <span class="label">{{ $t('generated.common_influence_f198c0') }}</span>
        <span class="value">{{ approval.impact_summary }}</span>
      </div>
      <div v-if="approval.rollback_hint" class="detail-row">
        <span class="label">{{ $t('generated.assistantAssistantApprovalCard_rollback_d4f68a') }}</span>
        <span class="value">{{ approval.rollback_hint }}</span>
      </div>
    </div>

    <div v-if="approval.status === 'pending'" class="card-actions">
      <el-button
        type="danger"
        size="small"
        @click="handleReject"
      >
        {{ $t('generated.common_reject_03e210') }}
      </el-button>
      <el-button
        type="primary"
        size="small"
        @click="handleApprove"
      >
        {{ $t('generated.assistantAssistantApprovalCard_approval_for_execution_ea39c9') }}
      </el-button>
    </div>

    <div v-else class="card-status">
      <el-tag :type="getStatusTag(approval.status)" size="small">
        {{ getStatusLabel(approval.status) }}
      </el-tag>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { WarningFilled } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import type { AssistantApproval } from '@/api/assistant'

const props = defineProps<{
  approval: AssistantApproval
}>()

const emit = defineEmits<{
  approve: [approvalId: string, comment?: string]
  reject: [approvalId: string, comment?: string]
}>()

async function handleApprove() {
  try {
    const { value } = await ElMessageBox.prompt(
      translate('generatedScript.assistantAssistantApprovalCard_please_enter_approval_remarks_optional_f8dee9'),
      translate('generatedScript.assistantAssistantApprovalCard_approval_for_execution_ea39c9'),
      {
        confirmButtonText: translate('generatedScript.assistantAssistantApprovalCard_approve_62e26d'),
        cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
        inputPlaceholder: translate('generatedScript.assistantAssistantApprovalCard_approval_remarks_a4c9e3'),
        type: 'warning',
      }
    )
    emit('approve', props.approval.approval_id, value || undefined)
  } catch {
    // user cancelled
  }
}

async function handleReject() {
  try {
    const { value } = await ElMessageBox.prompt(
      translate('generatedScript.assistantAssistantApprovalCard_please_enter_a_reason_for_rejection_697a00'),
      translate('generatedScript.assistantAssistantApprovalCard_refusal_to_execute_35e4b5'),
      {
        confirmButtonText: translate('generatedScript.assistantAssistantApprovalCard_reject_03e210'),
        cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
        inputPlaceholder: translate('generatedScript.assistantAssistantApprovalCard_reason_for_rejection_aaa97e'),
        type: 'warning',
      }
    )
    emit('reject', props.approval.approval_id, value || undefined)
  } catch {
    // user cancelled
  }
}

function getRiskTag(level: string): string {
  const map: Record<string, string> = {
    medium: 'warning',
    high: 'danger',
    critical: 'danger',
  }
  return map[level] || 'warning'
}

function getStatusTag(status: string): string {
  const map: Record<string, string> = {
    approved: 'success',
    rejected: 'info',
    expired: 'info',
    executed: 'success',
    failed: 'danger',
  }
  return map[status] || 'info'
}

function getStatusLabel(status: string): string {
  const map: Record<string, string> = {
    approved: translate('generatedScript.assistantAssistantApprovalCard_approved_402875'),
    rejected: translate('generatedScript.common_rejected_4c7c52'),
    expired: translate('generatedScript.assistantAssistantApprovalCard_expired_135437'),
    executed: translate('generatedScript.assistantAssistantApprovalCard_executed_ab1f36'),
    failed: translate('generatedScript.common_execution_failed_9746cf'),
  }
  return map[status] || status
}
</script>

<style scoped>
.approval-card {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 12px;
}

.approval-card.pending {
  border-left: 3px solid #e6a23c;
  background: #fdf6ec;
}

.approval-card.approved {
  border-left: 3px solid #67c23a;
}

.approval-card.rejected {
  border-left: 3px solid #909399;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.approval-info {
  display: flex;
  align-items: center;
  gap: 6px;
}

.warning-icon {
  color: #e6a23c;
  font-size: 18px;
}

.approval-title {
  font-weight: 500;
  color: #303133;
}

.card-body {
  margin-bottom: 10px;
}

.detail-row {
  display: flex;
  margin-bottom: 4px;
  font-size: 13px;
}

.detail-row .label {
  color: #909399;
  width: 50px;
  flex-shrink: 0;
}

.detail-row .value {
  color: #606266;
}

.card-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid #ebeef5;
}

.card-status {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid #ebeef5;
}
</style>

<template>
  <div class="recommended-actions">
    <div class="section-header">
      <span class="section-title">{{ $t('generated.assistantRecommendedActionList_recommended_action_a77d2f') }}</span>
    </div>

    <div v-if="!actions?.length" class="empty-hint">
      {{ $t('generated.assistantRecommendedActionList_no_suggested_actions_yet_d7f0fb') }}
    </div>

    <div v-else class="action-list">
      <div
        v-for="action in actions"
        :key="action.action_id"
        class="action-item"
        :class="action.risk_level"
      >
        <div class="action-header">
          <el-tag :type="categoryTag(action.category)" size="small">
            {{ categoryLabel(action.category) }}
          </el-tag>
          <el-tag :type="riskTag(action.risk_level)" size="small">
            {{ action.risk_level }}
          </el-tag>
        </div>

        <div class="action-title">{{ action.title }}</div>
        <div class="action-description">{{ action.description }}</div>

        <div v-if="action.requires_approval" class="action-approval-hint">
          <el-icon><WarningFilled /></el-icon>
          <span>{{ $t('generated.assistantRecommendedActionList_requires_approval_before_execution_89c335') }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { WarningFilled } from '@element-plus/icons-vue'

interface Action {
  action_id: string
  category: string
  title: string
  description: string
  risk_level: string
  requires_approval: boolean
  related_evidence_ids: string[]
}

defineProps<{
  actions?: Action[]
}>()

function categoryTag(category: string): string {
  const map: Record<string, string> = {
    immediate_forensics: 'danger',
    temporary_containment: 'warning',
    remediation: 'success',
    detection_enhancement: 'info',
  }
  return map[category] || 'info'
}

function categoryLabel(category: string): string {
  const map: Record<string, string> = {
    immediate_forensics: translate('generatedScript.assistantRecommendedActionList_obtain_evidence_immediately_77c6fc'),
    temporary_containment: translate('generatedScript.assistantRecommendedActionList_temporary_disposal_381de2'),
    remediation: translate('generatedScript.assistantRecommendedActionList_repair_and_reinforcement_800888'),
    detection_enhancement: translate('generatedScript.assistantRecommendedActionList_detection_enhancement_9d9305'),
  }
  return map[category] || category
}

function riskTag(level: string): string {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'danger',
    medium: 'warning',
    low: 'info',
    readonly: 'info',
  }
  return map[level] || 'info'
}
</script>

<style scoped>
.recommended-actions {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
}

.section-header {
  margin-bottom: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.empty-hint {
  text-align: center;
  color: #909399;
  font-size: 13px;
  padding: 16px;
}

.action-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.action-item {
  padding: 12px;
  background: #f5f7fa;
  border-radius: 8px;
  border: 1px solid #e4e7ed;
}

.action-item.critical,
.action-item.high {
  border-left: 3px solid #f56c6c;
}

.action-item.medium {
  border-left: 3px solid #e6a23c;
}

.action-item.low,
.action-item.readonly {
  border-left: 3px solid #67c23a;
}

.action-header {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.action-title {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  margin-bottom: 4px;
}

.action-description {
  font-size: 13px;
  color: #606266;
  line-height: 1.5;
}

.action-approval-hint {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 8px;
  font-size: 12px;
  color: #e6a23c;
}
</style>

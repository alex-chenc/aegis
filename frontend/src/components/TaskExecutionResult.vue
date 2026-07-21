<template>
  <div v-if="displayResult" class="task-execution-result">
    <!-- 执行状态卡片 -->
    <div class="status-section">
      <div class="section-header">
        <el-icon><Monitor /></el-icon>
        <span>{{ $t('generated.taskExecutionResult_execution_status_141ab8') }}</span>
      </div>
      <div class="status-card">
        <div class="status-row">
          <span class="status-label">{{ $t('generated.common_task_status_b7d412') }}</span>
          <el-tag :type="executionStatusType(displayResult.status)" size="small">
            {{ displayResult.status }}
          </el-tag>
        </div>
        <div class="status-row">
          <span class="status-label">{{ $t('generated.taskExecutionResult_reason_for_exit_f2a226') }}</span>
          <span class="status-value">{{ displayResult.exit_reason }}</span>
        </div>
        <div v-if="displayResult.total_duration_ms > 0" class="status-row">
          <span class="status-label">{{ $t('generated.taskExecutionResult_total_time_spent_2db420') }}</span>
          <span class="status-value">{{ formatDuration(displayResult.total_duration_ms) }}</span>
        </div>
      </div>
    </div>

    <!-- 步骤结论 -->
    <div v-if="displayResult.steps.length > 0" class="steps-section">
      <div class="section-header">
        <el-icon><List /></el-icon>
        <span>{{ $t('generated.taskExecutionResult_step_execution_details_1d6e93') }}</span>
        <el-tag size="small" type="info">{{ completedStepCount }}/{{ displayResult.steps.length }}</el-tag>
      </div>
      <div class="steps-list">
        <div
          v-for="(step, index) in displayResult.steps"
          :key="step.step_id"
          class="step-card"
          :class="`step-${stepStatusType(step.status)}`"
        >
          <div class="step-header">
            <span class="step-index">{{ index + 1 }}</span>
            <span class="step-id">{{ step.step_id }}</span>
            <el-tag :type="stepStatusType(step.status)" size="small">{{ step.status }}</el-tag>
            <span v-if="step.duration_ms > 0" class="step-duration">{{ formatDuration(step.duration_ms) }}</span>
          </div>
          <div v-if="step.result" class="step-result">{{ step.result }}</div>
        </div>
      </div>
    </div>

    <!-- 分析结论 -->
    <div v-if="displayResult.conclusion" class="conclusion-section">
      <div class="section-header">
        <el-icon><Document /></el-icon>
        <span>{{ $t('generated.taskExecutionResult_analysis_conclusion_340c08') }}</span>
      </div>
      <div class="conclusion-card" :class="displayResult.conclusion.verdict">
        <div class="conclusion-header">
          <el-tag :type="getVerdictType(displayResult.conclusion.verdict)" size="large">
            {{ getVerdictText(displayResult.conclusion.verdict) }}
          </el-tag>
          <span v-if="displayResult.conclusion.summary" class="conclusion-summary">
            {{ displayResult.conclusion.summary }}
          </span>
        </div>
      </div>
    </div>

    <!-- 详细分析（从reasoning解析） -->
    <div v-if="parsedSections.length > 0" class="analysis-section">
      <div class="section-header">
        <el-icon><DataAnalysis /></el-icon>
        <span>{{ $t('generated.taskExecutionResult_detailed_analysis_64f5c5') }}</span>
      </div>
      <div v-for="(section, idx) in parsedSections" :key="idx" class="analysis-block">
        <div class="analysis-title">{{ section.title }}</div>
        <div class="analysis-content" v-html="formatAnalysisContent(section.content)"></div>
      </div>
    </div>

    <!-- 处置建议 -->
    <div v-if="remediationItems.length > 0" class="remediation-section">
      <div class="section-header">
        <el-icon><Warning /></el-icon>
        <span>{{ $t('generated.common_disposal_recommendations_3da99c') }}</span>
      </div>
      <div class="remediation-card">
        <div v-for="(item, idx) in remediationItems" :key="idx" class="remediation-item">
          <span class="remediation-index">{{ idx + 1 }}.</span>
          <span class="remediation-text">{{ item }}</span>
        </div>
      </div>
    </div>

    <!-- 错误信息 -->
    <div v-if="displayResult.errors.length > 0" class="errors-section">
      <div class="section-header">
        <el-icon><CircleClose /></el-icon>
        <span>{{ $t('generated.taskExecutionResult_error_message_f2f62e') }}{{ displayResult.errors.length }})</span>
      </div>
      <div class="errors-card">
        <div v-for="(err, idx) in displayResult.errors" :key="idx" class="error-item">
          {{ err }}
        </div>
      </div>
    </div>

    <!-- AI 自动阻断结果 -->
    <div v-if="props.aiAutoBlockResult?.triggered" class="ai-auto-block-section">
      <div class="section-header">
        <el-icon><Warning /></el-icon>
        <span>{{ $t('generated.taskExecutionResult_ai_automatic_blocking_9fc672') }}</span>
        <template v-if="props.aiAutoBlockResult.summary">
          <el-tag v-if="props.aiAutoBlockResult.summary.success > 0" type="success" size="small" style="margin-left: 6px;">
            {{ props.aiAutoBlockResult.summary.success }} {{ $t('generated.taskExecutionResult_a_success_4b9771') }}
          </el-tag>
          <el-tag v-if="props.aiAutoBlockResult.summary.failed > 0" type="danger" size="small" style="margin-left: 6px;">
            {{ props.aiAutoBlockResult.summary.failed }} {{ $t('generated.taskExecutionResult_a_failure_77b561') }}
          </el-tag>
          <el-tag v-if="props.aiAutoBlockResult.summary.skipped > 0" type="info" size="small" style="margin-left: 6px;">
            {{ props.aiAutoBlockResult.summary.skipped }} {{ $t('generated.taskExecutionResult_skipped_e9e54a') }}
          </el-tag>
        </template>
      </div>
      <div class="ai-auto-block-card">
        <div v-if="props.aiAutoBlockResult.results?.length" class="ai-auto-block-list">
          <div
            v-for="(item, idx) in props.aiAutoBlockResult.results"
            :key="idx"
            class="ai-auto-block-item"
            :class="item.status"
          >
            <div class="ai-auto-block-item-header">
              <el-tag :type="item.status === 'success' ? 'success' : item.status === 'failed' ? 'danger' : 'info'" size="small">
                {{ item.action || '-' }}
              </el-tag>
              <el-tag v-if="item.status === 'skipped'" type="info" size="small">{{ $t('generated.taskExecutionResult_jump_over_31a985') }}</el-tag>
              <span class="ai-auto-block-alert-id">{{ item.alert_id }}</span>
              <span v-if="item.target" class="ai-auto-block-target">{{ item.target }}</span>
            </div>
            <div class="ai-auto-block-message">{{ item.message }}</div>
          </div>
        </div>
        <div v-else-if="props.aiAutoBlockResult.reason" class="ai-auto-block-reason">
          {{ props.aiAutoBlockResult.reason }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed } from 'vue'
import { Monitor, List, Document, DataAnalysis, Warning, CircleClose } from '@element-plus/icons-vue'
import type { ExecutionResult, AIAutoBlockPayload } from '@/api/aiAnalysis'
import { normalizeExecutionResult, executionStatusType, stepStatusType } from '@/utils/taskExecutionResult'
import { getVerdictType, getVerdictText } from '@/utils/sessionStatus'

const props = defineProps<{
  result: ExecutionResult | null
  aiAutoBlockResult?: AIAutoBlockPayload | null
}>()

const displayResult = computed(() => {
  return props.result ? normalizeExecutionResult(props.result) : null
})

const completedStepCount = computed(() => {
  if (!displayResult.value) return 0
  return displayResult.value.steps.filter(s => stepStatusType(s.status) === 'success').length
})

interface AnalysisSection {
  title: string
  content: string
}

/**
 * 从conclusion.reasoning解析结构化段落
 * reasoning格式: 【段落标题】内容
 */
const parsedSections = computed<AnalysisSection[]>(() => {
  const reasoning = displayResult.value?.conclusion?.reasoning
  if (!reasoning) return []

  const sections: AnalysisSection[] = []
  const regex = /【([^】]+)】/g
  let match: RegExpExecArray | null
  const indices: { title: string; start: number }[] = []

  while ((match = regex.exec(reasoning)) !== null) {
    indices.push({ title: match[1], start: match.index + match[0].length })
  }

  for (let i = 0; i < indices.length; i++) {
    const end = i + 1 < indices.length ? reasoning.indexOf('【', indices[i].start) : reasoning.length
    const content = reasoning.substring(indices[i].start, end).trim()
    if (content) {
      // 跳过处置建议和判定结论（单独显示）
      if (indices[i].title.includes('处置建议') || /remediation|recommendation/i.test(indices[i].title)) continue
      if (indices[i].title.includes('判定结论') || /verdict|conclusion/i.test(indices[i].title)) continue
      sections.push({ title: indices[i].title, content })
    }
  }

  return sections
})

/**
 * 从conclusion.reasoning解析处置建议
 */
const remediationItems = computed<string[]>(() => {
  const reasoning = displayResult.value?.conclusion?.reasoning
  if (!reasoning) return []

  // 先尝试从【处置建议】段落提取
  const remediationMatch = reasoning.match(/【处置建议】([\s\S]*?)(?:【|$)/)
  if (remediationMatch) {
    const text = remediationMatch[1].trim()
    // 解析编号列表: 1. **xxx**：xxx 或 1. xxx
    const items = text
      .split(/\d+\.\s*/)
      .filter(Boolean)
      .map(s => s.replace(/\*\*/g, '').replace(/：/g, ': ').trim())
    if (items.length > 0) return items
  }

  // 如果没有结构化建议，根据verdict生成默认建议
  const verdict = displayResult.value?.conclusion?.verdict
  if (verdict === 'malicious') {
    return [translate('generatedScript.taskExecutionResult_immediately_isolate_the_affected_host_and_c9e767'), translate('generatedScript.taskExecutionResult_check_for_signs_of_lateral_movement_9abb1a'), translate('generatedScript.taskExecutionResult_troubleshooting_ssh_brute_force_cracking_source_406265'), translate('generatedScript.taskExecutionResult_disable_root_remote_login_e61409')]
  }
  if (verdict === 'suspicious') {
    return [translate('generatedScript.taskExecutionResult_further_monitoring_of_related_processes_and_949e9e'), translate('generatedScript.taskExecutionResult_gather_more_evidence_to_confirm_the_cd80db')]
  }
  return []
})

/**
 * 格式化分析内容：支持markdown加粗和换行
 */
function formatAnalysisContent(content: string): string {
  return content
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\n/g, '<br>')
}

function formatDuration(ms: number): string {
  if (ms < 1000) return translate('generatedScript.taskExecutionResult_ms_e9c462', { p0: ms })
  if (ms < 60000) return translate('generatedScript.taskExecutionResult_seconds_ad7dee', { p0: (ms / 1000).toFixed(1) })
  const minutes = Math.floor(ms / 60000)
  const seconds = Math.round((ms % 60000) / 1000)
  return translate('generatedScript.taskExecutionResult_minutes_seconds_b54206', { p0: minutes, p1: seconds })
}
</script>

<style scoped>
.task-execution-result {
  padding: 16px;
}

.section-header {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--el-text-color-primary);
}

/* 执行状态 */
.status-section {
  margin-bottom: 20px;
}

.status-card {
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
  padding: 12px 16px;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
}

.status-row + .status-row {
  border-top: 1px solid var(--el-border-color-extra-light);
}

.status-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.status-value {
  font-size: 13px;
  color: var(--el-text-color-primary);
  font-weight: 500;
}

/* 步骤结论 */
.steps-section {
  margin-bottom: 20px;
}

.steps-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.step-card {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 10px 14px;
  background: var(--el-bg-color);
}

.step-card.step-success {
  border-left: 3px solid var(--el-color-success);
}

.step-card.step-danger {
  border-left: 3px solid var(--el-color-danger);
  background: var(--el-color-danger-light-9);
}

.step-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.step-index {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--el-fill-color);
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
}

.step-id {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  font-family: monospace;
}

.step-duration {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-left: auto;
}

.step-result {
  font-size: 13px;
  color: var(--el-text-color-regular);
  line-height: 1.6;
  padding-left: 30px;
  white-space: pre-wrap;
}

/* 分析结论 */
.conclusion-section {
  margin-bottom: 20px;
}

.conclusion-card {
  border: 1px solid var(--el-border-color-lighter);
  border-left: 4px solid var(--el-color-info);
  border-radius: 8px;
  padding: 16px;
  background: var(--el-bg-color);
}

.conclusion-card.benign,
.conclusion-card.false_positive {
  border-left-color: var(--el-color-success);
}

.conclusion-card.malicious {
  border-left-color: var(--el-color-danger);
}

.conclusion-card.suspicious {
  border-left-color: var(--el-color-warning);
}

.conclusion-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.conclusion-summary {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

/* 详细分析 */
.analysis-section {
  margin-bottom: 20px;
}

.analysis-block {
  margin-bottom: 12px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;
}

.analysis-title {
  font-size: 13px;
  font-weight: 600;
  padding: 8px 14px;
  background: var(--el-fill-color-lighter);
  color: var(--el-text-color-primary);
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.analysis-content {
  font-size: 13px;
  line-height: 1.7;
  padding: 12px 14px;
  color: var(--el-text-color-regular);
}

.analysis-content :deep(strong) {
  color: var(--el-text-color-primary);
}

/* 处置建议 */
.remediation-section {
  margin-bottom: 20px;
}

.remediation-card {
  border: 1px solid var(--el-color-warning-light-7);
  border-left: 4px solid var(--el-color-warning);
  border-radius: 8px;
  padding: 14px 16px;
  background: var(--el-color-warning-light-9);
}

.remediation-item {
  display: flex;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-regular);
}

.remediation-index {
  color: var(--el-color-warning);
  font-weight: 600;
  flex-shrink: 0;
}

/* 错误信息 */
.errors-section {
  margin-bottom: 12px;
}

.errors-card {
  border: 1px solid var(--el-color-danger-light-7);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--el-color-danger-light-9);
}

.error-item {
  font-size: 12px;
  color: var(--el-color-danger);
  padding: 2px 0;
  font-family: monospace;
}

/* AI 自动阻断结果 */
.ai-auto-block-section {
  margin-bottom: 12px;
}

.ai-auto-block-card {
  border: 1px solid var(--el-border-color-lighter);
  border-left: 4px solid var(--el-color-primary);
  border-radius: 8px;
  padding: 14px 16px;
  background: var(--el-bg-color);
}

.ai-auto-block-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ai-auto-block-item {
  border: 1px solid var(--el-border-color-extra-light);
  border-radius: 6px;
  padding: 10px 12px;
  background: var(--el-fill-color-lighter);
}

.ai-auto-block-item.success {
  border-left: 3px solid var(--el-color-success);
}

.ai-auto-block-item.failed {
  border-left: 3px solid var(--el-color-danger);
}

.ai-auto-block-item.skipped {
  border-left: 3px solid var(--el-color-info);
}

.ai-auto-block-item-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.ai-auto-block-alert-id {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  font-family: monospace;
}

.ai-auto-block-target {
  font-size: 12px;
  color: var(--el-text-color-regular);
  margin-left: auto;
}

.ai-auto-block-message {
  font-size: 13px;
  color: var(--el-text-color-regular);
  line-height: 1.5;
}

.ai-auto-block-reason {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  font-style: italic;
}
</style>

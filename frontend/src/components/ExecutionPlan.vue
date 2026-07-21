<template>
  <div v-if="plan" class="execution-plan">
    <div class="plan-header" @click="toggleExpanded">
      <div class="plan-title">
        <el-icon><List /></el-icon>
        <span>{{ $t('generated.common_execution_plan_822376') }}</span>
        <el-tag size="small" type="info">{{ completedCount }}/{{ plan.steps.length }}</el-tag>
      </div>
      <el-icon class="expand-icon" :class="{ expanded: isExpanded }">
        <ArrowDown />
      </el-icon>
    </div>

    <div v-show="isExpanded" class="plan-body">
      <!-- Plan goal -->
      <div v-if="plan.goal && !titleOnly" class="plan-goal">
        <el-icon><Aim /></el-icon>
        <span>{{ plan.goal }}</span>
      </div>

      <!-- Steps list -->
      <div class="steps-list">
        <div
          v-for="(step, index) in plan.steps"
          :key="step.id"
          class="step-item"
          :class="[`step-${step.status}`]"
        >
          <div class="step-index">{{ index + 1 }}</div>
          <div class="step-info">
            <div class="step-desc" :title="stepTitle(step, index)">{{ stepTitle(step, index) }}</div>
            <div v-if="!titleOnly && step.tool_names?.length" class="step-tools">
              <el-tag
                v-for="tool in step.tool_names"
                :key="tool"
                size="small"
                type="info"
                class="tool-tag"
              >
                {{ tool }}
              </el-tag>
            </div>
            <div v-if="!titleOnly && step.result_summary" class="step-result">
              {{ step.result_summary }}
            </div>
          </div>
          <el-tag :type="statusTagType(step.status)" size="small" class="step-status">
            {{ statusLabel(step.status) }}
          </el-tag>
        </div>
      </div>

      <!-- Audit / Reflection / Correction timeline -->
      <div v-if="!titleOnly && timelineEvents.length > 0" class="timeline-section">
        <div class="timeline-title">
          <el-icon><Clock /></el-icon>
          <span>{{ $t('generated.executionPlan_analyze_events_095f08') }}</span>
        </div>
        <el-timeline>
          <el-timeline-item
            v-for="(evt, idx) in timelineEvents"
            :key="idx"
            :type="evt.type"
            :timestamp="evt.label"
            placement="top"
          >
            <div class="timeline-content">{{ evt.summary }}</div>
          </el-timeline-item>
        </el-timeline>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { ref, computed } from 'vue'
import { List, ArrowDown, Aim, Clock } from '@element-plus/icons-vue'
import type { PlanEvent, PlanStep, AuditEvent, ReflectionEvent, CorrectionEvent } from '@/api/aiAnalysis'

const props = defineProps<{
  plan: PlanEvent | null
  audits?: AuditEvent[]
  reflections?: ReflectionEvent[]
  corrections?: CorrectionEvent[]
  titleOnly?: boolean
}>()

const isExpanded = ref(true)

function toggleExpanded() {
  isExpanded.value = !isExpanded.value
}

const completedCount = computed(() => {
  if (!props.plan) return 0
  return props.plan.steps.filter(s => s.status === 'completed').length
})

const titleOnly = computed(() => props.titleOnly === true)

interface TimelineEntry {
  type: 'primary' | 'success' | 'warning' | 'danger'
  label: string
  summary: string
}

const timelineEvents = computed<TimelineEntry[]>(() => {
  const events: TimelineEntry[] = []

  if (props.audits) {
    for (const a of props.audits) {
      const riskLevel = a.risk_level || 'low'
      const decision = a.decision || translate('generatedScript.executionPlan_audit_completed_8ac138')
      const findings = Array.isArray(a.findings) ? a.findings : []
      events.push({
        type: riskLevel === 'high' ? 'danger' : riskLevel === 'medium' ? 'warning' : 'primary',
        label: translate('generatedScript.executionPlan_audit_e4e455', { p0: decision }),
        summary: findings.length > 0 ? findings.join('; ') : decision
      })
    }
  }

  if (props.reflections) {
    for (const r of props.reflections) {
      const rootCause = r.root_cause || r.summary || translate('generatedScript.executionPlan_reflection_completed_bc6009')
      const recommendation = r.recommendation || ''
      events.push({
        type: 'warning',
        label: translate('generatedScript.executionPlan_reflection_4086eb'),
        summary: recommendation ? `${rootCause} → ${recommendation}` : rootCause
      })
    }
  }

  if (props.corrections) {
    for (const c of props.corrections) {
      const reason = c.reason || c.summary || translate('generatedScript.executionPlan_correction_completed_fc3d15')
      const actions = Array.isArray(c.actions) ? c.actions : []
      events.push({
        type: 'success',
        label: translate('generatedScript.executionPlan_correct_500885'),
        summary: actions.length > 0 ? `${reason}: ${actions.join(', ')}` : reason
      })
    }
  }

  return events
})

function statusTagType(status: string): string {
  const map: Record<string, string> = {
    completed: 'success',
    running: '',
    waiting_approval: 'warning',
    retrying: 'warning',
    failed: 'danger',
    skipped: 'info',
    replaced: 'warning',
    invalidated: 'info',
    pending: 'info'
  }
  return map[status] || 'info'
}

function statusLabel(status: string): string {
  const map: Record<string, string> = {
    completed: translate('generatedScript.common_finish_33246f'),
    running: translate('generatedScript.common_executing_1f425b'),
    waiting_approval: translate('generatedScript.common_pending_approval_57fce0'),
    retrying: translate('generatedScript.executionPlan_retrying_b78013'),
    failed: translate('generatedScript.common_fail_3e3c80'),
    skipped: translate('generatedScript.executionPlan_jump_over_31a985'),
    replaced: translate('generatedScript.executionPlan_replace_855241'),
    invalidated: translate('generatedScript.executionPlan_invalid_eb645a'),
    pending: translate('generatedScript.common_to_be_executed_6cf0af')
  }
  return map[status] || status
}

function stepTitle(step: PlanStep, index: number) {
  return step.title || step.description || step.objective || translate('generatedScript.executionPlan_step_2b7b92', { p0: index + 1 })
}
</script>

<style scoped>
.execution-plan {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  margin-bottom: 12px;
  background: var(--el-bg-color);
}

.plan-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  cursor: pointer;
  user-select: none;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.plan-header:hover {
  background: var(--el-fill-color-lighter);
}

.plan-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 14px;
}

.expand-icon {
  transition: transform 0.2s;
}

.expand-icon.expanded {
  transform: rotate(180deg);
}

.plan-body {
  padding: 12px 14px;
}

.plan-goal {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-bottom: 12px;
  padding: 8px 10px;
  background: var(--el-fill-color-lighter);
  border-radius: 6px;
  font-size: 13px;
  color: var(--el-text-color-regular);
}

.steps-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.step-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid var(--el-border-color-extra-light);
  transition: background 0.15s;
}

.step-item:hover {
  background: var(--el-fill-color-lighter);
}

.step-item.step-running {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.step-item.step-retrying {
  border-color: var(--el-color-warning);
  background: var(--el-color-warning-light-9);
}

.step-item.step-completed {
  border-color: var(--el-color-success-light-5);
}

.step-item.step-failed {
  border-color: var(--el-color-danger-light-5);
  background: var(--el-color-danger-light-9);
}

.step-index {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--el-fill-color);
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
}

.step-info {
  flex: 1;
  min-width: 0;
}

.step-desc {
  font-size: 13px;
  color: var(--el-text-color-primary);
  line-height: 1.45;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.step-tools {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}

.tool-tag {
  font-size: 11px;
}

.step-result {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.step-status {
  flex-shrink: 0;
  margin-top: 2px;
}

.timeline-section {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-extra-light);
}

.timeline-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 10px;
}

.timeline-content {
  font-size: 13px;
  color: var(--el-text-color-regular);
}
</style>

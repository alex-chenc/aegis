<template>
  <div v-if="plan" class="execution-plan">
    <div class="plan-header" @click="toggleExpanded">
      <div class="plan-title">
        <el-icon><List /></el-icon>
        <span>执行计划</span>
        <el-tag size="small" type="info">{{ completedCount }}/{{ plan.steps.length }}</el-tag>
      </div>
      <el-icon class="expand-icon" :class="{ expanded: isExpanded }">
        <ArrowDown />
      </el-icon>
    </div>

    <div v-show="isExpanded" class="plan-body">
      <!-- Plan goal -->
      <div v-if="plan.goal" class="plan-goal">
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
            <div class="step-desc">{{ step.description }}</div>
            <div v-if="step.tool_names?.length" class="step-tools">
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
            <div v-if="step.result_summary" class="step-result">
              {{ step.result_summary }}
            </div>
          </div>
          <el-tag :type="statusTagType(step.status)" size="small" class="step-status">
            {{ statusLabel(step.status) }}
          </el-tag>
        </div>
      </div>

      <!-- Audit / Reflection / Correction timeline -->
      <div v-if="timelineEvents.length > 0" class="timeline-section">
        <div class="timeline-title">
          <el-icon><Clock /></el-icon>
          <span>分析事件</span>
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
import { ref, computed } from 'vue'
import { List, ArrowDown, Aim, Clock } from '@element-plus/icons-vue'
import type { PlanEvent, AuditEvent, ReflectionEvent, CorrectionEvent } from '@/api/aiAnalysis'

const props = defineProps<{
  plan: PlanEvent | null
  audits?: AuditEvent[]
  reflections?: ReflectionEvent[]
  corrections?: CorrectionEvent[]
}>()

const isExpanded = ref(true)

function toggleExpanded() {
  isExpanded.value = !isExpanded.value
}

const completedCount = computed(() => {
  if (!props.plan) return 0
  return props.plan.steps.filter(s => s.status === 'completed').length
})

interface TimelineEntry {
  type: 'primary' | 'success' | 'warning' | 'danger'
  label: string
  summary: string
}

const timelineEvents = computed<TimelineEntry[]>(() => {
  const events: TimelineEntry[] = []

  if (props.audits) {
    for (const a of props.audits) {
      events.push({
        type: a.risk_level === 'high' ? 'danger' : a.risk_level === 'medium' ? 'warning' : 'primary',
        label: `审计 - ${a.decision}`,
        summary: (a.findings || []).join('; ') || a.decision
      })
    }
  }

  if (props.reflections) {
    for (const r of props.reflections) {
      events.push({
        type: 'warning',
        label: '反思',
        summary: `${r.root_cause} → ${r.recommendation}`
      })
    }
  }

  if (props.corrections) {
    for (const c of props.corrections) {
      events.push({
        type: 'success',
        label: '纠正',
        summary: `${c.reason}${c.actions?.length ? ': ' + c.actions.join(', ') : ''}`
      })
    }
  }

  return events
})

function statusTagType(status: string): string {
  const map: Record<string, string> = {
    completed: 'success',
    running: '',
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
    completed: '完成',
    running: '执行中',
    failed: '失败',
    skipped: '跳过',
    replaced: '替换',
    invalidated: '无效',
    pending: '待执行'
  }
  return map[status] || status
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

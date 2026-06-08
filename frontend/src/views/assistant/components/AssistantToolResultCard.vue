<template>
  <div class="tool-result-card tool-call-card" :class="`status-${toolCall.status}`">
    <div class="tool-result-header">
      <div class="tool-main">
        <div class="tool-icon">
          <el-icon><SetUp /></el-icon>
        </div>
        <div class="tool-heading">
          <div class="tool-name">{{ toolCall.tool_name }}</div>
          <div class="tool-meta">
            <el-tag size="small" :type="statusTagType">{{ statusLabel }}</el-tag>
            <el-tag size="small" :type="riskTagType" effect="plain">{{ riskLabel }}</el-tag>
            <span v-if="durationLabel" class="duration">{{ durationLabel }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="summary" class="tool-summary">{{ summary }}</div>

    <AssistantTaskProgressCard
      v-if="hasTaskRef"
      :tool-call="toolCall"
      class="task-progress"
    />

    <div
      v-if="detailText"
      class="tool-call-result"
      :class="{ 'is-json': isJsonDetail }"
    >
      {{ displayedDetailText }}
    </div>

    <button
      v-if="isLongDetail"
      type="button"
      class="tool-result-toggle"
      @click="expanded = !expanded"
    >
      {{ expanded ? '收起结果' : '展开完整结果' }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { SetUp } from '@element-plus/icons-vue'
import AssistantTaskProgressCard from './AssistantTaskProgressCard.vue'
import type { AssistantToolCall } from '@/api/assistant'

const props = defineProps<{
  toolCall: AssistantToolCall
}>()

const DETAIL_PREVIEW_LENGTH = 560
const expanded = ref(false)
const normalizedResult = computed<Record<string, any>>(() => normalizeResult(props.toolCall.result))

const hasTaskRef = computed(() => Boolean(
  normalizedResult.value.task_ref ||
  normalizedResult.value.task_group_id ||
  normalizedResult.value.task_id ||
  normalizedResult.value.scan_id
))

const summary = computed(() => {
  if (props.toolCall.error_message) return props.toolCall.error_message
  if (props.toolCall.result_summary) return props.toolCall.result_summary

  const result = normalizedResult.value
  if (!Object.keys(result).length) {
    return props.toolCall.status === 'running' ? '工具正在执行，等待返回结果。' : ''
  }

  if (result.message) return String(result.message)
  if (result.summary && typeof result.summary === 'string') return result.summary
  if (result.task_ref) {
    return taskRefSummary(result.task_ref)
  }
  if (result.scan_id) {
    return `漏洞扫描已启动，扫描ID：${shortId(String(result.scan_id))}`
  }
  if (result.task_group_id) {
    return `任务组已创建：${shortId(String(result.task_group_id))}`
  }
  if (typeof result.total === 'number') {
    return `查询完成，共 ${result.total} 条结果。`
  }
  if (Array.isArray(result.data)) {
    return `查询完成，返回 ${result.data.length} 条结果。`
  }
  if (result.status) {
    return `执行状态：${result.status}`
  }
  return '工具执行完成，详情已折叠。'
})

const detailText = computed(() => {
  const value = props.toolCall.error_message
    ? { error: props.toolCall.error_message }
    : props.toolCall.result || props.toolCall.result_summary
  if (!value) return ''
  try {
    return typeof value === 'string'
      ? formatStringDetail(value)
      : JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
})

const isJsonDetail = computed(() => {
  const value = props.toolCall.result
  if (typeof value === 'object' && value !== null) return true
  if (typeof value !== 'string') return false
  const trimmed = value.trim()
  if (!trimmed) return false
  return (trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))
})

const isLongDetail = computed(() => detailText.value.length > DETAIL_PREVIEW_LENGTH)

const displayedDetailText = computed(() => {
  if (!isLongDetail.value || expanded.value) return detailText.value
  return `${detailText.value.slice(0, DETAIL_PREVIEW_LENGTH)}\n...`
})

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    pending: '等待中',
    running: '执行中',
    completed: '成功',
    success: '成功',
    failed: '失败',
    approval_required: '待审批',
    rejected: '已拒绝',
    cancelled: '已取消',
  }
  return map[props.toolCall.status] || props.toolCall.status
})

const statusTagType = computed(() => {
  switch (props.toolCall.status) {
    case 'completed':
    case 'success':
      return 'success'
    case 'running':
    case 'approval_required':
      return 'warning'
    case 'failed':
      return 'danger'
    default:
      return 'info'
  }
})

const riskLabel = computed(() => {
  const map: Record<string, string> = {
    readonly: '只读',
    low: '低风险',
    medium: '中风险',
    high: '高风险',
    critical: '严重风险',
  }
  return map[props.toolCall.risk_level] || props.toolCall.risk_level
})

const riskTagType = computed(() => {
  const map: Record<string, string> = {
    readonly: 'info',
    low: 'success',
    medium: 'warning',
    high: 'danger',
    critical: 'danger',
  }
  return map[props.toolCall.risk_level] || 'info'
})

const durationLabel = computed(() => {
  const ms = props.toolCall.duration_ms || 0
  if (!ms) return ''
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
})

function normalizeResult(input: any): Record<string, any> {
  if (!input) return {}
  if (typeof input === 'string') {
    try {
      const parsed = JSON.parse(input)
      return parsed && typeof parsed === 'object' ? parsed : {}
    } catch {
      return {}
    }
  }
  return typeof input === 'object' ? input : {}
}

function formatStringDetail(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return ''
  if ((trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return value
    }
  }
  return value
}

function taskRefSummary(ref: any) {
  const kind = String(ref.kind || '')
  const id = String(ref.task_group_id || ref.id || '')
  const map: Record<string, string> = {
    asset_collection: '资产采集任务已创建',
    baseline_task: '基线任务组已创建',
    vulnerability_scan: '漏洞扫描已启动',
    vulnerability_task: '漏洞脚本任务已创建',
  }
  const prefix = map[kind] || '任务已创建'
  return id ? `${prefix}：${shortId(id)}` : prefix
}

function shortId(value: string) {
  return value.length > 16 ? `${value.slice(0, 8)}...${value.slice(-6)}` : value
}
</script>

<style scoped>
.tool-result-card {
  background: #fff;
  border: 1px solid #dbe4ef;
  border-radius: 8px;
  padding: 12px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.05);
}

.tool-result-card.status-running {
  border-color: #facc15;
  background: #fffbeb;
}

.tool-result-card.status-success,
.tool-result-card.status-completed {
  border-color: #86efac;
}

.tool-result-card.status-failed {
  border-color: #fca5a5;
  background: #fef2f2;
}

.tool-result-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.tool-main {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 0;
}

.tool-icon {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
  color: #0f766e;
  background: #ccfbf1;
}

.tool-heading {
  min-width: 0;
}

.tool-name {
  color: #1f2937;
  font-size: 14px;
  font-weight: 750;
  overflow-wrap: anywhere;
}

.tool-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 5px;
}

.duration {
  color: #64748b;
  font-size: 12px;
}

.tool-summary {
  margin-top: 10px;
  color: #334155;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.task-progress {
  margin-top: 10px;
}

.tool-call-result {
  margin-top: 10px;
  max-height: 280px;
  overflow: auto;
  padding: 10px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #f8fafc;
  color: #334155;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-call-result.is-json {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
}

.tool-result-toggle {
  margin-top: 8px;
  padding: 0;
  border: none;
  background: transparent;
  color: #2563eb;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.tool-result-toggle:hover {
  color: #1d4ed8;
}
</style>

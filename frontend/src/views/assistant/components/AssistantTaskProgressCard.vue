<template>
  <div v-if="taskRef" class="task-progress-card">
    <div class="task-progress-header">
      <div>
        <div class="task-title">{{ taskTitle }}</div>
        <div class="task-id">{{ displayId }}</div>
      </div>
      <el-tag :type="statusTagType" size="small">{{ statusLabel }}</el-tag>
    </div>

    <el-progress
      :percentage="progressPercent"
      :status="progressStatus"
      :stroke-width="8"
      :show-text="false"
    />

    <div class="task-metrics">
      <div class="metric">
        <span>总数</span>
        <strong>{{ progress.total }}</strong>
      </div>
      <div class="metric">
        <span>成功</span>
        <strong>{{ progress.success }}</strong>
      </div>
      <div class="metric">
        <span>运行</span>
        <strong>{{ progress.running }}</strong>
      </div>
      <div class="metric">
        <span>失败</span>
        <strong>{{ progress.failed }}</strong>
      </div>
    </div>

    <div v-if="progress.message" class="task-message">{{ progress.message }}</div>

    <div v-if="latestLog" class="task-log">
      <span>{{ latestLog.hostname || latestLog.host_id || latestLog.id }}</span>
      <em>{{ latestLog.status }}</em>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { getCollectionTask } from '@/api/assets'
import { getTaskLogs, getTaskStatus, type TaskLog } from '@/api/tasks'
import { getScanStatus } from '@/api/vulnerability'
import type { AssistantToolCall } from '@/api/assistant'

type TaskRef = {
  kind: string
  id?: string
  task_group_id?: string
  route_path?: string
  status_url?: string
}

type ProgressState = {
  status: string
  total: number
  success: number
  running: number
  pending: number
  failed: number
  timeout: number
  message: string
  percent?: number
}

const props = defineProps<{
  toolCall: AssistantToolCall
}>()

const progress = ref<ProgressState>({
  status: 'pending',
  total: 0,
  success: 0,
  running: 0,
  pending: 0,
  failed: 0,
  timeout: 0,
  message: '',
})
const latestLog = ref<Partial<TaskLog> | null>(null)
let pollTimer: ReturnType<typeof window.setInterval> | null = null

const result = computed<Record<string, any>>(() => normalizeResult(props.toolCall.result))
const taskRef = computed<TaskRef | null>(() => resolveTaskRef(result.value))

const displayId = computed(() => {
  const ref = taskRef.value
  if (!ref) return ''
  const id = ref.task_group_id || ref.id || ''
  return id.length > 16 ? `${id.slice(0, 8)}...${id.slice(-6)}` : id
})

const taskTitle = computed(() => {
  const kind = taskRef.value?.kind
  const map: Record<string, string> = {
    asset_collection: '资产采集进度',
    baseline_task: '基线任务进度',
    vulnerability_scan: '漏洞扫描进度',
    vulnerability_task: '漏洞脚本任务',
  }
  return map[kind || ''] || '任务进度'
})

const progressPercent = computed(() => {
  if (typeof progress.value.percent === 'number') {
    return clampPercent(progress.value.percent)
  }
  if (!progress.value.total) return 0
  const done = progress.value.success + progress.value.failed + progress.value.timeout
  return clampPercent(Math.round((done / progress.value.total) * 100))
})

const statusLabel = computed(() => {
  const status = String(progress.value.status || '').toLowerCase()
  const map: Record<string, string> = {
    pending: '等待中',
    running: '运行中',
    collecting: '采集中',
    scanning: '扫描中',
    analyzing: '分析中',
    success: '成功',
    completed: '完成',
    failed: '失败',
    timeout: '超时',
    stopped: '已停止',
    stopping: '停止中',
  }
  return map[status] || status || '等待中'
})

const statusTagType = computed(() => {
  const status = String(progress.value.status || '').toLowerCase()
  if (['success', 'completed'].includes(status)) return 'success'
  if (['failed', 'timeout'].includes(status)) return 'danger'
  if (['running', 'pending', 'collecting', 'scanning', 'analyzing', 'stopping'].includes(status)) return 'warning'
  return 'info'
})

const progressStatus = computed<'success' | 'exception' | 'warning' | undefined>(() => {
  const status = String(progress.value.status || '').toLowerCase()
  if (['success', 'completed'].includes(status)) return 'success'
  if (['failed', 'timeout'].includes(status)) return 'exception'
  return undefined
})

watch(taskRef, () => {
  stopPolling()
  if (taskRef.value) {
    void pollOnce()
    pollTimer = window.setInterval(() => void pollOnce(), 2500)
  }
}, { immediate: true, deep: true })

onUnmounted(stopPolling)

async function pollOnce() {
  const ref = taskRef.value
  if (!ref) return
  try {
    if (ref.kind === 'asset_collection') {
      await pollAssetCollection(ref)
    } else if (ref.kind === 'vulnerability_scan') {
      await pollVulnerabilityScan(ref)
    } else {
      await pollTaskGroup(ref)
    }
    if (isTerminal(progress.value.status)) {
      stopPolling()
    }
  } catch {
    progress.value.message = '进度暂时不可用'
  }
}

async function pollTaskGroup(ref: TaskRef) {
  const taskGroupId = ref.task_group_id || ref.id
  if (!taskGroupId) return
  const [status, logs] = await Promise.all([
    getTaskStatus(taskGroupId),
    getTaskLogs(taskGroupId).catch(() => [] as TaskLog[]),
  ])
  progress.value = {
    status: status.status,
    total: status.total,
    success: status.success,
    running: status.running,
    pending: status.pending,
    failed: status.failed,
    timeout: status.timeout || 0,
    message: status.total ? '' : '任务已创建，等待执行明细',
  }
  latestLog.value = logs[0] || null
}

async function pollAssetCollection(ref: TaskRef) {
  if (!ref.id) return
  const detail = await getCollectionTask(ref.id)
  const task = detail.task
  const hosts = detail.hosts || []
  const running = hosts.filter((host: any) => ['collecting', 'running'].includes(String(host.status).toLowerCase())).length
  progress.value = {
    status: task.status,
    total: task.total_hosts,
    success: task.success_hosts,
    running,
    pending: Math.max(task.total_hosts - task.success_hosts - task.failed_hosts - running, 0),
    failed: task.failed_hosts,
    timeout: 0,
    message: task.current_stage || task.error_message || '',
  }
}

async function pollVulnerabilityScan(ref: TaskRef) {
  if (!ref.id) return
  const status = await getScanStatus(ref.id)
  progress.value = {
    status: status.status,
    total: status.total_hosts,
    success: status.scanned_hosts,
    running: ['pending', 'scanning', 'analyzing'].includes(status.status) ? 1 : 0,
    pending: Math.max(status.total_hosts - status.scanned_hosts, 0),
    failed: status.status === 'failed' ? 1 : 0,
    timeout: 0,
    message: status.message || status.error_message || '',
    percent: status.progress,
  }
}

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

function resolveTaskRef(input: Record<string, any>): TaskRef | null {
  if (input.task_ref && typeof input.task_ref === 'object') {
    return input.task_ref as TaskRef
  }
  if (input.task_group_id) {
    return {
      kind: input.task_type === 'POC_VERIFY' || input.task_type === 'VULNERABILITY_FIX'
        ? 'vulnerability_task'
        : 'baseline_task',
      id: String(input.task_group_id),
      task_group_id: String(input.task_group_id),
    }
  }
  if (input.task_kind === 'asset_collection' && input.task_id) {
    return { kind: 'asset_collection', id: String(input.task_id) }
  }
  if (input.scan_id) {
    return { kind: 'vulnerability_scan', id: String(input.scan_id) }
  }
  return null
}

function clampPercent(value: number) {
  return Math.max(0, Math.min(100, value))
}

function isTerminal(status: string) {
  return ['success', 'completed', 'failed', 'timeout', 'stopped', 'cancelled'].includes(String(status || '').toLowerCase())
}

function stopPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}
</script>

<style scoped>
.task-progress-card {
  padding: 12px;
  border: 1px solid #dbe4ef;
  border-radius: 8px;
  background: #f8fafc;
}

.task-progress-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.task-title {
  color: #1f2937;
  font-size: 13px;
  font-weight: 700;
}

.task-id {
  margin-top: 2px;
  color: #64748b;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 11px;
}

.task-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  margin-top: 10px;
}

.metric {
  min-width: 0;
  padding: 7px 8px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #fff;
}

.metric span {
  display: block;
  color: #64748b;
  font-size: 11px;
}

.metric strong {
  color: #1f2937;
  font-size: 14px;
}

.task-message,
.task-log {
  margin-top: 9px;
  color: #475569;
  font-size: 12px;
  line-height: 1.5;
}

.task-log {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding-top: 8px;
  border-top: 1px solid #e2e8f0;
}

.task-log span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-log em {
  flex-shrink: 0;
  color: #64748b;
  font-style: normal;
}
</style>

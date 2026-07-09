<template>
  <div class="task-detail">
    <el-page-header @back="goBack" title="返回" :content="'任务详情: ' + taskGroupId" />

    <el-card style="margin-top: 20px" v-loading="loading">
      <template #header>
        <div class="card-header">
          <span>任务进度</span>
          <el-button @click="refresh" :loading="refreshing">刷新</el-button>
        </div>
      </template>

      <el-row :gutter="20" v-if="status">
        <el-col :span="3">
          <el-statistic title="总任务数" :value="status.total" />
        </el-col>
        <el-col :span="3">
          <el-statistic title="待执行" :value="status.pending" />
        </el-col>
        <el-col :span="3">
          <el-statistic title="执行中" :value="status.running" />
        </el-col>
        <el-col :span="3">
          <el-statistic title="已完成" :value="status.success + status.failed" />
        </el-col>
        <el-col :span="3">
          <el-statistic title="超时" :value="status.timeout || 0" />
        </el-col>
        <el-col :span="6">
          <el-progress
            :percentage="progressPercent"
            :status="progressStatus"
            :stroke-width="20"
            style="margin-top: 10px"
          />
        </el-col>
      </el-row>
    </el-card>

    <el-card style="margin-top: 20px">
      <template #header>
        <div class="card-header">
          <span>任务列表</span>
          <div style="display: flex; align-items: center; gap: 8px;">
            <span style="font-size: 13px; color: #666;">类型筛选：</span>
            <el-select v-model="typeFilter" size="small" style="width: 120px;">
              <el-option label="全部" value="all" />
              <el-option v-if="!isVulnerabilityTask" label="检测" value="CHECK" />
              <el-option v-if="!isVulnerabilityTask" label="修复" value="FIX" />
              <el-option v-if="isVulnerabilityTask" label="POC验证" value="POC_VERIFY" />
              <el-option v-if="isVulnerabilityTask" label="漏洞修复" value="VULNERABILITY_FIX" />
            </el-select>
          </div>
        </div>
      </template>

      <el-table :data="tasksWithState" style="width: 100%">
        <el-table-column prop="rule_title" label="规则标题" min-width="180">
          <template #default="{ row }">
            {{ getRuleTitle(row) }}
          </template>
        </el-table-column>
        <el-table-column prop="hostname" label="主机" min-width="150">
          <template #default="{ row }">
            {{ getHostname(row.host_id) }}
          </template>
        </el-table-column>
        <el-table-column prop="task_type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="getTaskTypeTag(row.task_type)" size="small">
              {{ getTaskTypeLabel(row.task_type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="140">
          <template #default="{ row }">
            <el-tooltip
              v-if="row.displayState === '大模型修复失败' || row.displayState === '检测失败' || row.displayState === '修复失败'"
              :content="row.healingStatus?.last_error || row.stderr || '未知错误'"
              placement="top"
            >
              <el-tag :type="getStateTagType(row.displayState)" size="small">
                {{ row.displayState }}
              </el-tag>
            </el-tooltip>
            <el-tooltip
              v-else-if="row.displayState === '审计未通过'"
              :content="row.audit_info?.error_message || '脚本存在恶意命令，下发已阻止'"
              placement="top"
            >
              <el-tag type="danger" size="small">
                {{ row.displayState }}
              </el-tag>
            </el-tooltip>
            <el-tag v-else :type="getStateTagType(row.displayState)" size="small">
              {{ row.displayState }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="脚本" width="100">
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              size="small"
              @click="showScript(row, 'script')"
              :disabled="row.displayState === '大模型修复中'"
            >
              {{ row.displayState === '大模型修复中' ? '修复中' : '查看脚本' }}
            </el-button>
          </template>
        </el-table-column>
        <el-table-column label="结果" width="100">
          <template #default="{ row }">
            <el-button
              v-if="row.stdout || row.stderr"
              link
              type="primary"
              size="small"
              @click="showScript(row, 'result')"
            >
              查看结果
            </el-button>
            <span v-else style="color: #999">-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button
              link
              type="danger"
              size="small"
              @click="deleteTask(row)"
              :disabled="row.status === 'running' || row.status === 'pending' || row.displayState === '大模型修复中'"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card v-if="healingProcessTasks.length" style="margin-top: 20px">
      <template #header>
        <span>自动修复过程</span>
      </template>
      <div class="healing-process-list">
        <section v-for="task in healingProcessTasks" :key="task.id" class="healing-process-item">
          <div class="healing-process-header">
            <div>
              <strong>{{ getRuleTitle(task) }}</strong>
              <span>{{ getHostname(task.host_id) }} · {{ getTaskTypeLabel(task.task_type) }}</span>
            </div>
            <el-tag :type="task.healingStatus?.status === 'healed' ? 'success' : task.healingStatus?.status === 'failed' || task.healingStatus?.status === 'timeout' ? 'danger' : 'warning'">
              {{ healingStatusText(task.healingStatus?.status) }}
            </el-tag>
          </div>
          <div class="healing-process-meta">
            <span>当前轮次 {{ task.healingStatus?.total_attempts || 0 }} / {{ task.healingStatus?.max_attempts || 0 }}</span>
            <span v-if="task.healingStatus?.concurrency_limit">并发上限 {{ task.healingStatus.concurrency_limit }}</span>
            <span v-if="task.healingStatus?.queue_position">排队 {{ task.healingStatus.queue_position }}</span>
          </div>
          <ol v-if="task.healingStatus?.steps?.length" class="healing-steps">
            <li v-for="(step, index) in task.healingStatus.steps" :key="index">
              <strong>{{ step.phase }}</strong>
              <span>{{ step.summary }}</span>
              <em>{{ healingStepStatusText(step.status) }}</em>
            </li>
          </ol>
          <p v-if="task.healingStatus?.last_error" class="healing-error">{{ task.healingStatus.last_error }}</p>
        </section>
      </div>
    </el-card>

    <el-card v-if="tasksWithState.some(t => t.audit_info)" style="margin-top: 20px">
      <template #header>
        <span>审计拦截信息</span>
      </template>
      <div v-for="task in tasksWithState.filter(t => t.audit_info)" :key="task.id" style="margin-bottom: 16px; padding: 12px; background: #fef2f2; border-radius: 4px; border: 1px solid #fecaca">
        <div style="font-weight: 600; margin-bottom: 8px">{{ getRuleTitle(task) }} - {{ getHostname(task.host_id) }}</div>
        <div v-if="task.audit_info?.error_message" style="color: #dc2626; margin-bottom: 8px">{{ task.audit_info.error_message }}</div>
        <div v-if="task.audit_info?.hit_rules?.length" style="margin-bottom: 8px">
          <div style="font-size: 13px; color: #666; margin-bottom: 4px">命中规则:</div>
          <div v-for="(rule, i) in task.audit_info.hit_rules" :key="i" style="font-size: 13px; padding: 2px 0">
            <el-tag :type="rule.severity === 'critical' ? 'danger' : 'warning'" size="small">{{ rule.severity }}</el-tag>
            <span style="margin-left: 8px">{{ rule.rule_name }} (第{{ rule.line_number }}行)</span>
          </div>
        </div>
        <el-button
          v-if="task.audit_info?.audit_log_id"
          link
          type="primary"
          size="small"
          @click="router.push('/settings/audit-logs')"
        >
          查看审计日志
        </el-button>
      </div>
    </el-card>

    <el-dialog v-model="scriptDialogVisible" :title="scriptDialogTitle" width="70%">
      <div class="script-viewer">
        <pre class="script-content">{{ currentScript }}</pre>
      </div>
    </el-dialog>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getTaskLogs,
  getTaskStatus,
  deleteTask as deleteTaskApi,
  normalizeType,
  normalizeStatus,
  type TaskLog,
  type TaskGroupStatus,
  type HealingStatus
} from '@/api/tasks'
import { useHostStore } from '@/store/hosts'

type DisplayState = '检测中' | '通过' | '未通过' | '检测失败' | '修复中' | '修复成功' | '修复失败' | '大模型修复中' | '大模型修复成功' | '大模型修复失败' | '大模型修复超时' | '检测超时' | '修复超时' | 'POC验证中' | 'POC验证成功' | 'POC验证失败' | '漏洞修复中' | '漏洞修复成功' | '漏洞修复失败' | '审计未通过'

const route = useRoute()
const router = useRouter()
const hostStore = useHostStore()

const taskGroupId = route.params.id as string
const loading = ref(false)
const refreshing = ref(false)
const tasks = ref<TaskLog[]>([])
const healingStatusMap = ref<Record<string, HealingStatus>>({})
const status = ref<TaskGroupStatus | null>(null)
const scriptDialogVisible = ref(false)
const scriptDialogTitle = ref('')
const currentScript = ref('')
const typeFilter = ref<string>('all')
let pollTimer: number | null = null

const isVulnerabilityTask = computed(() => route.path.startsWith('/vulnerability/tasks'))
const taskCenterPath = computed(() =>
  isVulnerabilityTask.value ? '/vulnerability/tasks' : '/baseline/tasks'
)

const tasksWithState = computed(() => {
  return tasks.value
    .filter(task => typeFilter.value === 'all' || normalizeType(task.task_type) === typeFilter.value)
    .map(task => {
      const healingStatus = healingStatusMap.value[task.id]
      const displayState = getDisplayState(normalizeType(task.task_type), normalizeStatus(task.status), task.exit_code, healingStatus)
      return {
        ...task,
        displayState,
        healingStatus
      }
    })
})

const groupPassRate = computed(() => {
  if (!tasks.value.length) return 0
  const terminalTasks = tasks.value.filter(task => isTaskTerminal(task))
  if (!terminalTasks.length) return 0
  const passed = terminalTasks.filter(task => isTaskPassed(task)).length
  return Math.round((passed / terminalTasks.length) * 100)
})

const healingProcessTasks = computed(() =>
  tasksWithState.value.filter(task => task.healingStatus)
)

function getTaskTypeTag(type: string): string {
  const normalized = normalizeType(type)
  switch (normalized) {
    case 'CHECK': return 'primary'
    case 'FIX': return 'warning'
    case 'POC_VERIFY': return 'success'
    case 'VULNERABILITY_FIX': return 'danger'
    default: return 'info'
  }
}

function getTaskTypeLabel(type: string): string {
  const normalized = normalizeType(type)
  switch (normalized) {
    case 'CHECK': return '检测'
    case 'FIX': return '修复'
    case 'POC_VERIFY': return 'POC验证'
    case 'VULNERABILITY_FIX': return '漏洞修复'
    default: return type
  }
}

function getDisplayState(taskType: string, taskStatus: string, exitCode: number | undefined, healingStatus?: HealingStatus): DisplayState {
  const isFix = taskType === 'FIX' || taskType === 'VULNERABILITY_FIX'
  const isPoc = taskType === 'POC_VERIFY'

  if (taskStatus === 'running' || taskStatus === 'pending') {
    if (isPoc) return 'POC验证中'
    if (isFix) return '修复中'
    return '检测中'
  }
  if (taskStatus === 'timeout') {
    if (isPoc) return '检测超时'
    if (isFix) return '修复超时'
    return '检测超时'
  }
  if (taskStatus === 'audit_blocked') {
    return '审计未通过'
  }
  if (taskStatus === 'success') {
    if (isPoc) return exitCode === 0 ? 'POC验证成功' : 'POC验证失败'
    if (isFix) return (exitCode ?? 0) === 0 ? '修复成功' : '修复失败'
    if (exitCode === 0) return '通过'
    return '未通过'
  }
  if (taskStatus === 'failed') {
    if (!healingStatus) {
      if (isPoc) return 'POC验证失败'
      if (isFix) return '修复失败'
      return '检测失败'
    }
    if (healingStatus.status === 'healing') return '大模型修复中'
    if (healingStatus.status === 'queued') return '大模型修复中'
    if (healingStatus.status === 'healed') return '大模型修复成功'
    if (healingStatus.status === 'failed') return '大模型修复失败'
    if (healingStatus.status === 'timeout') return '大模型修复超时'
  }
  if (isPoc) return 'POC验证失败'
  if (isFix) return '修复失败'
  return '检测失败'
}

function getStateTagType(state: DisplayState): string {
  switch (state) {
    case '检测中':
    case '修复中':
    case 'POC验证中':
    case '漏洞修复中':
    case '大模型修复中':
      return 'warning'
    case '通过':
    case '修复成功':
    case 'POC验证成功':
    case '漏洞修复成功':
    case '大模型修复成功':
      return 'success'
    case '未通过':
      return 'info'
    case '检测失败':
    case '修复失败':
    case 'POC验证失败':
    case '漏洞修复失败':
    case '大模型修复失败':
    case '大模型修复超时':
    case '检测超时':
    case '修复超时':
    case '审计未通过':
      return 'danger'
    default: return 'info'
  }
}

function isTaskTerminal(task: TaskLog) {
  const status = normalizeStatus(task.status)
  return status === 'success' || status === 'failed' || status === 'timeout' || status === 'audit_blocked'
}

function isTaskPassed(task: TaskLog) {
  const status = normalizeStatus(task.status)
  const type = normalizeType(task.task_type)
  if (status !== 'success') return false
  if (type === 'CHECK' || type === 'POC_VERIFY') return (task.exit_code ?? 1) === 0
  return (task.exit_code ?? 0) === 0
}

function getTaskPassRate(task: TaskLog) {
  if (!isTaskTerminal(task)) return 0
  return isTaskPassed(task) ? 100 : 0
}

function healingStatusText(status?: string) {
  switch (status) {
    case 'healed': return '已修复'
    case 'failed': return '修复失败'
    case 'timeout': return '修复超时'
    case 'queued': return '排队中'
    case 'healing': return '修复中'
    default: return '未知'
  }
}

function healingStepStatusText(status?: string) {
  switch (status) {
    case 'completed': return '已完成'
    case 'failed': return '失败'
    case 'running': return '进行中'
    case 'queued': return '排队中'
    default: return status || '未知'
  }
}

const progressPercent = computed(() => {
  if (!status.value || status.value.total === 0) return 0
  const completed = (status.value.success || 0) + (status.value.failed || 0) + (status.value.timeout || 0)
  return Math.round((completed / status.value.total) * 100)
})

const progressStatus = computed(() => {
  if (!status.value) return ''
  if (status.value.status === 'success') return 'success'
  if (status.value.status === 'failed') return 'exception'
  return ''
})

const getRuleTitle = (task: TaskLog) => task.rule_title || task.vulnerability_id || task.rule_id?.substring(0, 8) + '...' || ''

const getHostname = (hostId: string) => {
  const host = hostStore.hosts.find(h => h.id === hostId)
  return host ? host.hostname + ' (' + host.ip_address + ')' : hostId.substring(0, 8)
}

const showScript = (task: TaskLog, type: 'script' | 'result') => {
  if (type === 'script') {
    scriptDialogTitle.value = '脚本内容'
    currentScript.value = task.script_content || '// 脚本内容为空'
  } else {
    scriptDialogTitle.value = '执行结果'
    let content = ''
    if (task.stdout) content += '=== STDOUT ===\n' + task.stdout + '\n\n'
    if (task.stderr) content += '=== STDERR ===\n' + task.stderr + '\n\n'
    if (task.exit_code !== undefined) content += '=== EXIT CODE: ' + task.exit_code + ' ==='
    currentScript.value = content || '// 无执行结果'
  }
  scriptDialogVisible.value = true
}

const deleteTask = async (task: TaskLog) => {
  try {
    await ElMessageBox.confirm('确定要删除该任务吗？', '删除确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await deleteTaskApi(task.id)
    ElMessage.success('删除成功')
    delete healingStatusMap.value[task.id]
    await refresh()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || '删除失败')
    }
  }
}

const refresh = async () => {
  refreshing.value = true
  try {
    const [logs, statusData] = await Promise.all([getTaskLogs(taskGroupId), getTaskStatus(taskGroupId)])
    tasks.value = logs
    status.value = statusData
    
    // 直接使用后端返回的 healing_status，无需单独请求
    for (const task of tasks.value) {
      if (task.healing_status) {
        healingStatusMap.value[task.id] = task.healing_status
      }
    }
  } catch (e: any) {
    ElMessage.error(e.message || '刷新失败')
  } finally {
    refreshing.value = false
  }
}

const goBack = () => router.push(taskCenterPath.value)

const startPolling = () => {
  pollTimer = window.setInterval(async () => {
    const hasRunning = status.value && (status.value.pending > 0 || status.value.running > 0)
    const hasHealing = Object.values(healingStatusMap.value).some(h => h.status === 'healing' || h.status === 'queued')
    if (hasRunning || hasHealing) await refresh()
  }, 3000)
}

onMounted(async () => {
  await hostStore.fetchHosts()
  loading.value = true
  try {
    await refresh()
    startPolling()
  } finally {
    loading.value = false
  }
})

onUnmounted(() => { if (pollTimer) clearInterval(pollTimer) })
</script>

<style scoped>
.card-header { display: flex; justify-content: space-between; align-items: center; }
.script-viewer { background: #1e1e1e; border-radius: 4px; padding: 12px; max-height: 500px; overflow: auto; }
.script-content { color: #d4d4d4; font-family: 'Fira Code', monospace; font-size: 13px; white-space: pre-wrap; word-break: break-all; margin: 0; }
.healing-process-list { display: flex; flex-direction: column; gap: 12px; }
.healing-process-item { padding: 14px; border: 1px solid var(--aegis-border); border-radius: 8px; background: #fff; }
.healing-process-header { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; }
.healing-process-header strong { display: block; margin-bottom: 4px; }
.healing-process-header span,
.healing-process-meta { color: var(--aegis-text-muted); font-size: 12px; }
.healing-process-meta { display: flex; gap: 12px; flex-wrap: wrap; margin-top: 10px; }
.healing-steps { margin: 12px 0 0; padding-left: 20px; }
.healing-steps li { margin-bottom: 8px; color: #475569; }
.healing-steps strong { margin-right: 8px; color: var(--aegis-text); }
.healing-steps em { margin-left: 8px; color: var(--aegis-text-muted); font-style: normal; }
.healing-error { margin: 10px 0 0; color: #b91c1c; font-size: 13px; }
</style>

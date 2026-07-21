<template>
  <div class="task-detail">
    <el-page-header @back="goBack" :title="$t('generated.common_return_11d024')" :content="$t('dynamic.taskDetailsWithId', { id: taskGroupId })" />

    <el-card style="margin-top: 20px" v-loading="loading">
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.taskDetail_task_progress_01eaf9') }}</span>
          <el-button @click="refresh" :loading="refreshing">{{ $t('generated.common_refresh_38108e') }}</el-button>
        </div>
      </template>

      <el-row :gutter="20" v-if="status">
        <el-col :span="3">
          <el-statistic :title="$t('generated.taskDetail_total_number_of_tasks_3be5a5')" :value="status.total" />
        </el-col>
        <el-col :span="3">
          <el-statistic :title="$t('generated.common_to_be_executed_6cf0af')" :value="status.pending" />
        </el-col>
        <el-col :span="3">
          <el-statistic :title="$t('generated.common_executing_1f425b')" :value="status.running" />
        </el-col>
        <el-col :span="3">
          <el-statistic :title="$t('generated.taskDetail_completed_e99b48')" :value="status.success + status.failed" />
        </el-col>
        <el-col :span="3">
          <el-statistic :title="$t('generated.common_time_out_ff06c2')" :value="status.timeout || 0" />
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
          <span>{{ $t('generated.taskDetail_task_list_cfd748') }}</span>
          <div style="display: flex; align-items: center; gap: 8px;">
            <span style="font-size: 13px; color: #666;">{{ $t('generated.taskDetail_type_filter_0aea4f') }}</span>
            <el-select v-model="typeFilter" size="small" style="width: 120px;">
              <el-option :label="$t('generated.taskDetail_all_778fc8')" value="all" />
              <el-option v-if="!isVulnerabilityTask" :label="$t('generated.common_detection_b3ff0c')" value="CHECK" />
              <el-option v-if="!isVulnerabilityTask" :label="$t('generated.common_repair_590253')" value="FIX" />
              <el-option v-if="isVulnerabilityTask" :label="$t('generated.common_poc_verification_2e1c70')" value="POC_VERIFY" />
              <el-option v-if="isVulnerabilityTask" :label="$t('generated.common_bug_fixes_091102')" value="VULNERABILITY_FIX" />
            </el-select>
          </div>
        </div>
      </template>

      <el-table :data="tasksWithState" style="width: 100%">
        <el-table-column prop="rule_title" :label="$t('generated.common_rule_title_298a16')" min-width="180">
          <template #default="{ row }">
            {{ getRuleTitle(row) }}
          </template>
        </el-table-column>
        <el-table-column prop="hostname" :label="$t('generated.common_host_2e8a0c')" min-width="150">
          <template #default="{ row }">
            {{ getHostname(row.host_id) }}
          </template>
        </el-table-column>
        <el-table-column prop="task_type" :label="$t('generated.common_type_e4e46c')" width="100">
          <template #default="{ row }">
            <el-tag :type="getTaskTypeTag(row.task_type)" size="small">
              {{ getTaskTypeLabel(row.task_type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_state_62e951')" width="140">
          <template #default="{ row }">
            <el-tooltip
              v-if="row.displayState === $t('dynamic.taskState.healingFailed') || row.displayState === $t('dynamic.taskState.checkFailed') || row.displayState === $t('dynamic.taskState.fixFailed')"
              :content="row.healingStatus?.last_error || row.stderr || $t('dynamic.unknownError')"
              placement="top"
            >
              <el-tag :type="getStateTagType(row.displayState)" size="small">
                {{ row.displayState }}
              </el-tag>
            </el-tooltip>
            <el-tooltip
              v-else-if="row.displayState === $t('dynamic.taskState.auditRejected')"
              :content="row.audit_info?.error_message || $t('dynamic.maliciousScriptBlocked')"
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
        <el-table-column :label="$t('generated.taskDetail_script_7fbccb')" width="100">
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              size="small"
              @click="showScript(row, 'script')"
              :disabled="row.displayState === $t('dynamic.taskState.healing')"
            >
              {{ row.displayState === $t('dynamic.taskState.healing') ? $t('dynamic.taskState.fixing') : $t('dynamic.viewScript') }}
            </el-button>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_result_0a2c91')" width="100">
          <template #default="{ row }">
            <el-button
              v-if="row.stdout || row.stderr"
              link
              type="primary"
              size="small"
              @click="showScript(row, 'result')"
            >
              {{ $t('generated.taskDetail_view_results_0ef384') }}
            </el-button>
            <span v-else style="color: #999">-</span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="90">
          <template #default="{ row }">
            <el-button
              link
              type="danger"
              size="small"
              @click="deleteTask(row)"
              :disabled="row.status === 'running' || row.status === 'pending' || row.displayState === $t('dynamic.taskState.healing')"
            >
              {{ $t('generated.common_delete_3755f5') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card v-if="healingProcessTasks.length" style="margin-top: 20px">
      <template #header>
        <span>{{ $t('generated.taskDetail_automatic_repair_process_2cb789') }}</span>
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
            <span>{{ $t('generated.taskDetail_current_round_0f55b9') }} {{ task.healingStatus?.total_attempts || 0 }} / {{ task.healingStatus?.max_attempts || 0 }}</span>
            <span v-if="task.healingStatus?.concurrency_limit">{{ $t('generated.taskDetail_concurrency_limit_118e6a') }} {{ task.healingStatus.concurrency_limit }}</span>
            <span v-if="task.healingStatus?.queue_position">{{ $t('generated.taskDetail_queue_cfb281') }} {{ task.healingStatus.queue_position }}</span>
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
        <span>{{ $t('generated.taskDetail_audit_interception_information_aacfa7') }}</span>
      </template>
      <div v-for="task in tasksWithState.filter(t => t.audit_info)" :key="task.id" style="margin-bottom: 16px; padding: 12px; background: #fef2f2; border-radius: 4px; border: 1px solid #fecaca">
        <div style="font-weight: 600; margin-bottom: 8px">{{ getRuleTitle(task) }} - {{ getHostname(task.host_id) }}</div>
        <div v-if="task.audit_info?.error_message" style="color: #dc2626; margin-bottom: 8px">{{ task.audit_info.error_message }}</div>
        <div v-if="task.audit_info?.hit_rules?.length" style="margin-bottom: 8px">
          <div style="font-size: 13px; color: #666; margin-bottom: 4px">{{ $t('generated.taskDetail_hit_rules_dca5ee') }}</div>
          <div v-for="(rule, i) in task.audit_info.hit_rules" :key="i" style="font-size: 13px; padding: 2px 0">
            <el-tag :type="rule.severity === 'critical' ? 'danger' : 'warning'" size="small">{{ rule.severity }}</el-tag>
            <span style="margin-left: 8px">{{ rule.rule_name }} {{ $t('generated.taskDetail_no_0332ba') }}{{ rule.line_number }}{{ $t('generated.taskDetail_ok_e2040a') }}</span>
          </div>
        </div>
        <el-button
          v-if="task.audit_info?.audit_log_id"
          link
          type="primary"
          size="small"
          @click="router.push('/settings/audit-logs')"
        >
          {{ $t('generated.taskDetail_view_audit_log_2b8f46') }}
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
import { translate } from '@/i18n'

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

type DisplayState = string

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
    case 'CHECK': return translate('generatedScript.common_detection_b3ff0c')
    case 'FIX': return translate('generatedScript.common_repair_590253')
    case 'POC_VERIFY': return translate('generatedScript.common_poc_verification_2e1c70')
    case 'VULNERABILITY_FIX': return translate('generatedScript.common_bug_fixes_091102')
    default: return type
  }
}

function getDisplayState(taskType: string, taskStatus: string, exitCode: number | undefined, healingStatus?: HealingStatus): DisplayState {
  const isFix = taskType === 'FIX' || taskType === 'VULNERABILITY_FIX'
  const isPoc = taskType === 'POC_VERIFY'

  if (taskStatus === 'running' || taskStatus === 'pending') {
    if (isPoc) return translate('dynamic.taskState.pocRunning')
    if (isFix) return translate('dynamic.taskState.fixing')
    return translate('dynamic.taskState.checking')
  }
  if (taskStatus === 'timeout') {
    if (isPoc) return translate('dynamic.taskState.checkTimeout')
    if (isFix) return translate('dynamic.taskState.fixTimeout')
    return translate('dynamic.taskState.checkTimeout')
  }
  if (taskStatus === 'audit_blocked') {
    return translate('dynamic.taskState.auditRejected')
  }
  if (taskStatus === 'success') {
    if (isPoc) return exitCode === 0 ? translate('dynamic.taskState.pocSuccess') : translate('dynamic.taskState.pocFailed')
    if (isFix) return (exitCode ?? 0) === 0 ? translate('dynamic.taskState.fixSuccess') : translate('dynamic.taskState.fixFailed')
    if (exitCode === 0) return translate('dynamic.taskState.passed')
    return translate('dynamic.taskState.notPassed')
  }
  if (taskStatus === 'failed') {
    if (!healingStatus) {
      if (isPoc) return translate('dynamic.taskState.pocFailed')
      if (isFix) return translate('dynamic.taskState.fixFailed')
      return translate('dynamic.taskState.checkFailed')
    }
    if (healingStatus.status === 'healing') return translate('dynamic.taskState.healing')
    if (healingStatus.status === 'queued') return translate('dynamic.taskState.healing')
    if (healingStatus.status === 'healed') return translate('dynamic.taskState.healingSuccess')
    if (healingStatus.status === 'failed') return translate('dynamic.taskState.healingFailed')
    if (healingStatus.status === 'timeout') return translate('dynamic.taskState.healingTimeout')
  }
  if (isPoc) return translate('dynamic.taskState.pocFailed')
  if (isFix) return translate('dynamic.taskState.fixFailed')
  return translate('dynamic.taskState.checkFailed')
}

function getStateTagType(state: DisplayState): string {
  switch (state) {
    case translate('dynamic.taskState.checking'):
    case translate('dynamic.taskState.fixing'):
    case translate('dynamic.taskState.pocRunning'):
    case translate('dynamic.taskState.vulnerabilityFixing'):
    case translate('dynamic.taskState.healing'):
      return 'warning'
    case translate('dynamic.taskState.passed'):
    case translate('dynamic.taskState.fixSuccess'):
    case translate('dynamic.taskState.pocSuccess'):
    case translate('dynamic.taskState.vulnerabilityFixSuccess'):
    case translate('dynamic.taskState.healingSuccess'):
      return 'success'
    case translate('dynamic.taskState.notPassed'):
      return 'info'
    case translate('dynamic.taskState.checkFailed'):
    case translate('dynamic.taskState.fixFailed'):
    case translate('dynamic.taskState.pocFailed'):
    case translate('dynamic.taskState.vulnerabilityFixFailed'):
    case translate('dynamic.taskState.healingFailed'):
    case translate('dynamic.taskState.healingTimeout'):
    case translate('dynamic.taskState.checkTimeout'):
    case translate('dynamic.taskState.fixTimeout'):
    case translate('dynamic.taskState.auditRejected'):
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
    case 'healed': return translate('generatedScript.taskDetail_fixed_50138c')
    case 'failed': return translate('generatedScript.taskDetail_repair_failed_3d6dfb')
    case 'timeout': return translate('generatedScript.taskDetail_fix_timeout_a9a36a')
    case 'queued': return translate('generatedScript.taskDetail_queuing_4dcbbc')
    case 'healing': return translate('generatedScript.taskDetail_under_repair_20ff36')
    default: return translate('generatedScript.common_unknown_d9c32a')
  }
}

function healingStepStatusText(status?: string) {
  switch (status) {
    case 'completed': return translate('generatedScript.common_completed_e99b48')
    case 'failed': return translate('generatedScript.common_fail_3e3c80')
    case 'running': return translate('generatedScript.taskDetail_in_progress_6f1972')
    case 'queued': return translate('generatedScript.taskDetail_queuing_4dcbbc')
    default: return status || translate('generatedScript.common_unknown_d9c32a')
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
    scriptDialogTitle.value = translate('generatedScript.common_script_content_2a33ea')
    currentScript.value = task.script_content || translate('generatedScript.taskDetail_the_script_content_is_empty_273767')
  } else {
    scriptDialogTitle.value = translate('generatedScript.taskDetail_execution_result_1b213f')
    let content = ''
    if (task.stdout) content += '=== STDOUT ===\n' + task.stdout + '\n\n'
    if (task.stderr) content += '=== STDERR ===\n' + task.stderr + '\n\n'
    if (task.exit_code !== undefined) content += '=== EXIT CODE: ' + task.exit_code + ' ==='
    currentScript.value = content || translate('generatedScript.taskDetail_no_execution_result_b8660b')
  }
  scriptDialogVisible.value = true
}

const deleteTask = async (task: TaskLog) => {
  try {
    await ElMessageBox.confirm(translate('generatedScript.taskDetail_are_you_sure_you_want_to_2ddd4f'), translate('generatedScript.common_delete_confirmation_726b6e'), {
      confirmButtonText: translate('generatedScript.common_sure_f526c8'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning'
    })
    await deleteTaskApi(task.id)
    ElMessage.success(translate('generatedScript.common_delete_successfully_86e8d1'))
    delete healingStatusMap.value[task.id]
    await refresh()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || translate('generatedScript.common_delete_failed_72250c'))
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
    ElMessage.error(e.message || translate('generatedScript.taskDetail_refresh_failed_be6ff1'))
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

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
        <el-col :span="4">
          <el-statistic title="总任务数" :value="status.total" />
        </el-col>
        <el-col :span="4">
          <el-statistic title="待执行" :value="status.pending" />
        </el-col>
        <el-col :span="4">
          <el-statistic title="执行中" :value="status.running" />
        </el-col>
        <el-col :span="4">
          <el-statistic title="已完成" :value="status.success + status.failed" />
        </el-col>
        <el-col :span="4">
          <el-statistic title="超时" :value="status.timeout || 0" />
        </el-col>
        <el-col :span="4">
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
        <span>任务列表</span>
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
        <el-table-column prop="task_type" label="类型" width="80">
          <template #default="{ row }">
            <el-tag :type="row.task_type === 'check' ? 'primary' : 'warning'" size="small">
              {{ row.task_type === 'check' ? '检测' : '修复' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="140">
          <template #default="{ row }">
            <el-tooltip 
              v-if="row.displayState === '脚本修复失败' || row.displayState === '检测失败' || row.displayState === '修复失败'" 
              :content="row.healingStatus?.last_error || row.stderr || '未知错误'" 
              placement="top"
            >
              <el-tag :type="getStateTagType(row.displayState)" size="small">
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
              :disabled="row.displayState === '脚本修复中'"
            >
              {{ row.displayState === '脚本修复中' ? '修复中' : '查看脚本' }}
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
        <el-table-column label="操作" width="320">
          <template #default="{ row }">
            <el-button 
              v-if="canShowScriptRepair(row)"
              link 
              type="warning" 
              size="small" 
              @click="triggerScriptRepair(row)"
              :loading="repairingTask === row.id"
            >
              脚本修复
            </el-button>
            <el-button 
              v-if="canReExecute(row)"
              link 
              type="primary" 
              size="small" 
              @click="reExecute(row)"
              :loading="reexecutingTask === row.id"
            >
              重新下发
            </el-button>
            <el-button 
              v-if="canShowSuggestion(row)"
              link 
              type="info" 
              size="small" 
              @click="openSuggestionDialog(row)"
            >
              修复建议
            </el-button>
            <el-button 
              link 
              type="danger" 
              size="small" 
              @click="deleteTask(row)"
              :disabled="row.status === 'running' || row.status === 'pending' || row.displayState === '脚本修复中'"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="scriptDialogVisible" :title="scriptDialogTitle" width="70%">
      <div class="script-viewer">
        <pre class="script-content">{{ currentScript }}</pre>
      </div>
    </el-dialog>

    <el-dialog v-model="suggestionDialogVisible" title="修复建议" width="500px">
      <el-form>
        <el-form-item label="修复建议">
          <el-input
            v-model="suggestionText"
            type="textarea"
            :rows="4"
            placeholder="请输入您的修复建议，系统会将建议发送给大模型进行脚本修复"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="suggestionDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitSuggestion" :loading="submittingSuggestion">
          提交修复
        </el-button>
      </template>
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
  triggerSelfHealing, 
  getHealingStatus,
  redispatchTask,
  deleteTask as deleteTaskApi,
  type TaskLog, 
  type TaskGroupStatus,
  type HealingStatus 
} from '@/api/tasks'
import { useHostStore } from '@/store/hosts'

type DisplayState = '检测中' | '通过' | '未通过' | '检测失败' | '修复中' | '修复成功' | '修复失败' | '脚本修复中' | '脚本修复成功' | '脚本修复失败' | '检测超时' | '修复超时'

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
const reexecutingTask = ref<string | null>(null)
const repairingTask = ref<string | null>(null)
const suggestionDialogVisible = ref(false)
const suggestionText = ref('')
const selectedTask = ref<TaskLog | null>(null)
const submittingSuggestion = ref(false)
let pollTimer: number | null = null

const tasksWithState = computed(() => {
  return tasks.value.map(task => {
    const healingStatus = healingStatusMap.value[task.id]
    const displayState = getDisplayState(task.task_type, task.status, task.exit_code, healingStatus)
    return {
      ...task,
      displayState,
      healingStatus
    }
  })
})

function getDisplayState(taskType: string, taskStatus: string, exitCode: number | undefined, healingStatus?: HealingStatus): DisplayState {
  const isFix = taskType === 'fix'
  
  if (taskStatus === 'running' || taskStatus === 'pending') {
    return isFix ? '修复中' : '检测中'
  }
  if (taskStatus === 'timeout') {
    return isFix ? '修复超时' : '检测超时'
  }
  if (taskStatus === 'success') {
    if (isFix) {
      return '修复成功'
    }
    if (exitCode === 0) {
      return '通过'
    }
    return '未通过'
  }
  if (taskStatus === 'failed') {
    if (!healingStatus) {
      return isFix ? '修复失败' : '检测失败'
    }
    if (healingStatus.status === 'healing') return '脚本修复中'
    if (healingStatus.status === 'healed') return '脚本修复成功'
    if (healingStatus.status === 'failed') return '脚本修复失败'
  }
  return isFix ? '修复失败' : '检测失败'
}

function getStateTagType(state: DisplayState): string {
  switch (state) {
    case '检测中':
    case '修复中':
    case '脚本修复中':
      return 'warning'
    case '通过':
    case '修复成功':
    case '脚本修复成功':
      return 'success'
    case '未通过':
      return 'info'
    case '检测失败':
    case '修复失败':
    case '脚本修复失败':
    case '检测超时':
    case '修复超时':
      return 'danger'
    default: return 'info'
  }
}

function canShowScriptRepair(row: any): boolean {
  if (row.task_type === 'check') {
    return row.displayState === '检测失败' || row.displayState === '脚本修复失败'
  } else {
    return row.displayState === '修复失败' || row.displayState === '脚本修复失败'
  }
}

function canReExecute(row: any): boolean {
  return row.displayState === '检测失败' || 
         row.displayState === '修复失败' || 
         row.displayState === '脚本修复成功' || 
         row.displayState === '检测超时' || 
         row.displayState === '修复超时'
}

function canShowSuggestion(row: any): boolean {
  return row.displayState === '检测失败' || 
         row.displayState === '修复失败' || 
         row.displayState === '脚本修复失败'
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

const getRuleTitle = (task: TaskLog) => task.rule_title || task.rule_id.substring(0, 8) + '...'

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

const triggerScriptRepair = async (task: TaskLog) => {
  repairingTask.value = task.id
  try {
    await triggerSelfHealing(task.id, '')
    ElMessage.success('脚本修复已触发，正在调用大模型进行修复...')
    await fetchHealingStatuses()
  } catch (e: any) {
    ElMessage.error(e.message || '脚本修复失败')
  } finally {
    repairingTask.value = null
  }
}

const openSuggestionDialog = (task: TaskLog) => {
  selectedTask.value = task
  suggestionText.value = ''
  suggestionDialogVisible.value = true
}

const submitSuggestion = async () => {
  if (!selectedTask.value) return
  submittingSuggestion.value = true
  try {
    await triggerSelfHealing(selectedTask.value.id, suggestionText.value)
    ElMessage.success('修复建议已提交，系统正在进行脚本修复')
    suggestionDialogVisible.value = false
    await fetchHealingStatuses()
  } catch (e: any) {
    ElMessage.error(e.message || '提交失败')
  } finally {
    submittingSuggestion.value = false
  }
}

const reExecute = async (task: TaskLog) => {
  reexecutingTask.value = task.id
  try {
    await redispatchTask(task.id)
    ElMessage.success('重新下发成功')
    delete healingStatusMap.value[task.id]
    await refresh()
  } catch (e: any) {
    ElMessage.error(e.message || '重新下发失败')
  } finally {
    reexecutingTask.value = null
  }
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

const fetchHealingStatuses = async () => {
  const failedTasks = tasks.value.filter(t => t.status === 'failed')
  const healingTasks = Object.values(healingStatusMap.value).filter(h => h.status === 'healing')
  const taskIds = [...new Set([...failedTasks.map(t => t.id), ...healingTasks.map(h => h.original_task_id)])]
  for (const taskId of taskIds) {
    try {
      const healingStatus = await getHealingStatus(taskId)
      if (healingStatus) healingStatusMap.value[taskId] = healingStatus
    } catch { }
  }
}

const refresh = async () => {
  refreshing.value = true
  try {
    const [logs, statusData] = await Promise.all([getTaskLogs(taskGroupId), getTaskStatus(taskGroupId)])
    tasks.value = logs
    status.value = statusData
    await fetchHealingStatuses()
  } catch (e: any) {
    ElMessage.error(e.message || '刷新失败')
  } finally {
    refreshing.value = false
  }
}

const goBack = () => router.push('/tasks')

const startPolling = () => {
  pollTimer = window.setInterval(async () => {
    const hasRunning = status.value && (status.value.pending > 0 || status.value.running > 0)
    const hasHealing = Object.values(healingStatusMap.value).some(h => h.status === 'healing')
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
</style>
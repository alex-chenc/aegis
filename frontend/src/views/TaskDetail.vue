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
          <el-statistic title="成功" :value="status.success" />
        </el-col>
        <el-col :span="4">
          <el-statistic title="失败" :value="status.failed" />
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

      <el-table :data="tasks" style="width: 100%">
        <el-table-column prop="rule_title" label="规则标题" min-width="180">
          <template #default="{ row }">
            {{ getRuleTitle(row.rule_id) }}
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
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="脚本" width="150">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="showScript(row, 'script')">
              查看脚本
            </el-button>
          </template>
        </el-table-column>
        <el-table-column label="结果" width="150">
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
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'failed'"
              link
              type="warning"
              size="small"
              @click="triggerHealing(row.id)"
            >
              自愈修复
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getTaskLogs, getTaskStatus, triggerSelfHealing, type TaskLog, type TaskGroupStatus } from '@/api/tasks'
import { useHostStore } from '@/store/hosts'

const route = useRoute()
const router = useRouter()
const hostStore = useHostStore()

const taskGroupId = route.params.id as string
const loading = ref(false)
const refreshing = ref(false)
const tasks = ref<TaskLog[]>([])
const status = ref<TaskGroupStatus | null>(null)
const scriptDialogVisible = ref(false)
const scriptDialogTitle = ref('')
const currentScript = ref('')
let pollTimer: number | null = null

const progressPercent = computed(() => {
  if (!status.value || status.value.total === 0) return 0
  return Math.round(((status.value.success + status.value.failed) / status.value.total) * 100)
})

const progressStatus = computed(() => {
  if (!status.value) return ''
  if (status.value.status === 'success') return 'success'
  if (status.value.status === 'failed') return 'exception'
  return ''
})

const getRuleTitle = (ruleId: string) => {
  return ruleId.substring(0, 8) + '...'
}

const getHostname = (hostId: string) => {
  const host = hostStore.hosts.find(h => h.id === hostId)
  return host ? `${host.hostname} (${host.ip_address})` : hostId.substring(0, 8)
}

const getStatusType = (status: string) => {
  switch (status) {
    case 'pending': return 'info'
    case 'running': return 'warning'
    case 'success': return 'success'
    case 'failed': return 'danger'
    default: return 'info'
  }
}

const getStatusText = (status: string) => {
  switch (status) {
    case 'pending': return '待执行'
    case 'running': return '执行中'
    case 'success': return '成功'
    case 'failed': return '失败'
    default: return status
  }
}

const showScript = (task: TaskLog, type: 'script' | 'result') => {
  if (type === 'script') {
    scriptDialogTitle.value = `检测脚本`
    currentScript.value = task.script_content || '// 脚本内容为空'
  } else {
    scriptDialogTitle.value = `执行结果`
    let content = ''
    if (task.stdout) {
      content += `=== STDOUT ===\n${task.stdout}\n\n`
    }
    if (task.stderr) {
      content += `=== STDERR ===\n${task.stderr}\n\n`
    }
    if (task.exit_code !== undefined) {
      content += `=== EXIT CODE: ${task.exit_code} ===`
    }
    currentScript.value = content || '// 无执行结果'
  }
  scriptDialogVisible.value = true
}

const triggerHealing = async (taskId: string) => {
  try {
    const result = await triggerSelfHealing(taskId)
    if (result.success) {
      ElMessage.success('自愈修复已触发，请稍后刷新查看结果')
      await refresh()
    }
  } catch (e: any) {
    ElMessage.error(e.message || '自愈修复失败')
  }
}

const refresh = async () => {
  refreshing.value = true
  try {
    const [logs, statusData] = await Promise.all([
      getTaskLogs(taskGroupId),
      getTaskStatus(taskGroupId)
    ])
    tasks.value = logs
    status.value = statusData
  } catch (e: any) {
    ElMessage.error(e.message || '刷新失败')
  } finally {
    refreshing.value = false
  }
}

const goBack = () => {
  router.push('/')
}

const startPolling = () => {
  pollTimer = window.setInterval(() => {
    if (status.value && (status.value.pending > 0 || status.value.running > 0)) {
      refresh()
    }
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

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
  }
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.script-viewer {
  background: #1e1e1e;
  border-radius: 4px;
  padding: 12px;
  max-height: 500px;
  overflow: auto;
}

.script-content {
  color: #d4d4d4;
  font-family: 'Fira Code', monospace;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
</style>

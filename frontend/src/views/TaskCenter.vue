<template>
  <div class="task-center">
    <div class="overview-grid">
      <div class="overview-card">
        <span>任务组</span>
        <strong>{{ taskOverview.total }}</strong>
      </div>
      <div class="overview-card">
        <span>执行中</span>
        <strong>{{ taskOverview.active }}</strong>
      </div>
      <div class="overview-card">
        <span>成功</span>
        <strong>{{ taskOverview.success }}</strong>
      </div>
      <div class="overview-card">
        <span>失败/超时</span>
        <strong>{{ taskOverview.failed }}</strong>
      </div>
      <div class="overview-card">
        <span>平均通过率</span>
        <strong>{{ taskOverview.passRate }}%</strong>
      </div>
    </div>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ taskCenterTitle }}</span>
          <div class="header-actions">
            <span class="live-indicator" :class="{ active: hasLiveTasks }">
              <i />
              {{ hasLiveTasks ? '实时刷新中' : '实时空闲' }}
            </span>
            <span class="refresh-time">最后刷新 {{ lastRefreshText }}</span>
            <el-dropdown @command="handleReportCommand">
              <el-button>
                合规报告
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="pdf">导出 PDF</el-dropdown-item>
                  <el-dropdown-item command="excel">导出 Excel</el-dropdown-item>
                  <el-dropdown-item command="weekly">每周报告</el-dropdown-item>
                  <el-dropdown-item command="monthly">每月报告</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button
              type="danger"
              :disabled="selectedTaskIds.length === 0"
              @click="handleBatchDelete"
              v-if="taskGroups.length > 0"
            >
              批量删除 ({{ selectedTaskIds.length }})
            </el-button>
            <el-button @click="refresh" :loading="loading">刷新</el-button>
          </div>
        </div>
      </template>

      <div class="filter-bar">
        <el-select v-model="filters.status" placeholder="状态" clearable style="width: 120px" @change="handleFilterChange">
          <el-option label="待执行" value="pending" />
          <el-option label="执行中" value="running" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
        </el-select>

        <el-select v-model="filters.task_type" placeholder="类型" clearable style="width: 120px; margin-left: 10px" @change="handleFilterChange">
          <el-option v-if="!isVulnerabilityTask" label="检测" value="CHECK" />
          <el-option v-if="!isVulnerabilityTask" label="修复" value="FIX" />
          <el-option v-if="isVulnerabilityTask" label="POC验证" value="POC_VERIFY" />
          <el-option v-if="isVulnerabilityTask" label="漏洞修复" value="VULNERABILITY_FIX" />
        </el-select>

        <el-input
          v-model="filters.search"
          placeholder="搜索规则名称"
          clearable
          style="width: 200px; margin-left: 10px"
          @keyup.enter="handleFilterChange"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <el-table
        :data="taskGroups"
        style="width: 100%; margin-top: 15px"
        v-loading="loading"
        :row-class-name="getRowClassName"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="55" />
        <el-table-column prop="task_group_id" label="任务组ID" min-width="280">
          <template #default="{ row }">
            <el-link type="primary" @click="goToDetail(row.task_group_id)">
              {{ row.task_group_id.substring(0, 8) }}...
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="task_type" label="类型" min-width="140">
          <template #default="{ row }">
            <div style="display: flex; gap: 4px; flex-wrap: wrap;">
              <el-tag v-if="row.has_check || normalizeType(row.task_type) === 'CHECK'" type="primary" size="small">检测</el-tag>
              <el-tag v-if="row.has_fix || normalizeType(row.task_type) === 'FIX'" type="warning" size="small">修复</el-tag>
              <el-tag v-if="normalizeType(row.task_type) === 'POC_VERIFY'" type="success" size="small">POC验证</el-tag>
              <el-tag v-if="normalizeType(row.task_type) === 'VULNERABILITY_FIX'" type="danger" size="small">漏洞修复</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="task_count" label="任务数" width="80" />
        <el-table-column label="通过率" width="150">
          <template #default="{ row }">
            <el-progress
              :percentage="getPassRate(row)"
              :stroke-width="10"
              :status="getPassRate(row) >= 100 ? 'success' : normalizeStatus(row.status) === 'failed' ? 'exception' : undefined"
            />
          </template>
        </el-table-column>
        <el-table-column label="进度" min-width="220">
          <template #default="{ row }">
            <div class="progress-info">
              <span class="success">{{ row.success_count }}</span> /
              <span class="failed">{{ row.failed_count }}</span> /
              <span class="timeout">{{ row.timeout_count || 0 }}</span> /
              <span class="pending">{{ row.pending_count }}</span> /
              <span class="running">{{ row.running_count }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" min-width="160">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="goToDetail(row.task_group_id)">
              详情
            </el-button>
            <el-button
              link
              type="danger"
              size="small"
              @click="handleDeleteTaskGroup(row)"
              :disabled="row.status === 'running' || row.status === 'pending'"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :page-sizes="[10, 20, 50]"
          :total="pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import {
  listTasks,
  deleteTaskGroup,
  batchDeleteTasks,
  normalizeType,
  normalizeStatus,
  type TaskGroupSummary
} from '@/api/tasks'

const route = useRoute()
const router = useRouter()

const isVulnerabilityTask = computed(() => route.path.startsWith('/vulnerability/tasks'))
const taskCenterTitle = computed(() => isVulnerabilityTask.value ? '漏洞任务中心' : '基线任务中心')
const detailBasePath = computed(() => isVulnerabilityTask.value ? '/vulnerability/tasks' : '/baseline/tasks')
const defaultTypeScope = computed(() =>
  isVulnerabilityTask.value ? 'POC_VERIFY,VULNERABILITY_FIX' : 'CHECK,FIX'
)

const loading = ref(false)
const taskGroups = ref<TaskGroupSummary[]>([])
const selectedTaskIds = ref<string[]>([])
const lastRefreshAt = ref<Date | null>(null)
const changedTaskIds = ref<Set<string>>(new Set())
let autoRefreshTimer: number | null = null

const filters = reactive({
  status: '',
  task_type: '',
  search: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

const hasLiveTasks = computed(() => taskGroups.value.some(task => ['pending', 'running'].includes(normalizeStatus(task.status))))

const taskOverview = computed(() => {
  const total = pagination.total
  const active = taskGroups.value.filter(task => ['pending', 'running'].includes(normalizeStatus(task.status))).length
  const success = taskGroups.value.filter(task => normalizeStatus(task.status) === 'success').length
  const failed = taskGroups.value.filter(task => ['failed', 'timeout'].includes(normalizeStatus(task.status))).length
  const passRate = total
    ? Math.round(taskGroups.value.reduce((sum, task) => sum + getPassRate(task), 0) / total)
    : 0
  return { total, active, success, failed, passRate }
})

const lastRefreshText = computed(() => {
  if (!lastRefreshAt.value) return '-'
  return lastRefreshAt.value.toLocaleTimeString('zh-CN', { hour12: false })
})

const fetchTasks = async () => {
  loading.value = true
  try {
    const result = await listTasks({
      page: pagination.page,
      page_size: pagination.pageSize,
      status: filters.status || undefined,
      task_type: filters.task_type || defaultTypeScope.value,
      search: filters.search || undefined
    })
    const previousStatus = new Map(taskGroups.value.map(item => [item.task_group_id, item.status]))
    taskGroups.value = result.items
    const changed = new Set<string>()
    taskGroups.value.forEach(item => {
      if (previousStatus.has(item.task_group_id) && previousStatus.get(item.task_group_id) !== item.status) {
        changed.add(item.task_group_id)
      }
    })
    changedTaskIds.value = changed
    if (changed.size > 0) {
      window.setTimeout(() => {
        changedTaskIds.value = new Set()
      }, 900)
    }
    pagination.total = result.total
    selectedTaskIds.value = []
    lastRefreshAt.value = new Date()
  } catch (e: any) {
    ElMessage.error(e.message || '获取任务列表失败')
  } finally {
    loading.value = false
  }
}

const refresh = () => {
  fetchTasks()
}

const handleFilterChange = () => {
  pagination.page = 1
  fetchTasks()
}

const handleSizeChange = () => {
  pagination.page = 1
  fetchTasks()
}

const handlePageChange = () => {
  fetchTasks()
}

const handleSelectionChange = (selection: TaskGroupSummary[]) => {
  selectedTaskIds.value = selection.map(item => item.task_group_id)
}

const getRowClassName = ({ row }: { row: TaskGroupSummary }) => {
  return changedTaskIds.value.has(row.task_group_id) ? 'task-row-changed' : ''
}

const getStatusType = (status: string) => {
  switch (status) {
    case 'pending': return 'info'
    case 'running': return 'warning'
    case 'success': return 'success'
    case 'failed': return 'danger'
    case 'timeout': return 'danger'
    default: return 'info'
  }
}

const getStatusText = (status: string) => {
  switch (status) {
    case 'pending': return '待执行'
    case 'running': return '执行中'
    case 'success': return '成功'
    case 'failed': return '失败'
    case 'timeout': return '超时'
    default: return status
  }
}

const getPassRate = (row: TaskGroupSummary) => {
  if (typeof row.pass_rate === 'number') return Math.round(row.pass_rate)
  if (!row.task_count) return 0
  return Math.round(((row.success_count || 0) / row.task_count) * 100)
}

const formatTime = (time: string) => {
  if (!time) return '-'
  return time.replace('T', ' ').substring(0, 19)
}

const goToDetail = (taskGroupId: string) => {
  router.push(`${detailBasePath.value}/${taskGroupId}`)
}

const handleReportCommand = (command: string) => {
  if (command === 'pdf') {
    window.print()
    return
  }
  if (command === 'excel') {
    exportExcelReport()
    return
  }
  const label = command === 'weekly' ? '每周' : '每月'
  localStorage.setItem('baseline_report_schedule', command)
  ElMessage.success(`${label}合规报告已启用`)
}

const exportExcelReport = () => {
  const rows = [
    ['任务组ID', '类型', '任务数', '通过率', '成功', '失败', '超时', '待执行', '执行中', '状态', '创建时间'],
    ...taskGroups.value.map(row => [
      row.task_group_id,
      row.task_type,
      row.task_count,
      `${getPassRate(row)}%`,
      row.success_count,
      row.failed_count,
      row.timeout_count || 0,
      row.pending_count,
      row.running_count,
      getStatusText(normalizeStatus(row.status)),
      formatTime(row.created_at)
    ])
  ]
  const table = rows
    .map(cols => `<tr>${cols.map(col => `<td>${String(col).replace(/[<&>]/g, s => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;' }[s] || s))}</td>`).join('')}</tr>`)
    .join('')
  const blob = new Blob([`<table>${table}</table>`], { type: 'application/vnd.ms-excel;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${isVulnerabilityTask.value ? 'vulnerability' : 'baseline'}-compliance-report.xls`
  link.click()
  URL.revokeObjectURL(url)
}

const startAutoRefresh = () => {
  if (autoRefreshTimer) window.clearInterval(autoRefreshTimer)
  autoRefreshTimer = window.setInterval(() => {
    if (hasLiveTasks.value) {
      fetchTasks()
    }
  }, 5000)
}

const handleDeleteTaskGroup = async (row: TaskGroupSummary) => {
  if (row.status === 'running' || row.status === 'pending') {
    ElMessage.warning('运行中的任务无法删除')
    return
  }

  try {
    await ElMessageBox.confirm(`确定删除任务组 "${row.task_group_id.substring(0, 8)}..." ？`, '确认删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await deleteTaskGroup(row.task_group_id)
    ElMessage.success('任务已删除')
    fetchTasks()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || '删除失败')
    }
  }
}

const handleBatchDelete = async () => {
  if (selectedTaskIds.value.length === 0) {
    ElMessage.warning('请先选择要删除的任务')
    return
  }

  const deletableTasks = taskGroups.value.filter(
    item => selectedTaskIds.value.includes(item.task_group_id) &&
            item.status !== 'running' &&
            item.status !== 'pending'
  )

  const skippedCount = selectedTaskIds.value.length - deletableTasks.length

  if (deletableTasks.length === 0) {
    ElMessage.warning('选中的任务都在运行中，无法删除')
    return
  }

  let message = `确定删除选中的 ${deletableTasks.length} 个任务？`
  if (skippedCount > 0) {
    message += `\n（已跳过 ${skippedCount} 个运行中的任务）`
  }

  try {
    await ElMessageBox.confirm(message, '批量删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })

    const result = await batchDeleteTasks(deletableTasks.map(t => t.task_group_id))
    const deletedCount = result?.deleted_count ?? 0
    const resultSkippedCount = result?.skipped_count ?? 0
    ElMessage.success(`成功删除 ${deletedCount} 个任务${resultSkippedCount > 0 ? `，跳过 ${resultSkippedCount} 个` : ''}`)
    fetchTasks()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || '批量删除失败')
    }
  }
}

onMounted(() => {
  fetchTasks()
  startAutoRefresh()
})

onUnmounted(() => {
  if (autoRefreshTimer) window.clearInterval(autoRefreshTimer)
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 14px;
  margin-bottom: 16px;
}

.overview-card {
  padding: 16px;
  border: 1px solid var(--aegis-border);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 12px 32px rgba(15, 23, 42, 0.07);
}

.overview-card span {
  display: block;
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.overview-card strong {
  display: block;
  margin-top: 8px;
  color: var(--aegis-text);
  font-size: 26px;
}

.live-indicator {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #64748b;
  font-size: 12px;
}

.live-indicator i {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}

.live-indicator.active i {
  background: #22c55e;
  box-shadow: 0 0 0 5px rgba(34, 197, 94, 0.14);
}

.refresh-time {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.pagination-container {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.progress-info {
  font-family: 'Fira Code', monospace;
  font-size: 13px;
}

.progress-info .success {
  color: #67c23a;
}

.progress-info .failed {
  color: #f56c6c;
}

.progress-info .pending {
  color: #909399;
}

.progress-info .running {
  color: #e6a23c;
}

.progress-info .timeout {
  color: #f56c6c;
  font-weight: bold;
}

:deep(.el-table__row) {
  transition: background 220ms ease;
}

:deep(.task-row-changed) {
  background: rgba(34, 197, 94, 0.08);
}

@media (max-width: 1000px) {
  .overview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>

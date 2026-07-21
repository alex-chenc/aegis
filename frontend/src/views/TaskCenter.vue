<template>
  <div class="task-center">
    <div class="overview-grid">
      <div class="overview-card">
        <span>{{ $t('generated.taskCenter_task_force_cf61d3') }}</span>
        <strong>{{ taskOverview.total }}</strong>
      </div>
      <div class="overview-card">
        <span>{{ $t('generated.common_executing_1f425b') }}</span>
        <strong>{{ taskOverview.active }}</strong>
      </div>
      <div class="overview-card">
        <span>{{ $t('generated.common_success_51991a') }}</span>
        <strong>{{ taskOverview.success }}</strong>
      </div>
      <div class="overview-card">
        <span>{{ $t('generated.taskCenter_failure_timeout_7ff2a6') }}</span>
        <strong>{{ taskOverview.failed }}</strong>
      </div>
      <div class="overview-card">
        <span>{{ $t('generated.taskCenter_average_pass_rate_bb1021') }}</span>
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
              {{ hasLiveTasks ? $t('dynamic.liveRefreshing') : $t('dynamic.liveIdle') }}
            </span>
            <span class="refresh-time">{{ $t('generated.taskCenter_last_refresh_be7bef') }} {{ lastRefreshText }}</span>
            <el-button @click="exportExcelReport" :loading="exporting">
              {{ $t('generated.taskCenter_export_compliance_reports_9d881a') }}
            </el-button>
            <el-button
              type="danger"
              :disabled="selectedTaskIds.length === 0"
              @click="handleBatchDelete"
              v-if="taskGroups.length > 0"
            >
              {{ $t('generated.common_batch_delete_4edb06') }}{{ selectedTaskIds.length }})
            </el-button>
            <el-button @click="refresh" :loading="loading">{{ $t('generated.common_refresh_38108e') }}</el-button>
          </div>
        </div>
      </template>

      <div class="filter-bar">
        <el-select v-model="filters.status" :placeholder="$t('generated.common_state_62e951')" clearable style="width: 120px" @change="handleFilterChange">
          <el-option :label="$t('generated.common_to_be_executed_6cf0af')" value="pending" />
          <el-option :label="$t('generated.common_executing_1f425b')" value="running" />
          <el-option :label="$t('generated.common_success_51991a')" value="success" />
          <el-option :label="$t('generated.common_fail_3e3c80')" value="failed" />
        </el-select>

        <el-select v-model="filters.task_type" :placeholder="$t('generated.common_type_e4e46c')" clearable style="width: 120px; margin-left: 10px" @change="handleFilterChange">
          <el-option v-if="!isVulnerabilityTask" :label="$t('generated.common_detection_b3ff0c')" value="CHECK" />
          <el-option v-if="!isVulnerabilityTask" :label="$t('generated.common_repair_590253')" value="FIX" />
          <el-option v-if="isVulnerabilityTask" :label="$t('generated.common_poc_verification_2e1c70')" value="POC_VERIFY" />
          <el-option v-if="isVulnerabilityTask" :label="$t('generated.common_bug_fixes_091102')" value="VULNERABILITY_FIX" />
        </el-select>

        <el-input
          v-model="filters.search"
          :placeholder="$t('generated.taskCenter_search_rule_name_ba7bfb')"
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
        <el-table-column prop="task_group_id" :label="$t('generated.taskCenter_task_group_id_c17fc0')" min-width="280">
          <template #default="{ row }">
            <el-link type="primary" @click="goToDetail(row.task_group_id)">
              {{ row.task_group_id.substring(0, 8) }}...
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="task_type" :label="$t('generated.common_type_e4e46c')" min-width="140">
          <template #default="{ row }">
            <div style="display: flex; gap: 4px; flex-wrap: wrap;">
              <el-tag v-if="row.has_check || normalizeType(row.task_type) === 'CHECK'" type="primary" size="small">{{ $t('generated.common_detection_b3ff0c') }}</el-tag>
              <el-tag v-if="row.has_fix || normalizeType(row.task_type) === 'FIX'" type="warning" size="small">{{ $t('generated.common_repair_590253') }}</el-tag>
              <el-tag v-if="normalizeType(row.task_type) === 'POC_VERIFY'" type="success" size="small">{{ $t('generated.common_poc_verification_2e1c70') }}</el-tag>
              <el-tag v-if="normalizeType(row.task_type) === 'VULNERABILITY_FIX'" type="danger" size="small">{{ $t('generated.common_bug_fixes_091102') }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="task_count" :label="$t('generated.taskCenter_number_of_tasks_cd75bc')" width="80" />
        <el-table-column :label="$t('generated.common_pass_rate_b4582b')" width="100">
          <template #default="{ row }">
            <span :style="{ color: getPassRate(row) >= 100 ? '#67c23a' : getPassRate(row) > 0 ? '#e6a23c' : '#f56c6c', fontWeight: 600 }">
              {{ getPassRate(row) }}%
            </span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_schedule_acf014')" min-width="220">
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
        <el-table-column prop="status" :label="$t('generated.common_state_62e951')" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" :label="$t('generated.common_creation_time_84e380')" min-width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" min-width="160">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="goToDetail(row.task_group_id)">
              {{ $t('generated.common_details_4f55ee') }}
            </el-button>
            <el-button
              link
              type="danger"
              size="small"
              @click="handleDeleteTaskGroup(row)"
              :disabled="row.status === 'running' || row.status === 'pending'"
            >
              {{ $t('generated.common_delete_3755f5') }}
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
import { translate } from '@/i18n'
import { formatTime as formatLocaleTime } from '@/i18n/formatters'

import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import {
  listTasks,
  getTaskLogs,
  deleteTaskGroup,
  batchDeleteTasks,
  normalizeType,
  normalizeStatus,
  type TaskGroupSummary
} from '@/api/tasks'
import { buildCsv, downloadCsv } from '@/utils/csv'

const route = useRoute()
const router = useRouter()

const isVulnerabilityTask = computed(() => route.path.startsWith('/vulnerability/tasks'))
const taskCenterTitle = computed(() => isVulnerabilityTask.value ? translate('generatedScript.taskCenter_vulnerability_task_center_7d3137') : translate('generatedScript.taskCenter_baseline_mission_center_1ddf31'))
const detailBasePath = computed(() => isVulnerabilityTask.value ? '/vulnerability/tasks' : '/baseline/tasks')
const defaultTypeScope = computed(() =>
  isVulnerabilityTask.value ? 'POC_VERIFY,VULNERABILITY_FIX' : 'CHECK,FIX'
)

const loading = ref(false)
const exporting = ref(false)
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
  return formatLocaleTime(lastRefreshAt.value)
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
    ElMessage.error(e.message || translate('generatedScript.taskCenter_failed_to_get_task_list_bae2e0'))
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
    case 'pending': return translate('generatedScript.common_to_be_executed_6cf0af')
    case 'running': return translate('generatedScript.common_executing_1f425b')
    case 'success': return translate('generatedScript.common_success_51991a')
    case 'failed': return translate('generatedScript.common_fail_3e3c80')
    case 'timeout': return translate('generatedScript.common_time_out_ff06c2')
    default: return status
  }
}

const getPassRate = (row: TaskGroupSummary) => {
  // 优先使用后端返回的 pass_rate 字段
  if (typeof row.pass_rate === 'number') return Math.round(row.pass_rate)
  if (!row.task_count) return 0
  // 通过率 = 通过的任务数 / 总任务数（通过 = SUCCESS 且 exit_code=0）
  // 这里用 success_count 作为近似值（后端会精确计算）
  return Math.round(((row.success_count || 0) / row.task_count) * 100)
}

const formatTime = (time: string) => {
  if (!time) return '-'
  return time.replace('T', ' ').substring(0, 19)
}

const goToDetail = (taskGroupId: string) => {
  router.push(`${detailBasePath.value}/${taskGroupId}`)
}

const getTaskTypeLabel = (type: string) => {
  const normalized = normalizeType(type)
  switch (normalized) {
    case 'CHECK': return translate('generatedScript.common_detection_b3ff0c')
    case 'FIX': return translate('generatedScript.common_repair_590253')
    case 'POC_VERIFY': return translate('generatedScript.common_poc_verification_2e1c70')
    case 'VULNERABILITY_FIX': return translate('generatedScript.common_bug_fixes_091102')
    default: return type
  }
}

const getTaskDisplayStatus = (task: any) => {
  const taskType = normalizeType(task.task_type)
  const status = normalizeStatus(task.status)
  const exitCode = task.exit_code

  if (status === 'pending') return translate('generatedScript.common_to_be_executed_6cf0af')
  if (status === 'running') return translate('generatedScript.common_executing_1f425b')
  if (status === 'timeout') return translate('generatedScript.common_time_out_ff06c2')
  if (status === 'audit_blocked') return translate('generatedScript.taskCenter_audit_failed_85ee93')
  if (status === 'success') {
    if (taskType === 'CHECK' || taskType === 'POC_VERIFY') {
      return exitCode === 0 ? translate('generatedScript.taskCenter_pass_dcc423') : translate('generatedScript.taskCenter_failed_349c9e')
    }
    return exitCode === 0 ? translate('generatedScript.common_success_51991a') : translate('generatedScript.common_fail_3e3c80')
  }
  if (status === 'failed') return translate('generatedScript.common_fail_3e3c80')
  return status
}

const exportExcelReport = async () => {
  exporting.value = true
  try {
    const headers = [
      translate('generatedScript.taskCenter_task_group_id_c17fc0'), translate('generatedScript.taskCenter_rule_title_298a16'), translate('generatedScript.common_host_2e8a0c'), translate('generatedScript.common_host_id_62fac9'), translate('generatedScript.taskCenter_task_type_4a6f41'), translate('generatedScript.common_state_62e951'), translate('generatedScript.taskCenter_exit_code_8c8923'),
      translate('generatedScript.taskCenter_pass_dcc423'), translate('generatedScript.taskCenter_vulnerability_id_6066ec'), translate('generatedScript.taskCenter_automatic_verification_30b2d5'), translate('generatedScript.taskCenter_number_of_verification_rounds_6996eb'), translate('generatedScript.taskCenter_maximum_number_of_rounds_7b621d'),
      translate('generatedScript.common_script_content_2a33ea'), translate('generatedScript.taskCenter_standard_output_8286ef'), translate('generatedScript.taskCenter_error_output_d6e03d'),
      translate('generatedScript.taskCenter_creation_time_84e380'), translate('generatedScript.taskCenter_start_time_e8868a'), translate('generatedScript.taskCenter_end_time_a0bb9f'),
    ]

    const allRows: (string | number)[][] = []

    for (const group of taskGroups.value) {
      try {
        const tasks = await getTaskLogs(group.task_group_id)
        for (const task of tasks) {
          const passed = normalizeStatus(task.status) === 'success' && (task.exit_code ?? 0) === 0 ? translate('generatedScript.taskCenter_yes_30160a') : translate('generatedScript.taskCenter_no_8bf5c1')
          allRows.push([
            group.task_group_id,
            task.rule_title || task.rule_id || task.vulnerability_id || '-',
            task.hostname || task.host_id || '-',
            task.host_id || '-',
            getTaskTypeLabel(task.task_type),
            getTaskDisplayStatus(task),
            task.exit_code ?? '-',
            passed,
            task.vulnerability_id || '-',
            task.auto_verify ? translate('generatedScript.taskCenter_yes_30160a') : translate('generatedScript.taskCenter_no_8bf5c1'),
            task.verify_round ?? '-',
            task.max_rounds ?? '-',
            task.script_content || '-',
            task.stdout || '-',
            task.stderr || '-',
            formatTime(task.created_at || group.created_at),
            formatTime(task.started_at),
            formatTime(task.finished_at),
          ])
        }
      } catch {
        allRows.push([
          group.task_group_id,
          translate('generatedScript.taskCenter_failed_to_obtain_details_3c92f5'),
          '-', '-',
          getTaskTypeLabel(group.task_type),
          getStatusText(normalizeStatus(group.status)),
          '-', '-', '-', '-', '-', '-', '-', '-', '-',
          formatTime(group.created_at),
          '-', '-',
        ])
      }
    }

    if (allRows.length === 0) {
      ElMessage.warning(translate('generatedScript.taskCenter_there_are_currently_no_tasks_available_2b4313'))
      return
    }

    const csv = buildCsv(headers, allRows)
    const ts = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '')
    const prefix = isVulnerabilityTask.value ? translate('generatedScript.taskCenter_vulnerability_task_details_9af140') : translate('generatedScript.taskCenter_baseline_task_details_4ec2cd')
    downloadCsv(`${prefix}_${ts}.csv`, csv)
    ElMessage.success(translate('generatedScript.taskCenter_task_details_have_been_exported_a73be4', { p0: allRows.length }))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.taskCenter_export_failed_675338'))
  } finally {
    exporting.value = false
  }
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
    ElMessage.warning(translate('generatedScript.taskCenter_running_tasks_cannot_be_deleted_211561'))
    return
  }

  try {
    await ElMessageBox.confirm(translate('generatedScript.taskCenter_are_you_sure_you_want_to_18b598', { p0: row.task_group_id.substring(0, 8) }), translate('generatedScript.common_confirm_deletion_3c06ab'), {
      confirmButtonText: translate('generatedScript.common_delete_3755f5'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning'
    })

    await deleteTaskGroup(row.task_group_id)
    ElMessage.success(translate('generatedScript.taskCenter_task_deleted_1ae6b1'))
    fetchTasks()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || translate('generatedScript.common_delete_failed_72250c'))
    }
  }
}

const handleBatchDelete = async () => {
  if (selectedTaskIds.value.length === 0) {
    ElMessage.warning(translate('generatedScript.taskCenter_please_select_the_task_to_delete_de9134'))
    return
  }

  const deletableTasks = taskGroups.value.filter(
    item => selectedTaskIds.value.includes(item.task_group_id) &&
            item.status !== 'running' &&
            item.status !== 'pending'
  )

  const skippedCount = selectedTaskIds.value.length - deletableTasks.length

  if (deletableTasks.length === 0) {
    ElMessage.warning(translate('generatedScript.taskCenter_the_selected_tasks_are_all_running_5dc594'))
    return
  }

  let message = translate('generatedScript.taskCenter_are_you_sure_you_want_to_ff9528', { p0: deletableTasks.length })
  if (skippedCount > 0) {
    message += translate('generatedScript.taskCenter_running_tasks_skipped_606245', { p0: skippedCount })
  }

  try {
    await ElMessageBox.confirm(message, translate('generatedScript.common_batch_deletion_confirmation_dc0217'), {
      confirmButtonText: translate('generatedScript.common_delete_3755f5'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning'
    })

    const result = await batchDeleteTasks(deletableTasks.map(t => t.task_group_id))
    const deletedCount = result?.deleted_count ?? 0
    const resultSkippedCount = result?.skipped_count ?? 0
    ElMessage.success(translate('generatedScript.taskCenter_successfully_deleted_tasks_9ed20a', { p0: deletedCount, p1: resultSkippedCount > 0 ? translate('generatedScript.taskCenter_skipping_040c7c', { p0: resultSkippedCount }) : '' }))
    fetchTasks()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || translate('generatedScript.common_batch_deletion_failed_b59edb'))
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

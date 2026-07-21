<template>
  <div class="collections-page">
    <!-- 筛选区 -->
    <el-card class="filter-card">
      <div class="filter-row">
        <el-select
          v-model="filters.status"
          :placeholder="$t('generated.common_task_status_b7d412')"
          clearable
          style="width: 150px"
        >
          <el-option :label="$t('generated.hostsAssetsCollections_collecting_b5de8d')" value="collecting" />
          <el-option :label="$t('generated.hostsAssetsCollections_analyzing_2c2a14')" value="analyzing" />
          <el-option :label="$t('generated.common_finish_33246f')" value="completed" />
          <el-option :label="$t('generated.common_fail_3e3c80')" value="failed" />
          <el-option :label="$t('generated.hostsAssetsCollections_canceled_a5ffdc')" value="cancelled" />
        </el-select>

        <el-button type="primary" @click="handleSearch">
          <el-icon><Search /></el-icon>
          {{ $t('generated.common_query_711363') }}
        </el-button>

        <el-button @click="handleReset">
          <el-icon><RefreshRight /></el-icon>
          {{ $t('generated.common_reset_3d8134') }}
        </el-button>
      </div>
    </el-card>

    <!-- 数据表格 -->
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.hostsAssetsCollections_collection_tasks_d3aae6') }}</span>
          <div class="header-actions">
            <el-button type="success" @click="triggerManualCollection" :loading="collecting">
              <el-icon><Refresh /></el-icon>
              {{ $t('generated.common_collect_now_abbbcf') }}
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="collectionTasks"
        v-loading="loading"
        border
        stripe
        style="width: 100%"
      >
        <el-table-column prop="id" :label="$t('generated.hostsAssetsCollections_task_id_041516')" width="280" show-overflow-tooltip />

        <el-table-column prop="task_type" :label="$t('generated.hostsAssetsCollections_task_type_4a6f41')" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ getTaskTypeLabel(row.task_type) }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="trigger_source" :label="$t('generated.hostsAssetsCollections_trigger_mode_f67f18')" width="100">
          <template #default="{ row }">
            <el-tag :type="getTriggerType(row.trigger_source)" size="small">
              {{ getTriggerLabel(row.trigger_source) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="scope" :label="$t('generated.hostsAssetsCollections_host_range_58c301')" width="100" show-overflow-tooltip />

        <el-table-column prop="status" :label="$t('generated.common_state_62e951')" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="total_hosts" :label="$t('generated.hostsAssetsCollections_total_number_of_hosts_a2c65d')" width="100" align="center" />

        <el-table-column prop="success_hosts" :label="$t('generated.common_success_51991a')" width="80" align="center">
          <template #default="{ row }">
            <span class="success-count">{{ row.success_hosts }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="failed_hosts" :label="$t('generated.common_fail_3e3c80')" width="80" align="center">
          <template #default="{ row }">
            <span :class="{ 'fail-count': row.failed_hosts > 0 }">{{ row.failed_hosts }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="current_stage" :label="$t('generated.hostsAssetsCollections_current_stage_0a2489')" width="120" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.current_stage || '-' }}
          </template>
        </el-table-column>

        <el-table-column prop="error_message" :label="$t('generated.hostsAssetsCollections_error_summary_e12773')" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="error-message">{{ row.error_message || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="started_at" :label="$t('generated.hostsAssetsCollections_start_time_e8868a')" width="180">
          <template #default="{ row }">
            {{ row.started_at ? formatTime(row.started_at) : '-' }}
          </template>
        </el-table-column>

        <el-table-column prop="finished_at" :label="$t('generated.hostsAssetsCollections_end_time_a0bb9f')" width="180">
          <template #default="{ row }">
            {{ row.finished_at ? formatTime(row.finished_at) : '-' }}
          </template>
        </el-table-column>

        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewDetail(row.id)">
              {{ $t('generated.common_details_4f55ee') }}
            </el-button>
            <el-button
              v-if="row.status === 'failed'"
              link
              type="warning"
              @click="retryTask(row.id)"
            >
              {{ $t('generated.common_try_again_e2d53a') }}
            </el-button>
            <el-button
              v-if="row.status === 'collecting' || row.status === 'analyzing'"
              link
              type="danger"
              @click="cancelTask(row.id)"
            >
              {{ $t('generated.common_cancel_4d0b46') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="filters.page"
          v-model:page-size="filters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          :total="taskTotal"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <!-- 任务详情抽屉 -->
    <el-drawer
      v-model="showDetailDrawer"
      :title="$t('generated.hostsAssetsCollections_mission_details_b19fb2')"
      size="700px"
    >
      <div v-if="selectedTask" class="task-detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_task_id_041516')">{{ selectedTask.id }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_task_type_4a6f41')">{{ getTaskTypeLabel(selectedTask.task_type) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_trigger_mode_f67f18')">
            <el-tag :type="getTriggerType(selectedTask.trigger_source)">
              {{ getTriggerLabel(selectedTask.trigger_source) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_state_62e951')">
            <el-tag :type="getStatusType(selectedTask.status)">
              {{ getStatusLabel(selectedTask.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_total_number_of_hosts_a2c65d')">{{ selectedTask.total_hosts }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_number_of_successful_hosts_e801e1')">{{ selectedTask.success_hosts }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_number_of_failed_hosts_f68623')">{{ selectedTask.failed_hosts }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_request_user_b54206')">{{ selectedTask.requested_by || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_start_time_e8868a')">{{ selectedTask.started_at ? formatTime(selectedTask.started_at) : '-' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsCollections_end_time_a0bb9f')">{{ selectedTask.finished_at ? formatTime(selectedTask.finished_at) : '-' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_error_message_a38a81')" :span="2">
            {{ selectedTask.error_message || '-' }}
          </el-descriptions-item>
        </el-descriptions>

        <!-- 主机执行明细 -->
        <div class="section-title">{{ $t('generated.hostsAssetsCollections_host_execution_details_7427cb') }}</div>
        <el-table :data="taskHosts" border stripe size="small">
          <el-table-column prop="hostname" :label="$t('generated.common_hostname_981e96')" width="150" />
          <el-table-column prop="ip_address" :label="$t('generated.common_ip_address_010efa')" width="120" />
          <el-table-column prop="status" :label="$t('generated.common_state_62e951')" width="100">
            <template #default="{ row }">
              <el-tag :type="getStatusType(row.status)" size="small">
                {{ getStatusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="software_count" :label="$t('generated.hostsAssetsCollections_number_of_software_06ef05')" width="80" align="center" />
          <el-table-column prop="process_count" :label="$t('generated.common_number_of_processes_f2b9d5')" width="80" align="center" />
          <el-table-column prop="application_count" :label="$t('generated.hostsAssetsCollections_number_of_applications_2ffa48')" width="80" align="center" />
          <el-table-column prop="error_message" :label="$t('generated.common_error_message_a38a81')" show-overflow-tooltip />
          <el-table-column prop="collect_started_at" :label="$t('generated.hostsAssetsCollections_start_time_e8868a')" width="160">
            <template #default="{ row }">
              {{ row.collect_started_at ? formatTime(row.collect_started_at) : '-' }}
            </template>
          </el-table-column>
          <el-table-column prop="collect_finished_at" :label="$t('generated.hostsAssetsCollections_end_time_a0bb9f')" width="160">
            <template #default="{ row }">
              {{ row.collect_finished_at ? formatTime(row.collect_finished_at) : '-' }}
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { ref, onMounted } from 'vue'
import { Search, RefreshRight, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAssetStore } from '@/store/assets'
import { storeToRefs } from 'pinia'
import { getCollectionTask, type CollectionTask } from '@/api/assets'

const assetStore = useAssetStore()
const {
  collectionTasks,
  taskTotal,
  loading,
  collecting,
  taskFilters: filters,
} = storeToRefs(assetStore)

const showDetailDrawer = ref(false)
const selectedTask = ref<CollectionTask | null>(null)
const taskHosts = ref<any[]>([])

// 初始化
onMounted(() => {
  assetStore.fetchCollectionTasks()
})

// 搜索
function handleSearch() {
  filters.value.page = 1
  assetStore.fetchCollectionTasks()
}

// 重置筛选
function handleReset() {
  assetStore.resetTaskFilters()
  assetStore.fetchCollectionTasks()
}

// 分页处理
function handleSizeChange() {
  filters.value.page = 1
  assetStore.fetchCollectionTasks()
}

function handlePageChange() {
  assetStore.fetchCollectionTasks()
}

// 触发手动采集
async function triggerManualCollection() {
  try {
    await ElMessageBox.confirm(translate('generatedScript.common_are_you_sure_you_want_to_aabc95'), translate('generatedScript.common_confirm_b56d9a'), {
      confirmButtonText: translate('generatedScript.common_sure_f526c8'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'info',
    })

    await assetStore.triggerCollection({
      scope: 'all_hosts',
      types: ['process', 'software', 'application_analysis'],
    })

    ElMessage.success(translate('generatedScript.hostsAssetsCollections_collection_task_has_been_created_d1f008'))
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(translate('generatedScript.common_trigger_acquisition_failed_3c4208'))
    }
  }
}

// 查看详情
async function viewDetail(taskId: string) {
  try {
    const result = await getCollectionTask(taskId)
    selectedTask.value = result.task
    taskHosts.value = result.hosts || []
    showDetailDrawer.value = true
  } catch (error) {
    ElMessage.error(translate('generatedScript.common_failed_to_get_details_b8ba3a'))
  }
}

// 重试任务
async function retryTask(taskId: string) {
  try {
    await ElMessageBox.confirm(translate('generatedScript.hostsAssetsCollections_are_you_sure_you_want_to_6cd8d4'), translate('generatedScript.common_confirm_b56d9a'), {
      confirmButtonText: translate('generatedScript.common_sure_f526c8'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning',
    })

    await assetStore.retryTask(taskId)
    ElMessage.success(translate('generatedScript.hostsAssetsCollections_retry_task_created_471a9c'))
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(translate('generatedScript.hostsAssetsCollections_retry_failed_8af761'))
    }
  }
}

// 取消任务
async function cancelTask(taskId: string) {
  try {
    await ElMessageBox.confirm(translate('generatedScript.hostsAssetsCollections_are_you_sure_you_want_to_a2eb3e'), translate('generatedScript.common_confirm_b56d9a'), {
      confirmButtonText: translate('generatedScript.common_sure_f526c8'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning',
    })

    await assetStore.cancelTask(taskId)
    ElMessage.success(translate('generatedScript.hostsAssetsCollections_task_canceled_6df9b7'))
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(translate('generatedScript.hostsAssetsCollections_cancellation_failed_3e62cc'))
    }
  }
}

// 格式化时间
function formatTime(time: string) {
  if (!time) return '-'
  return formatDateTime(time)
}

// 获取任务类型标签
function getTaskTypeLabel(type: string) {
  const labels: Record<string, string> = {
    full: translate('generatedScript.hostsAssetsCollections_complete_collection_6abc5f'),
    process: translate('generatedScript.hostsAssetsCollections_process_collection_8627b4'),
    application_analysis: translate('generatedScript.hostsAssetsCollections_application_analysis_dbaa8d'),
  }
  return labels[type] || type
}

// 获取触发方式类型
function getTriggerType(trigger: string) {
  const types: Record<string, string> = {
    manual: 'primary',
    schedule: 'success',
    agent_register: 'info',
  }
  return types[trigger] || 'info'
}

// 获取触发方式标签
function getTriggerLabel(trigger: string) {
  const labels: Record<string, string> = {
    manual: translate('generatedScript.hostsAssetsCollections_manual_2a4a4d'),
    schedule: translate('generatedScript.hostsAssetsCollections_cycle_8a12b1'),
    agent_register: translate('generatedScript.hostsAssetsCollections_register_trigger_ccef5a'),
  }
  return labels[trigger] || trigger
}

// 获取状态类型
function getStatusType(status: string) {
  const types: Record<string, string> = {
    collecting: 'warning',
    analyzing: 'primary',
    completed: 'success',
    failed: 'danger',
    cancelled: 'info',
  }
  return types[status] || 'info'
}

// 获取状态标签
function getStatusLabel(status: string) {
  const labels: Record<string, string> = {
    collecting: translate('generatedScript.common_collecting_b5de8d'),
    analyzing: translate('generatedScript.common_analyzing_2c2a14'),
    completed: translate('generatedScript.common_finish_33246f'),
    failed: translate('generatedScript.common_fail_3e3c80'),
    cancelled: translate('generatedScript.common_canceled_a5ffdc'),
  }
  return labels[status] || status
}
</script>

<style scoped>
.collections-page {
  padding: 20px;
}

.filter-card {
  margin-bottom: 20px;
}

.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.success-count {
  color: #67C23A;
  font-weight: 600;
}

.fail-count {
  color: #F56C6C;
  font-weight: 600;
}

.error-message {
  color: #F56C6C;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.task-detail {
  padding: 20px;
}

.section-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin: 20px 0 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid #EBEEF5;
}
</style>

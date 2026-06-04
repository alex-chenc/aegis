<template>
  <div class="collections-page">
    <!-- 筛选区 -->
    <el-card class="filter-card">
      <div class="filter-row">
        <el-select
          v-model="filters.status"
          placeholder="任务状态"
          clearable
          style="width: 150px"
        >
          <el-option label="采集中" value="collecting" />
          <el-option label="分析中" value="analyzing" />
          <el-option label="完成" value="completed" />
          <el-option label="失败" value="failed" />
          <el-option label="已取消" value="cancelled" />
        </el-select>

        <el-button type="primary" @click="handleSearch">
          <el-icon><Search /></el-icon>
          查询
        </el-button>

        <el-button @click="handleReset">
          <el-icon><RefreshRight /></el-icon>
          重置
        </el-button>
      </div>
    </el-card>

    <!-- 数据表格 -->
    <el-card>
      <template #header>
        <div class="card-header">
          <span>采集任务</span>
          <div class="header-actions">
            <el-button type="success" @click="triggerManualCollection" :loading="collecting">
              <el-icon><Refresh /></el-icon>
              立即采集
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
        <el-table-column prop="id" label="任务 ID" width="280" show-overflow-tooltip />

        <el-table-column prop="task_type" label="任务类型" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ getTaskTypeLabel(row.task_type) }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="trigger_source" label="触发方式" width="100">
          <template #default="{ row }">
            <el-tag :type="getTriggerType(row.trigger_source)" size="small">
              {{ getTriggerLabel(row.trigger_source) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="scope" label="主机范围" width="100" />

        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="total_hosts" label="总主机数" width="100" align="center" />

        <el-table-column prop="success_hosts" label="成功" width="80" align="center">
          <template #default="{ row }">
            <span class="success-count">{{ row.success_hosts }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="failed_hosts" label="失败" width="80" align="center">
          <template #default="{ row }">
            <span :class="{ 'fail-count': row.failed_hosts > 0 }">{{ row.failed_hosts }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="current_stage" label="当前阶段" width="120">
          <template #default="{ row }">
            {{ row.current_stage || '-' }}
          </template>
        </el-table-column>

        <el-table-column prop="error_message" label="错误摘要" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="error-message">{{ row.error_message || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="started_at" label="开始时间" width="180">
          <template #default="{ row }">
            {{ row.started_at ? formatTime(row.started_at) : '-' }}
          </template>
        </el-table-column>

        <el-table-column prop="finished_at" label="结束时间" width="180">
          <template #default="{ row }">
            {{ row.finished_at ? formatTime(row.finished_at) : '-' }}
          </template>
        </el-table-column>

        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewDetail(row.id)">
              详情
            </el-button>
            <el-button
              v-if="row.status === 'failed'"
              link
              type="warning"
              @click="retryTask(row.id)"
            >
              重试
            </el-button>
            <el-button
              v-if="row.status === 'collecting' || row.status === 'analyzing'"
              link
              type="danger"
              @click="cancelTask(row.id)"
            >
              取消
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
      title="任务详情"
      size="700px"
    >
      <div v-if="selectedTask" class="task-detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="任务 ID">{{ selectedTask.id }}</el-descriptions-item>
          <el-descriptions-item label="任务类型">{{ getTaskTypeLabel(selectedTask.task_type) }}</el-descriptions-item>
          <el-descriptions-item label="触发方式">
            <el-tag :type="getTriggerType(selectedTask.trigger_source)">
              {{ getTriggerLabel(selectedTask.trigger_source) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="getStatusType(selectedTask.status)">
              {{ getStatusLabel(selectedTask.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="总主机数">{{ selectedTask.total_hosts }}</el-descriptions-item>
          <el-descriptions-item label="成功主机数">{{ selectedTask.success_hosts }}</el-descriptions-item>
          <el-descriptions-item label="失败主机数">{{ selectedTask.failed_hosts }}</el-descriptions-item>
          <el-descriptions-item label="请求用户">{{ selectedTask.requested_by || '-' }}</el-descriptions-item>
          <el-descriptions-item label="开始时间">{{ selectedTask.started_at ? formatTime(selectedTask.started_at) : '-' }}</el-descriptions-item>
          <el-descriptions-item label="结束时间">{{ selectedTask.finished_at ? formatTime(selectedTask.finished_at) : '-' }}</el-descriptions-item>
          <el-descriptions-item label="错误信息" :span="2">
            {{ selectedTask.error_message || '-' }}
          </el-descriptions-item>
        </el-descriptions>

        <!-- 主机执行明细 -->
        <div class="section-title">主机执行明细</div>
        <el-table :data="taskHosts" border stripe size="small">
          <el-table-column prop="hostname" label="主机名" width="150" />
          <el-table-column prop="ip_address" label="IP 地址" width="120" />
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="getStatusType(row.status)" size="small">
                {{ getStatusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="software_count" label="软件数" width="80" align="center" />
          <el-table-column prop="process_count" label="进程数" width="80" align="center" />
          <el-table-column prop="application_count" label="应用数" width="80" align="center" />
          <el-table-column prop="error_message" label="错误信息" show-overflow-tooltip />
          <el-table-column prop="collect_started_at" label="开始时间" width="160">
            <template #default="{ row }">
              {{ row.collect_started_at ? formatTime(row.collect_started_at) : '-' }}
            </template>
          </el-table-column>
          <el-table-column prop="collect_finished_at" label="结束时间" width="160">
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
    await ElMessageBox.confirm('确定要立即执行资产采集吗？', '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'info',
    })

    await assetStore.triggerCollection({
      scope: 'all_hosts',
      types: ['process', 'application_analysis'],
    })

    ElMessage.success('采集任务已创建')
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('触发采集失败')
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
    ElMessage.error('获取详情失败')
  }
}

// 重试任务
async function retryTask(taskId: string) {
  try {
    await ElMessageBox.confirm('确定要重试失败的主机吗？', '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })

    await assetStore.retryTask(taskId)
    ElMessage.success('重试任务已创建')
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('重试失败')
    }
  }
}

// 取消任务
async function cancelTask(taskId: string) {
  try {
    await ElMessageBox.confirm('确定要取消此任务吗？', '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })

    await assetStore.cancelTask(taskId)
    ElMessage.success('任务已取消')
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('取消失败')
    }
  }
}

// 格式化时间
function formatTime(time: string) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

// 获取任务类型标签
function getTaskTypeLabel(type: string) {
  const labels: Record<string, string> = {
    full: '完整采集',
    process: '进程采集',
    application_analysis: '应用分析',
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
    manual: '手动',
    schedule: '周期',
    agent_register: '注册触发',
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
    collecting: '采集中',
    analyzing: '分析中',
    completed: '完成',
    failed: '失败',
    cancelled: '已取消',
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

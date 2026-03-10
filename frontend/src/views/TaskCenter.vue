<template>
  <div class="task-center">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>任务中心</span>
          <el-button @click="refresh" :loading="loading">刷新</el-button>
        </div>
      </template>

      <div class="filter-bar">
        <el-select v-model="filters.status" placeholder="状态" clearable style="width: 120px" @change="handleFilterChange">
          <el-option label="待执行" value="pending" />
          <el-option label="执行中" value="running" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="部分成功" value="partial" />
        </el-select>

        <el-select v-model="filters.task_type" placeholder="类型" clearable style="width: 100px; margin-left: 10px" @change="handleFilterChange">
          <el-option label="检测" value="check" />
          <el-option label="修复" value="fix" />
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

      <el-table :data="taskGroups" style="width: 100%; margin-top: 15px" v-loading="loading">
        <el-table-column prop="task_group_id" label="任务组ID" width="280">
          <template #default="{ row }">
            <el-link type="primary" @click="goToDetail(row.task_group_id)">
              {{ row.task_group_id.substring(0, 8) }}...
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="task_type" label="类型" width="80">
          <template #default="{ row }">
            <el-tag :type="row.task_type === 'check' ? 'primary' : 'warning'" size="small">
              {{ row.task_type === 'check' ? '检测' : '修复' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="task_count" label="任务数" width="80" />
        <el-table-column label="进度" width="200">
          <template #default="{ row }">
            <div class="progress-info">
              <span class="success">{{ row.success_count }}</span> /
              <span class="failed">{{ row.failed_count }}</span> /
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
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="goToDetail(row.task_group_id)">
              详情
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
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { listTasks, type TaskGroupSummary } from '@/api/tasks'

const router = useRouter()

const loading = ref(false)
const taskGroups = ref<TaskGroupSummary[]>([])

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

const fetchTasks = async () => {
  loading.value = true
  try {
    const result = await listTasks({
      page: pagination.page,
      page_size: pagination.pageSize,
      status: filters.status || undefined,
      task_type: filters.task_type || undefined,
      search: filters.search || undefined
    })
    taskGroups.value = result.items
    pagination.total = result.total
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

const getStatusType = (status: string) => {
  switch (status) {
    case 'pending': return 'info'
    case 'running': return 'warning'
    case 'success': return 'success'
    case 'failed': return 'danger'
    case 'partial': return 'warning'
    default: return 'info'
  }
}

const getStatusText = (status: string) => {
  switch (status) {
    case 'pending': return '待执行'
    case 'running': return '执行中'
    case 'success': return '成功'
    case 'failed': return '失败'
    case 'partial': return '部分成功'
    default: return status
  }
}

const formatTime = (time: string) => {
  if (!time) return '-'
  return time.replace('T', ' ').substring(0, 19)
}

const goToDetail = (taskGroupId: string) => {
  router.push(`/tasks/${taskGroupId}`)
}

onMounted(() => {
  fetchTasks()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.filter-bar {
  display: flex;
  align-items: center;
}

.progress-info {
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

.pagination-container {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>
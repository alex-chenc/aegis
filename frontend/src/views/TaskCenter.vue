<template>
  <div class="task-center">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ isVulnerabilityTask ? '漏洞任务中心' : '任务中心' }}</span>
          <div class="header-actions">
            <el-button v-if="selectedGroups.length > 0" type="danger" @click="batchDeleteTasks">
              批量删除 ({{ selectedGroups.length }})
            </el-button>
            <el-button :icon="Refresh" circle @click="fetchTasks" />
          </div>
        </div>
      </template>

      <el-tabs v-model="activeTab" @tab-change="handleTabChange">
        <el-tab-pane label="全部任务" name="all">
          <task-group-table
            :groups="taskGroups"
            :is-vulnerability="isVulnerabilityTask"
            :loading="loading"
            :selected="selectedGroups"
            @select="handleSelect"
            @view="viewGroupDetail"
            @delete="deleteTaskGroup"
          />
        </el-tab-pane>

        <el-tab-pane label="运行中" name="running">
          <task-group-table
            :groups="runningGroups"
            :is-vulnerability="isVulnerabilityTask"
            :loading="loading"
            :selected="selectedGroups"
            @select="handleSelect"
            @view="viewGroupDetail"
            @delete="deleteTaskGroup"
          />
        </el-tab-pane>

        <el-tab-pane label="已完成" name="success">
          <task-group-table
            :groups="successGroups"
            :is-vulnerability="isVulnerabilityTask"
            :loading="loading"
            :selected="selectedGroups"
            @select="handleSelect"
            @view="viewGroupDetail"
            @delete="deleteTaskGroup"
          />
        </el-tab-pane>

        <el-tab-pane label="失败/超时" name="failed">
          <task-group-table
            :groups="failedGroups"
            :is-vulnerability="isVulnerabilityTask"
            :loading="loading"
            :selected="selectedGroups"
            @select="handleSelect"
            @view="viewGroupDetail"
            @delete="deleteTaskGroup"
          />
        </el-tab-pane>
      </el-tabs>

      <div class="pagination">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <!-- 任务组详情对话框 -->
    <el-dialog v-model="detailVisible" :title="`任务组详情 - ${selectedGroup?.task_group_id?.substring(0, 8) || ''}`" width="90%" top="5vh">
      <div v-if="groupTasks.length > 0" class="group-detail">
        <el-descriptions :column="4" border size="small" class="group-stats">
          <el-descriptions-item label="总任务数">{{ selectedGroup?.task_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="待执行">
            <el-tag type="info">{{ selectedGroup?.pending_count || 0 }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="执行中">
            <el-tag type="warning">{{ selectedGroup?.running_count || 0 }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="成功">
            <el-tag type="success">{{ selectedGroup?.success_count || 0 }}</el-tag>
          </el-descriptions-item>
        </el-descriptions>

        <el-table :data="groupTasks" stripe border style="width: 100%; margin-top: 16px" max-height="400">
          <el-table-column prop="task_type" label="类型" width="100">
            <template #default="{ row }">
              <el-tag :type="getTaskTypeTag(row.task_type)" size="small">
                {{ getTaskTypeLabel(row.task_type) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="getStatusTag(row.status)" size="small">
                {{ getStatusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="host_id" label="主机" width="180">
            <template #default="{ row }">
              {{ getHostLabel(row.host_id) }}
            </template>
          </el-table-column>
          <el-table-column prop="exit_code" label="退出码" width="80" align="center">
            <template #default="{ row }">
              <span :class="{'text-success': row.exit_code === 0, 'text-danger': row.exit_code !== 0 && row.exit_code !== null}">
                {{ row.exit_code ?? '-' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150" fixed="right">
            <template #default="{ row }">
              <el-button size="small" @click="viewTaskOutput(row)">查看输出</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <el-empty v-else description="暂无任务数据" />
    </el-dialog>

    <!-- 任务输出对话框 -->
    <el-dialog v-model="outputVisible" title="任务输出" width="800px">
      <el-descriptions :column="2" border v-if="selectedTask">
        <el-descriptions-item label="任务 ID">{{ selectedTask.id }}</el-descriptions-item>
        <el-descriptions-item label="退出码">
          <span :class="{'text-success': selectedTask.exit_code === 0, 'text-danger': selectedTask.exit_code !== 0 && selectedTask.exit_code !== null}">
            {{ selectedTask.exit_code ?? '-' }}
          </span>
        </el-descriptions-item>
      </el-descriptions>
      <el-tabs v-if="selectedTask">
        <el-tab-pane label="标准输出">
          <pre class="log-output">{{ selectedTask.stdout || '无输出' }}</pre>
        </el-tab-pane>
        <el-tab-pane label="错误输出">
          <pre class="log-output error">{{ selectedTask.stderr || '无错误' }}</pre>
        </el-tab-pane>
      </el-tabs>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, defineComponent, h } from 'vue'
import { ElMessage, ElMessageBox, ElTag, ElButton, ElTable, ElTableColumn, ElCheckbox } from 'element-plus'
import { Refresh, Loading, View, Delete } from '@element-plus/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import { useHostStore } from '@/store/hosts'
import * as taskApi from '@/api/task'

interface Task {
  id: string
  task_group_id: string
  rule_id: string | null
  host_id: string
  vulnerability_id: string | null
  task_type: string
  status: 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'TIMEOUT'
  script_content: string | null
  script_version: number | null
  stdout: string | null
  stderr: string | null
  exit_code: number | null
  started_at: string | null
  finished_at: string | null
  created_at: string
}

interface TaskGroup {
  task_group_id: string
  task_count: number
  task_type: string
  status: string
  success_count: number
  failed_count: number
  pending_count: number
  running_count: number
  created_at: string
  finished_at: string | null
}

// Task Group Table Component
const TaskGroupTable = defineComponent({
  name: 'TaskGroupTable',
  props: {
    groups: { type: Array as () => TaskGroup[], required: true },
    isVulnerability: { type: Boolean, default: false },
    loading: { type: Boolean, default: false },
    selected: { type: Array as () => string[], default: () => [] }
  },
  emits: ['select', 'view', 'delete'],
  setup(props, { emit }) {
    const handleSelect = (groupId: string, checked: boolean) => {
      emit('select', groupId, checked)
    }
    const handleView = (group: TaskGroup) => {
      emit('view', group)
    }
    const handleDelete = (group: TaskGroup) => {
      emit('delete', group)
    }

    return () => {
      const getTaskTypeTag = (type: string) => {
        const map: Record<string, string> = {
          'CHECK': '', 'FIX': 'primary', 'VULNERABILITY_FIX': 'warning', 'POC_VERIFY': 'success'
        }
        return map[type] || ''
      }
      const getTaskTypeLabel = (type: string) => {
        const map: Record<string, string> = {
          'CHECK': '基线检查', 'FIX': '基线修复', 'VULNERABILITY_FIX': '漏洞修复', 'POC_VERIFY': 'POC验证'
        }
        return map[type] || type
      }
      const getStatusTag = (status: string) => {
        const map: Record<string, string> = {
          'pending': 'info', 'running': 'warning', 'success': 'success', 'failed': 'danger', 'partial': 'warning'
        }
        return map[status] || ''
      }
      const getStatusLabel = (status: string) => {
        const map: Record<string, string> = {
          'pending': '等待中', 'running': '执行中', 'success': '成功', 'failed': '失败', 'partial': '部分成功'
        }
        return map[status] || status
      }
      const formatTime = (time: string | null) => {
        if (!time) return '-'
        return new Date(time).toLocaleString('zh-CN')
      }

      return h(ElTable, {
        data: props.groups,
        stripe: true,
        border: true,
        style: 'width: 100%'
      }, {
        default: () => [
          h(ElTableColumn, { type: 'selection', width: '55' }),
          h(ElTableColumn, { prop: 'task_group_id', label: '任务组ID', width: '150' }, {
            default: ({ row }: { row: TaskGroup }) => h('span', {}, row.task_group_id.substring(0, 8) + '...')
          }),
          h(ElTableColumn, { prop: 'task_type', label: '类型', width: '100' }, {
            default: ({ row }: { row: TaskGroup }) => 
              h(ElTag, { type: getTaskTypeTag(row.task_type), size: 'small' }, () => getTaskTypeLabel(row.task_type))
          }),
          h(ElTableColumn, { prop: 'task_count', label: '任务数', width: '80', align: 'center' }),
          h(ElTableColumn, { label: '进度', width: '200' }, {
            default: ({ row }: { row: TaskGroup }) => 
              h('div', { class: 'progress-info' }, [
                h(ElTag, { type: 'info', size: 'small' }, () => `待执行: ${row.pending_count}`),
                h(ElTag, { type: 'warning', size: 'small' }, () => `执行中: ${row.running_count}`),
                h(ElTag, { type: 'success', size: 'small' }, () => `成功: ${row.success_count}`),
                h(ElTag, { type: 'danger', size: 'small' }, () => `失败: ${row.failed_count}`)
              ])
          }),
          h(ElTableColumn, { prop: 'status', label: '状态', width: '100' }, {
            default: ({ row }: { row: TaskGroup }) => 
              h(ElTag, { type: getStatusTag(row.status), size: 'small' }, () => getStatusLabel(row.status))
          }),
          h(ElTableColumn, { prop: 'created_at', label: '创建时间', width: '180' }, {
            default: ({ row }: { row: TaskGroup }) => formatTime(row.created_at)
          }),
          h(ElTableColumn, { label: '操作', width: '150', fixed: 'right' }, {
            default: ({ row }: { row: TaskGroup }) => [
              h(ElButton, { size: 'small', onClick: () => handleView(row) }, () => '详情'),
              h(ElButton, { 
                size: 'small', 
                type: 'danger', 
                disabled: row.status === 'running' || row.status === 'pending',
                onClick: () => handleDelete(row) 
              }, () => '删除')
            ]
          })
        ]
      })
    }
  }
})

const route = useRoute()
const router = useRouter()
const hostStore = useHostStore()
const activeTab = ref('all')
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const taskGroups = ref<TaskGroup[]>([])
const selectedGroups = ref<string[]>([])
const detailVisible = ref(false)
const selectedGroup = ref<TaskGroup | null>(null)
const groupTasks = ref<Task[]>([])
const outputVisible = ref(false)
const selectedTask = ref<Task | null>(null)
let refreshInterval: ReturnType<typeof setInterval> | null = null

// 根据路由判断是否是漏洞任务中心
const isVulnerabilityTask = computed(() => {
  return route.path.startsWith('/vulnerability/tasks')
})

const runningGroups = computed(() => 
  taskGroups.value.filter(g => g.status === 'running' || g.status === 'pending')
)
const successGroups = computed(() => 
  taskGroups.value.filter(g => g.status === 'success')
)
const failedGroups = computed(() => 
  taskGroups.value.filter(g => g.status === 'failed' || g.status === 'partial')
)

onMounted(async () => {
  await hostStore.fetchHosts()
  fetchTasks()
  // 每 5 秒自动刷新
  refreshInterval = setInterval(() => {
    fetchTasks()
  }, 5000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})

async function fetchTasks() {
  loading.value = true
  try {
    const params: taskApi.TaskListParams = {
      page: currentPage.value,
      page_size: pageSize.value,
      status: activeTab.value === 'all' ? undefined : 
              activeTab.value === 'running' ? 'RUNNING,PENDING' :
              activeTab.value === 'success' ? 'SUCCESS' :
              'FAILED,TIMEOUT',
      // 漏洞任务中心只显示漏洞相关任务
      task_type: isVulnerabilityTask.value ? 'VULNERABILITY_FIX,POC_VERIFY' : undefined
    }
    const result = await taskApi.getTasks(params)
    taskGroups.value = result.items || []
    total.value = result.total || 0
  } catch (err: any) {
    ElMessage.error('获取任务列表失败：' + err.message)
  } finally {
    loading.value = false
  }
}

function handleTabChange() {
  currentPage.value = 1
  fetchTasks()
}

function handleSizeChange(size: number) {
  pageSize.value = size
  fetchTasks()
}

function handlePageChange(page: number) {
  currentPage.value = page
  fetchTasks()
}

function handleSelect(groupId: string, checked: boolean) {
  if (checked) {
    if (!selectedGroups.value.includes(groupId)) {
      selectedGroups.value.push(groupId)
    }
  } else {
    selectedGroups.value = selectedGroups.value.filter(id => id !== groupId)
  }
}

async function viewGroupDetail(group: TaskGroup) {
  selectedGroup.value = group
  detailVisible.value = true
  
  try {
    const tasks = await taskApi.getTaskGroupLogs(group.task_group_id)
    groupTasks.value = tasks || []
  } catch (err: any) {
    ElMessage.error('获取任务详情失败：' + err.message)
    groupTasks.value = []
  }
}

function viewTaskOutput(task: Task) {
  selectedTask.value = task
  outputVisible.value = true
}

async function deleteTaskGroup(group: TaskGroup) {
  if (group.status === 'running' || group.status === 'pending') {
    ElMessage.warning('运行中的任务无法删除')
    return
  }

  try {
    await ElMessageBox.confirm('确定要删除此任务组吗？', '确认删除', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    
    await fetch(`/api/v1/tasks/group/${group.task_group_id}`, { method: 'DELETE' })
    ElMessage.success('删除成功')
    fetchTasks()
  } catch (err: any) {
    if (err !== 'cancel') {
      ElMessage.error('删除失败：' + err.message)
    }
  }
}

async function batchDeleteTasks() {
  if (selectedGroups.value.length === 0) {
    ElMessage.warning('请选择要删除的任务')
    return
  }

  try {
    await ElMessageBox.confirm(`确定要删除选中的 ${selectedGroups.value.length} 个任务组吗？`, '确认删除', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await fetch('/api/v1/tasks/batch', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ task_ids: selectedGroups.value })
    })
    ElMessage.success('批量删除成功')
    selectedGroups.value = []
    fetchTasks()
  } catch (err: any) {
    if (err !== 'cancel') {
      ElMessage.error('批量删除失败：' + err.message)
    }
  }
}

function getTaskTypeTag(type: string) {
  const map: Record<string, string> = {
    'CHECK': '', 'FIX': 'primary', 'VULNERABILITY_FIX': 'warning', 'POC_VERIFY': 'success'
  }
  return map[type] || ''
}

function getTaskTypeLabel(type: string) {
  const map: Record<string, string> = {
    'CHECK': '基线检查', 'FIX': '基线修复', 'VULNERABILITY_FIX': '漏洞修复', 'POC_VERIFY': 'POC验证'
  }
  return map[type] || type
}

function getStatusTag(status: string) {
  const map: Record<string, string> = {
    'PENDING': 'info', 'RUNNING': 'warning', 'SUCCESS': 'success', 'FAILED': 'danger', 'TIMEOUT': 'warning'
  }
  return map[status] || ''
}

function getStatusLabel(status: string) {
  const map: Record<string, string> = {
    'PENDING': '等待中', 'RUNNING': '执行中', 'SUCCESS': '成功', 'FAILED': '失败', 'TIMEOUT': '超时'
  }
  return map[status] || status
}

function getHostLabel(hostId: string) {
  const host = hostStore.hosts.find(h => h.id === hostId)
  return host ? `${host.ip_address} (${host.hostname})` : hostId
}

function formatTime(time: string | null) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}
</script>

<style scoped>
.task-center {
  padding: 20px;
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

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.progress-info {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.group-stats {
  margin-bottom: 16px;
}

.log-output {
  background-color: #1e1e1e;
  color: #d4d4d4;
  padding: 16px;
  border-radius: 4px;
  max-height: 400px;
  overflow-y: auto;
  font-family: 'Fira Code', 'Monaco', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-wrap: break-word;
}

.log-output.error {
  color: #f48771;
}

.text-success {
  color: #67c23a;
  font-weight: 500;
}

.text-danger {
  color: #f56c6c;
  font-weight: 500;
}
</style>
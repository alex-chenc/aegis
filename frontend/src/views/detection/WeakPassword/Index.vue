<template>
  <div class="weak-password-page">
    <div class="page-toolbar">
      <div>
        <h1>智能弱密码检测</h1>
        <p>基于在线主机应用资产和字典匹配的凭据检查。</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Refresh" @click="refreshAll">刷新</el-button>
        <el-button :icon="Collection" @click="router.push('/risk/weak-password/dictionaries')">字典管理</el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="workspace-tabs">
      <el-tab-pane label="应用资产分析" name="analysis">
        <section class="panel">
          <div class="filter-row">
            <el-select v-model="scope.application_types" multiple collapse-tags placeholder="应用类型">
              <el-option v-for="item in applicationTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
            <el-select v-model="store.candidateFilters.confidence" placeholder="置信度" clearable>
              <el-option label="高" value="high" />
              <el-option label="中" value="medium" />
              <el-option label="低" value="low" />
            </el-select>
            <el-input v-model="keyword" :prefix-icon="Search" placeholder="搜索应用、主机或 IP" clearable @keyup.enter="runAnalysis" />
            <el-button type="primary" :icon="Cpu" :loading="store.analyzing" @click="runAnalysis">一键分析资产应用</el-button>
            <el-button type="success" :icon="Key" :loading="store.creatingTask" :disabled="store.candidates.length === 0" @click="openBatchCheck">一键检测</el-button>
          </div>

          <el-alert
            v-if="store.analysisResult?.error_code === 'no_application_assets'"
            type="warning"
            show-icon
            :closable="false"
            title="暂无可分析的应用资产。"
            :description="store.analysisResult?.message || '请先执行资产采集，系统将基于采集到的应用资产分析可能存在密码的配置位置。'"
          >
            <template #default>
              <div class="alert-actions">
                <el-button type="primary" size="small" @click="router.push('/hosts/assets')">去采集资产</el-button>
                <el-button size="small" @click="refreshAll">刷新资产状态</el-button>
              </div>
            </template>
          </el-alert>

          <el-empty
            v-else-if="!store.loading && store.candidates.length === 0"
            description="暂无可分析的应用资产。"
          >
            <el-button type="primary" @click="router.push('/hosts/assets')">去采集资产</el-button>
            <el-button @click="refreshAll">刷新资产状态</el-button>
          </el-empty>

          <el-table v-else v-loading="store.loading" :data="store.candidates" class="dense-table">
            <el-table-column label="主机" min-width="170">
              <template #default="{ row }">
                <div class="primary-cell">{{ row.hostname || row.host_id }}</div>
                <div class="secondary-cell">{{ row.ip_address || row.host_id }}</div>
              </template>
            </el-table-column>
            <el-table-column label="应用" min-width="180">
              <template #default="{ row }">
                <div class="primary-cell">{{ row.application_name }}</div>
                <div class="secondary-cell">{{ row.application_version || row.application_type }}</div>
              </template>
            </el-table-column>
            <el-table-column label="可能密码位置" min-width="260">
              <template #default="{ row }">
                <div class="path-list">
                  <el-tag v-for="path in row.candidate_paths.slice(0, 2)" :key="path" effect="plain">{{ path }}</el-tag>
                  <span v-if="row.candidate_paths.length === 0" class="secondary-cell">待受控辅助定位</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="130">
              <template #default="{ row }">
                <el-tag
                  :type="scanStatusType(row.scan_status)"
                  :class="{ clickable: row.scan_status === 'alert' }"
                  @click="openFindingDetail(row)"
                >
                  {{ scanStatusLabel(row.scan_status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="AI 置信度" width="120">
              <template #default="{ row }">
                <el-tag :type="confidenceTag(row.confidence)">{{ confidenceLabel(row.confidence) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="140" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="openSingleCheck(row)">检查弱密码</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="store.candidateTotal > 0" class="pagination-bar">
            <el-pagination
              v-model:current-page="store.candidateFilters.page"
              v-model:page-size="store.candidateFilters.page_size"
              :page-sizes="[10, 20, 50, 100]"
              layout="total, sizes, prev, pager, next"
              :total="store.candidateTotal"
              @size-change="fetchCandidatesPage"
              @current-change="fetchCandidatesPage"
            />
          </div>
        </section>
      </el-tab-pane>

      <el-tab-pane label="弱密码检查" name="tasks">
        <section class="panel">
          <div class="panel-head">
            <h2>检查任务</h2>
            <el-button :icon="Refresh" @click="store.fetchTasks">刷新</el-button>
          </div>
          <el-table v-loading="store.loading" :data="store.tasks" class="dense-table">
            <el-table-column label="任务" min-width="220" prop="name" />
            <el-table-column label="状态" width="150">
              <template #default="{ row }">
                <el-tag :type="taskStatusType(row.status)">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="进度" min-width="180">
              <template #default="{ row }">
                <el-progress :percentage="row.progress || 0" :stroke-width="10" />
              </template>
            </el-table-column>
            <el-table-column label="命中" width="90" prop="matched_findings" />
            <el-table-column label="失败应用" width="100" prop="failed_applications" />
            <el-table-column label="操作" width="210" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="router.push(`/risk/weak-password/tasks/${row.id}`)">查看详情</el-button>
                <el-button link type="danger" :disabled="!canDeleteTask(row.status)" @click="deleteTask(row.id)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="store.taskTotal > 0" class="pagination-bar">
            <el-pagination
              v-model:current-page="store.taskFilters.page"
              v-model:page-size="store.taskFilters.page_size"
              :page-sizes="[10, 20, 50, 100]"
              layout="total, sizes, prev, pager, next"
              :total="store.taskTotal"
              @size-change="fetchTasksPage"
              @current-change="fetchTasksPage"
            />
          </div>
        </section>
      </el-tab-pane>
    </el-tabs>

    <el-drawer v-model="checkVisible" :title="checkMode === 'batch' ? '一键检测弱密码' : '检查弱密码'" size="560px">
      <div class="drawer-stack">
        <template v-if="checkMode === 'single' && selectedCandidate">
          <div class="fact-row"><span>目标主机</span><strong>{{ selectedCandidate.hostname || selectedCandidate.host_id }}</strong></div>
          <div class="fact-row"><span>目标应用</span><strong>{{ selectedCandidate.application_name }}</strong></div>
        </template>
        <template v-else>
          <div class="fact-row"><span>检测范围</span><strong>当前 {{ store.candidates.length }} 个应用</strong></div>
        </template>

        <el-form label-position="top">
          <el-form-item label="字典策略">
            <el-checkbox-group v-model="selectedDictionaryIds" class="dictionary-list">
              <el-checkbox v-for="dict in availableDictionaries" :key="dict.id" :label="dict.id">
                {{ dict.name }}（{{ dict.entry_count }} 条）
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>
          <el-form-item label="AI 策略">
            <el-checkbox v-model="repairCollectionErrors">读取失败时 AI 修复定位</el-checkbox>
          </el-form-item>
        </el-form>
        <div class="drawer-actions">
          <el-button @click="checkVisible = false">取消</el-button>
          <el-button type="primary" :loading="store.creatingTask" @click="confirmCheck">确认检查</el-button>
        </div>
      </div>
    </el-drawer>

    <el-drawer v-model="findingVisible" title="弱密码详情" size="640px">
      <div v-if="selectedCandidate" class="drawer-stack">
        <div class="fact-row"><span>应用</span><strong>{{ selectedCandidate.application_name }}</strong></div>
        <div class="fact-row"><span>主机</span><strong>{{ selectedCandidate.hostname || selectedCandidate.host_id }}</strong></div>
        <el-table :data="selectedCandidate.findings || []" class="dense-table">
          <el-table-column label="账号" prop="account" min-width="120" />
          <el-table-column label="密码" min-width="160">
            <template #default="{ row }">
              <code class="password-mask">{{ revealedPasswords[row.id] || row.matched_password_mask }}</code>
            </template>
          </el-table-column>
          <el-table-column label="进程 PID" width="120">
            <template #default="{ row }">{{ row.process_pid || '-' }}</template>
          </el-table-column>
          <el-table-column label="来源" min-width="220">
            <template #default="{ row }">
              <div class="secondary-cell">{{ row.source_path }}</div>
              <div class="secondary-cell">{{ row.field_path }}</div>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="revealFinding(row.id)">查看明文</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Collection, Cpu, Key, Refresh, Search } from '@element-plus/icons-vue'
import { revealWeakPasswordFinding } from '@/api/weakPassword'
import { useWeakPasswordStore } from '@/store/weakPassword'
import type { WeakPasswordCandidateApplication, WeakPasswordDictionary } from '@/types/weakPassword'

const router = useRouter()
const store = useWeakPasswordStore()
const activeTab = ref('analysis')
const keyword = ref('')
const checkVisible = ref(false)
const findingVisible = ref(false)
const checkMode = ref<'single' | 'batch'>('single')
const selectedCandidate = ref<WeakPasswordCandidateApplication | null>(null)
const selectedDictionaryIds = ref<string[]>([])
const repairCollectionErrors = ref(true)
const revealedPasswords = reactive<Record<string, string>>({})

const scope = reactive({
  host_ids: [] as string[],
  host_group_ids: [] as string[],
  application_types: [] as string[],
  online_agents_only: true,
})

const applicationTypeOptions = [
  { label: '数据库', value: 'database' },
  { label: 'Redis', value: 'redis' },
  { label: 'MySQL', value: 'mysql' },
  { label: 'PostgreSQL', value: 'postgresql' },
  { label: 'Web 服务', value: 'web_service' },
  { label: 'AI Agent', value: 'ai_agent' },
  { label: 'MCP 服务', value: 'mcp_server' },
  { label: 'LLM 网关', value: 'llm_service' },
]

const availableDictionaries = computed(() => {
  const seen = new Set<string>()
  const items: WeakPasswordDictionary[] = []
  if (store.defaultDictionary) {
    seen.add(store.defaultDictionary.id)
    items.push(store.defaultDictionary)
  }
  for (const dict of store.dictionaries) {
    if (!seen.has(dict.id)) {
      seen.add(dict.id)
      items.push(dict)
    }
  }
  return items
})

async function runAnalysis() {
  scope.online_agents_only = true
  const result = await store.analyze({ scope })
  ensureDefaultDictionarySelected()
  if (result.error_code === 'no_application_assets') {
    return
  }
  ElMessage.success(`发现 ${result.candidate_count} 个可检查应用`)
}

async function refreshAll() {
  await Promise.all([store.fetchCandidates(), store.fetchTasks(), store.fetchDictionaries()])
  ensureDefaultDictionarySelected()
}

async function fetchCandidatesPage() {
  await store.fetchCandidates()
}

async function fetchTasksPage() {
  await store.fetchTasks()
}

function openSingleCheck(row: WeakPasswordCandidateApplication) {
  selectedCandidate.value = row
  checkMode.value = 'single'
  ensureDefaultDictionarySelected()
  checkVisible.value = true
}

function openBatchCheck() {
  selectedCandidate.value = null
  checkMode.value = 'batch'
  ensureDefaultDictionarySelected()
  checkVisible.value = true
}

function openFindingDetail(row: WeakPasswordCandidateApplication) {
  if (row.scan_status !== 'alert') return
  selectedCandidate.value = row
  findingVisible.value = true
}

async function confirmCheck() {
  if (selectedDictionaryIds.value.length === 0) {
    ElMessage.warning('请至少勾选一个字典')
    return
  }
  const dictionary_policy = buildDictionaryPolicy()
  const ai_policy = {
    repair_collection_errors: repairCollectionErrors.value,
    max_agent_tool_calls_per_app: 10,
  }
  if (checkMode.value === 'single') {
    if (!selectedCandidate.value) return
    const result = await store.createTask({
      candidate_application_id: selectedCandidate.value.candidate_application_id,
      dictionary_policy,
      ai_policy,
    })
    checkVisible.value = false
    router.push(`/risk/weak-password/tasks/${result.task_id}`)
    return
  }

  const result = await store.createBatchTasks({
    candidate_application_ids: store.candidates.map(item => item.candidate_application_id),
    dictionary_policy,
    ai_policy,
  })
  checkVisible.value = false
  activeTab.value = 'tasks'
  ElMessage.success(`已创建 ${result.created.length} 个检测任务，跳过 ${result.skipped.length} 个离线或不可检测应用`)
}

function buildDictionaryPolicy() {
  const defaultID = store.defaultDictionary?.id || ''
  return {
    use_default_1000: defaultID ? selectedDictionaryIds.value.includes(defaultID) : false,
    dictionary_ids: selectedDictionaryIds.value.filter(id => id !== defaultID),
    use_ai_generated: false,
  }
}

function ensureDefaultDictionarySelected() {
  if (selectedDictionaryIds.value.length > 0) return
  if (store.defaultDictionary?.id) {
    selectedDictionaryIds.value = [store.defaultDictionary.id]
  }
}

async function revealFinding(findingId: string) {
  try {
    const password = await ElMessageBox.prompt('请输入当前系统密码', '查看命中密码', {
      confirmButtonText: '查看',
      cancelButtonText: '取消',
      inputType: 'password',
      inputPattern: /.+/,
      inputErrorMessage: '系统密码不能为空',
    })
    const revealed = await revealWeakPasswordFinding(findingId, password.value)
    revealedPasswords[findingId] = revealed.matched_password
  } catch {
    // user cancelled
  }
}

async function deleteTask(taskId: string) {
  try {
    await ElMessageBox.confirm('确定删除此弱密码检测任务吗？', '删除任务', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await store.deleteTask(taskId)
    ElMessage.success('已删除任务')
  } catch {
    // user cancelled
  }
}

function canDeleteTask(status: string) {
  return !['pending', 'analyzing_assets', 'collecting_credentials', 'repairing_collection', 'matching'].includes(status)
}

function confidenceLabel(value: number) {
  if (value >= 0.8) return '高'
  if (value >= 0.5) return '中'
  return '低'
}

function confidenceTag(value: number) {
  if (value >= 0.8) return 'success'
  if (value >= 0.5) return 'warning'
  return 'info'
}

function scanStatusLabel(status: string) {
  if (status === 'alert') return '告警'
  if (status === 'safe') return '安全'
  return '未扫描'
}

function scanStatusType(status: string) {
  if (status === 'alert') return 'danger'
  if (status === 'safe') return 'success'
  return 'info'
}

function taskStatusType(status: string) {
  if (status === 'completed') return 'success'
  if (status === 'failed' || status === 'partial_failed') return 'danger'
  if (status === 'matching' || status === 'collecting_credentials') return 'warning'
  return 'info'
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped>
.weak-password-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.page-toolbar,
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.page-toolbar h1,
.panel-head h2 {
  margin: 0;
  color: #0f172a;
}

.page-toolbar p {
  margin: 6px 0 0;
  color: #64748b;
}

.toolbar-actions,
.filter-row,
.drawer-actions,
.alert-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.workspace-tabs {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 10px 14px 16px;
}

.panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.filter-row :deep(.el-select),
.filter-row :deep(.el-input) {
  width: 220px;
}

.dense-table {
  width: 100%;
}

.pagination-bar {
  display: flex;
  justify-content: flex-end;
}

.primary-cell {
  font-weight: 700;
  color: #0f172a;
}

.secondary-cell {
  color: #64748b;
  font-size: 12px;
}

.path-list,
.dictionary-list {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.dictionary-list {
  flex-direction: column;
  align-items: flex-start;
  max-height: 280px;
  overflow: auto;
}

.drawer-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.drawer-actions {
  justify-content: flex-end;
}

.fact-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid #e2e8f0;
}

.fact-row span {
  color: #64748b;
}

.fact-row strong {
  color: #0f172a;
  text-align: right;
  word-break: break-word;
}

.clickable {
  cursor: pointer;
}

.password-mask {
  color: #0f172a;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  letter-spacing: 0;
}
</style>

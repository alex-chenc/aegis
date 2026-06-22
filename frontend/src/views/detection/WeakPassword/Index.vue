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
            <el-table-column label="可能密码位置" min-width="240">
              <template #default="{ row }">
                <div class="path-list">
                  <el-tag v-for="path in row.candidate_paths.slice(0, 2)" :key="path" effect="plain">{{ path }}</el-tag>
                  <span v-if="row.candidate_paths.length === 0" class="secondary-cell">待受控辅助定位</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="凭据类型" width="150">
              <template #default="{ row }">
                <el-tag v-for="type in row.credential_types" :key="type" size="small" type="info">{{ type }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="AI 置信度" width="120">
              <template #default="{ row }">
                <el-tag :type="confidenceTag(row.confidence)">{{ confidenceLabel(row.confidence) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="210" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="openCheck(row)">检查弱密码</el-button>
                <el-button link @click="openEvidence(row)">查看分析依据</el-button>
              </template>
            </el-table-column>
          </el-table>
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
        </section>
      </el-tab-pane>

      <el-tab-pane label="检测结果" name="results">
        <section class="panel">
          <el-table :data="store.findings" class="dense-table">
            <el-table-column label="应用" prop="application_name" />
            <el-table-column label="账号" prop="account" />
            <el-table-column label="凭据类型" prop="credential_type" />
            <el-table-column label="命中密码" prop="matched_password_mask" />
            <el-table-column label="状态" prop="match_status" />
            <el-table-column label="匹配方式" prop="match_rule" />
          </el-table>
        </section>
      </el-tab-pane>
    </el-tabs>

    <el-drawer v-model="evidenceVisible" title="分析依据" size="520px">
      <div v-if="selectedCandidate" class="drawer-stack">
        <div class="fact-row"><span>目标应用</span><strong>{{ selectedCandidate.application_name }}</strong></div>
        <div class="fact-row"><span>推荐 Profile</span><strong>{{ selectedCandidate.profile_id }}</strong></div>
        <div class="fact-row"><span>凭据类型</span><strong>{{ selectedCandidate.credential_types.join(', ') }}</strong></div>
        <div>
          <h3>可能密码位置</h3>
          <el-tag v-for="path in selectedCandidate.candidate_paths" :key="path" effect="plain">{{ path }}</el-tag>
        </div>
      </div>
    </el-drawer>

    <el-drawer v-model="checkVisible" title="检查弱密码" size="560px">
      <div v-if="selectedCandidate" class="drawer-stack">
        <div class="fact-row"><span>目标主机</span><strong>{{ selectedCandidate.hostname || selectedCandidate.host_id }}</strong></div>
        <div class="fact-row"><span>目标应用</span><strong>{{ selectedCandidate.application_name }}</strong></div>
        <el-form label-position="top">
          <el-form-item label="字典策略">
            <el-checkbox v-model="taskPolicy.dictionary_policy.use_default_1000">默认 1000 字典</el-checkbox>
            <el-checkbox v-model="taskPolicy.dictionary_policy.hybrid">混合规则</el-checkbox>
            <el-checkbox v-model="taskPolicy.dictionary_policy.fuzzy">模糊规则</el-checkbox>
          </el-form-item>
          <el-form-item label="AI 策略">
            <el-checkbox v-model="taskPolicy.ai_policy.repair_collection_errors">读取失败时 AI 修复定位</el-checkbox>
            <el-checkbox v-model="taskPolicy.ai_policy.encrypted_password_llm_match">加密/hash LLM 匹配</el-checkbox>
          </el-form-item>
        </el-form>
        <div class="drawer-actions">
          <el-button @click="checkVisible = false">取消</el-button>
          <el-button type="primary" :loading="store.creatingTask" @click="confirmCheck">确认检查</el-button>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Collection, Cpu, Refresh, Search } from '@element-plus/icons-vue'
import { useWeakPasswordStore } from '@/store/weakPassword'
import type { WeakPasswordCandidateApplication } from '@/types/weakPassword'

const router = useRouter()
const store = useWeakPasswordStore()
const activeTab = ref('analysis')
const keyword = ref('')
const evidenceVisible = ref(false)
const checkVisible = ref(false)
const selectedCandidate = ref<WeakPasswordCandidateApplication | null>(null)

const scope = reactive({
  host_ids: [] as string[],
  host_group_ids: [] as string[],
  application_types: [] as string[],
  online_agents_only: true,
})

const taskPolicy = reactive({
  dictionary_policy: {
    use_default_1000: true,
    dictionary_ids: [] as string[],
    use_ai_generated: false,
    hybrid: true,
    fuzzy: true,
  },
  ai_policy: {
    repair_collection_errors: true,
    encrypted_password_llm_match: true,
    max_agent_tool_calls_per_app: 10,
  },
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

async function runAnalysis() {
  scope.online_agents_only = true
  const result = await store.analyze({ scope })
  if (result.error_code === 'no_application_assets') {
    return
  }
  ElMessage.success(`发现 ${result.candidate_count} 个可检查应用`)
}

async function refreshAll() {
  await Promise.all([store.fetchCandidates(), store.fetchTasks()])
}

function openEvidence(row: WeakPasswordCandidateApplication) {
  selectedCandidate.value = row
  evidenceVisible.value = true
}

function openCheck(row: WeakPasswordCandidateApplication) {
  selectedCandidate.value = row
  checkVisible.value = true
}

async function confirmCheck() {
  if (!selectedCandidate.value) return
  const result = await store.createTask({
    candidate_application_id: selectedCandidate.value.candidate_application_id,
    dictionary_policy: taskPolicy.dictionary_policy,
    ai_policy: taskPolicy.ai_policy,
  })
  checkVisible.value = false
  router.push(`/risk/weak-password/tasks/${result.task_id}`)
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

.primary-cell {
  font-weight: 700;
  color: #0f172a;
}

.secondary-cell {
  color: #64748b;
  font-size: 12px;
}

.path-list {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.drawer-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
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

</style>

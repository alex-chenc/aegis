<template>
  <div class="weak-password-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ $t('generated.detectionWeakPasswordIndex_intelligent_weak_password_detection_032e9e') }}</h1>
        <p>{{ $t('generated.detectionWeakPasswordIndex_credential_checking_based_on_online_host_f891ac') }}</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Refresh" @click="refreshAll">{{ $t('generated.common_refresh_38108e') }}</el-button>
        <el-button :icon="Collection" @click="router.push('/risk/weak-password/dictionaries')">{{ $t('generated.detectionWeakPasswordIndex_dictionary_management_3bdfb2') }}</el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="workspace-tabs">
      <el-tab-pane :label="$t('generated.detectionWeakPasswordIndex_apply_asset_analysis_fb81db')" name="analysis">
        <section class="panel">
          <div class="filter-row">
            <el-select v-model="scope.application_types" multiple collapse-tags :placeholder="$t('generated.detectionWeakPasswordIndex_application_type_5865b7')">
              <el-option v-for="item in applicationTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
            <el-select v-model="store.candidateFilters.confidence" :placeholder="$t('generated.common_confidence_b78c2d')" clearable>
              <el-option :label="$t('generated.common_high_b096b3')" value="high" />
              <el-option :label="$t('generated.common_middle_086907')" value="medium" />
              <el-option :label="$t('generated.common_low_b9ee25')" value="low" />
            </el-select>
            <el-input v-model="keyword" :prefix-icon="Search" :placeholder="$t('generated.detectionWeakPasswordIndex_search_for_an_application_host_or_8e42a4')" clearable @keyup.enter="runAnalysis" />
            <el-button type="primary" :icon="Cpu" :loading="store.analyzing" @click="runAnalysis">{{ $t('generated.detectionWeakPasswordIndex_one_click_asset_analysis_application_e8f98d') }}</el-button>
            <el-button type="success" :icon="Key" :loading="store.creatingTask" :disabled="store.candidates.length === 0" @click="openBatchCheck">{{ $t('generated.detectionWeakPasswordIndex_one_click_detection_385bad') }}</el-button>
          </div>

          <el-alert
            v-if="store.analysisResult?.error_code === 'no_application_assets'"
            type="warning"
            show-icon
            :closable="false"
            :title="$t('generated.detectionWeakPasswordIndex_there_are_currently_no_application_assets_a988ff')"
            :description="store.analysisResult?.message || $t('dynamic.collectAssetsFirst')"
          >
            <template #default>
              <div class="alert-actions">
                <el-button type="primary" size="small" @click="router.push('/hosts/assets')">{{ $t('generated.detectionWeakPasswordIndex_to_collect_assets_c8bc9c') }}</el-button>
                <el-button size="small" @click="refreshAll">{{ $t('generated.detectionWeakPasswordIndex_refresh_asset_status_0a5002') }}</el-button>
              </div>
            </template>
          </el-alert>

          <el-empty
            v-else-if="!store.loading && store.candidates.length === 0"
            :description="$t('generated.detectionWeakPasswordIndex_there_are_currently_no_application_assets_a988ff')"
          >
            <el-button type="primary" @click="router.push('/hosts/assets')">{{ $t('generated.detectionWeakPasswordIndex_to_collect_assets_c8bc9c') }}</el-button>
            <el-button @click="refreshAll">{{ $t('generated.detectionWeakPasswordIndex_refresh_asset_status_0a5002') }}</el-button>
          </el-empty>

          <el-table v-else v-loading="store.loading" :data="store.candidates" class="dense-table">
            <el-table-column :label="$t('generated.common_host_2e8a0c')" min-width="170">
              <template #default="{ row }">
                <div class="primary-cell">{{ row.hostname || row.host_id }}</div>
                <div class="secondary-cell">{{ row.ip_address || row.host_id }}</div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_application_456202')" min-width="180">
              <template #default="{ row }">
                <div class="primary-cell">{{ row.application_name }}</div>
                <div class="secondary-cell">{{ row.application_version || row.application_type }}</div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_label_ae0a7a')" width="120">
              <template #default="{ row }">
                <el-tag :type="row.is_container ? 'success' : 'info'" size="small" effect="plain">
                  {{ row.is_container ? $t('dynamic.containerApplication') : $t('dynamic.hostApplication') }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.detectionWeakPasswordIndex_possible_password_location_4d7766')" min-width="260">
              <template #default="{ row }">
                <div class="path-list">
                  <el-tag v-for="path in row.candidate_paths.slice(0, 2)" :key="path" effect="plain">{{ path }}</el-tag>
                  <span v-if="row.candidate_paths.length === 0" class="secondary-cell">{{ $t('generated.detectionWeakPasswordIndex_to_be_controlled_assisted_positioning_6867b7') }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_state_62e951')" width="130">
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
            <el-table-column :label="$t('generated.detectionWeakPasswordIndex_ai_confidence_4815bf')" width="120">
              <template #default="{ row }">
                <el-tag :type="confidenceTag(row.confidence)">{{ confidenceLabel(row.confidence) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="140" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="openSingleCheck(row)">{{ $t('generated.detectionWeakPasswordIndex_check_for_weak_passwords_a88bb2') }}</el-button>
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

      <el-tab-pane :label="$t('generated.detectionWeakPasswordIndex_weak_password_check_ebc6ef')" name="tasks">
        <section class="panel">
          <div class="panel-head">
            <h2>{{ $t('generated.detectionWeakPasswordIndex_check_tasks_95b778') }}</h2>
            <div class="toolbar-actions">
              <el-button
                type="danger"
                plain
                :disabled="selectedTaskRows.length === 0"
                @click="deleteSelectedTasks"
              >
                {{ $t('generated.common_batch_delete_362aed') }}
              </el-button>
              <el-button :icon="Refresh" @click="store.fetchTasks">{{ $t('generated.common_refresh_38108e') }}</el-button>
            </div>
          </div>
          <el-table
            v-loading="store.loading"
            :data="store.tasks"
            class="dense-table"
            row-key="id"
            @selection-change="handleTaskSelectionChange"
          >
            <el-table-column type="selection" width="48" :selectable="isTaskSelectable" />
            <el-table-column :label="$t('generated.detectionWeakPasswordIndex_task_3172b3')" min-width="220" prop="name" />
            <el-table-column :label="$t('generated.common_state_62e951')" width="150">
              <template #default="{ row }">
                <el-tag :type="taskStatusType(row.status)">{{ weakPasswordStatusLabel(row.status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_schedule_acf014')" min-width="180">
              <template #default="{ row }">
                <div class="progress-cell">
                  <el-progress :percentage="row.progress || 0" :stroke-width="10" />
                  <span v-if="row.current_stage" class="secondary-cell">{{ weakPasswordStatusLabel(row.current_stage) }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_hit_7a130d')" width="90" prop="matched_findings" />
            <el-table-column :label="$t('generated.detectionWeakPasswordIndex_failed_application_4385c2')" width="100" prop="failed_applications" />
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="210" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="router.push(`/risk/weak-password/tasks/${row.id}`)">{{ $t('generated.detectionWeakPasswordIndex_check_the_details_faea8c') }}</el-button>
                <el-button link type="danger" :disabled="!canDeleteTask(row.status)" @click="deleteTask(row.id)">{{ $t('generated.common_delete_3755f5') }}</el-button>
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

    <el-drawer v-model="checkVisible" :title="checkMode === 'batch' ? $t('dynamic.oneClickWeakPasswordCheck') : $t('dynamic.weakPasswordCheck')" size="560px">
      <div class="drawer-stack">
        <template v-if="checkMode === 'single' && selectedCandidate">
          <div class="fact-row"><span>{{ $t('generated.detectionWeakPasswordIndex_target_host_d24ba7') }}</span><strong>{{ selectedCandidate.hostname || selectedCandidate.host_id }}</strong></div>
          <div class="fact-row"><span>{{ $t('generated.detectionWeakPasswordIndex_target_application_832078') }}</span><strong>{{ selectedCandidate.application_name }}</strong></div>
        </template>
        <template v-else>
          <div class="fact-row"><span>{{ $t('generated.detectionWeakPasswordIndex_detection_range_05ea8f') }}</span><strong>{{ $t('generated.detectionWeakPasswordIndex_current_25e74d') }} {{ store.candidates.length }} {{ $t('generated.detectionWeakPasswordIndex_apps_d77eb8') }}</strong></div>
        </template>

        <el-form label-position="top">
          <el-form-item :label="$t('generated.detectionWeakPasswordIndex_dictionary_strategy_28c4d1')">
            <el-checkbox-group v-model="selectedDictionaryIds" class="dictionary-list">
              <el-checkbox v-for="dict in availableDictionaries" :key="dict.id" :label="dict.id">
                {{ dict.name }}（{{ dict.entry_count }} {{ $t('generated.detectionWeakPasswordIndex_strip_372545') }}
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>
          <el-form-item :label="$t('generated.detectionWeakPasswordIndex_ai_strategy_26e7b9')">
            <el-checkbox v-model="repairCollectionErrors">{{ $t('generated.detectionWeakPasswordIndex_ai_repair_positioning_when_reading_fails_702e33') }}</el-checkbox>
          </el-form-item>
          <el-form-item :label="$t('generated.detectionWeakPasswordIndex_number_of_detection_rounds_da1eca')">
            <el-input-number v-model="detectionRounds" :min="10" :max="50" :step="1" controls-position="right" />
          </el-form-item>
        </el-form>
        <div class="drawer-actions">
          <el-button @click="checkVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
          <el-button type="primary" :loading="store.creatingTask" @click="confirmCheck">{{ $t('generated.detectionWeakPasswordIndex_confirmation_check_641d53') }}</el-button>
        </div>
      </div>
    </el-drawer>

    <el-drawer v-model="findingVisible" :title="$t('generated.detectionWeakPasswordIndex_weak_password_details_95b795')" size="640px">
      <div v-if="selectedCandidate" class="drawer-stack">
        <div class="fact-row"><span>{{ $t('generated.common_application_456202') }}</span><strong>{{ selectedCandidate.application_name }}</strong></div>
        <div class="fact-row"><span>{{ $t('generated.common_host_2e8a0c') }}</span><strong>{{ selectedCandidate.hostname || selectedCandidate.host_id }}</strong></div>
        <el-table :data="selectedCandidate.findings || []" class="dense-table">
          <el-table-column :label="$t('generated.common_account_901384')" prop="account" min-width="120" />
          <el-table-column :label="$t('generated.detectionWeakPasswordIndex_password_c839a8')" min-width="160">
            <template #default="{ row }">
              <code class="password-mask">{{ revealedPasswords[row.id] || row.matched_password_mask }}</code>
            </template>
          </el-table-column>
          <el-table-column :label="$t('generated.detectionWeakPasswordIndex_process_pid_b31aa4')" width="120">
            <template #default="{ row }">{{ row.process_pid || '-' }}</template>
          </el-table-column>
          <el-table-column :label="$t('generated.common_source_c63f79')" min-width="220">
            <template #default="{ row }">
              <div class="secondary-cell">{{ row.source_path }}</div>
              <div class="secondary-cell">{{ row.field_path }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="120" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="revealFinding(row.id)">{{ $t('generated.detectionWeakPasswordIndex_view_clear_text_cf0ecc') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Collection, Cpu, Key, Refresh, Search } from '@element-plus/icons-vue'
import { revealWeakPasswordFinding } from '@/api/weakPassword'
import { useWeakPasswordStore } from '@/store/weakPassword'
import type { WeakPasswordCandidateApplication, WeakPasswordDictionary, WeakPasswordTask } from '@/types/weakPassword'
import { weakPasswordStatusLabel } from '@/utils/weakPasswordLabels'

const router = useRouter()
const store = useWeakPasswordStore()
const activeTab = ref('analysis')
const keyword = ref('')
const checkVisible = ref(false)
const findingVisible = ref(false)
const checkMode = ref<'single' | 'batch'>('single')
const selectedCandidate = ref<WeakPasswordCandidateApplication | null>(null)
const selectedDictionaryIds = ref<string[]>([])
const selectedTaskRows = ref<WeakPasswordTask[]>([])
const repairCollectionErrors = ref(true)
const detectionRounds = ref(10)
const revealedPasswords = reactive<Record<string, string>>({})
let taskRefreshTimer: number | undefined

const scope = reactive({
  host_ids: [] as string[],
  host_group_ids: [] as string[],
  application_types: [] as string[],
  online_agents_only: true,
})

const applicationTypeOptions = computed(() => [
  { label: translate('generatedScript.common_database_f4dbbc'), value: 'database' },
  { label: 'Redis', value: 'redis' },
  { label: 'MySQL', value: 'mysql' },
  { label: 'PostgreSQL', value: 'postgresql' },
  { label: 'OpenSSH', value: 'openssh' },
  { label: 'Tomcat', value: 'tomcat' },
  { label: 'FTP', value: 'ftp' },
  { label: translate('generatedScript.common_web_services_e3d112'), value: 'web_service' },
  { label: 'AI Agent', value: 'ai_agent' },
  { label: translate('generatedScript.detectionWeakPasswordIndex_mcp_service_463f3b'), value: 'mcp_server' },
  { label: translate('generatedScript.detectionWeakPasswordIndex_llm_gateway_0d3bd6'), value: 'llm_service' },
])

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

const hasRunningTasks = computed(() => store.tasks.some(task => isRunningTaskStatus(task.status)))

async function runAnalysis() {
  scope.online_agents_only = true
  const result = await store.analyze({ scope })
  ensureDefaultDictionarySelected()
  if (result.error_code === 'no_application_assets') {
    return
  }
  ElMessage.success(translate('generatedScript.detectionWeakPasswordIndex_checkable_apps_found_23fe91', { p0: result.candidate_count }))
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
    ElMessage.warning(translate('generatedScript.detectionWeakPasswordIndex_please_check_at_least_one_dictionary_a9786c'))
    return
  }
  const dictionary_policy = buildDictionaryPolicy()
  const ai_policy = {
    repair_collection_errors: repairCollectionErrors.value,
    detection_rounds: detectionRounds.value,
    max_agent_tool_calls_per_app: detectionRounds.value,
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
  await store.fetchTasks()
  checkVisible.value = false
  activeTab.value = 'tasks'
  ElMessage.success(translate('generatedScript.detectionWeakPasswordIndex_detection_tasks_created_offline_or_undetectable_af617c', { p0: result.created.length, p1: result.skipped.length }))
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
    const password = await ElMessageBox.prompt(translate('generatedScript.common_please_enter_the_current_system_password_bb50b2'), translate('generatedScript.common_view_hit_password_af06b5'), {
      confirmButtonText: translate('generatedScript.common_check_f7acef'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      inputType: 'password',
      inputPattern: /.+/,
      inputErrorMessage: translate('generatedScript.common_system_password_cannot_be_empty_2be9ab'),
    })
    const revealed = await revealWeakPasswordFinding(findingId, password.value)
    revealedPasswords[findingId] = revealed.matched_password
  } catch {
    // user cancelled
  }
}

async function deleteTask(taskId: string) {
  try {
    await ElMessageBox.confirm(translate('generatedScript.common_are_you_sure_you_want_to_be98d5'), translate('generatedScript.common_delete_task_070581'), {
      confirmButtonText: translate('generatedScript.common_delete_3755f5'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning',
    })
    await store.deleteTask(taskId)
    ElMessage.success(translate('generatedScript.common_task_deleted_b3b2ec'))
  } catch {
    // user cancelled
  }
}

function handleTaskSelectionChange(rows: WeakPasswordTask[]) {
  selectedTaskRows.value = rows
}

function isTaskSelectable(row: WeakPasswordTask) {
  return canDeleteTask(row.status)
}

async function deleteSelectedTasks() {
  const taskIds = selectedTaskRows.value.filter(row => canDeleteTask(row.status)).map(row => row.id)
  if (taskIds.length === 0) {
    ElMessage.warning(translate('generatedScript.detectionWeakPasswordIndex_please_select_a_task_that_can_b42ac8'))
    return
  }
  try {
    await ElMessageBox.confirm(translate('generatedScript.detectionWeakPasswordIndex_are_you_sure_you_want_to_077086', { p0: taskIds.length }), translate('generatedScript.detectionWeakPasswordIndex_delete_tasks_in_batches_e8f5fa'), {
      confirmButtonText: translate('generatedScript.common_delete_3755f5'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning',
    })
    const result = await store.deleteTasks(taskIds)
    selectedTaskRows.value = []
    const skipped = result.skipped?.length || 0
    if (skipped > 0) {
      ElMessage.warning(translate('generatedScript.detectionWeakPasswordIndex_tasks_deleted_tasks_skipped_75a23e', { p0: result.count, p1: skipped }))
      return
    }
    ElMessage.success(translate('generatedScript.detectionWeakPasswordIndex_tasks_deleted_b14d37', { p0: result.count }))
  } catch {
    // user cancelled
  }
}

function canDeleteTask(status: string) {
  return !isRunningTaskStatus(status)
}

function isRunningTaskStatus(status: string) {
  return ['pending', 'analyzing_assets', 'collecting_credentials', 'repairing_collection', 'matching'].includes(status)
}

function startTaskAutoRefresh() {
  if (taskRefreshTimer !== undefined) return
  taskRefreshTimer = window.setInterval(() => {
    store.fetchTasks()
  }, 5000)
}

function stopTaskAutoRefresh() {
  if (taskRefreshTimer === undefined) return
  window.clearInterval(taskRefreshTimer)
  taskRefreshTimer = undefined
}

function syncTaskAutoRefresh() {
  if (activeTab.value === 'tasks' && hasRunningTasks.value) {
    startTaskAutoRefresh()
    return
  }
  stopTaskAutoRefresh()
}

function confidenceLabel(value: number) {
  if (value >= 0.8) return translate('generatedScript.common_high_b096b3')
  if (value >= 0.5) return translate('generatedScript.common_middle_086907')
  return translate('generatedScript.common_low_b9ee25')
}

function confidenceTag(value: number) {
  if (value >= 0.8) return 'success'
  if (value >= 0.5) return 'warning'
  return 'info'
}

function scanStatusLabel(status: string) {
  if (status === 'alert') return translate('generatedScript.common_alarm_507842')
  if (status === 'safe') return translate('generatedScript.detectionWeakPasswordIndex_safety_8e662a')
  return translate('generatedScript.detectionWeakPasswordIndex_not_scanned_81d39e')
}

function scanStatusType(status: string) {
  if (status === 'alert') return 'danger'
  if (status === 'safe') return 'success'
  return 'info'
}

function taskStatusType(status: string) {
  if (status === 'completed') return 'success'
  if (status === 'failed' || status === 'partial_failed') return 'danger'
  if (['matching', 'collecting_credentials', 'repairing_collection', 'analyzing_assets', 'pending'].includes(status)) return 'warning'
  return 'info'
}

watch([activeTab, hasRunningTasks], syncTaskAutoRefresh)

onMounted(async () => {
  await refreshAll()
  syncTaskAutoRefresh()
})

onUnmounted(() => {
  stopTaskAutoRefresh()
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
.dictionary-list,
.progress-cell {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.progress-cell {
  flex-direction: column;
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

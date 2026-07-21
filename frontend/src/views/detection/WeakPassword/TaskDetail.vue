<template>
  <div class="weak-task-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ store.currentTask?.name || $t('dynamic.weakPasswordTaskDetail') }}</h1>
        <p>{{ store.currentTask ? weakPasswordStatusLabel(store.currentTask.status) : $t('dynamic.loading') }}</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Back" @click="router.push('/risk/weak-password')">{{ $t('generated.common_return_11d024') }}</el-button>
        <el-button :icon="Refresh" @click="loadDetail">{{ $t('generated.common_refresh_38108e') }}</el-button>
        <el-button v-if="store.currentTask" type="danger" plain :disabled="!canDeleteTask(store.currentTask.status)" @click="deleteTask">{{ $t('generated.common_delete_3755f5') }}</el-button>
        <el-button v-if="store.currentTask?.status === 'failed'" type="primary" @click="retryFailed">{{ $t('generated.detectionWeakPasswordTaskDetail_retry_failed_items_5fefad') }}</el-button>
      </div>
    </div>

    <section class="panel">
      <div class="progress-grid">
        <div class="progress-main">
          <el-progress :percentage="store.progress?.progress || 0" :stroke-width="14" />
          <div class="stage-list">
            <span v-for="stage in stages" :key="stage">{{ stage }}</span>
          </div>
        </div>
        <div class="metric">
          <span>{{ $t('generated.detectionWeakPasswordTaskDetail_current_application_f5837b') }}</span>
          <strong>{{ store.progress?.current_application || '-' }}</strong>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>{{ $t('generated.detectionWeakPasswordTaskDetail_host_execution_e9b200') }}</h2>
      </div>
      <el-table v-loading="store.loading" :data="store.hosts" class="dense-table">
        <el-table-column label="IP" min-width="150">
          <template #default="{ row }">
            <div class="primary-cell">{{ row.ip_address || '-' }}</div>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_host_name_823990')" min-width="180">
          <template #default="{ row }">
            <div class="primary-cell">{{ row.hostname || row.host_id }}</div>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_agent_status_70d10b')" width="120">
          <template #default="{ row }">{{ weakPasswordStatusLabel(row.agent_status) }}</template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_state_62e951')" width="150">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">{{ weakPasswordStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_collection_records_c56530')" prop="collected_records" width="120" />
        <el-table-column :label="$t('generated.common_hit_7a130d')" prop="matched_findings" width="100" />
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_reason_for_failure_918468')" min-width="240">
          <template #default="{ row }">
            <div>{{ weakPasswordErrorCodeLabel(row.error_code) }}</div>
            <div v-if="row.error_message" class="secondary-cell">{{ row.error_message }}</div>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="store.hostTotal > 0" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.hostFilters.page"
          v-model:page-size="store.hostFilters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          :total="store.hostTotal"
          @size-change="loadDetail"
          @current-change="loadDetail"
        />
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>{{ $t('generated.detectionWeakPasswordTaskDetail_hit_result_7b052d') }}</h2>
      </div>
      <el-table :data="store.findings" class="dense-table">
        <el-table-column :label="$t('generated.common_application_456202')" prop="application_name" min-width="140" />
        <el-table-column :label="$t('generated.common_account_901384')" prop="account" min-width="120" />
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_credential_type_afeeac')" width="150">
          <template #default="{ row }">{{ weakPasswordCredentialTypeLabel(row.credential_type) }}</template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_hit_code_9035b8')" width="150">
          <template #default="{ row }">
            <span class="password-mask">{{ row.matched_password_mask }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_state_62e951')" width="180">
          <template #default="{ row }">
            <el-tag :type="row.match_status === 'confirmed' ? 'success' : 'warning'">{{ weakPasswordMatchStatusLabel(row.match_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_configuration_source_6055a9')" min-width="220">
          <template #default="{ row }">
            <div class="secondary-cell">{{ row.source_path }}</div>
            <div class="secondary-cell">{{ row.field_path }}</div>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showFindingDetail(row.id)">{{ $t('generated.common_details_4f55ee') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="store.findingTotal > 0" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.findingFilters.page"
          v-model:page-size="store.findingFilters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          :total="store.findingTotal"
          @size-change="loadDetail"
          @current-change="loadDetail"
        />
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>{{ $t('generated.detectionWeakPasswordTaskDetail_collection_progress_ff7bf1') }}</h2>
      </div>
      <el-table :data="store.errors" class="dense-table collection-progress-table" height="420">
        <el-table-column :label="$t('generated.common_application_456202')" prop="application_name" min-width="140" />
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_rounds_3d6450')" prop="round" width="90" />
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_tool_a72ef1')" min-width="230">
          <template #default="{ row }">{{ weakPasswordToolNameLabel(row.tool_name) }}</template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_state_62e951')" width="130">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">{{ weakPasswordStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_error_code_e08c1d')" min-width="170">
          <template #default="{ row }">{{ weakPasswordErrorCodeLabel(row.error_code) }}</template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_time_consuming_a9704e')" width="110">
          <template #default="{ row }">{{ row.execution_time_ms || 0 }}ms</template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_collection_path_afa1b2')" min-width="260">
          <template #default="{ row }">
            <el-tooltip :content="collectionCellValue(row.source_path, $t('dynamic.pathNotRecorded'))" placement="top" :show-after="250">
              <div class="secondary-cell multiline-cell clipped-multiline-cell">{{ collectionCellValue(row.source_path, $t('dynamic.pathNotRecorded')) }}</div>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordTaskDetail_collection_field_2e584e')" min-width="220">
          <template #default="{ row }">
            <el-tooltip :content="collectionCellValue(row.field_name, $t('dynamic.fieldNotRecorded'))" placement="top" :show-after="250">
              <div class="secondary-cell multiline-cell clipped-multiline-cell">{{ collectionCellValue(row.field_name, $t('dynamic.fieldNotRecorded')) }}</div>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="store.errorTotal > 10" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.errorFilters.page"
          v-model:page-size="store.errorFilters.page_size"
          :page-sizes="[10]"
          :pager-count="10"
          layout="total, prev, pager, next"
          :total="store.errorTotal"
          @size-change="loadDetail"
          @current-change="loadDetail"
        />
      </div>
    </section>

    <el-dialog v-model="passwordDialogVisible" :title="$t('generated.detectionWeakPasswordTaskDetail_hit_password_details_16fb7c')" width="460px" destroy-on-close>
      <div v-if="revealedFinding" class="password-detail">
        <div class="fact-row"><span>{{ $t('generated.common_application_456202') }}</span><strong>{{ revealedFinding.application_name }}</strong></div>
        <div class="fact-row"><span>{{ $t('generated.common_account_901384') }}</span><strong>{{ revealedFinding.account || '-' }}</strong></div>
        <div class="fact-row"><span>{{ $t('generated.detectionWeakPasswordTaskDetail_credential_type_afeeac') }}</span><strong>{{ weakPasswordCredentialTypeLabel(revealedFinding.credential_type) }}</strong></div>
        <div class="fact-row password-row"><span>{{ $t('generated.detectionWeakPasswordTaskDetail_full_password_30fbbe') }}</span><code>{{ revealedFinding.matched_password }}</code></div>
        <div class="fact-row"><span>{{ $t('generated.detectionWeakPasswordTaskDetail_configuration_source_6055a9') }}</span><strong>{{ revealedFinding.source_path || '-' }}</strong></div>
      </div>
      <template #footer>
        <el-button type="primary" @click="passwordDialogVisible = false">{{ $t('generated.common_closure_6c14bd') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Back, Refresh } from '@element-plus/icons-vue'
import { revealWeakPasswordFinding } from '@/api/weakPassword'
import { useWeakPasswordStore } from '@/store/weakPassword'
import type { RevealedWeakPasswordFinding } from '@/types/weakPassword'
import {
  weakPasswordCredentialTypeLabel,
  weakPasswordErrorCodeLabel,
  weakPasswordMatchStatusLabel,
  weakPasswordStatusLabel,
  weakPasswordToolNameLabel,
} from '@/utils/weakPasswordLabels'

const route = useRoute()
const router = useRouter()
const store = useWeakPasswordStore()
let timer: number | undefined
const passwordDialogVisible = ref(false)
const revealedFinding = ref<RevealedWeakPasswordFinding | null>(null)

const stages = [translate('generatedScript.detectionWeakPasswordTaskDetail_asset_analysis_a5ad9a'), translate('generatedScript.detectionWeakPasswordTaskDetail_connect_to_host_a77571'), translate('generatedScript.detectionWeakPasswordTaskDetail_read_configuration_911d44'), translate('generatedScript.detectionWeakPasswordTaskDetail_password_matching_02ba78'), translate('generatedScript.detectionWeakPasswordTaskDetail_results_stored_in_database_88c130')]
const taskId = computed(() => String(route.params.id || ''))

async function loadDetail() {
  if (taskId.value) {
    await store.fetchTaskDetail(taskId.value)
  }
}

async function retryFailed() {
  await store.retryFailed(taskId.value)
  ElMessage.success(translate('generatedScript.detectionWeakPasswordTaskDetail_submitted_to_try_again_88b212'))
}

async function deleteTask() {
  if (!store.currentTask || !canDeleteTask(store.currentTask.status)) return
  try {
    await ElMessageBox.confirm(translate('generatedScript.common_are_you_sure_you_want_to_be98d5'), translate('generatedScript.common_delete_task_070581'), {
      confirmButtonText: translate('generatedScript.common_delete_3755f5'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'warning',
    })
    await store.deleteTask(store.currentTask.id)
    ElMessage.success(translate('generatedScript.common_task_deleted_b3b2ec'))
    router.push('/risk/weak-password')
  } catch {
    // user cancelled
  }
}

async function showFindingDetail(findingId: string) {
  try {
    const password = await ElMessageBox.prompt(translate('generatedScript.common_please_enter_the_current_system_password_bb50b2'), translate('generatedScript.common_view_hit_password_af06b5'), {
      confirmButtonText: translate('generatedScript.common_check_f7acef'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      inputType: 'password',
      inputPattern: /.+/,
      inputErrorMessage: translate('generatedScript.common_system_password_cannot_be_empty_2be9ab'),
    })
    revealedFinding.value = await revealWeakPasswordFinding(findingId, password.value)
    passwordDialogVisible.value = true
  } catch {
    // user cancelled
  }
}

function canDeleteTask(status: string) {
  return !['pending', 'analyzing_assets', 'collecting_credentials', 'repairing_collection', 'matching'].includes(status)
}

function statusType(status: string) {
  if (status === 'completed') return 'success'
  if (['failed', 'partial_failed'].includes(status)) return 'danger'
  if (['matching', 'collecting', 'collecting_credentials', 'repairing', 'repairing_collection', 'analyzing_assets', 'pending', 'executing'].includes(status)) return 'warning'
  return 'info'
}

function collectionCellValue(value: string | undefined, fallback: string) {
  return value?.trim() || fallback
}

onMounted(() => {
  loadDetail()
  timer = window.setInterval(loadDetail, 5000)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<style scoped>
.weak-task-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.page-toolbar,
.panel-head,
.toolbar-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.toolbar-actions {
  justify-content: flex-end;
}

.page-toolbar h1,
.panel-head h2 {
  margin: 0;
  color: #0f172a;
}

.page-toolbar p,
.secondary-cell {
  color: #64748b;
}

.panel {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 14px;
}

.progress-grid {
  display: grid;
  grid-template-columns: minmax(360px, 1fr) 220px;
  gap: 14px;
  align-items: stretch;
}

.progress-main,
.metric {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 12px;
}

.stage-list {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
  margin-top: 10px;
  color: #64748b;
  font-size: 12px;
}

.metric {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
}

.metric span {
  color: #64748b;
  font-size: 12px;
}

.metric strong {
  color: #0f172a;
  font-size: 18px;
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

.multiline-cell {
  white-space: pre-line;
  word-break: break-word;
}

.clipped-multiline-cell {
  display: -webkit-box;
  max-height: 96px;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 5;
}

.collection-progress-table {
  min-height: 420px;
}

.password-mask {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  letter-spacing: 0;
}

.password-detail {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.fact-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 10px 0;
  border-bottom: 1px solid #e2e8f0;
}

.fact-row span {
  color: #64748b;
}

.fact-row strong,
.fact-row code {
  color: #0f172a;
  text-align: right;
  word-break: break-all;
}

.password-row code {
  padding: 6px 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>

<template>
  <div class="detection-alerts-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-select v-model="severity" :placeholder="$t('generated.common_severity_d918e4')" clearable class="filter-item">
          <el-option :label="$t('generated.common_serious_81ffc6')" value="critical" />
          <el-option :label="$t('generated.common_high_risk_e62ee8')" value="high" />
          <el-option :label="$t('generated.common_medium_risk_1098e6')" value="medium" />
          <el-option :label="$t('generated.detectionAlerts_low_risk_478c8d')" value="low" />
        </el-select>
        <el-select v-model="status" :placeholder="$t('generated.common_state_62e951')" clearable class="filter-item">
          <el-option :label="$t('generated.detectionAlerts_to_be_disposed_338bdb')" value="pending" />
          <el-option :label="$t('generated.detectionAlerts_disposed_02383d')" value="resolved" />
        </el-select>
        <el-select v-model="judgmentSource" :placeholder="$t('generated.detectionAlerts_determine_the_source_d1076c')" clearable class="filter-item">
          <el-option :label="$t('generated.detectionAlerts_system_judgment_3a560c')" value="system" />
          <el-option :label="$t('generated.detectionAlerts_ai_judgment_d51ad5')" value="ai" />
        </el-select>
        <el-input v-model="query" :placeholder="$t('generated.detectionAlerts_search_for_host_or_mitre_21d560')" clearable class="search-input">
          <template #append>
            <el-button :icon="Search" @click="loadAlerts" />
          </template>
        </el-input>
        <el-button type="primary" @click="loadAlerts">{{ $t('generated.common_query_711363') }}</el-button>
        <el-button type="danger" :disabled="selectedAlerts.length === 0" @click="handleBatchDelete">
          {{ $t('generated.common_batch_delete_4edb06') }}{{ selectedAlerts.length }})
        </el-button>
      </div>
    </el-card>

    <el-card>
      <el-table v-loading="alertLoading" :data="alerts" border stripe @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="rule_title" :label="$t('generated.common_rule_name_1937bc')" min-width="180">
          <template #default="{ row }">
            {{ row.rule_title || row.mitre_id || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="hostname" :label="$t('generated.common_host_2e8a0c')" min-width="150" />
        <el-table-column label="MITRE" width="120">
          <template #default="{ row }">
            <el-link type="primary" @click="goToRules(row.mitre_id)">{{ row.mitre_id || '-' }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="severity" :label="$t('generated.common_severity_d918e4')" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="severityTagType(row.severity)">{{ severityLabel(row.severity) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="hit_count" :label="$t('generated.detectionAlerts_number_of_hits_abb17d')" width="90" align="center" />
        <el-table-column prop="process_count" :label="$t('generated.common_number_of_processes_f2b9d5')" width="80" align="center">
          <template #default="{ row }">{{ row.process_count || '-' }}</template>
        </el-table-column>
        <el-table-column prop="judgment_source" :label="$t('generated.detectionAlerts_determine_the_source_d1076c')" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.judgment_source === 'ai' ? 'warning' : 'info'" size="small">
              {{ judgmentSourceLabel(row.judgment_source) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="$t('generated.common_state_62e951')" width="90" align="center">
          <template #default="{ row }">
            <el-tooltip
              v-if="row.block_status === 'failed' && row.block_message"
              :content="row.block_message"
              placement="top"
            >
              <el-tag :type="statusTagType(row.status, row.block_status)">
                {{ statusLabel(row.status, row.block_status) }}
              </el-tag>
            </el-tooltip>
            <el-tag v-else :type="statusTagType(row.status, row.block_status)">
              {{ statusLabel(row.status, row.block_status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="last_seen_at" :label="$t('generated.detectionAlerts_recent_hit_367257')" width="160">
          <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="280" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" @click="showDetail(row)">{{ $t('generated.common_details_4f55ee') }}</el-button>
            <el-button size="small" type="success" :disabled="row.status !== 'pending'" @click="handleResolve(row)">
              {{ $t('generated.detectionAlerts_dispose_4f136d') }}
            </el-button>
            <el-button size="small" type="danger" :disabled="row.status !== 'pending' || row.block_status === 'success'" @click="showBlockDialog(row)">
              {{ $t('generated.detectionAlerts_block_8b8621') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="alertTotal"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="loadAlerts"
          @size-change="loadAlerts"
        />
      </div>
    </el-card>

    <el-dialog v-model="detailVisible" :title="$t('generated.detectionAlerts_alarm_details_775787')" width="900px">
      <el-descriptions v-if="selectedAlert" :column="2" border>
        <el-descriptions-item :label="$t('generated.common_rule_name_1937bc')" :span="2">{{ selectedAlert.rule_title || selectedAlert.mitre_id || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_host_2e8a0c')">{{ selectedAlert.hostname || selectedAlert.host_id }}</el-descriptions-item>
        <el-descriptions-item label="MITRE ID">
          <el-link type="primary" @click="goToRules(selectedAlert.mitre_id)">{{ selectedAlert.mitre_id }}</el-link>
        </el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionAlerts_process_pid_936acf')">{{ selectedAlert.pid }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_number_of_processes_f2b9d5')">{{ selectedAlert.process_count || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionAlerts_determine_the_source_d1076c')">{{ judgmentSourceLabel(selectedAlert.judgment_source) }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_state_62e951')">{{ statusLabel(selectedAlert.status, selectedAlert.block_status) }}</el-descriptions-item>
        <el-descriptions-item v-if="selectedAlert.block_status" :label="$t('generated.detectionAlerts_blocking_state_cb24f1')">
          <el-tag :type="selectedAlert.block_status === 'success' ? 'success' : selectedAlert.block_status === 'failed' ? 'danger' : 'warning'">
            {{ selectedAlert.block_status === 'success' ? $t('dynamic.blockSuccess') : selectedAlert.block_status === 'failed' ? $t('dynamic.blockFailed') : $t('dynamic.blocking') }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item v-if="selectedAlert.block_message" :label="$t('generated.detectionAlerts_block_results_9d7a33')" :span="2">
          <el-text :type="selectedAlert.block_status === 'failed' ? 'danger' : 'success'">{{ selectedAlert.block_message }}</el-text>
        </el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionAlerts_number_of_hits_abb17d')">{{ selectedAlert.hit_count }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionAlerts_first_discovered_2b984c')">{{ formatTime(selectedAlert.first_seen_at) }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionAlerts_recently_discovered_a65be4')">{{ formatTime(selectedAlert.last_seen_at) }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionAlerts_llm_summary_d886f9')" :span="2">
          {{ selectedAlert.llm_summary || $t('dynamic.waitingAiAnalysis') }}
        </el-descriptions-item>
        <el-descriptions-item v-if="selectedAlert.llm_disposal_strategy" :label="$t('generated.detectionAlerts_disposal_strategy_0b0a6a')" :span="2">
          {{ selectedAlert.llm_disposal_strategy }}
        </el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_describe_412f54')" :span="2">{{ selectedAlert.description || '-' }}</el-descriptions-item>
      </el-descriptions>
      <div v-if="selectedAlert" class="process-tree-section">
        <h4>{{ $t('generated.detectionAlerts_process_tree_274ce7') }}</h4>
        <ProcessTree :process-tree="selectedAlert.process_tree" />
      </div>
    </el-dialog>

    <el-dialog v-model="blockDialogVisible" :title="$t('generated.detectionAlerts_select_blocking_action_59cb1e')" width="400px">
      <el-form label-width="100px">
        <el-form-item :label="$t('generated.common_blocking_action_b3edea')">
          <el-select v-model="blockAction" :placeholder="$t('generated.detectionAlerts_please_select_382f4b')">
            <el-option :label="$t('generated.common_terminate_process_58d47f')" value="kill_process" />
            <el-option :label="$t('generated.common_quarantine_files_749329')" value="quarantine_file" />
            <el-option :label="$t('generated.common_block_network_3260dd')" value="block_connection" />
            <el-option :label="$t('generated.common_disable_user_638157')" value="disable_user" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="blockDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="primary" @click="confirmBlock">{{ $t('generated.detectionAlerts_confirm_blocking_67829a') }}</el-button>
      </template>
    </el-dialog>

    </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { useDetectionStore } from '@/store/detection'
import * as api from '@/api/detection'
import type { Alert } from '@/types'
import { SeverityLabelKeys, AlertStatusLabelKeys, BlockStatusLabelKeys, JudgmentSourceLabelKeys } from '@/types'
import ProcessTree from '@/components/ProcessTree.vue'

const router = useRouter()

const store = useDetectionStore()

const severity = ref('')
const status = ref('')
const judgmentSource = ref('')
const query = ref('')
const page = ref(1)
const pageSize = ref(10)
const detailVisible = ref(false)
const selectedAlert = ref<Alert | null>(null)
const blockDialogVisible = ref(false)
const blockAction = ref('kill_process')
const blockTargetAlert = ref<Alert | null>(null)
const selectedAlerts = ref<Alert[]>([])

function handleSelectionChange(selection: Alert[]) {
  selectedAlerts.value = selection
}

const alerts = computed(() => store.alerts)
const alertTotal = computed(() => store.alertTotal)
const alertLoading = computed(() => store.alertLoading)

function formatTime(time: string) {
  if (!time) return '-'
  return formatDateTime(time)
}

function goToRules(mitreId: string) {
  if (!mitreId) return
  router.push({ path: '/detection/rules', query: { query: mitreId } })
}

function severityTagType(level: string) {
  if (level === 'critical') return 'danger'
  if (level === 'high') return 'warning'
  if (level === 'medium') return 'info'
  return 'success'
}

function severityLabel(level: string) {
  return SeverityLabelKeys[level] ? translate(SeverityLabelKeys[level]) : level
}

function statusTagType(status: string, blockStatus?: string) {
  if (blockStatus === 'failed') return 'danger'
  if (blockStatus === 'success') return 'success'
  if (blockStatus === 'blocking') return 'warning'
  if (status === 'resolved') return 'success'
  return 'warning'
}

function statusLabel(status: string, blockStatus?: string) {
  if (blockStatus) return BlockStatusLabelKeys[blockStatus] ? translate(BlockStatusLabelKeys[blockStatus]) : blockStatus
  return AlertStatusLabelKeys[status] ? translate(AlertStatusLabelKeys[status]) : status
}

function judgmentSourceLabel(source: string) {
  return JudgmentSourceLabelKeys[source] ? translate(JudgmentSourceLabelKeys[source]) : source
}

async function loadAlerts() {
  await store.fetchAlerts({
    page: page.value,
    pageSize: pageSize.value,
    severity: severity.value || undefined,
    status: status.value || undefined,
    judgment_source: judgmentSource.value || undefined,
    query: query.value || undefined
  })
}

async function showDetail(row: Alert) {
  selectedAlert.value = await api.getAlertDetail(row.alert_id)
  detailVisible.value = true
}

async function handleResolve(row: Alert) {
  await api.resolveAlert(row.alert_id)
  ElMessage.success(translate('generatedScript.detectionAlerts_alarm_has_been_handled_ee126b'))
  loadAlerts()
}

function showBlockDialog(row: Alert) {
  blockTargetAlert.value = row
  blockAction.value = 'kill_process'
  blockDialogVisible.value = true
}

async function confirmBlock() {
  if (!blockTargetAlert.value) return
  const record = await api.blockAlert(blockTargetAlert.value.alert_id, blockAction.value)
  if (record.success) {
    ElMessage.success(record.message || translate('generatedScript.detectionAlerts_blocked_successfully_92ac16'))
  } else {
    ElMessage.error(record.message || translate('generatedScript.detectionAlerts_blocking_failed_reason_unknown_aca7f4'))
  }
  blockDialogVisible.value = false
  loadAlerts()
}

async function handleBatchDelete() {
  if (selectedAlerts.value.length === 0) {
    ElMessage.warning(translate('generatedScript.detectionAlerts_please_select_the_alarm_you_want_ac9f5c'))
    return
  }
  
  try {
    const alertIds = selectedAlerts.value.map(a => a.alert_id)
    await api.deleteAlerts(alertIds)
    ElMessage.success(translate('generatedScript.detectionAlerts_alerts_deleted_a58aa2', { p0: alertIds.length }))
    selectedAlerts.value = []
    loadAlerts()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || translate('generatedScript.common_delete_failed_72250c'))
  }
}

onMounted(() => {
  loadAlerts()
})
</script>

<style scoped>
.detection-alerts-page {
  padding: 20px;
}

.filter-card {
  margin-bottom: 16px;
}

.filter-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-item {
  width: 140px;
}

.search-input {
  width: 240px;
}

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.form-hint {
  margin-left: 10px;
  color: #909399;
  font-size: 12px;
}

.process-tree-section {
  margin-top: 20px;
  border-top: 1px solid #ebeef5;
  padding-top: 16px;
}

.process-tree-section h4 {
  margin: 0 0 12px 0;
  color: #303133;
  font-size: 14px;
  font-weight: 500;
}

</style>

<template>
  <div class="detection-rules-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="searchQuery"
          :placeholder="$t('generated.detectionRules_search_rule_title_rule_id_mitre_ddfcda')"
          clearable
          class="search-input"
          @keyup.enter="loadRules"
        />
        <el-select v-model="status" :placeholder="$t('generated.detectionRules_rule_status_1bf43e')" clearable class="filter-item">
          <el-option :label="$t('generated.common_pending_review_f53b68')" value="pending" />
          <el-option :label="$t('generated.detectionRules_experimental_2600d7')" value="experimental" />
          <el-option :label="$t('generated.detectionRules_activated_b1eea7')" value="active" />
          <el-option :label="$t('generated.common_disabled_0fe5a9')" value="disabled" />
        </el-select>
        <el-button type="primary" @click="loadRules">{{ $t('generated.common_query_711363') }}</el-button>
        <el-button type="success" @click="showAIGenerateDialog">{{ $t('generated.detectionRules_ai_rules_1e58b8') }}</el-button>
        <el-dropdown :disabled="selectedRules.length === 0" @command="handleBatchCommand">
          <el-button type="danger" :disabled="selectedRules.length === 0">
            {{ $t('generated.detectionRules_batch_operations_0b9953') }}{{ selectedRules.length }})<el-icon class="el-icon--right"><arrow-down /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="enable">{{ $t('generated.detectionRules_enable_checked_5af677') }}</el-dropdown-item>
              <el-dropdown-item command="disable">{{ $t('generated.detectionRules_disable_selected_27e218') }}</el-dropdown-item>
              <el-dropdown-item command="delete" divided>{{ $t('generated.detectionRules_remove_selected_42b7a0') }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button type="warning" @click="showAIConfigDrawer = true" class="ai-config-btn">{{ $t('generated.detectionRules_ai_rules_automatically_update_configuration_191f3a') }}</el-button>
        <el-button type="primary" @click="showImportDialog">{{ $t('generated.detectionRules_import_rules_819d0c') }}</el-button>
      </div>
    </el-card>

    <!-- AI规则自动更新配置抽屉 -->
    <el-drawer
      v-model="showAIConfigDrawer"
      :title="$t('generated.detectionRules_ai_rules_automatically_update_configuration_191f3a')"
      direction="rtl"
      size="600px"
    >
      <AIConfigPanel />
    </el-drawer>

    <el-card>
      <el-table 
        v-loading="ruleLoading" 
        :data="rules" 
        border 
        stripe
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="55" />
        <el-table-column prop="title" :label="$t('generated.common_rule_title_298a16')" min-width="280" />
        <el-table-column label="MITRE" width="120">
          <template #default="{ row }">
            <el-link type="primary" @click="goToPolicies(row.mitre_id)">{{ row.mitre_id }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="severity" :label="$t('generated.common_severity_d918e4')" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="severityTagType(row.severity)">{{ severityLabel(row.severity) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="$t('generated.common_state_62e951')" width="100" align="center">
          <template #default="{ row }">
            <el-tooltip :content="statusHelp(row.status)" placement="top">
              <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="version" :label="$t('generated.common_version_989d1a')" width="80" align="center" />
        <el-table-column prop="created_at" :label="$t('generated.common_creation_time_84e380')" width="160">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="260" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" @click="showDetail(row)">{{ $t('generated.common_details_4f55ee') }}</el-button>
            <el-button size="small" type="success" :disabled="row.status === 'active'" @click="approveRule(row.rule_id)">
              {{ $t('generated.common_enable_d4e9ca') }}
            </el-button>
            <el-button size="small" type="warning" :disabled="row.status === 'disabled'" @click="disableRule(row.rule_id)">
              {{ $t('generated.common_disable_be70be') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="ruleTotal"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="loadRules"
          @size-change="loadRules"
        />
      </div>
    </el-card>

    <el-dialog v-model="detailVisible" :title="$t('generated.detectionRules_rule_details_86c2d6')" width="780px">
      <el-descriptions v-if="selectedRule" :column="2" border>
        <el-descriptions-item :label="$t('generated.common_rule_id_36c0e3')">{{ selectedRule.rule_id }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_title_748d7d')">{{ selectedRule.title || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_state_62e951')">{{ statusLabel(selectedRule.status) }}</el-descriptions-item>
        <el-descriptions-item label="MITRE">{{ selectedRule.mitre_id || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_severity_d918e4')">{{ severityLabel(selectedRule.severity) || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_version_989d1a')">{{ selectedRule.version }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionRules_generation_method_9d4852')">{{ selectedRule.generated_by === 'llm' ? $t('dynamic.generatedByLLM') : $t('dynamic.importedManually') }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_creation_time_84e380')">{{ formatTime(selectedRule.created_at) }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.detectionRules_activation_time_aa327d')">{{ formatTime(selectedRule.activated_at || '') }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_describe_412f54')" :span="2">{{ selectedRule.description || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_rule_content_3bfca1')" :span="2">
          <pre class="content-block">{{ selectedRule.content || '-' }}</pre>
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>

    <el-dialog v-model="aiGenerateVisible" :title="$t('generated.detectionRules_ai_generates_sigma_rules_4a9651')" width="700px" :close-on-click-modal="false">
      <el-form :model="aiGenerateForm" label-width="100px">
        <el-form-item :label="$t('generated.detectionRules_detect_events_63e761')" required>
          <el-input
            v-model="aiGenerateForm.event"
            type="textarea"
            :rows="3"
            :placeholder="$t('generated.detectionRules_describe_the_security_events_to_be_10c3cb')"
          />
        </el-form-item>
        <el-form-item :label="$t('generated.detectionRules_detection_method_6235a2')">
          <el-input
            v-model="aiGenerateForm.method"
            type="textarea"
            :rows="3"
            :placeholder="$t('generated.detectionRules_describe_the_detection_method_for_example_6dabd1')"
          />
        </el-form-item>
        <el-form-item :label="$t('generated.detectionRules_miter_technology_89ff8e')">
          <el-input v-model="aiGenerateForm.mitre_id" :placeholder="$t('generated.detectionRules_optional_for_example_t1059_004_7a06d2')" />
        </el-form-item>
        <el-form-item :label="$t('generated.common_severity_d918e4')">
          <el-select v-model="aiGenerateForm.severity" :placeholder="$t('generated.detectionRules_select_severity_194ba7')">
            <el-option :label="$t('generated.common_low_b9ee25')" value="low" />
            <el-option :label="$t('generated.common_middle_086907')" value="medium" />
            <el-option :label="$t('generated.common_high_b096b3')" value="high" />
            <el-option :label="$t('generated.common_serious_81ffc6')" value="critical" />
          </el-select>
        </el-form-item>
      </el-form>

      <div v-if="aiGenerateResult" class="ai-result">
        <el-divider>{{ $t('generated.common_generate_results_99045f') }}</el-divider>
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('generated.common_rule_id_36c0e3')">{{ aiGenerateResult.rule_id }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_rule_title_298a16')">{{ aiGenerateResult.title }}</el-descriptions-item>
          <el-descriptions-item label="MITRE">{{ aiGenerateResult.mitre_id || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_severity_d918e4')">{{ aiGenerateResult.severity }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.detectionRules_time_consuming_to_generate_070b51')">{{ aiGenerateResult.duration }}{{ $t('generated.detectionRules_second_eb6aab') }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_state_62e951')">
            <el-tag type="warning">{{ $t('generated.detectionRules_experimental_2600d7') }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_rule_content_3bfca1')" :span="2">
            <pre class="content-block">{{ aiGenerateResult.content }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <template #footer>
        <el-button @click="aiGenerateVisible = false">{{ $t('generated.common_closure_6c14bd') }}</el-button>
        <el-button type="primary" :loading="aiGenerateLoading" @click="generateRule">
          {{ aiGenerateLoading ? $t('common.status.generating') : $t('dynamic.startGenerating') }}
        </el-button>
        <el-button
          v-if="aiGenerateResult"
          type="success"
          @click="enableGeneratedRule"
        >
          {{ $t('generated.detectionRules_enable_rules_33676b') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deleteConfirmVisible" :title="$t('generated.common_confirm_deletion_3c06ab')" width="500px">
      <div v-if="deleteCheckResult">
        <el-alert
          v-if="deleteCheckResult.has_alerts"
          :title="$t('generated.detectionRules_warning_the_selected_rule_is_associated_e53c8b')"
          type="warning"
          :closable="false"
          show-icon
          style="margin-bottom: 16px"
        >
          <template #default>
            <p>{{ $t('generated.detectionRules_the_following_rules_have_associated_alarms_9748b1') }}</p>
            <ul style="margin: 8px 0; padding-left: 20px;">
              <li v-for="rule in deleteCheckResult.rules_with_alerts" :key="rule.rule_id">
                {{ rule.title || rule.rule_id }} ({{ rule.alert_count }} {{ $t('generated.detectionRules_alarm_a3f965') }}
              </li>
            </ul>
          </template>
        </el-alert>
        
        <p>{{ $t('generated.detectionRules_are_you_sure_you_want_to_2cf5b8') }} {{ selectedRules.length }} {{ $t('generated.detectionRules_a_rule_ed3d22') }}</p>
        <p v-if="deleteCheckResult.has_alerts" style="color: #e6a23c;">
          {{ $t('generated.detectionRules_will_also_be_deleted_e50036') }} {{ deleteCheckResult.total_alerts }} {{ $t('generated.detectionRules_associated_alarms_and_corresponding_blocking_rules_765fab') }}
        </p>
      </div>
      
      <template #footer>
        <el-button @click="deleteConfirmVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="danger" :loading="deleteLoading" @click="deleteSelectedRules">
          {{ $t('generated.common_confirm_deletion_3c06ab') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importVisible" :title="$t('generated.detectionRules_import_sigma_rules_56f5e1')" width="600px" @close="handleImportDialogClose">
      <div class="import-dialog-content">
        <el-upload
          ref="uploadRef"
          class="sigma-upload"
          drag
          :auto-upload="false"
          :limit="10"
          accept=".yaml,.yml,.zip"
          multiple
          :on-exceed="handleExceed"
          :on-change="handleFileChange"
          :on-remove="handleFileRemove"
        >
          <el-icon class="el-icon--upload"><upload-filled /></el-icon>
          <div class="el-upload__text">
            {{ $t('generated.detectionRules_drag_and_drop_files_here_or_971bc4') }} <em>{{ $t('generated.common_click_to_upload_69aaf1') }}</em>
          </div>
          <template #tip>
            <div class="el-upload__tip">
              {{ $t('generated.detectionRules_supports_yaml_yml_or_zip_format_0421d4') }}
            </div>
          </template>
        </el-upload>

        <div v-if="importResult" class="import-result">
          <el-divider>{{ $t('generated.detectionRules_import_results_747404') }}</el-divider>
          <el-alert
            v-if="importResult.success"
            :title="$t('generated.detectionRules_import_successful_425516')"
            type="success"
            :description="$t('dynamic.rulesParsed', { parsed: importResult.parsed_count, skipped: importResult.skipped_count })"
            show-icon
          />
          <el-alert
            v-else-if="importResult.failed_count > 0"
            :title="$t('generated.detectionRules_partial_import_failed_98e026')"
            type="warning"
            :description="$t('dynamic.rulesPartiallyParsed', { parsed: importResult.parsed_count, failed: importResult.failed_count, skipped: importResult.skipped_count })"
            show-icon
          />
          <el-alert
            v-else
            :title="$t('generated.detectionRules_import_failed_a01a8d')"
            type="error"
            :description="importError"
            show-icon
          />
          <div v-if="importResult.failed_files && importResult.failed_files.length > 0" class="imported-rules" style="margin-top: 16px;">
            <h4>{{ $t('generated.detectionRules_failed_file_1e8f12') }}</h4>
            <el-alert
              v-for="file in importResult.failed_files"
              :key="file"
              type="error"
              :title="file"
              :closable="false"
              show-icon
              style="margin-bottom: 8px;"
            />
          </div>
          <div v-if="importResult.rules && importResult.rules.length > 0" class="imported-rules">
            <h4>{{ $t('generated.detectionRules_imported_rules_823975') }}</h4>
            <el-table :data="importResult.rules" size="small" border>
              <el-table-column prop="rule_id" :label="$t('generated.common_rule_id_36c0e3')" />
              <el-table-column prop="title" :label="$t('generated.common_title_748d7d')" />
              <el-table-column prop="mitre_id" label="MITRE" />
              <el-table-column prop="severity" :label="$t('generated.common_severity_d918e4')" />
            </el-table>
          </div>
        </div>
      </div>

      <template #footer>
        <el-button @click="closeImportDialog">{{ $t('generated.common_closure_6c14bd') }}</el-button>
        <el-button type="primary" :loading="importLoading" :disabled="selectedFiles.length === 0" @click="handleImport">
          {{ $t('generated.detectionRules_start_import_e4a45c') }}{{ selectedFiles.length }} {{ $t('generated.detectionRules_files_df8f22') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { UploadFilled, ArrowDown } from '@element-plus/icons-vue'
import { useDetectionStore } from '@/store/detection'
import type { SigmaRule } from '@/types'
import { SeverityLabelKeys, RuleStatusLabelKeys } from '@/types'
import * as api from '@/api/detection'
import type { UploadRawFile, UploadInstance } from 'element-plus'
import AIConfigPanel from '@/components/detection/AIConfigPanel.vue'

const route = useRoute()
const router = useRouter()
const store = useDetectionStore()

const searchQuery = ref('')
const status = ref('')
const page = ref(1)
const pageSize = ref(10)
const detailVisible = ref(false)
const selectedRule = ref<SigmaRule | null>(null)

const selectedRules = ref<SigmaRule[]>([])
const deleteConfirmVisible = ref(false)
const showAIConfigDrawer = ref(false)
const deleteLoading = ref(false)
const deleteCheckResult = ref<{
  has_alerts: boolean
  rules_with_alerts: Array<{ rule_id: string; title: string; alert_count: number }>
  total_alerts: number
} | null>(null)

const aiGenerateVisible = ref(false)
const aiGenerateLoading = ref(false)
const aiGenerateForm = ref({
  event: '',
  method: '',
  mitre_id: '',
  severity: 'medium'
})
const aiGenerateResult = ref<{
  rule_id: string
  title: string
  mitre_id: string
  severity: string
  content: string
  duration: number
} | null>(null)

const importVisible = ref(false)
const importLoading = ref(false)
const uploadRef = ref<UploadInstance | null>(null)
const selectedFiles = ref<UploadRawFile[]>([])
const importError = ref('')
const importResult = ref<{
  success: boolean
  parsed_count: number
  failed_count: number
  skipped_count: number
  rules: Array<{
    rule_id: string
    title: string
    status: string
    mitre_id: string
    severity: string
  }>
  failed_files?: string[]
} | null>(null)

const rules = computed(() => store.rules)
const ruleTotal = computed(() => store.ruleTotal)
const ruleLoading = computed(() => store.ruleLoading)

function formatTime(time: string) {
  if (!time) return '-'
  return formatDateTime(time)
}

function statusTagType(ruleStatus: string) {
  if (ruleStatus === 'active') return 'success'
  if (ruleStatus === 'disabled') return 'info'
  if (ruleStatus === 'experimental') return 'warning'
  return 'danger'
}

function statusLabel(ruleStatus: string) {
  return RuleStatusLabelKeys[ruleStatus] ? translate(RuleStatusLabelKeys[ruleStatus]) : ruleStatus
}

function statusHelp(ruleStatus: string) {
  if (ruleStatus === 'pending') return translate('generatedScript.detectionRules_pending_review_not_sent_to_agent_8be1ab')
  if (ruleStatus === 'experimental') return translate('generatedScript.detectionRules_in_trial_operation_has_been_delivered_520ffe')
  if (ruleStatus === 'active') return translate('generatedScript.detectionRules_officially_enabled_and_distributed_to_agent_3267d7')
  if (ruleStatus === 'disabled') return translate('generatedScript.detectionRules_disabled_not_delivered_to_agent_3a3cf0')
  return translate('generatedScript.detectionRules_unknown_status_4e72a1')
}

function severityTagType(level: string) {
  if (level === 'critical') return 'danger'
  if (level === 'high') return 'warning'
  if (level === 'medium') return 'info'
  if (level === 'low' || level === 'informational') return 'success'
  return 'info'
}

function severityLabel(level?: string) {
  if (!level) return ''
  return SeverityLabelKeys[level] ? translate(SeverityLabelKeys[level]) : level
}

function goToPolicies(mitreId: string) {
  if (!mitreId) return
  router.push({ path: '/detection/policies', query: { query: mitreId } })
}

async function loadRules() {
  await store.fetchRules({
    page: page.value,
    pageSize: pageSize.value,
    status: status.value || undefined,
    query: searchQuery.value || undefined
  })
}

function handleSelectionChange(selection: SigmaRule[]) {
  selectedRules.value = selection
}

function showDetail(rule: SigmaRule) {
  selectedRule.value = rule
  detailVisible.value = true
}

async function approveRule(ruleId: string) {
  await store.updateRuleStatus(ruleId, 'active')
  ElMessage.success(translate('generatedScript.detectionRules_rule_is_enabled_5b9fa6'))
}

async function disableRule(ruleId: string) {
  await store.updateRuleStatus(ruleId, 'disabled')
  ElMessage.success(translate('generatedScript.detectionRules_rule_disabled_edbe5d'))
}

async function confirmDeleteSelected() {
  if (selectedRules.value.length === 0) {
    ElMessage.warning(translate('generatedScript.detectionRules_please_select_the_rules_to_delete_49abad'))
    return
  }

  deleteLoading.value = true
  try {
    const ruleIds = selectedRules.value.map(r => r.rule_id)
    const result = await api.checkRulesBeforeDelete(ruleIds)
    deleteCheckResult.value = result
    deleteConfirmVisible.value = true
  } catch (error: any) {
    ElMessage.error(error.message || translate('generatedScript.detectionRules_check_rule_failed_137ed4'))
  } finally {
    deleteLoading.value = false
  }
}

async function deleteSelectedRules() {
  deleteLoading.value = true
  try {
    const ruleIds = selectedRules.value.map(r => r.rule_id)
    const result = await api.deleteRules(ruleIds)
    ElMessage.success(translate('generatedScript.detectionRules_rules_alarms_blocking_rules_have_been_a0f7d2', { p0: result.deleted_rules, p1: result.deleted_alerts, p2: result.deleted_policies }))
    deleteConfirmVisible.value = false
    selectedRules.value = []
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || translate('generatedScript.common_delete_failed_72250c'))
  } finally {
    deleteLoading.value = false
  }
}

async function batchEnableSelected() {
  if (selectedRules.value.length === 0) {
    ElMessage.warning(translate('generatedScript.detectionRules_please_select_a_rule_to_enable_2b65a2'))
    return
  }

  try {
    const ruleIds = selectedRules.value.map(r => r.rule_id)
    const promises = ruleIds.map(ruleId => store.updateRuleStatus(ruleId, 'active'))
    await Promise.all(promises)
    ElMessage.success(translate('generatedScript.detectionRules_rules_enabled_33ba61', { p0: ruleIds.length }))
    selectedRules.value = []
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || translate('generatedScript.detectionRules_failed_to_enable_b8a2f0'))
  }
}

async function batchDisableSelected() {
  if (selectedRules.value.length === 0) {
    ElMessage.warning(translate('generatedScript.detectionRules_please_select_a_rule_to_disable_83d9ce'))
    return
  }

  try {
    const ruleIds = selectedRules.value.map(r => r.rule_id)
    const promises = ruleIds.map(ruleId => store.updateRuleStatus(ruleId, 'disabled'))
    await Promise.all(promises)
    ElMessage.success(translate('generatedScript.detectionRules_rules_disabled_40aca4', { p0: ruleIds.length }))
    selectedRules.value = []
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || translate('generatedScript.detectionRules_disable_failed_8c9b4c'))
  }
}

async function handleBatchCommand(command: string) {
  if (selectedRules.value.length === 0) {
    ElMessage.warning(translate('generatedScript.detectionRules_please_select_the_rule_you_want_3b7af7'))
    return
  }
  if (command === 'enable') {
    await batchEnableSelected()
  } else if (command === 'disable') {
    await batchDisableSelected()
  } else if (command === 'delete') {
    await confirmDeleteSelected()
  }
}

function showAIGenerateDialog() {
  aiGenerateForm.value = {
    event: '',
    method: '',
    mitre_id: '',
    severity: 'medium'
  }
  aiGenerateResult.value = null
  aiGenerateVisible.value = true
}

async function generateRule() {
  if (!aiGenerateForm.value.event) {
    ElMessage.warning(translate('generatedScript.detectionRules_please_enter_a_detection_event_description_d0c494'))
    return
  }

  aiGenerateLoading.value = true
  try {
    const result = await api.generateSigmaRule({
      event: aiGenerateForm.value.event,
      method: aiGenerateForm.value.method,
      mitre_id: aiGenerateForm.value.mitre_id,
      severity: aiGenerateForm.value.severity
    })
    aiGenerateResult.value = result
    ElMessage.success(translate('generatedScript.detectionRules_rule_generation_successful_b88234'))
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || translate('generatedScript.detectionRules_rule_generation_failed_7b38f1'))
  } finally {
    aiGenerateLoading.value = false
  }
}

async function enableGeneratedRule() {
  if (!aiGenerateResult.value) return

  try {
    await store.updateRuleStatus(aiGenerateResult.value.rule_id, 'active')
    ElMessage.success(translate('generatedScript.detectionRules_the_rule_has_been_enabled_and_e2ceee'))
    aiGenerateVisible.value = false
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || translate('generatedScript.detectionRules_failed_to_enable_rule_21b930'))
  }
}

function showImportDialog() {
  selectedFiles.value = []
  importResult.value = null
  importError.value = ''
  // Clear the el-upload component's internal file list to prevent previously selected files from reappearing
  if (uploadRef.value) {
    uploadRef.value.clearFiles()
  }
  importVisible.value = true
}

function handleExceed(files: File[]) {
  ElMessage.warning(translate('generatedScript.detectionRules_supports_up_to_10_files_please_17cdbc'))
}

function handleFileChange(file: UploadRawFile, fileList: UploadRawFile[]) {
  // Use spread operator to create a new array for proper reactivity
  selectedFiles.value = [...fileList]
}

function handleFileRemove(file: UploadRawFile, fileList: UploadRawFile[]) {
  // Use spread operator to create a new array for proper reactivity
  selectedFiles.value = [...fileList]
}

function handleImportDialogClose() {
  selectedFiles.value = []
  importResult.value = null
  importError.value = ''
}

function closeImportDialog() {
  // Clear the el-upload component's internal file list
  if (uploadRef.value) {
    uploadRef.value.clearFiles()
  }
  // Reset state
  selectedFiles.value = []
  importResult.value = null
  importError.value = ''
  importVisible.value = false
}

async function handleImport() {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning(translate('generatedScript.detectionRules_please_select_the_file_to_import_91e2f0'))
    return
  }

  importLoading.value = true
  importError.value = ''
  importResult.value = null

  let totalParsedCount = 0
  let totalFailedCount = 0
  let totalSkippedCount = 0
  const allRules: Array<{
    rule_id: string
    title: string
    status: string
    mitre_id: string
    severity: string
  }> = []
  const allFailedFiles: string[] = []

  try {
    for (const fileItem of selectedFiles.value) {
      try {
        const result = await api.uploadSigmaRules(fileItem.raw as File)
        totalParsedCount += result.parsed_count
        totalFailedCount += result.failed_count
        totalSkippedCount += result.skipped_count
        if (result.rules) {
          allRules.push(...result.rules)
        }
        if (result.failed_files) {
          allFailedFiles.push(...result.failed_files)
        }
      } catch (error: any) {
        totalFailedCount++
        allFailedFiles.push(fileItem.name)
        // Extract error message from axios error response
        const errorMsg = error.response?.data?.message || error.message || translate('generatedScript.common_upload_failed_a6f805')
        ElMessage.warning(translate('generatedScript.detectionRules_file_failed_to_upload_cc7b3a', { p0: fileItem.name, p1: errorMsg }))
      }
    }

    const overallSuccess = totalFailedCount === 0
    importResult.value = {
      success: overallSuccess,
      parsed_count: totalParsedCount,
      failed_count: totalFailedCount,
      skipped_count: totalSkippedCount,
      rules: allRules,
      failed_files: allFailedFiles
    }

    if (overallSuccess) {
      ElMessage.success(translate('generatedScript.detectionRules_import_successful_rules_parsed_duplicate_rules_bdda6b', { p0: totalParsedCount, p1: totalSkippedCount }))
    } else {
      ElMessage.warning(translate('generatedScript.detectionRules_import_completed_items_successful_items_failed_c26454', { p0: totalParsedCount, p1: totalFailedCount, p2: totalSkippedCount }))
    }
    loadRules()
  } catch (error: any) {
    // Extract error message from axios error response
    const errorMsg = error.response?.data?.message || error.message || translate('generatedScript.detectionRules_import_failed_a01a8d')
    importError.value = errorMsg
    ElMessage.error(errorMsg)
  } finally {
    importLoading.value = false
  }
}

onMounted(() => {
  const queryParam = route.query.query as string
  if (queryParam) {
    searchQuery.value = queryParam
  }
  loadRules()
})
</script>

<style scoped>
.detection-rules-page {
  padding: 20px;
}

.filter-card {
  margin-bottom: 16px;
}

.filter-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: flex-start;
}

.ai-config-btn {
  margin-left: auto;
}

.filter-item {
  width: 160px;
}

.search-input {
  width: 280px;
}

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.content-block {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 300px;
  overflow-y: auto;
  background: #f5f7fa;
  padding: 10px;
  border-radius: 4px;
  font-size: 12px;
}

.ai-result {
  margin-top: 16px;
}

.import-dialog-content {
  min-height: 200px;
}

.sigma-upload {
  width: 100%;
}

.sigma-upload .el-upload-dragger {
  padding: 40px 20px;
}

.import-result {
  margin-top: 20px;
}

.imported-rules {
  margin-top: 16px;
}

.imported-rules h4 {
  margin: 12px 0;
  font-size: 14px;
  color: #606266;
}
</style>

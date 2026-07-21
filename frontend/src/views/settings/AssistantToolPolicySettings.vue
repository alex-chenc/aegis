<template>
  <div class="settings page-shell">
    <section class="page-hero settings-hero">
      <div>
        <span class="hero-kicker">Agent Tool Policy</span>
        <h1>{{ $t('generated.settingsAssistantToolPolicySettings_agent_tool_permissions_92394c') }}</h1>
        <p>{{ $t('generated.settingsAssistantToolPolicySettings_configure_the_approval_mode_and_whitelist_0c18a3') }}</p>
      </div>
    </section>

    <div class="settings-grid">
      <!-- 审批模式选择 -->
      <el-card class="aegis-card settings-card">
        <template #header>
          <div class="card-header">
            <span>{{ $t('generated.settingsAssistantToolPolicySettings_tool_approval_mode_b911c9') }}</span>
            <el-tag :type="modeTagType" size="small">{{ modeLabel }}</el-tag>
          </div>
        </template>

        <el-alert
          :title="modeAlertTitle"
          :type="modeAlertType"
          show-icon
          :closable="false"
          class="settings-alert"
        >
          <p>{{ modeAlertDescription }}</p>
        </el-alert>

        <el-radio-group v-model="currentMode" class="mode-selector" @change="handleModeChange">
          <el-radio-button value="request_approval">
            <el-icon><Lock /></el-icon>
            {{ $t('generated.settingsAssistantToolPolicySettings_request_approval_4e7978') }}
          </el-radio-button>
          <el-radio-button value="whitelist">
            <el-icon><List /></el-icon>
            {{ $t('generated.common_whitelist_8f74cd') }}
          </el-radio-button>
          <el-radio-button value="full_access">
            <el-icon><Unlock /></el-icon>
            {{ $t('generated.settingsAssistantToolPolicySettings_full_permission_da805f') }}
          </el-radio-button>
        </el-radio-group>
      </el-card>

      <!-- 工具白名单管理 -->
      <el-card class="aegis-card settings-card" :class="{ 'card-disabled': currentMode === 'request_approval' || currentMode === 'full_access' }">
        <template #header>
          <div class="card-header">
            <span>{{ $t('generated.settingsAssistantToolPolicySettings_tool_whitelist_36efe9') }}</span>
            <div class="card-header-actions">
              <el-tag size="small" type="info">{{ $t('generated.common_common_3b6ef8') }} {{ totalTools }} {{ $t('generated.settingsAssistantToolPolicySettings_tools_2a84b2') }}</el-tag>
              <el-button size="small" :loading="resetting" @click="handleResetDefaults">
                {{ $t('generated.settingsAssistantToolPolicySettings_restore_default_whitelist_88c462') }}
              </el-button>
            </div>
          </div>
        </template>

        <!-- 搜索和筛选 -->
        <div class="filter-bar">
          <el-input
            v-model="keyword"
            :placeholder="$t('generated.settingsAssistantToolPolicySettings_search_tool_name_description_8f0f51')"
            clearable
            class="filter-input"
            @input="debouncedFetch"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
          <el-select v-model="domainFilter" :placeholder="$t('generated.settingsAssistantToolPolicySettings_field_388b18')" clearable class="filter-select" @change="fetchTools">
            <el-option v-for="d in domainOptions" :key="d.value" :label="d.label" :value="d.value" />
          </el-select>
          <el-select v-model="riskFilter" :placeholder="$t('generated.common_risk_level_a90f1e')" clearable class="filter-select" @change="fetchTools">
            <el-option v-for="r in riskOptions" :key="r.value" :label="r.label" :value="r.value" />
          </el-select>
          <el-select v-model="whitelistFilter" :placeholder="$t('generated.settingsAssistantToolPolicySettings_whitelist_status_1bc7d6')" clearable class="filter-select" @change="fetchTools">
            <el-option :label="$t('generated.settingsAssistantToolPolicySettings_already_added_to_whitelist_7ebb98')" value="true" />
            <el-option :label="$t('generated.settingsAssistantToolPolicySettings_not_whitelisted_67ea0c')" value="false" />
          </el-select>
          <el-button
            size="small"
            :disabled="!hasSelection"
            @click="handleBatchWhitelist(true)"
          >
            {{ $t('generated.settingsAssistantToolPolicySettings_add_to_whitelist_in_batches_029e50') }}
          </el-button>
          <el-button
            size="small"
            :disabled="!hasSelection"
            @click="handleBatchWhitelist(false)"
          >
            {{ $t('generated.settingsAssistantToolPolicySettings_remove_from_whitelist_in_batches_b7d790') }}
          </el-button>
        </div>

        <!-- 工具表格 -->
        <el-table
          v-loading="loading"
          :data="tools"
          class="tool-table"
          @selection-change="handleSelectionChange"
        >
          <el-table-column type="selection" width="45" />
          <el-table-column prop="tool_name" :label="$t('generated.settingsAssistantToolPolicySettings_tool_name_e28d76')" min-width="180" show-overflow-tooltip />
          <el-table-column prop="domain" :label="$t('generated.settingsAssistantToolPolicySettings_field_388b18')" width="100">
            <template #default="{ row }">
              <el-tag size="small" type="info">{{ row.domain }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="operation" :label="$t('generated.common_operate_f3ea6d')" width="90">
            <template #default="{ row }">
              <el-tag size="small">{{ row.operation }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="risk_level" :label="$t('generated.settingsAssistantToolPolicySettings_risk_96a969')" width="90">
            <template #default="{ row }">
              <el-tag :type="getRiskTagType(row.risk_level)" size="small">{{ riskLabel(row.risk_level) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="description" :label="$t('generated.settingsAssistantToolPolicySettings_tool_details_9bbf4f')" min-width="240" show-overflow-tooltip />
          <el-table-column :label="$t('generated.common_whitelist_8f74cd')" width="90" align="center">
            <template #default="{ row }">
              <el-switch
                v-model="row.whitelisted"
                :disabled="currentMode !== 'whitelist'"
                @change="(val: boolean) => handleWhitelistChange(row, val)"
              />
            </template>
          </el-table-column>
        </el-table>

        <!-- 分页 -->
        <div class="pagination-bar">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :total="totalTools"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            @current-change="fetchTools"
            @size-change="fetchTools"
          />
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { List, Lock, Search, Unlock } from '@element-plus/icons-vue'
import {
  getToolApprovalPolicy,
  updateToolApprovalPolicy,
  getTools,
  updateToolWhitelist,
  batchUpdateWhitelist,
  resetWhitelistDefaults
} from '@/api/assistant'
import type { AssistantToolApprovalMode, AssistantRiskLevel } from '@/api/assistant'

// ---- 审批模式 ----
const currentMode = ref<AssistantToolApprovalMode>('whitelist')
const modeLoading = ref(false)

const modeLabel = computed(() => {
  const map: Record<AssistantToolApprovalMode, string> = {
    request_approval: translate('generatedScript.settingsAssistantToolPolicySettings_request_approval_4e7978'),
    whitelist: translate('generatedScript.settingsAssistantToolPolicySettings_whitelist_8f74cd'),
    full_access: translate('generatedScript.settingsAssistantToolPolicySettings_full_permission_da805f')
  }
  return map[currentMode.value] || currentMode.value
})

const modeTagType = computed(() => {
  const map: Record<AssistantToolApprovalMode, 'danger' | 'warning' | 'success'> = {
    request_approval: 'danger',
    whitelist: 'warning',
    full_access: 'success'
  }
  return map[currentMode.value] || 'info'
})

const modeAlertTitle = computed(() => {
  const map: Record<AssistantToolApprovalMode, string> = {
    request_approval: translate('generatedScript.settingsAssistantToolPolicySettings_request_approval_mode_1187df'),
    whitelist: translate('generatedScript.settingsAssistantToolPolicySettings_whitelist_mode_default_ec1c26'),
    full_access: translate('generatedScript.settingsAssistantToolPolicySettings_full_access_mode_f25595')
  }
  return map[currentMode.value]
})

const modeAlertType = computed(() => {
  const map: Record<AssistantToolApprovalMode, 'error' | 'warning' | 'success'> = {
    request_approval: 'error',
    whitelist: 'warning',
    full_access: 'success'
  }
  return map[currentMode.value]
})

const modeAlertDescription = computed(() => {
  const map: Record<AssistantToolApprovalMode, string> = {
    request_approval: translate('generatedScript.settingsAssistantToolPolicySettings_all_tool_calls_will_await_manual_6b5ee9'),
    whitelist: translate('generatedScript.settingsAssistantToolPolicySettings_whitelisted_tools_are_automatically_executed_non_2b5ace'),
    full_access: translate('generatedScript.settingsAssistantToolPolicySettings_all_tools_selected_by_the_agent_d24473')
  }
  return map[currentMode.value]
})

async function handleModeChange(mode: AssistantToolApprovalMode) {
  modeLoading.value = true
  try {
    await updateToolApprovalPolicy({ mode })
    ElMessage.success(translate('generatedScript.settingsAssistantToolPolicySettings_approval_mode_has_been_updated_0a8fb3'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
    // 回滚
    await fetchApprovalMode()
  } finally {
    modeLoading.value = false
  }
}

async function fetchApprovalMode() {
  try {
    // 拦截器已解包 data，res = { mode: "whitelist" }
    const res = await getToolApprovalPolicy()
    currentMode.value = (res as any).mode || 'whitelist'
  } catch {
    currentMode.value = 'whitelist'
  }
}

// ---- 工具白名单 ----
const tools = ref<any[]>([])
const totalTools = ref(0)
const loading = ref(false)
const resetting = ref(false)
const keyword = ref('')
const domainFilter = ref('')
const riskFilter = ref('')
const whitelistFilter = ref('')
const currentPage = ref(1)
const pageSize = ref(20)
const selectedTools = ref<any[]>([])
const hasSelection = computed(() => selectedTools.value.length > 0)

const domainOptions = computed(() => [
  { label: translate('generatedScript.common_system_1a1f6d'), value: 'system' },
  { label: translate('generatedScript.common_host_2e8a0c'), value: 'host' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_assets_713fd9'), value: 'asset' },
  { label: translate('generatedScript.common_baseline_4bb193'), value: 'baseline' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_task_3172b3'), value: 'task' },
  { label: translate('generatedScript.common_loopholes_86835d'), value: 'vulnerability' },
  { label: translate('generatedScript.common_detection_b3ff0c'), value: 'detection' },
  { label: translate('generatedScript.common_sigma_rules_80c495'), value: 'sigma_rule' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_block_8b8621'), value: 'block' },
  { label: translate('generatedScript.common_test_kit_757931'), value: 'package' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_configuration_d7d7ce'), value: 'config' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_audit_3a96c5'), value: 'audit' },
  { label: 'Agent', value: 'agent' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_research_and_judge_13bb05'), value: 'investigation' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_external_mcp_72d932'), value: 'external_mcp' },
  { label: translate('generatedScript.settingsAssistantToolPolicySettings_notify_7a66c0'), value: 'notification' }
])

const riskOptions = computed(() => [
  { label: translate('generatedScript.common_read_only_ffc1d0'), value: 'readonly' },
  { label: translate('generatedScript.common_low_b9ee25'), value: 'low' },
  { label: translate('generatedScript.common_middle_086907'), value: 'medium' },
  { label: translate('generatedScript.common_high_b096b3'), value: 'high' },
  { label: translate('generatedScript.common_serious_81ffc6'), value: 'critical' }
])

function riskLabel(risk: string): string {
  const map: Record<string, string> = {
    readonly: translate('generatedScript.common_read_only_ffc1d0'),
    low: translate('generatedScript.common_low_b9ee25'),
    medium: translate('generatedScript.common_middle_086907'),
    high: translate('generatedScript.common_high_b096b3'),
    critical: translate('generatedScript.common_serious_81ffc6')
  }
  return map[risk] || risk
}

function getRiskTagType(risk: string): 'info' | 'success' | 'warning' | 'danger' {
  const map: Record<string, 'info' | 'success' | 'warning' | 'danger'> = {
    readonly: 'info',
    low: 'success',
    medium: 'warning',
    high: 'danger',
    critical: 'danger'
  }
  return map[risk] || 'info'
}

async function fetchTools() {
  loading.value = true
  try {
    const params: Record<string, any> = {
      page: currentPage.value,
      page_size: pageSize.value
    }
    if (keyword.value) params.keyword = keyword.value
    if (domainFilter.value) params.domain = domainFilter.value
    if (riskFilter.value) params.risk_level = riskFilter.value
    if (whitelistFilter.value) params.whitelisted = whitelistFilter.value

    // 拦截器已解包 data，res = { tools: [...], total: N }
    const res = await getTools(params) as any
    tools.value = res.tools || res.items || []
    totalTools.value = res.total || 0
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsAssistantToolPolicySettings_failed_to_get_tool_list_26bc71'))
  } finally {
    loading.value = false
  }
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function debouncedFetch() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    currentPage.value = 1
    fetchTools()
  }, 300)
}

function handleSelectionChange(selection: any[]) {
  selectedTools.value = selection
}

async function handleWhitelistChange(row: any, whitelisted: boolean) {
  // critical 工具加入白名单需要二次确认
  if (whitelisted && row.risk_level === 'critical') {
    try {
      await ElMessageBox.confirm(
        translate('generatedScript.settingsAssistantToolPolicySettings_are_you_sure_you_want_to_b0e02d', { p0: row.tool_name }),
        translate('generatedScript.settingsAssistantToolPolicySettings_second_confirmation_06a584'),
        { confirmButtonText: translate('generatedScript.common_sure_f526c8'), cancelButtonText: translate('generatedScript.common_cancel_4d0b46'), type: 'warning' }
      )
    } catch {
      // 取消，回滚
      row.whitelisted = false
      return
    }
  }

  try {
    await updateToolWhitelist(row.tool_name, { whitelisted })
    ElMessage.success(whitelisted ? translate('generatedScript.settingsAssistantToolPolicySettings_already_added_to_whitelist_7ebb98') : translate('generatedScript.settingsAssistantToolPolicySettings_removed_from_whitelist_c79ac9'))
  } catch (e: any) {
    row.whitelisted = !whitelisted
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
  }
}

async function handleBatchWhitelist(whitelisted: boolean) {
  if (!hasSelection.value) return

  // 检查是否有 critical 工具加入白名单
  if (whitelisted) {
    const criticalTools = selectedTools.value.filter(t => t.risk_level === 'critical')
    if (criticalTools.length > 0) {
      try {
        await ElMessageBox.confirm(
          translate('generatedScript.settingsAssistantToolPolicySettings_serious_risk_tools_are_selected_are_1ce99a', { p0: criticalTools.length }),
          translate('generatedScript.settingsAssistantToolPolicySettings_second_confirmation_06a584'),
          { confirmButtonText: translate('generatedScript.common_sure_f526c8'), cancelButtonText: translate('generatedScript.common_cancel_4d0b46'), type: 'warning' }
        )
      } catch {
        return
      }
    }
  }

  try {
    const items = selectedTools.value.map(t => ({
      tool_name: t.tool_name,
      whitelisted
    }))
    await batchUpdateWhitelist({ items })
    ElMessage.success(whitelisted ? translate('generatedScript.settingsAssistantToolPolicySettings_already_added_to_whitelist_in_batches_e1cb18') : translate('generatedScript.settingsAssistantToolPolicySettings_removed_from_whitelist_in_batches_58a481'))
    await fetchTools()
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsAssistantToolPolicySettings_batch_update_failed_6d420d'))
  }
}

async function handleResetDefaults() {
  try {
    await ElMessageBox.confirm(
      translate('generatedScript.settingsAssistantToolPolicySettings_are_you_sure_you_want_to_4b2c7d'),
      translate('generatedScript.settingsAssistantToolPolicySettings_restore_default_a19193'),
      { confirmButtonText: translate('generatedScript.common_sure_f526c8'), cancelButtonText: translate('generatedScript.common_cancel_4d0b46'), type: 'warning' }
    )
  } catch {
    return
  }

  resetting.value = true
  try {
    await resetWhitelistDefaults()
    ElMessage.success(translate('generatedScript.settingsAssistantToolPolicySettings_default_whitelist_restored_464589'))
    await fetchTools()
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsAssistantToolPolicySettings_recovery_failed_76842a'))
  } finally {
    resetting.value = false
  }
}

onMounted(async () => {
  await Promise.all([fetchApprovalMode(), fetchTools()])
})
</script>

<style scoped>
.settings-hero {
  margin-bottom: 0;
}

.hero-kicker {
  display: inline-flex;
  margin-bottom: 8px;
  color: #0891b2;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.settings-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px;
}

.settings-card {
  overflow: hidden;
}

.card-disabled {
  opacity: 0.6;
  pointer-events: none;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.card-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.settings-alert {
  margin-bottom: 20px;
}

.mode-selector {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.mode-selector :deep(.el-radio-button__inner) {
  min-width: 110px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  border-radius: 999px;
  font-weight: 650;
}

.mode-selector :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  border-color: transparent;
  background: linear-gradient(135deg, #2563eb, #0891b2);
  box-shadow: 0 10px 24px rgba(37, 99, 235, 0.22);
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
  align-items: center;
}

.filter-input {
  width: 220px;
}

.filter-select {
  width: 130px;
}

.tool-table {
  margin-bottom: 16px;
}

.pagination-bar {
  display: flex;
  justify-content: flex-end;
}
</style>

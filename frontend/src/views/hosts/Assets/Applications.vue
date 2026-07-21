<template>
  <div class="applications-page">
    <el-card class="category-tabs-card">
      <el-tabs :model-value="activeTab" @tab-change="handleCategoryTabChange">
        <el-tab-pane
          v-for="tab in categoryTabs"
          :key="tab.value"
          :label="tab.label"
          :name="tab.value"
        />
      </el-tabs>
    </el-card>

    <!-- 筛选区 -->
    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="filters.keyword"
          :placeholder="$t('generated.hostsAssetsApplications_search_application_name_host_name_ip_1d7a46')"
          clearable
          style="width: 300px"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select
          v-model="filters.review_status"
          :placeholder="$t('generated.hostsAssetsApplications_review_status_976e03')"
          clearable
          style="width: 150px"
        >
          <el-option :label="$t('generated.hostsAssetsApplications_automatic_4afad8')" value="auto" />
          <el-option :label="$t('generated.common_to_be_reviewed_4607ba')" value="pending" />
          <el-option :label="$t('generated.hostsAssetsApplications_confirmed_d9fea6')" value="confirmed" />
          <el-option :label="$t('generated.hostsAssetsApplications_rejected_4c7c52')" value="rejected" />
        </el-select>

        <el-button type="primary" @click="handleSearch">
          <el-icon><Search /></el-icon>
          {{ $t('generated.common_query_711363') }}
        </el-button>

        <el-button @click="handleReset">
          <el-icon><RefreshRight /></el-icon>
          {{ $t('generated.common_reset_3d8134') }}
        </el-button>
      </div>
    </el-card>

    <!-- 数据表格 -->
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ pageTitle }}</span>
          <div class="header-actions">
            <el-button type="success" @click="handleExport" :loading="exporting">
              <el-icon><Download /></el-icon>
              {{ $t('generated.common_export_csv_7e9cc8') }}
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="applicationAssets"
        v-loading="loading"
        border
        stripe
        style="width: 100%"
      >
        <el-table-column prop="hostname" :label="$t('generated.common_host_name_823990')" width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <div>
              <div class="hostname">{{ row.hostname }}</div>
              <div class="host-id">{{ row.host_id }}</div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="ip_address" :label="$t('generated.common_ip_address_010efa')" width="140" show-overflow-tooltip />

        <el-table-column prop="group_name" :label="$t('generated.common_group_name_65731c')" width="120" show-overflow-tooltip />

        <el-table-column prop="os_type" :label="$t('generated.common_operating_system_7c3009')" width="100">
          <template #default="{ row }">
            <el-tag size="small">{{ row.os_type }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="name" :label="$t('generated.hostsAssetsApplications_application_name_2d87d5')" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="app-name">{{ row.display_name || row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="category" :label="$t('generated.common_classification_435c52')" width="120">
          <template #default="{ row }">
            <el-tag :type="getCategoryType(row.category)" size="small">
              {{ getCategoryLabel(row.category) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="$t('generated.common_label_ae0a7a')" width="120">
          <template #default="{ row }">
            <el-tag :type="row.is_container ? 'success' : 'info'" size="small" effect="plain">
              {{ applicationRuntimeLabel(row) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="version" :label="$t('generated.common_installed_version_73333c')" width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.version || 'unknown' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="related_pids" label="PID" width="130">
          <template #default="{ row }">
            <div v-if="displayPids(row).length" class="pid-list">
              <el-tag
                v-for="pid in displayPids(row).slice(0, 3)"
                :key="pid"
                size="small"
                class="pid-tag"
              >
                {{ pid }}
              </el-tag>
              <el-tag v-if="displayPids(row).length > 3" size="small" type="info">
                +{{ displayPids(row).length - 3 }}
              </el-tag>
            </div>
            <span v-else class="no-data">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="listen_ports" :label="$t('generated.hostsAssetsApplications_listening_port_7e95f4')" width="150">
          <template #default="{ row }">
            <div v-if="row.listen_ports && row.listen_ports.length > 0">
              <el-tag
                v-for="port in row.listen_ports.slice(0, 3)"
                :key="port"
                size="small"
                class="port-tag"
              >
                {{ port }}
              </el-tag>
              <el-tag v-if="row.listen_ports.length > 3" size="small" type="info">
                +{{ row.listen_ports.length - 3 }}
              </el-tag>
            </div>
            <span v-else class="no-data">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="run_user" :label="$t('generated.hostsAssetsApplications_start_user_c1334d')" width="100" show-overflow-tooltip />

        <el-table-column prop="start_path" :label="$t('generated.common_startup_path_b782b1')" width="180" show-overflow-tooltip />

        <el-table-column prop="confidence" :label="$t('generated.common_confidence_b78c2d')" width="120">
          <template #default="{ row }">
            <el-progress
              :percentage="Math.round(getConfidence(row) * 100)"
              :status="getConfidenceStatus(getConfidence(row))"
              :stroke-width="10"
            />
          </template>
        </el-table-column>

        <el-table-column prop="review_status" :label="$t('generated.common_state_62e951')" width="100">
          <template #default="{ row }">
            <el-tag :type="getReviewStatusType(row.review_status)" size="small">
              {{ getReviewStatusLabel(row.review_status) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="collected_at" :label="$t('generated.common_record_time_650b38')" width="180">
          <template #default="{ row }">
            {{ formatTime(row.collected_at) }}
          </template>
        </el-table-column>

        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewDetail(row.id)">
              {{ $t('generated.common_details_4f55ee') }}
            </el-button>
            <el-button link type="warning" @click="openReview(row)">
              {{ $t('generated.hostsAssetsApplications_review_b38c92') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="filters.page"
          v-model:page-size="filters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          :total="applicationTotal"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <!-- 详情抽屉 -->
    <el-drawer
      v-model="showDetailDrawer"
      :title="$t('generated.hostsAssetsApplications_application_details_5c1aa2')"
      size="600px"
    >
      <div v-if="selectedApp" class="app-detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('generated.hostsAssetsApplications_application_name_2d87d5')">{{ selectedApp.display_name || selectedApp.name }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_classification_435c52')">
            <el-tag :type="getCategoryType(selectedApp.category)">
              {{ getCategoryLabel(selectedApp.category) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_version_989d1a')">{{ selectedApp.version || 'unknown' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_label_ae0a7a')">
            <el-tag :type="selectedApp.is_container ? 'success' : 'info'" size="small" effect="plain">
              {{ applicationRuntimeLabel(selectedApp) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_confidence_b78c2d')">{{ Math.round(getConfidence(selectedApp) * 100) }}%</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_hostname_981e96')">{{ selectedApp.hostname }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_ip_address_010efa')">{{ selectedApp.ip_address }}</el-descriptions-item>
          <el-descriptions-item label="PID">
            <template v-if="displayPids(selectedApp).length">
              <el-tag v-for="pid in displayPids(selectedApp)" :key="pid" class="pid-tag">
                {{ pid }}
              </el-tag>
            </template>
            <span v-else>-</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsApplications_start_user_c1334d')">{{ selectedApp.run_user || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_startup_path_b782b1')">{{ selectedApp.start_path || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsApplications_listening_port_7e95f4')" :span="2">
            <el-tag v-for="port in selectedApp.listen_ports" :key="port" class="port-tag">
              {{ port }}
            </el-tag>
            <span v-if="!selectedApp.listen_ports?.length">-</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.hostsAssetsApplications_configuration_path_3902cc')" :span="2">
            <div v-if="selectedApp.config_paths?.length">
              <div v-for="path in selectedApp.config_paths" :key="path">{{ path }}</div>
            </div>
            <span v-else>-</span>
          </el-descriptions-item>
        </el-descriptions>

        <!-- AI 证据 -->
        <div class="section-title">{{ $t('generated.hostsAssetsApplications_ai_identifies_evidence_e5ef6a') }}</div>
        <div v-if="appDetail?.tool_calls?.length" class="evidence-list">
          <div v-for="call in appDetail.tool_calls" :key="call.id" class="evidence-item">
            <div class="evidence-header">
              <span class="tool-name">{{ call.tool_name }}</span>
              <el-tag :type="call.success ? 'success' : 'danger'" size="small">
                {{ call.success ? $t('common.status.success') : $t('common.status.failed') }}
              </el-tag>
            </div>
            <div class="evidence-time">{{ formatTime(call.created_at) }}</div>
          </div>
        </div>
        <el-empty v-else :description="$t('generated.hostsAssetsApplications_no_tool_call_record_yet_7987fe')" />
      </div>
    </el-drawer>

    <!-- 复核对话框 -->
    <el-dialog
      v-model="showReviewDialog"
      :title="$t('generated.hostsAssetsApplications_manual_review_2d5939')"
      width="500px"
    >
      <el-form :model="reviewForm" label-width="100px">
        <el-form-item :label="$t('generated.hostsAssetsApplications_application_name_2d87d5')">
          <el-input v-model="reviewForm.name" />
        </el-form-item>
        <el-form-item :label="$t('generated.common_classification_435c52')">
          <el-select v-model="reviewForm.category" style="width: 100%">
            <el-option :label="$t('generated.common_database_f4dbbc')" value="database" />
            <el-option :label="$t('generated.common_web_services_e3d112')" value="web_service" />
            <el-option :label="$t('generated.common_web_framework_0d07e2')" value="web_framework" />
            <el-option :label="$t('generated.common_website_671e5d')" value="web_site" />
            <el-option label="AI LLM" value="llm_service" />
            <el-option label="AI Agent" value="ai_agent" />
            <el-option label="MCP" value="mcp_server" />
            <el-option :label="$t('generated.hostsAssetsApplications_other_1a26ed')" value="other" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('generated.common_version_989d1a')">
          <el-input v-model="reviewForm.version" />
        </el-form-item>
        <el-form-item :label="$t('generated.common_installation_path_7e1561')">
          <el-input v-model="reviewForm.install_path" />
        </el-form-item>
        <el-form-item :label="$t('generated.hostsAssetsApplications_review_status_976e03')">
          <el-radio-group v-model="reviewForm.review_status">
            <el-radio label="confirmed">{{ $t('generated.hostsAssetsApplications_confirm_b56d9a') }}</el-radio>
            <el-radio label="rejected">{{ $t('generated.common_reject_03e210') }}</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showReviewDialog = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="primary" @click="submitReview" :loading="submitting">{{ $t('generated.common_sure_f526c8') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { computed, ref, reactive, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Search, RefreshRight, Download } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useAssetStore } from '@/store/assets'
import { storeToRefs } from 'pinia'
import { getApplicationDetail, reviewApplication, listApplicationAssets, type ApplicationAsset } from '@/api/assets'
import { buildCsv, downloadCsv } from '@/utils/csv'

const assetStore = useAssetStore()
const router = useRouter()
const props = defineProps<{
  defaultCategory?: string
}>()

const {
  applicationAssets,
  applicationTotal,
  loading,
  applicationFilters: filters,
} = storeToRefs(assetStore)

const showDetailDrawer = ref(false)
const showReviewDialog = ref(false)
const submitting = ref(false)
const exporting = ref(false)
const selectedApp = ref<ApplicationAsset | null>(null)
const appDetail = ref<any>(null)

const categoryTabs = computed(() => [
  { label: translate('generatedScript.hostsAssetsApplications_all_applications_cf2c50'), value: '', path: '/hosts/assets/applications' },
  { label: translate('generatedScript.common_database_f4dbbc'), value: 'database', path: '/hosts/assets/databases' },
  { label: translate('generatedScript.common_web_services_e3d112'), value: 'web_service', path: '/hosts/assets/web-services' },
  { label: translate('generatedScript.common_web_framework_0d07e2'), value: 'web_framework', path: '/hosts/assets/web-frameworks' },
  { label: translate('generatedScript.common_website_671e5d'), value: 'web_site', path: '/hosts/assets/web-sites' },
  { label: 'AI LLM', value: 'llm_service', path: '/hosts/assets/llm-services' },
  { label: 'AI Agent', value: 'ai_agent', path: '/hosts/assets/ai-agents' },
  { label: 'MCP', value: 'mcp_server', path: '/hosts/assets/mcp-servers' },
  { label: translate('generatedScript.hostsAssetsApplications_other_1a26ed'), value: 'other', path: '/hosts/assets/other-applications' },
])

const categoryTitles = computed<Record<string, string>>(() => ({
  database: translate('generatedScript.hostsAssetsApplications_database_assets_4bcda1'),
  web_service: translate('generatedScript.hostsAssetsApplications_web_service_assets_4c4456'),
  web_framework: translate('generatedScript.hostsAssetsApplications_web_framework_assets_be088e'),
  web_site: translate('generatedScript.hostsAssetsApplications_web_site_assets_f18909'),
  llm_service: translate('generatedScript.hostsAssetsApplications_ai_llm_assets_9c8404'),
  ai_agent: translate('generatedScript.hostsAssetsApplications_ai_agent_assets_a8c3ef'),
  mcp_server: translate('generatedScript.hostsAssetsApplications_mcp_assets_09f37f'),
  other: translate('generatedScript.hostsAssetsApplications_other_application_assets_90a01d'),
}))

const activeTab = computed(() => props.defaultCategory || filters.value.category || '')

const pageTitle = computed(() => {
  return categoryTitles.value[props.defaultCategory || ''] || translate('generatedScript.hostsAssetsApplications_other_1a26ed')
})

const reviewForm = reactive({
  id: '',
  name: '',
  category: '',
  version: '',
  install_path: '',
  review_status: 'confirmed',
})

watch(() => props.defaultCategory, (category) => {
  filters.value.category = category || ''
  filters.value.page = 1
  assetStore.fetchApplicationAssets()
}, { immediate: true })

function handleCategoryTabChange(name: string | number) {
  const category = String(name)
  const tab = categoryTabs.value.find(item => item.value === category)
  if (!tab) return
  router.push(tab.path)
}

// 搜索
function handleSearch() {
  filters.value.page = 1
  assetStore.fetchApplicationAssets()
}

// 重置筛选
function handleReset() {
  assetStore.resetApplicationFilters()
  filters.value.category = props.defaultCategory || ''
  assetStore.fetchApplicationAssets()
}

// 分页处理
function handleSizeChange() {
  filters.value.page = 1
  assetStore.fetchApplicationAssets()
}

function handlePageChange() {
  assetStore.fetchApplicationAssets()
}

// 导出 CSV（导出当前筛选条件下的全部应用资产，不受当前分页限制）
async function handleExport() {
  if (exporting.value) return
  exporting.value = true
  try {
    const pageSize = Math.max(applicationTotal.value, applicationAssets.value.length, 1)
    const result = await listApplicationAssets({
      ...filters.value,
      page: 1,
      page_size: pageSize,
    })
    const items = result.items || []
    if (items.length === 0) {
      ElMessage.warning(translate('generatedScript.hostsAssetsApplications_there_are_no_exportable_app_assets_9986ee'))
      return
    }

    const headers = [
      translate('generatedScript.hostsAssetsApplications_host_name_823990'), translate('generatedScript.common_host_id_62fac9'), translate('generatedScript.hostsAssetsApplications_ip_address_ae92a6'), translate('generatedScript.hostsAssetsApplications_group_name_65731c'), translate('generatedScript.hostsAssetsApplications_operating_system_7c3009'),
      translate('generatedScript.hostsAssetsApplications_application_name_2d87d5'), translate('generatedScript.hostsAssetsApplications_classification_435c52'), translate('generatedScript.hostsAssetsApplications_label_ae0a7a'), translate('generatedScript.hostsAssetsApplications_installed_version_73333c'), 'PID',
      translate('generatedScript.hostsAssetsApplications_listening_port_7e95f4'), translate('generatedScript.hostsAssetsApplications_start_user_c1334d'), translate('generatedScript.hostsAssetsApplications_startup_path_b782b1'), translate('generatedScript.hostsAssetsApplications_confidence_b78c2d'), translate('generatedScript.common_state_62e951'), translate('generatedScript.hostsAssetsApplications_record_time_650b38'),
    ]
    const rows = items.map((app: ApplicationAsset) => [
      app.hostname,
      app.host_id,
      app.ip_address,
      app.group_name,
      app.os_type,
      app.display_name || app.name,
      getCategoryLabel(app.category),
      applicationRuntimeLabel(app),
      app.version || 'unknown',
      displayPids(app).join(';'),
      (app.listen_ports || []).join(';'),
      app.run_user,
      app.start_path,
      `${Math.round(getConfidence(app) * 100)}%`,
      getReviewStatusLabel(app.review_status),
      formatTime(app.collected_at),
    ])

    const csv = buildCsv(headers, rows)
    const ts = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '')
    downloadCsv(translate('generatedScript.hostsAssetsApplications_application_assets_csv_f298d3', { p0: ts }), csv)
    ElMessage.success(translate('generatedScript.hostsAssetsApplications_application_assets_have_been_exported_f087a2', { p0: items.length }))
  } catch (error) {
    console.error(translate('generatedScript.hostsAssetsApplications_exporting_app_assets_csv_failed_a3f295'), error)
    ElMessage.error(translate('generatedScript.hostsAssetsApplications_export_to_csv_failed_79f5a6'))
  } finally {
    exporting.value = false
  }
}

// 查看详情
async function viewDetail(id: string) {
  try {
    appDetail.value = await getApplicationDetail(id)
    selectedApp.value = appDetail.value.application
    showDetailDrawer.value = true
  } catch (error) {
    ElMessage.error(translate('generatedScript.common_failed_to_get_details_b8ba3a'))
  }
}

// 打开复核对话框
function openReview(app: ApplicationAsset) {
  selectedApp.value = app
  reviewForm.id = app.id
  reviewForm.name = app.name
  reviewForm.category = app.category
  reviewForm.version = app.version
  reviewForm.install_path = ''
  reviewForm.review_status = 'confirmed'
  showReviewDialog.value = true
}

// 提交复核
async function submitReview() {
  submitting.value = true
  try {
    await reviewApplication(reviewForm.id, reviewForm)
    ElMessage.success(translate('generatedScript.hostsAssetsApplications_review_submitted_361cd5'))
    showReviewDialog.value = false
    assetStore.fetchApplicationAssets()
  } catch (error) {
    ElMessage.error(translate('generatedScript.hostsAssetsApplications_review_submission_failed_b77ca5'))
  } finally {
    submitting.value = false
  }
}

// 格式化时间
function formatTime(time: string) {
  if (!time) return '-'
  return formatDateTime(time)
}

// 获取分类类型
function getCategoryType(category: string) {
  const types: Record<string, string> = {
    database: 'warning',
    web_service: 'danger',
    web_framework: 'primary',
    web_site: 'success',
    llm_service: 'primary',
    ai_agent: 'success',
    mcp_server: 'warning',
    other: 'info',
    unknown: 'info',
  }
  return types[category] || 'info'
}

// 获取分类标签
function getCategoryLabel(category: string) {
  const labels: Record<string, string> = {
    database: translate('generatedScript.common_database_f4dbbc'),
    web_service: translate('generatedScript.common_web_services_e3d112'),
    web_framework: translate('generatedScript.common_web_framework_0d07e2'),
    web_site: translate('generatedScript.common_website_671e5d'),
    llm_service: 'AI LLM',
    ai_agent: 'AI Agent',
    mcp_server: 'MCP',
    other: translate('generatedScript.hostsAssetsApplications_other_1a26ed'),
    unknown: translate('generatedScript.common_unknown_d9c32a'),
  }
  return labels[category] || category
}

// 获取置信度状态
function getConfidenceStatus(confidence: number) {
  if (confidence >= 0.8) return 'success'
  if (confidence >= 0.5) return 'warning'
  return 'exception'
}

function getConfidence(app: ApplicationAsset) {
  return app.confidence ?? app.ai_confidence ?? 0
}

function displayPids(app: ApplicationAsset | null) {
  if (!app?.related_pids?.length) return []
  return [...new Set(app.related_pids.filter(pid => Number.isFinite(pid) && pid > 0))].sort((a, b) => a - b)
}

function applicationRuntimeLabel(app: ApplicationAsset | null) {
  if (!app?.is_container) return translate('generatedScript.hostsAssetsApplications_host_application_baeb1b')
  return translate('generatedScript.hostsAssetsApplications_container_application_023b6e')
}

// 获取复核状态类型
function getReviewStatusType(status: string) {
  const types: Record<string, string> = {
    auto: 'info',
    pending: 'warning',
    confirmed: 'success',
    rejected: 'danger',
  }
  return types[status] || 'info'
}

// 获取复核状态标签
function getReviewStatusLabel(status: string) {
  const labels: Record<string, string> = {
    auto: translate('generatedScript.hostsAssetsApplications_automatic_4afad8'),
    pending: translate('generatedScript.hostsAssetsApplications_to_be_reviewed_4607ba'),
    confirmed: translate('generatedScript.hostsAssetsApplications_confirmed_d9fea6'),
    rejected: translate('generatedScript.common_rejected_4c7c52'),
  }
  return labels[status] || status
}
</script>

<style scoped>
.applications-page {
  padding: 20px;
}

.category-tabs-card {
  margin-bottom: 12px;
}

.category-tabs-card :deep(.el-card__body) {
  padding: 0 16px;
}

.category-tabs-card :deep(.el-tabs__header) {
  margin-bottom: 0;
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

.hostname {
  font-weight: 600;
  color: #303133;
}

.host-id {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.app-name {
  font-weight: 500;
}

.port-tag {
  margin-right: 4px;
  margin-bottom: 4px;
}

.pid-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.pid-tag {
  margin-right: 4px;
  margin-bottom: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
}

.no-data {
  color: #C0C4CC;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.app-detail {
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

.evidence-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.evidence-item {
  padding: 12px;
  background: #F5F7FA;
  border-radius: 4px;
}

.evidence-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.tool-name {
  font-weight: 600;
  color: #303133;
}

.evidence-time {
  font-size: 12px;
  color: #909399;
}
</style>

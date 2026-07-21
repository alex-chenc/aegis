<template>
  <div class="assets-overview">
    <div class="overview-toolbar">
      <div>
        <h2>{{ $t('generated.hostsAssetsOverview_asset_overview_daa7c9') }}</h2>
        <div class="overview-meta">
          <span>{{ $t('generated.hostsAssetsOverview_recently_collected_014935') }} {{ formatTime(summary?.last_collection_at) }}</span>
          <span>{{ $t('generated.common_to_be_reviewed_4607ba') }} {{ summary?.needs_review_count || 0 }}</span>
          <span v-if="currentTaskStatus">
            {{ $t('generated.hostsAssetsOverview_current_task_e94d42') }}
            <el-tag size="small" :type="getTaskStatusType(currentTaskStatus)">
              {{ getTaskStatusLabel(currentTaskStatus) }}
            </el-tag>
          </span>
        </div>
      </div>
      <div class="toolbar-actions">
        <el-tooltip :content="$t('generated.hostsAssetsOverview_refresh_overview_78d03b')" placement="bottom">
          <el-button :icon="Refresh" circle :loading="loading" @click="refreshOverview" />
        </el-tooltip>
        <el-button type="primary" :loading="collecting" @click="triggerManualCollection">
          <el-icon><Refresh /></el-icon>
          {{ $t('generated.common_collect_now_abbbcf') }}
        </el-button>
        <el-button @click="showConfigDrawer = true">
          <el-icon><Setting /></el-icon>
          {{ $t('generated.hostsAssetsOverview_period_configuration_6c2ac1') }}
        </el-button>
      </div>
    </div>

    <el-row :gutter="16" class="stats-row">
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/software')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.software_count || 0 }}</div>
              <div class="stat-label">{{ $t('generated.hostsAssetsOverview_software_package_e71277') }}</div>
            </div>
            <el-icon class="stat-icon" :size="36"><Box /></el-icon>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/applications')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.application_count || 0 }}</div>
              <div class="stat-label">{{ $t('generated.hostsAssetsOverview_application_assets_aabf38') }}</div>
            </div>
            <el-icon class="stat-icon success" :size="36"><Monitor /></el-icon>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/databases')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.database_count || 0 }}</div>
              <div class="stat-label">{{ $t('generated.common_database_f4dbbc') }}</div>
            </div>
            <el-icon class="stat-icon warning" :size="36"><Coin /></el-icon>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/web-services')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.web_service_count || 0 }}</div>
              <div class="stat-label">{{ $t('generated.common_web_services_e3d112') }}</div>
            </div>
            <el-icon class="stat-icon danger" :size="36"><Connection /></el-icon>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="stats-row">
      <el-col :xs="24" :sm="12" :lg="8">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/llm-services')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.llm_service_count || 0 }}</div>
              <div class="stat-label">AI LLM</div>
            </div>
            <el-icon class="stat-icon" :size="36"><Cpu /></el-icon>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="8">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/ai-agents')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.ai_agent_count || 0 }}</div>
              <div class="stat-label">AI Agent</div>
            </div>
            <el-icon class="stat-icon success" :size="36"><Avatar /></el-icon>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="8">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/mcp-servers')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.mcp_server_count || 0 }}</div>
              <div class="stat-label">MCP</div>
            </div>
            <el-icon class="stat-icon warning" :size="36"><Connection /></el-icon>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <div class="overview-grid">
      <el-card shadow="never" class="category-panel">
        <template #header>
          <div class="panel-header">
            <span>{{ $t('generated.common_asset_classification_74af01') }}</span>
            <el-tag size="small" type="info">V5.8</el-tag>
          </div>
        </template>
        <div class="category-grid">
          <button
            v-for="item in categoryCards"
            :key="item.path"
            class="category-button"
            type="button"
            @click="navigateTo(item.path)"
          >
            <el-icon :size="28" :class="['category-icon', item.tone]">
              <component :is="item.icon" />
            </el-icon>
            <span class="category-copy">
              <span class="category-name">{{ item.name }}</span>
              <span class="category-count">{{ item.count }}</span>
            </span>
          </button>
        </div>
      </el-card>

      <el-card shadow="never" class="analysis-panel">
        <template #header>
          <div class="panel-header">
            <span>{{ $t('generated.hostsAssetsOverview_application_analysis_dbaa8d') }}</span>
            <el-tag :type="applicationAnalysisTag.type" size="small">
              {{ applicationAnalysisTag.label }}
            </el-tag>
          </div>
        </template>
        <div class="analysis-metrics">
          <div class="analysis-metric">
            <span class="metric-label">{{ $t('generated.common_web_framework_0d07e2') }}</span>
            <strong>{{ summary?.web_framework_count || 0 }}</strong>
          </div>
          <div class="analysis-metric">
            <span class="metric-label">{{ $t('generated.common_website_671e5d') }}</span>
            <strong>{{ summary?.web_site_count || 0 }}</strong>
          </div>
          <div class="analysis-metric">
            <span class="metric-label">AI LLM</span>
            <strong>{{ summary?.llm_service_count || 0 }}</strong>
          </div>
          <div class="analysis-metric">
            <span class="metric-label">AI Agent</span>
            <strong>{{ summary?.ai_agent_count || 0 }}</strong>
          </div>
          <div class="analysis-metric">
            <span class="metric-label">MCP</span>
            <strong>{{ summary?.mcp_server_count || 0 }}</strong>
          </div>
          <div class="analysis-metric">
            <span class="metric-label">{{ $t('generated.common_to_be_reviewed_4607ba') }}</span>
            <strong>{{ summary?.needs_review_count || 0 }}</strong>
          </div>
        </div>
      </el-card>
    </div>

    <el-drawer v-model="showConfigDrawer" :title="$t('generated.hostsAssetsOverview_periodic_collection_configuration_783916')" size="420px">
      <el-form :model="configForm" label-width="120px">
        <el-form-item :label="$t('generated.hostsAssetsOverview_enable_periodic_collection_7afde7')">
          <el-switch v-model="configForm.enabled" />
        </el-form-item>
        <el-form-item :label="$t('generated.hostsAssetsOverview_collection_cycle_9b415a')">
          <el-select v-model="configForm.interval_hours" style="width: 100%">
            <el-option :value="6" :label="$t('generated.hostsAssetsOverview_6_hours_43499b')" />
            <el-option :value="12" :label="$t('generated.hostsAssetsOverview_12_hours_a43b0a')" />
            <el-option :value="24" :label="$t('generated.hostsAssetsOverview_24_hours_284b07')" />
            <el-option :value="48" :label="$t('generated.hostsAssetsOverview_48_hours_af7980')" />
            <el-option :value="72" :label="$t('generated.hostsAssetsOverview_72_hours_982319')" />
            <el-option :value="168" :label="$t('generated.hostsAssetsOverview_168_hours_29ff95')" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('generated.hostsAssetsOverview_collect_content_7c7964')">
          <el-checkbox-group v-model="configForm.collect_types">
            <el-checkbox label="process" disabled>{{ $t('generated.hostsAssetsOverview_process_snapshot_cc8b8c') }}</el-checkbox>
            <el-checkbox label="software">{{ $t('generated.common_software_manifest_33aa7e') }}</el-checkbox>
            <el-checkbox label="application_analysis">{{ $t('generated.hostsAssetsOverview_ai_application_analysis_bebe4e') }}</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item :label="$t('generated.hostsAssetsOverview_next_execution_768b2d')">
          <el-input :value="formatTime(configForm.next_run_at)" disabled />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveConfig">{{ $t('generated.common_save_configuration_817af1') }}</el-button>
        </el-form-item>
      </el-form>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Box,
  Monitor,
  Coin,
  Connection,
  Cpu,
  Setting,
  Refresh,
  Grid,
  Link,
  Avatar,
} from '@element-plus/icons-vue'
import { useAssetStore } from '@/store/assets'
import { storeToRefs } from 'pinia'
import { getCollectionTask } from '@/api/assets'

const router = useRouter()
const assetStore = useAssetStore()

const {
  summary,
  collectionConfig,
  loading,
  collecting,
} = storeToRefs(assetStore)

const showConfigDrawer = ref(false)
const saving = ref(false)
const currentTaskStatus = ref('')
let taskPollTimer: ReturnType<typeof window.setTimeout> | null = null

const configForm = reactive({
  enabled: true,
  interval_hours: 12,
  collect_types: ['process', 'software', 'application_analysis'],
  scope: 'all_hosts',
  next_run_at: null as string | null,
})

const categoryCards = computed(() => [
  {
    name: translate('generatedScript.common_software_manifest_33aa7e'),
    count: summary.value?.software_count || 0,
    path: '/hosts/assets/software',
    icon: Box,
    tone: 'primary',
  },
  {
    name: translate('generatedScript.hostsAssetsOverview_application_assets_aabf38'),
    count: summary.value?.application_count || 0,
    path: '/hosts/assets/applications',
    icon: Grid,
    tone: 'success',
  },
  {
    name: translate('generatedScript.common_database_f4dbbc'),
    count: summary.value?.database_count || 0,
    path: '/hosts/assets/databases',
    icon: Coin,
    tone: 'warning',
  },
  {
    name: translate('generatedScript.common_web_services_e3d112'),
    count: summary.value?.web_service_count || 0,
    path: '/hosts/assets/web-services',
    icon: Connection,
    tone: 'danger',
  },
  {
    name: translate('generatedScript.common_web_framework_0d07e2'),
    count: summary.value?.web_framework_count || 0,
    path: '/hosts/assets/web-frameworks',
    icon: Cpu,
    tone: 'primary',
  },
  {
    name: translate('generatedScript.common_website_671e5d'),
    count: summary.value?.web_site_count || 0,
    path: '/hosts/assets/web-sites',
    icon: Link,
    tone: 'success',
  },
  {
    name: 'AI LLM',
    count: summary.value?.llm_service_count || 0,
    path: '/hosts/assets/llm-services',
    icon: Cpu,
    tone: 'primary',
  },
  {
    name: 'AI Agent',
    count: summary.value?.ai_agent_count || 0,
    path: '/hosts/assets/ai-agents',
    icon: Avatar,
    tone: 'success',
  },
  {
    name: 'MCP',
    count: summary.value?.mcp_server_count || 0,
    path: '/hosts/assets/mcp-servers',
    icon: Connection,
    tone: 'warning',
  },
])

const applicationAnalysisTag = computed(() => {
  if ((summary.value?.application_count || 0) > 0) {
    return { label: translate('generatedScript.hostsAssetsOverview_recognized_129cec'), type: 'success' as const }
  }
  if (summary.value?.last_collection_at) {
    return { label: translate('generatedScript.hostsAssetsOverview_no_results_9b7997'), type: 'warning' as const }
  }
  return { label: translate('generatedScript.hostsAssetsOverview_not_collected_72a70d'), type: 'info' as const }
})

onMounted(async () => {
  await Promise.all([
    assetStore.fetchSummary(),
    assetStore.fetchCollectionConfig(),
  ])

  if (collectionConfig.value) {
    Object.assign(configForm, collectionConfig.value)
    configForm.collect_types = normalizeConfigTypes(configForm.collect_types)
  }
})

onBeforeUnmount(() => {
  clearTaskPoll()
})

function navigateTo(path: string) {
  router.push(path)
}

async function refreshOverview() {
  await assetStore.fetchSummary()
}

async function triggerManualCollection() {
  try {
    await ElMessageBox.confirm(translate('generatedScript.common_are_you_sure_you_want_to_aabc95'), translate('generatedScript.common_confirm_b56d9a'), {
      confirmButtonText: translate('generatedScript.common_sure_f526c8'),
      cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
      type: 'info',
    })

    const task = await assetStore.triggerCollection({
      scope: 'all_hosts',
      types: ['process', 'software', 'application_analysis'],
    })

    currentTaskStatus.value = task.status
    pollCollectionTask(task.task_id)
    ElMessage.success(translate('generatedScript.hostsAssetsOverview_asset_collection_has_started_d8ba5d'))
    window.setTimeout(() => assetStore.fetchSummary(), 6000)
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(translate('generatedScript.common_trigger_acquisition_failed_3c4208'))
    }
  }
}

async function saveConfig() {
  saving.value = true
  try {
    configForm.collect_types = normalizeConfigTypes(configForm.collect_types)
    await assetStore.saveCollectionConfig(configForm)
    ElMessage.success(translate('generatedScript.hostsAssetsOverview_configuration_saved_c4e4c1'))
    showConfigDrawer.value = false
    await assetStore.fetchSummary()
  } catch (error) {
    ElMessage.error(translate('generatedScript.hostsAssetsOverview_failed_to_save_configuration_83498c'))
  } finally {
    saving.value = false
  }
}

function formatTime(time?: string | null) {
  if (!time) return '-'
  return formatDateTime(time)
}

function normalizeConfigTypes(types: string[]) {
  const next = ['process']
  if (types.includes('software')) {
    next.push('software')
  }
  if (types.includes('application_analysis')) {
    next.push('application_analysis')
  }
  return next
}

function clearTaskPoll() {
  if (taskPollTimer) {
    window.clearTimeout(taskPollTimer)
    taskPollTimer = null
  }
}

async function pollCollectionTask(taskId: string) {
  clearTaskPoll()
  const poll = async () => {
    try {
      const detail = await getCollectionTask(taskId)
      currentTaskStatus.value = detail.task.status
      if (['completed', 'failed', 'cancelled'].includes(detail.task.status)) {
        await assetStore.fetchSummary()
        clearTaskPoll()
        return
      }
      taskPollTimer = window.setTimeout(poll, 3000)
    } catch {
      clearTaskPoll()
    }
  }
  taskPollTimer = window.setTimeout(poll, 1000)
}

function getTaskStatusType(status: string) {
  const types: Record<string, string> = {
    collecting: 'warning',
    analyzing: 'primary',
    completed: 'success',
    failed: 'danger',
    cancelled: 'info',
  }
  return types[status] || 'info'
}

function getTaskStatusLabel(status: string) {
  const labels: Record<string, string> = {
    collecting: translate('generatedScript.common_collecting_b5de8d'),
    analyzing: translate('generatedScript.common_analyzing_2c2a14'),
    completed: translate('generatedScript.common_finish_33246f'),
    failed: translate('generatedScript.common_fail_3e3c80'),
    cancelled: translate('generatedScript.common_canceled_a5ffdc'),
  }
  return labels[status] || status
}
</script>

<style scoped>
.assets-overview {
  padding: 20px;
}

.overview-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 16px;
}

.overview-toolbar h2 {
  margin: 0 0 8px;
  font-size: 22px;
  font-weight: 650;
  color: #303133;
}

.overview-meta {
  display: flex;
  gap: 16px;
  color: #606266;
  font-size: 13px;
}

.toolbar-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.stats-row {
  margin-bottom: 16px;
}

.stat-card {
  cursor: pointer;
  margin-bottom: 16px;
}

.stat-card :deep(.el-card__body) {
  padding: 18px;
}

.stat-content {
  min-height: 76px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.stat-value {
  font-size: 30px;
  font-weight: 700;
  line-height: 1;
  color: #303133;
}

.stat-label {
  margin-top: 8px;
  font-size: 13px;
  color: #606266;
}

.stat-icon,
.category-icon.primary {
  color: #409eff;
}

.stat-icon.success,
.category-icon.success {
  color: #67c23a;
}

.stat-icon.warning,
.category-icon.warning {
  color: #e6a23c;
}

.stat-icon.danger,
.category-icon.danger {
  color: #f56c6c;
}

.overview-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(280px, 0.8fr);
  gap: 16px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.category-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.category-button {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 76px;
  padding: 14px;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  background: #fff;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.category-button:hover {
  border-color: #c6e2ff;
  background: #f8fbff;
}

.category-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.category-name {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.category-count {
  font-size: 18px;
  font-weight: 700;
  color: #303133;
}

.analysis-metrics {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.analysis-metric {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 48px;
  padding: 0 2px;
  border-bottom: 1px solid #ebeef5;
}

.analysis-metric:last-child {
  border-bottom: 0;
}

.metric-label {
  color: #606266;
  font-size: 13px;
}

.analysis-metric strong {
  font-size: 20px;
  color: #303133;
}

@media (max-width: 960px) {
  .overview-toolbar {
    flex-direction: column;
  }

  .toolbar-actions {
    justify-content: flex-start;
  }

  .overview-grid {
    grid-template-columns: 1fr;
  }

  .category-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .category-grid {
    grid-template-columns: 1fr;
  }
}
</style>

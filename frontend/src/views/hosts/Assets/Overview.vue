<template>
  <div class="assets-overview">
    <div class="overview-toolbar">
      <div>
        <h2>资产概览</h2>
        <div class="overview-meta">
          <span>最近采集 {{ formatTime(summary?.last_collection_at) }}</span>
          <span>待复核 {{ summary?.needs_review_count || 0 }}</span>
          <span v-if="currentTaskStatus">
            当前任务
            <el-tag size="small" :type="getTaskStatusType(currentTaskStatus)">
              {{ getTaskStatusLabel(currentTaskStatus) }}
            </el-tag>
          </span>
        </div>
      </div>
      <div class="toolbar-actions">
        <el-tooltip content="刷新概览" placement="bottom">
          <el-button :icon="Refresh" circle :loading="loading" @click="refreshOverview" />
        </el-tooltip>
        <el-button type="primary" :loading="collecting" @click="triggerManualCollection">
          <el-icon><Refresh /></el-icon>
          立即采集
        </el-button>
        <el-button @click="showConfigDrawer = true">
          <el-icon><Setting /></el-icon>
          周期配置
        </el-button>
      </div>
    </div>

    <el-row :gutter="16" class="stats-row">
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover" class="stat-card" @click="navigateTo('/hosts/assets/software')">
          <div class="stat-content">
            <div>
              <div class="stat-value">{{ summary?.software_count || 0 }}</div>
              <div class="stat-label">软件包</div>
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
              <div class="stat-label">应用资产</div>
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
              <div class="stat-label">数据库</div>
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
              <div class="stat-label">Web 服务</div>
            </div>
            <el-icon class="stat-icon danger" :size="36"><Connection /></el-icon>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <div class="overview-grid">
      <el-card shadow="never" class="category-panel">
        <template #header>
          <div class="panel-header">
            <span>资产分类</span>
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
            <span>应用分析</span>
            <el-tag :type="applicationAnalysisTag.type" size="small">
              {{ applicationAnalysisTag.label }}
            </el-tag>
          </div>
        </template>
        <div class="analysis-metrics">
          <div class="analysis-metric">
            <span class="metric-label">Web 框架</span>
            <strong>{{ summary?.web_framework_count || 0 }}</strong>
          </div>
          <div class="analysis-metric">
            <span class="metric-label">Web 站点</span>
            <strong>{{ summary?.web_site_count || 0 }}</strong>
          </div>
          <div class="analysis-metric">
            <span class="metric-label">待复核</span>
            <strong>{{ summary?.needs_review_count || 0 }}</strong>
          </div>
        </div>
      </el-card>
    </div>

    <el-drawer v-model="showConfigDrawer" title="周期采集配置" size="420px">
      <el-form :model="configForm" label-width="120px">
        <el-form-item label="启用周期采集">
          <el-switch v-model="configForm.enabled" />
        </el-form-item>
        <el-form-item label="采集周期">
          <el-select v-model="configForm.interval_hours" style="width: 100%">
            <el-option :value="6" label="6 小时" />
            <el-option :value="12" label="12 小时" />
            <el-option :value="24" label="24 小时" />
            <el-option :value="48" label="48 小时" />
            <el-option :value="72" label="72 小时" />
            <el-option :value="168" label="168 小时" />
          </el-select>
        </el-form-item>
        <el-form-item label="采集内容">
          <el-checkbox-group v-model="configForm.collect_types">
            <el-checkbox label="process" disabled>进程快照</el-checkbox>
            <el-checkbox label="application_analysis">AI 应用分析</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item label="下一次执行">
          <el-input :value="formatTime(configForm.next_run_at)" disabled />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveConfig">保存配置</el-button>
        </el-form-item>
      </el-form>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
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
  collect_types: ['process', 'application_analysis'],
  scope: 'all_hosts',
  next_run_at: null as string | null,
})

const categoryCards = computed(() => [
  {
    name: '软件清单',
    count: summary.value?.software_count || 0,
    path: '/hosts/assets/software',
    icon: Box,
    tone: 'primary',
  },
  {
    name: '应用资产',
    count: summary.value?.application_count || 0,
    path: '/hosts/assets/applications',
    icon: Grid,
    tone: 'success',
  },
  {
    name: '数据库',
    count: summary.value?.database_count || 0,
    path: '/hosts/assets/databases',
    icon: Coin,
    tone: 'warning',
  },
  {
    name: 'Web 服务',
    count: summary.value?.web_service_count || 0,
    path: '/hosts/assets/web-services',
    icon: Connection,
    tone: 'danger',
  },
  {
    name: 'Web 框架',
    count: summary.value?.web_framework_count || 0,
    path: '/hosts/assets/web-frameworks',
    icon: Cpu,
    tone: 'primary',
  },
  {
    name: 'Web 站点',
    count: summary.value?.web_site_count || 0,
    path: '/hosts/assets/web-sites',
    icon: Link,
    tone: 'success',
  },
])

const applicationAnalysisTag = computed(() => {
  if ((summary.value?.application_count || 0) > 0) {
    return { label: '已识别', type: 'success' as const }
  }
  if (summary.value?.last_collection_at) {
    return { label: '无结果', type: 'warning' as const }
  }
  return { label: '未采集', type: 'info' as const }
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
    await ElMessageBox.confirm('确定要立即执行资产采集吗？', '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'info',
    })

    const task = await assetStore.triggerCollection({
      scope: 'all_hosts',
      types: ['process', 'application_analysis'],
    })

    currentTaskStatus.value = task.status
    pollCollectionTask(task.task_id)
    ElMessage.success('资产采集已开始')
    window.setTimeout(() => assetStore.fetchSummary(), 6000)
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('触发采集失败')
    }
  }
}

async function saveConfig() {
  saving.value = true
  try {
    configForm.collect_types = normalizeConfigTypes(configForm.collect_types)
    await assetStore.saveCollectionConfig(configForm)
    ElMessage.success('配置已保存')
    showConfigDrawer.value = false
    await assetStore.fetchSummary()
  } catch (error) {
    ElMessage.error('保存配置失败')
  } finally {
    saving.value = false
  }
}

function formatTime(time?: string | null) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

function normalizeConfigTypes(types: string[]) {
  const next = ['process']
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
    collecting: '采集中',
    analyzing: '分析中',
    completed: '完成',
    failed: '失败',
    cancelled: '已取消',
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

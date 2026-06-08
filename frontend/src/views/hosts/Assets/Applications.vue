<template>
  <div class="applications-page">
    <!-- 筛选区 -->
    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="filters.keyword"
          placeholder="搜索应用名、主机名、IP"
          clearable
          style="width: 300px"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select
          v-model="filters.category"
          placeholder="应用分类"
          clearable
          style="width: 150px"
        >
          <el-option label="数据库" value="database" />
          <el-option label="Web 服务" value="web_service" />
          <el-option label="Web 框架" value="web_framework" />
          <el-option label="Web 站点" value="web_site" />
          <el-option label="AI LLM" value="llm_service" />
          <el-option label="AI Agent" value="ai_agent" />
          <el-option label="MCP" value="mcp_server" />
          <el-option label="其他" value="other" />
        </el-select>

        <el-select
          v-model="filters.review_status"
          placeholder="复核状态"
          clearable
          style="width: 150px"
        >
          <el-option label="自动" value="auto" />
          <el-option label="待复核" value="pending" />
          <el-option label="已确认" value="confirmed" />
          <el-option label="已拒绝" value="rejected" />
        </el-select>

        <el-button type="primary" @click="handleSearch">
          <el-icon><Search /></el-icon>
          查询
        </el-button>

        <el-button @click="handleReset">
          <el-icon><RefreshRight /></el-icon>
          重置
        </el-button>
      </div>
    </el-card>

    <!-- 数据表格 -->
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ pageTitle }}</span>
          <div class="header-actions">
            <el-button type="success" @click="handleExport">
              <el-icon><Download /></el-icon>
              导出 CSV
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
        <el-table-column prop="hostname" label="主机名称" width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <div>
              <div class="hostname">{{ row.hostname }}</div>
              <div class="host-id">{{ row.host_id }}</div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="ip_address" label="IP 地址" width="140" show-overflow-tooltip />

        <el-table-column prop="group_name" label="分组名称" width="120" show-overflow-tooltip />

        <el-table-column prop="os_type" label="操作系统" width="100">
          <template #default="{ row }">
            <el-tag size="small">{{ row.os_type }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="name" label="应用名称" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="app-name">{{ row.display_name || row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="category" label="分类" width="120">
          <template #default="{ row }">
            <el-tag :type="getCategoryType(row.category)" size="small">
              {{ getCategoryLabel(row.category) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="version" label="安装版本" width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.version || 'unknown' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="listen_ports" label="监听端口" width="150">
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

        <el-table-column prop="run_user" label="启动用户" width="100" show-overflow-tooltip />

        <el-table-column prop="start_path" label="启动路径" width="180" show-overflow-tooltip />

        <el-table-column prop="confidence" label="置信度" width="120">
          <template #default="{ row }">
            <el-progress
              :percentage="Math.round(getConfidence(row) * 100)"
              :status="getConfidenceStatus(getConfidence(row))"
              :stroke-width="10"
            />
          </template>
        </el-table-column>

        <el-table-column prop="review_status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getReviewStatusType(row.review_status)" size="small">
              {{ getReviewStatusLabel(row.review_status) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="collected_at" label="记录时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.collected_at) }}
          </template>
        </el-table-column>

        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewDetail(row.id)">
              详情
            </el-button>
            <el-button link type="warning" @click="openReview(row)">
              复核
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
      title="应用详情"
      size="600px"
    >
      <div v-if="selectedApp" class="app-detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="应用名称">{{ selectedApp.display_name || selectedApp.name }}</el-descriptions-item>
          <el-descriptions-item label="分类">
            <el-tag :type="getCategoryType(selectedApp.category)">
              {{ getCategoryLabel(selectedApp.category) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="版本">{{ selectedApp.version || 'unknown' }}</el-descriptions-item>
          <el-descriptions-item label="置信度">{{ Math.round(getConfidence(selectedApp) * 100) }}%</el-descriptions-item>
          <el-descriptions-item label="主机名">{{ selectedApp.hostname }}</el-descriptions-item>
          <el-descriptions-item label="IP 地址">{{ selectedApp.ip_address }}</el-descriptions-item>
          <el-descriptions-item label="启动用户">{{ selectedApp.run_user || '-' }}</el-descriptions-item>
          <el-descriptions-item label="启动路径">{{ selectedApp.start_path || '-' }}</el-descriptions-item>
          <el-descriptions-item label="监听端口" :span="2">
            <el-tag v-for="port in selectedApp.listen_ports" :key="port" class="port-tag">
              {{ port }}
            </el-tag>
            <span v-if="!selectedApp.listen_ports?.length">-</span>
          </el-descriptions-item>
          <el-descriptions-item label="配置路径" :span="2">
            <div v-if="selectedApp.config_paths?.length">
              <div v-for="path in selectedApp.config_paths" :key="path">{{ path }}</div>
            </div>
            <span v-else>-</span>
          </el-descriptions-item>
        </el-descriptions>

        <!-- AI 证据 -->
        <div class="section-title">AI 识别证据</div>
        <div v-if="appDetail?.tool_calls?.length" class="evidence-list">
          <div v-for="call in appDetail.tool_calls" :key="call.id" class="evidence-item">
            <div class="evidence-header">
              <span class="tool-name">{{ call.tool_name }}</span>
              <el-tag :type="call.success ? 'success' : 'danger'" size="small">
                {{ call.success ? '成功' : '失败' }}
              </el-tag>
            </div>
            <div class="evidence-time">{{ formatTime(call.created_at) }}</div>
          </div>
        </div>
        <el-empty v-else description="暂无工具调用记录" />
      </div>
    </el-drawer>

    <!-- 复核对话框 -->
    <el-dialog
      v-model="showReviewDialog"
      title="人工复核"
      width="500px"
    >
      <el-form :model="reviewForm" label-width="100px">
        <el-form-item label="应用名称">
          <el-input v-model="reviewForm.name" />
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="reviewForm.category" style="width: 100%">
            <el-option label="数据库" value="database" />
            <el-option label="Web 服务" value="web_service" />
            <el-option label="Web 框架" value="web_framework" />
            <el-option label="Web 站点" value="web_site" />
            <el-option label="AI LLM" value="llm_service" />
            <el-option label="AI Agent" value="ai_agent" />
            <el-option label="MCP" value="mcp_server" />
            <el-option label="其他" value="other" />
          </el-select>
        </el-form-item>
        <el-form-item label="版本">
          <el-input v-model="reviewForm.version" />
        </el-form-item>
        <el-form-item label="安装路径">
          <el-input v-model="reviewForm.install_path" />
        </el-form-item>
        <el-form-item label="复核状态">
          <el-radio-group v-model="reviewForm.review_status">
            <el-radio label="confirmed">确认</el-radio>
            <el-radio label="rejected">拒绝</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showReviewDialog = false">取消</el-button>
        <el-button type="primary" @click="submitReview" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch } from 'vue'
import { Search, RefreshRight, Download } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useAssetStore } from '@/store/assets'
import { storeToRefs } from 'pinia'
import { getApplicationDetail, reviewApplication, type ApplicationAsset } from '@/api/assets'

const assetStore = useAssetStore()
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
const selectedApp = ref<ApplicationAsset | null>(null)
const appDetail = ref<any>(null)

const categoryTitles: Record<string, string> = {
  database: '数据库资产',
  web_service: 'Web 服务资产',
  web_framework: 'Web 框架资产',
  web_site: 'Web 站点资产',
  llm_service: 'AI LLM 资产',
  ai_agent: 'AI Agent 资产',
  mcp_server: 'MCP 资产',
}

const pageTitle = computed(() => {
  return categoryTitles[props.defaultCategory || ''] || '应用资产'
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

// 导出 CSV
function handleExport() {
  // TODO: 实现 CSV 导出
}

// 查看详情
async function viewDetail(id: string) {
  try {
    appDetail.value = await getApplicationDetail(id)
    selectedApp.value = appDetail.value.application
    showDetailDrawer.value = true
  } catch (error) {
    ElMessage.error('获取详情失败')
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
    ElMessage.success('复核已提交')
    showReviewDialog.value = false
    assetStore.fetchApplicationAssets()
  } catch (error) {
    ElMessage.error('复核提交失败')
  } finally {
    submitting.value = false
  }
}

// 格式化时间
function formatTime(time: string) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
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
    database: '数据库',
    web_service: 'Web 服务',
    web_framework: 'Web 框架',
    web_site: 'Web 站点',
    llm_service: 'AI LLM',
    ai_agent: 'AI Agent',
    mcp_server: 'MCP',
    other: '其他',
    unknown: '未知',
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
    auto: '自动',
    pending: '待复核',
    confirmed: '已确认',
    rejected: '已拒绝',
  }
  return labels[status] || status
}
</script>

<style scoped>
.applications-page {
  padding: 20px;
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

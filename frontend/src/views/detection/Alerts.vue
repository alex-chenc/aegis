<template>
  <div class="detection-alerts-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-select v-model="severity" placeholder="严重程度" clearable class="filter-item">
          <el-option label="严重" value="critical" />
          <el-option label="高危" value="high" />
          <el-option label="中危" value="medium" />
          <el-option label="低危" value="low" />
        </el-select>
        <el-select v-model="status" placeholder="状态" clearable class="filter-item">
          <el-option label="待处置" value="pending" />
          <el-option label="已处置" value="resolved" />
        </el-select>
        <el-select v-model="judgmentSource" placeholder="判定来源" clearable class="filter-item">
          <el-option label="系统判定" value="system" />
          <el-option label="AI判定" value="ai" />
        </el-select>
        <el-input v-model="query" placeholder="搜索主机或MITRE" clearable class="search-input">
          <template #append>
            <el-button :icon="Search" @click="loadAlerts" />
          </template>
        </el-input>
        <el-button type="primary" @click="loadAlerts">查询</el-button>
        <el-button type="danger" :disabled="selectedAlerts.length === 0" @click="handleBatchDelete">
          批量删除 ({{ selectedAlerts.length }})
        </el-button>
      </div>
    </el-card>

    <el-card>
      <el-table v-loading="alertLoading" :data="alerts" border stripe @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="rule_title" label="规则名称" min-width="180">
          <template #default="{ row }">
            {{ row.rule_title || row.mitre_id || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="hostname" label="主机" min-width="150" />
        <el-table-column label="MITRE" width="120">
          <template #default="{ row }">
            <el-link type="primary" @click="goToRules(row.mitre_id)">{{ row.mitre_id || '-' }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="severity" label="严重程度" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="severityTagType(row.severity)">{{ severityLabel(row.severity) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="hit_count" label="命中次数" width="90" align="center" />
        <el-table-column prop="judgment_source" label="判定来源" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.judgment_source === 'ai' ? 'warning' : 'info'" size="small">
              {{ judgmentSourceLabel(row.judgment_source) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status, row.block_status)">
              {{ statusLabel(row.status, row.block_status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="last_seen_at" label="最近命中" width="160">
          <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" @click="showDetail(row)">详情</el-button>
            <el-button size="small" type="success" :disabled="row.status !== 'pending'" @click="handleResolve(row)">
              处置
            </el-button>
            <el-button size="small" type="danger" :disabled="row.status !== 'pending' || row.block_status === 'success'" @click="showBlockDialog(row)">
              阻断
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

    <el-dialog v-model="detailVisible" title="告警详情" width="900px">
      <el-descriptions v-if="selectedAlert" :column="2" border>
        <el-descriptions-item label="规则名称" :span="2">{{ selectedAlert.rule_title || selectedAlert.mitre_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="主机">{{ selectedAlert.hostname || selectedAlert.host_id }}</el-descriptions-item>
        <el-descriptions-item label="MITRE ID">
          <el-link type="primary" @click="goToRules(selectedAlert.mitre_id)">{{ selectedAlert.mitre_id }}</el-link>
        </el-descriptions-item>
        <el-descriptions-item label="进程PID">{{ selectedAlert.pid }}</el-descriptions-item>
        <el-descriptions-item label="判定来源">{{ judgmentSourceLabel(selectedAlert.judgment_source) }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ statusLabel(selectedAlert.status, selectedAlert.block_status) }}</el-descriptions-item>
        <el-descriptions-item v-if="selectedAlert.block_status" label="阻断状态">
          <el-tag :type="selectedAlert.block_status === 'success' ? 'success' : selectedAlert.block_status === 'failed' ? 'danger' : 'warning'">
            {{ selectedAlert.block_status === 'success' ? '阻断成功' : selectedAlert.block_status === 'failed' ? '阻断失败' : '阻断中' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item v-if="selectedAlert.block_message" label="阻断结果" :span="2">
          <el-text :type="selectedAlert.block_status === 'failed' ? 'danger' : 'success'">{{ selectedAlert.block_message }}</el-text>
        </el-descriptions-item>
        <el-descriptions-item label="命中次数">{{ selectedAlert.hit_count }}</el-descriptions-item>
        <el-descriptions-item label="首次发现">{{ formatTime(selectedAlert.first_seen_at) }}</el-descriptions-item>
        <el-descriptions-item label="最近发现">{{ formatTime(selectedAlert.last_seen_at) }}</el-descriptions-item>
        <el-descriptions-item label="LLM摘要" :span="2">
          {{ selectedAlert.llm_summary || '等待AI分析' }}
        </el-descriptions-item>
        <el-descriptions-item v-if="selectedAlert.llm_disposal_strategy" label="处置策略" :span="2">
          {{ selectedAlert.llm_disposal_strategy }}
        </el-descriptions-item>
        <el-descriptions-item label="描述" :span="2">{{ selectedAlert.description || '-' }}</el-descriptions-item>
      </el-descriptions>
      <div v-if="selectedAlert" class="process-tree-section">
        <h4>进程树</h4>
        <ProcessTree :process-tree="selectedAlert.process_tree" />
      </div>
    </el-dialog>

    <el-dialog v-model="blockDialogVisible" title="选择阻断动作" width="400px">
      <el-form label-width="100px">
        <el-form-item label="阻断动作">
          <el-select v-model="blockAction" placeholder="请选择">
            <el-option label="终止进程" value="kill_process" />
            <el-option label="隔离文件" value="quarantine_file" />
            <el-option label="阻断网络" value="block_connection" />
            <el-option label="禁用用户" value="disable_user" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="blockDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmBlock">确认阻断</el-button>
      </template>
    </el-dialog>

    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { useDetectionStore } from '@/store/detection'
import * as api from '@/api/detection'
import type { Alert } from '@/types'
import { SeverityLabels, AlertStatusLabels, BlockStatusLabels, JudgmentSourceLabels } from '@/types'
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
  return new Date(time).toLocaleString('zh-CN')
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
  return SeverityLabels[level] || level
}

function statusTagType(status: string, blockStatus?: string) {
  if (blockStatus === 'failed') return 'danger'
  if (blockStatus === 'success') return 'success'
  if (blockStatus === 'blocking') return 'warning'
  if (status === 'resolved') return 'success'
  return 'warning'
}

function statusLabel(status: string, blockStatus?: string) {
  if (blockStatus) return BlockStatusLabels[blockStatus] || blockStatus
  return AlertStatusLabels[status] || status
}

function judgmentSourceLabel(source: string) {
  return JudgmentSourceLabels[source] || source
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
  ElMessage.success('告警已处置')
  loadAlerts()
}

function showBlockDialog(row: Alert) {
  blockTargetAlert.value = row
  blockAction.value = 'kill_process'
  blockDialogVisible.value = true
}

async function confirmBlock() {
  if (!blockTargetAlert.value) return
  await api.blockAlert(blockTargetAlert.value.alert_id, blockAction.value)
  ElMessage.success('阻断指令已下发')
  blockDialogVisible.value = false
  loadAlerts()
}

async function handleBatchDelete() {
  if (selectedAlerts.value.length === 0) {
    ElMessage.warning('请先选择要删除的告警')
    return
  }
  
  try {
    const alertIds = selectedAlerts.value.map(a => a.alert_id)
    await api.deleteAlerts(alertIds)
    ElMessage.success(`已删除 ${alertIds.length} 条告警`)
    selectedAlerts.value = []
    loadAlerts()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '删除失败')
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

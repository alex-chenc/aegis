<template>
  <div class="detection-alerts-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-select v-model="severity" placeholder="严重程度" clearable class="filter-item">
          <el-option label="Critical" value="Critical" />
          <el-option label="High" value="High" />
          <el-option label="Medium" value="Medium" />
          <el-option label="Low" value="Low" />
        </el-select>
        <el-select v-model="status" placeholder="状态" clearable class="filter-item">
          <el-option label="active" value="active" />
          <el-option label="resolved" value="resolved" />
        </el-select>
        <el-input v-model="query" placeholder="搜索主机或MITRE" clearable class="search-input">
          <template #append>
            <el-button :icon="Search" @click="loadAlerts" />
          </template>
        </el-input>
        <el-button type="primary" @click="loadAlerts">查询</el-button>
      </div>
    </el-card>

    <el-card>
      <el-table v-loading="alertLoading" :data="alerts" border stripe>
        <el-table-column prop="alert_id" label="告警ID" min-width="180" />
        <el-table-column prop="hostname" label="主机" min-width="150" />
        <el-table-column prop="mitre_id" label="MITRE" width="120" />
        <el-table-column prop="severity" label="严重程度" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="severityTagType(row.severity)">{{ row.severity }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="hit_count" label="命中次数" width="100" align="center" />
        <el-table-column prop="status" label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'danger' : 'success'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="last_seen_at" label="最近命中" width="180">
          <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" @click="showDetail(row)">详情</el-button>
            <el-button size="small" type="success" :disabled="row.status === 'resolved'" @click="handleResolve(row)">
              处置
            </el-button>
            <el-button size="small" type="warning" :disabled="row.manual_blocked" @click="handleBlock(row)">
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

    <el-dialog v-model="detailVisible" title="告警详情" width="760px">
      <el-descriptions v-if="selectedAlert" :column="2" border>
        <el-descriptions-item label="告警ID">{{ selectedAlert.alert_id }}</el-descriptions-item>
        <el-descriptions-item label="主机">{{ selectedAlert.hostname || selectedAlert.host_id }}</el-descriptions-item>
        <el-descriptions-item label="MITRE">{{ selectedAlert.mitre_id }}</el-descriptions-item>
        <el-descriptions-item label="威胁名称">{{ selectedAlert.mitre_name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="进程PID">{{ selectedAlert.pid }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ selectedAlert.status }}</el-descriptions-item>
        <el-descriptions-item label="首次发现">{{ formatTime(selectedAlert.first_seen_at) }}</el-descriptions-item>
        <el-descriptions-item label="最近发现">{{ formatTime(selectedAlert.last_seen_at) }}</el-descriptions-item>
        <el-descriptions-item label="LLM摘要" :span="2">{{ selectedAlert.llm_summary || '-' }}</el-descriptions-item>
        <el-descriptions-item label="描述" :span="2">{{ selectedAlert.description || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { useDetectionStore } from '@/store/detection'
import * as api from '@/api/detection'
import type { Alert } from '@/types'

const store = useDetectionStore()

const severity = ref('')
const status = ref('')
const query = ref('')
const page = ref(1)
const pageSize = ref(10)
const detailVisible = ref(false)
const selectedAlert = ref<Alert | null>(null)

const alerts = computed(() => store.alerts)
const alertTotal = computed(() => store.alertTotal)
const alertLoading = computed(() => store.alertLoading)

function formatTime(time: string) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

function severityTagType(level: string) {
  if (level === 'Critical') return 'danger'
  if (level === 'High') return 'warning'
  if (level === 'Medium') return 'info'
  return 'success'
}

async function loadAlerts() {
  await store.fetchAlerts({
    page: page.value,
    page_size: pageSize.value,
    severity: severity.value || undefined,
    status: status.value || undefined,
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

async function handleBlock(row: Alert) {
  await api.blockAlert(row.alert_id)
  ElMessage.success('阻断指令已下发')
  loadAlerts()
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
</style>

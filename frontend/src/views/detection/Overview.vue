<template>
  <div class="detection-overview-page">
    <el-row :gutter="16" class="stats-row">
      <el-col :span="6">
        <el-card shadow="hover">
          <el-statistic title="今日告警" :value="threatStats?.today_alerts || 0" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <el-statistic title="今日阻断" :value="threatStats?.today_blocks || 0" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <el-statistic title="受影响主机" :value="threatStats?.affected_hosts || 0" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <el-statistic title="生效规则" :value="threatStats?.active_rules || 0" />
        </el-card>
      </el-col>
    </el-row>

    <el-card class="trend-card">
      <template #header>
        <div class="card-header">
          <span>告警趋势（近24小时）</span>
          <el-button size="small" @click="refreshData">刷新</el-button>
        </div>
      </template>
      <el-table :data="alertTrend" border stripe size="small" empty-text="暂无趋势数据">
        <el-table-column prop="time_bucket" label="时间" min-width="180" />
        <el-table-column prop="count" label="告警数" width="120" align="center" />
      </el-table>
    </el-card>

    <el-card class="matrix-card">
      <template #header>
        <span>MITRE ATT&CK 战术矩阵（14战术）</span>
      </template>
      <el-row :gutter="12">
        <el-col v-for="t in tactics" :key="t.key" :xs="24" :sm="12" :md="8" :lg="6" class="matrix-col">
          <el-card shadow="never" class="tactic-card">
            <div class="tactic-title">{{ t.label }}</div>
            <el-tag type="info">{{ getTacticHitCount(t.key) }} 告警</el-tag>
          </el-card>
        </el-col>
      </el-row>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useDetectionStore } from '@/store/detection'

const store = useDetectionStore()

const tactics = [
  { key: 'TA0001', label: '初始访问' },
  { key: 'TA0002', label: '执行' },
  { key: 'TA0003', label: '持久化' },
  { key: 'TA0004', label: '权限提升' },
  { key: 'TA0005', label: '防御绕过' },
  { key: 'TA0006', label: '凭证访问' },
  { key: 'TA0007', label: '发现' },
  { key: 'TA0008', label: '横向移动' },
  { key: 'TA0009', label: '数据收集' },
  { key: 'TA0010', label: '命令与控制' },
  { key: 'TA0011', label: '数据窃取' },
  { key: 'TA0040', label: '影响' },
  { key: 'TA0042', label: '资源开发' },
  { key: 'TA0043', label: '侦察' }
]

const threatStats = computed(() => store.threatStats)
const alertTrend = computed(() => store.alertTrend)
const alerts = computed(() => store.alerts)

function getTacticHitCount(tacticId: string) {
  return alerts.value.filter(a => (a.mitre_id || '').startsWith(tacticId)).length
}

async function refreshData() {
  await Promise.all([
    store.fetchThreatStatistics(),
    store.fetchAlertTrend(24),
    store.fetchAlerts({ page: 1, page_size: 200 })
  ])
}

onMounted(() => {
  refreshData()
})
</script>

<style scoped>
.detection-overview-page {
  padding: 20px;
}

.stats-row {
  margin-bottom: 16px;
}

.trend-card {
  margin-bottom: 16px;
}

.matrix-col {
  margin-bottom: 12px;
}

.tactic-card {
  min-height: 92px;
}

.tactic-title {
  margin-bottom: 10px;
  color: #303133;
  font-weight: 500;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

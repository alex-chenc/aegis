<template>
  <div class="detection-overview-page">
    <el-row :gutter="16" class="stats-row">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card clickable" @click="goToAlerts()">
          <el-statistic :title="$t('generated.detectionOverview_alarm_today_fe59ff')" :value="threatStats?.today_alerts || 0" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <el-statistic :title="$t('generated.detectionOverview_block_today_b0054f')" :value="threatStats?.today_blocks || 0" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <el-statistic :title="$t('generated.detectionOverview_affected_hosts_546286')" :value="threatStats?.affected_hosts || 0" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <el-statistic :title="$t('generated.detectionOverview_effective_rules_f03848')" :value="threatStats?.active_rules || 0" />
        </el-card>
      </el-col>
    </el-row>

    <el-card class="trend-card">
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.detectionOverview_alarm_trend_last_24_hours_5aba66') }}</span>
          <el-button size="small" @click="refreshData">{{ $t('generated.common_refresh_38108e') }}</el-button>
        </div>
      </template>
      <div ref="chartRef" class="trend-chart"></div>
    </el-card>

    <el-card class="matrix-card">
      <template #header>
        <span>{{ $t('generated.detectionOverview_miter_att_ck_tactical_matrix_14_3448b4') }}</span>
      </template>
      <el-row :gutter="12">
        <el-col v-for="tactic in attackMatrix?.tactics || []" :key="tactic.id" :xs="24" :sm="12" :md="8" :lg="6" class="matrix-col">
          <el-card shadow="never" class="tactic-card clickable" @click="goToAlertsByMitre(tactic.id)">
            <div class="tactic-title">{{ tactic.name }}</div>
            <div class="tactic-techniques">
              <span v-for="tech in tactic.techniques.slice(0, 3)" :key="tech.id" class="tech-tag">
                {{ tech.name }} ({{ tech.alert_count }})
              </span>
              <span v-if="tactic.techniques.length > 3" class="tech-more">
                +{{ tactic.techniques.length - 3 }} {{ $t('generated.common_more_9b0c6c') }}
              </span>
            </div>
          </el-card>
        </el-col>
      </el-row>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useDetectionStore } from '@/store/detection'
import * as echarts from 'echarts'

const router = useRouter()
const store = useDetectionStore()
const chartRef = ref<HTMLElement | null>(null)
let chartInstance: echarts.ECharts | null = null

const threatStats = computed(() => store.threatStats)
const alertTrend = computed(() => store.alertTrend)
const attackMatrix = computed(() => store.attackMatrix)

function goToAlerts() {
  router.push('/detection/alerts')
}

function goToAlertsByMitre(mitreId: string) {
  router.push({ path: '/detection/alerts', query: { mitre_id: mitreId } })
}

function initChart() {
  if (!chartRef.value) return
  
  if (chartInstance) {
    chartInstance.dispose()
  }
  
  chartInstance = echarts.init(chartRef.value)
  
  const times = alertTrend.value.map((item: any) => {
    const date = new Date(item.time_bucket)
    return `${date.getHours()}:00`
  })
  const counts = alertTrend.value.map((item: any) => item.count)
  
  const option = {
    tooltip: {
      trigger: 'axis'
    },
    xAxis: {
      type: 'category',
      data: times,
      axisLabel: {
        rotate: 45
      }
    },
    yAxis: {
      type: 'value',
      minInterval: 1
    },
    series: [{
      data: counts,
      type: 'line',
      smooth: true,
      areaStyle: {
        opacity: 0.3
      },
      lineStyle: {
        width: 2
      },
      itemStyle: {
        color: '#409EFF'
      }
    }]
  }
  
  chartInstance.setOption(option)
}

async function refreshData() {
  await Promise.all([
    store.fetchThreatStatistics(),
    store.fetchAlertTrend(24),
    store.fetchAttackMatrix()
  ])
  await nextTick()
  initChart()
}

onMounted(() => {
  refreshData()
  window.addEventListener('resize', () => {
    chartInstance?.resize()
  })
})

watch(alertTrend, () => {
  nextTick(() => initChart())
})
</script>

<style scoped>
.detection-overview-page {
  padding: 20px;
}

.stats-row {
  margin-bottom: 16px;
}

.stat-card {
  text-align: center;
}

.stat-card.clickable {
  cursor: pointer;
}

.stat-card.clickable:hover {
  transform: translateY(-2px);
  transition: transform 0.2s;
}

.trend-card {
  margin-bottom: 16px;
}

.trend-chart {
  width: 100%;
  height: 300px;
}

.matrix-col {
  margin-bottom: 12px;
}

.tactic-card {
  min-height: 120px;
  cursor: pointer;
}

.tactic-card:hover {
  border-color: #409EFF;
}

.tactic-title {
  margin-bottom: 10px;
  color: #303133;
  font-weight: 500;
  font-size: 14px;
}

.tactic-techniques {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.tech-tag {
  font-size: 12px;
  color: #606266;
  background: #f4f4f5;
  padding: 2px 6px;
  border-radius: 3px;
}

.tech-more {
  font-size: 12px;
  color: #909399;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

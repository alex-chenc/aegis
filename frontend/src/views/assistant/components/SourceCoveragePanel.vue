<template>
  <div class="source-coverage">
    <div class="section-header">
      <span class="section-title">数据源覆盖</span>
      <span v-if="coverage" class="coverage-ratio">
        {{ coverage.available_sources }}/{{ coverage.total_sources }} 可用
      </span>
    </div>

    <div v-if="!coverage" class="empty-hint">
      暂无覆盖数据
    </div>

    <div v-else class="source-list">
      <div
        v-for="source in coverage.sources"
        :key="source.source_type"
        class="source-item"
        :class="{ unavailable: !source.available }"
      >
        <div class="source-status">
          <el-icon v-if="source.available" class="status-icon available"><CircleCheck /></el-icon>
          <el-icon v-else class="status-icon unavailable"><CircleClose /></el-icon>
        </div>
        <div class="source-info">
          <div class="source-name">{{ sourceLabel(source.source_type) }}</div>
          <div class="source-detail">
            <span v-if="source.available">{{ source.evidence_count }} 条证据</span>
            <span v-else class="source-error">{{ source.error || '不可用' }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { CircleCheck, CircleClose } from '@element-plus/icons-vue'

interface SourceCoverageData {
  sources: Array<{
    source_type: string
    source_name: string
    available: boolean
    evidence_count: number
    error?: string
  }>
  total_sources: number
  available_sources: number
}

defineProps<{
  coverage?: SourceCoverageData
}>()

function sourceLabel(type: string): string {
  const map: Record<string, string> = {
    aegis_alert: 'Aegis 告警',
    runtime_event: '运行时事件',
    agent_process: 'Agent 进程',
    agent_network: 'Agent 网络',
    agent_file: 'Agent 文件',
    agent_log: 'Agent 日志',
    baseline: '基线检查',
    vulnerability: '漏洞数据',
    external_siem: '外部 SIEM',
    external_cmdb: '外部 CMDB',
    external_edr: '外部 EDR',
    external_ticket: '外部工单',
    external_threat_intel: '威胁情报',
    block_record: '阻断记录',
    detection_package: '检测包',
    audit_log: '审计日志',
  }
  return map[type] || type
}
</script>

<style scoped>
.source-coverage {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.coverage-ratio {
  font-size: 13px;
  color: #606266;
}

.empty-hint {
  text-align: center;
  color: #909399;
  font-size: 13px;
  padding: 16px;
}

.source-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.source-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  background: #f5f7fa;
  border-radius: 6px;
}

.source-item.unavailable {
  opacity: 0.6;
}

.status-icon {
  font-size: 18px;
}

.status-icon.available {
  color: #67c23a;
}

.status-icon.unavailable {
  color: #f56c6c;
}

.source-info {
  flex: 1;
}

.source-name {
  font-size: 13px;
  font-weight: 500;
  color: #303133;
}

.source-detail {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.source-error {
  color: #f56c6c;
}
</style>

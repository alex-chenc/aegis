<template>
  <div class="source-coverage">
    <div class="section-header">
      <span class="section-title">{{ $t('generated.assistantSourceCoveragePanel_data_source_coverage_f28900') }}</span>
      <span v-if="coverage" class="coverage-ratio">
        {{ coverage.available_sources }}/{{ coverage.total_sources }} {{ $t('generated.assistantSourceCoveragePanel_available_e91365') }}
      </span>
    </div>

    <div v-if="!coverage" class="empty-hint">
      {{ $t('generated.assistantSourceCoveragePanel_no_coverage_data_yet_2cb12d') }}
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
            <span v-if="source.available">{{ source.evidence_count }} {{ $t('generated.common_pieces_of_evidence_bf9c74') }}</span>
            <span v-else class="source-error">{{ source.error || $t('dynamic.unavailable') }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

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
    aegis_alert: translate('generatedScript.common_aegis_alert_cc5691'),
    runtime_event: translate('generatedScript.common_runtime_events_3afbe9'),
    agent_process: translate('generatedScript.common_agent_process_449f5d'),
    agent_network: translate('generatedScript.common_agent_network_26dcfa'),
    agent_file: translate('generatedScript.common_agent_file_fa1097'),
    agent_log: translate('generatedScript.common_agent_log_b9eb43'),
    baseline: translate('generatedScript.common_baseline_check_3c75ff'),
    vulnerability: translate('generatedScript.common_vulnerability_data_792410'),
    external_siem: translate('generatedScript.common_external_siem_b9a80d'),
    external_cmdb: translate('generatedScript.common_external_cmdb_a2b97e'),
    external_edr: translate('generatedScript.common_external_edr_593ae6'),
    external_ticket: translate('generatedScript.assistantSourceCoveragePanel_external_work_order_1cafbf'),
    external_threat_intel: translate('generatedScript.assistantSourceCoveragePanel_threat_intelligence_1ed632'),
    block_record: translate('generatedScript.common_block_recording_729ea3'),
    detection_package: translate('generatedScript.common_test_kit_757931'),
    audit_log: translate('generatedScript.common_audit_log_7666cd'),
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

<template>
  <div class="evidence-matrix">
    <div class="section-header">
      <span class="section-title">{{ $t('generated.assistantEvidenceMatrixTable_evidence_matrix_6931aa') }}</span>
      <el-tag v-if="evidenceCount > 0" size="small">{{ evidenceCount }} {{ $t('generated.common_pieces_of_evidence_bf9c74') }}</el-tag>
      <el-button
        v-if="evidenceCount > 0"
        type="primary"
        link
        size="small"
        @click="expanded = !expanded"
      >
        {{ expanded ? $t('dynamic.collapse') : $t('dynamic.expand') }}
      </el-button>
    </div>

    <div v-if="evidenceCount === 0" class="empty-hint">
      {{ $t('generated.assistantEvidenceMatrixTable_no_evidence_yet_bcab71') }}
    </div>

    <div v-else-if="!expanded" class="evidence-summary">
      <div class="summary-chips">
        <el-tag
          v-for="(ids, source) in bySource"
          :key="source"
          size="small"
          type="info"
          class="source-chip"
        >
          {{ sourceLabel(source) }}: {{ ids.length }}
        </el-tag>
      </div>
    </div>

    <div v-else class="evidence-table-wrapper">
      <el-table :data="items" size="small" stripe max-height="400">
        <el-table-column prop="severity" :label="$t('generated.assistantEvidenceMatrixTable_severity_9272e8')" width="80">
          <template #default="{ row }">
            <el-tag :type="severityTag(row.severity)" size="small">
              {{ row.severity }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="source_type" :label="$t('generated.common_source_c63f79')" width="120">
          <template #default="{ row }">
            {{ sourceLabel(row.source_type) }}
          </template>
        </el-table-column>
        <el-table-column prop="title" :label="$t('generated.common_title_748d7d')" min-width="200" show-overflow-tooltip />
        <el-table-column prop="summary" :label="$t('generated.assistantEvidenceMatrixTable_summary_46d4c1')" min-width="250" show-overflow-tooltip />
        <el-table-column prop="mitre_id" label="MITRE" width="120" />
        <el-table-column prop="confidence" :label="$t('generated.common_confidence_b78c2d')" width="80">
          <template #default="{ row }">
            {{ (row.confidence * 100).toFixed(0) }}%
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.assistantEvidenceMatrixTable_support_35f2ab')" width="100">
          <template #default="{ row }">
            <el-tag
              v-for="s in (row.supports || []).slice(0, 2)"
              :key="s"
              size="small"
              type="info"
              class="support-tag"
            >
              {{ supportLabel(s) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.assistantEvidenceMatrixTable_external_2ffba3')" width="60" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.is_external" size="small" type="warning">{{ $t('generated.assistantEvidenceMatrixTable_yes_30160a') }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { ref, computed } from 'vue'

interface EvidenceItem {
  evidence_id: string
  source_type: string
  source_name: string
  object_type: string
  object_id?: string
  severity: string
  mitre_id?: string
  title: string
  summary: string
  supports?: string[]
  confidence: number
  is_external: boolean
  is_truncated: boolean
}

const props = defineProps<{
  items: EvidenceItem[]
  evidenceCount: number
  bySource?: Record<string, string[]>
}>()

const expanded = ref(false)

const bySource = computed(() => props.bySource || {})

function severityTag(severity: string): string {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'danger',
    medium: 'warning',
    low: 'info',
    info: 'info',
  }
  return map[severity] || 'info'
}

function sourceLabel(source: string): string {
  const map: Record<string, string> = {
    aegis_alert: translate('generatedScript.common_alarm_507842'),
    runtime_event: translate('generatedScript.common_runtime_events_3afbe9'),
    agent_process: translate('generatedScript.common_process_4eb476'),
    agent_network: translate('generatedScript.common_network_0cbda6'),
    agent_file: translate('generatedScript.common_document_49deaf'),
    agent_log: translate('generatedScript.assistantEvidenceMatrixTable_log_4de508'),
    baseline: translate('generatedScript.common_baseline_4bb193'),
    vulnerability: translate('generatedScript.common_loopholes_86835d'),
    external_mcp: translate('generatedScript.assistantEvidenceMatrixTable_external_data_source_be76ca'),
    block_record: translate('generatedScript.common_block_recording_729ea3'),
    detection_package: translate('generatedScript.common_test_kit_757931'),
    audit_log: translate('generatedScript.common_audit_log_7666cd'),
  }
  return map[source] || source
}

function supportLabel(s: string): string {
  const map: Record<string, string> = {
    compromise: translate('generatedScript.assistantEvidenceMatrixTable_fall_0c714b'),
    entry_point: translate('generatedScript.assistantEvidenceMatrixTable_entrance_38e9cd'),
    persistence: translate('generatedScript.common_persistence_63cca6'),
    lateral_movement: translate('generatedScript.assistantEvidenceMatrixTable_horizontal_5f1456'),
    exfiltration: translate('generatedScript.assistantEvidenceMatrixTable_outreach_95f3dd'),
  }
  return map[s] || s
}
</script>

<style scoped>
.evidence-matrix {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.empty-hint {
  text-align: center;
  color: #909399;
  font-size: 13px;
  padding: 16px;
}

.evidence-summary {
  padding: 8px 0;
}

.summary-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.source-chip {
  font-size: 12px;
}

.evidence-table-wrapper {
  margin-top: 4px;
}

.support-tag {
  margin-right: 4px;
  margin-bottom: 2px;
}
</style>

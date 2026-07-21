<template>
  <el-drawer :model-value="visible" :title="$t('generated.settingsAuditLogsAuditDetailDrawer_audit_details_18c5e1')" size="60%" @close="$emit('close')">
    <div v-if="log">
      <el-descriptions :column="2" border style="margin-bottom: 16px">
        <el-descriptions-item :label="$t('generated.common_script_type_f1574a')">{{ scriptTypeLabelKeys[log.script_type] ? $t(scriptTypeLabelKeys[log.script_type]) : log.script_type }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_audit_source_9af972')">{{ auditSourceLabelKeys[log.audit_source] ? $t(auditSourceLabelKeys[log.audit_source]) : log.audit_source }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_number_of_attempts_3fd524')">{{ log.attempt }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_result_0a2c91')">
          <el-tag :type="log.passed ? 'success' : 'danger'" size="small">
            {{ log.passed ? $t('dynamic.passed') : $t('common.status.failed') }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_risk_level_a90f1e')">
          <el-tag :type="log.risk_level === 'critical' ? 'danger' : 'warning'" size="small">{{ log.risk_level }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_time_consuming_a9704e')">{{ log.duration_ms }}ms</el-descriptions-item>
      </el-descriptions>

      <el-card style="margin-bottom: 16px">
        <template #header><span>{{ $t('generated.common_script_content_2a33ea') }}</span></template>
        <pre class="code-block">{{ log.script_content }}</pre>
      </el-card>

      <el-card v-if="log.blacklist_hits?.length" style="margin-bottom: 16px">
        <template #header><span>{{ $t('generated.settingsAuditLogsAuditDetailDrawer_blacklist_hit_details_0002ce') }}</span></template>
        <el-table :data="log.blacklist_hits" size="small">
          <el-table-column prop="rule_name" :label="$t('generated.common_rule_name_1937bc')" />
          <el-table-column prop="line_number" :label="$t('generated.settingsAuditLogsAuditDetailDrawer_line_number_f81e57')" width="80" />
          <el-table-column prop="matched_text" :label="$t('generated.settingsAuditLogsAuditDetailDrawer_match_text_4a58dc')" show-overflow-tooltip />
        </el-table>
      </el-card>

      <el-card v-if="log.ai_analysis?.length" style="margin-bottom: 16px">
        <template #header><span>{{ $t('generated.settingsAuditLogsAuditDetailDrawer_ai_audit_results_65c293') }}</span></template>
        <div v-for="(issue, i) in log.ai_analysis" :key="i" style="padding: 8px 0; border-bottom: 1px solid #eee">
          <el-tag type="warning" size="small" style="margin-right: 8px">{{ issue.type }}</el-tag>
          <span>{{ issue.description }}</span>
          <div v-if="issue.line_range" style="color: #999; font-size: 12px; margin-top: 4px">{{ $t('generated.settingsAuditLogsAuditDetailDrawer_row_range_637e0e') }} {{ issue.line_range }}</div>
          <div v-if="issue.suggestion" style="color: #67c23a; font-size: 12px; margin-top: 4px">{{ $t('generated.common_suggestion_aeb940') }} {{ issue.suggestion }}</div>
        </div>
      </el-card>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import type { AuditLog } from '@/api/audit-logs'
import { scriptTypeLabelKeys, auditSourceLabelKeys } from '../constants'

defineProps<{
  visible: boolean
  log: AuditLog | null
}>()

defineEmits<{
  (e: 'close'): void
}>()
</script>

<style scoped>
.code-block {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  font-family: 'Fira Code', monospace;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 400px;
  overflow: auto;
  margin: 0;
}
</style>

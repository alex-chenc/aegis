<template>
  <el-drawer :model-value="visible" title="审计详情" size="60%" @close="$emit('close')">
    <div v-if="log">
      <el-descriptions :column="2" border style="margin-bottom: 16px">
        <el-descriptions-item label="脚本类型">{{ scriptTypeLabels[log.script_type] || log.script_type }}</el-descriptions-item>
        <el-descriptions-item label="审计来源">{{ auditSourceLabels[log.audit_source] || log.audit_source }}</el-descriptions-item>
        <el-descriptions-item label="尝试次数">{{ log.attempt }}</el-descriptions-item>
        <el-descriptions-item label="结果">
          <el-tag :type="log.passed ? 'success' : 'danger'" size="small">
            {{ log.passed ? '通过' : '失败' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="风险等级">
          <el-tag :type="log.risk_level === 'critical' ? 'danger' : 'warning'" size="small">{{ log.risk_level }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="耗时">{{ log.duration_ms }}ms</el-descriptions-item>
      </el-descriptions>

      <el-card style="margin-bottom: 16px">
        <template #header><span>脚本内容</span></template>
        <pre class="code-block">{{ log.script_content }}</pre>
      </el-card>

      <el-card v-if="log.blacklist_hits?.length" style="margin-bottom: 16px">
        <template #header><span>黑名单命中详情</span></template>
        <el-table :data="log.blacklist_hits" size="small">
          <el-table-column prop="rule_name" label="规则名称" />
          <el-table-column prop="line_number" label="行号" width="80" />
          <el-table-column prop="matched_text" label="匹配文本" show-overflow-tooltip />
        </el-table>
      </el-card>

      <el-card v-if="log.ai_analysis?.length" style="margin-bottom: 16px">
        <template #header><span>AI审计结果</span></template>
        <div v-for="(issue, i) in log.ai_analysis" :key="i" style="padding: 8px 0; border-bottom: 1px solid #eee">
          <el-tag type="warning" size="small" style="margin-right: 8px">{{ issue.type }}</el-tag>
          <span>{{ issue.description }}</span>
          <div v-if="issue.line_range" style="color: #999; font-size: 12px; margin-top: 4px">行范围: {{ issue.line_range }}</div>
          <div v-if="issue.suggestion" style="color: #67c23a; font-size: 12px; margin-top: 4px">建议: {{ issue.suggestion }}</div>
        </div>
      </el-card>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import type { AuditLog } from '@/api/audit-logs'
import { scriptTypeLabels, auditSourceLabels } from '../constants'

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

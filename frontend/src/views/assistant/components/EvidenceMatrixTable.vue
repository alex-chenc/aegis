<template>
  <div class="evidence-matrix">
    <div class="section-header">
      <span class="section-title">证据矩阵</span>
      <el-tag v-if="evidenceCount > 0" size="small">{{ evidenceCount }} 条证据</el-tag>
      <el-button
        v-if="evidenceCount > 0"
        type="primary"
        link
        size="small"
        @click="expanded = !expanded"
      >
        {{ expanded ? '收起' : '展开' }}
      </el-button>
    </div>

    <div v-if="evidenceCount === 0" class="empty-hint">
      暂无证据
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
        <el-table-column prop="severity" label="严重度" width="80">
          <template #default="{ row }">
            <el-tag :type="severityTag(row.severity)" size="small">
              {{ row.severity }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="source_type" label="来源" width="120">
          <template #default="{ row }">
            {{ sourceLabel(row.source_type) }}
          </template>
        </el-table-column>
        <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip />
        <el-table-column prop="summary" label="摘要" min-width="250" show-overflow-tooltip />
        <el-table-column prop="mitre_id" label="MITRE" width="120" />
        <el-table-column prop="confidence" label="置信度" width="80">
          <template #default="{ row }">
            {{ (row.confidence * 100).toFixed(0) }}%
          </template>
        </el-table-column>
        <el-table-column label="支撑" width="100">
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
        <el-table-column label="外部" width="60" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.is_external" size="small" type="warning">是</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
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
    aegis_alert: '告警',
    runtime_event: '运行时事件',
    agent_process: '进程',
    agent_network: '网络',
    agent_file: '文件',
    agent_log: '日志',
    baseline: '基线',
    vulnerability: '漏洞',
    external_mcp: '外部数据源',
    block_record: '阻断记录',
    detection_package: '检测包',
    audit_log: '审计日志',
  }
  return map[source] || source
}

function supportLabel(s: string): string {
  const map: Record<string, string> = {
    compromise: '失陷',
    entry_point: '入口',
    persistence: '持久化',
    lateral_movement: '横向',
    exfiltration: '外联',
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

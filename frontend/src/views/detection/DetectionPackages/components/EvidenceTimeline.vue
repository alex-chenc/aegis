<template>
  <div class="evidence-timeline">
    <el-timeline>
      <el-timeline-item
        v-for="(item, index) in evidence"
        :key="index"
        :timestamp="formatTime(item.timestamp)"
        :type="getStepColor(index)"
        placement="top"
      >
        <el-card shadow="never">
          <div class="evidence-header">
            <el-tag size="small" :type="getStepColor(index)">步骤 {{ index + 1 }}</el-tag>
            <el-text tag="b" style="margin-left: 8px;">{{ item.rule_id }}</el-text>
          </div>
          <el-descriptions :column="2" size="small" border style="margin-top: 8px;">
            <el-descriptions-item label="Event Type">{{ item.event_type }}</el-descriptions-item>
            <el-descriptions-item label="PID">{{ item.pid }}</el-descriptions-item>
            <el-descriptions-item v-if="item.ppid" label="PPID">{{ item.ppid }}</el-descriptions-item>
            <el-descriptions-item label="UID">{{ item.uid }}</el-descriptions-item>
            <el-descriptions-item v-if="item.image" label="进程" :span="2">{{ item.image }}</el-descriptions-item>
          </el-descriptions>
          <el-collapse v-if="item.fields && Object.keys(item.fields).length > 0" style="margin-top: 8px;">
            <el-collapse-item title="详细字段">
              <pre class="fields-json">{{ JSON.stringify(item.fields, null, 2) }}</pre>
            </el-collapse-item>
          </el-collapse>
        </el-card>
      </el-timeline-item>
    </el-timeline>
  </div>
</template>

<script setup lang="ts">
export interface EvidenceItem {
  rule_id: string
  event_type: string
  timestamp?: number
  pid: number
  ppid?: number
  uid: number
  image?: string
  fields?: Record<string, unknown>
}

defineProps<{
  evidence: EvidenceItem[]
}>()

function formatTime(ns?: number) {
  if (!ns) return ''
  const ms = ns / 1_000_000
  return new Date(ms).toLocaleString()
}

function getStepColor(index: number) {
  const colors = ['primary', 'success', 'warning', 'danger']
  return colors[index % colors.length] as any
}
</script>

<style scoped>
.fields-json {
  font-family: monospace;
  font-size: 12px;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>

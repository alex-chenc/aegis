<template>
  <div class="result-card">
    <div class="card-header">
      <el-icon><DataBoard /></el-icon>
      <span class="card-title">{{ card.title }}</span>
      <el-tag size="small">{{ getTypeLabel(card.type) }}</el-tag>
    </div>

    <!-- Markdown 渲染 -->
    <div v-if="card.type === 'markdown'" class="markdown-content" v-html="renderMarkdown(card.payload)"></div>

    <!-- JSON 渲染 -->
    <div v-else-if="card.type === 'json'" class="json-content">
      <pre>{{ JSON.stringify(card.payload, null, 2) }}</pre>
    </div>

    <!-- 主机列表 -->
    <div v-else-if="card.type === 'host_list'" class="host-list">
      <div v-for="(host, idx) in card.payload.items" :key="idx" class="host-item">
        <span class="host-name">{{ host.hostname }}</span>
        <span class="host-ip">{{ host.ip_address }}</span>
        <el-tag :type="host.status === 'online' ? 'success' : 'danger'" size="small">
          {{ host.status }}
        </el-tag>
      </div>
    </div>

    <!-- 告警列表 -->
    <div v-else-if="card.type === 'alert_list'" class="alert-list">
      <div v-for="(alert, idx) in card.payload.items" :key="idx" class="alert-item">
        <el-tag :type="getSeverityTag(alert.severity)" size="small">
          {{ alert.severity }}
        </el-tag>
        <span class="alert-title">{{ alert.rule_title }}</span>
      </div>
    </div>

    <!-- 攻击研判结果 -->
    <div v-else-if="card.type === 'host_attack_investigation'" class="investigation-result">
      <HostAttackInvestigationPanel :data="card.payload" />
    </div>

    <!-- 任务状态摘要 -->
    <div v-else-if="card.type === 'task_status'" class="task-status-summary">
      <div class="task-status-topline">
        <span>{{ card.payload.task_group_id || card.payload.task_id || '任务' }}</span>
        <el-tag :type="getTaskStatusTag(String(card.payload.status || 'pending'))" size="small">
          {{ getTaskStatusLabel(String(card.payload.status || 'pending')) }}
        </el-tag>
      </div>
      <div class="task-status-grid">
        <div v-for="item in getTaskMetrics(card.payload)" :key="item.label" class="task-status-metric">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
        </div>
      </div>
    </div>

    <!-- 默认渲染 -->
    <div v-else class="default-content">
      <pre>{{ JSON.stringify(card.payload, null, 2) }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { DataBoard } from '@element-plus/icons-vue'
import type { AssistantResultCard } from '@/api/assistant'
import HostAttackInvestigationPanel from './HostAttackInvestigationPanel.vue'

defineProps<{
  card: AssistantResultCard
}>()

function getTypeLabel(type: string): string {
  const map: Record<string, string> = {
    host_list: '主机列表',
    alert_list: '告警列表',
    task_status: '任务状态',
    package_summary: '检测包摘要',
    attack_graph: '攻击图',
    host_attack_investigation: '攻击研判',
    evidence_matrix: '证据矩阵',
    markdown: 'Markdown',
    json: 'JSON',
  }
  return map[type] || type
}

function getSeverityTag(severity: string): string {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'danger',
    medium: 'warning',
    low: 'info',
  }
  return map[severity] || 'info'
}

function getTaskStatusTag(status: string): string {
  const normalized = status.toLowerCase()
  if (['success', 'completed'].includes(normalized)) return 'success'
  if (['failed', 'timeout'].includes(normalized)) return 'danger'
  if (['running', 'pending'].includes(normalized)) return 'warning'
  return 'info'
}

function getTaskStatusLabel(status: string): string {
  const map: Record<string, string> = {
    pending: '等待中',
    running: '运行中',
    success: '成功',
    completed: '完成',
    failed: '失败',
    timeout: '超时',
  }
  return map[status.toLowerCase()] || status
}

function getTaskMetrics(payload: Record<string, any>) {
  return [
    { label: '总数', value: payload.total ?? payload.task_count ?? 0 },
    { label: '成功', value: payload.success ?? payload.success_count ?? 0 },
    { label: '运行', value: payload.running ?? payload.running_count ?? 0 },
    { label: '失败', value: payload.failed ?? payload.failed_count ?? 0 },
  ]
}

function renderMarkdown(payload: Record<string, unknown>): string {
  const content = (payload.content as string) || JSON.stringify(payload)
  return content
    .replace(/\n/g, '<br>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/`(.*?)`/g, '<code>$1</code>')
}
</script>

<style scoped>
.result-card {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 12px;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 500;
}

.card-title {
  flex: 1;
}

.markdown-content {
  font-size: 14px;
  line-height: 1.6;
}

.json-content pre {
  background: #f5f7fa;
  padding: 12px;
  border-radius: 6px;
  font-size: 12px;
  overflow-x: auto;
}

.host-list,
.alert-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.host-item,
.alert-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  background: #f5f7fa;
  border-radius: 6px;
}

.host-name {
  font-weight: 500;
}

.host-ip {
  color: #909399;
}

.alert-title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.investigation-result {
  margin-top: 8px;
}

.task-status-summary {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-status-topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: #1f2937;
  font-size: 13px;
  font-weight: 650;
}

.task-status-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}

.task-status-metric {
  padding: 8px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #f8fafc;
}

.task-status-metric span {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.task-status-metric strong {
  color: #1f2937;
  font-size: 15px;
}

.default-content pre {
  background: #f5f7fa;
  padding: 12px;
  border-radius: 6px;
  font-size: 12px;
  overflow-x: auto;
}
</style>

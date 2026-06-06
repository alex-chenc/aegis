<template>
  <aside class="context-rail">
    <!-- 上下文对象 -->
    <div class="rail-section">
      <div class="section-header">
        <el-icon><Connection /></el-icon>
        <span>上下文对象</span>
      </div>
      <div v-if="contextRefs.length" class="context-list">
        <div
          v-for="ref in contextRefs"
          :key="ref.id"
          class="context-item"
        >
          <el-tag :type="getObjectTypeTag(ref.object_type)" size="small">
            {{ getObjectTypeLabel(ref.object_type) }}
          </el-tag>
          <div class="context-info">
            <div class="context-title">{{ ref.title || ref.object_id }}</div>
            <div v-if="ref.summary" class="context-summary">{{ ref.summary }}</div>
          </div>
        </div>
      </div>
      <el-empty v-else description="无上下文对象" :image-size="48" />
    </div>

    <!-- 待审批动作 -->
    <div v-if="approvals.length" class="rail-section">
      <div class="section-header">
        <el-icon><Bell /></el-icon>
        <span>待审批动作</span>
        <el-badge :value="approvals.length" type="warning" />
      </div>
      <div class="approval-list">
        <div
          v-for="approval in approvals"
          :key="approval.approval_id"
          class="approval-item"
        >
          <div class="approval-info">
            <el-tag :type="getRiskTag(approval.risk_level)" size="small">
              {{ approval.risk_level }}
            </el-tag>
            <span class="approval-tool">{{ approval.tool_name }}</span>
          </div>
          <div class="approval-title">{{ approval.title }}</div>
        </div>
      </div>
    </div>

    <!-- 工具调用记录 -->
    <div class="rail-section">
      <div class="section-header">
        <el-icon><Tools /></el-icon>
        <span>工具调用记录</span>
      </div>
      <div v-if="toolCalls.length" class="tool-call-list">
        <div
          v-for="call in toolCalls"
          :key="call.call_id"
          class="tool-call-item"
        >
          <el-tag :type="getRiskTag(call.risk_level)" size="small">
            {{ call.risk_level }}
          </el-tag>
          <span class="tool-name">{{ call.tool_name }}</span>
          <el-tag :type="getStatusTag(call.status)" size="small">
            {{ getStatusLabel(call.status) }}
          </el-tag>
        </div>
      </div>
      <el-empty v-else description="暂无工具调用" :image-size="48" />
    </div>
  </aside>
</template>

<script setup lang="ts">
import { Connection, Bell, Tools } from '@element-plus/icons-vue'
import type { AssistantContextRef, AssistantToolCall, AssistantApproval } from '@/api/assistant'

defineProps<{
  contextRefs: AssistantContextRef[]
  approvals: AssistantApproval[]
  toolCalls: AssistantToolCall[]
}>()

function getObjectTypeTag(type: string): string {
  const map: Record<string, string> = {
    host: 'primary',
    alert: 'danger',
    task: 'warning',
    vulnerability: 'danger',
    package: 'info',
    rule: 'info',
  }
  return map[type] || 'info'
}

function getObjectTypeLabel(type: string): string {
  const map: Record<string, string> = {
    host: '主机',
    alert: '告警',
    task: '任务',
    vulnerability: '漏洞',
    package: '检测包',
    rule: '规则',
  }
  return map[type] || type
}

function getRiskTag(level: string): string {
  const map: Record<string, string> = {
    readonly: 'info',
    low: 'success',
    medium: 'warning',
    high: 'danger',
    critical: 'danger',
  }
  return map[level] || 'info'
}

function getStatusTag(status: string): string {
  const map: Record<string, string> = {
    pending: 'info',
    running: 'warning',
    completed: 'success',
    success: 'success',
    failed: 'danger',
    approval_required: 'warning',
    rejected: 'info',
    cancelled: 'info',
  }
  return map[status] || 'info'
}

function getStatusLabel(status: string): string {
  const map: Record<string, string> = {
    pending: '等待中',
    running: '执行中',
    completed: '成功',
    success: '成功',
    failed: '失败',
    approval_required: '待审批',
    rejected: '已拒绝',
    cancelled: '已取消',
  }
  return map[status] || status
}
</script>

<style scoped>
.context-rail {
  width: 360px;
  background: #fff;
  border-left: 1px solid #e4e7ed;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.rail-section {
  padding: 16px;
  border-bottom: 1px solid #e4e7ed;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.context-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.context-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px;
  background: #f5f7fa;
  border-radius: 6px;
}

.context-info {
  flex: 1;
  min-width: 0;
}

.context-title {
  font-size: 13px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.context-summary {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.approval-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.approval-item {
  padding: 8px;
  background: #fdf6ec;
  border: 1px solid #faecd8;
  border-radius: 6px;
}

.approval-info {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}

.approval-tool {
  font-size: 13px;
  font-weight: 500;
}

.approval-title {
  font-size: 12px;
  color: #606266;
}

.tool-call-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tool-call-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  background: #f5f7fa;
  border-radius: 4px;
  font-size: 12px;
}

.tool-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mode-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>

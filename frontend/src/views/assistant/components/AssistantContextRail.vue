<template>
  <aside class="context-rail">
    <!-- 执行计划 -->
    <div v-if="plan" class="rail-section">
      <ExecutionPlan :plan="plan" title-only />
    </div>
    <div v-else class="rail-section">
      <div class="section-header">
        <el-icon><List /></el-icon>
        <span>执行计划</span>
      </div>
      <el-empty description="暂无执行计划" :image-size="48" />
    </div>

    <!-- 会话上下文 -->
    <div class="rail-section">
      <div class="section-header">
        <el-icon><Document /></el-icon>
        <span>会话上下文</span>
        <el-badge v-if="contextRefs.length" :value="contextRefs.length" type="primary" />
      </div>
      <div v-if="contextRefs.length" class="context-ref-list">
        <div
          v-for="ref in contextRefs"
          :key="ref.id || `${ref.object_type}-${ref.object_id}`"
          class="context-ref-item"
        >
          <div class="context-ref-topline">
            <el-tag size="small" type="info">{{ getContextTypeLabel(ref.object_type) }}</el-tag>
            <span class="context-ref-title">{{ ref.title || ref.object_id }}</span>
          </div>
          <div v-if="ref.summary" class="context-ref-summary">{{ ref.summary }}</div>
        </div>
      </div>
      <el-empty v-else description="暂无上下文" :image-size="48" />
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
import { Bell, Document, List, Tools } from '@element-plus/icons-vue'
import ExecutionPlan from '@/components/ExecutionPlan.vue'
import type { PlanEvent } from '@/api/aiAnalysis'
import type { AssistantToolCall, AssistantApproval, AssistantContextRef } from '@/api/assistant'

defineProps<{
  plan: PlanEvent | null
  contextRefs: AssistantContextRef[]
  approvals: AssistantApproval[]
  toolCalls: AssistantToolCall[]
}>()

function getContextTypeLabel(type: string): string {
  const map: Record<string, string> = {
    file: '文件',
    baseline_template: '基线',
    sigma_rule_upload: 'Sigma',
    host: '主机',
    alert: '告警',
    task: '任务',
    vulnerability: '漏洞',
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
  will-change: transform, opacity;
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

/* 计划模块样式 */
.plan-goal {
  font-size: 13px;
  color: #606266;
  margin-bottom: 12px;
  padding: 8px;
  background: #f5f7fa;
  border-radius: 6px;
}

.plan-steps {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.plan-step {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  background: #f5f7fa;
  border-radius: 6px;
  font-size: 12px;
}

.plan-step.completed {
  background: #f0f9ff;
}

.plan-step.running {
  background: #fdf6ec;
}

.plan-step.failed {
  background: #fef0f0;
}

.step-number {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 600;
  flex-shrink: 0;
}

.plan-step.completed .step-number {
  background: #67c23a;
  color: #fff;
}

.plan-step.running .step-number {
  background: #e6a23c;
  color: #fff;
}

.plan-step.failed .step-number {
  background: #f56c6c;
  color: #fff;
}

.step-content {
  flex: 1;
  min-width: 0;
}

.step-title {
  font-size: 12px;
  font-weight: 500;
  color: #303133;
}

.step-result {
  font-size: 11px;
  color: #909399;
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 审批样式 */
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

.context-ref-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.context-ref-item {
  padding: 9px 10px;
  border: 1px solid #dbe4ef;
  border-radius: 6px;
  background: #f8fafc;
}

.context-ref-topline {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.context-ref-title {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 650;
  color: #1f2937;
}

.context-ref-summary {
  margin-top: 6px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-word;
}

/* 工具调用样式 */
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
</style>

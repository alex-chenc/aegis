<template>
  <div class="conversation-container" ref="containerRef">
    <div class="message-list">
      <div
        v-for="msg in messages"
        :key="msg.message_id"
        class="message-group"
      >
        <!-- 用户消息 -->
        <div v-if="msg.role === 'user'" class="message user">
          <div class="message-avatar">
            <el-icon><User /></el-icon>
          </div>
          <div class="message-bubble">
            <div class="message-content">{{ msg.content }}</div>
          </div>
        </div>

        <!-- 助手消息 -->
        <div v-else-if="msg.role === 'assistant'" class="message assistant">
          <div class="message-avatar">
            <el-icon><Monitor /></el-icon>
          </div>
          <div class="message-body">
            <!-- 思考过程 -->
            <div v-if="msg.thinking" class="thinking-block">
              <div class="thinking-header">
                <el-icon><Loading /></el-icon>
                <span>思考过程</span>
              </div>
              <div class="thinking-content">{{ msg.thinking }}</div>
            </div>

            <!-- 计划卡片 -->
            <ExecutionPlan v-if="msg.plan" :plan="normalizeMessagePlan(msg)" />

            <!-- 工具调用卡片 -->
            <div v-if="getMessageToolCalls(msg).length" class="tool-calls">
              <AssistantToolCallCard
                v-for="call in getMessageToolCalls(msg)"
                :key="call.call_id"
                :call="call"
              />
            </div>

            <!-- 审批卡片 -->
            <div v-if="msg.approvals?.length" class="approvals">
              <AssistantApprovalCard
                v-for="approval in msg.approvals"
                :key="approval.approval_id"
                :approval="approval"
                @approve="(id, comment) => $emit('approve', id, comment)"
                @reject="(id, comment) => $emit('reject', id, comment)"
              />
            </div>

            <!-- 消息内容 -->
            <div v-if="msg.content" class="message-bubble">
              <div class="message-content" v-html="formatContent(msg.content)"></div>
            </div>

            <!-- 结果卡片 -->
            <div v-if="msg.result_cards?.length" class="result-cards">
              <AssistantResultRenderer
                v-for="(card, idx) in msg.result_cards"
                :key="idx"
                :card="card"
              />
            </div>
          </div>
        </div>

        <!-- 系统消息 -->
        <div v-else-if="msg.role === 'system'" class="message system">
          <div class="system-content">
            <el-icon><InfoFilled /></el-icon>
            <span>{{ msg.content }}</span>
          </div>
        </div>
      </div>

      <!-- 流式输入指示器 -->
      <div v-if="streaming" class="message assistant">
        <div class="message-avatar">
          <el-icon><Monitor /></el-icon>
        </div>
        <div class="message-body">
          <div class="typing-indicator">
            <span></span>
            <span></span>
            <span></span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { User, Monitor, Loading, InfoFilled } from '@element-plus/icons-vue'
import ExecutionPlan from '@/components/ExecutionPlan.vue'
import { normalizePlanEvent } from '@/utils/aiAnalysisRuntime'
import AssistantToolCallCard from './AssistantToolCallCard.vue'
import AssistantApprovalCard from './AssistantApprovalCard.vue'
import AssistantResultRenderer from './AssistantResultRenderer.vue'
import type { AssistantMessage, AssistantToolCall, AssistantApproval, AssistantResultCard } from '@/api/assistant'
import type { PlanEvent } from '@/api/aiAnalysis'

defineEmits<{
  approve: [approvalId: string, comment?: string]
  reject: [approvalId: string, comment?: string]
}>()

const props = defineProps<{
  messages: AssistantMessage[]
  toolCalls: AssistantToolCall[]
  approvals: AssistantApproval[]
  resultCards: AssistantResultCard[]
  streaming: boolean
}>()

const containerRef = ref<HTMLElement>()

function scrollToBottom() {
  nextTick(() => {
    if (containerRef.value) {
      containerRef.value.scrollTop = containerRef.value.scrollHeight
    }
  })
}

watch(() => props.messages, scrollToBottom, { deep: true })
watch(() => props.streaming, scrollToBottom)

/**
 * 获取消息关联的工具调用
 * 优先使用 store 级别的 toolCalls（SSE 实时更新状态）
 * 降级使用消息级别的 tool_calls（历史数据）
 */
function getMessageToolCalls(msg: AssistantMessage): AssistantToolCall[] {
  // 优先使用 store 级别的 toolCalls（SSE 实时更新）
  const storeCalls = props.toolCalls.filter(tc =>
    tc.message_id === msg.message_id || tc.message_id === msg.id
  )
  if (storeCalls.length > 0) return storeCalls
  // 降级使用消息级别的 tool_calls（历史数据）
  return msg.tool_calls || []
}

function normalizeMessagePlan(msg: AssistantMessage): PlanEvent | null {
  if (!msg.plan) return null
  return normalizePlanEvent({
    id: `plan-${msg.id || msg.message_id}`,
    plan_id: `plan-${msg.id || msg.message_id}`,
    ...msg.plan,
  })
}

function formatContent(content: string): string {
  return content
    .replace(/\n/g, '<br>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/`(.*?)`/g, '<code>$1</code>')
}

</script>

<style scoped>
.conversation-container {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  background: #f9fafb;
}

.message-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.message {
  display: flex;
  gap: 12px;
}

.message.user {
  flex-direction: row-reverse;
}

.message-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  font-size: 18px;
  color: #909399;
}

.message.user .message-avatar {
  background: #409eff;
  color: #fff;
}

.message.assistant .message-avatar {
  background: #67c23a;
  color: #fff;
}

.message-body {
  max-width: 75%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.message-bubble {
  background: #fff;
  padding: 12px 16px;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.message.user .message-bubble {
  background: #409eff;
  color: #fff;
}

.message-content {
  font-size: 14px;
  line-height: 1.6;
  word-break: break-word;
}

.message-content :deep(code) {
  background: rgba(0, 0, 0, 0.05);
  padding: 2px 4px;
  border-radius: 3px;
  font-family: monospace;
  font-size: 13px;
}

.message.user .message-content :deep(code) {
  background: rgba(255, 255, 255, 0.2);
}

.thinking-block {
  background: #f0f9ff;
  border: 1px solid #bae6fd;
  border-radius: 8px;
  padding: 12px;
}

.thinking-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #0284c7;
  margin-bottom: 8px;
}

.thinking-content {
  font-size: 13px;
  color: #64748b;
  white-space: pre-wrap;
}

.plan-card {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 12px;
}

.plan-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  font-size: 14px;
  font-weight: 500;
}

.plan-goal {
  font-size: 13px;
  color: #606266;
  margin-bottom: 12px;
}

.plan-steps {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.plan-step {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  background: #f5f7fa;
  border-radius: 6px;
}

.step-number {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
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
  font-size: 13px;
  font-weight: 500;
}

.step-result {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.tool-calls,
.approvals,
.result-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.system-content {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  background: #f0f9ff;
  border-radius: 8px;
  font-size: 13px;
  color: #606266;
  align-self: center;
}

.typing-indicator {
  display: flex;
  gap: 4px;
  padding: 12px 16px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.typing-indicator span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #909399;
  animation: typing 1.4s infinite;
}

.typing-indicator span:nth-child(2) {
  animation-delay: 0.2s;
}

.typing-indicator span:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes typing {
  0%, 60%, 100% { transform: translateY(0); }
  30% { transform: translateY(-8px); }
}
</style>

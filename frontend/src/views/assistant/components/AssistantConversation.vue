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

        <!-- 助手消息：每个展示段都有独立头像 -->
        <template v-else-if="msg.role === 'assistant'">
          <div
            v-for="segment in getAssistantSegments(msg)"
            :key="segment.key"
            class="message assistant"
          >
            <div class="message-avatar">
              <el-icon><Monitor /></el-icon>
            </div>
            <div class="message-body">
              <!-- 思考步骤 -->
              <div v-if="segment.type === 'thinking'" class="thinking-block">
                <div class="thinking-header">
                  <span class="thinking-pulse" aria-hidden="true"></span>
                  <span>思考</span>
                </div>
                <div class="thinking-content">
                  <div
                    v-for="(step, index) in segment.steps"
                    :key="`${segment.key}-thinking-${index}`"
                    class="thinking-step"
                  >
                    {{ step }}
                  </div>
                </div>
              </div>

              <!-- 消息内容（思考后的结果） -->
              <div v-else-if="segment.type === 'content'" class="message-bubble">
                <div class="message-content" v-html="formatContent(segment.content)"></div>
              </div>

              <!-- 工具调用和执行结果放在同一个框内 -->
              <div v-else-if="segment.type === 'tool'" class="tool-calls">
                <div
                  class="tool-call-card"
                  :class="`status-${segment.toolCall.status}`"
                >
                  <div class="tool-call-header">
                    <el-icon><SetUp /></el-icon>
                    <span class="tool-name">{{ segment.toolCall.tool_name }}</span>
                    <el-tag size="small" :type="getToolStatusType(segment.toolCall.status)">
                      {{ segment.toolCall.status }}
                    </el-tag>
                  </div>
                  <div
                    v-if="getToolResultText(segment.toolCall)"
                    class="tool-call-result"
                    :class="{ 'is-json': isJsonToolResult(segment.toolCall) }"
                  >
                    {{ getDisplayedToolResult(segment.toolCall) }}
                  </div>
                  <button
                    v-if="isLongToolResult(segment.toolCall)"
                    type="button"
                    class="tool-result-toggle"
                    @click="toggleToolResult(segment.toolCall)"
                  >
                    {{ isToolResultExpanded(segment.toolCall) ? '收起结果' : '展开完整结果' }}
                  </button>
                </div>
              </div>

              <!-- 审批卡片 -->
              <div v-else-if="segment.type === 'approvals'" class="approvals">
                <AssistantApprovalCard
                  v-for="approval in segment.approvals"
                  :key="approval.approval_id"
                  :approval="approval"
                  @approve="(id, comment) => $emit('approve', id, comment)"
                  @reject="(id, comment) => $emit('reject', id, comment)"
                />
              </div>

              <!-- 步骤结果 -->
              <div v-else-if="segment.type === 'step-results'" class="step-results">
                <div
                  v-for="(result, index) in segment.results"
                  :key="`${segment.key}-result-${index}`"
                  class="step-result-card"
                >
                  <div class="step-result-header">
                    <el-icon><CircleCheck /></el-icon>
                    <span>{{ result.title }}</span>
                  </div>
                  <div class="step-result-content">{{ result.summary }}</div>
                </div>
              </div>

              <!-- 结果卡片 -->
              <div v-else-if="segment.type === 'result-cards'" class="result-cards">
                <AssistantResultRenderer
                  v-for="(card, idx) in segment.cards"
                  :key="idx"
                  :card="card"
                />
              </div>
            </div>
          </div>
        </template>

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
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { User, Monitor, InfoFilled, CircleCheck, SetUp } from '@element-plus/icons-vue'
import { gsap } from 'gsap'
import AssistantApprovalCard from './AssistantApprovalCard.vue'
import AssistantResultRenderer from './AssistantResultRenderer.vue'
import type { AssistantMessage, AssistantToolCall, AssistantApproval, AssistantResultCard } from '@/api/assistant'

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
const expandedToolResults = ref<Record<string, boolean>>({})
const TOOL_RESULT_PREVIEW_LENGTH = 900
let motionContext: ReturnType<typeof gsap.context> | null = null
let motionMedia: ReturnType<typeof gsap.matchMedia> | null = null

type StepResult = { title: string; summary: string }
type AssistantSegment =
  | { type: 'thinking'; key: string; steps: string[] }
  | { type: 'content'; key: string; content: string }
  | { type: 'tool'; key: string; toolCall: AssistantToolCall }
  | { type: 'approvals'; key: string; approvals: AssistantApproval[] }
  | { type: 'step-results'; key: string; results: StepResult[] }
  | { type: 'result-cards'; key: string; cards: AssistantResultCard[] }

function scrollToBottom() {
  nextTick(() => {
    if (containerRef.value) {
      containerRef.value.scrollTop = containerRef.value.scrollHeight
    }
  })
}

watch(() => props.messages, scrollToBottom, { deep: true })
watch(() => props.streaming, scrollToBottom)

function getThinkingSteps(msg: AssistantMessage): string[] {
  // 支持数组格式（新）和字符串格式（旧）
  if (Array.isArray(msg.thinking)) {
    return msg.thinking.filter(Boolean)
  }
  return (msg.thinking || '')
    .split('\n')
    .map(step => step.trim())
    .filter(Boolean)
}

function getMessageToolCalls(msg: AssistantMessage): AssistantToolCall[] {
  if (msg.tool_calls?.length) return msg.tool_calls
  return props.toolCalls.filter(tc => tc.message_id === msg.message_id || tc.message_id === msg.id)
}

function getStepResults(msg: AssistantMessage): StepResult[] {
  if (!msg.plan?.steps) return []
  return msg.plan.steps
    .filter(step => step.result_summary)
    .map(step => ({
      title: step.title,
      summary: step.result_summary || ''
    }))
}

function getAssistantSegments(msg: AssistantMessage): AssistantSegment[] {
  const segments: AssistantSegment[] = []
  const baseKey = msg.message_id || msg.id

  getThinkingSteps(msg).forEach((step, index) => {
    segments.push({
      type: 'thinking',
      key: `${baseKey}-thinking-${index}`,
      steps: [step],
    })
  })

  if (msg.content) {
    segments.push({
      type: 'content',
      key: `${baseKey}-content`,
      content: msg.content,
    })
  }

  for (const toolCall of getMessageToolCalls(msg)) {
    segments.push({
      type: 'tool',
      key: `${baseKey}-tool-${toolCall.call_id || toolCall.id}`,
      toolCall,
    })
  }

  if (msg.approvals?.length) {
    segments.push({
      type: 'approvals',
      key: `${baseKey}-approvals`,
      approvals: msg.approvals,
    })
  }

  const stepResults = getStepResults(msg)
  if (stepResults.length) {
    segments.push({
      type: 'step-results',
      key: `${baseKey}-step-results`,
      results: stepResults,
    })
  }

  if (msg.result_cards?.length) {
    segments.push({
      type: 'result-cards',
      key: `${baseKey}-result-cards`,
      cards: msg.result_cards,
    })
  }

  return segments
}

function getToolResultKey(toolCall: AssistantToolCall): string {
  return toolCall.call_id || toolCall.id
}

function formatToolResultValue(value: unknown): { text: string; isJson: boolean } {
  if (value === undefined || value === null) return { text: '', isJson: false }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if ((trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
      try {
        return { text: JSON.stringify(JSON.parse(trimmed), null, 2), isJson: true }
      } catch {
        return { text: value, isJson: false }
      }
    }
    return { text: value, isJson: false }
  }

  if (typeof value === 'object') {
    try {
      return { text: JSON.stringify(value, null, 2), isJson: true }
    } catch {
      return { text: String(value), isJson: false }
    }
  }

  return { text: String(value), isJson: false }
}

function getToolResultView(toolCall: AssistantToolCall): { text: string; isJson: boolean } {
  if (toolCall.error_message) return { text: toolCall.error_message, isJson: false }
  const resultView = formatToolResultValue(toolCall.result)
  if (resultView.text) return resultView
  return formatToolResultValue(toolCall.result_summary || '')
}

function getToolResultText(toolCall: AssistantToolCall): string {
  return getToolResultView(toolCall).text
}

function isJsonToolResult(toolCall: AssistantToolCall): boolean {
  return getToolResultView(toolCall).isJson
}

function isLongToolResult(toolCall: AssistantToolCall): boolean {
  return getToolResultText(toolCall).length > TOOL_RESULT_PREVIEW_LENGTH
}

function isToolResultExpanded(toolCall: AssistantToolCall): boolean {
  return Boolean(expandedToolResults.value[getToolResultKey(toolCall)])
}

function getDisplayedToolResult(toolCall: AssistantToolCall): string {
  const result = getToolResultText(toolCall)
  if (!isLongToolResult(toolCall) || isToolResultExpanded(toolCall)) return result
  return `${result.slice(0, TOOL_RESULT_PREVIEW_LENGTH)}\n...`
}

function toggleToolResult(toolCall: AssistantToolCall) {
  const key = getToolResultKey(toolCall)
  expandedToolResults.value[key] = !expandedToolResults.value[key]
}

function getToolStatusType(status: string): '' | 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'completed':
    case 'success':
      return 'success'
    case 'running':
      return 'warning'
    case 'failed':
      return 'danger'
    default:
      return 'info'
  }
}

function formatContent(content: string): string {
  return content
    .replace(/\n/g, '<br>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/`(.*?)`/g, '<code>$1</code>')
}

function animateNewContent() {
  if (!containerRef.value || typeof window === 'undefined') return

  nextTick(() => {
    const root = containerRef.value
    if (!root) return
    const targets = Array.from(root.querySelectorAll<HTMLElement>('.message-group:not([data-motion-ready])'))
    const thinkingTargets = Array.from(root.querySelectorAll<HTMLElement>('.thinking-block:not([data-motion-ready])'))
    targets.forEach(el => { el.dataset.motionReady = 'true' })
    thinkingTargets.forEach(el => { el.dataset.motionReady = 'true' })
    if (!targets.length && !thinkingTargets.length) return

    motionContext?.add(() => {
      if (targets.length) {
        gsap.from(targets, {
          autoAlpha: 0,
          y: 12,
          duration: 0.26,
          ease: 'power2.out',
          stagger: 0.03,
        })
      }
      if (thinkingTargets.length) {
        gsap.from(thinkingTargets, {
          autoAlpha: 0,
          y: 8,
          duration: 0.24,
          ease: 'power1.out',
          stagger: 0.02,
        })
      }
    })
  })
}

function setupConversationMotion() {
  if (!containerRef.value || typeof window === 'undefined') return
  motionContext = gsap.context(() => {
    motionMedia = gsap.matchMedia()
    motionMedia.add('(prefers-reduced-motion: reduce)', () => {
      gsap.set('.message-group, .thinking-block', { clearProps: 'all' })
    })
    motionMedia.add('(prefers-reduced-motion: no-preference)', () => {
      animateNewContent()
    })
  }, containerRef.value)
}

watch(() => props.messages.length, animateNewContent)
watch(() => props.messages.map(message => message.thinking || '').join('|'), animateNewContent)

onMounted(setupConversationMotion)
onUnmounted(() => {
  motionMedia?.revert()
  motionContext?.revert()
  motionMedia = null
  motionContext = null
})

</script>

<style scoped>
.conversation-container {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  background: linear-gradient(180deg, #f8fafc 0%, #f9fafb 100%);
}

.message-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 980px;
  margin: 0 auto;
  width: 100%;
}

.message-group {
  display: flex;
  flex-direction: column;
  gap: 10px;
  will-change: transform, opacity;
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
  max-width: min(82%, 820px);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.message-bubble {
  background: #fff;
  padding: 12px 16px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.05);
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
  background: #f8fbff;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  padding: 0;
  overflow: hidden;
  will-change: transform, opacity;
}

.thinking-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  font-size: 13px;
  color: #1d4ed8;
  font-weight: 600;
  cursor: pointer;
  list-style: none;
  user-select: none;
}

.thinking-header::-webkit-details-marker {
  display: none;
}

.thinking-pulse {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #3b82f6;
  box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.35);
  animation: thinkingPulse 1.8s ease-out infinite;
  flex-shrink: 0;
}

.thinking-content {
  padding: 0 14px 12px 14px;
  font-size: 13px;
  color: #334155;
  line-height: 1.65;
}

.thinking-step {
  padding: 4px 0;
  border-bottom: 1px solid #e5e7eb;
}

.thinking-step:last-child {
  border-bottom: none;
}

.step-results {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.step-result-card {
  background: #f0fdf4;
  border: 1px solid #86efac;
  border-radius: 8px;
  padding: 12px;
}

.step-result-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #166534;
  margin-bottom: 8px;
}

.step-result-header .el-icon {
  color: #22c55e;
}

.step-result-content {
  font-size: 13px;
  color: #334155;
  line-height: 1.65;
  white-space: pre-wrap;
}

.tool-calls {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tool-call-card {
  background: #fffbeb;
  border: 1px solid #fcd34d;
  border-radius: 8px;
  padding: 10px 12px;
}

.tool-call-card.status-completed,
.tool-call-card.status-success {
  background: #f0fdf4;
  border-color: #86efac;
}

.tool-call-card.status-failed {
  background: #fef2f2;
  border-color: #fca5a5;
}

.tool-call-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #92400e;
}

.tool-call-card.status-completed .tool-call-header,
.tool-call-card.status-success .tool-call-header {
  color: #166534;
}

.tool-call-card.status-failed .tool-call-header {
  color: #991b1b;
}

.tool-call-header .el-icon {
  font-size: 14px;
}

.tool-name {
  flex: 1;
}

.tool-call-result {
  margin-top: 8px;
  font-size: 12px;
  color: #64748b;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.tool-call-result.is-json {
  padding: 10px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #f8fafc;
  color: #334155;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  overflow-x: auto;
}

.tool-result-toggle {
  margin-top: 8px;
  padding: 0;
  border: none;
  background: transparent;
  color: #2563eb;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.tool-result-toggle:hover {
  color: #1d4ed8;
}

@keyframes thinkingPulse {
  0% {
    box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.35);
  }
  70% {
    box-shadow: 0 0 0 8px rgba(59, 130, 246, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(59, 130, 246, 0);
  }
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
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.05);
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

@media (max-width: 900px) {
  .conversation-container {
    padding: 16px;
  }

  .message-body {
    max-width: calc(100% - 48px);
  }
}
</style>

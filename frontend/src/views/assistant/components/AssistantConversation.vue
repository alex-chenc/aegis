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
            <div
              class="message-body"
              :class="{ 'message-body-report': segment.type === 'content' && isSecurityConclusionContent(segment.content) }"
            >
              <!-- 思考步骤 -->
              <div v-if="segment.type === 'thinking'" class="thinking-block">
                <div class="thinking-header">
                  <span class="thinking-pulse" aria-hidden="true"></span>
                  <span>{{ $t('generated.assistantAssistantConversation_think_a6c149') }}</span>
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
              <template v-else-if="segment.type === 'content'">
                <AssistantConclusionCard
                  v-if="isSecurityConclusionContent(segment.content)"
                  :content="segment.content"
                />
                <div v-else class="message-bubble">
                  <div class="message-content" v-html="formatContent(segment.content)"></div>
                </div>
              </template>

              <!-- 工具调用和执行结果放在同一个框内 -->
              <div v-else-if="segment.type === 'tool'" class="tool-calls">
                <AssistantToolResultCard :tool-call="segment.toolCall" />
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
                  :class="`step-result-${result.status}`"
                >
                  <div class="step-result-header">
                    <el-icon>
                      <CircleClose v-if="result.status === 'failed'" />
                      <RemoveFilled v-else-if="result.status === 'skipped'" />
                      <Refresh v-else-if="result.status === 'retrying'" />
                      <CircleCheck v-else />
                    </el-icon>
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
import { translate } from '@/i18n'

import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { User, Monitor, InfoFilled, CircleCheck, CircleClose, RemoveFilled, Refresh } from '@element-plus/icons-vue'
import { gsap } from 'gsap'
import AssistantApprovalCard from './AssistantApprovalCard.vue'
import AssistantConclusionCard from './AssistantConclusionCard.vue'
import AssistantResultRenderer from './AssistantResultRenderer.vue'
import AssistantToolResultCard from './AssistantToolResultCard.vue'
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
let motionContext: ReturnType<typeof gsap.context> | null = null
let motionMedia: ReturnType<typeof gsap.matchMedia> | null = null

type StepResultStatus = 'completed' | 'failed' | 'skipped' | 'retrying'

type StepResult = { key: string; title: string; summary: string; status: StepResultStatus }
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
  const normalize = (steps: string[]) => steps
    .map(step => String(step).trim())
    .filter(Boolean)
    .filter(step => !isHiddenInternalThinkingStep(step))

  if (Array.isArray(msg.thinking)) {
    return normalize(msg.thinking)
  }
  return normalize((msg.thinking || '')
    .split('\n')
  )
}

function isHiddenInternalThinkingStep(step: string) {
  return /^(?:正在反思执行过程|Reflecting on execution)/i.test(step) ||
    /^(?:反思结果|Reflection result)[:：]/i.test(step) ||
    /^(?:步骤失败|Step failed)[:：]/i.test(step) ||
    /^(?:正在重试步骤|Retrying step)[:：]/i.test(step) ||
    /^(?:正在审计执行进度|Auditing execution progress)/i.test(step) ||
    /^(?:审计完成|Audit complete)(?:[:：]|$)/i.test(step)
}

function getMessageToolCalls(msg: AssistantMessage): AssistantToolCall[] {
  const related = props.toolCalls.filter(tc =>
    tc.message_id === msg.message_id ||
    tc.message_id === msg.id ||
    msg.tool_calls?.some(item => item.call_id === tc.call_id || item.id === tc.id)
  )
  const byKey = new Map<string, AssistantToolCall>()
  for (const tc of msg.tool_calls || []) {
    byKey.set(getToolCallKey(tc), tc)
  }
  for (const tc of related) {
    const existing = byKey.get(getToolCallKey(tc))
    byKey.set(getToolCallKey(tc), existing ? { ...existing, ...tc } : tc)
  }
  return Array.from(byKey.values()).sort((left, right) => {
    const leftTime = new Date(left.created_at || '').getTime() || 0
    const rightTime = new Date(right.created_at || '').getTime() || 0
    return leftTime - rightTime
  })
}

function getMessageApprovals(msg: AssistantMessage, toolCalls: AssistantToolCall[]): AssistantApproval[] {
  const byKey = new Map<string, AssistantApproval>()
  for (const approval of msg.approvals || []) {
    byKey.set(approval.approval_id || approval.id, approval)
  }
  const messageToolCallIds = new Set(toolCalls.flatMap(toolCall =>
    [toolCall.call_id, toolCall.id].filter(Boolean)
  ))
  for (const approval of props.approvals) {
    if (
      approval.session_id === msg.session_id &&
      (
        approval.tool_call_id && messageToolCallIds.has(approval.tool_call_id) ||
        msg.approvals?.some(item => item.approval_id === approval.approval_id || item.id === approval.id)
      )
    ) {
      byKey.set(approval.approval_id || approval.id, approval)
    }
  }
  return Array.from(byKey.values()).sort((left, right) => {
    const leftTime = new Date(left.created_at || '').getTime() || 0
    const rightTime = new Date(right.created_at || '').getTime() || 0
    return leftTime - rightTime
  })
}

function getToolCallKey(toolCall: AssistantToolCall): string {
  return toolCall.call_id || toolCall.id
}

function matchToolCallThinkingStep(step: string): string | null {
  const match = step.match(/^(?:正在调用工具|Calling tool)[:：]\s*(.+)$/i)
  return match?.[1]?.trim() || null
}

function matchStepCompletedThinkingStep(step: string): string | null {
  const match = step.match(/^(?:步骤完成|Step completed)[:：]\s*(.+)$/i)
  return match?.[1]?.trim() || null
}

function isHistoryDisplayMessage(msg: AssistantMessage) {
  return msg.message_id.includes('_history_')
}

function shouldRenderStepResults(msg: AssistantMessage): boolean {
  if (!msg.plan?.steps) return false
  const hasCompletedStepThinking = getThinkingSteps(msg).some(step => Boolean(matchStepCompletedThinkingStep(step)))
  return !(isHistoryDisplayMessage(msg) && Boolean(msg.content) && !hasCompletedStepThinking)
}

function isToolCallDisplaySettled(toolCall: AssistantToolCall): boolean {
  return [
    'accepted',
    'completed',
    'success',
    'failed',
    'blocked',
    'cancelled',
    'approval_required',
    'rejected',
  ].includes(toolCall.status)
}

function getStepResults(msg: AssistantMessage): StepResult[] {
  if (!shouldRenderStepResults(msg)) return []
  const plan = msg.plan
  if (!plan) return []
  const terminalStatuses: string[] = ['completed', 'failed', 'skipped', 'retrying']
  return plan.steps
    .filter(step => step.result_summary || terminalStatuses.includes(step.status))
    .map(step => {
      const status = (terminalStatuses.includes(step.status) ? step.status : 'completed') as StepResultStatus
      return {
        key: step.step_id || step.title,
        title: step.title,
        summary: step.result_summary || defaultStepSummary(status),
        status,
      }
    })
}

function defaultStepSummary(status: StepResultStatus): string {
  switch (status) {
    case 'failed':
      return translate('assistant.step.failed')
    case 'skipped':
      return translate('assistant.step.skipped')
    case 'retrying':
      return translate('assistant.step.retrying')
    default:
      return translate('assistant.step.completed')
  }
}

function getAssistantSegments(msg: AssistantMessage): AssistantSegment[] {
  const segments: AssistantSegment[] = []
  const baseKey = msg.message_id || msg.id
  const toolCalls = getMessageToolCalls(msg)
  const approvals = getMessageApprovals(msg, toolCalls)
  const stepResults = getStepResults(msg)
  const messageIndex = props.messages.findIndex(item =>
    item.id === msg.id &&
    item.message_id === msg.message_id
  )
  const nextAssistantMessage = messageIndex >= 0
    ? props.messages.slice(messageIndex + 1).find(item => item.role === 'assistant')
    : undefined
  const nextMessageToolCalls = nextAssistantMessage ? getMessageToolCalls(nextAssistantMessage) : []
  const usedToolCalls = new Set<string>()
  const usedStepResults = new Set<string>()
  let blockedByPendingTool = false

  const pushThinkingStep = (step: string, index: number) => {
    segments.push({
      type: 'thinking',
      key: `${baseKey}-thinking-${index}`,
      steps: [step],
    })
  }

  const pushToolCall = (toolCall: AssistantToolCall) => {
    const key = getToolCallKey(toolCall)
    usedToolCalls.add(key)
    segments.push({
      type: 'tool',
      key: `${baseKey}-tool-${key}`,
      toolCall,
    })
    if (!isToolCallDisplaySettled(toolCall)) {
      blockedByPendingTool = true
    }
  }

  const pushStepResult = (stepResult: StepResult, index: number) => {
    usedStepResults.add(stepResult.key)
    segments.push({
      type: 'step-results',
      key: `${baseKey}-step-result-${index}-${stepResult.key}`,
      results: [stepResult],
    })
  }

  const takeToolCall = (toolName: string) => {
    const normalizedName = toolName.trim()
    const match = toolCalls.find(toolCall =>
      !usedToolCalls.has(getToolCallKey(toolCall)) &&
      toolCall.tool_name === normalizedName
    )
    return match
  }

  const hasMatchingToolCallInNextMessage = (toolName: string) => {
    const normalizedName = toolName.trim()
    return nextMessageToolCalls.some(toolCall => toolCall.tool_name === normalizedName)
  }

  const takeStepResult = (stepTitle: string, index: number): StepResult => {
    const normalizedTitle = stepTitle.trim()
    const match = stepResults.find(result =>
      !usedStepResults.has(result.key) &&
      result.title === normalizedTitle
    )
    if (match) return match

    return {
      key: `fallback-${index}-${normalizedTitle}`,
      title: normalizedTitle,
      summary: translate('generatedScript.assistantAssistantConversation_completed_steps_00a764', { p0: normalizedTitle }),
      status: 'completed' as StepResultStatus,
    }
  }

  getThinkingSteps(msg).forEach((step, index) => {
    if (blockedByPendingTool) return

    const toolName = matchToolCallThinkingStep(step)
    if (toolName) {
      const toolCall = takeToolCall(toolName)
      if (toolCall) {
        pushThinkingStep(step, index)
        pushToolCall(toolCall)
      } else if (hasMatchingToolCallInNextMessage(toolName)) {
        pushThinkingStep(step, index)
      } else if (!msg.content) {
        blockedByPendingTool = true
      }
      return
    }

    pushThinkingStep(step, index)
    const completedStepTitle = matchStepCompletedThinkingStep(step)
    if (completedStepTitle) {
      pushStepResult(takeStepResult(completedStepTitle, index), index)
    }
  })

  if (!blockedByPendingTool) {
    for (const toolCall of toolCalls) {
      if (usedToolCalls.has(getToolCallKey(toolCall))) continue
      pushToolCall(toolCall)
      if (blockedByPendingTool) break
    }
  }

  if (!blockedByPendingTool) {
    if (approvals.length) {
      segments.push({
        type: 'approvals',
        key: `${baseKey}-approvals`,
        approvals,
      })
    }

    const remainingStepResults = stepResults.filter(result => !usedStepResults.has(result.key))
    if (remainingStepResults.length) {
      segments.push({
        type: 'step-results',
        key: `${baseKey}-step-results`,
        results: remainingStepResults,
      })
    }

    if (msg.content) {
      segments.push({
        type: 'content',
        key: `${baseKey}-content`,
        content: msg.content,
      })
    }

    if (msg.result_cards?.length) {
      segments.push({
        type: 'result-cards',
        key: `${baseKey}-result-cards`,
        cards: msg.result_cards,
      })
    }
  }

  return segments
}

function formatContent(content: string): string {
  return escapeHtml(content)
    .replace(/\n/g, '<br>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
}

function isSecurityConclusionContent(content: string): boolean {
  const normalized = String(content || '')
  const structuredHeadings = normalized.match(/^#{2,3}\s+(?:结论|具体高风险|高风险项|处置建议|证据边界|Conclusion|Specific high-risk items|Recommended actions|Evidence limits)/gim)
  if ((structuredHeadings?.length || 0) >= 2) return true
  return normalized.length >= 100 && /(?:安全风险|高风险|risk)/i.test(normalized) && /(?:建议|recommend)/i.test(normalized)
}

function escapeHtml(content: string): string {
  return content
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
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
  padding: 18px 24px;
  background: linear-gradient(180deg, #f8fafc 0%, #f9fafb 100%);
}

.message-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 980px;
  margin: 0 auto;
  width: 100%;
}

.message-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
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
  gap: 6px;
}

.message-body.message-body-report {
  max-width: min(92%, 920px);
  width: min(92%, 920px);
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
  padding: 9px 12px;
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
  padding: 0 14px 10px 14px;
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
  padding: 10px 12px;
}

.step-result-card.step-result-failed {
  background: #fef2f2;
  border-color: #fca5a5;
}

.step-result-card.step-result-failed .step-result-header {
  color: #991b1b;
}

.step-result-card.step-result-failed .step-result-header .el-icon {
  color: #ef4444;
}

.step-result-card.step-result-skipped {
  background: #f9fafb;
  border-color: #d1d5db;
}

.step-result-card.step-result-skipped .step-result-header {
  color: #4b5563;
}

.step-result-card.step-result-skipped .step-result-header .el-icon {
  color: #9ca3af;
}

.step-result-card.step-result-retrying {
  background: #fffbeb;
  border-color: #fcd34d;
}

.step-result-card.step-result-retrying .step-result-header {
  color: #92400e;
}

.step-result-card.step-result-retrying .step-result-header .el-icon {
  color: #f59e0b;
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
  max-height: 260px;
  overflow-y: auto;
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

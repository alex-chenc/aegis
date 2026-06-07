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
            <details v-if="msg.thinking" class="thinking-block" open>
              <summary class="thinking-header">
                <span class="thinking-pulse" aria-hidden="true"></span>
                <span>思考步骤</span>
                <el-tag size="small" effect="plain">{{ getThinkingSteps(msg).length }} 步</el-tag>
              </summary>
              <ol class="thinking-steps">
                <li
                  v-for="(step, index) in getThinkingSteps(msg)"
                  :key="`${msg.message_id}-thinking-${index}`"
                >
                  {{ step }}
                </li>
              </ol>
            </details>

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
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { User, Monitor, InfoFilled } from '@element-plus/icons-vue'
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
let motionContext: ReturnType<typeof gsap.context> | null = null
let motionMedia: ReturnType<typeof gsap.matchMedia> | null = null

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
  return (msg.thinking || '')
    .split('\n')
    .map(step => step.trim())
    .filter(Boolean)
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

.thinking-steps {
  margin: 0;
  padding: 0 14px 12px 34px;
  font-size: 13px;
  color: #334155;
  line-height: 1.65;
}

.thinking-steps li + li {
  margin-top: 4px;
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

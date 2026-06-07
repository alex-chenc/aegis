<template>
  <div class="context-budget-indicator" v-if="visible">
    <el-popover
      placement="bottom-end"
      :width="280"
      trigger="hover"
      :show-after="200"
    >
      <template #reference>
        <div class="ring-container" :class="statusClass">
          <svg :width="size" :height="size" viewBox="0 0 36 36">
            <circle
              class="ring-bg"
              cx="18" cy="18" r="15.915"
              fill="none"
              :stroke-width="strokeWidth"
            />
            <circle
              class="ring-fill"
              cx="18" cy="18" r="15.915"
              fill="none"
              :stroke-width="strokeWidth"
              :stroke-dasharray="dashArray"
              :stroke-dashoffset="dashOffset"
              :stroke="ringColor"
              stroke-linecap="round"
            />
          </svg>
          <span class="ring-label">{{ percentageLabel }}</span>
        </div>
      </template>

      <div class="budget-popover">
        <div class="popover-title">上下文预算</div>
        <div class="popover-row">
          <span class="popover-key">使用率</span>
          <span class="popover-value" :class="statusClass">{{ percentageLabel }}</span>
        </div>
        <div class="popover-row">
          <span class="popover-key">已用 Tokens</span>
          <span class="popover-value">{{ formatTokens(promptTokensUsed) }}</span>
        </div>
        <div class="popover-row">
          <span class="popover-key">可用 Tokens</span>
          <span class="popover-value">{{ formatTokens(availableTokens) }}</span>
        </div>
        <div class="popover-row">
          <span class="popover-key">最大上下文</span>
          <span class="popover-value">{{ formatTokens(budget.max_context_tokens) }}</span>
        </div>
        <div class="popover-row">
          <span class="popover-key">预留输出</span>
          <span class="popover-value">{{ formatTokens(budget.reserved_output_tokens) }}</span>
        </div>
        <div v-if="compressionCount > 0" class="popover-divider" />
        <div v-if="compressionCount > 0" class="popover-row">
          <span class="popover-key">压缩次数</span>
          <span class="popover-value">{{ compressionCount }} 次</span>
        </div>
        <div v-if="totalTokensUsed > 0" class="popover-row">
          <span class="popover-key">总 Tokens</span>
          <span class="popover-value">{{ formatTokens(totalTokensUsed) }}</span>
        </div>
      </div>
    </el-popover>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ContextBudgetEvent, ContextCompressedEvent } from '@/api/aiAnalysis'

const props = withDefaults(defineProps<{
  budget?: ContextBudgetEvent | null
  compressionRecords?: ContextCompressedEvent[]
  totalPromptTokens?: number
  totalCompletionTokens?: number
  size?: number
  strokeWidth?: number
}>(), {
  budget: null,
  compressionRecords: () => [],
  totalPromptTokens: 0,
  totalCompletionTokens: 0,
  size: 36,
  strokeWidth: 3,
})

const visible = computed(() => props.budget != null)

const promptTokensUsed = computed(() => {
  if (!props.budget) return 0
  const estimated = Number(props.budget.estimated_prompt_tokens || 0)
  const observed = Number(props.budget.prompt_tokens_observed || 0)
  const snapshotPrompt = Math.max(estimated, observed)
  if (snapshotPrompt <= 64 && props.totalPromptTokens > 64) {
    return props.totalPromptTokens
  }
  return snapshotPrompt
})

const ratio = computed(() => {
  if (!props.budget || props.budget.max_context_tokens <= 0) return 0
  return Math.min((promptTokensUsed.value + props.budget.reserved_output_tokens) / props.budget.max_context_tokens, 1.0)
})

const percentage = computed(() => Math.round(ratio.value * 100))

const percentageLabel = computed(() => `${percentage.value}%`)

const statusClass = computed(() => {
  if (percentage.value >= 90) return 'status-critical'
  if (percentage.value >= 70) return 'status-warning'
  return 'status-ok'
})

const availableTokens = computed(() => {
  if (!props.budget) return 0
  return Math.max(0, props.budget.max_context_tokens - promptTokensUsed.value - props.budget.reserved_output_tokens)
})

const circumference = 2 * Math.PI * 15.915

const dashArray = computed(() => `${circumference}`)

const dashOffset = computed(() => {
  const filled = ratio.value * circumference
  return `${circumference - filled}`
})

const ringColor = computed(() => {
  if (percentage.value >= 90) return 'var(--el-color-danger)'
  if (percentage.value >= 70) return 'var(--el-color-warning)'
  return 'var(--el-color-success)'
})

const compressionCount = computed(() => props.compressionRecords?.length ?? 0)

const totalTokensUsed = computed(() => props.totalPromptTokens + props.totalCompletionTokens)

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}
</script>

<style scoped>
.context-budget-indicator {
  display: inline-flex;
  align-items: center;
}

.ring-container {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.ring-container svg {
  transform: rotate(-90deg);
}

.ring-bg {
  stroke: var(--el-border-color-lighter, #e4e7ed);
}

.ring-label {
  position: absolute;
  font-size: 9px;
  font-weight: 600;
  line-height: 1;
  color: var(--el-text-color-regular);
}

.status-critical .ring-label {
  color: var(--el-color-danger);
}

.status-warning .ring-label {
  color: var(--el-color-warning);
}

.status-ok .ring-label {
  color: var(--el-color-success);
}

.budget-popover {
  font-size: 13px;
}

.popover-title {
  font-weight: 600;
  font-size: 14px;
  margin-bottom: 8px;
  color: var(--el-text-color-primary);
}

.popover-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 3px 0;
}

.popover-key {
  color: var(--el-text-color-secondary);
}

.popover-value {
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.popover-value.status-critical {
  color: var(--el-color-danger);
}

.popover-value.status-warning {
  color: var(--el-color-warning);
}

.popover-value.status-ok {
  color: var(--el-color-success);
}

.popover-divider {
  height: 1px;
  background: var(--el-border-color-lighter);
  margin: 6px 0;
}
</style>

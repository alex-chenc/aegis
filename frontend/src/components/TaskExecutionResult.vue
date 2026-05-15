<template>
  <div v-if="displayResult" class="task-execution-result">
    <!-- 结论卡片 - 仅显示结论 -->
    <div v-if="displayResult.conclusion" class="conclusion-section">
      <div class="section-title">
        <el-icon><Document /></el-icon>
        分析结论
      </div>
      <div class="conclusion-card" :class="displayResult.conclusion.verdict">
        <div class="conclusion-header">
          <el-tag :type="getVerdictType(displayResult.conclusion.verdict)" size="large">
            {{ getVerdictText(displayResult.conclusion.verdict) }}
          </el-tag>
        </div>
        <div v-if="displayResult.conclusion.summary" class="conclusion-summary">
          {{ displayResult.conclusion.summary }}
        </div>

        <!-- 处置建议框 - 仅非误报场景显示 -->
        <div v-if="!isFalsePositive(displayResult.conclusion.verdict)" class="remediation-suggestion">
          <el-alert
            title="处置建议"
            type="warning"
            :closable="false"
            show-icon
          >
            <template #default>
              <p>{{ getRemediationSuggestion(displayResult.conclusion.verdict) }}</p>
            </template>
          </el-alert>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Document } from '@element-plus/icons-vue'
import type { ExecutionResult } from '@/api/aiAnalysis'
import { normalizeExecutionResult } from '@/utils/taskExecutionResult'
import { isFalsePositive, getRemediationSuggestion, getVerdictType, getVerdictText } from '@/utils/sessionStatus'

const props = defineProps<{
  result: ExecutionResult | null
}>()

const displayResult = computed(() => {
  return props.result ? normalizeExecutionResult(props.result) : null
})
</script>

<style scoped>
.task-execution-result {
  padding: 16px;
}

.conclusion-section {
  margin-bottom: 24px;
}

.section-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.conclusion-card {
  border: 1px solid var(--el-border-color-lighter);
  border-left: 4px solid var(--el-color-info);
  border-radius: 8px;
  padding: 16px;
  background: var(--el-bg-color);
}

.conclusion-card.benign,
.conclusion-card.false_positive {
  border-left-color: var(--el-color-success);
}

.conclusion-card.malicious {
  border-left-color: var(--el-color-danger);
}

.conclusion-card.suspicious {
  border-left-color: var(--el-color-warning);
}

.conclusion-header {
  margin-bottom: 12px;
}

.conclusion-summary {
  font-size: 16px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin-bottom: 16px;
}

.remediation-suggestion {
  margin-top: 16px;
}

.remediation-suggestion p {
  margin: 0;
  font-size: 14px;
  line-height: 1.6;
  color: var(--el-text-color-regular);
}
</style>

<template>
  <section class="recovery-card" :class="`risk-${recovery.risk_level}`">
    <header class="recovery-header">
      <div>
        <div class="recovery-eyebrow">需要您的选择</div>
        <h4>{{ recovery.summary }}</h4>
      </div>
      <el-tag :type="riskTagType" effect="dark">{{ riskLabel }}</el-tag>
    </header>

    <p v-if="recovery.detail" class="recovery-detail">{{ recovery.detail }}</p>

    <div v-if="requiredHooks.length" class="hook-diff">
      <div class="hook-diff-title">建议新增的 Hook</div>
      <code v-for="hook in requiredHooks" :key="`${hook.attach_type}:${hook.attach}`">
        {{ hook.attach_type }} · {{ hook.attach }}
      </code>
    </div>

    <div v-if="proposalMessage" class="proposal-result">{{ proposalMessage }}</div>

    <div class="safety-note">
      只有您明确确认后才会修改安全配置；未列出的动作不会执行。
    </div>

    <div class="recovery-actions">
      <el-button
        v-for="action in actions"
        :key="action.id"
        :type="buttonType(action)"
        :plain="action.risk_level !== 'high' && action.risk_level !== 'critical'"
        :loading="busy && selectedAction === action.id"
        :disabled="busy"
        @click="selectAction(action)"
      >
        {{ action.label }}
      </el-button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import type {
  AssistantRecoveryAction,
  AssistantRecoveryRequest,
} from '@/api/assistant'

const props = defineProps<{
  recovery: AssistantRecoveryRequest
  busy?: boolean
}>()

const emit = defineEmits<{
  decide: [recoveryId: string, actionId: string, input?: Record<string, any>]
}>()

const selectedAction = ref('')

const actions = computed<AssistantRecoveryAction[]>(() => {
  const value = props.recovery.actions as any
  if (Array.isArray(value)) return value
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
})

const recoveryContext = computed<Record<string, any>>(() => {
  const value = props.recovery.context as any
  if (value && typeof value === 'object') return value
  if (typeof value === 'string') {
    try {
      return JSON.parse(value)
    } catch {
      return {}
    }
  }
  return {}
})

const requiredHooks = computed<Array<{ attach_type: string; attach: string }>>(() =>
  Array.isArray(recoveryContext.value.required_hooks)
    ? recoveryContext.value.required_hooks
    : []
)

const resolutionResult = computed<Record<string, any>>(() => {
  const value = props.recovery.resolution_result as any
  if (value && typeof value === 'object') return value
  if (typeof value === 'string') {
    try {
      return JSON.parse(value)
    } catch {
      return {}
    }
  }
  return {}
})

const proposalMessage = computed(() => {
  if (resolutionResult.value.manual_review_required) {
    return `变更建议需要人工复核：${resolutionResult.value.validation_error || '缺少可验证的 Hook 信息'}`
  }
  const added = resolutionResult.value.added_hooks
  if (Array.isArray(added)) {
    return added.length
      ? `已生成变更建议：新增 ${added.length} 个 Hook。您仍可选择执行、继续暂停或取消。`
      : '当前白名单已包含建议 Hook。您仍可选择重新同步并继续原任务。'
  }
  return ''
})

const riskLabel = computed(() => {
  const labels: Record<string, string> = {
    readonly: '只读',
    low: '低风险',
    medium: '中风险',
    high: '高风险',
    critical: '关键风险',
  }
  return labels[props.recovery.risk_level] || props.recovery.risk_level
})

const riskTagType = computed(() =>
  props.recovery.risk_level === 'critical' || props.recovery.risk_level === 'high'
    ? 'danger'
    : props.recovery.risk_level === 'medium'
      ? 'warning'
      : 'info'
)

function buttonType(action: AssistantRecoveryAction) {
  if (action.id === 'cancel') return 'danger'
  if (action.risk_level === 'critical' || action.risk_level === 'high') return 'warning'
  if (action.id === 'pause') return 'info'
  return 'primary'
}

async function selectAction(action: AssistantRecoveryAction) {
  try {
    let input: Record<string, any> | undefined
    if (action.input_required) {
      const result = await ElMessageBox.prompt(
        '请说明您希望如何处理。该说明会被记录，但系统不会据此执行未声明的动作。',
        action.label,
        {
          confirmButtonText: '提交',
          cancelButtonText: '取消',
          inputType: 'textarea',
          inputValidator: value => Boolean(String(value || '').trim()) || '请输入处理说明',
        }
      )
      input = { comment: String(result.value || '').trim() }
    } else if (action.confirmation_required) {
      await ElMessageBox.confirm(
        action.description || '该操作会改变安全配置，是否继续？',
        action.label,
        {
          type: 'warning',
          confirmButtonText: '确认执行',
          cancelButtonText: '取消',
          distinguishCancelAndClose: true,
        }
      )
    }
    selectedAction.value = action.id
    emit('decide', props.recovery.recovery_id, action.id, input)
  } catch {
    // Closing or cancelling the confirmation does not change recovery state.
  }
}
</script>

<style scoped>
.recovery-card {
  border: 1px solid #e6a23c;
  border-left-width: 4px;
  border-radius: 10px;
  padding: 14px 16px;
  background: #fffaf0;
  box-shadow: 0 4px 14px rgb(0 0 0 / 6%);
}

.recovery-card.risk-critical,
.recovery-card.risk-high {
  border-color: #e6a23c;
}

.recovery-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.recovery-eyebrow {
  margin-bottom: 4px;
  color: #b36b00;
  font-size: 12px;
  font-weight: 600;
}

h4 {
  margin: 0;
  color: #303133;
  font-size: 15px;
}

.recovery-detail {
  margin: 10px 0 0;
  color: #606266;
  font-size: 13px;
  line-height: 1.6;
}

.hook-diff {
  display: grid;
  gap: 6px;
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  background: #f5f7fa;
}

.hook-diff-title {
  color: #606266;
  font-size: 12px;
  font-weight: 600;
}

.hook-diff code {
  color: #303133;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.safety-note {
  margin-top: 10px;
  color: #909399;
  font-size: 12px;
}

.proposal-result {
  margin-top: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  color: #606266;
  background: #ecf5ff;
  font-size: 12px;
}

.recovery-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
}

.recovery-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}
</style>

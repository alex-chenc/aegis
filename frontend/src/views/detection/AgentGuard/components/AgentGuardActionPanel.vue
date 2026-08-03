<template>
  <section class="guard-action-panel" aria-live="polite">
    <header>
      <div>
        <h3>{{ t('agentGuard.actions.title') }}</h3>
        <p>{{ t('agentGuard.actions.scopeNotice') }}</p>
      </div>
      <el-tag :type="statusType(unit.status)">{{ statusLabel(unit.status) }}</el-tag>
    </header>

    <el-alert
      v-if="!coverageAllowsActions"
      type="warning"
      :title="coverageMessage"
      :closable="false"
      show-icon
    />
    <el-alert
      v-else-if="!canOperate"
      type="info"
      :title="t('agentGuard.actions.permissionDenied')"
      :closable="false"
      show-icon
    />

    <div v-if="coverageAllowsActions && canOperate" class="guard-action-buttons">
      <el-button
        v-if="canFreeze"
        data-action="freeze"
        type="warning"
        :loading="loading"
        @click="openDialog('freeze_execution_unit')"
      >
        {{ t('agentGuard.actions.freeze') }}
      </el-button>
      <el-button
        v-if="canResume"
        data-action="resume"
        type="primary"
        :loading="loading"
        @click="openDialog('resume_execution_unit')"
      >
        {{ t('agentGuard.actions.resume') }}
      </el-button>
      <el-button
        v-if="canKill"
        data-action="kill"
        type="danger"
        :loading="loading"
        @click="openDialog('kill_execution_unit')"
      >
        {{ t('agentGuard.actions.kill') }}
      </el-button>
    </div>

    <el-alert
      v-if="error"
      class="guard-action-error"
      type="error"
      :title="t('agentGuard.actions.failed')"
      :description="error"
      :closable="false"
      show-icon
    />

    <div class="guard-action-timeline">
      <h4>{{ t('agentGuard.actions.timeline') }}</h4>
      <el-empty v-if="actions.length === 0" :description="t('agentGuard.actions.noHistory')" />
      <el-timeline v-else>
        <el-timeline-item
          v-for="item in actions"
          :key="item.id"
          :timestamp="item.completed_at || item.dispatched_at || item.requested_at"
          :type="timelineType(item.status)"
        >
          <div class="timeline-heading">
            <strong>{{ actionLabel(item.action) }}</strong>
            <el-tag size="small" :type="statusType(item.status)">{{ statusLabel(item.status) }}</el-tag>
          </div>
          <div class="timeline-meta">
            <span>{{ item.requested_by || '-' }}</span>
            <span>{{ item.reason }}</span>
          </div>
          <p v-if="item.error_code || item.error_message" class="timeline-error">
            {{ [item.error_code, item.error_message].filter(Boolean).join(' · ') }}
          </p>
        </el-timeline-item>
      </el-timeline>
    </div>

    <el-dialog
      v-model="dialogVisible"
      width="560px"
      :title="dialogTitle"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div class="action-target-summary">
        <dl>
          <div><dt>{{ t('agentGuard.actions.host') }}</dt><dd>{{ hostLabel }}</dd></div>
          <div><dt>{{ t('agentGuard.actions.agent') }}</dt><dd>{{ agentLabel }}</dd></div>
          <div><dt>{{ t('agentGuard.actions.instance') }}</dt><dd>{{ instanceLabel }}</dd></div>
          <div><dt>{{ t('agentGuard.actions.unit') }}</dt><dd>{{ unit.id }}</dd></div>
          <div><dt>{{ t('agentGuard.actions.processes') }}</dt><dd>{{ unit.process_count ?? '-' }}</dd></div>
        </dl>
        <el-alert
          type="warning"
          :title="t('agentGuard.actions.scopeNotice')"
          :closable="false"
          show-icon
        />
        <p v-if="selectedAction === 'resume_execution_unit'" class="resume-warning">
          {{ t('agentGuard.actions.resumePolicyWarning') }}
        </p>
      </div>
      <label class="action-field">
        <span>{{ t('agentGuard.actions.reason') }}</span>
        <el-input v-model="reason" type="textarea" :rows="3" maxlength="500" show-word-limit />
      </label>
      <el-checkbox v-if="selectedAction === 'freeze_execution_unit'" v-model="hold">
        {{ t('agentGuard.actions.hold') }}
      </el-checkbox>
      <label v-if="selectedAction === 'kill_execution_unit'" class="action-field">
        <span>{{ t('agentGuard.actions.confirmPhrase', { phrase: killPhrase }) }}</span>
        <el-input v-model="confirmation" type="textarea" :rows="1" autocomplete="off" />
      </label>
      <template #footer>
        <el-button autofocus data-action="cancel" @click="closeDialog">
          {{ t('common.actions.cancel') }}
        </el-button>
        <el-button
          data-action="confirm"
          :type="selectedAction === 'kill_execution_unit' ? 'danger' : 'primary'"
          :disabled="confirmDisabled"
          :loading="loading"
          @click="confirmAction"
        >
          {{ t('agentGuard.actions.confirm') }}
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  AgentExecutionUnit,
  AgentGuardAction,
  AgentGuardActionName,
  AgentGuardActionRequest,
} from '@/types/agentGuard'

const props = withDefaults(defineProps<{
  unit: AgentExecutionUnit
  hostLabel: string
  agentLabel: string
  instanceLabel: string
  actions: AgentGuardAction[]
  canOperate: boolean
  loading?: boolean
  error?: string
}>(), {
  loading: false,
  error: '',
})

const emit = defineEmits<{
  (event: 'execute', action: Extract<AgentGuardActionName,
    'freeze_execution_unit' | 'resume_execution_unit' | 'kill_execution_unit'>,
  payload: AgentGuardActionRequest): void
}>()

const { t } = useI18n()
const dialogVisible = ref(false)
const selectedAction = ref<Extract<AgentGuardActionName,
  'freeze_execution_unit' | 'resume_execution_unit' | 'kill_execution_unit'>>('freeze_execution_unit')
const reason = ref('')
const confirmation = ref('')
const hold = ref(false)

const coverageAllowsActions = computed(() => [
  'full_enforcement',
  'behavior_monitor_escape_enforce',
].includes(props.unit.coverage_level))
const canFreeze = computed(() => props.unit.status === 'running')
const canResume = computed(() => ['frozen', 'freezing'].includes(props.unit.status))
const canKill = computed(() => ['running', 'frozen', 'freezing'].includes(props.unit.status))
const killPhrase = computed(() => `KILL ${props.unit.id.slice(-8)}`)
const confirmDisabled = computed(() =>
  reason.value.trim().length < 8
  || (selectedAction.value === 'kill_execution_unit' && confirmation.value !== killPhrase.value),
)
const dialogTitle = computed(() => actionLabel(selectedAction.value))
const coverageMessage = computed(() => props.unit.coverage_level === 'remote_unobservable'
  ? t('agentGuard.actions.remoteUnobservable')
  : t('agentGuard.actions.enforcementUnavailable'))

function openDialog(action: typeof selectedAction.value) {
  selectedAction.value = action
  reason.value = ''
  confirmation.value = ''
  hold.value = false
  dialogVisible.value = true
}

function closeDialog() {
  dialogVisible.value = false
}

function confirmAction() {
  if (confirmDisabled.value) return
  emit('execute', selectedAction.value, {
    reason: reason.value.trim(),
    ...(selectedAction.value === 'freeze_execution_unit' ? { hold: hold.value } : {}),
  })
  closeDialog()
}

function actionLabel(action: AgentGuardActionName) {
  return t(`agentGuard.actions.names.${action}`)
}

function statusLabel(status: string) {
  return t(`agentGuard.actions.status.${status}`)
}

function statusType(status: string) {
  if (status === 'success' || status === 'frozen') return 'success'
  if (status === 'failed' || status === 'expired') return 'danger'
  if (status === 'pending' || status === 'dispatching' || status === 'running' || status === 'freezing') return 'warning'
  return 'info'
}

function timelineType(status: string) {
  return statusType(status)
}
</script>

<style scoped>
.guard-action-panel {
  display: grid;
  gap: 14px;
  margin-top: 16px;
  padding: 16px;
  border: 1px solid var(--aegis-border);
  border-radius: 10px;
}

.guard-action-panel > header,
.timeline-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.guard-action-panel h3,
.guard-action-panel h4,
.guard-action-panel p {
  margin: 0;
}

.guard-action-panel header p,
.timeline-meta {
  color: var(--aegis-text-muted);
  font-size: 13px;
}

.guard-action-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.timeline-meta {
  display: flex;
  gap: 12px;
  margin-top: 4px;
}

.timeline-error {
  margin-top: 6px !important;
  color: var(--el-color-danger);
}

.action-target-summary,
.action-field {
  display: grid;
  gap: 10px;
}

.action-target-summary dl {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 16px;
  margin: 0;
}

.action-target-summary dl > div {
  min-width: 0;
}

.action-target-summary dt {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.action-target-summary dd {
  overflow-wrap: anywhere;
  margin: 2px 0 0;
}

.action-field {
  margin-top: 14px;
}

.resume-warning {
  color: var(--el-color-warning-dark-2);
}
</style>

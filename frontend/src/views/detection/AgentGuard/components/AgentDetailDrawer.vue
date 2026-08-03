<template>
  <el-drawer
    class="agent-guard-detail-drawer"
    :model-value="visible"
    direction="rtl"
    size="76%"
    style="min-width: 880px"
    :append-to-body="true"
    :destroy-on-close="false"
    @close="emit('close')"
  >
    <template #header>
      <div class="guard-drawer-header">
        <div>
          <h2>{{ drawerTitle }}</h2>
          <div class="guard-drawer-meta">
            <span>{{ t('agentGuard.drawer.host') }}：{{ hostLabel }}</span>
            <span>{{ t('agentGuard.drawer.type') }}：{{ agent?.agent_type || '-' }}</span>
            <span>{{ t('agentGuard.runtime.instances', { count: agent?.running_instance_count || 0 }) }}</span>
            <CoverageBadge
              v-if="agent"
              :coverage="agent.coverage_level"
              :reasons="agent.coverage_reasons || []"
            />
          </div>
        </div>
      </div>
    </template>

    <div class="guard-drawer-body">
      <el-skeleton v-if="loading.instances" :rows="2" animated />
      <el-alert
        v-else-if="errors.instances"
        type="error"
        :title="t('agentGuard.states.detailUnavailable')"
        :description="errors.instances"
        show-icon
      >
        <template #default>
          <el-button size="small" @click="emit('retry', 'instances')">
            {{ t('common.actions.retry') }}
          </el-button>
        </template>
      </el-alert>
      <AgentRuntimeSelector
        v-else
        :instances="instances"
        :total="instanceTotal"
        :selected-instance-id="selectedInstanceId"
        @select="emit('select-instance', $event)"
      />

      <el-tabs :model-value="detailTab" class="guard-detail-tabs" @tab-change="changeTab">
        <el-tab-pane :label="panoramaLabel" name="panorama">
          <div class="detail-tab-panel">
            <el-skeleton v-if="loading.panorama" :rows="6" animated />
            <el-alert
              v-else-if="errors.panorama"
              type="error"
              :title="t('agentGuard.states.detailUnavailable')"
              :description="errors.panorama"
              show-icon
            >
              <template #default>
                <el-button size="small" @click="emit('retry', 'panorama')">
                  {{ t('common.actions.retry') }}
                </el-button>
              </template>
            </el-alert>
            <AgentPanoramaExplorer
              v-else-if="panoramaNodes.length"
              :nodes="panoramaNodes"
              :mode="mode"
              :load-children="loadPanoramaChildren"
              @select="emit('select-panorama-node', $event)"
            >
              <template v-if="mode === 'escape' && selectedExecutionUnit" #detail>
                <IsolationBaselinePanel :unit="selectedExecutionUnit" />
              </template>
            </AgentPanoramaExplorer>
            <el-empty
              v-else
              :description="mode === 'behavior'
                ? t('agentGuard.states.panoramaNotCollected')
                : t('agentGuard.states.sandboxNotCollected')"
            />
            <AgentGuardActionPanel
              v-if="mode === 'escape' && selectedExecutionUnit"
              :unit="selectedExecutionUnit"
              :host-label="hostLabel"
              :agent-label="agent?.display_name || agent?.agent_type || '-'"
              :instance-label="selectedInstanceLabel"
              :actions="actions"
              :can-operate="canOperateActions"
              :loading="actionLoading"
              :error="actionError"
              @execute="(action, payload) => emit('execute-action', action, payload)"
            />
          </div>
        </el-tab-pane>

        <el-tab-pane :label="analysisLabel" name="analysis">
          <div class="detail-tab-panel">
            <el-skeleton v-if="loading.analysis" :rows="5" animated />
            <el-alert
              v-else-if="errors.analysis"
              type="error"
              :title="t('agentGuard.states.detailUnavailable')"
              :description="errors.analysis"
              show-icon
            >
              <template #default>
                <el-button size="small" @click="emit('retry', 'analysis')">
                  {{ t('common.actions.retry') }}
                </el-button>
              </template>
            </el-alert>
            <AgentSecurityAnalysis
              v-else
              :rules="builtinRules"
              :findings="findings"
              :analyses="analyses"
              :selected-finding-id="detailReferenceId || ''"
              :analysis-pending="loading.analysis"
              @select-finding="emit('select-finding', $event)"
              @analyze="emit('analyze-finding', $event)"
              @open-evidence="emit('open-evidence', $event)"
            >
            </AgentSecurityAnalysis>
            <AgentBehaviorEvidence
              v-if="selectedBehavior"
              class="selected-behavior-evidence"
              :behavior="selectedBehavior"
            />
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  AgentBehaviorIndex,
  AgentExecutionUnit,
  AgentGuardAction,
  AgentGuardActionName,
  AgentGuardActionRequest,
  AgentGuardAgentSummary,
  AgentGuardDetailTab,
  AgentGuardMode,
  AgentRuntimeInstance,
  AgentSecurityAnalysisRun,
  AgentSecurityFindingSummary,
  BuiltinAgentBehaviorRuleSummary,
  PanoramaTreeNode,
} from '@/types/agentGuard'
import AgentRuntimeSelector from './AgentRuntimeSelector.vue'
import AgentPanoramaExplorer from './AgentPanoramaExplorer.vue'
import AgentBehaviorEvidence from './AgentBehaviorEvidence.vue'
import AgentSecurityAnalysis from './AgentSecurityAnalysis.vue'
import CoverageBadge from './CoverageBadge.vue'
import IsolationBaselinePanel from './IsolationBaselinePanel.vue'
import AgentGuardActionPanel from './AgentGuardActionPanel.vue'

const props = withDefaults(defineProps<{
  visible: boolean
  mode: AgentGuardMode
  agent: AgentGuardAgentSummary | null
  detailTab: AgentGuardDetailTab
  instances: AgentRuntimeInstance[]
  instanceTotal?: number
  selectedInstanceId: string
  panoramaNodes: PanoramaTreeNode[]
  loadPanoramaChildren?: (nodeId: string) => Promise<PanoramaTreeNode[]>
  selectedExecutionUnit?: AgentExecutionUnit | null
  builtinRules?: BuiltinAgentBehaviorRuleSummary[]
  findings: AgentSecurityFindingSummary[]
  analyses?: AgentSecurityAnalysisRun[]
  selectedBehavior?: AgentBehaviorIndex | null
  actions?: AgentGuardAction[]
  canOperateActions?: boolean
  actionLoading?: boolean
  actionError?: string
  detailReferenceId?: string
  loading: {
    instances: boolean
    panorama: boolean
    analysis: boolean
  }
  errors: {
    instances: string
    panorama: string
    analysis: string
  }
}>(), {
  loadPanoramaChildren: async () => [],
  selectedExecutionUnit: null,
  builtinRules: () => [],
  analyses: () => [],
  selectedBehavior: null,
  actions: () => [],
  canOperateActions: false,
  actionLoading: false,
  actionError: '',
  instanceTotal: 0,
})

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'update:detailTab', tab: AgentGuardDetailTab): void
  (event: 'tab-change', tab: AgentGuardDetailTab): void
  (event: 'select-instance', instanceId: string): void
  (event: 'select-panorama-node', node: PanoramaTreeNode): void
  (event: 'select-finding', findingId: string): void
  (event: 'analyze-finding', findingId: string): void
  (event: 'open-evidence', eventId: string): void
  (event: 'execute-action', action: Extract<AgentGuardActionName,
    'freeze_execution_unit' | 'resume_execution_unit' | 'kill_execution_unit'>,
  payload: AgentGuardActionRequest): void
  (event: 'retry', target: 'instances' | 'panorama' | 'analysis'): void
}>()

const { t } = useI18n()

const drawerTitle = computed(() => t(
  props.mode === 'behavior'
    ? 'agentGuard.title.behaviorDetail'
    : 'agentGuard.title.escapeDetail',
  { name: props.agent?.display_name || props.agent?.agent_type || '-' },
))

const hostLabel = computed(() => {
  if (!props.agent) return '-'
  return [props.agent.host?.hostname, props.agent.host?.ip].filter(Boolean).join(' / ') || '-'
})

const selectedInstanceLabel = computed(() => {
  const selected = props.instances.find(instance => instance.id === props.selectedInstanceId)
  return selected ? `PID ${selected.controller_pid}` : props.selectedInstanceId || '-'
})

const panoramaLabel = computed(() => t(
  props.mode === 'behavior'
    ? 'agentGuard.drawer.behaviorPanorama'
    : 'agentGuard.drawer.sandboxPanorama',
))

const analysisLabel = computed(() => t(
  props.mode === 'behavior'
    ? 'agentGuard.drawer.securityAnalysis'
    : 'agentGuard.drawer.escapeAnalysis',
  { count: props.findings.length },
))

function changeTab(value: string | number) {
  const tab: AgentGuardDetailTab = value === 'analysis' ? 'analysis' : 'panorama'
  emit('update:detailTab', tab)
  emit('tab-change', tab)
}

</script>

<style>
.agent-guard-detail-drawer {
  min-width: 880px;
}
</style>

<style scoped>
.guard-drawer-header h2 {
  margin: 0 0 8px;
  color: var(--aegis-text);
  font-size: 20px;
}

.guard-drawer-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 16px;
  color: var(--aegis-text-muted);
  font-size: 13px;
}

.guard-drawer-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.guard-detail-tabs {
  min-height: 480px;
}

.detail-tab-panel {
  min-height: 400px;
  padding-top: 8px;
}

.verified-index-note {
  max-width: 520px;
  margin: 8px auto 0;
  color: var(--aegis-text-muted);
  font-size: 13px;
}

</style>

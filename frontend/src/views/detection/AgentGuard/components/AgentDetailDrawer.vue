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
            <span>{{ t('agentGuard.runtime.instances', { count: detailInstanceCount }) }}</span>
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
      <el-skeleton v-if="mode === 'behavior' && loading.instances" :rows="2" animated />
      <el-alert
        v-else-if="mode === 'behavior' && errors.instances"
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
        v-if="mode === 'behavior' && !loading.instances && !errors.instances"
        :sessions="sessions"
        :total="sessionTotal"
        :page="sessionPage"
        :page-size="sessionPageSize"
        :selected-session-id="selectedSessionId"
        :selected-session-ids="selectedSessionIds"
        :can-delete-sessions="canDeleteSessions"
        @select="emit('select-session', $event)"
        @page-change="emit('session-page-change', $event)"
        @selection-change="emit('session-selection-change', $event)"
        @delete="emit('delete-sessions', $event)"
      />

      <el-tabs :model-value="effectiveDetailTab" class="guard-detail-tabs" @tab-change="changeTab">
        <el-tab-pane v-if="mode === 'behavior'" :label="panoramaLabel" name="panorama">
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
              :total="panoramaTotal"
              :page="panoramaPage"
              :page-size="panoramaPageSize"
              :mode="mode"
              :load-children="loadPanoramaChildren"
              @select="emit('select-panorama-node', $event)"
              @page-change="emit('panorama-page-change', $event)"
            >
            </AgentPanoramaExplorer>
            <el-empty
              v-else
              :description="mode === 'behavior'
                ? t('agentGuard.states.panoramaNotCollected')
                : t('agentGuard.states.sandboxNotCollected')"
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
            <AgentEscapeAnalysis
              v-if="mode === 'escape'"
              :findings="findings"
              :finding="selectedFinding"
              :selected-finding-id="selectedFinding?.id || detailReferenceId || ''"
              :finding-total="findingTotal"
              :finding-page="findingPage"
              :finding-page-size="findingPageSize"
              @select-finding="emit('select-finding', $event)"
              @finding-page-change="emit('finding-page-change', $event)"
            />
            <AgentSecurityAnalysis
              v-else
              :rules="builtinRules"
              :findings="findings"
              :selected-finding="selectedFinding"
              :finding-total="findingTotal"
              :finding-page="findingPage"
              :finding-page-size="findingPageSize"
              :analyses="analyses"
              :analysis-total="analysisTotal"
              :analysis-page="analysisPage"
              :analysis-page-size="analysisPageSize"
              :selected-finding-id="selectedFinding?.id || detailReferenceId || ''"
              :analysis-pending="loading.analysis"
              @select-finding="emit('select-finding', $event)"
              @finding-page-change="emit('finding-page-change', $event)"
              @analysis-page-change="emit('analysis-page-change', $event)"
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
  AgentBehaviorSession,
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
import AgentEscapeAnalysis from './AgentEscapeAnalysis.vue'
import CoverageBadge from './CoverageBadge.vue'

const props = withDefaults(defineProps<{
  visible: boolean
  mode: AgentGuardMode
  agent: AgentGuardAgentSummary | null
  detailTab: AgentGuardDetailTab
  instances: AgentRuntimeInstance[]
  instanceTotal?: number
  selectedInstanceId: string
  sessions?: AgentBehaviorSession[]
  sessionTotal?: number
  sessionPage?: number
  sessionPageSize?: number
  selectedSessionId?: string
  selectedSessionIds?: string[]
  canDeleteSessions?: boolean
  panoramaNodes: PanoramaTreeNode[]
  panoramaTotal?: number
  panoramaPage?: number
  panoramaPageSize?: number
  loadPanoramaChildren?: (nodeId: string) => Promise<PanoramaTreeNode[]>
  selectedExecutionUnit?: AgentExecutionUnit | null
  builtinRules?: BuiltinAgentBehaviorRuleSummary[]
  findings: AgentSecurityFindingSummary[]
  selectedFinding?: AgentSecurityFindingSummary | null
  findingTotal?: number
  findingPage?: number
  findingPageSize?: number
  analyses?: AgentSecurityAnalysisRun[]
  analysisTotal?: number
  analysisPage?: number
  analysisPageSize?: number
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
  sessions: () => [],
  sessionTotal: 0,
  sessionPage: 1,
  sessionPageSize: 20,
  selectedSessionId: '',
  selectedSessionIds: () => [],
  canDeleteSessions: false,
  panoramaTotal: 0,
  panoramaPage: 1,
  panoramaPageSize: 20,
  builtinRules: () => [],
  analyses: () => [],
  selectedFinding: null,
  findingTotal: 0,
  findingPage: 1,
  findingPageSize: 20,
  analysisTotal: 0,
  analysisPage: 1,
  analysisPageSize: 10,
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
  (event: 'select-session', sessionId: string): void
  (event: 'session-page-change', page: number): void
  (event: 'session-selection-change', sessionIds: string[]): void
  (event: 'delete-sessions', sessionIds: string[]): void
  (event: 'select-panorama-node', node: PanoramaTreeNode): void
  (event: 'panorama-page-change', page: number): void
  (event: 'select-finding', findingId: string): void
  (event: 'finding-page-change', page: number): void
  (event: 'analysis-page-change', page: number): void
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
  return props.selectedSessionId || selected?.id || props.selectedInstanceId || '-'
})

const detailInstanceCount = computed(() =>
  props.instanceTotal || props.instances.length || props.agent?.running_instance_count || 0,
)

const panoramaLabel = computed(() => t(
  props.mode === 'behavior'
    ? 'agentGuard.drawer.behaviorPanorama'
    : 'agentGuard.drawer.sandboxPanorama',
))

const analysisLabel = computed(() => props.mode === 'behavior'
  ? t('agentGuard.drawer.securityAnalysis')
  : t('agentGuard.drawer.escapeAnalysis', { count: props.findingTotal }))
const effectiveDetailTab = computed<AgentGuardDetailTab>(() => props.mode === 'escape' ? 'analysis' : props.detailTab)

function changeTab(value: string | number) {
	if (props.mode === 'escape') {
		emit('update:detailTab', 'analysis')
		emit('tab-change', 'analysis')
		return
	}
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

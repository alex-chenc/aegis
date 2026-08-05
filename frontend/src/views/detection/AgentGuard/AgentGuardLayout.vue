<template>
  <div class="agent-guard-page page-shell">
    <section class="page-hero agent-guard-hero">
      <div>
        <h1>{{ pageTitle }}</h1>
        <p>{{ pageDescription }}</p>
      </div>
      <el-button type="primary" plain @click="policyDialogVisible = true">
        {{ t('agentGuard.policy.title') }}
      </el-button>
      <el-button
        v-if="canOperate('agent_guard_settings')"
        type="primary"
        @click="runtimeSettingsDialogVisible = true"
      >
        {{ t('agentGuard.settings.button') }}
      </el-button>
    </section>

    <AgentEscapePolicyOverview v-if="mode === 'escape'" />

    <div class="agent-guard-metric-grid" aria-live="polite">
      <div v-for="metric in metrics" :key="metric.key" class="metric-card">
        <el-skeleton v-if="store.loading.overview" :rows="1" animated />
        <template v-else>
          <div class="metric-label">{{ metric.label }}</div>
          <el-tooltip
            :content="metric.value == null ? t('agentGuard.metrics.unavailableHint') : ''"
            :disabled="metric.value != null"
          >
            <div class="metric-value" :class="{ 'metric-unavailable': metric.value == null }">
              {{ metric.value == null ? '-' : metric.value }}
            </div>
          </el-tooltip>
        </template>
      </div>
    </div>

    <el-alert
      v-if="isStale"
      type="warning"
      :title="t('agentGuard.states.stale', { time: lastUpdatedLabel })"
      :closable="false"
      show-icon
    />
    <el-alert
      v-if="hasDegradedCoverage"
      type="warning"
      :title="t('agentGuard.states.degraded')"
      :closable="false"
      show-icon
    />
    <el-alert
      v-if="realtimeDisconnected"
      type="warning"
      :title="t('agentGuard.states.websocketDisconnected')"
      :closable="false"
      show-icon
    />
    <el-alert
      v-if="store.errors.overview && store.overview"
      type="warning"
      :title="t('agentGuard.states.retryHint')"
      :closable="false"
      show-icon
    />

    <el-card class="guard-filter-card">
      <el-form class="guard-filter-form" @submit.prevent="applyFilters">
        <div class="filter-row">
          <el-form-item :label="t('agentGuard.filters.host')">
            <el-input
              v-model="draftFilters.host_id"
              clearable
              :placeholder="t('agentGuard.filters.hostPlaceholder')"
            />
          </el-form-item>
          <el-form-item :label="t('agentGuard.filters.agentType')">
            <el-select v-model="draftFilters.agent_types" multiple clearable collapse-tags>
              <el-option
                v-for="agentType in agentTypeOptions"
                :key="agentType"
                :value="agentType"
                :label="t(`agentGuard.agentTypes.${agentType}`)"
              />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('agentGuard.filters.runtimeStatus')">
            <el-select v-model="draftFilters.runtime_status" clearable>
              <el-option value="running" :label="t('agentGuard.runtime.running')" />
              <el-option value="stale" :label="t('agentGuard.runtime.stale')" />
              <el-option value="stopped" :label="t('agentGuard.runtime.stopped')" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('agentGuard.filters.coverage')">
            <el-select v-model="draftFilters.coverage" clearable>
              <el-option
                v-for="coverage in coverageOptions"
                :key="coverage"
                :value="coverage"
                :label="t(`agentGuard.coverage.${coverage}`)"
              />
            </el-select>
          </el-form-item>
          <el-form-item v-if="mode === 'escape'" :label="t('agentGuard.filters.isolationType')">
            <el-select v-model="draftFilters.isolation_type" clearable>
              <el-option value="local_process_tree" :label="t('agentGuard.isolation.local_process_tree')" />
              <el-option value="linux_namespace" :label="t('agentGuard.isolation.linux_namespace')" />
              <el-option value="oci_container" :label="t('agentGuard.isolation.oci_container')" />
              <el-option value="remote_sandbox" :label="t('agentGuard.isolation.remote_sandbox')" />
            </el-select>
          </el-form-item>
        </div>
        <div class="filter-row filter-actions-row">
          <el-form-item class="keyword-field" :label="t('agentGuard.filters.keyword')">
            <el-input
              v-model="draftFilters.keyword"
              clearable
              :placeholder="t('agentGuard.filters.keywordPlaceholder')"
              @keyup.enter="applyFilters"
            />
          </el-form-item>
          <el-button native-type="submit" type="primary">{{ t('common.actions.query') }}</el-button>
          <el-button @click="resetFilters">{{ t('common.actions.reset') }}</el-button>
        </div>
      </el-form>
    </el-card>

    <el-card class="agent-list-card">
      <template #header>
        <div class="agent-list-header">
          <strong>{{ pageTitle }}</strong>
          <el-button :loading="store.loading.agents" @click="refreshPage">
            {{ t('common.actions.refresh') }}
          </el-button>
        </div>
      </template>

      <el-result
        v-if="permissionDenied && store.agents.length === 0"
        icon="warning"
        :title="t('agentGuard.states.permissionDenied')"
      />
      <el-result
        v-else-if="store.errors.agents && store.agents.length === 0"
        icon="error"
        :title="t('agentGuard.states.loadFailed')"
        :sub-title="safeErrorMessage"
      >
        <template #extra>
          <el-button type="primary" @click="refreshPage">{{ t('common.actions.retry') }}</el-button>
        </template>
      </el-result>
      <template v-else>
        <el-alert
          v-if="store.errors.agents"
          class="preserved-data-alert"
          type="warning"
          :title="t('agentGuard.states.retryHint')"
          :closable="false"
          show-icon
        >
          <template #default>
            <el-button size="small" @click="refreshPage">{{ t('common.actions.retry') }}</el-button>
          </template>
        </el-alert>
        <el-skeleton v-if="store.loading.agents && store.agents.length === 0" :rows="6" animated />
        <el-empty
          v-else-if="store.agents.length === 0"
          :description="hasAppliedFilters ? t('agentGuard.states.noMatches') : t('agentGuard.states.noAssets')"
        >
          <p v-if="!hasAppliedFilters" class="empty-help">{{ t('agentGuard.states.noAssetsHelp') }}</p>
        </el-empty>
        <AgentSummaryTable
          v-else
          :agents="store.agents"
          :loading="store.loading.agents"
          :total="store.agentTotal"
          :page="parsedQuery.page"
          :page-size="parsedQuery.pageSize"
          :mode="mode"
          @open="openAgent"
          @page-change="changePage"
          @size-change="changePageSize"
        />
      </template>
    </el-card>

    <AgentDetailDrawer
      :visible="drawerVisible"
      :mode="mode"
      :agent="selectedAgent"
      :detail-tab="parsedQuery.detail.tab"
      :instances="store.instances"
      :instance-total="store.instanceTotal"
      :selected-instance-id="parsedQuery.detail.instanceId"
      :sessions="store.sessions"
      :session-total="store.sessionTotal"
      :session-page="store.sessionPage"
      :session-page-size="store.sessionPageSize"
      :selected-session-id="parsedQuery.detail.sessionId"
      :selected-session-ids="selectedSessionIds"
      :can-delete-sessions="canOperate('agent_guard_session_delete')"
      :panorama-nodes="store.panoramaNodes"
      :panorama-total="store.panoramaTotal"
      :panorama-page="store.panoramaPage"
      :panorama-page-size="store.panoramaPageSize"
      :load-panorama-children="store.fetchPanoramaChildren"
      :selected-execution-unit="store.selectedExecutionUnit"
      :builtin-rules="store.builtinRules"
      :findings="store.findings"
      :selected-finding="store.selectedFinding"
      :finding-total="store.findingTotal"
      :finding-page="store.findingPage"
      :finding-page-size="store.findingPageSize"
      :analyses="store.analyses"
      :analysis-total="store.analysisTotal"
      :analysis-page="store.analysisPage"
      :analysis-page-size="store.analysisPageSize"
      :selected-behavior="store.selectedBehavior"
      :actions="store.actions"
      :can-operate-actions="canOperate('agent_guard_action')"
      :action-loading="store.loading.actions"
      :action-error="store.errors.actions"
      :detail-reference-id="parsedQuery.detail.findingId || parsedQuery.detail.eventId"
      :loading="store.loading"
      :errors="store.errors"
      @close="closeDrawer"
      @update:detail-tab="changeDetailTab"
      @select-instance="changeInstance"
      @select-session="changeSession"
      @session-page-change="changeSessionPage"
      @session-selection-change="selectedSessionIds = $event"
      @delete-sessions="deleteSessions"
      @select-panorama-node="selectPanoramaNode"
      @panorama-page-change="changePanoramaPage"
      @select-finding="selectFinding"
      @finding-page-change="changeFindingPage"
      @analysis-page-change="changeAnalysisPage"
      @analyze-finding="analyzeFinding"
      @open-evidence="openEvidence"
      @execute-action="executeUnitAction"
      @retry="retryDetail"
    />
    <AgentGuardPolicyDialog
      :visible="policyDialogVisible"
      :mode="mode"
      @close="policyDialogVisible = false"
    />
    <AgentGuardRuntimeSettingsDialog
      :visible="runtimeSettingsDialogVisible"
      :hosts="runtimeSettingHosts"
      :mode="mode"
      @close="runtimeSettingsDialogVisible = false"
      @saved="refreshPage"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { formatDateTime } from '@/i18n/formatters'
import { useAgentGuardStore } from '@/store/agentGuard'
import { useRole } from '@/composables/useRole'
import type {
  AgentGuardAgentQuery,
  AgentGuardAgentSummary,
  AgentGuardDetailTab,
  AgentGuardListFilters,
  AgentGuardMode,
  AgentGuardActionName,
  AgentGuardActionRequest,
} from '@/types/agentGuard'
import AgentSummaryTable from './components/AgentSummaryTable.vue'
import AgentDetailDrawer from './components/AgentDetailDrawer.vue'
import AgentGuardPolicyDialog from './components/AgentGuardPolicyDialog.vue'
import AgentGuardRuntimeSettingsDialog from './components/AgentGuardRuntimeSettingsDialog.vue'
import AgentEscapePolicyOverview from './components/AgentEscapePolicyOverview.vue'
import { AGENT_GUARD_AGENT_TYPE_FILTERS } from './agentGuardProfiles'
import {
  DEFAULT_AGENT_GUARD_FILTERS,
	buildAgentGuardDetailInstanceQuery,
	clearAgentGuardDetailQuery,
	parseAgentGuardQuery,
	selectPreferredAgentGuardInstance,
	serializeAgentGuardListQuery,
  withAgentGuardDetailQuery,
} from './agentGuardQuery'

const props = defineProps<{
  mode: AgentGuardMode
}>()

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const store = useAgentGuardStore()
const { canOperate } = useRole()
const selectedSessionIds = ref<string[]>([])

const agentTypeOptions = AGENT_GUARD_AGENT_TYPE_FILTERS
const coverageOptions = [
  'full_enforcement',
  'monitor_only',
  'no_isolation',
  'remote_unobservable',
  'unsupported',
  'unsupported_profile',
  'degraded',
]

const parsedQuery = computed(() => parseAgentGuardQuery(route.query))
const draftFilters = reactive<AgentGuardListFilters>({ ...DEFAULT_AGENT_GUARD_FILTERS })
const detailScopeLoaded = ref('')
const policyDialogVisible = ref(false)
const runtimeSettingsDialogVisible = ref(false)
const realtimeDisconnected = ref(false)
let realtimeSocket: WebSocket | null = null
let realtimeRefreshTimer: ReturnType<typeof setTimeout> | null = null
let realtimeReconnectTimer: ReturnType<typeof setTimeout> | null = null
let actionPollTimer: ReturnType<typeof setInterval> | null = null
let actionPollInFlight = false
let realtimeStopped = false

const pageTitle = computed(() => t(
  props.mode === 'behavior' ? 'agentGuard.title.events' : 'agentGuard.title.escape',
))
const pageDescription = computed(() => t(
  props.mode === 'behavior' ? 'agentGuard.description.events' : 'agentGuard.description.escape',
))

const metrics = computed(() => props.mode === 'behavior'
  ? [
      { key: 'assets', label: t('agentGuard.metrics.assets'), value: store.overview?.agent_assets ?? store.agentTotal },
      { key: 'running', label: t('agentGuard.metrics.runningInstances'), value: store.overview?.running_instances },
      { key: 'risk', label: t('agentGuard.metrics.highRiskFindings'), value: store.overview?.high_risk_findings },
      { key: 'blocked', label: t('agentGuard.metrics.blocked'), value: store.overview?.successful_blocks },
    ]
  : [
      { key: 'assets', label: t('agentGuard.metrics.assets'), value: store.overview?.agent_assets ?? store.agentTotal },
      { key: 'monitored', label: t('agentGuard.metrics.monitoredInstances'), value: store.overview?.monitored_instances },
      { key: 'escape', label: t('agentGuard.metrics.escapeAttempts'), value: store.overview?.escape_attempts },
      { key: 'frozen', label: t('agentGuard.metrics.frozen'), value: store.overview?.frozen_units },
    ])

const selectedAgent = computed<AgentGuardAgentSummary | null>(() => {
  const assetId = parsedQuery.value.detail.assetId || parsedQuery.value.detail.scopeKey
  const directContext = store.selectedFinding || store.findings[0] || store.selectedBehavior
  if (!assetId && !directContext) return null
  const effectiveAssetId = assetId || directContext?.asset_id || directContext?.agent_scope_key || ''
  return store.agents.find(agent =>
    agent.asset_id === effectiveAssetId || agent.agent_scope_key === effectiveAssetId,
  ) || {
    agent_scope_key: directContext?.agent_scope_key || effectiveAssetId || 'unresolved',
    asset_id: directContext?.asset_id || effectiveAssetId || undefined,
    host: {
      id: directContext?.host?.id || '',
      hostname: directContext?.host?.hostname || '-',
      ip: directContext?.host?.ip || '-',
    },
    agent_type: directContext?.agent_type || '-',
    display_name: directContext?.agent_display_name
      || directContext?.agent_type
      || effectiveAssetId
      || parsedQuery.value.detail.findingId
      || parsedQuery.value.detail.eventId
      || '-',
    running_instance_count: store.instances.length,
    controller_pids: store.instances.map(instance => instance.controller_pid),
    runtime_status: 'unknown',
    isolation_types: [],
    coverage_level: 'unknown',
    coverage_reasons: [],
    high_risk_finding_count: 0,
    escape_finding_count: 0,
  }
})

const runtimeSettingHosts = computed(() => {
  const hosts = new Map<string, { id: string; hostname: string; ip: string }>()
  for (const agent of store.agents) {
    if (agent.host.id && !hosts.has(agent.host.id)) {
      hosts.set(agent.host.id, {
        id: agent.host.id,
        hostname: agent.host.hostname,
        ip: agent.host.ip,
      })
    }
  }
  return [...hosts.values()]
})

const drawerVisible = computed(() => Boolean(
  parsedQuery.value.detail.assetId
  || parsedQuery.value.detail.scopeKey
  || parsedQuery.value.detail.findingId
  || parsedQuery.value.detail.eventId,
))

const hasAppliedFilters = computed(() => {
  const filters = parsedQuery.value.filters
  return Boolean(
    filters.host_id
    || filters.agent_types.length
    || filters.runtime_status
    || filters.coverage
    || filters.isolation_type
    || filters.keyword,
  )
})

const hasDegradedCoverage = computed(() =>
  store.agents.some(agent => [
    'monitor_only',
    'no_isolation',
    'remote_unobservable',
    'unsupported',
    'unsupported_profile',
    'degraded',
  ].includes(agent.coverage_level)),
)

const isStale = computed(() =>
  Boolean(store.overview?.stale)
  || store.agents.some(agent => agent.runtime_status === 'stale'),
)

const lastUpdatedLabel = computed(() =>
  store.lastUpdatedAt ? formatDateTime(store.lastUpdatedAt) : '-',
)

const permissionDenied = computed(() =>
  /forbidden|access denied|禁止|权限/i.test(store.errors.agents),
)

const safeErrorMessage = computed(() =>
  permissionDenied.value ? t('agentGuard.states.permissionDenied') : t('common.messages.requestFailed'),
)

const listQueryFingerprint = computed(() => JSON.stringify({
  filters: parsedQuery.value.filters,
  page: parsedQuery.value.page,
  pageSize: parsedQuery.value.pageSize,
  mode: props.mode,
}))

const detailFingerprint = computed(() => JSON.stringify({
  ...parsedQuery.value.detail,
  mode: props.mode,
}))

watch(listQueryFingerprint, async () => {
  const current = parsedQuery.value
  Object.assign(draftFilters, {
    ...current.filters,
    agent_types: [...current.filters.agent_types],
  })
  await store.fetchPage(toAgentQuery(current.filters, current.page, current.pageSize))
}, { immediate: true })

watch(detailFingerprint, async () => {
  const detail = parsedQuery.value.detail
  const scope = detail.assetId || detail.scopeKey || detail.instanceId
    || detail.findingId || detail.eventId || detail.sessionId
  if (!scope) return

  if (detailScopeLoaded.value !== scope) {
    detailScopeLoaded.value = scope
    store.resetDetail()
    selectedSessionIds.value = []
    if (detail.assetId || detail.scopeKey || detail.instanceId) {
      await store.fetchInstances(buildAgentGuardDetailInstanceQuery(detail))
      if (store.instances.length && props.mode === 'behavior') {
        if (detail.instanceId) {
          await store.fetchSessions(detail.instanceId)
        } else {
          await store.fetchSessionsForInstances(store.instances.map(instance => instance.id))
        }
      }
    }

	if (props.mode === 'behavior' && (detail.assetId || detail.scopeKey) && !detail.instanceId && store.instances.length) {
		const preferred = selectPreferredAgentGuardInstance(store.instances, store.sessions)
		if (!preferred) return
		await changeInstance(preferred.id)
      return
    }
  }

  await loadDetailTab(detail.tab)
}, { immediate: true })

onMounted(() => {
  realtimeStopped = false
  connectRealtime()
  actionPollTimer = setInterval(pollPendingAction, 3000)
})

onBeforeUnmount(() => {
  realtimeStopped = true
  if (realtimeRefreshTimer) clearTimeout(realtimeRefreshTimer)
  if (realtimeReconnectTimer) clearTimeout(realtimeReconnectTimer)
  if (actionPollTimer) clearInterval(actionPollTimer)
  realtimeSocket?.close()
  realtimeSocket = null
})

function toAgentQuery(filters: AgentGuardListFilters, page: number, pageSize: number): AgentGuardAgentQuery {
  return {
    host_ids: filters.host_id ? [filters.host_id] : undefined,
    agent_types: filters.agent_types.length ? filters.agent_types : undefined,
    runtime_status: filters.runtime_status || undefined,
    coverage: filters.coverage || undefined,
    isolation_type: props.mode === 'escape' ? filters.isolation_type || undefined : undefined,
    keyword: filters.keyword || undefined,
    page,
    page_size: pageSize,
  }
}

async function loadDetailTab(tab: AgentGuardDetailTab) {
  const detail = parsedQuery.value.detail
	if (props.mode === 'escape') tab = 'analysis'
  if (tab === 'analysis' && detail.findingId) {
    const scope = findingScopeParams()
    const [finding] = await Promise.all([
      store.fetchFinding(detail.findingId, scope),
      props.mode === 'escape' ? store.fetchEscapeRules() : store.fetchBuiltinRules(),
    ])
    if (finding) await loadFindingPage(1, true)
    return
  }
  if (tab === 'analysis' && detail.eventId) {
    await Promise.all([
      store.fetchBehavior(detail.eventId),
      props.mode === 'escape' ? store.fetchEscapeRules() : store.fetchBuiltinRules(),
    ])
    return
  }
  // Analysis is a separate tab in behavior mode too. Keep this branch before
  // the behavior-panorama branch; otherwise the tab change only reloads the
  // process tree and never requests the scoped finding list.
  if (tab === 'analysis') {
    if (props.mode === 'escape') {
      await loadFindingPage(1)
      await store.fetchEscapeRules()
      return
    }
    if (!(await ensureCurrentSessionForAnalysis())) return
    await Promise.all([
      loadFindingPage(1),
      store.fetchBuiltinRules(),
    ])
    return
  }
  const selected = selectedAgent.value
  const scopeParams = selected?.agent_scope_key
    ? { agent_scope_key: selected.agent_scope_key }
    : selected?.asset_id
      ? { asset_id: selected.asset_id }
      : {}
  if (props.mode === 'behavior') {
    if (!detail.instanceId) return
    const sessions = store.sessions.filter(session => session.instance_id === detail.instanceId).length
      ? store.sessions.filter(session => session.instance_id === detail.instanceId)
      : await store.fetchSessions(detail.instanceId, store.sessionPage)
    const sessionId = detail.sessionId || sessions[0]?.id
    if (!sessionId) {
      store.panoramaNodes = []
      return
    }
    if (sessionId !== detail.sessionId) {
      await changeSession(sessionId)
      return
    }
    await store.fetchPanorama({
      ...scopeParams,
      instance_ids: [detail.instanceId],
      session_id: sessionId,
      page: store.panoramaPage,
      page_size: store.panoramaPageSize,
    })
    return
  }

	// Escape detail intentionally has no sessions or sandbox panorama. Its
	// only detail surface is the finding evidence chain loaded above.
}

function applyFilters() {
  router.replace({
    query: serializeAgentGuardListQuery(draftFilters, 1, parsedQuery.value.pageSize),
  })
}

function resetFilters() {
  Object.assign(draftFilters, {
    ...DEFAULT_AGENT_GUARD_FILTERS,
    agent_types: [],
  })
  router.replace({
    query: serializeAgentGuardListQuery(draftFilters, 1, parsedQuery.value.pageSize),
  })
}

async function refreshPage() {
  const current = parsedQuery.value
  await store.fetchPage(toAgentQuery(current.filters, current.page, current.pageSize))
  if (drawerVisible.value) await loadDetailTab(current.detail.tab)
}

function changePage(page: number) {
  router.replace({
    query: serializeAgentGuardListQuery(parsedQuery.value.filters, page, parsedQuery.value.pageSize),
  })
}

function changePageSize(pageSize: number) {
  router.replace({
    query: serializeAgentGuardListQuery(parsedQuery.value.filters, 1, pageSize),
  })
}

function openAgent(agent: AgentGuardAgentSummary) {
  router.replace({
    query: withAgentGuardDetailQuery(route.query, {
      // The outer row is a logical host/type scope. Prefer its signed scope so
      // assetless runtimes and runtimes linked to historical aliases stay in
      // the same drawer. Legacy asset-id deep links remain supported.
      assetId: undefined,
      scopeKey: agent.agent_scope_key,
      tab: 'panorama',
    }),
  })
}

function closeDrawer() {
  detailScopeLoaded.value = ''
  router.replace({ query: clearAgentGuardDetailQuery(route.query) })
}

function changeDetailTab(tab: AgentGuardDetailTab) {
  const detail = parsedQuery.value.detail
  router.replace({
    query: withAgentGuardDetailQuery(route.query, {
      ...detail,
      tab,
    }),
  })
}

async function changeInstance(instanceId: string) {
  const detail = parsedQuery.value.detail
  store.panoramaPage = 1
  await router.replace({
    query: withAgentGuardDetailQuery(route.query, {
      ...detail,
      instanceId,
      sessionId: undefined,
      tab: detail.tab,
    }),
  })
}

async function changeSession(sessionId: string) {
  const detail = parsedQuery.value.detail
  const session = store.sessions.find(item => item.id === sessionId)
  store.panoramaPage = 1
  await router.replace({
    query: withAgentGuardDetailQuery(route.query, {
      ...detail,
      instanceId: session?.instance_id || detail.instanceId,
      sessionId,
      tab: detail.tab,
    }),
  })
}

async function deleteSessions(sessionIds: string[]) {
  const ids = [...new Set(sessionIds.filter(Boolean))]
  if (!ids.length) return
  try {
    await ElMessageBox.confirm(
      t('agentGuard.drawer.deleteSessionsConfirm', { count: ids.length }),
      t('agentGuard.drawer.deleteSessionsTitle'),
      { type: 'warning', confirmButtonText: t('common.actions.confirm'), cancelButtonText: t('common.actions.cancel') },
    )
    const current = parsedQuery.value.detail
    await store.deleteSessions(ids)
    selectedSessionIds.value = []
    if (current.sessionId && ids.includes(current.sessionId)) {
      const replacement = store.sessions[0]
      if (replacement) {
        await changeSession(replacement.id)
        return
      }
      await router.replace({
        query: withAgentGuardDetailQuery(route.query, {
          ...current,
          sessionId: undefined,
        }),
      })
      return
    }
    if (current.instanceId) {
      await store.fetchSessions(current.instanceId, Math.min(store.sessionPage, Math.max(1, Math.ceil(store.sessionTotal / store.sessionPageSize))))
    }
    await loadDetailTab(current.tab)
    ElMessage.success(t('agentGuard.drawer.deleteSessionsSuccess', { count: ids.length }))
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(t('common.messages.requestFailed'))
  }
}

async function changeSessionPage(page: number) {
  store.sessionPage = page
  const detail = parsedQuery.value.detail
  if (detail.instanceId) {
    await store.fetchSessions(detail.instanceId, page)
    if (detail.tab === 'panorama') await loadDetailTab('panorama')
  }
}

function retryDetail(target: 'instances' | 'panorama' | 'analysis') {
  const detail = parsedQuery.value.detail
  if (target === 'instances') {
    return store.fetchInstances(buildAgentGuardDetailInstanceQuery(detail))
  }
  return loadDetailTab(target === 'analysis' ? 'analysis' : 'panorama')
}

async function selectPanoramaNode(node: { node_type: string; object_id?: string; execution_unit_id?: string }) {
  store.selectPanoramaNode(node as any)
  const unitID = node.execution_unit_id || (node.node_type === 'execution_unit' ? node.object_id : '')
  if (unitID) {
    await Promise.all([
      store.fetchExecutionUnit(unitID),
      store.fetchExecutionUnitTimeline(unitID),
    ])
  }
}

async function executeUnitAction(
  action: Extract<AgentGuardActionName,
    'freeze_execution_unit' | 'resume_execution_unit' | 'kill_execution_unit'>,
  payload: AgentGuardActionRequest,
) {
  const unit = store.selectedExecutionUnit
  if (!unit) return
  try {
    await store.executeUnitAction(unit.id, action, payload)
    ElMessage.success(t('agentGuard.actions.accepted'))
  } catch {
    ElMessage.error(t('agentGuard.actions.failed'))
  }
}

async function selectFinding(id: string) {
  await store.fetchFinding(id, findingScopeParams())
}

function findingScopeParams() {
  const detail = parsedQuery.value.detail
  const finding = store.selectedFinding
  const instanceId = detail.instanceId || finding?.instance_id
  const sessionId = detail.sessionId || finding?.session_id
  if (sessionId) return { instance_id: instanceId || undefined, session_id: sessionId }
  if (instanceId) return { instance_id: instanceId }
  if (finding?.agent_scope_key) return { agent_scope_key: finding.agent_scope_key }
  if (finding?.asset_id) return { asset_id: finding.asset_id }
  if (selectedAgent.value?.agent_scope_key && selectedAgent.value.agent_scope_key !== 'unresolved') {
    return { agent_scope_key: selectedAgent.value.agent_scope_key }
  }
  if (selectedAgent.value?.asset_id) return { asset_id: selectedAgent.value.asset_id }
  return {}
}

async function loadFindingPage(page: number, preserveSelection = false) {
  const scope = findingScopeParams()
  if (props.mode === 'behavior' && !scope.session_id) return
  if (!scope.instance_id && !scope.session_id && !scope.agent_scope_key && !scope.asset_id) return
  await store.fetchFindings({
    ...scope,
    finding_domain: props.mode === 'behavior' ? 'tool' : 'escape',
    page,
    page_size: store.findingPageSize,
  })
  if (!preserveSelection && store.findings[0]) {
    await store.fetchFinding(store.findings[0].id, {
      instance_id: scope.instance_id,
      session_id: scope.session_id,
    })
  }
}

async function ensureCurrentSessionForAnalysis(): Promise<boolean> {
  const detail = parsedQuery.value.detail
  let instanceId = detail.instanceId
  if (!instanceId) {
    const preferred = selectPreferredAgentGuardInstance(store.instances, store.sessions)
    if (!preferred) return false
    await changeInstance(preferred.id)
    return false
  }

  let sessions = store.sessions.filter(session => session.instance_id === instanceId)
  if (!sessions.length) {
    sessions = await store.fetchSessions(instanceId, store.sessionPage)
  }
  const sessionId = detail.sessionId || sessions[0]?.id
  if (!sessionId) return false
  if (sessionId !== detail.sessionId) {
    await changeSession(sessionId)
    return false
  }
  return true
}

async function changeFindingPage(page: number) {
  await loadFindingPage(page)
}

async function changePanoramaPage(page: number) {
  store.panoramaPage = page
  await loadDetailTab('panorama')
}

async function changeAnalysisPage(page: number) {
  const findingId = store.selectedFinding?.id || store.findings[0]?.id
  if (!findingId) return
  await store.fetchFindingAnalyses(findingId, {
    page,
    page_size: store.analysisPageSize,
  })
}

async function analyzeFinding(id: string) {
  await store.analyzeFinding(id)
}

function openEvidence(eventId: string) {
  const detail = parsedQuery.value.detail
  router.replace({
    query: withAgentGuardDetailQuery(route.query, {
      ...detail,
      eventId,
      findingId: undefined,
      tab: 'analysis',
    }),
  })
}

function connectRealtime() {
  if (realtimeStopped || realtimeSocket) return
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  realtimeSocket = new WebSocket(`${protocol}//${window.location.host}/api/v1/detection/runtime/ws`)
  realtimeSocket.onopen = () => {
    realtimeDisconnected.value = false
  }
  realtimeSocket.onmessage = event => {
    try {
      const message = JSON.parse(String(event.data || '{}')) as { type?: string }
      if (!message.type?.startsWith('agent_guard.')) return
      scheduleRealtimeRefresh(message.type)
    } catch {
      // An invalid message is ignored; API refresh remains the source of truth.
    }
  }
  realtimeSocket.onclose = () => {
    realtimeSocket = null
    if (!realtimeStopped) {
      realtimeDisconnected.value = true
      realtimeReconnectTimer = setTimeout(connectRealtime, 3000)
    }
  }
  realtimeSocket.onerror = () => {
    realtimeSocket?.close()
  }
}

function scheduleRealtimeRefresh(messageType: string) {
  if (realtimeRefreshTimer) clearTimeout(realtimeRefreshTimer)
  realtimeRefreshTimer = setTimeout(async () => {
    await refreshPage()
    if (messageType === 'agent_guard.delivery_updated' && policyDialogVisible.value) {
      await store.fetchPolicies()
    }
    if (messageType === 'agent_guard.action_updated' && store.selectedExecutionUnit) {
      await store.fetchExecutionUnitTimeline(store.selectedExecutionUnit.id)
      await store.fetchExecutionUnit(store.selectedExecutionUnit.id)
    }
  }, 300)
}

async function pollPendingAction() {
  if (actionPollInFlight || !drawerVisible.value || props.mode !== 'escape') return
  const unit = store.selectedExecutionUnit
  if (!unit || !store.actions.some(action => ['pending', 'dispatching', 'running'].includes(action.status))) return
  actionPollInFlight = true
  try {
    await Promise.all([
      store.fetchExecutionUnitTimeline(unit.id),
      store.fetchExecutionUnit(unit.id),
    ])
  } finally {
    actionPollInFlight = false
  }
}
</script>

<style scoped>
.agent-guard-hero {
  min-height: 126px;
}

.agent-guard-metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.metric-unavailable {
  color: var(--aegis-text-muted);
}

.guard-filter-form {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.filter-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 0 14px;
}

.filter-row :deep(.el-form-item) {
  min-width: 190px;
  margin-bottom: 14px;
}

.filter-row :deep(.el-select),
.filter-row :deep(.el-input) {
  width: 220px;
}

.filter-actions-row {
  align-items: center;
}

.filter-actions-row .keyword-field {
  flex: 1 1 440px;
}

.filter-actions-row .keyword-field :deep(.el-input) {
  width: 100%;
}

.agent-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.preserved-data-alert {
  margin-bottom: 14px;
}

.empty-help {
  max-width: 560px;
  margin: 8px auto 0;
  color: var(--aegis-text-muted);
}

@media (max-width: 1279px) {
  .agent-guard-page {
    min-width: 1000px;
  }
}
</style>

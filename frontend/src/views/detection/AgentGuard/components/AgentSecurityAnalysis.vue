<template>
  <div class="security-analysis">
    <div class="analysis-workspace">
      <section class="finding-list matched-rule-list">
        <header class="panel-header">
          <div>
            <h3>{{ t('agentGuard.analysis.matchedRules') }}</h3>
            <p>{{ t('agentGuard.analysis.matchedRulesHint') }}</p>
          </div>
          <el-tag size="small" effect="plain">{{ ruleRows.length }}</el-tag>
        </header>

        <el-empty
          v-if="ruleRows.length === 0"
          :description="t('agentGuard.analysis.noMatchedRules')"
        />
        <button
          v-for="row in ruleRows"
          v-else
          :key="row.id"
          class="finding-row"
          type="button"
          :class="{ selected: activeRule?.id === row.id }"
          @click="selectRule(row)"
        >
          <span>
            <strong>{{ row.name }}</strong>
            <small>{{ row.ruleKey || t('agentGuard.analysis.unclassifiedRule') }}</small>
          </span>
          <span class="finding-risk">
            <el-tag size="small" :type="severityType(row.severity)" effect="plain">
              {{ row.severity || 'info' }}
            </el-tag>
            <small>{{ t('agentGuard.analysis.eventCount', { count: row.eventCount }) }}</small>
          </span>
        </button>
        <el-pagination
          v-if="findingTotal > findingPageSize"
          class="finding-pagination"
          background
          small
          layout="total, prev, pager, next"
          :current-page="findingPage"
          :page-size="findingPageSize"
          :total="findingTotal"
          @current-change="emit('finding-page-change', $event)"
        />
      </section>

      <section class="finding-detail rule-tool-detail">
        <el-empty
          v-if="!activeRule"
          :description="t('agentGuard.analysis.selectMatchedRule')"
        />
        <template v-else>
          <header>
            <div>
              <h3>{{ activeRule.name }}</h3>
              <p>
                {{ activeRule.ruleKey || t('agentGuard.analysis.unclassifiedRule') }}
                <span v-if="activeRule.ruleVersion">@{{ activeRule.ruleVersion }}</span>
                · {{ t('agentGuard.analysis.eventCount', { count: activeRule.eventCount }) }}
              </p>
            </div>
            <el-button
              v-if="activeFinding && !isAIOnly(activeFinding)"
              size="small"
              :loading="analysisPending"
              @click="emit('analyze', activeFinding.id)"
            >
              {{ t('agentGuard.analysis.reanalyze') }}
            </el-button>
          </header>

          <div class="finding-section tool-call-section">
            <div class="section-title-row">
              <div>
                <h4>{{ t('agentGuard.analysis.matchedToolCalls') }}</h4>
                <p>{{ t('agentGuard.analysis.matchedToolCallsHint') }}</p>
              </div>
              <el-tag size="small" type="danger" effect="plain">
                {{ t('agentGuard.analysis.matchedEventCount', { count: activeRule.eventCount }) }}
              </el-tag>
            </div>
            <div v-if="activeRule.detail?.tool_calls?.length" class="tool-call-list">
              <article
                v-for="call in activeRule.detail.tool_calls"
                :key="call.event_id"
                class="tool-call-card"
              >
                <header>
                  <strong>{{ call.tool_name }}</strong>
                  <el-tag size="small" :type="call.outcome === 'failed' ? 'danger' : 'success'" effect="plain">
                    {{ call.outcome || 'unknown' }}
                  </el-tag>
                </header>
                <p class="tool-call-meta">
                  {{ call.occurred_at || '-' }}
                  <span v-if="call.pid"> · PID {{ call.pid }}</span>
                  <span v-if="call.ppid !== undefined"> · PPID {{ call.ppid }}</span>
                  <span v-if="call.correlation_status === 'unmatched'"> · {{ t('agentGuard.analysis.pidUnmatched') }}</span>
                  <span v-else-if="call.correlation_status"> · {{ call.correlation_status }}</span>
                </p>
                <div v-if="call.command" class="tool-command-row">
                  <span>{{ t('agentGuard.analysis.toolCommand') }}</span>
                  <code class="tool-command">{{ call.command }}</code>
                </div>
                <dl>
                  <div v-if="call.tool_input !== undefined">
                    <dt>{{ t('agentGuard.analysis.toolInput') }}</dt>
                    <dd>{{ formatToolPayload(call.tool_input) }}</dd>
                  </div>
                  <div v-if="call.tool_response !== undefined">
                    <dt>{{ t('agentGuard.analysis.toolResponse') }}</dt>
                    <dd>{{ formatToolPayload(call.tool_response) }}</dd>
                  </div>
                  <div v-if="call.command_line">
                    <dt>{{ t('agentGuard.analysis.correlatedCommandLine') }}</dt>
                    <dd>{{ call.command_line }}</dd>
                  </div>
                </dl>
              </article>
            </div>
            <el-empty
              v-else
              :description="t('agentGuard.analysis.noToolCalls')"
              :image-size="56"
            />
          </div>
        </template>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  AgentSecurityFindingRuleDetail,
  AgentSecurityFindingSummary,
  BuiltinAgentBehaviorRuleSummary,
} from '@/types/agentGuard'

interface MatchedRuleRow {
  id: string
  findingId: string
  name: string
  ruleKey: string
  ruleVersion?: number
  severity?: string
  eventCount: number
  detail?: AgentSecurityFindingRuleDetail
}

const props = withDefaults(defineProps<{
  rules: BuiltinAgentBehaviorRuleSummary[]
  findings: AgentSecurityFindingSummary[]
  selectedFinding?: AgentSecurityFindingSummary | null
  findingTotal?: number
  findingPage?: number
  findingPageSize?: number
  selectedFindingId: string
  analysisPending?: boolean
}>(), {
  selectedFinding: null,
  findingTotal: 0,
  findingPage: 1,
  findingPageSize: 20,
  analysisPending: false,
})

const emit = defineEmits<{
  (event: 'select-finding', id: string): void
  (event: 'analyze', id: string): void
  (event: 'finding-page-change', page: number): void
}>()

const { t, te } = useI18n()
const localRuleId = ref('')
const ruleNameKeys: Record<string, string> = {
  'AGB-BUILTIN-001': 'AGB-BUILTIN-001',
  'AGB-BUILTIN-002': 'AGB-BUILTIN-002',
  'AGB-BUILTIN-003': 'AGB-BUILTIN-003',
  'AGB-BUILTIN-004': 'AGB-BUILTIN-004',
  'AGB-BUILTIN-005': 'AGB-BUILTIN-005',
  access_container_runtime_socket: 'access_container_runtime_socket',
  join_external_namespace: 'join_external_namespace',
  mount_host_path: 'mount_host_path',
  write_cgroupfs: 'write_cgroupfs',
  credential_or_capability_gain: 'credential_or_capability_gain',
  isolation_baseline_drift: 'isolation_baseline_drift',
  leave_expected_cgroup: 'leave_expected_cgroup',
}

const ruleRows = computed<MatchedRuleRow[]>(() => props.findings.flatMap(finding => {
  const details = finding.matched_rules || []
  const hits = finding.rule_hits || []
  if (details.length) {
    return details.map(detail => ({
      id: `${finding.id}:${detail.rule_key}:${detail.rule_version || 0}`,
      findingId: finding.id,
      name: displayRuleName(detail.rule_key, detail.name),
      ruleKey: detail.rule_key,
      ruleVersion: detail.rule_version,
      severity: detail.severity || finding.severity,
      eventCount: detail.event_ids?.length || 0,
      detail,
    }))
  }
  if (hits.length) {
    return hits.map((hit, index) => {
      const rule = props.rules.find(item => item.rule_key === hit.rule_key &&
        (!hit.rule_version || item.rule_version === hit.rule_version))
      const eventIds = uniqueStrings([
        hit.event_id || '',
        ...(hit.event_ids || []),
        ...(hit.evidence_event_ids || []),
      ])
      return {
        id: `${finding.id}:${hit.rule_key || 'rule'}:${hit.rule_version || index}`,
        findingId: finding.id,
        name: displayRuleName(hit.rule_key || '', hit.rule_name || rule?.name || finding.title),
        ruleKey: hit.rule_key || '',
        ruleVersion: hit.rule_version,
        severity: hit.severity || finding.severity,
        eventCount: eventIds.length,
      }
    })
  }
  return [{
    id: `${finding.id}:unclassified`,
    findingId: finding.id,
    name: displayRuleName('', finding.title),
    ruleKey: '',
    ruleVersion: undefined,
    severity: finding.severity,
    eventCount: finding.evidence_event_ids?.length || 0,
  }]
}))

const activeRule = computed(() =>
  ruleRows.value.find(row => row.id === localRuleId.value) || ruleRows.value[0] || null,
)

const activeFinding = computed(() => {
  const row = activeRule.value
  if (!row) return null
  if (props.selectedFinding?.id === row.findingId) return props.selectedFinding
  return props.findings.find(item => item.id === row.findingId) || null
})

watch([ruleRows, () => props.selectedFindingId], ([rows, selectedFindingId]) => {
  const selectedRow = rows.find(row => row.findingId === selectedFindingId)
  if (!rows.some(row => row.id === localRuleId.value)) {
    localRuleId.value = selectedRow?.id || rows[0]?.id || ''
  }
}, { immediate: true })

function selectRule(row: MatchedRuleRow) {
  localRuleId.value = row.id
  if (row.findingId !== props.selectedFindingId) emit('select-finding', row.findingId)
}

function isAIOnly(finding: AgentSecurityFindingSummary) {
  return finding.decision_sources?.length === 1 && finding.decision_sources[0] === 'ai'
}

function uniqueStrings(values: string[]) {
  return [...new Set(values.map(value => value.trim()).filter(Boolean))]
}

function severityType(severity?: string) {
  if (severity === 'critical' || severity === 'high') return 'danger'
  if (severity === 'medium') return 'warning'
  if (severity === 'low') return 'success'
  return 'info'
}

function formatToolPayload(payload: unknown) {
  if (typeof payload === 'string') return payload
  try {
    return JSON.stringify(payload)
  } catch {
    return String(payload)
  }
}

function displayRuleName(ruleKey: string, fallback?: string) {
  const key = ruleNameKeys[ruleKey]
  const messageKey = key ? `agentGuard.analysis.ruleNames.${key}` : ''
  if (messageKey && te(messageKey)) return t(messageKey)
  return fallback || ruleKey || t('agentGuard.analysis.unclassifiedRule')
}
</script>

<style scoped>
.security-analysis {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.analysis-workspace {
  display: grid;
  grid-template-columns: minmax(280px, 2fr) minmax(520px, 5fr);
  gap: 14px;
}

.finding-list,
.finding-detail {
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--aegis-border);
  border-radius: 14px;
  background: #fff;
}

.panel-header,
.finding-detail > header,
.section-title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.panel-header h3,
.finding-detail h3,
.finding-section h4 {
  margin: 0;
}

.panel-header p,
.finding-detail p {
  margin: 5px 0 0;
  color: var(--aegis-text-muted);
}

.finding-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.finding-pagination {
  justify-content: flex-end;
  margin-top: 4px;
}

.finding-list > button {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  padding: 11px;
  border: 1px solid var(--aegis-border);
  border-radius: 10px;
  background: #fff;
  text-align: left;
  cursor: pointer;
}

.finding-list > button.selected {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.finding-list button > span {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.finding-list strong,
.finding-list small {
  overflow-wrap: anywhere;
}

.finding-list small,
.finding-detail small {
  color: var(--aegis-text-muted);
}

.finding-risk {
  align-items: flex-end;
  white-space: nowrap;
}

.finding-section {
  padding: 14px 0 0;
  border-top: 1px solid var(--aegis-border);
}

.tool-call-section {
  margin-top: 16px;
}

.section-title-row p {
  margin-bottom: 0;
}

.tool-call-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 62vh;
  margin-top: 12px;
  overflow: auto;
  padding-right: 4px;
}

.tool-call-card {
  padding: 12px;
  border: 1px solid var(--aegis-border);
  border-radius: 10px;
  background: var(--aegis-surface, #fff);
}

.tool-call-card > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.tool-call-meta {
  margin: 6px 0;
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.tool-command,
.tool-call-card dd {
  display: block;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.tool-command {
  margin: 8px 0;
  color: var(--aegis-text);
  font: 12px/1.45 ui-monospace, SFMono-Regular, Menlo, monospace;
}

.tool-call-card dl {
  margin: 0;
}

.tool-call-card dl > div {
  display: grid;
  grid-template-columns: 110px minmax(0, 1fr);
  gap: 10px;
  padding: 7px 0;
  border-top: 1px solid var(--aegis-border);
}

.tool-call-card dt {
  color: var(--aegis-text-muted);
}

.tool-call-card dd {
  margin: 0;
}

@media (max-width: 1200px) {
  .analysis-workspace {
    grid-template-columns: 1fr;
  }
}
</style>

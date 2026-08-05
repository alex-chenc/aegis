<template>
  <div class="escape-analysis">
    <div class="analysis-workspace">
      <section class="finding-list escape-finding-list">
        <header class="panel-header">
          <div>
            <h3>{{ t('agentGuard.drawer.escapeAnalysisDetail.alertList') }}</h3>
            <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.alertListHint') }}</p>
          </div>
          <el-tag size="small" effect="plain">{{ findingCount }}</el-tag>
        </header>

        <el-empty
          v-if="findings.length === 0"
          :description="t('agentGuard.drawer.escapeAnalysisDetail.noAlerts')"
        />
        <button
          v-for="row in findings"
          v-else
          :key="row.id"
          class="finding-row"
          type="button"
          :class="{ selected: activeFinding?.id === row.id }"
          @click="emit('select-finding', row.id)"
        >
          <span class="finding-row-main">
            <strong>{{ ruleLabel(row) }}</strong>
            <small class="finding-row-title" :title="row.title || ''">{{ row.title || t('agentGuard.drawer.escapeAnalysisDetail.untitled') }}</small>
            <small>{{ formatObservedAt(row.last_observed_at) }}</small>
          </span>
          <span class="finding-risk">
            <el-tag size="small" :type="severityType(row.severity)" effect="plain">
              {{ row.severity || 'info' }}
            </el-tag>
            <small>{{ verdictLabel(row.verdict) }}</small>
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

      <section class="finding-detail">
        <el-empty
          v-if="!activeFinding"
          :description="t('agentGuard.drawer.escapeAnalysisDetail.noFinding')"
        />
        <template v-else>
          <el-alert
            :type="alertType"
            :title="warningTitle"
            :description="warningDescription"
            :closable="false"
            show-icon
          />
          <div class="finding-heading">
            <div>
              <h3>{{ activeFinding.title }}</h3>
              <p>
                {{ ruleLabel(activeFinding) }}
                · {{ t('agentGuard.drawer.escapeAnalysisDetail.verdict') }}: {{ verdictLabel(activeFinding.verdict) }}
                · {{ t('agentGuard.drawer.escapeAnalysisDetail.observedAt') }}: {{ formatObservedAt(activeFinding.last_observed_at) }}
              </p>
            </div>
            <el-tag :type="severityType(activeFinding.severity)" effect="plain">
              {{ activeFinding.severity || 'info' }}
            </el-tag>
          </div>
          <p v-if="activeFinding.summary" class="finding-summary">{{ activeFinding.summary }}</p>

          <article class="detail-section">
            <header class="detail-section-header">
              <div>
                <h4>{{ t('agentGuard.drawer.escapeAnalysisDetail.hook') }}</h4>
                <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.hookHint') }}</p>
              </div>
              <el-tag size="small" effect="plain">{{ hookEventIds.length }}</el-tag>
            </header>
            <div v-if="hookEventIds.length" class="event-id-list">
              <code v-for="eventId in hookEventIds" :key="eventId">{{ eventId }}</code>
            </div>
            <p v-else class="detail-empty">{{ t('agentGuard.drawer.escapeAnalysisDetail.noEvidence') }}</p>
            <div v-if="toolCalls.length" class="tool-call-list">
              <article v-for="(call, index) in toolCalls" :key="call.event_id || `${call.tool_name}-${index}`" class="tool-call-card">
                <div class="field-grid">
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.tool') }}</span><strong>{{ call.tool_name || '-' }}</strong></div>
                  <div class="field field-wide"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.command') }}</span><code>{{ toolCommand(call) }}</code></div>
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.eventId') }}</span><code>{{ call.event_id || '-' }}</code></div>
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.outcome') }}</span><strong>{{ call.outcome || '-' }}</strong></div>
                </div>
              </article>
            </div>
            <p v-else class="detail-empty">{{ t('agentGuard.drawer.escapeAnalysisDetail.noToolEvidence') }}</p>
          </article>

          <article class="detail-section">
            <header class="detail-section-header">
              <div>
                <h4>{{ t('agentGuard.drawer.escapeAnalysisDetail.process') }}</h4>
                <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.processHint') }}</p>
              </div>
            </header>
            <div v-if="processRows.length" class="process-list">
              <article v-for="(process, index) in processRows" :key="`${process.pid}-${process.startTicks}-${index}`" class="process-card">
                <div class="field-grid">
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.processId') }}</span><strong>{{ process.pid || '-' }}</strong></div>
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.parentProcessId') }}</span><strong>{{ process.ppid || '-' }}</strong></div>
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.startTicks') }}</span><code>{{ process.startTicks || '-' }}</code></div>
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.status') }}</span><strong>{{ process.status || '-' }}</strong></div>
                  <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.processName') }}</span><code>{{ process.name || process.exe || '-' }}</code></div>
                  <div v-if="process.cmdline" class="field field-wide"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.commandLine') }}</span><code>{{ process.cmdline }}</code></div>
                  <div v-if="process.instance" class="field field-wide"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.instance') }}</span><code>{{ process.instance }}</code></div>
                </div>
              </article>
            </div>
            <p v-else class="detail-empty">{{ t('agentGuard.drawer.escapeAnalysisDetail.noProcessEvidence') }}</p>
          </article>

          <article class="detail-section">
            <header class="detail-section-header">
              <div>
                <h4>{{ t('agentGuard.drawer.escapeAnalysisDetail.proc') }}</h4>
                <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.procHint') }}</p>
              </div>
              <el-tag size="small" :type="procEvidence.length ? 'success' : 'warning'" effect="plain">
                {{ procEvidence.length ? t('agentGuard.drawer.escapeAnalysisDetail.evidenceFound') : t('agentGuard.drawer.escapeAnalysisDetail.evidenceMissing') }}
              </el-tag>
            </header>
            <div v-if="procRows.length" class="evidence-list">
              <div v-for="row in procRows" :key="`${row.key}-${row.value}`" class="evidence-row">
                <span>{{ prettyEvidenceKey(row.key) }}</span>
                <code>{{ row.value }}</code>
              </div>
            </div>
            <p v-else class="detail-empty">{{ t('agentGuard.drawer.escapeAnalysisDetail.noProcEvidence') }}</p>
            <div class="compliance-summary" :class="`compliance-${procCompliance.type}`">
              <strong>{{ procCompliance.label }}</strong>
              <span>{{ procCompliance.reason }}</span>
            </div>
            <div v-if="allGaps.length" class="reason-box">
              <strong>{{ t('agentGuard.drawer.escapeAnalysisDetail.reason') }}</strong>
              <span v-for="reason in allGaps" :key="reason">{{ reason }}</span>
            </div>
          </article>

          <article class="detail-section">
            <header class="detail-section-header">
              <div>
                <h4>{{ t('agentGuard.drawer.escapeAnalysisDetail.decision') }}</h4>
                <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.decisionHint') }}</p>
              </div>
              <el-tag :type="severityType(activeFinding.severity)" effect="plain">{{ activeFinding.severity || 'info' }}</el-tag>
            </header>
            <div class="decision-list">
              <div class="decision-row"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.verdict') }}</span><strong>{{ verdictLabel(activeFinding.verdict) }}</strong></div>
              <div class="decision-row"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.reverification') }}</span><strong>{{ reverificationLabel(reverification) }}</strong></div>
              <div class="decision-row"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.findingStatus') }}</span><strong>{{ statusLabel(activeFinding.status) }}</strong></div>
              <div class="decision-row"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.recommendedAction') }}</span><strong>{{ actionLabel(activeFinding.recommended_action) }}</strong></div>
            </div>
          </article>
        </template>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  AgentSecurityFindingProcessNode,
  AgentSecurityFindingSummary,
  AgentSecurityFindingToolCall,
} from '@/types/agentGuard'

const props = withDefaults(defineProps<{
  findings?: AgentSecurityFindingSummary[]
  finding?: AgentSecurityFindingSummary | null
  selectedFindingId?: string
  findingTotal?: number
  findingPage?: number
  findingPageSize?: number
}>(), {
  findings: () => [],
  finding: null,
  selectedFindingId: '',
  findingTotal: 0,
  findingPage: 1,
  findingPageSize: 20,
})

const emit = defineEmits<{
  (event: 'select-finding', findingId: string): void
  (event: 'finding-page-change', page: number): void
}>()

const { t, te, locale } = useI18n()

const activeFinding = computed(() => props.finding || props.findings.find(item => item.id === props.selectedFindingId) || props.findings[0] || null)
const findingCount = computed(() => props.findingTotal || props.findings.length)
const reverification = computed(() => activeFinding.value?.escape_chain?.reverification || 'inconclusive')
const isBaselineDrift = computed(() => activeFinding.value?.rule_hits?.some(hit => hit.rule_key === 'isolation_baseline_drift') || activeFinding.value?.matched_rules?.some(rule => rule.rule_key === 'isolation_baseline_drift'))
const alertType = computed(() => {
  if (activeFinding.value?.verdict === 'malicious' || activeFinding.value?.severity === 'critical' || activeFinding.value?.severity === 'high') return 'error'
  return reverification.value === 'complete' ? 'success' : 'warning'
})
const warningTitle = computed(() => {
  if (reverification.value === 'complete') return t('agentGuard.drawer.escapeAnalysisDetail.warningCompleteTitle')
  return isBaselineDrift.value
    ? t('agentGuard.drawer.escapeAnalysisDetail.warningTitle')
    : t('agentGuard.drawer.escapeAnalysisDetail.warningSignalTitle')
})
const warningDescription = computed(() => {
  if (reverification.value === 'complete') return t('agentGuard.drawer.escapeAnalysisDetail.warningComplete')
  if (reverification.value === 'partial') return t('agentGuard.drawer.escapeAnalysisDetail.warningPartial')
  return t(isBaselineDrift.value
    ? 'agentGuard.drawer.escapeAnalysisDetail.warningInconclusive'
    : 'agentGuard.drawer.escapeAnalysisDetail.warningSignalInconclusive')
})

const allGaps = computed(() => unique([
  ...(activeFinding.value?.evidence_completeness?.reasons || []),
  ...(activeFinding.value?.escape_chain?.gaps || []),
]))

const toolCalls = computed<AgentSecurityFindingToolCall[]>(() => {
  const matchedCalls = (activeFinding.value?.matched_rules || []).flatMap(rule => rule.tool_calls || [])
  const hookCalls: AgentSecurityFindingToolCall[] = (activeFinding.value?.escape_chain?.hook_events || []).map(event => ({
    event_id: event.event_id,
    tool_name: event.tool_name || event.process_name || event.event_type || 'Hook',
    command: event.command || event.command_line,
    occurred_at: undefined,
    outcome: event.outcome,
    pid: event.pid,
    ppid: event.ppid,
    process_start_ticks: event.process_start_ticks,
    command_line: event.command_line,
    correlation_status: event.decision,
  }))
  return uniqueBy([...matchedCalls, ...hookCalls], call => call.event_id)
})

const hookEventIds = computed(() => unique([
  ...(activeFinding.value?.escape_chain?.hook_event_ids || []),
  ...(activeFinding.value?.evidence_event_ids || []),
  ...((activeFinding.value?.rule_hits || []).flatMap(hit => hit.evidence_event_ids || [])),
  ...toolCalls.value.map(call => call.event_id),
]))

const processRows = computed(() => {
  const finding = activeFinding.value
  const rows: ProcessRow[] = []
  for (const process of finding?.escape_chain?.process_evidence || []) rows.push(normalizeProcess(process))
  for (const rule of finding?.matched_rules || []) {
    for (const process of rule.process_tree || []) rows.push(normalizeProcess(process))
  }
  for (const call of toolCalls.value) {
    if (call.pid || call.ppid || call.process_start_ticks || call.command_line) {
      rows.push(normalizeProcess({
        pid: call.pid, ppid: call.ppid, start_ticks: call.process_start_ticks,
        cmdline: call.command_line, name: call.tool_name,
      }))
    }
  }
  const uniqueRows = uniqueBy(rows, row => `${row.pid}|${row.ppid}|${row.startTicks}|${row.cmdline}`)
  if (uniqueRows.length === 0 && finding?.instance_id) {
    uniqueRows.push(normalizeProcess({ instance: finding.instance_id }))
  }
  return uniqueRows
})

const procEvidence = computed(() => activeFinding.value?.escape_chain?.proc_cgroup_evidence || [])
const procRows = computed(() => {
  const rows: EvidenceRow[] = []
  for (const item of procEvidence.value) {
    flattenEvidence(item, '', rows)
  }
  return uniqueBy(rows, row => `${row.key}|${row.value}`)
})
const procCompliance = computed<ComplianceSummary>(() => {
  const evidence = procEvidence.value[0] || {}
  const actual = evidence.actual as Record<string, unknown> | undefined
  const baseline = evidence.baseline as Record<string, unknown> | undefined
  const actualCgroup = valueString(actual?.cgroup_path)
  const baselineCgroup = valueString(baseline?.cgroup_path)
  if (!actualCgroup || !baselineCgroup) {
    return {
      type: 'unknown',
      label: t('agentGuard.drawer.escapeAnalysisDetail.complianceUnknown'),
      reason: t('agentGuard.drawer.escapeAnalysisDetail.complianceUnknownReason'),
    }
  }
  if (actualCgroup === baselineCgroup) {
    return {
      type: 'compliant',
      label: t('agentGuard.drawer.escapeAnalysisDetail.compliant'),
      reason: t('agentGuard.drawer.escapeAnalysisDetail.compliantReason', { path: actualCgroup }),
    }
  }
  return {
    type: 'not-compliant',
    label: t('agentGuard.drawer.escapeAnalysisDetail.notCompliant'),
    reason: t('agentGuard.drawer.escapeAnalysisDetail.notCompliantReason', { expected: baselineCgroup, observed: actualCgroup }),
  }
})

interface ProcessRow {
  instance?: string
  pid?: string
  ppid?: string
  startTicks?: string
  name?: string
  exe?: string
  cmdline?: string
  status?: string
}

interface EvidenceRow { key: string; value: string }
interface ComplianceSummary { type: 'compliant' | 'not-compliant' | 'unknown'; label: string; reason: string }

function normalizeProcess(process: Record<string, unknown> | AgentSecurityFindingProcessNode): ProcessRow {
  const raw = process as Record<string, unknown>
  return {
    instance: valueString(raw.instance),
    pid: valueString(process.pid), ppid: valueString(process.ppid),
    startTicks: valueString(process.start_ticks ?? process.process_start_ticks),
    name: valueString(process.name ?? process.process_name),
    exe: valueString(process.exe ?? process.process_exe),
    cmdline: valueString(process.cmdline ?? process.command_line),
    status: valueString(process.status ?? process.process_status),
  }
}

function toolCommand(call: AgentSecurityFindingToolCall) {
  return call.command || call.command_line || extractCommand(call.tool_input) || '-'
}

function extractCommand(input: unknown): string {
  if (!input) return ''
  if (typeof input === 'string') return input
  if (typeof input === 'object' && input !== null) {
    const record = input as Record<string, unknown>
    for (const key of ['command', 'cmd', 'command_line', 'script']) {
      if (typeof record[key] === 'string' && record[key]) return record[key] as string
    }
  }
  return formatValue(input)
}

function valueString(value: unknown) {
  if (value === undefined || value === null || value === '') return undefined
  return String(value)
}

function formatValue(value: unknown): string {
  if (typeof value === 'string') return value
  try { return JSON.stringify(value) || String(value) } catch { return String(value) }
}

function prettyEvidenceKey(key: string) {
  const normalized = key.replace(/[._-]+/g, ' ')
  return normalized.charAt(0).toUpperCase() + normalized.slice(1)
}

function flattenEvidence(value: unknown, prefix: string, rows: EvidenceRow[], depth = 0) {
  if (value === undefined || value === null || value === '') return
  if (depth >= 3 || typeof value !== 'object') {
    rows.push({ key: prefix || 'evidence', value: formatValue(value) })
    return
  }
  if (Array.isArray(value)) {
    const shown = value.length > 8 ? `${formatValue(value.slice(0, 8))} … (+${value.length - 8})` : formatValue(value)
    rows.push({ key: prefix || 'evidence', value: shown })
    return
  }
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
    flattenEvidence(child, prefix ? `${prefix}.${key}` : key, rows, depth + 1)
  }
}

function uniqueBy<T>(items: T[], keyOf: (item: T) => string): T[] {
  const seen = new Set<string>()
  return items.filter(item => {
    const key = keyOf(item)
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function ruleLabel(finding: AgentSecurityFindingSummary) {
  const key = finding.rule_hits?.[0]?.rule_key || finding.matched_rules?.[0]?.rule_key
  if (!key) return t('agentGuard.drawer.escapeAnalysisDetail.unclassifiedRule')
  const path = `agentGuard.analysis.ruleNames.${key}`
  return te(path) ? t(path) : key
}

function verdictLabel(verdict?: string) {
  if (!verdict) return t('agentGuard.drawer.escapeAnalysisDetail.unknownVerdict')
  const path = `agentGuard.drawer.escapeAnalysisDetail.verdicts.${verdict}`
  return te(path) ? t(path) : verdict
}

function reverificationLabel(value?: string) {
  if (!value) return t('agentGuard.drawer.escapeAnalysisDetail.unknownVerdict')
  const path = `agentGuard.drawer.escapeAnalysisDetail.reverificationStates.${value}`
  return te(path) ? t(path) : value
}

function statusLabel(value?: string) {
  if (!value) return '-'
  const path = `agentGuard.drawer.escapeAnalysisDetail.statuses.${value}`
  return te(path) ? t(path) : value
}

function actionLabel(value?: string) {
  if (!value) return '-'
  const path = `agentGuard.drawer.escapeAnalysisDetail.actions.${value}`
  return te(path) ? t(path) : value
}

function formatObservedAt(value?: string) {
  if (!value) return t('agentGuard.drawer.escapeAnalysisDetail.unknownTime')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short' }).format(date)
}

function severityType(severity?: string) {
  if (severity === 'critical' || severity === 'high') return 'danger'
  if (severity === 'medium') return 'warning'
  if (severity === 'low') return 'info'
  return 'success'
}

function unique(items: string[]) { return [...new Set(items.filter(Boolean))] }
</script>

<style scoped>
.escape-analysis { display: grid; gap: 18px; }
.analysis-workspace { display: grid; min-width: 0; grid-template-columns: minmax(320px, 360px) minmax(0, 1fr); gap: 16px; align-items: start; }
.finding-list, .finding-detail { box-sizing: border-box; min-width: 0; max-width: 100%; padding: 14px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-fill-color-blank); }
.finding-list { display: grid; gap: 8px; overflow: hidden; }
.panel-header, .finding-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.panel-header > div, .finding-heading > div { min-width: 0; }
.panel-header h3, .finding-heading h3 { margin: 0; color: var(--aegis-text); font-size: 16px; }
.panel-header p, .finding-heading p, .finding-summary { margin: 5px 0 0; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
.finding-row { box-sizing: border-box; display: flex; width: 100%; max-width: 100%; min-width: 0; align-items: flex-start; justify-content: space-between; gap: 8px; padding: 10px; border: 1px solid transparent; border-radius: 8px; background: transparent; color: inherit; text-align: left; cursor: pointer; }
.finding-row:hover, .finding-row.selected { border-color: var(--el-color-primary-light-5); background: var(--el-color-primary-light-9); }
.finding-row-main { min-width: 0; display: grid; gap: 3px; }
.finding-row strong, .finding-row small { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.finding-row strong { font-size: 13px; line-height: 1.35; overflow-wrap: anywhere; }
.finding-row small { color: var(--el-text-color-secondary); font-size: 11px; white-space: nowrap; }
.finding-row .finding-row-title { display: -webkit-box; white-space: normal; overflow-wrap: anywhere; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-height: 1.35; }
.finding-risk { display: grid; flex: none; justify-items: end; gap: 3px; }
.finding-risk small { color: var(--el-text-color-secondary); font-size: 11px; }
.finding-pagination { margin-top: 4px; }
.finding-detail { display: grid; max-height: calc(100vh - 230px); gap: 14px; overflow-y: auto; }
.finding-summary { margin: -4px 0 0; font-size: 13px; }
.detail-section { display: grid; gap: 12px; padding: 16px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-fill-color-blank); }
.detail-section-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding-bottom: 8px; border-bottom: 1px solid var(--el-border-color-lighter); }
.detail-section-header h4 { margin: 0; color: var(--aegis-text); font-size: 16px; }
.detail-section-header p { margin: 4px 0 0; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.5; }
.event-id-list { display: flex; flex-wrap: wrap; gap: 8px; }
.event-id-list code, .field code, .evidence-row code { overflow-wrap: anywhere; font-family: var(--el-font-family); font-size: 12px; }
.event-id-list code { padding: 5px 8px; border-radius: 6px; background: var(--el-fill-color-light); }
.tool-call-list, .process-list { display: grid; gap: 10px; }
.tool-call-card, .process-card { padding: 12px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-fill-color-lighter); }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 18px; }
.field { min-width: 0; display: grid; gap: 3px; }
.field-wide { grid-column: 1 / -1; }
.field span, .evidence-row span, .decision-row span { color: var(--el-text-color-secondary); font-size: 12px; }
.field strong, .decision-row strong { overflow-wrap: anywhere; font-size: 13px; font-weight: 500; }
.detail-empty { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; }
.evidence-list, .decision-list { display: grid; gap: 0; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; overflow: hidden; }
.evidence-row, .decision-row { display: grid; grid-template-columns: minmax(130px, 0.35fr) minmax(0, 1fr); gap: 16px; padding: 10px 12px; border-bottom: 1px solid var(--el-border-color-lighter); }
.evidence-row:last-child, .decision-row:last-child { border-bottom: 0; }
.reason-box { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; color: var(--el-text-color-secondary); font-size: 12px; }
.reason-box span { padding: 4px 8px; border-radius: 12px; background: var(--el-fill-color-light); }
.compliance-summary { display: grid; gap: 3px; padding: 10px 12px; border-left: 3px solid var(--el-color-info); background: var(--el-fill-color-lighter); font-size: 12px; }
.compliance-summary span { color: var(--el-text-color-secondary); }
.compliance-compliant { border-left-color: var(--el-color-success); }
.compliance-not-compliant { border-left-color: var(--el-color-danger); }
.compliance-unknown { border-left-color: var(--el-color-warning); }
@media (max-width: 1100px) { .analysis-workspace { grid-template-columns: 1fr; } .finding-detail { max-height: none; } }
@media (max-width: 640px) { .field-grid, .evidence-row, .decision-row { grid-template-columns: 1fr; gap: 4px; } }
</style>

<template>
  <div class="escape-analysis">
    <div class="analysis-workspace">
      <section class="finding-list escape-finding-list">
        <header class="panel-header">
          <div>
            <h3>{{ t('agentGuard.drawer.escapeAnalysisDetail.alertList') }}</h3>
            <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.permissionHint') }}</p>
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

          <article class="detail-section permission-section">
            <header class="detail-section-header">
              <div>
                <h4>{{ t('agentGuard.drawer.escapeAnalysisDetail.permission') }}</h4>
                <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.permissionHint') }}</p>
              </div>
              <el-tag size="small" :type="permissionTagType" effect="plain">{{ permissionLabel }}</el-tag>
            </header>
            <div class="permission-grid">
              <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.agentType') }}</span><strong>{{ agentLabel }}</strong></div>
              <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.backend') }}</span><code>{{ permission?.backend || '-' }}</code></div>
              <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.permissionClass') }}</span><strong>{{ permissionClassLabel }}</strong></div>
              <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.boundary') }}</span><strong>{{ boundaryLabel }}</strong></div>
              <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.sandboxMode') }}</span><code>{{ permission?.sandbox_mode || '-' }}</code></div>
              <div class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.networkAccess') }}</span><strong>{{ networkLabel }}</strong></div>
              <div v-if="permission?.workspace_access" class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.workspaceAccess') }}</span><strong>{{ permission.workspace_access }}</strong></div>
              <div v-if="permission?.safe_write_root" class="field field-wide"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.safeWriteRoot') }}</span><code>{{ permission.safe_write_root }}</code></div>
              <div v-if="permission?.allowed_domains?.length" class="field field-wide"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.allowedDomains') }}</span><code>{{ permission.allowed_domains.join(', ') }}</code></div>
              <div v-if="permission?.elevated" class="field"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.elevated') }}</span><strong>{{ locale === 'zh-CN' ? '是' : 'yes' }}</strong></div>
              <div class="field field-wide"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.workspaceRoots') }}</span><code>{{ rootsLabel(permission?.workspace_roots) }}</code></div>
              <div class="field field-wide"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.tempRoots') }}</span><code>{{ rootsLabel(permission?.temp_roots) }}</code></div>
            </div>
          </article>

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
                <h4>{{ t('agentGuard.drawer.escapeAnalysisDetail.execution') }}</h4>
                <p>{{ t('agentGuard.drawer.escapeAnalysisDetail.executionHint') }}</p>
              </div>
              <el-tag size="small" :type="executionEvidence.length ? 'success' : 'warning'" effect="plain">
                {{ executionEvidence.length ? t('agentGuard.drawer.escapeAnalysisDetail.evidenceFound') : t('agentGuard.drawer.escapeAnalysisDetail.evidenceMissing') }}
              </el-tag>
            </header>
            <div v-if="executionEvidence.length" class="evidence-list">
              <div v-for="(row, index) in executionEvidence" :key="`${row.event_id || 'execution'}-${index}`" class="evidence-row">
                <span>{{ row.operation || t('agentGuard.drawer.escapeAnalysisDetail.execution') }}</span>
                <code>{{ formatValue(row) }}</code>
              </div>
            </div>
            <p v-else class="detail-empty">{{ t('agentGuard.drawer.escapeAnalysisDetail.noExecutionEvidence') }}</p>
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
              <div class="decision-row"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.classification') }}</span><strong>{{ classificationLabel }}</strong></div>
              <div class="decision-row"><span>{{ t('agentGuard.drawer.escapeAnalysisDetail.verdict') }}</span><strong>{{ verdictLabel(activeFinding.verdict) }}</strong></div>
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
  AgentBehaviorSession,
  AgentSecurityFindingProcessNode,
  AgentSecurityFindingSummary,
  AgentSecurityFindingToolCall,
} from '@/types/agentGuard'

const props = withDefaults(defineProps<{
  findings?: AgentSecurityFindingSummary[]
  finding?: AgentSecurityFindingSummary | null
  session?: AgentBehaviorSession | null
  selectedFindingId?: string
  findingTotal?: number
  findingPage?: number
  findingPageSize?: number
}>(), {
  findings: () => [],
  finding: null,
  session: null,
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
const permission = computed(() => activeFinding.value?.escape_chain?.permission || props.session?.permission)
const permissionClass = computed(() => permission.value?.class || 'unknown')
const agentLabel = computed(() => {
  const value = permission.value?.agent_type || props.session?.source || '-'
  const names: Record<string, string> = { codex: 'Codex', 'claude-code': 'Claude Code', openclaw: 'OpenClaw', hermes: 'Hermes', zcode: 'Zcode' }
  return names[value] || value
})
const boundaryLabel = computed(() => {
  const value = permission.value?.boundary || '-'
  const names: Record<string, string> = { enforced: locale.value === 'zh-CN' ? '已执行边界' : 'enforced', none: locale.value === 'zh-CN' ? '无边界' : 'none', no_isolation: locale.value === 'zh-CN' ? '明确无隔离' : 'no isolation', remote_unobservable: locale.value === 'zh-CN' ? '远端不可观测' : 'remote unobservable' }
  return names[value] || value
})
const permissionLabel = computed(() => {
  if (permissionClass.value === 'full_access') return t('agentGuard.drawer.escapeAnalysisDetail.fullAccess')
  if (permissionClass.value === 'restricted') return t('agentGuard.drawer.escapeAnalysisDetail.restricted')
  return t('agentGuard.drawer.escapeAnalysisDetail.unknownPermission')
})
const permissionClassLabel = computed(() => {
  if (permissionClass.value === 'full_access') return t('agentGuard.drawer.escapeAnalysisDetail.fullAccess')
  if (permissionClass.value === 'restricted' && !permission.value?.permission_mode) return t('agentGuard.drawer.escapeAnalysisDetail.restricted')
  if (permissionClass.value === 'unknown') return t('agentGuard.drawer.escapeAnalysisDetail.unknownPermission')
  return permission.value?.permission_mode || permissionClass.value
})
const permissionTagType = computed(() => permissionClass.value === 'full_access' ? 'info' : permissionClass.value === 'restricted' ? 'warning' : 'danger')
const networkLabel = computed(() => permission.value?.network_access === true ? 'enabled' : permission.value?.network_access === false ? 'disabled' : '-')
const classification = computed(() => activeFinding.value?.escape_chain?.classification || 'policy_violation_attempt')
const classificationLabel = computed(() => {
  if (classification.value === 'confirmed_escape') return t('agentGuard.drawer.escapeAnalysisDetail.confirmedEscape')
  if (classification.value === 'authorized_boundary_expansion') return t('agentGuard.drawer.escapeAnalysisDetail.authorizedExpansion')
  return t('agentGuard.drawer.escapeAnalysisDetail.policyViolationAttempt')
})
const executionEvidence = computed(() => activeFinding.value?.escape_chain?.execution_evidence || [])
const alertType = computed(() => {
  if (activeFinding.value?.verdict === 'malicious' || activeFinding.value?.severity === 'critical' || activeFinding.value?.severity === 'high') return 'error'
  return classification.value === 'confirmed_escape' ? 'error' : 'warning'
})
const warningTitle = computed(() => {
  if (classification.value === 'confirmed_escape') return t('agentGuard.drawer.escapeAnalysisDetail.confirmedEscape')
  if (permissionClass.value === 'unknown') return t('agentGuard.drawer.escapeAnalysisDetail.unknownPermission')
  return t('agentGuard.drawer.escapeAnalysisDetail.policyViolationAttempt')
})
const warningDescription = computed(() => {
  if (classification.value === 'confirmed_escape') return t('agentGuard.drawer.escapeAnalysisDetail.warningConfirmed')
  if (permissionClass.value === 'unknown') return t('agentGuard.drawer.escapeAnalysisDetail.warningUnknown')
  return t('agentGuard.drawer.escapeAnalysisDetail.warningRestricted')
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

function normalizeProcess(process: Record<string, unknown> | AgentSecurityFindingProcessNode): ProcessRow {
  const raw = process as Record<string, unknown>
  return {
    instance: valueString(raw.instance),
    pid: valueString(raw.pid), ppid: valueString(raw.ppid),
    startTicks: valueString(raw.start_ticks ?? raw.process_start_ticks),
    name: valueString(raw.name ?? raw.process_name),
    exe: valueString(raw.exe ?? raw.process_exe),
    cmdline: valueString(raw.cmdline ?? raw.command_line),
    status: valueString(raw.status ?? raw.process_status),
  }
}

function toolCommand(call: AgentSecurityFindingToolCall) {
  return call.command || call.command_line || extractCommand(call.tool_input) || '-'
}

function rootsLabel(roots?: string[]) {
  return roots?.length ? roots.join(', ') : '-'
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
.escape-analysis, .analysis-workspace, .finding-list, .finding-detail, .detail-section { box-sizing: border-box; min-width: 0; width: 100%; max-width: 100%; }
.escape-analysis { display: grid; gap: 18px; overflow-x: hidden; }
.analysis-workspace { display: grid; min-width: 0; grid-template-columns: minmax(280px, 2fr) minmax(520px, 5fr); gap: 14px; align-items: start; }
.finding-list, .finding-detail { box-sizing: border-box; min-width: 0; max-width: 100%; padding: 14px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-fill-color-blank); }
.finding-list { display: flex; flex-direction: column; gap: 8px; overflow: visible; }
.panel-header, .finding-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.panel-header > div, .finding-heading > div { min-width: 0; }
.panel-header h3, .finding-heading h3 { margin: 0; color: var(--aegis-text); font-size: 16px; }
.panel-header p, .finding-heading p, .finding-summary { margin: 5px 0 0; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
.finding-row { box-sizing: border-box; display: flex; width: 100%; max-width: 100%; min-width: 0; align-items: flex-start; justify-content: space-between; gap: 8px; padding: 11px; border: 1px solid var(--aegis-border); border-radius: 10px; background: var(--el-fill-color-blank); color: inherit; text-align: left; cursor: pointer; overflow: visible; }
.finding-row:hover, .finding-row.selected { border-color: var(--el-color-primary-light-5); background: var(--el-color-primary-light-9); }
.finding-row-main { display: flex; flex: 1 1 auto; width: auto; min-width: 0; flex-direction: column; gap: 4px; }
.finding-row strong, .finding-row small { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.finding-row strong { font-size: 13px; line-height: 1.35; overflow-wrap: anywhere; }
.finding-row small { color: var(--el-text-color-secondary); font-size: 11px; white-space: nowrap; }
.finding-row .finding-row-title { display: -webkit-box; white-space: normal; overflow-wrap: anywhere; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-height: 1.35; }
.finding-risk { display: flex; flex: 0 0 auto; min-width: 66px; flex-direction: column; align-items: flex-end; gap: 3px; white-space: nowrap; }
.finding-risk small { max-width: 100%; color: var(--el-text-color-secondary); font-size: 11px; overflow-wrap: anywhere; text-align: right; }
.finding-pagination { margin-top: 4px; }
.finding-detail { display: grid; max-height: calc(100vh - 230px); gap: 14px; overflow: auto; }
.finding-summary { margin: -4px 0 0; font-size: 13px; }
.detail-section { display: grid; gap: 12px; padding: 16px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-fill-color-blank); }
.permission-section { border-color: var(--el-color-primary-light-7); background: var(--el-color-primary-light-9); }
.permission-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 18px; }
.detail-section-header { display: flex; min-width: 0; align-items: flex-start; justify-content: space-between; gap: 12px; padding-bottom: 8px; border-bottom: 1px solid var(--el-border-color-lighter); }
.detail-section-header > div { min-width: 0; }
.detail-section-header h4 { margin: 0; color: var(--aegis-text); font-size: 16px; }
.detail-section-header p { margin: 4px 0 0; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
.event-id-list { display: flex; flex-wrap: wrap; gap: 8px; }
.event-id-list code, .field code, .evidence-row code { display: block; min-width: 0; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; white-space: pre-wrap; font-family: var(--el-font-family); font-size: 12px; }
.event-id-list code { padding: 5px 8px; border-radius: 6px; background: var(--el-fill-color-light); }
.tool-call-list, .process-list { display: grid; gap: 10px; }
.tool-call-card, .process-card { padding: 12px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-fill-color-lighter); }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 18px; }
.field { min-width: 0; display: grid; gap: 3px; }
.field-wide { grid-column: 1 / -1; }
.field span, .evidence-row span, .decision-row span { color: var(--el-text-color-secondary); font-size: 12px; }
.field strong, .decision-row strong { min-width: 0; overflow-wrap: anywhere; word-break: break-word; font-size: 13px; font-weight: 500; }
.detail-empty { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; }
.evidence-list, .decision-list { display: grid; gap: 0; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; overflow: hidden; }
.evidence-row, .decision-row { display: grid; min-width: 0; grid-template-columns: minmax(130px, 0.35fr) minmax(0, 1fr); gap: 16px; padding: 10px 12px; border-bottom: 1px solid var(--el-border-color-lighter); }
.evidence-row > *, .decision-row > * { min-width: 0; }
.evidence-row:last-child, .decision-row:last-child { border-bottom: 0; }
.reason-box { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; color: var(--el-text-color-secondary); font-size: 12px; }
.reason-box span { padding: 4px 8px; border-radius: 12px; background: var(--el-fill-color-light); }
.compliance-summary { display: grid; gap: 3px; padding: 10px 12px; border-left: 3px solid var(--el-color-info); background: var(--el-fill-color-lighter); font-size: 12px; }
.compliance-summary span, .reason-box span { min-width: 0; overflow-wrap: anywhere; word-break: break-word; color: var(--el-text-color-secondary); }
.compliance-compliant { border-left-color: var(--el-color-success); }
.compliance-not-compliant { border-left-color: var(--el-color-danger); }
.compliance-unknown { border-left-color: var(--el-color-warning); }
:deep(.el-alert) { box-sizing: border-box; min-width: 0; max-width: 100%; overflow: hidden; }
:deep(.el-alert__content), :deep(.el-alert__title), :deep(.el-alert__description) { min-width: 0; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; white-space: normal; }
:deep(.el-alert__icon) { flex: 0 0 auto; }
:deep(.el-tag) { max-width: 100%; }
@media (max-width: 1100px) { .analysis-workspace { grid-template-columns: 1fr; } .finding-detail { max-height: none; } }
@media (max-width: 640px) { .field-grid, .permission-grid, .evidence-row, .decision-row { grid-template-columns: 1fr; gap: 4px; } }
</style>

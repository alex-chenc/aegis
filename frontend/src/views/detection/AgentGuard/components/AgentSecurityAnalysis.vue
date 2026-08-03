<template>
  <div class="security-analysis">
    <section class="builtin-rules">
      <header>
        <h3>{{ t('agentGuard.analysis.builtinRules') }}</h3>
        <span>{{ t('agentGuard.analysis.ruleCount', { count: rules.length }) }}</span>
      </header>
      <div class="rule-grid">
        <article v-for="rule in rules" :key="`${rule.rule_key}:${rule.rule_version}`">
          <div>
            <strong>{{ rule.rule_key }}@{{ rule.rule_version }}</strong>
            <el-tag size="small" effect="plain">
              {{ rule.enabled ?? rule.default_enabled ? t('agentGuard.analysis.enabled') : t('agentGuard.analysis.disabled') }}
            </el-tag>
          </div>
          <span>{{ rule.name }}</span>
          <small>
            {{ rule.severity || rule.default_severity || 'info' }}
            · {{ rule.action || rule.default_action || 'audit' }}
          </small>
        </article>
      </div>
    </section>

    <div class="analysis-workspace">
      <section class="finding-list">
        <el-empty v-if="findings.length === 0" :description="t('agentGuard.states.analysisNotAvailable')" />
        <button
          v-for="finding in findings"
          v-else
          :key="finding.id"
          type="button"
          :class="{ selected: activeFinding?.id === finding.id }"
          @click="selectFinding(finding)"
        >
          <span>
            <strong>{{ finding.title }}</strong>
            <small>{{ finding.id }}</small>
          </span>
          <span class="finding-risk">
            <el-tag size="small" :type="severityType(finding.severity)" effect="plain">
              {{ finding.severity }}
            </el-tag>
            <small>{{ finding.verdict || 'inconclusive' }}</small>
          </span>
        </button>
      </section>

      <section class="finding-detail">
        <el-empty v-if="!activeFinding" :description="t('agentGuard.analysis.selectFinding')" />
        <template v-else>
          <el-alert
            v-if="isAIOnly(activeFinding)"
            class="ai-only-notice"
            type="info"
            :closable="false"
            :title="t('agentGuard.findings.aiOnlyNotice')"
            show-icon
          />
          <header>
            <div>
              <h3>{{ activeFinding.title }}</h3>
              <p>
                {{ activeFinding.verdict || 'inconclusive' }}
                · {{ t('agentGuard.findings.confidence') }}
                {{ percent(activeFinding.confidence) }}
              </p>
            </div>
            <el-button
              v-if="!isAIOnly(activeFinding)"
              size="small"
              :loading="analysisPending"
              @click="emit('analyze', activeFinding.id)"
            >
              {{ t('agentGuard.analysis.reanalyze') }}
            </el-button>
          </header>

          <div class="finding-section">
            <h4>{{ t('agentGuard.analysis.attackChain') }}</h4>
            <ol v-if="activeFinding.attack_stages?.length">
              <li v-for="(stage, index) in activeFinding.attack_stages" :key="index">
                {{ formatValue(stage) }}
              </li>
            </ol>
            <p v-else>{{ t('agentGuard.analysis.noAttackChain') }}</p>
          </div>

          <div class="finding-section">
            <h4>{{ t('agentGuard.analysis.evidence') }}</h4>
            <div class="event-ids">
              <button
                v-for="eventId in evidenceIDs(activeFinding)"
                :key="eventId"
                type="button"
                @click="emit('open-evidence', eventId)"
              >
                {{ eventId }}
              </button>
              <span v-if="evidenceIDs(activeFinding).length === 0">-</span>
            </div>
          </div>

          <div class="finding-section completeness">
            <h4>{{ t('agentGuard.analysis.completeness') }}</h4>
            <strong>{{ activeFinding.evidence_completeness?.visibility || 'unknown' }}</strong>
            <span>{{ activeFinding.evidence_completeness?.reasons?.join(' · ') || '-' }}</span>
          </div>

          <div v-if="counterEvidence(activeFinding).length" class="finding-section">
            <h4>{{ t('agentGuard.analysis.counterEvidence') }}</h4>
            <ul>
              <li v-for="item in counterEvidence(activeFinding)" :key="item">{{ item }}</li>
            </ul>
          </div>

          <div v-if="uncertainties(activeFinding).length" class="finding-section">
            <h4>{{ t('agentGuard.analysis.uncertainties') }}</h4>
            <ul>
              <li v-for="item in uncertainties(activeFinding)" :key="item">{{ item }}</li>
            </ul>
          </div>

          <div v-if="latestAnalysis" class="finding-section">
            <h4>{{ t('agentGuard.analysis.history') }}</h4>
            <p>
              {{ latestAnalysis.status }} · {{ latestAnalysis.provider || '-' }}
              / {{ latestAnalysis.model || '-' }} · {{ latestAnalysis.prompt_version }}
            </p>
            <small>{{ latestAnalysis.input_digest }}</small>
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
  AgentSecurityAnalysisRun,
  AgentSecurityFindingSummary,
  BuiltinAgentBehaviorRuleSummary,
} from '@/types/agentGuard'

const props = withDefaults(defineProps<{
  rules: BuiltinAgentBehaviorRuleSummary[]
  findings: AgentSecurityFindingSummary[]
  analyses?: AgentSecurityAnalysisRun[]
  selectedFindingId: string
  analysisPending?: boolean
}>(), {
  analyses: () => [],
  analysisPending: false,
})

const emit = defineEmits<{
  (event: 'select-finding', id: string): void
  (event: 'open-evidence', id: string): void
  (event: 'analyze', id: string): void
}>()

const { t } = useI18n()
const localFindingId = ref(props.selectedFindingId)

watch(() => props.selectedFindingId, value => {
  localFindingId.value = value
})

watch(() => props.findings, findings => {
  if (!localFindingId.value && findings.length) localFindingId.value = findings[0].id
}, { immediate: true })

const activeFinding = computed(() =>
  props.findings.find(item => item.id === localFindingId.value) || props.findings[0] || null,
)

const latestAnalysis = computed(() => {
  if (!activeFinding.value) return null
  return props.analyses
    .filter(item => item.finding_id === activeFinding.value?.id)
    .sort((left, right) => right.attempt - left.attempt)[0] || null
})

function selectFinding(finding: AgentSecurityFindingSummary) {
  localFindingId.value = finding.id
  emit('select-finding', finding.id)
}

function isAIOnly(finding: AgentSecurityFindingSummary) {
  return finding.decision_sources?.length === 1 && finding.decision_sources[0] === 'ai'
}

function evidenceIDs(finding: AgentSecurityFindingSummary) {
  const ids = new Set(finding.evidence_event_ids || [])
  for (const hit of finding.rule_hits || []) {
    for (const id of hit.evidence_event_ids || hit.event_ids || []) ids.add(id)
  }
  return [...ids]
}

function counterEvidence(finding: AgentSecurityFindingSummary) {
  return finding.counter_evidence || latestAnalysis.value?.output?.counter_evidence || []
}

function uncertainties(finding: AgentSecurityFindingSummary) {
  return finding.uncertainties || latestAnalysis.value?.output?.uncertainties || []
}

function percent(value?: number) {
  if (value == null) return '-'
  return `${Math.round(value * 100)}%`
}

function formatValue(value: unknown) {
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

function severityType(severity: string) {
  if (severity === 'critical' || severity === 'high') return 'danger'
  if (severity === 'medium') return 'warning'
  if (severity === 'low') return 'success'
  return 'info'
}
</script>

<style scoped>
.security-analysis {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.builtin-rules,
.finding-list,
.finding-detail {
  padding: 12px;
  border: 1px solid var(--aegis-border);
  border-radius: 14px;
  background: #fff;
}

.builtin-rules > header,
.finding-detail > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.builtin-rules h3,
.finding-detail h3 {
  margin: 0;
}

.rule-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  margin-top: 10px;
}

.rule-grid article {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
  padding: 10px;
  border: 1px solid var(--aegis-border);
  border-radius: 10px;
}

.rule-grid article > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.rule-grid strong,
.rule-grid span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rule-grid small,
.finding-detail p,
.finding-detail small {
  color: var(--aegis-text-muted);
}

.analysis-workspace {
  display: grid;
  grid-template-columns: minmax(280px, 2fr) minmax(420px, 3fr);
  gap: 14px;
}

.finding-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
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

.finding-list small {
  color: var(--aegis-text-muted);
}

.finding-risk {
  align-items: flex-end;
}

.finding-section {
  padding: 12px 0;
  border-top: 1px solid var(--aegis-border);
}

.finding-section h4 {
  margin: 0 0 7px;
}

.finding-section ul,
.finding-section ol {
  margin: 0;
  padding-left: 20px;
}

.event-ids {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.event-ids button {
  padding: 3px 7px;
  border: 1px solid var(--aegis-border);
  border-radius: 7px;
  background: #f8fafc;
  color: var(--el-color-primary);
  cursor: pointer;
}

.completeness {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 5px 12px;
}

.completeness h4 {
  grid-column: 1 / -1;
}

@media (max-width: 1200px) {
  .rule-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .analysis-workspace {
    grid-template-columns: 1fr;
  }
}
</style>

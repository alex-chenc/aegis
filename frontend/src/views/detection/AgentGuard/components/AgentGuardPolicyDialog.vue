<template>
  <el-dialog
    :model-value="visible"
    :title="t('agentGuard.policy.title')"
    width="1080px"
    destroy-on-close
    @close="emit('close')"
  >
    <el-alert
      type="info"
      :title="t('agentGuard.policy.builtinNotice')"
      :closable="false"
      show-icon
    />

    <section v-if="mode === 'escape'" class="catalog-section">
      <div class="section-heading"><div><h3>{{ t('agentGuard.policy.escapePolicies') }}</h3><p>{{ t('agentGuard.policy.escapePoliciesHint') }}</p></div><el-tag type="danger">权限 → Hook → PID → eBPF</el-tag></div>
      <div class="builtin-rule-list">
        <article v-for="rule in sortedEscapeRules" :key="`${rule.rule_key}:${rule.rule_version}`" class="builtin-rule-card">
          <div class="rule-card-heading"><div><h4>{{ rule.name }}</h4><span class="rule-key">{{ rule.rule_key }} · v{{ rule.rule_version }}</span></div><el-tag type="danger">{{ ruleAction(rule) }}</el-tag></div>
          <p class="rule-description">{{ rule.description }}</p>
          <div v-if="rule.agent_types?.length || rule.backends?.length" class="rule-scope-tags">
            <el-tag v-for="agent in rule.agent_types || []" :key="`agent-${agent}`" size="small">{{ agentDisplayName(agent) }}</el-tag>
            <el-tag v-for="backend in rule.backends || []" :key="`backend-${backend}`" size="small" type="info">{{ backend }}</el-tag>
            <el-tag v-for="semantic in rule.boundary_semantics || []" :key="`semantic-${semantic}`" size="small" type="warning">{{ semantic }}</el-tag>
          </div>
          <div class="rule-detail-grid"><div><span class="meta-label">{{ t('agentGuard.policy.hookPoints') }}</span><div class="detail-list"><code v-for="item in rule.hook_points || []" :key="item">{{ item }}</code></div></div><div><span class="meta-label">{{ t('agentGuard.policy.requiredEvidence') }}</span><div class="detail-list"><code v-for="item in rule.required_evidence || []" :key="item">{{ item }}</code></div></div><div><span class="meta-label">{{ t('agentGuard.policy.defaultState') }}</span><span>{{ rule.default_enabled === false ? t('agentGuard.policy.disabled') : t('agentGuard.policy.enabled') }}</span></div><div><span class="meta-label">{{ t('agentGuard.policy.digest') }}</span><code class="digest">{{ rule.digest || '-' }}</code></div></div>
        </article>
      </div>
    </section>

    <section v-if="mode === 'behavior'" class="catalog-section">
      <div class="section-heading">
        <div>
          <h3>{{ t('agentGuard.policy.builtinPolicies') }}</h3>
          <p>{{ t('agentGuard.policy.builtinPoliciesHint') }}</p>
        </div>
        <el-tag type="success">{{ t('agentGuard.policy.builtinReadonly') }}</el-tag>
      </div>

      <div class="builtin-policy-grid">
        <article v-for="policy in builtinPolicies" :key="policy.policyKey" class="builtin-policy-card">
          <div class="policy-card-heading">
            <div>
              <h4>{{ t(policy.nameKey) }}</h4>
              <span class="policy-key">{{ policy.policyKey }}</span>
            </div>
            <el-tag type="warning">{{ t(policy.modeKey) }}</el-tag>
          </div>
          <p class="policy-description">{{ t(policy.descriptionKey) }}</p>
          <dl class="policy-facts">
            <div>
              <dt>{{ t('agentGuard.policy.scope') }}</dt>
              <dd>{{ t('agentGuard.policy.allAgents') }}</dd>
            </div>
            <div>
              <dt>{{ t('agentGuard.policy.categories') }}</dt>
              <dd>
                <el-tag v-for="category in policy.categoryKeys" :key="category" size="small">
                  {{ categoryLabel(category) }}
                </el-tag>
              </dd>
            </div>
            <div>
              <dt>{{ t('agentGuard.policy.ruleCount') }}</dt>
              <dd>{{ rulesForPolicy(policy).length }}</dd>
            </div>
          </dl>
          <div class="policy-rule-keys">
            <span v-for="rule in rulesForPolicy(policy)" :key="rule.rule_key" class="rule-key-chip">
              {{ rule.rule_key }} · {{ rule.name }}
            </span>
          </div>
        </article>
      </div>
    </section>

    <section v-if="mode === 'behavior'" class="catalog-section">
      <div class="section-heading">
        <div>
          <h3>{{ t('agentGuard.policy.builtinRules') }}</h3>
          <p>{{ t('agentGuard.policy.builtinRulesHint') }}</p>
        </div>
        <el-tag>{{ sortedRules.length }} {{ t('agentGuard.policy.ruleUnit') }}</el-tag>
      </div>

      <el-alert
        v-if="store.errors.analysis && sortedRules.length === 0"
        type="error"
        :title="store.errors.analysis"
        :closable="false"
        show-icon
      />
      <el-empty
        v-else-if="sortedRules.length === 0"
        :description="t('agentGuard.policy.noBuiltinRules')"
      />
      <div v-else class="builtin-rule-list">
        <article v-for="rule in sortedRules" :key="`${rule.rule_key}:${rule.rule_version}`" class="builtin-rule-card">
          <div class="rule-card-heading">
            <div>
              <h4>{{ rule.name }}</h4>
              <span class="rule-key">{{ rule.rule_key }} · v{{ rule.rule_version }}</span>
            </div>
            <div class="rule-tags">
              <el-tag :type="severityTagType(ruleSeverity(rule))">
                {{ severityLabel(ruleSeverity(rule)) }}
              </el-tag>
              <el-tag>{{ actionLabel(ruleAction(rule)) }}</el-tag>
            </div>
          </div>

          <p class="rule-description">{{ rule.description || t('agentGuard.policy.noDescription') }}</p>

          <div class="rule-meta-grid">
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.categories') }}</span>
              <div class="meta-values">
                <el-tag v-for="category in rule.categories || []" :key="category" size="small">
                  {{ categoryLabel(category) }}
                </el-tag>
                <span v-if="!rule.categories?.length" class="muted">-</span>
              </div>
            </div>
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.execution') }}</span>
              <span>{{ executionOwnerLabel(rule) }}</span>
            </div>
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.defaultState') }}</span>
              <span>{{ rule.default_enabled === false ? t('agentGuard.policy.disabled') : t('agentGuard.policy.enabled') }}</span>
            </div>
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.recommendedAction') }}</span>
              <span>{{ actionLabel(rule.recommended_action || ruleAction(rule)) }}</span>
            </div>
          </div>

          <div class="rule-detail-grid">
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.requiredEvidence') }}</span>
              <div class="detail-list">
                <code v-for="item in rule.required_evidence || []" :key="item">{{ item }}</code>
                <span v-if="!rule.required_evidence?.length" class="muted">-</span>
              </div>
            </div>
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.allowConditions') }}</span>
              <div class="detail-list">
                <code v-for="item in rule.allow_conditions || []" :key="item">{{ item }}</code>
                <span v-if="!rule.allow_conditions?.length" class="muted">-</span>
              </div>
            </div>
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.mitre') }}</span>
              <div class="detail-list">
                <el-tag v-for="item in rule.mitre || []" :key="item" size="small">{{ item }}</el-tag>
                <span v-if="!rule.mitre?.length" class="muted">-</span>
              </div>
            </div>
            <div>
              <span class="meta-label">{{ t('agentGuard.policy.digest') }}</span>
              <code class="digest">{{ rule.digest || '-' }}</code>
            </div>
          </div>

          <details class="rule-parameters">
            <summary>{{ t('agentGuard.policy.parameters') }}</summary>
            <div class="parameter-columns">
              <div>
                <span class="meta-label">{{ t('agentGuard.policy.defaultParameters') }}</span>
                <pre>{{ prettyJson(rule.default_parameters) }}</pre>
              </div>
              <div>
                <span class="meta-label">{{ t('agentGuard.policy.parameterSchema') }}</span>
                <pre>{{ prettyJson(rule.parameters_schema) }}</pre>
              </div>
            </div>
          </details>
        </article>
      </div>
    </section>

    <template #footer>
      <el-button @click="emit('close')">{{ t('common.actions.close') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAgentGuardStore } from '@/store/agentGuard'
import type { BuiltinAgentBehaviorRuleSummary, BuiltinAgentEscapeRuleSummary } from '@/types/agentGuard'
import {
  BUILTIN_AGENT_GUARD_POLICIES,
  ruleExecutionOwner,
  rulesForBuiltinPolicy,
  type BuiltinAgentGuardPolicyView,
} from '../agentGuardBuiltinPolicies'

const props = withDefaults(defineProps<{ visible: boolean; mode?: 'behavior' | 'escape' }>(), { mode: 'behavior' })
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const store = useAgentGuardStore()

const builtinPolicies = BUILTIN_AGENT_GUARD_POLICIES
const sortedRules = computed(() => [...store.builtinRules].sort((left, right) => left.rule_key.localeCompare(right.rule_key)))
const sortedEscapeRules = computed<BuiltinAgentEscapeRuleSummary[]>(() => [...store.escapeRules].sort((left, right) => left.rule_key.localeCompare(right.rule_key)))

function rulesForPolicy(policy: BuiltinAgentGuardPolicyView): BuiltinAgentBehaviorRuleSummary[] {
  return rulesForBuiltinPolicy(policy, store.builtinRules)
}

function categoryLabel(category: string): string {
  const key = `agentGuard.policy.categoryNames.${category}`
  const translated = t(key)
  return translated === key ? category : translated
}

function agentDisplayName(agent: string): string {
  const names: Record<string, string> = { codex: 'Codex', 'claude-code': 'Claude Code', openclaw: 'OpenClaw', hermes: 'Hermes', zcode: 'Zcode' }
  return names[agent] || agent
}

function severityLabel(severity: string): string {
  const key = `agentGuard.policy.severityNames.${severity}`
  const translated = t(key)
  return translated === key ? severity : translated
}

function actionLabel(action: string): string {
  const key = `agentGuard.policy.actionNames.${action}`
  const translated = t(key)
  return translated === key ? action : translated
}

function executionOwnerLabel(rule: BuiltinAgentBehaviorRuleSummary): string {
  const owner = ruleExecutionOwner(rule)
  const key = `agentGuard.policy.executionNames.${owner}`
  const translated = t(key)
  return translated === key ? owner : translated
}

function ruleSeverity(rule: BuiltinAgentBehaviorRuleSummary): string {
  return rule.severity || rule.default_severity || 'unknown'
}

function ruleAction(rule: BuiltinAgentBehaviorRuleSummary | BuiltinAgentEscapeRuleSummary): string {
  return rule.action || rule.default_action || 'unknown'
}

function severityTagType(severity: string): 'danger' | 'warning' | 'info' | 'success' {
  if (severity === 'critical' || severity === 'high') return 'danger'
  if (severity === 'medium') return 'warning'
  if (severity === 'low') return 'info'
  return 'success'
}

function prettyJson(value?: Record<string, unknown>): string {
  if (!value || Object.keys(value).length === 0) return '{}'
  return JSON.stringify(value, null, 2)
}

watch(() => props.visible, visible => {
  if (visible) void (props.mode === 'escape' ? store.fetchEscapeRules() : store.fetchBuiltinRules())
})
</script>

<style scoped>
.catalog-section {
  margin-top: 22px;
}

.section-heading,
.policy-card-heading,
.rule-card-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.section-heading h3,
.policy-card-heading h4,
.rule-card-heading h4 {
  margin: 0;
  color: #172b4d;
}

.section-heading p,
.policy-description,
.rule-description {
  margin: 6px 0 0;
  color: #6b7a90;
  line-height: 1.6;
}

.builtin-policy-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  margin-top: 14px;
}

.builtin-policy-card,
.builtin-rule-card {
  border: 1px solid #dfe7f3;
  border-radius: 10px;
  background: #fbfdff;
  padding: 18px;
}

.policy-key,
.rule-key,
.muted {
  color: #8592a6;
  font-size: 12px;
}

.policy-facts,
.rule-meta-grid,
.rule-detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 18px;
  margin: 16px 0 0;
}

.policy-facts dt,
.policy-facts dd {
  display: inline;
  margin: 0;
}

.policy-facts dt,
.meta-label {
  display: block;
  margin-bottom: 5px;
  color: #8592a6;
  font-size: 12px;
}

.policy-facts dd,
.rule-meta-grid > div > span:last-child {
  color: #344563;
}

.policy-facts dd :deep(.el-tag) {
  margin: 0 5px 5px 0;
}

.policy-rule-keys,
.rule-tags,
.meta-values,
.detail-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.policy-rule-keys {
  margin-top: 16px;
}

.rule-key-chip {
  border-radius: 5px;
  background: #eef5ff;
  color: #3c6fb6;
  padding: 5px 8px;
  font-size: 12px;
}

.builtin-rule-list {
  display: grid;
  gap: 14px;
  margin-top: 14px;
}

.rule-card-heading {
  align-items: center;
}

.rule-tags :deep(.el-tag) {
  margin-left: 6px;
}

.rule-meta-grid,
.rule-detail-grid {
  border-top: 1px solid #edf1f7;
  padding-top: 14px;
}

.detail-list code,
.digest {
  border-radius: 4px;
  background: #f1f4f8;
  color: #52627a;
  padding: 3px 5px;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.rule-parameters {
  margin-top: 16px;
  border-top: 1px solid #edf1f7;
  padding-top: 12px;
}

.rule-parameters summary {
  cursor: pointer;
  color: #3c6fb6;
}

.parameter-columns {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  margin-top: 12px;
}

pre {
  max-height: 220px;
  margin: 0;
  overflow: auto;
  border-radius: 6px;
  background: #f5f7fa;
  padding: 10px;
  color: #4b5d78;
  font-size: 12px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

@media (max-width: 800px) {
  .builtin-policy-grid,
  .policy-facts,
  .rule-meta-grid,
  .rule-detail-grid,
  .parameter-columns {
    grid-template-columns: 1fr;
  }
}
</style>

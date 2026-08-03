<template>
  <section class="isolation-baseline">
    <el-alert
      v-if="unit.coverage_level === 'no_isolation'"
      type="warning"
      :closable="false"
      :title="t('agentGuard.baseline.noIsolation')"
      show-icon
    />
    <el-alert
      v-else-if="unit.coverage_level === 'remote_unobservable'"
      type="info"
      :closable="false"
      :title="t('agentGuard.baseline.remoteUnobservable')"
      show-icon
    />

    <header>
      <div>
        <strong>{{ unit.unit_type }}</strong>
        <span>{{ unit.status }} · {{ unit.coverage_level }}</span>
      </div>
      <el-tag effect="plain">{{ unit.coverage_level }}</el-tag>
    </header>

    <div class="baseline-grid">
      <article
        v-for="group in groups"
        :key="group.key"
        class="baseline-card"
        :class="`state-${group.status}`"
      >
        <div>
          <strong>{{ group.label }}</strong>
          <code>{{ group.status }}</code>
        </div>
        <dl>
          <div>
            <dt>{{ t('agentGuard.baseline.expected') }}</dt>
            <dd>{{ formatEvidence(group.expected) }}</dd>
          </div>
          <div>
            <dt>{{ t('agentGuard.baseline.observed') }}</dt>
            <dd>{{ formatEvidence(group.observed) }}</dd>
          </div>
          <div v-if="group.diff">
            <dt>{{ t('agentGuard.baseline.diff') }}</dt>
            <dd>{{ formatEvidence(group.diff) }}</dd>
          </div>
        </dl>
      </article>
    </div>

    <p v-if="unit.coverage_reasons?.length" class="coverage-reasons">
      {{ t('agentGuard.baseline.reasons') }}：{{ unit.coverage_reasons.join(' · ') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentExecutionUnit } from '@/types/agentGuard'

const props = defineProps<{ unit: AgentExecutionUnit }>()
const { t } = useI18n()

const groupKeys = ['namespace', 'cgroup', 'mount', 'capability', 'seccomp'] as const

const groups = computed(() => groupKeys.map(key => {
  const expected = evidenceGroup(props.unit.isolation_baseline, key)
  const observed = evidenceGroup(props.unit.isolation_actual, key)
  const diff = evidenceGroup(props.unit.isolation_diff, key)
  return {
    key,
    label: t(`agentGuard.baseline.groups.${key}`),
    expected,
    observed,
    diff,
    status: evidenceStatus(expected, observed, diff),
  }
}))

function evidenceGroup(value: Record<string, unknown> | undefined, key: string): unknown {
  if (!value) return undefined
  return value[key] ?? value[`${key}s`]
}

function evidenceStatus(expected: unknown, observed: unknown, diff: unknown) {
  const explicit = statusValue(diff) || statusValue(observed) || statusValue(expected)
  if (explicit) return explicit
  if (hasEvidence(diff)) return 'drift'
  if (props.unit.coverage_level === 'no_isolation') return 'not_applicable'
  if (props.unit.coverage_level === 'remote_unobservable' ||
    props.unit.coverage_level === 'degraded') return 'unobservable'
  if (!hasEvidence(expected) || !hasEvidence(observed)) return 'unobservable'
  return JSON.stringify(expected) === JSON.stringify(observed) ? 'normal' : 'drift'
}

function statusValue(value: unknown) {
  if (!value || typeof value !== 'object') return ''
  const status = String((value as Record<string, unknown>).status || '')
  if (['normal', 'drift', 'unobservable', 'not_applicable'].includes(status)) return status
  return ''
}

function hasEvidence(value: unknown) {
  if (value == null) return false
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'object') return Object.keys(value as object).length > 0
  return String(value).length > 0
}

function formatEvidence(value: unknown) {
  if (!hasEvidence(value)) return t('agentGuard.baseline.unavailable')
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  const encoded = JSON.stringify(value)
  return encoded.length > 240 ? `${encoded.slice(0, 240)}…` : encoded
}
</script>

<style scoped>
.isolation-baseline {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.isolation-baseline > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.isolation-baseline > header div {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.isolation-baseline > header span,
.coverage-reasons {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.baseline-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.baseline-card {
  padding: 12px;
  border: 1px solid var(--aegis-border);
  border-left: 4px solid var(--el-color-info);
  border-radius: 12px;
}

.baseline-card.state-normal {
  border-left-color: var(--el-color-success);
}

.baseline-card.state-drift {
  border-left-color: var(--el-color-danger);
}

.baseline-card > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.baseline-card code {
  color: var(--aegis-text-muted);
  font-size: 11px;
}

.baseline-card dl > div {
  display: grid;
  grid-template-columns: 76px minmax(0, 1fr);
  gap: 8px;
  margin-top: 8px;
}

.baseline-card dt {
  color: var(--aegis-text-muted);
}

.baseline-card dd {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
}

@media (max-width: 1100px) {
  .baseline-grid {
    grid-template-columns: 1fr;
  }
}
</style>

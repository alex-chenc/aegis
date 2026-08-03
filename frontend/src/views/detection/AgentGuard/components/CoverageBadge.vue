<template>
  <el-tooltip :content="helpText" placement="top" :disabled="!helpText">
    <el-tag
      class="coverage-badge"
      :class="`coverage-${normalizedCoverage}`"
      :type="tagType"
      effect="plain"
      role="status"
      :aria-label="label"
    >
      <span class="coverage-marker" aria-hidden="true" />
      {{ label }}
    </el-tag>
  </el-tooltip>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentGuardCoverage } from '@/types/agentGuard'

const props = withDefaults(defineProps<{
  coverage?: AgentGuardCoverage | string
  reasons?: string[]
}>(), {
  coverage: 'unknown',
  reasons: () => [],
})

const { t, te } = useI18n()

const supported = new Set<AgentGuardCoverage>([
  'full_enforcement',
  'behavior_monitor_escape_enforce',
  'monitor_only',
  'no_isolation',
  'remote_unobservable',
  'unsupported',
  'unsupported_profile',
  'degraded',
  'unknown',
])

const normalizedCoverage = computed<AgentGuardCoverage>(() =>
  supported.has(props.coverage as AgentGuardCoverage)
    ? props.coverage as AgentGuardCoverage
    : 'unknown',
)

const label = computed(() => t(`agentGuard.coverage.${normalizedCoverage.value}`))

const HELP_KEYS: Partial<Record<AgentGuardCoverage, string>> = {
  monitor_only: 'agentGuard.coverage.monitorOnlyHelp',
  no_isolation: 'agentGuard.coverage.noIsolationHelp',
  remote_unobservable: 'agentGuard.coverage.remoteUnobservableHelp',
  unsupported: 'agentGuard.coverage.unsupportedHelp',
  unsupported_profile: 'agentGuard.coverage.unsupportedHelp',
  degraded: 'agentGuard.coverage.degradedHelp',
}

const helpKey = computed(() => HELP_KEYS[normalizedCoverage.value] || '')

const helpText = computed(() => {
  const parts = []
  if (helpKey.value && te(helpKey.value)) parts.push(t(helpKey.value))
  if (props.reasons.length) parts.push(props.reasons.join('；'))
  return parts.join(' ')
})

const tagType = computed(() => {
  switch (normalizedCoverage.value) {
    case 'full_enforcement':
    case 'behavior_monitor_escape_enforce':
      return 'success'
    case 'monitor_only':
    case 'no_isolation':
      return 'warning'
    case 'degraded':
      return 'danger'
    default:
      return 'info'
  }
})
</script>

<style scoped>
.coverage-badge {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  gap: 6px;
}

.coverage-marker {
  width: 7px;
  height: 7px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: currentColor;
}
</style>

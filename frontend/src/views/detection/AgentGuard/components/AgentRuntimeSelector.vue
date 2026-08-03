<template>
  <div class="runtime-selector">
    <span class="runtime-selector-label">{{ t('agentGuard.drawer.instanceSelector') }}</span>
    <div class="runtime-options" role="group" :aria-label="t('agentGuard.drawer.instanceSelector')">
      <button
        type="button"
        class="runtime-option"
        :class="{ active: !selectedInstanceId }"
        @click="emit('select', '')"
      >
        {{ t('agentGuard.runtime.allInstances', { count: total }) }}
      </button>
      <el-tooltip
        v-for="instance in instances"
        :key="instance.id"
        :content="`PID ${instance.controller_pid} · start_ticks ${instance.controller_start_ticks}`"
        placement="bottom"
      >
        <button
          type="button"
          class="runtime-option"
          :class="{ active: selectedInstanceId === instance.id }"
          @click="emit('select', instance.id)"
        >
          PID {{ instance.controller_pid }}
        </button>
      </el-tooltip>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AgentRuntimeInstance } from '@/types/agentGuard'

defineProps<{
  instances: AgentRuntimeInstance[]
  total: number
  selectedInstanceId: string
}>()

const emit = defineEmits<{
  (event: 'select', instanceId: string): void
}>()

const { t } = useI18n()
</script>

<style scoped>
.runtime-selector {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--aegis-border);
  border-radius: 14px;
  background: rgba(248, 250, 252, 0.9);
}

.runtime-selector-label {
  flex: 0 0 auto;
  padding-top: 7px;
  color: var(--aegis-text-muted);
  font-size: 13px;
  font-weight: 700;
}

.runtime-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.runtime-option {
  min-height: 32px;
  padding: 5px 12px;
  border: 1px solid rgba(37, 99, 235, 0.2);
  border-radius: 999px;
  color: #334155;
  background: #fff;
  cursor: pointer;
}

.runtime-option.active {
  border-color: var(--aegis-action-blue);
  color: #fff;
  background: var(--aegis-action-blue);
}

.runtime-option:focus-visible {
  outline: 3px solid rgba(37, 99, 235, 0.24);
  outline-offset: 2px;
}
</style>

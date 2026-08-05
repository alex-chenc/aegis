<template>
  <div class="runtime-selector">
    <div class="runtime-selector-heading">
      <span class="runtime-selector-label">{{ t('agentGuard.drawer.sessionSelector') }}</span>
      <el-button
        size="small"
        type="danger"
        plain
        v-if="canDeleteSessions"
        :disabled="selectedSessionIds.length === 0"
        @click="emit('delete', [...selectedSessionIds])"
      >
        {{ t('agentGuard.drawer.deleteSessions', { count: selectedSessionIds.length }) }}
      </el-button>
    </div>
    <div class="runtime-options" role="group" :aria-label="t('agentGuard.drawer.sessionSelector')">
      <div
        v-for="session in visibleSessions"
        :key="session.id"
        class="runtime-option-wrap"
      >
        <input
          type="checkbox"
          :checked="selectedSessionIds.includes(session.id)"
          :aria-label="t('agentGuard.drawer.selectSession', { id: session.external_session_id || session.id })"
          @change="toggleSession(session.id)"
        >
        <button
          type="button"
          class="runtime-option"
          :class="{ active: selectedSessionId === session.id }"
          @click="emit('select', session.id)"
        >
          {{ session.external_session_id || session.id }}
        </button>
      </div>
    </div>
    <el-pagination
      v-if="total > pageSize"
      class="session-pagination"
      background
      layout="prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="emit('page-change', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentBehaviorSession } from '@/types/agentGuard'

const props = withDefaults(defineProps<{
  sessions: AgentBehaviorSession[]
  total: number
  page?: number
  pageSize?: number
  selectedSessionId: string
  selectedSessionIds?: string[]
  canDeleteSessions?: boolean
}>(), {
  page: 1,
  pageSize: 20,
  selectedSessionIds: () => [],
  canDeleteSessions: false,
})

const emit = defineEmits<{
  (event: 'select', sessionId: string): void
  (event: 'page-change', page: number): void
  (event: 'selection-change', sessionIds: string[]): void
  (event: 'delete', sessionIds: string[]): void
}>()

const { t } = useI18n()

const visibleSessions = computed(() => {
  return props.sessions
})

function toggleSession(sessionId: string) {
  const next = props.selectedSessionIds.includes(sessionId)
    ? props.selectedSessionIds.filter(id => id !== sessionId)
    : [...props.selectedSessionIds, sessionId]
  emit('selection-change', next)
}
</script>

<style scoped>
.runtime-selector {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: start;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--aegis-border);
  border-radius: 14px;
  background: rgba(248, 250, 252, 0.9);
}

.runtime-selector-heading {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.runtime-selector-label {
  flex: 0 0 auto;
  padding-top: 7px;
  color: var(--aegis-text-muted);
  font-size: 13px;
  font-weight: 700;
}

.runtime-options {
  flex: 1 1 auto;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.runtime-option-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
}

.runtime-option-wrap input {
  flex: 0 0 auto;
}

.session-pagination {
  flex: 0 0 auto;
  margin-left: auto;
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

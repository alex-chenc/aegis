<template>
  <div class="agent-summary-table">
    <el-table
      v-loading="loading"
      :data="agents"
      row-key="agent_scope_key"
      class="summary-table"
      @row-click="handleRowClick"
    >
      <el-table-column :label="t('agentGuard.table.agent')" min-width="180" fixed="left">
        <template #default="{ row }">
          <div class="agent-identity">
            <strong>{{ row.display_name || row.agent_type }}</strong>
            <span>{{ row.agent_type }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('agentGuard.table.host')" min-width="190">
        <template #default="{ row }">
          <div class="stacked-value">
            <strong>{{ row.host?.hostname || '-' }}</strong>
            <span>{{ row.host?.ip || '-' }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('agentGuard.table.instances')" width="128" align="center">
        <template #default="{ row }">
          <span v-if="row.running_instance_count > 0">
            {{ t('agentGuard.runtime.instances', { count: row.running_instance_count }) }}
          </span>
          <span v-else class="muted-text">{{ t('agentGuard.runtime.noInstance') }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('agentGuard.table.controllerPid')" min-width="150">
        <template #default="{ row }">
          <div v-if="row.controller_pids?.length" class="pid-list">
            <el-tag v-for="pid in row.controller_pids.slice(0, 2)" :key="pid" size="small" effect="plain">
              PID {{ pid }}
            </el-tag>
            <span v-if="row.controller_pids.length > 2" class="muted-text">
              {{ t('agentGuard.table.morePids', { count: row.controller_pids.length - 2 }) }}
            </span>
          </div>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('agentGuard.table.runtimeStatus')" width="120">
        <template #default="{ row }">
          <el-tag :type="runtimeTagType(row.runtime_status)" effect="plain">
            {{ runtimeLabel(row.runtime_status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('agentGuard.table.coverage')" min-width="170">
        <template #default="{ row }">
          <CoverageBadge :coverage="row.coverage_level" :reasons="row.coverage_reasons || []" />
        </template>
      </el-table-column>
      <el-table-column
        v-if="mode === 'escape'"
        :label="t('agentGuard.table.isolation')"
        min-width="170"
      >
        <template #default="{ row }">
          {{ isolationSummary(row.isolation_types) }}
        </template>
      </el-table-column>
      <el-table-column
        :label="mode === 'behavior' ? t('agentGuard.table.highRisk') : t('agentGuard.table.escapeFindings')"
        width="100"
        align="center"
      >
        <template #default="{ row }">
          <strong :class="{ 'risk-number': riskCount(row) > 0 }">{{ riskCount(row) }}</strong>
        </template>
      </el-table-column>
      <el-table-column
        v-if="mode === 'escape'"
        :label="t('agentGuard.table.actionStatus')"
        min-width="120"
      >
        <template #default="{ row }">{{ row.action_status || '-' }}</template>
      </el-table-column>
      <el-table-column :label="t('agentGuard.table.lastActivity')" min-width="170">
        <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('agentGuard.table.actions')" width="120" fixed="right" align="center">
        <template #default="{ row }">
          <el-button link type="primary" @click.stop="emit('open', row)">
            {{ t('common.actions.details') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="summary-pagination">
      <el-pagination
        :current-page="page"
        :page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @current-change="emit('page-change', $event)"
        @size-change="emit('size-change', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/i18n/formatters'
import type {
  AgentGuardAgentSummary,
  AgentGuardMode,
  AgentRuntimeStatus,
  ExecutionUnitType,
} from '@/types/agentGuard'
import CoverageBadge from './CoverageBadge.vue'

const props = defineProps<{
  agents: AgentGuardAgentSummary[]
  loading: boolean
  total: number
  page: number
  pageSize: number
  mode: AgentGuardMode
}>()

const emit = defineEmits<{
  (event: 'open', agent: AgentGuardAgentSummary): void
  (event: 'page-change', page: number): void
  (event: 'size-change', pageSize: number): void
}>()

const { t, te } = useI18n()

function handleRowClick(row: AgentGuardAgentSummary) {
  emit('open', row)
}

function runtimeLabel(status: AgentRuntimeStatus) {
  const key = `agentGuard.runtime.${status}`
  return te(key) ? t(key) : t('agentGuard.runtime.unknown')
}

function runtimeTagType(status: AgentRuntimeStatus) {
  if (status === 'running') return 'success'
  if (status === 'stale') return 'warning'
  if (status === 'stopped') return 'info'
  return 'info'
}

function isolationSummary(types: ExecutionUnitType[]) {
  if (!types?.length) return t('agentGuard.isolation.none')
  return types
    .map(type => {
      const key = `agentGuard.isolation.${type}`
      return te(key) ? t(key) : type
    })
    .join(' / ')
}

function riskCount(row: AgentGuardAgentSummary) {
  return props.mode === 'escape'
    ? row.escape_finding_count || 0
    : row.high_risk_finding_count || 0
}

function formatTime(value?: string) {
  return value ? formatDateTime(value) : '-'
}
</script>

<style scoped>
.summary-table :deep(.el-table__row) {
  cursor: pointer;
}

.agent-identity,
.stacked-value {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.agent-identity span,
.stacked-value span,
.muted-text {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.pid-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.risk-number {
  color: var(--aegis-risk-critical);
}

.summary-pagination {
  display: flex;
  justify-content: flex-end;
  padding-top: 18px;
}
</style>

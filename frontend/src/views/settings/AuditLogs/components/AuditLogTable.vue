<template>
  <el-card style="width: 100%">
    <template #header>
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px">
        <span>{{ $t('generated.common_audit_log_7666cd') }}</span>
        <div style="display: flex; gap: 8px; align-items: center">
          <el-button
            data-testid="batch-delete-btn"
            type="danger"
            size="small"
            :disabled="selectedRows.length === 0"
            @click="handleBatchDelete"
          >
            {{ $t('generated.common_delete_3755f5') }}{{ selectedRows.length > 0 ? ` (${selectedRows.length})` : '' }}
          </el-button>
          <el-select v-model="filters.result" :placeholder="$t('generated.common_result_0a2c91')" size="small" clearable style="width: 100px" @change="handleFilter">
            <el-option :label="$t('generated.common_pass_dcc423')" value="passed" />
            <el-option :label="$t('generated.common_fail_3e3c80')" value="failed" />
          </el-select>
          <el-select v-model="filters.script_type" :placeholder="$t('generated.common_script_type_f1574a')" size="small" clearable style="width: 120px" @change="handleFilter">
            <el-option :label="$t('generated.common_detection_b3ff0c')" value="check" />
            <el-option :label="$t('generated.common_repair_590253')" value="fix" />
            <el-option label="POC" value="poc_verify" />
            <el-option :label="$t('generated.settingsAuditLogsAuditLogTable_self_healing_1df800')" value="self_healing" />
          </el-select>
          <el-select v-model="filters.audit_source" :placeholder="$t('generated.common_audit_source_9af972')" size="small" clearable style="width: 120px" @change="handleFilter">
            <el-option :label="$t('generated.settingsAuditLogsAuditLogTable_generation_phase_aa85e2')" value="generation" />
            <el-option :label="$t('generated.settingsAuditLogsAuditLogTable_release_stage_a0a9a2')" value="dispatch" />
            <el-option :label="$t('generated.settingsAuditLogsAuditLogTable_agent_side_ae5643')" value="agent" />
          </el-select>
        </div>
      </div>
    </template>

    <el-table ref="tableRef" :data="logs" v-loading="loading" style="width: 100%" @selection-change="handleSelectionChange">
      <el-table-column type="selection" width="55" />
      <el-table-column prop="created_at" :label="$t('generated.settingsAuditLogsAuditLogTable_time_89b4aa')" width="170">
        <template #default="{ row }">
          {{ formatTime(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column prop="script_type" :label="$t('generated.common_script_type_f1574a')" width="100">
        <template #default="{ row }">
          {{ scriptTypeLabelKeys[row.script_type] ? $t(scriptTypeLabelKeys[row.script_type]) : row.script_type }}
        </template>
      </el-table-column>
      <el-table-column prop="audit_source" :label="$t('generated.common_audit_source_9af972')" min-width="100">
        <template #default="{ row }">
          {{ auditSourceLabelKeys[row.audit_source] ? $t(auditSourceLabelKeys[row.audit_source]) : row.audit_source }}
        </template>
      </el-table-column>
      <el-table-column prop="attempt" :label="$t('generated.common_number_of_attempts_3fd524')" width="90" />
      <el-table-column prop="passed" :label="$t('generated.common_result_0a2c91')" width="80">
        <template #default="{ row }">
          <el-tag :type="row.passed ? 'success' : 'danger'" size="small">
            {{ row.passed ? $t('dynamic.passed') : $t('common.status.failed') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="risk_level" :label="$t('generated.common_risk_level_a90f1e')" width="90">
        <template #default="{ row }">
          <el-tag :type="riskTagType(row.risk_level)" size="small">{{ row.risk_level }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="duration_ms" :label="$t('generated.common_time_consuming_a9704e')" width="90">
        <template #default="{ row }">
          {{ row.duration_ms }}ms
        </template>
      </el-table-column>
      <el-table-column :label="$t('generated.common_details_4f55ee')" width="80">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="$emit('detail', row)">{{ $t('generated.common_details_4f55ee') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-if="total > 0"
      style="margin-top: 16px; justify-content: flex-end"
      :current-page="currentPage"
      :page-size="pageSize"
      :total="total"
      layout="total, prev, pager, next"
      @current-change="handlePageChange"
    />
  </el-card>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { ref, reactive } from 'vue'
import { ElMessageBox } from 'element-plus'
import type { AuditLog } from '@/api/audit-logs'
import { scriptTypeLabelKeys, auditSourceLabelKeys } from '../constants'

defineProps<{
  logs: AuditLog[]
  loading: boolean
  total: number
}>()

const emit = defineEmits<{
  (e: 'detail', log: AuditLog): void
  (e: 'filter', params: Record<string, any>): void
  (e: 'delete', ids: string[]): void
}>()

const tableRef = ref()
const currentPage = ref(1)
const pageSize = ref(20)
const selectedRows = ref<AuditLog[]>([])
const filters = reactive({ result: '', script_type: '', audit_source: '' })

function riskTagType(level: string): string {
  if (level === 'critical') return 'danger'
  if (level === 'high') return 'warning'
  if (level === 'safe') return 'success'
  return 'info'
}

function formatTime(ts: string): string {
  if (!ts) return '-'
  return ts.replace('T', ' ').substring(0, 19)
}

function handleSelectionChange(rows: AuditLog[]) {
  selectedRows.value = rows
}

async function handleBatchDelete() {
  if (selectedRows.value.length === 0) return
  try {
    await ElMessageBox.confirm(
      translate('generatedScript.settingsAuditLogsAuditLogTable_are_you_sure_you_want_to_29e715', { p0: selectedRows.value.length }),
      translate('generatedScript.common_batch_deletion_confirmation_dc0217'),
      { confirmButtonText: translate('generatedScript.settingsAuditLogsAuditLogTable_confirm_deletion_56e90a'), cancelButtonText: translate('generatedScript.common_cancel_4d0b46'), type: 'warning' }
    )
    const ids = selectedRows.value.map(r => r.id)
    emit('delete', ids)
    selectedRows.value = []
    tableRef.value?.clearSelection()
  } catch {
    // user cancelled
  }
}

function handleFilter() {
  currentPage.value = 1
  emitParams()
}

function handlePageChange(page: number) {
  currentPage.value = page
  emitParams()
}

function emitParams() {
  const passed = filters.result === 'passed' ? 'true' : filters.result === 'failed' ? 'false' : undefined
  emit('filter', {
    page: currentPage.value,
    page_size: pageSize.value,
    passed,
    script_type: filters.script_type || undefined,
    audit_source: filters.audit_source || undefined
  })
}
</script>

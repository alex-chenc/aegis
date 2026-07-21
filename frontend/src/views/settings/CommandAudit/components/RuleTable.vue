<template>
  <el-card>
    <template #header>
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px">
        <span>{{ $t('generated.settingsCommandAuditRuleTable_blacklist_rules_9510de') }}</span>
        <div style="display: flex; gap: 8px; align-items: center">
          <el-button type="primary" size="small" @click="$emit('add')">{{ $t('generated.settingsCommandAuditRuleTable_add_new_rule_945c08') }}</el-button>
          <el-input v-model="keyword" :placeholder="$t('generated.settingsCommandAuditRuleTable_search_name_or_pattern_9e71c3')" size="small" style="width: 200px" clearable @clear="handleSearch" @keyup.enter="handleSearch" />
        </div>
      </div>
      <div style="display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap">
        <el-select v-model="filters.category" :placeholder="$t('generated.common_classification_435c52')" size="small" clearable style="width: 120px" @change="handleFilter">
          <el-option :label="$t('generated.common_file_system_42949b')" value="filesystem" />
          <el-option :label="$t('generated.common_permissions_560165')" value="permission" />
          <el-option :label="$t('generated.common_network_0cbda6')" value="network" />
          <el-option :label="$t('generated.common_system_1a1f6d')" value="system" />
          <el-option :label="$t('generated.common_privilege_escalation_b6f22d')" value="privilege" />
        </el-select>
        <el-select v-model="filters.severity" :placeholder="$t('generated.common_severity_level_a0681b')" size="small" clearable style="width: 120px" @change="handleFilter">
          <el-option :label="$t('generated.common_serious_81ffc6')" value="critical" />
          <el-option :label="$t('generated.common_high_risk_e62ee8')" value="high" />
          <el-option :label="$t('generated.common_medium_risk_1098e6')" value="medium" />
        </el-select>
        <el-select v-model="filters.match_type" :placeholder="$t('generated.common_match_type_575d0d')" size="small" clearable style="width: 120px" @change="handleFilter">
          <el-option :label="$t('generated.settingsCommandAuditRuleTable_regular_fd769f')" value="regex" />
          <el-option :label="$t('generated.settingsCommandAuditRuleTable_accurate_be955b')" value="exact" />
        </el-select>
      </div>
    </template>

    <el-table :data="rules" v-loading="loading" style="width: 100%">
      <el-table-column prop="name" :label="$t('generated.settingsCommandAuditRuleTable_name_1be7ae')" min-width="180">
        <template #default="{ row }">
          {{ row.name }}
          <el-tag v-if="row.is_preset" size="small" type="info" style="margin-left: 4px">{{ $t('generated.settingsCommandAuditRuleTable_preset_e0a915') }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="category" :label="$t('generated.common_classification_435c52')" width="100">
        <template #default="{ row }">
          <el-tag size="small">{{ categoryLabels[row.category] || row.category }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="severity" :label="$t('generated.settingsCommandAuditRuleTable_grade_5c42c0')" width="80">
        <template #default="{ row }">
          <el-tag :type="severityTagType(row.severity)" size="small">{{ severityLabels[row.severity] || row.severity }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="match_type" :label="$t('generated.common_type_e4e46c')" width="70">
        <template #default="{ row }">
          {{ row.match_type === 'regex' ? $t('dynamic.regex') : $t('dynamic.exact') }}
        </template>
      </el-table-column>
      <el-table-column prop="pattern" :label="$t('generated.common_match_pattern_a48291')" min-width="200" show-overflow-tooltip />
      <el-table-column :label="$t('generated.common_state_62e951')" width="80">
        <template #default="{ row }">
          <el-switch v-model="row.is_enabled" size="small" @change="$emit('toggle', row.id)" />
        </template>
      </el-table-column>
      <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="140">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="$emit('edit', row)">{{ $t('generated.common_edit_a7f814') }}</el-button>
          <el-button link type="danger" size="small" :disabled="row.is_preset" @click="$emit('delete', row.id)">{{ $t('generated.common_delete_3755f5') }}</el-button>
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
import type { CommandAuditRule } from '@/api/command-audit'

defineProps<{
  rules: CommandAuditRule[]
  loading: boolean
  total: number
}>()

const emit = defineEmits<{
  (e: 'add'): void
  (e: 'edit', rule: CommandAuditRule): void
  (e: 'delete', id: string): void
  (e: 'toggle', id: string): void
  (e: 'filter', params: Record<string, any>): void
}>()

const keyword = ref('')
const currentPage = ref(1)
const pageSize = ref(20)
const filters = reactive({ category: '', severity: '', match_type: '' })

const categoryLabels: Record<string, string> = {
  filesystem: translate('generatedScript.settingsCommandAuditRuleTable_file_system_42949b'),
  permission: translate('generatedScript.settingsCommandAuditRuleTable_permissions_560165'),
  network: translate('generatedScript.common_network_0cbda6'),
  system: translate('generatedScript.common_system_1a1f6d'),
  privilege: translate('generatedScript.common_privilege_escalation_b6f22d')
}

const severityLabels: Record<string, string> = {
  critical: translate('generatedScript.common_serious_81ffc6'),
  high: translate('generatedScript.common_high_risk_e62ee8'),
  medium: translate('generatedScript.common_medium_risk_1098e6')
}

function severityTagType(severity: string): string {
  if (severity === 'critical') return 'danger'
  if (severity === 'high') return 'warning'
  return 'info'
}

function handleSearch() {
  currentPage.value = 1
  emitParams()
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
  emit('filter', {
    page: currentPage.value,
    page_size: pageSize.value,
    keyword: keyword.value || undefined,
    category: filters.category || undefined,
    severity: filters.severity || undefined,
    match_type: filters.match_type || undefined
  })
}
</script>

<template>
  <el-card style="width: 100%">
    <template #header>
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px">
        <span>审计日志</span>
        <div style="display: flex; gap: 8px">
          <el-select v-model="filters.result" placeholder="结果" size="small" clearable style="width: 100px" @change="handleFilter">
            <el-option label="通过" value="passed" />
            <el-option label="失败" value="failed" />
          </el-select>
          <el-select v-model="filters.script_type" placeholder="脚本类型" size="small" clearable style="width: 120px" @change="handleFilter">
            <el-option label="检测" value="check" />
            <el-option label="修复" value="fix" />
            <el-option label="POC" value="poc_verify" />
            <el-option label="自愈" value="self_healing" />
          </el-select>
          <el-select v-model="filters.audit_source" placeholder="审计来源" size="small" clearable style="width: 120px" @change="handleFilter">
            <el-option label="黑名单" value="blacklist" />
            <el-option label="AI" value="ai" />
          </el-select>
        </div>
      </div>
    </template>

    <el-table :data="logs" v-loading="loading" style="width: 100%">
      <el-table-column prop="created_at" label="时间" width="170">
        <template #default="{ row }">
          {{ formatTime(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column prop="script_type" label="脚本类型" width="100">
        <template #default="{ row }">
          {{ scriptTypeLabels[row.script_type] || row.script_type }}
        </template>
      </el-table-column>
      <el-table-column prop="audit_source" label="审计来源" min-width="100">
        <template #default="{ row }">
          {{ auditSourceLabels[row.audit_source] || row.audit_source }}
        </template>
      </el-table-column>
      <el-table-column prop="attempt_count" label="尝试次数" width="90" />
      <el-table-column prop="result" label="结果" width="80">
        <template #default="{ row }">
          <el-tag :type="row.result === 'passed' ? 'success' : 'danger'" size="small">
            {{ row.result === 'passed' ? '通过' : '失败' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="risk_level" label="风险等级" width="90">
        <template #default="{ row }">
          <el-tag :type="riskTagType(row.risk_level)" size="small">{{ row.risk_level }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="duration_ms" label="耗时" width="90">
        <template #default="{ row }">
          {{ row.duration_ms }}ms
        </template>
      </el-table-column>
      <el-table-column label="详情" width="80">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="$emit('detail', row)">详情</el-button>
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
import { ref, reactive } from 'vue'
import type { AuditLog } from '@/api/audit-logs'

defineProps<{
  logs: AuditLog[]
  loading: boolean
  total: number
}>()

const emit = defineEmits<{
  (e: 'detail', log: AuditLog): void
  (e: 'filter', params: Record<string, any>): void
}>()

const currentPage = ref(1)
const pageSize = ref(20)
const filters = reactive({ result: '', script_type: '', audit_source: '' })

const scriptTypeLabels: Record<string, string> = {
  check: '检测',
  fix: '修复',
  poc_verify: 'POC',
  self_healing: '自愈'
}

const auditSourceLabels: Record<string, string> = {
  blacklist: '黑名单',
  ai: 'AI'
}

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
    result: filters.result || undefined,
    script_type: filters.script_type || undefined,
    audit_source: filters.audit_source || undefined
  })
}
</script>

<template>
  <el-card>
    <template #header>
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px">
        <span>黑名单规则</span>
        <div style="display: flex; gap: 8px; align-items: center">
          <el-button type="primary" size="small" @click="$emit('add')">新增规则</el-button>
          <el-input v-model="keyword" placeholder="搜索名称或模式" size="small" style="width: 200px" clearable @clear="handleSearch" @keyup.enter="handleSearch" />
        </div>
      </div>
      <div style="display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap">
        <el-select v-model="filters.category" placeholder="分类" size="small" clearable style="width: 120px" @change="handleFilter">
          <el-option label="文件系统" value="filesystem" />
          <el-option label="权限" value="permission" />
          <el-option label="网络" value="network" />
          <el-option label="系统" value="system" />
          <el-option label="权限提升" value="privilege" />
        </el-select>
        <el-select v-model="filters.severity" placeholder="严重等级" size="small" clearable style="width: 120px" @change="handleFilter">
          <el-option label="严重" value="critical" />
          <el-option label="高危" value="high" />
          <el-option label="中危" value="medium" />
        </el-select>
        <el-select v-model="filters.match_type" placeholder="匹配类型" size="small" clearable style="width: 120px" @change="handleFilter">
          <el-option label="正则" value="regex" />
          <el-option label="精确" value="exact" />
        </el-select>
      </div>
    </template>

    <el-table :data="rules" v-loading="loading" style="width: 100%">
      <el-table-column prop="name" label="名称" min-width="180">
        <template #default="{ row }">
          {{ row.name }}
          <el-tag v-if="row.is_preset" size="small" type="info" style="margin-left: 4px">预置</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="category" label="分类" width="100">
        <template #default="{ row }">
          <el-tag size="small">{{ categoryLabels[row.category] || row.category }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="severity" label="等级" width="80">
        <template #default="{ row }">
          <el-tag :type="severityTagType(row.severity)" size="small">{{ severityLabels[row.severity] || row.severity }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="match_type" label="类型" width="70">
        <template #default="{ row }">
          {{ row.match_type === 'regex' ? '正则' : '精确' }}
        </template>
      </el-table-column>
      <el-table-column prop="pattern" label="匹配模式" min-width="200" show-overflow-tooltip />
      <el-table-column label="状态" width="80">
        <template #default="{ row }">
          <el-switch v-model="row.is_enabled" size="small" @change="$emit('toggle', row.id)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="$emit('edit', row)">编辑</el-button>
          <el-button link type="danger" size="small" :disabled="row.is_preset" @click="$emit('delete', row.id)">删除</el-button>
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
  filesystem: '文件系统',
  permission: '权限',
  network: '网络',
  system: '系统',
  privilege: '权限提升'
}

const severityLabels: Record<string, string> = {
  critical: '严重',
  high: '高危',
  medium: '中危'
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

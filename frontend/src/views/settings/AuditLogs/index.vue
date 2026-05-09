<template>
  <div class="audit-logs-page">
    <h2 style="margin-bottom: 16px">审计日志</h2>

    <AuditStatsCard :stats="stats" style="margin-bottom: 16px" />

    <AuditLogTable
      :logs="logs"
      :loading="loading"
      :total="total"
      @detail="showDetail"
      @filter="handleFilter"
      @delete="handleDelete"
    />

    <AuditDetailDrawer
      :visible="drawerVisible"
      :log="selectedLog"
      @close="drawerVisible = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import AuditStatsCard from './components/AuditStatsCard.vue'
import AuditLogTable from './components/AuditLogTable.vue'
import AuditDetailDrawer from './components/AuditDetailDrawer.vue'
import { useAuditLogs } from './composables/useAuditLogs'
import type { AuditLog } from '@/api/audit-logs'

const {
  logs, total, loading, stats,
  fetchLogs, fetchStats, fetchLogDetail, deleteLogs
} = useAuditLogs()

const drawerVisible = ref(false)
const selectedLog = ref<AuditLog | null>(null)

onMounted(async () => {
  await Promise.all([fetchLogs(), fetchStats()])
})

async function showDetail(log: AuditLog) {
  try {
    selectedLog.value = await fetchLogDetail(log.id)
    drawerVisible.value = true
  } catch {
    selectedLog.value = log
    drawerVisible.value = true
  }
}

async function handleFilter(params: Record<string, any>) {
  await fetchLogs(params)
}

async function handleDelete(ids: string[]) {
  try {
    const deleted = await deleteLogs(ids)
    ElMessage.success(`成功删除 ${deleted} 条审计日志`)
  } catch {
    ElMessage.error('删除失败，请重试')
  }
}
</script>

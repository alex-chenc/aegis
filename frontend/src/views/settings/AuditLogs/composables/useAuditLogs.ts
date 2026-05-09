import { ref } from 'vue'
import { auditLogApi, type AuditLog, type AuditStats, type AuditLogParams } from '@/api/audit-logs'

export function useAuditLogs() {
  const logs = ref<AuditLog[]>([])
  const total = ref(0)
  const loading = ref(false)
  const stats = ref<AuditStats | null>(null)
  const queryParams = ref<AuditLogParams>({ page: 1, page_size: 20 })

  const fetchLogs = async (params?: AuditLogParams) => {
    if (params) queryParams.value = { ...queryParams.value, ...params }
    loading.value = true
    try {
      const res = await auditLogApi.getLogs(queryParams.value)
      logs.value = res.items || []
      total.value = res.total || 0
    } finally {
      loading.value = false
    }
  }

  const fetchStats = async () => {
    stats.value = await auditLogApi.getStats()
  }

  const fetchLogDetail = async (id: string) => {
    return await auditLogApi.getLog(id)
  }

  const deleteLogs = async (ids: string[]) => {
    const res = await auditLogApi.deleteLogs(ids)
    await fetchLogs()
    await fetchStats()
    return res.deleted
  }

  return {
    logs,
    total,
    loading,
    stats,
    queryParams,
    fetchLogs,
    fetchStats,
    fetchLogDetail,
    deleteLogs
  }
}

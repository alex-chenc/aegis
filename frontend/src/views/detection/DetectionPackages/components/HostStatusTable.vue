<template>
  <div>
    <el-row :gutter="12" class="status-stats">
      <el-col :span="6">
        <el-card shadow="never">
          <el-statistic title="安装中" :value="stats.installing" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never">
          <el-statistic title="成功" :value="stats.success" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never">
          <el-statistic title="错误" :value="stats.failed" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never">
          <el-statistic title="超时" :value="stats.timeout" />
        </el-card>
      </el-col>
    </el-row>

    <el-table :data="hosts" border size="small" style="margin-top: 12px;">
      <el-table-column prop="hostname" label="主机名" min-width="150" />
      <el-table-column prop="kernel_release" label="内核版本" width="150" />
      <el-table-column prop="arch" label="架构" width="80" />
      <el-table-column label="状态" width="140">
        <template #default="{ row }">
          <PackageStatusTag :status="row.status" />
        </template>
      </el-table-column>
      <el-table-column prop="active_artifact" label="Artifact" width="100" />
      <el-table-column label="已加载 Hooks" min-width="200">
        <template #default="{ row }">
          <el-tag v-for="h in row.loaded_hooks" :key="h" size="small" style="margin: 2px;">{{ h }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="error_message" label="错误信息" min-width="200" show-overflow-tooltip />
      <el-table-column prop="last_reported_at" label="最近上报" width="170">
        <template #default="{ row }">
          {{ row.last_reported_at ? new Date(row.last_reported_at).toLocaleString() : '-' }}
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-if="total > pageSize"
      :current-page="currentPage"
      :page-size="pageSize"
      :total="total"
      layout="total, prev, pager, next"
      style="margin-top: 12px; justify-content: flex-end;"
      @current-change="(p: number) => { currentPage = p; $emit('page-change', p) }"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { PackageHostStatus } from '@/api/detection-packages'
import PackageStatusTag from './PackageStatusTag.vue'

const props = defineProps<{
  hosts: PackageHostStatus[]
  total: number
}>()

defineEmits<{
  (e: 'page-change', page: number): void
}>()

const currentPage = ref(1)
const pageSize = ref(20)

const stats = computed(() => {
  const installing = props.hosts.filter(h => ['pending', 'downloading', 'installing'].includes(h.status)).length
  const success = props.hosts.filter(h => h.status === 'active').length
  const failed = props.hosts.filter(h => ['load_failed', 'signature_failed', 'blocked_by_hook_allowlist'].includes(h.status)).length
  const timeout = props.hosts.filter(h => h.status === 'timeout').length
  return { installing, success, failed, timeout }
})
</script>

<style scoped>
.status-stats :deep(.el-card) {
  text-align: center;
}
</style>

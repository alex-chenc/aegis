<template>
  <el-table :data="hooks" border size="small">
    <el-table-column prop="name" label="Hook 名称" min-width="150" />
    <el-table-column prop="attach_type" label="类型" width="120">
      <template #default="{ row }">
        <el-tag :type="getTypeColor(row.attach_type)" size="small">{{ row.attach_type }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="attach" label="挂载点" min-width="200" />
    <el-table-column prop="program" label="程序" min-width="150" />
    <el-table-column label="Allowlist" width="100" align="center">
      <template #default="{ row }">
        <el-tag v-if="row.allowed === true" type="success" size="small">允许</el-tag>
        <el-tag v-else-if="row.allowed === false" type="danger" size="small">受限</el-tag>
        <el-tag v-else type="info" size="small">未检查</el-tag>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import type { PackageHook } from '@/api/detection-packages'

defineProps<{
  hooks: PackageHook[]
}>()

function getTypeColor(type: string) {
  const map: Record<string, string> = {
    tracepoint: 'success',
    kprobe: 'warning',
    lsm: 'danger',
    xdp: 'danger',
    tc: 'danger',
  }
  return map[type] || 'info'
}
</script>

<template>
  <el-table :data="hooks" border size="small">
    <el-table-column prop="name" :label="$t('generated.detectionDetectionPackagesHookSummaryTable_hook_name_c2a7e1')" min-width="150" />
    <el-table-column prop="attach_type" :label="$t('generated.common_type_e4e46c')" width="120">
      <template #default="{ row }">
        <el-tag :type="getTypeColor(row.attach_type)" size="small">{{ row.attach_type }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="attach" :label="$t('generated.detectionDetectionPackagesHookSummaryTable_mount_point_8517da')" min-width="200" />
    <el-table-column prop="program" :label="$t('generated.detectionDetectionPackagesHookSummaryTable_program_1a68c2')" min-width="150" />
    <el-table-column label="Allowlist" width="100" align="center">
      <template #default="{ row }">
        <el-tag v-if="row.allowed === true" type="success" size="small">{{ $t('generated.detectionDetectionPackagesHookSummaryTable_allow_4c0c0a') }}</el-tag>
        <el-tag v-else-if="row.allowed === false" type="danger" size="small">{{ $t('generated.detectionDetectionPackagesHookSummaryTable_restricted_936333') }}</el-tag>
        <el-tag v-else type="info" size="small">{{ $t('generated.detectionDetectionPackagesHookSummaryTable_not_checked_ed9ec8') }}</el-tag>
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

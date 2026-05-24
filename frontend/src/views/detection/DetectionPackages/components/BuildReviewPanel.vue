<template>
  <div class="build-review-panel">
    <el-descriptions :column="2" border size="small">
      <el-descriptions-item label="Package ID">{{ build?.package_id }}</el-descriptions-item>
      <el-descriptions-item label="版本">{{ build?.version }}</el-descriptions-item>
      <el-descriptions-item label="状态">
        <PackageStatusTag :status="build?.status || ''" />
      </el-descriptions-item>
      <el-descriptions-item label="Builder Image Digest">
        <el-text truncated>{{ build?.builder_image_digest }}</el-text>
      </el-descriptions-item>
      <el-descriptions-item label="Clang 版本">{{ build?.clang_version }}</el-descriptions-item>
      <el-descriptions-item label="构建时间">{{ build?.created_at ? new Date(build.created_at).toLocaleString() : '-' }}</el-descriptions-item>
    </el-descriptions>

    <el-divider content-position="left">Hook 列表</el-divider>
    <HookSummaryTable :hooks="build?.hook_summary || []" />

    <el-divider content-position="left">Artifact</el-divider>
    <el-table :data="build?.artifacts || []" border size="small">
      <el-table-column prop="name" label="文件名" min-width="200" />
      <el-table-column prop="transport" label="Transport" width="120">
        <template #default="{ row }">
          <el-tag :type="row.transport === 'ringbuf' ? 'success' : ''" size="small">{{ row.transport }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="120">
        <template #default="{ row }">{{ formatSize(row.size) }}</template>
      </el-table-column>
      <el-table-column prop="sha256" label="SHA256" min-width="200" show-overflow-tooltip />
    </el-table>

    <el-divider content-position="left">构建日志</el-divider>
    <el-input
      type="textarea"
      :rows="10"
      :model-value="build?.build_log_tail || ''"
      readonly
      class="log-textarea"
    />

    <div v-if="build?.error_message" style="margin-top: 12px;">
      <el-alert :title="build.error_message" type="error" show-icon :closable="false" />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { DetectionPackageBuild } from '@/api/detection-packages'
import HookSummaryTable from './HookSummaryTable.vue'
import PackageStatusTag from './PackageStatusTag.vue'

defineProps<{
  build: DetectionPackageBuild | null
}>()

function formatSize(bytes: number) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}
</script>

<style scoped>
.log-textarea :deep(textarea) {
  font-family: monospace;
  font-size: 12px;
}
</style>

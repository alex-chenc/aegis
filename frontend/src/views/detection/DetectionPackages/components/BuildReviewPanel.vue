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
    <div style="margin-top: 8px; text-align: right;">
      <el-button size="small" @click="handleDownloadLog" :loading="downloadingLog" :disabled="!build?.build_log_object_key">下载完整日志</el-button>
    </div>

    <template v-if="build?.status === 'awaiting_review'">
      <el-divider content-position="left">构建审核</el-divider>
      <el-input type="textarea" :rows="3" v-model="reviewComment" placeholder="审核意见" style="margin-bottom: 8px;" />
      <div v-if="canOperate('review')" style="display: flex; gap: 8px;">
        <el-button type="success" :loading="reviewing" @click="handleReview(true)">审核通过</el-button>
        <el-button type="danger" :loading="reviewing" @click="handleReview(false)">审核拒绝</el-button>
      </div>
    </template>

    <template v-if="build?.status === 'built'">
      <el-divider content-position="left">签名发布</el-divider>
      <el-button v-if="canOperate('sign')" type="warning" @click="emit('sign')">签名发布</el-button>
    </template>

    <div v-if="build?.error_message" style="margin-top: 12px;">
      <el-alert :title="build.error_message" type="error" show-icon :closable="false" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { DetectionPackageBuild } from '@/api/detection-packages'
import { detectionPackageApi } from '@/api/detection-packages'
import { useRole } from '@/composables/useRole'
import { ElMessage } from 'element-plus'
import HookSummaryTable from './HookSummaryTable.vue'
import PackageStatusTag from './PackageStatusTag.vue'

const props = defineProps<{
  build: DetectionPackageBuild | null
}>()

const emit = defineEmits<{ (e: 'sign'): void }>()

const { canOperate } = useRole()
const reviewComment = ref('')
const reviewing = ref(false)
const downloadingLog = ref(false)

async function handleReview(approved: boolean) {
  reviewing.value = true
  try {
    await detectionPackageApi.reviewBuild(props.build!.id, { approved, comment: reviewComment.value })
    ElMessage.success(approved ? '审核通过' : '审核拒绝')
  } catch (e: any) {
    ElMessage.error(e.message || '审核失败')
  } finally {
    reviewing.value = false
  }
}

async function handleDownloadLog() {
  downloadingLog.value = true
  try {
    const res = await detectionPackageApi.getBuildLog(props.build!.id)
    window.open(res.log_url, '_blank')
  } catch (e: any) {
    ElMessage.error(e.message || '下载日志失败')
  } finally {
    downloadingLog.value = false
  }
}

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

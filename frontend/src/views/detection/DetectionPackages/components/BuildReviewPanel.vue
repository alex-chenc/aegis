<template>
  <div class="build-review-panel">
    <el-descriptions :column="2" border size="small">
      <el-descriptions-item label="Package ID">{{ build?.package_id }}</el-descriptions-item>
      <el-descriptions-item :label="$t('generated.common_version_989d1a')">{{ build?.version }}</el-descriptions-item>
      <el-descriptions-item :label="$t('generated.common_state_62e951')">
        <PackageStatusTag :status="build?.status || ''" />
      </el-descriptions-item>
      <el-descriptions-item label="Builder Image Digest">
        <el-text truncated>{{ build?.builder_image_digest }}</el-text>
      </el-descriptions-item>
      <el-descriptions-item :label="$t('generated.detectionDetectionPackagesBuildReviewPanel_clang_version_e3dd84')">{{ build?.clang_version }}</el-descriptions-item>
      <el-descriptions-item :label="$t('generated.common_build_time_ed9e57')">{{ build?.created_at ? formatDateTime(build.created_at) : '-' }}</el-descriptions-item>
    </el-descriptions>

    <el-divider content-position="left">{{ $t('generated.detectionDetectionPackagesBuildReviewPanel_hook_list_87da21') }}</el-divider>
    <HookSummaryTable :hooks="build?.hook_summary || []" />

    <el-divider content-position="left">Artifact</el-divider>
    <el-table :data="build?.artifacts || []" border size="small">
      <el-table-column prop="name" :label="$t('generated.detectionDetectionPackagesBuildReviewPanel_file_name_1275f6')" min-width="200" />
      <el-table-column prop="transport" label="Transport" width="120">
        <template #default="{ row }">
          <el-tag :type="row.transport === 'ringbuf' ? 'success' : ''" size="small">{{ row.transport }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="$t('generated.detectionDetectionPackagesBuildReviewPanel_size_fd2070')" width="120">
        <template #default="{ row }">{{ formatSize(row.size) }}</template>
      </el-table-column>
      <el-table-column prop="sha256" label="SHA256" min-width="200" show-overflow-tooltip />
    </el-table>

    <el-divider content-position="left">{{ $t('generated.detectionDetectionPackagesBuildReviewPanel_build_log_c92b71') }}</el-divider>
    <el-input
      type="textarea"
      :rows="10"
      :model-value="build?.build_log_tail || ''"
      readonly
      class="log-textarea"
    />
    <div style="margin-top: 8px; text-align: right;">
      <el-button size="small" @click="handleDownloadLog" :loading="downloadingLog" :disabled="!build?.build_log_object_key">{{ $t('generated.detectionDetectionPackagesBuildReviewPanel_download_full_log_5f7a73') }}</el-button>
    </div>

    <template v-if="build?.status === 'awaiting_review'">
      <el-divider content-position="left">{{ $t('generated.common_build_review_7ebabd') }}</el-divider>
      <el-input type="textarea" :rows="3" v-model="reviewComment" :placeholder="$t('generated.detectionDetectionPackagesBuildReviewPanel_review_comments_5f937a')" style="margin-bottom: 8px;" />
      <div v-if="canOperate('review')" style="display: flex; gap: 8px;">
        <el-button type="success" :loading="reviewing" @click="handleReview(true)">{{ $t('generated.detectionDetectionPackagesBuildReviewPanel_approved_637104') }}</el-button>
        <el-button type="danger" :loading="reviewing" @click="handleReview(false)">{{ $t('generated.common_review_rejection_8942f5') }}</el-button>
      </div>
    </template>

    <template v-if="build?.status === 'success'">
      <el-divider content-position="left">{{ $t('generated.common_signature_release_bae7c7') }}</el-divider>
      <el-button v-if="canOperate('sign')" type="warning" @click="emit('sign')">{{ $t('generated.common_signature_release_bae7c7') }}</el-button>
    </template>

    <div v-if="build?.error_message" style="margin-top: 12px;">
      <el-alert :title="build.error_message" type="error" show-icon :closable="false" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

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

const emit = defineEmits<{
  (e: 'sign'): void
  (e: 'reviewed'): void
}>()

const { canOperate } = useRole()
const reviewComment = ref('')
const reviewing = ref(false)
const downloadingLog = ref(false)

async function handleReview(approved: boolean) {
  reviewing.value = true
  try {
    await detectionPackageApi.reviewBuild(props.build!.id, { approved, comment: reviewComment.value })
    ElMessage.success(approved ? translate('generatedScript.detectionDetectionPackagesBuildReviewPanel_approved_637104') : translate('generatedScript.common_review_rejection_8942f5'))
    emit('reviewed')
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.detectionDetectionPackagesBuildReviewPanel_review_failed_7a3e0e'))
  } finally {
    reviewing.value = false
  }
}

async function handleDownloadLog() {
  downloadingLog.value = true
  try {
    const res = await detectionPackageApi.getBuildLog(props.build!.id)
    const url = res.log_url || res.url
    if (!url) {
      ElMessage.warning(translate('generatedScript.detectionDetectionPackagesBuildReviewPanel_full_log_not_available_yet_ab100a'))
      return
    }
    window.open(url, '_blank')
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.detectionDetectionPackagesBuildReviewPanel_download_log_failed_c9f5e5'))
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

<template>
  <el-tag :type="tagType" :effect="effect" size="small">
    {{ statusText }}
  </el-tag>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed } from 'vue'

const props = defineProps<{
  status: string
  effect?: 'dark' | 'light' | 'plain'
}>()

const statusMap: Record<string, { type: string; textKey: string }> = {
  draft: { type: 'info', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_draft_0f4368' },
  build_pending: { type: 'info', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_to_be_built_099567' },
  build_running: { type: 'warning', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_under_construction_556441' },
  build_failed: { type: 'danger', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_build_failed_6518bf' },
  build_success: { type: 'success', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_build_successful_ac0ccc' },
  built: { type: '', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_built_2061e6' },
  signed: { type: 'success', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_signed_d7dfc4' },
  enabled: { type: 'success', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_enabled_25d284' },
  active: { type: 'success', textKey: 'generatedScript.common_running_594249' },
  degraded: { type: 'warning', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_downgrade_d85188' },
  load_failed: { type: 'danger', textKey: 'generatedScript.common_loading_failed_f6b7a4' },
  timeout: { type: 'danger', textKey: 'generatedScript.common_time_out_ff06c2' },
  disabled: { type: 'info', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_disabled_0fe5a9' },
  uninstalled: { type: 'info', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_uninstalled_c2ad33' },
  pending: { type: 'info', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_pending_59a9eb' },
  running: { type: 'warning', textKey: 'generatedScript.common_running_594249' },
  awaiting_review: { type: 'warning', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_pending_review_f53b68' },
  review_rejected: { type: 'danger', textKey: 'generatedScript.common_review_rejection_8942f5' },
  success: { type: 'success', textKey: 'generatedScript.common_success_51991a' },
  failed: { type: 'danger', textKey: 'generatedScript.common_fail_3e3c80' },
  downloading: { type: 'warning', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_downloading_327d59' },
  signature_failed: { type: 'danger', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_signature_verification_failed_7ef0f1' },
  blocked_by_hook_allowlist: { type: 'danger', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_hook_is_restricted_6b5d6b' },
  installing: { type: 'warning', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_installing_411642' },
  disabled_by_policy: { type: 'info', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_policy_disabled_91871a' },
  disabled_by_rate: { type: 'warning', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_speed_limit_disabled_47d216' },
  rolled_back: { type: 'warning', textKey: 'generatedScript.detectionDetectionPackagesPackageStatusTag_rolled_back_c4ab8c' },
}

const tagType = computed(() => statusMap[props.status]?.type || 'info')
const statusText = computed(() => statusMap[props.status] ? translate(statusMap[props.status].textKey) : props.status)
</script>

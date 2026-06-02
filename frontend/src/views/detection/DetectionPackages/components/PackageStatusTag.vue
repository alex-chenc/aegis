<template>
  <el-tag :type="tagType" :effect="effect" size="small">
    {{ statusText }}
  </el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  effect?: 'dark' | 'light' | 'plain'
}>()

const statusMap: Record<string, { type: string; text: string }> = {
  draft: { type: 'info', text: '草稿' },
  build_pending: { type: 'info', text: '待构建' },
  build_running: { type: 'warning', text: '构建中' },
  build_failed: { type: 'danger', text: '构建失败' },
  build_success: { type: 'success', text: '构建成功' },
  built: { type: '', text: '已构建' },
  signed: { type: 'success', text: '已签名' },
  enabled: { type: 'success', text: '已启用' },
  active: { type: 'success', text: '运行中' },
  degraded: { type: 'warning', text: '降级' },
  load_failed: { type: 'danger', text: '加载失败' },
  timeout: { type: 'danger', text: '超时' },
  disabled: { type: 'info', text: '已禁用' },
  uninstalled: { type: 'info', text: '已卸载' },
  pending: { type: 'info', text: '待处理' },
  running: { type: 'warning', text: '运行中' },
  awaiting_review: { type: 'warning', text: '待审核' },
  review_rejected: { type: 'danger', text: '审核拒绝' },
  success: { type: 'success', text: '成功' },
  failed: { type: 'danger', text: '失败' },
  downloading: { type: 'warning', text: '下载中' },
  signature_failed: { type: 'danger', text: '签名验证失败' },
  blocked_by_hook_allowlist: { type: 'danger', text: 'Hook 受限' },
  installing: { type: 'warning', text: '安装中' },
  disabled_by_policy: { type: 'info', text: '策略禁用' },
  disabled_by_rate: { type: 'warning', text: '限速禁用' },
  rolled_back: { type: 'warning', text: '已回滚' },
}

const tagType = computed(() => statusMap[props.status]?.type || 'info')
const statusText = computed(() => statusMap[props.status]?.text || props.status)
</script>

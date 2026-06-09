<template>
  <div class="composer">
    <div class="composer-toolbar">
      <el-radio-group
        :model-value="approvalMode"
        size="small"
        :disabled="disabled || modeLoading"
        class="approval-mode"
        @change="handleApprovalModeChange"
      >
        <el-radio-button value="request_approval">
          <el-icon><Lock /></el-icon>
          请求确认
        </el-radio-button>
        <el-radio-button value="whitelist">
          <el-icon><List /></el-icon>
          白名单
        </el-radio-button>
        <el-radio-button value="full_access">
          <el-icon><Unlock /></el-icon>
          全权限
        </el-radio-button>
      </el-radio-group>

      <div class="upload-controls">
        <el-select
          v-model="uploadPurpose"
          size="small"
          class="purpose-select"
          :disabled="disabled || uploading"
        >
          <el-option label="分析文件" value="analysis" />
          <el-option label="基线模板" value="baseline_template" />
          <el-option label="Sigma 规则" value="sigma_rule" />
        </el-select>
        <el-button
          size="small"
          :loading="uploading"
          :disabled="disabled || uploading"
          @click="openFilePicker"
        >
          <el-icon><Upload /></el-icon>
          上传
        </el-button>
        <input
          ref="fileInputRef"
          class="hidden-file-input"
          type="file"
          @change="handleFileSelected"
        >
      </div>
    </div>

    <div v-if="uploadItems.length" class="upload-chip-row">
      <div
        v-for="item in uploadItems"
        :key="item.id"
        class="upload-chip"
        :class="item.status"
      >
        <el-icon class="file-icon"><Document /></el-icon>
        <span class="file-name" :title="item.name">{{ item.name }}</span>
        <el-icon v-if="item.status === 'uploading'" class="status-icon loading"><Loading /></el-icon>
        <el-icon v-else-if="item.status === 'success'" class="status-icon success"><CircleCheck /></el-icon>
        <el-tooltip v-else :content="item.error || '上传失败'" placement="top">
          <el-icon class="status-icon error"><CircleClose /></el-icon>
        </el-tooltip>
        <el-button
          class="chip-close"
          link
          :icon="Close"
          @click="$emit('remove-upload', item.id)"
        />
      </div>
    </div>

    <div class="composer-input">
      <el-input
        ref="inputRef"
        v-model="inputText"
        type="textarea"
        :rows="2"
        :placeholder="disabled ? '等待响应完成...' : '输入消息，Enter 发送...'"
        :disabled="disabled"
        @keydown="handleKeydown"
      />
    </div>
    <div class="composer-actions">
      <div class="composer-hint">
        <span class="hint-text">Enter 发送，Shift+Enter 换行</span>
      </div>
      <el-button
        type="primary"
        :loading="disabled"
        :disabled="!inputText.trim() || disabled"
        @click="handleSend"
      >
        <el-icon><Promotion /></el-icon>
        发送
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { CircleCheck, CircleClose, Close, Document, List, Loading, Lock, Promotion, Unlock, Upload } from '@element-plus/icons-vue'
import type { AssistantFileUploadPurpose, AssistantToolApprovalMode } from '@/api/assistant'

type UploadItem = {
  id: string
  name: string
  purpose: AssistantFileUploadPurpose
  status: 'uploading' | 'success' | 'error'
  error?: string
}

const props = withDefaults(defineProps<{
  disabled: boolean
  approvalMode?: AssistantToolApprovalMode
  modeLoading?: boolean
  uploading?: boolean
  uploadItems?: UploadItem[]
}>(), {
  approvalMode: 'whitelist',
  modeLoading: false,
  uploading: false,
  uploadItems: () => [],
})

const emit = defineEmits<{
  send: [content: string]
  'approval-mode-change': [mode: AssistantToolApprovalMode]
  'upload-file': [file: File, purpose: AssistantFileUploadPurpose]
  'remove-upload': [id: string]
}>()

const inputRef = ref()
const fileInputRef = ref<HTMLInputElement | null>(null)
const inputText = ref('')
const uploadPurpose = ref<AssistantFileUploadPurpose>('analysis')

function handleKeydown(e: KeyboardEvent) {
  // Enter 发送，Shift+Enter 换行
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

function handleSend() {
  const content = inputText.value.trim()
  if (!content || props.disabled) return
  emit('send', content)
  inputText.value = ''
}

function handleApprovalModeChange(value: string | number | boolean | undefined) {
  if (typeof value !== 'string') return
  emit('approval-mode-change', value as AssistantToolApprovalMode)
}

function openFilePicker() {
  if (props.disabled || props.uploading) return
  fileInputRef.value?.click()
}

function handleFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) {
    emit('upload-file', file, uploadPurpose.value)
  }
  input.value = ''
}
</script>

<style scoped>
.composer {
  padding: 12px;
  background: #fff;
  border: 1px solid #dbe4ef;
  border-radius: 8px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
}

.composer-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}

.approval-mode {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.approval-mode :deep(.el-radio-button__inner) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 86px;
  justify-content: center;
  border-radius: 6px;
  font-weight: 650;
}

.upload-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.purpose-select {
  width: 122px;
}

.hidden-file-input {
  display: none;
}

.upload-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 2px 0 10px;
}

.upload-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: min(100%, 360px);
  min-height: 30px;
  padding: 4px 6px 4px 9px;
  border: 1px solid #dbe4ef;
  border-radius: 7px;
  background: #f8fafc;
  color: #334155;
  font-size: 12px;
}

.upload-chip.error {
  border-color: #fecaca;
  background: #fff1f2;
  color: #991b1b;
}

.file-icon {
  color: #64748b;
  flex: none;
}

.file-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.status-icon {
  flex: none;
}

.status-icon.loading {
  color: #409eff;
  animation: spin 0.8s linear infinite;
}

.status-icon.success {
  color: #16a34a;
}

.status-icon.error {
  color: #dc2626;
}

.chip-close {
  width: 20px;
  height: 20px;
  min-height: 20px;
  padding: 0;
  color: #64748b;
  flex: none;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.composer-input {
  margin-bottom: 10px;
}

.composer-input :deep(.el-textarea__inner) {
  min-height: 72px !important;
  border-radius: 8px;
  border-color: #dbe4ef;
  background: #f8fafc;
  box-shadow: none;
  line-height: 1.55;
  transition: border-color 0.18s ease, background 0.18s ease, box-shadow 0.18s ease;
}

.composer-input :deep(.el-textarea__inner:focus) {
  border-color: #409eff;
  background: #fff;
  box-shadow: 0 0 0 3px rgba(64, 158, 255, 0.12);
}

.composer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.composer-hint {
  font-size: 12px;
  color: #909399;
}

.hint-text {
  display: flex;
  align-items: center;
  gap: 4px;
}

.composer-actions :deep(.el-button) {
  border-radius: 6px;
  padding: 8px 18px;
  font-weight: 600;
}

@media (max-width: 720px) {
  .composer-toolbar {
    align-items: stretch;
  }

  .upload-controls,
  .approval-mode {
    width: 100%;
  }

  .purpose-select {
    flex: 1;
  }
}
</style>

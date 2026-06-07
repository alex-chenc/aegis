<template>
  <div class="composer">
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
import { Promotion } from '@element-plus/icons-vue'

defineProps<{
  disabled: boolean
}>()

const emit = defineEmits<{
  send: [content: string]
}>()

const inputRef = ref()
const inputText = ref('')

function handleKeydown(e: KeyboardEvent) {
  // Enter 发送，Shift+Enter 换行
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

function handleSend() {
  const content = inputText.value.trim()
  if (!content) return
  emit('send', content)
  inputText.value = ''
}
</script>

<style scoped>
.composer {
  padding: 14px;
  background: #fff;
  border: 1px solid #dbe4ef;
  border-radius: 16px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
}

.composer-input {
  margin-bottom: 10px;
}

.composer-input :deep(.el-textarea__inner) {
  min-height: 72px !important;
  border-radius: 12px;
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
  border-radius: 999px;
  padding: 8px 18px;
  font-weight: 600;
}
</style>

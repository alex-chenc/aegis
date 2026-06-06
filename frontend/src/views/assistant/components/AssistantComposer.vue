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
  padding: 16px 20px;
  background: #fff;
  border-top: 1px solid #e4e7ed;
}

.composer-input {
  margin-bottom: 8px;
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
</style>

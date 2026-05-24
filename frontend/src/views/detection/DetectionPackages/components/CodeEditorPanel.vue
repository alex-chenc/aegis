<template>
  <div class="code-editor-panel">
    <div class="editor-header">
      <el-select v-model="currentLang" size="small" style="width: 120px;">
        <el-option label="YAML" value="yaml" />
        <el-option label="C" value="c" />
        <el-option label="JSON" value="json" />
      </el-select>
      <el-text v-if="errorMsg" type="danger" size="small" style="margin-left: 12px;">{{ errorMsg }}</el-text>
    </div>
    <el-input
      ref="editorRef"
      type="textarea"
      :rows="20"
      :model-value="modelValue"
      @update:model-value="$emit('update:modelValue', $event)"
      @blur="validate"
      class="code-textarea"
      :placeholder="placeholder"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  modelValue: string
  language?: 'yaml' | 'c' | 'json'
  placeholder?: string
}>()

defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const currentLang = ref(props.language || 'yaml')
const errorMsg = ref('')

function validate() {
  errorMsg.value = ''
  if (!props.modelValue?.trim()) return

  if (currentLang.value === 'json') {
    try {
      JSON.parse(props.modelValue)
    } catch (e: any) {
      errorMsg.value = `JSON 格式错误: ${e.message}`
    }
  }
}
</script>

<style scoped>
.editor-header {
  margin-bottom: 8px;
  display: flex;
  align-items: center;
}
.code-textarea :deep(textarea) {
  font-family: 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.5;
}
</style>

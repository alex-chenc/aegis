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
      @input="validate"
      class="code-textarea"
      :placeholder="placeholder"
      :readonly="readonly"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import * as yaml from 'js-yaml'

const props = defineProps<{
  modelValue: string
  language?: 'yaml' | 'c' | 'json'
  placeholder?: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'validationError', error: string): void
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

  if (currentLang.value === 'yaml') {
    try {
      yaml.load(props.modelValue)
    } catch (e: any) {
      errorMsg.value = `YAML 格式错误: ${e.message}`
    }
  }

  emit('validationError', errorMsg.value)
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

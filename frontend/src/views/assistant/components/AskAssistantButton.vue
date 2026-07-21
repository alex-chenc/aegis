<template>
  <el-button
    class="ask-assistant-btn"
    :type="type"
    :size="size"
    @click="navigateToAssistant"
  >
    <el-icon><MagicStick /></el-icon>
    <span>{{ $t('generated.assistantAskAssistantButton_ask_assistant_50785c') }}</span>
  </el-button>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { MagicStick } from '@element-plus/icons-vue'

const props = withDefaults(defineProps<{
  objectType?: string
  objectId?: string
  prompt?: string
  type?: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  size?: 'large' | 'default' | 'small'
}>(), {
  objectType: '',
  objectId: '',
  prompt: '',
  type: 'primary',
  size: 'small',
})

const router = useRouter()

function navigateToAssistant() {
  const query: Record<string, string> = {}
  if (props.objectType) {
    query.context_type = props.objectType
  }
  if (props.objectId) {
    query.context_id = props.objectId
  }
  if (props.prompt) {
    query.prompt = props.prompt
  }
  router.push({ path: '/assistant', query })
}
</script>

<style scoped>
.ask-assistant-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
</style>

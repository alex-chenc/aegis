<template>
  <el-tag :type="tagType" :effect="effect" size="small">
    {{ severity }}
  </el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  severity: 'Critical' | 'High' | 'Medium' | 'Low'
}>()

const tagType = computed(() => {
  switch (props.severity) {
    case 'Critical': return 'danger'
    case 'High': return 'warning'
    case 'Medium': return ''
    case 'Low': return 'success'
    default: return 'info'
  }
})

const effect = computed(() => {
  return props.severity === 'Critical' || props.severity === 'High' ? 'dark' : 'light'
})
</script>

<style scoped>
.el-tag {
  font-weight: 500;
  min-width: 70px;
  text-align: center;
}
</style>
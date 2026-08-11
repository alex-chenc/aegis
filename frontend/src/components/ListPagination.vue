<template>
  <div v-if="total > 0" class="list-pagination">
    <el-pagination
      background
      layout="total, prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="$emit('change', $event)"
    />
  </div>
</template>

<script setup lang="ts">
defineProps<{ page: number; pageSize: number; total: number }>()
defineEmits<{ change: [page: number] }>()
</script>

<style scoped>
.list-pagination { display: flex; justify-content: flex-end; padding-top: 16px; }
</style>

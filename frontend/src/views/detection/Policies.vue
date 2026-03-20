<template>
  <div class="detection-policies-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>阻断策略</span>
          <el-button size="small" @click="loadPolicies">刷新</el-button>
        </div>
      </template>

      <el-table v-loading="policyLoading" :data="blockPolicies" border stripe>
        <el-table-column prop="mitre_id" label="MITRE" width="140" />
        <el-table-column prop="mitre_name" label="策略名称" min-width="220" />
        <el-table-column prop="action" label="阻断动作" width="140" />
        <el-table-column prop="updated_at" label="更新时间" width="180">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="启用" width="120" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="(v: boolean) => handleToggleEnabled(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column label="自动阻断" width="140" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.auto_block" @change="(v: boolean) => handleToggleAutoBlock(row.mitre_id, v)" />
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useDetectionStore } from '@/store/detection'

const store = useDetectionStore()

const blockPolicies = computed(() => store.blockPolicies)
const policyLoading = computed(() => store.policyLoading)

function formatTime(time: string) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

async function loadPolicies() {
  await store.fetchBlockPolicies()
}

async function handleToggleEnabled(mitreId: string, enabled: boolean) {
  await store.updateBlockPolicy(mitreId, { enabled })
  ElMessage.success('策略启用状态已更新')
}

async function handleToggleAutoBlock(mitreId: string, autoBlock: boolean) {
  await store.updateBlockPolicy(mitreId, { auto_block: autoBlock })
  ElMessage.success('自动阻断状态已更新')
}

onMounted(() => {
  loadPolicies()
})
</script>

<style scoped>
.detection-policies-page {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

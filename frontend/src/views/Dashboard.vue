<template>
  <div class="dashboard">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>主机列表</span>
          <el-button @click="refresh">刷新</el-button>
        </div>
      </template>
      
      <el-table :data="hosts" v-loading="loading" style="width: 100%">
        <el-table-column prop="ip_address" label="IP 地址" />
        <el-table-column prop="hostname" label="主机名" />
        <el-table-column prop="os_type" label="系统类型" />
        <el-table-column prop="agent_version" label="Agent 版本" />
        <el-table-column prop="last_heartbeat_at" label="最后心跳" />
        <el-table-column label="状态">
          <template #default="{ row }">
            <el-tag :type="row.online ? 'success' : 'danger'">
              {{ row.online ? '在线' : '离线' }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useHostStore } from '@/store/hosts'

const hostStore = useHostStore()
const hosts = ref([])
const loading = ref(false)

const refresh = () => {
  hostStore.fetchHosts()
}

onMounted(() => {
  refresh()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
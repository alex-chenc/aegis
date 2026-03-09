<template>
  <div class="dashboard">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>主机列表</span>
          <el-button @click="refresh" :loading="loading">刷新</el-button>
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
      
      <el-empty v-if="!loading && hosts.length === 0" description="暂无数据" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useHostStore } from '@/store/hosts'

const hostStore = useHostStore()
const { hosts, loading } = storeToRefs(hostStore)

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
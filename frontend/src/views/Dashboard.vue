<template>
  <div class="dashboard page-shell">
    <section class="page-hero dashboard-hero">
      <h1>主机资产态势</h1>
      <p>集中查看 Agent 在线状态、主机身份和最后心跳，快速定位失联节点与需要处置的资产。</p>
    </section>

    <div class="metric-grid">
      <div class="metric-card">
        <div class="metric-label">总主机</div>
        <div class="metric-value">{{ hosts.length }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">在线 Agent</div>
        <div class="metric-value">{{ onlineCount }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">离线节点</div>
        <div class="metric-value">{{ offlineCount }}</div>
      </div>
    </div>

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
            <span class="status-pill" :class="row.online ? 'status-online' : 'status-offline'">
              {{ row.online ? '在线' : '离线' }}
            </span>
          </template>
        </el-table-column>
      </el-table>
      
      <el-empty v-if="!loading && hosts.length === 0" description="暂无数据" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useHostStore } from '@/store/hosts'

const hostStore = useHostStore()
const { hosts, loading } = storeToRefs(hostStore)
const onlineCount = computed(() => hosts.value.filter((host: any) => host.online).length)
const offlineCount = computed(() => Math.max(hosts.value.length - onlineCount.value, 0))

const refresh = () => {
  hostStore.fetchHosts()
}

onMounted(() => {
  refresh()
})
</script>

<style scoped>
.dashboard-hero {
  margin-bottom: 0;
}

.status-online {
  color: #047857;
  background: rgba(16, 185, 129, 0.1);
}

.status-offline {
  color: #be123c;
  background: rgba(225, 29, 72, 0.1);
}
</style>

<template>
  <div class="dashboard-page">
    <el-alert
      v-if="wsDisconnected"
      title="实时更新已断开，正在尝试重连..."
      type="warning"
      show-icon
      class="mb-20"
      :closable="false"
    />

    <el-card class="box-card">
      <template #header>
        <div class="card-header">
          <span>资产展示大盘</span>
          <el-input
            v-model="searchQuery"
            placeholder="搜索主机名 / IP"
            class="search-input"
            prefix-icon="Search"
            @input="handleSearch"
            clearable
          />
        </div>
      </template>

      <div v-if="hostStore.error" class="error-state">
        <el-alert :title="hostStore.error" type="error" show-icon />
        <el-button type="primary" @click="fetchData" class="mt-20">重试</el-button>
      </div>

      <template v-else>
        <el-skeleton :rows="5" animated v-if="hostStore.isLoading && !hostStore.hosts.length" />
        
        <el-empty v-else-if="hostStore.hosts.length === 0" description="暂无资产数据，请先部署 Agent" />

        <el-table
          v-else
          :data="hostStore.hosts"
          style="width: 100%"
          @row-click="handleRowClick"
          v-loading="hostStore.isLoading"
          class="host-table"
        >
          <el-table-column prop="hostname" label="主机名" min-width="150" />
          <el-table-column prop="ip_address" label="IP 地址" width="140" />
          <el-table-column label="在线状态" width="100">
            <template #default="scope">
              <div class="status-cell">
                <span class="status-dot" :class="scope.row.is_online ? 'online' : 'offline'"></span>
                {{ scope.row.is_online ? '在线' : '离线' }}
              </div>
            </template>
          </el-table-column>
          <el-table-column label="操作系统" min-width="180">
            <template #default="scope">
              {{ scope.row.os_type }} {{ scope.row.os_version }}
            </template>
          </el-table-column>
          <el-table-column label="CPU/内存" width="120">
            <template #default="scope">
              <div class="metrics">
                <span>CPU: {{ scope.row.cpu_load_1min?.toFixed(1) || '-' }}%</span>
                <span>内存: {{ scope.row.mem_usage_percent?.toFixed(1) || '-' }}%</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="agent_version" label="Agent版本" width="100" />
          <el-table-column label="最后心跳" width="160">
            <template #default="scope">
              {{ formatTime(scope.row.last_heartbeat_at) }}
            </template>
          </el-table-column>
        </el-table>

        <div class="pagination-container mt-20">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            layout="total, sizes, prev, pager, next, jumper"
            :total="hostStore.total"
            @size-change="handleSizeChange"
            @current-change="handleCurrentChange"
          />
        </div>
      </template>
    </el-card>

    <el-drawer
      v-model="drawerVisible"
      title="主机详情"
      direction="rtl"
      size="45%"
    >
      <div v-if="detailLoading" class="p-20">
        <el-skeleton :rows="6" animated />
      </div>
      <div v-else-if="detailError" class="p-20 error-state">
        <el-alert title="获取详细信息失败" type="error" show-icon />
        <el-button type="primary" @click="fetchDetailData" class="mt-20">重试</el-button>
      </div>
      <div v-else-if="selectedHost" class="host-details">
        <el-descriptions title="基本信息" :column="2" border>
          <el-descriptions-item label="主机名">{{ selectedHost.hostname }}</el-descriptions-item>
          <el-descriptions-item label="IP 地址">{{ selectedHost.ip_address }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="selectedHost.is_online ? 'success' : 'info'">
              {{ selectedHost.is_online ? '在线' : '离线' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Agent 版本">{{ selectedHost.agent_version }}</el-descriptions-item>
          <el-descriptions-item label="操作系统">{{ selectedHost.os_type }} {{ selectedHost.os_version }}</el-descriptions-item>
          <el-descriptions-item label="内核版本">{{ selectedHost.kernel_version || '-' }}</el-descriptions-item>
          <el-descriptions-item label="架构">{{ selectedHost.architecture }}</el-descriptions-item>
          <el-descriptions-item label="总内存">{{ selectedHost.total_memory_mb }} MB</el-descriptions-item>
        </el-descriptions>
        
        <el-divider />
        
        <el-descriptions title="实时资源" :column="1" border>
          <el-descriptions-item label="CPU 负载">
            <el-progress 
              :percentage="Math.min(100, Math.round(selectedHost.cpu_load_1min || 0))" 
              :color="progressColor" 
            />
          </el-descriptions-item>
          <el-descriptions-item label="内存使用率">
            <el-progress 
              :percentage="Math.min(100, Math.round(selectedHost.mem_usage_percent || 0))" 
              :color="progressColor" 
            />
          </el-descriptions-item>
        </el-descriptions>

        <el-divider />

        <el-descriptions title="CPU 信息" :column="2" border v-if="selectedHost.cpu_info">
          <el-descriptions-item label="型号">{{ selectedHost.cpu_info?.model_name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="核心数">{{ selectedHost.cpu_info?.cores || '-' }}</el-descriptions-item>
          <el-descriptions-item label="线程数">{{ selectedHost.cpu_info?.threads || '-' }}</el-descriptions-item>
          <el-descriptions-item label="频率">{{ selectedHost.cpu_info?.frequency || '-' }} MHz</el-descriptions-item>
        </el-descriptions>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import { useHostStore } from '@/store/hosts';
import type { Host } from '@/types';
import { Search } from '@element-plus/icons-vue';

const hostStore = useHostStore();

const searchQuery = ref('');
const currentPage = ref(1);
const pageSize = ref(10);
let searchTimeout: ReturnType<typeof setTimeout> | null = null;

const wsDisconnected = ref(false);

const drawerVisible = ref(false);
const detailLoading = ref(false);
const detailError = ref(false);
const selectedHost = ref<Host | null>(null);

const fetchData = async () => {
  await hostStore.fetchHosts(currentPage.value, pageSize.value, searchQuery.value);
};

const handleSearch = () => {
  if (searchTimeout) clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    currentPage.value = 1;
    fetchData();
  }, 500);
};

const handleSizeChange = (val: number) => {
  pageSize.value = val;
  fetchData();
};

const handleCurrentChange = (val: number) => {
  currentPage.value = val;
  fetchData();
};

const formatTime = (isoString?: string) => {
  if (!isoString) return '-';
  const date = new Date(isoString);
  return date.toLocaleString();
};

const handleRowClick = (row: Host) => {
  selectedHost.value = row;
  drawerVisible.value = true;
  fetchDetailData();
};

const fetchDetailData = () => {
  detailLoading.value = true;
  detailError.value = false;
  setTimeout(() => {
    if (Math.random() < 0.1) {
      detailError.value = true;
    }
    detailLoading.value = false;
  }, 300);
};

const progressColor = (percentage: number) => {
  if (percentage < 50) return '#67c23a';
  if (percentage < 80) return '#e6a23c';
  return '#f56c6c';
};

onMounted(() => {
  fetchData();
});

onUnmounted(() => {
  if (searchTimeout) clearTimeout(searchTimeout);
});
</script>

<style scoped>
.mb-20 {
  margin-bottom: 20px;
}
.mt-20 {
  margin-top: 20px;
}
.p-20 {
  padding: 20px;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.search-input {
  width: 300px;
}
.status-cell {
  display: flex;
  align-items: center;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 8px;
  display: inline-block;
}
.status-dot.online {
  background-color: #67c23a;
  box-shadow: 0 0 4px #67c23a;
}
.status-dot.offline {
  background-color: #909399;
}
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px 0;
}
.pagination-container {
  display: flex;
  justify-content: flex-end;
}
.host-table {
  cursor: pointer;
}
.host-details {
  padding: 20px;
}
.metrics {
  display: flex;
  flex-direction: column;
  font-size: 12px;
  color: #606266;
  gap: 2px;
}
</style>
<template>
  <div class="software-page">
    <!-- 筛选区 -->
    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="filters.keyword"
          placeholder="搜索软件名、版本、主机名、IP"
          clearable
          style="width: 300px"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select
          v-model="filters.package_manager"
          placeholder="包管理器"
          clearable
          style="width: 150px"
        >
          <el-option label="RPM" value="rpm" />
          <el-option label="DPKG" value="dpkg" />
          <el-option label="APK" value="apk" />
        </el-select>

        <el-select
          v-model="filters.os_type"
          placeholder="操作系统"
          clearable
          style="width: 150px"
        >
          <el-option label="Linux" value="linux" />
          <el-option label="Windows" value="windows" />
        </el-select>

        <el-button type="primary" @click="handleSearch">
          <el-icon><Search /></el-icon>
          查询
        </el-button>

        <el-button @click="handleReset">
          <el-icon><RefreshRight /></el-icon>
          重置
        </el-button>
      </div>
    </el-card>

    <!-- 数据表格 -->
    <el-card>
      <template #header>
        <div class="card-header">
          <span>软件清单</span>
          <div class="header-actions">
            <el-button type="success" @click="handleExport">
              <el-icon><Download /></el-icon>
              导出 CSV
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="softwareAssets"
        v-loading="loading"
        border
        stripe
        style="width: 100%"
      >
        <el-table-column prop="hostname" label="主机名称" width="200">
          <template #default="{ row }">
            <div>
              <div class="hostname">{{ row.hostname }}</div>
              <div class="host-id">{{ row.host_id }}</div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="ip_address" label="IP 地址" width="140" />

        <el-table-column prop="group_name" label="分组名称" width="120" />

        <el-table-column prop="os_type" label="操作系统" width="100">
          <template #default="{ row }">
            <el-tag size="small">{{ row.os_type }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="name" label="软件名称" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="software-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="version" label="安装版本" width="150">
          <template #default="{ row }">
            <span>{{ row.version || 'unknown' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="package_manager" label="包管理器" width="100">
          <template #default="{ row }">
            <el-tag :type="getPackageManagerType(row.package_manager)" size="small">
              {{ row.package_manager }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="install_paths" label="安装路径" min-width="200">
          <template #default="{ row }">
            <div v-if="row.install_paths && row.install_paths.length > 0">
              <div>{{ row.install_paths[0] }}</div>
              <el-popover
                v-if="row.install_paths.length > 1"
                trigger="hover"
                width="400"
              >
                <template #reference>
                  <el-link type="primary" size="small">
                    +{{ row.install_paths.length - 1 }} 更多
                  </el-link>
                </template>
                <div class="path-list">
                  <div v-for="(path, index) in row.install_paths" :key="index" class="path-item">
                    {{ path }}
                  </div>
                </div>
              </el-popover>
            </div>
            <span v-else class="no-data">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="last_modified_at" label="最后更新时间" width="180">
          <template #default="{ row }">
            {{ row.last_modified_at ? formatTime(row.last_modified_at) : '-' }}
          </template>
        </el-table-column>

        <el-table-column prop="collected_at" label="记录时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.collected_at) }}
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="filters.page"
          v-model:page-size="filters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          :total="softwareTotal"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { Search, RefreshRight, Download } from '@element-plus/icons-vue'
import { useAssetStore } from '@/store/assets'
import { storeToRefs } from 'pinia'

const assetStore = useAssetStore()
const {
  softwareAssets,
  softwareTotal,
  loading,
  softwareFilters: filters,
} = storeToRefs(assetStore)

// 初始化
onMounted(() => {
  assetStore.fetchSoftwareAssets()
})

// 搜索
function handleSearch() {
  filters.value.page = 1
  assetStore.fetchSoftwareAssets()
}

// 重置筛选
function handleReset() {
  assetStore.resetSoftwareFilters()
  assetStore.fetchSoftwareAssets()
}

// 分页处理
function handleSizeChange() {
  filters.value.page = 1
  assetStore.fetchSoftwareAssets()
}

function handlePageChange() {
  assetStore.fetchSoftwareAssets()
}

// 导出 CSV
function handleExport() {
  // TODO: 实现 CSV 导出
}

// 格式化时间
function formatTime(time: string) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

// 获取包管理器类型
function getPackageManagerType(pm: string) {
  const types: Record<string, string> = {
    rpm: 'primary',
    dpkg: 'success',
    apk: 'warning',
  }
  return types[pm] || 'info'
}
</script>

<style scoped>
.software-page {
  padding: 20px;
}

.filter-card {
  margin-bottom: 20px;
}

.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.hostname {
  font-weight: 600;
  color: #303133;
}

.host-id {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.software-name {
  font-weight: 500;
}

.no-data {
  color: #C0C4CC;
}

.path-list {
  max-height: 300px;
  overflow-y: auto;
}

.path-item {
  padding: 4px 0;
  border-bottom: 1px solid #EBEEF5;
  font-size: 12px;
  word-break: break-all;
}

.path-item:last-child {
  border-bottom: none;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>

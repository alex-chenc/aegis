<template>
  <div class="detection-packages-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>动态检测包管理</span>
          <div>
            <el-button type="primary" @click="$router.push('/detection/packages/new')">
              <el-icon><Plus /></el-icon>新建检测包
            </el-button>
          </div>
        </div>
      </template>

      <div class="filter-bar">
        <el-input
          v-model="searchText"
          placeholder="搜索 Package ID / 标题 / CVE"
          clearable
          style="width: 300px;"
          @clear="handleSearch"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-select v-model="statusFilter" placeholder="状态筛选" clearable style="width: 150px; margin-left: 12px;" @change="handleSearch">
          <el-option label="草稿" value="draft" />
          <el-option label="已构建" value="built" />
          <el-option label="已签名" value="signed" />
          <el-option label="已启用" value="enabled" />
          <el-option label="运行中" value="active" />
          <el-option label="已禁用" value="disabled" />
        </el-select>
        <el-button
          v-if="selectedPackages.length > 0"
          type="danger"
          style="margin-left: 12px;"
          @click="batchDeleteDialogVisible = true"
        >
          批量删除 ({{ selectedPackages.length }})
        </el-button>
      </div>

      <el-table
        :data="packages"
        v-loading="loading"
        border
        stripe
        style="margin-top: 16px;"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="50" />
        <el-table-column prop="package_id" label="Package ID" min-width="200" show-overflow-tooltip />
        <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip />
        <el-table-column prop="version" label="版本" width="100" />
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <PackageStatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="CVE" min-width="150">
          <template #default="{ row }">
            <el-tag v-for="cve in (row.cve_ids || [])" :key="cve" size="small" style="margin: 2px;">{{ cve }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Hooks" width="80" align="center">
          <template #default="{ row }">
            {{ (row.hook_summary || []).length }}
          </template>
        </el-table-column>
        <el-table-column label="安装率" width="120">
          <template #default="{ row }">
            <span v-if="row.host_total">
              {{ row.host_active || 0 }}/{{ row.host_total }}
            </span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="170">
          <template #default="{ row }">
            {{ new Date(row.updated_at).toLocaleString() }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="$router.push(`/detection/packages/${row.package_id}`)">详情</el-button>
            <el-button
              v-if="row.status === 'signed'"
              link type="success" size="small"
              @click="handleEnable(row)"
            >启用</el-button>
            <el-button
              v-if="['enabled', 'active'].includes(row.status)"
              link type="warning" size="small"
              @click="handleDisable(row)"
            >禁用</el-button>
            <el-button
              v-if="['enabled', 'active', 'disabled'].includes(row.status)"
              link type="danger" size="small"
              @click="handleUninstall(row)"
            >卸载</el-button>
            <el-button
              v-if="!['enabled', 'active'].includes(row.status)"
              link type="danger" size="small"
              @click="handleDelete(row)"
            >删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-if="total > pageSize"
        :current-page="currentPage"
        :page-size="pageSize"
        :total="total"
        layout="total, prev, pager, next"
        style="margin-top: 16px; justify-content: flex-end;"
        @current-change="handlePageChange"
      />
    </el-card>

    <el-dialog v-model="enableDialogVisible" title="确认启用" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          该 DetectionPackage 将下发到全部 agent。离线 agent 上线后也会收到安装指令。
        </template>
      </el-alert>
      <el-descriptions :column="1" border size="small" style="margin-top: 16px;">
        <el-descriptions-item label="Package ID">{{ selectedPackage?.package_id }}</el-descriptions-item>
        <el-descriptions-item label="版本">{{ selectedPackage?.version }}</el-descriptions-item>
        <el-descriptions-item label="Hooks">{{ (selectedPackage?.hook_summary || []).length }} 个</el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="enableDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="loading" @click="confirmEnable">确认启用</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="disableDialogVisible" title="确认禁用" width="400">
      <el-text>确定要禁用该检测包吗？所有 agent 将停止该包的检测。</el-text>
      <template #footer>
        <el-button @click="disableDialogVisible = false">取消</el-button>
        <el-button type="warning" :loading="loading" @click="confirmDisable">确认禁用</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="uninstallDialogVisible" title="确认卸载" width="400">
      <el-text type="danger">确定要卸载该检测包吗？所有 agent 将删除该包的本地文件。</el-text>
      <template #footer>
        <el-button @click="uninstallDialogVisible = false">取消</el-button>
        <el-button type="danger" :loading="loading" @click="confirmUninstall">确认卸载</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deleteDialogVisible" title="确认删除" width="400">
      <el-text type="danger">确定要删除检测包 "{{ selectedPackage?.package_id }}" 吗？此操作不可恢复。</el-text>
      <template #footer>
        <el-button @click="deleteDialogVisible = false">取消</el-button>
        <el-button type="danger" :loading="loading" @click="confirmDelete">确认删除</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchDeleteDialogVisible" title="批量删除" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          确定要删除以下 {{ selectedPackages.length }} 个检测包吗？此操作不可恢复。
        </template>
      </el-alert>
      <div style="margin-top: 12px; max-height: 200px; overflow-y: auto;">
        <el-tag v-for="pkg in selectedPackages" :key="pkg.package_id" style="margin: 2px;">{{ pkg.package_id }}</el-tag>
      </div>
      <template #footer>
        <el-button @click="batchDeleteDialogVisible = false">取消</el-button>
        <el-button type="danger" :loading="loading" @click="confirmBatchDelete">确认删除</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus, Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useDetectionPackages } from './composables/useDetectionPackages'
import { detectionPackageApi } from '@/api/detection-packages'
import type { DetectionPackage } from '@/api/detection-packages'
import PackageStatusTag from './components/PackageStatusTag.vue'

const {
  packages, total, loading,
  fetchPackages, enablePackage, disablePackage, uninstallPackage,
} = useDetectionPackages()

const currentPage = ref(1)
const pageSize = ref(20)
const searchText = ref('')
const statusFilter = ref('')

const enableDialogVisible = ref(false)
const disableDialogVisible = ref(false)
const uninstallDialogVisible = ref(false)
const deleteDialogVisible = ref(false)
const batchDeleteDialogVisible = ref(false)
const selectedPackage = ref<DetectionPackage | null>(null)
const selectedPackages = ref<DetectionPackage[]>([])

function handleSearch() {
  currentPage.value = 1
  loadPackages()
}

function handlePageChange(page: number) {
  currentPage.value = page
  loadPackages()
}

function loadPackages() {
  fetchPackages({
    page: currentPage.value,
    page_size: pageSize.value,
    search: searchText.value || undefined,
    status: statusFilter.value || undefined,
  })
}

function handleSelectionChange(selection: DetectionPackage[]) {
  selectedPackages.value = selection
}

function handleEnable(pkg: DetectionPackage) {
  selectedPackage.value = pkg
  enableDialogVisible.value = true
}

function handleDisable(pkg: DetectionPackage) {
  selectedPackage.value = pkg
  disableDialogVisible.value = true
}

function handleUninstall(pkg: DetectionPackage) {
  selectedPackage.value = pkg
  uninstallDialogVisible.value = true
}

function handleDelete(pkg: DetectionPackage) {
  selectedPackage.value = pkg
  deleteDialogVisible.value = true
}

async function confirmEnable() {
  if (!selectedPackage.value) return
  await enablePackage(selectedPackage.value.package_id)
  enableDialogVisible.value = false
  loadPackages()
}

async function confirmDisable() {
  if (!selectedPackage.value) return
  await disablePackage(selectedPackage.value.package_id)
  disableDialogVisible.value = false
  loadPackages()
}

async function confirmUninstall() {
  if (!selectedPackage.value) return
  await uninstallPackage(selectedPackage.value.package_id)
  uninstallDialogVisible.value = false
  loadPackages()
}

async function confirmDelete() {
  if (!selectedPackage.value) return
  try {
    await detectionPackageApi.deletePackage(selectedPackage.value.package_id)
    ElMessage.success('删除成功')
    deleteDialogVisible.value = false
    loadPackages()
  } catch (e: any) {
    ElMessage.error(e.message || '删除失败')
  }
}

async function confirmBatchDelete() {
  if (selectedPackages.value.length === 0) return
  try {
    await detectionPackageApi.batchDeletePackages(selectedPackages.value.map(p => p.package_id))
    ElMessage.success(`成功删除 ${selectedPackages.value.length} 个检测包`)
    batchDeleteDialogVisible.value = false
    selectedPackages.value = []
    loadPackages()
  } catch (e: any) {
    ElMessage.error(e.message || '批量删除失败')
  }
}

onMounted(() => {
  loadPackages()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.filter-bar {
  display: flex;
  align-items: center;
}
</style>

<template>
  <div class="detection-packages-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.detectionDetectionPackagesIndex_dynamic_detection_package_management_4561f4') }}</span>
          <div>
            <el-button type="primary" @click="$router.push('/detection/packages/new')">
              <el-icon><Plus /></el-icon>{{ $t('generated.detectionDetectionPackagesIndex_create_a_new_detection_package_6f3c86') }}
            </el-button>
          </div>
        </div>
      </template>

      <div class="filter-bar">
        <el-input
          v-model="searchText"
          :placeholder="$t('generated.detectionDetectionPackagesIndex_search_package_id_title_cve_ee4a04')"
          clearable
          style="width: 300px;"
          @clear="handleSearch"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-select v-model="statusFilter" :placeholder="$t('generated.detectionDetectionPackagesIndex_status_filter_f0b213')" clearable style="width: 150px; margin-left: 12px;" @change="handleSearch">
          <el-option :label="$t('generated.detectionDetectionPackagesIndex_draft_0f4368')" value="draft" />
          <el-option :label="$t('generated.detectionDetectionPackagesIndex_build_failed_6518bf')" value="build_failed" />
          <el-option :label="$t('generated.common_pending_review_f53b68')" value="awaiting_review" />
          <el-option :label="$t('generated.common_review_rejection_8942f5')" value="review_rejected" />
          <el-option :label="$t('generated.detectionDetectionPackagesIndex_built_2061e6')" value="built" />
          <el-option :label="$t('generated.detectionDetectionPackagesIndex_signed_d7dfc4')" value="signed" />
          <el-option :label="$t('generated.detectionDetectionPackagesIndex_enabled_25d284')" value="enabled" />
          <el-option :label="$t('generated.detectionDetectionPackagesIndex_running_594249')" value="active" />
          <el-option :label="$t('generated.common_disabled_0fe5a9')" value="disabled" />
        </el-select>
        <el-button
          v-if="selectedPackages.length > 0"
          type="danger"
          style="margin-left: 12px;"
          @click="batchDeleteDialogVisible = true"
        >
          {{ $t('generated.common_batch_delete_4edb06') }}{{ selectedPackages.length }})
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
        <el-table-column prop="package_id" label="Package ID" min-width="200">
          <template #default="{ row }">
            <el-tooltip :content="row.package_id" placement="top" :show-after="300">
              <span class="package-id-cell">{{ row.package_id }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="title" :label="$t('generated.common_title_748d7d')" min-width="200" show-overflow-tooltip />
        <el-table-column prop="version" :label="$t('generated.common_version_989d1a')" width="100" />
        <el-table-column :label="$t('generated.common_state_62e951')" width="120">
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
        <el-table-column :label="$t('generated.detectionDetectionPackagesIndex_install_rate_fd1160')" width="120">
          <template #default="{ row }">
            <span v-if="row.host_total">
              {{ row.host_active || 0 }}/{{ row.host_total }}
            </span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_update_time_093dea')" width="170">
          <template #default="{ row }">
            {{ formatDateTime(row.updated_at) }}
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="$router.push(`/detection/packages/${row.package_id}`)">{{ $t('generated.common_details_4f55ee') }}</el-button>
            <el-button
              v-if="row.status === 'draft'"
              link type="primary" size="small"
              @click="$router.push(`/detection/packages/${row.package_id}?tab=build`)"
            >{{ $t('generated.detectionDetectionPackagesIndex_build_81a94d') }}</el-button>
            <el-button
              v-if="row.status === 'awaiting_review' || buildStatusMap[row.package_id]?.status === 'awaiting_review'"
              link type="warning" size="small"
              @click="$router.push(`/detection/packages/${row.package_id}?tab=build`)"
            >{{ $t('generated.detectionDetectionPackagesIndex_review_fe945e') }}</el-button>
            <el-button
              v-if="row.status === 'built' || buildStatusMap[row.package_id]?.status === 'success'"
              link type="warning" size="small"
              @click="$router.push(`/detection/packages/${row.package_id}?tab=build`)"
            >{{ $t('generated.detectionDetectionPackagesIndex_sign_8ba46c') }}</el-button>
            <el-button
              v-if="['signed', 'disabled'].includes(row.status)"
              link type="success" size="small"
              @click="handleEnable(row)"
            >{{ $t('generated.common_enable_d4e9ca') }}</el-button>
            <el-button
              v-if="['enabled', 'active'].includes(row.status)"
              link type="warning" size="small"
              @click="handleDisable(row)"
            >{{ $t('generated.common_disable_be70be') }}</el-button>
            <el-button
              v-if="['enabled', 'active', 'disabled'].includes(row.status)"
              link type="danger" size="small"
              @click="handleUninstall(row)"
            >{{ $t('generated.common_uninstall_200cf1') }}</el-button>
            <el-button
              v-if="!['enabled', 'active'].includes(row.status)"
              link type="danger" size="small"
              @click="handleDelete(row)"
            >{{ $t('generated.common_delete_3755f5') }}</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <el-dialog v-model="enableDialogVisible" :title="$t('generated.common_confirm_enable_62d826')" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          {{ $t('generated.common_the_detectionpackage_will_be_delivered_to_bc5580') }}
        </template>
      </el-alert>
      <el-descriptions :column="1" border size="small" style="margin-top: 16px;">
        <el-descriptions-item label="Package ID">{{ selectedPackage?.package_id }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_version_989d1a')">{{ selectedPackage?.version }}</el-descriptions-item>
        <el-descriptions-item label="Hooks">{{ (selectedPackage?.hook_summary || []).length }} {{ $t('generated.common_indivual_f7b2a6') }}</el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="enableDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="primary" :loading="loading" @click="confirmEnable">{{ $t('generated.common_confirm_enable_62d826') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="disableDialogVisible" :title="$t('generated.common_confirm_disable_816c68')" width="400">
      <el-text>{{ $t('generated.detectionDetectionPackagesIndex_are_you_sure_you_want_to_819e7a') }}</el-text>
      <template #footer>
        <el-button @click="disableDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="warning" :loading="loading" @click="confirmDisable">{{ $t('generated.common_confirm_disable_816c68') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="uninstallDialogVisible" :title="$t('generated.common_confirm_uninstall_c7ff7b')" width="400">
      <el-text type="danger">{{ $t('generated.detectionDetectionPackagesIndex_are_you_sure_you_want_to_5d6446') }}</el-text>
      <template #footer>
        <el-button @click="uninstallDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="danger" :loading="loading" @click="confirmUninstall">{{ $t('generated.common_confirm_uninstall_c7ff7b') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deleteDialogVisible" :title="$t('generated.common_confirm_deletion_3c06ab')" width="400">
      <el-text type="danger">{{ $t('generated.detectionDetectionPackagesIndex_are_you_sure_you_want_to_6f9ac3') }}{{ selectedPackage?.package_id }}{{ $t('generated.detectionDetectionPackagesIndex_this_operation_is_irreversible_e0ce49') }}</el-text>
      <template #footer>
        <el-button @click="deleteDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="danger" :loading="loading" @click="confirmDelete">{{ $t('generated.common_confirm_deletion_3c06ab') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchDeleteDialogVisible" :title="$t('generated.common_batch_delete_362aed')" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          {{ $t('generated.detectionDetectionPackagesIndex_make_sure_you_want_to_delete_5c7d4b') }} {{ selectedPackages.length }} {{ $t('generated.detectionDetectionPackagesIndex_a_test_kit_this_operation_is_e86a00') }}
        </template>
      </el-alert>
      <div style="margin-top: 12px; max-height: 200px; overflow-y: auto;">
        <el-tag v-for="pkg in selectedPackages" :key="pkg.package_id" style="margin: 2px;">{{ pkg.package_id }}</el-tag>
      </div>
      <template #footer>
        <el-button @click="batchDeleteDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="danger" :loading="loading" @click="confirmBatchDelete">{{ $t('generated.common_confirm_deletion_3c06ab') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { ref, onMounted, reactive } from 'vue'
import { Plus, Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useDetectionPackages } from './composables/useDetectionPackages'
import { detectionPackageApi } from '@/api/detection-packages'
import type { DetectionPackage, DetectionPackageBuild } from '@/api/detection-packages'
import PackageStatusTag from './components/PackageStatusTag.vue'

const {
  packages, total, loading,
  fetchPackages, enablePackage, disablePackage, uninstallPackage,
} = useDetectionPackages()

const currentPage = ref(1)
const pageSize = ref(20)
const searchText = ref('')
const statusFilter = ref('')

// 构建状态缓存
const buildStatusMap = reactive<Record<string, DetectionPackageBuild | null>>({})

// 批量获取构建状态
async function fetchBuildStatuses(packages: DetectionPackage[]) {
  const statusesWithBuild = new Set(['draft', 'build_pending', 'build_running', 'build_failed', 'build_success', 'awaiting_review', 'review_rejected', 'built'])
  await Promise.all(packages.map(async (pkg) => {
    if (statusesWithBuild.has(pkg.status)) {
      try {
        const build = await detectionPackageApi.getLatestBuild(pkg.package_id)
        buildStatusMap[pkg.package_id] = build
      } catch {
        buildStatusMap[pkg.package_id] = null
      }
    }
  }))
}

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

function handleSizeChange() {
  currentPage.value = 1
  loadPackages()
}

function handlePageChange() {
  loadPackages()
}

async function loadPackages() {
  await fetchPackages({
    page: currentPage.value,
    page_size: pageSize.value,
    search: searchText.value || undefined,
    status: statusFilter.value || undefined,
  })
  // 获取构建状态
  await fetchBuildStatuses(packages.value)
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
    ElMessage.success(translate('generatedScript.common_delete_successfully_86e8d1'))
    deleteDialogVisible.value = false
    loadPackages()
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_delete_failed_72250c'))
  }
}

async function confirmBatchDelete() {
  if (selectedPackages.value.length === 0) return
  try {
    await detectionPackageApi.batchDeletePackages(selectedPackages.value.map(p => p.package_id))
    ElMessage.success(translate('generatedScript.detectionDetectionPackagesIndex_detection_packages_successfully_deleted_795cec', { p0: selectedPackages.value.length }))
    batchDeleteDialogVisible.value = false
    selectedPackages.value = []
    loadPackages()
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_batch_deletion_failed_b59edb'))
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
.pagination-container {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
.package-id-cell {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}
</style>

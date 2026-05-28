<template>
  <div class="package-detail-page">
    <el-card v-loading="loading">
      <template #header>
        <div class="card-header">
          <div>
            <el-button link @click="$router.push('/detection/packages')">
              <el-icon><ArrowLeft /></el-icon> 返回列表
            </el-button>
            <span style="margin-left: 12px; font-weight: bold;">{{ currentPackage?.title || currentPackage?.package_id }}</span>
            <PackageStatusTag :status="currentPackage?.status || ''" style="margin-left: 8px;" />
          </div>
          <div class="actions">
            <el-button
              v-if="currentPackage?.status === 'signed'"
              type="success"
              :loading="loading"
              :disabled="!canOperate('enable')"
              @click="enableDialogVisible = true"
            >启用</el-button>
            <el-button
              v-if="['enabled', 'active'].includes(currentPackage?.status || '')"
              type="warning"
              :loading="loading"
              :disabled="!canOperate('disable')"
              @click="disableDialogVisible = true"
            >禁用</el-button>
            <el-button
              v-if="['enabled', 'active', 'disabled'].includes(currentPackage?.status || '')"
              type="danger"
              :loading="loading"
              :disabled="!canOperate('uninstall')"
              @click="uninstallDialogVisible = true"
            >卸载</el-button>
            <el-button
              v-if="currentPackage?.status === 'built'"
              type="primary"
              :loading="loading"
              :disabled="!canOperate('sign')"
              @click="signDialogVisible = true"
            >签名发布</el-button>
            <el-button
              v-if="['draft', 'build_failed'].includes(currentPackage?.status || '')"
              type="primary"
              :loading="loading"
              @click="$router.push(`/detection/packages/${packageId}/edit`)"
            >编辑</el-button>
          </div>
        </div>
      </template>

      <el-tabs v-model="activeTab" @tab-change="handleTabChange">
        <el-tab-pane label="基础信息" name="info">
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="Package ID">{{ currentPackage?.package_id }}</el-descriptions-item>
            <el-descriptions-item label="版本">{{ currentPackage?.version }}</el-descriptions-item>
            <el-descriptions-item label="状态">
              <PackageStatusTag :status="currentPackage?.status || ''" />
            </el-descriptions-item>
            <el-descriptions-item label="CVE">
              <el-tag v-for="cve in (currentPackage?.cve_ids || [])" :key="cve" size="small" style="margin: 2px;">{{ cve }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="描述" :span="2">{{ currentPackage?.description || '-' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ currentPackage?.created_at ? new Date(currentPackage.created_at).toLocaleString() : '-' }}</el-descriptions-item>
            <el-descriptions-item label="更新时间">{{ currentPackage?.updated_at ? new Date(currentPackage.updated_at).toLocaleString() : '-' }}</el-descriptions-item>
          </el-descriptions>
        </el-tab-pane>

        <el-tab-pane label="构建审核" name="build">
          <div v-if="!currentBuild && currentPackage?.status === 'draft'" style="text-align: center; padding: 40px;">
            <el-text type="info">草稿状态，尚未构建</el-text>
            <div style="margin-top: 16px;">
              <el-button type="primary" :loading="loading" :disabled="!canOperate('build')" @click="handleBuild">提交构建</el-button>
            </div>
          </div>
          <template v-else>
            <BuildReviewPanel :build="currentBuild" />
            <div v-if="currentBuild?.status === 'failed'" style="margin-top: 16px; text-align: center;">
              <el-button type="primary" :loading="loading" :disabled="!canOperate('build')" @click="handleBuild">重新构建</el-button>
            </div>
          </template>
        </el-tab-pane>

        <el-tab-pane label="主机状态" name="hosts">
          <HostStatusTable
            :hosts="hostStatuses"
            :total="hostTotal"
            @page-change="handleHostPageChange"
          />
        </el-tab-pane>

        <el-tab-pane label="Event Schema" name="schema">
          <pre class="schema-json">{{ JSON.stringify(currentBuild?.event_schema || currentPackage?.event_schema || {}, null, 2) }}</pre>
        </el-tab-pane>

        <el-tab-pane label="告警证据" name="alerts">
          <div v-loading="alertsLoading">
            <el-empty v-if="!alertsLoading && alerts.length === 0" description="暂无告警数据" />
            <el-collapse v-else>
              <el-collapse-item
                v-for="(alert, index) in alerts"
                :key="index"
                :title="alert.title || `告警 ${index + 1}`"
              >
                <EvidenceTimeline :evidence="alert.evidence || []" />
              </el-collapse-item>
            </el-collapse>
          </div>
        </el-tab-pane>

        <el-tab-pane label="版本历史" name="versions">
          <el-table :data="versionHistory" border size="small">
            <el-table-column prop="version" label="版本号" />
            <el-table-column prop="status" label="状态">
              <template #default="{ row }">
                <PackageStatusTag :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column prop="build_time" label="构建时间">
              <template #default="{ row }">{{ row.build_time ? new Date(row.build_time).toLocaleString() : '-' }}</template>
            </el-table-column>
            <el-table-column prop="sign_time" label="签名时间">
              <template #default="{ row }">{{ row.sign_time && row.sign_time !== '-' ? new Date(row.sign_time).toLocaleString() : '-' }}</template>
            </el-table-column>
            <el-table-column prop="enable_time" label="启用时间">
              <template #default="{ row }">{{ row.enable_time && row.enable_time !== '-' ? new Date(row.enable_time).toLocaleString() : '-' }}</template>
            </el-table-column>
            <el-table-column prop="operator" label="操作人" />
            <el-table-column label="操作" width="160">
              <template #default="{ row }">
                <el-button
                  v-if="canOperate('rollback')"
                  type="warning"
                  size="small"
                  link
                  @click="handleRollback(row)"
                >回滚到此版本</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog v-model="signDialogVisible" title="确认签名发布" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          签名发布后不可修改。请确认已审核构建结果。
        </template>
      </el-alert>
      <el-descriptions :column="1" border size="small" style="margin-top: 16px;">
        <el-descriptions-item label="Package ID">{{ currentPackage?.package_id }}</el-descriptions-item>
        <el-descriptions-item label="版本">{{ currentPackage?.version }}</el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="signDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="loading" @click="confirmSign">确认签名</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="enableDialogVisible" title="确认启用" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          该 DetectionPackage 将下发到全部 agent。离线 agent 上线后也会收到安装指令。
        </template>
      </el-alert>
      <template #footer>
        <el-button @click="enableDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="loading" @click="confirmEnable">确认启用</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="disableDialogVisible" title="确认禁用" width="400">
      <el-text>确定要禁用该检测包吗？</el-text>
      <template #footer>
        <el-button @click="disableDialogVisible = false">取消</el-button>
        <el-button type="warning" :loading="loading" @click="confirmDisable">确认禁用</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="uninstallDialogVisible" title="确认卸载" width="400">
      <el-text type="danger">确定要卸载该检测包吗？</el-text>
      <template #footer>
        <el-button @click="uninstallDialogVisible = false">取消</el-button>
        <el-button type="danger" :loading="loading" @click="confirmUninstall">确认卸载</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import { useRole } from '@/composables/useRole'
import { detectionPackageApi } from '@/api/detection-packages'
import { useDetectionPackages } from './composables/useDetectionPackages'
import PackageStatusTag from './components/PackageStatusTag.vue'
import BuildReviewPanel from './components/BuildReviewPanel.vue'
import HostStatusTable from './components/HostStatusTable.vue'
import EvidenceTimeline from './components/EvidenceTimeline.vue'

const route = useRoute()
const { canOperate } = useRole()
const {
  currentPackage, currentBuild, hostStatuses, hostTotal, loading,
  fetchPackage, fetchBuild, fetchLatestBuild, startBuild, signPackage,
  enablePackage, disablePackage, uninstallPackage, fetchHostStatus,
} = useDetectionPackages()

const packageId = ref(route.params.id as string)
const activeTab = ref('info')

const signDialogVisible = ref(false)
const enableDialogVisible = ref(false)
const disableDialogVisible = ref(false)
const uninstallDialogVisible = ref(false)

const alerts = ref<any[]>([])
const alertsLoading = ref(false)
const versionHistory = ref<any[]>([])

function handleTabChange(tab: string) {
  if (tab === 'alerts') {
    loadAlerts()
  }
}

async function loadAlerts() {
  alertsLoading.value = true
  try {
    const res = await detectionPackageApi.getPackageAlerts(packageId.value)
    alerts.value = res.data || []
  } finally {
    alertsLoading.value = false
  }
}

async function handleRollback(row: any) {
  try {
    await ElMessageBox.confirm(
      `确定要回滚到版本 ${row.version} 吗？`,
      '确认回滚',
      { confirmButtonText: '确认', cancelButtonText: '取消', type: 'warning' }
    )
    await detectionPackageApi.rollbackPackage(packageId.value, row.version)
    loadPackage()
  } catch {}
}

async function loadPackage() {
  await fetchPackage(packageId.value)
  if (currentPackage.value?.status !== 'draft') {
    fetchHostStatus(packageId.value)
  }
  // Load latest build for non-draft packages
  if (currentPackage.value && currentPackage.value.status !== 'draft') {
    await fetchLatestBuild(packageId.value)
  }
  if (currentPackage.value) {
    versionHistory.value = [{
      version: currentPackage.value.version,
      status: currentPackage.value.status,
      build_time: currentPackage.value.created_at,
      sign_time: currentPackage.value.status === 'signed' || currentPackage.value.status === 'enabled' || currentPackage.value.status === 'active' || currentPackage.value.status === 'disabled' ? currentPackage.value.updated_at : '-',
      enable_time: ['enabled', 'active', 'disabled'].includes(currentPackage.value.status) ? currentPackage.value.updated_at : '-',
      operator: '-',
    }]
  }
}

async function handleBuild() {
  const build = await startBuild(packageId.value)
  if (build) {
    pollBuildStatus(build.id)
  }
}

async function pollBuildStatus(buildId: string) {
  const poll = async () => {
    const build = await fetchBuild(buildId)
    if (build && ['build_pending', 'build_running'].includes(build.status)) {
      setTimeout(poll, 2000)
    } else {
      loadPackage()
    }
  }
  poll()
}

function handleHostPageChange(page: number) {
  fetchHostStatus(packageId.value, { page, page_size: 20 })
}

async function confirmSign() {
  await signPackage(packageId.value)
  signDialogVisible.value = false
  loadPackage()
}

async function confirmEnable() {
  await enablePackage(packageId.value)
  enableDialogVisible.value = false
  loadPackage()
}

async function confirmDisable() {
  await disablePackage(packageId.value)
  disableDialogVisible.value = false
  loadPackage()
}

async function confirmUninstall() {
  await uninstallPackage(packageId.value)
  uninstallDialogVisible.value = false
  loadPackage()
}

onMounted(() => {
  loadPackage()
})

watch(() => route.params.id, (newId) => {
  if (newId) {
    packageId.value = newId as string
    loadPackage()
  }
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.actions {
  display: flex;
  gap: 8px;
}
.schema-json {
  font-family: monospace;
  font-size: 13px;
  background: #f5f7fa;
  padding: 16px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>

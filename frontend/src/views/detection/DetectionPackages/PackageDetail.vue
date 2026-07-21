<template>
  <div class="package-detail-page">
    <el-card v-loading="loading">
      <template #header>
        <div class="card-header">
          <div>
            <el-button link @click="$router.push('/detection/packages')">
              <el-icon><ArrowLeft /></el-icon> {{ $t('generated.detectionDetectionPackagesPackageDetail_return_to_list_53505b') }}
            </el-button>
            <span style="margin-left: 12px; font-weight: bold;">{{ currentPackage?.title || currentPackage?.package_id }}</span>
            <PackageStatusTag :status="currentPackage?.status || ''" style="margin-left: 8px;" />
          </div>
          <div class="actions">
            <el-button
              v-if="['signed', 'disabled'].includes(currentPackage?.status || '')"
              type="success"
              :loading="loading"
              :disabled="!canOperate('enable')"
              @click="enableDialogVisible = true"
            >{{ $t('generated.common_enable_d4e9ca') }}</el-button>
            <el-button
              v-if="['enabled', 'active'].includes(currentPackage?.status || '')"
              type="warning"
              :loading="loading"
              :disabled="!canOperate('disable')"
              @click="disableDialogVisible = true"
            >{{ $t('generated.common_disable_be70be') }}</el-button>
            <el-button
              v-if="['enabled', 'active', 'disabled'].includes(currentPackage?.status || '')"
              type="danger"
              :loading="loading"
              :disabled="!canOperate('uninstall')"
              @click="uninstallDialogVisible = true"
            >{{ $t('generated.common_uninstall_200cf1') }}</el-button>
            <el-button
              v-if="currentPackage?.status === 'built'"
              type="primary"
              :loading="loading"
              :disabled="!canOperate('sign')"
              @click="signDialogVisible = true"
            >{{ $t('generated.common_signature_release_bae7c7') }}</el-button>
            <el-button
              v-if="['draft', 'build_failed', 'review_rejected'].includes(currentPackage?.status || '')"
              type="primary"
              :loading="loading"
              @click="$router.push(`/detection/packages/${packageId}/edit`)"
            >{{ $t('generated.common_edit_a7f814') }}</el-button>
            <el-button
              :loading="loading"
              @click="activeTab = 'raw'; loadRawDraft()"
            >{{ $t('generated.detectionDetectionPackagesPackageDetail_original_information_45b901') }}</el-button>
          </div>
        </div>
      </template>

      <el-tabs v-model="activeTab" @tab-change="handleTabChange">
        <el-tab-pane :label="$t('generated.common_basic_information_41654e')" name="info">
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="Package ID">{{ currentPackage?.package_id }}</el-descriptions-item>
            <el-descriptions-item :label="$t('generated.common_version_989d1a')">{{ currentPackage?.version }}</el-descriptions-item>
            <el-descriptions-item :label="$t('generated.common_state_62e951')">
              <PackageStatusTag :status="currentPackage?.status || ''" />
            </el-descriptions-item>
            <el-descriptions-item label="CVE">
              <el-tag v-for="cve in (currentPackage?.cve_ids || [])" :key="cve" size="small" style="margin: 2px;">{{ cve }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item :label="$t('generated.common_describe_412f54')" :span="2">{{ currentPackage?.description || '-' }}</el-descriptions-item>
            <el-descriptions-item :label="$t('generated.common_creation_time_84e380')">{{ currentPackage?.created_at ? formatDateTime(currentPackage.created_at) : '-' }}</el-descriptions-item>
            <el-descriptions-item :label="$t('generated.common_update_time_093dea')">{{ currentPackage?.updated_at ? formatDateTime(currentPackage.updated_at) : '-' }}</el-descriptions-item>
          </el-descriptions>
        </el-tab-pane>

        <el-tab-pane :label="$t('generated.common_build_review_7ebabd')" name="build">
          <div v-if="!currentBuild && currentPackage?.status === 'draft'" style="text-align: center; padding: 40px;">
            <el-text type="info">{{ $t('generated.detectionDetectionPackagesPackageDetail_draft_status_not_yet_built_87fd4c') }}</el-text>
            <div style="margin-top: 16px;">
              <el-button type="primary" :loading="loading" :disabled="!canOperate('build')" @click="handleBuild">{{ $t('generated.detectionDetectionPackagesPackageDetail_submit_build_77f2f5') }}</el-button>
            </div>
          </div>
          <template v-else>
            <BuildReviewPanel
              :build="currentBuild"
              @sign="signDialogVisible = true"
              @reviewed="loadPackage"
            />
            <div v-if="['failed', 'review_rejected'].includes(currentBuild?.status || '')" style="margin-top: 16px; text-align: center;">
              <el-button type="primary" :loading="loading" :disabled="!canOperate('build')" @click="handleBuild">{{ $t('generated.detectionDetectionPackagesPackageDetail_rebuild_c13dc5') }}</el-button>
            </div>
          </template>
        </el-tab-pane>

        <el-tab-pane :label="$t('generated.detectionDetectionPackagesPackageDetail_host_status_56e199')" name="hosts">
          <HostStatusTable
            :hosts="hostStatuses"
            :total="hostTotal"
            @page-change="handleHostPageChange"
          />
        </el-tab-pane>

        <el-tab-pane label="Event Schema" name="schema">
          <EventSchemaTable
            :schema="currentBuild?.event_schema || currentPackage?.event_schema"
            :schema-json="currentBuild?.event_schema_json"
          />
        </el-tab-pane>

        <el-tab-pane :label="$t('generated.detectionDetectionPackagesPackageDetail_original_information_45b901')" name="raw">
          <el-empty v-if="!currentDraft" :description="$t('generated.detectionDetectionPackagesPackageDetail_no_original_draft_information_yet_fae81b')" />
          <el-tabs v-else v-model="rawTab" class="raw-tabs">
            <el-tab-pane label="HookPlan" name="hookplan">
              <CodeEditorPanel
                :model-value="currentDraft.hook_plan_yaml || ''"
                language="yaml"
                readonly
              />
            </el-tab-pane>
            <el-tab-pane :label="$t('generated.common_ebpf_source_code_ada0b4')" name="ebpf">
              <CodeEditorPanel
                :model-value="currentDraft.ebpf_source || ''"
                language="c"
                readonly
              />
            </el-tab-pane>
            <el-tab-pane :label="$t('generated.common_sigma_atomic_rules_0c85cc')" name="sigma">
              <CodeEditorPanel
                :model-value="currentDraft.sigma_rules_yaml || ''"
                language="yaml"
                readonly
              />
            </el-tab-pane>
            <el-tab-pane :label="$t('generated.detectionDetectionPackagesPackageDetail_correlation_final_rule_9425fd')" name="correlation">
              <CodeEditorPanel
                :model-value="currentDraft.correlation_yaml || ''"
                language="yaml"
                readonly
              />
            </el-tab-pane>
          </el-tabs>
        </el-tab-pane>

        <el-tab-pane :label="$t('generated.common_associated_alarms_0cf24b')" name="alerts">
          <div v-loading="alertsLoading">
            <el-empty v-if="!alertsLoading && alerts.length === 0" :description="$t('generated.detectionDetectionPackagesPackageDetail_no_associated_alarms_yet_865340')" />
            <el-table v-else :data="alerts" border size="small">
              <el-table-column type="expand">
                <template #default="{ row }">
                  <EvidenceTimeline :evidence="parseEvidence(row.event_data)" />
                </template>
              </el-table-column>
              <el-table-column prop="rule_title" :label="$t('generated.common_rule_name_1937bc')" min-width="180" show-overflow-tooltip>
                <template #default="{ row }">{{ row.rule_title || row.matched_rule_id || '-' }}</template>
              </el-table-column>
              <el-table-column prop="severity" :label="$t('generated.common_severity_d918e4')" width="100" />
              <el-table-column prop="pid" label="PID" width="100" />
              <el-table-column prop="created_at" :label="$t('generated.detectionDetectionPackagesPackageDetail_reporting_time_9d4a3f')" width="180">
                <template #default="{ row }">{{ row.created_at ? formatDateTime(row.created_at) : '-' }}</template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="$t('generated.detectionDetectionPackagesPackageDetail_version_history_877041')" name="versions">
          <el-table :data="versionHistory" border size="small">
            <el-table-column prop="version" :label="$t('generated.detectionDetectionPackagesPackageDetail_version_number_5d55e2')" />
            <el-table-column prop="status" :label="$t('generated.common_state_62e951')">
              <template #default="{ row }">
                <PackageStatusTag :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column prop="build_time" :label="$t('generated.common_build_time_ed9e57')">
              <template #default="{ row }">{{ row.build_time ? formatDateTime(row.build_time) : '-' }}</template>
            </el-table-column>
            <el-table-column prop="sign_time" :label="$t('generated.detectionDetectionPackagesPackageDetail_signing_time_678d63')">
              <template #default="{ row }">{{ row.sign_time && row.sign_time !== '-' ? formatDateTime(row.sign_time) : '-' }}</template>
            </el-table-column>
            <el-table-column prop="enable_time" :label="$t('generated.detectionDetectionPackagesPackageDetail_activation_time_15b6fc')">
              <template #default="{ row }">{{ row.enable_time && row.enable_time !== '-' ? formatDateTime(row.enable_time) : '-' }}</template>
            </el-table-column>
            <el-table-column prop="operator" :label="$t('generated.common_operator_06858d')" />
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="160">
              <template #default="{ row }">
                <el-button
                  v-if="canOperate('rollback')"
                  type="warning"
                  size="small"
                  link
                  @click="handleRollback(row)"
                >{{ $t('generated.detectionDetectionPackagesPackageDetail_roll_back_to_this_version_5fad9b') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog v-model="signDialogVisible" :title="$t('generated.detectionDetectionPackagesPackageDetail_confirm_signature_release_e0e3a4')" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          {{ $t('generated.detectionDetectionPackagesPackageDetail_the_signature_cannot_be_modified_after_d11427') }}
        </template>
      </el-alert>
      <el-descriptions :column="1" border size="small" style="margin-top: 16px;">
        <el-descriptions-item label="Package ID">{{ currentPackage?.package_id }}</el-descriptions-item>
        <el-descriptions-item :label="$t('generated.common_version_989d1a')">{{ currentPackage?.version }}</el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="signDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="primary" :loading="loading" @click="confirmSign">{{ $t('generated.detectionDetectionPackagesPackageDetail_confirm_signature_d670c2') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="enableDialogVisible" :title="$t('generated.common_confirm_enable_62d826')" width="500">
      <el-alert type="warning" :closable="false" show-icon>
        <template #title>
          {{ $t('generated.common_the_detectionpackage_will_be_delivered_to_bc5580') }}
        </template>
      </el-alert>
      <template #footer>
        <el-button @click="enableDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="primary" :loading="loading" @click="confirmEnable">{{ $t('generated.common_confirm_enable_62d826') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="disableDialogVisible" :title="$t('generated.common_confirm_disable_816c68')" width="400">
      <el-text>{{ $t('generated.detectionDetectionPackagesPackageDetail_are_you_sure_you_want_to_6d6fcd') }}</el-text>
      <template #footer>
        <el-button @click="disableDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="warning" :loading="loading" @click="confirmDisable">{{ $t('generated.common_confirm_disable_816c68') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="uninstallDialogVisible" :title="$t('generated.common_confirm_uninstall_c7ff7b')" width="400">
      <el-text type="danger">{{ $t('generated.detectionDetectionPackagesPackageDetail_are_you_sure_you_want_to_c25a81') }}</el-text>
      <template #footer>
        <el-button @click="uninstallDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="danger" :loading="loading" @click="confirmUninstall">{{ $t('generated.common_confirm_uninstall_c7ff7b') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { ref, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import { useRole } from '@/composables/useRole'
import { detectionPackageApi } from '@/api/detection-packages'
import type { PackageRuntimeEvent } from '@/api/detection-packages'
import { useDetectionPackages } from './composables/useDetectionPackages'
import PackageStatusTag from './components/PackageStatusTag.vue'
import BuildReviewPanel from './components/BuildReviewPanel.vue'
import EventSchemaTable from './components/EventSchemaTable.vue'
import HostStatusTable from './components/HostStatusTable.vue'
import EvidenceTimeline from './components/EvidenceTimeline.vue'
import CodeEditorPanel from './components/CodeEditorPanel.vue'

const route = useRoute()
const { canOperate } = useRole()
const {
  currentPackage, currentDraft, currentBuild, hostStatuses, hostTotal, loading,
  fetchPackage, fetchBuild, fetchLatestBuild, startBuild, signPackage,
  enablePackage, disablePackage, uninstallPackage, fetchHostStatus, fetchDraft,
} = useDetectionPackages()

const packageId = ref(route.params.id as string)
const activeTab = ref(route.query.tab as string || 'info')
const rawTab = ref('hookplan')

const signDialogVisible = ref(false)
const enableDialogVisible = ref(false)
const disableDialogVisible = ref(false)
const uninstallDialogVisible = ref(false)

const alerts = ref<PackageRuntimeEvent[]>([])
const alertsLoading = ref(false)
const versionHistory = ref<any[]>([])

function handleTabChange(tab: string) {
  if (tab === 'alerts') {
    loadAlerts()
  }
  if (tab === 'raw') {
    loadRawDraft()
  }
}

async function loadRawDraft() {
  await fetchDraft(packageId.value)
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
      translate('generatedScript.detectionDetectionPackagesPackageDetail_are_you_sure_you_want_to_3da510', { p0: row.version }),
      translate('generatedScript.detectionDetectionPackagesPackageDetail_confirm_rollback_2a814f'),
      { confirmButtonText: translate('generatedScript.common_confirm_b56d9a'), cancelButtonText: translate('generatedScript.common_cancel_4d0b46'), type: 'warning' }
    )
    await detectionPackageApi.rollbackPackage(packageId.value, row.version)
    loadPackage()
  } catch {}
}

async function loadPackage() {
  await fetchPackage(packageId.value)
  await loadRawDraft()
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
    if (build && ['pending', 'running', 'build_pending', 'build_running'].includes(build.status)) {
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

function parseEvidence(eventData: string) {
  if (!eventData) return []
  try {
    const parsed = JSON.parse(eventData)
    if (Array.isArray(parsed)) return parsed
    if (Array.isArray(parsed.evidence)) return parsed.evidence
    return []
  } catch {
    return []
  }
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
</style>

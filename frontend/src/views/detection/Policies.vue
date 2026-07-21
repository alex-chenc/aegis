<template>
  <div class="detection-policies-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="searchQuery"
          :placeholder="$t('generated.detectionPolicies_search_miter_id_rule_title_d4c850')"
          clearable
          class="search-input"
          @keyup.enter="loadPolicies"
        />
        <el-button type="primary" @click="loadPolicies">{{ $t('generated.common_query_711363') }}</el-button>
        <el-button @click="loadPolicies">{{ $t('generated.common_refresh_38108e') }}</el-button>
      </div>
    </el-card>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.detectionPolicies_blocking_strategies_total_c42b1b') }} {{ total }} {{ $t('generated.detectionPolicies_strip_a676a7') }}</span>
        </div>
      </template>

      <el-table v-loading="policyLoading" :data="blockPolicies" border stripe>
        <el-table-column label="MITRE" width="140">
          <template #default="{ row }">
            <el-link type="primary" @click="goToRules(row.mitre_id)">{{ row.mitre_id }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="rule_title" :label="$t('generated.detectionPolicies_association_rules_cd8126')" min-width="220">
          <template #default="{ row }">
            <div class="rule-cell">
              <span>{{ row.rule_title || row.mitre_name || '-' }}</span>
              <el-tag v-if="row.rule_count" size="small" type="info">{{ row.rule_count }} {{ $t('generated.common_strip_bce2ef') }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_blocking_action_b3edea')" width="160" align="center">
          <template #default="{ row }">
            <el-select v-model="row.action" size="small" @change="(v: string) => handleUpdateAction(row.mitre_id, v)">
              <el-option :label="$t('generated.common_terminate_process_58d47f')" value="kill_process" />
              <el-option :label="$t('generated.common_quarantine_files_749329')" value="quarantine_file" />
              <el-option :label="$t('generated.common_block_network_3260dd')" value="block_connection" />
              <el-option :label="$t('generated.common_disable_user_638157')" value="disable_user" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_enable_d4e9ca')" width="80" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="(v: boolean) => handleToggleEnabled(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionPolicies_automatically_block_312be8')" width="100" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.auto_block" @change="(v: boolean) => handleToggleAutoBlock(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionPolicies_ai_automatically_blocks_97e4e0')" width="120" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.ai_auto_block" @change="(v: boolean) => handleToggleAIAutoBlock(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionPolicies_automatic_disposal_1138b5')" width="100" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.auto_dispose" @change="(v: boolean) => handleToggleAutoDispose(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" :label="$t('generated.common_update_time_093dea')" width="160">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="100" align="center">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="handleDelete(row.mitre_id)">
              {{ $t('generated.common_delete_3755f5') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="loadPolicies"
          @current-change="loadPolicies"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as api from '@/api/detection'

const router = useRouter()
const searchQuery = ref('')
const blockPolicies = ref<any[]>([])
const policyLoading = ref(false)
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
let ws: WebSocket | null = null

function formatTime(time: string) {
  if (!time) return '-'
  return formatDateTime(time)
}

function goToRules(mitreId: string) {
  router.push({ path: '/detection/rules', query: { query: mitreId } })
}

async function loadPolicies() {
  policyLoading.value = true
  try {
    const res = await api.getBlockPolicies({ 
      page: page.value, 
      page_size: pageSize.value,
      query: searchQuery.value || undefined
    })
    blockPolicies.value = res.data || []
    total.value = res.total || 0
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_loading_failed_f6b7a4'))
  } finally {
    policyLoading.value = false
  }
}

async function handleToggleEnabled(mitreId: string, enabled: boolean) {
  try {
    await api.updateBlockPolicy(mitreId, { enabled })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
      blockPolicies.value[index].enabled = enabled
    }
    ElMessage.success(translate('generatedScript.detectionPolicies_policy_enablement_status_updated_135d58'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
  }
}

async function handleToggleAutoBlock(mitreId: string, autoBlock: boolean) {
  try {
    await api.updateBlockPolicy(mitreId, { auto_block: autoBlock })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
      blockPolicies.value[index].auto_block = autoBlock
      if (autoBlock) blockPolicies.value[index].ai_auto_block = false
    }
    ElMessage.success(translate('generatedScript.detectionPolicies_automatic_blocking_status_has_been_updated_8bd4f7'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
  }
}

async function handleToggleAIAutoBlock(mitreId: string, aiAutoBlock: boolean) {
  try {
    await api.updateBlockPolicy(mitreId, { ai_auto_block: aiAutoBlock })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
      blockPolicies.value[index].ai_auto_block = aiAutoBlock
      if (aiAutoBlock) blockPolicies.value[index].auto_block = false
    }
    ElMessage.success(translate('generatedScript.detectionPolicies_ai_automatic_blocking_status_has_been_a67fb2'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
  }
}

async function handleToggleAutoDispose(mitreId: string, autoDispose: boolean) {
  try {
    await api.updateBlockPolicy(mitreId, { auto_dispose: autoDispose })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
      blockPolicies.value[index].auto_dispose = autoDispose
    }
    ElMessage.success(translate('generatedScript.detectionPolicies_automatic_disposal_status_updated_82fa7b'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
  }
}

async function handleUpdateAction(mitreId: string, action: string) {
  try {
    await api.updateBlockPolicy(mitreId, { action })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
      blockPolicies.value[index].action = action
    }
    ElMessage.success(translate('generatedScript.detectionPolicies_blocking_action_updated_b0d00e'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
  }
}

async function handleDelete(mitreId: string) {
  try {
    await ElMessageBox.confirm(
      translate('generatedScript.detectionPolicies_deleting_this_blocking_policy_will_also_d3f448'),
      translate('generatedScript.common_confirm_deletion_3c06ab'),
      {
        confirmButtonText: translate('generatedScript.common_sure_f526c8'),
        cancelButtonText: translate('generatedScript.common_cancel_4d0b46'),
        type: 'warning',
      }
    )
    await api.deleteBlockPolicy(mitreId)
    ElMessage.success(translate('generatedScript.common_delete_successfully_86e8d1'))
    loadPolicies()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || translate('generatedScript.common_delete_failed_72250c'))
    }
  }
}

function connectWebSocket() {
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsHost = window.location.host.replace(':8081', ':8080')
  const wsUrl = `${wsProtocol}//${wsHost}/api/v1/detection/runtime/ws`

  ws = new WebSocket(wsUrl)

  ws.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data)
      if (message.type === 'policy_update' && message.data) {
        const updatedPolicy = message.data
        const index = blockPolicies.value.findIndex(p => p.mitre_id === updatedPolicy.mitre_id)
        if (index !== -1) {
          blockPolicies.value[index] = { ...blockPolicies.value[index], ...updatedPolicy }
        }
      }
    } catch {
      // Ignore parse errors
    }
  }

  ws.onerror = () => {
    console.warn('WebSocket connection error')
  }

  ws.onclose = () => {
    setTimeout(connectWebSocket, 3000)
  }
}

onMounted(() => {
  loadPolicies()
  connectWebSocket()
})

onUnmounted(() => {
  if (ws) {
    ws.close()
    ws = null
  }
})
</script>

<style scoped>
.detection-policies-page {
  padding: 20px;
}

.rule-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.filter-card {
  margin-bottom: 16px;
}

.filter-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.search-input {
  width: 280px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pagination-container {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}
</style>

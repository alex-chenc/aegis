<template>
  <div class="detection-policies-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="searchQuery"
          placeholder="搜索MITRE ID / 规则标题"
          clearable
          class="search-input"
          @keyup.enter="loadPolicies"
        />
        <el-button type="primary" @click="loadPolicies">查询</el-button>
        <el-button @click="loadPolicies">刷新</el-button>
      </div>
    </el-card>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>阻断策略 (共 {{ total }} 条)</span>
        </div>
      </template>

      <el-table v-loading="policyLoading" :data="blockPolicies" border stripe>
        <el-table-column label="MITRE" width="140">
          <template #default="{ row }">
            <el-link type="primary" @click="goToRules(row.mitre_id)">{{ row.mitre_id }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="rule_title" label="规则标题" min-width="180">
          <template #default="{ row }">
            {{ row.rule_title || row.mitre_name || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="阻断动作" width="160" align="center">
          <template #default="{ row }">
            <el-select v-model="row.action" size="small" @change="(v: string) => handleUpdateAction(row.mitre_id, v)">
              <el-option label="终止进程" value="kill_process" />
              <el-option label="隔离文件" value="quarantine_file" />
              <el-option label="阻断网络" value="block_connection" />
              <el-option label="禁用用户" value="disable_user" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="(v: boolean) => handleToggleEnabled(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column label="自动阻断" width="100" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.auto_block" @change="(v: boolean) => handleToggleAutoBlock(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column label="AI自动阻断" width="120" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.ai_auto_block" @change="(v: boolean) => handleToggleAIAutoBlock(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column label="自动处置" width="100" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.auto_dispose" @change="(v: boolean) => handleToggleAutoDispose(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="160">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100" align="center">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="handleDelete(row.mitre_id)">
              删除
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
  return new Date(time).toLocaleString('zh-CN')
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
    ElMessage.error(e.message || '加载失败')
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
    ElMessage.success('策略启用状态已更新')
  } catch (e: any) {
    ElMessage.error(e.message || '更新失败')
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
    ElMessage.success('自动阻断状态已更新')
  } catch (e: any) {
    ElMessage.error(e.message || '更新失败')
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
    ElMessage.success('AI自动阻断状态已更新')
  } catch (e: any) {
    ElMessage.error(e.message || '更新失败')
  }
}

async function handleToggleAutoDispose(mitreId: string, autoDispose: boolean) {
  try {
    await api.updateBlockPolicy(mitreId, { auto_dispose: autoDispose })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
      blockPolicies.value[index].auto_dispose = autoDispose
    }
    ElMessage.success('自动处置状态已更新')
  } catch (e: any) {
    ElMessage.error(e.message || '更新失败')
  }
}

async function handleUpdateAction(mitreId: string, action: string) {
  try {
    await api.updateBlockPolicy(mitreId, { action })
    const index = blockPolicies.value.findIndex(p => p.mitre_id === mitreId)
    if (index !== -1) {
      blockPolicies.value[index].action = action
    }
    ElMessage.success('阻断动作已更新')
  } catch (e: any) {
    ElMessage.error(e.message || '更新失败')
  }
}

async function handleDelete(mitreId: string) {
  try {
    await ElMessageBox.confirm(
      '删除该阻断策略将同时删除关联的规则和告警，是否继续？',
      '确认删除',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
    await api.deleteBlockPolicy(mitreId)
    ElMessage.success('删除成功')
    loadPolicies()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || '删除失败')
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
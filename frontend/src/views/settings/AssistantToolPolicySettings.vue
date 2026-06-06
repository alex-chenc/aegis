<template>
  <div class="settings page-shell">
    <section class="page-hero settings-hero">
      <div>
        <span class="hero-kicker">Agent Tool Policy</span>
        <h1>智能体工具权限</h1>
        <p>配置智能体工具的审批模式和白名单策略，控制工具调用的安全边界。</p>
      </div>
    </section>

    <div class="settings-grid">
      <!-- 审批模式选择 -->
      <el-card class="aegis-card settings-card">
        <template #header>
          <div class="card-header">
            <span>工具审批模式</span>
            <el-tag :type="modeTagType" size="small">{{ modeLabel }}</el-tag>
          </div>
        </template>

        <el-alert
          :title="modeAlertTitle"
          :type="modeAlertType"
          show-icon
          :closable="false"
          class="settings-alert"
        >
          <p>{{ modeAlertDescription }}</p>
        </el-alert>

        <el-radio-group v-model="currentMode" class="mode-selector" @change="handleModeChange">
          <el-radio-button value="request_approval">
            <el-icon><Lock /></el-icon>
            请求批准
          </el-radio-button>
          <el-radio-button value="whitelist">
            <el-icon><List /></el-icon>
            白名单
          </el-radio-button>
          <el-radio-button value="full_access">
            <el-icon><Unlock /></el-icon>
            完全权限
          </el-radio-button>
        </el-radio-group>
      </el-card>

      <!-- 工具白名单管理 -->
      <el-card class="aegis-card settings-card" :class="{ 'card-disabled': currentMode === 'request_approval' || currentMode === 'full_access' }">
        <template #header>
          <div class="card-header">
            <span>工具白名单</span>
            <div class="card-header-actions">
              <el-tag size="small" type="info">共 {{ totalTools }} 个工具</el-tag>
              <el-button size="small" :loading="resetting" @click="handleResetDefaults">
                恢复默认白名单
              </el-button>
            </div>
          </div>
        </template>

        <!-- 搜索和筛选 -->
        <div class="filter-bar">
          <el-input
            v-model="keyword"
            placeholder="搜索工具名、描述..."
            clearable
            class="filter-input"
            @input="debouncedFetch"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
          <el-select v-model="domainFilter" placeholder="领域" clearable class="filter-select" @change="fetchTools">
            <el-option v-for="d in domainOptions" :key="d.value" :label="d.label" :value="d.value" />
          </el-select>
          <el-select v-model="riskFilter" placeholder="风险等级" clearable class="filter-select" @change="fetchTools">
            <el-option v-for="r in riskOptions" :key="r.value" :label="r.label" :value="r.value" />
          </el-select>
          <el-select v-model="whitelistFilter" placeholder="白名单状态" clearable class="filter-select" @change="fetchTools">
            <el-option label="已加入白名单" value="true" />
            <el-option label="未加入白名单" value="false" />
          </el-select>
          <el-button
            size="small"
            :disabled="!hasSelection"
            @click="handleBatchWhitelist(true)"
          >
            批量加入白名单
          </el-button>
          <el-button
            size="small"
            :disabled="!hasSelection"
            @click="handleBatchWhitelist(false)"
          >
            批量移出白名单
          </el-button>
        </div>

        <!-- 工具表格 -->
        <el-table
          v-loading="loading"
          :data="tools"
          class="tool-table"
          @selection-change="handleSelectionChange"
        >
          <el-table-column type="selection" width="45" />
          <el-table-column prop="tool_name" label="工具名称" min-width="180" show-overflow-tooltip />
          <el-table-column prop="domain" label="领域" width="100">
            <template #default="{ row }">
              <el-tag size="small" type="info">{{ row.domain }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="operation" label="操作" width="90">
            <template #default="{ row }">
              <el-tag size="small">{{ row.operation }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="risk_level" label="风险" width="90">
            <template #default="{ row }">
              <el-tag :type="getRiskTagType(row.risk_level)" size="small">{{ riskLabel(row.risk_level) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="description" label="工具详情" min-width="240" show-overflow-tooltip />
          <el-table-column label="白名单" width="90" align="center">
            <template #default="{ row }">
              <el-switch
                v-model="row.whitelisted"
                :disabled="currentMode !== 'whitelist'"
                @change="(val: boolean) => handleWhitelistChange(row, val)"
              />
            </template>
          </el-table-column>
        </el-table>

        <!-- 分页 -->
        <div class="pagination-bar">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :total="totalTools"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            @current-change="fetchTools"
            @size-change="fetchTools"
          />
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { List, Lock, Search, Unlock } from '@element-plus/icons-vue'
import {
  getToolApprovalPolicy,
  updateToolApprovalPolicy,
  getTools,
  updateToolWhitelist,
  batchUpdateWhitelist,
  resetWhitelistDefaults
} from '@/api/assistant'
import type { AssistantToolApprovalMode, AssistantRiskLevel } from '@/api/assistant'

// ---- 审批模式 ----
const currentMode = ref<AssistantToolApprovalMode>('whitelist')
const modeLoading = ref(false)

const modeLabel = computed(() => {
  const map: Record<AssistantToolApprovalMode, string> = {
    request_approval: '请求批准',
    whitelist: '白名单',
    full_access: '完全权限'
  }
  return map[currentMode.value] || currentMode.value
})

const modeTagType = computed(() => {
  const map: Record<AssistantToolApprovalMode, 'danger' | 'warning' | 'success'> = {
    request_approval: 'danger',
    whitelist: 'warning',
    full_access: 'success'
  }
  return map[currentMode.value] || 'info'
})

const modeAlertTitle = computed(() => {
  const map: Record<AssistantToolApprovalMode, string> = {
    request_approval: '请求批准模式',
    whitelist: '白名单模式（默认）',
    full_access: '完全权限模式'
  }
  return map[currentMode.value]
})

const modeAlertType = computed(() => {
  const map: Record<AssistantToolApprovalMode, 'error' | 'warning' | 'success'> = {
    request_approval: 'error',
    whitelist: 'warning',
    full_access: 'success'
  }
  return map[currentMode.value]
})

const modeAlertDescription = computed(() => {
  const map: Record<AssistantToolApprovalMode, string> = {
    request_approval: '所有工具调用都将等待人工批准，包含只读查询。适用于生产环境初期、安全演示、强审计场景。',
    whitelist: '白名单内工具自动执行；非白名单工具需要人工批准。兼顾效率和安全。',
    full_access: '所有被本轮智能体选中的工具将直接执行，仍会记录审计。适用于测试环境、离线演练、受控管理员环境。'
  }
  return map[currentMode.value]
})

async function handleModeChange(mode: AssistantToolApprovalMode) {
  modeLoading.value = true
  try {
    await updateToolApprovalPolicy({ mode })
    ElMessage.success('审批模式已更新')
  } catch (e: any) {
    ElMessage.error(e.message || '更新失败')
    // 回滚
    await fetchApprovalMode()
  } finally {
    modeLoading.value = false
  }
}

async function fetchApprovalMode() {
  try {
    // 拦截器已解包 data，res = { mode: "whitelist" }
    const res = await getToolApprovalPolicy()
    currentMode.value = (res as any).mode || 'whitelist'
  } catch {
    currentMode.value = 'whitelist'
  }
}

// ---- 工具白名单 ----
const tools = ref<any[]>([])
const totalTools = ref(0)
const loading = ref(false)
const resetting = ref(false)
const keyword = ref('')
const domainFilter = ref('')
const riskFilter = ref('')
const whitelistFilter = ref('')
const currentPage = ref(1)
const pageSize = ref(20)
const selectedTools = ref<any[]>([])
const hasSelection = computed(() => selectedTools.value.length > 0)

const domainOptions = [
  { label: '系统', value: 'system' },
  { label: '主机', value: 'host' },
  { label: '基线', value: 'baseline' },
  { label: '任务', value: 'task' },
  { label: '漏洞', value: 'vulnerability' },
  { label: '检测', value: 'detection' },
  { label: 'Sigma 规则', value: 'sigma_rule' },
  { label: '阻断', value: 'block' },
  { label: '检测包', value: 'package' },
  { label: '配置', value: 'config' },
  { label: '审计', value: 'audit' },
  { label: 'Agent', value: 'agent' },
  { label: '研判', value: 'investigation' },
  { label: '外部 MCP', value: 'external_mcp' },
  { label: '通知', value: 'notification' }
]

const riskOptions = [
  { label: '只读', value: 'readonly' },
  { label: '低', value: 'low' },
  { label: '中', value: 'medium' },
  { label: '高', value: 'high' },
  { label: '严重', value: 'critical' }
]

function riskLabel(risk: string): string {
  const map: Record<string, string> = {
    readonly: '只读',
    low: '低',
    medium: '中',
    high: '高',
    critical: '严重'
  }
  return map[risk] || risk
}

function getRiskTagType(risk: string): 'info' | 'success' | 'warning' | 'danger' {
  const map: Record<string, 'info' | 'success' | 'warning' | 'danger'> = {
    readonly: 'info',
    low: 'success',
    medium: 'warning',
    high: 'danger',
    critical: 'danger'
  }
  return map[risk] || 'info'
}

async function fetchTools() {
  loading.value = true
  try {
    const params: Record<string, any> = {
      page: currentPage.value,
      page_size: pageSize.value
    }
    if (keyword.value) params.keyword = keyword.value
    if (domainFilter.value) params.domain = domainFilter.value
    if (riskFilter.value) params.risk_level = riskFilter.value
    if (whitelistFilter.value) params.whitelisted = whitelistFilter.value

    // 拦截器已解包 data，res = { tools: [...], total: N }
    const res = await getTools(params) as any
    tools.value = res.tools || res.items || []
    totalTools.value = res.total || 0
  } catch (e: any) {
    ElMessage.error(e.message || '获取工具列表失败')
  } finally {
    loading.value = false
  }
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function debouncedFetch() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    currentPage.value = 1
    fetchTools()
  }, 300)
}

function handleSelectionChange(selection: any[]) {
  selectedTools.value = selection
}

async function handleWhitelistChange(row: any, whitelisted: boolean) {
  // critical 工具加入白名单需要二次确认
  if (whitelisted && row.risk_level === 'critical') {
    try {
      await ElMessageBox.confirm(
        `确定要将 ${row.tool_name}（严重风险）加入白名单吗？加入后该工具将自动执行，无需人工审批。`,
        '二次确认',
        { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
      )
    } catch {
      // 取消，回滚
      row.whitelisted = false
      return
    }
  }

  try {
    await updateToolWhitelist(row.tool_name, { auto_approve: whitelisted })
    ElMessage.success(whitelisted ? '已加入白名单' : '已移出白名单')
  } catch (e: any) {
    row.whitelisted = !whitelisted
    ElMessage.error(e.message || '更新失败')
  }
}

async function handleBatchWhitelist(whitelisted: boolean) {
  if (!hasSelection.value) return

  // 检查是否有 critical 工具加入白名单
  if (whitelisted) {
    const criticalTools = selectedTools.value.filter(t => t.risk_level === 'critical')
    if (criticalTools.length > 0) {
      try {
        await ElMessageBox.confirm(
          `选中包含 ${criticalTools.length} 个严重风险工具，确定要加入白名单吗？`,
          '二次确认',
          { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
        )
      } catch {
        return
      }
    }
  }

  try {
    const entries = selectedTools.value.map(t => ({
      tool_name: t.tool_name,
      auto_approve: whitelisted
    }))
    await batchUpdateWhitelist({ entries })
    ElMessage.success(whitelisted ? '已批量加入白名单' : '已批量移出白名单')
    await fetchTools()
  } catch (e: any) {
    ElMessage.error(e.message || '批量更新失败')
  }
}

async function handleResetDefaults() {
  try {
    await ElMessageBox.confirm(
      '确定要恢复默认白名单吗？这将把所有工具的白名单状态重置为系统默认值。',
      '恢复默认',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }

  resetting.value = true
  try {
    await resetWhitelistDefaults()
    ElMessage.success('已恢复默认白名单')
    await fetchTools()
  } catch (e: any) {
    ElMessage.error(e.message || '恢复失败')
  } finally {
    resetting.value = false
  }
}

onMounted(async () => {
  await Promise.all([fetchApprovalMode(), fetchTools()])
})
</script>

<style scoped>
.settings-hero {
  margin-bottom: 0;
}

.hero-kicker {
  display: inline-flex;
  margin-bottom: 8px;
  color: #0891b2;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.settings-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px;
}

.settings-card {
  overflow: hidden;
}

.card-disabled {
  opacity: 0.6;
  pointer-events: none;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.card-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.settings-alert {
  margin-bottom: 20px;
}

.mode-selector {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.mode-selector :deep(.el-radio-button__inner) {
  min-width: 110px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  border-radius: 999px;
  font-weight: 650;
}

.mode-selector :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  border-color: transparent;
  background: linear-gradient(135deg, #2563eb, #0891b2);
  box-shadow: 0 10px 24px rgba(37, 99, 235, 0.22);
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
  align-items: center;
}

.filter-input {
  width: 220px;
}

.filter-select {
  width: 130px;
}

.tool-table {
  margin-bottom: 16px;
}

.pagination-bar {
  display: flex;
  justify-content: flex-end;
}
</style>

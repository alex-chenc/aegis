# 前端详细设计文档 - V5.2 完整版

**版本**: 5.2
**状态**: 定稿
**日期**: 2026-03-26

---

## 1. 项目结构

```
/frontend
├── src/
│   ├── api/
│   │   ├── detection.ts        # 异常检测API
│   │   ├── hosts.ts
│   │   └── ...
│   ├── views/
│   │   └── detection/
│   │       ├── Overview.vue    # 安全概览
│   │       ├── Alerts.vue      # 告警中心
│   │       ├── Policies.vue    # 阻断策略
│   │       └── Rules.vue       # 规则管理
│   ├── store/
│   │   └── detection.ts        # Pinia状态
│   ├── types/
│   │   └── index.ts            # 类型定义
│   └── components/
│       └── ProcessTree.vue     # 进程树组件
├── package.json
├── vite.config.ts
└── Dockerfile
```

---

## 2. 类型定义

```typescript
// src/types/index.ts

export interface Alert {
  id: string
  alert_id: string
  host_id: string
  hostname: string
  pid: number
  ppid: number
  command_line: string
  process_tree: string
  mitre_id: string        // MITRE技术编号（大写T格式）
  mitre_name: string
  severity: 'critical' | 'high' | 'medium' | 'low'
  description: string
  llm_summary: string
  status: 'pending' | 'resolved'
  judgment_source: 'system' | 'ai'
  block_status: 'blocking' | 'success' | 'failed' | null
  block_message: string
  auto_blocked: boolean
  auto_dispose: boolean
  hit_count: number
  rule_id: string
  rule_title: string
  first_seen_at: string
  last_seen_at: string
}

export interface BlockPolicy {
  id: string
  mitre_id: string        // MITRE技术编号（大写T格式）
  mitre_name: string      // 规则标题（与规则表一致）
  rule_title: string      // 规则标题（API返回）
  enabled: boolean
  auto_block: boolean
  auto_dispose: boolean
  action: 'kill_process' | 'quarantine_file' | 'block_connection' | 'disable_user'
  created_at: string
  updated_at: string
}

export interface SigmaRule {
  id: string
  rule_id: string
  title: string
  mitre_id: string        // MITRE技术编号（大写T格式，唯一）
  severity: 'critical' | 'high' | 'medium' | 'low'
  status: 'pending' | 'experimental' | 'active' | 'disabled'
  content: string
  version: string
  generated_by: 'import' | 'llm'
  created_at: string
  updated_at: string
  activated_at: string
}

export interface AttackMatrix {
  tactics: AttackTactic[]
}

export interface AttackTactic {
  id: string
  name: string
  techniques: AttackTechnique[]
}

export interface AttackTechnique {
  id: string
  name: string
  alert_count: number
}

export interface LLMAggregation {
  aggregation_id: string
  event_count: number
  alert_count: number
  ai_judged_count: number
  auto_dispose_count: number
  status: 'pending' | 'processing' | 'completed' | 'failed'
  llm_response: string
}

export interface GenerateSigmaRuleRequest {
  event: string
  method?: string
  mitre_id?: string
  severity?: string
}

export interface GenerateSigmaRuleResponse {
  rule_id: string
  title: string
  mitre_id: string
  severity: string
  content: string
  duration: number
}

// 中文标签映射
export const SeverityLabels: Record<string, string> = {
  critical: '严重',
  high: '高危',
  medium: '中危',
  low: '低危'
}

export const AlertStatusLabels: Record<string, string> = {
  pending: '待处置',
  resolved: '已处置'
}

export const BlockStatusLabels: Record<string, string> = {
  blocking: '阻断中',
  success: '阻断成功',
  failed: '阻断失败'
}

export const JudgmentSourceLabels: Record<string, string> = {
  system: '系统判定',
  ai: 'AI判定'
}

export const RuleStatusLabels: Record<string, string> = {
  pending: '待审核',
  experimental: '实验性',
  active: '已激活',
  disabled: '已禁用'
}
```

---

## 3. API定义

```typescript
// src/api/detection.ts

import request from '@/utils/request'
import type {
  Alert,
  BlockPolicy,
  SigmaRule,
  AttackMatrix,
  LLMAggregation,
  GenerateSigmaRuleRequest,
  GenerateSigmaRuleResponse
} from '@/types'

// ==================== 告警管理 ====================

// 告警列表
export async function getAlerts(params: {
  page?: number
  page_size?: number
  severity?: string
  status?: string
  judgment_source?: string
  query?: string
}): Promise<{ data: Alert[]; total: number }> {
  const res = await request.get('/detection/alerts', { params })
  return res.data
}

// 告警详情
export async function getAlertDetail(alertId: string): Promise<Alert> {
  const res = await request.get(`/detection/alerts/${alertId}`)
  return res.data
}

// 处置告警
export async function resolveAlert(alertId: string): Promise<void> {
  await request.post(`/detection/alerts/${alertId}/resolve`)
}

// 阻断告警
export async function blockAlert(alertId: string, action: string): Promise<void> {
  await request.post(`/detection/alerts/${alertId}/block`, { action })
}

// 批量删除告警
export async function deleteAlerts(alertIds: string[]): Promise<{ deleted_count: number }> {
  return request.delete('/detection/alerts', { data: { alert_ids: alertIds } })
}

// ==================== 阻断策略 ====================

// 阻断策略列表（带规则标题）
export async function getBlockPolicies(params: {
  page?: number
  page_size?: number
  query?: string
}): Promise<{ data: BlockPolicy[]; total: number }> {
  const res = await request.get('/detection/block-policies', { params })
  return res.data
}

// 更新阻断策略
export async function updateBlockPolicy(
  mitreId: string,
  data: Partial<BlockPolicy>
): Promise<void> {
  await request.put(`/detection/block-policies/${mitreId}`, data)
}

// 同步阻断策略
export async function syncBlockPolicies(): Promise<{ created: number; total_rules: number }> {
  const res = await request.post('/detection/block-policies/sync')
  return res.data
}

// 规范化MITRE ID
export async function normalizeMitreIDs(): Promise<{
  rules_updated: number
  policies_updated: number
  alerts_updated: number
}> {
  const res = await request.post('/detection/block-policies/normalize')
  return res.data
}

// ==================== 规则管理 ====================

// 规则列表
export async function getRules(params: {
  page?: number
  pageSize?: number
  status?: string
  query?: string
}): Promise<{ data: SigmaRule[]; total: number }> {
  const res = await request.get('/detection/rules', { params })
  return res.data
}

// 规则详情
export async function getRuleDetail(ruleId: string): Promise<SigmaRule> {
  const res = await request.get(`/detection/rules/${ruleId}`)
  return res.data
}

// 更新规则状态
export async function updateRuleStatus(ruleId: string, status: string): Promise<void> {
  await request.put(`/detection/rules/${ruleId}/status`, { status })
}

// AI生成Sigma规则
export async function generateSigmaRule(data: GenerateSigmaRuleRequest): Promise<GenerateSigmaRuleResponse> {
  return request.post('/detection/rules/generate', data)
}

// 删除前检查
export async function checkRulesBeforeDelete(ruleIds: string[]): Promise<{
  has_alerts: boolean
  rules_with_alerts: Array<{ rule_id: string; title: string; alert_count: number }>
  total_alerts: number
}> {
  return request.post('/detection/rules/check-delete', { rule_ids: ruleIds })
}

// 批量删除规则
export async function deleteRules(ruleIds: string[]): Promise<{
  deleted_rules: number
  deleted_alerts: number
  deleted_policies: number
}> {
  return request.delete('/detection/rules', { data: { rule_ids: ruleIds } })
}

// ==================== MITRE矩阵 ====================

export async function getAttackMatrix(): Promise<AttackMatrix> {
  const res = await request.get('/detection/attack-matrix')
  return res.data
}

// ==================== AI降噪 ====================

export async function startLLMAggregation(params: {
  start_time: string
  end_time: string
  host_ids?: string[]
  auto_dispose?: boolean
}): Promise<LLMAggregation> {
  const res = await request.post('/detection/llm/aggregate', params)
  return res.data
}

export async function getLLMAggregationStatus(aggregationId: string): Promise<LLMAggregation> {
  const res = await request.get(`/detection/llm/aggregate/${aggregationId}`)
  return res.data
}
```

---

## 4. 页面组件

### 4.1 阻断策略页面 (Policies.vue)

#### 界面布局

```
┌──────────────────────────────────────────────────────────────────────┐
│ [搜索MITRE ID/规则标题] [查询] [刷新]                                   │
├──────────────────────────────────────────────────────────────────────┤
│ 阻断策略 (共 38 条)                                                    │
├──────────────────────────────────────────────────────────────────────┤
│ MITRE    │ 规则标题       │ 阻断动作     │ 启用 │ 自动阻断 │ 自动处置 │
│ T1003    │ 凭据窃取       │ [隔离文件 ▼] │ [✓]  │ [ ]      │ [✓]     │
│ T1059.004│ 反向Shell      │ [终止进程 ▼] │ [✓]  │ [✓]      │ [ ]     │
├──────────────────────────────────────────────────────────────────────┤
│                          分页器 (每页10条)                              │
└──────────────────────────────────────────────────────────────────────┘
```

#### 核心功能

```vue
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
      <el-table v-loading="loading" :data="policies" border stripe>
        <el-table-column label="MITRE" width="140">
          <template #default="{ row }">
            <el-link type="primary" @click="goToRules(row.mitre_id)">
              {{ row.mitre_id }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="规则标题" min-width="180">
          <template #default="{ row }">
            {{ row.rule_title || row.mitre_name || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="阻断动作" width="160" align="center">
          <template #default="{ row }">
            <el-select v-model="row.action" size="small" 
              @change="(v: string) => handleUpdateAction(row.mitre_id, v)">
              <el-option label="终止进程" value="kill_process" />
              <el-option label="隔离文件" value="quarantine_file" />
              <el-option label="阻断网络" value="block_connection" />
              <el-option label="禁用用户" value="disable_user" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" 
              @change="(v: boolean) => handleToggleEnabled(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column label="自动阻断" width="100" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.auto_block" 
              @change="(v: boolean) => handleToggleAutoBlock(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column label="自动处置" width="100" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.auto_dispose" 
              @change="(v: boolean) => handleToggleAutoDispose(row.mitre_id, v)" />
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="160">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
      </el-table>
      <!-- 分页器 -->
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
const searchQuery = ref('')

function goToRules(mitreId: string) {
  router.push({ path: '/detection/rules', query: { query: mitreId } })
}

async function loadPolicies() {
  const res = await api.getBlockPolicies({ 
    page: page.value, 
    page_size: pageSize.value,
    query: searchQuery.value || undefined
  })
  policies.value = res.data || []
  total.value = res.total || 0
}
</script>
```

### 4.2 规则管理页面 (Rules.vue)

#### 界面布局

```
┌──────────────────────────────────────────────────────────────────────┐
│ [搜索规则标题/MITRE] [规则状态 ▼] [查询] [AI规则] [删除选中 (0)]         │
├──────────────────────────────────────────────────────────────────────┤
│ ☐ │ 规则标题           │ MITRE    │ 严重程度 │ 状态     │ 操作          │
│ ☐ │ 屏幕截图           │ T1113    │ [中危]   │ [已激活] │ 详情 启用 禁用│
│ ☐ │ 反向Shell检测      │ T1059.004│ [严重]   │ [实验性] │ 详情 启用 禁用│
├──────────────────────────────────────────────────────────────────────┤
│                          分页器                              [已选: 0] │
└──────────────────────────────────────────────────────────────────────┘
```

#### 核心功能

```vue
<template>
  <div class="detection-rules-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="searchQuery"
          placeholder="搜索规则标题/规则ID/MITRE"
          clearable
          class="search-input"
          @keyup.enter="loadRules"
        />
        <el-select v-model="status" placeholder="规则状态" clearable class="filter-item">
          <el-option label="待审核" value="pending" />
          <el-option label="实验性" value="experimental" />
          <el-option label="已激活" value="active" />
          <el-option label="已禁用" value="disabled" />
        </el-select>
        <el-button type="primary" @click="loadRules">查询</el-button>
        <el-button type="success" @click="showAIGenerateDialog">AI规则</el-button>
        <el-button 
          type="danger" 
          :disabled="selectedRules.length === 0"
          @click="confirmDeleteSelected"
        >
          删除选中 ({{ selectedRules.length }})
        </el-button>
      </div>
    </el-card>

    <el-card>
      <el-table 
        v-loading="ruleLoading" 
        :data="rules" 
        border 
        stripe
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="55" />
        <el-table-column prop="title" label="规则标题" min-width="280" />
        <el-table-column label="MITRE" width="120">
          <template #default="{ row }">
            <el-link type="primary" @click="goToPolicies(row.mitre_id)">
              {{ row.mitre_id }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="severity" label="严重程度" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="severityTagType(row.severity)">{{ severityLabel(row.severity) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="版本" width="80" align="center" />
        <el-table-column prop="created_at" label="创建时间" width="160">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" @click="showDetail(row)">详情</el-button>
            <el-button size="small" type="success" 
              :disabled="row.status === 'active'" 
              @click="approveRule(row.rule_id)">启用</el-button>
            <el-button size="small" type="warning" 
              :disabled="row.status === 'disabled'" 
              @click="disableRule(row.rule_id)">禁用</el-button>
          </template>
        </el-table-column>
      </el-table>
      <!-- 分页器 -->
    </el-card>

    <!-- AI规则生成对话框 -->
    <el-dialog v-model="aiGenerateVisible" title="AI生成Sigma规则" width="700px" 
      :close-on-click-modal="false">
      <el-form :model="aiGenerateForm" label-width="100px">
        <el-form-item label="检测事件" required>
          <el-input v-model="aiGenerateForm.event" type="textarea" :rows="3"
            placeholder="描述要检测的安全事件，例如：检测反向Shell连接行为" />
        </el-form-item>
        <el-form-item label="检测方式">
          <el-input v-model="aiGenerateForm.method" type="textarea" :rows="3"
            placeholder="描述检测方式，例如：监控进程命令行参数，检测bash -i、nc -e等反向Shell特征" />
        </el-form-item>
        <el-form-item label="MITRE技术">
          <el-input v-model="aiGenerateForm.mitre_id" placeholder="可选，例如：T1059.004" />
        </el-form-item>
        <el-form-item label="严重程度">
          <el-select v-model="aiGenerateForm.severity" placeholder="选择严重程度">
            <el-option label="低" value="low" />
            <el-option label="中" value="medium" />
            <el-option label="高" value="high" />
            <el-option label="严重" value="critical" />
          </el-select>
        </el-form-item>
      </el-form>

      <div v-if="aiGenerateResult" class="ai-result">
        <el-divider>生成结果</el-divider>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="规则ID">{{ aiGenerateResult.rule_id }}</el-descriptions-item>
          <el-descriptions-item label="规则标题">{{ aiGenerateResult.title }}</el-descriptions-item>
          <el-descriptions-item label="MITRE">{{ aiGenerateResult.mitre_id }}</el-descriptions-item>
          <el-descriptions-item label="严重程度">{{ aiGenerateResult.severity }}</el-descriptions-item>
          <el-descriptions-item label="生成耗时">{{ aiGenerateResult.duration }}秒</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag type="warning">实验性</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="规则内容" :span="2">
            <pre class="content-block">{{ aiGenerateResult.content }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <template #footer>
        <el-button @click="aiGenerateVisible = false">关闭</el-button>
        <el-button type="primary" :loading="aiGenerateLoading" @click="generateRule">
          {{ aiGenerateLoading ? '生成中...' : '开始生成' }}
        </el-button>
        <el-button v-if="aiGenerateResult" type="success" @click="enableGeneratedRule">
          启用规则
        </el-button>
      </template>
    </el-dialog>

    <!-- 删除确认对话框 -->
    <el-dialog v-model="deleteConfirmVisible" title="确认删除" width="500px">
      <div v-if="deleteCheckResult">
        <el-alert v-if="deleteCheckResult.has_alerts"
          title="警告：选中的规则关联了告警数据"
          type="warning"
          :closable="false"
          show-icon
          style="margin-bottom: 16px">
          <template #default>
            <p>以下规则存在关联告警，删除规则将同时删除这些告警和阻断策略：</p>
            <ul>
              <li v-for="rule in deleteCheckResult.rules_with_alerts" :key="rule.rule_id">
                {{ rule.title || rule.rule_id }} ({{ rule.alert_count }} 条告警)
              </li>
            </ul>
          </template>
        </el-alert>
        <p>确定要删除选中的 {{ selectedRules.length }} 条规则吗？</p>
      </div>
      <template #footer>
        <el-button @click="deleteConfirmVisible = false">取消</el-button>
        <el-button type="danger" :loading="deleteLoading" @click="deleteSelectedRules">
          确认删除
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { useRouter, useRoute } from 'vue-router'

const router = useRouter()
const route = useRoute()

// URL参数自动搜索
onMounted(() => {
  const queryParam = route.query.query as string
  if (queryParam) {
    searchQuery.value = queryParam
  }
  loadRules()
})

function goToPolicies(mitreId: string) {
  router.push({ path: '/detection/policies', query: { query: mitreId } })
}

// AI规则生成
async function generateRule() {
  if (!aiGenerateForm.value.event) {
    ElMessage.warning('请输入检测事件描述')
    return
  }
  aiGenerateLoading.value = true
  try {
    const result = await api.generateSigmaRule({
      event: aiGenerateForm.value.event,
      method: aiGenerateForm.value.method,
      mitre_id: aiGenerateForm.value.mitre_id,
      severity: aiGenerateForm.value.severity
    })
    aiGenerateResult.value = result
    ElMessage.success('规则生成成功')
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || '规则生成失败')
  } finally {
    aiGenerateLoading.value = false
  }
}

// 批量删除
async function confirmDeleteSelected() {
  const ruleIds = selectedRules.value.map(r => r.rule_id)
  const result = await api.checkRulesBeforeDelete(ruleIds)
  deleteCheckResult.value = result
  deleteConfirmVisible.value = true
}

async function deleteSelectedRules() {
  const ruleIds = selectedRules.value.map(r => r.rule_id)
  const result = await api.deleteRules(ruleIds)
  ElMessage.success(`已删除 ${result.deleted_rules} 条规则`)
  deleteConfirmVisible.value = false
  selectedRules.value = []
  loadRules()
}
</script>
```

### 4.3 告警中心页面 (Alerts.vue)

#### 界面布局

```
┌──────────────────────────────────────────────────────────────────────┐
│ [严重程度 ▼] [状态 ▼] [判定来源 ▼] [搜索主机或MITRE] [查询] [AI降噪] [批量删除] │
├──────────────────────────────────────────────────────────────────────┤
│ ☐ │ MITRE    │ 规则标题     │ 主机    │ 严重程度 │ 状态   │ 操作     │
│ ☐ │ T1113    │ 屏幕截图     │ web-01  │ [中危]   │ 已处置 │ 详情     │
│ ☐ │ T1059.004│ 反向Shell    │ db-01   │ [严重]   │ 待处置 │ 详情 阻断│
├──────────────────────────────────────────────────────────────────────┤
│                          分页器                              [已选: 0] │
└──────────────────────────────────────────────────────────────────────┘
```

#### 核心功能

```vue
<template>
  <div class="detection-alerts-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <!-- 筛选条件 -->
        <el-input v-model="query" placeholder="搜索主机或MITRE" clearable class="search-input">
          <template #append>
            <el-button :icon="Search" @click="loadAlerts" />
          </template>
        </el-input>
        <el-button type="primary" @click="loadAlerts">查询</el-button>
        <el-button type="warning" @click="showAIDenoiseDialog">AI降噪</el-button>
        <el-button type="danger" :disabled="selectedAlerts.length === 0" @click="handleBatchDelete">
          批量删除 ({{ selectedAlerts.length }})
        </el-button>
      </div>
    </el-card>

    <el-card>
      <el-table :data="alerts" border stripe @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="50" />
        <el-table-column label="MITRE" width="120">
          <template #default="{ row }">
            <el-link type="primary" @click="goToRules(row.mitre_id)">
              {{ row.mitre_id || '-' }}
            </el-link>
          </template>
        </el-table-column>
        <!-- 其他列 -->
      </el-table>
    </el-card>

    <!-- 详情对话框中的MITRE点击 -->
    <el-dialog v-model="detailVisible" title="告警详情" width="900px">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="MITRE ID">
          <el-link type="primary" @click="goToRules(selectedAlert.mitre_id)">
            {{ selectedAlert.mitre_id }}
          </el-link>
        </el-descriptions-item>
        <!-- 其他字段 -->
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

function goToRules(mitreId: string) {
  if (!mitreId) return
  router.push({ path: '/detection/rules', query: { query: mitreId } })
}

async function handleBatchDelete() {
  const alertIds = selectedAlerts.value.map(a => a.alert_id)
  await api.deleteAlerts(alertIds)
  ElMessage.success('批量删除成功')
  loadAlerts()
}
</script>
```

### 4.4 安全概览页面 (Overview.vue)

核心功能：
- 统计卡片（可点击跳转）
- MITRE矩阵可视化（点击技术跳转到告警列表）
- 告警趋势折线图

---

## 5. 状态管理

```typescript
// src/store/detection.ts
import { defineStore } from 'pinia'
import type { Alert, SigmaRule } from '@/types'
import * as api from '@/api/detection'

export const useDetectionStore = defineStore('detection', {
  state: () => ({
    // 告警
    alerts: [] as Alert[],
    alertTotal: 0,
    alertLoading: false,
    
    // 规则
    rules: [] as SigmaRule[],
    ruleTotal: 0,
    ruleLoading: false,
  }),

  actions: {
    async fetchAlerts(params: any) {
      this.alertLoading = true
      try {
        const res = await api.getAlerts(params)
        this.alerts = res.data || []
        this.alertTotal = res.total || 0
      } finally {
        this.alertLoading = false
      }
    },

    async fetchRules(params: any) {
      this.ruleLoading = true
      try {
        const res = await api.getRules(params)
        this.rules = res.data || []
        this.ruleTotal = res.total || 0
      } finally {
        this.ruleLoading = false
      }
    },

    async updateRuleStatus(ruleId: string, status: string) {
      await api.updateRuleStatus(ruleId, status)
      const index = this.rules.findIndex(r => r.rule_id === ruleId)
      if (index !== -1) {
        this.rules[index].status = status
      }
    },
  },
})
```

---

## 6. 构建配置

```json
// package.json
{
  "name": "aegis-system-frontend",
  "version": "5.2.0",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc --noEmit && vite build",
    "preview": "vite preview",
    "lint": "eslint . --ext .vue,.js,.jsx,.cjs,.mjs,.ts,.tsx",
    "type-check": "vue-tsc --noEmit"
  }
}
```

```dockerfile
# Dockerfile
FROM nginx:1.23-alpine
COPY dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

---

## 7. 构建与部署

```bash
# 构建
cd frontend
npm run build

# 构建Docker镜像
docker build -t aegis-system/frontend:latest .

# 启动服务
docker compose up -d frontend
```

---

**文档结束**
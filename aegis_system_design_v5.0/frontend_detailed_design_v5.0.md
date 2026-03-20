# 前端详细设计文档 - V5.0

**版本**: 5.0
**状态**: 设计中
**日期**: 2026-03-20

---

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 5.0 | 2026-03-20 | 安全产品团队 | **新增智能异常检测模块**。新增`/detection`一级菜单；包含安全概览（ATT&CK矩阵）、告警中心、阻断策略、规则管理四个子页面。新增`detection` API层和Pinia Store；更新路由和类型定义。 |

---

## 2. 概述

本文档为Aegis智能主机安全系统的前端应用提供V5.0版本的设计规范。V5.0引入了核心的"智能异常检测"功能，旨在为用户提供从ATT&CK矩阵可视化到告警处理、阻断策略配置、规则审核的全流程操作体验。

**技术栈**: Vue 3 (Composition API), Vite, TypeScript, Pinia, Element Plus, Axios

---

## 3. 项目结构（V5.0扩展）

```
/frontend
|-- /src
|   |-- /api
|   |   |-- ... (existing files)
|   |   |-- detection.ts           # V5.0 新增
|   |-- /components
|   |   |-- /common
|   |   |   |-- ... (existing components)
|   |   |   |-- SeverityTag.vue    # V3.0 已有，复用
|   |   |-- /detection             # V5.0 新增
|   |   |   |-- AttackMatrix.vue   # ATT&CK矩阵组件
|   |   |   |-- AlertCard.vue      # 告警卡片组件
|   |   |   |-- AlertDetailDialog.vue  # 告警详情弹窗
|   |   |   |-- BlockPolicyTable.vue   # 阻断策略表格
|   |   |   |-- RuleCard.vue       # 规则卡片组件
|   |   |   |-- RuleDetailDialog.vue   # 规则详情弹窗
|   |   |   |-- ThreatStatCards.vue    # 威胁统计卡片
|   |   |   |-- AlertTrendChart.vue    # 告警趋势图
|   |-- /store
|   |   |-- ... (existing stores)
|   |   |-- detection.ts           # V5.0 新增
|   |-- /types
|   |   |-- index.ts               # V5.0 更新
|   |-- /views
|   |   |-- ... (existing views)
|   |   |-- /detection             # V5.0 新增
|   |   |   |-- Overview.vue       # 安全概览
|   |   |   |-- Alerts.vue         # 告警中心
|   |   |   |-- Policies.vue       # 阻断策略
|   |   |   |-- Rules.vue          # 规则管理
|   |-- /router
|   |   |-- index.ts               # V5.0 更新
|   |-- ... (other files)
|-- ... (config files)
```

---

## 4. 路由（`/src/router`）

### 4.1 路由配置

```typescript
// src/router/index.ts

import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    redirect: '/hosts'
  },
  {
    path: '/hosts',
    name: 'Hosts',
    component: () => import('@/views/Dashboard.vue')
  },
  {
    path: '/baseline/workbench',
    name: 'BaselineWorkbench',
    component: () => import('@/views/Workbench.vue')
  },
  {
    path: '/baseline/tasks',
    name: 'BaselineTasks',
    component: () => import('@/views/TaskCenter.vue')
  },
  {
    path: '/vulnerability',
    name: 'Vulnerability',
    component: () => import('@/views/Vulnerability.vue')
  },
  // V5.0 新增
  {
    path: '/detection/overview',
    name: 'DetectionOverview',
    component: () => import('@/views/detection/Overview.vue')
  },
  {
    path: '/detection/alerts',
    name: 'DetectionAlerts',
    component: () => import('@/views/detection/Alerts.vue')
  },
  {
    path: '/detection/policies',
    name: 'DetectionPolicies',
    component: () => import('@/views/detection/Policies.vue')
  },
  {
    path: '/detection/rules',
    name: 'DetectionRules',
    component: () => import('@/views/detection/Rules.vue')
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('@/views/Settings.vue')
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
```

### 4.2 菜单配置

左侧菜单栏需要更新，新增"智能异常检测"一级菜单及四个子菜单：

```typescript
// 菜单数据结构
const menuItems = [
  // ... existing items
  {
    title: '智能异常检测',
    icon: 'Shield',
    children: [
      { title: '安全概览', path: '/detection/overview' },
      { title: '告警中心', path: '/detection/alerts', badge: 'alertCount' },
      { title: '阻断策略', path: '/detection/policies' },
      { title: '规则管理', path: '/detection/rules' }
    ]
  },
  { title: '系统配置', icon: 'Setting', path: '/settings' }
]
```

---

## 5. API通讯层（`/src/api/detection.ts`）

```typescript
// src/api/detection.ts

import { request } from './index'
import type {
  Alert, AlertListParams, PaginatedResponse,
  BlockPolicy,
  SigmaRule, RuleListParams,
  AttackMatrix, ThreatStatistics, AlertTrendPoint,
  RuleDetail
} from '@/types'

// ==================== ATT&CK矩阵 ====================

export function getAttackMatrix(): Promise<AttackMatrix> {
  return request.get('/api/v1/attack-matrix')
}

// ==================== 威胁统计 ====================

export function getThreatStatistics(): Promise<ThreatStatistics> {
  return request.get('/api/v1/statistics/threats')
}

export function getAlertTrend(hours: number = 24): Promise<AlertTrendPoint[]> {
  return request.get('/api/v1/statistics/alert-trend', { params: { hours } })
}

// ==================== 告警管理 ====================

export function getAlerts(params: AlertListParams): Promise<PaginatedResponse<Alert>> {
  return request.get('/api/v1/alerts', { params })
}

export function getAlertDetail(alertId: string): Promise<Alert> {
  return request.get(`/api/v1/alerts/${alertId}`)
}

export function resolveAlert(alertId: string): Promise<void> {
  return request.post(`/api/v1/alerts/${alertId}/resolve`)
}

export function blockAlert(alertId: string): Promise<void> {
  return request.post(`/api/v1/alerts/${alertId}/block`)
}

// ==================== 阻断策略 ====================

export function getBlockPolicies(): Promise<BlockPolicy[]> {
  return request.get('/api/v1/block-policies')
}

export function updateBlockPolicy(mitreId: string, data: { enabled?: boolean; auto_block?: boolean }): Promise<void> {
  return request.put(`/api/v1/block-policies/${mitreId}`, data)
}

// ==================== 规则管理 ====================

export function getRules(params: RuleListParams): Promise<PaginatedResponse<SigmaRule>> {
  return request.get('/api/v1/rules', { params })
}

export function getRuleDetail(ruleId: string): Promise<RuleDetail> {
  return request.get(`/api/v1/rules/${ruleId}`)
}

export function updateRuleStatus(ruleId: string, status: 'active' | 'disabled'): Promise<void> {
  return request.put(`/api/v1/rules/${ruleId}/status`, { status })
}
```

---

## 6. 状态管理（`/src/store/detection.ts`）

```typescript
// src/store/detection.ts

import { defineStore } from 'pinia'
import * as api from '@/api/detection'
import type {
  Alert, AlertListParams,
  BlockPolicy,
  SigmaRule, RuleListParams,
  AttackMatrix, ThreatStatistics, AlertTrendPoint
} from '@/types'

export const useDetectionStore = defineStore('detection', {
  state: () => ({
    // ATT&CK矩阵
    attackMatrix: null as AttackMatrix | null,

    // 威胁统计
    threatStats: null as ThreatStatistics | null,
    alertTrend: [] as AlertTrendPoint[],

    // 告警
    alerts: [] as Alert[],
    alertTotal: 0,
    alertLoading: false,

    // 阻断策略
    blockPolicies: [] as BlockPolicy[],
    policyLoading: false,

    // 规则
    rules: [] as SigmaRule[],
    ruleTotal: 0,
    ruleLoading: false,

    // WebSocket连接状态
    wsConnected: false
  }),

  actions: {
    // ==================== ATT&CK矩阵 ====================

    async fetchAttackMatrix() {
      this.attackMatrix = await api.getAttackMatrix()
    },

    // ==================== 威胁统计 ====================

    async fetchThreatStatistics() {
      this.threatStats = await api.getThreatStatistics()
    },

    async fetchAlertTrend(hours: number = 24) {
      this.alertTrend = await api.getAlertTrend(hours)
    },

    // ==================== 告警管理 ====================

    async fetchAlerts(params: AlertListParams) {
      this.alertLoading = true
      try {
        const response = await api.getAlerts(params)
        this.alerts = response.data
        this.alertTotal = response.total
      } finally {
        this.alertLoading = false
      }
    },

    async resolveAlert(alertId: string) {
      await api.resolveAlert(alertId)
      // 更新本地状态
      const alert = this.alerts.find(a => a.alert_id === alertId)
      if (alert) alert.status = 'resolved'
    },

    async blockAlert(alertId: string) {
      await api.blockAlert(alertId)
      // 更新本地状态
      const alert = this.alerts.find(a => a.alert_id === alertId)
      if (alert) alert.manual_blocked = true
    },

    // WebSocket推送新增告警
    addAlertFromWS(alert: Alert) {
      // 检查是否已存在（去重）
      const existing = this.alerts.find(a => a.alert_id === alert.alert_id)
      if (existing) {
        // 更新命中次数
        existing.hit_count = alert.hit_count
        existing.last_seen_at = alert.last_seen_at
      } else {
        this.alerts.unshift(alert)
        this.alertTotal++
      }
    },

    // ==================== 阻断策略 ====================

    async fetchBlockPolicies() {
      this.policyLoading = true
      try {
        this.blockPolicies = await api.getBlockPolicies()
      } finally {
        this.policyLoading = false
      }
    },

    async updateBlockPolicy(mitreId: string, data: { enabled?: boolean; auto_block?: boolean }) {
      await api.updateBlockPolicy(mitreId, data)
      // 更新本地状态
      const policy = this.blockPolicies.find(p => p.mitre_id === mitreId)
      if (policy) {
        if (data.enabled !== undefined) policy.enabled = data.enabled
        if (data.auto_block !== undefined) policy.auto_block = data.auto_block
      }
    },

    // ==================== 规则管理 ====================

    async fetchRules(params: RuleListParams) {
      this.ruleLoading = true
      try {
        const response = await api.getRules(params)
        this.rules = response.data
        this.ruleTotal = response.total
      } finally {
        this.ruleLoading = false
      }
    },

    async updateRuleStatus(ruleId: string, status: 'active' | 'disabled') {
      await api.updateRuleStatus(ruleId, status)
      // 更新本地状态
      const rule = this.rules.find(r => r.rule_id === ruleId)
      if (rule) rule.status = status
    }
  }
})
```

---

## 7. 类型定义（`/src/types/index.ts`扩展）

```typescript
// 在现有类型定义基础上新增

// ==================== V5.0 智能异常检测 ====================

// ATT&CK矩阵
export interface AttackTechnique {
  id: string          // e.g., "T1059.004"
  name: string        // e.g., "Unix Shell"
  alert_count: number
}

export interface AttackTactic {
  id: string          // e.g., "TA0002"
  name: string        // e.g., "执行"
  techniques: AttackTechnique[]
}

export interface AttackMatrix {
  tactics: AttackTactic[]
}

// 威胁统计
export interface ThreatStatistics {
  today_alerts: number
  today_blocks: number
  affected_hosts: number
  active_rules: number
}

export interface AlertTrendPoint {
  timestamp: string
  count: number
}

// 告警
export interface Alert {
  alert_id: string
  host_id: string
  hostname: string
  pid: number
  mitre_id: string
  mitre_name?: string
  severity: 'Critical' | 'High' | 'Medium' | 'Low'
  description: string
  llm_summary?: string
  hit_count: number
  auto_blocked: boolean
  manual_blocked: boolean
  status: 'active' | 'resolved'
  first_seen_at: string
  last_seen_at: string
  created_at: string
}

export interface AlertListParams {
  page?: number
  pageSize?: number
  host_id?: string
  severity?: string
  mitre_id?: string
  status?: string
  start_time?: string
  end_time?: string
}

// 阻断策略
export interface BlockPolicy {
  mitre_id: string
  mitre_name: string
  enabled: boolean
  auto_block: boolean
  action: string  // 固定为 "kill_process"
  updated_at: string
}

// 规则
export interface SigmaRule {
  rule_id: string
  title: string
  mitre_id: string
  status: 'pending' | 'experimental' | 'active' | 'disabled'
  severity: 'critical' | 'high' | 'medium' | 'low'
  generated_by: 'llm' | 'manual'
  version: string
  created_at: string
  activated_at?: string
  description?: string
}

export interface RuleVersion {
  version: string
  content: string
  change_reason: string
  created_at: string
}

export interface RuleDetail extends SigmaRule {
  content: string  // YAML格式规则内容
  versions: RuleVersion[]
}

export interface RuleListParams {
  page?: number
  pageSize?: number
  status?: string
  mitre_id?: string
  query?: string
}

// 通用分页响应
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
}

// WebSocket消息
export interface WSAlertMessage {
  type: 'alert'
  data: Alert
}

export interface WSBlockStatusMessage {
  type: 'block_status'
  data: {
    block_id: string
    host_id: string
    hostname: string
    action: string
    pid: number
    success: boolean
    timestamp: number
  }
}

export interface WSRuleUpdateMessage {
  type: 'rule_update'
  data: {
    rule_id: string
    action: string
    status: string
    reason: string
    timestamp: number
  }
}

export type WSMessage = WSAlertMessage | WSBlockStatusMessage | WSRuleUpdateMessage
```

---

## 8. 页面与组件设计

### 8.1 页面总览

| 页面 | 路由 | 组件 | 功能 |
|:---|:---|:---|:---|
| 安全概览 | `/detection/overview` | `Overview.vue` | ATT&CK矩阵、威胁统计、趋势图 |
| 告警中心 | `/detection/alerts` | `Alerts.vue` | 告警列表、筛选、手动阻断 |
| 阻断策略 | `/detection/policies` | `Policies.vue` | 阻断策略开关配置 |
| 规则管理 | `/detection/rules` | `Rules.vue` | 规则列表、状态审核 |

---

### 8.2 Overview.vue - 安全概览

**页面路径**: `/detection/overview`

**页面布局**:

```vue
<template>
  <div class="detection-overview">
    <!-- 顶部：威胁统计卡片 -->
    <ThreatStatCards :stats="threatStats" />

    <!-- 中部：ATT&CK矩阵 -->
    <el-card class="attack-matrix-card">
      <template #header>
        <span>ATT&CK 攻击矩阵</span>
      </template>
      <AttackMatrix :data="attackMatrix" @technique-click="handleTechniqueClick" />
    </el-card>

    <!-- 底部：告警趋势图 -->
    <el-card class="trend-card">
      <template #header>
        <span>24小时告警趋势</span>
      </template>
      <AlertTrendChart :data="alertTrend" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useDetectionStore } from '@/store/detection'
import ThreatStatCards from '@/components/detection/ThreatStatCards.vue'
import AttackMatrix from '@/components/detection/AttackMatrix.vue'
import AlertTrendChart from '@/components/detection/AlertTrendChart.vue'

const router = useRouter()
const store = useDetectionStore()

const threatStats = computed(() => store.threatStats)
const attackMatrix = computed(() => store.attackMatrix)
const alertTrend = computed(() => store.alertTrend)

onMounted(async () => {
  await Promise.all([
    store.fetchThreatStatistics(),
    store.fetchAttackMatrix(),
    store.fetchAlertTrend(24)
  ])
})

function handleTechniqueClick(techniqueId: string) {
  router.push({ path: '/detection/alerts', query: { mitre_id: techniqueId } })
}
</script>
```

**子组件**:

#### ThreatStatCards.vue

```vue
<template>
  <el-row :gutter="16" class="stat-cards">
    <el-col :span="6">
      <el-card>
        <el-statistic title="今日告警数" :value="stats?.today_alerts ?? 0" />
      </el-card>
    </el-col>
    <el-col :span="6">
      <el-card>
        <el-statistic title="今日阻断数" :value="stats?.today_blocks ?? 0" />
      </el-card>
    </el-col>
    <el-col :span="6">
      <el-card>
        <el-statistic title="受影响主机" :value="stats?.affected_hosts ?? 0" />
      </el-card>
    </el-col>
    <el-col :span="6">
      <el-card>
        <el-statistic title="活跃规则数" :value="stats?.active_rules ?? 0" />
      </el-card>
    </el-col>
  </el-row>
</template>
```

#### AttackMatrix.vue

ATT&CK矩阵展示组件，14个战术横向排列，每个战术卡片内显示该战术下的技术列表。

```vue
<template>
  <div class="attack-matrix">
    <el-row :gutter="8">
      <el-col v-for="tactic in data?.tactics" :key="tactic.id" :span="24/7">
        <div class="tactic-card" @click="toggleExpand(tactic.id)">
          <div class="tactic-header">
            <span class="tactic-name">{{ tactic.name }}</span>
            <span class="tactic-id">{{ tactic.id }}</span>
          </div>
          <div class="tactic-count">
            {{ getTotalAlerts(tactic) }} 告警
          </div>
          <!-- 展开的技术列表 -->
          <div v-if="expandedTactic === tactic.id" class="technique-list">
            <div
              v-for="tech in tactic.techniques"
              :key="tech.id"
              class="technique-item"
              @click.stop="$emit('technique-click', tech.id)"
            >
              <span class="tech-name">{{ tech.name }}</span>
              <el-badge :value="tech.alert_count" :hidden="tech.alert_count === 0" />
            </div>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>
```

#### AlertTrendChart.vue

使用 ECharts 绘制24小时告警趋势折线图。

---

### 8.3 Alerts.vue - 告警中心

**页面路径**: `/detection/alerts`

**页面布局**:

```vue
<template>
  <div class="detection-alerts">
    <!-- 筛选区 -->
    <el-card class="filter-card">
      <el-row :gutter="16">
        <el-col :span="4">
          <el-select v-model="filters.host_id" placeholder="主机" clearable>
            <el-option v-for="h in hosts" :key="h.id" :label="h.hostname" :value="h.id" />
          </el-select>
        </el-col>
        <el-col :span="4">
          <el-select v-model="filters.severity" placeholder="严重程度" clearable>
            <el-option label="Critical" value="critical" />
            <el-option label="High" value="high" />
            <el-option label="Medium" value="medium" />
            <el-option label="Low" value="low" />
          </el-select>
        </el-col>
        <el-col :span="4">
          <el-input v-model="filters.mitre_id" placeholder="MITRE ID" clearable />
        </el-col>
        <el-col :span="4">
          <el-select v-model="filters.status" placeholder="状态" clearable>
            <el-option label="活跃" value="active" />
            <el-option label="已解决" value="resolved" />
          </el-select>
        </el-col>
        <el-col :span="8">
          <el-date-picker
            v-model="filters.dateRange"
            type="datetimerange"
            start-placeholder="开始时间"
            end-placeholder="结束时间"
          />
        </el-col>
      </el-row>
    </el-card>

    <!-- 告警列表 -->
    <el-card v-loading="alertLoading" class="alert-list-card">
      <AlertCard
        v-for="alert in alerts"
        :key="alert.alert_id"
        :alert="alert"
        @detail="showDetail(alert)"
        @resolve="handleResolve(alert)"
        @block="handleBlock(alert)"
      />

      <el-empty v-if="!alertLoading && alerts.length === 0" description="暂无告警" />

      <!-- 分页 -->
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="alertTotal"
        layout="total, prev, pager, next"
        @current-change="loadAlerts"
      />
    </el-card>

    <!-- 告警详情弹窗 -->
    <AlertDetailDialog
      v-model="detailVisible"
      :alert="selectedAlert"
      @resolve="handleResolve"
    />
  </div>
</template>
```

#### AlertCard.vue - 告警卡片

```vue
<template>
  <div class="alert-card">
    <div class="alert-header">
      <span class="alert-id">{{ alert.alert_id }}</span>
      <el-tag :type="severityType" size="small">{{ alert.severity }}</el-tag>
      <el-tag type="info" size="small">{{ alert.mitre_id }}</el-tag>
      <span class="hostname">{{ alert.hostname }}</span>
      <span class="hit-count">命中 {{ alert.hit_count }} 次</span>
    </div>
    <div class="alert-body">
      <div class="alert-info">
        <span>进程: {{ alert.description }} (PID: {{ alert.pid }})</span>
      </div>
      <div class="alert-time">
        <span>首次: {{ alert.first_seen_at }}</span>
        <span>最近: {{ alert.last_seen_at }}</span>
      </div>
      <div class="alert-status">
        <span>状态: {{ alert.status === 'active' ? '活跃' : '已解决' }}</span>
        <span v-if="alert.auto_blocked">自动阻断: ✅ 已执行</span>
        <span v-else>自动阻断: ❌ 未开启</span>
      </div>
    </div>
    <div class="alert-actions">
      <el-button size="small" @click="$emit('detail', alert)">详情</el-button>
      <el-button v-if="alert.status === 'active'" size="small" @click="$emit('resolve', alert)">解决</el-button>
      <el-button
        v-if="!alert.auto_blocked && alert.status === 'active'"
        type="danger"
        size="small"
        @click="$emit('block', alert)"
      >
        阻断
      </el-button>
    </div>
  </div>
</template>
```

#### AlertDetailDialog.vue - 告警详情弹窗

展示告警的完整信息，包括LLM研判摘要。

---

### 8.4 Policies.vue - 阻断策略

**页面路径**: `/detection/policies`

**页面布局**:

```vue
<template>
  <div class="detection-policies">
    <el-card>
      <template #header>
        <span>阻断策略配置</span>
      </template>

      <el-table :data="blockPolicies" v-loading="policyLoading">
        <el-table-column prop="mitre_id" label="ATT&CK ID" width="120" />
        <el-table-column prop="mitre_name" label="名称" />
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              @change="(val) => handleToggle(row.mitre_id, 'enabled', val)"
            />
          </template>
        </el-table-column>
        <el-table-column label="自动阻断" width="100">
          <template #default="{ row }">
            <el-switch
              :model-value="row.auto_block"
              @change="(val) => handleToggle(row.mitre_id, 'auto_block', val)"
            />
          </template>
        </el-table-column>
        <el-table-column label="动作" width="140">
          <template #default>
            <el-tag type="info">kill_process</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>
```

---

### 8.5 Rules.vue - 规则管理

**页面路径**: `/detection/rules`

**页面布局**:

```vue
<template>
  <div class="detection-rules">
    <!-- 筛选区 -->
    <el-card class="filter-card">
      <el-row :gutter="16">
        <el-col :span="6">
          <el-select v-model="filters.status" placeholder="状态" clearable>
            <el-option label="待审核" value="pending" />
            <el-option label="实验性" value="experimental" />
            <el-option label="正式" value="active" />
            <el-option label="禁用" value="disabled" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <el-input v-model="filters.mitre_id" placeholder="MITRE ID" clearable />
        </el-col>
        <el-col :span="6">
          <el-input v-model="filters.query" placeholder="搜索规则名称" clearable />
        </el-col>
      </el-row>
    </el-card>

    <!-- 规则列表 -->
    <el-card v-loading="ruleLoading" class="rule-list-card">
      <RuleCard
        v-for="rule in rules"
        :key="rule.rule_id"
        :rule="rule"
        @detail="showDetail(rule)"
        @approve="handleApprove(rule)"
        @disable="handleDisable(rule)"
      />

      <el-empty v-if="!ruleLoading && rules.length === 0" description="暂无规则" />

      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="ruleTotal"
        layout="total, prev, pager, next"
        @current-change="loadRules"
      />
    </el-card>

    <!-- 规则详情弹窗 -->
    <RuleDetailDialog
      v-model="detailVisible"
      :rule="selectedRule"
      @approve="handleApprove"
      @disable="handleDisable"
    />
  </div>
</template>
```

#### RuleCard.vue - 规则卡片

```vue
<template>
  <div class="rule-card">
    <div class="rule-header">
      <span class="rule-id">{{ rule.rule_id }}</span>
      <el-tag :type="statusType" size="small">{{ statusText }}</el-tag>
      <el-tag type="info" size="small">{{ rule.mitre_id }}</el-tag>
    </div>
    <div class="rule-body">
      <span>版本: {{ rule.version }}</span>
      <span>来源: {{ rule.generated_by === 'llm' ? 'LLM自动生成' : '人工创建' }}</span>
      <span>创建: {{ rule.created_at }}</span>
    </div>
    <div class="rule-desc" v-if="rule.description">{{ rule.description }}</div>
    <div class="rule-actions">
      <el-button size="small" @click="$emit('detail', rule)">详情</el-button>
      <el-button
        v-if="rule.status === 'pending' || rule.status === 'experimental'"
        type="success"
        size="small"
        @click="$emit('approve', rule)"
      >
        批准
      </el-button>
      <el-button
        v-if="rule.status !== 'disabled'"
        type="danger"
        size="small"
        @click="$emit('disable', rule)"
      >
        禁用
      </el-button>
    </div>
  </div>
</template>
```

#### RuleDetailDialog.vue - 规则详情弹窗

展示规则的YAML内容、版本历史，支持批准/禁用操作。

---

## 9. WebSocket集成

### 9.1 WebSocket连接

```typescript
// src/utils/websocket.ts

import { useDetectionStore } from '@/store/detection'
import type { WSMessage } from '@/types'

class DetectionWebSocket {
  private ws: WebSocket | null = null
  private reconnectTimer: number | null = null

  connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/runtime/ws`)

    this.ws.onopen = () => {
      console.log('[DetectionWS] Connected')
      const store = useDetectionStore()
      store.wsConnected = true
    }

    this.ws.onmessage = (event) => {
      const message: WSMessage = JSON.parse(event.data)
      this.handleMessage(message)
    }

    this.ws.onclose = () => {
      console.log('[DetectionWS] Disconnected')
      const store = useDetectionStore()
      store.wsConnected = false
      // 5秒后重连
      this.reconnectTimer = window.setTimeout(() => this.connect(), 5000)
    }

    this.ws.onerror = (err) => {
      console.error('[DetectionWS] Error:', err)
    }
  }

  private handleMessage(message: WSMessage) {
    const store = useDetectionStore()

    switch (message.type) {
      case 'alert':
        store.addAlertFromWS(message.data)
        // 显示通知
        ElNotification({
          title: '安全告警',
          message: message.data.description,
          type: message.data.severity === 'Critical' ? 'error' : 'warning',
          duration: 5000
        })
        break

      case 'block_status':
        // 更新告警的阻断状态
        // ...
        break

      case 'rule_update':
        // 刷新规则列表
        store.fetchRules({})
        break
    }
  }

  disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
    }
    this.ws?.close()
  }
}

export const detectionWS = new DetectionWebSocket()
```

### 9.2 在App.vue中初始化

```typescript
// App.vue
import { detectionWS } from '@/utils/websocket'

onMounted(() => {
  detectionWS.connect()
})

onUnmounted(() => {
  detectionWS.disconnect()
})
```

---

## 10. 数据流图

```
用户操作               前端页面                 Pinia Store              后端API
   │                      │                        │                       │
   ├─访问安全概览──────→│                        │                       │
   │                      ├─fetchAttackMatrix()──→│                       │
   │                      │                        ├─GET /attack-matrix──→│
   │                      │                        │ ←──AttackMatrix─────│
   │                      ├─fetchThreatStats()────→│                       │
   │                      │                        ├─GET /stats/threats──→│
   │                      │                        │ ←──Statistics───────│
   │                      │                        │                       │
   ├─访问告警中心──────→│                        │                       │
   │                      ├─fetchAlerts(params)───→│                       │
   │                      │                        ├─GET /alerts─────────→│
   │                      │                        │ ←──Alert[]──────────│
   │                      │                        │                       │
   │  ←──WebSocket推送───┤  addAlertFromWS()────→│                       │
   │                      │                        │                       │
   ├─点击"阻断"─────────→│                        │                       │
   │                      ├─blockAlert(id)────────→│                       │
   │                      │                        ├─POST /alerts/:id/block→│
   │                      │                        │                       │
   ├─访问阻断策略──────→│                        │                       │
   │                      ├─fetchPolicies()───────→│                       │
   │                      │                        ├─GET /block-policies──→│
   │                      │                        │ ←──Policy[]─────────│
   │                      │                        │                       │
   ├─切换开关───────────→│                        │                       │
   │                      ├─updatePolicy(id, data)→│                       │
   │                      │                        ├─PUT /policies/:id───→│
   │                      │                        │                       │
   ├─访问规则管理──────→│                        │                       │
   │                      ├─fetchRules(params)────→│                       │
   │                      │                        ├─GET /rules──────────→│
   │                      │                        │ ←──Rule[]───────────│
   │                      │                        │                       │
   ├─点击"批准"─────────→│                        │                       │
   │                      ├─updateRuleStatus(id,   │                       │
   │                      │   "active")───────────→│                       │
   │                      │                        ├─PUT /rules/:id/status→│
```

---

## 11. 全局入口集成

### 11.1 App.vue更新

在现有App.vue中集成WebSocket连接：

```typescript
// App.vue
import { detectionWS } from '@/utils/websocket'

onMounted(() => {
  detectionWS.connect()
})

onUnmounted(() => {
  detectionWS.disconnect()
})
```

### 11.2 路由守卫

```typescript
// src/router/index.ts
router.beforeEach((to, from, next) => {
  // 检查LLM配置状态
  // 如果访问/detection/*页面且未配置LLM，显示提示
  next()
})
```

---

## 12. 样式与主题

### 12.1 告警严重程度颜色

```typescript
const severityColors = {
  Critical: '#F56C6C',
  High: '#E6A23C',
  Medium: '#409EFF',
  Low: '#909399'
}
```

### 12.2 规则状态颜色

```typescript
const ruleStatusColors = {
  pending: '#909399',      // 灰色
  experimental: '#E6A23C', // 橙色
  active: '#67C23A',       // 绿色
  disabled: '#F56C6C'      // 红色
}
```

---

## 13. 异常处理

- **API请求失败**: Axios拦截器捕获错误，使用`ElMessage.error`显示全局错误提示
- **WebSocket断连**: 自动重连（5秒间隔），重连期间显示离线提示
- **LLM超时**: 告警详情中显示"LLM分析中"状态
- **阻断失败**: 告警卡片显示红色"阻断失败"标签

---

**文档结束**

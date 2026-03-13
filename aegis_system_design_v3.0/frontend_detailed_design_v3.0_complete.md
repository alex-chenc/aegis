# 前端项目详细设计文档 - V3.0 完整版

**版本**: 3.0
**状态**: 定稿
**作者**: 安全产品团队
**日期**: 2026-03-13

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 3.0 | 2026-03-13 | 安全产品团队 | **新增智能漏洞检查与修复模块**。根据PRD v3.0，重构导航菜单；新增`/vulnerability`页面及相关组件（扫描、列表、修复、POC）；新增`vulnerability` API层和Pinia Store；更新类型定义。 |
| 2.15 | 2026-03-12 | Sisyphus | 任务详情修复按钮与类型筛选。 |
| 2.14 | 2026-03-12 | Sisyphus | 任务中心表格布局优化。 |
| 2.13 | 2026-03-12 | Sisyphus | 任务超时与删除修复。 |
| 2.12 | 2026-03-11 | Sisyphus | 脚本生成校验。 |

## 2. 概述

本文档为Aegis智能主机安全系统的前端应用提供全面、可执行的设计规范。V3.0版本引入了核心的"智能漏洞检查与修复"功能，旨在为用户提供从漏洞发现到修复的全流程闭环操作体验。

*   **技术栈**: Vue 3 (Composition API), Vite, TypeScript, Pinia, Element Plus, Axios

## 3. 项目结构

为支持新功能，项目结构扩展如下：

```
/frontend
|-- /src
|   |-- /api
|   |   |-- ... (existing files)
|   |   |-- vulnerability.ts # V3.0 新增
|   |-- /components
|   |   |-- /common
|   |   |   |-- ... (existing components)
|   |   |   |-- SeverityTag.vue       # V3.0 新增
|   |   |   |-- ScriptPreview.vue     # V3.0 新增
|   |-- /store
|   |   |-- ... (existing stores)
|   |   |-- vulnerability.ts # V3.0 新增
|   |-- /types
|   |   |-- index.ts         # 统一业务类型定义
|   |-- /views
|   |   |-- ... (existing views)
|   |   |-- Vulnerability.vue# V3.0 新增
|   |-- /router
|   |   |-- index.ts         # V3.0 更新
|   |-- ... (other files)
|-- ... (config files)
```

## 4. 路由 (`/src/router`)

路由配置更新，加入新的"智能漏洞检查与修复"页面。

```typescript
import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    redirect: '/hosts' // 重定向到主机列表
  },
  {
    path: '/hosts',
    name: 'Hosts',
    component: () => import('@/views/Dashboard.vue') // 复用Dashboard作为主机列表
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
    path: '/vulnerability', // V3.0 新增
    name: 'Vulnerability',
    component: () => import('@/views/Vulnerability.vue')
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

## 5. API 通讯层 (`/src/api`)

新增 `vulnerability.ts` 模块用于处理与漏洞相关的所有后端API交互。

```typescript
// src/api/vulnerability.ts

import { request } from './index';

// 触发漏洞扫描
export function startVulnerabilityScan(hostIds: string[]): Promise<{ scan_id: string }> {
  return request.post('/api/v1/vulnerability/scan', { host_ids: hostIds });
}

// 获取扫描状态
export function getVulnerabilityScanStatus(scanId: string): Promise<ScanStatus> {
  return request.get(`/api/v1/vulnerability/scan/${scanId}/status`);
}

// 获取漏洞列表
export function getVulnerabilities(params: VulnerabilityListParams): Promise<PaginatedResponse<Vulnerability>> {
  return request.get('/api/v1/vulnerability', { params });
}

// 生成修复脚本
export function generateFixScript(cveId: string, hostIds: string[]): Promise<{ script: string }> {
  return request.post(`/api/v1/vulnerability/${cveId}/fix`, { host_ids: hostIds });
}

// 生成POC验证脚本
export function generatePocScript(cveId: string, hostId: string): Promise<{ script: string }> {
  return request.post(`/api/v1/vulnerability/${cveId}/poc`, { host_id: hostId });
}
```

## 6. 状态管理 (Pinia) (`/src/store`)

新增 `vulnerability.ts` store 来管理漏洞模块的全局状态。

```typescript
// src/store/vulnerability.ts

import { defineStore } from 'pinia';
import * as api from '@/api/vulnerability';

export const useVulnerabilityStore = defineStore('vulnerability', {
  state: () => ({
    vulnerabilities: [] as Vulnerability[],
    total: 0,
    loading: false,
    scanStatus: null as ScanStatus | null,
  }),
  actions: {
    async fetchVulnerabilities(params: VulnerabilityListParams) {
      this.loading = true;
      try {
        const response = await api.getVulnerabilities(params);
        this.vulnerabilities = response.data;
        this.total = response.total;
      } finally {
        this.loading = false;
      }
    },
    async startScan(hostIds: string[]) {
      const { scan_id } = await api.startVulnerabilityScan(hostIds);
      // Start polling for status
      this.pollScanStatus(scan_id);
    },
    async pollScanStatus(scanId: string) {
      this.scanStatus = await api.getVulnerabilityScanStatus(scanId);
      if (this.scanStatus.status === 'scanning' || this.scanStatus.status === 'analyzing') {
        setTimeout(() => this.pollScanStatus(scanId), 5000);
      } else {
        // Scan finished, refresh the list
        this.fetchVulnerabilities({});
      }
    },
    // ... other actions for fix and poc
  },
});
```

## 7. 类型定义 (`/src/types/index.ts`)

在现有类型定义基础上，新增漏洞管理相关的接口。

```typescript
// src/types/index.ts

// ... (existing interfaces)

// V3.0 新增
export interface Vulnerability {
  cve_id: string;
  severity: 'Critical' | 'High' | 'Medium' | 'Low';
  description: string;
  affected_packages: string[];
  affected_hosts_count: number;
  discovered_at: string;
}

export interface VulnerabilityListParams {
  page?: number;
  pageSize?: number;
  severity?: string[];
  query?: string;
  date_range?: [string, string];
}

export interface ScanStatus {
  status: 'pending' | 'scanning' | 'analyzing' | 'completed' | 'failed';
  progress: number;
  message: string;
}
```

## 8. 页面与组件设计

### 8.1 `Vulnerability.vue` - 智能漏洞检查与修复页面

此页面是V3.0新增的核心页面。

**页面布局**:
-   **顶部操作区**: `el-card` 包含主机选择器 (`ElSelect`)、一键扫描按钮 (`ElButton`)、筛选器（严重程度、日期范围）和搜索框。
-   **中部数据区**: `el-card` 包含漏洞列表 (`ElTable`) 和分页 (`ElPagination`)。
-   **底部统计区**: 一行 `el-card`，使用 `el-col` 和 `el-statistic` 展示漏洞统计数据。

**核心功能**:
1.  **主机选择**: 多选主机，未选择时"一键扫描"禁用。
2.  **一键扫描**:
    -   点击后调用 `useVulnerabilityStore` 的 `startScan` action。
    -   扫描期间，表格区域显示骨架屏和扫描进度提示。
3.  **漏洞列表**:
    -   表格列定义严格遵循 PRD 6.2.2 节。
    -   `severity` 列使用 `SeverityTag.vue` 组件。
    -   `affected_hosts` 列提供展开功能，点击后在行内显示受影响主机的详细列表。
    -   操作列包含"一键修复"和"POC验证"按钮。
4.  **修复与验证**:
    -   点击"一键修复"或"POC验证"按钮，弹出 `FixConfirmationDialog.vue` 对话框。
    -   对话框内部处理脚本生成、预览和执行确认。

### 8.2 `SeverityTag.vue` - 严重程度标签组件

一个简单的展示性组件，根据传入的`severity` prop 显示不同颜色和类型的 `ElTag`。

```vue
<!-- src/components/common/SeverityTag.vue -->
<template>
  <el-tag :type="tagType" :effect="effect">{{ severity }}</el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  severity: 'Critical' | 'High' | 'Medium' | 'Low';
}>();

const tagType = computed(() => {
  switch (props.severity) {
    case 'Critical': return 'danger';
    case 'High': return 'warning';
    case 'Medium': return 'info';
    case 'Low': return 'success';
    default: return 'info';
  }
});

const effect = computed(() => (props.severity === 'Critical' || props.severity === 'High') ? 'dark' : 'light');
</script>
```

### 8.3 `FixConfirmationDialog.vue` - 修复/验证对话框组件

一个可复用的对话框组件，用于处理修复和POC验证流程。

**Props**:
-   `mode: 'fix' | 'poc'`
-   `cve: Vulnerability`
-   `visible: boolean`

**内部状态**:
-   `script: Ref<string>`
-   `loadingScript: Ref<boolean>`
-   `selectedHosts: Ref<string[]>`

**交互逻辑**:
1.  对话框打开时，显示CVE信息和受影响主机列表。
2.  用户点击"生成脚本"按钮，调用 `generateFixScript` 或 `generatePocScript` API。
3.  脚本返回后，显示在 `ScriptPreview.vue` 组件中。
4.  用户确认后，发出 `execute` 事件，由父组件处理实际的执行逻辑。

### 8.4 `ScriptPreview.vue` - 脚本预览组件

用于在对话框中展示LLM生成的脚本。

**Props**:
-   `script: string`
-   `language: 'shell'`

**功能**:
-   使用 `highlight.js` 或类似库进行语法高亮。
-   提供复制到剪贴板的功能。
-   显示安全风险提示。

## 9. 异常处理

遵循 PRD 第7节的设计，在UI层面提供明确的错误反馈。

-   **API请求失败**: Axios 拦截器捕获错误，使用 `ElMessage.error` 显示全局错误提示。
-   **LLM超时/生成失败**: 在对话框内显示 `ElAlert`，类型为 `warning`，并提供"重试"按钮。
-   **Agent离线/脚本执行失败**: 在漏洞列表或任务详情中，用红色标签或图标标记失败状态，并提供查看日志的入口。

## 10. 数据流图

```
用户操作                Vulnerability.vue         useVulnerabilityStore         后端 API
   │                          │                           │                         │
   ├─选择主机, 点击扫描───→│                           │                         │
   │                          ├─startScan(hostIds)──────→│                         │
   │                          │                           ├─POST /vuln/scan───────→ │
   │                          │                           │ ←──scan_id──────────── │
   │                          │                           │                         │
   │                          │                           ├─(轮询)GET /scan/status→ │
   │                          │                           │ ←──ScanStatus───────── │
   │                          │                           │                         │
   │                          │ 刷新列表──────────────────→│                         │
   │                          │                           ├─GET /vulnerability────→ │
   │                          │                           │ ←──Vulnerability[]──── │
   │                          │                           │                         │
   │                          │ 渲染列表<──────────────────┤                         │
   │                          │                           │                         │
   ├─点击"一键修复"──────────→│                           │                         │
   │                          ├─打开修复对话框────────────→│                         │
   │                          │ (对话框内部)              │                         │
   │                          │  └─生成脚本───────────────→│                         │
   │                          │                              ├─POST /vuln/{id}/fix─→ │
   │                          │                              │ ←──{script}────────── │
   │                          │  └─预览脚本<──────────────────┤                         │
   │                          │                              │                         │
   │                          │  └─确认执行─────────────────→│ ...                     │
   │                                                      │                         │
```

# 前端项目详细设计文档 - V2.13 完整版

**版本**: 2.13
**状态**: 定稿
**作者**: Manus AI, Sisyphus

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 2.13 | 2026-03-12 | Sisyphus | **任务超时与删除修复**。新增：TaskCenter支持timeout状态筛选和显示；TaskDetail支持超时状态显示（检测超时/修复超时）；超时任务支持重新下发；修复任务删除API调用（deleteTaskGroup用于任务组删除，deleteTask用于单个任务删除）。 |
| 2.12 | 2026-03-11 | Sisyphus | **脚本生成校验**。新增：下发检测/修复任务前校验脚本生成状态；计算属性checkScriptStatus/fixScriptStatus检查选中规则的脚本状态；el-alert组件显示警告提示；下发按钮禁用逻辑增加脚本状态校验。 |
| 2.11 | 2026-03-11 | Sisyphus | **UI重构**。规则列表改为大框架设计（一个"规则列表"卡片内包含所有文件分组）；文件分页改为每页5个；添加滚动条（最大高度600px，自定义滚动条样式）；"一键生成检测/修复脚本"按钮移至文件头部右侧；UI美化（圆角卡片、悬浮效果、文件图标、规则数量badge、分离背景）；修复批量脚本生成函数名冲突导致的无限递归问题。 |
| 2.10 | 2026-03-11 | Sisyphus | **规则列表增强**。新增：规则列表分页（每个文件下规则表格支持分页）；检测内容/修复方法列显示（支持tooltip悬浮查看）；一键生成检测/修复脚本按钮（批量生成，并发数2）；已上传文件列表分页。 |
| 2.9 | 2026-03-11 | Sisyphus | **文件管理增强**。新增：MD5去重（前端计算MD5，重复时弹窗提示）；文件大小限制（>5MB禁止上传）；规则列表按文件分组（卡片+折叠面板）；显示解析时间（绝对时间格式）；删除文件功能（弹窗确认后删除文件及规则）。 |
| 2.7 | 2026-03-11 | Metis | **Bug 修复：解析状态刷新优化**。修复 Workbench 组件中 refreshStatus 函数在 completed 状态下刷新时未重置 parseStatus 的问题。现在刷新 completed 状态会先显示加载状态，然后加载规则列表。 |
| 2.6 | 2026-03-11 | Sisyphus | **UI优化**。移除冗余的"脚本状态"列，修复脚本按钮对齐问题，优化解析状态重置逻辑。后端自动为缺少shebang的脚本添加#!/bin/bash。 |
| 2.5 | 2026-03-09 | Sisyphus | **LLM 配置显示优化**。API Key 支持脱敏显示（sk-****-ea1a），点击眼睛图标可查看完整 API Key。后端添加获取完整 API Key 接口 (/api/v1/config/llm/full-key)。 |
| 2.4 | 2026-03-09 | Sisyphus | **动态 LLM 配置支持**。前端现在正确显示数据库中保存的 LLM 配置，支持刷新页面后持久化显示。复制功能支持降级方案 (execCommand)。 |
| 2.3 | 2026-03-09 | Sisyphus | **修复实现问题**。修复复制按钮 (支持降级方案)，扩展规则列表字段 (检测内容、修复内容、版本)，修复复制功能，实现真实 LLM 连接测试。 |
| 2.2 | 2026-03-09 | Sisyphus | **完整实现**。实现 Workbench 完整功能（规则列表、脚本查看、主机选择、任务下发），修复 Settings 复制按钮，统一 TypeScript 类型定义，添加任务 API 层和 Store，实现前后端数据贯通。 |
| 1.6 | 2026-03-05 | Manus AI | **完整重写**。确保文档独立、完整，包含所有模块的详细设计，移除 WebSocket，明确数据更新策略，移除所有外部引用。 |
| 1.5 | 2026-03-05 | Manus AI | 移除 WebSocket，改为刷新更新，细化组件规范。 |

## 2. 概述

本文档为自动化基线检查与自愈系统的前端应用提供全面、可执行的设计规范。前端项目旨在为用户提供一个清晰、高效、易于操作的界面来管理主机、执行基线检查和查看结果。

*   **技术栈**: Vue 3 (Composition API), Vite, TypeScript, Pinia, Element Plus, Axios

## 3. 项目结构

```
/frontend
|-- /src
|   |-- /api             # API 通讯层，按模块划分
|   |   |-- index.ts     # Axios 实例和拦截器
|   |   |-- hosts.ts     # 主机相关 API
|   |   |-- config.ts    # 配置相关 API
|   |   |-- templates.ts # 模板相关 API
|   |   |-- tasks.ts     # 任务相关 API (新增)
|   |-- /assets          # 静态资源 (CSS, images, fonts)
|   |-- /components      # 全局可复用组件
|   |   |-- /base        # 基础原子组件 (BaseTable, BaseButton)
|   |   |-- /common      # 通用业务组件 (LogTerminal, PageHeader)
|   |-- /composables     # 可复用的 Vue Composition API 函数
|   |   |-- usePagination.ts # 封装分页逻辑
|   |-- /layouts         # 页面布局组件 (DefaultLayout.vue)
|   |-- /router          # 路由配置 (index.ts)
|   |-- /store           # 全局状态管理 (Pinia)
|   |   |-- config.ts    # 配置状态管理
|   |   |-- hosts.ts     # 主机状态管理
|   |   |-- tasks.ts     # 任务状态管理 (新增)
|   |-- /types           # TypeScript 类型定义 (新增)
|   |   |-- index.ts     # 统一业务类型定义
|   |-- /utils           # 工具函数 (e.g., date formatting)
|   |-- /views           # 页面级组件
|   |   |-- Settings.vue # 设置页面
|   |   |-- Dashboard.vue# 仪表盘页面
|   |   |-- Workbench.vue# 工作台页面
|   |-- App.vue          # 根组件
|   |-- main.ts          # 入口文件
|-- package.json
|-- vite.config.ts
|-- tsconfig.json
|-- Makefile
|-- build.sh
|-- Dockerfile
```

## 4. 数据获取与更新策略

**核心策略**: 系统不使用任何实时通讯技术（如 WebSocket）。所有数据的更新都由用户操作显式触发。

1.  **页面加载时获取**: 每个需要展示后端数据的页面，都在其 `setup` 函数的 `onMounted` 生命周期钩子中，调用对应的 Pinia Store action 来从后端获取初始数据。在数据返回前，页面应显示加载状态（例如，表格显示 `loading` 骨架屏）。
2.  **用户手动刷新**: 关键数据展示区域（如资产大盘、任务列表）必须提供一个明确的"刷新"按钮。用户点击该按钮会再次调用相应的 Pinia action 来获取最新数据，并重新渲染视图。
3.  **操作后刷新**: 当用户执行一个会改变后端数据的操作后（例如，下发一个检查任务、保存配置），应在操作的 API 调用成功后，立即主动调用相关的 Pinia action 来刷新当前页面的数据，为用户提供即时、准确的反馈。

## 5. API 通讯层 (`/src/api`)

*   **`index.ts`**: 创建并导出一个 Axios 实例。配置请求拦截器，为每个请求附加 `Authorization` Header（如果 Token 存在）。配置响应拦截器，用于统一处理 API 错误（例如，`401` 未授权则跳转到登录页，`5xx` 则显示全局错误提示）。
*   **模块化**: 每个业务模块（如 `hosts.ts`, `tasks.ts`）都封装该模块相关的 API 请求函数，这些函数内部使用上述创建的 Axios 实例。

### 5.1 API 模块详解

#### `hosts.ts` - 主机管理 API
```typescript
export function getHosts(params?: { page?: number; pageSize?: number; query?: string }): Promise<Host[]>
export function getHost(id: string): Promise<Host>
```

#### `config.ts` - 配置管理 API
```typescript
export function getLLMConfig(): Promise<LLMConfig>
export function saveLLMConfig(data: { api_key: string; base_url: string; model_name: string }): Promise<void>
export function testLLMConnection(data: { api_key: string; base_url: string; model_name: string }): Promise<void>
export function getInstallCommand(): Promise<InstallCommand>
```

#### `templates.ts` - 模板管理 API
```typescript
export function uploadTemplate(file: File): Promise<{ template_id: string }>
export function getTemplates(params?: { page?: number; pageSize?: number }): Promise<Template[]>
export function getTemplateStatus(id: string): Promise<ParseStatus>
export function getTemplateRules(id: string): Promise<BaselineRule[]>
export function deleteTemplate(id: string): Promise<void>
export function batchGenerateScripts(templateId: string, scriptType: 'CHECK' | 'FIX'): Promise<BatchGenerateResponse>
```

#### `tasks.ts` - 任务管理 API (新增)
```typescript
export function runCheck(data: RunCheckRequest): Promise<RunTaskResponse>
export function runFix(data: RunFixRequest): Promise<RunTaskResponse>
export function getTaskLogs(taskGroupId: string): Promise<TaskLog[]>
```

## 6. 状态管理 (Pinia) (`/src/store`)

### 6.1 `useConfigStore`

*   **State**: `llmConfig: Ref<LLMConfig | null>`, `installCommand: Ref<InstallCommand | null>`, `loading: Ref<boolean>`
*   **Actions**:
    *   `fetchLLMConfig()`: 调用 API 获取 LLM 配置。
    *   `saveLLMConfig(apiKey, baseURL, modelName)`: 保存配置。
    *   `testConnection(apiKey, baseURL, modelName)`: 测试连通性。
    *   `fetchInstallCommand()`: 获取 Agent 安装命令。

### 6.2 `useHostStore`

*   **State**: `hosts: Ref<Host[]>`, `total: Ref<number>`, `loading: Ref<boolean>`
*   **Actions**:
    *   `fetchHosts(page?, pageSize?, query?)`: 获取主机列表。

### 6.3 `useTaskStore` (新增)

*   **State**: 
    *   `selectedRules: Ref<BaselineRule[]>` - 用户选中的规则
    *   `selectedHostIds: Ref<string[]>` - 用户选中的主机ID
    *   `currentTaskGroupId: Ref<string | null>` - 当前任务组ID
    *   `taskLogs: Ref<TaskLog[]>` - 任务日志
    *   `loading: Ref<boolean>`
*   **Getters**:
    *   `selectedRuleIds`: 返回选中规则的ID数组
    *   `hasSelection`: 检查是否同时选中了规则和主机
*   **Actions**:
    *   `setSelectedRules(rules)`: 设置选中的规则。
    *   `toggleRule(rule)`: 切换单个规则的选中状态。
    *   `setSelectedHosts(hostIds)`: 设置选中的主机。
    *   `clearSelection()`: 清空选择。
    *   `executeCheck()`: 下发检查任务。
    *   `executeFix()`: 下发修复任务。
    *   `fetchTaskLogs(taskGroupId)`: 获取任务日志。

## 7. 类型定义 (`/src/types/index.ts`) (新增)

统一的 TypeScript 类型定义文件，确保前后端数据类型一致：

```typescript
export interface Host {
  id: string
  ip_address: string
  hostname: string
  os_type: string
  agent_version: string
  last_heartbeat_at: string
  online: boolean
  created_at: string
}

export interface Template {
  id: string
  name: string
  file_type: string
  status: 'parsing' | 'completed' | 'failed'
  error_message?: string
  rule_count: number
  created_at: string
  updated_at: string
}

export interface BaselineRule {
  id: string
  template_id: string
  title: string
  check_content: string
  fix_content: string
  generated_check_script?: string
  generated_fix_script?: string
  check_script_version: number
  fix_script_version: number
  check_script_status: 'pending' | 'generating' | 'generated' | 'failed'
  fix_script_status: 'pending' | 'generating' | 'generated' | 'failed'
  check_script_error?: string
  fix_script_error?: string
}

export interface ParseStatus {
  status: 'parsing' | 'completed' | 'failed'
  progress: number
  message: string
}

export interface InstallCommand {
  command: string
  server_ip: string
  http_port: number
  grpc_port: number
}

export interface LLMConfig {
  api_key_masked: string
  base_url: string
  model_name: string
  is_active: boolean
}

export interface TaskLog {
  id: string
  task_group_id: string
  rule_id: string
  host_id: string
  task_type: 'check' | 'fix'
  status: 'pending' | 'running' | 'success' | 'failed'
  script_content: string
  stdout: string
  stderr: string
  exit_code: number
  started_at: string
  finished_at: string
}
```

## 8. 页面组件设计 (`/src/views`)

### 8.1 Workbench.vue - 工作台页面 (完整实现)

工作台是核心功能页面，提供模板上传、规则查看、脚本预览、主机选择和任务下发功能。

**页面布局 (V2.11重构)**:
- 左侧 (el-col :span="16"): 
  - 模板上传卡片 (upload-card)
  - 规则列表大卡片 (rules-card) - 包含所有文件分组
- 右侧 (el-col :span="8"): 
  - 已上传文件卡片 (sidebar-card)
  - 选择执行主机卡片
  - 执行操作卡片

**规则列表卡片设计 (V2.11)**:
- 卡片标题："规则列表"，显示总规则数和文件数
- 规则容器 (rules-container): 最大高度600px，超出可滚动，自定义滚动条样式
- 每个文件显示为一个带圆角背景的区域 (file-section)
- 文件头部 (file-header): 
  - 左侧：文件图标、文件名、状态标签、规则数和时间
  - 右侧：一键生成检测脚本、一键生成修复脚本、删除按钮
- 规则折叠面板 (rules-collapse): 点击展开查看规则表格
- 底部分页：每页5个文件

**核心功能**:

1. **模板上传**: 支持 PDF、Word、YAML 格式的拖拽上传，MD5去重，5MB大小限制
2. **规则列表**: 大框架设计，文件分组显示，每页5个文件，支持滚动
3. **批量脚本生成**: "一键生成检测脚本"和"一键生成修复脚本"按钮位于文件头部右侧
4. **脚本查看**: 点击"检测"/"修复"按钮打开编辑对话框
5. **主机选择**: Checkbox列表选择执行主机，显示在线状态
6. **任务下发**: "下发检测"和"下发修复"按钮

**脚本状态校验 (V2.12)**:
- 下发检测任务前校验所有选中规则的检测脚本状态
- 下发修复任务前校验所有选中规则的修复脚本状态
- 未生成完成时禁用下发按钮，显示警告提示
- 提示信息："有 X 个检测脚本未生成，请点击'一键生成检测脚本'或单独生成"
- 正在生成时提示："有 X 个检测脚本正在生成中，请稍候..."

**计算属性**:
```typescript
const checkScriptStatus = computed(() => {
  const rules = allSelectedRules.value
  let pending = 0, generating = 0
  rules.forEach(rule => {
    if (rule.check_script_status === 'generating') generating++
    else if (rule.check_script_status !== 'generated') pending++
  })
  return { ready: pending === 0 && generating === 0, pending, generating }
})
```

**UI组件**:
```vue
<el-alert v-if="checkScriptWarning" :title="checkScriptWarning" type="warning" />
<el-button :disabled="!checkScriptStatus.ready">下发检测</el-button>
```

**UI设计规范 (V2.11)**:
- 圆角卡片 (border-radius: 12px)
- 悬浮效果 (hover时边框颜色变化和阴影)
- 文件图标 (Document图标)
- 规则数量badge
- 自定义滚动条 (宽度6px，圆角)
- 分离背景色 (#fafbfc)
- 文件区域悬浮效果

**关键代码结构**:
```vue
<template>
  <el-row :gutter="24">
    <el-col :span="16">
      <!-- 模板上传卡片 -->
      <el-card class="upload-card">...</el-card>
      
      <!-- 规则列表大卡片 -->
      <el-card class="rules-card">
        <div class="rules-container" style="max-height: 600px; overflow-y: auto">
          <div v-for="tpl in paginatedTemplates" class="file-section">
            <div class="file-header">
              <!-- 文件信息 + 操作按钮（靠右） -->
            </div>
            <el-collapse>
              <!-- 规则表格 -->
            </el-collapse>
          </div>
        </div>
        <!-- 文件分页（每页5个） -->
      </el-card>
    </el-col>
    <el-col :span="8">
      <!-- 已上传文件卡片 -->
      <!-- 主机选择卡片 -->
      <!-- 执行操作卡片 -->
    </el-col>
  </el-row>
  <!-- 脚本编辑对话框 -->
</template>
```

### 8.2 Settings.vue - 设置页面

**功能**:
1. LLM 配置表单（API Key、Base URL、模型名称）
2. 保存配置按钮
3. 测试连接按钮
4. Agent 安装命令显示（带复制按钮）

**复制按钮实现**:
```vue
<el-button @click="copyCommand" :disabled="!installCommand">
  <el-icon><CopyDocument /></el-icon>
</el-button>
```

### 8.3 Dashboard.vue - 仪表盘页面

**功能**:
- 主机列表表格（IP、主机名、系统类型、Agent版本、最后心跳、在线状态）
- 刷新按钮

## 9. 组件设计 (`/src/components`)

### 9.1 基础组件 (`/base`)

*   **`BaseTable.vue`**: 高度可配置的表格组件。
    *   **Props**: `columns: ColumnDef[]`, `data: any[]`, `loading: boolean`, `pagination: PaginationConfig`。
    *   **Events**: `@page-change`, `@sort-change`。
    *   **Slots**: `#cell-{columnKey}` (自定义单元格渲染), `#empty` (空状态)。

*   **`BaseButton.vue`**: 封装 `ElButton`。
    *   **Props**: `type`, `size`, `loading`, `disabled`, `icon`。
    *   **Features**: 内置点击节流，防止 300ms 内的重复点击。

### 9.2 通用组件 (`/common`)

*   **`LogTerminal.vue`**: 模拟终端的日志展示组件。
    *   **Props**: `logs: LogEntry[]` (日志条目数组)。
    *   **Features**: 自动滚动到底部；`stdout` 和 `stderr` 使用不同颜色；支持清空日志。

*   **`PageHeader.vue`**: 页面顶部的标题和操作区。
    *   **Props**: `title: string`, `subtitle: string`。
    *   **Slots**: `#extra` (用于放置刷新按钮、操作按钮组等)。

*   **`FileUpload.vue`**: 封装 `ElUpload`。
    *   **Props**: `action: string`, `fileTypes: string[]`, `maxSizeMb: number`。
    *   **Events**: `@upload-success`, `@upload-error`。
    *   **Features**: 提供清晰的上传状态（上传中、成功、失败）和详细的错误提示（文件类型错误、大小超限）。

## 10. 路由 (`/src/router`)

使用 `vue-router` 并采用懒加载模式。

```typescript
import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    name: 'Workbench',
    component: () => import('@/views/Workbench.vue')
  },
  {
    path: '/dashboard',
    name: 'Dashboard',
    component: () => import('@/views/Dashboard.vue')
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

## 11. 数据流图

```
用户操作                前端组件                Pinia Store            后端 API
   │                      │                       │                     │
   ├──上传模板──────────→│                       │                     │
   │                      ├──uploadTemplate()──→│                     │
   │                      │                       ├──POST /templates/upload
   │                      │                       │←───template_id─────│
   │                      │                       │                     │
   │                      ├──轮询解析状态─────────────────────────────→│
   │                      │                       │←─ParseStatus───────│
   │                      │                       │                     │
   │                      ├──getTemplateRules()────────────────────────→│
   │                      │                       │←─BaselineRule[]────│
   │                      │                       │                     │
   ├──选择规则/主机─────→│                       │                     │
   │                      │                       │                     │
   ├──下发检测──────────→│                       │                     │
   │                      ├──executeCheck()────→│                     │
   │                      │                       ├──POST /tasks/run-check
   │                      │                       │←─task_group_id─────│
   │                      │                       │                     │
   │                      ├──getTaskLogs()────────────────────────────→│
   │                      │                       │←─TaskLog[]─────────│
```

## 12. 构建

*   **开发环境**: `npm run dev` 启动 Vite 开发服务器。
*   **生产构建**: `npm run build` 将应用打包到 `/dist` 目录。该目录包含一个 `index.html` 和压缩、混淆、tree-shaking 后的 JS/CSS 资源。
*   **Docker 镜像**: `Dockerfile` 使用 `nginx:alpine` 作为基础镜像，将 `/dist` 目录下的静态文件复制到 Nginx 的 `html` 目录中，实现生产环境的托管。

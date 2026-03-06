# 前端项目详细设计文档 - V1.6 完整版

**版本**: 2.1
**状态**: 定稿
**作者**: Manus AI

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
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
|   |   |-- settings.ts  # 配置相关 API
|   |   |-- tasks.ts     # 任务相关 API
|   |-- /assets          # 静态资源 (CSS, images, fonts)
|   |-- /components      # 全局可复用组件
|   |   |-- /base        # 基础原子组件 (BaseTable, BaseButton)
|   |   |-- /common      # 通用业务组件 (LogTerminal, PageHeader)
|   |-- /composables     # 可复用的 Vue Composition API 函数
|   |   |-- usePagination.ts # 封装分页逻辑
|   |-- /layouts         # 页面布局组件 (DefaultLayout.vue)
|   |-- /router          # 路由配置 (index.ts)
|   |-- /store           # 全局状态管理 (Pinia)
|   |   |-- config.ts
|   |   |-- hosts.ts
|   |   |-- tasks.ts
|   |-- /types           # TypeScript 类型定义
|   |   |-- api.ts       # API 响应体类型
|   |   |-- index.ts     # 通用业务类型
|   |-- /utils           # 工具函数 (e.g., date formatting)
|   |-- /views           # 页面级组件
|   |   |-- Settings.vue
|   |   |-- Dashboard.vue
|   |   |-- Workbench.vue
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
2.  **用户手动刷新**: 关键数据展示区域（如资产大盘、任务列表）必须提供一个明确的“刷新”按钮。用户点击该按钮会再次调用相应的 Pinia action 来获取最新数据，并重新渲染视图。
3.  **操作后刷新**: 当用户执行一个会改变后端数据的操作后（例如，下发一个检查任务、保存配置），应在操作的 API 调用成功后，立即主动调用相关的 Pinia action 来刷新当前页面的数据，为用户提供即时、准确的反馈。

## 5. API 通讯层 (`/src/api`)

*   **`index.ts`**: 创建并导出一个 Axios 实例。配置请求拦截器，为每个请求附加 `Authorization` Header（如果 Token 存在）。配置响应拦截器，用于统一处理 API 错误（例如，`401` 未授权则跳转到登录页，`5xx` 则显示全局错误提示）。
*   **模块化**: 每个业务模块（如 `hosts.ts`, `tasks.ts`）都封装该模块相关的 API 请求函数，这些函数内部使用上述创建的 Axios 实例。

## 6. 状态管理 (Pinia) (`/src/store`)

### 6.1 `useConfigStore`

*   **State**: `config: Ref<LlmConfig>`, `status: Ref<'idle' | 'loading' | 'success' | 'error'>`
*   **Actions**:
    *   `fetchConfig()`: 调用 `settings.ts` 中的 API，获取配置并更新 state。
    *   `updateConfig(newConfig)`: 调用 `settings.ts` 中的 API，保存配置并更新 state。
    *   `testConfig(config)`: 调用 `settings.ts` 中的 API，测试连通性。

### 6.2 `useHostStore`

*   **State**: `hosts: Ref<Host[]>`, `total: Ref<number>`, `loading: Ref<boolean>`
*   **Actions**:
    *   `fetchHosts(params)`: 接受分页和搜索参数，调用 `hosts.ts` 中的 API，获取数据并更新 state。

### 6.3 `useTaskStore`

*   **State**: `templates: Ref<Template[]>`, `rules: Ref<Map<string, Rule[]>>`, `taskLogs: Ref<Map<string, LogEntry[]>>`, `status: Ref<...>`
*   **Actions**:
    *   `uploadTemplate(file)`: 上传模板，成功后刷新 `templates` 列表。
    *   `fetchRules(templateId)`: 获取规则并存入 `rules` Map。
    *   `runCheck(ruleId, hostIds)`: 下发检查任务，并开始轮询任务日志接口。
    *   `pollLogs(taskId)`: 定时（例如每 2 秒）调用 `tasks.ts` 中的 API 获取日志，直到任务完成。

## 7. 组件设计 (`/src/components`)

### 7.1 基础组件 (`/base`)

*   **`BaseTable.vue`**: 高度可配置的表格组件。
    *   **Props**: `columns: ColumnDef[]`, `data: any[]`, `loading: boolean`, `pagination: PaginationConfig`。
    *   **Events**: `@page-change`, `@sort-change`。
    *   **Slots**: `#cell-{columnKey}` (自定义单元格渲染), `#empty` (空状态)。

*   **`BaseButton.vue`**: 封装 `ElButton`。
    *   **Props**: `type`, `size`, `loading`, `disabled`, `icon`。
    *   **Features**: 内置点击节流，防止 300ms 内的重复点击。

### 7.2 通用组件 (`/common`)

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

## 8. 路由 (`/src/router`)

使用 `vue-router` 并采用懒加载模式。

```typescript
import { createRouter, createWebHistory } from 'vue-router';

const routes = [
  {
    path: '/',
    redirect: '/dashboard/hosts',
  },
  {
    path: '/dashboard',
    component: () => import('@/layouts/DefaultLayout.vue'),
    children: [
      { path: 'hosts', component: () => import('@/views/Dashboard.vue') },
    ],
  },
  {
    path: '/workbench',
    component: () => import('@/layouts/DefaultLayout.vue'),
    children: [
      { path: '', component: () => import('@/views/Workbench.vue') },
    ],
  },
  {
    path: '/settings',
    component: () => import('@/layouts/DefaultLayout.vue'),
    children: [
      { path: '', component: () => import('@/views/Settings.vue') },
    ],
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
```

## 9. 构建

*   **开发环境**: `npm run dev` 启动 Vite 开发服务器。
*   **生产构建**: `npm run build` 将应用打包到 `/dist` 目录。该目录包含一个 `index.html` 和压缩、混淆、tree-shaking 后的 JS/CSS 资源。
*   **Docker 镜像**: `Dockerfile` 使用 `nginx:alpine` 作为基础镜像，将 `/dist` 目录下的静态文件复制到 Nginx 的 `html` 目录中，实现生产环境的托管。

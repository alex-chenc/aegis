# Aegis智能主机安全系统 V5.5 前端详细设计文档

**版本**: 5.5
**日期**: 2026-03-30
**状态**: 定稿

---

## 1. 项目概述

V5.5版本前端基于Vue 3 + TypeScript + Vite + Element Plus构建，主要新增了Agent管理界面、智能配置面板和微服务状态监控功能。

---

## 2. 项目结构

```
/frontend
├── src/
│   ├── api/
│   │   ├── detection.ts        # 异常检测API
│   │   ├── hosts.ts            # 主机管理API
│   │   ├── agent.ts            # Agent管理API (V5.5新增)
│   │   └── ...
│   │
│   ├── views/
│   │   ├── detection/
│   │   │   ├── Overview.vue    # 安全概览
│   │   │   ├── Alerts.vue      # 告警中心
│   │   │   ├── Policies.vue    # 阻断策略
│   │   │   └── Rules.vue       # 规则管理
│   │   │
│   │   ├── agent/              # Agent管理 (V5.5新增)
│   │   │   ├── List.vue        # Agent列表
│   │   │   ├── Status.vue      # Agent状态
│   │   │   └── Config.vue      # Agent智能配置
│   │   │
│   │   └── system/
│   │       └── Services.vue    # 微服务状态 (V5.5新增)
│   │
│   ├── store/
│   │   ├── detection.ts
│   │   └── agent.ts            # Agent状态管理 (V5.5新增)
│   │
│   ├── types/
│   │   └── index.ts
│   │
│   └── components/
│       ├── ProcessTree.vue
│       └── AlertDetail.vue
│
├── package.json
├── vite.config.ts
└── Dockerfile
```

---

## 3. V5.5新增功能

### 3.1 Agent管理界面

```typescript
// Agent相关类型 (V5.5新增)
interface Agent {
  id: string
  agent_id: string
  hostname: string
  ip_address: string
  os_type: string
  agent_version: string
  status: 'online' | 'offline' | 'unknown'
  last_heartbeat_at: string
  capabilities: string[]
  
  // 智能配置
  intelligence_config: {
    fork_threshold: number
    exec_threshold: number
    network_threshold: number
    window_size: number
    auto_block_enabled: boolean
    local_block_enabled: boolean
  }
  
  // 运行时指标
  metrics: {
    cpu_percent: number
    memory_mb: number
    event_count: number
  }
}
```

### 3.2 页面设计

#### 3.2.1 Agent列表页面

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Agent管理                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ [搜索框] [状态筛选▼] [在线/离线]                                    │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 主机名      IP地址      Agent版本   状态    最后心跳    操作        │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ server-01  192.168.1.10  v3.0.0      在线    10秒前     [详情][配置] │   │
│  │ server-02  192.168.1.11  v3.0.0      离线    30分钟前   [详情][配置] │   │
│  │ server-03  192.168.1.12  v3.0.0      在线    5秒前      [详情][配置] │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  分页: 上一页  1/10  下一页                                     共100条    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 3.2.2 Agent智能配置面板

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Agent智能配置                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  基本配置                                                                    │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ 滑动窗口大小: [5秒 ▼]                                               │    │
│  │ 事件批量大小: [100条]                                               │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  异常检测阈值                                                                │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Fork频率: [10次/5秒]                                                │    │
│  │ Exec频率: [50次/5秒]                                                │    │
│  │ 网络调用: [20次/5秒]                                                │    │
│  │ 文件操作: [30次/5秒]                                                │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  阻断配置                                                                    │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ ☑ 启用自动阻断                                                       │    │
│  │ ☑ 启用本地阻断                                                       │    │
│  │ 阻断方式: [kill_process ▼]                                         │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  白名单管理                                                                  │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ [systemd] [sshd] [dockerd] [containerd]                        [+]  │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│                              [保存配置]  [重置默认]                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 3.2.3 微服务状态监控 (V5.5新增)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        服务状态监控                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐         │
│  │   API Service    │  │   Agent Hub      │  │ Pipeline Service │         │
│  │   ───────────    │  │   ───────────    │  │  ───────────    │         │
│  │   状态: 运行中    │  │   状态: 运行中    │  │  状态: 运行中    │         │
│  │   端口: 8080     │  │   端口: 19090    │  │  端口: 19091    │         │
│  │   请求: 1.2k/s   │  │   Agent在线: 50  │  │  消息处理: 100/s│         │
│  │   CPU: 15%       │  │   CPU: 10%       │  │  CPU: 30%       │         │
│  │   内存: 200MB    │  │   内存: 150MB    │  │  内存: 400MB    │         │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘         │
│                                                                              │
│                                                                              │
│  依赖服务                                                                    │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌────────────┐  │
│  │ PostgreSQL    │  │    Redis      │  │    Kafka      │  │   MinIO    │  │
│  │   运行中      │  │    运行中      │  │    运行中      │  │   运行中   │  │
│  └───────────────┘  └───────────────┘  └───────────────┘  └────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. API接口

### 4.1 Agent管理接口 (V5.5新增)

```typescript
// 获取Agent列表
GET /api/v1/agents
Query: { status?: 'online' | 'offline', hostname?: string }

// 获取Agent状态
GET /api/v1/agents/:id/status

// 获取Agent智能配置
GET /api/v1/agents/:id/config

// 更新Agent智能配置
PUT /api/v1/agents/:id/config

// 下发命令到Agent
POST /api/v1/agents/:id/command
Body: { command: string, timeout_seconds: number }
```

### 4.2 服务状态接口 (V5.5新增)

```typescript
// 获取微服务状态
GET /api/v1/services/status

// 获取Pipeline状态
GET /api/v1/pipeline/status
```

---

## 5. WebSocket消息

### 5.1 Agent状态推送

```typescript
// WebSocket消息格式
interface AgentStatusMessage {
    type: 'agent_status'
    data: {
        agent_id: string
        status: 'online' | 'offline'
        cpu_percent: number
        memory_mb: number
        event_count: number
    }
}
```

### 5.2 告警实时推送

```typescript
interface AlertMessage {
    type: 'alert_new' | 'alert_update'
    data: Alert
}
```

---

## 6. 组件清单

### 6.1 新增组件 (V5.5)

| 组件 | 说明 |
|------|------|
| views/agent/List.vue | Agent列表 |
| views/agent/Status.vue | Agent状态详情 |
| views/agent/Config.vue | Agent智能配置 |
| views/system/Services.vue | 微服务状态监控 |
| components/agent/MetricsChart.vue | Agent性能图表 |
| components/agent/AgentDetail.vue | Agent详情抽屉 |

---

## 7. 路由配置

```typescript
// router/index.ts

const routes = [
  {
    path: '/detection',
    name: 'Detection',
    children: [
      { path: '', component: () => import('@/views/detection/Overview.vue') },
      { path: 'alerts', component: () => import('@/views/detection/Alerts.vue') },
      { path: 'rules', component: () => import('@/views/detection/Rules.vue') },
      { path: 'policies', component: () => import('@/views/detection/Policies.vue') },
    ]
  },
  {
    path: '/agent',  // V5.5新增
    name: 'Agent',
    children: [
      { path: '', component: () => import('@/views/agent/List.vue') },
      { path: 'config', component: () => import('@/views/agent/Config.vue') },
    ]
  },
  {
    path: '/system',  // V5.5新增
    name: 'System',
    children: [
      { path: 'services', component: () => import('@/views/system/Services.vue') },
    ]
  }
]
```

---

**文档结束**
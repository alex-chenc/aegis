# Aegis智能主机安全系统 V5.6 产品需求文档

**版本**: 5.6
**日期**: 2026-04-14
**状态**: 设计中

---

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 5.6 | 2026-04-14 | 安全产品团队 | **智能异常检测模块全面增强**：Sigma规则上传解析、LangChain多轮分析、Agent智能体化、单Host精确下发 |
| 5.5 | 2026-03-30 | 安全产品团队 | Agent本地智能预处理、微服务架构分离 |
| 5.0 | 2026-03-20 | 安全产品团队 | 新增智能异常检测模块 |

---

## 2. 产品概述

### 2.1 产品定位

Aegis是一个AI-native的主机安全平台，通过集成LLM和Agent技术实现智能化的安全运营。V5.6版本在保持1C1G资源限制的前提下，深化Agent智能体能力，引入LangChain多轮分析模式，实现更精准的安全威胁检测与响应。

### 2.2 V5.6核心价值

- **Sigma规则解析**: 支持用户上传Sigma规则文件，解析入库后统一下发Agent
- **LangChain多轮分析**: AI降噪支持多轮对话式分析，深度探索事件上下文
- **Agent智能体化**: Agent成为真正的智能体，具备工具调用和自主决策能力
- **单Host精确下发**: 所有命令均通过IP精确下发到指定Agent，杜绝广播模式

### 2.3 目标用户

| 用户角色 | 核心需求 | 使用场景 |
|:---|:---|:---|
| 安全运维工程师 | 规则管理、AI降噪分析 | 上传Sigma规则、分析告警 |
| 安全分析师 | 深度调查、威胁狩猎 | 多轮分析、入侵链路还原 |
| 运维工程师 | 主机管理、远程控制 | 查看主机状态、执行诊断命令 |

---

## 3. 导航结构

### 3.1 左侧菜单栏

```
Aegis智能主机安全系统
├── 主机列表
├── 智能基线检查与修复
│   ├── 基线工作台
│   └── 基线任务中心
├── 智能漏洞检查与修复
├── 智能异常检测
│   ├── 安全概览
│   ├── 告警中心
│   ├── 阻断策略
│   └── 规则管理 (V5.6增强)
└── 系统配置
```

### 3.2 菜单项说明

| 菜单项 | 页面路径 | 功能说明 |
|:---|:---|:---|
| 主机列表 | `/hosts` | 展示所有已纳管主机资产，支持搜索、筛选和详情查看 |
| 智能基线检查与修复 | `/baseline` | 基线合规管理模块入口 |
| 智能漏洞检查与修复 | `/vulnerability` | 软件漏洞扫描、CVE分析、智能修复 |
| 智能异常检测 | `/detection` | AI运行时异常检测 |
| ├─ 安全概览 | `/detection/overview` | ATT&CK矩阵可视化、威胁统计 |
| ├─ 告警中心 | `/detection/alerts` | 告警列表、**AI降噪多轮分析(V5.6增强)** |
| ├─ 阻断策略 | `/detection/policies` | 自动阻断开关配置 |
| └─ 规则管理 | `/detection/rules` | **Sigma规则上传解析、AI规则更新配置(V5.6)** |
| 系统配置 | `/settings` | LLM配置、Agent安装、系统参数设置 |

---

## 4. 功能需求详述

### 4.1 规则管理增强 - Sigma规则解析

**页面路径**: `/detection/rules`

#### 4.1.1 功能目标

用户可以上传一个或多个Sigma规则文件（YAML格式），系统自动解析并入库，支持批量操作和规则版本管理。

#### 4.1.2 界面布局

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  规则管理                                                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  [上传Sigma规则文件]  [批量导入]  [导出选中]              搜索: [_________] │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  支持格式: .yaml, .yml                                                │   │
│  │  上传方式: 单文件上传 / 批量压缩包(zip)上传                            │   │
│  │  拖拽区域: 将文件拖拽到此处或点击上传                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  规则列表:                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ☐ │ 规则ID        │ 名称                  │ MITRE    │ 状态   │ 操作 │   │
│  ├─────┼───────────────┼──────────────────────┼──────────┼────────┼──────┤   │
│  │ ☐ │ rev_shell_001  │ Reverse Shell        │ T1059.004│ active │详情  │   │
│  │ ☐ │ ssh_brute_002  │ SSH Brute Force      │ T1021.001│ pending│详情  │   │
│  │ ☐ │ net_conn_003   │ Suspicious Network   │ T1071.004│ experi │详情  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 4.1.3 核心功能

| 功能 | 说明 |
|------|------|
| 文件上传 | 支持单个YAML文件、多个YAML文件、ZIP压缩包（内含多个YAML） |
| 解析验证 | 解析Sigma规则YAML结构，验证必填字段(title、logsource、detection等) |
| MITRE映射 | 自动提取MITRE ID (通过tags或title关键词匹配) |
| 状态初始化 | 新导入规则默认为 `pending` 状态 |
| 批量操作 | 支持批量审批、批量禁用、批量删除 |
| 规则下发 | 审批通过后自动下发到相关Agent |

#### 4.1.4 Sigma规则解析逻辑

```yaml
# Sigma规则标准结构
title: Reverse Shell Detection
id: rev_shell_001
status: experimental
description: Detects reverse shell execution
tags:
  - attack.t1059.004
  - attack.execution
logsource:
  category: process_creation
  product: linux
detection:
  selection:
    CommandLine|contains:
      - '/bin/bash -i'
      - 'nc -e'
  condition: selection
level: critical
```

解析字段映射:

| Sigma字段 | 系统字段 | 说明 |
|-----------|----------|------|
| title | rule_name | 规则名称 |
| id | rule_id | 规则唯一标识 |
| description | description | 规则描述 |
| tags | mitre_id | 从tags中提取MITRE ID |
| detection | content | 检测条件(YAML格式) |
| level | severity | critical/high/medium/low |
| status | status | pending/active/disabled |

#### 4.1.5 规则下发流程

1. 用户上传Sigma文件
2. 系统解析并校验格式
3. 规则入库，状态为 `pending`
4. 管理员审批，状态变为 `active`
5. **Server层根据主机IP精确下发**到对应Agent（不再广播）
6. Agent加载规则并生效

---

### 4.2 规则管理增强 - AI规则更新配置

**页面路径**: `/detection/rules` → AI配置标签页

#### 4.2.1 功能目标

配置AI自动规则更新功能：当系统检测到异常事件时，自动调用LLM分析并生成/更新规则。

#### 4.2.2 界面布局

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  AI规则更新配置                                                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────┐  ┌─────────────────────────────────────┐   │
│  │ 功能开关                     │  │ 自动规则更新                        │   │
│  │ [✓] 启用AI规则更新           │  │ ○ 关闭  ● 开启(仅建议)  ○ 开启(自动)│   │
│  └─────────────────────────────┘  └─────────────────────────────────────┘   │
│                                                                              │
│  触发条件:                                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 同一MITRE ID在 [ 1 ] 小时内触发 [ 10 ] 次，即进行AI更新规则            │   │
│  │                                                                              │   │
│  │ 小时范围: 1-24 (默认1)                                                  │   │
│  │ 触发次数: 10-100 (默认10)                                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  规则生成策略:                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 生成模式: [████████] 保守  ────●─── 激进                             │   │
│  │                                                                              │   │
│  │ ☑ 规则生成后发送审核通知                                                │   │
│  │ (仅建议模式) ☑ 无人审核后24小时自动从待审核调整为实验性                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  [保存配置]  [测试规则生成]                                                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 4.2.3 AI规则更新流程

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                          AI规则更新流程                                        │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  1. 触发检测                                                                   │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ 触发条件满足 → 启动AI分析流程                                        │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    ↓                                          │
│  2. AI分析事件                                                                │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ LLM分析告警上下文:                                                  │    │
│     │   - 进程树结构                                                       │    │
│     │   - 命令行参数                                                       │    │
│     │   - 网络连接情况                                                     │    │
│     │   - 历史告警记录                                                     │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    ↓                                          │
│  3. 规则生成                                                                  │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ LLM生成Sigma规则:                                                   │    │
│     │   - 检测条件 (detection)                                           │    │
│     │   - MITRE映射                                                       │    │
│     │   - 严重程度                                                         │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    ↓                                          │
│  4. 人工审核                                                                  │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ 规则进入审核队列 → 管理员审批 → 激活/拒绝                            │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    ↓                                          │
│  5. 规则下发                                                                  │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ 激活后按IP精确下发到相关Agent                                       │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                                                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

### 4.3 告警中心增强 - AI降噪多轮分析

**页面路径**: `/detection/alerts`

#### 4.3.1 功能目标

用户可以选择多个告警事件，结合时间范围，进行AI多轮对话式降噪分析。系统引入LangChain模式，让大模型能够主动调用Agent工具进行深入调查。

#### 4.3.2 界面布局

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  告警中心                                                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  筛选: 主机[全部▼]  严重程度[全部▼]  MITRE[全部▼]  状态[全部▼]  时间[___~___] │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ☑ 全选    已选择 3 个告警                    [AI降噪分析] [导出]       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  告警列表:                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ☐ │ ALT-001 │ web-server-01 │ T1059.004 │ Critical │ 3 │ Active    │   │
│  │ ☑ │ ALT-002 │ db-server-02  │ T1059.004 │ High     │ 1 │ Active    │   │
│  │ ☑ │ ALT-003 │ app-server-01 │ T1071.004 │ Medium   │ 5 │ Active    │   │
│  │ ☐ │ ALT-004 │ web-server-01 │ T1021.001 │ Low      │ 2 │ Resolved  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ AI降噪分析                                                    [清除] │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │                                                                      │   │
│  │ [AI] 请选择需要分析的告警和时间范围。当前选择:                         │   │
│  │     - ALT-002 (db-server-01, T1059.004)                              │   │
│  │     - ALT-003 (app-server-01, T1071.004)                              │   │
│  │     - 时间范围: 最近1小时                                             │   │
│  │                                                                      │   │
│  │ [AI] 基于当前告警，初步判断:                                          │   │
│  │     - ALT-002可能是正常的数据库管理操作                               │   │
│  │     - ALT-003存在异常网络行为，需要进一步调查                         │   │
│  │     是否需要我进一步调查ALT-003的进程树和父进程信息?                   │   │
│  │                                                                      │   │
│  │ [用户] 是，调查ALT-003                                               │   │
│  │                                                                      │   │
│  │ [AI→Agent] 调用工具: GetProcessTree(host="app-server-01", pid=12345)  │   │
│  │ [Agent] 返回: bash(12345) → python(11111) → nc(22222)                │   │
│  │                                                                      │   │
│  │ [AI] 确认ALT-003为恶意行为:                                           │   │
│  │     进程链显示: bash → python → nc(外连)                              │   │
│  │     典型的反弹shell特征。建议: 阻断并隔离该主机进行进一步排查。        │   │
│  │                                                                      │   │
│  │ 分析结论:                                                             │   │
│  │   [ ] ALT-002 标记为误报                                             │   │
│  │   [✓] ALT-003 确认为真实威胁                                         │   │
│  │   [ ] 生成针对ALT-003的新规则                                         │   │
│  │                                                      [应用结论]       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 4.3.3 核心功能

| 功能 | 说明 |
|------|------|
| 多选降噪 | 支持勾选多个告警同时分析 |
| 时间范围 | 支持设置分析的时间范围（1小时/6小时/24小时/自定义） |
| 多轮对话 | AI能够进行多轮交互，逐步深入分析 |
| 工具调用 | AI可以调用Agent工具获取更多上下文（进程树、网络连接等） |
| 结论应用 | 支持将分析结论应用到告警（标记误报/确认威胁/生成规则） |

#### 4.3.4 LangChain多轮分析流程

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                      LangChain多轮分析流程                                     │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         LangChain Agent                                │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────┐   │   │
│  │  │   LLM       │← │  Memory     │← │  Tool Executor              │   │   │
│  │  │ (Claude)    │  │  (上下文)    │  │  (Agent工具调用)             │   │   │
│  │  └──────┬──────┘  └─────────────┘  └─────────────────────────────┘   │   │
│  │         │                           │                                 │   │
│  │         │                           ↓                                 │   │
│  │         │              ┌─────────────────────────────────┐           │   │
│  │         │              │       可用工具 (Tools)           │           │   │
│  │         │              │  ┌────────────────────────────┐ │           │   │
│  │         │              │  │ GetProcessTree             │ │           │   │
│  │         │              │  │ GetNetworkConnections      │ │           │   │
│  │         │              │  │ GetOpenFiles               │ │           │   │
│  │         │              │  │ GetUserSessions            │ │           │   │
│  │         │              │  │ GetRunningProcesses        │ │           │   │
│  │         │              │  │ QueryHistoricalLogs       │ │           │   │
│  │         │              │  └────────────────────────────┘ │           │   │
│  │         │              └─────────────────────────────────┘           │   │
│  │         │                           │                                 │   │
│  └─────────┼───────────────────────────┼─────────────────────────────────┘   │
│            │                           │                                   │
│            ↓                           ↓                                   │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         Server层                                      │   │
│  │  ┌────────────────────────────────────────────────────────────────┐ │   │
│  │  │  工具调用路由                                                    │ │   │
│  │  │  根据host_id精确找到对应Agent                                   │ │   │
│  │  │  发送ToolRequest获取Agent执行结果                               │ │   │
│  │  └────────────────────────────────────────────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         Agent层                                      │   │
│  │  ┌────────────────────────────────────────────────────────────────┐ │   │
│  │  │  工具执行器                                                      │ │   │
│  │  │  - GetProcessTree: 读取/proc/{pid}/status等                     │ │   │
│  │  │  - GetNetworkConnections: 读取/proc/net/tcp等                   │ │   │
│  │  │  - GetOpenFiles: 读取/proc/{pid}/fd等                           │ │   │
│  │  │  - GetUserSessions: 读取/var/run/utmp等                          │ │   │
│  │  └────────────────────────────────────────────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

### 4.4 Agent智能体增强

#### 4.4.1 Agent智能体架构

V5.6版本将Agent从"事件采集器"升级为"智能体"，具备以下能力：

| 能力 | 说明 |
|------|------|
| 工具调用 | 支持接收并执行来自Server的工具调用请求 |
| 主动上报 | 支持在发现异常时主动上报（不再等待Server轮询） |
| 本地推理 | 支持本地轻量级推理决策 |
| 有状态 | 维护与Server的长连接，支持请求/响应模式 |

#### 4.4.2 Agent工具列表

| 工具名称 | 参数 | 返回值 | 说明 |
|----------|------|--------|------|
| `GetProcessTree` | host_id, pid | 进程树结构 | 获取指定进程的完整树状结构 |
| `GetNetworkConnections` | host_id, pid | 网络连接列表 | 获取进程的网络连接情况 |
| `GetOpenFiles` | host_id, pid | 打开文件列表 | 获取进程打开的文件描述符 |
| `GetRunningProcesses` | host_id, filter | 进程列表 | 获取当前运行的进程（支持过滤） |
| `GetUserSessions` | host_id | 用户会话列表 | 获取当前登录用户会话 |
| `QueryHistoricalLogs` | host_id, start_time, end_time, filter | 日志条目 | 查询历史日志 |

#### 4.4.3 工具调用流程

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                          工具调用完整流程                                      │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  1. AI发起工具调用                                                            │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ LangChain Agent决定调用 GetProcessTree                            │    │
│     │ ToolRequest: { call_id: "xxx", tool: "GetProcessTree",            │    │
│     │               host_id: "host-123", pid: 12345 }                   │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    ↓                                          │
│  2. Server路由到目标Agent                                                     │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ Server根据host_id精确找到对应Agent连接                             │    │
│     │ 通过gRPC Stream发送ToolRequest                                     │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    ↓                                          │
│  3. Agent执行工具                                                             │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ Agent接收请求，执行本地命令收集:                                    │    │
│     │   - 读取/proc/{pid}/status                                        │    │
│     │   - 读取/proc/{pid}/children                                      │    │
│     │   - 组装进程树JSON                                                 │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                    ↓                                          │
│  4. 返回结果给AI                                                              │
│     ┌────────────────────────────────────────────────────────────────────┐    │
│     │ ToolResponse通过gRPC返回Server，再由Server返回给API Server         │    │
│     │ LangChain Agent收到结果，继续推理                                  │    │
│     └────────────────────────────────────────────────────────────────────┘    │
│                                                                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

### 4.5 单Host精确下发机制

#### 4.5.1 问题背景

V5.5及之前版本存在**广播模式**问题：
- 规则更新时，更新所有Agent
- 某些命令下发时也是广播

这导致：
- 网络带宽浪费
- 无法针对特定主机下发特定规则
- 安全风险（不需要的规则被下发到所有主机）

#### 4.5.2 V5.6解决方案

**所有命令下发均通过host_id精确指定目标Agent**

| 场景 | V5.5行为 | V5.6行为 |
|------|---------|---------|
| 规则更新 | 广播所有Agent | 根据主机IP精确下发到相关Agent |
| 阻断命令 | 精确到单Host | 保持精确（已正确） |
| 工具调用 | N/A | 根据host_id精确调用 |
| 远程诊断 | 广播 | 根据host_id精确执行 |

#### 4.5.3 精确下发流程

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                          单Host精确下发流程                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                       API Server                                      │   │
│  │  根据host_id查找目标Agent                                             │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                       Server (gRPC Client)                            │   │
│  │  调用AgentStream.Send(CommandRequest{Execute{host_id, ...}})         │   │
│  │  注意: 不使用Range遍历所有连接，只发送给指定host_id的连接              │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ↓                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                       目标Agent                                       │   │
│  │  接收并执行命令，只返回结果给Server                                    │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### 4.5.4 广播模式代码修复

**需要修改的代码位置**（在server/internal/grpc_server/server.go中）：

```go
// ❌ V5.5 错误代码 - 广播模式
func (s *GRPCServer) BroadcastRuleUpdate(update *pb.RuleUpdate) {
    s.agentConnections.Range(func(key, value interface{}) bool) {
        // 广播到所有Agent - 错误！
        conn.Stream.Send(...)
        return true
    })
}

// ✅ V5.6 正确代码 - 单Host下发
func (s *GRPCServer) SendRuleUpdateToHost(hostID uuid.UUID, update *pb.RuleUpdate) error {
    conn, ok := s.agentConnections.Load(hostID)
    if !ok {
        return fmt.Errorf("agent not connected: %s", hostID)
    }
    agentConn := conn.(*AgentConnection)
    return agentConn.Stream.Send(&pb.CommandRequest{
        Request: &pb.CommandRequest_RuleUpdate{
            RuleUpdate: &pb.RuleUpdateRequest{
                Action: "incremental",
                Rules:  []*pb.RuleUpdate{update},
            },
        },
    })
}
```

---

## 5. 数据库设计

### 5.1 新增表

#### 5.1.1 sigma_rules表（已有，补充字段）

```sql
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS source VARCHAR(50) DEFAULT 'manual';
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_name VARCHAR(255);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS file_hash VARCHAR(64);
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parsed_at TIMESTAMP;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS ai_generated BOOLEAN DEFAULT FALSE;
ALTER TABLE sigma_rules ADD COLUMN IF NOT EXISTS parent_rule_id VARCHAR(100);
```

#### 5.1.2 ai_analysis_session表（新增）

```sql
CREATE TABLE ai_analysis_session (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) UNIQUE NOT NULL,
    user_id VARCHAR(100),
    host_filter JSONB,
    alert_ids JSONB,
    time_range_start TIMESTAMP,
    time_range_end TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 5.1.3 ai_analysis_message表（新增）

```sql
CREATE TABLE ai_analysis_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100) NOT NULL,
    role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    tool_calls JSONB,
    tool_results JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### 5.1.4 tool_execution_log表（新增）

```sql
CREATE TABLE tool_execution_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(100),
    call_id VARCHAR(100),
    tool_name VARCHAR(50) NOT NULL,
    host_id UUID,
    parameters JSONB,
    result JSONB,
    error TEXT,
    execution_time_ms INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### 5.2 索引优化

```sql
CREATE INDEX IF NOT EXISTS idx_sigma_rules_mitre_id ON sigma_rules(mitre_id);
CREATE INDEX IF NOT EXISTS idx_sigma_rules_status ON sigma_rules(status);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_session_status ON ai_analysis_session(status);
CREATE INDEX IF NOT EXISTS idx_tool_execution_log_session_id ON tool_execution_log(session_id);
```

---

## 6. API接口设计

### 6.1 规则管理API

```
# 上传Sigma规则文件
POST /api/v1/detection/rules/upload
Content-Type: multipart/form-data
Request:
  - file: Sigma规则文件(yaml/yml/zip)
Response:
  {
    "success": true,
    "parsed_count": 5,
    "failed_count": 1,
    "rules": [
      {"rule_id": "xxx", "title": "xxx", "status": "pending"}
    ]
  }

# 批量导入规则
POST /api/v1/detection/rules/batch-import
Request:
  {
    "rules": [
      {"title": "xxx", "content": "yaml...", "mitre_id": "T1059.004"}
    ]
  }
Response:
  {
    "success": true,
    "imported_count": 10
  }

# 获取规则列表
GET /api/v1/detection/rules
Parameters: page, pageSize, status, mitre_id, source, query

# 更新规则状态
PUT /api/v1/detection/rules/:id/status
Request: { "status": "active" | "disabled" }

# 获取AI规则更新配置
GET /api/v1/detection/rules/ai-rule-config

# 更新AI规则更新配置
PUT /api/v1/detection/rules/ai-rule-config
Request: {
  "enabled": true,
  "mode": "suggest" | "auto",
  "thresholds": {
    "high_frequency_count": 10,   // 触发次数 (10-100)
    "high_frequency_hours": 1     // 时间窗口 (1-24)
  },
  "conservatism": 0.5,             // 生成策略 (0.0-1.0)
  "require_approval": true,
  "auto_activate_after_approval": false  // 仅suggest模式下可设置
}
Response: {
  "id": "xxx",
  "name": "default",
  "enabled": true,
  "mode": "suggest",
  "thresholds": {
    "high_frequency_count": 10,
    "high_frequency_hours": 1
  },
  "conservatism": 0.5,
  "require_approval": true,
  "auto_activate_after_approval": false,
  "activation_delay_hours": 24,
  "notify_on_generation": true,
  "notify_on_approval": true,
  "notification_targets": [],
  "rules_generated_count": 0,
  "rules_approved_count": 0
}
```

### 6.2 AI降噪分析API

```
# 创建分析会话
POST /api/v1/detection/alerts/ai-analysis/session
Request:
  {
    "alert_ids": ["ALT-001", "ALT-002"],
    "time_range": {
      "start": "2026-04-14T10:00:00Z",
      "end": "2026-04-14T11:00:00Z"
    },
    "host_filter": ["host-1", "host-2"]
  }
Response:
  {
    "session_id": "sess_xxx",
    "status": "ready"
  }

# 发送消息
POST /api/v1/detection/alerts/ai-analysis/:session_id/message
Request:
  {
    "content": "请分析这些告警",
    "stream": false
  }
Response:
  {
    "message_id": "msg_xxx",
    "role": "assistant",
    "content": "分析结果...",
    "tool_calls": [
      {
        "call_id": "call_xxx",
        "tool": "GetProcessTree",
        "arguments": {"host_id": "host-1", "pid": 12345}
      }
    ]
  }

# 获取工具执行结果
POST /api/v1/detection/alerts/ai-analysis/:session_id/tool-result
Request:
  {
    "call_id": "call_xxx",
    "result": {...}
  }

# 应用分析结论
POST /api/v1/detection/alerts/ai-analysis/:session_id/conclusion
Request:
  {
    "conclusions": [
      {"alert_id": "ALT-001", "action": "mark_false_positive"},
      {"alert_id": "ALT-002", "action": "confirm_threat"},
      {"alert_id": "ALT-003", "action": "generate_rule"}
    ]
  }

# 获取分析历史
GET /api/v1/detection/alerts/ai-analysis/:session_id/history
```

### 6.3 工具调用API

```
# 调用Agent工具（内部API，Server层使用）
POST /api/v1/internal/tools/execute
Request:
  {
    "tool": "GetProcessTree",
    "host_id": "host-xxx",
    "parameters": {"pid": 12345},
    "timeout": 30
  }
Response:
  {
    "success": true,
    "result": {
      "pid": 12345,
      "name": "bash",
      "children": [...]
    }
  }
```

---

## 7. gRPC通信设计

### 7.1 Server → Agent 工具调用

```protobuf
// agent_comm.proto
service AgentService {
    rpc ExecuteCommand(stream CommandRequest) returns (stream CommandResponse);
    rpc ExecuteTool(ToolRequest) returns (ToolResponse);
    rpc ReportEvent(ReportEventRequest) returns (ReportEventResponse);
}

message ToolRequest {
    string call_id = 1;
    string host_id = 2;
    string tool = 3;        // GetProcessTree, GetNetworkConnections, etc.
    string arguments = 4;   // JSON格式参数
}

message ToolResponse {
    string call_id = 1;
    bool success = 2;
    string result = 3;       // JSON格式结果
    string error = 4;
}
```

### 7.2 API Server → Server 工具转发

```protobuf
// api_server_comm.proto
service APIServerToServer {
    rpc ForwardCommand(ForwardCommandRequest) returns (ForwardCommandResponse);
    rpc ExecuteTool(ToolExecuteRequest) returns (ToolExecuteResponse);
    rpc GetAgentStatus(GetAgentStatusRequest) returns (GetAgentStatusResponse);
    // ...
}

message ToolExecuteRequest {
    string call_id = 1;
    string host_id = 2;           // 目标主机ID（精确指定）
    string tool = 3;
    string arguments = 4;
}

message ToolExecuteResponse {
    string call_id = 1;
    bool success = 2;
    string result = 3;
    string error = 4;
}
```

---

## 8. 前端组件设计

### 8.1 规则管理页面组件

| 组件 | 功能 |
|------|------|
| `RuleUpload.vue` | Sigma规则文件上传组件 |
| `RuleList.vue` | 规则列表展示 |
| `RuleDetail.vue` | 规则详情弹窗 |
| `RuleFilter.vue` | 规则筛选组件 |
| `AIConfigPanel.vue` | AI规则更新配置面板 |

### 8.2 告警中心页面组件

| 组件 | 功能 |
|------|------|
| `AlertTable.vue` | 告警列表表格（支持多选） |
| `AlertFilter.vue` | 告警筛选组件 |
| `AIAnalysisPanel.vue` | AI降噪分析面板 |
| `ChatMessage.vue` | 聊天消息展示 |
| `ToolCallBlock.vue` | 工具调用块展示 |
| `ConclusionForm.vue` | 分析结论表单 |

---

## 9. 验收标准

### 9.1 Sigma规则解析

- [ ] 支持上传单个YAML文件
- [ ] 支持上传ZIP压缩包（含多个YAML）
- [ ] 正确解析Sigma规则所有字段
- [ ] 自动提取MITRE ID
- [ ] 规则状态正确初始化为pending
- [ ] 批量导入/导出功能正常

### 9.2 AI规则更新

- [ ] 配置开关生效
- [ ] 触发条件正确判断
- [ ] LLM生成规则格式正确
- [ ] 规则进入审核队列
- [ ] 审核通过后正确下发到Agent

### 9.3 AI降噪多轮分析

- [ ] 支持多告警选择
- [ ] 支持时间范围设置
- [ ] 多轮对话正常
- [ ] AI可调用Agent工具
- [ ] 工具返回结果正确展示
- [ ] 分析结论可应用

### 9.4 Agent智能体

- [ ] 工具调用协议实现
- [ ] 工具执行结果正确返回
- [ ] 长连接稳定
- [ ] 异常处理正常

### 9.5 单Host精确下发

- [ ] 所有命令通过host_id精确下发
- [ ] 消除广播模式
- [ ] 网络带宽降低
- [ ] 下发速度满足要求

---

## 10. 非功能性需求

### 10.1 性能需求

| 指标 | 要求 |
|------|------|
| Sigma规则解析 | 单文件<1秒，100个规则<10秒 |
| AI分析响应 | 首轮<3秒，后续轮次<2秒 |
| 工具调用延迟 | <500ms |
| 规则下发延迟 | 单Host<100ms |

### 10.2 安全需求

| 需求 | 说明 |
|------|------|
| 传输加密 | gRPC TLS 1.3 |
| 认证授权 | Token + JWT |
| 审计日志 | 所有操作记录 |
| 隔离性 | Agent间隔离，工具调用需明确授权 |

---

---

## 11. 全局消息通知中心 (V5.6新增)

### 11.1 功能概述

在系统顶栏最右侧"刷新"按钮旁新增**铃铛图标 + 未读 Badge**，点击后展开右侧消息抽屉（Drawer），集中展示所有系统通知（AI规则生成通知、告警触发通知、审核待办通知等）。

**核心价值**：
- 安全运维人员无需切换页面即可感知关键安全事件
- AI规则更新配置（4.2节）产生的审核通知可实时推达
- 告警中心事件（4.3节）可汇聚至通知中心统一处理

---

### 11.2 UI 交互模块设计

#### 11.2.1 顶栏集成位置

```
┌──────────────────────────────────────────────────────────────────────────┐
│  面包屑: 首页 / 智能异常检测 / 告警中心                                        │
│                                                    [🔔 99+] [刷新按钮]   │
└──────────────────────────────────────────────────────────────────────────┘
```

铃铛图标在 `App.vue` 的 `header-right` 区域，**位于刷新按钮左侧**。
使用 Element Plus `el-badge` 包裹 `el-button`，Badge 显示未读数量。

规则：
- 未读数为 0：不显示 Badge
- 未读数 1–99：显示精确数字
- 未读数 > 99：显示 `99+`

#### 11.2.2 消息抽屉（Drawer）结构

```
┌────────────────────────────── Drawer (宽 400px) ──────────────────────────┐
│  消息通知                                            [全部标为已读] [✕关闭] │
├─────────────────────────────────────────────────────────────────────────┤
│  [未读 (3)]  [已读]                                                        │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │ 🔴 [critical]  AI规则生成通知                          2分钟前    │   │
│  │ 检测到 T1059.004 高频告警，AI已生成新规则，请审核。               │   │
│  │                                                   [前往审核 →]   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │ 🟠 [high]  告警触发通知                               15分钟前   │   │
│  │ 主机 web-server-01 触发 T1071.004 告警，需确认。                 │   │
│  │                                                   [前往查看 →]   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ─────────── 没有更多通知 ───────────                                    │
└──────────────────────────────────────────────────────────────────────────┘
```

#### 11.2.3 组件层级关系

```
App.vue
└── header-right
    ├── NotificationBell.vue          # 铃铛 + Badge + 触发抽屉
    └── NotificationDrawer.vue        # 抽屉主体
        ├── el-tabs (未读 / 已读)
        │   └── NotificationList.vue  # 通知列表（虚拟滚动）
        │       └── NotificationItem.vue  # 单条通知
        └── DrawerFooter.vue          # 底部操作（全部已读 / 加载更多）
```

---

### 11.3 状态管理模块设计

#### 11.3.1 Pinia Store 定义

```typescript
// stores/notification.ts

export interface Notification {
  id: string
  title: string
  content: string
  is_read: boolean
  timestamp: string          // ISO 8601
  severity: 'critical' | 'high' | 'medium' | 'low' | 'info'
  type: 'rule_generated' | 'alert_triggered' | 'approval_required' | 'system'
  link?: string              // 跳转链接（可选）
  metadata?: Record<string, any>  // 扩展业务字段
}

export const useNotificationStore = defineStore('notification', {
  state: () => ({
    notifications: [] as Notification[],
    unreadCount: 0,
    drawerVisible: false,
    activeTab: 'unread' as 'unread' | 'read',
    loading: false,
    polling: null as ReturnType<typeof setInterval> | null
  }),

  getters: {
    unreadList: (state) => state.notifications.filter(n => !n.is_read),
    readList:   (state) => state.notifications.filter(n => n.is_read),
    badgeCount: (state) => state.unreadCount > 99 ? '99+' : String(state.unreadCount)
  },

  actions: {
    // 拉取通知列表
    async fetchNotifications() { ... },
    // 标记单条已读
    async markAsRead(id: string) { ... },
    // 全部标为已读
    async markAllAsRead() { ... },
    // 切换抽屉显示
    toggleDrawer() { this.drawerVisible = !this.drawerVisible },
    // 启动轮询（每60秒）
    startPolling() { ... },
    stopPolling() { ... }
  }
})
```

#### 11.3.2 点击"标为已读"后的状态流转

```
用户点击 [标为已读]
    │
    ├─► 1. 乐观更新本地状态（立即反映 UI）
    │       notifications[id].is_read = true
    │       unreadCount -= 1
    │
    ├─► 2. 调用 PUT /api/v1/notifications/:id/read
    │
    └─► 3. 成功 → 不再操作（本地已更新）
         失败 → 回滚本地状态 + 弹出 ElMessage.error
```

---

### 11.4 数据协议模块设计

#### 11.4.1 通知对象 JSON 结构

```json
{
  "id": "3f6c2a1b-...",
  "title": "AI规则生成通知",
  "content": "检测到 T1059.004 高频告警（已触发 12 次/1h），AI已生成新Sigma规则，请前往规则管理页面审核。",
  "is_read": false,
  "timestamp": "2026-04-16T10:00:00Z",
  "severity": "high",
  "type": "rule_generated",
  "link": "/detection/rules?highlight=rev_shell_002",
  "metadata": {
    "rule_id": "rev_shell_002",
    "mitre_id": "T1059.004",
    "trigger_count": 12,
    "trigger_hours": 1,
    "alert_ids": ["ALT-010", "ALT-011"],
    "host_ids": ["host-abc123"]
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string (UUID) | ✅ | 通知唯一标识 |
| `title` | string | ✅ | 通知标题（≤50字符） |
| `content` | string | ✅ | 通知正文（≤200字符） |
| `is_read` | boolean | ✅ | 已读状态 |
| `timestamp` | string (ISO 8601) | ✅ | 通知产生时间 |
| `severity` | enum | ✅ | 告警级别：critical/high/medium/low/info |
| `type` | enum | ✅ | 通知类型：rule_generated/alert_triggered/approval_required/system |
| `link` | string | ❌ | 关联页面跳转路径 |
| `metadata.rule_id` | string | ❌ | AI生成规则ID（type=rule_generated时携带） |
| `metadata.mitre_id` | string | ❌ | 相关MITRE ID |
| `metadata.trigger_count` | number | ❌ | 触发次数 |
| `metadata.trigger_hours` | number | ❌ | 触发时间窗口（小时） |
| `metadata.alert_ids` | string[] | ❌ | 相关告警ID列表 |
| `metadata.host_ids` | string[] | ❌ | 相关主机ID列表 |

#### 11.4.2 API 接口定义

```
# 获取通知列表
GET /api/v1/notifications
Parameters:
  page      int    第几页 (default: 1)
  pageSize  int    每页条数 (default: 20, max: 100)
  is_read   bool   过滤已读/未读 (可选)
  type      string 过滤通知类型 (可选)
Response:
{
  "success": true,
  "data": {
    "list": [ ...Notification ],
    "total": 42,
    "unread_count": 3,
    "page": 1,
    "page_size": 20
  }
}

# 标记单条已读
PUT /api/v1/notifications/:id/read
Response: { "success": true }

# 全部标为已读
PUT /api/v1/notifications/read-all
Response: { "success": true, "updated_count": 3 }
```

---

### 11.5 全链路测试用例

#### 11.5.1 功能测试

| ID | 测试场景 | 前置条件 | 操作步骤 | 预期结果 |
|----|---------|---------|---------|---------|
| FT-01 | 收到新通知时 Badge 实时更新 | 系统有1条未读通知 | 等待前端轮询（60s）或后端 Push 新通知 | Badge 数字从 0 变为 1 |
| FT-02 | 点击铃铛打开抽屉 | Badge 显示 3 | 点击铃铛图标 | 抽屉从右侧滑入，默认显示"未读"Tab，列出3条通知 |
| FT-03 | 点击单条"标为已读" | 抽屉打开，未读列表有1条通知 | 点击通知右侧的"已读"按钮 | 该通知从未读列表消失，移至已读列表；Badge 数字-1 |
| FT-04 | 点击"全部标为已读" | 抽屉打开，未读列表有3条通知 | 点击抽屉顶部"全部标为已读" | 未读列表清空，Badge 消失（为0不显示），3条通知出现在已读列表 |
| FT-05 | 通知跳转链接可用 | 通知包含 `link` 字段 | 点击通知的"前往审核"按钮 | 路由跳转至对应页面，并自动关闭抽屉 |
| FT-06 | 已读 Tab 展示历史通知 | 系统有已读通知 | 切换至"已读"Tab | 已读通知按时间倒序展示，Badge 不受影响 |
| FT-07 | AI 规则审核通知正确显示 | AI 生成一条新规则（type=rule_generated） | 查看抽屉未读列表 | 通知标题含"AI规则生成通知"，severity 标签显示正确颜色，metadata.mitre_id 展示 |

#### 11.5.2 边界测试

| ID | 测试场景 | 前置条件 | 操作步骤 | 预期结果 |
|----|---------|---------|---------|---------|
| BT-01 | 通知数量超过 99 条 | 100+ 条未读通知 | 查看顶栏 Badge | Badge 显示 `99+` 而非实际数字 |
| BT-02 | 通知数量为 0 | 无任何未读通知 | 查看顶栏 | Badge 不显示（隐藏而非显示0） |
| BT-03 | 消息内容过长折行处理 | 通知 content 超过 200 字符 | 查看通知列表 | 超出部分用省略号截断，悬浮 Tooltip 显示全文 |
| BT-04 | 通知标题为空 | title 字段为空字符串 | 查看通知列表 | 显示默认标题"系统通知" |
| BT-05 | 高频刷新不触发重复请求 | 轮询间隔 60 秒 | 快速点击刷新按钮多次 | 防抖机制保证单次请求，不发出重复 HTTP 请求 |
| BT-06 | 并发标记已读 | 同时点击多条通知的"标为已读" | 快速连续点击3条通知 | 3条通知均正确标为已读，unreadCount 正确递减 |
| BT-07 | 网络异常时乐观更新回滚 | 模拟网络断开 | 点击"标为已读" | 请求失败后通知恢复未读状态，提示"操作失败，请重试" |
| BT-08 | 抽屉滚动加载更多 | 未读通知超过 20 条（分页） | 滚动到抽屉底部 | 自动加载下一页通知（无限滚动） |

#### 11.5.3 交互测试

| ID | 测试场景 | 前置条件 | 操作步骤 | 预期结果 |
|----|---------|---------|---------|---------|
| IT-01 | 抽屉弹出动画流畅 | 任意状态 | 点击铃铛 | 抽屉以 300ms 滑入动画出现，无卡顿 |
| IT-02 | 点击非抽屉区域关闭抽屉 | 抽屉已打开 | 点击抽屉外部遮罩区域 | 抽屉关闭，页面恢复正常交互 |
| IT-03 | ESC 键关闭抽屉 | 抽屉已打开 | 按下 ESC 键 | 抽屉关闭 |
| IT-04 | 抽屉打开期间路由切换 | 抽屉已打开 | 点击左侧菜单跳转其他页面 | 抽屉自动关闭，路由跳转正常 |
| IT-05 | severity 标签颜色区分 | 通知包含不同级别 | 查看通知列表 | critical=红色，high=橙色，medium=黄色，low=蓝色，info=灰色 |
| IT-06 | 通知列表时间格式友好 | 通知时间为 2 分钟前 | 查看时间列 | 显示相对时间"2分钟前"，超过1天显示具体日期 |

---

### 11.6 验收标准

- [ ] 顶栏铃铛图标已添加至刷新按钮左侧
- [ ] Badge 正确反映未读数量（0不显示，>99显示99+）
- [ ] 右侧抽屉（宽400px）可正常展开/关闭
- [ ] 未读/已读两个 Tab 分别展示对应通知
- [ ] 单条和全部标为已读功能正常，状态实时同步
- [ ] AI规则更新通知（type=rule_generated）包含 mitre_id、severity 字段并正确显示
- [ ] 通知跳转链接可正常路由到目标页面
- [ ] 边界场景（99+、空列表、长文本）显示正常
- [ ] 点击抽屉外部可关闭抽屉

---

**文档结束**

# Aegis智能主机安全系统 V5.7 产品需求文档

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 5.7 | 2026-05-07 | 安全产品团队 | **脚本安全审计体系、eBPF运行时增强、智能体优化**：命令审计黑名单、AI审计重试、下发校验、文件/网络事件采集、内核适配、智能体迭代优化 |
| 5.6 | 2026-04-14 | 安全产品团队 | 智能异常检测模块全面增强：Sigma规则上传解析、LangChain多轮分析、Agent智能体化 |
| 5.5 | 2026-03-30 | 安全产品团队 | Agent本地智能预处理、微服务架构分离 |

---

## 2. 产品概述

### 2.1 产品定位

Aegis是一个AI-native的主机安全平台。V5.7版本聚焦三大方向：**脚本安全审计体系建设**、**eBPF运行时监控能力增强**、**智能体体验优化**。核心目标是建立从脚本生成到下发执行的全链路安全防线，扩展运行时威胁检测的感知维度，并提升AI分析的效率和可靠性。

### 2.2 V5.7核心价值

- **全链路脚本安全审计**: 建立"AI审计 + 黑名单审计 + 下发前校验"三重防线，确保任何脚本在生成和下发过程中均经过严格安全审查
- **可配置命令黑名单**: 支持管理员配置命令审计规则，覆盖正则匹配和全命令匹配，预置安全专家审查的默认规则集
- **eBPF运行时感知扩展**: 新增文件事件（openat）和网络事件（connect）的eBPF采集，适配多内核版本
- **智能体优化**: 降低无效迭代、优化工具调用可靠性、增强可观测性

### 2.3 目标用户

| 用户角色 | 核心需求 | 使用场景 |
|:---|:---|:---|
| 安全运维工程师 | 规则管理、脚本审计配置 | 配置命令黑名单、审查审计日志 |
| 安全分析师 | 深度调查、威胁狩猎 | AI智能体分析、文件/网络事件溯源 |
| 运维工程师 | 主机管理、脚本下发 | 基线检查、漏洞修复脚本执行 |
| 系统管理员 | 系统安全配置 | 管理审计规则、查看审计报告 |

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
│   └── 规则管理
└── 系统配置
    ├── 模型配置
    ├── 命令审计配置 (V5.7新增)
    ├── 审计日志 (V5.7新增)
    └── Agent 安装
```

### 3.2 菜单项变更

| 菜单项 | 页面路径 | 变更类型 | 功能说明 |
|:---|:---|:---|:---|
| 命令审计配置 | `/settings/command-audit` | **新增** | 配置脚本命令黑名单规则 |
| 审计日志 | `/settings/audit-logs` | **新增** | 查看脚本审计历史记录 |

---

## 4. 功能需求详述

### 4.1 命令审计黑名单配置

**页面路径**: `/settings/command-audit`

#### 4.1.1 功能目标

管理员可以在系统配置中管理命令审计黑名单，配置不允许出现在生成脚本中的危险命令。支持正则表达式和全命令精确匹配两种模式，系统预置经过安全专家审查的默认规则集。

#### 4.1.2 核心需求

| 编号 | 需求描述 | 优先级 |
|:---|:---|:---|
| REQ-001 | 支持新增、编辑、删除、启停黑名单规则 | P0 |
| REQ-002 | 支持正则表达式匹配模式 | P0 |
| REQ-003 | 支持全命令精确匹配模式 | P0 |
| REQ-004 | 预置默认规则集，涵盖文件系统破坏、权限滥用、网络外联、反弹shell等类别 | P0 |
| REQ-005 | 规则支持分类标签（filesystem/permission/network/system/privilege） | P1 |
| REQ-006 | 规则支持严重等级（critical/high/medium） | P1 |
| REQ-007 | 规则支持按脚本类型分别启用（baseline/vulnerability/poc/self_healing） | P1 |
| REQ-008 | 支持规则的导入导出 | P2 |

#### 4.1.3 黑名单规则数据模型

| 字段 | 类型 | 说明 |
|:---|:---|:---|
| `id` | UUID | 规则唯一标识 |
| `name` | VARCHAR(200) | 规则名称 |
| `description` | TEXT | 规则描述 |
| `rule_type` | ENUM | `hard_block`（硬拦截）/ `soft_warn`（软告警） |
| `match_type` | ENUM | `exact`（全命令匹配）/ `regex`（正则匹配） |
| `pattern` | TEXT | 匹配模式内容 |
| `category` | VARCHAR(50) | 分类：filesystem/permission/network/system/privilege |
| `severity` | ENUM | `critical` / `high` / `medium` |
| `applies_to` | JSONB | 适用脚本类型数组：`["all"]` 或 `["baseline", "vulnerability"]` |
| `is_preset` | BOOLEAN | 是否为系统预置规则（预置规则不可删除） |
| `is_enabled` | BOOLEAN | 启停状态 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

#### 4.1.4 预置默认规则集

| 规则名称 | 匹配类型 | 模式 | 分类 | 严重等级 |
|:---|:---|:---|:---|:---|
| 递归删除根目录 | regex | `rm\s+(-[a-zA-Z]*r[a-zA-Z]*f\|-[a-zA-Z]*f[a-zA-Z]*r)\s+/` | filesystem | critical |
| 格式化磁盘 | regex | `mkfs\.` | filesystem | critical |
| 覆盖磁盘 | regex | `dd\s+if=/dev/(zero\|random)` | filesystem | critical |
| 写入磁盘设备 | regex | `>\s*/dev/sd[a-z]` | filesystem | critical |
| Fork炸弹 | exact | `:(){ :\|:& };:` | system | critical |
| 递归777权限 | regex | `chmod\s+(-R\s+)?777\s+/` | permission | high |
| 递归修改属主 | regex | `chown\s+(-R\s+)?root:root\s+/` | permission | high |
| 管道执行远程脚本 | regex | `(curl\|wget).*\|\s*(bash\|sh\|zsh)` | network | critical |
| Netcat反弹Shell | regex | `nc\s+.*-e\s*/bin/(ba)?sh` | network | critical |
| Bash反弹Shell | regex | `bash\s+-i\s+>&\s+/dev/tcp/` | network | critical |
| Python Socket反弹 | regex | `python.*import\s+socket.*connect` | network | high |
| 禁用防火墙 | regex | `systemctl\s+(stop\|disable)\s+(firewalld\|iptables\|ufw)` | system | high |
| 停止关键服务 | regex | `systemctl\s+stop\s+(sshd\|systemd)` | system | high |
| 删除系统用户 | regex | `userdel\s+(-r\s+)?root` | system | critical |
| 修改shadow文件 | regex | `(echo\|printf\|cat.*>)\s*.*>\s*/etc/shadow` | system | critical |

#### 4.1.5 界面设计要求

- 规则列表支持按分类、严重等级、匹配类型筛选
- 规则支持搜索（名称和模式内容）
- 预置规则有明显标识，不可删除但可启停
- 新增/编辑规则时提供正则表达式测试功能
- 支持批量启停操作

---

### 4.2 脚本AI安全审计

#### 4.2.1 功能目标

所有脚本生成后（包括基线check/fix、漏洞修复、POC验证、自愈脚本），在黑名单审计之前先经过AI安全审计。AI审计专注于黑名单无法覆盖的上下文相关风险，如隐蔽权限提升、数据外泄、条件性恶意行为等。

#### 4.2.2 核心需求

| 编号 | 需求描述 | 优先级 |
|:---|:---|:---|
| REQ-101 | 所有脚本生成后先经过黑名单审计（确定性、无成本），再经过AI审计（上下文判断） | P0 |
| REQ-102 | AI审计通过专门的ScriptAuditPrompt执行，输出结构化JSON审计结果 | P0 |
| REQ-103 | 审计不通过时，将审计结果和原脚本反馈给LLM重新生成，最多重试3次 | P0 |
| REQ-104 | 3次仍违规则标记为audit_failed，记录完整审计日志，通知管理员 | P0 |
| REQ-105 | 统一的ScriptAuditService，所有脚本生成管线共用 | P0 |
| REQ-106 | 审计结果持久化到script_audit_log表，支持追溯和规则优化 | P1 |
| REQ-107 | AI审计支持配置开关，允许在LLM不可用时仅使用黑名单审计 | P1 |

#### 4.2.3 审计流程

```
LLM生成脚本
    ↓
ParseScript（提取脚本内容）
    ↓
黑名单审计（确定性检查，无LLM调用）
    ├─ 命中hard_block → 重试生成（注入失败原因）
    ├─ 命中soft_warn → 记录警告，继续AI审计
    └─ 通过 → 继续AI审计
    ↓
AI安全审计（LLM调用，上下文分析）
    ├─ 不通过 → 重试生成（注入审计结果）
    └─ 通过 → 脚本入库
    ↓
重试机制（最多3次循环）
    ├─ 第1次：正常生成
    ├─ 第2次：prompt注入第1次审计失败原因
    ├─ 第3次：prompt注入前两次审计失败原因
    └─ 第3次仍失败 → audit_failed，通知管理员
```

#### 4.2.4 AI审计提示词设计要素

**系统提示词**:
- 角色：资深Shell脚本安全审计专家
- 任务：审查脚本是否存在安全风险

**输入**:
- 脚本内容
- 脚本类型（check/fix/vulnerability_fix/poc_verify/self_healing）
- 黑名单审计结果（已通过的检查项）

**输出格式** (JSON):
```json
{
  "passed": true,
  "risk_level": "safe",
  "issues": [
    {
      "type": "privilege_escalation",
      "description": "问题描述",
      "line_range": "15-20",
      "suggestion": "修复建议"
    }
  ],
  "summary": "审计总结"
}
```

**审计重点**:
- 隐蔽权限提升（sudo嵌套、环境变量注入、PATH劫持）
- 数据外泄风险（编码后外传、DNS隧道、隐写术）
- 条件性恶意行为（时间触发、环境检测后执行恶意代码）
- 脚本意图与声明不一致（声明检查但包含修改操作）
- 资源耗尽攻击（大文件创建、无限循环、内存炸弹）

---

### 4.3 脚本下发前黑名单校验

#### 4.3.1 功能目标

在脚本从API Server下发到Server之前，增加一次黑名单校验作为纵深防御。即使脚本在生成阶段通过了审计，以下场景仍可能产生风险：
- 管理员手动修改了已审计的脚本内容
- 审计规则更新后，旧脚本不再合规
- 存储层被篡改

#### 4.3.2 核心需求

| 编号 | 需求描述 | 优先级 |
|:---|:---|:---|
| REQ-201 | 在task_service.go的dispatchToAgent()入口处增加黑名单校验 | P0 |
| REQ-202 | 校验失败时返回详细的违规信息（规则名、匹配模式、行号、上下文） | P0 |
| REQ-203 | 校验失败时记录审计日志并阻止下发 | P0 |
| REQ-204 | 校验失败的脚本在前端TaskLog中显示"脚本存在恶意命令，下发已阻止" | P1 |
| REQ-205 | Agent侧executor增加轻量级黑名单校验作为最后一道防线 | P1 |
| REQ-206 | 下发校验与生成审计共用同一个BlacklistChecker实例 | P0 |

#### 4.3.3 校验点位置

```
API Server
├── 脚本生成阶段（已有，增强）
│   └── ScriptGenerationService → validateScript()
│       └── 增强为：黑名单审计 + AI审计（需求4.2）
│
├── 下发前阶段（新增）
│   └── TaskService.dispatchToAgent()
│       └── BlacklistChecker.Check(scriptContent)  ← 新增校验点
│           ├─ 通过 → ForwardCommand()
│           └─ 不通过 → 返回错误 + 记录审计日志
│
└── Agent侧（新增）
    └── Executor.ExecuteCommand()
        └── BlacklistChecker.Check(scriptContent)  ← 最后防线
            ├─ 通过 → 执行脚本
            └─ 不通过 → 返回执行失败 + 上报事件
```

---

### 4.4 eBPF文件事件采集

#### 4.4.1 功能目标

激活已有的openat eBPF程序，实现文件访问事件的实时采集。接入Go层loader、pipeline和事件上报链路，与Sigma规则引擎集成。

#### 4.4.2 现状分析

| 组件 | 状态 | 说明 |
|:---|:---|:---|
| openat.bpf.c | **已完成** | tracepoint/syscalls/sys_enter_openat，捕获pid/uid/flags/comm/filename |
| .bpf.o编译 | **已完成** | bpf/obj/openat.bpf.o已存在 |
| Go Loader接入 | **未完成** | LoadAll()未加载openat，无FileEvent Go结构体 |
| Pipeline处理 | **未完成** | processEvent()无openat case |
| Sigma规则映射 | **已预留** | buildEventMap()已有"file_access"->"file_event"映射 |

#### 4.4.3 核心需求

| 编号 | 需求描述 | 优先级 |
|:---|:---|:---|
| REQ-301 | 新增FileEvent Go结构体（pid, uid, flags, comm, filename, timestamp） | P0 |
| REQ-302 | 在LoadAll()中加载openat程序 | P0 |
| REQ-303 | 在processEvent()中新增openat事件处理 | P0 |
| REQ-304 | 在pipeline中将file_access事件映射到Sigma规则引擎 | P0 |
| REQ-305 | 内核态过滤：只捕获对敏感路径（/etc/, /root/, /var/, /tmp/）的访问 | P1 |
| REQ-306 | 用户态采样率配置，防止高频openat调用导致性能问题 | P1 |
| REQ-307 | 事件去重：同一进程对同一文件的短时间重复访问合并为一次事件 | P2 |

#### 4.4.4 事件数据结构

```go
type FileEvent struct {
    PID      uint32
    UID      uint32
    Flags    int32    // openat flags (O_RDONLY, O_WRONLY, O_RDWR, O_CREAT, etc.)
    Comm     [16]byte // 进程名
    Filename [256]byte // 文件路径
}
```

#### 4.4.5 事件分类映射

| flags组合 | 事件子类型 | 风险等级 |
|:---|:---|:---|
| O_RDONLY | file_read | 低 |
| O_WRONLY / O_RDWR | file_write | 中 |
| O_CREAT | file_create | 中 |
| O_WRONLY \| O_TRUNC | file_truncate | 高 |
| 敏感路径（/etc/shadow等）+ 任何写标志 | sensitive_file_write | 高 |

---

### 4.5 eBPF网络事件采集

#### 4.5.1 功能目标

激活已有的connect eBPF程序，实现网络连接事件的实时采集。增强C代码以支持IPv6和源地址捕获。

#### 4.5.2 现状分析

| 组件 | 状态 | 说明 |
|:---|:---|:---|
| connect.bpf.c | **已完成但不完整** | 仅IPv4，仅目标地址/端口，无源地址，无协议标识 |
| .bpf.o编译 | **已完成** | bpf/obj/connect.bpf.o已存在 |
| Go Loader接入 | **未完成** | LoadAll()未加载connect，无ConnEvent Go结构体 |
| Pipeline处理 | **未完成** | processEvent()无connect case |
| Sigma规则映射 | **已预留** | buildEventMap()已有"network_connect"->"network_connection"映射 |

#### 4.5.3 核心需求

| 编号 | 需求描述 | 优先级 |
|:---|:---|:---|
| REQ-401 | 增强connect.bpf.c支持IPv4和IPv6双栈 | P0 |
| REQ-402 | 捕获源地址和源端口 | P0 |
| REQ-403 | 新增ConnEvent Go结构体 | P0 |
| REQ-404 | 在LoadAll()中加载connect程序 | P0 |
| REQ-405 | 在processEvent()中新增connect事件处理 | P0 |
| REQ-406 | 内核态过滤：排除本地回环和常见内部通信 | P1 |
| REQ-407 | 用户态采样率配置 | P1 |
| REQ-408 | 高风险目标端口标记（如4444/5555等常见C2端口） | P2 |

#### 4.5.4 增强后的事件数据结构

```go
type ConnEvent struct {
    PID       uint32
    UID       uint32
    Comm      [16]byte  // 进程名
    Family    uint16    // AF_INET(2) 或 AF_INET6(10)
    SAddr     [16]byte  // 源地址（IPv4用前4字节）
    DAddr     [16]byte  // 目标地址（IPv4用前4字节）
    SPort     uint16    // 源端口（网络字节序）
    DPort     uint16    // 目标端口（网络字节序）
    Protocol  uint8     // IPPROTO_TCP(6) / IPPROTO_UDP(17)
}
```

---

### 4.6 eBPF内核版本适配

#### 4.6.1 功能目标

确保eBPF程序在不同Linux内核版本上可靠运行，实现ringbuf/perf buffer自动降级、BTF/非BTF兼容、以及/proc轮询兜底。

#### 4.6.2 核心需求

| 编号 | 需求描述 | 优先级 |
|:---|:---|:---|
| REQ-501 | Go层内核能力检测：BTF可用性、ringbuf支持、内核版本 | P0 |
| REQ-502 | ringbuf -> perf buffer -> /proc 轮询三级降级策略 | P0 |
| REQ-503 | CO-RE 和非CO-RE 两套编译产物 | P1 |
| REQ-504 | Makefile增加bpf-core和bpf-noncore构建目标 | P1 |
| REQ-505 | Agent启动时输出内核能力检测报告到日志 | P1 |
| REQ-506 | 运行时eBPF事件丢失监控（ring buffer溢出计数） | P2 |

#### 4.6.3 内核适配策略

```
Agent启动
    ↓
检测内核版本和能力
    ├─ Kernel >= 5.8 + BTF → CO-RE + ringbuf（最优方案）
    ├─ Kernel >= 5.4 + BTF → CO-RE + perf buffer
    ├─ Kernel >= 4.18 → 非CO-RE + perf buffer
    └─ Kernel < 4.18 → /proc轮询（兜底方案）
    ↓
加载对应的eBPF程序
    ↓
启动事件采集循环
```

---

### 4.7 智能体优化

#### 4.7.1 功能目标

优化ReAct智能体的迭代效率、工具调用可靠性和可观测性，降低无效token消耗，提升分析质量。

#### 4.7.2 核心需求

| 编号 | 需求描述 | 优先级 |
|:---|:---|:---|
| REQ-601 | 降低最大迭代次数从50到20，提高单轮分析质量 | P0 |
| REQ-602 | 放宽无动作容忍从2次到3次，给LLM更多思考空间 | P0 |
| REQ-603 | 优化工具描述，减少工具名模糊匹配的发生 | P1 |
| REQ-604 | 改进Observation截断策略：结构化输出保留首尾，列表按严重度排序截断 | P1 |
| REQ-605 | 增加工具调用安全边界：单会话上限100次、单工具频率限制 | P1 |
| REQ-606 | 增强可观测性：迭代次数、工具调用次数、token消耗、失败率统计 | P0 |
| REQ-607 | Session超时自动清理（30分钟无活动归档） | P2 |
| REQ-608 | 启动时从DB恢复最近24小时内的活跃会话 | P2 |

#### 4.7.3 优化前后对比

| 指标 | 当前值 | 优化目标 | 说明 |
|:---|:---|:---|:---|
| 最大迭代次数 | 50 | 20 | 减少无效循环 |
| 无动作容忍 | 2次 | 3次 | 给LLM更多思考空间 |
| Observation截断 | 12000字符硬截断 | 智能截断 | 保留结构化信息 |
| 工具调用上限 | 无限制 | 100次/会话 | 防止失控 |
| 分析耗时统计 | 无 | 有 | 可观测性 |
| 失败率统计 | 无 | 有 | 可观测性 |

---

## 5. 非功能性需求

### 5.1 性能要求

| 指标 | 要求 |
|:---|:---|
| 黑名单校验延迟 | < 50ms（单脚本） |
| AI审计延迟 | < 30s（单次LLM调用） |
| eBPF事件采集CPU开销 | < 5%（单核） |
| eBPF事件采集内存开销 | < 50MB |
| 下发前校验不阻塞正常下发 | P99 < 100ms |

### 5.2 可靠性要求

| 指标 | 要求 |
|:---|:---|
| eBPF加载失败时自动降级到/proc | 100%覆盖 |
| AI审计LLM不可用时降级为仅黑名单审计 | 自动降级 |
| 脚本审计日志完整性 | 所有审计结果100%记录 |

### 5.3 兼容性要求

| 维度 | 要求 |
|:---|:---|
| Linux内核 | 4.18+（最低/proc轮询），5.8+（完整eBPF） |
| CPU架构 | amd64, arm64 |
| 数据库 | PostgreSQL 14+ |

---

## 6. 版本交付范围

| 模块 | 交付内容 |
|:---|:---|
| API Server | ScriptAuditService、BlacklistChecker、命令审计配置API、审计日志API |
| Agent | eBPF loader扩展（openat/connect）、内核适配、事件pipeline扩展 |
| Frontend | 命令审计配置页、审计日志页、脚本审计状态展示 |
| Database | command_audit_rules表、script_audit_log表、system_configs表 |

---

## 7. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|:---|:---|:---|
| AI审计增加LLM调用成本 | 运营成本上升 | 黑名单先过滤，减少AI调用次数；支持仅黑名单模式 |
| openat高频调用影响性能 | Agent CPU升高 | 内核态过滤 + 用户态采样 + 事件去重 |
| 内核版本碎片化 | 部分主机无法使用eBPF | 三级降级策略 + /proc兜底 |
| 正则匹配ReDoS | 黑名单校验阻塞 | 正则预编译 + 执行超时保护 |

---

## 8. 附录：V5.7需求追溯矩阵

| 需求编号 | 需求名称 | 设计文档 | 优先级 |
|:---|:---|:---|:---|
| REQ-001~008 | 命令审计黑名单配置 | command_audit_blacklist_config_design.md | P0-P2 |
| REQ-101~107 | 脚本AI安全审计 | ai_audit_retry_design.md | P0-P1 |
| REQ-201~206 | 下发前黑名单校验 | pre_dispatch_blacklist_validation_design.md | P0-P1 |
| REQ-301~307 | eBPF文件事件采集 | ebpf_file_network_event_design.md | P0-P2 |
| REQ-401~408 | eBPF网络事件采集 | ebpf_file_network_event_design.md | P0-P2 |
| REQ-501~506 | eBPF内核版本适配 | ebpf_kernel_adaptation_design.md | P0-P2 |
| REQ-601~608 | 智能体优化 | agent_optimization_design.md | P0-P2 |
| - | 统一脚本审计服务 | script_audit_service_design.md | P0 |

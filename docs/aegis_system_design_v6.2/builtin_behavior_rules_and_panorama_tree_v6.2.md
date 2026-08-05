# Aegis V6.2 首批内置行为规则与全景树设计

**版本**：6.2  
**日期**：2026-08-06
**状态**：内置规则目录、工具命令匹配和会话范围展示已按当前实现更新

> 当前实现基线见 [current_implementation_baseline_2026-08-06.md](current_implementation_baseline_2026-08-06.md)。
> 内置规则是只读目录；前端不再创建/编辑/发布策略。Agent Guard 工具命令规则由
> api-server 匹配，Agent eBPF 只做进程事实和 PID/PPID 关联，DC 只做投影。

## 1. 设计目标

V6.2 第一批内置以下五个规则族：

| 稳定规则 ID | 名称 | 行为域 |
| --- | --- | --- |
| `AGB-BUILTIN-001` | 操作敏感目录 | file |
| `AGB-BUILTIN-002` | 外部网络连接 | network |
| `AGB-BUILTIN-003` | 文件生成 | file |
| `AGB-BUILTIN-004` | 敏感命令执行 | tool/process |
| `AGB-BUILTIN-005` | 提权行为 | identity/process |

规则定义随 Aegis 版本发布并在数据库中版本化。前端以“行为监控”和“工具命令”
两个只读内置策略视图展示规则详情，包括中文/英文名称、版本、描述、类别、严重级别、
动作、执行位置、所需证据、允许条件、MITRE、默认参数和 Schema。历史策略和 Bundle
接口仍保留兼容性，但不再作为当前页面的编辑/发布入口。修改检测语义必须发布新的
rule version，确保历史 finding 可复核。

五个规则不是五个简单字符串匹配。每个规则都需要：

```text
真实 Agent 进程归属
  + 规范化行为
  + 资源或状态证据
  + outcome/errno
  + allow/negative condition
  -> rule hit
```

单个 rule hit 表示“发生了需要关注的行为”，不一定表示攻击。通用 OS 规则仍可由
Agent/DC 的既有链路关联；Agent Guard 工具命中由 api-server 产生，DC 不重复创建。

## 2. 通用规则契约

```json
{
  "rule_key": "AGB-BUILTIN-001",
  "rule_version": 1,
  "name": "操作敏感目录",
  "source": "builtin",
  "engine": "agent_and_dc",
  "categories": ["file"],
  "default_enabled": true,
  "default_severity": "medium",
  "default_action": "alert",
  "recommended_action": "alert",
  "parameters_schema": {},
  "required_evidence": [],
  "allow_conditions": [],
  "mitre": [],
  "immutable": true
}
```

字段要求：

| 字段 | 含义 |
| --- | --- |
| `engine` | 历史兼容字段，可为 `agent_atomic`、`dc_single_event`、`dc_correlation` 或 `agent_and_dc`；当前 `AGB-BUILTIN-004` 的工具事件执行位置由 `execution_location=api_server_tool_event` 明确覆盖 |
| `default_action` | 首次部署使用的动作，首批规则不得默认全局 freeze |
| `recommended_action` | 证据充分、完成灰度后的建议动作 |
| `parameters_schema` | 前后端共同校验的参数 JSON Schema |
| `required_evidence` | 构成 hit 所需的 actor/resource/outcome 字段 |
| `allow_conditions` | 规则内置反例，策略还可追加业务 allowlist |
| `immutable` | 内置语义不可原地编辑 |

策略只保存引用和覆盖：

```json
{
  "rule_key": "AGB-BUILTIN-002",
  "rule_version": 1,
  "enabled": true,
  "severity_override": "medium",
  "action_override": "alert",
  "parameters": {},
  "exceptions": []
}
```

如果 Agent/DC 不支持指定 rule version，下发状态必须为 `unsupported_rule_version`，不能悄悄使用其他版本。

## 3. `AGB-BUILTIN-001` 操作敏感目录

### 3.1 检测对象

检测已归属 Agent 的进程对敏感目录或文件执行：

```text
open_intent
read_observed
write
create
truncate
delete
rename
chmod
chown
execute
```

首批内置资源分组：

| 分组 | 代表性路径 | 默认读风险 | 默认修改风险 |
| --- | --- | --- | --- |
| `credential` | `/etc/shadow`、`/etc/gshadow`、`/root/.ssh/**`、`/home/*/.ssh/**` | high | critical |
| `privilege_policy` | `/etc/sudoers`、`/etc/sudoers.d/**`、`/etc/pam.d/**` | high | critical |
| `cloud_or_cluster_credential` | `/root/.aws/**`、`/home/*/.aws/**`、`/root/.kube/**`、`/home/*/.kube/**`、`/etc/kubernetes/pki/**` | high | critical |
| `persistence` | `/etc/systemd/system/**`、`/etc/cron.d/**`、`/var/spool/cron/**`、用户 shell profile | medium | high |
| `security_control` | Aegis 安装路径、配置路径、本地策略和 pin 的 BPF map 路径 | high | critical |
| `container_control` | Docker/containerd/CRI/Podman socket 和配置路径 | high | critical |

“Aegis 安装路径”由 Agent 启动时根据真实可执行文件和配置目录注册，不在规则中猜测固定目录。

### 3.2 事件条件

```text
category = file
resource.classification in configured_sensitive_classes
operation in configured_operations
actor belongs to Agent runtime instance
```

必需展示字段：

```text
PID
PPID
cmdline
operation
file_name
raw_path
resolved_path
host_path（可解析时）
outcome/errno
resource classification
```

### 3.3 默认动作

| 条件 | 默认 severity | 默认动作 |
| --- | --- | --- |
| read credential/security control | high | alert |
| write/delete/rename/chmod/chown privilege/security control | critical | alert |
| read persistence/configuration | medium | audit |
| write persistence path | high | alert |

首期不默认 deny。管理员完成主机和 Agent Profile 灰度后，可以对 exact/prefix 且可由 BPF LSM 可靠匹配的规则启用 deny。路径 unresolved 时不得自动 freeze。

### 3.4 例外

例外至少支持：

- Agent/Profile/host/host group。
- process executable + digest。
- path/resource group。
- operation。
- 用户/UID。
- 时间范围和变更单号。

只按 `process_name` 配置例外无效，至少需要 resolved executable 或签名/digest 证据。

## 4. `AGB-BUILTIN-002` 外部网络连接

### 4.1 检测对象

检测已归属 Agent 的进程主动连接非本机、非内网、非管理员信任网段的地址：

```text
category = network
operation = connect
direction = outbound
externality = external
```

地址分类顺序：

1. 用户配置的 `trusted_cidrs/trusted_domains`。
2. 主机和容器本地地址。
3. loopback、link-local。
4. IPv4 private、IPv6 ULA、集群/容器网段。
5. 保留、文档、multicast、CGNAT 等 `special_or_unknown`。
6. 其余公网可路由地址标记 `external`。

`special_or_unknown` 不得自动归为公网恶意连接。管理员可以覆盖分类。

### 4.2 事件证据

必需展示字段：

```text
PID
PPID
cmdline
protocol
destination_ip
destination_port
observed_domain（有可信 DNS 关联时）
direction
outcome/errno
connection_source
```

域名规则：

- `observed_domain` 只来源于同一 execution unit、进程或 cgroup 的 DNS 证据关联。
- 反向 DNS 只能显示为 `reverse_lookup_hint`，不能作为访问事实。
- 通过企业代理连接时，网络事实是代理地址；工具 Hook/argv 中的目标只能作为补充证据，标记来源。

### 4.3 默认动作

| 条件 | severity | 默认动作 |
| --- | --- | --- |
| 首次访问公网 IP/域名 | medium | alert |
| trusted destination | info | audit/抑制 finding |
| 外链前发生凭据读取、打包或文件批量读取 | high/critical | 由关联规则告警 |
| 访问公网失败 | low/medium | audit，保留 attempt |

首期不因“公网连接”单点默认 deny。管理员可以对明确禁止的 CIDR/域名或端口启用 socket connect deny；域名 deny 必须说明 DNS/代理限制。

### 4.4 例外

- 企业代理、制品库、代码仓库、模型 API、软件更新源。
- 允许的 CIDR、IP、domain suffix、port/protocol。
- 特定 Agent/Profile/process digest。
- 临时审批窗口。

域名 suffix 匹配必须按 DNS label 边界，`example.com` 不能匹配 `example.com.invalid`。

## 5. `AGB-BUILTIN-003` 文件生成

### 5.1 检测对象

检测 Agent 进程成功创建一个此前不存在的文件：

```text
category = file
operation = create
outcome = success
resource.inode_created = true
```

仅观察到带 `O_CREAT` 的 open intent 但最终失败时，记录 `create_attempt`，不能显示为“文件已生成”。

### 5.2 事件证据

必需展示字段：

```text
PID
PPID
cmdline
file_name
raw_path
resolved_path
host_path（可解析时）
owner UID/GID
mode
size metadata
executable flag
hidden flag
SHA-256（用户态异步计算成功时）
outcome/errno
```

不采集文件内容。hash 未计算时显示 `hash_status=not_collected|skipped|failed`，不能显示空 hash 冒充校验成功。

### 5.3 风险分层

| 文件位置/属性 | severity | 默认动作 |
| --- | --- | --- |
| Agent workspace 普通文件 | info | audit |
| `/tmp` 等临时目录普通文件 | low | audit |
| 隐藏文件、脚本或可执行权限文件 | medium | alert |
| persistence/sensitive/security control 路径 | high/critical | alert |
| 外链下载后生成并执行 | high | 由关联规则提升 |

该规则必须采集所有 Agent 文件创建事实，但前端默认可以隐藏 info/low，避免噪声淹没高危行为。

## 6. `AGB-BUILTIN-004` 敏感命令执行

### 6.1 检测对象

检测 Agent 进程执行具有高权限、网络传输、隔离控制、持久化、破坏或防御规避能力的命令。

首批命令分类：

| 分类 | 代表命令 | 关注证据 |
| --- | --- | --- |
| `network_transfer` | `curl`、`wget`、`nc/ncat`、`socat`、`ssh/scp` | executable、argv、外链事件 |
| `privilege` | `sudo`、`su`、`pkexec`、`setcap` | argv、后续 credential change |
| `permission_change` | `chmod`、`chown`、`setfacl`、`setcap` | argv、目标文件和权限 before/after |
| `namespace_mount` | `nsenter`、`unshare`、`mount/umount`、`chroot` | argv、namespace/mount 事件 |
| `account_persistence` | `useradd/usermod`、`crontab`、`systemctl` | argv、文件/服务状态变化 |
| `destructive` | `rm`、`dd`、`shred`、`mkfs*` | argv、目标资源和实际结果 |
| `security_control` | `auditctl`、防火墙工具、终止/修改 Aegis 的命令 | argv、目标进程/配置和结果 |

列表是规则分类，不是恶意命令黑名单。上述命令在开发、运维和 Agent 正常工作中可能合法。

### 6.2 匹配方式

AGB-BUILTIN-004 的规则输入是可信 Native Hook 工具事件，不是 Agent eBPF exec
事件，也不是 DC 从全量进程树推导出的命令。api-server 消费：

```text
tool_call_started
tool_call_completed
tool_call_failed
```

同一 `tool_call_id` 的开始和终态合并为一个逻辑调用；规则在工具调用完成或失败
事件到达时对工具名称、工具输入/attributes 中提取的命令和结果进行匹配。匹配优先级：

1. resolved executable path + inode/dev 或 digest。
2. executable path + basename。
3. 规范化 argv 结构条件。
4. shell `-c` 中可见的命令作为补充证据。

禁止只对完整 cmdline 做无边界 substring 匹配。例如参数文件名包含 `sudo` 不能命中 privilege command。
Shell `-c` 只有在 Hook 工具输入中可见时才作为命令证据；仅有 eBPF exec 不能单独
形成 Agent Guard 工具命中。

Agent eBPF/`/proc` 只补充实际进程关联：根据命令行、发生时间、PID start_ticks、
父子关系和 tool correlation 解析 PID/PPID。关联失败时 Finding 仍保留工具命中，
但标记 `correlation_status=unattributed`，不能用 Agent 主进程 PID 冒充。

必需展示字段：

```text
PID
PPID
executable
cmdline/脱敏 argv
cwd
command category
matched argument condition
outcome/exit code（可获得时）
tool name
tool call ID
session ID
source event ID
```

### 6.3 默认动作

| 条件 | severity | 默认动作 |
| --- | --- | --- |
| 仅执行敏感命令，无其他高危证据 | medium | alert |
| 命令失败且未产生状态变化 | low/medium | audit |
| 命令后发生提权、持久化、破坏或逃逸 | high/critical | 关联规则告警 |
| 明确操作 Aegis 自保护目标 | critical | 按自保护策略 deny/freeze |

AGB-BUILTIN-004 的默认结果由 api-server 写入 `agent_security_findings`，直接引用
Hook 工具事件的 `raw_event_id`，并记录 `evidence_graph.rule_owner=api-server`。
eBPF 关联信息只是补充证据。BPF LSM/Agent 本地策略仍可对独立的自保护或逃逸
规则做前置 deny，但不能把本规则的工具命中重复上报为 Agent/DC 命中。

## 7. `AGB-BUILTIN-005` 提权行为

### 7.1 检测对象

检测 Agent 进程尝试或成功获得高于原基线的身份或 capability：

```text
setuid/setgid/setresuid/setresgid
capset/capability gain
exec setuid/setgid binary
sudo/su/pkexec + credential transition
user namespace/capability transition
```

### 7.2 Attempt 与 Confirmed

提权必须区分：

| 状态 | 证据 |
| --- | --- |
| `attempted` | 调用了提权 syscall/命令，但失败或没有观察到身份变化 |
| `succeeded` | outcome success，且 effective UID/GID/capability before/after 证明权限提高 |
| `inconclusive` | 事件丢失、before/after 不完整或远程不可观测 |

必需展示字段：

```text
PID
PPID
cmdline
operation
real/effective/saved UID before/after
GID before/after
capability permitted/effective before/after
user namespace before/after
outcome/errno
```

“UID 数值变化”不必然等于提权。规则需要结合目标 UID、capability 和 namespace；容器 user namespace 内 UID 0 不能直接显示为宿主机 root。

### 7.3 默认动作

| 条件 | severity | 默认动作 |
| --- | --- | --- |
| 提权 attempt 失败 | medium | alert |
| 获得宿主机 effective UID 0 | critical | alert，灰度后可 freeze |
| 获得 Profile 未声明 capability | high/critical | alert |
| 容器内预期 UID 切换 | info/medium | allow/audit |
| 提权后访问敏感资源、逃逸或外链 | critical | 关联规则可请求 freeze |

只有内核可证明的高置信 credential/capability 变化才能作为自动动作证据。仅执行 `sudo` 但失败不能自动 freeze。

## 8. 五规则联合攻击链

首批内置关联模板（通用 OS 事实与工具事件可在服务端形成证据关系；不表示 DC
重新执行工具规则）：

```text
AGB-BUILTIN-002 外链
  -> AGB-BUILTIN-003 文件生成
  -> AGB-BUILTIN-004 敏感命令执行
  -> AGB-BUILTIN-005 提权
  -> AGB-BUILTIN-001 操作敏感目录
```

关联条件：

- 同一 Agent instance。
- 优先同一 Native Hook 真实 session；没有可信 session ID 时不得伪造会话关联。
- 同一 execution unit 或存在真实 process/correlation 边。
- 默认窗口 5 分钟，可按策略调整。
- 前后事件 outcome 和资源关系一致。
- 不跨主机依赖时间接近直接合并。

风险提升示例：

| 组合 | 建议结论 |
| --- | --- |
| 外链 + 文件生成 | suspicious/medium |
| 外链 + 文件生成 + 敏感命令执行 | suspicious/high |
| 敏感命令 + confirmed 提权 | malicious/high/critical |
| confirmed 提权 + 修改敏感目录 | malicious/critical |
| 五项形成完整链且无 allow condition | malicious/critical，可按策略 freeze |

Finding 必须逐项引用五个 rule hit 对应的 behavior event ID，不能只保存一句模型摘要。

## 9. 全景树信息架构

全景图第一版使用树状关系，不使用自由布局关系图。外层页面不显示全景；
点击 Agent 后先对真实会话 ID 分页，再按运行实例和实际进程父子关系展示行为。
安全分析不使用这棵全量行为树，而只展示命中规则的工具调用及关联进程字段。

```text
选中 Agent：Codex @ host-a · 2 个运行实例
├── 实例：controller PID 4100
│   └── Session：task-7（confirmed）
│       └── Execution Unit：linux_namespace
│           └── bash · PID 4120 · PPID 4100
│               └── curl · PID 4121 · PPID 4120
│                   ├── [命令] exec curl ...
│                   ├── [外链] <PUBLIC-IP>:443/TCP
│                   └── [文件生成] /workspace/payload.sh
└── 实例：controller PID 4400
```

Host 作为 Agent 根节点数据字段返回，不单独成为树节点。前端详情抽屉选中
Agent 后返回/展示其子树：

```text
Host
  -> AgentAsset/AgentType
    -> AgentRuntimeInstance
      -> BehaviorSession
        -> AgentExecutionUnit
          -> Process
            -> Child Process
            -> Command Operation
            -> File Operation
            -> Network Operation
            -> Identity/Privilege Operation
            -> Isolation/Kernel Operation
            -> Rule Hit/Finding
```

同一文件或网络地址被多个进程操作时，在各自进程节点下重复显示资源摘要；详情页通过 resource hash 查询所有关联行为。第一版不通过跨树连线破坏父子关系。
同类型多个 controller 实例必须分开；跨 Agent 发起关系只作为
related/launched-by 证据，不把其他 Agent 的全景合并进当前抽屉。

## 10. 全景树节点契约

公共字段：

```ts
interface PanoramaTreeNode {
  id: string
  parent_id?: string
  node_type:
    | 'agent_asset'
    | 'instance'
    | 'session'
    | 'execution_unit'
    | 'process'
    | 'command'
    | 'file'
    | 'network'
    | 'privilege'
    | 'isolation'
    | 'rule_hit'
    | 'finding'
  occurred_at?: string
  label: string
  severity?: 'info' | 'low' | 'medium' | 'high' | 'critical'
  outcome?: 'success' | 'failure' | 'denied' | 'unknown'
  has_children: boolean
  child_count: number
  data: Record<string, unknown>
}
```

### 10.1 Process 节点

页面必须直接显示：

```text
process name
PID
PPID
cmdline
```

详情显示：

```text
start ticks
exe
cwd
UID/GID
container/cgroup
namespace
first/last seen
```

PID 必须和 start ticks 组合为节点身份，不能只用 PID。

### 10.2 Command 节点

直接显示：

```text
PID
executable
cmdline（脱敏）
cwd
exit code/signal
matched sensitive command category
```

如果 cmdline 不完整，显示 `partial` 标记和原因。

### 10.3 File 节点

直接显示：

```text
operation
file_name
full resolved path
outcome
```

详情显示 raw path、host path、inode/dev、owner/mode、hash status 和 resource classification。禁止展示文件内容。

### 10.4 Network 节点

直接显示：

```text
destination_ip:port
protocol
observed_domain（存在时）
outcome
externality
```

详情显示 direction、source address、DNS evidence source、proxy limitation 和 trusted rule。

### 10.5 Privilege 节点

直接显示：

```text
operation
before UID/GID/capability
after UID/GID/capability
attempted/succeeded/inconclusive
```

### 10.6 Rule/Finding 节点

直接显示规则名称、rule ID/version、severity、decision source 和 action 状态。点击后跳转 Finding 详情，并能回到引用的行为节点。

## 11. 前端页面

### 11.1 内置规则页

内置规则作为“智能体事件感知与防护”子页的内部视图，不增加侧边栏菜单。可通过以下深链进入：

```text
/detection/agent-guard/events?view=rules
```

规则卡片固定展示五项：

- 名称、稳定 rule ID、version。
- 中文名称、英文名称和描述。
- 默认 severity/action、执行位置和规则归属。
- 所需 evidence、allow conditions、MITRE 映射。
- 默认参数、参数 Schema、规则 digest。
- 行为监控/工具命令所属内置策略视图。

内置规则不能删除或原地编辑。当前页面不提供 enabled/disabled、范围、参数覆盖、
例外、灰度、发布或下发状态编辑；历史策略 API 仅为旧 Bundle 和审计数据兼容保留。
Agent Guard 工具适配器、会话 Hook 以及 Codex/Claude Code/OpenClaw/Hermes/Zcode
注入使用单独的运行时设置开关，开启即立即请求 Agent 应用，关闭即清理 Hook 并停止上报。

### 11.2 Agent 详情抽屉中的行为全景

“智能体事件感知与防护”外层只显示 Agent 基本信息列表。点击 Agent 后打开
详情抽屉，实例/session 通过 query 定位：

```text
/detection/agent-guard/events?asset_id=<id>&detail_tab=panorama
/detection/agent-guard/events?asset_id=<id>&instance_id=<id>&detail_tab=panorama
/detection/agent-guard/events?asset_id=<id>&session_id=<id>&detail_tab=panorama
```

抽屉结构：

```text
抽屉顶部：选中 Agent 基本信息、会话 ID 分页 selector
内部 Tabs：行为全景 / 安全分析
全景顶部：运行实例、Session、时间范围、风险、规则筛选
左侧：Agent 行为全景树
安全分析：命中规则名称、命中工具、工具输入/结果、匹配命令行和关联 PID/PPID
```

默认筛选最近 30 分钟、当前 session；允许选择：

- 全部/仅风险。
- 五个内置规则。
- process/file/network/identity/isolation。
- success/failure/denied/unknown。
- 展开深度和隐藏 info/low。

树交互：

- 具体实例默认展开 instance → session → unit → 首层 process；“全部实例”
  下各 instance 为并列分支，只默认展开高风险实例。
- 点击 process 懒加载子进程和操作。
- 同一进程下操作按 `occurred_at + agent_sequence` 排序。
- 命中规则的操作节点显示风险徽标。
- 点击规则节点高亮它引用的行为节点。
- 支持搜索 PID、cmdline、文件名/路径、IP/domain。
- 支持“定位到 PID”和“定位到事件”。
- 大树使用虚拟滚动、懒加载和每节点分页。
- 安全分析严格按当前 session 查询，不显示其他 session 或全量 findings；没有 PID
  关联时显示 `unattributed`，不回退为统一的 controller PID。

## 12. 全景树 API

```text
GET /api/v1/agent-guard/panorama
GET /api/v1/agent-guard/instances/:id/panorama
GET /api/v1/agent-guard/sessions/:id/panorama
GET /api/v1/agent-guard/panorama/nodes/:node_id/children
```

查询参数：

```text
host_id
instance_id
session_id
start_time/end_time
categories
rule_keys
severity
outcomes
risk_only
search
depth
cursor/page_size
```

响应：

```json
{
  "root": {
    "id": "instance:uuid",
    "node_type": "instance",
    "label": "Codex",
    "has_children": true,
    "child_count": 2,
    "data": {
      "host_id": "uuid",
      "controller_pid": 4001,
      "controller_cmdline": "codex ..."
    }
  },
  "children": [],
  "next_cursor": "",
  "completeness": {
    "visibility": "partial",
    "lost_events": 2,
    "limitations": ["tool_semantics_unobservable"]
  }
}
```

`node_id` 是服务端签名/编码的短期查询标识，客户端不得自行拼接 SQL 条件。API 必须校验 node 所属 host/instance/session 和用户权限。

## 13. 测试与验收

### 13.1 规则

- 五个 rule ID/version 稳定，migration seed 幂等。
- 内置定义不可删除或原地修改。
- 敏感目录规则展示 PID、cmdline、文件名和完整路径。
- 外链规则正确区分公网、内网、loopback、IPv6 ULA、CGNAT 和自定义 trusted CIDR。
- DNS 无可信关联时只显示 IP，不伪造 domain。
- 文件 create 成功与 create attempt 分离。
- 敏感命令按 executable/argv 边界匹配，不使用无边界 substring。
- 提权 attempt、succeeded、container user namespace root 分离。
- 五规则组合形成一个可复核 finding，不跨 session 错误关联。

### 13.2 全景树

- 节点严格挂在真实发起 PID 下。
- process 节点展示 PID、PPID、cmdline。
- file 节点展示 filename、resolved path 和 operation。
- network 节点展示 destination IP/domain、port 和 protocol。
- PID reuse 生成不同节点。
- 乱序事件按 agent sequence 稳定排序。
- search 可以定位 PID、cmdline、文件路径和连接地址。
- 万级行为下使用懒加载/虚拟滚动，不一次返回全树。
- 缺失、截断和远程不可观测在树根和节点上可见。
- 无 evidence 权限时 cmdline/path 按后端返回脱敏摘要，不依赖前端遮挡。

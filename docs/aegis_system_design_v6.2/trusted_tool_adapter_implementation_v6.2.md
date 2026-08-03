# V6.2 可信工具 Adapter 实施契约

## 1. 启用条件

工具语义默认不可观测。只有以下条件同时满足，Agent 才创建本地 Unix hook
socket：

1. api-server `AGENT_GUARD_TOOL_ADAPTER_ENABLED=true`；
2. 已发布 policy 的 `collection.tool_adapter_enabled=true`，且 categories 包含
   `tool`；
3. Agent 本地 `AgentGuardToolAdapterEnabled=true`；
4. `AgentGuardToolSourceManifest` 和 `AgentGuardToolHookSocket` 指向有效配置；
5. bundle、manifest、peer credential 和逐事件签名全部验证通过。

任一条件不满足时保持 `tool_semantics_unobservable`，不得从进程名、命令行或
普通终端输出推断工具名称。

## 2. Source manifest

manifest 为 root/Agent EUID 所有的普通文件，文件及 adapter artifact 不允许
group/world write。`digest` 是将 manifest 的 `digest` 置空后按实现结构编码的
canonical JSON SHA-256。

```json
{
  "schema": "aegis.agent_guard.tool_sources.v1",
  "socket": {
    "mode": "0660",
    "group_id": 991
  },
  "sources": [
    {
      "source_id": "claude-code-hook-v1",
      "source_type": "adapter_hook",
      "product": "claude-code",
      "version": "1.0.0",
      "verifier": "ed25519",
      "public_key": "BASE64_ED25519_PUBLIC_KEY",
      "allowed_uids": [1000],
      "allowed_gids": [991]
    }
  ],
  "digest": "sha256:64_HEX_DIGITS"
}
```

- 本地 ingress 的 `source_type` 仅允许 `agent_official`、`adapter_hook`、
  `aegis_wrapper`；manifest 也可登记 `aegis_remote_sensor`，但它只用于验证
  嵌套远端证据，不能直接作为本地 ingress source。
- 每个 source 必须固定 Ed25519 public key 和至少一个 `allowed_uids`；
  `allowed_gids` 可选。
- socket 默认 `0600`。只有显式 `0660 + group_id` 才允许受控组访问；仍需逐条
  通过 `source_id + SO_PEERCRED UID/GID + Ed25519`，group membership 或
  correlation token 单独都不是认证。
- artifact path/digest 只能附加证明 adapter artifact，不能替代单条事件签名。

## 3. Hook 事件

socket 使用一行一个 JSON 事件。事件不得包含工具输出、prompt、stdin/stdout、
环境变量值、文件内容、网络 payload、password、token 或 secret。示例字段：

```json
{
  "event_id": "UUID",
  "source_id": "claude-code-hook-v1",
  "source_version": "1.0.0",
  "operation": "tool_call_started",
  "tool_name": "shell",
  "tool_call_id": "UUID",
  "correlation_token": "HIGH_ENTROPY_EPHEMERAL_VALUE",
  "pid": 12345,
  "start_ticks": 99887766,
  "process_event_id": "UUID",
  "resource_event_ids": ["UUID"],
  "occurred_at": "2026-08-03T10:00:00Z",
  "issued_at": "2026-08-03T10:00:00Z",
  "proof": "BASE64_ED25519_SIGNATURE"
}
```

允许的 operation 为 `tool_call_started`、`tool_call_completed`、
`tool_call_failed`。签名覆盖 proof 置空后的本地事件；嵌套 remote evidence 使用
远端 source key 独立签名。PID 必须与 start_ticks 同时匹配一个已确认 execution
unit。event ID 重放、签名篡改、过期时间、未授权 UID/GID 或未知 source 均拒绝。

Agent 只在内存中使用 correlation token，并上报 `sha256:<64hex>`；原 token
不写入 RuntimeEvent、日志或数据库。DC 只把 hash 当 join key，并且必须再次
匹配真实 process/resource 或远端 OS sensor event 后才建立证据边。

### 3.1 Codex 会话生命周期元数据

同一签名 manifest/socket 还允许独立的 metadata-only 生命周期 operation：
`session_started`、`session_activated`、`session_ended`。它们必须携带非空
`external_session_id`、根 `pid + start_ticks`、时间和 Ed25519 proof，不携带工具名、
correlation token 或任何会话正文。

- `SessionStart` 产生 `session_started`，首个 Hook helper 的 PPID 固定为会话根。
- `PreToolUse` 产生 `session_activated`，用于共享 Codex app-server 在 fork 工具进程前
  切换当前真实会话，事件本身不产生工具语义。
- `SessionEnd` 产生 `session_ended`，精确关闭该 external session 的行为 session/unit。

生命周期由独立本地开关 `AgentGuardSessionHookEnabled` 控制，不会因为启用它而把
`collection.tool_adapter_enabled` 或工具语义自动升级为可信。工具事件仍严格遵守本章
原有三重灰度和 correlation 契约。

## 4. 远端证据和动作边界

远端关联采用精确 event ID、host、execution unit、hash 和 ±5 分钟窗口的有界
二阶段查询。远端事件还必须来自 `ebpf/procfs`、confirmed attribution、完整
visibility、无 drop/truncate/aggregation。条件不满足时保持
`remote_unobservable`。

可信工具语义只用于补充 Finding evidence graph。tool-only、remote token-only
和 AI-only 结论都不能创建自动 action，也不能提升 execution unit enforcement
coverage。

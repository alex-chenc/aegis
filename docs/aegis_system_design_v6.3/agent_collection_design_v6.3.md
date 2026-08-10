# Aegis V6.3 Agent 会话静态采集设计

## 1. 采集模型

V6.3 按 Uber ADR Sensor 的方式读取 AI 编码工具已经落盘的本地会话文件，不通过
Claude Code/Codex Hook 获取正文，也不安装文件监听器：

```text
Agent 定时任务/手工触发
  -> 枚举纳管 UID 的固定会话目录
  -> stat 过滤最近修改的 JSONL
  -> 从本地 byte cursor 读取新增完整行
  -> 来源 parser 归一化
  -> 本机脱敏
  -> 加密 spool 和批量上报
```

“静态采集”表示数据来源是磁盘快照。Aegis Agent 可周期性重复执行扫描以降低页面
延迟，但每次扫描都是只读文件枚举和解析，不向 Claude/Codex 进程注入回调，也不
依赖进程生命周期事件。

## 2. 模块边界

新增目录：

```text
agent/internal/agentsession/
  manager.go
  scheduler.go
  target_resolver.go
  scanner.go
  file_guard.go
  normalizer.go
  redactor.go
  cursor_store.go
  spool.go
  batcher.go
  capability.go
  adapters/
    claude/
      discovery.go
      parser.go
      schema_fingerprint.go
    codex/
      discovery.go
      parser.go
      schema_fingerprint.go
```

V6.3 不新增 `aegis-session-hook`、本地 ingress socket 或产品 Hook 配置。现有
`agentguard` Managed Hook 继续承担 V6.2 的运行实例/工具事件能力，但不是会话正文
采集的前置条件，开关和失败状态也不得与本模块耦合。

## 3. 与 ADR Sensor 的继承和调整

| 能力 | ADR Sensor | Aegis V6.3 |
| --- | --- | --- |
| 来源 | 本地 Claude/Codex JSONL | 相同 |
| 触发 | CLI 单次执行 | Agent 启动、固定周期、配置变更或手工扫描 |
| 发现 | `Path.glob("**/*.jsonl")` | 有界 root、深度、文件数、mtime 和时间预算 |
| 读取 | 每次读取完整文件 | 首次读取 + dev/inode/offset 增量读取 |
| 输出 | 完整 `AgentEvent`/JSON | item batch，经脱敏、spool、gRPC/Kafka |
| 增量 | 已导出 session 文件过滤 | 文件 cursor + item 幂等，支持活跃文件追加 |
| 监听/Hook | 无 | 无 |

保留 ADR 的 source-specific parser 和统一模型思想；为生产容量、断网恢复与敏感
信息边界增加 cursor、脱敏、配额和 ACK。

## 4. 纳管目标和静态目录发现

禁止扫描整个 `/home`、`/root`、挂载点或任意用户目录。扫描目标只来自服务端签名
collection bundle 中显式的 UID/home 映射，或管理员显式配置的附加根。

默认根：

```text
Claude Code: <managed_home>/.claude/projects/**/*.jsonl
Codex CLI:   <managed_home>/.codex/sessions/**/*.jsonl
```

自定义 `CLAUDE_CONFIG_DIR`、`CODEX_HOME` 和 Codex `archived_sessions` 不通过读取
运行进程环境推断；只有管理员将 canonical root 明确加入策略后才扫描。V6.3 默认
不读取 `archived_sessions`，避免把历史归档自动纳入首版范围。

默认发现策略：

- Agent 启动后执行一次初始扫描；
- `scan_interval_seconds=30`，加入 0～20% jitter；
- 首次仅发现最近 14 天修改的文件；这沿用 ADR Claude parser 默认窗口，并为 ADR
  Codex parser 当前无年龄限制的行为增加生产边界；
- 单轮最多 2,000 个候选文件、2 秒目录枚举时间和 64 MiB 新增读取量；
- 超限后保存 continuation cursor，下一轮继续，不从头反复扫描；
- 配置更新和管理员手工操作可触发额外扫描，但同一主机只允许一个 scan lease；
- 不使用 inotify/fanotify 作为完整性或低延迟通道。

每个候选文件必须通过：

- `lstat` 后以 `openat`/`O_NOFOLLOW` 打开，拒绝 symlink race；
- 是普通文件，不是 socket、device、FIFO 或目录；
- canonical path 仍在允许 root 内；
- owner UID 与纳管 UID 一致；
- 文件大小、mtime、深度和单轮预算未超限；
- 打开后再次 `fstat`，dev/inode/owner 与打开前一致；
- 全程只读，不锁文件、不 chmod、不 truncate、不删除来源文件。

## 5. 通用解析流程

```text
complete JSONL record
  -> source schema fingerprint/version gate
  -> field allowlist
  -> visible/metadata/hidden classification
  -> normalize Session/Item/Tool
  -> secret redaction
  -> path/project pseudonymization
  -> tool-specific truncation
  -> source/item digest
  -> append encrypted spool
```

Agent 不计算页面 Token；DC 对收到的脱敏统一 item 使用权威
`aegis_visible_v1` 算法计算，避免 Agent 版本产生不同结果。

未知 record type 只增加未知类型计数和 source digest，不上传正文。无法识别关键
session/message 结构时，该文件标记 `unsupported`，不得根据相似字段名猜测。

## 6. Claude Code 静态 Parser

参考 ADR `ClaudeParser`：

```text
<managed_home>/.claude/projects/**/*.jsonl
```

在经过 fixture 验证的 schema 中允许解析：

- `sessionId`、message UUID/parent UUID、timestamp、cwd、model；
- `type=user|assistant` 的用户可见 message content；
- assistant content block 中的 `text`、`tool_use`；
- user content block 中的 `tool_result`；
- compact boundary 和用户可见 summary；
- source usage 数字字段。

明确禁止：

- thinking、redacted thinking、隐藏 analysis 或 chain-of-thought；
- `tool-results/` spill、`file-history/`、`paste-cache/`、`image-cache/`；
- 认证文件、配置正文或项目中的任意文件；
- `history.jsonl` 作为完整会话来源。

如果用户设置无会话持久化、文件已被清理或目录不存在，Aegis 只能报告
`source_not_persisted`/`source_not_found`，不能从 Hook 补采，也不能将空结果解释为
安全会话。

## 7. Codex CLI 静态 Parser

参考 ADR `CodexParser`：

```text
<managed_home>/.codex/sessions/**/*.jsonl
```

在经过 fixture 验证的 schema 中允许解析：

- `session_meta`：source session ID、timestamp、cwd；
- `turn_context`：model 和 turn metadata；
- `response_item.message`：用户可见 `input_text`/`output_text`；
- `response_item.function_call`：call ID、name、允许字段 arguments；
- `response_item.function_call_output`：call ID、status 和脱敏 result summary；
- source/turn usage 数字字段。

明确丢弃：

- reasoning、reasoning summary、encrypted reasoning 和隐藏状态；
- auth、provider credential、完整环境和未 allowlist 内部记录；
- `history.jsonl` 作为 assistant/tool 完整会话来源。

Codex schema 无稳定兼容承诺，因此 parser 以合成 fixture、source record type 集合和
schema fingerprint 为门禁；未知格式 fail closed。

## 8. 会话和生命周期推断

静态文件没有可靠的 Hook start/end 信号。页面状态必须标记为推断值：

```text
active_inferred: 文件最近 90 秒有追加记录
idle_inferred:   90 秒至 30 分钟无追加
ended_observed:  已验证 schema 中存在明确结束记录
ended_inferred:  超过 30 分钟无追加
unknown:         时间戳缺失、文件不可读或 schema 不支持
```

一次后续扫描发现文件继续追加时，`ended_inferred` 可回到 `active_inferred`；这不是
新会话。resume/compact/subagent 只在落盘记录包含可验证 ID/类型时表达，否则保持
unknown，不构造生命周期事件。

## 9. 顺序、幂等和文件 Cursor

本地 cursor identity：

```text
source_identity = owner_uid + agent_type + root_hash + dev + inode
cursor = byte_offset + last_complete_line_digest + last_file_size + last_mtime
```

处理规则：

- 半行保留在受限内存 buffer，下一轮追加后再解析；Agent 重启可从行首安全重读；
- 首次扫描从文件开头读取，但受 lookback、文件和字节预算限制；
- append 时从上次完整行后的 offset 继续；
- inode 变化视为新文件 identity；旧 inode 不再可达时以已 ACK 数据结束；
- 文件 size 小于 offset 视为 truncate，从 0 重读并依赖 source ID/revision/digest 去重；
- 同一 message revision 使用 upsert，不重复创建 item；
- 无稳定 source ID 时使用 session、类型、source timestamp 和 normalized digest，
  `identity_confidence=derived`；
- 来源文件删除只更新 coverage，不删除 Aegis 已保存副本；
- cursor 只有在 item 写入 spool 后才前移，避免崩溃造成静默缺口。

中心侧收到的 `source_sequence` 是 Aegis 针对归一化 session 生成的稳定序号，不是
文件行号；同一 session 的多文件记录按 source timestamp、source ID 和 deterministic
tie-breaker 排序。

## 10. Redaction

顺序不可交换：

```text
allowlist
  -> drop hidden/internal
  -> secret detector
  -> path/user/project pseudonymization
  -> tool field policy
  -> byte limit
  -> digest
```

首批 secret 类别：

- Authorization/Bearer/Cookie；
- OpenAI/Anthropic/GitHub/云 provider token；
- PEM/private key；
- password/passwd/secret/api_key 环境变量和 CLI 参数；
- database URL、云 access/secret key、SSH material；
- 高熵疑似凭据。

替换值只保留类别：

```text
[REDACTED:OPENAI_API_KEY]
[REDACTED:PRIVATE_KEY]
[REDACTED:AUTHORIZATION]
```

redaction 失败、文本解码异常或单 item 超限时，正文 `suppressed`，仅上报 digest、
长度、类型和 safe error code。

## 11. 本地 Spool 和上报

- spool 使用 `/var/lib/aegis/agent-session-spool/` 的独立 root-owned `0700` 目录；
- record 使用 Agent 本地密钥加密，单段有 checksum；
- quota 默认 512 MiB，可配置；
- batch 限制为 100 items/1 MiB；
- Server ACK 到 `accepted_through_sequence` 后才删除对应 record；
- spool 满时停止推进来源 cursor，记录 coverage gap/pressure，不删除来源文件；
- Server/Kafka 不可用时指数退避，静态扫描可继续到 spool 配额边界。

## 12. 配置 Bundle

```json
{
  "schema": "aegis.agent_session.collection_bundle.v1",
  "version": 1,
  "targets": {
    "host_ids": [],
    "subjects": [
      {"uid": 1000, "home": "/home/redacted-by-dispatch"}
    ],
    "agent_types": ["claude-code", "codex"]
  },
  "collection": {
    "enabled": true,
    "mode": "static_scan",
    "content_mode": "redacted_text",
    "scan_interval_seconds": 30,
    "initial_lookback_days": 14,
    "max_files_per_scan": 2000,
    "max_scan_duration_ms": 2000,
    "max_new_bytes_per_scan": 67108864,
    "max_item_bytes": 262144,
    "max_session_bytes": 52428800
  },
  "sources": {
    "claude": {"enabled": true, "extra_roots": []},
    "codex": {"enabled": true, "extra_roots": []}
  }
}
```

服务端保存/审计纳管 UID，但不得把明文 home、路径或用户名写入普通日志。下发给该
主机的 home 只用于本地 canonical path 校验，Agent 上报时使用 root hash。

## 13. 日志和指标

稳定日志事件：

```text
agent_session_scanner_started                 INFO
agent_session_scan_completed                  DEBUG/INFO（汇总、限频）
agent_session_scan_budget_exhausted           WARN（限频）
agent_session_source_file_rejected            WARN（按原因聚合）
agent_session_parser_unsupported              WARN（按 source/version 限频）
agent_session_redaction_suppressed             WARN（无正文）
agent_session_cursor_recovered                 INFO
agent_session_source_truncated                 WARN
agent_session_spool_pressure                   WARN
agent_session_batch_deferred                   WARN（退避限频）
agent_session_batch_acked                      DEBUG
agent_session_scanner_stopped                 INFO
```

允许字段：host ID、agent type、source UID、session audit ID、source version、
schema fingerprint、文件数、sequence range、item/byte count、digest、error code、
retry 和 latency。禁止字段：source session 明文、prompt、assistant text、tool
args/result、明文路径、用户名、secret 和原始 JSON。

指标：

```text
agent_session_scan_duration_seconds{agent_type}
agent_session_files_discovered{agent_type}
agent_session_file_cursor_lag_bytes{agent_type}
agent_session_items_collected_total{agent_type,item_type}
agent_session_items_suppressed_total{reason}
agent_session_parser_unsupported_total{agent_type,version}
agent_session_scan_budget_exhausted_total{reason}
agent_session_sequence_gap_total{agent_type}
agent_session_spool_bytes
agent_session_batch_retry_total{reason}
```

## 14. Agent 采集测试

- ADR 风格 Claude/Codex 合成 JSONL 可被静态扫描和归一化；
- scanner 没有 Hook 配置写入、socket 创建或进程注入副作用；
- 首次扫描、第二次无变化扫描、文件追加和多文件 session 不重复；
- 半行、invalid JSON、truncate、inode 替换、删除、权限变化和 symlink race；
- 14 天 lookback、文件数/字节/时间预算和 continuation cursor；
- active/idle/ended 推断可逆且 UI 明确显示 inferred；
- unknown schema fail closed，隐藏 reasoning 和 secret fixture 不离开主机；
- Agent 重启、spool 满、Server 断网和 ACK 重放无静默丢失；
- 扫描前后来源 JSONL 的 inode、size、mtime 和内容不被 Aegis 修改。

# V6.3 Uber ADR 参考分析与设计决策

调研基线：Uber ADR `main` commit
`89ebfe647ccef3088ef5ac3b885e69fccc95bb85`（2026-08-08）。

## 1. 结论

Uber ADR 开源 Sensor 的 Claude Code/Codex 会话采集是静态本地日志解析，不是
Hook 采集：

```text
adr-sensor CLI / AgentObserver.ingest_all()
  -> source_parser.parse_all()
  -> glob 本地 JSONL/SQLite/JSON 文件
  -> 读取完整文件并归一化 AgentEvent
  -> 控制台展示或 JSON/JSONL 导出
```

Claude parser 扫描 `~/.claude/projects/**/*.jsonl`，Codex parser 扫描
`~/.codex/sessions/**/*.jsonl`。开源 Sensor 中没有用 Hook、inotify 或 watcher
获取这两个来源的会话正文。

V6.3 因此以“静态文件扫描”为唯一正文采集方式。周期调度、byte cursor 和 spool
是 Aegis 为生产运行增加的机制，不改变数据来源，也不依赖 Claude/Codex 进程回调。

## 2. ADR 调用链证据

### 2.1 CLI 和 Observer

ADR `Sensor/adr_sensor/cli.py` 创建 `AgentObserver`，调用：

```python
entries, system_config_data = observer.ingest_all(args.source)
```

`AgentObserver.ingest_all()` 再为选中的来源执行：

```python
entries = getattr(self, f"{source}_parser").parse_all()
```

调用完成后进程展示并导出结果，没有常驻循环或 Hook ingress。`--save-sessions`
所称 incremental 是按已导出的 session 文件过滤，不是对源 JSONL 做实时监听。

### 2.2 Claude Code

ADR `ClaudeParser`：

```python
self.base_path = Path.home() / ".claude/projects"
jsonl_files = list(self.base_path.glob("**/*.jsonl"))
```

默认按文件 mtime 跳过 14 天以前的日志，随后逐个打开 JSONL、逐行解析，并组合
session、user/assistant message 和 tool use/result。

### 2.3 OpenAI Codex CLI

ADR `CodexParser`：

```python
self.base_path = Path.home() / ".codex/sessions"
jsonl_files = list(self.base_path.glob("**/*.jsonl"))
```

它逐个完整读取 JSONL，通过 `session_meta`、`turn_context` 和 `response_item` 等
记录恢复 session、message、function call 和 output。与 Claude parser 不同，当前
Codex parser 没有 `max_age_days` 过滤，会遍历发现到的全部 session 文件。

## 3. 可复用思想

### 3.1 来源适配器

| Agent | ADR 发现路径 | 主要识别内容 |
| --- | --- | --- |
| Claude Code | `~/.claude/projects/**/*.jsonl` | user/assistant、`tool_use`、`tool_result`、session/model/cwd |
| Codex CLI | `~/.codex/sessions/**/*.jsonl` | `session_meta`、`turn_context`、`response_item` message/function call/output |

V6.3 采用相同 Adapter 思路：来源差异终止在 Agent parser，后续只处理统一
`AgentConversationSession` 和 `ConversationItem`。

### 3.2 统一会话模型

ADR 将不同来源归一为 `AgentEvent -> ChatMessage -> ToolUsage`。V6.3 保留公共字段，
但将完整 chat history 拆成可增量、可定位和可分页的 item 流，以支持：

- 单 item 幂等和增量回补；
- 风险命中字符区间；
- 长会话 Token 分段；
- 工具调用与 OS 行为关联；
- partial、redacted、truncated、unsupported 和 missing range 表达。

### 3.3 两级检测

ADR Detection 采用高召回 triage，再对可疑会话做高精度 reasoning。V6.3 借鉴
“便宜初筛、昂贵深析”的资源分层，但产品化为：

```text
确定性规则分析（全部新 item，零模型调用）
  -> 触发/策略决定是否进入 AI
  -> Token 有界 chunk AI 分析
  -> chunk 结构化结果的 session reduce
```

规则不是 LLM triage，因此模型不可用时提示注入/越狱检测仍可运行。

## 4. 不直接复制的 ADR 行为

| ADR 行为/限制 | Aegis 风险 | V6.3 调整 |
| --- | --- | --- |
| CLI 单次扫描并完整读取文件 | 大目录开销、重复解析、活跃文件增长 | Agent 周期静态扫描 + dev/inode/offset cursor |
| Codex parser 不限制文件年龄 | 首次扫描历史目录可能耗时/占用过大 | 两个来源统一使用可配置 initial lookback，默认 14 天 |
| `glob("**/*.jsonl")` | 越界目录、文件数量和扫描耗时不可控 | 纳管 UID 固定 root、深度/mtime/文件数/时间预算 |
| `--save-sessions` 按输出文件跳过 session | 活跃 session 后续追加可能被跳过 | item revision/digest 幂等，持续读取新完整行 |
| triage 接收完整 conversation | 上下文溢出和成本不可控 | 规则先行，AI 按动态 Token 预算分段 |
| Codex reasoning summary 可进入 assistant 内容 | 可能采集内部推理 | 丢弃 reasoning/summary/encrypted reasoning |
| 工具字段按长度截断 | 截断前可能含 secret | allowlist -> redaction -> tool policy -> 截断 |
| hostname、username、project path 直接进入事件 | 个人和项目隐私暴露 | host ID、UID、project/root hash 和脱敏路径 |
| 单个 AgentEvent 保存完整 chat history | 无法高效分页和增量 | Session/Item/Tool 三层持久化 |
| debug/error 可能包含正文 | 会话内容泄漏 | 只记录 ID/hash/count/status/safe error |

ADR Detection 的开源说明以 benchmark/research 为主要定位，因此检测代码也不直接
作为 Aegis 生产 worker 嵌入。

## 5. V6.3 关键架构决策

### D1：只使用静态会话文件

- **选择**：正文只来自纳管 UID 的 Claude/Codex 落盘 JSONL。
- **排除**：不安装 `aegis-session-hook`，不创建 session ingress socket，不将 Hook
  作为 identity、lifecycle 或正文补采来源。
- **影响**：采集时效由扫描周期决定；status/end/resume 只能从文件记录和 mtime
  推断，并必须标记 `inferred`。

### D2：不嵌入 ADR Python Sensor

- **选择**：在 Aegis Agent 内用 Go 实现 ADR 风格的 discovery/parser。
- **原因**：复用现有 Agent 身份、ConfigSync、gRPC、spool、日志和离线发布链路，
  不增加 Python runtime 或第二个守护进程。
- **约束**：parser 行为必须用从公开格式构造的合成 fixtures 固化，不读取开发者
  真实会话作为测试数据。

### D3：周期扫描而非文件监听

- **选择**：Agent 启动、配置更新、手工触发和默认每 30 秒执行有界静态扫描。
- **原因**：与 ADR 数据来源一致，行为简单可验证，也避免 inotify overflow 被误当
  完整性保证。
- **代价**：页面不是事件级实时；目标为新增完整记录 p95 90 秒内可见。

### D4：规则分析与 AI 分析分离

- **选择**：两个 run、两个结果页签、独立版本和状态。
- **原因**：规则命中是可重现事实，AI 是概率判断；合并会掩盖分歧和失败。

### D5：专用正文数据面

- **选择**：新增 gRPC batch、Kafka topic 和会话专用表。
- **原因**：现有 `aegis.security.events` 和 `RuntimeEvent` 不适合承载敏感大文本。

### D6：默认只保存脱敏文本

- **选择**：V6.3 不实现原始会话上传、reveal 或 export。
- **原因**：先交付可观测和检测闭环，避免 KMS、审批、legal hold 和原文泄漏扩大
  首版风险。

### D7：Token 指标分层

- **选择**：分开保存可见内容估算、来源 usage、Aegis AI usage。
- **原因**：来源 input token 可能包含重复上下文/cache，不能代表当前会话正文大小；
  分析调用 Token 又属于 Aegis 成本。

### D8：无自动阻断

- **选择**：会话规则和 AI 的动作上限是标记、告警、人工复核。
- **原因**：静态文字是可被用户控制的语义材料，不是不可抵赖的执行证据。

## 6. 实施时必须重新核对的上游

- [ADR Sensor README](https://github.com/uber/ADR/blob/main/Sensor/README.md)
- [ADR Claude parser](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/parsers/claude_parser.py)
- [ADR Codex parser](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/parsers/codex_parser.py)
- [ADR Observer](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/observer.py)
- [ADR CLI](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/cli.py)
- [ADR unified schema](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/schemas/agent_event_schema.py)

上游变化只能影响 Adapter，不能改变静态只读采集、目录边界、脱敏、Token 指标和
分析安全边界。

# V6.2 智能体配置安全检测设计

**版本**：6.2
**状态**：新增方案，按最小闭环实施
**范围**：Codex、Claude Code、OpenClaw、OpenCode、Hermes 的本地配置采集、Hook 点提取、脱敏展示和权限基线检测

## 1. 问题与成功标准

Agent Guard 当前已能展示智能体运行行为和隔离逃逸，但无法回答“智能体当前配置允许做什么”。本方案新增“智能体配置检测”标签页，以主机为范围读取受支持智能体的已知配置文件，提取脱敏内容，并把配置字段映射为可审计的安全检查结果。

成功标准：

1. 用户能从 Agent Guard 下的“智能体配置检测”页面选择在线主机并触发扫描。
2. 页面能展示 Codex、Claude Code、OpenClaw、OpenCode、Hermes 的配置文件路径、格式、读取状态、脱敏原文、字段风险和独立 Hook 点清单。
3. 对 `danger-full-access`、`approval_policy=never`、全量 shell/edit/tool 放行、关闭沙箱、通配符 allow 等高风险配置给出高危结果；对凭据字段只显示掩码，不在日志、响应或持久化数据中泄露原值。
4. 文件不存在、不可读、格式不支持或解析失败都显示明确状态，不把“未采集”误报为“安全”。
5. 采集失败不会影响智能体正常运行，扫描接口只读且有超时、文件大小和路径白名单限制。

## 2. 目标数据流

```text
前端配置检测页
  -> GET /agent-guard/agents（选择主机）
  -> GET /agent-guard/configurations?host_id=...
  -> api-server ExecuteTool("AgentConfigScan")
  -> server 转发到 Agent ExecuteTool
  -> Agent 固定路径读取并脱敏
  -> api-server 解析、字段展平、配置安全规则评估
  -> 返回脱敏原文 + 字段结果 + 文件状态
```

本期不新增 protobuf RPC、不新增数据库表、不把原文写入 Kafka/MinIO。配置快照是按用户请求生成的短生命周期响应；如后续需要趋势、整改闭环，再单独设计加密存储和保留策略。

## 3. 支持范围和读取白名单

只读取 Linux Agent 当前用户可见的固定文件；不接受前端传入任意路径。

| 智能体 | 配置文件 | 格式 |
| --- | --- | --- |
| Codex | `$CODEX_HOME/config.toml`、默认 `~/.codex/config.toml`、`~/.codex/hooks.json` | TOML/JSON |
| Claude Code | `~/.claude/settings.json`、`~/.claude/settings.local.json`、`~/.claude.json` | JSON |
| OpenClaw | `~/.openclaw/openclaw.json`、`~/.openclaw/config.json` | JSON |
| OpenCode | `~/.config/opencode/opencode.json`、`~/.config/opencode/opencode.jsonc` | JSON/JSONC |
| Hermes | `~/.hermes/config.yaml`、`~/.hermes/config.yml`、`~/.hermes/config.json` | YAML/JSON |

`CODEX_HOME`、`HOME` 只由 Agent 本地环境解析；API-server 不拼接或执行用户提供的路径。单文件最大 256 KiB，单次最多返回 32 个文件，单主机扫描超时 30 秒。

## 4. 返回模型和安全边界

响应核心字段如下：

- `host_id`、`hostname`、`scanned_at`：扫描上下文。
- `agents[]`：智能体类型、显示名、文件列表、总体风险计数。
- `files[]`：绝对路径、格式、状态、大小、修改时间、脱敏后的 `content`、解析错误和 `findings[]`。
- `hooks[]`：智能体、配置文件、配置字段路径、Hook 事件、脱敏命令/脚本、执行方式、可执行文件路径和 `findings[]`。
- `findings[]`：稳定规则 ID、字段路径、脱敏值摘要、严重级别、风险原因、修复建议。

脱敏规则在 Agent 读取端执行，字段名匹配 `token`、`secret`、`password`、`api_key`、`access_key`、`private_key`、`cookie` 等关键字时只保留字段名和 `***`。API-server 和前端不记录原文，日志只记录 host、agent、文件数、finding 数和耗时。

## 5. 首批安全规则

规则是 API-server 内置的只读目录，规则命中不执行自动修复。

| 规则 ID | 检查内容 | 默认级别 |
| --- | --- | --- |
| `AGC-001` | `approval_policy=never`、`defaultMode=bypass`、`dangerously-skip-permissions`、`skip_approval/auto_approve=true` | high |
| `AGC-002` | `sandbox_mode=danger-full-access`、sandbox disabled/off 或等价无限制模式 | critical |
| `AGC-003` | shell/bash/exec/edit/write/tool 的全量 `allow`、`*` allow 或 legacy `tools.*=true` | high |
| `AGC-004` | OpenCode `permissions` 规则对任意 action/resource 使用 `effect=allow` | high |
| `AGC-005` | `network_access=true`、网络模式 unrestricted/off 或允许任意外部目录 | medium/high |
| `AGC-006` | 配置中的凭据、Token、私钥等敏感值 | high |
| `AGC-007` | 配置解析失败、文件不可读或发现了未知权限模式 | medium |
| `AGC-008` | Hook 使用 shell/解释器执行、命令路径相对或位于用户可写目录、Hook 事件使用 `*` 通配且无明确限制 | high |
| `AGC-009` | 高风险 Hook 点（工具执行前/后、会话启动/结束）未设置显式失败策略、审批或来源校验 | medium |

规则评估保留原始字段路径和规则版本，通配符只在对应字段语义中判断，不通过模糊全文搜索把普通描述文本误报为权限。

## 6. 接口与权限

新增只读接口：

```http
GET /api/v1/agent-guard/configurations?host_id=<uuid>
```

沿用 `agent_guard:read` 权限。`host_id` 必填且只能扫描已连接 Agent；无 Agent、超时和工具失败分别返回可定位的 404/504/502 语义。页面保持旧的事件和逃逸路由不变，新增 `/detection/agent-guard/configurations`，Agent Guard 默认仍跳转事件页。

## 7. 测试设计

### Agent

- 只读取白名单文件，不读取任意 `path` 参数。
- JSON/TOML/YAML/JSONC 文件成功读取；超大文件、目录、软链接越界、不可读文件返回状态而不是原文。
- `token`、`api_key`、`private_key` 等字段在原文响应中为 `***`。

### API-server

- Codex、Claude Code、OpenCode 样例分别命中 `AGC-001`～`AGC-005`。
- Codex/Claude Code/OpenClaw/Hermes 配置中的 Hook 节点能提取为事件、命令、来源路径；OpenCode 插件/Hook 配置也能按同一模型展示。
- 绝对路径的只读 Hook 脚本不因“存在 Hook”误报；shell `-c`、相对脚本、用户可写目录和全量事件通配分别命中对应 Hook 规则。
- 安全的 `ask/deny`、`read-only`、`workspace-write` 配置不误报高危权限。
- 解析失败、未知权限值和凭据字段分别产生可解释结果。
- ExecuteTool 失败、空响应、非法 JSON 和超时均返回稳定错误，不记录配置正文。

### Frontend

- 导航和路由同时包含新标签页，中英文 key 完整。
- 选择主机、扫描 loading、空结果、错误、文件切换和风险筛选可用。
- 页面渲染 `***` 脱敏值，不自行展示或拼接原始秘密。

## 8. 兼容性、风险与回滚

本方案不修改现有 proto、数据库和 Agent Guard 行为事件链路；旧 Agent 未实现 `AgentConfigScan` 时仅该页面提示“Agent 版本不支持”，其他功能不受影响。回滚时删除新页面/API/工具分支和本设计文件即可，不需要数据迁移或数据清理。

# 多智能体原生 Hook 适配修复

## 目标

让 Codex、Claude Code、OpenClaw、Hermes 和 Zcode 使用同一条可信会话/工具链路：

1. 使用智能体原生 Hook 提供的真实会话 ID，不用进程 ID 或本地扫描结果伪造会话。
2. 会话开始、工具调用开始/结束、会话结束都进入同一个 Ed25519 签名的本地 Unix socket。
3. Hook 给出的 PID 只作为控制器根证据；实际执行命令的 PID 由 Agent 的 eBPF 进程事件关联，避免所有行为都显示为 Hook helper 的 PID。
4. 工具输入和结果只作为签名工具语义用于规则匹配；没有可信会话或无法关联实际进程时，不产生会话行为事件。

## 产品适配

| 产品 | 官方入口 | 适配方式 | 会话结束语义 |
| --- | --- | --- | --- |
| Codex | managed native hooks | `SessionStart`、`PreToolUse`、`PostToolUse`、`PostToolUseFailure`、`SessionEnd` | 原生 `SessionEnd` |
| Claude Code | command hooks | `SessionStart`、`PreToolUse`、`PostToolUse`、`PostToolUseFailure`、`SessionEnd` | 原生 `SessionEnd` |
| OpenClaw | typed plugin hooks | `session_start`、`before_tool_call`、`after_tool_call`、`session_end` | 原生 `session_end` |
| Hermes | shell hooks | `on_session_start`、`pre_tool_call`、`post_tool_call`、`on_session_end` | 原生 `on_session_end` |
| Zcode | Claude-compatible command hooks | `SessionStart`、`PreToolUse`、`PostToolUse`、`PostToolUseFailure` | 不把 `Stop` 当作结束；等待原生会话结束或根进程退出 |

Hook helper 接受五种产品的原始 JSON，并规范化为现有 `TrustedSessionEvent` / `TrustedToolEvent`。适配器不依赖命令行文本猜测工具调用；工具匹配命令来自 Hook 输入，实际 PID/PPID/cmdline 只由 eBPF/`/proc` 关联补充。

## 安全约束

- 多个产品 source 共同写入一个 manifest 时必须保留既有 source，并重新计算 manifest digest。
- 每个 source 使用固定 source ID、产品类型、Ed25519 公钥和受信 helper artifact digest。
- OpenClaw plugin 只负责把原生 typed hook 转换为 JSON 并调用本地 helper，不能持有私钥，也不直接写数据库。
- Hermes shell hook 的执行失败必须是观察失败，不能改变智能体工具执行结果。
- Zcode `Stop` 是模型停止点，不是会话生命周期结束点，不能误发 `session_ended`。

## 验收标准

- 四个产品都能通过 helper 单元测试生成签名事件，且输入字段不会因额外官方字段而被拒绝。
- 每个产品具有独立 source manifest 条目和 profile；zcode 也能在资产、进程和前端类型筛选中出现。
- Claude/Zcode/Hermes 配置合并保持原始用户配置，重复执行幂等。
- OpenClaw plugin 覆盖 session/tool 四个 typed hook，helper 不可用时只记录稳定错误码，不阻塞工具。
- Agent、API、Frontend 的定向测试和构建通过。

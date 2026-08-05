# Codex 工具调用采集与命令关联

## 目标

行为全景和安全分析以真实 Codex `session_id` 为范围，只展示该会话期间的工具调用。页面展示工具名称、Hook 输入、工具结果、规则命中以及后台关联的 PID/PPID 和命令行，不展示进程树。

## 数据流

```text
Codex SessionStart/End
  └─ PreToolUse/PostToolUse
       └─ aegis-codex-hook（签名：session_id、tool_name、tool_use_id、tool_input、tool_response）
            └─ Agent Hook socket
                 ├─ 建立工具调用关联窗口
                 └─ eBPF process_fork/process_exec 关联实际 PID/命令行
                      └─ agent_behavior(category=tool)
                           └─ DC 按工具命令内容执行规则匹配
```

## PID 语义

Codex Hook 官方输入包含会话和工具字段，不提供工具工作进程 PID。Hook 只能可靠取得其父进程 PID 作为会话锚点；Agent 不把该锚点误当作工具 PID，而是通过工具调用活动窗口内的 eBPF fork/exec 事件关联实际执行进程。优先按工具输入命令和 eBPF `/proc` 命令行匹配，无法精确匹配时使用同一调用窗口内的后代进程，并在事件属性记录关联方法。

## 安全边界

- 工具输入和结果与工具调用身份一起签名，DC 不接受前端拼接的工具内容。
- 工具调用内容可以命中规则并生成安全分析 Finding，但仅凭 Hook 声明不触发阻断；处置仍要求独立的操作系统证据。
- 输入、结果和命令行沿用 Agent/DC 的敏感字段脱敏和长度限制。
- `tool_use_id` 按 Codex 的不透明调用 ID 校验，不强制要求 UUID；内部事件和 eBPF 证据 ID 仍使用 UUID。

## 验收标准

1. 一个真实 Codex 会话只显示该会话的 `tool_call` 事件，并支持分页。
2. 每个工具调用显示工具名、输入/命令、结果、调用 ID、命中规则和关联状态。
3. `curl`、`sudo`、`chmod` 等敏感命令出现在工具输入中时，安全分析命中“敏感命令执行”。
4. 页面不请求或渲染行为全景的进程树；eBPF 关联信息只作为工具调用字段返回。

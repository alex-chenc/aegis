# Codex MCP 资产采集设计（v6.3）

## 1. 背景与目标

Aegis 原有 `MCPCollector` 支持 JSON 格式的 Claude、Cursor、Windsurf、VS Code
以及项目级 `.mcp.json`，但 Codex 将 MCP 配置写入 TOML：

```text
$CODEX_HOME/config.toml
  └── [mcp_servers.<name>]
```

本变更让 Agent 在主机资产采集阶段识别 Codex MCP，并输出已有的
`AIAsset{category: "mcp_server"}`，不新增服务端 API 或数据库结构。

## 2. 采集流程

1. 复用 Agent 的用户 Home 发现逻辑。
2. 当前用户在 `CODEX_HOME` 为绝对路径时读取该目录，否则读取对应用户的
   `~/.codex/config.toml`。
3. 使用 TOML 解析器读取 `mcp_servers`，每个条目生成一个 `mcp_server` 资产。
4. 资产沿用现有采集、上传、持久化链路，额外标记 `agent=codex`、`format=toml`
   和配置文件路径。

Codex URL MCP 在未显式声明 transport 时标记为 `streamable_http`；本地 command
MCP 标记为 `stdio`。采集只发现配置，不启动 MCP 进程、不访问远程 URL。

## 3. 凭证与隐私边界

- `env` 只保留环境变量名，绝不保留环境变量值。
- `bearer_token_env_var` 只保留变量名。
- command、参数和 URL 经过现有 `RedactCmdline` 脱敏，覆盖 token、password、
  secret、URL 用户密码等常见凭证形式。
- 日志只记录 MCP 名称、Agent、transport 和配置路径，不记录配置内容。

## 4. 验收与回滚

验收包括 Codex TOML 解析、stdio/streamable HTTP transport 判断、环境变量名采集、
命令参数脱敏、URL 凭证脱敏和 disabled 状态采集测试，并执行 Agent 定向测试及构建。

如需回滚，移除 Codex TOML 分支和对应测试即可；已有 JSON MCP 采集路径、资产接口
和其他 Agent 配置采集不受影响。

# Aegis 本地主机查询 MCP 设计

## 1. 范围与成功标准

在 Aegis 仓库中提供一个本地 stdio MCP Server，并注册到当前用户的 Codex。MCP 只读调用现有 api-server 主机接口，支持：

- `list_hosts`：按 IP 或主机名查询主机列表。
- `get_host`：按主机 UUID 查询单台主机及在线状态。

成功标准：

1. MCP Server 可以通过 JSON-RPC stdio 完成 `initialize`、`tools/list` 和 `tools/call`。
2. 查询请求携带 Aegis Bearer Token，不在日志、代码或 Codex 配置中写入 Token 明文。
3. Aegis API 不可用、未认证、参数非法时返回可读的 MCP tool error，不导致进程崩溃。
4. 当前用户的 Codex 配置包含一个指向本地 MCP Server 的 stdio 条目。

## 2. 数据流与接口

```text
Codex
  -> stdio JSON-RPC
  -> tools/aegis-mcp/aegis_mcp.py
  -> GET /api/v1/hosts?query=...
  -> GET /api/v1/hosts/{host_id}
  -> Aegis api-server:8082
```

MCP Server 默认访问 `http://127.0.0.1:8082`，可由 `AEGIS_API_URL` 覆盖。认证从 `AEGIS_API_TOKEN` 读取；也支持通过 `AEGIS_API_TOKEN_FILE` 指向权限为 0600 的本地 Token 文件。

## 3. 安全与兼容性

- 只实现 GET 查询，不提供写操作、命令执行、任意 URL 访问或数据库访问。
- `get_host` 对 UUID 做格式校验，避免把用户输入拼接成任意路径。
- Token 只放在 HTTP Authorization Header，不出现在 MCP 响应和 stderr 日志中。
- 保持 Aegis 现有认证和主机响应结构，MCP 只做适配。
- 删除 Codex 的 MCP 条目即可回滚；仓库内删除 `tools/aegis-mcp/` 即可移除实现。

## 4. 测试设计

- 初始化和工具目录返回正确。
- `list_hosts` 透传查询参数和 Bearer Header，并返回 API 数据。
- `get_host` 透传合法 UUID。
- 非法 UUID 被 MCP 层拒绝，不发起 HTTP 请求。
- API 认证失败或网络失败返回 `isError=true` 的 tool result。

# Aegis 自身作为远程 MCP Server 设计

## 1. 范围与成功标准

`tools/aegis-mcp` 提供一个由 Aegis 自身实现的远程 Streamable HTTP MCP Server，
并通过 V6.3 MCP 聚合管控平台将它作为普通远程 Server 接入。stdio 仅保留为开发兼容
transport，不进入聚合平台准入路径。

暴露的只读工具：

- `get_aegis_health`：查询 Aegis API 健康状态，不需要 Session Token；
- `list_hosts`：按 IP 或主机名查询主机列表；
- `get_host`：按主机 UUID 查询单台主机及在线状态。

成功标准：

1. `POST /mcp` 可以完成 `initialize`、`tools/list` 和 `tools/call`；
2. Aegis 聚合平台可以通过 `http://aegis-mcp:8085/mcp` 完成远程发现；
3. 发现出的工具进入 Server Revision，并等待平台审批后才能发布；
4. 主机查询请求继续使用 Aegis Bearer Token，不在日志、响应或配置模板中写入 Token 明文；
5. API 不可用、未认证、参数非法时返回可读的 MCP tool error，不导致进程崩溃。

## 2. 数据流与接口

```text
MCP Client / Aegis onboarding
  -> POST /mcp
  -> tools/aegis-mcp (aegis-mcp:8085)
  -> Aegis api-server:8082
```

Compose 服务名 `aegis-mcp` 只在 `dev` 环境被 Aegis 控制面显式允许；其他私网地址仍被
SSRF 防护拒绝，生产环境必须使用 HTTPS 的外部远程 MCP Server。

环境变量：

- `AEGIS_API_URL`：默认 `http://127.0.0.1:8082`，Compose 中为 `http://api-server:8082`；
- `AEGIS_API_TOKEN` 或 `AEGIS_API_TOKEN_FILE`：主机查询所需的 Aegis 会话凭据；
- `AEGIS_MCP_ACCESS_TOKEN`：可选，保护 MCP HTTP 入口的 Bearer Token；
- `AEGIS_MCP_HOST`、`AEGIS_MCP_PORT`：HTTP 监听地址和端口。

## 3. 安全与治理

- Server 只实现只读工具，不提供写操作、命令执行、任意 URL 访问或数据库访问；
- `get_host` 对 UUID 做格式校验，避免用户输入形成任意路径；
- Token 只放在 HTTP Authorization Header，不出现在 MCP 响应和 stderr 日志中；
- Aegis 聚合平台仍执行 initialize、tools/list、Schema 校验、安全扫描、审批、发布和审计；
- 发现完成不等于可调用，Server 必须经过审批并进入有效 Catalog Release；
- 删除或暂停平台中的 Server/Tool/Release 即可回滚，保留历史 Revision 和审计证据。

## 4. 测试设计

- HTTP MCP 初始化、工具目录和工具调用；
- stdio 兼容模式初始化和工具调用；
- `list_hosts` 透传查询参数和 Bearer Header；
- `get_host` 透传合法 UUID，非法 UUID 不发起 HTTP 请求；
- API 认证失败或网络失败返回 `isError=true`；
- Aegis onboarding 能发现 3 个工具并创建待审批的 Server Revision；
- `aegis-mcp` 容器健康检查和 Aegis API 健康调用通过。

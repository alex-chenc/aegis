# Aegis Remote MCP Server

这里是一个由 Aegis 自身提供的远程 MCP Server，默认使用 MCP Streamable HTTP：

```text
POST http://<host>:8085/mcp
GET  http://<host>:8085/health
```

它只暴露只读工具：

- `get_aegis_health`：查询 Aegis API 健康状态，不需要 Aegis Session Token；
- `list_hosts(query?)`：查询 Aegis 主机列表，需要 `AEGIS_API_TOKEN` 或 Token 文件；
- `get_host(host_id)`：查询主机详情，需要 `AEGIS_API_TOKEN` 或 Token 文件。

## 本地运行

```bash
AEGIS_API_URL=http://127.0.0.1:8082 \
python3 /code/aegis/tools/aegis-mcp/aegis_mcp.py \
  --transport http --host 0.0.0.0 --port 8085
```

旧的 stdio 开发方式仍然可用：

```bash
python3 /code/aegis/tools/aegis-mcp/aegis_mcp.py --transport stdio
```

如果希望保护 MCP 入口，可配置 `AEGIS_MCP_ACCESS_TOKEN`；服务端会要求
`Authorization: Bearer <token>`，不会在日志中打印 Token。

使用 Docker Compose 时，通过 `AEGIS_API_TOKEN_FILE` 指定宿主机上的 Aegis API
Token 文件路径。Compose 会以只读方式挂载该文件，容器启动时读取后降权运行，
不会将 Token 写入镜像。

## 接入 Aegis 聚合平台

开发环境使用以下远程地址：

```text
http://aegis-mcp:8085/mcp
```

在“系统配置 → MCP 聚合管控”中创建远程接入任务：

- 服务名称：`Aegis Local MCP`
- Endpoint：`http://aegis-mcp:8085/mcp`
- 认证：`none`（仅用于本地发现；调用主机工具时仍需配置 API Token）
- 环境：`dev`
- 发布策略：`manual`

平台会执行 `initialize`、`tools/list`、工具 Schema 校验、安全扫描和审批记录。
未经审批和发布的工具不会进入可用 Catalog。

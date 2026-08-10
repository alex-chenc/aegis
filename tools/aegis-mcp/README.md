# Aegis Host Query MCP

这是一个只读的本地 stdio MCP Server，供 Codex 查询 Aegis 已登记主机。

## 工具

- `list_hosts(query?)`：按主机名或 IP 子串查询主机。
- `get_host(host_id)`：按 UUID 查询主机详情和在线状态。

## 认证

MCP Server 调用 `api-server` 时需要 Aegis 登录会话 Token：

```bash
export AEGIS_API_URL=http://127.0.0.1:8082
export AEGIS_API_TOKEN='从 Aegis 登录接口获取的 session token'
```

也可以使用本地文件，推荐权限为 0600：

```bash
install -d -m 700 ~/.config/aegis
install -m 600 /path/to/token ~/.config/aegis/mcp-token
export AEGIS_API_TOKEN_FILE=/absolute/path/to/mcp-token
```

## 手动运行

```bash
python3 /code/aegis/tools/aegis-mcp/aegis_mcp.py
```

## Codex 注册

```bash
codex mcp add aegis-hosts \
  --env AEGIS_API_URL=http://127.0.0.1:8082 \
  --env AEGIS_API_TOKEN_FILE=/root/.config/aegis/mcp-token \
  -- python3 /code/aegis/tools/aegis-mcp/aegis_mcp.py
```

注册后可用 `codex mcp get aegis-hosts` 检查配置。Token 文件不存在或 Token 过期时，工具会返回可读错误，不会让 MCP 进程退出。

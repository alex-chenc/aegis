# MCP OPA Runtime Validation v6.3

日期：2026-09-16（Asia/Shanghai）

## 构建与 Compose 状态

- 业务最终 freeze（16:17）哈希已核对：OPA `dff765d5…123e6`、service `adc5d215…336d1`、Compose `e8e92375…7c49`；之后仅因启动 smoke 失败修复启动迁移兼容逻辑。
- 最终 api-server 构建命令：`docker compose build api-server`，退出码 0。最终镜像 `aegis-api-server:latest` 为 `sha256:64f5e945c820794a43baaae6a60c32f6ba4374cf0bcd6b7efe6066849547817c`，创建时间 `2026-09-16T16:55:04+08:00`。
- 最终运行状态：api-server、mcp-gateway、aegis-mcp、postgres、redis、minio、zookeeper、kafka、server、dc、frontend 均 Up/healthy；builder Up/running；db-migrate、minio-init Exited (0)。服务保持运行。
- 039 迁移已加入 Compose `db-migrate` 挂载和执行顺序，并在启动依赖时重复幂等执行成功。039 授权表、`mcp_invocations` 关联列均存在。

## Health、MCP 与拒绝门禁

- api-server `GET /health`：HTTP 200，`{"status":"ok"}`。
- mcp-gateway `GET /health`：HTTP 200，`{"status":"ok"}`；`GET /ready`：HTTP 200，`{"mode":"runtime_policy","status":"ready"}`。
- aegis-mcp `GET /health`：HTTP 200，`{"status":"ok","server":"aegis-local-mcp","version":"0.2.0"}`；frontend `GET /health`：HTTP 200，`ok`。
- aegis-mcp JSON-RPC `initialize`、`tools/list`：HTTP 200；协商协议 `2025-11-25`，返回 3 个只读工具。`tools/call get_aegis_health` 返回 HTTP 200、`isError=false`、健康状态 ok。
- aegis-mcp `tools/call list_hosts` 在空 token 文件下返回 HTTP 200、`isError=true`，提示未配置 API token；未知工具返回 HTTP 200、`isError=true`。
- mcp-gateway client endpoint 无 Bearer 返回 HTTP 401；dummy Bearer 的 initialize/list/call 返回 JSON-RPC `-32003 tool is not allowed for this client`。直接 runtime 错误 secret 返回 HTTP 401，正确 secret 但无 client fixture 返回 HTTP 403；无 snapshot catalog 返回 HTTP 503 `catalog snapshot unavailable`。API 未认证 hosts 请求返回 HTTP 401。

## OPA allow 边界与启动修复

- 真实 OPA allow/deny 上游链路未执行：数据库中没有 client、credential、grant、release-tool 或 policy fixture，Compose token 文件为空；因此没有虚报 allow。已验证 health/ready/initialize/list 及认证、client、snapshot 和未知工具拒绝门禁，拒绝均在上游前返回。
- 启动 smoke 发现既有 PostgreSQL 约束名与 GORM 预期不一致，api-server 曾因 `uni_auth_users_username`、`uni_system_configs_config_key` 不存在而退出。`api-server/internal/repository/db.go` 已将启动 AutoMigrate 收窄为没有正式 SQL 迁移的 `AIConfig`、`ImageModelConfig`、`Notification`，迁移管理表由正式 SQL 负责；未删表、删卷或手工改约束。修复后最终镜像健康。
- 当前实现按单 api-server 进程设计；多副本 activation CAS、签名 bundle/LKG、trusted user delegation、策略差异/回滚 API 和前端策略表单不在本轮范围。

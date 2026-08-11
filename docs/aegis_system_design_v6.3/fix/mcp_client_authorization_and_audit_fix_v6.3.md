# MCP Client 授权与调用审计修复设计（v6.3）

## 1. 修复目标

本次修复收敛 MCP 接入端、Client 授权和调用审计之间的职责边界：

- 新增接入端时只选择一个已发布 MCP 服务，不在创建流程选择工具；
- 创建完成后默认授权该服务当前全部已发布工具；
- 工具开启/关闭只在“Client 授权”中维护，并由运行时每次调用实时校验；
- 调用审计按“服务 -> 工具 -> Client”展示，不再展示 AI 状态；
- 审计中的“禁用”仅撤销该调用所属 Client 对该工具的权限，不影响同服务的其他 Client；
- Client 授权页只保留接入端授权表，删除重复的旧 Client 列表。

## 2. 根因

1. `GET /mcp-platform/invocations` 直接返回 `mcp_invocations`，没有关联
   `mcp_clients`、`mcp_tool_revisions`、`mcp_server_revisions` 和 `mcp_servers`，前端无法按服务和
   Client 聚合。
2. 审计页只有只读调用明细，没有从 invocation 定位 active grant 并撤销单项工具权限的接口。
3. 接入端创建请求同时接受 `server_id` 和 `tool_allowlist`，导致“绑定服务”和“授权工具”两个动作
   混在一个表单中。
4. Client 授权页同时展示接入端表和旧 Client 表，形成重复信息。
5. Codex 可能缓存一次 `tools/list` 结果；因此只更新展示列表不构成安全边界。安全边界必须位于
   `RuntimeCall`，每次调用都读取当前 active grant 并拒绝不在 allowlist 中的工具。

## 3. API 与数据流

### 3.1 审计查询

`GET /api/v1/mcp-platform/invocations` 返回的每项增加：

- `server_id`、`server_name`；
- `client_id`、`client_key`、`client_name`；
- `tool_enabled`，表示该 Client 当前是否仍获准调用该工具。

响应仍只包含元数据和摘要，不返回工具参数、上游结果正文或认证信息。

### 3.2 从审计禁用工具

新增：

```text
POST /api/v1/mcp-platform/invocations/{invocation_id}/disable-tool
permission: mcp:grant:write
```

服务端根据 invocation 的 `client_id` 和 `tool_revision_id` 验证该工具确实属于 Client 当前绑定的
单一服务，然后以 active grant 为目标，从 allowlist 移除工具别名。重复调用保持幂等。

### 3.3 运行时

```text
Codex tools/call
  -> mcp-gateway
  -> POST /internal/mcp-runtime/call
  -> resolve active credential + active grant
  -> check current tool_allowlist
  -> denied: return MCP tool-not-allowed, do not call upstream
  -> allowed: create invocation and call upstream MCP Server
```

`tools/list` 同样读取当前授权。已经启动的 Agent 若缓存旧工具，界面中可能暂时仍看到旧名称，但实际
调用必须立即失败；重新打开 Codex 会话后工具清单同步收敛。

## 4. 前端交互

- 调用审计按服务卡片分组；服务内按“工具 + Client”汇总调用次数、最近状态、策略和最近调用时间。
- 每个“工具 + Client”行提供“禁用”按钮；已禁用时显示“已禁用”且按钮不可用。
- 禁用前二次确认，文案明确只影响目标 Client。
- 新增接入端抽屉只保留标识、名称和绑定服务。
- 工具开关只保留在 Client 授权的工具控制抽屉。

## 5. 验收与回归

1. 创建接入端不提交 `tool_allowlist`，默认授权所选服务全部 approved 工具。
2. Client 授权关闭任意工具后，`tools/list` 不再返回该工具。
3. 对已关闭工具直接发送 `tools/call`，返回 not allowed，且上游调用次数保持为零。
4. 审计接口返回正确的服务、工具、Client 关联及当前授权状态。
5. 审计“禁用”只删除目标 Client 的单个工具授权，重复禁用不报错。
6. 调用审计无 AI 状态列；Client 授权页无重复旧 Client 表。

## 6. 日志与安全

成功撤权记录 `mcp_client_tool_disabled_from_audit`，只包含 invocation、client、grant、server ID、
工具别名和操作者；不得记录 Client Token、上游凭据、工具参数或返回正文。运行时继续 fail-closed。

## 7. 回滚

前端可回滚为原平铺审计；新增 API 保持向后兼容。若误撤权，管理员在“Client 授权”重新开启对应工具
即可恢复，不需要重建接入端或 Token。

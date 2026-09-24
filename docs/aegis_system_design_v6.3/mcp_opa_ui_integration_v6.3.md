# MCP OPA 管理 UI 与 Rego 契约（V6.3）

## 问题与目标

MCP 聚合页面需要把运行时授权策略作为独立的 OPA 页签管理。概览页只展示 MCP 资源指标和接入操作，不把策略编辑器混在首屏。策略作者编辑实际执行的 Rego 源码，服务端负责 OPA 编译、决策契约校验和原子发布。

## 使用方式

进入“设置 → MCP 聚合管控 → OPA 策略”。页签按需读取当前 active 版本、语言版本、摘要和发布人。具备 `mcp:policy:read` 的账号可以查看；具备 `mcp:policy:publish` 的账号可以编辑源码、先校验，再点击“发布并生效”。页签状态通过 `?tab=opaPolicy` 保留，允许刷新和深链接直接进入策略页。

源码必须声明 `package aegis.mcp.authz`，并提供 `data.aegis.mcp.authz.decision`。输入是服务端构造的 `aegis.mcp.authz.input.v1`；决策对象必须包含 `contract_version`、`allow`、`reason_code`、`deny_rule_ids`、`audit_rule_ids` 和 `policy_revision`。页面源码编辑区只是 Rego 编辑器，不将结构化字段伪装成 OPA 规则。旧版结构化策略仍可读取和运行；打开后会显示迁移提示，发布 Rego 后切换为源码策略。

## API 与服务边界

- `GET /api/v1/mcp-platform/authorization/policies`：需要 `mcp:policy:read`，返回 active/empty 状态、版本、摘要、发布人和有权限读取的 `rego_source`。
- `POST /api/v1/mcp-platform/authorization/policies/validate`：需要 `mcp:policy:read`，使用与发布完全相同的 body 限制、未知字段拒绝、OPA 编译和决策契约检查；不写库、不切换运行时快照。
- `POST /api/v1/mcp-platform/authorization/policies`：需要 `mcp:policy:publish`。通过同一 `mcpauthz.Prepare` 校验后把源码作为 immutable version 持久化并原子激活；持久化或切换失败时旧 evaluator 和旧数据库版本保留。

`rego_source` 允许与旧的 typed `permissions` DTO 共存。没有 `rego_source` 时继续使用代码拥有的受控模板；有 `rego_source` 时源码是唯一运行时决策来源，typed permissions 不会覆盖源码。服务启动会重新编译 active source，失败则 fail-closed，不把坏版本作为允许策略。

## 安全约束

源码上限为 256 KiB，HTTP body 上限为 1 MiB，策略 JSON 禁止未知字段和尾随 JSON。只接受一个 module，必须使用固定 package；禁止 `http.send`、`opa.runtime`、`opa.eval`、`io.jwt` 相关能力，OPA capabilities 不允许网络连接。决策查询固定为 `data.aegis.mcp.authz.decision`，undefined、null、多结果、缺字段、类型错误、版本不一致均拒绝。源码策略无法提前声明工具可见性时，`tools/list` 仅保留 Go grant allowlist，最终放行必须经过 `tools/call` 的 Rego 决策。

## 测试与回滚

后端覆盖源码编译与允许决策、禁止 builtin、缺失决策字段、validate 不持久化和发布失败保留旧版本；前端覆盖独立页签、源码编辑器、服务端校验/发布路径和权限只读状态。发布使用 `mcp_policy_versions` 的事务性 active/superseded 切换，源码校验或数据库失败均不会替换旧快照。当前版本只展示 latest active，不提供历史版本选择器；回滚通过管理员重新发布已审查源码或后续补充显式版本回滚接口。

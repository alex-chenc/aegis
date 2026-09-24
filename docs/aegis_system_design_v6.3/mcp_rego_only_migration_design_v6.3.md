# MCP Rego-only 安全迁移设计与验收

## 范围与结果

MCP 聚合调用的安全动作由受限、预编译的 Rego evaluator 统一产生。调用前和调用后使用版本化的 `aegis.mcp.security.input.v2` / `aegis.mcp.security.decision.v2` 合同；Go 只负责构造最小输入、执行超时与并发边界、持久化决定和交付结果。认证、凭据、grant、catalog release、tool schema、SSRF 和 invocation 审计仍由 Go 硬门禁负责。

Rego pre 阶段支持 L4、敏感输入键和输入注入特征的 deny。Rego post 阶段支持敏感输出键、输出注入特征的 deny，以及响应过大和上游失败的 audit。post deny 只阻止结果交付，不能撤销已经发生的上游副作用；高风险工具必须在 pre 阶段控制。

Rego 故障、超时、合同错误、未定义结果和错误 policy revision 均 fail closed，并将 invocation 标记为失败或阻断；不会回退到旧 Go matcher，也不会把旧规则命中伪造为新 Rego 结果。

## 审计与兼容

`mcp_authorization_decisions` 保留调用前授权决定。`mcp_security_verdicts` 保留 invocation 级风险投影，并记录 `engine=rego`、`source=rego`、phase、action、policy revision、rule IDs、audit IDs 和脱敏 evidence refs。旧 verdict 由迁移标记为 `engine=legacy/source=legacy`，历史 matched rule 只来自只读归档。

迁移 `040_v6.4_mcp_rego_only_archive.sql` 为 `mcp_rule_definitions` 和 `mcp_rule_hits` 建立冻结归档副本，保留 034/036 原表及外键；运行时不再读取或写入这两张旧表。旧 `/security-rules` 列表和单条 enabled 开关 API、仓储方法与前端 drawer 已删除。OPA policy 页面是唯一 MCP 策略配置入口。

## 验收

- Rego contract 测试覆盖 pre L4 deny、post 敏感输出 deny、phase/upstream 不一致拒绝、未知 action 拒绝和 policy 编译失败。
- Runtime 测试确认 pre deny 的 upstream 调用次数为零，post deny 不返回敏感结果，且新 verdict 不写 `mcp_rule_hits`。
- 静态检查确认 MCP runtime 不引用 `evaluateMCPSecurity`、`ListEnabledSecurityRules` 或 `/security-rules`；Sigma、Agent Guard、Session、Skill 规则目录不在迁移范围内。
- 发布包和 Compose 按顺序执行 040 forward migration；重复执行使用 `IF NOT EXISTS` / `ON CONFLICT`，不会删除历史表或重写历史 invocation。

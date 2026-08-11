# MCP 上下文采集与安全规则匹配校准（v6.3）

## 1. 会话结论

本次 MCP 聚合管控联调确认了两个容易混淆的问题：

1. 安全规则列表与调用安全判定是两个不同视图。页面只保留一个“查看安全规则”按钮，完整规则表在右侧抽屉中展示；调用安全判定位于按钮下方，不再显示 AI 状态列。
2. 调用记录只有在经过运行时规则评估时，才能显示真实命中的规则。历史调用如果只有请求/结果 digest，没有原始或脱敏上下文，就不能回溯匹配规则，也不能伪造命中结果。

## 2. 当前已实现行为

### 2.1 MCP 调用链

```text
MCP Client
  -> mcp-gateway /mcp/v1/clients/{client_key}
  -> api-server /internal/mcp-runtime/call
  -> active Client credential + Grant + 当前工具 allowlist
  -> pre 规则评估
  -> Remote MCP Server tools/call
  -> post 规则评估
  -> 保存 invocation / rule hit / security verdict
  -> 结果交付或安全阻断
```

`RuntimeCall` 当前已经取得并使用以下上下文：

- Client key、Client ID、Grant 和当前授权工具集合；
- 工具别名、上游工具名、Tool Revision 和工具风险等级；
- `tools/call` 的 JSON 参数对象；
- 上游 MCP 返回对象或上游错误；
- 调用状态、请求/结果 digest、创建时间和完成时间。

上下文在内存中传给规则引擎，不把敏感字段值写入普通日志。当前数据库对调用主体主要保存元数据和 digest，历史记录不保证存在可重新评估的正文。

### 2.2 确定性规则

当前默认启用七条规则，按 `pre`/`post` 阶段从数据库加载并逐条匹配：

| 规则 | 阶段 | 匹配输入 | 命中后果 |
| --- | --- | --- | --- |
| L4 工具调用阻断 | pre | Tool 风险等级 | 阻断上游调用 |
| 敏感输入字段阻断 | pre | 参数对象的字段路径 | 阻断上游调用 |
| 注入型输入阻断 | pre | 参数对象中的文本模式 | 阻断上游调用 |
| 敏感结果字段阻断 | post | 上游结果字段路径 | 不向 Client 交付结果 |
| 工具结果提示词注入阻断 | post | 上游结果文本模式 | 不向 Client 交付结果 |
| 超大结果审计 | post | 序列化结果字节数 | 记录中风险审计结论 |
| 上游调用失败审计 | post | 上游错误 | 记录中风险审计结论 |

命中时写入 `mcp_rule_hits`，同时生成 `mcp_security_verdicts`。安全判定接口通过 `mcp_rule_hits -> mcp_rule_definitions` 关联返回规则名称；无命中时返回空规则数组和 `no_rule_matched` 证据。

Gateway 错误码保持区分：

- `-32003`：Client 当前 Grant 不允许该工具；
- `-32004`：工具调用被 MCP 安全规则阻断。

## 3. 历史记录为何无法命中

规则匹配需要参数或结果上下文。例如：

- 敏感输入规则需要读取 `$.token`、`$.password` 等字段路径；
- 注入规则需要读取参数文本；
- 敏感输出规则需要读取结果字段路径；
- 超大结果规则需要知道序列化后的结果大小。

本次上线前的历史记录只包含调用元数据、`request_digest`、`result_digest` 和状态，没有保存可供规则引擎读取的原始/脱敏上下文。因此迁移只创建保守的历史投影：

```json
{
  "type": "historical_projection",
  "reason": "historical_payload_unavailable",
  "status": "succeeded"
}
```

此类记录必须显示“历史调用投影（无正文）”，不能显示为“未命中规则”或补造规则命中。规则引擎启用后的新调用才有确定性匹配证据。

## 4. MCP 上下文采集目标

### 4.1 四阶段上下文

后续完善应保留四个逻辑阶段，但每个阶段都必须先脱敏、限长和分类：

1. Client 请求：JSON-RPC 方法、请求 ID、工具别名、参数摘要和脱敏参数；
2. Effective request：策略变换后实际发送给上游的工具名、参数摘要和上游身份引用；
3. Upstream response：上游结果/错误的脱敏摘要、大小、结果 digest 和规则证据；
4. Delivered response：实际交付 Client 的结果摘要、隔离/裁剪状态和 delivered digest。

`initialize`、`tools/list` 等协议上下文还应记录协议版本、Client 能力、服务能力和工具目录 digest；这些数据用于审计和漂移分析，不应被当作调用参数。

### 4.2 保存分层

| 层级 | 内容 | 保存位置 | 约束 |
| --- | --- | --- | --- |
| 实时评估上下文 | 参数对象、结果对象、错误对象 | Gateway/api-server 内存 | 只在调用生命周期内使用，规则完成后释放 |
| 审计摘要 | 字段路径、类型、大小、digest、规则证据、状态 | PostgreSQL | 不保存 Token/密码/私钥值，不保存无限正文 |
| 受限完整上下文 | 经过脱敏、加密和保留策略处理的四阶段 payload | MinIO 加密对象 | 需要专用权限、purpose、二次认证和 reveal 审计 |

缺少完整上下文不影响同步规则阻断，但必须将证据状态标记为 `partial` 或 `historical_payload_unavailable`，不能降级成绿色安全。

### 4.3 安全边界

- Authorization、Bearer token、cookie、password、private key、API key 等值永不进入普通日志、Kafka、前端状态或 AI 输入；
- 规则证据优先保存字段路径和匹配规则键，不保存敏感字段值；
- 上下文对象设置最大字节数和截断标记，拒绝超限请求或按策略生成中风险审计；
- 上游结果是“不可信数据”，不能作为规则引擎或 AI worker 的指令；
- AI 当前不参与同步阻断，后续 AI 只能使用脱敏结构化上下文，不能降低确定性风险。

## 5. 前端最终交互

安全分析标签页的最终布局为：

```text
[查看安全规则]

[调用安全判定：服务 / 工具 / Client / 命中规则 / 确定性风险 / 综合风险 / 判定依据 / 时间]
```

点击“查看安全规则”后，右侧抽屉展示完整规则表、规则阶段、匹配条件、风险、防护动作、启停状态和分页。主页面不直接展开规则表，不为每一行单独提供规则详情按钮；调用安全判定不显示 AI 状态列。

## 6. 验证证据

- 传入 `{"token":"..."}` 的新调用被 `敏感输入字段阻断` 命中，返回 JSON-RPC `-32004`，上游不再收到请求；
- 新调用的安全判定返回 `matched_rules`、规则 evidence、critical deterministic severity 和 critical overall risk；
- 正常空参数调用生成 `no_rule_matched` 的 low 结论；
- 历史调用保持 `historical_projection`，不伪造命中；
- 前端安全规则抽屉可打开完整 7 条规则并分页；
- MCP 相关前端测试 11 项通过，Frontend Docker 构建成功，API Server 定向 MCP 安全测试通过。

## 7. 后续工作边界

本校准文档不把完整 MinIO payload、Kafka 四阶段事件、跨调用 Activity 规则或异步 AI 分析宣称为已完成。后续实现必须先补上下文脱敏/加密存储、权限控制、保留策略和回放测试，再扩大规则与 AI 分析范围。

# MCP 分页与安全防护闭环补齐设计（v6.3）

## 1. 问题与目标

当前 MCP 聚合管控存在三类不完整行为：

1. 工具列表按工具平铺，服务信息依赖前端从另一份服务列表推断，无法稳定按服务展示；
2. 部分接口虽支持分页，但前端固定请求前 100 条且没有分页控件；工具和 Client 接入端接口本身仍为全量返回；
3. 数据库已有规则、规则命中和安全结论表，但没有预置规则、规则管理 API，也没有在运行时调用链执行规则并生成结论，因此安全分析为空且不能实施阻断。

目标：

- 远程服务、工具列表、Client 授权、审批中心、调用审计和安全分析均使用服务端分页；
- 工具查询直接返回所属服务，页面按“服务 -> 工具”分组；
- 提供默认启用的确定性安全规则及规则启停控制；
- 每次已进入 MCP 上游调度的调用都生成安全结论；高风险命中可在调用前阻断，敏感结果可在交付给 Client 前阻断；
- 安全分析显示服务、工具、Client、命中规则、确定性风险、综合风险和证据摘要。
- 安全规则列表每条记录提供规则详情弹窗；调用安全判定通过 `mcp_rule_hits` 关联并显示实际命中的规则名称。安全判定页面不展示 AI 状态列，AI 字段仍保留在后端作为兼容性数据，不参与确定性阻断展示。

## 2. 分页契约

所有列表接口接收 `page`、`page_size`，返回：

```json
{
  "items": [],
  "total": 0,
  "page": 1,
  "page_size": 10
}
```

本次覆盖：

- `GET /mcp-platform/servers`
- `GET /mcp-platform/tools`
- `GET /mcp-platform/client-endpoints`
- `GET /mcp-platform/approvals`
- `GET /mcp-platform/invocations`
- `GET /mcp-platform/security-verdicts`
- `GET /mcp-platform/security-rules`

工具列表项由 Repository 关联 server revision 和 server，直接返回 `server_id`、`server_name`，避免前端依赖当前服务页推断归属。分页仍以工具记录为计数单位，当前页内再按服务分组。

## 3. 安全规则

新增幂等迁移预置七条规则：

| 规则 | 阶段 | 默认动作 | 说明 |
| --- | --- | --- | --- |
| L4 工具调用阻断 | pre | block | 已发布工具风险达到 L4 时调用前阻断 |
| 敏感结果字段阻断 | post | block | 返回对象字段名疑似 token、secret、password、credential、private key 时交付前阻断 |
| 超大结果审计 | post | audit | 返回 JSON 超过 512 KiB 时标记中风险 |
| 上游调用失败审计 | post | audit | 上游调用失败时形成中风险结论 |
| 敏感输入字段阻断 | pre | block | 参数键名疑似 password、token、secret、credential 时调用前阻断 |
| 注入型输入阻断 | pre | block | 检测路径穿越、SQL、Shell、Header 和指令注入特征，调用前阻断 |
| 工具结果提示词注入阻断 | post | block | 不可信工具结果包含典型提示词覆盖指令时交付前阻断 |

规则 definition 只保存 matcher、阈值/键名和 action。规则管理 API：

```text
GET /mcp-platform/security-rules
PUT /mcp-platform/security-rules/{id}/enabled
body: {"enabled": true|false}
```

读取使用 `mcp:policy:read`，启停使用 `mcp:policy:write`。规则变更记录操作者、规则 ID、规则 key 和目标状态，不记录调用参数或结果正文。

## 4. 运行时防护

```text
Client tools/call
  -> 鉴权 + active grant allowlist
  -> 创建 invocation
  -> 执行 pre 规则
      -> block: 写 rule hit/verdict，拒绝且不请求上游
  -> 请求上游 MCP Server
  -> 执行 post 规则
      -> block: 写 rule hit/verdict，结果不交付 Client
      -> audit/safe: 写 rule hit/verdict，按原行为交付或报告失败
```

安全结论至少包含确定性风险和综合风险。AI 分析是附加层：当前同步数据面不等待 LLM，`ai_verdict=not_run`；确定性规则始终执行且 AI 未来不得降低其风险下限。这样即使 AI 未配置或不可用，阻断边界仍然有效。

Gateway 将安全阻断映射为 JSON-RPC `-32004`，Client 工具授权拒绝仍为 `-32003`，避免把安全拦截误报为普通权限不足。

敏感字段规则只把命中的字段路径写入 evidence，不保存字段值；超大结果只记录字节数和摘要；任何安全日志均不含 Token、Authorization、工具参数或结果正文。

## 5. 历史数据

迁移为尚无 verdict 的历史 invocation 生成保守投影：成功调用为 low，失败调用为 medium，并标记 `historical_payload_unavailable`。历史数据无法重新检查已丢弃的正文，不伪造规则命中。

## 6. 验收测试

1. 工具接口分页总数正确，每项包含服务 ID/名称，前端按服务分组。
2. 六个主标签页均显示分页并在翻页时发送对应 page/page_size。
3. Client 接入端分页不再一次返回全量数据。
4. 新调用无规则命中也生成 low/safe 安全结论。
5. L4 工具在上游调用前被拒绝。
6. 返回对象含敏感字段时生成 critical rule hit/verdict，Client 收不到上游结果。
7. 禁用敏感字段规则后相同结果可正常交付；重新启用后恢复阻断。
8. 安全规则和安全结论列表均可分页。
9. 路径穿越或典型 SQL/Shell/提示词注入参数在请求上游前被阻断，证据只记录字段路径和规则模式。
10. 规则详情按钮可打开完整 matcher、阈值/模式、动作、版本和 digest；安全判定能显示对应规则名称，未命中时显示“未命中规则”。

## 7. 回滚

- 前端可回滚分组和分页组件，不影响接口兼容；
- 规则可在安全分析页逐条关闭，无需删除数据；
- 回滚运行时评估代码后，既有 verdict/rule hit 仅作为审计历史保留；
- 迁移只新增预置数据和索引，不删除业务数据。

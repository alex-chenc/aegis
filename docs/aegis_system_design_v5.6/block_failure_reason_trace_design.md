# 阻断失败原因透传设计

## 背景

告警列表当前只突出显示“阻断失败”，失败原因需要进入详情才容易看到；部分 Agent 执行错误也会被 Server 包装为通用发送失败，导致页面无法判断是 Agent 未连接、目标无效、权限不足还是策略执行失败。

## 目标

1. Agent 执行阻断失败时必须返回可读原因。
2. Server 返回给 API 的失败原因应保留 Agent 原始原因，不泛化为无意义的“发送失败”。
3. API 必须把失败原因保存到 `block_records.message` 与 `alerts.block_message`。
4. 前端告警列表和详情都能直接看到失败原因。
5. 真实测试覆盖三种阻断策略的成功与失败路径。

## 失败原因约定

| 层级 | 原因来源 | 展示内容 |
| --- | --- | --- |
| API 前置校验 | 缺失 PID、文件路径、IP | `missing ...` 或 `invalid ...` |
| Server 连接层 | Agent 未连接、通道不可用 | `agent not connected: <host_id>` |
| Agent 执行层 | 进程、文件、iptables 执行失败 | Agent 返回的具体错误 |

## 实现策略

- Agent `Blocker.Execute` 对空 action、空 target、非法 IP/CIDR 做前置校验。
- `block_connection` 捕获 iptables stderr，返回真实命令失败原因。
- Agent `HandleBlockCommand` 对空命令返回明确错误，并在失败时带 action/target 上下文。
- Server `ExecuteBlockCommand` 对 `SendBlockCommand` 返回的错误直接填入 response error。
- 前端告警列表增加阻断结果列，失败时显示原因摘要和 tooltip。

## 验证

- 单元/集成测试先覆盖 Agent 失败原因和 Server 失败原因透传。
- 主机真实测试通过 curl 构造告警并调用阻断接口，覆盖：
  - `kill_process` 成功与失败。
  - `quarantine_file` 成功与失败。
  - `block_connection` 成功与失败。

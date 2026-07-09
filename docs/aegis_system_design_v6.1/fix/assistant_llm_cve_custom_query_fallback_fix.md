# 智能助手 LLM CVE 参数与自定义查询回退修复

## 1. 问题、范围与成功标准

智能助手处理“为指定 CVE 生成 POC/修复脚本并下发”时存在两个问题：

1. 用户消息已经包含 CVE 编号，工具裁决仍可能提示缺少 `cve_id`。
2. `Vulnerability.List` 未查到 CVE 时，Runtime 无法调用已有自定义 CVE 查询服务补录漏洞。

本次只修改 `api-server` 的智能助手意图、工具注册、参数绑定和提示词，不改变公网 HTTP API、数据库表、gRPC 协议或主机执行权限。

成功标准：

- 生产请求不再通过规则意图预解析或规则工具选择降级，业务语义由 LLM 输出。
- 用户原文中的标准 CVE 标识在后端只做格式规范化和参数来源校验，不因 LLM 结构化输出遗漏而丢失。
- `Vulnerability.List` 精确绑定 `query=cve_id`。
- Runtime 可见自定义 CVE 查询启动和状态工具，并能在漏洞列表为空时动态选择它们。
- 自定义查询成功后可使用返回的 `result_vulnerability_id` 继续漏洞脚本流程；失败或超时必须明确停止。
- 写操作继续遵守现有工具审批策略。

## 2. 当前与目标行为

当前链路在 LLM 工具选择前先执行规则 IntentBreakdown，并在后续 LLM 拆解中混合规则结果。自定义 CVE 服务只绑定 HTTP Handler，没有注册到助手 ToolRegistry。

目标链路：

```text
LLM IntentRouter
  -> LLM IntentDecomposer
  -> LLM Tool Selection
  -> ToolDecisionEngine
  -> agent-runtime
       -> Vulnerability.List(query=cve_id)
       -> 结果非空：继续受影响主机/脚本流程
       -> 结果为空：Vulnerability.CustomQuery.Start
                    -> Vulnerability.CustomQuery.Status
                    -> 成功后继续脚本流程
```

ToolDecisionEngine 仍负责工具存在性、参数来源、风险、审批和允许工具集合，但不根据关键词替代 LLM 解释业务意图。

## 3. 设计决策

### 3.1 LLM 是唯一业务语义来源

- `IntentRouter.Classify` 在配置 LLM 后始终使用 LLM，不再根据规则置信度选择规则结果。
- Orchestrator 删除首次 `EnableLLMDecomposition=false` 的规则预解析。
- LLM IntentBreakdown 在工具选择前完成；只有明确的 `need_clarification` 才停止工具选择。
- LLM 调用、JSON 解析或结构校验失败时直接失败，不回退规则解析。

### 3.2 CVE 只做字面规范化

后端允许对用户明确写出的 CVE 标识执行以下非语义处理：

- 大小写统一为大写；
- Unicode/全角横线 `‐‑‒–—―−－` 统一为 ASCII `-`；
- 校验 `CVE-YYYY-NNNN...` 格式；
- 将规范化结果作为 `user_message` 参数来源写入 IntentBreakdown。

这不判断用户意图、不选择工具，仅保证显式标识不会在结构化传递中丢失。

### 3.3 自定义 CVE 工具

新增内部助手工具：

- `Vulnerability.CustomQuery.Start(cve_id)`：调用现有 `CustomCVEService.StartCustomQuery`。
- `Vulnerability.CustomQuery.Status(query_id, wait_seconds)`：查询状态，可在单次调用内短暂等待，返回 `result_vulnerability_id`。

`Start` 是会创建查询记录并调用外部 LLM 的中风险写工具，沿用审批策略；`Status` 是只读工具。状态轮询结果不能复用旧缓存。

### 3.4 Runtime 动态分支

工具集合和提示词向 Runtime 说明：

- 必须先精确调用 `Vulnerability.List(query=CVE)`；
- 只有 `total=0` 时才调用自定义查询；
- 自定义查询成功后使用 `result_vulnerability_id`，不得把 CVE 字符串当 UUID；
- 用户指定在线主机时使用 `Host.List(status=online)`；自定义 CVE 没有扫描产生的主机关联，不得因 `AffectedHosts` 为空直接认定没有执行目标。

后端只提供允许工具和参数绑定，不暗中补跑任何工具。

## 4. 日志与失败处理

- 自定义查询创建、成功和失败记录 `query_id`、`cve_id`、`vulnerability_id` 与高层错误类型。
- 不记录 LLM 凭证、原始响应、脚本内容或主机敏感数据。
- 同一 CVE 已有进行中查询时复用其 `query_id`；其他 CVE 查询占用全局槽位时返回明确错误。
- 状态等待受调用上下文和最大等待秒数约束，不无限轮询。

## 5. 测试设计

| 用例 | 期望 |
| --- | --- |
| LLM breakdown 漏掉原文中的 ASCII CVE | CVE 被规范化绑定，不追问 `cve_id` |
| 用户使用 Unicode 横线 | 规范化为 ASCII CVE |
| CVE 修复计划 | `Vulnerability.List.args.query` 和脚本工具 `cve_id` 正确 |
| CVE 修复允许工具集合 | 包含自定义查询 Start/Status |
| 自定义查询启动 | 返回 `query_id/cve_id/status` |
| 同一 CVE 查询已在进行 | 复用现有查询 |
| 状态成功 | 返回 `result_vulnerability_id` |
| 状态失败或参数非法 | 返回明确错误，不继续脚本流程 |
| 普通问答或非 CVE 请求 | 不新增无关写工具 |

验证命令：

```bash
cd api-server
go test ./internal/assistant ./internal/assistant/tools -count=1
make build
```

实现验证结果：

- `go test ./internal/assistant/... -count=1`：通过。
- `make build`：通过。
- 扩大到 `go test ./internal/... -count=1` 时，助手、工具、服务、仓储与 LLM 等相关模块通过；仓库现有的 `internal/api/handler` 用例仍有与本次链路无关的失败，包括 AI 分析期望值、审计日志测试表复用，以及自定义 CVE HTTP Handler 测试路由参数名与 Handler 读取参数名不一致。

## 6. 兼容性、风险与回滚

- 不新增迁移；现有前端自定义 CVE API 保持不变。
- LLM 不可用时请求将严格失败，不再规则降级，这是最新通用 Agent 设计要求的行为变化。
- 自定义查询依赖当前全局单查询限制；不同 CVE 并发请求仍会被拒绝。
- 回滚本次代码和文档即可恢复旧行为，无数据回滚。

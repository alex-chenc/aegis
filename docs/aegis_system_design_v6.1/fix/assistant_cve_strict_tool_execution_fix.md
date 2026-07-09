# 智能体 CVE 工具链严格失败修复

## 1. 问题描述与现象

会话 `asst_ed97188b` 收到“为 CVE-2023-43641 生成 POC 和修复脚本、对受影响主机下发并执行 5 轮自动修复”后出现以下异常：

- 后端能力映射已经存在，但运行时仍执行 `Tool.Search`；
- 搜索返回空结果后错误声明脚本生成、任务下发和自动修复不受支持；
- `Vulnerability.AffectedHosts` 将 CVE 编号当成内部 UUID，连续调用失败；
- 未实际调用脚本生成或执行工具，却把“生成 POC 和修复脚本”步骤标记为完成；
- 最终输出仍是 `action=tool_call`，会话却被标记为 `completed`；
- 智能体工具无法传递用户要求的 `max_rounds=5`，即使执行也会落到默认 3 轮。

数据库核验显示该会话没有为 CVE-2023-43641 创建 `vulnerability_scripts` 或 `task_logs`，界面完成状态没有工具证据。

## 2. 复现步骤

1. 新建智能体会话。
2. 输入：`针对漏洞CVE-2023-43641生成 POC 和修复脚本，并对存在的主机下发 POC 修复，轮数控制在 5 轮自动修复`。
3. 观察计划中出现“搜索可用工具”。
4. 观察工具调用中 `Vulnerability.AffectedHosts(vulnerability_id=CVE-...)` 返回 UUID 格式错误。
5. 观察没有脚本或任务记录，但计划步骤被标记完成，会话状态变为 `completed`。

## 3. 根因分析

### 3.1 意图拆解错误被静默降级

LLM 返回的 `IntentBreakdown.objects` 结构不合法时，`IntentDecomposer.Decompose` 返回规则拆解结果。规则拆解只保留了自然语言对象类型，没有从用户原文解析 CVE ID，导致 `cve_id` 参数丢失。

### 3.2 resident 工具绕过能力映射

`ToolSelector` 和 `ToolDecisionEngine` 无条件追加 `Tool.Search`、`Context.Get`、`Session.Summarize`，resident 工具又不受评分阈值限制。这与 V6.1 “ready 状态只注入执行计划内工具”的设计冲突。

### 3.3 缺失关键写工具没有阻断

脚本生成和执行工具因缺少 `cve_id` 被标记为 `clarification_required`，但只要存在任意已接受的只读/resident 工具，`ClarificationGate` 就继续执行部分计划，导致用户核心目标被丢弃。

### 3.4 CVE 编号与内部漏洞 UUID 未分层

`Vulnerability.AffectedHosts` 要求内部 `vulnerability_id` UUID；用户输入的是 `cve_id`。现有计划没有先执行 `Vulnerability.List(query=CVE)` 并把返回的内部 ID绑定到后续步骤。

### 3.5 搜索和完成态缺少证据约束

`Tool.Search` 使用整段查询做包含匹配，复合关键词返回空。更关键的是 `ToolResultVerifier` 虽已实现但没有接入执行链，Orchestrator 对任意非空 `FinalAnswer` 都写入 completed，即使内容仍是工具调用请求或计划有未完成步骤。

### 3.6 最大轮数未进入工具契约

`Vulnerability.Script.Execute` 没有 `max_rounds` 参数，工具接口调用默认 `ExecuteScripts`，固定回落到 3 轮。

## 4. 修复设计

### 4.1 严格失败策略

用户明确要求：不再执行降级。

- LLM 意图拆解启用后，只要客户端创建、调用、JSON 解析或结构校验失败，直接返回错误；
- LLM 工具选择启用后失败，直接返回错误，不回退规则选择器；
- ToolDecisionEngine 启用后失败，直接返回错误，不回退 preliminary selection；
- 用户核心写能力被 `clarification_required/rejected` 时停止；如果参数可从原文确定则绑定，否则返回明确追问，不执行部分计划；
- 运行时工具结果、后置条件或最终输出格式不合法时会话标记 `failed`，不得写 `completed`；
- 不再通过 Tool.Search 搜索已在后台 mapping 中注册的能力。

严格失败不等于把正常的业务追问当成系统错误：用户确实未提供主机范围等业务参数时，返回 `clarification_required`；内部解析/调用失败则返回 `failed`。

### 4.2 CVE 结构化参数

规则拆解从用户消息提取标准 CVE：`CVE-YYYY-NNNN...`，写入：

```text
IntentObject{Type: "cve", ID: "CVE-2023-43641", Source: "user_message"}
```

参数绑定规则将 `cve` 实体绑定到 `cve_id`。`Vulnerability.AffectedHosts` 不接受 CVE 字符串作为 `vulnerability_id`，必须从 `Vulnerability.List` 的结果解析内部 UUID。

### 4.3 固定漏洞脚本工作流

对“指定 CVE + 受影响主机 + 生成/执行 POC/FIX”意图生成如下计划：

```text
Vulnerability.List(query=cve_id)
  -> Vulnerability.AffectedHosts(vulnerability_id=previous_step.data[0].id)
  -> Vulnerability.Script.Generate(cve_id, poc)
  -> Vulnerability.Script.Generate(cve_id, fix)
  -> Vulnerability.Script.Status(cve_id, poc/fix)
  -> Vulnerability.Script.Execute(cve_id, poc, host_ids, max_rounds)
  -> Vulnerability.Script.Execute(cve_id, fix, host_ids, max_rounds)
  -> Task.GetDetail / Task.List
```

生成和执行属于写操作，继续遵守审批策略。后续参数必须来自用户消息或已验证前置步骤，不允许模型编造。

### 4.4 Tool.Search 策略

- 不再作为 resident 工具无条件注入；
- 后端 mapping 找不到能力时直接报告未映射能力；
- 运行中需要新增能力时必须返回 `additional_capability_request` 重新裁决，不允许运行时自行搜索和扩大工具集；
- 保留 Tool.Search 注册只用于显式的“搜索工具”管理请求，不参与普通业务计划。

### 4.5 结果与完成态校验

- CVE 固定工作流逐步校验：漏洞唯一 UUID、非空主机 UUID、脚本状态 `generated` 与 `script_id`、执行结果 `task_group_id` 与非空 `task_ids`；
- 未知 `ToolResultVerifier` 后置条件由默认通过改为直接失败；
- Orchestrator 在写入 completed 前校验运行时无错误、结果非空且最终答案不是 `tool_call`/`additional_capability_request` 等未完成控制动作；
- 校验失败返回可检索错误日志并标记会话 failed。

### 4.6 最大轮数

`Vulnerability.Script.Execute` 增加 `max_rounds`，范围 `1..10`，由用户消息绑定；工具调用 `ExecuteScriptsWithOptions`，设置 `AutoVerify=true` 与指定轮数。

## 5. 接口变化

`Vulnerability.Script.Execute` 新增可选参数：

```json
{
  "max_rounds": 5
}
```

未指定时仍采用服务默认值 3；指定非法值直接报参数错误，不做降级或截断。

内部工具服务接口增加带 `ExecuteScriptsOptions` 的执行方法。无数据库迁移和公网 HTTP API 变化。

## 6. 日志与可观测性

使用现有 zap 日志记录意图拆解、工具选择、工具裁决、运行时构建、工具调用和工作流校验错误。日志保留 session_id、run_id、tool_name 和错误摘要；不记录密钥、Token 或脚本正文。

## 7. 回归测试与验收标准

| 编号 | 场景 | 期望 |
| --- | --- | --- |
| TC-01 | LLM IntentBreakdown JSON 结构错误 | Decompose 返回错误，不返回规则 breakdown |
| TC-02 | LLM 工具选择失败 | Orchestrator 返回错误，不调用规则 selector |
| TC-03 | ToolDecisionEngine 失败 | Orchestrator 返回错误，不使用 preliminary selection |
| TC-04 | 普通漏洞执行请求 | 最终计划不包含 Tool.Search/Context.Get/Session.Summarize |
| TC-05 | 用户消息含 CVE-2023-43641 | breakdown 中存在 `type=cve,id=CVE-2023-43641` |
| TC-06 | 脚本工具缺少不可推导参数 | 停止并追问/报错，不执行部分只读计划 |
| TC-07 | AffectedHosts 参数绑定 | CVE 字符串不直接绑定到 vulnerability_id，参数来自 Vulnerability.List 结果 |
| TC-08 | Tool.Search 复合词 | 普通业务链路不调用 Tool.Search |
| TC-09 | 未执行脚本生成却标记完成 | 完成态校验失败，会话状态 failed |
| TC-10 | FinalAnswer 为 action=tool_call | 完成态校验失败，不发布 completed |
| TC-11 | Execute max_rounds=5 | task_logs.max_rounds 保存为 5 |
| TC-12 | Execute max_rounds 非法 | 工具直接返回参数错误 |
| TC-13 | CVE POC/FIX 完整链路 | 生成、状态、执行、任务查询均有工具证据后才能完成 |
| TC-14 | 运行日志 | 严格失败日志包含 session/run/trace/tool，不包含敏感值 |

## 8. 安全、兼容性与风险

- 正向影响：杜绝模型绕过 mapping、伪造完成状态以及丢失用户指定修复轮数。
- 行为变化：过去会静默降级的 LLM/解析错误将直接失败，用户会看到明确错误；这是本次明确要求。
- Tool.Search 仍保留注册，显式工具管理场景兼容；普通业务计划不再自动注入。
- 写工具仍需审批，不因固定工作流降低权限。

## 9. 验证步骤

1. `go test ./internal/assistant -count=1`
2. `go test ./internal/api/handler -run Assistant -count=1`
3. `go test ./internal/service -run HostVulnerabilityScript -count=1`
4. `make build`
5. `docker compose up -d --build api-server`
6. `curl http://localhost:8082/health`
7. 检查新会话中无 Tool.Search，错误时 session=failed，成功执行时 max_rounds=5。

## 10. 回滚方案

回滚本次代码即可恢复旧行为；无数据库迁移。由于旧行为会产生伪完成和错误执行风险，不提供运行时开关重新启用静默降级。

## 11. 影响组件

- `api-server/internal/assistant/`
- `api-server/internal/assistant/tools/vulnerability_tools.go`
- `api-server/internal/service/host_vulnerability_script_service.go`
- API Server 容器

## 12. 实施与验证结果（2026-07-09）

- 普通业务请求不再自动注入 `Tool.Search`、`Context.Get`、`Session.Summarize`；只有显式工具管理意图可选择 resident 工具。
- CVE 自动处置在通用 LLM/工具裁决前进入确定性主路径，后端固定展开 POC/FIX，不再因缺少 `script_type` 追问而提前结束。
- `Vulnerability.Script.Execute.max_rounds` 已透传 `ExecuteScriptsWithOptions`，范围 `1..10`，自动验证开启。
- IntentRouter、IntentDecomposer、LLM 工具选择、ToolDecisionEngine 与 Runtime Build/Run 的已启用路径错误均直接返回；显式工具序列补执行已从运行链路移除。
- Runtime 已关闭 `AllowSkipFailedStep` 与 `AllowBestEffortAnswer`，未完成控制 JSON 不再写 completed。
- 测试通过：`go test -count=1 ./internal/assistant/...`、相关 service 测试、`make build`。
- 容器已通过 `docker compose up -d --build api-server` 重建，`GET /health` 返回 `{"status":"ok"}`。
- 真实 API 严格失败用例 `asst_00da83a1`：无效 CVE 最终 `status=failed`，仅调用一次 `Vulnerability.List`，`Tool.Search=0`，脚本生成/执行调用均为 0。
- 目标只读验证：`CVE-2023-43641` 唯一解析为漏洞 UUID `6ced16c4-2a38-4e30-8725-42562b3e3ba4`，受影响主机数为 1；未在验证阶段执行高风险脚本下发。
- 补充回归：自然操作入口统一提前于通用工具裁决，避免无关 `Task.GetDetail`/弱密码进度工具抢先触发 `task_id` 追问。真实会话 `asst_2b11eb23` 对“帮我进行一次在线主机的漏洞扫描任务”仅调用 `Host.List -> Vulnerability.Scan.Start`，成功创建扫描任务 `e0e6bc2c-d542-42a5-bc2e-ee8e1598e043`。

# 任务执行结果 UI 真实设计

**版本**: V5.7
**日期**: 2026-05-15
**状态**: 待实现
**适用范围**: 仅前端执行结果展示与测试设计，不新增后端接口，不修改后端返回协议。

---

## 1. 背景与真实问题

当前目标是让 AI 分析任务的执行结果以结构化、中文化、可快速判断的方式展示，替代只把最终文本放进 `pre` 文本框的展示方式。

此前 Claude 方案没有真正生效，核心原因不是后端缺少结构化数据，而是前端对接口返回值的判断与 Axios 封装不一致：

1. `frontend` 的 Axios 响应拦截器已经对后端响应做了解包。
2. `getExecutionResult(sessionId)` 实际返回的是 `ExecutionResult` 本体，而不是 `{ success, data }` 包装对象。
3. `AIAnalysis.vue` 仍按 `response.success && response.data` 判断接口结果。
4. 因此即使接口拿到了执行结果，判断也会失败，结构化结果不会进入展示状态，页面继续退回到纯文本展示。

修复方向是明确 `getExecutionResult` 的真实返回类型，并让 `AIAnalysis.vue` 直接按 `ExecutionResult | null` 消费。

---

## 2. 展示目标

执行结果展示需要覆盖两条来源路径，并统一渲染到同一个结构化结果组件：

1. **历史会话路径**：通过 `getExecutionResult(sessionId)` 拉取历史执行结果。接口经 Axios 解包后直接返回结构化 `ExecutionResult`。
2. **实时完成路径**：WebSocket 或流式过程结束时，最终文本可能只包含类似 `Task status`、`Exit reason`、`Completed step`、`Errors` 的文本块。前端需要解析该文本为结构化结果，再交给同一个组件展示。

用户体验目标：

- 任务状态、退出原因、总耗时突出显示。
- 每个步骤独立展示，不再混在一个 `pre` 文本框里。
- 错误信息单独展示，避免和正常结果混淆。
- 页面文案、状态、退出原因、步骤状态、结论使用中文。
- 最后始终生成一个总结结论，帮助用户快速判断处置方向。

---

## 3. 数据来源与归一化设计

### 3.1 历史接口返回

接口：

```text
GET /api/v1/detection/alerts/ai-analysis/:session_id/execution-result
```

前端真实消费类型：

```ts
const executionResult = await getExecutionResult(sessionId)
// executionResult 是 ExecutionResult 本体，不是 { success, data }
```

目标类型：

```ts
interface ExecutionResult {
  execution_id?: string
  task_id?: string
  session_id?: string
  status: string
  exit_reason?: string
  started_at?: string
  ended_at?: string
  total_duration_ms?: number
  steps?: ExecutionStep[]
  errors?: string[]
  conclusion?: ExecutionConclusion
}

interface ExecutionStep {
  step_id: string
  status: string
  result?: string
  started_at?: string
  ended_at?: string
  duration_ms?: number
}

interface ExecutionConclusion {
  verdict?: string
  summary?: string
  reasoning?: string
}
```

历史路径处理规则：

- `getExecutionResult` 返回非空对象时，直接设置为当前结构化执行结果。
- 不再使用 `response.success` 和 `response.data` 判断。
- 如果接口返回 `404` 或空结果，允许页面继续使用实时文本解析结果或原有消息流。
- 如果接口失败但已有实时解析结果，不覆盖已有结构化展示。

### 3.2 实时最终文本解析

实时任务完成后，最终文本可能是：

```text
Task status: completed
Exit reason: normal_completed
Completed step_1: Process 4181522 (base64 -d) has exited...
Completed step_2: 经分析，目标进程已退出...
Completed step_3: Benign / False Positive
Completed step_4: Benign / False Positive
Errors: open /proc/4181522/stat: no such file or directory...
```

只要最终文本包含以下字段之一，就认为它是可结构化解析的执行结果文本：

- `Task status:`
- `Exit reason:`
- `Completed step_`
- `Failed step_`
- `Running step_`
- `Errors:`

解析规则：

- `Task status: <value>` 解析为 `ExecutionResult.status`。
- `Exit reason: <value>` 解析为 `ExecutionResult.exit_reason`。
- `Completed step_N: <text>` 解析为 `steps[]`，`step_id = step_N`，`status = completed`，`result = text`。
- `Failed step_N: <text>` 解析为 `steps[]`，`status = failed`。
- `Running step_N: <text>` 解析为 `steps[]`，`status = running`。
- `Errors: <text>` 解析为 `errors[]`。多行错误按行拆分，空行过滤。
- 未识别的补充行优先追加到最近一个步骤的 `result`，没有步骤时追加到错误或总结候选文本。

实时路径处理规则：

- 完成事件到达时先解析最终文本。
- 解析成功后立即展示结构化结果，避免继续只显示 `pre` 文本框。
- 解析后仍应触发一次 `getExecutionResult(sessionId)` 拉取历史结构化结果。
- 如果接口返回真实 `ExecutionResult`，以接口结果为准覆盖临时解析结果。
- 如果接口暂时查不到结果，保留前端解析结果。

---

## 4. 双路径展示架构

```text
历史会话打开
  -> getExecutionResult(sessionId)
  -> ExecutionResult 本体
  -> TaskExecutionResult 结构化展示

实时任务完成
  -> 收到最终文本
  -> parseExecutionResultText(text)
  -> 临时 ExecutionResult
  -> TaskExecutionResult 结构化展示
  -> getExecutionResult(sessionId)
  -> 接口结果存在则替换临时结果
```

组件建议：

```text
AIAnalysis.vue
  ├── 负责拉取历史执行结果
  ├── 负责监听实时完成事件
  ├── 负责调用文本解析函数
  └── 将 ExecutionResult 传给 TaskExecutionResult.vue

TaskExecutionResult.vue
  ├── 状态摘要
  ├── 步骤列表
  ├── 错误信息
  └── 总结结论
```

展示降级规则：

- 有 `ExecutionResult`：展示结构化组件。
- 无 `ExecutionResult`，但有普通对话消息：展示原消息流。
- 只有不可解析最终文本：可以保留纯文本展示，但不能作为首选路径。

---

## 5. 中文化规则

### 5.1 任务状态

| 原始值 | 中文显示 | 类型 |
| --- | --- | --- |
| `completed` | 已完成 | success |
| `failed` | 执行失败 | danger |
| `interrupted` | 已中断 | warning |
| `limited` | 已受限 | warning |
| `running` | 执行中 | primary |
| `pending` | 等待中 | info |
| `cancelled` | 已取消 | info |
| 其他值 | 未知状态 | info |

### 5.2 退出原因

| 原始值 | 中文显示 |
| --- | --- |
| `normal_completed` | 正常完成 |
| `max_iterations` | 达到最大轮次 |
| `timeout` | 执行超时 |
| `user_cancelled` | 用户取消 |
| `cancelled` | 已取消 |
| `error` | 执行错误 |
| `audit_rejected` | 审计拒绝 |
| `drift_detected` | 检测到计划漂移 |
| `tool_failed` | 工具执行失败 |
| 其他值 | 未知原因 |

### 5.3 步骤状态

| 原始值 | 中文显示 | 类型 |
| --- | --- | --- |
| `completed` | 已完成 | success |
| `failed` | 失败 | danger |
| `running` | 执行中 | primary |
| `pending` | 等待中 | info |
| `skipped` | 已跳过 | info |
| 其他值 | 未知 | info |

### 5.4 结论判定

| 原始值或文本命中 | 中文显示 | 类型 |
| --- | --- | --- |
| `benign`、`false positive`、`Benign / False Positive` | 良性/误报 | success |
| `malicious`、`confirmed malicious` | 恶意 | danger |
| `suspicious`、`potentially malicious` | 可疑 | warning |
| `unknown`、无法判断 | 未知 | info |

中文化规则必须集中实现，避免在多个组件中散落硬编码。

---

## 6. 最后总结结论生成规则

结构化组件底部始终展示“分析结论”区域。结论来源按优先级生成：

1. **接口已有 `conclusion`**：直接使用接口的 `verdict`、`summary`、`reasoning`，仅做中文映射和空值兜底。
2. **步骤结果可推断结论**：从最后一个包含判定词的步骤中提取结论，例如 `Benign / False Positive`、`malicious`、`suspicious`。
3. **状态与错误兜底**：
   - `status = completed` 且无错误：结论为“执行完成，未发现明确异常结论”，判定为 `unknown`。
   - `status = completed` 且存在错误：结论为“执行完成，但存在采集或检查错误，需要结合错误信息复核”，判定为 `unknown`。
   - `status = failed`：结论为“执行失败，无法形成可靠安全结论”，判定为 `unknown`。
   - `status = interrupted` 或 `cancelled`：结论为“任务未完整执行，当前结果仅供参考”，判定为 `unknown`。
4. **推理依据生成**：
   - 优先取最后一个分析类步骤的自然语言结果。
   - 如果没有分析类步骤，取最后一个非空步骤结果。
   - 如果只有错误，使用错误摘要作为依据。
   - 所有依据都需要限制长度，超长内容在 UI 中折叠或截断。

结论区域字段：

```ts
interface DisplayConclusion {
  verdict: 'benign' | 'malicious' | 'suspicious' | 'unknown'
  verdictText: string
  type: 'success' | 'danger' | 'warning' | 'info'
  summary: string
  reasoning: string
}
```

---

## 7. UI 展示设计

```text
任务执行结果
├── 摘要
│   ├── 执行状态：已完成
│   ├── 退出原因：正常完成
│   └── 总耗时：5分30秒
├── 步骤执行详情
│   ├── step_1 已完成
│   │   └── Process 4181522 (base64 -d) has exited...
│   ├── step_2 已完成
│   │   └── 经分析，目标进程已退出...
│   └── step_3 已完成
│       └── 良性/误报
├── 错误信息
│   └── open /proc/4181522/stat: no such file or directory
└── 分析结论
    ├── 判定结果：良性/误报
    ├── 总结：判定为良性活动
    └── 依据：目标进程已退出，未发现异常外联或持久化迹象。
```

交互要求：

- 步骤结果默认展示摘要，长文本支持展开。
- 错误区域仅在 `errors.length > 0` 时展示。
- 没有耗时字段时不显示空耗时。
- 未知原始值可以在 tooltip 或次级文本中保留，主展示仍使用中文兜底。

---

## 8. TDD 测试计划

### 8.1 组件渲染测试

覆盖 `TaskExecutionResult.vue`：

- 给定完整 `ExecutionResult`，应渲染中文状态、退出原因、步骤、错误和结论。
- 给定无错误结果，不应渲染错误信息区域。
- 给定未知状态或未知退出原因，应渲染中文兜底文案。
- 给定长步骤结果，应保留内容并支持展开/折叠状态。

### 8.2 文本解析测试

覆盖 `parseExecutionResultText(text)`：

- 能解析 `Task status` 和 `Exit reason`。
- 能解析多个 `Completed step_N` 为 `steps[]`。
- 能解析 `Failed step_N`、`Running step_N` 并映射步骤状态。
- 能解析单行和多行 `Errors`。
- 遇到不可解析普通文本时返回 `null` 或明确的不可结构化结果。
- 能从 `Benign / False Positive` 推断 `benign` 结论。

### 8.3 `getExecutionResult` 返回类型测试

覆盖 `frontend/src/api/aiAnalysis.ts`：

- Mock Axios 拦截器解包后的返回值，确认 `getExecutionResult` 返回 `ExecutionResult` 本体。
- 测试调用方不再依赖 `{ success, data }`。
- 404 或空结果应可被调用方安全处理，不导致结构化展示状态被错误清空。

### 8.4 实时完成后拉取执行结果测试

覆盖 `AIAnalysis.vue`：

- 实时最终文本包含 `Task status` 等字段时，应先解析并展示结构化结果。
- 实时完成后必须调用 `getExecutionResult(sessionId)`。
- 接口返回结构化结果时，应以接口结果覆盖前端解析结果。
- 接口失败或暂无结果时，应保留前端解析出的结构化结果。
- 不应继续只显示纯 `pre` 文本框作为完成结果主视图。

---

## 9. 验收标准

- 历史会话打开后，能展示接口返回的结构化执行结果。
- 实时任务完成后，即使只有最终文本，也能解析并展示结构化执行结果。
- `AIAnalysis.vue` 不再按 `{ success, data }` 判断 `getExecutionResult`。
- 所有状态、退出原因、步骤状态、结论判定均有中文展示和兜底。
- 最后总结结论始终可见，且来源规则可测试。
- 前端测试覆盖组件渲染、文本解析、接口返回类型、实时完成后拉取执行结果。

---

## 10. 关联文档

- [agent优化设计](./agent_optimization_design.md) - TaskResult 持久化设计
- [前端详细设计](./frontend_detailed_design_v5.7.md) - 前端展示层设计
- [数据库结构设计](./database_structure_design_v5.7.md) - Agent 执行表结构

# V5.7 智能体模型上下文压缩设计

**版本**: 5.7  
**日期**: 2026-05-18  
**状态**: 设计中  
**适用范围**: `/code/agent-runtime` 上下文预算与压缩、Aegis `api-server` 适配器调用

---

## 1. 背景

Aegis V5.7 已将 AI 分析执行引擎迁移到 `agent-runtime`，执行流程为：

```text
Plan -> Execute Steps (ReAct per step) -> Reflect -> Audit -> Correct -> Summarize
```

在多步骤、多工具调用场景中，模型上下文会持续增长，尤其是：

- `QueryHistoricalLogs` 返回大量日志
- `GetRunningProcesses` 返回大量进程
- `GetNetworkConnections` 返回大量连接
- ReAct 当前步骤内存在多轮 action / observation / progress summary
- 用户一次勾选多个告警事件，初始上下文过大

当前实现中，`agent-runtime` 已有 `LLMUsage` 和 `ModelCallRecord.TokensUsed`，但 Aegis 侧尚未完整透传模型返回的 usage；同时，工具结果目前主要依赖固定长度截断，不能保证保留关键安全证据。

本设计目标是在不影响页面对话 UI、历史回放、审计持久化的前提下，只压缩发送给大模型的模型上下文。

### 1.1 核心硬性约束

本方案明确采用 **256K tokens 作为模型上下文最大上限**。

```text
MaxContextTokens = 256000
```

该值是运行时上下文预算的 hard limit：

- 任何单次 LLM 请求在发送前都必须满足 `estimated_prompt_tokens + reserved_output_tokens <= 256000`。
- 默认 `reserved_output_tokens = 8192`，因此默认可用 prompt 预算约为 `247808 tokens`。
- 达到或超过 256K 时，不允许直接请求大模型。
- 用户输入永不压缩；如果用户输入本身导致超限，只能要求用户缩短或拆分任务。
- 事件核心字段永不压缩；事件扩展详情、工具结果、历史步骤过程可压缩。

---

## 2. 设计原则

### 2.1 只压缩模型上下文

压缩仅作用于下一次 LLM 请求的 `LLMRequest.Messages`。

以下数据流不压缩、不覆盖：

- SSE 页面展示流
- AI 分析历史消息
- 工具调用历史
- 工具结果持久化
- `TaskResult`
- 审计、反思、纠偏记录

推荐边界：

```text
TaskEventLog / TaskResult / SSE Collector -> UI显示与持久化，不压缩
ContextMemory / PromptBuilder             -> LLM输入，可压缩
CompressionRecords                        -> 记录压缩事件，可选展示
```

### 2.2 用户输入永不压缩

以下内容为 hard pinned，不允许压缩、摘要、丢弃：

- `TaskInput.UserInput`
- 用户后续显式输入的问题或补充指令
- 系统输出格式约束
- 当前 step 的 title / objective / expected output
- 告警核心字段：`alert_id`、`rule_id`、`host_id`、`hostname`、`pid`、`ppid`、`commandline`、`timestamp`、`severity`、`rule_name`

如果用户输入本身超过模型上下文上限，不能通过压缩规避，应返回明确错误，提示用户缩短输入或拆分任务。

### 2.3 事件上下文分层

用户勾选告警事件后，事件上下文分为：

| 类型 | 是否压缩 | 说明 |
| --- | --- | --- |
| 用户输入文本 | 否 | 用户手写问题或指令 |
| 事件核心字段 | 否 | 每条事件的核心身份和安全判断字段 |
| 事件扩展详情 | 可压缩 | 原始日志、长 JSON、重复字段、超长 payload |
| 批量事件列表 | 可分批 | 每条事件保留核心卡片，详情按需展开 |

---

## 3. 上下文大小与预算设计

上下文压缩按“下一次 LLM 请求压力”触发，而不是按累计 token 成本触发。

```text
context_ratio = (estimated_prompt_tokens + reserved_output_tokens) / max_context_tokens
```

### 3.1 最大上下文

系统统一使用 256K tokens 作为最大上下文窗口：

```text
max_context_tokens = 256000
reserved_output_tokens = 8192
max_prompt_tokens = 247808
```

说明：

- `max_context_tokens` 是模型输入与输出的总窗口，不是只给 prompt 的窗口。
- `reserved_output_tokens` 用于避免 prompt 塞满后模型无法输出。
- UI 展示的上下文占用比例应使用 `context_ratio`，而不是累计 token 消耗比例。
- 不同模型真实窗口小于 256K 时，应由 Aegis 模型配置或 runtime config 下调 `MaxContextTokens`。

默认配置：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `MaxContextTokens` | `256000` | 最大模型上下文窗口 |
| `ReservedOutputTokens` | `8192` | 为模型输出预留 token |
| `ToolCompressionRatio` | `0.70` | 工具结果程序化压缩触发阈值 |
| `StepCompressionRatio` | `0.80` | 非当前步骤折叠触发阈值 |
| `LLMCompressionRatio` | `0.95` | LLM 摘要压缩触发阈值 |
| `CompressionTargetRatio` | `0.60` | 压缩后目标比例 |
| `RecentTurnsToKeep` | `6` | 95% 档保留最近 ReAct turns |

### 3.2 上下文组成

单次 LLM 请求的上下文由以下部分组成：

| 组成 | 来源 | 是否可压缩 | 说明 |
| --- | --- | --- | --- |
| System prompt | `PromptProvider` | 否 | 角色、工具规则、输出格式 |
| 用户输入 | `TaskInput.UserInput` | 否 | 用户原始问题，永不压缩 |
| 告警核心字段 | `UserContext` / alert context | 否 | alert_id、host_id、pid、commandline 等 |
| 告警扩展详情 | alert context extended payload | 是 | 原始日志、长 JSON、重复字段 |
| 当前 step | plan current step | 否 | title、objective、expected output |
| 当前 step 最近 turns | `ReactTurn` | 部分可压缩 | 最近 6 轮保留原文，工具结果可压缩 |
| 历史 step 过程 | `StepExecution.ReactTurns` | 是 | 80% 后折叠为 step result |
| 工具 observation | `Observation.Content` | 是 | 70% 后程序化压缩 |
| 历史经验 | `ExperienceProvider` | 是 | 初始过大时裁剪为 top N |
| 压缩摘要 | `CompressionRecord.Summary` | 否 | 已经压缩后的工作记忆 |

### 3.3 预算区间与展示状态

| 上下文占用 | 状态 | 行为 | UI 状态 |
| --- | --- | --- | --- |
| `< 60%` | 充足 | 不压缩 | 正常 |
| `60%-70%` | 关注 | 仅记录预算，不压缩 | 可显示灰色占用 |
| `70%-80%` | 工具压缩 | 程序化压缩工具结果 | 显示“已压缩工具结果” |
| `80%-95%` | 步骤折叠 | 非当前步骤仅保留步骤结果 | 显示“已折叠历史步骤” |
| `95%-100%` | 强压缩 | LLM 压缩 6 轮之前对话 | 显示“已生成上下文摘要” |
| `>=100%` | 超限 | 禁止直接请求模型，降级/分批/报错 | 显示错误或分批提示 |

token 估算优先级：

1. 使用模型调用返回的 `usage.prompt_tokens` 作为真实观测值，更新估算偏差。
2. LLM 调用前使用 runtime 内置 token estimator 估算。
3. provider 不支持 usage 时，使用近似估算器作为降级。

---

## 4. 三档压缩策略

### 4.1 70%：程序化压缩工具结果

触发条件：

```text
context_ratio >= 70%
```

目标：压缩旧的 `Observation.Content`，也就是工具返回结果。页面展示和数据库原始结果不变。

压缩方式：不调用大模型，使用程序化结构化压缩。

| 工具 | 压缩保留内容 |
| --- | --- |
| `QueryHistoricalLogs` | 时间范围、命中数量、关键时间线、异常日志 top N、与告警 PID/IP/命令行相关的日志 |
| `GetProcessTree` | PID/PPID 链、进程名、用户、命令行、可疑父子关系 |
| `GetNetworkConnections` | 远端 IP/端口、连接状态、进程关联、监听端口、异常外联统计 |
| `GetOpenFiles` | 敏感路径、可执行文件、配置文件、临时目录、删除文件、数量统计 |
| `GetRunningProcesses` | 告警相关进程、可疑进程、命令行、用户、父进程，普通进程聚合统计 |
| `GetUserSessions` | 用户、来源 IP、登录时间、TTY、活跃状态 |

发送给模型的压缩格式示例：

```text
Observation from QueryHistoricalLogs compressed:
- tool_call_id: tool-123
- original_result_tokens: 18000
- retained_evidence:
  1. 10:21:03 sshd spawned bash -i, pid=1234
  2. 10:21:07 bash connected to 1.2.3.4:4444
- omitted: 428 routine log lines
```

实现要求：

- 优先解析 JSON；解析失败时走文本规则压缩。
- 保留 `tool_call_id` 和原始 token 估算，便于追溯。
- 不改变 `ReactTurn.Observation.Content` 的原始持久化数据；只改变 prompt builder 使用的上下文视图。
- 如果压缩失败，回退到现有安全截断策略。

### 4.2 80%：压缩非当前步骤执行过程

触发条件：

```text
context_ratio >= 80%
```

目标：只保留当前步骤的完整 ReAct 对话，其他步骤只保留步骤结果。

保留：

```text
Current Step:
- system prompt
- 当前 step title / objective / expected output
- 用户原始输入
- 当前 step 内完整最近对话
- 当前 step 内工具 observation，必要时经过 70% 工具压缩
```

折叠历史步骤：

```text
Previous completed steps:
- step_id
- title
- status
- result
- evidence
- confidence
- error 摘要
- 关键工具调用摘要
```

示例：

```text
Previous completed steps:
- step_1 [completed]: 确认告警进程 bash 由 sshd 拉起。
  Evidence: pid=1234, ppid=889, commandline="bash -i ..."
- step_2 [completed]: 发现 bash 建立到 1.2.3.4:4444 的外联。
  Evidence: remote=1.2.3.4:4444, status=ESTABLISHED
```

实现要求：

- 不调用大模型，使用已有 `StepExecution.Result`、`Evidence`、`Error` 程序化折叠。
- 当前 step 内的 turns 仍可继续受 70% 工具结果压缩影响。
- 非当前步骤不再把每轮 action / observation / progress summary 塞进模型上下文。

### 4.3 95%：LLM 压缩 6 轮之前对话

触发条件：

```text
context_ratio >= 95%
```

目标：强制压缩当前步骤中 6 轮之前的所有 ReAct 对话，最近 6 轮保留原文。

定义：

```text
1 轮 = assistant action + tool observation + progress summary
```

保留原文：

- 用户原始输入
- system prompt / 输出格式要求
- 当前 step 信息
- 最近 6 个 ReAct turns
- 关键证据账本

调用大模型压缩：

- 当前 step 中第 1 到 N-6 轮对话
- 已压缩工具结果
- 历史 step summaries

LLM 压缩输出必须为结构化 JSON：

```json
{
  "summary": "到目前为止的调查摘要",
  "facts": [],
  "timeline": [],
  "evidence": [
    {
      "source": "tool_call_id",
      "fact": "bash 进程连接 1.2.3.4:4444",
      "confidence": "high"
    }
  ],
  "open_questions": [],
  "risks": [],
  "discarded_detail": "省略的重复或低价值内容说明"
}
```

压缩后下一轮模型上下文：

```text
Compressed prior turns summary:
<structured JSON summary>

Recent 6 turns:
<原始 action / observation / progress>
```

实现要求：

- 该压缩调用也必须经过 `LLMClient`，并记录 usage。
- 新增 `LLMPurpose` 建议为 `compress`，避免混用 `summarize`。
- 压缩失败时不发送超限请求，回退到 emergency deterministic compression。
- LLM 压缩摘要不得覆盖 UI 历史和 `TaskResult` 原始数据。

---

## 5. 初始上下文过大处理

在第一次 Plan 前执行 preflight budget check。

| 初始占用 | 策略 |
| --- | --- |
| `< 70%` | 正常开始 |
| `70%-95%` | 启动前程序化压缩事件扩展详情和历史经验，只保留核心字段 |
| `95%-100%` | 更激进预压缩：每条事件只保留核心卡片 + 关键证据 top N |
| `> 100%` | 不直接请求模型，优先分批；不能分批则返回可解释错误 |

### 5.1 批量事件 Map-Reduce

用户一次勾选大量事件导致初始上下文超过 256K 时，推荐分批：

```text
Batch 1: 分析事件 1-20 -> batch_summary
Batch 2: 分析事件 21-40 -> batch_summary
Batch 3: 分析事件 41-60 -> batch_summary
Final: 汇总所有 batch_summary + 用户输入 -> 最终结论
```

要求：

- 每个 batch 都保留用户输入原文。
- 每条事件核心字段保留。
- 事件扩展详情只在对应 batch 内展开。
- batch summary 必须保留可追溯的 alert_id 列表。

### 5.2 用户输入本身超限

如果：

```text
user_input_tokens + required_system_tokens + reserved_output_tokens > 256000
```

则无法合法压缩，应直接返回错误：

```text
用户输入超过模型上下文上限，请缩短输入或拆分任务。
```

---

## 6. 模块拆分

### 6.1 agent-runtime 负责的能力

所有上下文压缩核心能力放入 `/code/agent-runtime`。

建议新增包：

```text
/code/agent-runtime/contextbudget/
  estimator.go              # token 估算接口和默认估算器
  budget.go                 # 上下文预算计算
  policy.go                 # 70/80/95 触发策略
  compressor.go             # 压缩协调器
  tool_compressor.go        # 工具 Observation 程序化压缩
  step_folder.go            # 非当前步骤折叠
  llm_summarizer.go         # 95% LLM 压缩
  records.go                # CompressionRecord 结构
```

`agent-runtime` 需要新增或扩展：

- `RuntimeConfig`：增加 context budget 配置
- `ConfigPatch`：允许运行时调整压缩配置
- `LLMPurpose`：新增 `compress`
- `TaskContext`：记录压缩状态和压缩事件
- `TaskSnapshot` / `TaskResult`：暴露聚合 token 和压缩事件摘要
- `ModelCallRecord`：保留 prompt/completion/total token 明细
- prompt builder：发送 LLM 前通过 context budget layer 生成压缩后的 message view

### 6.2 Aegis 负责的能力

Aegis 只调用 `agent-runtime`，不实现压缩算法。

需要修改：

| 文件 | 责任 |
| --- | --- |
| `api-server/internal/llm/client.go` | 解析 provider usage，返回 prompt/completion/total token |
| `api-server/internal/llm/adapters/llm_client_adapter.go` | 将 usage 透传到 `agentruntime.LLMResponse.Usage` |
| `api-server/internal/llm/adapters/runtime_factory.go` | 设置 `RuntimeConfig` 的上下文预算和压缩阈值 |
| `api-server/internal/model/agent_execution.go` | 持久化 token 聚合和压缩事件摘要，必要时新增模型 |
| `api-server/internal/api/handler/ai_analysis_handler.go` | 返回执行详情时可展示压缩次数、token 消耗 |

禁止事项：

- 禁止在 Aegis adapter 内实现工具结果压缩算法。
- 禁止让前端参与模型上下文压缩。
- 禁止通过删除 UI 历史来降低模型上下文。

---

## 7. 数据结构建议

### 7.1 CompressionRecord

```go
type CompressionRecord struct {
    CompressionID string
    TaskID        string
    StepID        string
    Strategy      string // tool_results | historical_steps | llm_prior_turns | preflight | emergency
    TriggerRatio  float64
    BeforeTokens  int
    AfterTokens   int
    CompressedRef []string
    Summary       string
    ModelCallID   string
    CreatedAt     time.Time
}
```

### 7.2 ContextBudgetSnapshot

```go
type ContextBudgetSnapshot struct {
    MaxContextTokens     int
    ReservedOutputTokens int
    EstimatedPromptTokens int
    ContextRatio         float64
    PromptTokensObserved int
    CompletionTokens     int
    TotalTokens          int
    CompressionCount     int
}
```

### 7.3 LLMUsage 扩展

当前已有：

```go
type LLMUsage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

建议在 `ModelCallRecord` 中拆分保存：

```go
PromptTokens     int
CompletionTokens int
TotalTokens      int
```

保留 `TokensUsed` 作为兼容字段，值等于 `TotalTokens`。

---

## 8. 执行流程

### 8.1 ReAct 调用前

```text
build raw messages
-> estimate context ratio
-> if >=70% compress tool observations
-> estimate again
-> if >=80% fold historical steps
-> estimate again
-> if >=95% LLM summarize turns older than 6
-> estimate again
-> if still >100% emergency deterministic compression or return limited result
-> Complete(ctx, LLMRequest{Messages: compressedView})
```

### 8.2 Plan 调用前

```text
build plan messages
-> preflight check selected events + user input + prompt
-> if initial context >=70% compress event extended context
-> if initial context >100% split events into batches or return clear error
-> Complete(ctx, PurposePlan)
```

### 8.3 Summarize 调用前

最终总结应优先使用：

- 用户输入
- 每个 step 的结果和 evidence
- 压缩摘要
- 关键错误和未完成项

不要求回放完整旧 turns。

---

## 9. 前端页面 UI 设计

前端只展示上下文预算、token 消耗和压缩提示，不参与压缩决策，不向后端回传压缩后的上下文。

### 9.1 展示位置

建议在 `frontend/src/views/detection/AIAnalysis.vue` 增加三个展示区域：

| 区域 | 位置 | 用途 |
| --- | --- | --- |
| 上下文圆形指示器 | AI 分析输入区、“开始 AI 分析”按钮附近或会话标题右侧 | 开始前和运行中展示上下文占用 |
| 运行中压缩提示 | 消息流顶部或当前步骤状态区域 | 运行时提示已发生压缩 |
| 执行详情抽屉 | 历史会话或任务详情 | 展示 token 与压缩事件明细 |

### 9.2 上下文圆形指示器

上下文大小不使用长条作为主视觉，使用类似 Codex 的动态圆形上下文指示器。

默认收起态：

- 显示一个圆环。
- 圆环进度表示 `context_ratio`。
- 圆环中间显示简短百分比，例如 `55%`；空间不足时只显示圆环。
- 圆环颜色随占用动态变化。
- 圆环旁可选显示简短标签，例如 `上下文`。

选中/点击/悬浮展开态展示：

```text
上下文 141K / 256K tokens · 55%
预留输出 8K tokens
已压缩 2 次，节省约 82K tokens
```

字段来源：

| UI 字段 | 后端字段 |
| --- | --- |
| 当前估算 tokens | `ContextBudgetSnapshot.EstimatedPromptTokens + ReservedOutputTokens` |
| 最大上下文 | `ContextBudgetSnapshot.MaxContextTokens`，默认 `256000` |
| 占用比例 | `ContextBudgetSnapshot.ContextRatio` |
| 预留输出 | `ContextBudgetSnapshot.ReservedOutputTokens` |

视觉状态：

| 比例 | 颜色 | 文案 |
| --- | --- | --- |
| `<70%` | 绿色/中性 | `上下文充足` |
| `70%-80%` | 蓝色/提示 | `将压缩工具结果` 或 `已压缩工具结果` |
| `80%-95%` | 黄色/警告 | `将折叠历史步骤` 或 `已折叠历史步骤` |
| `95%-100%` | 橙色/高风险 | `将生成上下文摘要` 或 `已生成上下文摘要` |
| `>=100%` | 红色/错误 | `上下文超限，请减少事件或缩小时间范围` |

交互要求：

- 圆环必须根据 `context_ratio` 动态更新。
- 点击圆环后显示 popover/dropdown 详情。
- 再次点击或点击外部区域关闭详情。
- 运行中收到 `context_budget_checked` 或 `context_compressed` 事件时更新圆环。
- 圆环只展示模型上下文状态，不代表页面历史大小。
- 移动端点击展开，桌面端支持 hover 和 click。

展开详情建议布局：

```text
上下文
55%

141K / 256K tokens
预留输出: 8K
当前阶段: 工具结果已压缩
压缩次数: 2
节省: 82K tokens

压缩只影响发送给模型的上下文，不会删除页面对话和历史记录。
```

前端样式建议：

- 圆环直径桌面端 `32-40px`，移动端 `28-36px`。
- 使用 SVG circle 或 Element Plus progress circle；不要使用纯文字 badge 替代。
- 圆环动画应平滑，但不需要复杂动效。
- 颜色不要只依赖颜色表达状态，展开详情中必须显示文字状态。
- 在 `>=95%` 时可轻微强调，但不要闪烁。

### 9.3 压缩提示文案

运行中发生压缩时，通过 SSE 或执行结果摘要展示轻量提示。提示不应插入为用户对话，不影响历史消息内容。

建议文案：

| 压缩策略 | UI 提示 |
| --- | --- |
| `tool_results` | `上下文接近上限，已压缩旧工具结果。页面历史不受影响。` |
| `historical_steps` | `上下文较大，已将非当前步骤折叠为步骤结果。` |
| `llm_prior_turns` | `上下文接近 256K 上限，已生成早期对话摘要，并保留最近 6 轮原文。` |
| `preflight` | `所选事件较多，已在分析前压缩事件扩展详情，核心字段已保留。` |
| `batching` | `所选事件超过单次 256K 上限，将分批分析后汇总。` |
| `over_limit` | `上下文超过 256K 上限，请减少勾选事件或缩小时间范围。` |

页面提示必须说明：

```text
压缩只影响发送给模型的上下文，不会删除页面对话和历史记录。
```

### 9.4 执行详情抽屉

执行详情中增加“上下文与 token”分组：

```text
最大上下文: 256K tokens
预留输出: 8K tokens
峰值上下文: 238K / 256K (93%)
总输入 tokens: 412K
总输出 tokens: 28K
总 tokens: 440K
上下文压缩: 3 次
累计节省: 96K tokens
```

压缩事件表：

| 时间 | 策略 | 触发比例 | 压缩前 | 压缩后 | 说明 |
| --- | --- | --- | --- | --- | --- |
| 10:21:03 | 工具结果压缩 | 72% | 184K | 141K | 压缩 2 个工具结果 |
| 10:25:10 | 历史步骤折叠 | 83% | 212K | 156K | 折叠 step_1、step_2 |
| 10:31:45 | LLM 摘要 | 96% | 246K | 153K | 压缩 6 轮之前对话 |

### 9.5 初始超限提示

用户勾选事件后，如果预估上下文过大：

| 情况 | UI 行为 |
| --- | --- |
| `70%-95%` | 允许开始，提示将自动压缩扩展详情 |
| `95%-100%` | 允许开始，强提示将只保留事件核心字段和关键证据 |
| `>100%` 且可分批 | 显示“将分批分析”提示 |
| `>100%` 且不可分批 | 禁用开始按钮，提示减少事件或缩小时间范围 |
| 用户输入本身超限 | 禁用开始按钮，提示缩短输入或拆分任务 |

按钮旁提示示例：

```text
已选择 86 个事件，预计上下文超过 256K。系统将分 5 批分析并汇总结果。
```

```text
用户输入过长，已超过 256K 上限。请缩短输入或拆分任务。
```

### 9.6 API/SSE 字段建议

新增或扩展后端响应字段：

```json
{
  "context_budget": {
    "max_context_tokens": 256000,
    "reserved_output_tokens": 8192,
    "estimated_prompt_tokens": 141000,
    "context_ratio": 0.55,
    "compression_count": 2
  },
  "compression_events": [
    {
      "strategy": "tool_results",
      "trigger_ratio": 0.72,
      "before_tokens": 184000,
      "after_tokens": 141000,
      "summary": "压缩 2 个工具结果"
    }
  ]
}
```

SSE 可新增轻量事件：

```json
{
  "type": "context_compressed",
  "strategy": "tool_results",
  "message": "上下文接近上限，已压缩旧工具结果。页面历史不受影响。",
  "before_tokens": 184000,
  "after_tokens": 141000
}
```

前端收到后只显示提示和更新圆形上下文指示器，不修改已有消息内容。

---

## 10. 可观测性

新增 HookEvent 建议：

| HookEventType | 说明 |
| --- | --- |
| `context_budget_checked` | 上下文预算检查完成 |
| `context_compressed` | 发生一次压缩 |
| `context_compression_failed` | 压缩失败并降级 |

Hook payload 示例：

```json
{
  "strategy": "tool_results",
  "trigger_ratio": 0.72,
  "before_tokens": 184000,
  "after_tokens": 141000,
  "compressed_refs": ["tool-1", "tool-2"]
}
```

前端默认只展示轻量压缩提示，可在执行详情或调试面板展示：

```text
上下文压缩 2 次，累计节省约 82000 tokens
```

---

## 11. 测试要求

### 11.1 agent-runtime 单元测试

- token estimator 基础估算
- 70% 工具结果压缩触发
- 80% 历史步骤折叠触发
- 95% 保留最近 6 turns，压缩更早 turns
- 用户输入永不压缩
- 事件核心字段永不压缩
- 初始上下文超过 256K 时不发送 LLM 请求
- LLM 压缩失败时 emergency fallback 生效
- 压缩记录写入 `TaskResult`

### 11.2 Aegis 单元测试

- OpenAI-compatible usage 解析
- Anthropic-compatible usage 解析
- `LLMClientAdapter` 透传 `LLMUsage`
- `RuntimeFactory` 设置 context budget 配置
- AI 分析历史不因上下文压缩缺失 UI 对话
- 上下文圆形指示器展示 256K 上限
- 上下文圆形指示器根据占用比例动态变化
- 点击/选中圆形指示器后展示 tokens、比例、预留输出、压缩次数和提示文案
- 70/80/95/超限状态文案正确

### 11.3 集成验证

- 选中大量告警事件，触发 preflight 压缩
- 构造大日志返回，触发 70% 工具压缩
- 多步骤任务触发 80% 步骤折叠
- 当前步骤超过 6 轮且接近 95%，触发 LLM 摘要
- 页面 Observation、工具调用历史、最终结论仍正常显示
- 前端提示“压缩只影响发送给模型的上下文，不会删除页面对话和历史记录”

---

## 12. 风险与降级

| 风险 | 影响 | 降级 |
| --- | --- | --- |
| token 估算不准 | 压缩触发偏早或偏晚 | 使用真实 usage 校准估算器 |
| 工具结果压缩丢关键证据 | 分析质量下降 | 保留核心字段、top N、证据引用和原始 tool_call_id |
| LLM 压缩失败 | 无法降低 95% 上下文 | emergency deterministic compression |
| 初始用户输入超限 | 无法调用模型 | 返回明确错误，不压缩用户输入 |
| 批量事件过多 | 单次 plan 超窗 | Map-Reduce 分批分析 |
| Aegis 侧误实现压缩 | 边界混乱 | 压缩算法必须在 `/code/agent-runtime` |

---

## 13. 实施顺序

1. 在 Aegis `llm/client.go` 补齐 usage 解析和返回结构。
2. 在 `LLMClientAdapter` 透传 usage 到 `agent-runtime`。
3. 在 `/code/agent-runtime` 增加 context budget 配置、估算器、压缩记录。
4. 实现 70% 工具结果程序化压缩。
5. 实现 80% 非当前步骤折叠。
6. 实现 95% LLM 压缩 6 轮之前对话。
7. 实现 preflight 初始上下文检查和批量事件降级策略。
8. Aegis `RuntimeFactory` 设置默认 256K 和阈值。
9. 持久化 token 聚合和压缩事件摘要。
10. 前端增加上下文圆形指示器、压缩提示、执行详情 token/压缩事件展示。
11. 补齐单元测试和集成验证。

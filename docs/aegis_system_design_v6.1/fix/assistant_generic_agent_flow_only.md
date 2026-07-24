# 智能体模式统一通用流程改造

> **工具选举设计已废弃（2026-07-24）**：本文中“动态规划模式”、
> `InitialPlan=nil`、`planning_mode=agent_runtime_dynamic` 以及 Runtime 可自由选择
> `tool_name` 的内容不得再作为实现依据。V6.1 的强制架构边界以
> `assistant_workflow_knowledge_high_level_tools_and_deterministic_execution_design_v6.1.md`
> 和 `assistant_baseline_tool_contract_and_full_access_fix.md` 为准：所有工具必须由
> capability Mapping 选举，Mapping 结果必须作为不可变 initial plan，Runtime
> 只能执行绑定步骤。该边界是代码级 fail-closed 不变量，不得恢复动态工具选举。
>
> 2026-07-10 补充：工具授权、异步结果、步骤完成和最终证据的后续修正见
> `assistant_mapping_authorization_and_truthful_completion_fix.md`。相关定义冲突时，
> 以后者为准。

## 1. 问题与目标

当前智能体链路虽然接入了大模型和 `agent-runtime`，但 Aegis 业务层仍会根据
资产采集、漏洞扫描、CVE 修复、告警阻断等已知场景扩展工具、绑定参数并生成固定
执行步骤。这样会产生两个规划器：

1. Aegis 业务层按预置场景生成 `ToolExecutionPlan`；
2. `agent-runtime` 再根据用户目标执行或修正计划。

双规划器会导致新增场景必须继续写规则、固定计划重复步骤、工具参数与真实返回值
不一致，以及模型无法根据执行结果选择条件分支。用户给出的 CVE 请求只是暴露该
架构问题的一个例子，修复不能以 CVE 为中心。

本次目标是实现纯智能体模式：任意用户请求都由大模型理解和拆解，唯一的执行规划器
是 `agent-runtime`。Aegis 业务层只负责提供上下文、动态工具目录、工具授权、安全
审批、调用隔离和证据记录，不再生成任何业务场景的执行流程。

## 2. 目标数据流

```text
User Request
  -> LLM IntentRouter（通用意图）
  -> LLM IntentDecomposer（从实时英文目录选择 exact capability）
  -> Capability Mapper（exact capability -> enabled tools）
  -> ToolAuthorizationEngine（只做授权和风险硬门）
  -> RuntimeFactory（动态规划模式）
  -> agent-runtime Planner / ReAct / Correct / Reflect
  -> ToolGateway（一次只执行 Runtime 明确请求的一个工具）
  -> Evidence / Final Answer
```

关键约束：

- Aegis 不把工具授权结果转换为 `agent-runtime.InitialPlan`。
- ToolDecisionEngine 不扩展、排序、重复或条件编排工具，不按工具名绑定业务参数。
- IntentDecomposer 使用开放对象、范围和参数结构，不包含 CVE、主机、基线等固定枚举。
- 不存在独立工具名选择器或工具评分器；模型只能输出目录中的英文 capability。
- Capability Mapper 只做 exact mapping，并可加入工具声明的只读状态/发现配套能力。
- Planner 根据工具 schema、说明、前后置语义和每次真实返回值决定顺序、参数、
  重复调用、轮询、跳过、重试和结束。
- ToolGateway 不暗中补跑工具。领域接口必要的参数校验、幂等、状态机校验和安全
  保护仍留在工具适配层，它们是执行边界，不是业务规划器。

## 3. 通用中间契约

### 3.1 意图拆解

`IntentBreakdown` 只表达通用信息：

- `goal`：用户最终目标；
- `domains`、`actions`：开放字符串数组；
- `objects`：开放类型的对象和可选 ID；
- `scope`：开放的范围类型和对象 ID；
- `parameters`：任意 JSON 参数，不限定为某种业务；
- `constraints`、`missing_info`；
- `requires_write`、`risk_hint`；
- `candidate_capabilities`；
- `need_clarification`、`clarifying_question`。

业务层只校验 JSON 结构、安全必需字段和一致性，不校验 CVE 脚本类型、固定主机范围
或某个工作流必须包含哪些能力。

### 3.2 工具授权

ToolDecisionEngine 的输出是授权审计结果，不是执行计划。为保持当前接口兼容，
结构暂时仍使用 `ToolExecutionPlan`，但其中每个 step 仅代表一个被授权的工具：

- 候选工具只来自 exact capability mapping、用户明确指定的唯一工具，以及工具声明
  的只读 completion/discovery 配套 capability；
- 不做关键词、领域、对象、上下文相关度召回和分数阈值；
- 不自动增加写前置、业务后续或场景工作流工具；
- 不决定执行顺序；
- 不为同一工具复制多个业务步骤；
- 不使用场景规则生成参数；
- 仅记录从页面上下文或通用对象契约中可以确定的参数来源。

后续接口演进可将该结构重命名为 `ToolAuthorization`，但运行时行为不得依赖旧名称。

### 3.3 动态执行计划

`agent-runtime` 收到：

- 原始用户消息；
- 页面/会话上下文；
- 被授权工具的实时 descriptor；
- `InitialPlan = nil`。

Planner 是唯一执行计划来源。它可根据任务复杂度生成任意数量和顺序的步骤，并由
ReAct 在真实结果后继续选工具或修正计划。工具目录未提供的能力必须作为证据缺口
报告，不得发明工具。

## 4. 安全与可观测性

- 写操作仍要求用户原文存在明确写意图，并按工具风险触发审批。
- 禁用、未注册、未授权工具在 ToolGateway 和 Dispatcher 双重拒绝。
- 必填参数如果可由前一步结果获得，不在授权阶段追问；由 Runtime 在执行阶段绑定。
- 只有缺失信息会改变目标或导致写操作对象不明确时才向用户追问。
- 日志记录 `planning_mode=agent_runtime_dynamic`、授权工具、拒绝工具、决策 trace、
  Runtime 最终计划和工具证据。
- 不记录密钥、脚本正文或其他敏感参数。

## 5. 模型提示词与能力标识语言

所有由 Aegis 或 `agent-runtime` 生成并发送给大模型的静态指令、字段说明、目录标签、
纠错信息和输出示例统一使用英文。用户原文、上传内容、页面上下文、历史工具结果等
动态业务数据保持原始语言；模型面向用户的最终回答应跟随用户语言。

`candidate_capabilities` 是机器契约，不是自然语言摘要，必须满足：

- 只包含小写英文字母、数字、下划线、连字符或点；
- 首字符必须是英文字母；
- 优先直接使用实时工具目录声明的 capability，不翻译为中文；
- 不得写工具名、中文能力描述或自由文本；
- 服务端对格式进行确定性校验，首次不合规时要求模型纠正，纠正后仍不合规则终止
  当前意图拆解，不得把无效 capability 交给 mapping。

工具的中文 UI 描述不直接拼入模型提示词。模型侧工具目录根据英文
`capability/domain/operation/object_types`、参数名、类型、枚举和授权契约生成，避免
界面语言影响 capability mapping。

## 6. 测试用例

- 中文用户请求可以产生英文 `candidate_capabilities`，例如
  `generate_vulnerability_script`。
- `candidate_capabilities=["生成漏洞脚本"]` 必须触发契约纠正；纠正后仍为中文时返回
  契约错误。
- capability 中包含空格、中文、emoji 或以数字开头时必须拒绝。
- Assistant 的 intent、tool selection、plan、ReAct、summarize 提示词静态部分均为英文。
- `agent-runtime` 的 assess、route、plan、ReAct、correct、reflect、audit、summarize
  提示词静态部分均为英文。
- 工具拥有中文 UI 描述和中文参数说明时，模型侧工具目录仍只包含英文契约元数据。

1. IntentDecomposer 接受任意 `scope.kind` 和任意 `parameters`，核心提示词不出现
   CVE 或命名工具工作流。
2. LLM 只选择一个工具时，ToolDecisionEngine 只授权这一个工具，不补充任何
   前置、状态或分析工具。
3. 任意工具的必填参数声明可从 `previous_step` 获取时，授权阶段不追问具体 ID。
4. 工具授权结果不会写入 `agent-runtime.InitialPlan`，Runtime 使用动态规划配置。
5. 同一工具需要按不同参数多次调用时，由 Planner 生成唯一标题的步骤；Aegis
   授权层不复制步骤，因此不会产生固定计划重复标题。
6. 写操作缺少明确写意图、工具被禁用、工具未注册或未授权时仍被硬门拒绝。
7. 工具调用失败、结果为空或状态未终止时，Runtime 能基于真实结果重试、改参、
   选择已授权替代工具或报告证据缺口。
8. 当 LLM 首次预选了错误工具时，通用契约召回可将结构化意图匹配的正确工具加入
   授权候选，且不生成执行步骤；Runtime 最终只调用完成目标所需的工具。
9. Assistant 包测试、api-server 构建、服务健康检查和真实 API 冒烟通过。

## 6. 回滚策略

- 本次不删除 `agent-runtime` 的 caller-supplied plan 兼容能力，只让 Assistant
  纯智能体入口不再使用它；其他调用方可独立迁移。
- 如果动态规划出现运行问题，可回滚 Aegis 提交恢复旧行为，无需回滚数据库。
- 回滚不得重新启用 ToolGateway 的隐式多工具补跑。

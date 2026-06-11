# Assistant 精确问答与执行计划对齐修复

## 问题描述

智能体模式存在三类偏差：

1. 简单问题没有稳定走“简单回答”，复杂问题也没有稳定呈现分计划执行。
2. 复杂任务的执行计划、工具调用状态与 AI 分析页面的展示效果不一致。
3. 用户输入后，计划和工具调用 SSE 事件有时无法挂到最终助手消息上，导致 UI 看起来没有执行计划或状态不更新。

## 根因分析

1. `IntentRouter.Classify` 主要按空格分词判断复杂度。中文连续句子如“分析最近的安全事件并给出修复建议”可能只被识别为一个词，复杂任务不能稳定进入更精细的 LLM 意图分类。
2. `EventPlan`、`step_started`、`tool_call` 等 SSE 事件没有携带本次运行的助手消息 `message_id`。前端 store 依赖 `event.message_id` 挂载计划，因此计划事件先到时会找不到消息。
3. `RuntimeFactory.Build` 使用用户消息 ID 作为工具调用归属，最终回答又保存为 `msg_<run_id>`，导致计划、工具调用和最终回答不在同一条消息上。
4. 智能体会话手写了一套执行计划 UI，没有复用 AI 分析的 `ExecutionPlan.vue` 和 `normalizePlanEvent`，状态映射与视觉表现不一致。
5. ReAct prompt 中“直接回复”的 JSON 示例缺少闭合括号，会诱导模型输出不可解析格式，影响回答精确性。

## 修复设计

1. 后端统一将一次运行的计划、步骤、工具调用、审批和最终回答归属到 `msg_<run_id>`。
2. SSE 事件补充顶层 `message_id`，前端可将实时事件挂到同一条助手消息。
3. 前端 store 在计划事件先到时创建空助手消息占位，最终 `message_delta` 再补齐内容。
4. 智能体对话区和右侧栏复用 AI 分析的 `ExecutionPlan.vue`，并通过 `normalizePlanEvent` 统一计划数据。
5. 意图路由增加中文连续句复杂度估算；低置信或分析/调查类请求调用 LLM 意图分类，简单高置信查询保留规则路径。
6. 修正 ReAct prompt 的 JSON 示例，并明确简单回答、简单查询、复杂研判的输出规则。

## 修改文件

- `api-server/internal/assistant/event.go`
- `api-server/internal/assistant/adapter_hook_sink.go`
- `api-server/internal/assistant/runtime_factory.go`
- `api-server/internal/assistant/orchestrator.go`
- `api-server/internal/assistant/intent_router.go`
- `api-server/internal/assistant/adapter_prompt_provider.go`
- `frontend/src/store/assistant.ts`
- `frontend/src/views/assistant/AssistantWorkspace.vue`
- `frontend/src/views/assistant/components/AssistantContextRail.vue`
- `frontend/src/views/assistant/components/AssistantConversation.vue`
- `frontend/src/components/ExecutionPlan.vue`
- `frontend/src/utils/aiAnalysisRuntime.ts`
- `frontend/src/api/aiAnalysis.ts`

## 验证用例

1. 发送“你好”：应直接简短回复，不生成执行计划。
2. 发送“主机列表”：应只调用必要查询工具并直接给出数据。
3. 发送“分析最近的安全事件并给出修复建议”：应显示执行计划，步骤状态随 SSE 更新，效果与 AI 分析页面一致。
4. 工具调用开始后应显示 running，工具结果返回后应更新为 completed 或 failed。
5. 审批中的步骤应显示“待审批”。

## 回滚计划

如出现运行异常，可回滚本修复涉及文件；数据库结构和接口路径无变更，不需要迁移回滚。

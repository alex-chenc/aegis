# 智能体安全分析深度与工具状态修复

## Bug description and symptoms

- 主机安全性/安全事件分析只查询主机基础信息，未系统读取基线、漏洞、告警和在线 Agent 取证数据。
- 工具调用在前端实时流中可能一直显示“执行中”，无法切换为完成态。
- 对话区重复展示计划和工具调用，右侧栏已有相同信息，页面噪音较大。
- 思考步骤没有像 AI 分析模块一样集中展示和折叠。

## Reproduction steps

1. 在智能体模式输入“帮我排查一下 192.168.152.159 这个机器上面有哪些安全问题”。
2. 查看最新会话落库记录，只有 `Host.List` 等基础工具调用。
3. 实时观察前端 SSE 工具调用状态，部分情况下开始事件和完成事件无法合并。
4. 查看对话区和右侧栏，计划、工具调用重复显示。

## Root cause analysis

- 安全分析提示词偏通用，虽然有安全流程片段，但没有明确要求主机安全分析必须覆盖主机画像、基线任务、漏洞、告警、Agent 进程/网络/日志证据。
- 复杂任务工具扩展只包含告警和 Agent 取证工具，缺少 `Task.List`、`Task.GetDetail`、`Vulnerability.List`、`Vulnerability.AffectedHosts`、`Software.Installed.Search` 等证据源。
- `AssistantToolGatewayAdapter` 发布开始事件时使用 agent-runtime 的 `req.CallID`，完成/失败事件使用 dispatcher 生成的 `call_xxx`，前端按 `call_id` 合并时无法更新同一条记录。
- 前端 `thinking` 事件未写入消息模型，对话区无法展示完整思考轨迹。

## Fix design

- 让 dispatcher 支持外部传入 call ID，工具开始、完成、失败事件统一使用同一个 `call_id`。
- 扩展复杂安全/主机分析工具集，覆盖主机、基线任务、漏洞、告警、Agent 在线取证和阻断/规则只读信息。
- 更新提示词：复杂安全分析必须先完成“数据覆盖检查”，没有基线/漏洞/告警/Agent 数据时明确说明证据不足；需要执行检查任务时先走审批。
- 前端对 `thinking` 事件聚合到当前助手消息，展示为可折叠“思考步骤”。
- 对话区移除计划、工具调用和审批卡片，保留右侧栏作为执行状态面板。
- 使用 GSAP 的 Vue 生命周期和 scoped context 做消息/思考块入场动效，并支持 reduced-motion。

## Verification steps

- `go test ./internal/assistant`
- `npm run test -- AssistantWorkspace.test.ts --run`
- `npm run build`
- `docker compose up -d --build api-server frontend`
- 通过登录后新建智能体会话，确认工具状态能从运行中变为成功/失败，安全分析计划覆盖基线、漏洞、告警、Agent 取证。

## Affected components

- `api-server/internal/assistant`
- `frontend/src/store/assistant.ts`
- `frontend/src/views/assistant/components/AssistantConversation.vue`

## Risk and rollback plan

- 风险：复杂任务可用工具增加后，LLM 可能执行更多只读工具，耗时略有增加。
- 回滚：还原本修复涉及文件，前端恢复对话区计划/工具卡片展示，后端恢复原工具 ID 生成逻辑。

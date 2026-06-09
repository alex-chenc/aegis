package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// AssistantPromptProvider 适配 agent-runtime PromptProvider 接口
// 为智能助手生成特定的 Plan/React/Summarize 提示词
type AssistantPromptProvider struct {
	toolDescriptors    []agentruntime.ToolDescriptor
	contextRefs        []ContextRefResult
	taskType           string
	userMessage        string
	reflectionMemories []string
}

// NewAssistantPromptProvider 创建提示词提供者
func NewAssistantPromptProvider(
	toolDescriptors []agentruntime.ToolDescriptor,
	contextRefs []ContextRefResult,
	taskType string,
	userMessage string,
) *AssistantPromptProvider {
	return &AssistantPromptProvider{
		toolDescriptors: toolDescriptors,
		contextRefs:     contextRefs,
		taskType:        taskType,
		userMessage:     userMessage,
	}
}

func (p *AssistantPromptProvider) WithReflectionMemories(memories []string) *AssistantPromptProvider {
	p.reflectionMemories = memories
	return p
}

// Build 实现 agentruntime.PromptProvider 接口
func (p *AssistantPromptProvider) Build(ctx context.Context, req agentruntime.PromptRequest) (agentruntime.PromptBundle, error) {
	switch req.Purpose {
	case agentruntime.PurposePlan:
		return p.buildPlanPrompt(), nil
	case agentruntime.PurposeReact:
		return p.buildReactPrompt(), nil
	case agentruntime.PurposeSummarize:
		return p.buildSummarizePrompt(), nil
	case agentruntime.PurposeAudit, agentruntime.PurposeReflect, agentruntime.PurposeCorrect:
		// 返回空 PromptBundle，使用 agent-runtime 内置默认
		return agentruntime.PromptBundle{}, nil
	default:
		return agentruntime.PromptBundle{}, nil
	}
}

// buildPlanPrompt 构建计划阶段提示词
func (p *AssistantPromptProvider) buildPlanPrompt() agentruntime.PromptBundle {
	toolList := p.formatToolList()
	reflectionGuide := p.formatReflectionGuide()
	hostSecurityGuide := hostSecurityAnalysisGuide()
	contextBlock := p.formatContextRefs()
	sequenceGuide := p.formatMandatoryToolSequenceGuide()
	naturalOperationGuide := p.formatNaturalOperationGuide()

	systemPrompt := fmt.Sprintf(`你是 Aegis 智能安全助手，专注于主机安全分析和运维操作。

## 你的能力
- 查询和分析主机资产、安全态势
- 分析告警、追溯攻击路径
- 管理基线检查、漏洞扫描
- 管理检测包、Sigma 规则
- 执行阻断策略
- 主机攻击研判

## 可用工具
%s

%s

%s

%s

%s

%s

## 规则
1. 所有操作必须通过工具调用完成，不能直接执行命令
2. 高风险操作需要用户审批
3. 所有结论必须基于数据和证据
4. 不确定时明确说明，不编造信息
5. 简单查询最多 1-2 个步骤；只有跨主机、跨数据源、安全研判、修复建议等复杂任务才拆成 3 个及以上步骤
6. 主机安全性、安全事件、入侵排查、风险研判属于复杂任务，计划必须覆盖：范围确认、主机画像、平台侧风险、运行态取证、关联研判、结论建议
7. 计划中只能写“可用工具”里存在的工具名；如果关键工具不可用，把该项写入证据缺口，不得发明工具名
8. 计划最多 8 个步骤。主机安全分析建议合并为 5-6 步：定位范围、收集平台证据、逐台运行态取证、关联风险、输出处置优先级
9. 当用户要求“全部主机、所有主机、整体平台、在线主机、每台主机”分析时，计划必须覆盖完整目标集合：先用 Host.List（page_size/limit=100，按需 status=online）获取目标主机列表，再按每台主机逐台分析，最后输出整体汇总；不得只挑第一台或单台代表主机下结论
10. 批量主机计划的步骤标题应体现“逐台”或“全部主机”，例如“定位全部目标主机”“逐台收集主机证据”“逐台 Agent 取证”“逐台结论与整体汇总”
11. 计划步骤标题要短，只写操作者能看懂的动作标题；详细目标、工具和证据要求放在执行过程内部，不依赖计划栏展示
12. 不要把“汇总分析结果/输出最终结论”规划成需要再次调用工具的独立步骤；最终回答由系统总结阶段生成。若确需汇总步骤，只能整合已有证据，不得重复查询已成功获取的数据

## 输出要求
⚠️ 严格要求：只输出一个JSON对象，不要输出任何其他文本、解释、问候或markdown格式。直接以 { 开头，以 } 结尾。

JSON格式如下：
{"goal":"任务目标描述","assumptions":["假设1","假设2"],"steps":[{"step_id":"step_1","title":"步骤标题","objective":"步骤目标","expected_output":"预期输出","suggested_tools":["ToolName1","ToolName2"]}]}`, toolList, reflectionGuide, hostSecurityGuide, contextBlock, sequenceGuide, naturalOperationGuide)

	userPrompt := p.userMessage
	if contextBlock != "" {
		userPrompt += "\n\n" + contextBlock
	}

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
		Messages: []agentruntime.LLMMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
}

// buildReactPrompt 构建 ReAct 阶段提示词
func (p *AssistantPromptProvider) buildReactPrompt() agentruntime.PromptBundle {
	toolList := p.formatToolListDetail()
	reflectionGuide := p.formatReflectionGuide()
	hostSecurityGuide := hostSecurityAnalysisGuide()
	contextBlock := p.formatContextRefs()
	sequenceGuide := p.formatMandatoryToolSequenceGuide()
	naturalOperationGuide := p.formatNaturalOperationGuide()

	systemPrompt := fmt.Sprintf(`你是 Aegis 智能安全助手，正在执行安全分析任务。

## 可用工具（必须严格使用以下工具名，不得发明新工具名）
%s

%s

%s

%s

%s

%s

## ⚠️ 严格输出格式要求（必须完全遵守，不得使用其他格式）
你的输出必须是一个JSON对象，包含 "action" 字段。不要输出任何非JSON内容。

### 直接回复（问候、简单问题、不需要工具的场景）：
{"action":"step_result","summary":"直接回复","step_result":{"result":"你的回复内容","evidence":[],"confidence":"high"}}

示例：
用户说"你好" → {"action":"step_result","summary":"问候回复","step_result":{"result":"你好！我是 Aegis 智能安全助手，专注于主机安全分析。有什么可以帮您的吗？","evidence":[],"confidence":"high"}}

### 调用工具（需要查询或操作数据时）：
{"action":"tool_call","summary":"调用目的简述","tool_call":{"tool_name":"上面列出的工具名之一","reason":"调用原因","args":{"参数名":"参数值"}}}

调用工具示例：
{"action":"tool_call","summary":"查询主机列表","tool_call":{"tool_name":"Host.List","reason":"需要获取所有主机信息","args":{"page":1,"page_size":20}}}

同时调用多个工具的示例（每个工具调用单独一个JSON，按顺序输出）：
{"action":"tool_call","summary":"查询主机列表","tool_call":{"tool_name":"Host.List","reason":"获取所有主机信息","args":{"page":1,"page_size":20}}}
{"action":"tool_call","summary":"查询 Agent 在线状态","tool_call":{"tool_name":"Host.AgentStatus.Get","reason":"获取 Agent 在线状态","args":{}}}

### 完成当前步骤（工具调用完成后）：
{"action":"step_result","summary":"完成总结","step_result":{"result":"步骤结果","evidence":["证据1","证据2"],"confidence":"high/medium/low"}}

### 无法继续：
{"action":"fail_step","summary":"失败总结","failure":{"reason":"失败原因","recoverable":true}}

## 判断规则
- 问候、闲聊、能力说明、概念解释等简单问题 → 直接回复，使用 step_result，回答要短，不要生成计划
- 简单数据查询 → 只调用必要工具，拿到数据后直接给结果，不要写分析报告
- 复杂问题（安全研判、攻击溯源、跨数据源调查、修复方案）→ 按计划逐步执行，每一步都基于工具结果
- 主机安全分析/安全事件分析 → 必须先定位主机，再读取平台侧证据，再在 Agent 在线时调用运行态取证工具，最后才能下结论
- 全部主机/所有主机/整体平台/在线主机分析 → Host.List 返回 N 台目标主机后，必须逐台覆盖这 N 台主机；平台侧工具可一次传 host_ids 批量查询，Host.Get 和 Agent.* 取证必须按 host_id 逐台执行或明确记录该主机未覆盖原因
- 批量主机分析中，不能只分析第一台主机、最新主机或告警最多的主机；如果只完成部分主机，最终必须列出“未完成逐台分析的主机”和原因
- Agent 不在线、工具失败或数据为空 → 明确记录为证据缺口，并给出保守结论
- 无数据支撑时明确说明“当前数据不足”，不要猜测具体主机、告警或结论
- 如果工具调用报错，先参考“历史反思”修正参数或替代工具后重试一次；同一工具再次失败时跳过该工具，把失败写入证据缺口，不要无限重试
- 调用工具前先检查当前步骤和历史步骤已获得的证据；同一工具用同一参数已成功时，直接复用已有结果，不要重复调用
- 当前步骤如果是“汇总、总结、输出结论、整理结果”，只能基于已有工具结果输出 step_result，不要再调用工具
- 中间步骤的 step_result 只写该步骤产物，不要提前输出完整最终报告；最终结论留给总结阶段一次性生成

## 禁止事项
- 禁止在需要调用工具时输出自然语言（如"我来帮您查询..."），必须直接输出JSON
- 禁止输出 {"name":"...","arguments":...} 格式（这是错误格式）
- 禁止输出 markdown 代码块（不要用三个反引号包裹）
- 禁止在JSON前后输出多余文字
- 必须使用 "action" 字段，不要使用 "name" 或 "type" 字段`, toolList, reflectionGuide, hostSecurityGuide, contextBlock, sequenceGuide, naturalOperationGuide)

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}
}

// buildSummarizePrompt 构建总结阶段提示词
func (p *AssistantPromptProvider) buildSummarizePrompt() agentruntime.PromptBundle {
	// 根据任务类型选择不同的总结格式
	systemPrompt := fmt.Sprintf(`你是 Aegis 智能安全助手。根据收集到的信息回复用户。

%s

## 回复规则

### 数据查询类（主机查询、资产查看、列表查询等）
直接告诉用户查询到了什么数据，简洁明了，不需要分析报告格式。
示例：
"共查询到 3 台主机：\n1. 192.168.1.10 (hostname-a) - 在线\n2. ..."

### 分析类（安全分析、攻击调查、漏洞评估等）
使用“结论先行 + 证据支撑 + 缺口说明”的结构化格式：
1. 安全结论：是否存在安全问题、最高风险等级、最需要立即处理的问题
2. 覆盖范围：目标主机、已覆盖数据源、未覆盖原因
3. 关键发现：按风险优先级列出问题、证据、影响
4. 证据缺口：明确哪些工具未调用、失败或数据为空
5. 处置建议：立即处理、短期加固、后续验证
6. 不要重复粘贴每个步骤已经输出过的完整报告；合并去重后只给一次最终结论

### 多主机分析类（全部主机、所有主机、在线主机、整体平台）
必须使用“覆盖范围 + 每台主机分析 + 整体汇总”的格式：
1. 覆盖范围：目标主机总数、已逐台分析数量、未覆盖主机及原因
2. 每台主机分析：主机名/IP、在线状态、风险等级、发现的问题、关键证据、证据缺口、建议
3. 整体结论：共同风险、差异点、最优先处置主机、最优先处置问题

## 要求
- 基于实际数据回复，不要编造
- 简单查询不要加"安全建议"、"后续操作"等多余内容
- 主机安全分析不能只依据主机基础信息下结论
- 多主机请求不能只输出单台主机结论；每个目标主机都必须有独立分析结果，再给整体结论
- 工具调用成功但结果为空时，数据源状态应写“已覆盖，未发现记录”；只有工具未调用或调用失败才写“未覆盖/证据缺口”
- 不得把“没有告警”写成“没有风险”；必须结合漏洞、基线、运行态和证据缺口说明置信度
- 不得和工具结果自相矛盾：例如已成功调用 Agent.Process.List，就不能写“Agent取证未覆盖”，应写“已覆盖进程取证，网络/文件/日志仍缺口”（如适用）
- 语言简洁，先给最清晰结论，再给证据`, hostSecurityAnalysisGuide())

	return agentruntime.PromptBundle{
		SystemPrompt: systemPrompt,
	}
}

func hostSecurityAnalysisGuide() string {
	return `## 主机安全整体分析准则
当用户要求分析主机、在线主机、全部主机、整体平台或“是否存在安全问题”时，必须按主机安全整体分析处理，而不是普通列表查询。

### 分析目标
1. 先确认分析范围：单台主机、全部主机、在线主机或用户指定主机集合。
2. 对每台目标主机给出清晰风险等级：严重/高/中/低/信息/未知，并说明触发依据。
3. 最终结论必须回答：是否存在安全问题、问题在哪里、证据是什么、哪些证据缺失、优先怎么处理。

### 证据维度
1. 主机身份与在线状态：host_id、主机名、IP、系统、Agent 版本、最后心跳、在线/离线。
2. 资产与暴露面：关键软件、服务、监听端口、外联连接、异常进程、打开文件或脚本痕迹。
3. 基线与任务结果：最近基线检查、任务执行结果、失败项、未覆盖项。
4. 漏洞风险：CVE、严重等级、受影响主机、可利用性、是否存在修复脚本或补丁建议。
5. 告警与检测：安全告警、趋势、影响主机、MITRE 技术、是否有阻断或处置记录。
6. 运行态取证：进程、网络、文件、日志等 Agent 证据；Agent 离线或工具失败时必须写入证据缺口。

### 工具使用策略
1. 范围定位优先使用 Host.List；在线主机场景使用 page_size/limit=100 且 status=online 或 agent_status=online。
2. 多主机场景中，Host.List 返回 N 台目标主机后必须覆盖全部 N 台；如果分页或 has_more 表示未取完，必须继续分页或说明未覆盖范围。
3. 平台侧证据可用 host_ids 批量查询；Host.Get 和 Agent.* 运行态取证必须按主机逐台覆盖，除非明确记录该主机未覆盖原因。
4. Agent 在线时至少尝试进程与网络取证；文件、日志、进程树按风险线索补充。Agent 离线时不能声称运行态安全，只能说明运行态未覆盖。
5. 工具成功但结果为空，应写“已覆盖，未发现记录”；工具未调用、失败、权限不足或 Agent 不在线，才写“证据缺口”。

### 风险判断口径
1. 严重/高风险：存在活跃告警、明确入侵迹象、可利用高危漏洞、异常外联/可疑进程、关键基线严重失败。
2. 中风险：存在未修复漏洞、弱基线项、可疑但未闭环证据、重要证据缺口。
3. 低/信息风险：仅有轻微信息、无明确攻击迹象且主要证据源已覆盖。
4. 未知：关键证据缺失，特别是 Agent 离线或主要工具失败；不得把未知写成安全。

### 输出要求
1. 先给结论摘要：最高风险等级、是否发现安全问题、最优先处置项。
2. 每台主机独立小节：身份、在线状态、风险等级、发现问题、关键证据、证据缺口、建议。
3. 最后给整体汇总：共同风险、差异点、优先处置主机、优先处置步骤。
4. 不要只罗列工具结果；必须把证据归纳成安全判断。不要因为“没有告警”就直接判定安全。`
}

// formatToolList 格式化工具列表（简要版）
func (p *AssistantPromptProvider) formatToolList() string {
	var buf strings.Builder
	for _, desc := range p.toolDescriptors {
		buf.WriteString(fmt.Sprintf("- %s: %s\n", desc.Name, desc.Description))
	}
	return buf.String()
}

// formatToolListDetail 格式化工具列表（详细版，含参数）
func (p *AssistantPromptProvider) formatToolListDetail() string {
	var buf strings.Builder
	for _, desc := range p.toolDescriptors {
		buf.WriteString(fmt.Sprintf("- %s: %s", desc.Name, desc.Description))
		if desc.ArgsSchema != nil {
			if props, ok := desc.ArgsSchema["properties"].(map[string]interface{}); ok {
				var params []string
				for k := range props {
					params = append(params, k)
				}
				if len(params) > 0 {
					buf.WriteString(fmt.Sprintf("。参数：%s", formatParamList(props, desc.ArgsSchema)))
				}
			}
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

func (p *AssistantPromptProvider) formatReflectionGuide() string {
	if len(p.reflectionMemories) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("## 历史反思\n")
	buf.WriteString("以下是本会话此前工具/步骤失败后的内部反思，只用于恢复执行，不需要复述给用户：\n")
	for i, memory := range p.reflectionMemories {
		memory = strings.TrimSpace(memory)
		if memory == "" {
			continue
		}
		buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, memory))
	}
	return strings.TrimSpace(buf.String())
}

type toolSequenceOccurrence struct {
	pos  int
	name string
}

func (p *AssistantPromptProvider) formatMandatoryToolSequenceGuide() string {
	message := strings.TrimSpace(p.userMessage)
	if message == "" || len(p.toolDescriptors) == 0 {
		return ""
	}

	var occurrences []toolSequenceOccurrence
	for _, desc := range p.toolDescriptors {
		name := strings.TrimSpace(desc.Name)
		if name == "" {
			continue
		}
		offset := 0
		for offset < len(message) {
			idx := strings.Index(message[offset:], name)
			if idx < 0 {
				break
			}
			pos := offset + idx
			occurrences = append(occurrences, toolSequenceOccurrence{pos: pos, name: name})
			offset = pos + len(name)
		}
	}
	if len(occurrences) == 0 {
		return ""
	}

	sort.SliceStable(occurrences, func(i, j int) bool {
		return occurrences[i].pos < occurrences[j].pos
	})

	var buf strings.Builder
	buf.WriteString("## 用户指定工具执行约束\n")
	buf.WriteString("用户消息中明确列出了以下可用工具名；这些工具不是建议，而是执行约束。必须按出现顺序调用，并等待每个工具成功或明确失败记录后，才能输出 step_result 或 fail_step：\n")
	for i, item := range occurrences {
		buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.name))
	}
	buf.WriteString("- 同一工具重复出现表示需要按上下文使用不同参数分别调用，例如 Baseline.Script.Generate(CHECK) 和 Baseline.Script.Generate(FIX) 分别使用 script_type=CHECK/FIX。\n")
	buf.WriteString("- 如果已上传上下文或用户消息提供 template_id、rule_id、host_id，直接使用这些 ID；不得因为已有上下文而声称看不到文件。\n")
	buf.WriteString("- 不要重复查询同一模板状态或规则列表超过一次，除非前一次调用失败或缺少必要参数。\n")
	if strings.Contains(message, "Task.RunCheck") || strings.Contains(message, "Task.RunFix") {
		buf.WriteString("- Task.List 只能在 Task.RunCheck/Task.RunFix 下发后用于查询进度或结果；不得用 Task.List 替代 Task.RunCheck 或 Task.RunFix。\n")
	}
	if strings.Contains(message, "Baseline.Script.Generate") && strings.Contains(message, "Task.RunCheck") {
		buf.WriteString("- 基线闭环中，完成检测脚本和修复脚本生成后，下一步必须下发 Task.RunCheck；如果用户要求修复，再调用 Task.RunFix；最后再调用 Task.List 或 Task.GetDetail 查询任务状态。\n")
	}
	if strings.Contains(message, "Asset.Collection.Trigger") {
		buf.WriteString("- 资产采集闭环中，如果 Asset.Collection.Trigger 的返回包含 asset_collection_sequence_complete=true 或 all_requested_tools_success=true，表示系统已经自动完成 Asset.Collection.Get、Asset.Application.List 和 Asset.Summary.Get 查询；必须立即基于 verified_result_summary 输出 step_result，不要再调用 Task.GetDetail、Tool.Search，也不要声称 Asset.Summary.Get 不存在或 task_id 缺失。\n")
	}
	if strings.Contains(message, "Vulnerability.Script.Execute") {
		buf.WriteString("- 漏洞 POC/FIX 闭环中，如果 Vulnerability.Script.Status 或 Vulnerability.Script.Generate 返回 vulnerability_script_sequence_complete=true，表示系统已经按用户要求自动下发 Vulnerability.Script.Execute；必须基于 executions 中的 task_group_id 查询或输出任务状态，不要停留在脚本状态查询。\n")
	}
	return strings.TrimSpace(buf.String())
}

func (p *AssistantPromptProvider) formatNaturalOperationGuide() string {
	message := normalizeNaturalOperationText(p.userMessage)
	if message == "" || !hasAnyOperationalIntentForPrompt(message) {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("## 自然语言工具使用推理原则\n")
	buf.WriteString("用户通常不会写工具名。你需要先理解业务目标，再自行拆解步骤并选择合适工具，而不是机械套固定流程。\n")
	buf.WriteString("1. 先抽取：目标对象（主机/资产/软件/漏洞/基线/告警/任务）、动作（查询/采集/扫描/分析/生成/下发/修复）、范围（全部/在线/指定）、约束和缺失信息。\n")
	buf.WriteString("2. 信息不足时先判断能否安全默认：只读查询可用合理默认；会创建任务、扫描、采集、修复、下发的操作如果目标或动作不清，应先追问。\n")
	buf.WriteString("3. 工具选择应覆盖用户最终目标，不要拿第一个工具结果直接收工。例如用户要求“采集并分析软件漏洞”，采集只是证据来源之一，还需要继续查询软件、漏洞和受影响范围。\n")
	buf.WriteString("4. 优先使用最贴近业务对象的工具；当前工具不足时使用 Tool.Search 搜索候选工具，不能发明不存在的工具名。\n")
	buf.WriteString("5. 写操作或任务型工具必须尊重审批结果；工具返回 task_id 后，如用户目标需要结果或进度，应继续用对应状态/详情工具查询。\n")
	buf.WriteString("6. 最终回答必须说明已调用的数据源、关键证据、没有覆盖的证据缺口和下一步建议；没有数据只能说“当前数据未发现”，不能宣称绝对安全。")
	return buf.String()
}

func hasAnyOperationalIntentForPrompt(text string) bool {
	return hasAssetCollectionIntent(text) ||
		hasVulnerabilityScanIntent(text) ||
		hasBaselineScanIntent(text) ||
		hasDetectionCheckIntent(text) ||
		strings.Contains(text, "扫描") ||
		strings.Contains(text, "采集") ||
		strings.Contains(text, "修复") ||
		strings.Contains(text, "下发") ||
		strings.Contains(text, "生成")
}

func (p *AssistantPromptProvider) formatContextRefs() string {
	if len(p.contextRefs) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("## 已上传/已关联上下文\n")
	buf.WriteString("以下对象已经绑定到当前会话；涉及文件、基线模板或规则时，必须优先使用这些对象的 ID 和状态，不要声称看不到文件：\n")
	for _, ref := range p.contextRefs {
		buf.WriteString(fmt.Sprintf("- %s (%s, id=%s)", ref.Title, ref.ObjectType, ref.ObjectID))
		if strings.TrimSpace(ref.Summary) != "" {
			buf.WriteString(": ")
			buf.WriteString(strings.TrimSpace(ref.Summary))
		}
		if len(ref.Data) > 0 {
			encoded, err := json.Marshal(ref.Data)
			if err == nil && len(encoded) > 0 {
				data := string(encoded)
				if len([]rune(data)) > 1600 {
					data = string([]rune(data)[:1600]) + "..."
				}
				buf.WriteString("\n  data: ")
				buf.WriteString(data)
			}
		}
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

// formatParamList 格式化参数列表
func formatParamList(props map[string]interface{}, schema map[string]interface{}) string {
	required, _ := schema["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		if s, ok := r.(string); ok {
			requiredSet[s] = true
		}
	}

	var parts []string
	for name, prop := range props {
		propMap, ok := prop.(map[string]interface{})
		if !ok {
			continue
		}
		desc, _ := propMap["description"].(string)
		if requiredSet[name] {
			parts = append(parts, fmt.Sprintf("%s（必填）", name))
		} else {
			parts = append(parts, fmt.Sprintf("%s（可选）", name))
		}
		_ = desc // 描述暂不输出，保持简洁
	}
	b, _ := json.Marshal(parts)
	return string(b)
}

package llm

import (
	"encoding/json"
	"fmt"
)

// Prompt templates for LLM interactions

// RuleExtractionPrompt extracts aegis rules from uploaded documents
const RuleExtractionPrompt = `你是一位资深的安全基线专家，擅长从技术文档中提取安全检查规则。

请从以下文档内容中提取所有的安全基线检查项，并以 JSON 数组格式返回。每个检查项包含：
- title: 规则标题（简洁明了）
- check_content: 检查内容描述（详细说明需要检查什么）
- fix_content: 修复方法描述（如果检查不通过，如何修复）

返回格式示例：
[
  {
    "title": "SSH 密码复杂度要求",
    "check_content": "检查/etc/pam.d/common-password 是否配置了密码复杂度要求",
    "fix_content": "在/etc/pam.d/common-password 中添加 pam_pwquality.so 模块配置"
  }
]

文档内容：
%s

请只返回 JSON 数组，不要包含其他文字说明。确保 JSON 格式正确，可以直接解析。`

// CheckScriptGenerationPrompt generates shell check scripts from rule content
const CheckScriptGenerationPrompt = `你是一位资深的 Shell 脚本工程师，擅长编写系统检查和审计脚本。

请根据以下基线规则的检查内容，生成一个 Shell 检查脚本。

要求：
1. 脚本必须是完整的、可执行的 bash 脚本，以#!/bin/bash 开头
2. 脚本执行成功返回 exit code 0，失败返回 exit code 1
3. 脚本应该有清晰的输出，说明检查通过或失败的原因
4. 使用标准的 bash 命令和工具
5. 脚本应该健壮，处理可能的异常情况

规则检查内容：
%s

请只返回脚本内容，不要包含其他文字说明。脚本应该可以直接执行。`

// FixScriptGenerationPrompt generates shell fix scripts from rule content
const FixScriptGenerationPrompt = `你是一位资深的 Shell 脚本工程师，擅长编写系统配置和修复脚本。

请根据以下基线规则的修复内容，生成一个 Shell 修复脚本。

要求：
1. 脚本必须是完整的、可执行的 bash 脚本，以#!/bin/bash 开头
2. 脚本执行成功返回 exit code 0，失败返回 exit code 1
3. 脚本应该有清晰的输出，说明修复操作的进度和结果
4. 使用标准的 bash 命令和工具
5. 脚本应该健壮，处理可能的异常情况
6. 对于需要 root 权限的操作，应该有权限检查

规则修复内容：
%s

请只返回脚本内容，不要包含其他文字说明。脚本应该可以直接执行。`

// SelfHealingFixPrompt generates fix scripts for failed scripts
const SelfHealingFixPrompt = `你是一位资深的 Shell 脚本调试专家，擅长分析和修复脚本错误。

有一个脚本执行失败了，请分析错误信息并生成修复后的脚本。

原始脚本：
%s

执行错误：
%s

退出码：%d

历史修复尝试（如果有）：
%s

请分析失败原因，生成一个修复后的脚本。修复应该：
1. 针对具体的错误信息进行修复
2. 保留原始脚本的正确部分
3. 修复应该是最小化的改动，不要重写整个脚本
4. 确保修复后的脚本可以正常执行

请只返回修复后的完整脚本内容，不要包含其他文字说明。`

// GetRuleExtractionPrompt returns the rule extraction prompt with document content
func GetRuleExtractionPrompt(documentContent string) string {
	return fmt.Sprintf(RuleExtractionPrompt, documentContent)
}

// GetCheckScriptGenerationPrompt returns the check script generation prompt
func GetCheckScriptGenerationPrompt(checkContent string) string {
	return fmt.Sprintf(CheckScriptGenerationPrompt, checkContent)
}

// GetFixScriptGenerationPrompt returns the fix script generation prompt
func GetFixScriptGenerationPrompt(fixContent string) string {
	return fmt.Sprintf(FixScriptGenerationPrompt, fixContent)
}

// GetSelfHealingFixPrompt returns the self-healing fix prompt
func GetSelfHealingFixPrompt(originalScript, errorMessage string, exitCode int, history string) string {
	if history == "" {
		history = "无"
	}
	return fmt.Sprintf(SelfHealingFixPrompt, originalScript, errorMessage, exitCode, history)
}

// CVEAnalysisPrompt analyzes software inventory to identify CVE vulnerabilities
const CVEAnalysisPrompt = `You are a senior cybersecurity expert specializing in CVE vulnerability analysis.

## MANDATORY Output Rules (CRITICAL - must follow in ALL cases)
1. You MUST output ONLY a valid JSON array, nothing else
2. No explanations, comments, or any other text
3. Even on errors, you MUST return a JSON array (empty array [] means no vulnerabilities)

## JSON Array Format (strict format):
[
  {
    "cve_id": "CVE-YYYY-NNNNN",
    "severity": "Critical|High|Medium|Low",
    "cvss_score": 9.8,
    "description": "Brief description of the vulnerability",
    "affected_package": "The vulnerable software package name",
    "affected_versions": "Version range affected",
    "fix_version": "Fixed version",
    "attack_vector": "Network|Local|Adjacent",
    "references": ["https://nvd.nist.gov/vuln/detail/CVE-YYYY-NNNNN"]
  }
]

## Rules
- If vulnerabilities found: return JSON array with vulnerability info (1 or more)
- If NO vulnerabilities found: MUST return empty array [] (NOT null, NOT empty string, NOT any text)
- If uncertain: analyze based on package name and known vulnerability patterns, return most likely CVEs

## FORBIDDEN
- No explanations or comments
- No empty lines or whitespace only
- No empty response (even on errors, MUST return [])

## Valid Output Examples
Correct: []
Correct: [{"cve_id":"CVE-2021-44228","severity":"Critical","cvss_score":10.0,"description":"Log4j RCE","affected_package":"log4j-core","affected_versions":"2.0-2.14.1","fix_version":"2.17.0","attack_vector":"Network","references":["https://nvd.nist.gov/vuln/detail/CVE-2021-44228"]}]
Incorrect: CVE-2021-44228
Incorrect: No vulnerabilities found
Incorrect: (empty)`

// CVEAnalysisPromptZH is the Chinese version for Chinese model
const CVEAnalysisPromptZH = `你是一个CVE漏洞分析助手，负责分析软件清单并识别安全漏洞。

## 强制输出要求（最重要，任何情况下都必须遵守）
1. 你必须且只能输出一个有效的JSON数组
2. 禁止输出任何其他文字、解释、说明、注释或空行
3. 即使发生错误，也必须返回一个JSON数组（空数组[]表示无漏洞）

## JSON数组格式（必须严格遵循）
[
  {
    "cve_id": "CVE-年份-编号",
    "severity": "Critical|High|Medium|Low",
    "cvss_score": 分数（0.0-10.0）,
    "description": "漏洞中文描述，简洁准确",
    "affected_package": "存在漏洞的软件包名称",
    "affected_versions": "受影响版本范围",
    "fix_version": "修复该漏洞的版本",
    "attack_vector": "攻击向量：Network|Local|Adjacent",
    "references": ["https://nvd.nist.gov/vuln/detail/CVE-XXXX-XXXXX"]
  }
]

## 分析规则
- 如果发现漏洞：返回包含漏洞信息的JSON数组（可能包含1个或多个漏洞）
- 如果未发现任何漏洞：必须返回空数组 []（不能返回其他任何内容）
- 如果无法确定：基于软件包名称和已知漏洞模式进行分析，给出最可能的CVE

## 禁止事项
- 禁止输出任何中文文字（"漏洞"、"分析"等）
- 禁止输出解释性文字
- 禁止输出注释
- 禁止输出空行或空白内容
- 禁止输出空响应（即使是错误情况也必须返回[]）

## 输出示例
正确：[]
正确：[{"cve_id":"CVE-2021-44228","severity":"Critical","cvss_score":10.0,"description":"Log4j远程代码执行漏洞","affected_package":"log4j-core","affected_versions":"2.0-2.14.1","fix_version":"2.17.0","attack_vector":"Network","references":["https://nvd.nist.gov/vuln/detail/CVE-2021-44228"]}]
错误：CVE-2021-44228
错误：未发现漏洞
错误：（无输出）`

// VulnerabilityFixPrompt generates secure fix scripts for vulnerabilities
const VulnerabilityFixPrompt = `你是一位资深的 DevOps 工程师，专门负责编写安全、可靠的服务器运维脚本。

## 脚本编写规范
1. 必须包含：脚本头部注释、前置检查、备份操作、修复操作、结果验证、错误处理
2. 禁止：rm -rf /、删除系统关键文件、创建后门账户、关闭防火墙
3. 安全要求：set -e, set -u, 使用绝对路径, HTTPS下载

## 输出要求
只输出完整的 Shell 脚本内容，不需要额外说明。`

// POCVerificationPrompt generates safe POC verification scripts for vulnerabilities
const POCVerificationPrompt = `你是一位专业的安全研究员，专门负责编写漏洞验证脚本（POC）。

## 绝对禁止
- 删除文件、修改系统配置、停止服务、创建后门、执行恶意代码
- 数据篡改、拒绝服务攻击

## 允许操作
- 版本检查、配置检查（只读）、特征检测、日志分析、无害探测

## 输出要求
输出安全的 Shell 验证脚本，退出码规范：0=安全, 1=漏洞存在, 2=验证出错`

// AIMessage represents a message in the AI conversation history
type AIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ReActPromptTemplate is the prompt template for ReAct agent
const ReActPromptTemplate = `You are Aegis, an AI-powered security analysis assistant.

Your task is to analyze security alerts and determine if they are real threats or false positives.

To help you analyze, you have access to the following tools:
- GetProcessTree: Get the process tree for a given PID. Parameters: host_id (required), pid (required)
- GetNetworkConnections: Get network connections for a process or all connections. Parameters: host_id (required), pid (optional)
- GetOpenFiles: Get open files for a process. Parameters: host_id (required), pid (required)
- GetRunningProcesses: List running processes (supports filtering). Parameters: host_id (required), filter (optional)
- GetUserSessions: Get active user login sessions. Parameters: host_id (required)
- QueryHistoricalLogs: Query historical logs within a time range. Parameters: host_id (required), start_time (required, RFC3339 format like "2026-04-14T10:00:00Z"), end_time (required, RFC3339 format), filter (optional)

IMPORTANT: When calling QueryHistoricalLogs, you MUST include start_time and end_time parameters. The session time range is provided in the Alert Context - use those values.

You must follow the ReAct (Reasoning + Acting) format:
Do NOT output markdown plans, prose-only analysis, or standalone JSON tool parameters.
Before the final answer, every investigation step MUST use Action and Action Input.
For the first investigation step, prefer QueryHistoricalLogs when alert context contains host_id and time range.

Format:
Thought: [your reasoning about what to do next]
Action: [the action to take, should be one of the available tools]
Action Input: [the input to the action in JSON format]
Observation: [the result of the action]
... (this cycle can repeat N times)

When you have gathered enough information, you MUST provide the final answer in a structured JSON format for an attack graph visualization:

Final Answer:
{
  "attack_graph": {
    "graphId": "graph_[timestamp]_[sequence]",
    "title": "[攻击类型，如：反弹Shell攻击链路溯源]",
    "summary": "[一句话描述攻击链路]",
    "threatLevel": "[critical/high/medium/low]",
    "nodes": [
      {
        "id": "[唯一ID，如：attacker_1]",
        "type": "[attacker/victim/process/file/network/command/malware]",
        "label": "[显示名称]",
        "detail": "[详细信息]",
        "properties": {},
        "severity": "[critical/high/medium/low/info]"
      }
    ],
    "edges": [
      {
        "id": "[唯一ID，如：edge_1]",
        "source": "[源节点ID]",
        "target": "[目标节点ID]",
        "type": "[spawns/connects/reads/writes/executes/downloads/encrypts/exfiltrates]",
        "label": "[边标签，如：外连、spawns]",
        "properties": {}
      }
    ],
    "timeline": [
      {"timestamp": "[ISO时间]", "event": "[事件描述]", "nodeIds": ["相关节点ID"]}
    ],
    "recommendations": ["[处置建议1]", "[处置建议2]", ...]
  },
  "conclusions": [
    {"alert_id": "[告警ID]", "action": "[mark_false_positive/confirm_threat/generate_rule]", "summary": "[该告警的中文分析结论]"}
  ]
}
Remember:
- Always include the host_id when calling tools
- Be thorough in your investigation
- Base your conclusions on evidence from tool results
- All user-facing text fields must be in Simplified Chinese, including title, summary, node labels/details, edge labels, timeline events, recommendations, and conclusions.summary
`

// BuildReActPrompt builds the prompt for ReAct agent
func BuildReActPrompt(userMessage string, history []*AIMessage, context map[string]interface{}) []Message {
	messages := []Message{
		{Role: "system", Content: ReActPromptTemplate},
	}

	// Add history messages
	for _, msg := range history {
		messages = append(messages, Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add alert context
	if len(context) > 0 {
		contextStr, _ := json.Marshal(context)
		messages = append(messages, Message{
			Role:    "system",
			Content: fmt.Sprintf("Alert Context: %s", string(contextStr)),
		})
	}

	// Build enhanced user message with explicit time range instructions
	enhancedMessage := userMessage
	if startTime, hasStart := context["start_time"].(string); hasStart {
		if endTime, hasEnd := context["end_time"].(string); hasEnd {
			enhancedMessage = fmt.Sprintf("%s\n\nIMPORTANT: When using QueryHistoricalLogs tool, you MUST use:\n- start_time: %s\n- end_time: %s\n\nDo not ask for these values - use the above values directly.", userMessage, startTime, endTime)
		}
	}

	// Add user message
	messages = append(messages, Message{
		Role:    "user",
		Content: enhancedMessage,
	})

	return messages
}

// ScriptAuditSystemPrompt is the system prompt for AI script security audit
const ScriptAuditSystemPrompt = `你是一位资深的Shell脚本安全审计专家。你的任务是审查由AI生成的Shell脚本，判断是否存在安全风险。

## 审查维度

1. **权限提升**: 是否存在隐蔽的权限提升手段
   - sudo嵌套使用
   - 环境变量注入（如PATH劫持、LD_PRELOAD）
   - 利用SUID/SGID文件
   - 利用capabilities

2. **数据外泄**: 是否存在数据外泄风险
   - 将敏感数据编码后外传（base64/hex编码后curl/wget）
   - DNS隧道（通过DNS查询外传数据）
   - ICMP隧道

3. **条件性恶意行为**: 是否存在触发条件后才执行的恶意代码
   - 时间触发（特定日期/时间执行恶意操作）
   - 环境检测（检测沙箱/虚拟机后改变行为）
   - 网络条件触发

4. **意图不一致**: 脚本是否与其声明的检查/修复意图不一致
   - 声称是检查脚本但包含修改操作
   - 声称是修复脚本但包含无关的系统操作

5. **资源耗尽**: 是否可能导致系统资源耗尽
   - 创建超大文件、无限循环、内存炸弹

6. **后门植入**: 是否存在后门或持久化机制
   - 添加SSH公钥、修改crontab、创建隐藏用户

## 输出格式（必须为JSON）

{
  "passed": true或false,
  "risk_level": "safe|low|medium|high|critical",
  "issues": [
    {
      "type": "privilege_escalation|data_exfiltration|conditional_malicious|intent_mismatch|resource_exhaustion|backdoor",
      "description": "问题描述",
      "line_range": "起始行-结束行",
      "suggestion": "修复建议"
    }
  ],
  "summary": "审计总结"
}

## 判断标准

- critical/high级别问题 → passed=false
- 仅medium级别 → passed=true，记录问题
- 正常系统管理操作（apt install、systemctl restart）不判为恶意
- 不确定时倾向通过，但记录疑虑
- 所有输出使用简体中文`

// DetectionPackageGenerationPrompt generates detection package drafts from CVE information
const DetectionPackageGenerationPrompt = `你是 Aegis V5.8 的 AI 安全规则生成器。你的输出是人工可修改的草稿，不是最终发布物。

## 输入

你会收到：
- CVE 编号
- 漏洞描述
- 攻击前置条件
- 利用链行为
- 可观测系统调用或内核 hook
- 误报约束
- 当前 agent 支持能力

## 输出

必须输出四段，每段使用对应语言标记的代码块：

1. HookPlan YAML - 只描述 hook、extract、filter、emit，不包含告警逻辑
2. eBPF C 源码草稿 - 只做事件采集和轻量过滤，不做复杂检测
3. Sigma atomic rules YAML - 只做单事件 atomic detection
4. Correlation DetectionSpec YAML - 只做 ordered sequence + window + by

## 关键规则

- HookPlan 只描述采集，不描述告警。
- eBPF 插件只做事件采集和轻量过滤。
- Sigma 只做单事件 atomic detection。
- Correlation 只做 ordered sequence + window + by。
- rule_id 使用 package_id.stable_name 格式。
- 不生成跨 package 依赖。
- 不使用未明确允许的 hook 类型（默认只允许 tracepoint）。
- 输出必须避免不可控事件风暴。

## 输出模板

请按以下章节输出：

## Package Metadata
package_id, version, title, description, cve_ids

## HookPlan
使用 yaml 代码块

## eBPF Source Draft
使用 c 代码块

## Sigma Atomic Rules
使用 yaml 代码块

## Correlation DetectionSpec
使用 yaml 代码块

## 风险与限制
说明检测的边界和潜在误报

## 安全边界声明

请在输出末尾明确写出：
该输出为草稿，必须经过人工修改、builder 容器编译、人工审核、人工签名发布和页面启用后，才能由 agent 安装。

CVE 信息：
%s

漏洞描述：
%s

攻击前置条件：
%s

利用链行为：
%s

误报约束：
%s`

// GetDetectionPackageGenerationPrompt returns the detection package generation prompt
func GetDetectionPackageGenerationPrompt(cveID, description, prerequisites, chain, constraints string) string {
	return fmt.Sprintf(DetectionPackageGenerationPrompt, cveID, description, prerequisites, chain, constraints)
}

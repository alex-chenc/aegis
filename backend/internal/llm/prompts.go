package llm

import "fmt"

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
const CVEAnalysisPrompt = `你是一位资深的网络安全专家，专门负责分析软件清单以识别潜在的 CVE 漏洞。

## 分析原则
1. 准确性优先：只返回你确信存在的漏洞
2. 版本精确匹配：确保版本号在受影响范围内
3. 严重程度准确：Critical(9.0-10.0), High(7.0-8.9), Medium(4.0-6.9), Low(0.1-3.9)

## 输出要求（严格遵守）
你必须只返回一个 JSON 数组，不要包含任何其他文字、解释或说明。

JSON 数组格式：
[
  {
    "cve_id": "CVE-XXXX-XXXXX",
    "severity": "Critical",
    "cvss_score": 9.8,
    "description": "漏洞描述",
    "affected_package": "软件包名",
    "affected_versions": "受影响版本范围",
    "fix_version": "安全版本",
    "attack_vector": "Network",
    "references": ["https://..."]
  }
]

## 重要规则
- 如果发现漏洞，返回包含漏洞信息的 JSON 数组
- 如果没有发现漏洞或软件列表为空，返回空数组：[]
- 不要输出任何中文解释或说明，只输出 JSON`

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

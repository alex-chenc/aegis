package llm

import "fmt"

// Prompt templates for LLM interactions

// RuleExtractionPrompt extracts baseline rules from uploaded documents
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

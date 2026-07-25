package llm

import (
	"encoding/json"
	"fmt"
)

// Prompt templates for LLM interactions

// RuleExtractionPrompt extracts aegis rules from uploaded documents
const RuleExtractionPrompt = `You are a senior security-baseline expert. Extract every security baseline check from the supplied document.

Return a JSON array. Each item must contain:
- title: a concise rule title
- check_content: a detailed description of what to check
- fix_content: remediation instructions for a failed check

Example:
[
  {
    "title": "SSH password complexity requirement",
    "check_content": "Check whether /etc/pam.d/common-password configures password complexity.",
    "fix_content": "Add an appropriate pam_pwquality.so configuration to /etc/pam.d/common-password."
  }
]

Document content:
%s

Return only a directly parseable JSON array. Do not output explanations or Markdown. Preserve the document's language in user-facing rule text when appropriate.`

// CheckScriptGenerationPrompt generates shell check scripts from rule content
const CheckScriptGenerationPrompt = `You are a senior shell engineer who writes system audit and compliance-check scripts.

Generate one complete executable Bash check script from the baseline check below.

Requirements:
1. Start with #!/bin/bash.
2. Use these exit codes exactly:
   - exit code 0: the check completed and the baseline item is compliant.
   - exit code 1: the check completed and the baseline item is non-compliant; this is a business result, not a script error.
   - exit code 2 or higher: execution error, missing dependency, insufficient permission, parse failure, or unsupported environment.
3. Print a clear reason for compliant, non-compliant, or execution-error results.
4. Use standard Bash commands and tools.
5. Handle expected failures and environment differences safely.
6. Never report non-compliance as a script error. Use 2 or higher only when the check cannot be completed reliably.

Baseline check:
%s

Return only the executable script, without Markdown or explanations.`

// FixScriptGenerationPrompt generates shell fix scripts from rule content
const FixScriptGenerationPrompt = `You are a senior shell engineer who writes safe system-configuration and remediation scripts.

Generate one complete executable Bash remediation script from the baseline check and remediation guidance below.

Requirements:
1. Start with #!/bin/bash.
2. Return exit code 0 on success and a non-zero exit code on failure.
3. Print clear progress and outcome messages.
4. Use standard Bash commands and tools.
5. Handle expected failures safely.
6. Check privileges before operations that require root.
7. Make the remediation satisfy the inverse of the check: an equivalent check must pass afterward.
8. Include post-remediation verification derived from the check. Return non-zero when verification fails.
9. Prefer the supplied remediation guidance and add necessary idempotency checks, backups, and verification when it is incomplete.
10. Be idempotent and preserve already-correct configuration.

Baseline check:
%s

Remediation guidance:
%s

Return only the executable script, without Markdown or explanations.`

// SelfHealingFixPrompt generates fix scripts for failed scripts
const SelfHealingFixPrompt = `You are a senior shell-script debugging expert. Analyze the failure and generate a corrected script.

Original script:
%s

Execution error:
%s

Exit code: %d

Previous remediation attempts, if any:
%s

Requirements:
1. Fix the specific observed error.
2. Preserve correct parts of the original script.
3. Make the smallest safe change; do not rewrite the entire script unnecessarily.
4. Produce a complete executable corrected script.

Return only the corrected script, without Markdown or explanations.`

// GetRuleExtractionPrompt returns the rule extraction prompt with document content
func GetRuleExtractionPrompt(documentContent string) string {
	return fmt.Sprintf(RuleExtractionPrompt, documentContent)
}

// RuleExtractionPromptStrict is used as a retry prompt when the model returns a
// response that cannot be parsed as JSON. It is more prescriptive: it demands a
// single JSON array wrapped in a ```json code block and no explanatory prose.
// NOTE: Go raw string literals cannot contain backticks, so the fence markers
// are concatenated as interpreted strings.
const RuleExtractionPromptStrict = `You are a senior security-baseline expert. Extract every security baseline check from the document below.

Strict requirements:
1. Output exactly one JSON array with no preface, explanation, or summary.
2. Wrap the entire JSON array in one ` + "```" + `json code fence.
3. Every item must contain "title", "check_content", and "fix_content".
4. Do not output Markdown lists, comments, or a JSON-escaped string.

Example:
` + "```json" + `
[
  {
    "title": "SSH password complexity requirement",
    "check_content": "Check whether /etc/pam.d/common-password configures password complexity.",
    "fix_content": "Add an appropriate pam_pwquality.so configuration."
  }
]
` + "```" + `

Document content:
%s`

// GetRuleExtractionPromptStrict returns the strict rule extraction prompt.
func GetRuleExtractionPromptStrict(documentContent string) string {
	return fmt.Sprintf(RuleExtractionPromptStrict, documentContent)
}

// GetCheckScriptGenerationPrompt returns the check script generation prompt
func GetCheckScriptGenerationPrompt(checkContent string) string {
	return fmt.Sprintf(CheckScriptGenerationPrompt, checkContent)
}

// GetFixScriptGenerationPrompt returns the fix script generation prompt
func GetFixScriptGenerationPrompt(checkContent, fixContent string) string {
	return fmt.Sprintf(FixScriptGenerationPrompt, checkContent, fixContent)
}

// GetSelfHealingFixPrompt returns the self-healing fix prompt
func GetSelfHealingFixPrompt(originalScript, errorMessage string, exitCode int, history string) string {
	if history == "" {
		history = "none"
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

// CVEAnalysisPromptZH retains its public name for compatibility. Its
// instructions are English while user-facing descriptions may remain Chinese.
const CVEAnalysisPromptZH = `You are a CVE analysis assistant. Analyze the supplied software inventory and identify security vulnerabilities.

Mandatory output requirements:
1. Output exactly one valid JSON array and nothing else.
2. Do not output explanations, comments, Markdown, or blank prose.
3. Return [] when no vulnerability is found or no supported conclusion can be produced.

Item schema:
{"cve_id":"CVE-YYYY-NNNNN","severity":"Critical|High|Medium|Low","cvss_score":0.0,"description":"concise vulnerability description in Simplified Chinese","affected_package":"package name","affected_versions":"affected range","fix_version":"fixed version","attack_vector":"Network|Local|Adjacent","references":["authoritative URL"]}

Use authoritative CVE identifiers and references. Never invent a CVE merely from a package-name pattern.`

// VulnerabilityFixPrompt generates secure fix scripts for vulnerabilities
const VulnerabilityFixPrompt = `You are a senior DevOps engineer who writes safe and reliable server-remediation scripts.

Requirements:
1. Include a header, preflight checks, backups, remediation, verification, and error handling.
2. Never delete critical system files, create backdoor accounts, disable the firewall, or use destructive commands such as rm -rf /.
3. Use set -e, set -u, absolute paths where practical, and HTTPS for downloads.
4. Make changes idempotent and verify the result locally.

Return only the complete shell script without Markdown or explanations.`

// POCVerificationPrompt generates safe POC verification scripts for vulnerabilities
const POCVerificationPrompt = `You are a security researcher who writes safe, non-destructive vulnerability verification scripts.

Forbidden:
- Deleting files, changing system configuration, stopping services, creating persistence or backdoors, or executing malicious payloads.
- Data modification or denial-of-service behavior.

Allowed:
- Version checks, read-only configuration checks, signature detection, log analysis, and harmless probes.

Return only a safe shell verification script. Exit codes: 0=safe or not affected, 1=vulnerability detected, 2=verification error.`

// AIMessage represents a message in the AI conversation history
type AIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ReActPromptTemplate is the prompt template for ReAct agent
const ReActPromptTemplate = `You are Aegis, an AI-powered security analysis assistant.

Your task is to analyze security alerts and determine if they are real threats or false positives.

To help you analyze, you have access to the following tools:
- GetProcessTree: Get the process tree for a given PID. Parameters: host_id (required), pid (optional, defaults to 1)
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
    "title": "[attack type, for example reverse-shell attack path]",
    "summary": "[one-sentence attack-path summary]",
    "threatLevel": "[critical/high/medium/low]",
    "nodes": [
      {
        "id": "[unique ID, for example attacker_1]",
        "type": "[attacker/victim/process/file/network/command/malware]",
        "label": "[display label]",
        "detail": "[details]",
        "properties": {},
        "severity": "[critical/high/medium/low/info]"
      }
    ],
    "edges": [
      {
        "id": "[unique ID, for example edge_1]",
        "source": "[source node ID]",
        "target": "[target node ID]",
        "type": "[spawns/connects/reads/writes/executes/downloads/encrypts/exfiltrates]",
        "label": "[edge label, for example outbound connection or spawns]",
        "properties": {}
      }
    ],
    "timeline": [
      {"timestamp": "[ISO timestamp]", "event": "[event description]", "nodeIds": ["related node ID"]}
    ],
    "recommendations": ["[recommendation 1]", "[recommendation 2]", ...]
  },
  "conclusions": [
    {"alert_id": "[alert ID]", "action": "[mark_false_positive/confirm_threat/generate_rule]", "summary": "[analysis conclusion in Simplified Chinese]"}
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
const ScriptAuditSystemPrompt = `You are a senior shell-script security auditor. Review an AI-generated shell script for security risks.

Review dimensions:
1. Privilege escalation: nested sudo, PATH or LD_PRELOAD injection, SUID or SGID abuse, and Linux capability abuse.
2. Data exfiltration: encoded outbound data, DNS tunneling, and ICMP tunneling.
3. Conditional malicious behavior: time triggers, sandbox or VM detection, and network-condition triggers.
4. Intent mismatch: a check script that modifies state, or a remediation script that performs unrelated system operations.
5. Resource exhaustion: oversized files, unbounded loops, fork bombs, or memory exhaustion.
6. Backdoors or persistence: SSH key injection, crontab changes, or hidden users.

Return JSON only:
{"passed":true,"risk_level":"safe|low|medium|high|critical","issues":[{"type":"privilege_escalation|data_exfiltration|conditional_malicious|intent_mismatch|resource_exhaustion|backdoor","description":"issue description in Simplified Chinese","line_range":"start-end","suggestion":"remediation in Simplified Chinese"}],"summary":"audit summary in Simplified Chinese"}

Decision criteria:
- Any critical or high issue means passed=false.
- Medium-only findings may keep passed=true but must be recorded.
- Normal administration such as apt install or systemctl restart is not inherently malicious when it matches the declared intent.
- When uncertain, record the concern and make the most evidence-based decision.`

// DetectionPackageGenerationPrompt generates detection package drafts from CVE information
const DetectionPackageGenerationPrompt = `You are the Aegis V5.8 AI security-rule generator. Produce an editable draft, never a final published artifact.

Input includes a CVE identifier, vulnerability description, prerequisites, exploitation-chain behavior, observable system calls or kernel hooks, false-positive constraints, and current agent capabilities.

Output these sections with correctly labeled code fences:
1. Coverage Decision YAML: declare whether the active Hook allowlist faithfully observes every core exploitation behavior.
2. HookPlan YAML: collection-only hook, extract, filter, and emit definitions; no alert logic.
3. eBPF C source draft: event collection and lightweight filtering only.
4. Sigma atomic rules YAML: single-event atomic detection only.
5. Correlation DetectionSpec YAML: ordered sequence, window, and by only.

Rules:
- Use rule_id in package_id.stable_name form.
- Every Sigma atomic rule must include an attack.txxxx MITRE technique tag. The correlation alert must include the same actionable mitre_id.
- Do not create cross-package dependencies.
- Use only hooks listed in the ACTIVE HOOK ALLOWLIST supplied below. Every attach_type and attach value must match exactly.
- Never substitute an unrelated allowlisted hook merely to make the package pass validation.
- If the active allowlist cannot faithfully observe every core behavior in the exploit chain, set Coverage Decision status to unsupported, list the uncovered behaviors, list the exact minimally required hooks, and omit all generated code artifacts.
- Avoid unbounded event volume.

## eBPF source requirements

Include these headers:

#include <linux/bpf.h>
#include <linux/types.h>
#include <linux/sched.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

Define these aliases:
typedef __u8 u8;
typedef __u16 u16;
typedef __u32 u32;
typedef __u64 u64;

Define this tracepoint context:
struct trace_event_raw_sys_enter {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;
    long id;
    unsigned long args[6];
};

For tracepoint programs:
- SEC("tracepoint/syscalls/sys_enter_xxx")
- Use int tracepoint__syscalls__sys_enter_xxx(struct trace_event_raw_sys_enter *ctx).
- Access arguments through ctx->args[0], ctx->args[1], and so on.
- Support both AEGIS_EVENT_PERF and AEGIS_EVENT_RINGBUF conditional compilation.
- The ring-buffer branch uses BPF_MAP_TYPE_RINGBUF with bpf_ringbuf_reserve and bpf_ringbuf_submit.
- The perf branch uses BPF_MAP_TYPE_PERF_EVENT_ARRAY with bpf_perf_event_output and a stack-allocated struct event; it must not call bpf_ringbuf_reserve.
- Obtain pid and tid with bpf_get_current_pid_tgid().
- Never call bpf_probe_read_kernel(), bpf_override_return(), bpf_setsockopt(), bpf_sk_redirect(), or bpf_get_current_task(). The builder rejects these helpers.
- Do not use kernel-source pointer annotations such as __user, __kernel, __iomem, or __force. They are not defined by the standalone Builder headers. Declare user pointers as const void * and read them only with bpf_probe_read_user().
- Never directly dereference struct task_struct, struct sock, struct file, or other internal kernel structures. Prefer tracepoint arguments or stable helpers.
- Use the agent event envelope:
  timestamp_ns, plugin_id_hash, event_type, pid, tid, uid, gid, payload_len, payload[256]
- Encode payload TLV as field_id(uint16 little-endian) + field_len(uint16 little-endian) + raw value, with no field_type byte.
- Set payload_len=0 when no fields are needed, and match only event_type in Sigma.
- Keep numeric eBPF event_type values consistent with HookPlan or package metadata event_schema.events keys.

## HookPlan requirements

Place schema_version, package_id, and version before hooks. Use schema_version "aegis.ebpf_plugin.v1". Every hook must include name, attach_type, attach, and program.

Schema example only; placeholders must be replaced with exact entries from the ACTIVE HOOK ALLOWLIST and must never appear literally:
schema_version: "aegis.ebpf_plugin.v1"
package_id: "cve-2026-xxxxx"
version: "1.0.0"
hooks:
  - name: allowed_hook_name
    attach_type: tracepoint
    attach: category/allowed_event
    program: tracepoint__allowed_hook_name

## Output template

## Package Metadata
package_id, version, title, description, cve_ids

## Coverage Decision
Use a yaml code fence with exactly these fields:
status: supported or unsupported
reason: concise coverage rationale
covered_behaviors: list of exploit-chain behaviors directly observable through the selected hooks
uncovered_core_behaviors: list of required behaviors that cannot be observed
required_hooks: list of exact minimally required attach_type and attach pairs; use [] when no additional hooks are required

Only when status is supported and uncovered_core_behaviors is empty, continue with the artifact sections below.

## HookPlan
Use a yaml code fence containing metadata followed by hooks.

## eBPF Source Draft
Use a c code fence.

## Sigma Atomic Rules
Use a yaml code fence.

## Correlation DetectionSpec
Use a yaml code fence.

## Risks and Limitations
Describe detection boundaries and potential false positives.

## Safety Boundary

End with this statement:
This output is a draft. It must be edited by a human, compiled in the builder container, reviewed, signed, published, and enabled in the UI before an agent may install it.

Package ID:
%s

CVE:
%s

Vulnerability description:
%s

Attack prerequisites:
%s

Exploitation-chain behavior:
%s

False-positive and platform constraints:
%s`

// GetDetectionPackageGenerationPrompt returns the detection package generation prompt
func GetDetectionPackageGenerationPrompt(packageID, cveID, description, prerequisites, chain, constraints string) string {
	return fmt.Sprintf(DetectionPackageGenerationPrompt, packageID, cveID, description, prerequisites, chain, constraints)
}

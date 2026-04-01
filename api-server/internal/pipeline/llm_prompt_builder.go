package pipeline

import (
	"encoding/json"
	"fmt"
	"time"
)

// LLMAnalysisInput represents the input structure for LLM analysis
type LLMAnalysisInput struct {
	HostID      string         `json:"host_id"`
	WindowStart string         `json:"window_start"`
	WindowEnd   string         `json:"window_end"`
	Events      []RuntimeEvent `json:"events"`
}

// ToolDefinition represents a tool available to the LLM
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// BuildAnalysisPrompt builds the complete prompt for LLM analysis
func BuildAnalysisPrompt(window *HostWindow) (string, error) {
	input := LLMAnalysisInput{
		HostID:      window.HostID,
		WindowStart: window.WindowStart.Format(time.RFC3339),
		WindowEnd:   window.WindowEnd.Format(time.RFC3339),
		Events:      window.Events,
	}

	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal input: %w", err)
	}

	systemPrompt := getSystemPrompt()
	return fmt.Sprintf("%s\n\n事件数据：\n%s", systemPrompt, string(inputJSON)), nil
}

// getSystemPrompt returns the system prompt for LLM analysis
func getSystemPrompt() string {
	return `你是一个主机安全分析专家。分析以下2分钟窗口内的安全事件，判断是否存在真实威胁。

可用工具：
1. get_process_tree(pid) - 获取进程树，用于确认父子进程链
2. get_network_connections(pid) - 获取网络连接，用于确认外部连接
3. get_file_info(path) - 获取文件信息，用于确认文件属性
4. get_user_info(username) - 获取用户信息，用于确认用户权限

请返回 JSON 格式，包含以下字段：
{
  "alerts": [
    {
      "rule_id": "规则ID，如 reverse_shell_001",
      "rule_title": "规则名称，如 反弹Shell检测",
      "mitre_id": "T1059.004",
      "mitre_name": "Command and Scripting Interpreter",
      "severity": "critical|high|medium|low",
      "pid": 12345,
      "description": "详细威胁描述，说明检测到的威胁行为",
      "llm_summary": "简要分析结论，一句话概括威胁",
      "disposal_strategy": "处置建议，如 建议立即终止进程",
      "block_action": "kill_process|quarantine_file|block_connection",
      "block_target": "进程PID或文件路径",
      "judgment_source": "ai"
    }
  ],
  "tool_calls": [
    {
      "tool": "get_process_tree",
      "params": {"pid": 12345},
      "reason": "调用原因"
    }
  ],
  "rule_adjustments": [
    {
      "rule_id": "rule_id",
      "action": "tighten|loosen",
      "reason": "调整原因"
    }
  ]
}

分析原则：
1. 仔细分析每条事件的命令行和上下文
2. 区分正常操作和恶意行为
3. 考虑进程的父子关系
4. 如果需要更多信息才能判断，使用工具调用
5. 最多调用10次工具
6. 如果事件是误报，alerts数组可以为空
7. severity应根据MITRE ATT&CK技术的危险程度判断
8. 优先关注反弹shell、提权、数据渗出等高危行为
9. description和llm_summary必须填写，不能为空`
}

// BuildToolResultPrompt builds a prompt to include tool call results
func BuildToolResultPrompt(originalPrompt string, toolName string, result string) string {
	return fmt.Sprintf(`%s

工具调用结果：
工具: %s
结果: %s

请根据工具结果继续分析，返回更新后的 JSON 格式分析结果。`, originalPrompt, toolName, result)
}

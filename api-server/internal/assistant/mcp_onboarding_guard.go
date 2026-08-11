package assistant

import "strings"

// isMCPOnboardingRequest identifies requests that try to create, register, or
// connect an MCP server. V6.3 deliberately keeps these mutations in the MCP
// aggregation control plane; they must not be delegated to the Assistant LLM.
func isMCPOnboardingRequest(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}

	mcpTerms := []string{"mcp", "远程模型上下文协议", "远程 mcp", "remote mcp"}
	actionTerms := []string{
		"接入", "注册", "添加", "连接", "挂载", "接入到",
		"connect", "onboard", "onboarding", "register", "add",
	}
	return containsAnyTerm(query, mcpTerms) && containsAnyTerm(query, actionTerms)
}

func isMCPRelatedQuery(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return containsAnyTerm(query, []string{
		"mcp", "聚合", "aggregation", "aggregated", "remote", "远程",
	})
}

// shouldGuideMCPOnboarding also handles the second turn of the old flow. The
// previous implementation persisted a clarification such as “what object?”;
// when the user answered it with “当前系统内的 MCP 聚合方案”, the LLM then
// produced an unrelated “missing context” response. Only an MCP-related
// continuation is intercepted so an unrelated new request can still replace a
// pending clarification normally.
func shouldGuideMCPOnboarding(input RunInput) bool {
	if isMCPOnboardingRequest(input.UserMessage) || isMCPOnboardingRequest(input.OriginalUserMessage) {
		return true
	}
	return input.PendingClarification != nil &&
		isMCPOnboardingRequest(input.PendingClarification.OriginalQuery) &&
		isMCPRelatedQuery(input.UserMessage)
}

func containsAnyTerm(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func mcpOnboardingGuidance(locale string) string {
	return localized(locale,
		"当前智能体模式不能在普通对话中直接创建或接入远程 MCP Server。请先打开“MCP 聚合管控”页面：添加并审核远程服务，然后为该服务创建并授权专用 Client。将页面生成的 Client Key 和一次性 Token 以 MCP_ASSISTANT_CLIENT_KEY、MCP_ASSISTANT_CLIENT_TOKEN 注入 api-server，同时设置 MCP_ASSISTANT_ENABLED=true，并重启 api-server。完成后，智能体只能通过已授权的只读 MCP 聚合能力查询工具和调用结果。",
		"The Assistant cannot create or connect a remote MCP Server from normal chat. Open the MCP aggregation control page first, add and approve the remote service, then create and authorize a dedicated Client for it. Inject the generated Client Key and one-time Token into api-server as MCP_ASSISTANT_CLIENT_KEY and MCP_ASSISTANT_CLIENT_TOKEN, set MCP_ASSISTANT_ENABLED=true, and restart api-server. After that, the Assistant can use only the authorized read-only MCP aggregation and invocation tools.")
}

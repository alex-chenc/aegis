package assistant

import "strings"

// isMCPOnboardingRequest identifies requests that try to create, register, or
// connect an MCP server. The intent router uses this only to select the
// governed onboarding workflow; execution still goes through the LLM plan,
// exact capability mapping, approval, and the MCP platform service.
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

func containsAnyTerm(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

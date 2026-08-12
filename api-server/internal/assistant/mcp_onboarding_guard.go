package assistant

import "strings"

// isMCPClientAuthorizationRequest identifies requests that create a Client
// authorization for an existing MCP service. This check must run before the
// broader onboarding check because users commonly say "接入这个服务" while
// asking for a Client grant, not a new Server registration.
func isMCPClientAuthorizationRequest(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || !containsAnyFold(query, "mcp", "远程模型上下文协议", "remote mcp") {
		return false
	}
	if !containsAnyFold(query, "client", "客户端", "mcp client") ||
		!containsAnyFold(query, "授权", "authorization", "authorize", "grant") {
		return false
	}
	// A read-only request about existing grants must remain a query. A write
	// marker makes the intent explicit; "接入" is included for the common
	// wording "接入这个服务" when it is paired with a new Client grant.
	if containsAnyFold(query, "查询", "查看", "列出", "list", "query") &&
		!containsAnyFold(query, "新增", "新建", "创建", "添加", "生成", "开通", "create", "new") {
		return false
	}
	return containsAnyFold(query,
		"新增", "新建", "创建", "添加", "生成", "开通", "接入",
		"create", "new", "authorize", "grant",
	)
}

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

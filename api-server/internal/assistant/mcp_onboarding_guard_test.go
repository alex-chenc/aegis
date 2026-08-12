package assistant

import "testing"

func TestIsMCPClientAuthorizationRequest(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "new client grant for existing service", query: "已审批，新增一个 Client 授权，接入这个服务 Remote MCP aegis-mcp", want: true},
		{name: "read existing grants", query: "查询已有 MCP Client 授权", want: false},
		{name: "new remote server onboarding", query: "把这个接入到远程 MCP", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMCPClientAuthorizationRequest(tt.query); got != tt.want {
				t.Fatalf("isMCPClientAuthorizationRequest(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestIsMCPOnboardingRequest(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "Chinese connect", query: "把这个接入到远程 MCP", want: true},
		{name: "English register", query: "register this remote MCP server", want: true},
		{name: "MCP explanation", query: "解释当前系统内的 MCP 聚合方案", want: false},
		{name: "ordinary connect", query: "连接当前系统", want: false},
		{name: "MCP query", query: "查询 MCP 工具列表", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMCPOnboardingRequest(tt.query); got != tt.want {
				t.Fatalf("isMCPOnboardingRequest(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

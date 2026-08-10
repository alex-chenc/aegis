package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pb "api-server/pkg/api/v1"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const agentConfigScanTool = "AgentConfigScan"

type AgentConfigToolClient interface {
	ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error)
}

type AgentConfigSecurityService struct {
	client AgentConfigToolClient
	logger *zap.Logger
}

type AgentConfigScanResult struct {
	HostID       string                   `json:"host_id"`
	Hostname     string                   `json:"hostname,omitempty"`
	ScannedAt    time.Time                `json:"scanned_at"`
	Agents       []AgentConfigResultAgent `json:"agents"`
	Errors       []AgentConfigScanError   `json:"errors,omitempty"`
	FindingCount int                      `json:"finding_count"`
}

type AgentConfigResultAgent struct {
	AgentType    string                  `json:"agent_type"`
	DisplayName  string                  `json:"display_name"`
	Files        []AgentConfigResultFile `json:"files"`
	Hooks        []AgentConfigHook       `json:"hooks"`
	FindingCount int                     `json:"finding_count"`
}

type AgentConfigResultFile struct {
	Path       string               `json:"path"`
	Format     string               `json:"format"`
	Status     string               `json:"status"`
	Size       int64                `json:"size,omitempty"`
	Mode       string               `json:"mode,omitempty"`
	ModifiedAt time.Time            `json:"modified_at,omitempty"`
	SHA256     string               `json:"sha256,omitempty"`
	Content    string               `json:"content,omitempty"`
	Error      string               `json:"error,omitempty"`
	Findings   []AgentConfigFinding `json:"findings"`
}

type AgentConfigHook struct {
	FilePath  string               `json:"file_path"`
	FieldPath string               `json:"field_path"`
	Event     string               `json:"event"`
	Command   string               `json:"command"`
	Executor  string               `json:"executor,omitempty"`
	Findings  []AgentConfigFinding `json:"findings"`
}

type AgentConfigFinding struct {
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	FieldPath   string `json:"field_path"`
	Value       string `json:"value,omitempty"`
	Title       string `json:"title"`
	Reason      string `json:"reason"`
	Remediation string `json:"remediation"`
}

// AgentConfigRuleDefinition is the immutable catalog entry used by the
// configuration scanner. Keeping this catalog explicit makes the rules shown
// in the UI auditable and keeps them aligned with the event-awareness catalog.
type AgentConfigRuleDefinition struct {
	RuleKey           string   `json:"rule_key"`
	RuleVersion       int64    `json:"rule_version"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Source            string   `json:"source"`
	Engine            string   `json:"engine"`
	Categories        []string `json:"categories"`
	DefaultEnabled    bool     `json:"default_enabled"`
	DefaultSeverity   string   `json:"default_severity"`
	DefaultAction     string   `json:"default_action"`
	RecommendedAction string   `json:"recommended_action"`
	Immutable         bool     `json:"immutable"`
	Digest            string   `json:"digest"`
}

var builtinAgentConfigRules = []AgentConfigRuleDefinition{
	{RuleKey: "AGC-001", Name: "自动放行或跳过审批", Description: "检测智能体可在没有人工确认的情况下执行高风险操作。", Categories: []string{"permission"}, DefaultSeverity: "high"},
	{RuleKey: "AGC-002", Name: "无限制沙箱配置", Description: "检测配置是否允许智能体脱离预期隔离边界。", Categories: []string{"sandbox"}, DefaultSeverity: "critical"},
	{RuleKey: "AGC-003", Name: "全量工具权限放行", Description: "检测通配或全量 allow，避免智能体绕过最小权限边界。", Categories: []string{"permission"}, DefaultSeverity: "high"},
	{RuleKey: "AGC-004", Name: "OpenCode 全量权限规则", Description: "检测 action 或 resource 为通配符的 OpenCode 权限规则。", Categories: []string{"permission"}, DefaultSeverity: "high"},
	{RuleKey: "AGC-005", Name: "不受限网络访问", Description: "检测智能体是否可以访问任意外部网络资源。", Categories: []string{"network"}, DefaultSeverity: "high"},
	{RuleKey: "AGC-006", Name: "敏感凭据字段", Description: "检测配置中可能包含 Token、密钥、密码或私钥的字段。", Categories: []string{"secret"}, DefaultSeverity: "high"},
	{RuleKey: "AGC-007", Name: "未知或不可解析配置", Description: "检测配置文件读取失败、格式错误或权限模式无法确认的情况。", Categories: []string{"integrity"}, DefaultSeverity: "medium"},
	{RuleKey: "AGC-008", Name: "Hook 执行边界过宽", Description: "检测 Hook 使用 Shell、相对路径、可写目录或通配事件导致的执行边界扩大。", Categories: []string{"hook"}, DefaultSeverity: "high"},
	{RuleKey: "AGC-009", Name: "高风险 Hook 缺少保护", Description: "检测工具或会话 Hook 是否缺少失败策略、审批或来源校验。", Categories: []string{"hook"}, DefaultSeverity: "medium"},
}

func BuiltinAgentConfigRules() []AgentConfigRuleDefinition {
	result := make([]AgentConfigRuleDefinition, 0, len(builtinAgentConfigRules))
	for _, rule := range builtinAgentConfigRules {
		rule.RuleVersion = 1
		rule.Source = "builtin"
		rule.Engine = "api_config_static"
		rule.DefaultAction = "alert"
		rule.RecommendedAction = "alert"
		rule.DefaultEnabled = true
		rule.Immutable = true
		payload, _ := json.Marshal(struct {
			RuleKey string `json:"rule_key"`
			Version int64  `json:"rule_version"`
			Name    string `json:"name"`
			Desc    string `json:"description"`
			Engine  string `json:"engine"`
		}{rule.RuleKey, rule.RuleVersion, rule.Name, rule.Description, rule.Engine})
		digest := sha256.Sum256(payload)
		rule.Digest = "sha256:" + hex.EncodeToString(digest[:])
		result = append(result, rule)
	}
	return result
}

type AgentConfigScanError struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type rawAgentConfigSnapshot struct {
	HostID      string                `json:"host_id"`
	CollectedAt time.Time             `json:"collected_at"`
	Agents      []rawAgentConfigAgent `json:"agents"`
	Errors      []struct {
		Stage   string `json:"stage"`
		Message string `json:"message"`
	} `json:"errors"`
}

type rawAgentConfigAgent struct {
	AgentType   string               `json:"agent_type"`
	DisplayName string               `json:"display_name"`
	Files       []rawAgentConfigFile `json:"files"`
}

type rawAgentConfigFile struct {
	Path       string    `json:"path"`
	Format     string    `json:"format"`
	Status     string    `json:"status"`
	Size       int64     `json:"size"`
	Mode       string    `json:"mode"`
	ModifiedAt time.Time `json:"modified_at"`
	SHA256     string    `json:"sha256"`
	Content    string    `json:"content"`
	Error      string    `json:"error"`
}

func NewAgentConfigSecurityService(client AgentConfigToolClient, logger *zap.Logger) *AgentConfigSecurityService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentConfigSecurityService{client: client, logger: logger}
}

func (s *AgentConfigSecurityService) Scan(ctx context.Context, hostID string) (*AgentConfigScanResult, error) {
	if strings.TrimSpace(hostID) == "" {
		return nil, fmt.Errorf("host_id is required")
	}
	if s.client == nil {
		return nil, fmt.Errorf("agent configuration tool client is unavailable")
	}
	args, _ := json.Marshal(map[string]string{"host_id": hostID})
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := s.client.ExecuteTool(callCtx, fmt.Sprintf("agent-config-%d", time.Now().UnixNano()), hostID, agentConfigScanTool, string(args), 30)
	if err != nil {
		return nil, fmt.Errorf("agent configuration scan failed: %w", err)
	}
	if resp == nil || !resp.Success {
		message := "agent configuration scan failed"
		if resp != nil && resp.Error != "" {
			message = resp.Error
		}
		return nil, fmt.Errorf("%s", message)
	}
	var raw rawAgentConfigSnapshot
	if err := json.Unmarshal([]byte(resp.Result), &raw); err != nil {
		return nil, fmt.Errorf("agent configuration result is invalid: %w", err)
	}
	result := &AgentConfigScanResult{
		HostID:    raw.HostID,
		ScannedAt: raw.CollectedAt,
		Agents:    make([]AgentConfigResultAgent, 0),
		Errors:    make([]AgentConfigScanError, 0),
	}
	if result.HostID == "" {
		result.HostID = hostID
	}
	for _, item := range raw.Errors {
		result.Errors = append(result.Errors, AgentConfigScanError{Stage: item.Stage, Message: item.Message})
	}
	for _, rawAgent := range raw.Agents {
		agent := AgentConfigResultAgent{
			AgentType:   rawAgent.AgentType,
			DisplayName: rawAgent.DisplayName,
			Files:       make([]AgentConfigResultFile, 0),
			Hooks:       make([]AgentConfigHook, 0),
		}
		for _, rawFile := range rawAgent.Files {
			file := AgentConfigResultFile{
				Path:       rawFile.Path,
				Format:     rawFile.Format,
				Status:     rawFile.Status,
				Size:       rawFile.Size,
				Mode:       rawFile.Mode,
				ModifiedAt: rawFile.ModifiedAt,
				SHA256:     rawFile.SHA256,
				Content:    rawFile.Content,
				Error:      rawFile.Error,
				Findings:   make([]AgentConfigFinding, 0),
			}
			if rawFile.Status != "ok" {
				file.Findings = append(file.Findings, configFinding("AGC-007", "medium", "", rawFile.Status, "配置文件未能安全读取", "文件状态不是 ok，不能将其视为安全配置。", "确认文件权限、路径和 Agent 版本后重新扫描。"))
			} else if values, parseErr := parseAgentConfig(rawFile.Format, rawFile.Content); parseErr != nil {
				file.Findings = append(file.Findings, configFinding("AGC-007", "medium", "", "parse_error", "配置格式无法解析", "配置文件无法解析，字段安全状态未知。", "修复配置格式后重新扫描。"))
			} else {
				file.Findings = evaluateConfig(rawAgent.AgentType, rawFile.Path, values)
				agent.Hooks = append(agent.Hooks, extractHooks(rawAgent.AgentType, rawFile.Path, values)...)
			}
			agent.FindingCount += len(file.Findings)
			for _, hook := range agent.Hooks {
				agent.FindingCount += len(hook.Findings)
			}
			agent.Files = append(agent.Files, file)
		}
		// Hook findings are counted after the full file set is built to avoid
		// counting the same hook once per file.
		agent.FindingCount = 0
		for _, file := range agent.Files {
			agent.FindingCount += len(file.Findings)
		}
		for _, hook := range agent.Hooks {
			agent.FindingCount += len(hook.Findings)
		}
		result.FindingCount += agent.FindingCount
		result.Agents = append(result.Agents, agent)
	}
	s.logger.Info("agent_configuration_scan_completed", zap.String("host_id", hostID), zap.Int("agent_count", len(result.Agents)), zap.Int("finding_count", result.FindingCount), zap.Int("error_count", len(result.Errors)))
	return result, nil
}

func parseAgentConfig(format, content string) (map[string]any, error) {
	var value map[string]any
	switch strings.ToLower(format) {
	case "json":
		err := json.Unmarshal([]byte(content), &value)
		return value, err
	case "jsonc":
		cleaned := stripJSONComments(content)
		err := json.Unmarshal([]byte(cleaned), &value)
		return value, err
	case "toml":
		err := toml.Unmarshal([]byte(content), &value)
		return value, err
	case "yaml", "yml":
		var raw any
		if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
			return nil, err
		}
		normalized, ok := normalizeMap(raw).(map[string]any)
		if !ok {
			return map[string]any{}, nil
		}
		return normalized, nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func evaluateConfig(agentType, filePath string, values map[string]any) []AgentConfigFinding {
	var findings []AgentConfigFinding
	var walk func(any, string)
	walk = func(value any, path string) {
		switch node := value.(type) {
		case map[string]any:
			for key, child := range node {
				fieldPath := joinConfigPath(path, key)
				lowerKey := strings.ToLower(key)
				if isSensitiveKey(lowerKey) {
					findings = append(findings, configFinding("AGC-006", "high", fieldPath, "***", "发现敏感凭据字段", "配置字段可能包含 Token、密钥或密码。", "改用 Agent 支持的环境变量或密钥存储，并轮换已暴露凭据。"))
				}
				if isPermissionField(lowerKey) && !isKnownPermissionValue(child) {
					findings = append(findings, configFinding("AGC-007", "medium", fieldPath, safeValue(child), "发现未知权限模式", "权限字段不是当前规则认识的安全模式，实际边界无法确认。", "核对智能体版本文档并将权限配置为明确的受限模式。"))
				}
				if isHighPrivilegeValue(lowerKey, child) {
					findings = append(findings, privilegeFinding(agentType, fieldPath, child))
				}
				if isGlobalAllowRule(lowerKey, child) {
					findings = append(findings, configFinding("AGC-003", "high", fieldPath, safeValue(child), "检测到全量工具权限放行", "通配或全量 allow 会让智能体绕过最小权限边界。", "改为 ask/deny，并按工具和资源配置最小范围 allow。"))
				}
				if lowerKey == "permissions" {
					findings = append(findings, permissionRuleFindings(fieldPath, child)...)
				}
				if lowerKey == "tools" {
					findings = append(findings, legacyToolFindings(fieldPath, child)...)
				}
				walk(child, fieldPath)
			}
		case []any:
			for i, child := range node {
				walk(child, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(values, "")
	return uniqueFindings(findings)
}

func privilegeFinding(agentType, path string, value any) AgentConfigFinding {
	key := strings.ToLower(filepath.Base(path))
	if key == "network_access" || key == "network" {
		return configFinding("AGC-005", "high", path, safeValue(value), "检测到不受限网络访问", "智能体可以访问任意外部网络资源，扩大了数据外传和远程执行面。", "改为按域名/地址白名单限制网络访问。")
	}
	if key == "sandbox_mode" || key == "sandbox" || strings.Contains(key, "isolation") {
		return configFinding("AGC-002", "critical", path, safeValue(value), "检测到无限制沙箱配置", fmt.Sprintf("%s 配置允许智能体脱离预期隔离边界。", agentType), "启用 read-only/workspace-write 或等价受限沙箱。")
	}
	return configFinding("AGC-001", "high", path, safeValue(value), "检测到自动放行或跳过审批", "智能体可在没有人工确认的情况下执行高风险操作。", "启用 ask/on-request，并为高风险工具保留明确审批。")
}

func permissionRuleFindings(path string, value any) []AgentConfigFinding {
	var result []AgentConfigFinding
	switch node := value.(type) {
	case string:
		if strings.EqualFold(node, "allow") {
			result = append(result, configFinding("AGC-003", "high", path, node, "检测到全量权限 allow", "所有权限默认自动放行。", "将默认权限改为 ask，并只对白名单资源放行。"))
		}
	case map[string]any:
		for key, child := range node {
			if (key == "*" || isDangerousTool(key)) && strings.EqualFold(fmt.Sprint(child), "allow") {
				result = append(result, configFinding("AGC-003", "high", joinConfigPath(path, key), "allow", "检测到高风险工具自动放行", "shell/edit/exec 等工具无需审批即可执行。", "按工具、命令和资源配置最小权限。"))
			}
		}
	case []any:
		for i, item := range node {
			rule, ok := item.(map[string]any)
			if !ok {
				continue
			}
			effect, _ := rule["effect"].(string)
			action, _ := rule["action"].(string)
			resource, _ := rule["resource"].(string)
			if strings.EqualFold(effect, "allow") && (action == "*" || resource == "*" || isDangerousTool(action)) {
				result = append(result, configFinding("AGC-004", "high", fmt.Sprintf("%s[%d]", path, i), "allow", "检测到 OpenCode 全量权限规则", "权限规则允许任意动作或任意资源。", "将 action/resource 改为明确白名单并保留 ask/deny 默认值。"))
			}
		}
	}
	return result
}

func legacyToolFindings(path string, value any) []AgentConfigFinding {
	result := []AgentConfigFinding{}
	if node, ok := value.(map[string]any); ok {
		for key, child := range node {
			if (key == "*" || isDangerousTool(key)) && child == true {
				result = append(result, configFinding("AGC-003", "high", joinConfigPath(path, key), "true", "检测到 legacy tools 全量启用", "旧版工具开关会把高风险工具直接开放给智能体。", "迁移到按工具和资源划分的 ask/deny/allow 规则。"))
			}
		}
	}
	return result
}

func extractHooks(agentType, filePath string, values map[string]any) []AgentConfigHook {
	var hooks []AgentConfigHook
	var walk func(any, string, string)
	walk = func(value any, path, event string) {
		switch node := value.(type) {
		case map[string]any:
			currentEvent := event
			if currentEvent == "" {
				currentEvent = hookEventFromPath(path)
			}
			command := firstString(node, "command", "cmd", "script", "run", "path", "command_line")
			if command != "" {
				hook := AgentConfigHook{
					FilePath:  filePath,
					FieldPath: path,
					Event:     currentEvent,
					Command:   maskCommand(command),
					Executor:  firstString(node, "executor", "type", "shell"),
					Findings:  make([]AgentConfigFinding, 0),
				}
				if isShellHook(hook.Executor, command) || !isSafeHookPath(command) || strings.Contains(currentEvent, "*") {
					hook.Findings = append(hook.Findings, configFinding("AGC-008", "high", path, hook.Command, "Hook 执行边界过宽", "Hook 使用 shell、相对路径、可写目录或通配事件，可能被配置篡改或扩大执行范围。", "使用绝对且不可写的脚本路径，限制事件范围并避免 shell -c。"))
				}
				if isHighRiskHookEvent(currentEvent) && !hasHookGuard(node) {
					hook.Findings = append(hook.Findings, configFinding("AGC-009", "medium", path, hook.Command, "高风险 Hook 缺少显式保护", "工具或会话 Hook 没有发现失败策略、审批或来源校验字段。", "为 Hook 增加失败策略、来源签名校验和必要的审批边界。"))
				}
				hooks = append(hooks, hook)
			}
			for key, child := range node {
				nextEvent := currentEvent
				if (strings.Contains(strings.ToLower(key), "hook") && strings.ToLower(key) != "hooks") || isHookEventKey(key) {
					nextEvent = key
				}
				walk(child, joinConfigPath(path, key), nextEvent)
			}
		case []any:
			for i, child := range node {
				walk(child, fmt.Sprintf("%s[%d]", path, i), currentEventFromPath(path, event))
			}
		}
	}
	walk(values, "", "")
	return hooks
}

func normalizeMap(value any) any {
	switch node := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range node {
			out[key] = normalizeMap(child)
		}
		return out
	case map[any]any:
		out := map[string]any{}
		for key, child := range node {
			out[fmt.Sprint(key)] = normalizeMap(child)
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, child := range node {
			out[i] = normalizeMap(child)
		}
		return out
	default:
		return value
	}
}

func stripJSONComments(input string) string {
	var out strings.Builder
	inString, escaped, lineComment, blockComment := false, false, false, false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		next := byte(0)
		if i+1 < len(input) {
			next = input[i+1]
		}
		if lineComment {
			if ch == '\n' {
				lineComment = false
				out.WriteByte(ch)
			}
			continue
		}
		if blockComment {
			if ch == '*' && next == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == '/' && next == '/' {
			lineComment = true
			i++
			continue
		}
		if ch == '/' && next == '*' {
			blockComment = true
			i++
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func joinConfigPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}
func currentEventFromPath(path, event string) string {
	if event != "" {
		return event
	}
	return hookEventFromPath(path)
}
func hookEventFromPath(path string) string {
	if path == "" {
		return "unknown"
	}
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}
func isHookEventKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "session") || strings.Contains(lower, "tool") || strings.Contains(lower, "command") || strings.Contains(lower, "event")
}
func isHighRiskHookEvent(event string) bool {
	lower := strings.ToLower(event)
	return strings.Contains(lower, "tool") || strings.Contains(lower, "exec") || strings.Contains(lower, "command") || strings.Contains(lower, "session")
}
func firstString(node map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := node[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func hasHookGuard(node map[string]any) bool {
	for key := range node {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "fail") || strings.Contains(lower, "approv") || strings.Contains(lower, "sign") || strings.Contains(lower, "verify") || strings.Contains(lower, "source") {
			return true
		}
	}
	return false
}
func isShellHook(executor, command string) bool {
	lower := strings.ToLower(executor + " " + command)
	return strings.Contains(lower, "shell") || strings.Contains(lower, "sh -c") || strings.Contains(lower, "bash -c") || strings.Contains(lower, "cmd /c")
}
func isSafeHookPath(command string) bool {
	first := strings.Fields(command)
	if len(first) == 0 {
		return false
	}
	path := strings.Trim(first[0], "\"'")
	return filepath.IsAbs(path) && !strings.HasPrefix(path, "/tmp/") && !strings.HasPrefix(path, "/var/tmp/")
}
func maskCommand(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "token=") || strings.Contains(lower, "secret=") || strings.Contains(lower, "password=") {
		return "***"
	}
	return value
}
func isMasked(value any) bool { return strings.Contains(fmt.Sprint(value), "***") }
func isSensitiveKey(key string) bool {
	for _, marker := range []string{"password", "passwd", "token", "secret", "api_key", "api-key", "access_key", "access-key", "private_key", "private-key", "cookie"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func isPermissionField(key string) bool {
	for _, field := range []string{"approval_policy", "defaultmode", "sandbox_mode", "permission_mode", "network_access"} {
		if key == field {
			return true
		}
	}
	return false
}

func isKnownPermissionValue(value any) bool {
	text := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	if text == "" || strings.HasPrefix(text, "map[") || strings.HasPrefix(text, "[") {
		return true
	}
	for _, allowed := range []string{"ask", "on-request", "on-failure", "untrusted", "never", "read-only", "workspace-write", "danger-full-access", "plan", "default", "bypass", "true", "false"} {
		if text == allowed {
			return true
		}
	}
	return false
}
func isDangerousTool(key string) bool {
	lower := strings.ToLower(key)
	return lower == "bash" || lower == "shell" || lower == "exec" || lower == "write" || lower == "edit" || lower == "patch" || lower == "command"
}
func isGlobalAllowRule(key string, value any) bool {
	lower := strings.ToLower(key)
	if lower != "permission" && lower != "approval" && lower != "sandbox" && lower != "network_access" && lower != "defaultmode" && lower != "skip_approval" && lower != "auto_approve" && lower != "dangerously_skip_permissions" && lower != "sandbox_mode" {
		return false
	}
	text := strings.ToLower(fmt.Sprint(value))
	return text == "never" || text == "bypass" || text == "danger-full-access" || text == "true" || text == "off" || text == "disabled" || text == "unrestricted" || text == "allow"
}
func isHighPrivilegeValue(key string, value any) bool { return isGlobalAllowRule(key, value) }
func safeValue(value any) string {
	text := fmt.Sprint(value)
	if len(text) > 120 {
		return text[:120] + "…"
	}
	return text
}
func configFinding(rule, severity, path, value, title, reason, remediation string) AgentConfigFinding {
	return AgentConfigFinding{RuleID: rule, Severity: severity, FieldPath: path, Value: value, Title: title, Reason: reason, Remediation: remediation}
}
func uniqueFindings(items []AgentConfigFinding) []AgentConfigFinding {
	seen := map[string]bool{}
	result := make([]AgentConfigFinding, 0, len(items))
	for _, item := range items {
		key := item.RuleID + "|" + item.FieldPath
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Severity > result[j].Severity })
	return result
}

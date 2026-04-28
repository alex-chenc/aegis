# Aegis智能主机安全系统 V5.6 后端详细设计文档

**版本**: 5.6
**日期**: 2026-04-14
**状态**: 设计中

---

## 1. 概述

### 1.1 设计目标

V5.6后端主要新增以下功能：

| 功能 | 说明 |
|------|------|
| Sigma规则解析服务 | 解析上传的Sigma规则YAML文件 |
| LangChain Agent服务 | 多轮对话式AI分析 |
| 工具调用服务 | AI调用Agent工具的能力 |
| 单Host精确下发 | 所有命令精确下发到指定Agent |
| 登录认证与首次改密 | 首次部署允许无账号密码进入，进入后强制设置账号密码，后续使用账号密码登录 |

---

## 2.5 登录认证服务

### 2.5.1 设计目标

认证模块只解决控制台登录和首次初始化，不引入多角色权限系统。系统初始状态下没有可长期使用的默认账号密码，管理员可在登录页点击“首次进入”获得一次受限会话；受限会话只能访问当前用户信息、退出登录和修改账号密码接口。修改完成后，首次入口关闭，后续必须使用本次设置的账号密码登录。

### 2.5.2 状态流

```text
未初始化
  -> POST /api/v1/auth/bootstrap-login
  -> force_password_change=true 的临时会话
  -> POST /api/v1/auth/change-credentials
  -> initialized=true / force_password_change=false
  -> POST /api/v1/auth/login
```

### 2.5.3 API 设计

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | `/api/v1/auth/status` | 否 | 返回是否已初始化 |
| POST | `/api/v1/auth/bootstrap-login` | 否 | 仅未初始化时允许首次进入 |
| POST | `/api/v1/auth/login` | 否 | 使用已设置账号密码登录 |
| GET | `/api/v1/auth/me` | 是 | 返回当前用户和是否强制改密 |
| POST | `/api/v1/auth/change-credentials` | 是 | 设置或修改账号密码 |
| POST | `/api/v1/auth/logout` | 是 | 删除当前会话 |

### 2.5.4 鉴权策略

- 客户端使用 `Authorization: Bearer <token>` 调用受保护 API。
- 后端只保存 token 的 SHA-256 摘要，明文 token 只在登录响应中返回一次。
- 密码使用 bcrypt 哈希保存。
- `force_password_change=true` 的会话只允许访问 `/auth/me`、`/auth/change-credentials`、`/auth/logout`，其他业务 API 返回 `403`。
- `/health`、`/api/v1/auth/*` 和 Agent 安装/下载脚本保持未鉴权，避免破坏 Agent 安装链路。
- 前端路由守卫不能作为安全边界。即使用户手工修改 URL 或前端代码，后端仍必须拒绝未认证请求访问所有业务 API。

### 2.5.5 验收测试

- 未初始化时，`bootstrap-login` 返回 token 且 `force_password_change=true`。
- 未初始化临时会话访问业务 API 返回 `403`。
- 无 token 访问任意业务 API 返回 `401`，不能因为前端路由被绕过而返回业务数据。
- 临时会话提交新账号密码后，`login` 可用新凭据成功登录。
- 完成初始化后再次调用 `bootstrap-login` 返回 `403`。
- 错误密码登录返回 `401`。

## 2. 新增服务模块

### 2.1 项目结构

```
api-server/
├── internal/
│   ├── handler/
│   │   ├── detection_handler.go    # 检测相关处理器
│   │   ├── sigma_rule_handler.go   # Sigma规则处理器 (增强)
│   │   └── ai_analysis_handler.go  # AI分析处理器 (新增)
│   │
│   ├── service/
│   │   ├── sigma_rule_service.go   # Sigma规则服务 (增强)
│   │   ├── ai_analysis_service.go  # AI分析服务 (新增)
│   │   └── llm/
│   │       ├── client.go           # LLM客户端
│   │       └── prompts.go          # Prompt模板
│   │       └── langchain/          # LangChain模块 (新增)
│   │           ├── agent.go
│   │           ├── memory.go
│   │           ├── tools/
│   │           │   ├── registry.go
│   │           │   ├── process_tree.go
│   │           │   ├── network.go
│   │           │   └── logs.go
│   │           └── executor.go
│   │
│   ├── repository/
│   │   ├── sigma_rule_repo.go
│   │   ├── ai_session_repo.go      # 新增
│   │   └── ai_message_repo.go      # 新增
│   │
│   ├── model/
│   │   ├── sigma_rule.go
│   │   ├── ai_session.go           # 新增
│   │   └── ai_message.go           # 新增
│   │
│   └── grpc/
│       └── client.go               # gRPC客户端
│
server/
├── internal/
│   ├── grpc_server/
│   │   ├── server.go               # 增强：工具调用路由
│   │   └── tool_executor.go        # 新增：Server端工具执行器
│   └── agent_registry.go           # Agent注册表
│
agent/
├── internal/
│   ├── tool_executor.go            # 新增：Agent端工具执行器
│   ├── tools/
│   │   ├── process_tree.go
│   │   ├── network.go
│   │   ├── files.go
│   │   ├── processes.go
│   │   ├── sessions.go
│   │   └── logs.go
│   └── ebpf/                       # 现有模块
│
dc/
└── internal/
    └── pipeline/                   # 现有模块
```

---

## 3. Sigma规则解析服务

### 3.1 Sigma规则解析器

```go
// api-server/internal/fileparser/sigma_parser.go
package fileparser

import (
    "fmt"
    "io"
    "regexp"
    "strings"

    "gopkg.in/yaml.v3"
)

// SigmaParser Sigma规则解析器
type SigmaRuleParser struct{}

// SigmaRule Sigma规则结构
type SigmaRule struct {
    Title       string   `yaml:"title"`
    ID         string   `yaml:"id"`
    Status     string   `yaml:"status"`
    Description string   `yaml:"description"`
    Tags       []string `yaml:"tags"`
    Level      string   `yaml:"level"`
    Logsource  struct {
        Category string `yaml:"category"`
        Product  string `yaml:"product"`
        Service  string `yaml:"service"`
    } `yaml:"logsource"`
    Detection  map[string]interface{} `yaml:"detection"`
    Fields     []string `yaml:"fields"`
    FalsePositives []string `yaml:"falsepositives"`
}

// Parse 从YAML内容解析Sigma规则
func (p *SigmaRuleParser) Parse(content []byte) (*SigmaRule, error) {
    var rule SigmaRule
    if err := yaml.Unmarshal(content, &rule); err != nil {
        return nil, fmt.Errorf("failed to parse YAML: %w", err)
    }

    // 验证必填字段
    if rule.Title == "" {
        return nil, fmt.Errorf("title is required")
    }
    if len(rule.Detection) == 0 {
        return nil, fmt.Errorf("detection is required")
    }

    // 生成规则ID（如果未提供）
    if rule.ID == "" {
        rule.ID = generateRuleID(rule.Title)
    }

    // 设置默认状态
    if rule.Status == "" {
        rule.Status = "experimental"
    }

    // 解析MITRE ID
    rule.Tags = parseMITRETags(rule.Tags, rule.Title)

    return &rule, nil
}

// ParseMITRETags 从tags和title解析MITRE ID
func parseMITRETags(tags []string, title string) []string {
    mitreTags := []string{}
    mitreRegex := regexp.MustCompile(`T\d{4}(?:\.\d{3})?`)

    // 从tags中提取
    for _, tag := range tags {
        matches := mitreRegex.FindAllString(tag, -1)
        mitreTags = append(mitreTags, matches...)
    }

    // 从title中提取
    titleMatches := mitreRegex.FindAllString(title, -1)
    mitreTags = append(mitreTags, titleMatches...)

    // 去重
    seen := make(map[string]bool)
    result := []string{}
    for _, t := range mitreTags {
        upper := strings.ToUpper(t)
        if !seen[upper] {
            seen[upper] = true
            result = append(result, upper)
        }
    }

    return result
}

// generateRuleID 从title生成规则ID
func generateRuleID(title string) string {
    // 转小写，替换空格和特殊字符
    id := strings.ToLower(title)
    id = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(id, "_")
    id = strings.Trim(id, "_")
    // 截断长度
    if len(id) > 50 {
        id = id[:50]
    }
    // 添加随机后缀
    return fmt.Sprintf("%s_%s", id, randomString(6))
}
```

### 3.2 Sigma规则服务

```go
// api-server/internal/service/sigma_rule_service.go

type SigmaRuleService struct {
    ruleRepo     *repository.SigmaRuleRepository
    serverClient *grpcclient.ServerClient
    parser       *fileparser.SigmaRuleParser
}

func NewSigmaRuleService(...) *SigmaRuleService {
    return &SigmaRuleService{
        ruleRepo:     ruleRepo,
        serverClient: serverClient,
        parser:       &fileparser.SigmaRuleParser{},
    }
}

// UploadRules 上传并解析Sigma规则文件
func (s *SigmaRuleService) UploadRules(file io.Reader, fileName string) (*UploadResult, error) {
    ext := strings.ToLower(path.Ext(fileName))

    switch ext {
    case ".yaml", ".yml":
        return s.parseSingleFile(file, fileName)
    case ".zip":
        return s.parseZipFile(file)
    default:
        return nil, fmt.Errorf("unsupported file format: %s", ext)
    }
}

// parseSingleFile 解析单个YAML文件
func (s *SigmaRuleService) parseSingleFile(file io.Reader, fileName string) (*UploadResult, error) {
    content, err := io.ReadAll(file)
    if err != nil {
        return nil, fmt.Errorf("failed to read file: %w", err)
    }

    rule, err := s.parser.Parse(content)
    if err != nil {
        return &UploadResult{
            Success:      false,
            ParsedCount:  0,
            FailedCount:  1,
            FailedFiles:  []string{fileName},
            Error:       err.Error(),
        }, nil
    }

    // 保存到数据库
    model := &model.SigmaRule{
        RuleID:      rule.ID,
        Title:       rule.Title,
        Description: rule.Description,
        Content:     string(content),
        Status:      "pending",
        Source:      "upload",
        FileName:    fileName,
        FileHash:    sha256Hash(content),
        MITREID:     strings.Join(rule.Tags, ","),
        Severity:    rule.Level,
        ParsedAt:    time.Now(),
    }

    if err := s.ruleRepo.Create(model); err != nil {
        return nil, err
    }

    return &UploadResult{
        Success:     true,
        ParsedCount: 1,
        Rules: []ParsedRule{
            {RuleID: rule.ID, Title: rule.Title, Status: "pending"},
        },
    }, nil
}

// ApproveRule 审批规则并精确下发
func (s *SigmaRuleService) ApproveRule(ruleID string, targetHostIDs []string) error {
    rule, err := s.ruleRepo.FindByID(ruleID)
    if err != nil {
        return err
    }

    // 更新状态
    if err := s.ruleRepo.UpdateStatus(ruleID, "active"); err != nil {
        return err
    }

    // 精确下发到目标Agent
    return s.dispatchRuleToHosts(rule, targetHostIDs)
}

// dispatchRuleToHosts 精确下发规则到指定主机
func (s *SigmaRuleService) dispatchRuleToHosts(rule *model.SigmaRule, hostIDs []string) error {
    if len(hostIDs) == 0 {
        // 空数组表示全量下发
        return s.broadcastRuleToAllHosts(rule)
    }

    for _, hostID := range hostIDs {
        err := s.serverClient.UpdateAgentRulesForHost(
            context.Background(),
            hostID,
            &pb.UpdateAgentRulesRequest{
                Action: "incremental",
                Rules: []*pb.AgentRuleUpdate{
                    {
                        RuleId:  rule.RuleID,
                        Action:  "add",
                        Content: rule.Content,
                    },
                },
            },
        )
        if err != nil {
            logger.Warn("failed to dispatch rule to host",
                zap.String("host_id", hostID),
                zap.String("rule_id", rule.RuleID),
                zap.Error(err))
        }
    }
    return nil
}
```

---

## 4. AI分析服务 (LangChain)

### 4.1 AI会话管理

```go
// api-server/internal/service/ai_analysis_service.go

type AIAnalysisService struct {
    sessionRepo     *repository.AISessionRepository
    messageRepo     *repository.AIMessageRepository
    serverClient    *grpcclient.ServerClient
    agent           *langchain.ReActAgent
    attackGraphSvc  *AttackGraphService
    vectorSvc       *VectorService
}

type CreateSessionRequest struct {
    AlertIDs      []string    `json:"alert_ids"`
    TimeRange     *TimeRange  `json:"time_range"`
    HostFilter    []string    `json:"host_filter"`
    MaxIterations int        `json:"max_iterations"` // 最大ReAct迭代次数，默认15，范围1-100；超过50时第50轮后强制输出Final Answer
}

type TimeRange struct {
    Start time.Time `json:"start"`
    End   time.Time `json:"end"`
}

func (s *AIAnalysisService) CreateSession(req *CreateSessionRequest) (*AISession, error) {
    session := &model.AISession{
        SessionID:    generateSessionID(),
        AlertIDs:     req.AlertIDs,
        TimeRange:    req.TimeRange,
        HostFilter:   req.HostFilter,
        Status:       "active",
    }

    if err := s.sessionRepo.Create(session); err != nil {
        return nil, err
    }

    // 初始化LangChain Agent
    s.agent.InitSession(session.SessionID, s.buildInitialContext(req))

    return session, nil
}

// SendMessage 发送消息并获取AI响应
func (s *AIAnalysisService) SendMessage(sessionID string, userMessage string) (*AIMessage, error) {
    session, err := s.sessionRepo.FindBySessionID(sessionID)
    if err != nil {
        return nil, err
    }

    // 保存用户消息
    userMsg := &model.AIMessage{
        SessionID: sessionID,
        Role:      "user",
        Content:   userMessage,
    }
    if err := s.messageRepo.Create(userMsg); err != nil {
        return nil, err
    }

    // 获取历史消息用于上下文
    history, err := s.messageRepo.GetBySessionID(sessionID)
    if err != nil {
        return nil, err
    }

    // 调用LangChain Agent
    response, err := s.agent.Invoke(userMessage, history)
    if err != nil {
        return nil, err
    }

    // 保存AI响应
    aiMsg := &model.AIMessage{
        SessionID:   sessionID,
        Role:        "assistant",
        Content:     response.Content,
        ToolCalls:  response.ToolCalls,
    }
    if err := s.messageRepo.Create(aiMsg); err != nil {
        return nil, err
    }

    return aiMsg, nil
}

// SubmitToolResult 提交工具执行结果
func (s *AIAnalysisService) SubmitToolResult(sessionID, callID string, result interface{}) error {
    // 更新消息记录
    return s.messageRepo.UpdateToolResult(sessionID, callID, result)
}

// ApplyConclusion 应用分析结论
func (s *AIAnalysisService) ApplyConclusion(sessionID string, conclusions []AlertConclusion) error {
    for _, c := range conclusions {
        switch c.Action {
        case "mark_false_positive":
            // 标记为误报
            if err := s.alertService.MarkAsFalsePositive(c.AlertID); err != nil {
                return err
            }
        case "confirm_threat":
            // 确认为威胁
            if err := s.alertService.ConfirmThreat(c.AlertID); err != nil {
                return err
            }
        case "generate_rule":
            // 生成新规则
            if err := s.ruleGenerationService.GenerateFromAlert(c.AlertID); err != nil {
                return err
            }
        }
    }
    return nil
}
```

### 4.2 ReAct Agent（借鉴 langchaingo 模式）

**借鉴 langchaingo 的 ReAct 模式实现，不直接依赖 langchaingo 库**

```go
// api-server/internal/service/llm/react_agent.go

// ToolExecutor 执行工具的接口
type ToolExecutor interface {
    Execute(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error)
}

type ReActAgent struct {
    llmClient     *LLMClient
    toolExecutor  ToolExecutor
    maxIterations int
    sessionID     string
    steps         []AgentStep
}

type AgentStep struct {
    Thought     string                 `json:"thought"`
    Action      string                 `json:"action"`
    ActionInput map[string]interface{} `json:"action_input"`
    Observation string                 `json:"observation"`
}

type AgentResponse struct {
    Content    string       `json:"content"`
    Steps      []AgentStep `json:"steps"`       // 思考过程
    ToolCalls  []*ToolCall `json:"tool_calls"`
    SessionID  string       `json:"session_id"`
}

// SSEWriter 流式输出接口
type SSEWriter interface {
    WriteThinking(content string) error
    WriteToolCall(tool, callID string, args interface{}) error
    WriteToolResult(callID string, result interface{}, timeMs int64) error
    WriteContent(content string) error
    WriteDone() error
}

// SSE 流式事件
type SSEEvent struct {
    Type    string      `json:"type"`    // thinking | tool_call | tool_result | content | done | error
    Content string      `json:"content"`
    Tool    string      `json:"tool,omitempty"`
    CallID  string      `json:"call_id,omitempty"`
    Args    interface{} `json:"args,omitempty"`
    Result  interface{} `json:"result,omitempty"`
    TimeMs  int64       `json:"time_ms,omitempty"`
    Error   string      `json:"error,omitempty"`
}

func NewReActAgent(llmClient *LLMClient, toolExecutor ToolExecutor, sessionID string, maxIterations int) *ReActAgent {
    if maxIterations <= 0 {
        maxIterations = 15 // 默认值
    }
    return &ReActAgent{
        llmClient:     llmClient,
        toolExecutor:   toolExecutor,
        maxIterations: maxIterations,
        sessionID:     sessionID,
    }
}

// Stream 流式执行（SSE）- 完整ReAct循环
func (a *ReActAgent) Stream(ctx context.Context, userMessage string, history []*AIMessage, writer *SSEWriter, context map[string]interface{}) error {
    prompt := BuildReActPrompt(userMessage, history, context)

    for iteration < a.maxIterations {
        // 使用流式LLM调用
        stream, err := a.llmClient.ChatCompletionStreamWithMessages(ctx, prompt, 0.7)
        if err != nil {
            writer.WriteError(fmt.Sprintf("LLM stream failed: %v", err))
            return err
        }

        // 处理流式响应，逐步解析 Thought/Action/Action Input
        // 关键：只有同时具有 Action 和 ActionInput 时才执行工具
        for {
            chunk, err := stream.Recv()
            if err == io.EOF {
                break
            }

            // 尝试解析完整步骤
            if step, done := a.tryParseStep(buffer); done {
                // 必须同时有 Action 和 ActionInput 才执行
                if step.Action != "" && step.ActionInput != nil && !actionExecuted {
                    // 执行工具
                    callID := generateCallID()
                    writer.WriteToolCall(actionName, callID, step.ActionInput)

                    result, err := a.toolExecutor.Execute(ctx, actionName, step.ActionInput)
                    // ...
                }
            }
        }

        // 检查是否需要继续循环
        if !actionExecuted {
            // 解析 Final Answer
            return nil
        }
    }

    writer.WriteError("Maximum iterations reached without final answer")
    return nil
}

// tryParseStep 解析ReAct格式的步骤
// 关键：必须同时找到 Action 和 ActionInput 才返回 true
func (a *ReActAgent) tryParseStep(buffer string) (*AgentStep, bool) {
    // 解析 Thought/Action/Action Input 行
    // 只有 foundAction && foundActionInput 才返回 true
}

// BuildReActPrompt 构建ReAct提示词
// 在用户消息中明确包含 start_time 和 end_time
func BuildReActPrompt(userMessage string, history []*AIMessage, context map[string]interface{}) []Message {
    // 1. 添加系统提示词模板
    // 2. 添加历史消息
    // 3. 添加上下文（包含 time_range 的 start_time/end_time）
    // 4. 在用户消息中明确指定时间参数
}
```

### 4.3 工具管理器

```go
// api-server/internal/service/llm/langchain/tools/registry.go

package tools

import (
    "context"
    "fmt"
)

type ToolManager struct {
    tools   map[string]Tool
    serverClient *grpc.ServerClient  // 用于调用Server的gRPC Client
}

type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]Parameter
    Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

type Parameter struct {
    Type        string
    Description string
    Required    bool
}

func NewToolManager(serverClient *grpc.ServerClient) *ToolManager {
    tm := &ToolManager{
        tools: make(map[string]Tool),
    }

    // 注册内置工具
    tm.tools["GetProcessTree"] = NewGetProcessTreeTool(tm)
    tm.tools["GetNetworkConnections"] = NewGetNetworkConnectionsTool(tm)
    tm.tools["GetOpenFiles"] = NewGetOpenFilesTool(tm)
    tm.tools["GetRunningProcesses"] = NewGetRunningProcessesTool(tm)
    tm.tools["GetUserSessions"] = NewGetUserSessionsTool(tm)
    tm.tools["QueryHistoricalLogs"] = NewQueryHistoricalLogsTool(tm)

    return tm
}

func (tm *ToolManager) Execute(ctx context.Context, toolName string, args map[string]interface{}) (interface{}, error) {
    tool, ok := tm.tools[toolName]
    if !ok {
        return nil, fmt.Errorf("unknown tool: %s", toolName)
    }

    // 验证参数
    if err := tm.validateArgs(tool, args); err != nil {
        return nil, err
    }

    // 执行工具
    return tool.Execute(ctx, args)
}

// GetProcessTreeTool
type GetProcessTreeTool struct {
    manager *ToolManager
}

func NewGetProcessTreeTool(m *ToolManager) *GetProcessTreeTool {
    return &GetProcessTreeTool{manager: m}
}

func (t *GetProcessTreeTool) Name() string {
    return "GetProcessTree"
}

func (t *GetProcessTreeTool) Description() string {
    return "获取指定进程的完整进程树结构，包括所有子进程"
}

func (t *GetProcessTreeTool) Parameters() map[string]Parameter {
    return map[string]Parameter{
        "host_id": {Type: "string", Description: "目标主机ID", Required: true},
        "pid":     {Type: "number", Description: "进程PID", Required: true},
    }
}

func (t *GetProcessTreeTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    hostID := args["host_id"].(string)
    pid := int(args["pid"].(float64))

    // 通过Server gRPC调用Agent
    resp, err := t.manager.serverClient.ExecuteTool(ctx, &pb.ToolExecuteRequest{
        CallId:    generateCallID(),
        HostId:    hostID,
        Tool:      "GetProcessTree",
        Arguments: fmt.Sprintf(`{"pid": %d}`, pid),
    })

    if err != nil {
        return nil, err
    }

    return parseJSON(resp.Result)
}
```

---

## 5. Server端工具调用路由

### 5.1 Server端工具执行器

```go
// server/internal/grpc_server/tool_executor.go

package grpc_server

import (
    "context"
    "encoding/json"
    "fmt"

    pb "server/pkg/api/v1"
    "server/pkg/logger"

    "go.uber.org/zap"
)

// ExecuteTool 处理工具调用请求
func (s *GRPCServer) ExecuteTool(ctx context.Context, req *pb.ToolRequest) (*pb.ToolResponse, error) {
    logger.Info("tool call received",
        zap.String("call_id", req.CallId),
        zap.String("host_id", req.HostId),
        zap.String("tool", req.Tool),
    )

    hostID, err := parseHostID(req.HostId)
    if err != nil {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   "invalid host id",
        }, nil
    }

    // 获取Agent连接
    conn, ok := s.agentConnections.Load(hostID)
    if !ok {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   "agent not connected",
        }, nil
    }

    agentConn := conn.(*AgentConnection)

    // 构建Agent工具调用命令
    toolCmd := fmt.Sprintf("#TOOL:%s#%s", req.Tool, req.Arguments)

    // 通过Stream发送命令
    err = agentConn.Stream.Send(&pb.CommandRequest{
        Request: &pb.CommandRequest_Execute{
            Execute: &pb.CommandExecute{
                TaskId:         req.CallId,
                HostId:         req.HostId,
                ScriptContent:  toolCmd,
                TimeoutSeconds: req.TimeoutSeconds,
            },
        },
    })

    if err != nil {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   fmt.Sprintf("failed to send tool request: %v", err),
        }, nil
    }

    // 等待结果（通过回调或Channel）
    result, err := s.waitForToolResult(req.CallId)
    if err != nil {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   err.Error(),
        }, nil
    }

    return &pb.ToolResponse{
        CallId:         req.CallId,
        Success:        true,
        Result:         result,
        ExecutionTimeMs: calculateExecutionTime(req.CallId),
    }, nil
}

// waitForToolResult 等待工具执行结果
func (s *GRPCServer) waitForToolResult(callID string) (string, error) {
    // 实现等待逻辑，通过callback或channel接收结果
    // 超时时间默认30秒
}
```

---

## 6. Server端工具调用 (V5.6 Callback模式)

### 6.1 Server端ExecuteTool实现 (V5.6 Callback模式)

```go
// server/internal/grpc_server/server.go V5.6

// AgentConnection 新增CallbackClient和CallbackConn字段
type AgentConnection struct {
    HostID         uuid.UUID
    Stream         pb.AgentService_ExecuteCommandServer
    Client         pb.AgentServiceClient  // nil - 单向调用不用
    CallbackClient pb.AgentServiceClient  // V5.6: Agent回调客户端
    CallbackConn   *grpc.ClientConn       // V5.6: 回调连接(需关闭)
    // ...
}

// ExecuteTool 通过回调连接调用Agent工具
func (s *GRPCServer) ExecuteTool(ctx context.Context, req *pb.ToolRequest) (*pb.ToolResponse, error) {
    logger.Info("tool call received",
        zap.String("call_id", req.CallId),
        zap.String("host_id", req.HostId),
        zap.String("tool", req.Tool),
    )

    hostID, err := parseHostID(req.HostId)
    if err != nil {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   "invalid host id",
        }, nil
    }

    conn, ok := s.agentConnections.Load(hostID)
    if !ok {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   "agent not connected",
        }, nil
    }

    agentConn := conn.(*AgentConnection)

    // V5.6: 通过回调客户端直接调用Agent的ExecuteTool
    if agentConn.CallbackClient == nil {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   "agent callback client not available (not registered with callback port)",
        }, nil
    }

    resp, err := agentConn.CallbackClient.ExecuteTool(ctx, req)
    if err != nil {
        return &pb.ToolResponse{
            CallId:  req.CallId,
            Success: false,
            Error:   err.Error(),
        }, nil
    }

    return resp, nil
}

// Register时存储回调端口并创建回调连接
func (s *GRPCServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
    // ... 原有注册逻辑 ...

    // V5.6: 存储回调端口
    if req.CallbackPort > 0 {
        s.callbackPorts.Store(hostID.String(), int(req.CallbackPort))
    }

    return &pb.RegisterResponse{
        Success: true,
        HostId:  hostID.String(),
    }, nil
}

// ExecuteCommand流处理时创建回调连接
func (s *GRPCServer) ExecuteCommand(stream pb.AgentService_ExecuteCommandServer) error {
    for {
        req, err := stream.Recv()
        if err != nil {
            // 清理连接
            if connection != nil {
                s.agentConnections.Delete(hostID)
                if connection.Cancel != nil {
                    connection.Cancel()
                }
                // V5.6: 关闭回调连接防止泄露
                if connection.CallbackConn != nil {
                    connection.CallbackConn.Close()
                }
            }
            return err
        }
        // ...
    }
}
```

### 6.2 连接管理 (V5.6新增)

Server使用`sync.Map`存储每个Agent的回调端口:
- `callbackPorts sync.Map // hostID -> callback port`

断开连接时关闭回调连接释放资源。

---

## 7. Agent端工具执行

### 7.1 Agent工具执行器

```go
// agent/internal/tool_executor.go

package tool

type Executor struct {
    tools map[string]ToolHandler
}

type ToolHandler func(args map[string]interface{}) (interface{}, error)

func NewExecutor() *Executor {
    e := &Executor{
        tools: make(map[string]ToolHandler),
    }
    e.registerTools()
    return e
}

func (e *Executor) registerTools() {
    e.tools["GetProcessTree"] = e.getProcessTree
    e.tools["GetNetworkConnections"] = e.getNetworkConnections
    e.tools["GetOpenFiles"] = e.getOpenFiles
    e.tools["GetRunningProcesses"] = e.getRunningProcesses
    e.tools["GetUserSessions"] = e.getUserSessions
    e.tools["QueryHistoricalLogs"] = e.queryHistoricalLogs
}

func (e *Executor) Execute(tool string, args map[string]interface{}) (interface{}, error) {
    handler, ok := e.tools[tool]
    if !ok {
        return nil, fmt.Errorf("unknown tool: %s", tool)
    }
    return handler(args)
}

// getProcessTree 获取进程树
func (e *Executor) getProcessTree(args map[string]interface{}) (interface{}, error) {
    pid := int(args["pid"].(float64))

    // 检查进程是否存在
    procPath := fmt.Sprintf("/proc/%d", pid)
    if _, err := os.Stat(procPath); os.IsNotExist(err) {
        return nil, fmt.Errorf("process %d not found", pid)
    }

    // 获取进程信息
    procInfo, err := e.readProcessInfo(pid)
    if err != nil {
        return nil, err
    }

    // 获取子进程
    children := e.getChildProcesses(pid)

    return map[string]interface{}{
        "process":  procInfo,
        "children": children,
        "captured": time.Now().Unix(),
    }, nil
}

func (e *Executor) readProcessInfo(pid int) (map[string]interface{}, error) {
    // 读取 /proc/{pid}/status
    statusPath := fmt.Sprintf("/proc/%d/status", pid)
    data, err := os.ReadFile(statusPath)
    if err != nil {
        return nil, err
    }

    info := parseProcStatus(data)
    info["pid"] = pid
    info["exe"] = e.readProcExe(pid)

    return info, nil
}

func (e *Executor) getChildProcesses(pid int) []map[string]interface{} {
    children := []map[string]interface{}{}

    // 遍历/proc查找父进程为pid的进程
    entries, err := os.ReadDir("/proc")
    if err != nil {
        return children
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }

        childPid, err := strconv.Atoi(entry.Name())
        if err != nil {
            continue
        }

        // 读取父进程ID
        ppid := e.getPPID(childPid)
        if ppid == pid {
            childInfo, _ := e.readProcessInfo(childPid)
            if childInfo != nil {
                childInfo["children"] = e.getChildProcesses(childPid)
                children = append(children, childInfo)
            }
        }
    }

    return children
}
```

---

## 7. gRPC接口定义

### 7.1 新增ToolExecute接口

```protobuf
// api_server_comm.proto

service APIServerToServer {
    rpc ForwardCommand(ForwardCommandRequest) returns (ForwardCommandResponse);
    rpc GetAgentStatus(GetAgentStatusRequest) returns (GetAgentStatusResponse);
    rpc ListConnectedAgents(ListConnectedAgentsRequest) returns (ListConnectedAgentsResponse);
    rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
    rpc UpdateAgentRules(UpdateAgentRulesRequest) returns (UpdateAgentRulesResponse);
    rpc UpdateAgentRulesForHost(UpdateAgentRulesForHostRequest) returns (UpdateAgentRulesResponse);  // V5.6新增
    rpc ExecuteBlockCommand(ExecuteBlockCommandRequest) returns (ExecuteBlockCommandResponse);
    rpc ExecuteTool(ToolExecuteRequest) returns (ToolExecuteResponse);  // V5.6新增
    rpc CollectSoftware(CollectSoftwareRequest) returns (CollectSoftwareResponse);
}

message UpdateAgentRulesForHostRequest {
    string host_id = 1;           // 目标主机ID（精确指定）
    string action = 2;
    repeated AgentRuleUpdate rules = 3;
}

message ToolExecuteRequest {
    string call_id = 1;
    string host_id = 2;           // 目标主机ID（精确指定）
    string tool = 3;              // 工具名称
    string arguments = 4;         // JSON格式参数
    int32 timeout_seconds = 5;    // 超时时间
}

message ToolExecuteResponse {
    string call_id = 1;
    bool success = 2;
    string result = 3;            // JSON格式结果
    string error = 4;
    int64 execution_time_ms = 5;
}
```

### 7.2 Agent Tool接口

```protobuf
// agent_comm.proto

service AgentService {
    rpc Register(RegisterRequest) returns (RegisterResponse);
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
    rpc ExecuteCommand(stream CommandRequest) returns (stream CommandResponse);
    rpc ExecuteTool(ToolRequest) returns (ToolResponse);  // V5.6新增
    rpc ReportEvent(ReportEventRequest) returns (ReportEventResponse);
    rpc UpdateRules(RuleUpdateRequest) returns (RuleUpdateResponse);
}

message ToolRequest {
    string call_id = 1;
    string host_id = 2;
    string tool = 3;              // 工具名称
    string arguments = 4;         // JSON格式参数
    int32 timeout_seconds = 5;
}

message ToolResponse {
    string call_id = 1;
    bool success = 2;
    string result = 3;
    string error = 4;
    int64 execution_time_ms = 5;
}
```

---

## 8. 错误处理

### 8.1 错误码定义

| 错误码 | 说明 |
|--------|------|
| `SIGMA_PARSE_ERROR` | Sigma规则解析失败 |
| `TOOL_NOT_FOUND` | 工具不存在 |
| `TOOL_EXECUTION_FAILED` | 工具执行失败 |
| `AGENT_NOT_CONNECTED` | Agent未连接 |
| `HOST_ID_INVALID` | 无效的主机ID |
| `SESSION_NOT_FOUND` | AI分析会话不存在 |
| `LLM_ERROR` | LLM调用失败 |

---

## 附录：V5.6 Sigma规则上传功能更新说明

### 1. 功能更新 (2026-04-17)

#### 1.1 文件上传限制
- 单个文件上传：支持 `.yaml`, `.yml`, `.zip` 格式
- 批量文件上传：最多支持10个文件同时选择
- 文件大小限制：单个文件不超过10MB

#### 1.2 MITRE验证规则
- **必填验证**：MITRE ID为空的规则不允许导入，系统返回错误信息
- **去重验证**：相同MITRE ID的规则不允许重复导入，已存在的规则会被跳过（skipped_duplicate）

#### 1.3 解析流程
```
上传文件 → 文件格式校验 → 文件hash去重 → YAML解析 → MITRE验证 → MITRE去重 → 入库
```

#### 1.4 严重程度映射
| Sigma level | 系统severity字段 |
|--------------|------------------|
| critical | critical |
| high | high |
| medium | medium |
| low | low |
| informational | low (视为低危) |
| warning | warning |

#### 1.5 API响应变化
```json
{
  "success": true,
  "parsed_count": 1,
  "failed_count": 0,
  "skipped_count": 0,
  "rules": [{
    "rule_id": "xxx",
    "title": "规则标题",
    "status": "pending",
    "mitre_id": "T1059.004",
    "severity": "high"
  }],
  "failed_files": []  // 新增：导入失败的文件列表
}
```

### 2. 前端更新 (2026-04-17)

#### 2.1 批量操作
- 合并删除、启用、禁用为单个下拉菜单"批量操作"
- 下拉菜单包含：启用选中、禁用选中、删除选中（删除有分隔线）

#### 2.2 导入对话框
- 支持拖拽和点击上传
- 支持多文件选择
- 显示导入结果，包括成功/失败/跳过的规则列表
- 失败文件单独显示错误信息

---

---

## 附录：V5.6 AI分析功能Bug修复说明

### 1. Bug修复记录 (2026-04-22)

#### 1.1 Bug #1: 重复消息问题

**问题描述**: AI分析时出现重复的思考消息

**根本原因**: 
- `createSSEHandler` 函数中，当收到 `tool_call` 事件时，思考内容被同时发送到两个消息气泡
- `pendingThinking` 没有被正确清空，导致后续思考内容与之前的重复

**修复方案**:
```javascript
// 添加 thinkingFlushedAsBubble 标志，防止重复
let thinkingFlushedAsBubble = false

case 'tool_call':
    cleanup()
    flushThinking()
    if (currentThinking && !thinkingFlushedAsBubble) {
        messages.value.push({
            role: 'assistant',
            content: '',
            thinking: currentThinking,
            toolCalls: []
        })
        thinkingFlushedAsBubble = true
    }
```

#### 1.2 Bug #2: "start_time is required" 错误

**问题描述**: 使用 QueryHistoricalLogs 工具时报错 "start_time is required"

**根本原因**:
1. ReAct Agent 在解析 LLM 输出时，在只看到 "Action:" 时就立即执行工具，此时 ActionInput 尚未解析
2. LLM 输出的时间参数格式为嵌套对象 `time_range: {start: ..., end: ...}`，但代码期望顶层 `start_time`, `end_time`
3. LLM 没有正确使用会话中提供的 time_range 参数

**修复方案**:
1. **prompts.go**: 增强提示词，明确告知 LLM 必须包含 start_time 和 end_time 参数
2. **react_agent.go**: 修改 `tryParseStep` 函数，必须同时有 Action 和 ActionInput 才执行工具
```go
// 修复前
if foundAction && step.Action != "" {
    return step, true
}

// 修复后
if foundAction && step.Action != "" && foundActionInput {
    return step, true
}
```
3. **ai_analysis_handler.go**: 
   - 支持嵌套 `time_range` 对象，自动提取 `start_time` 和 `end_time`
   - 在用户消息中直接提供时间参数值

#### 1.3 Bug #3: "Maximum iterations reached" 问题

**问题描述**: ReAct Agent 达到最大迭代次数仍未输出 Final Answer

**根本原因**:
- LLM 在多轮工具调用后未能正确输出 "Final Answer:" 标记
- 工具执行循环后没有继续推理给出最终结论
- 默认 maxIterations=10 不够用

**优化方案**:
- 将 `maxIterations` 默认值调整为 15
- 支持通过 API 参数配置 maxIterations (1-100)
- 当 maxIterations 设置超过 50 时，ReAct Agent 只允许最多 50 轮工具循环；第 50 轮后必须停止继续调用工具，并基于已有 Observation 强制输出 `Final Answer`。
- 当模型连续两轮未输出有效工具调用也未输出 `Final Answer` 时，后端必须停止继续诱导工具调用，并基于已有上下文强制输出最终结论，避免 SSE 长时间挂起。
- 模型输出的工具名或参数中如果包含 `...`、`[the action to take]`、`<host_id>` 等提示词占位符，不允许继续作为真实工具请求执行；`host_id` 占位符必须回退到会话中的真实主机 ID。
- 前端 AIAnalysis.vue 添加最大轮数配置输入框
- 在达到最大迭代后返回中间推理结果而不是错误
- 添加更详细的 ReAct Prompt 指导 LLM 正确输出格式

### 2. API Server ToolExecutor 实现

**文件**: `api-server/internal/api/handler/ai_analysis_handler.go`

```go
type ToolExecutor struct {
    serverClient   *grpc.ServerClient
    defaultHostIDs []string  // 从会话中提取的 host_ids
}

// Execute 执行工具调用
func (e *ToolExecutor) Execute(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error) {
    // 1. 参数规范化：camelCase -> snake_case
    normalizedArgs := normalizeArgs(args)

    // 2. 自动填充 host_id（如果未提供）
    if hostID == "" && len(e.defaultHostIDs) > 0 {
        hostID = e.defaultHostIDs[0]
    }

    // 3. QueryHistoricalLogs 特殊处理：支持嵌套 time_range
    if tool == "QueryHistoricalLogs" {
        if tr, ok := normalizedArgs["time_range"].(map[string]interface{}); ok {
            if start, ok := tr["start"].(string); ok {
                normalizedArgs["start_time"] = start
            }
        }
    }

    // 4. 调用 Server gRPC ExecuteTool
    resp, err := e.serverClient.ExecuteTool(ctx, callID, hostID, tool, argsJSON, 60)
}
```

### 3. ReAct Agent Stream 流程

```
用户消息 → 构建Prompt(包含time_range) → LLM流式响应
    ↓
解析 Thought/Action/Action Input
    ↓
等待 ActionInput 完整后执行工具（关键修复）
    ↓
工具结果作为 Observation 反馈给 LLM
    ↓
继续推理或输出 Final Answer
    ↓
发送 content + done 事件
```

---

## 10. V5.6 Update (2026-04-23) - 新增API端点

### 10.1 AI分析会话管理API

#### 获取会话列表（支持分页）

```
GET /api/v1/detection/alerts/ai-analysis/sessions
```

**请求参数：**
| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认10 |
| status | string | 否 | 过滤状态：active/completed |

**响应：**
```json
{
  "success": true,
  "data": {
    "sessions": [
      {
        "id": "uuid",
        "session_id": "uuid",
        "alert_ids": ["uuid"],
        "status": "active",
        "max_iterations": 15,
        "message_count": 5,
        "created_at": "2026-04-23T10:00:00Z"
      }
    ],
    "total": 100,
    "page": 1,
    "page_size": 10
  }
}
```

#### 获取会话历史

```
GET /api/v1/detection/alerts/ai-analysis/{session_id}/history
```

**响应：**
```json
{
  "success": true,
  "data": {
    "session_id": "uuid",
    "messages": [
      {
        "role": "user",
        "content": "用户消息",
        "thinking": "",
        "created_at": "2026-04-23T10:00:00Z"
      },
      {
        "role": "assistant",
        "content": "AI回复内容",
        "thinking": "AI思考过程",
        "created_at": "2026-04-23T10:00:01Z"
      }
    ]
  }
}
```

#### 删除会话

```
DELETE /api/v1/detection/alerts/ai-analysis/{session_id}
```

**响应：**
```json
{
  "success": true,
  "message": "session deleted"
}
```

### 10.2 SSE流式响应事件

ReAct Agent通过SSE流式输出以下事件：

| 事件类型 | 说明 | 字段 |
|---------|------|------|
| thinking | AI思考内容 | content |
| tool_call | 工具调用 | tool, call_id, args |
| tool_result | 工具执行结果 | call_id, result, time_ms |
| tool_error | 工具执行错误 | call_id, error |
| content | 最终回复内容 | content |
| done | 流式结束 | - |
| error | 错误 | content |

### 10.3 图片模型配置 API

系统配置页将文本 LLM 与图片模型分开保存。文本 LLM 继续用于告警分析、规则生成和漏洞分析；图片模型用于后续报告图、流程图或图片生成能力接入。

#### 获取图片模型配置

```
GET /api/v1/config/image-model
```

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "api_key_masked": "sk-c****xxxx",
    "provider": "minimax",
    "base_url": "https://api.minimax.io/v1",
    "model_name": "image-01",
    "is_active": true
  }
}
```

#### 保存图片模型配置

```
POST /api/v1/config/image-model
```

**请求：**
```json
{
  "api_key": "sk-xxx",
  "provider": "minimax",
  "base_url": "https://api.minimax.io/v1",
  "model_name": "image-01"
}
```

内置图片模型厂商配置：

| provider | base_url | model_name | endpoint |
|----------|----------|------------|----------|
| minimax | https://api.minimax.io/v1 | image-01 | POST /image_generation |
| zhipu | https://open.bigmodel.cn/api/paas/v4 | cogview-3-flash | POST /images/generations |
| custom | 用户自定义 | 用户自定义 | 按 provider 分支或自定义地址处理 |

### 10.4 AI分析流程图输出契约

AI分析最终回复必须继续输出 `attack_graph` JSON。后端持久化完整文本与工具轨迹，并在最终 `content` 输出后、SSE `done` 前，调用当前激活的图片模型生成攻击溯源图图片。

| 输出 | 说明 |
|------|------|
| 交互式溯源图 | 使用 `AttackGraph` 组件展示节点、边、时间线和处置建议 |
| 图片模型溯源图 | SSE 输出 `flowchart_image` 事件，`result.url` 为图片模型返回的图片 URL，作为调试/下载能力保留 |
| 结构化溯源图 | 前端以 `attack_graph` 为唯一主展示视图，展示节点、边、时间线和处置建议 |
| 本地 SVG 兜底 | 如果图片模型失败，前端仍可将 `attack_graph` 渲染为 SVG data URL |

SSE 事件顺序要求：

1. 文本 LLM 通过 `thinking`、`tool_call`、`tool_result` 和 `content` 完成分析。
2. 后端根据最终 `content` 解析 `attack_graph` 与 `conclusions`，并将每条告警的 AI 结论回写到告警表。
3. 后端根据最终 `content` 构造图片提示词，调用图片模型。
4. 后端发送 `flowchart_image`，成功时包含 `result.url`，失败时包含 `error`。
5. 后端发送 `done`，前端关闭 EventSource。

运行时约束补充：

- `Alert Context` 不能只传 `alert_ids`；必须携带所选告警的真实快照字段，至少包括 `alert_id`、`host_id`、`hostname`、`rule_title`、`severity`、`status`、`description`、`process_tree`、`llm_summary`、`first_seen_at` 和 `last_seen_at`。
- `Final Answer` 中的 `conclusions` 必须包含每条告警的 `alert_id`、`action` 和中文 `summary`，供告警详情页直接展示。
- 当 `Final Answer` 缺少最终答案而仅耗尽迭代次数时，后端返回的错误语义应明确表示“达到最大推理轮数，尚未形成最终结论”，而不是通用连接异常。

### 10.4.1 Agent事件到AI溯源图的运行时约束

为了保证“Sigma规则命中 -> 告警入库 -> AI分析 -> 溯源图输出”的端到端链路可复现，Server 处理 Agent 运行时事件时必须满足以下约束：

1. `ReportEvent` 收到事件后先确保 `hosts.id = req.host_id` 存在；如果注册记录尚未落库，使用事件中的 `host_id` 创建兜底主机，避免 `runtime_events` 和 `alerts` 外键失败。
2. `runtime_events` 表结构必须包含 DC 消费端写入的 `process_name` 字段，Server 和 DC 共享同一套运行时事件迁移。
3. `alerts.process_tree` 是 JSONB 字段，空进程树必须保存为空值或合法 JSON，不能写入空字符串。
4. AI 分析会话选择真实告警内部 UUID，后端从告警提取 `host_id`，后续 ReAct 工具调用只路由到相关 Agent。
5. 最终页面验收必须使用真实 Agent 上报的告警，并在 SSE `done` 前看到 `flowchart_image` 或前端 SVG 兜底溯源图。

2026-04-25 端到端验证记录：

- SigmaHQ `linux/process_creation` 规则通过规则上传接口解析并激活。
- 本机 Agent 上报真实进程事件后，`runtime_events` 与 `alerts` 均可落库，告警列表接口可查询到真实告警。
- AI 分析 SSE 使用真实告警创建会话，先走文本模型分析，随后在 `done` 前输出 `flowchart_image` 事件。
- 页面截图保存到 `docs/screenshots/ui-refresh/detection-ai-analysis.png` 和 `docs/screenshots/ui-refresh/detection-ai-analysis-flowchart.png`。

### 10.5 数据模型更新

#### AIMessage 新增字段

```go
type AIMessage struct {
    ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
    SessionID   string    `gorm:"type:varchar(100);index"`
    MessageID   string    `gorm:"type:varchar(100);uniqueIndex"`
    Role        string    `gorm:"type:varchar(20)"`
    Content     string    `gorm:"type:text"`
    Thinking    string    `gorm:"type:text"`    // 新增：AI思考过程
    ToolCalls   JSONB     `gorm:"type:jsonb"`
    ToolResults JSONB     `gorm:"type:jsonb"`
    Steps       JSONB     `gorm:"type:jsonb"`  // 新增：完整推理步骤
    CreatedAt   time.Time
}
```

**文档结束**

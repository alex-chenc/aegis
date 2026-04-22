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

---

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
    AlertIDs     []string    `json:"alert_ids"`
    TimeRange    *TimeRange  `json:"time_range"`
    HostFilter   []string    `json:"host_filter"`
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

type ReActAgent struct {
    llmClient     *llm.Client
    memory        *Memory
    toolManager   *ToolManager
    vectorService *VectorService  // RAG增强
    maxIterations int
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
    AttackGraph *AttackGraph `json:"attack_graph"` // 溯源图
}

// SSE 流式事件
type SSEEvent struct {
    Type    string      `json:"type"`    // thinking | tool_call | tool_result | content | done
    Content string      `json:"content"`
    Tool    string      `json:"tool,omitempty"`
    CallID  string      `json:"call_id,omitempty"`
    Args    interface{} `json:"args,omitempty"`
    Result  interface{} `json:"result,omitempty"`
    TimeMs  int64       `json:"time_ms,omitempty"`
}

func NewReActAgent(llmClient *llm.Client, memory *Memory, toolManager *ToolManager) *ReActAgent {
    return &ReActAgent{
        llmClient:     llmClient,
        memory:        NewMemory(),
        toolManager:   toolManager,
        maxIterations: 10,
    }
}

// Invoke 单轮对话（阻塞）
func (a *ReActAgent) Invoke(userMessage string, history []*model.AIMessage) (*AgentResponse, error) {
    // 1. 获取RAG上下文
    ragContext, _ := a.vectorService.BuildRAGContext(ctx, userMessage, "")

    // 2. 构建ReAct格式Prompt
    prompt := BuildReActPrompt(userMessage, history, a.memory.GetContext(), ragContext)

    // 3. 调用LLM
    response, err := a.llmClient.Chat(context.Background(), &llm.ChatRequest{
        Model:    "claude-3-5-sonnet",
        Messages: prompt,
    })
    if err != nil {
        return nil, fmt.Errorf("LLM调用失败: %w", err)
    }

    // 4. 解析ReAct输出
    steps, finalAnswer := a.parseReActOutput(response.Content)

    // 5. 执行工具调用循环
    for i := range steps {
        if steps[i].Action != "" {
            result, err := a.toolManager.Execute(ctx, steps[i].Action, steps[i].ActionInput)
            steps[i].Observation = a.formatObservation(result, err)
            a.memory.AddStep(steps[i])
        }
    }

    // 6. 解析attack_graph JSON
    attackGraph := a.parseAttackGraph(finalAnswer)

    // 7. 更新记忆
    a.memory.AddMessage("user", userMessage)
    a.memory.AddMessage("assistant", finalAnswer)

    return &AgentResponse{
        Content:     finalAnswer,
        Steps:       steps,
        ToolCalls:   a.extractToolCalls(steps),
        SessionID:   a.memory.GetSessionID(),
        AttackGraph: attackGraph,
    }, nil
}

// Stream 流式执行（SSE）
func (a *ReActAgent) Stream(ctx context.Context, userMessage string, writer SSEWriter) error {
    // 实现SSE流式输出，逐步发送 thinking -> tool_call -> tool_result -> content -> done
}

// parseReActOutput 解析ReAct格式输出
func (a *ReActAgent) parseReActOutput(content string) ([]AgentStep, string) {
    // 解析 Thought/Action/Observation 循环
    // 提取 Final Answer JSON
}

// parseAttackGraph 从Final Answer中解析attack_graph JSON
func (a *ReActAgent) parseAttackGraph(finalAnswer string) *AttackGraph {
    // 解析JSON中的attack_graph字段
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

**文档结束**

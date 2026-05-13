# AI 分析告警详情增强与连接中断修复设计

## 1. 问题描述

### 问题1：AI 分析初始对话缺少告警详细信息

**现象**：AI 分析开始时，发送给 LLM 的告警上下文信息不够详细，缺少进程级别关键字段。

**根因**：`AlertContextSnapshot` 结构体（`ai_analysis_handler.go:40-54`）仅包含 Alert 模型 28 个字段中的 13 个，缺少以下关键字段：

| 缺失字段 | 类型 | 对 AI 分析的价值 |
|:---|:---|:---|
| `PID` | int | 进程 ID，GetProcessTree 工具调用必需参数 |
| `PPID` | int | 父进程 ID，攻击链溯源关键信息 |
| `CommandLine` | string | 命令行参数，判断攻击手法的核心数据 |
| `MitreName` | string | MITRE ATT&CK 技术名称，辅助 LLM 理解威胁类型 |
| `RuleID` | string | 规则标识，用于关联和追溯 |
| `HitCount` | int | 命中次数，判断告警严重程度 |
| `AutoBlocked` | bool | 是否已自动阻断 |
| `ManualBlocked` | bool | 是否已手动阻断 |
| `BlockStatus` | *string | 阻断状态详情 |
| `BlockMessage` | string | 阻断消息 |
| `LLMDisposalStrategy` | string | 历史 AI 处置建议（若有） |
| `CreatedAt` | time.Time | 告警创建时间 |

**影响**：
- LLM 无法获取进程 PID/PPID，需要额外调用 GetRunningProcesses 工具才能获取，浪费推理轮次
- 缺少 CommandLine 信息，LLM 无法直接判断命令是否恶意
- 缺少阻断状态，LLM 可能对已阻断的告警重复建议阻断
- 缺少历史处置策略，LLM 无法参考之前的分析结论

### 问题2：AI 分析连接中断

**现象**：用户看到 "AI 分析失败: AI 分析连接中断，请稍后重试或查看服务日志"。

**根因**（两层问题）：

**根因 A — context 绑定错误**：`StreamMessage` handler 使用 `c.Request.Context()` 作为 agent runtime 的 context。当 SSE 连接断开（前端关闭、网络中断等），HTTP request context 被取消，导致所有正在进行的 LLM 调用失败：
```
LLM request failed with non-retryable error: "request failed: Post .../chat/completions: context canceled"
```

**根因 B — 缺少 WriteDone**：当 `runtime.Run()` 返回错误时，仅写入 `error` 事件但未写入 `done` 事件，SSE 流未正确终止。

**错误链路**：
```
前端 SSE 连接断开 → HTTP context 取消 → LLM 请求 context canceled → agent runtime 中断 → 前端显示"连接中断"
```

**修复**：
1. 使用 `context.WithTimeout(context.Background(), 15*time.Minute)` 替代 `c.Request.Context()`，使 agent runtime 独立于 SSE 连接生命周期
2. 在所有错误路径添加 `sseWriter.WriteDone()` 确保 SSE 流正确终止

## 2. 解决方案

### 2.1 告警详情增强

**修改文件**：`api-server/internal/api/handler/ai_analysis_handler.go`

扩展 `AlertContextSnapshot` 结构体，补充缺失字段：

```go
type AlertContextSnapshot struct {
    // 原有字段（13个）
    ID                  string    `json:"id"`
    AlertID             string    `json:"alert_id"`
    HostID              string    `json:"host_id"`
    Hostname            string    `json:"hostname"`
    RuleTitle           string    `json:"rule_title"`
    MitreID             string    `json:"mitre_id"`
    Severity            string    `json:"severity"`
    Status              string    `json:"status"`
    Description         string    `json:"description"`
    ProcessTree         string    `json:"process_tree,omitempty"`
    LLMSummary          string    `json:"llm_summary,omitempty"`
    FirstSeenAt         time.Time `json:"first_seen_at"`
    LastSeenAt          time.Time `json:"last_seen_at"`

    // 新增字段（12个）
    PID                 int       `json:"pid"`
    PPID                int       `json:"ppid"`
    CommandLine         string    `json:"command_line,omitempty"`
    MitreName           string    `json:"mitre_name,omitempty"`
    RuleID              string    `json:"rule_id,omitempty"`
    HitCount            int       `json:"hit_count"`
    AutoBlocked         bool      `json:"auto_blocked"`
    ManualBlocked       bool      `json:"manual_blocked"`
    BlockStatus         string    `json:"block_status,omitempty"`
    BlockMessage        string    `json:"block_message,omitempty"`
    LLMDisposalStrategy string    `json:"llm_disposal_strategy,omitempty"`
    CreatedAt           time.Time `json:"created_at"`
}
```

同步修改 `buildAlertSnapshots()` 函数，映射新增字段。

### 2.2 连接中断修复

**修改文件**：`api-server/internal/api/handler/ai_analysis_handler.go`

**修复 A — 解绑 context**：使用独立 context 替代 HTTP request context：

```go
// 修复前
ctx := c.Request.Context()

// 修复后
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
defer cancel()
```

**修复 B — 确保 SSE 流终止**：在错误路径添加 `WriteDone()`：

```go
if err != nil {
    logger.Error("agent runtime error", zap.Error(err), zap.String("session_id", sessionID))
    sseWriter.WriteError(fmt.Sprintf("agent runtime error: %v", err))
    sseWriter.WriteDone()  // 确保 SSE 流正确终止
    return
}
```

## 3. 影响范围

- **后端**：`ai_analysis_handler.go` - AlertContextSnapshot 结构体 + buildAlertSnapshots 函数 + StreamMessage 错误处理
- **前端**：无修改（前端已正确处理 `done` 和 `error` 事件）
- **数据库**：无修改（查询已包含所有字段，只是 snapshot 映射时丢弃了）
- **LLM prompt**：无修改（injectAlertContext 自动序列化所有 snapshot 字段）

## 4. 验证方案

1. **编译验证**：使用 aegis-build-test skill 编译 api-server
2. **接口测试**：curl 调用 AI 分析 session 接口，验证返回的告警上下文包含新字段
3. **错误场景**：模拟 LLM 调用失败，验证 SSE 流正确终止（收到 error + done 事件）

## 5. 验证结果（2026-05-12）

### 5.1 单元测试
- `TestBuildAlertSnapshotsIncludesAllFields` — 验证 25 个字段全部正确映射 ✓
- `TestBuildAlertSnapshotsMapsNilBlockStatusToEmptyString` — 验证 nil BlockStatus → 空字符串 ✓
- `TestBuildSessionContextIncludesNewAlertFields` — 验证 session context 包含新字段 ✓
- `TestSSEWriterErrorFollowedByDone` — 验证 error 事件后跟 done 事件 ✓

### 5.2 端到端测试（curl）
- **Session 65dbcfbf**: 成功完成，status: "completed"，content_len: 369，latency: 179s
- **SSE 事件流**: thinking → plan → tool calls → content → done ✓
- **无 context canceled 错误**: 日志中未出现 "context canceled" ✓
- **WriteDone 正常发送**: SSE 流以 done 事件正确终止 ✓

### 5.3 已修复问题
| 问题 | 状态 |
|:---|:---|
| AlertContextSnapshot 缺少 12 个字段 | ✅ 已修复 |
| context 绑定 HTTP request 导致 LLM 调用取消 | ✅ 已修复 |
| 错误路径缺少 WriteDone | ✅ 已修复 |
| 成功路径缺少 WriteDone | ✅ 已修复 |

### 5.4 已知遗留问题
| 问题 | 严重程度 | 说明 |
|:---|:---|:---|
| `extractFinalAnswerResult` 解析失败 | 低 | agent-runtime 不总是返回 JSON 格式 final answer，不影响功能 |
| 溯源图生成 401 错误 | 低 | 图片 API 认证问题，与本次修复无关 |
| SSE 连接缺少心跳/keepalive | **高** | 长时间 LLM/Tool 调用期间无 SSE 数据，反向代理超时断连 |

---

## 6. SSE Keepalive 心跳修复（2026-05-13）

### 6.1 问题描述

**现象**：用户仍然频繁看到 "AI 分析连接中断，请稍后重试或查看服务日志"。

**根因**：虽然 context 绑定和 WriteDone 已修复，但 SSE 连接在长时间运行期间**无数据传输**，导致反向代理（nginx）超时断连。

**具体场景**：
1. LLM 调用（PurposePlan/PurposeReact/PurposeSummarize）耗时可达 60s
2. gRPC 工具调用（QueryHistoricalLogs 等）耗时可达 60s
3. 在这些调用期间，`SSEHookSink` 不发送任何事件
4. nginx `proxy_read_timeout` 默认 60s，超时后断开连接
5. 前端 `EventSource.onerror` 触发 → 显示 "连接中断"

**错误链路**：
```
LLM/Tool 调用阻塞 >60s → nginx proxy_read_timeout → 连接断开 → EventSource.onerror → "连接中断"
```

### 6.2 解决方案

在 `StreamMessage` handler 中启动 keepalive 协程，定期发送 SSE 注释（comment）保持连接活跃。

**SSE 注释格式**：以 `:` 开头的行是 SSE 注释，浏览器 `EventSource` 会忽略它们，但它们能保持 TCP 连接活跃。

```
: keepalive\n\n
```

**设计要点**：
1. 使用 `SSEWriter.WriteComment()` 发送 SSE 注释
2. keepalive 间隔 15s（远小于 nginx 默认 60s 超时）
3. 使用 `context.WithCancel` 控制协程生命周期
4. `StreamMessage` 返回时自动取消协程（`defer cancel()`）

### 6.3 代码变更

**文件 1**：`api-server/internal/llm/sse_writer.go`

新增 `WriteComment` 方法：
```go
// WriteComment writes an SSE comment (ignored by EventSource, keeps connection alive)
func (w *SSEWriter) WriteComment(comment string) error {
    fmt.Fprintf(w.writer, ": %s\n\n", comment)
    if f, ok := w.writer.(http.Flusher); ok {
        f.Flush()
    }
    return nil
}
```

**文件 2**：`api-server/internal/api/handler/ai_analysis_handler.go`

在 `StreamMessage` 中启动 keepalive 协程：
```go
// Start keepalive goroutine to prevent proxy timeout
keepaliveCtx, keepaliveCancel := context.WithCancel(context.Background())
defer keepaliveCancel()
go func() {
    ticker := time.NewTicker(15 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-keepaliveCtx.Done():
            return
        case <-ticker.C:
            _ = sseWriter.WriteComment("keepalive")
        }
    }
}()
```

### 6.4 影响范围

- **后端**：`sse_writer.go`（新增 WriteComment）、`ai_analysis_handler.go`（新增 keepalive 协程）
- **前端**：无修改（SSE 注释被 EventSource 自动忽略）
- **协议**：无修改（SSE 注释是标准特性）

### 6.5 验证结果

#### 单元测试
- `TestWriteCommentOutputsSSECommentFormat` — 验证 SSE 注释格式 `: keepalive\n\n` ✓
- `TestWriteCommentDoesNotAffectSubsequentDataEvents` — 验证注释不影响后续 data 事件 ✓
- `TestWriteCommentWithEmptyString` — 验证空注释格式 ✓

#### 编译验证
- `go build ./...` — 编译成功 ✓
- `go vet ./...` — 无警告 ✓
- 所有 AI 分析相关测试（14 个）全部通过 ✓

#### curl 端到端验证
- SSE 流中出现 `: keepalive` 注释 ✓
- keepalive 注释位于事件之间，保持连接活跃 ✓
- SSE 数据事件正常传输不受影响 ✓

#### 代码审查修复
| 问题 | 严重程度 | 修复 |
|:---|:---|:---|
| SSEWriter 并发写入竞态条件 | HIGH | SSEWriter 添加 `sync.Mutex`，所有写方法加锁 |
| WriteComment 静默丢弃写入错误 | MEDIUM | 传播 `fmt.Fprintf` 错误 |
| keepalive context 未关联任务 context | MEDIUM | 改为从 task context 派生 |

#### 变更文件
| 文件 | 变更 |
|:---|:---|
| `api-server/internal/llm/sse_writer.go` | 新增 `WriteComment` 方法，SSEWriter 添加 `sync.Mutex` 保证并发安全 |
| `api-server/internal/llm/sse_writer_test.go` | 新增 4 个测试（含并发写入测试） |
| `api-server/internal/api/handler/ai_analysis_handler.go` | `StreamMessage` 新增 keepalive 协程（从 task context 派生） |

### 6.6 已修复问题汇总
| 问题 | 状态 |
|:---|:---|
| AlertContextSnapshot 缺少 12 个字段 | ✅ 已修复（v5.7 之前） |
| context 绑定 HTTP request 导致 LLM 调用取消 | ✅ 已修复（v5.7 之前） |
| 错误路径缺少 WriteDone | ✅ 已修复（v5.7 之前） |
| SSE 连接缺少 keepalive 导致代理超时断连 | ✅ 已修复（本次） |
| ReAct 阶段 LLM 幻觉工具名导致解析失败 | ✅ 已修复（本次） |

---

## 7. ReAct Prompt 工具名幻觉修复（2026-05-13）

### 7.1 问题描述

**现象**：AI 分析失败，报错 "AI 分析未完成全部执行计划"。数据库显示 `status: "limited"`, `reason: "max_parse_failures"`，3 次解析失败。

**根因**（两层问题）：

**根因 A — agent-runtime 忽略 PromptProvider**：`ReActExecutor.buildReactMessages()` 使用硬编码的英文系统 prompt，完全不调用 `PromptProvider.Build()`。即使 aegis 的 `prompt_provider.go` 定义了包含工具列表的 ReAct prompt，agent-runtime 也不会使用。

**根因 B — aegis ReAct prompt 缺少工具列表**：`reactJSONPromptTemplate` 写了"可用工具同规划阶段"但未列出具体工具名，且 JSON action 格式与 agent-runtime parser 不匹配（使用 `step_complete` 而非 `step_result`，缺少 `summary`/`reason`/`confidence` 字段）。

**错误链路**：
```
agent-runtime 硬编码 prompt（无工具列表）→ LLM 不知道可用工具 → 幻觉 "execute_command" → 工具注册表找不到 → 3 次 parse failure → max_parse_failures → "未完成全部执行计划"
```

### 7.2 解决方案

**修复 1 — agent-runtime**（`executor/react.go`）：

修改 `buildReactMessages` 方法，优先使用 `PromptProvider` 获取系统 prompt，仅在 provider 不可用时回退到硬编码 prompt：

```go
func (e *ReActExecutor) buildReactMessages(ctx context.Context, ...) []core.LLMMessage {
    var system string
    if e.provider != nil {
        bundle, err := e.provider.Build(ctx, core.PromptRequest{
            TaskID: taskCtx.TaskID, StepID: step.StepID, Purpose: core.PurposeReact,
        })
        if err == nil && bundle.SystemPrompt != "" {
            system = bundle.SystemPrompt
        }
    }
    if system == "" {
        system = `You are an AI agent...` // fallback
    }
    system += fmt.Sprintf("\n\n## Current Step\n- Title: %s\n- Objective: %s\n- Expected output: %s", ...)
}
```

**修复 2 — aegis**（`adapters/prompt_provider.go`）：

替换 `reactJSONPromptTemplate`，显式列出全部 6 个工具的名称和参数，并使用与 agent-runtime parser 匹配的 JSON action 格式：

```
可用工具（必须严格使用以下工具名，不得发明新工具名）
- GetProcessTree: 获取指定主机上指定进程的完整进程树
- GetNetworkConnections: 获取指定主机的网络连接信息
- GetOpenFiles: 获取指定进程打开的文件列表
- GetRunningProcesses: 获取指定主机上正在运行的进程列表
- GetUserSessions: 获取指定主机上的用户会话信息
- QueryHistoricalLogs: 查询指定主机的历史日志
```

JSON action 格式修正：
- `tool_call`: 增加 `summary`、`reason` 字段
- `step_result`: 从 `step_complete` 改为 `step_result`，增加 `confidence` 字段
- `fail_step`: 增加 `summary` 字段，`failure_reason` 改为 `failure.reason`

### 7.3 影响范围

| 文件 | 变更 |
|:---|:---|
| `agent-runtime/executor/react.go` | `buildReactMessages` 使用 PromptProvider，传递 ctx |
| `api-server/internal/llm/adapters/prompt_provider.go` | 替换 `reactJSONPromptTemplate`，显式列出 6 个工具 |
| `api-server/internal/llm/adapters/prompt_provider_test.go` | 新增 `TestBuildReactPrompt_ExplicitlyListsAllToolNames` |

### 7.4 验证结果

#### 单元测试
- `TestBuildReactPrompt_ExplicitlyListsAllToolNames` — 验证 6 个工具名全部显式列出 ✓
- 验证使用 `step_result`（非 `step_complete`）✓
- 验证包含 `confidence` 字段 ✓
- 验证不包含 "可用工具同规划阶段" ✓
- agent-runtime 全部测试通过 ✓

#### curl 端到端验证
- Session `8df283d2`: LLM 正确使用 `GetRunningProcesses` ✓
- 无 `execute_command` 幻觉 ✓
- 无 `tool_error` ✓
- 工具调用成功: `"result":"tool GetRunningProcesses executed successfully"` ✓

## 5. 关联文档

- [AI 分析 Bug 修复设计](ai_analysis_bug_fix_design.md) - GLM API 400 错误修复
- [AI 分析性能优化](ai_analysis_performance_optimization.md) - 告警加载性能优化
- [Agent 优化设计](agent_optimization_design.md) - agent-runtime 集成设计

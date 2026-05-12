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

## 5. 关联文档

- [AI 分析 Bug 修复设计](ai_analysis_bug_fix_design.md) - GLM API 400 错误修复
- [AI 分析性能优化](ai_analysis_performance_optimization.md) - 告警加载性能优化
- [Agent 优化设计](agent_optimization_design.md) - agent-runtime 集成设计

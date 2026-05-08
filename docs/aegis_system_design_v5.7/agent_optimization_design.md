# V5.7 智能体优化设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 现状分析

### 1.1 当前ReAct智能体架构

核心文件：
- `api-server/internal/llm/react_agent.go` — ReAct循环引擎
- `api-server/internal/api/handler/ai_analysis_handler.go` — Session管理与工具执行
- `api-server/internal/llm/prompts.go` — 系统提示词和工具描述

### 1.2 已知问题

| 问题 | 位置 | 影响 |
|:---|:---|:---|
| `forceFinalAnswerAfterIterations = 50` | react_agent.go | 浪费token，简单分析循环过多 |
| `maxNoActionIterations = 2` | react_agent.go | LLM仅2次无动作就被强制结束 |
| `maxObservationChars = 12000` 硬截断 | react_agent.go | 丢失结构化输出的尾部数据 |
| `normalizeToolName()` 模糊匹配 | react_agent.go | LLM经常返回错误工具名 |
| `inferToolFromInput()` 关键词推断 | react_agent.go | 简单匹配可能误判 |
| JSON流式提取不可靠 | react_agent.go | 流式场景下可能漏解析 |
| Session仅内存存储 | ai_analysis_handler.go | API Server重启丢失活跃会话 |
| 无可观测性指标 | 全局 | 无法量化分析效率和失败率 |

---

## 2. 优化方案

### 2.1 迭代参数优化

```go
// 改造前
const (
    forceFinalAnswerAfterIterations = 50
    maxNoActionIterations           = 2
)

// 改造后
const (
    forceFinalAnswerAfterIterations = 20  // 降低：减少无效循环
    maxNoActionIterations           = 3   // 放宽：更多思考空间
)

// 可配置化（通过system_configs表）
type AgentConfig struct {
    MaxIterations      int `json:"max_iterations"`       // 默认20
    MaxNoAction        int `json:"max_no_action"`        // 默认3
    MaxObsChars        int `json:"max_observation_chars"` // 默认12000
    MaxToolCalls       int `json:"max_tool_calls"`       // 默认100
    ToolRateLimitPerMin int `json:"tool_rate_limit"`     // 默认10
}
```

### 2.2 Observation智能截断

```go
func truncateObservation(content string, maxLen int) string {
    if len(content) <= maxLen {
        return content
    }
    // 尝试JSON数组截断：保留前5条+后5条
    var arr []map[string]any
    if err := json.Unmarshal([]byte(content), &arr); err == nil && len(arr) > 10 {
        head, tail := arr[:5], arr[len(arr)-5:]
        result := formatTruncatedJSON(head, tail, len(arr)-10)
        if len(result) <= maxLen {
            return result
        }
    }
    // 默认：前80%+后20%
    headLen := maxLen * 4 / 5
    return content[:headLen] + "\n\n[... 截断 ...]\n\n" + content[len(content)-(maxLen-headLen):]
}
```

### 2.3 工具描述优化

在prompts.go中增强工具描述，减少normalizeToolName的发生：

```
1. GetProcessTree
   Description: 获取指定主机上指定进程的完整进程树
   Parameters: {"host_id": "string", "pid": "number"}
   Use when: 分析进程父子关系、追踪恶意进程创建链

2. GetNetworkConnections
   Description: 获取网络连接信息（TCP/UDP状态）
   Parameters: {"host_id": "string", "pid": "number (optional)"}
   Use when: 分析网络外联、C2通信

IMPORTANT: 使用精确工具名称，不得缩写或自创。
```

### 2.4 工具调用安全边界

```go
type ToolCallGuard struct {
    sessionTotal     atomic.Int32
    perToolCounts    map[string]*slidingwindow.Counter
    maxSessionTotal  int  // 100
    maxPerToolPerMin int  // 10
}

func (g *ToolCallGuard) AllowCall(toolName string) (bool, string) {
    if g.sessionTotal.Load() >= int32(g.maxSessionTotal) {
        return false, "单会话工具调用已达上限"
    }
    if g.perToolCounts[toolName].Count() >= g.maxPerToolPerMin {
        return false, fmt.Sprintf("工具[%s]调用频率过高", toolName)
    }
    g.sessionTotal.Add(1)
    g.perToolCounts[toolName].Increment()
    return true, ""
}
```

### 2.5 可观测性增强

```go
// 全局Prometheus指标
var (
    agentIterationsHist    = prometheus.NewHistogramVec(...)
    agentToolCallsCounter  = prometheus.NewCounterVec(...)
    agentToolFailures      = prometheus.NewCounterVec(...)
    agentJSONParseErrors   = prometheus.NewCounter(...)
    agentSessionDuration   = prometheus.NewHistogram(...)
)

// 会话级指标
type AgentSessionMetrics struct {
    SessionID          string
    TotalIterations    int
    TotalToolCalls     int
    FailedToolCalls    int
    JSONParseErrors    int
    ToolNameMismatches int
    DurationMs         int64
    ConclusionAction   string
}
```

### 2.6 Session持久化

```go
// 启动时恢复最近24小时活跃会话
func (h *AIAnalysisHandler) RestoreSessions(ctx context.Context) {
    sessions, _ := h.sessionRepo.FindRecent(ctx, 24*time.Hour)
    for _, s := range sessions {
        if s.Status == "active" {
            h.sessions[s.SessionID] = restoreSession(s)
        }
    }
}

// 30分钟超时自动清理
func (h *AIAnalysisHandler) StartSessionCleaner() {
    ticker := time.NewTicker(5 * time.Minute)
    go func() {
        for range ticker.C {
            h.mu.Lock()
            for id, s := range h.sessions {
                if time.Since(s.LastActivity) > 30*time.Minute {
                    h.sessionRepo.UpdateStatus(ctx, id, "expired")
                    delete(h.sessions, id)
                }
            }
            h.mu.Unlock()
        }
    }()
}
```

---

## 3. 提示词增强

在ReAct系统提示词中增加：

```
## 工具使用规则
1. 必须使用精确工具名称，不得缩写或自创
2. 每次Action只调用一个工具
3. 连续3次工具调用无有用信息时，基于已有信息给出Final Answer

## 分析策略
1. 先概览（进程列表、网络连接），再深入调查
2. 关注异常父子关系（Web服务器fork出shell）
3. 关注非标准端口外联
4. 综合多工具结果关联分析，不凭单一证据下结论
```

---

## 4. 优化前后对比

| 指标 | 当前 | 优化目标 |
|:---|:---|:---|
| 最大迭代 | 50 | 20 |
| 无动作容忍 | 2次 | 3次 |
| Observation截断 | 硬截断 | 智能截断 |
| 工具调用上限 | 无 | 100/会话 |
| 工具频率限制 | 无 | 10/分钟/工具 |
| 可观测性 | 无 | Prometheus指标 |
| Session持久化 | 仅内存 | DB+内存 |
| Session清理 | 无 | 30分钟超时 |

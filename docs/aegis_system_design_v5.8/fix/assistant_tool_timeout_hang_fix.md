# Bug Fix: Assistant 工具调用超时导致前端卡住

## Bug 描述与症状

### 症状
在 AI 助手对话中，用户发送查询请求（如"查询当前所有主机的情况"），助手调用 `Host.List` 工具后，前端界面一直显示"正在查询中，请稍候..."，工具调用卡片停留在 `running` 状态，永远不会完成。

### 影响范围
- 所有通过 Assistant 工具调用执行的数据库查询操作
- 包括但不限于：`Host.List`、`Host.GetDetail`、`Host.FindOffline` 等
- 当数据库响应慢或连接池耗尽时触发

### 复现步骤
1. 打开 AI 助手对话界面
2. 发送消息："查询当前所有主机的情况"
3. 助手识别意图后调用 `Host.List` 工具
4. 观察工具调用卡片状态一直为 "执行中"
5. 前端永远不会收到 `tool_result` 或 `tool_error` 事件

## 根因分析

### 完整调用链

```
用户发送消息
  → POST /api/v1/assistant/sessions/:id/message
  → service.SendMessage()
  → go orchestrator.Run()
    → IntentRouter.Classify()
    → ToolSelector.Select()
    → runAgentRuntime()
    → agent-runtime.Run()
      → AssistantToolGatewayAdapter.Call()
        → ToolDispatcher.Dispatch()
          → registry.Execute("Host.List")
            → makeHostListHandler()
              → repo.FindAll()  ← 阻塞点
              → repo.Count()    ← 阻塞点
```

### 根因

**核心问题：工具执行无超时保护**

在 `api-server/internal/assistant/tools/host_tools.go` 中，`makeHostListHandler` 直接调用数据库查询，没有超时保护：

```go
func makeHostListHandler(repo *repository.HostRepository) assistant.ToolHandler {
    return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
        hosts, err := repo.FindAll(page, pageSize, query)  // 无超时
        total, err := repo.Count(query)                      // 无超时
        return map[string]interface{}{"data": hosts, "total": total}, nil
    }
}
```

**阻塞原因**：
1. `repo.FindAll()` 和 `repo.Count()` 是同步数据库调用
2. 如果数据库连接池耗尽、表锁、或数据库响应慢，这两个调用会无限期阻塞
3. `ToolDispatcher` 没有在工具执行层面设置超时
4. 虽然 `agent-runtime` 配置了 `ToolTimeout: 60s`，但这只在框架层面生效，实际的数据库调用不感知这个超时

**连锁反应**：
1. 工具处理器阻塞 → `ToolDispatcher.Dispatch()` 阻塞
2. `AssistantToolGatewayAdapter.Call()` 阻塞
3. `agent-runtime` 的工具调用阻塞
4. `orchestrator.Run()` 阻塞在 `runtime.Run()`
5. 没有新的 SSE 事件发布
6. 前端 tool card 停留在 `running` 状态

### 次要问题：SSE 事件丢失

编排器在 goroutine 中立即开始运行并发布事件，但前端的 EventSource 连接需要时间建立。在 `RunManager.Publish()` 中，如果没有订阅者，事件会被静默丢弃。

## 修复设计

### 方案：工具执行超时 + 数据库查询超时

在两个层面添加超时保护：

#### 1. 工具调度器层面（ToolDispatcher）

在 `ToolDispatcher.executeTool()` 中添加超时控制，确保任何工具执行都不会无限期阻塞。

```go
func (d *ToolDispatcher) executeTool(ctx context.Context, callID string, tool *ToolSpec, req DispatchRequest) (*DispatchResult, error) {
    // 创建带超时的 context
    toolCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    start := time.Now()
    resultCh := make(chan *ToolExecutionResult, 1)
    errCh := make(chan error, 1)

    go func() {
        result, err := d.registry.Execute(toolCtx, req.ToolName, req.Args)
        if err != nil {
            errCh <- err
        } else {
            resultCh <- result
        }
    }()

    select {
    case result := <-resultCh:
        // 正常返回
        duration := time.Since(start).Milliseconds()
        // ... 处理结果
    case err := <-errCh:
        // 工具执行错误
        duration := time.Since(start).Milliseconds()
        // ... 处理错误
    case <-toolCtx.Done():
        // 超时
        duration := time.Since(start).Milliseconds()
        return &DispatchResult{
            CallID:     callID,
            ToolName:   req.ToolName,
            Success:    false,
            Error:      "tool execution timeout",
            DurationMs: duration,
        }, nil
    }
}
```

#### 2. 工具处理器层面（host_tools.go）

在每个工具处理器中使用带超时的 context 进行数据库查询：

```go
func makeHostListHandler(repo *repository.HostRepository) assistant.ToolHandler {
    return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
        // 确保 context 有超时
        queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
        defer cancel()

        hosts, err := repo.FindAllWithContext(queryCtx, page, pageSize, query)
        if err != nil {
            if queryCtx.Err() == context.DeadlineExceeded {
                return nil, fmt.Errorf("database query timeout: %w", err)
            }
            return nil, fmt.Errorf("failed to list hosts: %w", err)
        }

        total, err := repo.CountWithContext(queryCtx, query)
        if err != nil {
            if queryCtx.Err() == context.DeadlineExceeded {
                return nil, fmt.Errorf("database count timeout: %w", err)
            }
            return nil, fmt.Errorf("failed to count hosts: %w", err)
        }

        return map[string]interface{}{"data": hosts, "total": total}, nil
    }
}
```

#### 3. Repository 层面支持 Context

在 `HostRepository` 中添加支持 context 的查询方法：

```go
func (r *HostRepository) FindAllWithContext(ctx context.Context, page, pageSize int, query string) ([]*model.Host, error) {
    var hosts []*model.Host
    db := r.db.WithContext(ctx)

    if query != "" {
        db = db.Where("ip_address ILIKE ? OR hostname ILIKE ?", "%"+query+"%", "%"+query+"%")
    }

    err := db.Order("created_at DESC").
        Offset((page - 1) * pageSize).
        Limit(pageSize).
        Find(&hosts).Error
    return hosts, err
}
```

### 超时参数设计

| 层级 | 超时时间 | 说明 |
|------|----------|------|
| ToolDispatcher | 30s | 工具执行最大超时 |
| 工具处理器 | 10s | 单次数据库查询超时 |
| agent-runtime ToolTimeout | 60s | 框架级超时（已有） |

超时层级：工具处理器(10s) < ToolDispatcher(30s) < agent-runtime(60s)

## 受影响组件

| 组件 | 文件 | 修改类型 |
|------|------|----------|
| ToolDispatcher | `api-server/internal/assistant/tool_dispatcher.go` | 添加执行超时 |
| HostRepository | `api-server/internal/repository/host_repo.go` | 添加 Context 支持方法 |
| host_tools.go | `api-server/internal/assistant/tools/host_tools.go` | 使用带超时的 context |
| (可选) 其他工具 | `api-server/internal/assistant/tools/*.go` | 统一添加超时保护 |

## 测试用例设计

### 测试用例 1：正常场景
- **前置条件**：数据库正常响应
- **操作**：调用 Host.List 工具
- **预期**：正常返回主机列表，耗时 < 1s

### 测试用例 2：数据库慢查询
- **前置条件**：模拟数据库慢响应（通过 context 超时）
- **操作**：调用 Host.List 工具
- **预期**：10s 后返回超时错误，前端显示错误信息

### 测试用例 3：工具执行超时
- **前置条件**：工具处理器阻塞
- **操作**：调用任意工具
- **预期**：30s 后 ToolDispatcher 返回超时错误

### 测试用例 4：SSE 事件完整性
- **前置条件**：工具调用正常完成
- **操作**：通过 SSE 流监听事件
- **预期**：收到 `tool_call` → `tool_result` 完整事件链

## 回归测试用例

### RT-1：Host.List 正常查询
```go
func TestHostListHandler_NormalQuery(t *testing.T) {
    // 模拟正常数据库响应
    // 调用 makeHostListHandler
    // 验证返回数据正确
    // 验证无超时错误
}
```

### RT-2：Host.List 超时处理
```go
func TestHostListHandler_Timeout(t *testing.T) {
    // 模拟数据库慢响应
    // 调用 makeHostListHandler
    // 验证返回超时错误
    // 验证错误信息包含 "timeout"
}
```

### RT-3：ToolDispatcher 执行超时
```go
func TestToolDispatcher_ExecutionTimeout(t *testing.T) {
    // 注册一个会阻塞的工具
    // 调用 Dispatch
    // 验证 30s 后返回超时错误
    // 验证 tool call 状态为 failed
}
```

### RT-4：正常工具调用不受影响
```go
func TestToolDispatcher_NormalExecution(t *testing.T) {
    // 注册一个正常工具
    // 调用 Dispatch
    // 验证正常返回
    // 验证 tool call 状态为 success
}
```

## 实现代码变更

### 变更文件清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `api-server/internal/assistant/tool_dispatcher.go` | 修改 | `executeTool()` 添加 30s 超时保护 |
| `api-server/internal/repository/host_repo.go` | 修改 | 添加 `FindAllWithContext`、`FindByIDWithContext`、`CountWithContext` 方法 |
| `api-server/internal/assistant/tools/host_tools.go` | 修改 | 所有 handler 使用 `context.WithTimeout` + `WithContext` 方法 |
| `api-server/internal/assistant/tool_dispatcher_test.go` | 新增 | 5 个回归测试用例 |

### 验证结果

- **构建**: `go build ./...` 通过
- **测试**: 5/5 通过
  - `TestToolDispatcher_NormalExecution` - PASS (0.00s)
  - `TestToolDispatcher_ExecutionTimeout` - PASS (30.01s)
  - `TestToolDispatcher_ToolHandlerError` - PASS (0.00s)
  - `TestToolDispatcher_ToolNotFound` - PASS (0.00s)
  - `TestToolDispatcher_TimeoutWithSlowDB` - PASS (5.00s)

## 风险与回滚计划

### 风险
1. 超时时间设置过短，导致正常查询被误杀
2. 超时时间设置过长，用户体验无改善
3. 并发 goroutine 泄漏（超时后 goroutine 仍在运行）

### 回滚计划
1. 如果超时时间不合适，调整 `ToolDispatcher` 和工具处理器中的超时参数
2. 如果出现 goroutine 泄漏，确保使用 `context.WithTimeout` 并正确 defer cancel
3. 如果影响正常功能，可以通过配置开关禁用超时保护

### 缓解措施
1. 超时时间设置为保守值（工具处理器 10s，调度器 30s）
2. 超时错误信息明确，便于调试
3. 添加日志记录超时事件（`zap.Warn` 级别），便于监控
4. `context.WithTimeout` + `defer cancel()` 确保无 goroutine 泄漏

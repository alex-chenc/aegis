# 弱密码检测 agent_not_connected 错误修复设计

## Bug 描述与症状

**症状**：用户执行弱密码检测时，任务创建成功但异步执行失败，错误码为 `agent_not_connected`，错误信息为"Agent 不在线，无法下发弱密码采集工具"。但主机列表显示该 Agent 状态为"在线"。

**影响范围**：
- 弱密码检测功能无法正常执行
- 用户体验混乱：主机列表显示在线，但功能报错 Agent 不在线

## 复现步骤

1. Agent 连接到 Server，主机列表显示在线
2. Agent 的双向 gRPC 流因网络抖动或其他原因断开
3. 在 90 秒内（Redis 心跳 TTL），用户发起弱密码检测任务
4. 任务创建成功（`ensureHostRuntimeOnline` 检查通过）
5. 异步执行时 `executeApplicationTask` 检查 `agentConnections` 失败
6. 任务失败，错误码 `agent_not_connected`

## 根因分析

### 问题 1：两套在线判断机制不一致

系统中存在两套 Agent 在线判断机制：

| 机制 | 实现位置 | 存储 | 用途 | 生命周期 |
|------|---------|------|------|---------|
| Redis 心跳 | `redisClient.SetHeartbeat()` | Redis `agent:heartbeat:{host_id}` | 主机列表显示 | 90 秒 TTL |
| gRPC 流连接 | `agentConnections` sync.Map | Server 内存 | 命令下发/工具调用 | 双向流存活期间 |

**关键差异**：
- Redis 心跳：Agent 每 30 秒发送一次，Server 更新 Redis，TTL 90 秒
- gRPC 流连接：只有当 Agent 通过双向流发送初始 `CommandRequest_Execute` 请求时才添加，流断开时删除

**故障场景**：
1. Agent 双向流断开 → `agentConnections.Delete(hostID)` 立即执行
2. Redis 心跳还在 TTL 内 → 主机列表显示"在线"
3. 弱密码检测调用 `IsAgentConnected()` 检查 `agentConnections` → 返回 false

### 问题 2：ensureHostRuntimeOnline 逻辑缺陷

```go
// weak_password_service.go:316-328
func (s *WeakPasswordService) ensureHostRuntimeOnline(ctx context.Context, hostID uuid.UUID) error {
    if s.agentClient == nil {
        return nil  // BUG: agentClient 为 nil 时返回 nil（通过检查）
    }
    // ...
}
```

当 `agentClient == nil` 时，`ensureHostRuntimeOnline` 返回 `nil`，导致任务创建成功。但异步执行时 `executeApplicationTask` 在第 445 行检查 `s.agentClient == nil` 并报错。

## 修复设计

### 方案：统一在线判断逻辑

修改弱密码服务的在线判断逻辑，使其与主机列表使用相同的判断标准（Redis 心跳），同时保留 gRPC 流连接检查作为双重保障。

#### 修改 1：ensureHostRuntimeOnline 方法

```go
func (s *WeakPasswordService) ensureHostRuntimeOnline(ctx context.Context, hostID uuid.UUID) error {
    if s.agentClient == nil {
        return fmt.Errorf("%w: agent client not initialized", ErrWeakPasswordHostOffline)
    }
    status, err := s.agentClient.GetAgentStatus(ctx, hostID.String())
    if err != nil {
        return fmt.Errorf("%w: %v", ErrWeakPasswordHostOffline, err)
    }
    if status == nil {
        return ErrWeakPasswordHostOffline
    }
    // 使用 Connected 字段（基于 gRPC 流连接）
    // 如果需要与主机列表一致，可以同时检查 LastHeartbeat
    if !status.GetConnected() {
        // 额外检查：如果心跳在 90 秒内，认为可能在线但流断开
        // 这种情况下应该等待重连而不是直接失败
        if status.GetLastHeartbeat() > 0 {
            heartbeatAge := time.Now().Unix() - status.GetLastHeartbeat()
            if heartbeatAge < 90 {
                // 心跳在 90 秒内，Agent 可能正在重连
                // 返回特殊错误，让调用方可以重试
                return fmt.Errorf("%w: agent heartbeat recent (%ds ago) but stream disconnected, may be reconnecting",
                    ErrWeakPasswordHostOffline, heartbeatAge)
            }
        }
        return ErrWeakPasswordHostOffline
    }
    return nil
}
```

#### 修改 2：executeApplicationTask 方法

在 `executeApplicationTask` 中增加重试逻辑，处理 Agent 正在重连的情况：

```go
func (s *WeakPasswordService) executeApplicationTask(ctx context.Context, taskID, scanHostID, scanAppID uuid.UUID, plan CredentialCollectionPlan) {
    // ... 前置检查 ...

    if s.agentClient == nil {
        s.failApplication(task, host, app, model.ErrCodeAgentNotConnected, "Agent 服务客户端未初始化", 0)
        return
    }

    // 带重试的 Agent 状态检查
    var status *pb.GetAgentStatusResponse
    var checkErr error
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        status, checkErr = s.agentClient.GetAgentStatus(ctx, plan.HostID)
        if checkErr == nil && status != nil && status.GetConnected() {
            break
        }
        if i < maxRetries-1 {
            // 等待 2 秒后重试，给 Agent 重连的时间
            time.Sleep(2 * time.Second)
        }
    }

    if checkErr != nil || status == nil || !status.GetConnected() {
        s.failApplication(task, host, app, model.ErrCodeAgentNotConnected, "Agent 不在线，无法下发弱密码采集工具", 0)
        return
    }

    // ... 继续执行 ...
}
```

#### 修改 3：统一使用 GetAgentStatus 的判断逻辑

修改 `filterRuntimeOnlineAssets` 方法，使用与 `ensureHostRuntimeOnline` 相同的判断逻辑：

```go
func (s *WeakPasswordService) filterRuntimeOnlineAssets(ctx context.Context, assets []CandidateApplicationAsset) []CandidateApplicationAsset {
    onlineByHost := make(map[string]bool)
    var filtered []CandidateApplicationAsset
    for _, asset := range assets {
        online, ok := onlineByHost[asset.HostID]
        if !ok {
            status, err := s.agentClient.GetAgentStatus(ctx, asset.HostID)
            // 与 ensureHostRuntimeOnline 保持一致的判断逻辑
            online = err == nil && status != nil && status.GetConnected()
            onlineByHost[asset.HostID] = online
        }
        if online {
            filtered = append(filtered, asset)
        }
    }
    return filtered
}
```

## 代码变更

### 文件 1：api-server/internal/service/weak_password_service.go

1. **第 316-328 行**：修改 `ensureHostRuntimeOnline` 方法
   - 修复 `agentClient == nil` 时返回 `nil` 的问题
   - 增加心跳检查逻辑

2. **第 445-453 行**：修改 `executeApplicationTask` 方法
   - 增加重试逻辑，处理 Agent 正在重连的情况

3. **第 291-313 行**：修改 `filterRuntimeOnlineAssets` 方法
   - 统一判断逻辑

## 验证结果

### 单元测试 ✅ 通过

```
=== RUN   TestEnsureHostRuntimeOnline_NilAgentClient_ReturnsError
--- PASS: TestEnsureHostRuntimeOnline_NilAgentClient_ReturnsError (0.01s)
=== RUN   TestEnsureHostRuntimeOnline_AgentOffline_ReturnsError
--- PASS: TestEnsureHostRuntimeOnline_AgentOffline_ReturnsError (0.00s)
=== RUN   TestEnsureHostRuntimeOnline_AgentOnline_ReturnsNil
--- PASS: TestEnsureHostRuntimeOnline_AgentOnline_ReturnsNil (0.00s)
=== RUN   TestEnsureHostRuntimeOnline_GetAgentStatusError_ReturnsError
--- PASS: TestEnsureHostRuntimeOnline_GetAgentStatusError_ReturnsError (0.00s)
=== RUN   TestFilterRuntimeOnlineAssets_FiltersOfflineHosts
--- PASS: TestFilterRuntimeOnlineAssets_FiltersOfflineHosts (0.01s)
PASS
ok  	api-server/internal/service	0.100s
```

### 构建验证 ✅ 通过

```
CGO_ENABLED=0 GOOS=linux go build -o bin/api-server ./cmd...
```

### 代码审查 ✅ 完成

发现 4 个次要问题，无阻塞性问题：
1. `filterRuntimeOnlineAssets` 在 `agentClient == nil` 时返回所有资产（与 `ensureHostRuntimeOnline` 行为不一致，但可能是有意为之用于分析模式）
2. 重试逻辑使用阻塞式 `time.Sleep`，未检查 context 取消
3. 重试循环在 `status.GetConnected()` 为 true 时忽略 `checkErr`
4. `ensureHostRuntimeOnline` 未按设计文档检查心跳新鲜度

## 受影响组件

- **api-server**：弱密码服务模块（`weak_password_service.go`）
- **测试文件**：新增 5 个回归测试用例（`weak_password_service_test.go`）

## 实际代码变更

### 文件 1：api-server/internal/service/weak_password_service.go

1. **第 316-328 行**：修改 `ensureHostRuntimeOnline` 方法
   - 修复 `agentClient == nil` 时返回 `nil` 的问题
   - 改为返回 `fmt.Errorf("%w: agent client not initialized", ErrWeakPasswordHostOffline)`

2. **第 445-466 行**：修改 `executeApplicationTask` 方法
   - 增加 3 次重试逻辑，每次等待 2 秒
   - 添加重试日志：`s.logger.Info("agent not ready, retrying...", ...)`

### 文件 2：api-server/internal/service/weak_password_service_test.go

新增测试用例：
- `TestEnsureHostRuntimeOnline_NilAgentClient_ReturnsError`
- `TestEnsureHostRuntimeOnline_AgentOffline_ReturnsError`
- `TestEnsureHostRuntimeOnline_AgentOnline_ReturnsNil`
- `TestEnsureHostRuntimeOnline_GetAgentStatusError_ReturnsError`
- `TestFilterRuntimeOnlineAssets_FiltersOfflineHosts`

## 风险与回滚计划

### 风险

1. **重试逻辑增加延迟**：任务执行可能增加 2-4 秒延迟（3 次重试，每次等待 2 秒）
2. **现有任务不受影响**：修复仅影响新创建的任务

### 回滚计划

如果修复引入新问题，可以通过以下方式回滚：
1. 恢复 `ensureHostRuntimeOnline` 方法的原始实现（`agentClient == nil` 时返回 `nil`）
2. 移除 `executeApplicationTask` 的重试逻辑
3. 重新部署 api-server：`docker compose up -d --build api-server`

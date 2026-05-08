# V5.7 脚本下发前黑名单校验设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 功能概述

在脚本从API Server下发到Server之前，增加一次黑名单校验作为纵深防御（Defense in Depth）。

### 1.1 为什么需要下发前校验

| 场景 | 说明 |
|:---|:---|
| 管理员手动修改脚本 | 通过数据库或MinIO直接修改了已审计的脚本内容 |
| 审计规则更新 | 新增了更严格的规则，旧脚本不再合规 |
| 存储层被篡改 | MinIO或数据库中的脚本被恶意替换 |
| 历史遗留脚本 | 未经过生成审计的脚本或手动导入的脚本 |

---

## 2. 校验点位置

```
API Server
├── 脚本生成阶段（V5.7增强）
│   └── ScriptAuditService.AuditWithRetry()  ← 黑名单+AI双重审计
│
├── 下发前阶段（V5.7新增）
│   └── TaskService.dispatchToAgent()
│       └── ScriptAuditService.AuditForDispatch()  ← 仅黑名单，快速校验
│           ├─ 通过 → ForwardCommand()
│           └─ 不通过 → 返回错误 + 审计日志
│
└── Agent侧（V5.7新增）
    └── Executor.ExecuteCommand()
        └── BlacklistChecker.Check()  ← 最后防线
            ├─ 通过 → 执行
            └─ 不通过 → 返回失败 + 上报事件
```

---

## 3. API Server侧实现

### 3.1 改造 TaskService.dispatchToAgent()

**改造前** (task_service.go:236):
```go
func (s *TaskService) dispatchToAgent(ctx context.Context, task *model.TaskLog, scriptContent string) {
    resp, err := s.serverClient.ForwardCommand(ctx, &pb.ForwardCommandRequest{...})
}
```

**改造后** (task_service.go:243):
```go
func (s *TaskService) dispatchToAgent(ctx context.Context, taskID, hostID, ruleID uuid.UUID, scriptContent, taskType string) {
    // V5.7: 下发前黑名单校验
    if s.auditService != nil {
        auditResult, err := s.auditService.AuditForDispatch(ctx, scriptContent, taskID.String())
        if err != nil {
            logger.Error("pre-dispatch audit error, proceeding (fail-open)", ...)
        } else if auditResult != nil && !auditResult.Passed {
            blockReason := formatAuditBlockReason(auditResult.BlacklistHits)
            now := time.Now()
            s.taskLogRepo.UpdateResult(taskID, nil, &blockReason, nil, "AUDIT_BLOCKED", now)
            return
        }
    }
    // 正常下发
    _, err := s.serverClient.ForwardCommand(bgCtx, &pb.ForwardCommandRequest{...})
}
```

### 3.2 错误信息格式

```
脚本存在恶意命令，下发已阻止。
命中规则：
  1. [critical] curl管道执行 (第5行, 匹配: curl | bash)
  2. [high] Python Socket反弹 (第12行, 匹配: python import socket)
```

### 3.3 前端展示

```
┌─────────────────────────────────────────────────────────┐
│  任务详情                                                │
│  状态: 脚本审计未通过                                   │
│  错误信息: 脚本存在恶意命令，下发已阻止。               │
│  [查看审计日志]  [重新生成脚本]                          │
└─────────────────────────────────────────────────────────┘
```

---

## 4. Agent侧实现

### 4.1 改造 Executor.ExecuteCommand()

**改造后**:
```go
func (e *Executor) ExecuteCommand(taskID, scriptContent string, timeoutSeconds int) *ExecuteResult {
    // V5.7: Agent侧黑名单校验
    if e.blacklistChecker != nil {
        result, _ := e.blacklistChecker.Check(ctx, scriptContent, "all")
        if result != nil && result.HasViolation {
            e.reportBlockedExecution(taskID, result.Hits)
            return &ExecuteResult{
                ExitCode: -1,
                Stderr:   "脚本存在恶意命令，执行已阻止",
            }
        }
    }
    // 正常执行
    ...
}
```

### 4.2 Agent侧规则同步

Agent的BlacklistChecker规则来源：
- 启动时从 `/etc/aegis-agent/audit_rules.json` 加载
- 运行时通过Server的 `UpdateRules` gRPC RPC增量更新

---

## 5. 策略配置

### 5.1 Fail-Open vs Fail-Close

| 校验点 | 策略 | 理由 |
|:---|:---|:---|
| 生成阶段审计 | Fail-Close | 审计失败不生成，安全优先 |
| 下发前校验 | Fail-Open | 校验异常不阻塞正常下发 |
| Agent侧校验 | Fail-Open | 校验异常不阻塞执行 |

### 5.2 开关控制

| config_key | 说明 | 默认值 |
|:---|:---|:---|
| `command_audit.settings.dispatch_check` | 下发前校验 | true |
| `command_audit.settings.agent_check` | Agent侧校验 | true |

---

## 6. 性能要求

| 指标 | 要求 |
|:---|:---|
| 下发前校验延迟 | P99 < 100ms |
| Agent侧校验延迟 | P99 < 50ms |
| 内存开销 | < 5MB |

下发前仅做黑名单校验（纯内存操作），25条规则匹配耗时 < 1ms。

---

## 7. 数据库约束

### 7.1 task_logs 状态约束

`task_logs` 表的检查约束必须包含 `AUDIT_BLOCKED` 状态：

```sql
ALTER TABLE task_logs ADD CONSTRAINT chk_task_status
  CHECK (status IN ('PENDING', 'RUNNING', 'SUCCESS', 'FAILED', 'TIMEOUT', 'HEALING', 'AUDIT_BLOCKED'));
```

**注意**: 如果约束缺少 `AUDIT_BLOCKED`，审计拦截时 `UpdateResult` 会失败，导致任务状态无法更新，最终超时。

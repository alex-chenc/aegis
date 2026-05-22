# Sigma 规则字段名大小写不敏感匹配设计文档

## 1. 问题描述

在 Aegis v5.7 运行时威胁检测流程中，发现 T1547 规则（"System Binary Directory Write"）无法检测到对系统二进制目录的写入操作。

**测试命令**：
```bash
echo "test" > /usr/bin/aegis_test_binary
```

**预期行为**：触发 T1547 规则匹配，生成安全告警。

**实际行为**：未触发任何告警。

## 2. Root Cause 分析

### Bug: Sigma 匹配器字段名大小写敏感 [P0]

- **文件**: `agent/internal/sigma/matcher.go:177-205`
- **问题**: `selectorMatches` 函数使用直接 map 查找 `event[field]`，该操作区分大小写。当 Sigma 规则字段名为 `targetfilename`（小写）时，无法匹配事件映射中的 `TargetFilename`（混合大小写）。
- **影响**: 所有使用非小写字段名的 Sigma 规则都无法正确匹配。

**问题代码**：
```go
func (cr *CompiledRule) selectorMatches(selectorName string, event map[string]interface{}) bool {
    // ...
    for field, patterns := range selector {
        eventVal := ""
        var eventRaw interface{}
        if v, ok := event[field]; ok {  // 大小写敏感的查找
            eventRaw = v
            eventVal = strings.ToLower(fmt.Sprint(v))
        }
        // ...
    }
    return true
}
```

**字段名映射不一致**：
- Sigma 规则定义：`TargetFilename|startswith` → 标准化为 `targetfilename`（小写）
- 事件映射构建：`eventMap["TargetFilename"] = event.FilePath`（混合大小写）
- Go map 查找：`event["targetfilename"]` 无法找到 `"TargetFilename"` 键

## 3. 修复方案

### Fix 1: 大小写不敏感字段查找

在 `matcher.go` 中新增 `lookupFieldCaseInsensitive` 函数，先尝试精确匹配（快速路径），再尝试大小写不敏感匹配。

```go
func lookupFieldCaseInsensitive(event map[string]interface{}, field string) (interface{}, bool) {
    // Try exact match first (fast path)
    if v, ok := event[field]; ok {
        return v, true
    }
    // Try case-insensitive match
    lowerField := strings.ToLower(field)
    for k, v := range event {
        if strings.ToLower(k) == lowerField {
            return v, true
        }
    }
    return nil, false
}
```

**修改 `selectorMatches` 函数**：
```go
func (cr *CompiledRule) selectorMatches(selectorName string, event map[string]interface{}) bool {
    // ...
    for field, patterns := range selector {
        eventVal := ""
        var eventRaw interface{}
        // Try case-insensitive lookup
        if v, found := lookupFieldCaseInsensitive(event, field); found {
            eventRaw = v
            eventVal = strings.ToLower(fmt.Sprint(v))
        }
        // ...
    }
    return true
}
```

### Fix 2: 添加调试日志

在 `pipeline.go` 和 `loader.go` 中添加文件事件的详细日志，便于排查问题。

**pipeline.go** - 文件事件处理日志：
```go
if event.EventType == "file_access" {
    logger.Info("[FileEvent] Processing file access event",
        zap.String("path", event.FilePath),
        zap.String("action", event.FileAction),
        zap.String("flags", event.FileFlags),
        zap.String("process", event.ProcessName),
        zap.Int("pid", event.PID),
        zap.String("category", fmt.Sprintf("%v", eventMap["category"])),
        zap.String("target_filename", fmt.Sprintf("%v", eventMap["TargetFilename"])),
        zap.String("target_filename_lower", fmt.Sprintf("%v", eventMap["targetfilename"])),
        zap.String("event_action", fmt.Sprintf("%v", eventMap["event.action"])))
}
```

**loader.go** - eBPF 文件事件捕获日志：
```go
logger.Info("[eBPF] File event captured",
    zap.String("path", path),
    zap.String("action", action),
    zap.Uint32("raw_action", e.Action),
    zap.Int32("raw_flags", e.Flags),
    zap.String("comm", comm),
    zap.Uint32("pid", e.Pid))
```

## 4. 测试用例

### 4.1 T1547 规则匹配测试

```go
func TestT1547SystemBinaryWriteDetection(t *testing.T) {
    // Load the T1547 rule
    loader := sigma.NewLoader(t.TempDir())
    err := loader.LoadAll([]sigma.Rule{
        {
            ID:    "aegis-file-system-binary-write",
            Title: "System Binary Directory Write",
            Level: "high",
            Tags:  []string{"attack.t1547"},
            Logsource: sigma.Logsource{
                Category: "file_event",
                Product:  "linux",
            },
            Detection: sigma.Detection{
                Selections: map[string]any{
                    "selection_path": map[string]any{
                        "TargetFilename|startswith": []any{
                            "/usr/bin/",
                            "/usr/sbin/",
                            "/bin/",
                            "/sbin/",
                        },
                    },
                    "selection_action": map[string]any{
                        "event.action": []any{
                            "open_write",
                            "create",
                            "truncate",
                            "rename",
                        },
                    },
                },
                Condition: "selection_path and selection_action",
            },
        },
    })
    if err != nil {
        t.Fatalf("load sigma rules: %v", err)
    }

    reporter := &testReporter{}
    collector := NewCollector("host-1", 10)
    pipeline := NewPipeline(collector, loader, reporter, "host-1", monitor.NewMetrics())

    done := make(chan struct{})
    go pipeline.Run(done)

    // Test case: echo "test" > /usr/bin/aegis_test_binary
    collector.events <- Event{
        HostID:      "host-1",
        Hostname:    "test-host",
        EventType:   "file_access",
        ProcessName: "bash",
        PID:         1234,
        PPID:        1,
        UID:         0,
        Timestamp:   time.Now().UnixMilli(),
        FilePath:    "/usr/bin/aegis_test_binary",
        FileName:    "aegis_test_binary",
        FileDir:     "/usr/bin",
        FileAction:  "create",
        FileFlags:   "O_WRONLY,O_CREAT,O_TRUNC",
    }

    close(done)
    time.Sleep(200 * time.Millisecond)

    if reporter.totalEvents() != 1 {
        t.Fatalf("expected exactly 1 reported event for T1547, got %d", reporter.totalEvents())
    }

    reported := reporter.firstEvent()
    if reported.GetMatchedRuleId() != "aegis-file-system-binary-write" {
        t.Fatalf("expected matched rule id aegis-file-system-binary-write, got %q", reported.GetMatchedRuleId())
    }
    if reported.GetMitreId() != "T1547" {
        t.Fatalf("expected mitre id T1547, got %q", reported.GetMitreId())
    }
}
```

### 4.2 字段名大小写不敏感测试

```go
func TestSigmaMatcherFieldNameCaseInsensitive(t *testing.T) {
    rule := sigma.Rule{
        ID:    "test-rule",
        Title: "Test Rule",
        Level: "high",
        Tags:  []string{"attack.t1547"},
        Logsource: sigma.Logsource{
            Category: "file_event",
            Product:  "linux",
        },
        Detection: sigma.Detection{
            Selections: map[string]any{
                "selection": map[string]any{
                    "TargetFilename|startswith": "/usr/bin/",
                },
            },
            Condition: "selection",
        },
    }

    compiled := sigma.CompileRule(&rule)

    // Test with mixed case key (current behavior)
    eventMapMixedCase := map[string]any{
        "category":       "file_event",
        "TargetFilename": "/usr/bin/test",
    }

    // Test with lowercase key (expected by Sigma)
    eventMapLowercase := map[string]any{
        "category":       "file_event",
        "targetfilename": "/usr/bin/test",
    }

    mixedCaseMatch := compiled.Match(eventMapMixedCase)
    lowercaseMatch := compiled.Match(eventMapLowercase)

    if !lowercaseMatch {
        t.Error("Lowercase field name should match")
    }
    if !mixedCaseMatch {
        t.Error("Mixed case field name should also match (case-insensitive lookup needed)")
    }
}
```

### 4.3 事件映射字段名测试

```go
func TestBuildEventMapFileAccessFieldNames(t *testing.T) {
    p := &Pipeline{
        hostname: "test-host",
        metrics:  monitor.NewMetrics(),
    }

    event := Event{
        EventType:   "file_access",
        PID:         1234,
        ProcessName: "bash",
        FilePath:    "/usr/bin/aegis_test_binary",
        FileName:    "aegis_test_binary",
        FileDir:     "/usr/bin",
        FileAction:  "create",
        FileFlags:   "O_WRONLY,O_CREAT,O_TRUNC",
    }

    eventMap := p.buildEventMap(event)

    // Test that category is set correctly
    if eventMap["category"] != "file_event" {
        t.Errorf("category = %q, want %q", eventMap["category"], "file_event")
    }

    // Test that TargetFilename is set (used by Sigma rules)
    targetFilename, ok := eventMap["TargetFilename"]
    if !ok {
        t.Error("TargetFilename not found in event map")
    }
    if targetFilename != "/usr/bin/aegis_test_binary" {
        t.Errorf("TargetFilename = %q, want %q", targetFilename, "/usr/bin/aegis_test_binary")
    }

    // Test that event.action is set (used by Sigma rules)
    action, ok := eventMap["event.action"]
    if !ok {
        t.Error("event.action not found in event map")
    }
    if action != "create" {
        t.Errorf("event.action = %q, want %q", action, "create")
    }

    // Test lowercase field names for Sigma compatibility
    lowerTargetFilename, ok := eventMap["targetfilename"]
    if !ok {
        t.Error("targetfilename (lowercase) not found in event map - Sigma matcher will fail!")
    }
    if lowerTargetFilename != "/usr/bin/aegis_test_binary" {
        t.Errorf("targetfilename = %q, want %q", lowerTargetFilename, "/usr/bin/aegis_test_binary")
    }
}
```

## 5. 实施计划

### 5.1 设计涉及文件清单

| 文件 | 修改内容 |
|------|----------|
| `agent/internal/sigma/matcher.go` | 新增 `lookupFieldCaseInsensitive` 函数，修改 `selectorMatches` 使用大小写不敏感查找 |
| `agent/internal/ebpf/pipeline.go` | 添加文件事件调试日志 |
| `agent/internal/ebpf/loader.go` | 添加 eBPF 文件事件捕获日志 |
| `agent/internal/ebpf/pipeline_test.go` | 新增 T1547 规则匹配测试、字段名大小写测试 |

### 5.2 计划验证

- [x] 执行 `TestT1547SystemBinaryWriteDetection` 测试，验证 T1547 规则正确匹配
- [x] 执行 `TestSigmaMatcherFieldNameCaseInsensitive` 测试，验证大小写不敏感查找
- [x] 执行 `TestBuildEventMapFileAccessFieldNames` 测试，验证事件映射包含正确的字段名
- [x] 执行 `agent/internal/sigma` 包所有测试，确保无回归
- [ ] 编译 agent 并部署到测试环境
- [ ] 执行 `echo "test" > /usr/bin/aegis_test_binary` 命令，验证 T1547 告警生成

### 5.3 部署

```bash
cd agent && make build
# 部署到 /opt/aegis-agent/aegis-agent.new
# 停止服务后替换: sudo mv aegis-agent.new aegis-agent
# 重启服务: sudo systemctl start aegis-agent
```

### 5.4 API 验证

```bash
# 查询 T1547 告警
curl -s "http://localhost:8082/api/v1/detection/alerts?mitre_id=T1547" \
  -H "Authorization: Bearer $TOKEN"
# 计划检查: 返回包含 "System Binary Directory Write" 的告警记录
```

## 6. 已知限制

| 限制项 | 说明 | 缓解方案 |
|--------|------|----------|
| 大小写不敏感查找性能 | 遍历所有 event map 键进行比较，时间复杂度 O(n) | 先尝试精确匹配（快速路径），仅在失败时遍历 |
| 字段名标准化 | 不同来源的事件可能使用不同的字段名大小写 | 在 `buildEventMap` 中同时设置混合大小写和小写键 |

## 7. 相关文档

- [Agent eBPF 检测 Bug 修复设计文档](agent_ebpf_detection_bugfix_design.md)
- [eBPF 文件网络事件设计](ebpf_file_network_event_design.md)

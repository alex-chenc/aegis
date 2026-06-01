# Bug Fix: Event Schema 数据为空

## Bug 描述

Detection Package 详情页的 Event Schema 框数据为空，无论是否有构建记录都显示"暂无 Event Schema"。

## 复现步骤

1. 创建动态检测包草稿并提交构建
2. 构建成功后进入检测包详情页
3. 切换到"Event Schema"tab → 显示"暂无 Event Schema"
4. API 返回 `event_schema: {}`

## 根因分析

### 数据流断点

```
Draft.HookPlanYAML → ensurePackageMetadata() → builder.PackageMetadataJSON → extractEventSchema() → build.EventSchema → package.EventSchema
```

**断点 1：Draft 的 HookPlanYAML 不包含 event_schema**

AI 生成的草稿只包含 `hooks` 字段，不包含 `event_schema`：

```yaml
# 当前 AI 生成的草稿
schema_version: "aegis.ebpf_plugin.v1"
package_id: "xxx"
version: "1.0.0"
hooks:
  - name: sys_enter_socket
    attach_type: tracepoint
    attach: syscalls/sys_enter_socket
    program: tracepoint__syscalls__sys_enter_socket
# ← 缺少 event_schema
```

**断点 2：ensurePackageMetadata 不注入 event_schema**

```go
// api-server/internal/service/detection_package_service.go:820-863
func ensurePackageMetadata(yamlStr, packageID, version string) string {
    // 只确保 schema_version, package_id, version
    // 不处理 event_schema
}
```

**断点 3：Builder 找不到 event_schema**

```go
// builder/internal/service/builder_service.go:530-547
func extractEventSchema(metadataJSON string) string {
    // 从 PackageMetadataJSON 中找 event_schema key
    // 找不到 → 返回空字符串
}
```

**断点 4：ReviewBuild 不同步 event_schema 到 package**

审核通过后只更新 build 状态和 draft 状态，不同步 event_schema 到 detection_packages 表。

## 修复方案

### Fix 1：ensurePackageMetadata 自动生成 event_schema

**文件**: `api-server/internal/service/detection_package_service.go`

当 `event_schema` 不在 metadata 中时，从 `hooks` 自动生成基本 schema：
- 每个 hook 对应一个 event，event ID 从 1001 开始递增
- 每个 event 包含标准 eBPF 元数据字段：timestamp, pid, tid, comm, uid
- hook name 作为 event name

```go
// 新增函数
func generateEventSchemaFromHooks(meta map[string]interface{}) map[string]interface{} {
    hooksRaw, ok := meta["hooks"]
    if !ok {
        return nil
    }
    hooksSlice, ok := hooksRaw.([]interface{})
    if !ok || len(hooksSlice) == 0 {
        return nil
    }

    events := make(map[string]interface{}, len(hooksSlice))
    for i, hookRaw := range hooksSlice {
        hook, ok := hookRaw.(map[string]interface{})
        if !ok {
            continue
        }
        name, _ := hook["name"].(string)
        if name == "" {
            name = fmt.Sprintf("event_%d", 1001+i)
        }
        eventID := fmt.Sprintf("%d", 1001+i)
        events[eventID] = map[string]interface{}{
            "name": name,
            "fields": map[string]interface{}{
                "1": map[string]interface{}{"name": "timestamp", "type": "uint64"},
                "2": map[string]interface{}{"name": "pid", "type": "uint32"},
                "3": map[string]interface{}{"name": "tid", "type": "uint32"},
                "4": map[string]interface{}{"name": "comm", "type": "string"},
                "5": map[string]interface{}{"name": "uid", "type": "uint32"},
            },
        }
    }
    if len(events) == 0 {
        return nil
    }
    return map[string]interface{}{"events": events}
}
```

在 `ensurePackageMetadata` 中调用：

```go
// event_schema: generate from hooks if missing
if _, ok := meta["event_schema"]; !ok {
    if es := generateEventSchemaFromHooks(meta); es != nil {
        meta["event_schema"] = es
        changed = true
    }
}
```

### Fix 2：ReviewBuild 同步 event_schema 到 package

**文件**: `api-server/internal/service/detection_package_service.go`

在 `ReviewBuild` 方法末尾，审核通过后将 event_schema 从 build 同步到 package：

```go
// Sync event_schema from build to the associated package when approved
if approved && build.EventSchema != nil && string(build.EventSchema) != "{}" {
    if pkg, err := s.repo.GetPackage(build.PackageID, build.Version); err == nil && pkg != nil {
        pkg.EventSchema = build.EventSchema
        _ = s.repo.UpdatePackage(pkg)
    }
}
```

## 修复后数据流

```
Draft.HookPlanYAML (有 hooks, 无 event_schema)
  ↓
ensurePackageMetadata() → 自动生成 event_schema from hooks
  ↓
builder.PackageMetadataJSON (有 event_schema)
  ↓
extractEventSchema() → 正常提取
  ↓
build.EventSchema = {"events": {"1001": {...}, "1002": {...}, ...}}
  ↓
ReviewBuild(approved) → 同步到 package.EventSchema
  ↓
前端 EventSchemaTable → 显示事件/字段表
```

## 验证结果

### 构建验证

| 组件 | 命令 | 结果 |
|------|------|------|
| API Server | `docker compose up -d --build api-server` | ✓ 编译成功 |

### API 验证

```
Build event_schema:
  1001: sys_enter_socket
  1002: sys_enter_bind
  1003: sys_enter_splice
  1004: sys_enter_sendmsg
  1005: sys_enter_execve

Package event_schema after review:
  1001: sys_enter_socket
  1002: sys_enter_bind
  1003: sys_enter_splice
  1004: sys_enter_sendmsg
  1005: sys_enter_execve
```

## 受影响文件

| 文件 | 修改类型 |
|------|----------|
| `api-server/internal/service/detection_package_service.go` | 新增 `generateEventSchemaFromHooks` 函数；修改 `ensurePackageMetadata`；修改 `ReviewBuild` |

## 事件 Schema 结构

```json
{
  "events": {
    "1001": {
      "name": "sys_enter_socket",
      "fields": {
        "1": {"name": "timestamp", "type": "uint64"},
        "2": {"name": "pid", "type": "uint32"},
        "3": {"name": "tid", "type": "uint32"},
        "4": {"name": "comm", "type": "string"},
        "5": {"name": "uid", "type": "uint32"}
      }
    }
  }
}
```

## 后续改进

1. **LLM Prompt 增强**: 修改 AI 生成草稿的 prompt，要求生成详细的 event_schema（包含 hook 特定字段）
2. **eBPF 源码解析**: 从编译后的 eBPF ELF sections 提取 event struct 偏移信息，生成更精确的 schema
3. **存量数据修复**: 对已有的空 event_schema 构建/包记录执行数据迁移

## 风险与回滚

- **风险**: 低 - 仅影响 Event Schema 数据生成和同步，不影响核心检测功能
- **回滚**: 如出现问题，可通过 git revert 回滚

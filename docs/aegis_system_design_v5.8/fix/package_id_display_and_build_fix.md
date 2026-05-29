# Bug Fix: Package ID 显示、构建编译错误、Event Schema 数据修复

## Bug 描述与症状

### Bug 1: Package ID 显示不全
- **症状**: 动态检测包管理列表页中，Package ID（UUID 格式如 `44aa6e3c-0ce1-4f54-...`）在表格列中被截断，无法查看完整 ID
- **影响**: 用户无法确认具体的 Package ID，影响操作准确性

### Bug 2: 构建审核报错 compile perf: exit status 1
- **症状**: 对检测包执行构建时，审核中显示 `compile perf: exit status 1` 错误
- **根因**: eBPF 源码编译失败，builder 工作目录中可能存在残留的编译产物导致冲突
- **影响**: 检测包无法完成构建流程

### Bug 3: Event Schema 数据问题
- **症状**:
  - 审核面板（BuildReviewPanel）中显示了 Event Schema，但外层 PackageDetail 的 Event Schema tab 无数据
  - 审核面板中不应重复显示 Event Schema（外层已有独立 tab）
- **根因**:
  - `event_schema` 仅在构建时从 `package_metadata_json` 提取并存储在 `detection_package_builds` 表中
  - `detection_packages` 表的 `event_schema` 字段默认为 `'{}'`，构建完成后未同步
  - 外层 Event Schema tab 数据源优先级问题
- **影响**: 用户无法在外层查看完整的 Event Schema 数据

## 复现步骤

### Bug 1
1. 进入 动态检测包管理 页面
2. 查看 Package ID 列，UUID 被截断
3. 鼠标悬停无法显示完整 ID

### Bug 2
1. 创建检测包并提交构建
2. 构建失败，错误信息显示 `compile perf: exit status 1`

### Bug 3
1. 进入检测包详情页
2. 查看"构建审核"tab - 有 Event Schema 数据
3. 切换到"Event Schema"tab - 显示"暂无 Event Schema"

## 根因分析

### Bug 1: Package ID 显示不全
- **文件**: `frontend/src/views/detection/DetectionPackages/index.vue:58`
- **原因**: `el-table-column` 使用 `show-overflow-tooltip` 但 tooltip 内容默认为截断后的文本
- **修复**: 使用 `el-tooltip` 包裹单元格内容，确保悬停时显示完整的 Package ID

### Bug 2: compile perf: exit status 1
- **文件**: `builder/internal/service/builder_service.go:187-194`
- **原因**: builder 使用 `buildID` 作为工作目录名，但 eBPF 编译的中间产物可能残留
- **修复**: 在构建开始前清理工作目录中的旧编译产物

### Bug 3: Event Schema 数据问题
- **文件**:
  - `frontend/src/views/detection/DetectionPackages/components/BuildReviewPanel.vue:19-20` - 审核面板重复显示
  - `api-server/internal/service/detection_package_service.go` - 构建完成时未同步 event_schema 到 detection_packages 表
- **修复**:
  1. 从 BuildReviewPanel 中移除 Event Schema section
  2. 构建完成并审核通过后，将 event_schema 同步到 detection_packages 表

## 修复设计

### Fix 1: Package ID 悬停显示完整 ID

**修改文件**: `frontend/src/views/detection/DetectionPackages/index.vue`

将 Package ID 列改为使用 `el-tooltip` 包裹，悬停时显示完整 ID。

### Fix 2: 构建前清理旧编译产物

**修改文件**: `builder/internal/service/builder_service.go`

在 `StartBuild` 方法中，创建 buildDir 后先清理旧的编译产物。

### Fix 3: Event Schema 修复

#### 3a: 从审核面板移除 Event Schema

**修改文件**: `frontend/src/views/detection/DetectionPackages/components/BuildReviewPanel.vue`

移除 Event Schema section（第 19-20 行）。

#### 3b: 构建完成后同步 event_schema 到 detection_packages 表

**修改文件**: `api-server/internal/service/detection_package_service.go`

在 `ReviewBuild` 方法中，审核通过后将 event_schema 从 build 同步到 package。

## 受影响组件

| 组件 | 文件 | 修改类型 |
|------|------|----------|
| 前端 - 列表页 | `frontend/src/views/detection/DetectionPackages/index.vue` | 修改 |
| 前端 - 审核面板 | `frontend/src/views/detection/DetectionPackages/components/BuildReviewPanel.vue` | 修改 |
| Builder 服务 | `builder/internal/service/builder_service.go` | 修改 |
| API Server 服务 | `api-server/internal/service/detection_package_service.go` | 修改 |

## 代码变更

### Fix 1: Package ID 悬停显示完整 ID
**文件**: `frontend/src/views/detection/DetectionPackages/index.vue`
```html
<!-- Before -->
<el-table-column prop="package_id" label="Package ID" min-width="200" show-overflow-tooltip />

<!-- After -->
<el-table-column prop="package_id" label="Package ID" min-width="200">
  <template #default="{ row }">
    <el-tooltip :content="row.package_id" placement="top" :show-after="300">
      <span class="package-id-cell">{{ row.package_id }}</span>
    </el-tooltip>
  </template>
</el-table-column>
```

### Fix 2: 构建前清理旧编译产物 + artifact 命名使用 Package ID
**文件**: `builder/internal/service/builder_service.go`
```go
// 在 StartBuild 方法中，创建 buildDir 后添加清理逻辑
// Clean old build artifacts to avoid conflicts from previous builds
if files, _ := filepath.Glob(filepath.Join(buildDir, "*.bpf.o")); files != nil {
    for _, f := range files {
        os.Remove(f)
    }
}
if files, _ := filepath.Glob(filepath.Join(buildDir, "*.tar.gz")); files != nil {
    for _, f := range files {
        os.Remove(f)
    }
}

// artifact 命名从 copyfail 改为使用 Package ID
copyFile(perfObj, filepath.Join(stagingDir, "plugin", req.PackageID+".perf.bpf.o"))
copyFile(ringbufObj, filepath.Join(stagingDir, "plugin", req.PackageID+".ringbuf.bpf.o"))
```

### Fix 3a: 从审核面板移除 Event Schema
**文件**: `frontend/src/views/detection/DetectionPackages/components/BuildReviewPanel.vue`
- 移除 Event Schema divider 和 EventSchemaTable 组件
- 移除未使用的 EventSchemaTable import

### Fix 3b: 构建审核通过后同步 event_schema
**文件**: `api-server/internal/service/detection_package_service.go`
```go
// 在 ReviewBuild 方法末尾添加
if approved && build.EventSchema != nil && string(build.EventSchema) != "{}" {
    if pkg, err := s.repo.GetPackage(build.PackageID, build.Version); err == nil && pkg != nil {
        pkg.EventSchema = build.EventSchema
        _ = s.repo.UpdatePackage(pkg)
    }
}
```

## 构建验证结果

| 组件 | 构建命令 | 结果 |
|------|---------|------|
| Frontend | `npm run build` | ✓ 成功 |
| API Server | `docker compose build api-server` | ✓ 成功 |
| Builder | `docker compose build builder` | ✓ 成功 |

## 验证步骤

1. **Package ID 显示**: 进入列表页，鼠标悬停在 Package ID 列上，应显示完整 UUID
2. **构建编译**: 创建新检测包并构建，应不再出现 `compile perf: exit status 1` 错误
3. **Event Schema**: 进入检测包详情页，外层 Event Schema tab 应显示完整数据；审核面板中不再重复显示 Event Schema

## 风险与回滚计划

- **风险**: 低 - 所有修改均为 UI 显示和数据同步逻辑，不影响核心业务流程
- **回滚**: 如出现问题，可通过 git revert 回滚到修改前的状态

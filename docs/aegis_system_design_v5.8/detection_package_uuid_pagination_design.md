# DetectionPackage UUID Package ID & Pagination Fix Design

**Version**: 5.8
**Date**: 2026-05-28
**Status**: 已实现

---

## 1. 需求

### 1.1 Package ID 使用 UUID

当前 `package_id` 是用户输入的字符串（如 `cve-2026-31431-copyfail`），或 AI 生成时从 CVE ID 派生。用户要求改为使用 UUID 作为 package_id。

**变更范围**:
- 后端：创建草稿时自动生成 UUID 作为 package_id，不再依赖用户输入
- 前端：新建/编辑页面移除 package_id 输入框，列表页显示 UUID 格式的 package_id
- AI 生成：不再从 CVE ID 派生 package_id，直接使用 UUID

### 1.2 检测包列表分页修复

当前 `ListPackages` 服务层存在分页 bug：分别对 `detection_packages` 和 `detection_package_drafts` 两张表独立分页查询，然后合并结果。这导致：
- 单页结果数量可能超过 `pageSize`（最多 2 * pageSize）
- 排序不正确（drafts 和 packages 各自按 updated_at 排序，合并后顺序混乱）
- 总数是两表 count 之和，但实际返回的条目可能覆盖不全

**修复方案**: 使用 UNION 查询在数据库层面合并两张表，统一排序和分页。

---

## 2. 设计方案

### 2.1 UUID Package ID

#### 2.1.1 后端变更

**model/detection_package.go**:
- `PackageID` 字段类型保持 `varchar(160)`，但值改为 UUID 格式
- 无需改数据库 schema（UUID 字符串长度 36 < 160）

**service/detection_package_service.go**:
- `CreateDraft`: 如果 `req.PackageID` 为空，自动生成 `uuid.New().String()`
- `CreateDraftRequest.PackageID` 的 `binding:"required"` 标签移除

**handler/detection_package_handler.go**:
- `AIGenerateDraft`: 不再从 CVE ID 派生 package_id，直接使用 `uuid.New().String()`
- `CreateDraft`: 如果请求中无 package_id，由 service 层自动生成

#### 2.1.2 前端变更

**PackageEditor.vue**:
- 移除 package_id 输入框（新建时不可编辑，编辑时只读显示）
- 新建草稿时不传 package_id，由后端自动生成

**index.vue**:
- package_id 列保持不变（已经是字符串，UUID 也能显示）

### 2.2 分页修复

#### 2.2.1 Repository 层变更

**repository/detection_package_repo.go**:

新增 `ListPackagesUnified` 方法，使用原生 SQL UNION 查询：

```go
func (r *DetectionPackageRepo) ListPackagesUnified(page, pageSize int, status, search string) ([]model.DetectionPackage, int64, error) {
    // 使用 UNION ALL 合并两张表
    // 统一排序、统一 offset/limit
    // 返回合并后的 DetectionPackage 列表
}
```

SQL 策略：
- 使用子查询 UNION ALL `detection_packages` 和 `detection_package_drafts`（drafts 转换为 DetectionPackage 字段格式）
- 在 UNION 外层统一 ORDER BY updated_at DESC
- 外层 COUNT 获取总数
- 外层 OFFSET/LIMIT 分页

#### 2.2.2 Service 层变更

**service/detection_package_service.go**:

简化 `ListPackages` 方法，直接调用 `ListPackagesUnified`：

```go
func (s *DetectionPackageService) ListPackages(ctx context.Context, page, pageSize int, status, search string) ([]model.DetectionPackage, int64, error) {
    return s.repo.ListPackagesUnified(page, pageSize, status, search)
}
```

---

## 3. 影响范围

| 组件 | 变更文件 | 变更内容 |
|------|----------|----------|
| api-server | `service/detection_package_service.go` | CreateDraft 自动生成 UUID；ListPackages 简化 |
| api-server | `repository/detection_package_repo.go` | 新增 ListPackagesUnified |
| api-server | `handler/detection_package_handler.go` | AIGenerateDraft 使用 UUID |
| frontend | `PackageEditor.vue` | 移除 package_id 输入 |
| frontend | `index.vue` | 无变更（已是字符串显示） |

---

## 4. 兼容性

- 已有的 package_id（字符串格式）不受影响，仍可正常查询和操作
- 新创建的检测包将使用 UUID 格式的 package_id
- API 路由 `/packages/:package_id` 参数类型不变（string），UUID 字符串兼容

# Bug Fix: AI 生成草稿后点击 Package 报错"请求的资源不存在"

## Bug 描述

用户使用 AI 生成检测规则草稿后，在 Package 列表中可以看到该草稿，但点击查看详情时返回 HTTP 404 错误，前端显示"请求的资源不存在"。

**受影响的 Package ID 格式**: `cve-XXXX_XXXXX` (下划线格式，如 `cve-2026_31431`)

## 根因分析

`GetPackage` API 端点 (`GET /api/v1/detection/packages/:package_id`) 只查询 `detection_packages` 表，但 AI 生成的草稿存储在 `detection_package_drafts` 表中。

**数据流不一致**:
- `ListPackages` 端点同时查询两个表并合并结果，所以草稿出现在列表中
- `GetPackage` 端点只查询 `detection_packages` 表，找不到草稿记录

**关键代码路径**:
- Handler: `api-server/internal/api/handler/detection_package_handler.go:54-62`
- Service: `api-server/internal/service/detection_package_service.go:466-468` (修复前)
- Repository: `api-server/internal/repository/detection_package_repo.go:60-64`

## 修复方案

修改 `DetectionPackageService.GetPackage()` 方法，当在 `detection_packages` 表中找不到记录时，回退查询 `detection_package_drafts` 表。

**修改文件**: `api-server/internal/service/detection_package_service.go`

```go
// 修复前
func (s *DetectionPackageService) GetPackage(ctx context.Context, packageID string) (*model.DetectionPackage, error) {
	return s.repo.GetLatestPackage(packageID)
}

// 修复后
func (s *DetectionPackageService) GetPackage(ctx context.Context, packageID string) (*model.DetectionPackage, error) {
	pkg, err := s.repo.GetLatestPackage(packageID)
	if err == nil {
		return pkg, nil
	}
	// Fall back to draft table (AI-generated drafts only exist in drafts table)
	draft, draftErr := s.repo.GetDraftByPackageID(packageID)
	if draftErr != nil {
		return nil, err // return original error
	}
	return &model.DetectionPackage{
		ID:          draft.ID,
		PackageID:   draft.PackageID,
		Version:     draft.TargetVersion,
		Title:       draft.Title,
		Description: draft.Description,
		CVEIDs:      draft.CVEIDs,
		Status:      draft.Status,
		CreatedAt:   draft.CreatedAt,
		UpdatedAt:   draft.UpdatedAt,
	}, nil
}
```

## 验证步骤

1. **构建验证**:
   ```bash
   cd api-server && go build ./...
   docker compose up -d --build api-server
   ```

2. **API 测试**:
   ```bash
   TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')

   # 测试 AI 生成的草稿 (之前返回 404)
   curl -s http://localhost:8082/api/v1/detection/packages/cve-2026_31431 \
     -H "Authorization: Bearer ${TOKEN}" | jq .
   ```

3. **预期结果**: 返回 HTTP 200，包含草稿详情

## 受影响组件

- **API Server**: `detection_package_service.go` - GetPackage 方法
- **前端**: 无需修改，使用相同的 API 端点

## 修复时间

2026-05-27

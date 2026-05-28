# Bug Fix: 构建失败不显示错误信息且无法重新构建

## Bug 描述

检测包构建失败后，页面只显示"failed"状态，不显示具体的错误原因。用户无法了解构建失败的原因，也无法重新构建。

## 根因分析

### 问题 1: 不显示错误信息

后端 `detection_package_builds` 表的 `error_message` 字段存储了错误信息，但：

1. **页面加载时不获取构建数据**: `loadPackage()` 函数只获取包详情，不获取构建数据
2. **`currentBuild` 为 null**: 由于不获取构建数据，`BuildReviewPanel` 组件收不到 build 对象
3. **错误信息在组件中**: `BuildReviewPanel.vue` 第 59-61 行有错误信息显示逻辑，但因为 `build` 为 null 所以不显示

### 问题 2: 无法重新构建

1. **构建按钮只在 draft 状态显示**: 第 70-75 行的条件是 `!currentBuild && currentPackage?.status === 'draft'`
2. **构建失败后状态变为 `build_failed`**: 不满足 `draft` 条件，构建按钮不显示
3. **没有重新构建按钮**: `BuildReviewPanel` 只有审核和签名按钮，没有重新构建按钮

## 修复方案

### 1. 后端: 添加获取最新构建的 API

**文件**: `api-server/internal/api/router.go`

添加路由:
```
GET /detection/packages/:package_id/latest-build
```

**文件**: `api-server/internal/api/handler/detection_package_handler.go`

添加 `GetLatestBuild` handler。

**文件**: `api-server/internal/service/detection_package_service.go`

添加 `GetLatestBuild` 方法（已有 repo 方法 `GetLatestBuild`）。

### 2. 前端: 添加 API 方法

**文件**: `frontend/src/api/detection-packages.ts`

```typescript
getLatestBuild: (packageId: string): Promise<DetectionPackageBuild> =>
  request.get(`/detection/packages/${packageId}/latest-build`),
```

### 3. 前端: 加载构建数据

**文件**: `frontend/src/views/detection/DetectionPackages/composables/useDetectionPackages.ts`

添加 `fetchLatestBuild` 函数。

**文件**: `frontend/src/views/detection/DetectionPackages/PackageDetail.vue`

在 `loadPackage()` 中调用 `fetchLatestBuild`:
```typescript
if (currentPackage.value && currentPackage.value.status !== 'draft') {
  await fetchLatestBuild(packageId.value)
}
```

### 4. 前端: 添加重新构建按钮

**文件**: `frontend/src/views/detection/DetectionPackages/PackageDetail.vue`

在构建失败时显示重新构建按钮:
```html
<div v-if="currentBuild?.status === 'build_failed'" style="margin-top: 16px; text-align: center;">
  <el-button type="primary" :loading="loading" :disabled="!canOperate('build')" @click="handleBuild">重新构建</el-button>
</div>
```

## 验证步骤

1. **构建验证**:
   ```bash
   cd api-server && go build ./...
   cd frontend && npm run build
   docker compose up -d --build api-server frontend
   ```

2. **API 测试**:
   ```bash
   TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')

   # 测试获取最新构建
   curl -s http://localhost:8082/api/v1/detection/packages/cve-2026_31431/latest-build \
     -H "Authorization: Bearer ${TOKEN}" | jq .
   ```

3. **预期结果**:
   - API 返回构建数据，包含 `error_message` 字段
   - 前端页面显示构建错误信息
   - 构建失败时显示"重新构建"按钮

## 受影响组件

- **Backend**: `router.go`, `detection_package_handler.go`, `detection_package_service.go`
- **Frontend**: `detection-packages.ts`, `useDetectionPackages.ts`, `PackageDetail.vue`

## 修复时间

2026-05-27

# Bug Fix: 编辑草稿时 HookPlan/eBPF/Sigma/Correlation 标签页数据为空

## Bug 描述

用户使用 AI 生成检测规则草稿后，点击编辑按钮打开编辑页面，发现 HookPlan、eBPF 源码、Sigma 原子规则、Correlation 四个标签页的内容都是空的，但 AI 生成草稿时这些字段都有数据。

## 根因分析

编辑页面 (`PackageEditor.vue`) 调用 `fetchPackage()` 获取包详情，该函数使用 `GET /api/v1/detection/packages/:package_id` 端点。

**问题**:
1. `GetPackage` API 返回 `DetectionPackage` 模型，该模型不包含 `hook_plan_yaml`、`ebpf_source`、`sigma_rules_yaml`、`correlation_yaml` 字段
2. 这些字段只存在于 `DetectionPackageDraft` 模型中
3. 编辑页面没有调用草稿专用的 `GET /api/v1/detection/packages/drafts/:package_id` 端点

**数据模型差异**:
- `DetectionPackage`: 用于已构建的包，不包含草稿专用字段
- `DetectionPackageDraft`: 包含 `hook_plan_yaml`、`ebpf_source`、`sigma_rules_yaml`、`correlation_yaml`

## 修复方案

### 1. 前端 API 层添加 `getDraft` 方法

**文件**: `frontend/src/api/detection-packages.ts`

```typescript
getDraft: (packageId: string): Promise<DetectionPackageDraft> =>
  request.get(`/detection/packages/drafts/${packageId}`),
```

### 2. Composable 添加 `fetchDraft` 函数

**文件**: `frontend/src/views/detection/DetectionPackages/composables/useDetectionPackages.ts`

```typescript
async function fetchDraft(packageId: string) {
  try {
    currentDraft.value = await detectionPackageApi.getDraft(packageId)
  } catch (e: any) {
    // Draft not found is expected for non-draft packages
  }
}
```

### 3. 编辑页面加载草稿数据

**文件**: `frontend/src/views/detection/DetectionPackages/PackageEditor.vue`

```typescript
async function loadDraft() {
  if (isEdit.value) {
    await fetchPackage(route.params.id as string)
    if (currentPackage.value) {
      form.package_id = currentPackage.value.package_id
      form.target_version = currentPackage.value.version
      form.title = currentPackage.value.title
      form.description = currentPackage.value.description || ''
      form.cve_ids = currentPackage.value.cve_ids || []
      // Fetch draft-specific fields if package is a draft
      if (currentPackage.value.status === 'draft') {
        await fetchDraft(currentPackage.value.package_id)
        if (currentDraft.value) {
          form.hook_plan_yaml = currentDraft.value.hook_plan_yaml || ''
          form.ebpf_source = currentDraft.value.ebpf_source || ''
          form.sigma_rules_yaml = currentDraft.value.sigma_rules_yaml || ''
          form.correlation_yaml = currentDraft.value.correlation_yaml || ''
        }
      }
    }
  }
}
```

## 验证步骤

1. **构建验证**:
   ```bash
   cd frontend && npm run build
   docker compose up -d --build frontend
   ```

2. **API 测试**:
   ```bash
   TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')

   # 测试 GetDraft 端点
   curl -s http://localhost:8082/api/v1/detection/packages/drafts/cve-2026_31431 \
     -H "Authorization: Bearer ${TOKEN}" | jq '.data | {hook_plan_yaml: (.hook_plan_yaml | length), ebpf_source: (.ebpf_source | length)}'
   ```

3. **预期结果**:
   - API 返回草稿数据，包含 `hook_plan_yaml`、`ebpf_source`、`sigma_rules_yaml`、`correlation_yaml` 字段
   - 前端编辑页面四个标签页显示正确内容

## 受影响组件

- **Frontend API**: `detection-packages.ts` - 添加 `getDraft` 方法
- **Frontend Composable**: `useDetectionPackages.ts` - 添加 `fetchDraft` 函数
- **Frontend Component**: `PackageEditor.vue` - 修改 `loadDraft` 函数

## 修复时间

2026-05-27

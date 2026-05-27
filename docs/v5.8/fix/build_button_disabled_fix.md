# Bug Fix: 提交构建按钮无效

## Bug 描述

用户在检测包详情页面点击"提交构建"按钮，按钮显示为禁用状态（灰色），无法点击。

## 根因分析

前端 `useRole` composable 中角色默认值硬编码为 `security_analyst`：

```typescript
const currentRole = ref<Role>('security_analyst')
```

`security_analyst` 角色的权限列表为 `['view', 'draft', 'ai_generate']`，不包含 `'build'` 权限。

**问题链**:
1. `useRole` composable 默认角色为 `security_analyst`
2. `setRole()` 函数从未被调用
3. `canOperate('build')` 返回 `false`
4. 构建按钮的 `:disabled="!canOperate('build')"` 属性为 `true`
5. 按钮被禁用

**根本原因**: 前端没有从后端获取用户角色，也没有在登录时存储角色。

## 修复方案

### 1. 后端: 在登录和 Me 端点返回角色

**文件**: `api-server/internal/api/handler/auth_handler.go`

- 添加 `roleRepo` 字段到 `AuthHandler`
- 修改 `Login` 端点返回 `role` 字段
- 修改 `Me` 端点返回 `role` 字段

### 2. 后端: 调整初始化顺序

**文件**: `api-server/cmd/main.go`

- 将 `roleRepo` 创建移到 `authHandler` 之前
- 将 `roleRepo` 传递给 `NewAuthHandler`

### 3. 前端: 存储角色

**文件**: `frontend/src/api/auth.ts`

- `AuthSession` 接口添加 `role` 字段

**文件**: `frontend/src/utils/auth.ts`

- `StoredAuth` 接口添加 `role` 字段
- `saveAuthSession` 函数存储 `role`

### 4. 前端: 获取并使用角色

**文件**: `frontend/src/composables/useRole.ts`

- 从 `localStorage` 读取已存储的角色
- 如果未存储，从 `GET /auth/me` 端点获取角色
- 将获取到的角色存储到 `localStorage`

## 验证步骤

1. **构建验证**:
   ```bash
   cd api-server && go build ./...
   cd frontend && npm run build
   docker compose up -d --build api-server frontend
   ```

2. **API 测试**:
   ```bash
   # 登录并检查 role 字段
   curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"Cc&324511"}' | jq .

   # Me 端点
   TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')
   curl -s http://localhost:8082/api/v1/auth/me \
     -H "Authorization: Bearer ${TOKEN}" | jq .

   # 测试构建端点
   curl -s -X POST http://localhost:8082/api/v1/detection/packages/cve-2026_31431/build \
     -H "Authorization: Bearer ${TOKEN}" | jq .
   ```

3. **预期结果**:
   - 登录响应包含 `"role": "admin"`
   - Me 端点包含 `"role": "admin"`
   - 前端构建按钮可点击（不再禁用）

## 受影响组件

- **Backend**: `auth_handler.go`, `main.go`
- **Frontend**: `auth.ts`, `auth.ts (utils)`, `useRole.ts`

## 修复时间

2026-05-27

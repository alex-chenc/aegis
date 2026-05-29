# Bug Fix: 提交构建按钮禁用（role_permissions 表为空）

## Bug 描述

用户以 admin 账号登录，在检测包详情页面点击"构建审核"tab，看到"草稿状态，尚未构建"提示，但"提交构建"按钮显示为禁用状态（灰色），无法点击。

## 复现步骤

1. 全新部署 Aegis 系统
2. 使用 admin/Cc&324511 登录
3. 创建一个检测包（草稿状态）
4. 进入检测包详情 → 构建审核 tab
5. "提交构建"按钮为灰色禁用状态

## 根因分析

### 问题链

1. 全新安装时，迁移 `008_detection_package_v5.8_fix.sql` 创建 `role_permissions` 表
2. 迁移中的种子逻辑从 `detection_package_operations` 表读取操作员：
   ```sql
   INSERT INTO role_permissions (user_id, role)
   SELECT DISTINCT operator, 'admin' FROM detection_package_operations
   WHERE operator IS NOT NULL AND operator != ''
   ON CONFLICT (user_id) DO NOTHING;
   ```
3. 全新安装时 `detection_package_operations` 表为空 → 没有角色被种子
4. admin 用户通过 bootstrap login 创建，`ChangeCredentials` 端点没有调用 `roleRepo.SetRole()`
5. 登录时 `auth_handler.go` 调用 `roleRepo.GetRole("admin")` → `ErrRecordNotFound` → 返回默认值 `"security_analyst"`
6. 前端 `useRole.ts` 存储角色为 `security_analyst`
7. `canOperate('build')` 检查 `PERMISSIONS['security_analyst']` → 不包含 `'build'` → 返回 `false`
8. 按钮 `:disabled="!canOperate('build')"` 为 `true` → 按钮禁用

### 关键发现

`RoleRepo.GetRole()` 在找不到记录时返回默认值 `"security_analyst"` 而不是错误，导致调用方无法区分"用户角色是 security_analyst"和"用户没有角色记录"。

## 修复方案

### 1. 后端: RoleRepo 添加 HasRoleRecord 和 HasAnyRoles 方法

**文件**: `api-server/internal/repository/role_repo.go`

- `HasRoleRecord(userID)`: 检查指定用户是否有角色记录
- `HasAnyRoles()`: 检查 role_permissions 表是否有任何记录（用于判断是否全新安装）

### 2. 后端: AuthHandler 添加 resolveUserRole 方法

**文件**: `api-server/internal/api/handler/auth_handler.go`

`resolveUserRole(username)` 逻辑：
1. 用户有角色记录 → 返回该角色
2. 用户无角色记录且 role_permissions 表为空（全新安装）→ 自动分配 admin 角色
3. 用户无角色记录但表不为空 → 分配 security_analyst（最低权限）

Login 和 Me 端点使用 `resolveUserRole` 替代直接调用 `GetRole`。

### 3. 数据库: 迁移种子 admin 用户角色

**文件**: `api-server/migrations/008_detection_package_v5.8_fix.sql`

添加从 `auth_users` 表种子角色的逻辑：
```sql
INSERT INTO role_permissions (user_id, role)
SELECT username, 'admin' FROM auth_users
WHERE username NOT IN (SELECT user_id FROM role_permissions)
ON CONFLICT (user_id) DO NOTHING;
```

## 验证步骤

1. **API 测试**:
   ```bash
   # 登录检查 role 字段
   curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"Cc&324511"}' | jq .

   # Me 端点
   TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')
   curl -s http://localhost:8082/api/v1/auth/me \
     -H "Authorization: Bearer ${TOKEN}" | jq .
   ```

2. **预期结果**:
   - 登录响应包含 `"role": "admin"`
   - Me 端点包含 `"role": "admin"`
   - 前端构建按钮可点击（不再禁用）

3. **回归测试**:
   ```bash
   cd api-server && go test ./internal/api/handler/ -run TestAuthHandler -v
   ```

## 受影响组件

- **Backend**: `auth_handler.go`, `role_repo.go`
- **Database**: `migrations/008_detection_package_v5.8_fix.sql`

## 安全考虑

- 自动分配 admin 角色仅在 `role_permissions` 表完全为空时生效（全新安装场景）
- 如果系统已有角色记录（非全新安装），无角色记录的用户将获得最低权限 `security_analyst`
- 防止未来用户创建功能引入权限提升风险

## 风险与回滚

- **风险**: 低。仅影响角色初始化逻辑，不影响现有功能
- **回滚**: 如果修复导致问题，可从 `role_permissions` 表删除对应记录

## 修复时间

2026-05-29

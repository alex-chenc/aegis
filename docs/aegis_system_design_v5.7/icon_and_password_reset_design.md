# 图标迁移与密码重置设计文档

**版本**: V5.7
**日期**: 2026-05-09
**状态**: 设计中

---

## 1. 图标文件迁移

### 1.1 需求描述

将系统图标 `icon.png` 迁移到前端项目内部目录 `frontend/docs/img/icon.png`，并更新前端引用位置。

### 1.2 当前状态

| 文件路径 | 说明 |
|:---|:---|
| `docs/img/icon.png` | 仓库根目录设计文档中的图标 |
| `frontend/public/icon.png` | 前端静态资源（Vite public 目录） |
| `frontend/dist/icon.png` | 构建产物中的图标 |

三个文件内容完全一致（MD5: `06f78f43c5e81b05f74aacae7acce2eb`）。

当前引用位置：`frontend/index.html` 第5行：
```html
<link rel="icon" type="image/png" href="/icon.png">
```

### 1.3 设计方案

1. 创建 `frontend/public/docs/img/` 目录
2. 复制 `docs/img/icon.png` 到 `frontend/public/docs/img/icon.png`
3. 更新 `frontend/index.html` 中的引用路径为 `/docs/img/icon.png`

**注意**: Vite 项目中，只有 `public/` 目录下的文件会被直接复制到构建产物中。因此需要将图标放在 `frontend/public/docs/img/icon.png`，引用路径为 `/docs/img/icon.png`。

### 1.4 影响范围

- `frontend/index.html` - favicon 引用路径
- `frontend/public/docs/img/icon.png` - 新增静态资源

---

## 2. 密码重置机制

### 2.1 需求描述

系统管理员密码 `Admin@123` 无法登录。经排查，数据库中的 bcrypt 密码哈希与 `Admin@123` 不匹配。系统当前缺少密码重置机制，管理员一旦忘记密码将完全无法登录。

### 2.2 问题根因

1. 系统使用两阶段引导流程：`BootstrapLogin` → `ChangeCredentials`
2. `ChangeCredentials` 设置的密码未被记录，用户可能设置了不同于 `Admin@123` 的密码
3. 系统没有密码重置功能，一旦密码丢失无法恢复
4. `BootstrapLogin` 仅在 `PasswordHash` 为空时可用，密码设置后无法再次使用

### 2.3 设计方案

#### 方案：数据库迁移重置 + 管理员密码重置 API

**阶段一：立即修复（数据库迁移）**

通过数据库迁移脚本重置管理员密码为 `Admin@123`：

```sql
-- migrations/011_v5.7_reset_admin_password.sql
UPDATE auth_users
SET password_hash = '<bcrypt_hash_of_Admin@123>',
    force_password_change = false,
    updated_at = NOW()
WHERE username = 'admin';
```

**阶段二：密码重置 API**

新增管理员密码重置端点，允许通过数据库中的重置密钥重置密码：

```
POST /api/v1/auth/reset-password
Body: { "reset_key": "<key>", "new_password": "<password>", "confirm_password": "<password>" }
```

重置密钥存储在 `system_configs` 表中，每次重置后自动更新。

### 2.4 数据库变更

#### 新增系统配置

在 `system_configs` 表中新增密码重置密钥配置：

```sql
INSERT INTO system_configs (category, key, value, description)
VALUES ('auth', 'password_reset_key', '<random_hex>', '管理员密码重置密钥');
```

### 2.5 API 变更

| 方法 | 路径 | 说明 | 认证 |
|:---|:---|:---|:---|
| POST | /api/v1/auth/reset-password | 通过重置密钥重置密码 | 无需认证 |

### 2.6 安全考虑

1. 重置密钥为一次性使用，重置后自动更换
2. 密码重置操作记录到审计日志
3. 重置密钥仅存储在数据库中，不暴露给前端
4. 密码最低8位，与现有策略一致

### 2.7 影响范围

- `api-server/internal/service/auth_service.go` - 新增 `ResetPassword` 方法
- `api-server/internal/api/handler/auth_handler.go` - 新增 `ResetPassword` handler
- `api-server/internal/repository/auth_repo.go` - 新增重置密钥相关查询
- `migrations/011_v5.7_reset_admin_password.sql` - 重置密码迁移

---

## 3. 头部用户菜单与登录后修改密码

### 3.1 需求描述

在主页面顶部刷新按钮右侧增加用户头像图标，点击后弹出下拉菜单，包含：
1. **版本信息** — 点击后弹框显示系统版本号（硬编码为 `V5.7`）
2. **修改密码** — 已登录用户可修改自己的密码
3. **退出登录** — 退出当前登录，返回登录页

### 3.2 设计方案

#### 3.2.1 后端：已登录用户修改密码 API

新增 `POST /api/v1/auth/change-password` 端点，需要认证：

```
POST /api/v1/auth/change-password
Header: Authorization: Bearer <token>
Body: {
  "current_password": "<当前密码>",
  "new_password": "<新密码>",
  "confirm_password": "<确认密码>"
}
```

**处理逻辑**：
1. 验证 JWT Token 有效性
2. 验证当前密码是否正确
3. 验证新密码与确认密码一致，且长度 ≥ 8，且必须同时包含字母和数字
4. 更新密码哈希
5. 使当前用户的所有会话失效
6. 创建新会话并返回新 Token

**与 reset-password 的区别**：
| 特性 | reset-password | change-password |
|:---|:---|:---|
| 认证要求 | 无需认证 | 需要 Bearer Token |
| 验证方式 | 数据库重置密钥 | 当前密码 |
| 使用场景 | 密码丢失恢复 | 已登录用户主动修改 |
| 会话处理 | 使所有会话失效 | 使所有会话失效，返回新 Token |

#### 3.2.2 前端：用户头像下拉菜单

在 `App.vue` 头部右侧刷新按钮右侧新增头像图标组件 `UserProfileDropdown`：

- 使用 `el-dropdown` 组件
- 头像图标使用 Element Plus 的 `UserFilled` 图标
- 下拉菜单包含：
  - "版本信息" — 点击后弹出 `el-dialog` 显示产品名称和版本号
  - 分割线
  - "修改密码" — 弹出密码修改对话框
  - "退出登录" — 调用 `POST /api/v1/auth/logout` 并清除本地 Token，跳转登录页

#### 3.2.3 前端：修改密码对话框

新增 `ChangePasswordDialog.vue` 组件：
- 使用 `el-dialog` 弹窗
- 包含当前密码、新密码、确认密码三个输入框
- 前端校验：密码长度 ≥ 8，必须包含字母和数字，两次输入一致
- 调用 `POST /api/v1/auth/change-password` 接口
- 成功后更新本地存储的 Token（因为旧会话已失效）

### 3.3 API 变更

| 方法 | 路径 | 说明 | 认证 |
|:---|:---|:---|:---|
| POST | /api/v1/auth/change-password | 已登录用户修改密码 | 需要 Bearer Token |

### 3.4 安全考虑

1. 修改密码需要验证当前密码
2. 修改后所有会话失效，需重新创建会话
3. 新 Token 通过响应返回，前端自动更新本地存储
4. 密码策略：最低 8 位，必须同时包含字母和数字

### 3.5 影响范围

**后端**：
- `api-server/internal/service/auth_service.go` — 新增 `ChangePassword` 方法
- `api-server/internal/api/handler/auth_handler.go` — 新增 `ChangePassword` handler 和请求结构体
- `api-server/internal/api/handler/auth_handler.go` — 注册新路由

**前端**：
- `frontend/src/api/auth.ts` — 新增 `changePassword` API 函数
- `frontend/src/components/UserProfileDropdown.vue` — 新增用户头像下拉菜单组件（版本弹框、修改密码、退出登录）
- `frontend/src/components/common/ChangePasswordDialog.vue` — 新增修改密码对话框组件
- `frontend/src/App.vue` — 在头部右侧集成 `UserProfileDropdown`

---

## 4. 登录错误提示与密码错误限流

### 4.1 需求描述

1. 登录页面输入错误密码时显示"密码错误"提示
2. 连续 3 次输入密码错误后，该用户禁止 10 分钟内再次登录

### 4.2 设计方案

#### 4.2.1 前端错误提示

修改 `Login.vue` 的 `handleLogin` 方法，添加 `catch` 块捕获 API 错误，使用 `ElMessage.error()` 显示服务端返回的错误消息。

#### 4.2.2 后端登录限流

使用 Redis 实现基于用户名的登录失败计数：

- **Key 格式**: `auth:login:fail:{username}`
- **TTL**: 10 分钟
- **最大尝试次数**: 3 次

**处理流程**:
1. 登录前检查 Redis 中该用户的失败计数
2. 若计数 ≥ 3，返回 HTTP 429，提示剩余锁定时间
3. 登录失败时递增计数（首次失败设置 10 分钟 TTL）
4. 登录成功时清除计数

**限流粒度**: 按用户名隔离，一个用户的失败不影响其他用户

#### 4.2.3 错误响应

| 场景 | HTTP 状态码 | 消息 |
|:---|:---|:---|
| 密码错误 | 401 | 密码错误 |
| 超过限流阈值 | 429 | 密码错误次数过多，请 X 分钟后再试 |

### 4.3 影响范围

**后端**：
- `api-server/internal/storage/redis_client.go` — 新增登录限流方法（`IncrementLoginFail`、`GetLoginFailCount`、`GetLoginFailTTL`、`ClearLoginFail`）
- `api-server/internal/service/auth_service.go` — 新增 `LoginRateLimitError` 类型，`Login` 方法增加限流检查
- `api-server/internal/api/handler/auth_handler.go` — `writeAuthError` 增加 429 状态码处理，错误消息改为"密码错误"
- `api-server/cmd/main.go` — `NewAuthService` 传入 `redisClient`

**前端**：
- `frontend/src/views/Login.vue` — `handleLogin` 增加错误捕获和 `ElMessage.error` 提示

# 会话空闲超时调整为 2 小时设计文档

## 1. 需求背景

当前系统前端空闲自动退出时间为 5 分钟，用户体验较差——用户在阅读页面、思考操作时容易被强制退出登录。需要将空闲超时调整为 2 小时，在安全性和用户体验之间取得更好的平衡。

## 2. 现有机制分析

### 2.1 前端空闲超时（本次修改目标）

- **常量定义**: `frontend/src/utils/sessionTimeout.ts` 第 1 行
  - `IDLE_LOGOUT_TIMEOUT_MS = 5 * 60 * 1000`（5 分钟）
- **监听事件**: `mousemove`, `mousedown`, `keydown`, `touchstart`, `scroll`, `visibilitychange`
- **触发位置**: `frontend/src/App.vue` 第 196-203 行
  - 已登录且不在认证页面时启用
  - 超时后清除本地认证状态，提示"5 分钟未操作，已自动退出登录"，跳转 `/login`
- **测试文件**: `frontend/src/utils/sessionTimeout.test.ts`

### 2.2 后端 Session TTL（本次不修改）

- **Token 有效期**: `api-server/internal/service/auth_service.go` 第 77 行
  - `sessionTTL: 24 * time.Hour`（24 小时绝对过期）
- **无滑动窗口机制**: Token 一旦签发，24 小时后绝对过期，不受用户活动影响
- **Token 校验**: 数据库查询 `WHERE token_hash = ? AND expires_at > ?`

### 2.3 关系说明

| 层 | 超时类型 | 当前值 | 修改后 | 说明 |
|---|---|---|---|---|
| 前端 | 空闲超时（无操作） | 5 分钟 | 2 小时 | 用户无交互时自动退出 |
| 后端 | Token 绝对有效期 | 24 小时 | 24 小时（不改） | Token 签发后的最大存活时间 |

前端空闲超时 < 后端 Token 有效期，因此前端空闲超时是用户体验的主要约束。将前端空闲超时从 5 分钟调整为 2 小时，仍远小于后端 24 小时的绝对有效期，安全边界保持不变。

## 3. 修改方案

### 3.1 前端常量修改

**文件**: `frontend/src/utils/sessionTimeout.ts`

```typescript
// 修改前
export const IDLE_LOGOUT_TIMEOUT_MS = 5 * 60 * 1000

// 修改后
export const IDLE_LOGOUT_TIMEOUT_MS = 2 * 60 * 60 * 1000
```

### 3.2 前端提示信息修改

**文件**: `frontend/src/App.vue` 第 200 行

```typescript
// 修改前
ElMessage.warning('5 分钟未操作，已自动退出登录')

// 修改后
ElMessage.warning('2 小时未操作，已自动退出登录')
```

### 3.3 测试用例更新

**文件**: `frontend/src/utils/sessionTimeout.test.ts`

- 更新测试描述从 "five minutes" 改为 "two hours"
- 测试逻辑不变（使用 `IDLE_LOGOUT_TIMEOUT_MS` 常量，自动适配）

### 3.4 设计文档同步

**文件**: `docs/aegis_system_design_v5.6/frontend_detailed_design_v5.6.md` 第 69-74 行及第 83 行

- 将"5 分钟"描述更新为"2 小时"

## 4. 不修改的部分

| 项目 | 原因 |
|---|---|
| 后端 sessionTTL (24h) | 前端空闲超时 2h 仍远小于 24h，无需调整 |
| Axios 请求超时 (5min) | 这是单次 HTTP 请求超时，非会话超时 |
| Login 速率限制 (3次/10min) | 与会话超时无关 |
| 401 拦截器 | 由后端 Token 过期触发，前端空闲超时调整不影响此逻辑 |

## 5. 验收标准

- 已登录用户 2 小时无操作后自动退出，提示"2 小时未操作，已自动退出登录"
- 用户活动（鼠标、键盘等）正常重置计时器
- 登录页和强制改密页不启动空闲计时
- 现有单元测试通过

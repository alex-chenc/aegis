# 登录页回车键确认登录设计

**版本**: 5.7
**日期**: 2026-05-09
**状态**: 已完成
**类型**: Bug修复 / 用户体验优化

---

## 1. 问题描述

登录页面（`Login.vue`）中，用户在输入账号或密码后按回车键（Enter）无法触发登录操作，必须手动点击"登录"按钮才能提交表单。这违反了常见的 Web 表单交互习惯，影响用户体验。

### 根因分析

Element Plus 的 `el-button` 组件默认 `native-type="button"`，而非 HTML 原生的 `"submit"`。当前登录按钮代码：

```vue
<el-button
  type="primary"
  size="large"
  class="submit-button"
  :loading="submitting"
  :disabled="submitting"
  @click="handleLogin"
>
  登录
</el-button>
```

虽然 `el-form` 上已绑定 `@submit.prevent="handleLogin"`，但由于按钮不是 `type="submit"`，浏览器在 input 中按 Enter 时不会触发表单的 submit 事件。

---

## 2. 解决方案

### 方案：为登录按钮添加 `native-type="submit"`

将 `el-button` 的 `native-type` 设置为 `"submit"`，使其成为 HTML 原生提交按钮。这样：

1. 用户在任意 input 中按 Enter 时，浏览器自动触发表单 submit 事件
2. `@submit.prevent="handleLogin"` 捕获事件并执行登录逻辑
3. 无需额外的 `@keydown.enter` 监听器，代码更简洁

**修改文件**: `frontend/src/views/Login.vue`

**修改内容**:
```vue
<!-- 修改前 -->
<el-button
  type="primary"
  ...
  @click="handleLogin"
>

<!-- 修改后 -->
<el-button
  type="primary"
  native-type="submit"
  ...
>
```

同时移除按钮上的 `@click="handleLogin"`，因为表单的 `@submit.prevent` 已经处理了提交逻辑。

**额外防护**：在 `handleLogin` 函数顶部添加 `if (submitting.value) return` 防抖守卫，防止并发执行（如快速双击或 Enter + 点击同时触发）。

---

## 3. 影响范围

| 项目 | 说明 |
|------|------|
| 修改文件 | `frontend/src/views/Login.vue` |
| 影响功能 | 登录表单提交行为 |
| 向后兼容 | 完全兼容，不影响现有功能 |
| 安全影响 | 无，仅修改 UI 交互方式 |

---

## 4. 测试策略

### 4.1 单元测试（Vitest）

在 `Login.test.ts` 中新增：

1. **回车键触发登录测试**：触发表单 submit 事件，验证 `login` 函数被调用且仅调用 1 次
2. **表单提交测试**：验证 `native-type="submit"` 后表单 submit 事件正确触发且仅调用 1 次

两个测试均使用 `expect(loginMock).toHaveBeenCalledTimes(1)` 断言防止重复调用回归。

### 4.2 接口测试（curl）

```bash
# 获取 token
TOKEN=$(curl -s -X POST http://localhost:8082/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Cc&324511"}' | jq -r '.token')

# 验证 token 有效性
curl -s http://localhost:8082/api/v1/auth/me \
  -H "Authorization: Bearer $TOKEN"
```

---

## 5. 设计决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 实现方式 | `native-type="submit"` | 符合 HTML 标准，最简洁 |
| 是否移除 `@click` | 移除 | 避免与 `@submit.prevent` 重复绑定 |
| 是否需要 `@keydown.enter` | 不需要 | 会导致与 `@submit.prevent` 双重调用 |
| 防抖守卫 | `if (submitting.value) return` | 防止并发执行和重复 API 请求 |

---

## 6. Code Review 发现与修正

### 6.1 [HIGH] 双重调用问题

初版实现中，密码输入框添加了 `@keydown.enter="handleLogin"`，与表单的 `@submit.prevent="handleLogin"` 冲突，导致 Enter 键按下时 `handleLogin` 被调用两次。

**原因**：`@keydown.enter` 不会调用 `preventDefault()`，浏览器在 keydown 后仍会触发表单 submit 事件。

**修正**：移除 `@keydown.enter`，仅依赖 `native-type="submit"` + `@submit.prevent` 的原生表单提交机制。

### 6.2 [MEDIUM] 测试断言不完整

初版测试仅断言 `toHaveBeenCalledWith`，未验证调用次数。

**修正**：两个测试均添加 `expect(loginMock).toHaveBeenCalledTimes(1)` 断言。

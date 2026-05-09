# V5.7 按钮可读性修复设计

**版本**: 5.7
**日期**: 2026-05-09
**状态**: 已设计

---

## 1. 问题描述

系统内表格操作区、弹窗确认区和工具栏中存在大量 `type="primary"` 按钮。当前全局主题直接覆盖 `.el-button--primary`，导致 `link type="primary"` 的文字按钮也被渲染成蓝色胶囊背景；同时主按钮叠加 `text-shadow` 与 `letter-spacing`，在“详情”等短中文小字号按钮上会产生发糊、发灰、边缘不清的问题。

## 2. 设计目标

- 普通 primary 按钮保持蓝底白字，但文字必须清晰、稳定、可读。
- `link type="primary"` 保持文字链接形态，不出现蓝色填充背景。
- 中文按钮不使用额外字距和文字阴影，避免小字号下发虚。
- 保留 hover、active、focus 的状态反馈，并满足普通文字 WCAG AA 4.5:1 对比度。

## 3. 样式策略

### 3.1 普通 primary 按钮

仅对 `.el-button--primary:not(.is-link)` 应用填充样式：

- 背景使用深蓝实色 `--aegis-action-blue-dark`，白色文字对比度高于 4.5:1。
- 显式设置 `color: #ffffff` 及 Element Plus 按钮文字变量，避免组件变量继承异常。
- 移除 `text-shadow` 与 `letter-spacing`，让中文字符边缘由字体渲染本身承担。
- 使用 `line-height: 1.2`，并沿用全局按钮较高字重，保证表格小按钮文字边缘稳定，不额外撑高表格行。

### 3.2 link primary 按钮

对 `.el-button.is-link.el-button--primary` 设定链接样式：

- 背景、边框、阴影均为透明/无。
- 文字使用深蓝 `--aegis-action-blue-dark`，hover 只改变文字色和浅色底。
- 不使用白字，不继承普通 primary 的蓝色填充。

## 4. 验收标准

- 主题测试覆盖普通 primary 与 link primary 的样式隔离。
- 普通 primary 不包含 `text-shadow` 与非零 `letter-spacing`。
- link primary 不被 `.el-button--primary` 的蓝底白字规则命中。
- 前端完成 `npm run test -- src/styles/aegis-theme.test.ts`、`npm run build` 验证。
- 服务验证使用 `curl` 登录 `admin/Cc&324511` 获取 token，并携带 token 调用接口确认前后端链路可用。

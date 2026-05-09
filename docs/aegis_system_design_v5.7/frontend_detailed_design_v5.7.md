# V5.7 前端详细设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 变更概述

V5.7前端新增两个页面：**命令审计配置** 和 **审计日志**，以及任务状态展示增强。

---

## 2. 新增页面

### 2.1 命令审计配置页

**路由**: `/settings/command-audit`

**组件结构**:
```
src/views/settings/CommandAudit/
├── index.vue
├── components/
│   ├── AuditPolicyCard.vue       # 审计策略开关（2列网格布局）
│   ├── RuleTable.vue             # 规则列表
│   └── RuleFormDialog.vue        # 新增/编辑对话框
└── composables/useCommandAudit.ts
```

#### 审计策略卡片
采用2列网格布局，每项策略包含图标、名称、描述和开关：
- 黑名单审计（图标B，红色调）- 基于预置规则的确定性检查，命中即拦截
- AI 审计（图标AI，紫色调）- 基于大模型的上下文风险分析，检测隐蔽威胁
- 下发前校验（图标P，橙色调）- 脚本从 API Server 下发前再次校验黑名单
- Agent 侧校验（图标A，绿色调）- Agent 执行前的最后一道防线
- 最大重试次数（1-5）- AI审计失败后重新生成脚本的最大尝试次数

#### 规则列表
- 搜索（名称+模式）
- 筛选：分类、严重等级、匹配类型、状态
- 操作：新增、编辑、删除（预置不可删）、启停
- 预置规则有明显标识

### 2.2 审计日志页

**路由**: `/settings/audit-logs`

**组件结构**:
```
src/views/settings/AuditLogs/
├── index.vue
├── components/
│   ├── AuditStatsCard.vue        # 统计卡片
│   ├── AuditLogTable.vue         # 日志列表
│   └── AuditDetailDrawer.vue     # 详情抽屉
└── composables/useAuditLogs.ts
```

#### 统计卡片
- 总审计次数
- 通过/失败次数
- 通过率（颜色编码：>=90%绿色，<90%黄色）
- 重试分布（1次/2次/3次/失败）

#### 日志列表
- 时间、脚本类型、审计来源、尝试次数
- 结果（通过/失败标签）
- 风险等级（颜色编码）
- 耗时
- 详情按钮

#### 详情抽屉
- 脚本内容（代码高亮）
- 黑名单命中详情（规则名、行号、匹配文本）
- AI审计结果（issues列表）
- 审计时间线（多次尝试的记录）

---

## 3. 现有页面改造

### 3.1 全局按钮可读性修复

**设计文档**: `button_readability_design.md`

全局主题对 Element Plus primary 按钮做样式隔离：

- 普通 `.el-button--primary:not(.is-link)` 保持深蓝实色背景与白字，显式设置文字色、行高和字重。
- 移除 primary 按钮上的 `text-shadow` 与额外 `letter-spacing`，避免短中文按钮在小字号下发糊。
- `.el-button.is-link.el-button--primary` 保持文字链接形态，不继承蓝底白字填充样式。
- hover、active、focus 状态继续通过颜色、浅底和焦点环提供反馈。

### 3.2 任务状态增强

```vue
<!-- 新增audit_blocked状态 -->
<el-tag v-else-if="row.status === 'audit_blocked'" type="danger">
  脚本审计未通过
</el-tag>
```

任务详情页增加审计信息展示：
```
状态: 脚本审计未通过
错误信息: 脚本存在恶意命令，下发已阻止。
命中规则:
  1. [critical] curl管道执行 (第5行)
[查看审计日志] [重新生成脚本]
```

#### API响应格式

`GET /api/v1/tasks/:id/logs` 和 `GET /api/v1/tasks/:id` 对 `AUDIT_BLOCKED` 状态的任务返回 `audit_info` 字段：

```json
{
  "id": "task-uuid",
  "status": "AUDIT_BLOCKED",
  "stderr": "脚本存在恶意命令，下发已阻止。",
  "audit_info": {
    "hit_rules": [
      {"rule_name": "curl管道执行", "severity": "critical", "line_number": 5, "matched_text": "curl | bash"}
    ],
    "error_message": "脚本存在恶意命令，下发已阻止。",
    "audit_log_id": "audit-log-uuid"
  }
}
```

`audit_info` 字段仅在 `status === "AUDIT_BLOCKED"` 时存在，其他状态为 `null`。
```

### 3.3 导航菜单

```typescript
{
  path: '/settings/command-audit',
  name: 'CommandAudit',
  meta: { title: '命令审计配置', icon: 'shield-check' }
},
{
  path: '/settings/audit-logs',
  name: 'AuditLogs',
  meta: { title: '审计日志', icon: 'document-checked' }
}
```

---

## 4. API封装

```typescript
// src/api/command-audit.ts
export const commandAuditApi = {
  getRules: (params) => request.get('/api/v1/settings/command-audit/rules', { params }),
  createRule: (data) => request.post('/api/v1/settings/command-audit/rules', data),
  updateRule: (id, data) => request.put(`/api/v1/settings/command-audit/rules/${id}`, data),
  deleteRule: (id) => request.delete(`/api/v1/settings/command-audit/rules/${id}`),
  toggleRule: (id) => request.put(`/api/v1/settings/command-audit/rules/${id}/toggle`),
  testPattern: (data) => request.post('/api/v1/settings/command-audit/rules/test', data),
  getSettings: () => request.get('/api/v1/settings/command-audit/settings'),
  updateSettings: (data) => request.put('/api/v1/settings/command-audit/settings', data),
}

// src/api/audit-logs.ts
export const auditLogApi = {
  getLogs: (params) => request.get('/api/v1/settings/audit-logs', { params }),
  getLog: (id) => request.get(`/api/v1/settings/audit-logs/${id}`),
  getStats: () => request.get('/api/v1/settings/audit-logs/stats'),
}
```

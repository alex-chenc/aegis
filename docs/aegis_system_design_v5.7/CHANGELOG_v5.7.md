# Aegis V5.7 变更日志

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 新增功能

### 命令审计配置（REQ-001~008）
- 系统设置新增「命令审计配置」页面
- 支持正则表达式和精确匹配两种规则类型
- 预置15条安全规则（涵盖文件系统、权限、网络、系统、特权5大类）
- 规则支持分类、严重等级、适用脚本类型配置
- 内置正则测试面板，支持在线验证规则匹配
- 全局策略开关：黑名单审计、AI审计、下发前校验、Agent侧校验

### 统一脚本审计服务（REQ-101~107）
- 新增 `ScriptAuditService`，统一处理所有5种脚本类型的审计
- 黑名单审计 + AI审计双层检查机制
- 支持最多3次重试，每次重试注入失败原因引导LLM修正
- 覆盖脚本类型：基线审计、修复、漏洞修复、漏洞POC、自愈

### 下发前黑名单校验（REQ-201~206）
- 任务下发Agent前增加黑名单二次校验
- 三层防御：生成阶段(fail-close) → 下发阶段(fail-open) → Agent侧(fail-open)
- Agent侧规则同步复用现有 `UpdateRules` gRPC RPC

### eBPF文件与网络事件采集（REQ-301~307）
- 接入 `openat` eBPF程序，采集文件访问事件
- 接入 `connect` eBPF程序，采集网络连接事件
- connect增强：支持IPv6、源地址、回环过滤
- openat内核态敏感路径过滤（/etc、/root、/var、/tmp）
- 用户态采样（openat 10%、connect 100%）与去重

### eBPF内核版本适配（REQ-401~408）
- 内核能力检测模块：自动识别BTF、ringbuf支持情况
- 三级降级策略：ringbuf(5.8+) → perf buffer(4.18+) → /proc轮询
- CO-RE与非CO-RE双编译产物
- Makefile新增 `bpf-noncore` 和 `bpf-all` 目标
- 补充 `exit` 程序到BPF_PROGRAMS

### 智能体优化（REQ-501~506）
- ReAct迭代参数优化：最大迭代50→20，无动作容忍2→3
- Observation智能截断：JSON数组保留头尾各5条，替代硬截断
- 工具调用安全边界：单会话上限100次，单工具10次/分钟
- Prometheus可观测性指标：迭代次数、工具调用、失败率、会话时长
- Session持久化：DB+内存双存储，启动恢复，30分钟超时清理

### 审计日志（REQ-601~608）
- 系统设置新增「审计日志」页面
- 统计卡片：总审计次数、通过/失败次数、通过率、重试分布
- 日志列表：时间、脚本类型、审计来源、结果、风险等级、耗时
- 详情抽屉：脚本内容、黑名单命中详情、AI分析结果、审计时间线

---

## 改造功能

### 任务状态增强
- 新增 `audit_blocked` 状态标签（脚本审计未通过）
- 任务详情页展示审计信息：命中规则、错误信息、审计日志链接

### 脚本生成服务改造
- 移除 `validateScript()` 硬编码校验
- 替换为 `ScriptAuditService.AuditWithRetry()` 统一审计

### 自愈服务改造
- 移除 `validateScript()` 硬编码校验
- 替换为 `ScriptAuditService.AuditWithRetry()` 统一审计

### 漏洞服务增强
- 漏洞修复和POC脚本增加 `ScriptAuditService.AuditWithRetry()` 调用

### 导航菜单
- 新增「命令审计配置」菜单项（shield-check图标）
- 新增「审计日志」菜单项（document-checked图标）

---

## 数据库变更

### 新增表
| 表名 | 说明 |
|:---|:---|
| command_audit_rules | 命令审计规则 |
| script_audit_log | 脚本审计日志 |
| system_configs | 通用系统配置 |

### 新增索引
- `idx_command_audit_rules_category`
- `idx_command_audit_rules_enabled`
- `idx_script_audit_log_task_id`
- `idx_script_audit_log_created_at`
- `idx_system_configs_category`

### 初始化数据
- 15条预置审计规则
- 1条命令审计全局配置

---

## 新增API

| 方法 | 路径 | 说明 |
|:---|:---|:---|
| GET | /api/v1/settings/command-audit/rules | 获取规则列表 |
| POST | /api/v1/settings/command-audit/rules | 创建规则 |
| PUT | /api/v1/settings/command-audit/rules/:id | 更新规则 |
| DELETE | /api/v1/settings/command-audit/rules/:id | 删除规则 |
| PUT | /api/v1/settings/command-audit/rules/:id/toggle | 启停规则 |
| POST | /api/v1/settings/command-audit/rules/batch-toggle | 批量启停 |
| POST | /api/v1/settings/command-audit/rules/test | 测试正则匹配 |
| GET | /api/v1/settings/command-audit/settings | 获取审计配置 |
| PUT | /api/v1/settings/command-audit/settings | 更新审计配置 |
| GET | /api/v1/settings/audit-logs | 获取审计日志 |
| GET | /api/v1/settings/audit-logs/:id | 获取日志详情 |
| GET | /api/v1/settings/audit-logs/stats | 获取审计统计 |

---

## V5.7 Bug修复记录 (2026-05-08)

### Bug 1: 审计拦截信息未传递到前端
**现象**: 脚本被审计拦截后，任务状态显示"检测中"→"检测超时"，而非"审计未通过"
**根因**: `TaskLogResponse` 结构体缺少 `AuditInfo` 字段。当任务被 `AUDIT_BLOCKED` 时，`stderr` 中包含拦截原因（纯文本），但前端期望的结构化 `audit_info`（含 `hit_rules`、`error_message`、`audit_log_id`）从未被填充。
**修复**: 在 `TaskLogResponse` 中新增 `AuditInfo` 字段，`GetTaskLogs` handler 中对 `AUDIT_BLOCKED` 状态的任务查询 `script_audit_log` 表填充审计信息。

### Bug 2: 审计日志API响应字段名不匹配
**现象**: 审计日志页面不显示数据
**根因**: 后端 `ListLogs` 返回 `{"logs": [...]}` 但前端期望 `res.items`。后端使用 `logs` 字段名，前端使用 `items`。
**修复**: 统一为 `items` 字段名。

### Bug 3: 审计统计API响应字段名不匹配
**现象**: 审计统计卡片显示空数据
**根因**: 后端 `GetStats` 返回 `{"total_audits": N}` 但前端期望 `res.total`。后端使用 `total_audits`/`passed`/`failed`/`pass_rate`/`by_source`/`by_type`，前端期望 `total`/`passed`/`failed`/`pass_rate`/`retry_distribution`。
**修复**: 后端返回字段名与前端接口对齐。

### Bug 4: 前端缺少全局中文字体设置
**现象**: 蓝色按钮上的中文文字显示不清晰
**根因**: 应用未设置全局 `font-family`，Element Plus 默认字体栈不包含优质中文字体。
**修复**: 在 `index.html` 中添加全局 CSS，设置包含 "PingFang SC"、"Microsoft YaHei"、"Noto Sans SC" 的字体栈。

### Bug 5: 数据库约束缺少 AUDIT_BLOCKED 状态
**现象**: 脚本被审计拦截后，任务状态显示"检测中"→"检测超时"，而非"审计未通过"
**根因**: 数据库 `task_logs` 表的检查约束 `chk_task_status` 只允许 `PENDING, RUNNING, SUCCESS, FAILED, TIMEOUT, HEALING`，不包含 `AUDIT_BLOCKED`。当审计检测到恶意脚本并尝试将状态更新为 `AUDIT_BLOCKED` 时，数据库报错 `violates check constraint "chk_task_status"`，导致状态更新失败，任务保持 `PENDING` 状态，最终超时变为 `TIMEOUT`。
**修复**: 更新数据库约束，添加 `AUDIT_BLOCKED` 状态。在 `migrations/010_v5.7_command_audit.sql` 中添加约束更新语句。

---

## V5.7 功能增强 (2026-05-09)

### 功能 1: 图标文件迁移
**需求**: 将系统图标迁移到前端项目内部目录
**方案**: 图标放置于 `frontend/public/docs/img/icon.png`，`index.html` 引用路径更新为 `/docs/img/icon.png`

### 功能 2: 管理员密码重置机制
**现象**: 管理员密码 `Admin@123` 无法登录，数据库中的 bcrypt 哈希与该密码不匹配
**根因**: 系统缺少密码重置功能，密码一旦丢失无法恢复
**方案**:
1. 数据库迁移重置管理员密码为 `Admin@123`
2. 新增 `POST /api/v1/auth/reset-password` API，支持通过重置密钥重置密码
3. 重置密钥存储在 `system_configs` 表中，每次重置后自动更换

### Bug 6: 蓝底白字按钮中文显示发糊
**现象**: 表格操作区“详情”等短中文按钮显示为蓝色胶囊背景，文字边缘发虚、阅读负担高。
**根因**: 全局主题直接覆盖 `.el-button--primary`，误伤 `link type="primary"` 文字按钮；同时普通 primary 按钮使用 `text-shadow` 和额外 `letter-spacing`，在小字号中文按钮上降低清晰度。
**修复**: 普通 primary 填充样式限定为 `.el-button--primary:not(.is-link)`；link primary 恢复透明文字链接形态；移除 primary 按钮文字阴影和额外字距，显式设置白色文字、行高、字重与焦点环。

---

## 设计文档索引

| 文档 | 说明 |
|:---|:---|
| prd_design_v5.7.md | 产品需求文档 |
| script_audit_service_design.md | 统一脚本审计服务设计 |
| command_audit_blacklist_config_design.md | 命令审计配置设计 |
| ai_audit_retry_design.md | AI审计与重试机制设计 |
| pre_dispatch_blacklist_validation_design.md | 下发前校验设计 |
| ebpf_file_network_event_design.md | eBPF文件/网络事件设计 |
| ebpf_kernel_adaptation_design.md | eBPF内核适配设计 |
| agent_optimization_design.md | 智能体优化设计 |
| backend_detailed_design_v5.7.md | 后端详细设计 |
| frontend_detailed_design_v5.7.md | 前端详细设计 |
| database_structure_design_v5.7.md | 数据库结构设计 |
| icon_and_password_reset_design.md | 图标迁移与密码重置设计 |
| button_readability_design.md | 按钮可读性修复设计 |

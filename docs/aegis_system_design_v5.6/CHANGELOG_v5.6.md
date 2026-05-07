# Aegis V5.6 版本变更日志

**版本**: 5.6.0
**日期**: 2026-05-06
**状态**: 已完成

---

## 1. 版本概述

V5.6版本在V5.5微服务架构基础上，聚焦**智能异常检测模块全面增强**，核心升级包括：

- **Sigma规则上传解析**: 支持用户上传Sigma规则文件（YAML/ZIP），自动解析入库并精确下发Agent
- **AI降噪多轮分析**: 引入LangChain模式，AI可进行多轮对话并调用Agent工具深入调查
- **Agent智能体化**: Agent从事件采集器升级为智能体，具备工具调用和自主决策能力
- **单Host精确下发**: 所有命令均通过host_id精确下发到指定Agent，消除广播模式
- **控制台认证系统**: 新增用户认证、会话管理、强制改密等安全机制
- **AI规则更新优化**: 保守策略映射、规则质量门控、冷却期机制

---

## 2. 新增功能

### 2.1 Sigma规则上传解析

#### 功能描述

用户可在规则管理页面上传Sigma规则文件（单个YAML、多个YAML或ZIP压缩包），系统自动解析、验证并入库。

#### 核心能力

| 功能 | 说明 |
|------|------|
| 文件上传 | 支持 `.yaml`、`.yml` 单文件（最多10个）、ZIP压缩包 |
| 解析验证 | 解析Sigma规则YAML结构，验证必填字段（title、logsource、detection） |
| MITRE映射 | 自动提取MITRE ID（通过tags匹配attack.t开头的标签） |
| MITRE验证 | MITRE ID为空的规则不允许导入 |
| MITRE去重 | 相同MITRE ID的规则不允许重复导入 |
| 严重程度映射 | Sigma level自动映射为系统严重等级（critical/high/medium/low） |
| 状态初始化 | 新导入规则默认为 `pending` 状态 |
| 批量操作 | 支持批量启用、批量禁用、批量删除 |

#### 规则状态语义

| 状态 | 中文 | 是否下发Agent | 含义 |
|------|------|---------------|------|
| `pending` | 待审核 | 否 | AI建议或人工导入后等待确认 |
| `experimental` | 实验性 | 是 | 已下发试运行，可能误报，处于观察期 |
| `active` | 已激活 | 是 | 正式启用规则 |
| `disabled` | 已禁用 | 否 | 停用规则，并从Agent删除 |

#### API接口

| API | 方法 | 说明 |
|-----|------|------|
| `/api/v1/detection/rules/upload` | POST | 上传Sigma规则文件 |
| `/api/v1/detection/rules` | GET | 获取规则列表（支持分页、筛选） |
| `/api/v1/detection/rules/:id` | PUT | 更新规则状态 |
| `/api/v1/detection/rules/batch` | POST | 批量操作（启用/禁用/删除） |

---

### 2.2 AI规则自动更新

#### 功能描述

当系统检测到高频异常事件时，自动调用LLM分析并生成/更新Sigma规则。支持两种模式：

- **仅建议模式 (`suggest`)**: AI只生成建议，不改变线上检测规则，需人工审核
- **自动模式 (`auto`)**: AI可以直接更新线上规则，但仍进入观察期

#### 配置项

| 配置 | 说明 | 默认值 |
|------|------|--------|
| 启用开关 | 是否启用AI规则更新 | 关闭 |
| 更新模式 | suggest（仅建议）/ auto（自动） | suggest |
| 高频阈值时间窗口 | 检测高频告警的时间范围 | 1小时 |
| 高频阈值次数 | 触发AI更新的告警次数阈值 | 10次 |
| 保守策略滑块 | 0.0（极保守）到 1.0（激进） | 0.5 |

#### 保守策略映射

| 范围 | 置信度要求 | 冷却期 | 行为 |
|------|-----------|--------|------|
| `0.0-0.2` | `0.90` | `24h` | 极低触发率，拒绝弱更新 |
| `0.2-0.4` | `0.85` | `12h` | 保守 |
| `0.4-0.6` | `0.80` | `6h` | 平衡 |
| `0.6-1.0` | `0.70` | `1h` | 激进 |

#### API接口

| API | 方法 | 说明 |
|-----|------|------|
| `/api/v1/detection/rules/ai-rule-config` | GET | 获取AI规则更新配置 |
| `/api/v1/detection/rules/ai-rule-config` | PUT | 更新AI规则更新配置 |

---

### 2.3 AI降噪多轮分析

#### 功能描述

用户可在告警中心选择多个告警事件，结合时间范围，进行AI多轮对话式降噪分析。系统引入LangChain模式，让大模型能够主动调用Agent工具进行深入调查。

#### 核心能力

| 功能 | 说明 |
|------|------|
| 多选降噪 | 支持勾选多个告警同时分析 |
| 时间范围 | 支持设置分析的时间范围（1小时/6小时/24小时/自定义） |
| 多轮对话 | AI能够进行多轮交互，逐步深入分析 |
| 工具调用 | AI可以调用Agent工具获取更多上下文（进程树、网络连接等） |
| 全链路展示 | SSE流式输出，实时展示AI思考过程、工具调用、结果返回 |
| RAG增强 | 基于历史相似案例辅助分析 |
| 攻击溯源图 | 分析完成后以可视化攻击链路图展示 |

#### Agent工具列表

| 工具名称 | 参数 | 返回值 | 说明 |
|----------|------|--------|------|
| `GetProcessTree` | host_id, pid | 进程树结构 | 获取指定进程的完整树状结构 |
| `GetNetworkConnections` | host_id, pid | 网络连接列表 | 获取进程的网络连接情况 |
| `GetOpenFiles` | host_id, pid | 打开文件列表 | 获取进程打开的文件描述符 |
| `GetRunningProcesses` | host_id, filter | 进程列表 | 获取当前运行的进程 |
| `GetUserSessions` | host_id | 用户会话列表 | 获取当前登录用户会话 |
| `QueryHistoricalLogs` | host_id, start_time, end_time, filter | 日志条目 | 查询历史日志 |

#### API接口

| API | 方法 | 说明 |
|-----|------|------|
| `/api/v1/detection/ai-analysis/start` | POST | 启动AI分析会话 |
| `/api/v1/detection/ai-analysis/stream` | GET | SSE流式获取分析结果 |
| `/api/v1/detection/ai-analysis/sessions` | GET | 获取分析会话列表 |
| `/api/v1/detection/ai-analysis/sessions/:id` | GET | 获取分析会话详情 |

---

### 2.4 控制台认证系统

#### 功能描述

新增控制台用户认证机制，支持用户登录、会话管理、强制首次改密等安全功能。

#### 核心能力

| 功能 | 说明 |
|------|------|
| 用户认证 | 基于用户名/密码的登录认证 |
| 会话管理 | Token-based会话，支持过期自动失效 |
| 强制改密 | 首次登录必须修改默认密码 |
| 空闲登出 | 配置无操作自动登出时间 |
| 密码安全 | 密码使用bcrypt哈希存储，数据库仅存SHA-256摘要 |

#### 数据库表

```sql
-- 控制台认证用户表
CREATE TABLE auth_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(64) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL DEFAULT '',
    force_password_change BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 控制台登录会话表
CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### API接口

| API | 方法 | 说明 |
|-----|------|------|
| `/api/v1/auth/login` | POST | 用户登录 |
| `/api/v1/auth/logout` | POST | 用户登出 |
| `/api/v1/auth/change-password` | POST | 修改密码 |
| `/api/v1/auth/status` | GET | 获取认证状态 |

---

### 2.5 单Host精确下发

#### 功能描述

V5.6版本将所有命令下发均改为通过host_id精确指定目标Agent，消除V5.5及之前的广播模式。

#### 行为变更

| 场景 | V5.5行为 | V5.6行为 |
|------|---------|---------|
| 规则更新 | 广播所有Agent | 根据主机IP精确下发到相关Agent |
| 阻断命令 | 精确到单Host | 保持精确（已正确） |
| 工具调用 | N/A | 根据host_id精确调用 |
| 远程诊断 | 广播 | 根据host_id精确执行 |

#### 实验性规则下发策略

实验性规则（`experimental`）也参与全量下发，不再按24小时静默期过滤：

- 全量同步必须包含 `active` 和所有 `experimental` 规则
- AI规则更新设为 `experimental` 后继续立即广播给在线Agent
- Agent重连后也能通过全量同步拿到刚进入 `experimental` 的规则
- `experimental` 仍保留7天后自动晋升 `active` 的观察期语义

---

### 2.6 通知系统

#### 功能描述

新增消息通知功能，支持AI规则生成、阻断事件等关键操作的通知推送。

#### 核心能力

| 功能 | 说明 |
|------|------|
| 通知列表 | 展示系统通知，支持已读/未读状态 |
| 通知抽屉 | 点击铃铛图标从右侧滑出通知面板 |
| 全部标为已读 | 一键标记所有通知为已读 |
| 实时推送 | 关键事件发生时实时推送通知 |

---

## 3. Bug修复

### 3.1 AI规则自动更新失常

**问题**: 配置了最保守规则生成后，告警未减少，反而持续增加。

**根因**:
1. DC BlockManager `ShouldAutoBlock` 检查的是 `AutoDispose` 而非 `AutoBlock`
2. `CheckAndAutoDispose` 从未在RuntimePipelineService中被调用
3. 实验性规则24小时静默期导致全量同步延迟
4. 广播顺序错误，可能导致Agent端未正确处理增量更新

**修复**:
- 修正 `ShouldAutoBlock` 检查 `AutoBlock` 字段
- 在 `onWindowFlush` 中添加 `CheckAndAutoDispose` 调用
- 实验性规则不再按24小时过滤，直接参与全量下发
- 使用 `full` 模式广播确保Agent收到完整规则

### 3.2 AI规则状态语义混乱

**问题**: `experimental` 同时被用来表达"试运行"和"未正式启用/可能不下发"，用户无法判断规则是否已下发到Agent。

**修复**:
- 重新定义状态语义：`experimental` 必须下发，`pending` 必须不下发
- `suggest` 模式下，AI对已有规则的优化创建新的 `pending` 建议规则，不覆盖原规则
- `auto` 模式下，AI新生成或优化的规则进入 `experimental`，立即下发
- 前端状态列通过悬浮提示说明是否下发到Agent

### 3.3 保守策略未生效

**问题**: 保守策略滑块只影响手动/测试规则生成提示词，自动收紧路径使用固定的误报置信度阈值。

**修复**:
- 将 `conservatism` 映射到自动收紧的置信度阈值和冷却期
- 新增规则质量门控：验证condition引用的selector存在、拒绝自然语言片段
- 新增冷却期机制：规则在配置的冷却窗口内不重复收紧

### 3.4 Agent Sigma匹配器条件语义

**问题**: Agent端matcher忽略Sigma condition语义，将所有selection字段视为单一宽泛OR集合。

**修复**:
- Agent matcher支持condition语义评估
- 支持单selector、布尔运算符（and/or/not）、括号、glob表达式
- 不支持的condition语法返回无匹配（fail closed）

### 3.5 通知抽屉不可见

**问题**: 点击铃铛图标后，通知抽屉内容显示在页面最底部，用户无法看到。

**修复**: 在 `el-drawer` 组件上添加 `append-to-body` 属性，使抽屉渲染到 `document.body`。

### 3.6 阻断失败原因不透明

**问题**: 告警列表只显示"阻断失败"，失败原因需要进入详情才能看到。

**修复**:
- Agent执行阻断失败时返回可读原因
- Server保留Agent原始原因，不泛化为"发送失败"
- API将失败原因保存到 `block_records.message` 与 `alerts.block_message`
- 前端告警列表和详情直接显示失败原因

### 3.7 前端桌面布局在移动端异常

**问题**: 桌面端布局在移动浏览器上显示异常。

**修复**:
- 保持桌面布局在移动端浏览器上的显示
- 添加viewport缩放适配
- 初始化viewport控制台渲染

---

## 4. 数据库变更

### 4.1 新增表

| 表名 | 说明 |
|------|------|
| `ai_analysis_session` | AI分析会话表 |
| `ai_analysis_message` | AI分析消息表 |
| `tool_execution_log` | 工具执行日志表 |
| `image_model_configs` | 图片模型配置表 |
| `auth_users` | 控制台认证用户表 |
| `auth_sessions` | 控制台登录会话表 |

### 4.2 字段变更

| 表 | 变更 | 说明 |
|------|------|------|
| `llm_configs` | 新增 `provider` 列 | LLM提供商标识 |

---

## 5. 文件变更清单

### 5.1 后端 (API Server)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/service/rule_generation_service.go` | 新增 | AI规则生成服务、保守策略映射、冷却期机制 |
| `internal/service/rule_generation_service_test.go` | 新增 | 规则生成服务单元测试 |
| `internal/api/handler/detection_handler.go` | 修改 | Sigma规则上传、AI分析、规则配置API |
| `internal/api/router.go` | 修改 | 新增认证、规则上传、AI分析路由 |
| `internal/service/alert_service.go` | 修改 | 添加CheckAndAutoDispose调用 |
| `internal/service/runtime_pipeline_service.go` | 修改 | 修复广播顺序和模式 |
| `internal/repository/sigma_rule_repo.go` | 修改 | GetActiveAndExperimental返回所有experimental规则 |
| `internal/llm/prompts.go` | 修改 | AI分析提示词优化 |

### 5.2 后端 (Server)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/grpc_server/` | 修改 | 工具调用路由、单Host精确下发 |

### 5.3 后端 (DC)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/block_manager/block_manager.go` | 修复 | ShouldAutoBlock检查AutoBlock字段 |

### 5.4 后端 (Agent)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/sigma/` | 修改 | Sigma matcher支持condition语义 |
| `internal/tools/` | 新增 | Agent工具执行器（GetProcessTree等） |

### 5.5 前端 (Frontend)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `src/views/detection/Rules.vue` | 修改 | Sigma规则上传、规则列表增强 |
| `src/views/detection/Alerts.vue` | 修改 | AI降噪多轮分析、阻断失败原因显示 |
| `src/components/ai-analysis/` | 新增 | AI分析组件（SSE流、攻击溯源图） |
| `src/components/notification/NotificationDrawer.vue` | 修复 | 添加append-to-body属性 |
| `src/views/auth/Login.vue` | 新增 | 登录页面 |
| `src/store/auth.ts` | 新增 | 认证状态管理 |
| `src/views/settings/ModelConfig.vue` | 修改 | 模型配置页面 |
| `src/views/settings/AgentInstall.vue` | 新增 | Agent安装指南页面 |

### 5.6 数据库迁移

| 文件 | 说明 |
|------|------|
| `migrations/` | 新增ai_analysis_session、ai_analysis_message、tool_execution_log、auth_users、auth_sessions表 |

---

## 6. API变更

### 6.1 新增API

| API | 方法 | 说明 |
|-----|------|------|
| `/api/v1/auth/login` | POST | 用户登录 |
| `/api/v1/auth/logout` | POST | 用户登出 |
| `/api/v1/auth/change-password` | POST | 修改密码 |
| `/api/v1/auth/status` | GET | 获取认证状态 |
| `/api/v1/detection/rules/upload` | POST | 上传Sigma规则文件 |
| `/api/v1/detection/rules/batch` | POST | 批量操作规则 |
| `/api/v1/detection/rules/ai-rule-config` | GET | 获取AI规则更新配置 |
| `/api/v1/detection/rules/ai-rule-config` | PUT | 更新AI规则更新配置 |
| `/api/v1/detection/ai-analysis/start` | POST | 启动AI分析会话 |
| `/api/v1/detection/ai-analysis/stream` | GET | SSE流式获取分析结果 |
| `/api/v1/detection/ai-analysis/sessions` | GET | 获取分析会话列表 |
| `/api/v1/detection/ai-analysis/sessions/:id` | GET | 获取分析会话详情 |
| `/api/v1/detection/alerts/:id/block` | POST | 手动阻断告警 |
| `/api/v1/notifications` | GET | 获取通知列表 |
| `/api/v1/notifications/read-all` | POST | 全部标为已读 |

---

## 7. 测试验证

### 7.1 Sigma规则上传测试

```bash
# 上传单个Sigma规则
curl -X POST http://localhost:8082/api/v1/detection/rules/upload \
  -F "file=@reverse_shell.yaml"

# 上传ZIP压缩包
curl -X POST http://localhost:8082/api/v1/detection/rules/upload \
  -F "file=@rules.zip"

# 验证规则列表
curl http://localhost:8082/api/v1/detection/rules
```

### 7.2 AI规则更新测试

```bash
# 获取AI规则配置
curl http://localhost:8082/api/v1/detection/rules/ai-rule-config

# 更新配置为自动模式
curl -X PUT http://localhost:8082/api/v1/detection/rules/ai-rule-config \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "mode": "auto", "conservatism": 0.3}'
```

### 7.3 AI分析测试

```bash
# 启动AI分析会话
curl -X POST http://localhost:8082/api/v1/detection/ai-analysis/start \
  -H "Content-Type: application/json" \
  -d '{"alert_ids": ["ALT-001", "ALT-002"], "time_range": {"hours": 1}}'

# SSE流式获取分析结果
curl -N http://localhost:8082/api/v1/detection/ai-analysis/stream?session_id=xxx
```

### 7.4 认证测试

```bash
# 登录
curl -X POST http://localhost:8082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "admin123"}'

# 修改密码
curl -X POST http://localhost:8082/api/v1/auth/change-password \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"old_password": "admin123", "new_password": "new_secure_password"}'
```

### 7.5 阻断策略主机验证

```bash
# 测试kill_process阻断
curl -X POST http://localhost:8082/api/v1/detection/alerts/ALT-xxx/block \
  -H "Content-Type: application/json" \
  -d '{"action": "kill_process"}'

# 测试quarantine_file阻断
curl -X POST http://localhost:8082/api/v1/detection/alerts/ALT-xxx/block \
  -H "Content-Type: application/json" \
  -d '{"action": "quarantine_file"}'

# 测试block_connection阻断
curl -X POST http://localhost:8082/api/v1/detection/alerts/ALT-xxx/block \
  -H "Content-Type: application/json" \
  -d '{"action": "block_connection"}'
```

---

## 8. 部署说明

### 8.1 构建步骤

```bash
# 后端服务
cd api-server && make build
cd server && make build
cd dc && make build

# Agent（需要先构建eBPF程序）
cd agent && make bpf && make build

# 前端
cd frontend && npm install && npm run build

# Docker全栈部署
cp .env.example .env
docker compose up -d --build
```

### 8.2 数据库迁移

V5.6新增多张数据库表，启动时会自动执行迁移。如需手动迁移：

```bash
# 连接数据库执行迁移脚本
psql -h localhost -U postgres -d aegis -f migrations/*.sql
```

### 8.3 配置变更

- **LLM配置**: 新增 `provider` 字段，需配置LLM提供商（openai/anthropic）
- **认证配置**: 首次部署需初始化admin用户
- **Agent配置**: Agent需更新到V5.6版本以支持工具调用

---

## 9. 已知限制

1. **Agent工具调用**: Agent需保持与Server的长连接，网络中断时工具调用会失败
2. **AI分析迭代次数**: 单次AI分析最多支持100轮迭代
3. **Sigma规则格式**: 目前仅支持标准Sigma规则格式，自定义扩展暂不支持
4. **实验性规则观察期**: 实验性规则7天后自动转为active，暂不支持自定义观察期

---

## 10. 后续计划

1. **V5.7**:
   - 多主机并行扫描优化
   - 扫描结果导出功能
   - 更丰富的Agent工具集
   - 规则版本管理与回滚

---

**文档结束**

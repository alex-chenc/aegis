# Bug Fix 设计文档：审批模式配置页面缺失 & Agent 工具调用条件修复

## 文档信息

| 项目 | 内容 |
|------|------|
| 版本 | V6.0 |
| 日期 | 2026-06-06 |
| 类型 | Bug Fix |
| 影响范围 | 前端（审批配置页面）、后端（工具选择逻辑） |

---

## Bug 1：3种审批模式配置页面缺失

### Bug 描述

V6.0 设计文档定义了三种全局审批模式（`request_approval` / `whitelist` / `full_access`），后端 API 完整实现了审批模式的读写接口，前端 API 层也定义了类型和函数，但**配置页面从未创建**，用户无法在界面上切换审批模式或管理工具白名单。

### 根因分析

- 后端 `ToolPolicyService` 完整实现了 `GetApprovalMode` / `SetApprovalMode` / `UpdateWhitelist` / `BatchUpdateWhitelist` / `ResetDefaultWhitelist`
- 后端 HTTP 路由已注册：`GET/PUT /assistant/tool-approval-policy`、`PUT /assistant/tools/:name/whitelist` 等
- 前端 `api/assistant.ts` 已定义 `getToolApprovalPolicy()` / `updateToolApprovalPolicy()` 等 API 函数
- 前端 `api/assistant.ts` 已定义 `AssistantToolApprovalMode` / `AssistantToolPolicy` / `AssistantToolWhitelistEntry` 类型
- **缺失**：`AssistantToolPolicySettings.vue` 组件未创建
- **缺失**：路由 `/settings/tool-policy` 未注册

### 修复方案

1. 创建 `frontend/src/views/settings/AssistantToolPolicySettings.vue` 组件
2. 在 `frontend/src/router/index.ts` 中注册路由 `/settings/tool-policy`
3. 组件功能：
   - 审批模式三选一（Radio Group）
   - 工具白名单表格（搜索、筛选、单个/批量操作）
   - 恢复默认白名单按钮

### 影响组件

- `frontend/src/views/settings/AssistantToolPolicySettings.vue`（新增）
- `frontend/src/router/index.ts`（修改）

---

## Bug 2：Agent 工具调用条件不准确

### Bug 描述

当前 LLM 对 agent 工具（`Agent.Process.List`、`Agent.Network.List` 等）的调用不准确。对于简单的资源查询（如"查看主机列表"、"有哪些资产"），LLM 不应该调用 agent 工具去目标主机采集进程/网络信息，而应该直接使用服务端数据库查询工具（如 `Host.List`）。

Agent 工具应该只在以下安全分析场景下被调用：
- 事件分析、安全分析
- 攻击研判、入侵溯源
- 威胁检测、告警处理

### 根因分析

**问题出在 `ToolSelector.Select()` 工具选择阶段**：

1. Agent 工具注册属性：`Domain=agent`, `Risk=readonly`, `AutoCallable=true`, `DefaultWhitelisted=true`
2. 当用户查询涉及主机相关关键词时，`IntentRouter.Classify()` 将 domain 识别为 `host`
3. `ToolSelector.scoreTool()` 中，agent 域工具虽然 domain 不直接匹配 `host`，但 agent 工具的 `Tags` 包含 "forensics"、`ObjectTypes` 包含 "host"，加上 `readonly` 风险的 bonus，得分仍然 > 0
4. 更关键的是，当用户在主机详情页面（`/hosts/:id`）提问时，context refs 中有 `host` 对象，会触发 `context_object_match +0.10`，使 agent 工具被选中
5. Agent 工具被选中后，由于 `AutoCallable=true`，LLM 会自动调用它们

**核心问题**：agent 工具的选取缺少**意图安全分析门控**——只有当用户意图明确指向安全分析时，才应该注入 agent 工具。

### 修复方案

在 `ToolSelector.Select()` 中增加 agent 工具的意图门控过滤：

```go
// IntentRequiresAgentTools 判断意图是否需要 agent 工具
// agent 工具（进程采集、网络采集等）只在安全分析场景下使用
// 普通资源查询（主机列表、资产统计等）应直接查数据库
func (s *ToolSelector) IntentRequiresAgentTools(intent IntentResult) bool {
    // 1. 分析类动作
    if intent.Action == "analyze" || intent.Action == "investigate" {
        return true
    }
    // 2. 安全分析相关领域
    securityDomains := map[string]bool{
        "detection":     true,
        "investigation": true,
        "sigma_rule":    true,
        "block":         true,
    }
    for _, d := range intent.Domains {
        if securityDomains[d] {
            return true
        }
    }
    return false
}
```

在 `Select()` 方法的过滤阶段，对 domain=agent 的工具应用此门控：

```go
// Filter: agent tools only for security analysis intents
if tool.Domain == DomainAgent && !s.IntentRequiresAgentTools(input.Intent) {
    continue
}
```

### 影响组件

- `api-server/internal/assistant/tool_selector.go`（修改）

### 影响分析

- **正面**：减少不必要的 agent gRPC 调用，降低目标主机负载，提升响应速度
- **正面**：LLM 回复更准确，不会在简单查询时执行采集操作
- **风险**：如果 intent 分类不准确，可能导致安全分析场景下 agent 工具未被注入
- **缓解**：`IntentRouter` 已有 LLM fallback 机制，低置信度时会调用 LLM 分类

### 测试场景

| 用户输入 | 期望 domain | 期望 action | Agent 工具是否注入 |
|---------|------------|------------|-------------------|
| "查看主机列表" | host | query | ❌ |
| "有哪些资产" | host | query | ❌ |
| "分析这台主机的安全状态" | host | analyze | ✅ |
| "调查这台主机的攻击路径" | investigation | analyze | ✅ |
| "查看告警详情" | detection | query | ❌ |
| "分析最近的告警" | detection | analyze | ✅ |
| "帮我溯源这个攻击" | investigation | analyze | ✅ |
| "查看主机上的进程" | host | query | ❌（应使用 Host.Detail） |
| "检查主机是否有可疑进程" | host, detection | analyze | ✅ |

---

---

## Bug 3：缺少已安装软件查询工具

### Bug 描述

用户问"哪个主机上存在pgsql软件"时，模型调用 `Host.List` 成功获取主机列表，但随后无法查询已安装软件信息，最终降级为建议用户手动执行 bash 命令（`rpm -qa | grep postgres`）。

### 根因分析

数据库中已有 `installed_software` 表（含 `package_name`、`host_id`、`package_version` 等字段），Repository 层也有 `GetInstalledSoftwareByHost(hostID)` 方法，但：

1. **没有暴露为 Assistant 工具**：`installed_software` 数据对 AI 助手不可见
2. **缺少反向查询能力**：只能按 host_id 查软件，不能按软件名反查主机
3. **IntentRouter 缺少"软件"关键词**：用户说"软件"时不会被识别为相关意图

### 修复方案

1. **Repository 层**：在 `VulnerabilityRepo` 中新增 `SearchInstalledSoftware(packageName, page, pageSize)` 方法，JOIN hosts 表返回主机+软件信息
2. **工具注册**：注册新工具 `Software.Installed.Search`，domain=vulnerability, operation=search, risk=readonly
3. **IntentRouter**：在 host 域关键词中添加"软件"、"安装"

### 影响组件

- `api-server/internal/repository/vulnerability_repo.go`（修改 - 新增方法）
- `api-server/internal/assistant/tools/vulnerability_tools.go`（修改 - 新增工具）
- `api-server/internal/assistant/intent_router.go`（修改 - 新增关键词）

### 修复后的行为

用户问"哪个主机上存在pgsql软件"时：
1. IntentRouter 识别 domain=host, action=query（"主机"+"软件"命中关键词）
2. ToolSelector 选中 `Software.Installed.Search`（keyword_match: "软件"匹配 aliases）
3. 模型调用 `Software.Installed.Search(package_name: "postgres")`
4. 返回已安装 postgres 的主机列表（含主机名、IP、版本信息）

---

---

## Bug 4：LLM 工具名幻觉（Software.List、Host.GetDetail 等不存在的工具名）

### Bug 描述

LLM 生成 `Software.List`、`Host.GetDetail`、`Host.FindOffline` 等不存在的工具名，导致工具调用失败，系统降级为建议用户手动执行 bash 命令。

### 根因分析

**prompt 中多处硬编码了与实际注册表不一致的工具名**，LLM 会直接复制这些错误名称：

| 幻觉工具名 | 实际工具名 | 错误来源 |
|---|---|---|
| `Host.GetDetail` | `Host.Get` | `prompt_fragments.go` 第 54 行 |
| `Host.FindOffline` | `Host.AgentStatus.Get` | `adapter_prompt_provider.go` 第 125 行示例 |
| `Alert.List` | `Detection.Alert.List` | `prompt_fragments.go` 第 76 行 |
| `Vulnerability.GetDetail` | `Vulnerability.AffectedHosts` | `prompt_fragments.go` 第 64 行 |
| `Block.Record.List` | `Block.Policy.List` | `investigation_tools.go` 第 117 行 |
| `Software.List` | `Software.Installed.Search` | LLM 语义推测，未在 prompt 中出现 |

### 修复方案

统一修正所有 prompt 中的工具名引用，与实际注册表对齐：

1. `adapter_prompt_provider.go`：修正示例中 `Host.FindOffline` → `Host.AgentStatus.Get`
2. `prompt_fragments.go`：修正 `Host.GetDetail` → `Host.Get`、`Alert.List` → `Detection.Alert.List`、`Vulnerability.GetDetail` → `Vulnerability.AffectedHosts`
3. `investigation_tools.go`：修正 `Block.Record.List` → `Block.Policy.List`
4. `llm_client_adapter.go`：更新 `knownTools` 列表与实际注册表一致

### 影响组件

- `api-server/internal/assistant/adapter_prompt_provider.go`（修改）
- `api-server/internal/assistant/prompt_fragments.go`（修改）
- `api-server/internal/assistant/tools/investigation_tools.go`（修改）
- `api-server/internal/llm/adapters/llm_client_adapter.go`（修改）

---

## Bug 5：历史会话无分页

### Bug 描述

历史会话列表只能看到最近 20 条，无法浏览更早的会话。后端 API 支持分页，但前端未实现翻页交互。

### 根因分析

- 后端 `ListSessions` 完整支持 `page`/`page_size` 参数
- 前端 `fetchSessions()` 调用时不传分页参数，默认 page=1, pageSize=20
- `AssistantSessionSidebar.vue` 没有"加载更多"按钮

### 修复方案

1. Store 层：`fetchSessions` 增加 `append` 参数，支持追加模式
2. Store 层：新增 `sessionTotal`、`hasMoreSessions`、`loadingMore` 分页状态
3. 侧边栏组件：新增"加载更多"按钮和 `loadMore` 事件
4. 工作区组件：传递分页 props，处理 `loadMore` 事件

### 影响组件

- `frontend/src/store/assistant.ts`（修改 - 分页状态和 append 模式）
- `frontend/src/views/assistant/components/AssistantSessionSidebar.vue`（修改 - 加载更多按钮）
- `frontend/src/views/assistant/AssistantWorkspace.vue`（修改 - 分页 props 和事件处理）

---

## 回滚计划

- Bug 1：删除 `AssistantToolPolicySettings.vue` 和路由配置即可回滚
- Bug 2：移除 `tool_selector.go` 中的 agent 工具门控逻辑即可回滚
- Bug 3：移除 `vulnerability_repo.go` 中的 `SearchInstalledSoftware` 方法、`vulnerability_tools.go` 中的工具注册、`intent_router.go` 中的关键词即可回滚
- Bug 4：还原 `adapter_prompt_provider.go`、`prompt_fragments.go`、`investigation_tools.go`、`llm_client_adapter.go` 中的工具名即可回滚
- Bug 5：还原 `assistant.ts`、`AssistantSessionSidebar.vue`、`AssistantWorkspace.vue` 中的分页逻辑即可回滚

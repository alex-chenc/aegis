# Aegis V6.0 智能体服务端工具、文件上传与任务进度 UI 设计

## 1. 问题背景

当前智能体模式已经具备基础对话、工具审批、主机/任务/漏洞/Sigma 查询等能力，但与 6.0 文档定义的“可执行运维智能体”仍有差距：

- 服务端工具不完整，资产采集、漏洞扫描、基线模板上传/识别/脚本生成/检测/修复等运维能力没有完整接入智能体工具目录。
- 智能体缺少文件上传入口，无法在对话中上传文档、基线模板、Sigma 规则并形成可引用上下文。
- 智能体触发的执行任务没有在对话中展示进度，也没有清晰复用运维模式已有任务数据。
- 6.0 文档要求的三档工具权限模式已在后端存在，但对话框附近缺少可见入口，前端请求字段也与后端接口不完全一致。
- 执行结果 UI 偏原始，JSON 结果和任务状态缺少可读摘要。

## 2. 设计目标

1. 智能体只接入服务端业务工具，不把 Agent 侧能力做成独立工具孤岛。
2. 资产采集、漏洞扫描、基线检测/修复等智能体任务全部复用现有运维服务、数据库和任务接口。
3. 对话中支持文件上传，按用途分别处理为普通分析文件、基线模板、Sigma 规则。
4. 对话框区域显示 `request_approval`、`whitelist`、`full_access` 三档权限，并复用后端 `assistant.tool_approval_mode` 策略。
5. 智能体工具返回结构化任务引用，前端展示任务进度卡片，并轮询现有运维接口。
6. 执行结果 UI 从原始 JSON 提升为摘要、状态、关键字段和详情分层展示。

## 3. 当前实现复用点

| 能力 | 现有服务/接口 | 智能体接入方式 |
| --- | --- | --- |
| 资产采集 | `AssetCollectionService.TriggerAssetCollection`、`/host-assets/collections` | 新增 `Asset.Collection.*` 工具，返回采集任务引用 |
| 资产查询 | `AssetQueryService`、`AssetCollectionRepository` | 新增资产汇总/软件/应用/采集详情工具 |
| 漏洞扫描 | `VulnerabilityService.StartScan/GetScanStatus/StopScan` | 新增 `Vulnerability.Scan.*` 工具 |
| 漏洞脚本 | `HostVulnerabilityScriptService` | 新增脚本生成、状态、执行工具 |
| 基线模板 | `TemplateService.UploadTemplate`、`ScriptGenerationService` | 上传接口接入模板上传，工具接入模板/规则/脚本生成 |
| 基线检测修复 | `TaskService.CreateAndDispatchTasks` | 既有 `Task.RunCheck/RunFix` 补充任务引用与进度入口 |
| Sigma 上传 | `SigmaRuleUploadService.UploadRules` | 上传接口按 `sigma_rule` 用途解析入库 |
| 文件解析 | `internal/fileparser` | 普通分析文件解析为智能体上下文引用 |
| 三档权限 | `ToolPolicyService`、`RiskPolicy`、`ApprovalGate` | 前端对话框显示与设置，工具执行沿用后端策略 |

## 4. 后端设计

### 4.1 工具目录扩展

新增或补齐以下工具，并注册到统一 `ToolCatalog`：

- `Asset.Summary.Get`：查询资产概览。
- `Asset.Software.List`：查询软件资产。
- `Asset.Application.List`：查询应用资产。
- `Asset.Collection.Trigger`：触发资产采集，复用资产采集服务。
- `Asset.Collection.List`：查询采集任务列表。
- `Asset.Collection.Get`：查询单次采集任务和主机进度。
- `Vulnerability.Scan.Start`：触发漏洞扫描。
- `Vulnerability.Scan.Status`：查询扫描进度。
- `Vulnerability.Scan.Stop`：停止扫描。
- `Vulnerability.Script.Generate`：生成 POC 或修复脚本。
- `Vulnerability.Script.Status`：查询脚本生成状态。
- `Vulnerability.Script.Execute`：执行 POC 或修复脚本，返回任务组。
- `Baseline.Template.List`：查询模板列表。
- `Baseline.Template.Status.Get`：查询模板解析状态。
- `Baseline.Template.Rules.List`：查询模板规则。
- `Baseline.Script.Generate`：按模板或规则生成检测/修复脚本。

工具风险级别：

- 只读查询工具：`low`，默认可加入白名单。
- 触发扫描、采集、脚本生成：`medium`，默认需要权限策略判断。
- 执行修复脚本：`high`，即使在白名单模式下也不默认白名单。

### 4.2 任务引用返回结构

所有会产生后台任务的工具统一返回任务引用字段，便于前端识别：

```json
{
  "task_ref": {
    "kind": "asset_collection|baseline_task|vulnerability_scan|vulnerability_task",
    "id": "collection-id-or-scan-id",
    "task_group_id": "optional-task-group-id",
    "status_url": "/api/v1/...",
    "route_path": "/..."
  }
}
```

前端不额外维护智能体任务表，而是根据 `task_ref.kind` 轮询运维模式已有接口。

### 4.3 文件上传接口

新增接口：

```http
POST /api/v1/assistant/sessions/:session_id/files
Content-Type: multipart/form-data

file=<file>
purpose=analysis|baseline_template|sigma_rule
```

处理逻辑：

- `analysis`：调用 `fileparser` 解析文件内容，创建 `assistant_context_refs`，`object_type=file`。
- `baseline_template`：调用 `TemplateService.UploadTemplate`，创建 `object_type=baseline_template` 上下文引用，模板解析继续走现有异步流程。
- `sigma_rule`：调用 `SigmaRuleUploadService.UploadRules`，创建 `object_type=sigma_rule_upload` 上下文引用。

上传接口返回 `context_ref`，前端刷新会话上下文侧栏。普通分析文件内容只写入智能体上下文摘要，避免新增独立文件存储链路。

### 4.4 权限策略

后端继续以 `assistant.tool_approval_mode` 为单一策略源：

- `request_approval`：所有工具执行前需要审批。
- `whitelist`：白名单工具自动执行，其他工具需要审批。
- `full_access`：工具直接执行，但仍保留 RBAC、审计、风险边界和工具调用记录。

工具审批记录中的风险级别使用工具 `Risk` 字段，避免新工具审批风险为空。

## 5. 前端设计

### 5.1 对话框权限模式

在智能体输入框工具条中加入三档权限选择：

- 请求确认：每次工具调用都需要审批。
- 白名单：白名单工具自动执行。
- 全权限：所选工具直接执行。

切换时调用现有策略接口，当前模式从后端拉取。设置页继续保留工具白名单管理。

### 5.2 文件上传

在输入框工具条中加入文件上传按钮和用途选择：

- 分析文件
- 基线模板
- Sigma 规则

上传成功后将上下文引用显示到右侧上下文栏，并在对话附近给出轻量状态反馈。

### 5.3 执行结果 UI

工具调用卡片分为：

- 工具状态区：工具名、风险、耗时、审批状态。
- 结果摘要区：自动提取 `summary/message/status` 等关键字段。
- 任务进度区：识别 `task_ref` 后展示进度条、主机数量、成功失败数量、最新日志和跳转入口。
- 详情区：折叠展示结构化 JSON。

### 5.4 任务进度互通

智能体不复制任务数据。前端根据 `task_ref.kind` 复用：

- 资产采集：`/host-assets/collections/:id`
- 基线任务：`/tasks/:task_group_id/status` 和 `/tasks/:task_group_id/logs`
- 漏洞扫描：漏洞扫描状态接口
- 漏洞脚本任务：`/tasks/:task_group_id/status` 和 `/tasks/:task_group_id/logs`

这样智能体触发的任务会自然出现在运维模式已有页面，运维模式已有任务也能被智能体查询。

## 6. 测试用例

### 6.1 后端单元测试

1. 工具目录包含资产、漏洞扫描、基线模板/脚本工具。
2. `Asset.Collection.Trigger` 调用资产采集服务并返回 `task_ref`。
3. `Vulnerability.Scan.Start` 调用扫描服务并返回扫描任务引用。
4. `Task.RunCheck/RunFix` 返回任务组引用和运维跳转路径。
5. 文件上传 `analysis` 创建 `file` 上下文引用。
6. 文件上传 `baseline_template` 调用模板服务并创建上下文引用。
7. 文件上传 `sigma_rule` 调用 Sigma 上传服务并创建上下文引用。
8. 审批记录风险级别来自 `ToolSpec.Risk`。

### 6.2 前端单元测试

1. 工具权限 API 请求字段为 `whitelisted/items`，与后端一致。
2. 对话输入框显示三档权限并能触发切换事件。
3. 文件上传能按用途提交 `FormData`。
4. 工具结果卡片能渲染摘要、任务引用和折叠详情。
5. 任务进度卡片能根据不同 `task_ref.kind` 调用正确轮询接口。

### 6.3 Playwright 验收

1. 打开智能体页面，输入框处可见三档权限控件。
2. 切换权限模式后，页面展示新模式并向策略接口发送请求。
3. 上传分析文件后，右侧上下文栏出现文件引用。
4. 注入一条带 `task_ref` 的工具结果，页面展示任务进度卡片。
5. 工具执行结果不再只显示原始 JSON，关键状态和摘要可见。

## 7. 兼容性与回滚

- 新工具只扩展目录，不改变原有工具名和原有接口。
- 上传接口为新增接口，不影响已有会话、消息和上下文接口。
- 前端权限选择复用现有策略接口，设置页继续可用。
- 若某类任务状态接口暂不可用，任务进度卡片降级显示工具返回的初始状态和运维跳转入口。
- 回滚时可移除新增工具注册、上传路由和前端组件，不需要数据库结构回滚。

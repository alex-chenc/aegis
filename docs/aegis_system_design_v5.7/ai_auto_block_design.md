# AI 自动阻断设计文档

**版本**: 5.7
**日期**: 2026-05-20
**状态**: 设计完成，待实现

---

## 1. 背景

当前阻断策略页已有 `自动阻断` 开关，对应 `block_policies.auto_block`。该能力在规则命中告警后立即触发，属于“规则命中即阻断”。

本次新增能力是“AI 分析后自动阻断”：当用户显式开启某条 MITRE 策略的 `AI自动阻断` 后，AI 分析会话完成并确认某条告警为恶意时，后端自动复用该 MITRE 策略配置的阻断动作进行阻断，并把成功、失败或跳过原因展示在当前会话和历史会话中。

核心边界：LLM 只输出结构化分析结论，不能直接调用阻断工具；阻断动作必须由后端在 AI 分析完成后确定性执行。

---

## 2. 已确认需求

| 编号 | 需求 | 结论 |
| --- | --- | --- |
| R1 | 新增开关 | 在阻断策略中新增独立 `AI自动阻断` 开关 |
| R2 | 互斥关系 | `自动阻断` 与 `AI自动阻断` 不能同时开启 |
| R3 | 触发条件 | 只在 AI 明确判定恶意时触发 |
| R4 | 可疑/未知/误报 | 不触发自动阻断 |
| R5 | 策略未启用 | `enabled=false` 时不生效 |
| R6 | 历史会话 | 历史会话加载只恢复展示，不补触发阻断 |
| R7 | 记录存储 | 复用 `block_records`，来源标记为 `issued_by="ai_auto"` |
| R8 | 幂等规则 | 任一来源已有阻断记录时，不重复执行 |
| R9 | 执行主体 | 后端执行，LLM 不直接调用阻断工具 |
| R10 | 多告警 | 逐条告警独立执行、独立记录、汇总展示 |
| R11 | 默认值 | 默认关闭 |

---

## 3. 非目标

- 不新增 LLM 阻断工具，不允许模型直接 kill 进程、隔离文件或写防火墙规则。
- 不改变现有 `自动阻断` 的规则命中即阻断语义。
- 不在历史会话加载时执行任何主机侧动作。
- 不新增独立 AI 阻断记录表；阻断事实继续落在 `block_records`。
- 不对 `suspicious`、`generate_rule`、`unknown`、`benign` 自动阻断。

---

## 4. 产品交互设计

### 4.1 阻断策略页

在 `frontend/src/views/detection/Policies.vue` 表格中新增一列：

| 列 | 控件 | 行为 |
| --- | --- | --- |
| `AI自动阻断` | `el-switch` | 控制 `block_policies.ai_auto_block` |

互斥交互：

1. 用户开启 `自动阻断` 时，前端提示“开启自动阻断后将关闭 AI自动阻断”，确认后发送 `{ "auto_block": true }`，后端自动写入 `ai_auto_block=false`。
2. 用户开启 `AI自动阻断` 时，前端提示“开启 AI自动阻断后将关闭自动阻断”，确认后发送 `{ "ai_auto_block": true }`，后端自动写入 `auto_block=false`。
3. 如果某行 `auto_block=true`，`AI自动阻断` 开关显示为未开启并带 tooltip 说明互斥关系。
4. 如果某行 `ai_auto_block=true`，`自动阻断` 开关显示为未开启并带 tooltip 说明互斥关系。
5. 如果策略 `enabled=false`，两个自动能力均不生效；前端保留配置值，但通过 tooltip 或弱提示说明“策略未启用，自动能力不会触发”。

### 4.2 AI 分析当前会话

AI 分析完成后，当前会话结论区新增“AI 自动阻断结果”摘要：

| 状态 | 展示文案 |
| --- | --- |
| 未触发 | `AI自动阻断未触发` |
| 全部成功 | `AI自动阻断成功` |
| 部分失败 | `AI自动阻断部分失败` |
| 全部失败 | `AI自动阻断失败` |
| 全部跳过 | `AI自动阻断已跳过` |

逐条结果展示字段：

- 告警 ID
- MITRE ID
- 策略动作：`kill_process` / `quarantine_file` / `block_connection` / `disable_user`
- 目标：PID、文件路径、IP/CIDR 或用户名
- 状态：`success` / `failed` / `skipped`
- 失败或跳过原因
- `block_id`（存在阻断记录时展示）

### 4.3 告警列表与详情

告警列表和详情继续沿用现有字段：

- `alerts.block_status`
- `alerts.block_message`
- `alerts.auto_blocked`
- `alerts.manual_blocked`

阻断来源通过 `block_records.issued_by` 区分：

| issued_by | 含义 |
| --- | --- |
| `manual` | 用户手动阻断 |
| `auto` | 规则命中后自动阻断 |
| `ai_auto` | AI 分析确认威胁后自动阻断 |

### 4.4 历史会话加载

历史会话加载必须恢复：

- 原始 AI 分析结论
- 关联告警快照
- 告警当前阻断状态
- 当次 AI 自动阻断结果摘要

历史会话加载禁止触发阻断。即使历史会话包含 `verdict=malicious` 或 `confirm_threat`，也只能读取已持久化的 `ai_auto_block` 结果和当前告警状态。

---

## 5. 数据模型设计

### 5.1 block_policies 新增字段

```sql
ALTER TABLE block_policies
  ADD COLUMN IF NOT EXISTS ai_auto_block BOOLEAN NOT NULL DEFAULT FALSE;
```

字段说明：

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `ai_auto_block` | BOOLEAN | `false` | AI 分析确认恶意后自动执行本策略阻断动作 |

### 5.2 互斥约束

数据库增加互斥约束，防止绕过 API 写出非法状态：

```sql
ALTER TABLE block_policies
  ADD CONSTRAINT chk_block_policies_auto_block_exclusive
  CHECK (NOT (auto_block = TRUE AND ai_auto_block = TRUE));
```

上线前兼容处理：

```sql
UPDATE block_policies
SET ai_auto_block = FALSE
WHERE auto_block = TRUE;
```

### 5.3 Go 模型

以下服务的 `BlockPolicy` 模型都需要补充字段，保持共享表结构一致：

- `api-server/internal/model/block_policy.go`
- `server/internal/model/block_policy.go`
- `dc/internal/model/block_policy.go`

```go
AIAutoBlock bool `gorm:"not null;default:false" json:"ai_auto_block"`
```

### 5.4 block_records 复用

不新增 AI 阻断记录表。AI 自动阻断写入：

```go
IssuedBy: "ai_auto"
```

其他字段继续使用现有结构：

- `block_id`
- `alert_id`
- `host_id`
- `action`
- `target`
- `success`
- `message`
- `created_at`

### 5.5 会话级结果持久化

为了历史会话能恢复“当次 AI 自动阻断结果”，在 `ai_analysis_session.conclusion` 中增加字段：

```json
{
  "verdict": "malicious",
  "summary": "确认反弹 shell 行为",
  "reasoning": "...",
  "attack_graph": {},
  "conclusions": [],
  "ai_auto_block": {
    "triggered": true,
    "summary": {
      "total": 3,
      "success": 2,
      "failed": 1,
      "skipped": 0
    },
    "results": [
      {
        "alert_id": "ALT-xxx",
        "mitre_id": "T1059.004",
        "action": "kill_process",
        "target": "12345",
        "status": "success",
        "message": "阻断执行成功",
        "block_id": "BLK-xxxx",
        "issued_by": "ai_auto"
      }
    ]
  }
}
```

---

## 6. API 设计

### 6.1 阻断策略列表

`GET /api/v1/detection/block-policies`

返回项新增：

```json
{
  "mitre_id": "T1059.004",
  "enabled": true,
  "auto_block": false,
  "ai_auto_block": true,
  "auto_dispose": false,
  "action": "kill_process"
}
```

### 6.2 更新阻断策略

`PUT /api/v1/detection/block-policies/:mitre_id`

请求体新增：

```json
{
  "ai_auto_block": true
}
```

后端规则：

1. 请求同时包含 `auto_block=true` 与 `ai_auto_block=true`，返回 `400`。
2. 请求 `auto_block=true` 时，后端写入 `ai_auto_block=false`。
3. 请求 `ai_auto_block=true` 时，后端写入 `auto_block=false`。
4. 请求关闭其中一个开关时，不自动开启另一个。
5. `enabled=false` 不清空开关配置，但触发时必须判定不生效。

### 6.3 AI 分析 SSE 事件

AI 自动阻断完成后，`done` 之前发送事件：

```json
{
  "type": "ai_auto_block",
  "result": {
    "triggered": true,
    "summary": {
      "total": 1,
      "success": 1,
      "failed": 0,
      "skipped": 0
    },
    "results": [
      {
        "alert_id": "ALT-xxx",
        "mitre_id": "T1059.004",
        "action": "kill_process",
        "target": "12345",
        "status": "success",
        "message": "阻断执行成功",
        "block_id": "BLK-xxxx",
        "issued_by": "ai_auto"
      }
    ]
  }
}
```

### 6.4 历史会话 API

`GET /api/v1/detection/alerts/ai-analysis/:session_id/history`

返回 `ai_auto_block`，优先来自 `ai_analysis_session.conclusion.ai_auto_block`：

```json
{
  "success": true,
  "data": {
    "session_id": "session-uuid",
    "messages": [],
    "execution_plan": {},
    "conclusion": {},
    "alerts": [],
    "ai_auto_block": {
      "triggered": true,
      "summary": {},
      "results": []
    }
  }
}
```

历史接口只读取，不执行阻断。

### 6.5 执行结果 API

`GET /api/v1/detection/alerts/ai-analysis/:session_id/execution-result`

`conclusion` 中保留 `ai_auto_block` 字段，供 `TaskExecutionResult` 展示。

---

## 7. 后端流程设计

### 7.1 新增服务

建议新增：

```text
api-server/internal/service/ai_auto_block_service.go
```

职责：

- 根据 AI 结构化结论筛选 `confirm_threat` 告警
- 按告警 MITRE ID 查询阻断策略
- 校验 `enabled && ai_auto_block && !auto_block`
- 检查该告警是否已有任一阻断记录
- 解析阻断目标
- 通过现有 Server gRPC 下发 `ExecuteBlockCommand`
- 写入 `block_records`
- 更新 `alerts.block_status` 与 `alerts.block_message`
- 返回会话级结果摘要

### 7.2 触发点

在 `api-server/internal/api/handler/ai_analysis_handler.go` 中，AI 分析达到结论后触发。

推荐将现有流程拆成：

```go
outcome := h.persistAnalysisOutcome(session, aiResponseContent)

if aiResponseContent != "" {
    sseWriter.WriteContent(aiResponseContent)
}

if h.aiAutoBlockService != nil && outcome.StructuredResult != nil {
    aiBlockResult := h.aiAutoBlockService.ExecuteForSession(ctx, session, outcome.StructuredResult)
    h.persistAIAutoBlockOutcome(session.SessionID, outcome.ConclusionMap, aiBlockResult)
    sseWriter.Write(llm.SSEEvent{Type: "ai_auto_block", Result: aiBlockResult})
}

sseWriter.WriteDone()
```

说明：

- `done` 必须在 AI 自动阻断结果之后发送。
- 执行过程中继续保留 SSE keepalive，避免阻断等待期间连接超时。
- 如果 AI 自动阻断服务不可用，写入 `triggered=false` 或 `skipped`，不能让 AI 分析整体失败。

### 7.3 恶意结论筛选

只处理结构化结论中的：

```json
{
  "action": "confirm_threat"
}
```

处理规则：

1. 一个会话中多个 `confirm_threat` 逐条执行。
2. `mark_false_positive` 不触发。
3. `generate_rule` 不触发。
4. 如果最终只有文本关键词 `verdict=malicious`，但没有结构化 `conclusions[].alert_id/action`，不自动阻断，避免误阻断整个会话范围。

### 7.4 策略校验

每条告警独立校验：

```text
policy.enabled == true
policy.ai_auto_block == true
policy.auto_block == false
```

不满足时记录为 `skipped`：

| 场景 | 跳过原因 |
| --- | --- |
| 无策略 | `未找到阻断策略` |
| 策略禁用 | `阻断策略未启用` |
| AI 自动阻断关闭 | `AI自动阻断未开启` |
| 自动阻断开启 | `自动阻断与AI自动阻断互斥，当前策略未触发AI自动阻断` |

### 7.5 幂等校验

AI 自动阻断执行前，查询 `block_records`：

```text
WHERE alert_id = alerts.id
```

只要存在任一来源的阻断记录，则不重复执行。

跳过结果示例：

```json
{
  "alert_id": "ALT-xxx",
  "status": "skipped",
  "message": "该告警已有阻断记录，不重复执行",
  "existing_block_id": "BLK-yyyy",
  "existing_issued_by": "manual"
}
```

### 7.6 阻断执行

目标解析复用现有规则：

| action | target |
| --- | --- |
| `kill_process` | 告警 PID |
| `quarantine_file` | 告警 `command_line` 中的文件路径 |
| `block_connection` | 告警 `command_line` 中的远端 IP |
| `disable_user` | 告警关联用户，若无法解析则失败 |

阻断记录：

```go
record := &model.BlockRecord{
    BlockID:  "BLK-" + uuid.New().String()[:8],
    AlertID:  &alert.ID,
    HostID:   alert.HostID,
    Action:   policy.Action,
    Target:   target,
    IssuedBy: "ai_auto",
}
```

阻断成功：

- `block_records.success=true`
- `block_records.message="阻断成功"`
- `alerts.auto_blocked=true`
- `alerts.block_status="success"`
- `alerts.block_message="AI自动阻断执行成功"`
- `alerts.status="resolved"`

阻断失败：

- `block_records.success=false`
- `block_records.message=<真实失败原因>`
- `alerts.auto_blocked=true`
- `alerts.block_status="failed"`
- `alerts.block_message=<真实失败原因>`
- `alerts.status` 保持原状态

### 7.7 失败原因

失败原因沿用 V5.6 阻断失败原因透传设计：

| 层级 | 原因 |
| --- | --- |
| API 前置校验 | 缺失 PID、文件路径、IP、用户名 |
| Server 连接层 | Agent 未连接、通道不可用 |
| Agent 执行层 | 进程不存在、权限不足、iptables 失败、文件隔离失败 |

前端展示 `block_records.message` 与 `alerts.block_message`。

---

## 8. 前端设计

### 8.1 类型

`frontend/src/types/index.ts`:

```ts
export interface BlockPolicy {
  ai_auto_block: boolean
}
```

`frontend/src/api/aiAnalysis.ts`:

```ts
export interface AIAutoBlockResultItem {
  alert_id: string
  mitre_id?: string
  action?: string
  target?: string
  status: 'success' | 'failed' | 'skipped'
  message: string
  block_id?: string
  issued_by?: 'ai_auto'
  existing_block_id?: string
  existing_issued_by?: string
}

export interface AIAutoBlockPayload {
  triggered: boolean
  summary: {
    total: number
    success: number
    failed: number
    skipped: number
  }
  results: AIAutoBlockResultItem[]
}
```

`SSEEventType` 增加：

```ts
| 'ai_auto_block'
```

### 8.2 Policies.vue

新增 `AI自动阻断` 列。

前端更新规则：

- `handleToggleAutoBlock(mitreId, true)` 成功后本地设置 `row.auto_block=true`、`row.ai_auto_block=false`
- `handleToggleAIAutoBlock(mitreId, true)` 成功后本地设置 `row.ai_auto_block=true`、`row.auto_block=false`
- 失败时 reload 当前页，避免本地状态和后端不一致

### 8.3 AIAnalysis.vue

新增状态：

```ts
const aiAutoBlockResult = ref<AIAutoBlockPayload | null>(null)
```

处理 SSE：

```ts
case 'ai_auto_block':
  aiAutoBlockResult.value = event.result as AIAutoBlockPayload
  attachAIAutoBlockResultToExecutionResult(aiAutoBlockResult.value)
  break
```

历史加载：

- 从 `payload.ai_auto_block` 或 `payload.conclusion?.ai_auto_block` 恢复
- 不调用任何阻断接口
- 同步展示告警当前 `block_status` / `block_message`

### 8.4 TaskExecutionResult.vue

在“分析结论”后、“处置建议”前新增“AI 自动阻断结果”区域。

UI 要求：

- 使用 `el-tag` 展示成功、失败、跳过状态
- 失败原因必须直接可见；长文本可用 tooltip 展示全文
- 多条结果用紧凑表格或列表展示，避免卡片套卡片
- 不用颜色作为唯一信息来源，标签文字必须明确

---

## 9. 测试计划

### 9.1 后端单元测试

| 测试 | 预期 |
| --- | --- |
| `UpdateBlockPolicy_AutoBlockDisablesAIAutoBlock` | 开启自动阻断后 AI自动阻断为 false |
| `UpdateBlockPolicy_AIAutoBlockDisablesAutoBlock` | 开启 AI自动阻断后自动阻断为 false |
| `UpdateBlockPolicy_RejectBothTrue` | 同时 true 返回 400 |
| `AIAutoBlock_ConfirmThreatTriggers` | `confirm_threat` 且策略开启时执行阻断 |
| `AIAutoBlock_SuspiciousDoesNotTrigger` | `generate_rule` 不阻断 |
| `AIAutoBlock_DisabledPolicySkips` | 策略禁用时 skipped |
| `AIAutoBlock_AutoBlockPolicySkips` | 自动阻断开启时 AI 阻断 skipped |
| `AIAutoBlock_ExistingBlockRecordSkips` | 任一阻断记录存在时不重复执行 |
| `AIAutoBlock_TargetMissingFails` | 目标缺失时写失败原因 |
| `AIAutoBlock_AgentFailurePersistsReason` | Agent 失败原因写入 record 和 alert |
| `GetSessionHistoryReturnsAIAutoBlock` | 历史接口返回 AI 自动阻断结果 |

### 9.2 前端单元测试

| 测试 | 预期 |
| --- | --- |
| `Policies` 开启自动阻断 | 本地关闭 AI自动阻断 |
| `Policies` 开启 AI自动阻断 | 本地关闭自动阻断 |
| `Policies` 同步失败 | 重新加载策略列表 |
| `AIAnalysis` 接收 `ai_auto_block` SSE | 展示结果摘要 |
| `loadSession` 历史加载 | 从 history payload 恢复结果，不执行阻断 |
| `TaskExecutionResult` 失败原因 | 失败原因直接可见 |

### 9.3 集成验证

1. 创建一条 `ai_auto_block=true`、`auto_block=false`、`enabled=true` 的策略。
2. 产生一条对应 MITRE 告警。
3. 发起 AI 分析，模型输出 `confirm_threat`。
4. 验证：
   - `block_records.issued_by = "ai_auto"`
   - `alerts.block_status` 为 `success` 或 `failed`
   - 失败时 `alerts.block_message` 为真实原因
   - 当前会话显示 AI 自动阻断摘要
   - 历史会话加载后仍显示同一结果
5. 对同一告警再次运行 AI 分析，验证不重复下发阻断。

---

## 10. 风险与缓解

| 风险 | 等级 | 缓解 |
| --- | --- | --- |
| AI 误判导致阻断 | 高 | 默认关闭，只对 `confirm_threat` 触发，不处理可疑/未知 |
| 双自动链路重复阻断 | 高 | 前端互斥、API 互斥、DB CHECK 约束 |
| 历史会话误触发 | 高 | history API 只读，前端只恢复结果 |
| 阻断重复执行 | 中 | 以 `block_records.alert_id` 做幂等检查 |
| 阻断耗时导致 SSE 超时 | 中 | 保留 keepalive，`done` 在结果后发送 |
| 失败原因不透明 | 中 | 复用失败原因透传链路，写入 record 与 alert |
| 旧数据缺 `ai_auto_block` | 低 | 默认 false，旧策略不触发 |

---

## 11. 验收标准

- 阻断策略页存在 `AI自动阻断` 独立开关。
- `自动阻断` 与 `AI自动阻断` 任何路径都不能同时为 true。
- AI 分析只有结构化 `confirm_threat` 告警会进入 AI 自动阻断流程。
- 策略未启用、AI自动阻断关闭、已有阻断记录时不会执行阻断。
- 多条恶意告警逐条执行并分别记录结果。
- 成功时显示阻断成功；失败时显示阻断失败原因。
- 历史会话加载能恢复 AI 自动阻断结果，且不会补执行。
- 后端测试、前端测试和接口验证覆盖成功、失败、跳过、互斥、历史恢复场景。

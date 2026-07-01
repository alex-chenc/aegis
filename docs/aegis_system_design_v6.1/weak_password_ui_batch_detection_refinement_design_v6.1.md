# Aegis V6.1 弱密码检测交互与批量检测调整设计

## 1. 问题与需求

本次调整聚焦弱密码页面的可用性、检测策略收敛和真实检测闭环：

- 应用资产分析页删除凭据类型展示，操作中删除“查看分析依据”。
- 弱密码检查抽屉中，字典策略改为显式勾选字典；删除混合规则、模糊规则和“加密/hash LLM 匹配”勾选项。
- AI 一键生成字典改为自然语言输入，由大模型/后端根据自然语言生成密码字典；删除生成目标、应用类型、组织关键词、账号关键词。
- 弱密码字典列表删除分类、来源、状态列；只保留类型，类型仅区分“内置”和“自定义”。
- 删除独立“检测结果”Tab。
- 应用资产分析列表新增状态：红色告警、绿色安全、灰色未扫描。红色状态点击后显示弱密码详情：账号、脱敏密码、进程 PID（如果有）。密码默认星号，用户输入系统密码后展示明文。
- 新增“一键检测”按钮，检测当前候选应用；检测前逐个判断主机是否在线，离线主机不创建任务。
- Agent 采集需考虑容器内应用的配置路径，允许通过进程 PID 的 `/proc/<pid>/root` 只读映射读取容器内路径。
- 后端不要求用户判断密码是明文还是 hash。Agent 返回明文时服务端字典匹配；判断为 hash 时，服务端 verifier 优先处理，不能本地验证的 hash 预留 LLM 匹配链路。
- 使用真实服务、真实 Agent 和 Playwright 进行弱密码功能测试，确保可检测出弱密码。

## 2. 当前行为

- `Index.vue` 的“应用资产分析”表格展示 `credential_types`，操作包含“检查弱密码”和“查看分析依据”。
- `Index.vue` 有独立“检测结果”Tab，但主页面并没有按应用维度呈现扫描状态。
- 创建任务只支持单个 `candidate_application_id`。
- `CreateTaskByApplicationRequest` 仍包含 `hybrid`、`fuzzy`、`encrypted_password_llm_match`，前端也暴露这些选项。
- `MatchCredentialRecords` 固定使用默认 1000 字典，未按用户勾选的字典集合匹配。
- Agent `CollectCredentials` 只读取计划中的原始路径，不会尝试容器进程根目录映射。
- AI 生成字典请求结构偏表单化，缺少自然语言输入字段。

## 3. 目标行为

### 3.1 前端

- 弱密码主页面只保留“应用资产分析”和“弱密码检查”两个 Tab。
- 应用资产分析表格新增状态列：
  - `未扫描`：灰色，暂无完成扫描或命中信息。
  - `安全`：绿色，最近一次完成扫描且无命中。
  - `告警`：红色，存在弱密码 finding。
- 点击红色状态打开弱密码详情抽屉，展示账号、脱敏密码、进程 PID（如存在）和来源路径；用户输入当前系统密码后可展示单条明文。
- 检查弱密码抽屉只展示字典多选列表。字典列表显示当前可用字典，默认勾选内置字典。
- 新增“一键检测”按钮：对当前候选列表按同一字典选择发起批量检测。
- 字典管理页的 AI 生成抽屉只保留自然语言描述、生成数量、与内置字典去重。
- 字典列表只展示名称、类型、条数、操作；类型映射为“内置/自定义”。

### 3.2 后端

- 候选应用 DTO 增加弱密码状态摘要和 finding 摘要。
- 新增批量创建任务接口，逐个候选执行在线校验，离线主机跳过并返回 skipped 列表。
- 单应用任务创建时保存用户选择的字典 ID；未选择时默认使用内置字典。
- 匹配阶段按任务字典策略加载字典条目。
- AI 生成字典请求增加 `natural_language`；生成逻辑从自然语言提取种子词并生成候选，当前无 LLM 配置时继续使用 deterministic fallback，保留 prompt/policy 记录。
- finding evidence 中记录 Agent 返回的 PID（如果有）。

### 3.3 Agent

- `ApplicationCollectPlan` 增加 `related_pids`。
- 对每个配置路径先尝试原始路径；失败时按 related PID 尝试 `/proc/<pid>/root/<path>`。
- 返回 `CredentialRecord` 时保留原始 source_path，新增 `process_pid` 便于页面展示。

## 4. 组件设计

| 组件 | 调整 |
|:---|:---|
| `WeakPasswordHandler` | 增加批量检测接口，保留单应用接口 |
| `WeakPasswordService` | 增加候选状态聚合、批量任务创建、按字典策略匹配、自然语言字典生成 |
| `WeakPasswordRepository` | 增加候选 finding 聚合查询 |
| `agent/internal/weakpass` | 支持 related PID 和容器路径映射读取 |
| `frontend/src/views/detection/WeakPassword` | 简化 UI，新增状态、详情、批量检测和字典勾选 |
| `frontend/e2e` | 更新模拟 E2E 与真实 E2E 覆盖新交互 |

## 5. 数据流

```text
用户点击一键分析
  -> api-server 查询在线应用资产
  -> 生成候选应用并聚合最近弱密码状态
  -> 前端展示状态

用户勾选字典并检测
  -> api-server 校验目标主机在线
  -> 创建任务并保存字典策略
  -> Agent 读取配置，必要时尝试 /proc/<pid>/root 容器映射
  -> api-server 根据 Agent 返回类型执行明文匹配或 hash verifier
  -> 写入 finding，候选状态变告警/安全
  -> 前端点击状态查看详情，输入系统密码后 reveal 明文
```

## 6. 接口变化

### 6.1 候选应用响应新增字段

```json
{
  "scan_status": "unscanned | safe | alert",
  "last_task_id": "task-uuid",
  "matched_findings": 1,
  "findings": [
    {
      "id": "finding-uuid",
      "account": "default",
      "matched_password_mask": "*********",
      "process_pid": 1234,
      "source_path": "/etc/redis/redis.conf"
    }
  ]
}
```

### 6.2 批量检测

`POST /api/v1/weak-password/tasks/by-applications`

请求：

```json
{
  "candidate_application_ids": ["candidate-uuid"],
  "dictionary_policy": {
    "use_default_1000": true,
    "dictionary_ids": ["dict-uuid"],
    "use_ai_generated": false
  },
  "ai_policy": {
    "repair_collection_errors": true,
    "max_agent_tool_calls_per_app": 10
  }
}
```

响应：

```json
{
  "created": [{"candidate_application_id": "candidate-uuid", "task_id": "task-uuid"}],
  "skipped": [{"candidate_application_id": "candidate-uuid", "reason": "host_offline"}]
}
```

### 6.3 AI 生成字典

请求新增：

```json
{
  "natural_language": "为 Redis 管理员和生产环境生成弱密码字典，包含公司名 aegis、年份和常见符号",
  "count": 200,
  "deduplicate_with_default": true
}
```

旧字段继续容忍，前端不再展示。

## 7. 数据库变化

本次不新增表。`weak_password_candidate_applications` 与 `weak_password_findings` 已有关联字段，可通过 `candidate_application_id -> scan_application -> finding` 聚合状态。

`WeakPasswordFinding.EvidenceJSON` 增加 `process_pid`，属于兼容 JSON 扩展，不需要 migration。

## 8. 配置变化

无新增配置。AI 字典生成当前优先使用已有服务能力；没有 LLM 配置时使用 deterministic fallback，确保离线测试可重复。

## 9. 安全影响

- 密码仍只以脱敏形式展示；明文 reveal 必须输入当前系统密码。
- reveal 操作保留服务端日志和审计表写入能力。
- Agent 容器路径读取仅使用 `/proc/<pid>/root` 加计划内路径，不执行 shell、不递归搜索。
- 批量检测不会为离线主机创建任务，避免无效任务堆积。

## 10. 兼容性影响

- 后端继续接受旧字段 `hybrid`、`fuzzy`、`encrypted_password_llm_match`，但前端不展示，匹配逻辑不依赖这些字段。
- 字典类型内部值仍保留 `default_1000`、`uploaded`、`ai_generated` 等；前端统一映射为“内置/自定义”。

## 11. 测试设计

- Go 单元测试：
  - 按自定义字典策略匹配明文弱密码。
  - 批量创建任务跳过离线主机。
  - 自然语言字典生成产生包含用户关键词的候选。
  - 容器路径映射读取成功。
- 前端 store/Vitest：
  - 加载候选状态和 finding 摘要。
  - 批量检测接口调用与结果写入。
  - 自然语言字典生成请求结构。
- Playwright 模拟 E2E：
  - 分析资产、勾选字典、单应用检测、一键检测、状态详情、明文 reveal、字典管理。
- 真实 E2E：
  - 启动全栈和本地 Agent。
  - 准备 Redis 弱密码配置或等价测试应用配置。
  - 使用 `admin/Admin@123` 登录。
  - 通过 Playwright 完整走通并确认检测出 `Admin@123`。

## 12. 回滚方案

- 前端回滚到旧 Tab 和表格展示不会影响后端数据。
- 后端新增批量接口与 DTO 字段为兼容扩展，可保留不用。
- Agent 容器路径映射只在原始路径读取失败后触发，若出现问题可回滚 `related_pids` 读取分支。

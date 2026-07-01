# 弱密码 AI 字典生成与智能体集成修复

## Bug Description

用户在弱密码字典页面点击“AI 一键生成字典”时可能看到“网络错误，请检查网络连接”。同时智能体模式虽然已有弱密码扫描工具，但未覆盖弱密码模块的完整操作链路：AI 密码字典生成、应用资产分析、任务创建、进度查询和结果查询。

## Reproduction

1. 登录系统：`admin/Admin@123`。
2. 调用 `POST /api/v1/weak-password/dictionaries/ai-generate`，使用页面默认参数 `count=200`、`deduplicate_with_default=true`。
3. 真实接口在 120 秒内无 HTTP 响应，客户端超时后表现为网络错误；后端仍在继续等待 LLM。

## Root Cause

调用链：

1. `Dictionaries.vue` 调用 `store.generateDictionary`。
2. `frontend/src/api/weakPassword.ts` 调用 `/weak-password/dictionaries/ai-generate`。
3. `WeakPasswordHandler.GenerateDictionary` 调用 `WeakPasswordService.GenerateAIDictionary`。
4. `GenerateAIDictionary` 使用 `context.Background()` 调用 `generateDictionaryWithAI`。
5. LLM 生成大批量字典时可能超过客户端超时；因为服务端没有使用请求上下文和明确生成超时，客户端断开后后端仍继续执行。

智能体模式问题：

1. `RegisterWeakPasswordTools` 已注册扫描、结果查询、解释工具。
2. 缺少生成字典、分析应用、查询进度工具，智能体无法完整编排弱密码模块任务。
3. 弱密码任务进度只在弱密码页面展示，智能体工具调用结果中没有结构化进度摘要。

真实 Playwright 回归时额外暴露一个任务创建问题：

1. `AnalyzeAssetApplications` 对同一主机同一资产同一应用类型执行 upsert。
2. repository 未把 upsert 后数据库中真实存在的 candidate ID 回写到返回 DTO。
3. 前端/测试拿到新生成但实际未持久化的 candidate ID 后调用 `CreateTaskByApplication`，服务端查询候选应用返回 `record not found`，任务创建失败。

## Fix Design

### Backend

- `GenerateAIDictionary` 增加 `context.Context` 入参，继承 HTTP 请求取消信号。
- AI 字典生成增加专用超时，超时后返回明确错误，不再让前端等待到网络超时。
- 将同步 AI 字典生成的默认实时数量降低到更适合在线交互的范围，避免一键操作天然拖到超时；仍然必须调用 LLM，不恢复随机或硬编码 fallback。
- Handler 将 LLM 超时/取消映射为可读中文错误。
- 智能体弱密码工具补齐：
  - `Credential.WeakPassword.GenerateDictionary`
  - `Credential.WeakPassword.AnalyzeApplications`
  - `Credential.WeakPassword.QueryProgress`
  - 保留 `Credential.WeakPassword.Scan`
  - 保留 `Credential.WeakPassword.QueryFindings`
- 进度查询工具返回 `task_progress` 与 `collection_progress`，供智能体模式展示弱密码检测进度。
- 应用分析候选 upsert 后按唯一键读回持久化行，并在入库后再组装 DTO，保证返回的 `candidate_application_id` 可直接创建检测任务。

### Frontend

- 字典页默认生成数量调整为实时可完成的范围。
- 生成失败时展示后端具体错误，不再只展示泛化网络错误。
- 对 axios timeout 单独提示“请求超时”，区别于真实网络断开。
- 弱密码任务列表在任务运行中自动刷新，使进度在当前模块页面持续显示。

## Regression Test Cases

1. AI 字典生成必须调用 LLM；LLM 不可用或超时时返回错误且不保存字典。
2. AI 字典生成使用请求上下文，取消后应停止并返回 context 错误。
3. 智能体工具注册包含生成字典、应用分析、扫描、进度查询和结果查询。
4. 智能体进度工具返回任务进度和采集进度。
5. 前端字典生成调用传递实时数量，并在成功后刷新字典列表。
6. 前端请求超时时展示“请求超时”而非“网络错误”。
7. 真实服务构建后，通过 Playwright 登录并测试弱密码字典生成、应用分析、检测任务和任务进度页面。
8. 重复分析同一 Redis 应用资产时，返回的 candidate ID 必须是数据库中真实持久化的 ID，并能继续进入任务创建流程。

## Risk And Rollback

- 风险：降低默认实时生成数量可能改变页面默认行为，但可避免用户长时间无响应。
- 风险：智能体工具增加写操作，需要保持审批策略；扫描工具仍保留审批。
- 回滚：恢复原 `GenerateAIDictionary` 签名、移除新增智能体工具和前端默认数量/超时提示修改。

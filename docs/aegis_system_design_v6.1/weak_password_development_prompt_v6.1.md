# Aegis V6.1 弱密码检测开发提示词

## 使用说明

将下面的提示词复制给负责开发的 AI 编程智能体或工程师使用。该提示词要求开发方先阅读 V6.1 设计文档，再按照数据库、Agent、api-server、前端和测试的顺序实现弱密码检测能力。

## 开发提示词

```text
你是 Aegis 项目的资深全栈开发智能体，需要在 /code/aegis 仓库中实现 V6.1 弱密码检测能力。请严格遵循现有项目架构、代码风格、目录约定和安全要求，不要引入与现有系统不一致的大型重构。

一、必须先阅读的设计文档

在开始写代码前，完整阅读以下文档，并按文档实现：

1. docs/aegis_system_design_v6.1/weak_password_detection_design_v6.1.md
2. docs/aegis_system_design_v6.1/weak_password_frontend_prd_v6.1.md
3. docs/aegis_system_design_v6.1/weak_password_database_design_v6.1.md
4. docs/aegis_system_design_v6.1/weak_password_api_server_design_v6.1.md
5. docs/aegis_system_design_v6.1/weak_password_agent_program_design_v6.1.md

如果发现设计文档之间有冲突，以拆分后的专项设计文档为准：

- 前端以 weak_password_frontend_prd_v6.1.md 为准。
- 数据库以 weak_password_database_design_v6.1.md 为准。
- api-server 以 weak_password_api_server_design_v6.1.md 为准。
- Agent 以 weak_password_agent_program_design_v6.1.md 为准。

二、开发目标

实现 Aegis V6.1 弱密码检测能力，核心闭环如下：

1. 前端支持“一键分析资产应用”，只分析资产采集结果中的应用资产。
2. 如果没有应用资产，提示用户先执行资产采集。
3. 分析后列出所有可能存在密码的应用。
4. 用户可以对单个应用发起弱密码检查。
5. api-server 基于应用资产和 LLM 生成采集计划。
6. server 复用现有 ExecuteTool 链路下发 Agent 工具。
7. Agent 实现 WeakPassword.CollectCredentials 和受控辅助定位工具。
8. Agent 按指定文件和字段读取账号、明文密码、hash、salt、auth string，并输出标准 CredentialRecord。
9. 明文密码在 api-server 直接用默认 1000 字典、上传字典、AI 生成字典、混合规则和模糊规则匹配。
10. 加密密码/hash 由 api-server 构造 LLMPasswordMatchJob，明确标注 dictionary_block 和 credential_block，让 LLM 返回候选，再由服务端 verifier 二次校验。
11. 如果 LLM 为定位配置文件累计调用 10 次 Agent 辅助工具仍失败，任务报 config_discovery_failed。
12. 前端展示进度条、Agent 工具调用次数、失败原因、命中结果、AI 解释和整改建议。
13. 支持默认 1000 条弱密码字典。
14. 支持“AI 一键生成字典”，并支持保存为任务字典或自定义字典。

三、必须遵守的安全约束

1. 禁止 Agent 使用 find、locate、grep -R /、任意 shell、递归全盘搜索。
2. Agent 只允许读取服务端下发计划中的路径、Profile 默认路径或受控辅助工具返回并经 api-server 校验后的路径。
3. Agent 日志、server 日志、api-server 日志都不得打印密码明文、hash 原文、salt、token、API key。
4. 普通业务表默认不保存原始 CredentialRecord.credential_value。
5. 命中密码默认只展示 matched_password_mask。
6. 查看完整命中密码必须走 reveal 审批和审计。
7. LLM prompt 原文不得写入普通日志；只能保存摘要、模型、批次和必要审计信息。
8. 如果使用公网 LLM，encrypted_password_llm_match 必须受管理员配置控制。
9. LLM 对 hash 或加密密码返回的候选，凡是服务端可校验的算法必须经过 verifier 二次校验。
10. 不可校验的专有格式只能标记为 ai_inferred_needs_confirm，不能标记为 confirmed。

四、建议实现顺序

第 1 阶段：数据库和模型

1. 新增迁移 migrations/006_v6.1_weak_password_detection.sql。
2. 按 weak_password_database_design_v6.1.md 创建表和索引。
3. 新增 api-server/internal/model/weak_password.go。
4. 新增 api-server/internal/repository/weak_password_repository.go。
5. 初始化默认 1000 条弱密码字典。
6. 编写 repository 单元测试，覆盖任务、应用分析、字典、finding、错误和 reveal 审计。

第 2 阶段：Agent 工具

1. 新增 agent/internal/weakpass/ 包。
2. 实现 WeakPassword.CollectCredentials。
3. 实现 WeakPassword.ProbePath。
4. 实现 WeakPassword.ListConfigDir。
5. 实现 WeakPassword.ReadConfigSlice。
6. 实现 WeakPassword.ServiceUnitInspect。
7. 实现 WeakPassword.ProcessConfigHints。
8. 实现 WeakPassword.PurgeCredentialCache。
9. 在 agent/internal/tools/tool_manager.go 注册弱密码工具。
10. 实现 shadow、ini、yaml、json、properties、line_key_value、htpasswd parser。
11. 编写 Agent 单元测试，重点验证禁止 find、禁止递归、禁止 shell、日志不泄密。

第 3 阶段：api-server 服务

1. 新增 WeakPasswordHandler。
2. 新增 WeakPasswordService。
3. 新增 WeakPasswordAssetPlanner。
4. 新增 WeakPasswordCollectionRepairService。
5. 新增 WeakPasswordMatcher。
6. 新增 WeakPasswordDictionaryService。
7. 新增智能体工具 Credential.WeakPassword.Scan、QueryFindings、Explain。
8. 接入现有 serverClient.ExecuteTool，下发 WeakPassword.CollectCredentials。
9. 实现每个应用最多 10 次 Agent 辅助工具调用限制。
10. 实现 no_application_assets、agent_not_connected、config_discovery_failed、llm_match_verify_failed 等错误流转。
11. 实现默认 1000 字典、上传字典、AI 生成字典、混合规则、模糊规则。
12. 实现明文直接匹配。
13. 实现加密/hash 的 LLMPasswordMatchJob 和服务端 verifier 二次校验。
14. 编写 api-server 单元测试和 handler 测试。

第 4 阶段：前端页面

1. 新增 frontend/src/api/weakPassword.ts。
2. 新增 frontend/src/types/weakPassword.ts。
3. 新增 frontend/src/store/weakPassword.ts。
4. 新增弱密码检测页面。
5. 实现应用资产分析 Tab。
6. 实现无应用资产空状态和“去采集资产”按钮。
7. 实现候选应用列表和“检查弱密码”单应用入口。
8. 实现任务详情进度条，展示阶段、百分比、当前应用、Agent 工具调用次数 4/10、最近工具和错误。
9. 实现 10 次 Agent 工具失败后的 config_discovery_failed 展示。
10. 实现字典管理页面，展示默认 1000 字典摘要。
11. 实现 AI 一键生成字典抽屉。
12. 实现检测结果表、采集失败表、reveal 审批入口。
13. 编写前端测试。

第 5 阶段：联调和验收

1. 验证无应用资产时不创建任务，并提示先采集资产。
2. 验证应用资产分析只分析应用资产，不扫描主机文件系统。
3. 验证单应用检查能下发 Agent 工具并返回 CredentialRecord。
4. 验证明文弱密码能命中默认 1000 字典。
5. 验证 AI 生成字典可以保存并参与任务。
6. 验证 hash/加密密码 LLM 匹配后必须经过 verifier。
7. 验证 10 次 Agent 辅助工具失败后任务变为 config_discovery_failed。
8. 验证前端进度条和错误展示。
9. 验证日志中不出现密码、hash、salt、token。
10. 验证 reveal 完整密码必须审批。

五、关键接口要求

必须实现或预留以下 HTTP API：

1. POST /api/v1/weak-password/asset-applications/analyze
2. GET /api/v1/weak-password/asset-applications
3. POST /api/v1/weak-password/tasks
4. POST /api/v1/weak-password/tasks/by-application
5. GET /api/v1/weak-password/tasks
6. GET /api/v1/weak-password/tasks/:id
7. GET /api/v1/weak-password/tasks/:id/progress
8. GET /api/v1/weak-password/tasks/:id/hosts
9. GET /api/v1/weak-password/tasks/:id/findings
10. POST /api/v1/weak-password/tasks/:id/retry-failed
11. GET /api/v1/weak-password/dictionaries/default
12. GET /api/v1/weak-password/dictionaries
13. POST /api/v1/weak-password/dictionaries
14. POST /api/v1/weak-password/dictionaries/ai-generate
15. POST /api/v1/weak-password/findings/:id/reveal
16. POST /api/v1/assistant/tools/weak-password.scan
17. POST /api/v1/assistant/tools/weak-password.explain

六、Agent 工具要求

必须实现以下 Agent 工具：

1. WeakPassword.CollectCredentials
2. WeakPassword.ProbePath
3. WeakPassword.ListConfigDir
4. WeakPassword.ReadConfigSlice
5. WeakPassword.ServiceUnitInspect
6. WeakPassword.ProcessConfigHints
7. WeakPassword.PurgeCredentialCache

工具参数和输出遵循 weak_password_agent_program_design_v6.1.md。

七、配置项要求

新增或支持以下配置项：

WEAK_PASSWORD_ENABLED=true
WEAK_PASSWORD_MAX_FILE_BYTES=1048576
WEAK_PASSWORD_MAX_RECORDS_PER_HOST=500
WEAK_PASSWORD_MAX_AGENT_TOOL_CALLS_PER_APP=10
WEAK_PASSWORD_FORBID_FIND=true
WEAK_PASSWORD_FORBID_RECURSIVE_SEARCH=true
WEAK_PASSWORD_DEFAULT_DICTIONARY_SIZE=1000
WEAK_PASSWORD_AI_DICTIONARY_GENERATE=true
WEAK_PASSWORD_AI_DICTIONARY_MAX_SIZE=1000
WEAK_PASSWORD_LLM_APP_ANALYSIS=true
WEAK_PASSWORD_LLM_REPAIR=true
WEAK_PASSWORD_LLM_ENCRYPTED_MATCH=true
WEAK_PASSWORD_LLM_MATCH_BATCH_SIZE=200
WEAK_PASSWORD_REQUIRE_SERVER_VERIFY=true
WEAK_PASSWORD_REVEAL_APPROVAL_REQUIRED=true

八、测试要求

至少完成以下测试：

1. 数据库迁移测试。
2. 默认 1000 字典 seed 测试。
3. Agent parser 单元测试。
4. Agent 安全限制测试：禁止 find、递归、shell、越权路径。
5. api-server 应用资产分析测试。
6. api-server 单应用任务创建测试。
7. api-server 10 次 Agent 工具调用上限测试。
8. api-server 明文匹配测试。
9. api-server LLM hash 匹配和 verifier 测试。
10. 前端应用资产分析页测试。
11. 前端进度条测试。
12. 前端字典管理和 AI 生成字典测试。
13. 前端 config_discovery_failed 展示测试。
14. reveal 审批测试。

九、构建验证

完成实现后执行最小必要验证：

1. cd api-server && go test ./...
2. cd server && go test ./...
3. cd agent && go test ./...
4. cd frontend && npm run test
5. cd frontend && npm run build

如果某项因环境、依赖或耗时无法执行，必须在最终说明中明确说明未执行原因。

十、完成标准

实现完成必须满足：

1. 代码可编译。
2. 测试通过或明确说明无法运行原因。
3. 前端页面可完成一键分析资产应用、单应用检查、进度查看、字典管理、AI 生成字典、结果查看。
4. Agent 工具能采集指定文件字段并标准化输出。
5. api-server 能完成任务编排、LLM 分析、Agent 下发、匹配、错误处理和结果入库。
6. 数据库表和索引完整。
7. 不泄露密码、hash、salt、token 到日志或普通页面。
8. 不使用 find、递归全盘搜索或任意 shell。
```

# Assistant 长耗时扫描提前结束与弱口令工作流漏执行修复

## 1. 缺陷摘要

最新 Assistant 会话在漏洞扫描仍处于 `analyzing` 状态时结束等待，并返回了任务未完成的兜底回答。后台漏洞扫描本身没有停止，问题发生在 Assistant 对异步任务的观察窗口。

同一请求还包含弱口令检查，但 Mapping 已接受的弱口令能力没有进入最终执行计划，因此漏洞扫描结束后也不会继续执行弱口令检查。

本修复不修改数据库、公开 REST API、gRPC 协议或漏洞扫描服务的数据处理逻辑，也不要求在开发验证阶段重启任何运行中的服务。

## 2. 现场证据与根因

### 2.1 长耗时漏洞扫描

- 会话：`asst_d187f40a`
- 运行：`run_6aeec082`
- 扫描：`92d158f6-e8e8-460d-9e61-43fdd7533933`
- Assistant 两次执行固定的 24 次自动轮询，均在扫描未达到终态时失败。
- 最后一次已观察进度为 84%，而后台扫描继续运行。

原固定计划统一使用 24 次自动轮询。按 2 秒初始退避和 30 秒最大退避计算，单次观察窗口约 10.5 分钟；一次步骤重试后总观察时间约 21 分钟，短于大批量漏洞分析的正常耗时。

根因是 Assistant 使用了通用异步任务预算，未按漏洞扫描和弱口令扫描的长耗时业务特征选择观察档位。

### 2.2 弱口令工作流漏执行

`weak_password_assessment` 已存在于工作流注册表，相关工具也通过 Mapping 被接受，但工作流编译器注册表没有对应编译器。组合工作流中，只要其他工作流已成功编译，缺少编译器的工作流会被跳过，通用计划路径也不会再补回这些步骤。

此外，原弱口令工具仅支持单个 `candidate_application_id` 和单个 `task_id`，且没有声明异步完成契约，无法安全承接应用分析产生的多条候选事实。

### 2.3 漏洞扫描兜底文案误分类

运行证据账本使用单一 `VulnerabilityWorkflow` 标记同时表示漏洞扫描和漏洞修复。扫描等待失败时，兜底回答会错误检查“脚本是否生成”和“任务组是否下发”，与实际执行的漏洞扫描不一致。

### 2.4 批量弱口令进度只轮询一次且最终结论漏项

后续现场会话：

- 会话：`asst_f682cdde`
- 运行：`run_6db4ec94`
- 漏洞扫描：`19b7a5cd-5360-474e-a3fa-fd1af3745a0b`

该运行已完成漏洞扫描并返回“发现 3 个漏洞”，随后识别 6 个弱口令候选应用并创建 6 个任务。第一次批量进度查询成功，聚合状态为 `running`；下一次自动轮询却连续两次出现：

```text
missing previous_step argument(s): task_ids
```

Runtime 最终以 `repeated identical failed tool call; stopping step` 终止弱口令步骤。

首个偏差是当前 Runtime 步骤只保留最近一次工具结果。弱口令扫描创建结果以 `task_resolved` facts 携带 6 个任务 ID，第一次进度结果覆盖该结果后没有继续声明这些 facts，导致下一次轮询无法绑定 `task_ids`。

同时，最终回答一致性校验只拒绝与证据矛盾的内容，不检查已执行工作流是否被遗漏。因此模型仅输出资产采集结果时，漏洞扫描和弱口令任务可以被静默省略。

现场后台任务最终状态为：

- 完成 2 个、失败 4 个；
- 产生弱口令命中 2 条；
- 失败任务保留 `field_not_found`、`config_discovery_failed` 或 `file_not_found` 等真实错误。

这些结果证明“弱口令步骤失败”不能被解释为“未发现弱口令”，必须在最终结论中分别报告完成、失败和命中。

### 2.5 弱口令部分失败被重复展示为查询工具失败

修复任务引用后，会话 `asst_c51e789a` 已能持续查询全部 6 个任务并取得终态：

- 完成 2 个、失败 4 个；
- OpenSSH 命中 2 条，PostgreSQL 完成且未命中；
- Kafka、ZooKeeper 为 `field_not_found`；
- Redis 为 `file_not_found`；
- Nginx 为 `config_discovery_failed`。

`Credential.WeakPassword.QueryProgress` 的三次工具传输状态实际均为 `success`，但结果契约把聚合状态 `partial_failed` 映射为 `operation_status=failed`。Runtime 的步骤完成校验只接受成功或跳过的终态证据，因而不能用已取得的失败终态结束只读观察步骤，模型随后重复执行了两次相同查询。前端又根据 `operation_status=failed` 显示统一的“业务操作执行失败”，隐藏了结果中已有的应用级原因。

首个偏差是混淆了两种状态：

- 查询执行状态：是否成功读取到一份可信终态；
- 被观察工作流状态：扫描最终是完成、部分失败、失败或取消。

只读进度查询成功取得失败终态时，查询本身已经完成；不能把它当作查询工具错误重试，也不能丢失被观察任务的失败事实。

## 3. 修复设计

### 3.1 按计划内容选择异步观察档位

固定计划继续保持有界轮询：

| 档位 | 触发条件 | 最大自动轮询 | 任务超时下限 |
| --- | --- | ---: | ---: |
| `default` | 普通固定计划 | 24 | 保持原配置 |
| `long_running_scan` | 包含 `Vulnerability.Scan.Start` 或 `Credential.WeakPassword.Scan` | 190 | 2 小时 |

在 30 秒最大退避下，190 次轮询约提供 93 分钟观察窗口。工具调用的单步骤预算和总预算同步按轮询上限扩容，避免先被通用调用次数限制截断。

运行时配置日志新增：

- `async_observation_profile`
- `task_timeout`
- 保留现有 `max_async_poll_attempts`、`max_tool_calls` 和 `max_tool_calls_per_step`

这些字段只在运行构建时记录一次，不为每次轮询增加日志。

### 3.2 新增弱口令确定性编译器

`weak_password_assessment` 编译为以下数据流：

```text
主机名/IP
  -> Host.Resolve
  -> Credential.WeakPassword.AnalyzeApplications
  -> Credential.WeakPassword.Scan
  -> Credential.WeakPassword.QueryProgress
```

若目标是精确主机 UUID，则跳过 `Host.Resolve`；若目标是所有在线主机，则由应用分析服务直接执行在线主机筛选与去重。

参数绑定规则：

- `host_ids` 只能来自用户提供的精确 UUID 或 `host_resolved` 事实。
- `candidate_application_ids` 只能来自应用分析结果的 `analysis.candidates[].candidate_application_id`。
- `task_ids` 只能来自批量创建结果的 `created[].task_id`。
- 缺少任何事实时不允许模型重新生成 ID。

弱口令扫描工具改为调用现有的 `CreateTasksByApplications` 服务方法，批量创建候选应用任务；进度工具聚合所有 `task_ids`：

- 任一任务仍非终态：`running`
- 全部完成：`completed`
- 全部失败/取消：`failed`
- 完成与失败混合：`partial_failed`

进度结果声明为扫描工具的完成能力，Runtime 会持续轮询直到聚合状态达到终态或有界观察窗口结束。

### 3.3 区分漏洞扫描与漏洞修复证据

证据账本新增独立字段：

- `VulnerabilityAssessment`
- `VulnerabilityRemediation`
- `VulnerabilityScanIDs`
- `VulnerabilityScanStatus`
- `VulnerabilityScanProgress`
- `VulnerabilityScanTerminal`

纯扫描任务等待失败时，兜底回答只报告真实扫描 ID、状态、进度和“后台仍在运行”；只有实际进入脚本生成/执行能力时，才检查脚本与任务组证据。

### 3.4 延续批量任务引用并强制最终证据覆盖

批量弱口令进度契约增加 `tasks[].task_id -> task_resolved` fact 绑定。Gateway 在同一异步步骤刷新结果时，如果进度响应未携带引用 facts，也保留已有的操作引用、side effects 和 facts。这样每次自动轮询都能从上一结果继续绑定完整 `task_ids`。

弱口令进度响应增加：

- 每个任务的 `matched_findings` 和 `failed_applications`；
- 聚合 `matched_findings`；
- 任务总数、完成数、失败数和运行中数量由证据账本计算。

漏洞扫描完成状态显式写入最终 `found_vulns`，不再只在 `message` 中包含数量而让结构化字段保持 0。

最终答案增加证据覆盖不变量：

- 已存在漏洞扫描 ID 时，最终答案必须引用真实扫描 ID；
- 已创建弱口令任务时，最终答案必须引用至少一个真实任务 ID；
- 缺少上述引用时记录 `vulnerability_scan_evidence_omitted` 或 `weak_password_evidence_omitted`，并使用证据账本重新生成完整汇总。

日志事件由“答案与证据矛盾”调整为“答案与证据冲突或遗漏”，保留结构化 `conflict_codes`，不记录密码或大段扫描结果。

### 3.5 分离进度查询状态与扫描终态

`Credential.WeakPassword.QueryProgress` 将以下聚合状态都视为查询成功取得的终态证据：

- `completed`
- `partial_failed`
- `failed`
- `cancelled`

`pending`、采集/匹配中间态和 `running` 仍保持非终态并继续自动轮询。原始 `status` 不被改写，扫描失败也不会降级为“未命中”。

进度结果新增稳定汇总字段：

- `task_total`
- `task_completed`
- `task_failed`
- `task_running`
- `matched_findings`
- `failed_tasks[]`，仅包含任务 ID、应用名、状态、错误码和错误说明

前端使用这些字段展示“完成/部分失败/失败/取消”及任务数量，不再对可信的终态扫描结果显示通用工具错误。异步终态日志同时记录 `operation_status` 和 `observed_status`，便于区分“查询已完成”和“扫描部分失败”；日志不包含凭据值或完整采集载荷。

## 4. 安全与兼容性

- 不改变漏洞扫描后台任务，不发送停止命令。
- 不重启、不替换运行中的服务；本地构建产物不会自动部署。
- 弱口令批量任务继续复用服务端在线状态校验、去重和跳过原因。
- Assistant 工具结果不返回明文密码，进度结果只包含任务和采集状态。
- 弱口令 Assistant 内部工具参数由单数 ID 改为复数 ID；公开 REST API 保持不变。
- 弱口令任务进度响应新增计数字段，属于向后兼容的响应扩展。
- `failed_tasks` 只包含诊断元数据，不包含密码、哈希、盐或配置文件内容。
- 公开 REST API、数据库状态和值域保持不变；只调整 Assistant 内部工具的终态证据语义和前端摘要。
- 无数据库迁移、无 protobuf 变更。

## 5. 验收标准

1. 含漏洞扫描的固定计划使用 `long_running_scan`，至少配置 190 次自动轮询和 2 小时任务超时。
2. 普通固定计划仍使用 24 次自动轮询，不扩大所有任务的等待时间。
3. `weak_password_assessment` 对主机名/IP 编译出“解析、分析、批量扫描、批量进度”四步，且通过编译计划校验。
4. 应用候选 ID 和任务 ID 均由前序事实绑定。
5. 弱口令批量进度在任一任务运行时保持非终态，在所有任务结束后返回确定的聚合终态。
6. 纯漏洞扫描的失败兜底不出现“脚本”“任务组”或“未下发任务”等修复语义。
7. Assistant 和弱口令工具定向单测通过，API Server 构建通过。
8. 第一次批量进度查询后，后续自动轮询仍能绑定全部 `task_ids`。
9. 漏洞扫描完成状态中的 `found_vulns` 与保存后的漏洞数量一致。
10. 最终回答遗漏漏洞扫描或弱口令任务引用时，必须回退为包含资产、漏洞和弱口令结果的证据汇总。
11. 弱口令部分失败时，结论分别报告完成数、失败数和命中数，不得表述为“未发现弱口令”。
12. `partial_failed`、`failed` 或 `cancelled` 被查询到后，`QueryProgress` 作为只读观察工具只产生一份终态成功证据，不重复执行相同查询。
13. 前端弱口令终态卡片展示扫描状态、总数、完成数、失败数、运行数和命中数，不显示通用“业务操作执行失败”。
14. 应用级 `error_code` 和 `error_message` 保留在 `failed_tasks` 中，且不包含任何凭据材料。

## 6. 测试与回滚

定向测试：

```bash
cd api-server
go test ./internal/assistant ./internal/assistant/tools
```

构建验证：

```bash
cd api-server
make build
```

回滚时可独立回退：

1. 长耗时观察档位与运行日志字段；
2. 弱口令编译器及批量工具契约；
3. 漏洞证据分类字段与兜底文案。

回滚代码不会影响已启动的后台漏洞扫描任务。

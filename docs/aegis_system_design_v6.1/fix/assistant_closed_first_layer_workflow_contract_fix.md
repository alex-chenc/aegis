# 修复：智能体第一层封闭工作流契约

## 问题

智能体第一层意图分类当前输出开放的 `domain`、`object_type` 和
`operation`，后端再根据这些开放字段猜测工作流。只要第一层返回语义上相关、
但不属于业务注册表的宽泛标识，后续能力目录就会在第二层意图拆解前丢失正确
工作流。第二层模型无法选择未曝光的能力，并可能在相邻业务域中选择错误工具。

最新动态检测包请求被归入漏洞评估，继而把 CVE 外部编号绑定到只接受内部 UUID
的影响主机工具，是该问题的一次具体表现。

## 目标行为

1. 第一层 LLM 读取完整、紧凑的工作流卡片目录。
2. 第一层只返回注册表中存在的 `workflow_ids`，不得生成开放工作流名称。
3. 后端解析精确工作流 ID；明确的 Aegis 业务对象名称仅形成“不得遗漏”的工作流
   契约，不直接选择工具或绕过 Mapping。
4. 声明完整 `ExposedCapabilities` 的工作流使用该集合收窄第二层能力目录。
5. 工具授权、参数 Schema、审批和异步终态校验继续由现有后端边界负责。
6. 一个请求可以声明多个有序工作流；后面的生成或检测动作不得覆盖前面的扫描。
7. 澄清后的下一条消息恢复原工作流，而不是把 IP、CVE 或 package_id 当成新任务。
8. 检测包生成并检测的请求至少执行到构建终态/审核边界，未签名启用前保持活动状态。
9. 用户已提供唯一 CVE 和漏洞/攻击链描述时，HookPlan、eBPF、Sigma、关联规则和
   目标部署环境属于生成器输出或平台约束，不得反向要求用户提供。
10. AI 生成的草稿必须持久化 `ai_generated=true` 和非敏感的生成输入摘要。
11. AI 结果进入草稿表前必须通过覆盖性、Hook 白名单和 builder 禁用 helper 预检。

## 数据流

```text
User message + context
  -> IntentRouter(available workflow cards)
  -> IntentResult.workflow_ids (ordered, one or more)
  -> WorkflowRegistry.Resolve
  -> closed capability catalog
  -> IntentDecomposer(candidate capabilities)
  -> Mapping / ordered workflow compilers / runtime
  -> needs_input checkpoint or evidence-backed completion
```

## 多工作流与延迟澄清

- `漏洞扫描 -> 动态检测包` 会保留两个工作流并按用户表达顺序编译。
- 后续工作流缺少只能由前序结果确定的字段时，不在入口处取消已就绪步骤。
- Runtime 先执行前序工作流，完成后将后续问题保存为
  `pending_clarification`，会话状态保持 `active`。
- 用户补充 CVE 等字段后，只恢复 `remaining_workflow_ids`，避免重复扫描。

## 检测包生命周期

```text
Package.Draft.Generate
  -> Package.Build.Start
  -> Package.Build.Status
  -> awaiting_review / success
  -> Package.Sign
  -> Package.Enable
```

- “只生成草稿”以草稿为完成目标。
- “生成并检测”要求最终证据为 `detection_package_enabled`。
- 构建到达审核边界后保存 `package_id` 和状态，等待审核/继续，不提前完成。
- 恢复请求必须保留 pending artifact 标识，并重新经过 Mapping 和权限校验。

### Hook 白名单生成契约

- `Package.Draft.Generate` 在调用模型前读取当前生效的 eBPF Hook 白名单，并把
  `attach_type + attach` 精确列表作为强制安全契约加入生成提示。
- 工具输入只要求 CVE 和漏洞/利用描述；HookPlan、eBPF、Sigma 与关联规则由 AI
  生成。第一层若将这些输出列为 `missing_info`，在动态检测包工作流内清除这些
  伪缺失项，但仍保留真实缺少 CVE 或漏洞描述的澄清。
- 模型必须先返回结构化 `Coverage Decision`。仅当状态为 `supported` 且
  `uncovered_core_behaviors` 为空时，后端才接受后续代码产物；明确不支持时失败
  关闭，不创建无检测意义的草稿。
- 生成结果在写入草稿表之前解析 HookPlan，并使用与 `Package.Build.Start`
  相同的白名单语义校验。
- 生成服务同步执行 builder 当前禁用 BPF helper 策略预检：
  `bpf_probe_read_kernel`、`bpf_override_return`、`bpf_setsockopt`、
  `bpf_sk_redirect`、`bpf_get_current_task`。
- 首次结果缺少结构、违反白名单或调用禁用 helper 时，允许一次携带精确校验错误
  的完整重新生成；第二次仍不合法则失败关闭，不创建不可构建的草稿，也不自动
  扩大白名单。
- 白名单无法忠实覆盖漏洞利用链时，不得使用无关但允许的 Hook 冒充检测能力。
- 保存草稿时记录 AI 来源、CVE、漏洞描述、攻击前提、利用链、误报约束和模型名；
  不保存 API Key 或完整原始提示词。
- Assistant 工具与 HTTP AI 生成入口共用 `DetectionPackageGenerationService`，避免页面
  入口绕过覆盖性、Hook 白名单、helper 预检和 AI 来源写入。
- 任一 `Package.*` 必需阶段失败后，生命周期保持失败或部分成功，不创建
  签名/启用续接，也不提示用户回复“继续”。

### 澄清状态消费

- `pending_clarification` 使用 `nil` 更新表示已消费并从会话 metadata 删除，不能
  继续保留旧问题污染后续独立请求。
- 删除仅针对明确传入的 metadata 键，不改变 locale 等无关会话字段。

## 兼容性

- `WorkflowRegistry.Match` 保留给测试和非生产兼容场景。
- 生产编排以第一层返回的 `workflow_ids` 为准。
- 不涉及 HTTP、gRPC、数据库或前端协议变化。
- 草稿表已有 `ai_generated` 与 `ai_generation_input` 字段，本修复仅补齐既有
  字段的写入，不新增数据库迁移。
- 明确业务名词匹配只生成必选工作流约束，不产生工具名、参数或授权结果。
- `partially_succeeded` 和 `needs_input` 保持会话活动；只有完整后置条件成功才完成。

## 失败处理

- 工作流 ID 不在目录中时，向同一分类模型发起一次契约纠正请求。
- 纠正后仍不合法则终止本次运行，不能回退到开放字符串猜测。
- 非业务直接回答允许 `workflow_ids` 为空；需要业务工具的意图必须选择至少一个
  工作流。

## 验证

- 第一层请求包含全部工作流卡片。
- 合法工作流 ID 可以解析。
- 发明的工作流 ID 被纠正或拒绝。
- 动态检测包工作流能力目录不包含漏洞影响主机能力。
- “先扫描、再生成检测包”保留两个工作流且保持顺序。
- 后续工作流缺少 CVE 时先执行扫描，再延迟追问。
- 澄清回答恢复原目标和 artifact，不重新路由为主机查询。
- 唯一 `host_resolved` 事实可绑定单数 `host_id`，多结果不任意选择。
- 动态检测包检测请求在签名启用前不会标记完成。
- 已有 CVE 和攻击链时不会追问 HookPlan、eBPF、Sigma 或部署环境。
- AI 草稿写入正确来源标记和非敏感生成输入摘要。
- 禁用 BPF helper 的首次生成会携带精确错误重生一次，非法源码不会落库。
- 当前 Hook 白名单明确无法覆盖攻击链时不落草稿。
- 已消费的 `pending_clarification` 会从 session metadata 删除。
- 现有 assistant 定向测试和 api-server 构建通过。

# Aegis V6.3 智能体会话感知 PRD

## 1. 产品背景

Aegis 当前“智能体事件感知与防护”能够回答智能体运行了哪些工具、产生了哪些
进程/文件/网络行为，却不能回答：

- 用户最初要求 Agent 做什么；
- Agent 在会话中看到了什么可疑指令；
- 风险行为是用户明确授权、间接提示注入，还是 Agent 自行偏离；
- 一个长会话中哪段发生了提示词越狱或安全语义变化；
- 会话正文大约占用多少 Token，安全分析又消耗了多少 Token。

V6.3 新增“智能体会话感知”页面和完整后端链路，首批支持 Claude Code 和
OpenAI Codex CLI。

## 2. 目标

1. 在纳管主机上增量提取 Claude Code/Codex CLI 的可见会话内容。
2. 统一展示用户消息、助手可见回复、工具调用/结果、权限、compact 和生命周期。
3. 对每个会话执行确定性“规则分析”，识别提示注入、越狱、系统提示窃取、
   编码混淆和多轮绕过等风险。
4. 对策略允许的会话执行“AI 分析”，按 Token 预算分段并形成会话级结论。
5. 显示会话可见内容 Token 估算、来源上报 usage 和 Aegis AI 分析用量。
6. 将会话风险与现有 Agent Guard 工具/OS 行为关联，但保持语义与执行事实分离。

## 3. 非目标

- V6.3 不支持 Cursor、OpenCode、Claude Desktop 或其他 Agent。
- 不采集隐藏推理、private chain-of-thought、encrypted reasoning 或模型内部状态。
- 不读取完整文件正文、环境变量、认证文件、Claude tool-results spill 目录或任意
  用户 home。
- 不保存或导出未脱敏原文，不提供 reveal、legal hold 和全文搜索。
- 不对提示词执行实时阻断；不由规则/AI 直接 freeze、kill 或 deny 工具。
- 不将来源 input tokens 当作“当前会话正文 Token”。
- 不承诺第三方内部 JSONL 永久兼容；未知版本必须降级。

## 4. 用户与权限

| 用户 | 核心需求 | 默认能力 |
| --- | --- | --- |
| 安全观察员 | 看数量、覆盖率、风险趋势 | 仅 metadata 和脱敏摘要 |
| 安全分析员 | 查看脱敏会话、规则证据、AI 结论和关联行为 | 正文只读、重新分析、处置 marking |
| 安全管理员 | 配置采集范围、规则开关、AI 预算和保留期 | 分析员 + 策略管理 |
| 审计人员 | 追踪谁查看、复制或重新分析了会话 | 访问审计，不拥有采集策略写权限 |

新增权限建议：

```text
agent_session_awareness:read
agent_session_awareness:content:read
agent_session_awareness:rule:read
agent_session_awareness:ai:read
agent_session_awareness:ai:run
agent_session_awareness:marking:handle
agent_session_awareness:settings:write
```

## 5. 核心用户故事

### US-01 会话总览

安全分析员进入页面后，可看到总会话数、活跃会话、风险会话、采集不完整会话
和近 24 小时可见内容 Token 估算。

### US-02 会话筛选

可按主机、Agent 类型、会话状态、时间、采集覆盖、规则风险、AI verdict 和
Token 区间筛选，并支持服务端分页和排序。

### US-03 会话正文

可在详情抽屉中按时间顺序查看脱敏 user/assistant/tool/permission/compact item，
每个 item 显示类型、时间、Token 估算、截断/脱敏状态和来源序号。

### US-04 规则分析

可查看命中规则、严重级别、规则版本、命中 item、字符区间、上下文解释和规则
分析时间；点击证据可定位并高亮对应会话内容。

### US-05 AI 分析

可查看 AI 总结、verdict、置信度、风险类别、反证、不确定性和建议处置；可看到
会话被拆成多少 chunk、每段状态和 Token 消耗。

### US-06 Token 用量

可明确区分：

1. `可见内容 Token（估算）`：用于描述当前脱敏会话大小；
2. `来源调用用量`：Claude/Codex transcript 可提供时展示 input/output/cache；
3. `Aegis 分析用量`：规则为 0，AI 展示实际 provider input/output tokens。

任何估算值必须带 `~` 或“估算”标签，并显示估算方法。

### US-07 关联行为

可从会话 tool call 跳到现有 Agent Guard 行为证据。没有 PID/eBPF 关联时显示
“未证实”，不能把语义意图翻译成“已执行”。

### US-08 降级说明

当来源目录不存在、会话未持久化、静态扫描预算耗尽、parser 版本未知、内容策略
关闭或存在丢段时，页面说明具体原因，不把 partial 显示为 complete。

## 6. 页面信息架构

侧边栏：

```text
智能体防护
├── 智能体事件感知与防护
├── 智能体逃逸防护
├── 智能体配置检测
└── 智能体会话感知
```

路由：

```text
/detection/agent-guard/session-awareness
```

页面沿用“智能体事件感知与防护”的结构：

```text
Hero 标题/说明/设置按钮
  -> KPI 指标卡
  -> 采集提示（静态采集、待重连）
  -> 筛选卡
  -> 资产 Agent 列表（复用 Agent Guard/资产中心数据）
  -> Agent 详情抽屉
       └── 该 Agent 的会话列表
            └── 会话详情（内容/规则分析/AI 分析）
```

## 7. 功能需求

### FR-01 来源识别

- 首批只接受 `claude-code`、`codex`。
- session 唯一键包含 host、source UID、Agent 类型、storage namespace 和 source
  session ID。
- 同一 session resume/compact 不创建重复审计会话。

### FR-02 内容采集

- 默认 `redacted_text`；管理员可切换为 `metadata_only`。
- user/assistant/tool 等内容全部从 Claude/Codex 已落盘 JSONL 静态读取。
- 不安装 `aegis-session-hook`，不创建产品 ingress socket，不依赖现有 Agent Guard
  Hook 才能采集正文。
- 只保存用户可见和安全 allowlist 字段。
- 所有脱敏、截断、缺失和不可观测状态可见。

### FR-03 增量与回补

- 只消费完整 JSONL record。
- Agent 重启后从 dev/inode/offset/digest cursor 恢复。
- Agent 启动、配置变更、手工触发和默认每 30 秒执行一次有界静态扫描；不使用
  inotify/fanotify。
- 首次扫描默认回看最近 14 天：沿用 ADR Claude parser 的默认窗口，并补足 ADR
  Codex parser 当前没有年龄过滤的生产容量边界；仅对纳管 UID 和允许 storage root
  生效。
- active/idle/end/resume 等生命周期没有明确落盘记录时必须显示为 inferred。
- `complete` 只表示截至最近一次静态扫描已读到文件当时末尾，不等于会话结束。

### FR-04 规则分析

- 每次新增/更新 item 后可做增量规则分析。
- 明确结束记录或 `idle_inferred` 后生成 session 级规则 run。
- 首批规则至少覆盖直接提示注入、间接提示注入、角色伪造、安全策略绕过、
  系统提示窃取、编码混淆、多轮越狱和敏感数据诱导。
- 结果必须引用真实 item ID 和 Unicode code point offset。

### FR-05 AI 分析

- 支持 `all_sessions`、`rule_hit_only`、`manual_only` 三种触发策略。
- 默认 chunk target 6,000、hard 8,000 tokens，按模型上下文动态下调。
- 保持 turn 和 tool call/result 原子关系；超长单 turn 才做段落级切分。
- 每 chunk 结构化输出并引用 item ID；最终 reducer 只接收 chunk 结果和必要高风险
  证据，不重新发送全会话。
- 模型失败、超时、无效 JSON 或证据 ID 越权时为 `inconclusive/failed`。

### FR-06 Token 指标

- item 和 session 保存可见内容估算值、method、tokenizer version 和更新时间。
- 来源 usage 存在时保存 input/output/cache usage 和 coverage。
- AI run/chunk 保存 provider 实际 input/output tokens；无 usage 时标记 unavailable，
  不用估算冒充实际值。

### FR-07 风险表达

- 规则结果：`not_run|clean|matched|failed`。
- AI verdict：`not_run|benign|suspicious|malicious|inconclusive|failed`。
- 综合风险只做展示和告警；详情必须同时展示两种原始结果。
- “提示注入出现”和“Agent 已遵循提示注入”必须区分。

### FR-08 实时更新

- WebSocket 只发送 ID、状态、计数、风险和更新时间。
- 不发送 prompt、助手回复、工具参数/结果或证据片段。
- 断线后前端显示 warning，并以 REST refresh 恢复。

### FR-09 审计

- 查看正文、复制脱敏片段、手工触发 AI、确认/驳回风险均记录审计。
- 审计日志只记 session/item ID、operator、purpose、结果，不记正文。

## 8. 非功能需求

| 类别 | 指标 |
| --- | --- |
| 静态扫描开销 | 单轮默认目录枚举 p95 < 2 秒；不修改、不锁定来源 JSONL |
| 采集时效 | 新增完整 JSONL record p95 90 秒内出现在页面 |
| 规则分析 | 新 item 投影后 p95 10 秒内完成；会话进入结束推断后 final run p95 30 秒 |
| AI 分析 | 排队后单 chunk 默认超时 60 秒；会话级最大 10 分钟 |
| 页面 | 列表首屏 p95 < 2 秒；item cursor 50，最大 200 |
| 可靠性 | 重放幂等；断网恢复无静默丢失；missing range 显式可见 |
| 安全 | secret fixture 在传输、存储、日志和 UI 的泄漏数为 0 |
| 容量 | 默认单主机 32 个 active session；单 session 脱敏正文 50 MiB 上限 |

## 9. KPI

- 纳管主机 session source 覆盖率；
- `complete/partial/metadata_only/unsupported/source_not_found` 分布；
- 规则命中率、AI 升级率、AI inconclusive 率；
- 人工确认率和 false positive 率；
- 会话 Token 估算总量、Aegis AI input/output tokens；
- parser unsupported、sequence gap、redaction suppressed 数量；
- 从 item 产生到风险出现在页面的延迟。

KPI 不采集或上报会话正文。

## 10. 验收场景

1. Claude Code 会话包含“忽略所有先前规则”，规则分析定位该 prompt；AI 判断
   Agent 是否实际遵循，并给出反证。
2. Codex 读取测试网页后遇到间接注入，规则命中 tool result，AI 引用原始用户
   目标和后续工具行为形成结论。
3. 合法安全研究会话讨论 jailbreak 但未要求执行，规则可命中，AI 应保留合法
   上下文并避免标为已攻击成功。
4. 40k Token 长会话被拆成多个不超过动态 hard budget 的 chunk，最终结论能
   跳回真实 item。
5. 超长单 prompt 被安全切分，UTF-8/code fence 不破坏，重叠部分不重复产生
   session hit。
6. 来源提供 usage 时，页面同时显示“可见内容约 12k”和“来源 input 80k”，并
   解释 input 包含多轮重复上下文。
7. 未知 JSONL schema 只展示安全文件 metadata，coverage 为 unsupported，不从
   Hook 补采、不误解析用户或助手内容。
8. 普通观察员打开详情只能看到 metadata，API 也不返回正文。
9. 会话中含测试 API key，所有离开主机的内容都已替换为 redaction 标记。
10. AI 服务不可用时规则分析正常，AI 状态为 failed/inconclusive，页面不显示
    “安全”。

## 11. 发布范围

V6.3 GA 前必须完成：

- Linux AMD64 Agent；
- Claude Code/Codex CLI 两个来源；
- metadata_only 和 redacted_text；
- 规则分析、AI 分段分析、Token 三类指标；
- 页面、权限、审计、灰度、回滚和离线发布包适配。

macOS/Windows、其他 Agent、原文审批和 prompt 执行前阻断进入后续版本评审。

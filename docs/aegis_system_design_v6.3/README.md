# Aegis V6.3 智能体会话感知设计文档

- **版本**：V6.3 方案版
- **日期**：2026-08-10
- **状态**：设计完成，尚未实现
- **首批产品**：Claude Code、OpenAI Codex CLI
- **页面名称**：智能体会话感知

## 1. 版本定位

V6.3 在 V6.2 Agent Guard 已实现的智能体运行实例、真实会话边界、可信工具
事件、eBPF 行为证据和安全发现之上，新增“会话正文感知与安全分析”能力。

V6.2 已经存在完整会话正文采集的 P5 草案，但当前代码只实现了 session
start/end 和工具生命周期，不采集 prompt、助手可见回复或完整工具结果。V6.3
将该草案收敛为首批可交付范围，并做以下调整：

1. 首批只支持 Claude Code 和 Codex CLI，不包含 OpenCode。
2. 页面固定命名为“智能体会话感知”，不再使用“智能体会话检测”。
3. 安全结论明确拆分为“规则分析”和“AI 分析”，两者结果独立可见。
4. AI 不接收无界完整会话，而按 Token 预算和 turn 边界分段分析。
5. 每个会话显示“可见内容 Token 估算”；存在来源 usage 时，另行显示来源
   上报的模型调用用量，禁止混为一个指标。
6. 默认只上传和保存脱敏文本，不在 V6.3 实现原文 reveal/export。

## 2. 核心数据流

```text
Claude Code / Codex CLI
  -> 本地会话 JSONL 落盘
  -> Aegis Agent 定时静态扫描（有界发现、版本化解析、文件游标）
  -> agentsession（校验、脱敏、排序、加密 spool）
  -> gRPC ReportAgentSessionBatch
  -> Server
  -> Kafka: aegis.agent.sessions.v1
  -> DC（幂等投影、完整性检查、行为关联）
  -> PostgreSQL
  -> api-server（Token 汇总、规则分析、AI 分段分析、查询）
  -> WebSocket 元数据通知
  -> Frontend 智能体会话感知
```

## 3. 文档索引

| 文档 | 内容 |
| --- | --- |
| [prd_design_v6.3.md](prd_design_v6.3.md) | PRD、用户故事、页面范围、验收指标 |
| [adr_reference_and_decisions_v6.3.md](adr_reference_and_decisions_v6.3.md) | Uber ADR 调研、复用点、差异和设计决策 |
| [overall_architecture_design_v6.3.md](overall_architecture_design_v6.3.md) | 总体架构、组件职责、数据流、信任边界 |
| [agent_collection_design_v6.3.md](agent_collection_design_v6.3.md) | Agent 静态扫描、Claude/Codex parser、脱敏、游标、spool |
| [security_analysis_design_v6.3.md](security_analysis_design_v6.3.md) | 提示词规则、AI 分段、Token 估算、综合风险 |
| [backend_api_protocol_design_v6.3.md](backend_api_protocol_design_v6.3.md) | Proto、Server、Kafka、DC、api-server、HTTP API |
| [database_design_v6.3.md](database_design_v6.3.md) | 数据模型、表、索引、迁移、保留与回滚 |
| [frontend_design_v6.3.md](frontend_design_v6.3.md) | 页面结构、列表、详情、状态、权限和测试 |
| [implementation_test_rollout_v6.3.md](implementation_test_rollout_v6.3.md) | 实施阶段、测试、日志、指标、灰度与回滚 |
| [development_prompt_v6.3.md](development_prompt_v6.3.md) | 可直接交给开发智能体的主提示词 |

## 4. V6.3 核心决策

| 编号 | 决策 |
| --- | --- |
| V63-D01 | 借鉴 ADR 的“来源 parser -> 统一模型”，但使用 Aegis Go Agent 和现有传输链路，不嵌入 ADR Python Sensor |
| V63-D02 | 正文只通过 ADR 风格的本地 JSONL 静态扫描获取；不安装或依赖 Claude/Codex Hook |
| V63-D03 | 会话正文使用专用 gRPC/Kafka/数据库链路，不进入 `RuntimeEvent.event_data_json` |
| V63-D04 | Agent 启动/周期/手工触发有界扫描，以 dev/inode/offset cursor 增量读取；不使用文件监听器 |
| V63-D05 | 规则分析是确定性检测，AI 分析是无工具、只读、结构化输出的异步语义检测 |
| V63-D06 | AI chunk 默认目标 6,000 tokens、硬上限 8,000 tokens，并根据模型上下文动态下调 |
| V63-D07 | 会话 Token 指标至少分成可见内容估算、来源上报用量、Aegis AI 分析用量三组 |
| V63-D08 | 会话文本和工具输出均是不可信数据，不能成为分析器指令，也不能触发分析器工具调用 |
| V63-D09 | V6.3 只做检测、展示、告警和人工研判，不由会话规则或会话 AI 自动 deny/freeze/kill |
| V63-D10 | 新增 migration 使用 `032_v6.3_agent_session_awareness.sql`；现有 030、031 已被 V6.2 使用 |

## 5. 完成标准

只有以下条件同时满足，才可宣称 V6.3 已完成：

1. Claude Code、Codex CLI 各至少两个受支持版本 fixture 和一个未知版本降级
   fixture 通过。
2. 新建、继续、compact、结束和子智能体会话不会串联或重复。
3. 用户消息、助手可见回复、工具调用/结果和权限事件按真实顺序展示；隐藏推理
   不采集。
4. 所有正文先脱敏再离开主机；secret fixture 不出现在 gRPC、Kafka、数据库、
   日志、WebSocket 和浏览器存储。
5. 规则分析可独立运行，首批提示注入/越狱规则能定位到 item 和字符区间。
6. AI 分析按 Token 预算分段，超长单 turn、compact、失败重试和最终聚合均有
   明确状态。
7. 每个会话显示 Token 估算方法与更新时间；来源 usage 和 Aegis 分析用量不被
   标成“会话内容 Token”。
8. 页面结构与“智能体事件感知与防护”一致，具备顶部指标、筛选、列表、详情
   抽屉、加载/空/错误/降级状态。
9. 未授权角色看不到正文，正文不进入 URL、console、埋点、普通错误信息或通知。
10. 断网、Kafka 重放、Agent/API 重启、源文件 rotate/truncate 后数据无重复；
    无法补全时显示 missing range，不静默宣称 complete。

## 6. 参考资料

- [Uber ADR](https://github.com/uber/ADR)
- [ADR Sensor](https://github.com/uber/ADR/tree/main/Sensor)
- [ADR Detection](https://github.com/uber/ADR/tree/main/Detection)
- [ADR Claude Parser](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/parsers/claude_parser.py)
- [ADR Codex Parser](https://github.com/uber/ADR/blob/main/Sensor/adr_sensor/parsers/codex_parser.py)
- [Claude Code Sessions](https://code.claude.com/docs/en/sessions)
- [Claude Code application data](https://code.claude.com/docs/en/claude-directory)

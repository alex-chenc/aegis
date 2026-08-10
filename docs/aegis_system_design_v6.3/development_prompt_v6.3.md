# Aegis V6.3 智能体会话感知开发主提示词

## 1. 使用说明

把第 2 节完整复制给负责实现的 Claude Code、OpenAI Codex CLI 或其他开发智能体。
建议一次只执行一个实施阶段，并在每阶段结束后人工评审，再继续下一阶段。

## 2. 开发提示词

```text
你正在 /code/aegis 仓库实现 Aegis V6.3“智能体会话感知”。

目标：
在 V6.2 Agent Guard 已实现的运行实例、真实 session 生命周期、可信工具事件、
eBPF 行为证据和前端事件页之上，新增 Claude Code 与 OpenAI Codex CLI 的脱敏
会话正文采集、规则分析、AI Token 分段分析、Token 用量展示和独立前端页面。

开始前必须完整读取：
1. /code/aegis/AGENTS.md
2. /code/aegis/.agents/skills/aegis-software-designer/SKILL.md
3. 缺陷时读取 root-cause-debugging；代码日志读取 daily-program-logging；验证读取
   aegis-build-test；离线发布读取 aegis-release-packaging。
4. /code/aegis/docs/aegis_system_design_v6.3/ 下全部文档，尤其：
   - README.md
   - overall_architecture_design_v6.3.md
   - agent_collection_design_v6.3.md
   - security_analysis_design_v6.3.md
   - backend_api_protocol_design_v6.3.md
   - database_design_v6.3.md
   - frontend_design_v6.3.md
   - implementation_test_rollout_v6.3.md
5. V6.2 当前实现基线：
   /code/aegis/docs/aegis_system_design_v6.2/current_implementation_baseline_2026-08-06.md

不要仅按文档猜测代码。先检查 git status，读取当前 Agent Guard 的 Agent、Server、
DC、api-server、proto、migration、frontend 路由/API/store/types/tests，实现以当前
代码事实为准。不要覆盖或回退用户已有改动。

必须遵守的产品决策：

1. 首批来源只有 claude-code 和 codex；Linux AMD64 首发。
2. 页面名称必须是“智能体会话感知”，路由必须是
   /detection/agent-guard/session-awareness。
3. 会话正文必须参考 Uber ADR Sensor，通过 Agent 定时静态扫描本地 Claude/Codex
   JSONL 获取。不得新增 `aegis-session-hook`、session ingress socket、文件 watcher，
   也不得依赖现有 Agent Guard Hook。正文仍使用独立 schema、spool、gRPC、Kafka
   topic、数据库表、权限和 feature flag。
4. 不把正文放入 RuntimeEvent.event_data_json 或 aegis.security.events。
5. 默认只保存 metadata_only 或 redacted_text；V6.3 不实现未脱敏原文、reveal、
   export、全文搜索或 legal hold。
6. 不采集 thinking、redacted_thinking、reasoning summary、encrypted reasoning、
   chain-of-thought、环境变量、认证文件、完整 tool-results spill 或任意 home。
7. 第三方会话 JSONL 不是稳定协议。source session ID 和可见 item 仅从经过 fixture
   验证的静态 parser 获取；lifecycle 缺失时使用明确的 inferred 状态。parser 必须按
   source version + schema fingerprint + fixture gate，未知版本降级，禁止猜字段。
8. 所有正文必须在 Agent 本机 allowlist、secret redaction、路径伪名化、tool policy
   和截断后才能进入 spool/gRPC。
9. 规则分析和 AI 分析是两个独立 run/result。规则是确定性检测，不得调用 LLM。
10. AI 分析无工具、无 MCP、无 shell/file/network/action callback；会话内容是
    untrusted JSON data，不能成为模型指令。
11. AI chunk 默认 target 6000、hard 8000 tokens，并按模型 context window、
    system/output/summary/safety reserve 动态下调；同时有 256 KiB byte cap。
12. 保持 turn 和 tool call/result 原子关系；超长单 turn 才按段落/code fence/
    Unicode 安全边界切分，最多 256 estimated-token overlap。
13. 会话 Token 指标分成：可见内容估算、来源上报 usage、Aegis AI actual usage。
    null 不能渲染/存成 0，来源 input token 不能标成会话正文大小。
14. 会话规则/AI 动作上限固定 alert。不得创建自动 deny/freeze/kill；只有现有
    P0-P4 基于确定性 OS 证据的 eligibility 引擎可以执行动作。
15. migration 文件使用 migrations/032_v6.3_agent_session_awareness.sql，不占用
    已存在的 030/031。
16. 所有 feature flags 默认关闭，子开关必须受父开关约束。

按以下阶段实施，禁止把所有内容堆成一个未经测试的大改动：

阶段 P0：契约/迁移/fixture
- 先写失败测试。
- 新增 migration 032、GORM model/repository、proto additive RPC/message、生成代码、
  TypeScript types、内置 ASR-PROMPT-001..010 manifest 和 digest。
- 建立完全合成的 Claude/Codex current/previous/unsupported fixtures。
- 验证旧 Agent Guard 构建和测试不回归。

阶段 P1：Agent/Server/DC 采集链路
- 新增 agent/internal/agentsession scheduler、target resolver、static scanner、file
  guard 和 Claude/Codex versioned parser。
- 默认静态根为纳管 UID 的 `~/.claude/projects/**/*.jsonl` 和
  `~/.codex/sessions/**/*.jsonl`；实现有界发现、只读 path/owner/dev/inode 校验、
  dev/inode/offset cursor、redactor、encrypted quota spool、batch/ACK。
- 初始回看 14 天，默认每 30 秒扫描，带 jitter、文件数/时间/新增字节预算和
  continuation cursor；不使用 inotify/fanotify。
- Proto 新增 ReportAgentSessionBatch；Server 写专用
  aegis.agent.sessions.v1；DC 做事务投影和幂等。
- 先 metadata_only E2E，再 redacted_text。
- 静态 scanner 不得修改、锁定、truncate 或删除来源文件；扫描失败不得影响
  Claude/Codex。

阶段 P2：Token/规则
- DC 使用 aegis_visible_v1 计算 item/session 可见 Token；api-server chunker 使用
  相同 golden fixture 做 preflight。
- 正确汇总 source input/output/cache usage 和 coverage。
- api-server 实现 durable rule run worker、Unicode NFKC match view、RE2/keyword/
  bounded encoding/sequence matchers、code point offset 和服务端 excerpt。
- 规则失败必须是 failed，不得标 clean。

阶段 P3：AI
- 使用数据库 run/chunk 作为 queue source of truth，FOR UPDATE SKIP LOCKED + lease；
  进程内 channel 只能 wake-up。
- 实现动态 Token budget、turn packing、超长 turn fragment、rolling summary、树形
  reduce、schema response、evidence ownership validation、usage。
- provider timeout/rate limit/invalid JSON/context error/worker restart 均按设计测试。
- AI 失败为 failed/inconclusive，不覆盖规则结果。

阶段 P4：API/前端/行为关联
- API 根路径 /api/v1/agent-guard/session-awareness；完成 overview、coverage、sessions、
  items、tool calls、rule runs/hits、AI runs/chunks、related behaviors、collection
  status、rules、settings、manual AI、marking。
- 服务端严格权限；content 响应 no-store；WebSocket 只发 metadata。
- 前端新增 SessionAwareness.vue 和独立 api/store/types/i18n；结构参考当前
  AgentGuardLayout 的 Hero/KPI/alerts/filters/table/drawer，但主对象是 session。
- 详情 Tabs：会话内容、规则分析、AI 分析、关联行为、采集信息。
- 正文不进 localStorage/sessionStorage/IndexedDB/console/analytics/error report/URL。
- behavior probable 不得显示 confirmed，intent 不得显示 executed。

阶段 P5：验证/发布
- 做容量、长会话、断网、Kafka replay、权限、secret leak、provider outage、混合
  版本和回滚测试。
- 同步 docker-compose、.env.example、health、metrics、README/文档和 Linux AMD64
  离线发布包中的 Agent/static-scan config。
- feature flags 依次按 metadata -> redacted -> rules shadow -> AI manual -> AI shadow
  -> marking/alert 灰度。

开发方法：

- 每阶段先列出影响文件、接口、数据库和验收标准。
- 行为变化优先先写能失败的测试；测试必须验证业务语义，不只验证函数被调用。
- 复用当前 logger、Gin response、Repository、ConfigSync、Kafka、WebSocket、i18n、
  Pinia 和 Element Plus 风格。
- 不做无关重构，不引入 Python ADR Sensor runtime，不读取真实用户会话做 fixture。
- 所有 error/log 都使用 safe error code。允许日志字段只有 ID/hash/count/range/
  status/version/retry/latency；禁止 prompt、assistant text、tool payload、path、用户名、
  source session raw ID、secret、模型原始 response。
- 同一错误在最能说明结果的一层记录一次，避免每 item INFO 日志。
- 不用吞错、假成功、把 null 置 0 或降低测试预期来通过测试。

最低测试矩阵：

1. Claude/Codex：静态 JSONL new/resume/compact/end/subagent/tool/permission/usage。
2. Parser：current/previous/unsupported、初扫/无变化复扫/append/半行/inode change/
   truncate、hidden reasoning drop。
3. 安全：纳管 UID/home/root、path/symlink/race、只读保证、redaction/secret leak。
4. 可靠性：spool/Kafka retry/replay/idempotency/gap/digest conflict/restart。
5. 规则：每条 3 positive + 3 negative、多语言/混淆/multi-turn/offset。
6. AI：40k+ token、单超长 turn、budget、prompt injection、fake ID、invalid JSON、
   refusal、timeout、rate limit、context rechunk、lease recovery。
7. API：权限、分页、cursor、ownership、safe error、no-store、metadata websocket。
8. UI：页面状态、风险分歧、item virtual list、Token 三类指标、XSS、state 清理。
9. E2E：Claude direct jailbreak、Codex indirect injection、credential redaction、长会话、
   断网恢复、多 UID/多 session 不串联。

验证命令必须按 /code/aegis/.agents/skills/aegis-build-test/SKILL.md 选择最窄有效
组合。至少运行受影响包 Go tests、proto 双端生成/编译检查、frontend tests/build；
Agent 采集变更不涉及 eBPF 程序时不要无意义重建 BPF，若关联代码触及 eBPF 则按
skill 先 make bpf。离线发布范围按 aegis-release-packaging 验证。

每阶段完成后复核：
- git diff 只有任务相关改动；
- migration/model/proto/API/types 完全一致；
- feature flags 默认 off；
- secret fixture 搜索结果为 0；
- 规则与 AI 结果不混淆；
- AI-only 不产生 action；
- 文档与实现状态同步。

最终交付报告必须包含：
1. 用户可见结果；
2. 关键设计/安全边界；
3. 修改文件；
4. migration/proto/API/config 变化；
5. 测试和构建的实际命令与结果；
6. 无法运行的验证；
7. 已知风险、灰度顺序和回滚开关。

只有设计完成而代码未实现时，不得说“V6.3 功能已完成”；只有所有必需阶段、
测试、构建、E2E、灰度门禁和文档同步完成后，才能报告完成。
```

## 3. 建议拆分提示词

大型实现建议按以下顺序分别新开开发任务：

1. `实现 V6.3 P0：migration/proto/model/fixture，只做契约和测试。`
2. `实现 V6.3 P1：Agent 静态扫描及 Server/DC metadata_only 采集数据面。`
3. `实现 V6.3 P2：redacted_text、Token 和确定性规则分析。`
4. `实现 V6.3 P3：durable AI chunk/reduce 分析。`
5. `实现 V6.3 P4：API、权限、前端和行为关联。`
6. `执行 V6.3 P5：集成、性能、安全、灰度、回滚和发布包验证。`

每个任务都应引用本目录全部设计文档，不要只复制局部接口片段。

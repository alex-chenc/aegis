# Aegis V6.2 实施状态

**目标版本**：6.2
**当前阶段**：P0～P4 代码增量已实现；P5“智能体会话检测”方案已完成、待开发
**状态**：P0～P4 组件测试与非破坏性构建通过；P3 专用宿主机门禁及 P5 实现尚未执行
**更新时间**：2026-08-03

## 1. 结论边界

仓库已经实现开发提示词中 P0～P4 的代码、数据库、控制面、数据面、前端和
离线发布结构增量。P5 已完成三类会话采集、专用传输/存储、AI 语义分析、
风险标记和第三个前端子标签的设计，但尚未新增 migration 030、会话采集代码、
API 或页面实现。

当前共享环境也没有执行真实 BPF LSM attach、`EPERM`、freeze、kill 或逃逸操作，
没有在专用宿主机完成一条 LSM deny + freeze/resume 链。因此本文只报告
“P0～P4 代码实现和非破坏性验证完成、P5 设计完成”，不报告“V6.2 发布门禁
全部通过”。

## 2. 分阶段实现事实

### P0：契约、数据库与只读控制面

- `migrations/029_v6.2_agent_guard.sql` 保持 11 张事实表；五个内置规则和六个
  Profile 使用稳定 UUID、version 和 canonical SHA-256 digest 幂等 seed。
- immutable rule/profile 冲突使用 `DO NOTHING`，由 api-server 启动校验报告
  digest mismatch，不静默覆盖已存在定义。
- api-server 提供 catalog、policy、runtime、behavior、finding、analysis、
  action repository/service/API，并执行 scope、权限、状态和脱敏校验。
- frontend 提供“智能体事件防护”和“智能体逃逸防护”两个子页、详情抽屉、
  懒加载全景树、Finding/analysis 与策略入口。

### P1：运行身份、归属和 monitor-only 行为链

- Agent 内置 Codex、OpenClaw、Hermes Profile，使用
  `host + controller_pid + controller_start_ticks` 区分实例；支持 fork 标签、
  PID reuse、cgroup v1/v2、systemd scope、OCI/container 和周期 `/proc` 校准。
- session、execution unit、进程与行为事件经 Server/Kafka 转发，由 DC 幂等
  投影并通过 WebSocket 发布；ambiguous/unattributed 不复制、不参与动作。
- 统一行为 Schema 覆盖 process/file/network/identity/persistence/isolation/
  kernel/IPC，包含聚合、drop/completeness 和统一脱敏，默认仅监控。
- ConfigSync bundle 使用版本/digest、last-known-good、applied/rejected 状态；
  Agent Guard 和行为监控本地开关默认关闭。

### P2：确定性规则、Finding、智能分析和逃逸 audit

- DC 实现 `AGB-BUILTIN-001..005` evaluator、attempt/success/inconclusive、
  资源分类、五分钟跨事件关联、乱序/Kafka replay 幂等和真实 evidence graph。
- Agent 建立 namespace/cgroup/mount/capability/seccomp/no_new_privs 基线，
  逃逸传感器只报告 audit/would_deny；未证明能力时使用 degraded 或
  enforcement_unavailable。
- api-server Evidence Window 有界、脱敏、标记 untrusted evidence；LLM 输出
  使用固定 JSON Schema 和真实 event ID 校验。AI-only 始终只到 alert/人工确认。
- 前端展示行为/沙箱全景、隔离基线差异、规则 Finding、分析历史、反证、
  uncertainty 和 evidence completeness。

### P3：BPF LSM、execution-unit action 和 freeze 实现

- Agent 内置 BPF LSM，安全范围收敛为明确 Unix runtime socket 与
  `BPF_PROG_LOAD/BPF_BTF_LOAD` 原子拒绝；glob、跨事件和无法证明的规则保留
  audit/would_deny。guarded PID/cgroup、LPM policy map 与 fork/exit 共用内核状态。
- freeze 优先 cgroup v2 freezer，受控 fallback 为 pidfd/SIGSTOP；包含 protected
  targets、PID/start_ticks、UUID registry、timeout/auto-resume 和人工 hold。
- api-server、Server、DC 和 Agent 使用同一 action UUID/`AG-GUARD-<UUID>`，
  严格区分 accepted/dispatching/running/terminal，拒绝 host-wide、PID/path、
  wildcard 和 tool-only/AI-only 动作。
- 前端仅对具有权限且覆盖能力已证明的单一 execution unit 展示动作；freeze、
  resume、kill 有原因/确认，kill 需要确认短语，并显示轮询/WebSocket timeline。
- 实现与非破坏性 build/test 已通过；真实 LSM/freeze 专用宿主机门禁未执行。

### P4：可信工具语义、远端关联和 Profile 扩展

- Agent 提供默认关闭的本地 Unix hook receiver。socket 默认 `0600`；只有
  manifest 显式配置 `0660 + group_id` 时开放受控组访问。每个 source 固定允许
  UID/GID，并对 `SO_PEERCRED` 校验；官方/Hook/Aegis wrapper 事件还必须匹配
  pinned Ed25519 key 和逐事件签名。adapter artifact digest 只能作为附加约束，
  不能替代事件签名。
- correlation token 原文不进入事件、日志或数据库，只以
  `sha256:<64hex>` 作为 join key；tool session 复用已确认 execution unit 的现有
  session，不拆断 OS 进程/资源证据链。
- DC 只接受 `agent_official|adapter_hook|aegis_wrapper`，构建真实
  `tool_call -> process -> resource` 边。tool event 单独不执行规则、不创建
  Finding、不进入 action。
- 远端关系必须匹配已入库的 remote host/unit/event/hash OS sensor 事实；否则
  保持 `remote_unobservable`。已验证关系写入 Finding evidence graph，但不升级
  execution unit enforcement coverage。
- Claude Code、OpenCode、Gemini CLI 与原有三种产品一起形成六个 stable
  Profile；Agent/API/SQL 使用相同 key、字段和 canonical digest 回归。
- Panorama 只在完整 proof/session/hash 条件满足时展示工具名称；否则显示
  `tool_semantics_unobservable`。token hash、proof digest、verifier 和 external
  session ID 不进入 Panorama 响应。

### P5：智能体会话检测（仅设计，未实现）

- 方案覆盖 Codex、Claude Code、OpenCode 三类正式会话。Codex/Claude Code
  采用受管 Hook 定位和版本化 transcript 补全；OpenCode 采用 Aegis 插件、
  认证本机 API/SSE 对账和固定版本 `export --sanitize` 回补。
- 设计了独立 `AgentSessionBatch`、Kafka topic `aegis.agent.sessions.v1`、DC
  会话投影和 7 张新表，不复用 P4 可信工具入口传输完整 prompt/tool output。
- 设计了会话分段/摘要、固定 AI 输出 Schema、真实 item/tool/event 引用校验、
  planned/attempted/executed 联合判定和人工风险标记；AI-only 不自动阻断。
- 前端新增第三个子标签“智能体会话检测”，外层为会话列表，点击后在 80%
  抽屉展示“完整会话”“AI 语义分析”“关联行为”三个 Tab。
- 详细设计见
  [agent_session_detection_design_v6.2.md](agent_session_detection_design_v6.2.md)
  和
  [agent_session_detection_frontend_prd_v6.2.md](agent_session_detection_frontend_prd_v6.2.md)。

## 3. 已执行验证

```text
Agent/eBPF:
  go test ./... -count=1
  go test -race ./internal/agentguard ./internal/ebpf ./internal/client ./internal/configmgr
  go vet ./...
  Docker builder: make bpf BPF_TRANSPORT=all
  make build（Linux AMD64 / ARM64）
  readelf 检查 LSM sections、BTF 和 policy maps

Server:
  go test ./internal/handler ./internal/grpc_server ./internal/queue -count=1
  go vet ./...
  go build ./...

DC:
  go test ./... -count=1
  go vet ./...
  go build ./...

api-server:
  go test ./internal/model ./internal/repository ./internal/service \
    ./internal/api/handler -run AgentGuard -count=1
  go build ./...

Frontend:
  14 个 Agent Guard 文件 / 40 个测试通过
  npm run build（3154 modules）

跨组件和发布:
  scripts/tests/agent_guard_cross_contract_test.sh
  scripts/tests/build_release_package_contract_test.sh
  docker compose config --quiet
  git diff --check
```

发布契约测试以临时目录执行 `GENERATE_ONLY=1`，检查 Compose、环境模板、029
migration、init SQL、启动脚本、MinIO build context 和权限。未构建/导出全部
业务与基础镜像，也未生成可对外宣称完成的离线 ZIP。

## 4. 已知基线和未执行验证

- api-server 全量 `go test ./...` 仍有 Agent Guard 范围外的既有失败：analysis
  pause/iteration、custom vulnerability query，以及 audit-log shared-memory SQLite
  测试隔离；Agent Guard 定向测试与 api-server build 通过。
- frontend `npm run type-check` 仍被既有 command-audit、Assistant、
  vulnerability 等错误阻断，输出中没有 Agent Guard 错误；仓库没有 ESLint
  配置，因此 `npm run lint` 无法启动。
- Compose 仍提示既有顶层 `version` 字段 obsolete，不影响配置解析。
- 当前未执行真实生产 Docker socket、namespace escape、BPF LSM attach、
  `EPERM`、freeze/kill；也未对用户的运行中 Compose 栈写入测试数据。
- 仍需在专用支持宿主机完成真实 monitor-only、跨事件攻击链和
  LSM deny + freeze/resume 发布资格链，并验证灰度停止/回滚。

## 5. Feature Flag、灰度和回滚

- api-server：`AGENT_GUARD_ENABLED`、`POLICY_WRITE`、`ANALYSIS`、`ACTION`、
  `TOOL_ADAPTER`；工具语义还必须同时满足 Agent 本地开关、签名 manifest 和
  hook socket 配置。
- DC：projection、rules、findings、analysis request、alert、action publish。
- Server：action consumer。
- Agent：enabled、behavior monitor、tool adapter、enforcement、freeze；工具
  manifest 和 hook socket 默认空。
- P5 api-server：session detection、analysis、reveal、export；DC：session
  projection、behavior link、analysis request、alert；Server：session ingest；
  Agent：session collector、hook ingress、transcript tail、history backfill，全部
  默认关闭。
- `.env.example`、根 Compose、离线发布脚本和 Agent 安装模板全部默认关闭。
  开启顺序保持 consumer → monitor/projection → rules/findings/alert → analysis →
  专用宿主机 deny → freeze。回滚先关闭 freeze/enforcement 并恢复非人工 hold，
  再关闭生产者；保留现有 11 张表和审计历史。P5 实现后应从 metadata-only →
  redacted-text → AI shadow → risk marking → behavior link 顺序灰度，原文 reveal/
  export 只在授权范围开启。

## 6. 剩余实施与发布门禁

### 6.1 P5 实施入口

1. 先完成 P5.0：migration 030、7 张表、统一 Schema、只读 API、权限和第三个
   子标签骨架。
2. 再完成 P5.1：三类 Adapter、独立 Agent ingress/cursor/spool、追加式 proto、
   Server/Kafka/DC，并从 metadata-only 灰度到 redacted-text。
3. 完成 P5.2：长会话分段、AI 语义分析、风险 marking、引用校验和人工确认。
4. 完成 P5.3：与 P0～P4 OS 行为关联及完整页面，准确区分 planned、attempted、
   executed。
5. 最后完成 P5.4：授权原文、reveal/copy/export 审计、保留/删除和规模化测试。

### 6.2 既有 P3/P4 发布资格

1. 在专用 Linux 测试宿主机完成一条真实 BPF LSM `EPERM`。
2. 对同一 execution unit 完成 freeze、timeout auto-resume 和人工 resume，证明
   不影响同机其他 Agent/实例。
3. 在隔离集成环境完成真实 Kafka/PostgreSQL/WebSocket monitor-only 行为链和
   可复核跨事件攻击链。

### 6.3 V6.2 最终报告边界

只有 P3 专用宿主机门禁、P5.0～P5.4 代码与测试、三类真实产品集成验证、隐私
权限验证和完整离线发布验证均通过后，才可报告“V6.2 开发完成”。P5 尚未实现
期间只能报告“P5 设计完成”。

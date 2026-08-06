# Aegis V6.2 实施状态

**目标版本**：6.2
**当前阶段**：Agent Guard 工具事件、真实会话边界、权限优先逃逸检测、运行时 Hook 设置和只读内置规则目录已实现；完整 P5 会话正文仍待开发
**状态**：当前链路已完成定向测试、构建和 Compose 健康验证；P3 专用宿主机门禁及完整 P5 正文语义仍未执行
**更新时间**：2026-08-06

> 当前实现基线见 [current_implementation_baseline_2026-08-06.md](current_implementation_baseline_2026-08-06.md)。

## 1. 结论边界

仓库已经实现 Agent Guard 的运行时设置、Native Hook 会话开始/结束边界、可信工具
事件采集、api-server 工具命令规则匹配、权限优先逃逸判定、会话范围安全分析和只读内置规则目录。
逃逸检测按真实 session 保存产品化权限快照，只有受限权限、可信 Hook、PID/start_ticks
和 eBPF 执行结果完整关联时才生成 finding；Full Access、明确无隔离、远端不可观测、
权限未知或证据链不完整均不生成逃逸 finding。越界失败归类为
`policy_violation_attempt`，越界成功且证据完整归类为 `confirmed_escape`。
完整会话正文采集、AI 语义分析、风险标记和第三个完整会话检测页面仍是 P5 后续范围，
不能与当前已实现的生命周期 Hook/工具事件混为一谈。

当前共享环境也没有执行真实 BPF LSM attach、`EPERM`、freeze、kill 或逃逸操作，
没有在专用宿主机完成一条 LSM deny + freeze/resume 链。因此本文只报告当前代码链路
和非破坏性验证结果，不报告“V6.2 发布门禁全部通过”。

## 2. 分阶段实现事实

### P0：契约、数据库与只读控制面

- `migrations/029_v6.2_agent_guard.sql` 保持 11 张事实表；五个内置规则和六个
  Profile 使用稳定 UUID、version 和 canonical SHA-256 digest 幂等 seed。
- immutable rule/profile 冲突使用 `DO NOTHING`，由 api-server 启动校验报告
  digest mismatch，不静默覆盖已存在定义。
- api-server 提供只读规则 catalog、历史 policy、runtime settings、behavior、finding、
  analysis、action repository/service/API，并执行 scope、权限、状态和脱敏校验。
- frontend 提供事件/逃逸子页、详情抽屉、懒加载全景树、会话分页、安全分析和只读
  内置规则目录；Hook/工具开关在设置对话框即时下发。

### P1：运行身份、归属和 monitor-only 行为链

- Agent 内置 Codex、Claude Code、OpenClaw、Hermes、Zcode Profile，使用
  `host + controller_pid + controller_start_ticks` 区分实例；支持 fork 标签、
  PID reuse、cgroup v1/v2、systemd scope、OCI/container 和周期 `/proc` 校准。
- session、execution unit、进程与行为事件经 Server/Kafka 转发，由 DC 幂等
  投影并通过 WebSocket 发布；ambiguous/unattributed 不复制、不参与动作。
- 统一行为 Schema 覆盖 process/file/network/identity/persistence/isolation/
  kernel/IPC，包含聚合、drop/completeness 和统一脱敏，默认仅监控。
- ConfigSync bundle 使用版本/digest、last-known-good、applied/rejected 状态；
  Native Hook/工具适配器另使用 `agent_guard_runtime_settings.v1` 即时控制，关闭
  后由 Agent 清理 Hook 并停止上报。

### P2：确定性规则、Finding、智能分析和逃逸 audit

- 通用 OS 行为链路保留资源分类、attempt/success/inconclusive、跨事件关联、乱序/Kafka
  replay 幂等和真实 evidence graph；Agent Guard 工具命中不再由 DC evaluator 创建。
  `AGB-BUILTIN-004` 由 api-server 消费可信工具事件后匹配并直接写 Finding。
- Agent 仍采集 namespace/cgroup/mount/capability/seccomp/no_new_privs 等 OS 事实，
  但这些隔离快照不再单独触发逃逸 finding；逃逸链路以权限边界、Hook/PID 关联和
  eBPF 执行结果为准，未证明能力时使用 degraded 或 enforcement_unavailable。
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

- Agent 提供默认关闭的 Native Agent Hook receiver，并支持 Codex、Claude Code、
  OpenClaw、Hermes、Zcode。运行时设置由 api-server 保存到 `system_configs`，在线
  Agent 应用后注入 Hook，关闭后清理 Hook；离线只记录等待重连状态。
- Hook 事件保留真实 session start/end 和 `tool_call_started/completed/failed`，按
  `tool_call_id` 幂等合并；普通终端输出或进程名猜测不能作为可信工具事件。
- api-server 消费可信工具事件，按工具输入/attributes 提取命令并匹配
  `AGB-BUILTIN-004`，直接写入当前 session 的 Finding；证据直接引用工具事件 ID，
  规则归属为 api-server。
- Agent eBPF/`/proc` 只做 PID/PPID/start_ticks/cmdline 和工具到进程的关联；DC 只做
  规范化、行为投影和 WebSocket 推送，不重复命中工具规则。
- correlation token 原文不进入事件、日志或数据库，只以
  `sha256:<64hex>` 作为 join key；关联失败时保留工具 Finding 并标记 unattributed。
- 远端关系必须匹配已入库的 remote host/unit/event/hash OS sensor 事实；否则
  保持 `remote_unobservable`。已验证关系写入 Finding evidence graph，但不升级
  execution unit enforcement coverage。
- Profile 目录和 Native Hook 注入支持五个 stable agent type；完整会话正文的
  Codex/Claude/OpenCode 等 P5 Adapter 仍未完成。
- Panorama 展示真实会话和实际进程关联；安全分析只展示命中规则的工具、命令行和
  可关联 PID/PPID，不展示全量进程树，也不回退为统一 controller PID。

### P5：完整智能体会话检测（生命周期 Hook 已实现，正文语义未实现）

- 当前 P4/P5 交界能力已覆盖五类智能体的真实 session start/end 和工具调用事件，
  但不包含完整 user/assistant/transcript 正文。
- 方案覆盖 Codex、Claude Code、OpenCode 三类完整正式会话。Codex/Claude Code
  采用受管 Hook 定位和版本化 transcript 补全；OpenCode 采用 Aegis 插件、
  认证本机 API/SSE 对账和固定版本 `export --sanitize` 回补。
- 设计了独立 `AgentSessionBatch`、Kafka topic `aegis.agent.sessions.v1`、DC
  会话投影和 7 张新表，不复用 P4 可信工具入口传输完整 prompt/tool output。
- 设计了会话分段/摘要、固定 AI 输出 Schema、真实 item/tool/event 引用校验、
  planned/attempted/executed 联合判定和人工风险标记；AI-only 不自动阻断。
- 完整 P5 前端第三个子标签“智能体会话检测”、正文抽屉和风险标记仍未实现；当前
  事件页已经支持真实 session ID 分页和按 session 的工具安全分析。
  完整 P5 外层为会话列表，点击后在 80%
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
  Agent Guard 定向测试：3 个文件 / 6 个测试通过
  npm run build 通过
  frontend Docker 镜像重建、重启，健康检查和 http://localhost:8081/ 返回 200

跨组件和发布:
  scripts/tests/agent_guard_cross_contract_test.sh
  scripts/tests/build_release_package_contract_test.sh
  docker compose config --quiet
  git diff --check
```

本次验证还覆盖 Agent Guard api-server runtime settings/tool rule 定向测试、DC
全量测试、Agent 全量测试和 Server 定向测试。发布契约测试以临时目录执行 `GENERATE_ONLY=1`，检查 Compose、环境模板、029
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
  `TOOL_ADAPTER`；Native Hook/工具适配器的运行时开关通过页面设置写入
  `agent_guard_runtime_settings.v1`，不要求手工编辑配置文件。
- api-server tool consumer：`aegis-api-server-consumer-agent-guard-tool-rules`，
  消费可信工具事件并匹配 `AGB-BUILTIN-004`。
- DC：projection、通用 rules、findings、analysis request、alert、action publish；
  Agent Guard 工具事件不在 DC 重复命中。
- Server：action consumer。
- Agent：enabled、behavior monitor、tool adapter、session Hook、enforcement、freeze；
  Hook 注入由运行时设置控制，manifest/socket 仍负责本地安全边界。
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

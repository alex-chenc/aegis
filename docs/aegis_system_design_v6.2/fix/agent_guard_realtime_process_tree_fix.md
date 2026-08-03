# Agent Guard 实时进程树与会话标识修复

**版本**：6.2-P1/P4 真实会话修复
**范围**：Agent、eBPF、DC 既有行为投影、api-server、frontend
**部署约束**：不删除或回填历史数据；仅滚动更新 frontend、api-server 与本机 Agent，
不重启 PostgreSQL、Kafka、server 或 dc

## 1. 问题与根因

当前 `运行实例` 以 `host_id + controller_pid + controller_start_ticks` 标识，
但 `sched_process_exit` 只清理内核 `guarded_pids`，没有把退出事实送到 Agent Guard。
周期 reconciler 也只删除进程标签，不把 controller、session 和 execution unit
转换为 stopped/ended。结果是已退出 PID 长期保留为 running。

行为全景中的 `local_process_tree` 目前直接把行为事件投影成 process 节点，没有
根据 `pid + start_ticks + ppid` 组装父子关系；没有行为事件的短生命周期 controller
因此只有执行单元空壳。现有 `activity_window` 是按运行实例生成的推断窗口，不是
Codex 对话会话；可信 Adapter 即使知道来源会话，也没有上传 `external_session_id`。

现场还有两个部署级首因：安装脚本把 `AgentGuardEnabled` 和行为监控默认写成 false，
导致新 Agent 虽已连接但不启动采集；目标内核又拒绝了 perf-event tracepoint link，
使 `sched_process_fork` 程序无法挂载。fork/exit 生命周期改用 raw tracepoint，规避
perf-event 依赖；安装脚本只默认开启监控，Tool Adapter、执行阻断与冻结仍保持关闭。

逻辑 Agent scope 的 finding 查询曾先枚举 scope 下全部 runtime instance，再受限于
100 条分页上限。历史实例持续增长后，即使当前只有少量运行实例也会返回
`agent_guard_scope_too_broad`。目标行为是让仓储层直接用签名 scope 中的
`host_id + agent_type + profile_key`（或 `asset_id`）约束 finding；只有客户端显式指定
instance 时才查询并验证该实例是否属于 scope。

## 2. 成功标准

1. eBPF fork 流同时产生 fork 和主线程 exit 事件；事件包含 PID、PPID、UID 和 comm。
2. Agent 对 fork/exec/exit 生成不可变行为事实，Actor 必须包含
   `pid、ppid、start_ticks`；controller 退出时实时上报 instance stopped、session ended
   和 execution unit stopped。丢失 exit 时，reconciler 使用 `/proc` 全量扫描兜底；
   每轮 reconcile 还会上报运行实例心跳，刷新 `last_seen_at`。
3. DC 继续使用现有 `agent_behavior_events` 不可变表保存进程事实，不增加可变树表。
4. API 每次展开执行单元或进程节点时，重新读取该 scope 的最新进程事实，以
   `pid + start_ticks` 建节点，以 PPID 建直接父子边；退出只改变节点状态，不删除历史。
5. Codex 使用原生 `SessionStart`、`PreToolUse`、`SessionEnd` Hook 直接上报真实
   `session_id`；每个真实 ID 创建独立行为 session/unit，页面原样展示该 ID。
   同一实例已有真实 Hook 会话时，不再混显 `activity_window`。
6. 运行实例选择器只默认展示 running 当前实例，并使用 API 返回的 total，
   不再把固定 `page_size=100` 显示为真实总数。
   数据库中状态仍为 running、但超过 90 秒（三次心跳周期）未收到 Agent 心跳的历史实例按 stale 投影，
   不计入当前运行数量，也不需要直接改写历史数据。
7. 安装脚本默认启动 Agent Guard 监控模式；fork/exit eBPF 对象声明并使用 raw
   tracepoint，目标主机日志出现 `program=fork attach=raw_tracepoint`。
8. 逻辑 Agent scope 的 finding 查询不枚举历史 runtime instance，不受实例累计数量影响；
   显式 instance 仍执行严格归属校验。

## 3. 数据流

```text
sched fork/exec/exit
  -> Agent eBPF Event(pid, ppid)
  -> IdentityTracker(pid, start_ticks, ppid, instance/session/unit)
  -> aegis.agent_behavior.v1
  -> Server/Kafka/DC
  -> agent_behavior_events immutable process facts
  -> API rebuilds current/historical tree per request
  -> UI lazy-expands PID tree and event evidence
```

树不是数据库实体，不缓存父子结构。PID 重用通过 `start_ticks` 区分。只有 PPID 而
没有父进程 start_ticks 时，上层只在同 execution unit、时间相容的候选中建立父边；
无法唯一关联时把节点保留为根，禁止错误串树。

## 4. 会话边界

- `agent_behavior_sessions.id`：Aegis 内部行为归因 ID。
- `external_session_id`：Codex thread/session 等来源稳定 ID；必须由已验证的
  Agent 官方事件、Adapter Hook 或 Aegis wrapper 提供，并包含在签名载荷中。
- `activity_window`：缺少来源会话时的推断窗口，只用于 OS 行为聚合，UI 不得称为
  Codex 会话。

Codex 0.145.0 的原生 Hook 标准输入提供 `session_id`。Aegis 不读取不稳定的
transcript 格式，也不从时间窗口推测 Codex 会话：

```text
SessionStart(session_id, hook PPID)
  -> 固定 PPID + /proc start_ticks 为该会话根进程
  -> 创建 confirmed BehaviorSession + local_process_tree
  -> 追加 session_root 进程事实（PID/PPID/脱敏 cmdline）

PreToolUse(session_id)
  -> 共享 app-server 场景下，在工具子进程 fork 前激活对应会话根
  -> 已归属的其他会话子树不变

sched_process_fork/exec/exit
  -> 继承根进程的 session/unit 标签

SessionEnd(session_id)
  -> 结束对应 session/unit 并清理其后代标签
  -> 不停止仍被其他会话共享的 Codex app-server RuntimeInstance
```

Hook helper 只上传 lifecycle、来源 session ID、PID、start_ticks、时间和签名，禁止
上传 prompt、response、transcript、环境变量、工具参数或输出。事件同时通过
`SO_PEERCRED + allowed UID + Ed25519 + event replay` 校验。安装后使用：

```bash
/opt/aegis-agent/aegis-codex-hook provision
```

命令生成私钥、可信 source manifest 和 `~/.codex/hooks.json` 三个 Hook 点，并输出
需要写入 `/etc/aegis-agent/config.toml` 的三项非敏感配置。非托管 Hook 仍必须按
Codex 官方机制在 `/hooks` 中审查信任；新建或恢复的会话随后生效。
受管主机可改用 `--hooks '' --managed-requirements /etc/codex/requirements.toml`；
helper 只在目标 requirements 文件不存在时创建，避免覆盖管理员现有策略。

完整 P5 会话内容、turn/item 和跨进程 resume 仍使用独立
`agent_conversation_sessions` 方案。本修复只建立可信来源会话标识和 OS 行为关联，
不采集提示词或响应正文。

## 5. 测试设计

- eBPF Go 解码：fork 与 exit 共用 map 时能生成正确 event type、PID、PPID。
- eBPF 配置/对象：fork 使用 raw tracepoint，两个对象均能加载为 RawTracepoint 类型。
- 安装脚本：api-server 与 server 的兼容脚本都默认开启 Guard 与行为监控，但不启用阻断。
- API scope：超过 100 个历史实例时逻辑 scope 查询仍成功；显式越权 instance 被拒绝。
- IdentityTracker：controller exit 转 stopped；child exit 不停止 controller；
  reconciler 丢失 PID 时产生同等停止转换。
- Manager：exit 在 `/proc/<pid>` 已消失后仍可使用 tracker 快照生成行为并排队停止状态。
- Tool Adapter：`external_session_id` 受签名保护、长度/控制字符校验，状态事件透传。
- Codex Hook：SessionStart 使用首个 Hook 父 PID 建根；PreToolUse 在共享父进程下
  切换当前 session；SessionEnd 只关闭对应 session/unit；签名篡改和重放拒绝。
- API：乱序行为事实按 `pid + start_ticks` 去重，直接父子关系正确，PID 重用不合并，
  exit 后节点状态为 stopped；根和子进程均显示 `PID + 脱敏 cmdline`。
- Frontend：真实 `external_session_id` 原样显示；同实例有真实会话时隐藏推断窗口；
  PID/PPID/cmdline 和 running/stopped 状态可见。

## 6. 日志、安全与回滚

只对 controller 生命周期结束记录一次结构化 INFO，字段限于 host、instance、PID、
原因；普通子进程 exit 属于高频路径，不记录 INFO。不上报环境变量、正文或完整命令。
`external_session_id` 入库前继续经过 DC 脱敏和长度限制。

回滚可独立恢复 Agent exit 事件格式和 API 树投影；行为表为追加式，旧事件仍兼容，
缺少 exit 的旧实例继续由 stale/运行状态修复策略处理。本次不对既有数据做回填或删除。

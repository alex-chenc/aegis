# Agent Guard 工具命令规则归属调整（v6.2）

## 1. 目标

Agent Guard 的规则命中以智能体上层 Hook 上报的工具事件为事实来源。工具名称、工具输入、工具响应和从输入中提取的命令行由 API-server 统一匹配规则并生成安全分析 finding。

Agent 不再对工具事件执行规则命中；Agent 的 eBPF 只用于把工具事件关联到真实的 PID、PPID、进程启动时间、命令行和资源事件。eBPF 关联失败不能阻断工具事件上报，也不能阻断 API-server 对工具命令的规则匹配。

## 2. 目标链路

```text
Agent Hook
  -> Agent：校验可信 Hook、绑定真实会话、用 eBPF 补充进程关联
  -> Server/Kafka：转发 aegis.agent_behavior.v1
  -> DC：只做 Agent Guard 行为投影和查询数据落库
  -> API-server：消费工具行为事件，匹配工具命令行规则，写 agent_security_findings
  -> 前端：按会话查询行为和该会话的 findings
```

`agent_security_findings.evidence_event_ids` 对工具命中只保存工具行为事件的 `raw_event_id`。PID/PPID/命令行是该工具事件的关联补充信息，不作为规则命中的替代来源。

## 3. 规则范围

第一阶段迁移 Agent Guard 工具命令规则（包括敏感命令、网络传输、提权、权限变更、命名空间/挂载、持久化、破坏和防御控制等命令类别）。规则名称继续从 API-server 的中英文规则目录读取，前端显示本地化后的规则名称。

旧的通用运行时 Sigma/eBPF 事件链路与 Agent Guard 工具命令规则是两个不同的数据面：本调整不把旧运行时事件伪装成工具调用，也不把 eBPF 的 `MatchedRuleId` 当作工具规则命中。Agent Guard 通用行为事件不再由 DC 的 `ProcessBehavior` 产生 finding。

## 4. 失败与兼容

- 工具事件没有 eBPF 关联时，仍由 API-server 匹配并生成 finding；详情中的 PID/PPID/命令行标记为未关联。
- 工具事件不是完成/失败终态时不产生命中，避免一次工具调用产生多个 finding。
- API-server 规则匹配失败只记录结构化错误和事件 ID，不记录完整工具输入或可能包含凭据的命令内容。
- 历史 DC finding 不删除；新数据由 API-server 生成，后续查询按会话、工具事件和规则命中展示。
- API-server 规则消费开关关闭时不生成新 finding，但 DC 仍继续保存行为事件。

## 5. 验收标准

1. Agent Hook 工具事件不再在 Agent 或 DC 产生工具规则命中。
2. API-server 能用没有 eBPF 关联的已完成工具命令生成命中。
3. 一个工具终态事件的 finding 只引用该工具事件，详情能显示工具名称、工具输入/命令行和关联 PID 信息。
4. 普通 `echo` 等不命中命令不生成敏感命令 finding。
5. API-server、Agent 和 DC 的定向测试通过，并完成相关服务构建。

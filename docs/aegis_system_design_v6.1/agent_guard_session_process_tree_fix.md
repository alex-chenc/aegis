# Agent Guard 会话进程树与安全分析详情修复

## 问题与成功标准

- 运行实例选择器显示 Agent 行为会话 UUID，不再显示控制器 PID。
- 行为全景在选中会话后直接展示该会话的完整 PID 进程树；节点显示 PID、PPID、状态和完整命令行，不再展示 Codex 根节点或右侧证据引用面板。
- 具备 Agent Guard 分析读取权限的用户可以打开 Finding 安全分析详情；原始行为证据接口仍保持独立证据权限。

## 目标数据流

```text
Agent scope -> runtime instance -> behavior session
                         selected session_id
                               -> panorama?session_id=...
                               -> process roots -> signed process children
```

后端复用现有会话、执行单元和 PID 树构建逻辑，仅在全景查询带 `session_id` 时返回该会话的进程根节点。进程子节点仍通过已签名的全景节点加载，保持主机、实例、会话和执行单元边界校验。

## 安全分析规则进程树

安全分析页按 Finding 的 `rule_hits` 展示实际命中的规则名称，而不是展示全部规则定义。读取 Finding 详情时，api-server 解析规则命中的事件 ID：先查询 `agent_behavior_events`，查不到时再按同一事件 ID 查询 `runtime_events`，将运行时事件中的 `actor`、关联会话 ID 和执行单元 ID 投影为统一行为事件，再按 Finding 的主机/实例/会话/执行单元边界查询进程事实，并返回 `matched_rules[].process_tree`。树节点包含 PID、PPID、启动时刻、进程名、可执行文件、工作目录、采集状态和完整命令行；直接触发规则的进程节点带有 `matched=true` 和命中事件 ID。

规则名称优先使用 Finding 命中记录中的名称，其次从版本化规则目录解析，解析失败时保留规则键。运行时事件无法解析或进程事实确实缺失时，仍返回规则名称和命中事件，并记录未解析计数；不将整条安全分析详情判定为不可用。该聚合结果随 Finding 详情返回并受 `agent_guard:analysis:read` 保护，不改变原始行为证据接口的 `agent_guard:evidence:read` 权限边界。

## 权限与兼容性

Finding 详情是安全分析页面展示所需的分析结果，使用 `agent_guard:analysis:read`；行为原文、原始证据和全景节点接口继续使用 `agent_guard:evidence:read`。旧的 `instance_id` 详情链接继续有效，首次进入时会自动选择该实例的最新会话。

## 测试与回滚

- 前端覆盖会话选择、按会话请求 PID 树、树节点命令行展示和分析详情状态。
- 后端覆盖 `session_id` 全景请求、会话越权拒绝、PID 树根节点、Finding 详情路由权限，以及 `runtime_events` 回退后按命中规则展示进程树。
- 回滚时移除前端会话参数和后端全景分支即可恢复原有 Agent 根节点展示；数据库无需迁移。

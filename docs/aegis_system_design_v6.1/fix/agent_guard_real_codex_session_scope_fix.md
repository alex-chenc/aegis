# Agent Guard 真实 Codex 会话范围修复

## 问题与成功标准

Agent 识别到 Codex 控制器后会先创建 `activity_window` 推断会话，并将其
进程事件上报；Hook 开始和结束只切换根进程标签，已有子进程标签可能继续
沿用旧会话。页面因此展示了大量不对应本地 Codex 会话文件的内部 UUID。

目标是：

- 只有通过签名 Codex Hook 成功开始的真实会话才允许进入行为事件流；
- 只有该真实会话根进程及其进程树内、且发生在会话活动期间的事件才上报；
- Hook 结束后，根进程和全部后代进程解除该真实会话归属，后续事件丢弃；
- 会话选择器展示 `external_session_id`，即本地 Codex 会话真实 ID，不展示
  数据库内部记录 UUID或 `activity_window` 推断会话；
- 保留历史数据，不执行数据库删除或回写。

## 数据流与实现决策

Codex Hook 的 `SessionStart`/`SessionEnd` 经过本地签名 Unix Socket进入
Agent。Agent 仍使用内部 UUID关联数据库中的行为记录，但每条真实会话
记录保留并返回 `external_session_id` 作为用户可见 ID。内部 UUID不再作为
页面会话 ID展示。

运行实例发现仍可用于维护控制器生命周期和进程标签，但默认
`activity_window` 只作为 Agent 内部未归属状态，不创建上报的会话/执行单元
生命周期，也不允许行为事件通过标准化器。

Hook 开始时，将同一运行实例当前已归属的进程标签整体绑定到新会话；Hook
结束时，将该会话的全部进程标签恢复为无会话状态。新 fork/exec 只能从仍在
活动的真实会话标签继承，PID 与 start_ticks 继续作为进程身份边界。

## 安全与兼容性

- Hook 签名、来源清单、UID校验和会话外部 ID校验保持不变；没有 Hook或
  Hook 校验失败时 fail closed。
- `correlation_token_hash` 继续脱敏；只返回经过校验的真实外部会话 ID。
- 不改变数据库表结构；旧的 `activity_window` 历史记录不删除，列表接口
  不再把它们作为 Codex 真实会话展示。

## 回归测试

- 控制器被发现但未开始真实会话时，行为事件和推断会话生命周期均不上报；
- Hook 开始后，根进程、已存在后代和之后 fork 的进程都绑定真实会话；
- Hook 结束后，根进程及全部后代的后续行为均被丢弃；
- 工具 Hook 的外部会话 ID必须与当前活动真实会话一致，否则拒绝；
- 会话列表返回真实 `external_session_id`，不返回 correlation hash，也不展示
  `activity_window`；
- 真实会话 ID可用于页面选择，但全景查询仍使用服务端内部 UUID做边界校验。

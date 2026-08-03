# V6.2 Agent Guard 逻辑身份与运行状态修复

## 问题与成功标准

当前 Agent Guard 列表把已删除的历史应用资产和每个 `asset_id` 都作为独立
智能体返回；运行实例又可能没有 `asset_id`，导致同一主机上的同一产品同时出现
“静态资产已停止”和“无资产运行实例”两行。Agent 还存在配置副本、注册前后
HostID 不一致及 P4 Profile 真实证据缺失，最终表现为所有行
`unsupported_profile/stopped`。

修复后的验收标准：

1. 同一主机、同一规范化 Agent 类型只返回一个逻辑智能体；删除资产不参与列表。
2. `codex/openai_codex`、`claude-code/claude_code` 等别名归一，多个 controller
   仍作为同一逻辑智能体下的独立运行实例保留。
3. 无 `asset_id` 的确认运行实例可与同主机同类型静态资产合并，列表和 Scope
   查询均能看到真实运行状态。
4. Codex、OpenClaw、Hermes、Claude Code、OpenCode、Gemini CLI 的真实配置
   证据都能被 `/proc` 扫描器发现；仅进程名仍不得提升为 confirmed。
5. Agent 只使用 `/etc/aegis-agent/config.toml`，安装目录中的配置为指向该文件的
   兼容链接；注册返回的服务端 HostID 在 Guard 启动和 bundle 校验前生效。
6. 默认仍保持 monitor/enforcement/freeze 关闭；当前验证主机只显式开启
   Agent Guard 与行为监控，不开启阻断能力。

## 设计决策

### 逻辑智能体身份

外层列表身份从单纯 `host_id + asset_id` 收敛为
`host_id + canonical_agent_type`。列表保留一个最近活动的非删除资产 ID 作为兼容
定位信息，但签名 Scope 使用 host/type/profile，避免遗漏未绑定或绑定到历史资产的
运行实例。详情中的实例仍按 `PID + start_ticks` 唯一，不做实例级合并。

已知别名在 API 和 DC 使用同一组规范值：`codex`、`openclaw`、`hermes`、
`claude-code`、`opencode`、`gemini-cli`；未知产品保留规范化后的原值。

### 资产与运行实例

Agent 生命周期事件允许携带可选 `asset_id`。DC 在未携带时，按 host、规范化
Agent 类型和非删除资产选择最近活动资产进行安全补链；API 同时保留按
host/type 合并的兼容查询，以覆盖升级前的历史无资产实例。补链只写资产 UUID，
不改变静态资产内容或删除历史记录。

### 配置与 HostID

`/etc/aegis-agent/config.toml` 是唯一真实配置。Client 注册成功后同步服务端返回的
canonical HostID，并在打开命令流前触发 Guard 初始化；Guard 的 tracker、bundle
store 和事件 ID 均使用 canonical HostID。注册失败时 Guard 不启动，Client 按现有
退避策略重连。HostID 变化记录 INFO，不记录 token 或完整配置。

## 测试设计

- API repository：删除资产不出现；同类型多个活动/别名资产合并；assetless
  confirmed runtime 合并到资产逻辑行并显示 running；不同产品保持分行。
- Agent Profile：六种 Profile 使用真实扫描器可产生的配置 marker 达到 confirmed；
  无独立证据仍是 candidate。
- Agent Client/Manager：注册返回不同 HostID 时，Guard 启动前完成 rebind，bundle
  不再发生 host scope mismatch。
- DC：生命周期 payload 保留可选 asset ID；缺失时按非删除同类型资产补链；冲突
  upsert 可从 NULL 回填但不覆盖已有绑定。
- 安装脚本：两处生成器都写 `/etc` 并建立 `/opt` 兼容链接，默认开关仍为 false。
- 集成：当前主机开启 monitor-only 后出现 Codex confirmed/running，静态 Codex
  只显示一行，服务与事件链路保持健康。

## 安全、兼容与回滚

逻辑 Scope 仍绑定 host/type/profile 并经过 HMAC 签名，不放宽跨主机查询。
enforcement、freeze 和 action 默认不启用。回滚代码不会删除新增的运行数据；将
Agent Guard 开关恢复为 false 并重启 Agent 即可停止采集。旧 asset-id 深链继续
可用，但新列表优先使用逻辑 Scope。

# Agent Guard 运行时开关与智能体 Hook 注入

## 问题与范围

Agent Guard 的 `AgentGuardToolAdapterEnabled` 与
`AgentGuardSessionHookEnabled` 当前只在 Agent 启动时从本地 TOML 读取，页面无法在
不改配置文件的情况下切换，也无法从页面触发 Claude Code、OpenClaw、Hermes、Zcode
和 Codex 的 Hook 注册。

本次改动把这两个开关和 Hook 注入状态纳入 Agent Guard 控制面：设置保存到
`system_configs`，通过已有 Server/Agent 配置同步通道发送给目标 Agent；Agent 在本地
校验后动态应用，并由受控 Hook Provisioner/Remover 完成智能体配置注册与清理。页面展示
目标主机的当前开关和每个智能体的 Hook 开关，不再展示中间下发状态。

## 成功标准

- 页面“智能体事件感知与防护”增加“设置”入口，可以读取并保存两个运行时开关。
- 设置不修改 `/etc/aegis-agent/config.toml`，Agent 重启后仍以页面持久化设置为准。
- 任一开关变化立即保存并下发；开启的智能体由 Agent 执行幂等 Hook 注册并开始上报，
  关闭的智能体由 Agent 清理对应 Aegis Hook 配置并停止该智能体事件上报。
- 注入失败不能伪装为成功；页面恢复原开关并提示保存/下发失败。
- Hook 注入只合并和清理 Aegis 自己的配置项，不删除、不覆盖用户已有 Hook；不记录原始命令、
  Token、私钥或 Hook 载荷到日志。

## 目标数据流

```text
Frontend settings dialog
  -> api-server GET/PUT /agent-guard/runtime-settings
  -> system_configs (agent_guard.runtime.<host_id>)
  -> api-server -> Server SyncAgentConfig
  -> Agent ConfigManager
       -> Manager.ApplyRuntimeSettings
       -> HookProvisioner/Remover (selected products, idempotent)
       -> reload trusted source manifest / update runtime ingress
  -> Agent Guard status event -> PostgreSQL/WebSocket -> Frontend
```

设置以主机为粒度。读取时没有 `host_id` 返回默认值和在线主机汇总；保存时必须指定
目标主机，避免把一个主机的用户目录配置误注入到另一台主机。

## 协议与状态

新增配置类型 `agent_guard_runtime_settings`，Payload 使用版本化 JSON：

```json
{
  "schema": "aegis.agent_guard.runtime_settings.v1",
  "version": 1,
  "host_id": "<uuid>",
  "tool_adapter_enabled": true,
  "session_hook_enabled": true,
  "injections": [
    {"agent_type": "claude-code", "enabled": true},
    {"agent_type": "openclaw", "enabled": true},
    {"agent_type": "hermes", "enabled": false},
    {"agent_type": "zcode", "enabled": true},
    {"agent_type": "codex", "enabled": true}
  ]
}
```

Agent 只接受合法 Schema、目标 host_id 匹配本机身份且版本不回退的设置。两个开关
分别控制工具语义和会话生命周期；两者都关闭时停止 Hook ingress。智能体开关只控制对应
产品的 Hook 生命周期和事件上报，Hook 注册或清理失败时不影响 eBPF 行为采集。

## 安全、兼容与回滚

- 继续使用 Agent 已有的配置同步认证边界；Agent 端拒绝未知产品、相对路径、非法
  配置和 host scope 不匹配的 Payload。
- Hook Provisioner/Remover 使用发布目录中的固定 helper 和固定产品 Profile，采用现有安全
  文件写入与幂等合并/清理逻辑；不会执行页面传来的任意 shell 字符串。
- 关闭智能体开关会移除对应配置中的 Aegis Hook、从可信 source manifest 移除对应来源，
  并停止 Agent 端 Hook ingress；用户其他 Hook/Plugin 配置保持不变。
- 回滚设置可在页面关闭开关或取消对应注入项；Agent 重启不需要改本地 TOML。

## 验证

- API：设置默认值、合法/非法 Payload、保存与 Agent 离线/在线分支。
- Agent：配置应用、host/version 校验、开关动态生效、Provisioner 幂等与失败状态。
- Frontend：设置对话框读取、开关即时保存、失败回滚和无中间状态展示。
- 构建并做 Compose 健康检查，确认 Agent 配置文件没有因页面保存而被修改。

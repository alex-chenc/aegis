# Prompt 05: Agent 动态 eBPF 实现

你是 Aegis agent/eBPF 工程师。请实现 V5.8 agent 动态 DetectionPackage 能力。

## 参考文档

- `docs/aegis_system_design_v5.8/agent_dynamic_ebpf_design_v5.8.md`
- `docs/aegis_system_design_v5.8/code_interfaces_v5.8.md`
- `agent/internal/ebpf/loader.go`
- `agent/internal/sigma/`
- `agent/internal/client/client.go`

## 新增模块

```text
agent/internal/dynpkg/
agent/internal/ebpf/plugin/
agent/internal/correlation/
```

## 关键行为

1. agent 未收到 `dynamic_ebpf_hook_allowlist` 前，不加载任何动态 eBPF package。
2. 收到 `DetectionPackageCommand install` 后下载 package 和 `.sig`。
3. 使用内置 Ed25519 公钥验签整个 tar.gz。
4. 验签通过才解包。
5. 解析 package.yaml 和 plugin.yaml。
6. 校验所有 hooks 都在当前 allowlist。
7. 优先加载 ringbuf，失败 fallback perf。
8. 插件事件使用统一 `aegis_plugin_event` 信封和 TLV payload。
9. TLV 按 plugin manifest event_schema 解码为 event map。
10. event map 进入 package scoped Sigma。
11. Sigma 命中生成 AtomicFinding。
12. AtomicFinding 进入本地 Correlation Engine。
13. 命中后上报 correlation alert + evidence。

## 限制

- 动态插件加载失败只禁用该 package，agent 继续运行。
- 原始插件事件默认不上报服务端。
- 卸载 package 时删除本地 artifact。
- 插件持续超限时自动禁用。
- 第一版 correlation 只支持 ordered sequence + window + by。

## 事件结构

eBPF 公共头需要提供：

```c
struct aegis_plugin_event {
    __u64 timestamp_ns;
    __u32 plugin_id_hash;
    __u32 event_type;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __u32 payload_len;
    __u8  payload[256];
};
```

## 验收

- 未签名包拒绝安装。
- allowlist 不匹配包拒绝加载。
- ringbuf 不可用时能尝试 perf。
- 插件事件能解码成 event map。
- AtomicFinding 能进入 correlation。
- 命中 sequence 后上报告警和 evidence。
- 卸载能 detach links 并删除文件。


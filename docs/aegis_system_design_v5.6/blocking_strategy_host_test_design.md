# 告警后阻断策略主机验证设计

## 背景

本次验证目标是从“规则命中产生告警”继续验证到“阻断任务真实执行”。现有阻断策略表通过 `action` 表达处置动作，当前需要覆盖三种实际阻断动作：

- `kill_process`：终止进程。
- `quarantine_file`：隔离文件。
- `block_connection`：阻断网络目的地址。

## 专用测试规则

新增一条只用于主机验证的 Sigma 规则：

- 规则 ID：`aegis-block-strategy-host-test`
- MITRE：`T1059.004`
- 匹配条件：Linux 进程命令行包含 `aegis-block-strategy-test`
- 目的：用于端到端验证规则匹配、告警生成、阻断策略绑定。

该规则不作为默认生产规则加载，只作为测试用例和手工验证材料。

## 阻断目标解析

阻断命令必须带明确目标，三种动作的目标规则如下：

| action | target |
| --- | --- |
| `kill_process` | 告警 PID |
| `quarantine_file` | 告警 `command_line` 中保存的文件路径 |
| `block_connection` | 告警 `command_line` 中保存的远端 IP |

如果 `quarantine_file` 或 `block_connection` 缺少目标，阻断记录应失败并写入错误原因，避免把 PID 错发给文件或网络阻断器。

## 告警后阻断链路

自动阻断链路：

1. Agent 命中专用规则并上报事件。
2. Server/API 生成告警。
3. 查询 `block_policies`，当 `enabled=true && auto_block=true` 时创建阻断记录。
4. 通过 Server gRPC 向 Agent 下发 `BlockCommand`。
5. Agent 执行对应动作，阻断记录和告警 `block_status` 更新为成功或失败。

手动阻断链路：

1. 用户通过 `POST /api/v1/detection/alerts/:id/block` 指定 action。
2. API 根据告警解析目标。
3. 通过 Server gRPC 下发到 Agent。
4. Agent 执行动作，API 返回阻断记录。

## 测试策略

先补测试，再修代码：

- Agent 规则测试：加载专用 Sigma 规则，验证测试命令能命中，普通命令不命中。
- Agent 主机阻断测试：真实执行三种动作。
  - `kill_process`：启动 `sleep`，执行后进程应退出。
  - `quarantine_file`：创建临时文件，执行后原路径消失，隔离目录出现文件。
  - `block_connection`：对 TEST-NET 地址追加 iptables DROP 规则，验证规则存在，测试结束清理。
- Server 自动阻断测试：`checkAutoActions` 对三种 action 都应向已连接 Agent stream 发送 `BlockCommand`。
- API 告警阻断测试：三种 action 的目标解析正确，缺少目标时失败且不下发错误目标。

## 成功标准

- 三种动作的专用测试全部真实通过，无跳过冒充通过。
- 服务重建后健康检查通过。
- 使用 curl 验证阻断策略接口和告警阻断接口返回真实结果。
- 主机 iptables 测试结束后清理新增规则。

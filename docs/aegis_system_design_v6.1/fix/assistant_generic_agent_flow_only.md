# 智能体模式统一通用流程改造

## 1. 问题与目标

当前智能体模式存在两类绕过通用 Runtime 的固定业务流程：

1. Orchestrator 根据“资产采集、漏洞扫描、基线扫描、异常检测、CVE 修复”等关键词，直接进入硬编码快捷函数并调用工具。
2. Tool Gateway 在一次工具调用成功后，根据用户文本暗中补跑资产、漏洞脚本、检测、安装包或基线固定序列。

这会造成实际执行链与意图拆解、工具映射、工具裁决和 Runtime 计划不一致，也会让会话提前结束、工具不可追踪或参数追问错误。

本次目标是删除上述固定执行链，所有请求统一经过：

```text
IntentRouter
  -> IntentDecomposer
  -> LLM Tool Selection
  -> ToolDecisionEngine
  -> RuntimeFactory.Build
  -> agent-runtime Plan / React
  -> ToolGateway 单次工具调用
```

## 2. 设计约束

- Orchestrator 不再识别或执行 natural-operation shortcut。
- Tool Gateway 只执行 Runtime 当前明确发出的工具调用，不自动补跑其他工具。
- 不允许工具选择、意图拆解或 Runtime 失败后降级到规则快捷执行；错误直接返回。
- `Tool.Search` 不作为业务工具不足时的自动兜底，只有用户明确要求调用时才允许出现。
- 通用工具裁决以 `IntentBreakdown.CandidateCapabilities` 映射结果为主，不把粗召回的全部 `CandidateTools` 直接当作可执行候选。
- 业务多步骤需求通过能力映射、工具契约和 Runtime 动态规划实现；后端只提供允许工具集合、依赖顺序和可确定参数，不代替 Runtime 执行固定步骤。

## 3. 通用规划修正

### 3.1 漏洞扫描

当意图包含漏洞扫描时，候选能力应包含：

- `list_hosts`
- `start_vulnerability_scan`
- `get_vulnerability_scan_status`

“在线主机”范围由 `Host.List(status=online)` 提供，`Vulnerability.Scan.Start.host_ids` 由前一步结果动态绑定，不要求用户提供 task ID。

### 3.2 CVE POC / 修复

通用候选能力包含：

- `list_vulnerabilities`
- `get_vulnerability_affected_hosts`
- `generate_vulnerability_script`
- `get_vulnerability_script_status`
- `execute_vulnerability_host_scripts`

CVE ID 从用户文本绑定；`vulnerability_id`、`host_ids` 允许由前序工具结果提供；`script_type` 由 Runtime 根据用户明确要求分别规划 `poc` 和 `fix` 调用；`max_rounds` 从用户约束传给执行工具。Gateway 不再自动生成或下发第二类脚本。

### 3.3 候选工具收敛

ToolDecisionEngine 只召回以下来源：

- 意图能力到工具的映射；
- LLM 明确选择且与候选能力一致的工具；
- 用户明确写出的工具名；
- 通用工作流依赖（例如扫描前的主机列表、扫描后的状态查询）。

不得把粗选阶段的全部 `CandidateTools` 直接加入执行计划，以避免“漏洞扫描任务”误选 `Task.GetDetail` 并询问 task ID。

## 4. 测试用例

1. “进行资产采集”“进行基线扫描”等自然语言操作仍启用 LLM 工具选择，不存在 shortcut bypass。
2. Tool Gateway 调用一个工具时只产生一次该工具调用，不自动调用同一业务序列中的其他工具。
3. “帮我进行一次在线主机的漏洞扫描任务”计划包含 `Host.List`、`Vulnerability.Scan.Start`、`Vulnerability.Scan.Status`，不包含 `Task.GetDetail`，且不询问任务 ID。
4. CVE POC / 修复请求计划包含漏洞查询、受影响主机、脚本生成/状态/执行工具，并正确保留最大修复轮数约束。
5. 意图、选工具、裁决或 Runtime 出错时直接返回错误，不进入任何固定或降级流程。

## 5. 日志与验证

- Orchestrator 记录请求进入统一通用 Runtime 的 session、run、selection mode 和最终工具集合。
- 执行 assistant 包定向测试和 api-server 构建。
- 重建并重启 `api-server`，检查健康状态。
- 通过真实 Assistant API 创建会话并验证元数据、工具裁决记录和事件中不存在 shortcut/自动补跑标记。

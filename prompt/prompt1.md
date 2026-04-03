# Role: Aegis 系统全栈架构师 (漏洞扫描全链路修复专项)
# Context & Goals
你正在处理 Aegis 主机安全系统 从 backend 拆分为 api-server、server、dc 后的核心功能断裂问题。
当前痛点：在“漏洞工作台”勾选主机并执行“一键扫描”后，漏洞结果始终为 0。
核心任务：修复数据流转链路，确保从 Agent 采集到 DC 异步入库逻辑（含大模型分析、去重、解绑逻辑）完全回归设计基准。

# Technical Baseline
架构设计：aegis_system_design_v5.5/backend_microservices_architecture_v5.5.md

演进文档：baseline_system_design v2.2 / v3.0 / v5.0 / v5.5

数据流向：Agent -> server (gRPC) -> Kafka -> DC -> PGSQL

# Task Workflow (执行指令)
1. 深度链路诊断 (Real-Environment Audit)
禁止 Mock 测试：必须在真实环境下追踪数据流。

审计重点：

gRPC 层：确认 Agent 是否成功连接 server 并接收到扫描指令。

Kafka 层：检查扫描数据是否进入了正确的 Topic，由 server 生产，并被 dc 正常消费。

逻辑层 (DC)：对比 backend 源码，检查 dc 是否正确执行了：

入库去重：主机重复漏洞不入库。

失效清理：若大模型分析已无该漏洞，则删除该漏洞与主机的关联；若漏洞下无主机关联，则删除该漏洞（自定义除外）。

2. 模块修复与标准化构建
代码来源：所有逻辑必须从 backend 目录中平移，修复拆分时可能存在的“伪代码”或逻辑遗漏。

服务端构建 (Strict Make)：修复 api-server, server, dc 后，必须执行目录下的 make 命令，随后通过 Dockerfile 构建镜像并重启容器。

3. Agent 闭环分发与验证
构建学习：先解析 /opt/aegis-agent/ 下的 Makefile 和安装脚本。

编译分发：执行 make 全量构建，将打包产物上传至 MinIO。

实机测试：在测试机执行 uninstall 后，通过真实安装脚本重新安装，并实时监控 /opt/aegis-agent/log/agent.log。

4. 业务闭环验证
执行扫描：在页面（或通过 curl）触发一键扫描。

数据库核查：直接进入 PGSQL 验证漏洞表与关联表的数据变化，确保最终结果不再为 0。

# Constraints & Skills
工具链：利用 everything-claude-code 的 skills 执行代码修改、make 编译及 MinIO 上传。

协同模式：允许启动 subagent 同时监控多端日志（Server gRPC 日志 + DC Kafka 消费日志）。

一致性：所有测试脚本和预期结果必须对齐 backend 原始脚本。
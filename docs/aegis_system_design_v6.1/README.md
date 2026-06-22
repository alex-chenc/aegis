# Aegis V6.1 设计文档

**版本**: 6.1
**状态**: 方案设计中
**主题**: 弱密码检测能力

---

## 文档索引

| 文档 | 说明 |
|:---|:---|
| `weak_password_detection_design_v6.1.md` | V6.1 AI 原生弱密码检测能力设计，覆盖 AI 编排、凭据来源、弱口令匹配、API、数据库、智能体工具、安全边界、测试和落地计划 |
| `weak_password_frontend_prd_v6.1.md` | 弱密码检测前端页面 PRD，覆盖应用资产一键分析、单应用检查、进度条、字典管理、AI 生成字典、结果和失败态 |
| `weak_password_database_design_v6.1.md` | 弱密码检测数据库设计，覆盖任务、应用分析、采集计划、Agent 工具调用、字典、匹配结果、错误和审计表 |
| `weak_password_api_server_design_v6.1.md` | 弱密码检测 api-server 设计，覆盖 HTTP API、任务编排、LLM 规划、Agent 下发、匹配、字典和错误处理 |
| `weak_password_agent_program_design_v6.1.md` | 弱密码检测 Agent 程序设计，覆盖工具入口、采集模型、parser、辅助定位工具、安全限制和测试 |
| `weak_password_development_prompt_v6.1.md` | 弱密码检测开发提示词，可复制给开发智能体或工程师，按设计文档实施数据库、Agent、api-server、前端和测试 |

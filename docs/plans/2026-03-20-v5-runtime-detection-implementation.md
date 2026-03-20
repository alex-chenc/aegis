# V5.0 智能异常检测实现计划

**日期**: 2026-03-20
**状态**: 执行中

---

## Phase 0: 基础设施（Proto + DB Migration）

### 0.1 扩展 Proto 定义
**文件**: `proto/agent_comm.proto`
**依赖**: 无

新增 RPC 和 Message：
- `ReportEvent(ReportEventRequest) returns (ReportEventResponse)` - 事件上报
- `ExecuteTool(ToolRequest) returns (ToolResponse)` - 工具调用
- `UpdateRules(RuleUpdateRequest) returns (RuleUpdateResponse)` - 规则下发
- `ExecuteBlockCommand(BlockCommand) returns (BlockResponse)` - 阻断指令下发

### 0.2 生成 Proto Go 代码
**命令**: `protoc --go_out=... --go-grpc_out=... proto/agent_comm.proto`

### 0.3 创建 DB Migration
**文件**: `backend/migrations/005_v5_detection_tables.sql`
**表**: alerts, block_records, block_policies, sigma_rules, sigma_rule_versions, tool_calls

---

## Phase 1: Agent 模块

### 1.1 扩展 Config
**文件**: `agent/internal/config/config.go`
**改动**: 新增字段 EventBufferSize, RuleDir, QuarantineDir

### 1.2 创建 Sigma 规则模块
**文件**: `agent/internal/sigma/{parser,matcher,index,loader}.go`
**功能**: Sigma YAML 解析、按 logsource 索引、事件匹配

### 1.3 创建事件采集模块
**文件**: `agent/internal/event/{collector,ebpf}.go`
**功能**: eBPF 事件采集（cilium/ebpf）

### 1.4 创建阻断模块
**文件**: `agent/internal/blocker/{blocker,process,file}.go`
**功能**: kill_process、quarantine_file

### 1.5 创建工具模块
**文件**: `agent/internal/tools/{manager,process,network,file,user,command}.go`
**功能**: get_process_tree, get_network_connections, get_file_info, get_user_info, execute_command

### 1.6 扩展 Client
**文件**: `agent/internal/client/client.go`
**改动**: 新增 ReportEvent, HandleToolCall, HandleRuleUpdate, HandleBlockCommand

### 1.7 更新 Main
**文件**: `agent/cmd/agent/main.go`
**改动**: 初始化新模块，启动事件处理循环

---

## Phase 2: Backend 模块

### 2.1 创建 Model
**文件**: `backend/internal/model/{alert,block_record,block_policy,sigma_rule,tool_call}.go`

### 2.2 创建 Repository
**文件**: `backend/internal/repository/{alert_repo,block_repo,block_policy_repo,sigma_rule_repo,tool_call_repo}.go`

### 2.3 创建 Service
**文件**: `backend/internal/service/{detection_pipeline_service,llm_analysis_service,alert_service,block_service,sigma_rule_service}.go`

### 2.4 创建 Handler
**文件**: `backend/internal/api/handler/detection_handler.go`

### 2.5 更新 Router
**文件**: `backend/internal/api/router.go`
**改动**: 注册 /detection/* 路由

### 2.6 更新 gRPC Server
**文件**: `backend/internal/grpc_server/server.go`
**改动**: 实现 ReportEvent, ExecuteTool, UpdateRules, ExecuteBlockCommand

### 2.7 更新 Main
**文件**: `backend/cmd/server/main.go`
**改动**: 初始化新模块，启动 Kafka consumer

### 2.8 创建 DB Migration
**文件**: `backend/migrations/005_v5_detection_tables.sql`

---

## Phase 3: Frontend 模块

### 3.1 创建类型定义
**文件**: `frontend/src/types/index.ts`
**改动**: 新增 Alert, BlockPolicy, SigmaRule 等类型

### 3.2 创建 API 层
**文件**: `frontend/src/api/detection.ts`

### 3.3 创建 Store
**文件**: `frontend/src/store/detection.ts`

### 3.4 创建组件
**文件**: `frontend/src/components/detection/*.vue`
**组件**: ThreatStatCards, AttackMatrix, AlertTrendChart, AlertCard, RuleCard

### 3.5 创建页面
**文件**: `frontend/src/views/detection/{Overview,Alerts,Policies,Rules}.vue`

### 3.6 更新路由
**文件**: `frontend/src/router/index.ts`

### 3.7 更新侧边栏
**文件**: `frontend/src/App.vue`

---

## Phase 4: 构建部署

### 4.1 更新 Dockerfile
### 4.2 更新 docker-compose.yml
### 4.3 构建镜像
### 4.4 启动服务
### 4.5 curl 测试所有接口

---


# V5.8 智能资产采集与漏洞扫描策略改造 - 实现设计文档

**版本**: 5.8
**日期**: 2026-06-04
**状态**: 待确认

---

## 1. 问题陈述

当前系统存在以下问题：
- 主机列表只展示基本心跳信息，无法展示软件包和运行应用
- 漏洞扫描每次都要调用 Agent 实时采集软件包，浪费资源
- 缺乏进程级别的应用识别和分类能力
- LLM 可能编造不存在的 CVE 编号

## 2. 实现范围

基于 PRD 和各模块设计文档，实现以下功能：

### 2.1 数据库层
- 新增 7 张表：asset_collection_configs, asset_collection_tasks, asset_collection_task_hosts, host_software_assets, host_process_snapshots, host_application_assets, host_application_tool_calls
- 扩展 host_vulnerabilities 表添加资产引用字段
- 迁移文件：migrations/015_v5.8_intelligent_asset_collection.sql

### 2.2 Agent 层
- 新增 agent/internal/assets/ 模块
- 实现 rpm/dpkg/apk 三类包管理器采集器
- 实现 /proc 进程快照采集器
- 实现 4 个只读工具：AssetGetProcessVersion, AssetReadConfigSummary, AssetListDirectoryHints, AssetResolvePackageByFile
- cmdline 脱敏处理

### 2.3 Proto 层
- 扩展 proto/agent_comm.proto 新增 CollectHostAssets RPC
- 扩展 proto/api_server_comm.proto 新增 CollectHostAssets 转发

### 2.4 Server 层
- 实现 CollectHostAssets gRPC 方法转发到 Agent

### 2.5 api-server 层
- 新增 repository: asset_collection_repo.go, host_software_repo.go, host_application_repo.go, host_process_snapshot_repo.go
- 新增 service: asset_collection_service.go, asset_analysis_service.go, asset_query_service.go
- 新增 handler: asset_handler.go
- 新增 API: /api/v1/host-assets/*
- 实现周期采集 worker
- 实现 LLM 应用识别编排
- 改造漏洞扫描服务读取资产库
- 新增漏洞真实性校验器

### 2.6 前端层
- 新增路由和页面：概览、软件清单、应用资产、数据库、Web 服务、Web 框架、Web 站点、采集任务
- 新增 API 模块：frontend/src/api/assets.ts
- 新增 Pinia store：frontend/src/store/assets.ts
- 改造漏洞扫描页面文案

---

## 3. 实现计划

### 阶段 1：数据库迁移
1. 创建 migrations/015_v5.8_intelligent_asset_collection.sql
2. 在 repository/db.go 添加 AutoMigrate 支持

### 阶段 2：Agent 资产采集模块
1. 创建 agent/internal/assets/ 目录和基础模型
2. 实现 rpm_collector.go, dpkg_collector.go, apk_collector.go
3. 实现 process_collector.go
4. 实现 redactor.go (cmdline 脱敏)
5. 实现 version_tools.go (只读工具)
6. 注册工具到 tool_manager.go

### 阶段 3：Proto 和 Server 扩展
1. 扩展 proto/agent_comm.proto
2. 扩展 proto/api_server_comm.proto
3. 重新生成 proto 代码
4. 实现 Server 端 CollectHostAssets 转发

### 阶段 4：api-server 资产模块
1. 创建 model 定义
2. 创建 repository 层
3. 创建 service 层
4. 创建 handler 层
5. 注册路由
6. 实现周期采集 worker

### 阶段 5：LLM 应用识别
1. 编写应用识别 prompt
2. 实现工具调用编排
3. 实现输出校验

### 阶段 6：漏洞扫描改造
1. 修改 vulnerability_service.go 读取资产库
2. 实现 vulnerability_match_validator.go
3. 更新 LLM prompt

### 阶段 7：前端实现
1. 创建 API 模块
2. 创建 Pinia store
3. 实现路由和页面组件
4. 改造漏洞扫描页面

---

## 4. 详细设计

### 4.1 数据库模型

#### asset_collection_configs
```sql
CREATE TABLE IF NOT EXISTS asset_collection_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enabled BOOLEAN NOT NULL DEFAULT true,
    interval_hours INT NOT NULL DEFAULT 12,
    collect_types JSONB NOT NULL DEFAULT '["software","process","application_analysis"]',
    scope VARCHAR(32) NOT NULL DEFAULT 'all_hosts',
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    updated_by VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_asset_collection_config_interval CHECK (interval_hours >= 1 AND interval_hours <= 168),
    CONSTRAINT chk_asset_collection_config_scope CHECK (scope IN ('all_hosts','host_group','hosts'))
);
```

#### host_software_assets
```sql
CREATE TABLE IF NOT EXISTS host_software_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname VARCHAR(255),
    ip_address VARCHAR(45),
    group_name VARCHAR(255) NOT NULL DEFAULT '默认分组',
    os_type VARCHAR(50) NOT NULL,
    package_manager VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(255),
    release VARCHAR(255),
    epoch VARCHAR(64),
    architecture VARCHAR(64),
    source_name VARCHAR(255),
    vendor VARCHAR(255),
    license VARCHAR(255),
    install_paths JSONB NOT NULL DEFAULT '[]',
    file_count INT NOT NULL DEFAULT 0,
    package_metadata JSONB NOT NULL DEFAULT '{}',
    fingerprint VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    last_modified_at TIMESTAMPTZ,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(host_id, package_manager, fingerprint)
);
```

#### host_application_assets
```sql
CREATE TABLE IF NOT EXISTS host_application_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    hostname VARCHAR(255),
    ip_address VARCHAR(45),
    group_name VARCHAR(255) NOT NULL DEFAULT '默认分组',
    os_type VARCHAR(50) NOT NULL,
    category VARCHAR(32) NOT NULL DEFAULT 'unknown',
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    version VARCHAR(255),
    version_source VARCHAR(64),
    install_path TEXT,
    start_path TEXT,
    config_paths JSONB NOT NULL DEFAULT '[]',
    site_paths JSONB NOT NULL DEFAULT '[]',
    domains JSONB NOT NULL DEFAULT '[]',
    listen_ports JSONB NOT NULL DEFAULT '[]',
    run_user VARCHAR(255),
    runtime_name VARCHAR(100),
    runtime_version VARCHAR(100),
    framework_name VARCHAR(100),
    framework_version VARCHAR(100),
    related_pids JSONB NOT NULL DEFAULT '[]',
    related_packages JSONB NOT NULL DEFAULT '[]',
    ai_confidence NUMERIC(4,3) NOT NULL DEFAULT 0,
    ai_evidence JSONB NOT NULL DEFAULT '[]',
    ai_raw_output JSONB NOT NULL DEFAULT '{}',
    manual_overrides JSONB NOT NULL DEFAULT '{}',
    review_status VARCHAR(32) NOT NULL DEFAULT 'auto',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    fingerprint VARCHAR(128) NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(host_id, fingerprint)
);
```

### 4.2 Agent 模块设计

#### 目录结构
```
agent/internal/assets/
├── collector.go          # 主采集器协调
├── model.go              # 数据模型定义
├── package_collector.go  # 包管理器探测和分发
├── rpm_collector.go      # RPM 包采集
├── dpkg_collector.go     # DPKG 包采集
├── apk_collector.go      # APK 包采集
├── process_collector.go  # 进程快照采集
├── redactor.go           # cmdline 脱敏
└── version_tools.go      # 版本工具实现
```

#### 核心接口
```go
// collector.go
type AssetCollector struct {
    packageCollector *PackageCollector
    processCollector *ProcessCollector
}

func (c *AssetCollector) Collect(ctx context.Context, opts CollectOptions) (*HostAssetSnapshot, error)

// package_collector.go
type PackageCollector struct {
    rpm  *RPMCollector
    dpkg *DPKGCollector
    apk  *APKCollector
}

func (c *PackageCollector) Collect(ctx context.Context) ([]PackageAsset, error)

// process_collector.go
type ProcessCollector struct {
    maxProcessCount int
}

func (c *ProcessCollector) Collect(ctx context.Context) ([]ProcessAsset, error)
```

### 4.3 API 设计

#### 资产概览
```
GET /api/v1/host-assets/summary
Response: { software_count, application_count, database_count, web_service_count, ... }
```

#### 软件列表
```
GET /api/v1/host-assets/software?page=1&page_size=20&keyword=&host_id=&package_manager=
Response: { items: SoftwareAsset[], total: int }
```

#### 应用列表
```
GET /api/v1/host-assets/applications?page=1&page_size=20&category=&keyword=&host_id=
Response: { items: ApplicationAsset[], total: int }
```

#### 触发采集
```
POST /api/v1/host-assets/collections
Body: { scope: "hosts", host_ids: ["uuid"], types: ["software", "process", "application_analysis"] }
Response: { task_id: "uuid", status: "pending" }
```

#### 周期配置
```
GET /api/v1/host-assets/collection-config
PUT /api/v1/host-assets/collection-config
Body: { enabled: true, interval_hours: 12, collect_types: [...] }
```

### 4.4 LLM 应用识别设计

#### Prompt 结构
```
系统提示：你是主机应用识别专家...
输入：主机信息、进程列表、软件包列表
输出：JSON 格式的应用列表
约束：只允许调用白名单工具获取版本
```

#### 工具调用流程
1. LLM 分析进程快照
2. 需要版本时调用 AssetGetProcessVersion
3. 需要配置时调用 AssetReadConfigSummary
4. 合并工具结果到应用资产

### 4.5 漏洞扫描改造

#### 改造点
1. 移除 vulnerability_service.go 中的 CollectSoftware 调用
2. 改为查询 host_software_assets 和 host_application_assets
3. 新增 vulnerability_match_validator.go 校验漏洞真实性
4. 更新 LLM prompt 要求真实来源

#### 校验器逻辑
```go
func ValidateMatch(match LLMVulnerabilityMatch, assets AssetContext) ValidationResult {
    if !assets.Contains(match.AssetID) {
        return Reject("asset_id not in scan context")
    }
    if !HasKnownVulnerabilityID(match.VulnerabilityID) {
        return Reject("missing vulnerability id")
    }
    if !HasTrustedSource(match.Source) {
        return Reject("missing trusted source")
    }
    return Verify()
}
```

---

## 5. 文件变更清单

### 5.1 新增文件

#### 数据库
- migrations/015_v5.8_intelligent_asset_collection.sql

#### Agent
- agent/internal/assets/collector.go
- agent/internal/assets/model.go
- agent/internal/assets/package_collector.go
- agent/internal/assets/rpm_collector.go
- agent/internal/assets/dpkg_collector.go
- agent/internal/assets/apk_collector.go
- agent/internal/assets/process_collector.go
- agent/internal/assets/redactor.go
- agent/internal/assets/version_tools.go
- agent/internal/assets/collector_test.go
- agent/internal/assets/redactor_test.go

#### Proto
- (修改) proto/agent_comm.proto
- (修改) proto/api_server_comm.proto

#### api-server Model
- api-server/internal/model/asset_collection.go

#### api-server Repository
- api-server/internal/repository/asset_collection_repo.go
- api-server/internal/repository/host_software_repo.go
- api-server/internal/repository/host_application_repo.go
- api-server/internal/repository/host_process_snapshot_repo.go

#### api-server Service
- api-server/internal/service/asset_collection_service.go
- api-server/internal/service/asset_analysis_service.go
- api-server/internal/service/asset_query_service.go
- api-server/internal/service/vulnerability_match_validator.go

#### api-server Handler
- api-server/internal/api/handler/asset_handler.go

#### api-server LLM
- (修改) api-server/internal/llm/prompts.go

#### Frontend API
- frontend/src/api/assets.ts

#### Frontend Store
- frontend/src/store/assets.ts

#### Frontend Views
- frontend/src/views/hosts/Assets/Overview.vue
- frontend/src/views/hosts/Assets/Software.vue
- frontend/src/views/hosts/Assets/Applications.vue
- frontend/src/views/hosts/Assets/Databases.vue
- frontend/src/views/hosts/Assets/WebServices.vue
- frontend/src/views/hosts/Assets/WebFrameworks.vue
- frontend/src/views/hosts/Assets/WebSites.vue
- frontend/src/views/hosts/Assets/Collections.vue
- frontend/src/views/hosts/Assets/components/CollectionConfigDrawer.vue
- frontend/src/views/hosts/Assets/components/AssetFilterBar.vue
- frontend/src/views/hosts/Assets/composables/useAssets.ts

### 5.2 修改文件

#### api-server
- api-server/cmd/main.go (添加依赖注入)
- api-server/internal/api/router.go (注册新路由)
- api-server/internal/repository/db.go (AutoMigrate)
- api-server/internal/service/vulnerability_service.go (改造扫描逻辑)
- api-server/internal/grpc/client.go (添加 CollectHostAssets 方法)

#### Server
- server/internal/grpc_server/server.go (添加 CollectHostAssets 实现)
- server/internal/grpc_server/api_server_impl.go (添加转发)

#### Agent
- agent/cmd/agent/main.go (初始化资产采集器)
- agent/internal/tools/tool_manager.go (注册新工具)

#### Frontend
- frontend/src/router/index.ts (添加新路由)
- frontend/src/App.vue (添加侧边栏菜单)
- frontend/src/views/vulnerability/Workbench.vue (改造扫描文案)

---

## 6. 测试策略

### 6.1 单元测试
- Agent: rpm/dpkg/apk 解析器、cmdline 脱敏、版本工具
- api-server: repository CRUD、service 业务逻辑、handler 参数校验
- 前端: API 类型、store 状态管理、组件渲染

### 6.2 集成测试
- gRPC 端到端：api-server -> server -> agent
- 数据库集成：repository 层实际数据库操作
- LLM 集成：应用识别端到端流程

### 6.3 验收测试
- Ubuntu 主机采集 dpkg 软件包
- CentOS/Rocky 主机采集 rpm 软件包
- Alpine 主机采集 apk 软件包
- Nginx/MariaDB/Java 进程识别正确分类
- 周期采集配置默认 12 小时
- 漏洞扫描不调用 Agent 采集
- LLM 编造 CVE 被拒绝

---

## 7. 风险和回滚

### 7.1 风险
- Agent 在不同 Linux 发行版上的包数据库路径差异
- /proc 文件系统权限问题
- LLM 应用识别准确率
- 大规模主机采集的性能

### 7.2 回滚方案
- 数据库迁移可保留新表，不影响旧模块
- 前端菜单可隐藏智能资产采集入口
- 周期任务配置关闭后不再触发采集
- 漏洞扫描可临时回退到旧扫描流程

---

## 8. 确认事项

请确认以下实现决策：

1. **数据库迁移**：按照设计文档创建 7 张新表 + 扩展 host_vulnerabilities，是否同意？
2. **Agent 模块**：新增 agent/internal/assets/ 目录，是否同意？
3. **Proto 扩展**：新增 CollectHostAssets RPC，保留旧 CollectSoftware 兼容，是否同意？
4. **API 设计**：按照设计文档实现 /api/v1/host-assets/* 系列 API，是否同意？
5. **前端路由**：在主机列表下新增智能资产采集子菜单，是否同意？
6. **漏洞扫描改造**：移除 Agent 采集调用，改为读取资产库，是否同意？
7. **实现顺序**：按阶段 1-7 依次实现，是否同意？

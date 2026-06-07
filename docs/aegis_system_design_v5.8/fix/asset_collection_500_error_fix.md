# Bug Fix: 智能资产采集报服务器内部错误 (500)

## Bug 描述与症状

**症状**: 用户访问智能资产采集相关页面或触发采集操作时，前端显示"服务器内部错误"，后端返回 HTTP 500。

**影响范围**: 所有智能资产采集 API 端点均受影响：
- `GET /api/v1/host-assets/summary` — 资产概览
- `GET /api/v1/host-assets/software` — 软件清单
- `GET /api/v1/host-assets/applications` — 应用资产列表
- `GET /api/v1/host-assets/applications/:id` — 应用详情
- `PUT /api/v1/host-assets/applications/:id/review` — 人工复核
- `POST /api/v1/host-assets/collections` — 触发采集
- `GET /api/v1/host-assets/collections` — 采集任务列表
- `GET /api/v1/host-assets/collections/:id` — 采集任务详情
- `POST /api/v1/host-assets/collections/:id/retry` — 重试采集
- `POST /api/v1/host-assets/collections/:id/cancel` — 取消采集
- `GET /api/v1/host-assets/collection-config` — 获取采集配置
- `PUT /api/v1/host-assets/collection-config` — 更新采集配置

## 复现步骤

1. 使用 `docker compose up -d --build` 启动全栈（非首次初始化，或仅挂载了 `001_init.sql`）
2. 登录前端，导航到"智能资产采集"页面
3. 页面加载时调用 `GET /api/v1/host-assets/summary` 返回 500
4. 尝试触发采集 `POST /api/v1/host-assets/collections` 返回 500

## 根因分析

### 直接原因

V5.8 智能资产采集功能依赖 7 张数据库表，但这些表在开发环境中从未被创建。

### 原因链

1. **Docker Compose 只挂载了初始迁移**: `docker-compose.yml` 中 postgres 容器仅挂载 `./migrations/001_init.sql:/docker-entrypoint-initdb.d/01-init.sql:ro`，`002` 到 `015` 的迁移文件均未挂载。

2. **AutoMigrate 未包含 V5.8 模型**: `api-server/internal/repository/db.go` 的 `db.AutoMigrate()` 调用中未包含以下模型：
   - `AssetCollectionConfig`
   - `AssetCollectionTask`
   - `AssetCollectionTaskHost`
   - `HostSoftwareAsset`
   - `HostProcessSnapshot`
   - `HostApplicationAsset`
   - `HostApplicationToolCall`

3. **缺少 ensureSchema 函数**: 项目中存在 `ensureDetectionEnhancementSchema`、`ensureSigmaRuleEnhancementSchema`、`ensureAIAnalysisTraceSchema` 等函数用于补充 schema，但没有对应的 `ensureAssetCollectionSchema` 函数。

4. **代码已在使用这些表**: `api-server/cmd/main.go` 第 159 行初始化了 `assetCollectionRepository`，第 275-279 行组装了 service 和 handler，这些代码在运行时会查询不存在的表。

### 表不存在导致的错误

当 GORM 尝试查询不存在的表时，PostgreSQL 返回 `ERROR: relation "asset_collection_xxx" does not exist`，GORM 将此作为 error 返回，Handler 层统一返回 HTTP 500。

## 修复设计

### 修复方案

在 `api-server/internal/repository/db.go` 中新增 `ensureAssetCollectionSchema` 函数，使用 `CREATE TABLE IF NOT EXISTS` 语句创建 V5.8 资产采集所需的全部 7 张表及其索引，并在 `NewDB` 函数中调用该函数。

### 为什么选择 ensureSchema 而非 AutoMigrate

1. **迁移文件包含复杂约束**: `015_v5.8_intelligent_asset_collection.sql` 包含 CHECK 约束、UNIQUE 约束、外键约束和 GIN 索引，GORM AutoMigrate 对这些支持不完善。
2. **与现有模式一致**: 项目中已有 3 个 `ensure*Schema` 函数使用 raw SQL 处理复杂 schema 变更。
3. **幂等性**: `CREATE TABLE IF NOT EXISTS` 和 `CREATE INDEX IF NOT EXISTS` 保证重复执行不会报错。
4. **兼容升级**: 使用 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 处理已有表的列补充。

### 涉及的表和约束

| 表名 | 主要约束 | 索引 |
|------|---------|------|
| `asset_collection_configs` | CHECK (interval_hours, scope) | next_run |
| `asset_collection_tasks` | CHECK (status) | status, created_at, host_filter (GIN) |
| `asset_collection_task_hosts` | UNIQUE(task_id, host_id), FK → tasks, hosts | task, host, status |
| `host_software_assets` | CHECK (package_manager, status), UNIQUE(host_id, package_manager, fingerprint) | host, name, version, manager, seen, paths (GIN) |
| `host_process_snapshots` | FK → tasks, hosts | host+collected_at, task, hash |
| `host_application_assets` | CHECK (category, review_status, status), UNIQUE(host_id, fingerprint) | host, category, name, version, ports (GIN), review, seen |
| `host_application_tool_calls` | UNIQUE(call_id), FK → tasks, assets, hosts | app, host+created_at, tool |

### host_vulnerabilities 表扩展

迁移文件还包含对 `host_vulnerabilities` 表的 ALTER 操作，添加资产引用字段。这些也需要在 ensureSchema 中处理。

## 代码变更

### 文件: `api-server/internal/repository/db.go`

1. 新增 `ensureAssetCollectionSchema(db *gorm.DB) error` 函数
2. 新增 `assetCollectionSchemaStatements() []string` 函数（返回所有 SQL 语句）
3. 在 `NewDB` 函数中 `ensureAIAnalysisTraceSchema` 之后调用 `ensureAssetCollectionSchema`

### SQL 语句内容

基于 `migrations/015_v5.8_intelligent_asset_collection.sql` 的内容，转换为幂等的 `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS` + `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 语句。

### 代码审查修复

代码审查发现 `host_vulnerabilities` 表新增的 `software_asset_id` 和 `application_asset_id` 列缺少外键约束。原迁移文件中这两列有 `REFERENCES ... ON DELETE SET NULL`，但初版实现遗漏了。已补充两个 `DO $$ ... ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY` 块。

## 验证步骤

1. 重启 api-server 容器：`docker compose up -d --build api-server`
2. 检查日志确认无报错：`docker compose logs api-server | grep -i "asset collection"`
3. 验证表已创建：连接 postgres 查询 `SELECT table_name FROM information_schema.tables WHERE table_name LIKE 'asset%' OR table_name LIKE 'host_software%' OR table_name LIKE 'host_process%' OR table_name LIKE 'host_application%';`
4. 测试 API 端点：`curl http://localhost:8082/api/v1/host-assets/summary`
5. 测试触发采集：`curl -X POST http://localhost:8082/api/v1/host-assets/collections -H 'Content-Type: application/json' -d '{"scope":"all_hosts","types":["process"]}'`

## 影响组件

| 组件 | 影响 |
|------|------|
| api-server | 修改 `db.go`，新增 schema 初始化 |
| postgres | 新增 7 张表和相关索引 |
| 前端 | 无需修改 |
| server | 无需修改 |
| agent | 无需修改 |

## 风险与回滚计划

### 风险

- **低风险**: 使用 `IF NOT EXISTS` 保证幂等，不会影响已有数据
- **低风险**: 不修改任何已有表结构（仅 ADD COLUMN IF NOT EXISTS）
- **无兼容性影响**: 新表独立存在，不影响现有功能

### 回滚

如果需要回滚，删除新增的表即可：
```sql
DROP TABLE IF EXISTS host_application_tool_calls CASCADE;
DROP TABLE IF EXISTS host_application_assets CASCADE;
DROP TABLE IF EXISTS host_process_snapshots CASCADE;
DROP TABLE IF EXISTS host_software_assets CASCADE;
DROP TABLE IF EXISTS asset_collection_task_hosts CASCADE;
DROP TABLE IF EXISTS asset_collection_tasks CASCADE;
DROP TABLE IF EXISTS asset_collection_configs CASCADE;
```

## 回归测试用例

### TC-1: API Server 启动成功
- **前置条件**: 数据库中不存在 V5.8 资产采集表
- **操作**: 启动 api-server
- **预期**: 启动成功，日志中包含 "database connected successfully"，无报错

### TC-2: 资产概览 API 返回正常
- **前置条件**: API Server 启动成功
- **操作**: `GET /api/v1/host-assets/summary`
- **预期**: 返回 200，`code: 0`，`data` 包含所有计数字段（初始值为 0）

### TC-3: 触发采集 API 返回正常
- **前置条件**: API Server 启动成功，至少有一个 Agent 在线
- **操作**: `POST /api/v1/host-assets/collections` with `{"scope":"all_hosts","types":["process"]}`
- **预期**: 返回 200，`code: 0`，`data` 包含 `task_id` 和 `status`

### TC-4: 采集任务列表 API 返回正常
- **前置条件**: API Server 启动成功
- **操作**: `GET /api/v1/host-assets/collections`
- **预期**: 返回 200，`code: 0`，`data` 包含 `items` 数组和 `total`

### TC-5: 重复启动不报错
- **前置条件**: 表已存在
- **操作**: 重启 api-server
- **预期**: 启动成功，无 "already exists" 错误

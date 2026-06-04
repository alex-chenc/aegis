# V5.8 前端设计: 智能资产采集

**版本**: 5.8  
**日期**: 2026-06-04  
**状态**: 设计中  

---

## 1. 目标

在主机列表下新增智能资产采集模块，提供软件包、数据库、Web 服务、Web 框架、Web 站点的分类列表和采集任务管理。页面风格延续当前 Vue 3 + Element Plus 的运营控制台形态，强调表格可读性、筛选效率和任务状态透明。

---

## 2. 路由与导航

新增路由：

| 路由 | 页面 | 组件建议 |
|:---|:---|:---|
| `/hosts/assets` | 智能资产概览 | `views/hosts/Assets/Overview.vue` |
| `/hosts/assets/software` | 软件清单 | `views/hosts/Assets/Software.vue` |
| `/hosts/assets/applications` | 应用资产 | `views/hosts/Assets/Applications.vue` |
| `/hosts/assets/databases` | 数据库 | `views/hosts/Assets/Databases.vue` |
| `/hosts/assets/web-services` | Web 服务 | `views/hosts/Assets/WebServices.vue` |
| `/hosts/assets/web-frameworks` | Web 框架 | `views/hosts/Assets/WebFrameworks.vue` |
| `/hosts/assets/web-sites` | Web 站点 | `views/hosts/Assets/WebSites.vue` |
| `/hosts/assets/collections` | 采集任务 | `views/hosts/Assets/Collections.vue` |

导航建议：

```text
主机列表
├── 主机资产态势
├── 智能资产采集
└── 采集任务
```

智能资产采集页面内部使用标签页：

```text
概览 | 软件清单 | 应用资产 | 数据库 | Web 服务 | Web 框架 | Web 站点
```

---

## 3. API 模块

新增前端 API 文件：

```text
frontend/src/api/assets.ts
```

建议方法：

```ts
export function getAssetSummary(params?: AssetSummaryParams): Promise<AssetSummary>
export function listSoftwareAssets(params: SoftwareAssetQuery): Promise<PageResult<SoftwareAsset>>
export function listApplicationAssets(params: ApplicationAssetQuery): Promise<PageResult<ApplicationAsset>>
export function listCollectionTasks(params: CollectionTaskQuery): Promise<PageResult<CollectionTask>>
export function triggerAssetCollection(payload: TriggerCollectionPayload): Promise<CollectionTask>
export function getCollectionTask(id: string): Promise<CollectionTaskDetail>
export function updateApplicationReview(id: string, payload: ApplicationReviewPayload): Promise<ApplicationAsset>
export function getCollectionConfig(): Promise<AssetCollectionConfig>
export function updateCollectionConfig(payload: AssetCollectionConfigPayload): Promise<AssetCollectionConfig>
```

---

## 4. 状态管理

新增 Pinia store：

```text
frontend/src/store/assets.ts
```

核心状态：

```ts
interface AssetState {
  summary: AssetSummary | null
  software: SoftwareAsset[]
  applications: ApplicationAsset[]
  tasks: CollectionTask[]
  total: number
  loading: boolean
  collecting: boolean
  filters: AssetFilters
}
```

状态规则：

- 列表页进入时读取 URL query 作为筛选条件。
- 手动刷新只刷新当前列表，不重置筛选。
- 手动采集成功后跳转或提示查看采集任务。
- 采集任务处于 `running`、`ai_analyzing` 时允许轮询，轮询间隔 5 秒。

---

## 5. 通用筛选

所有资产列表共用筛选区：

| 筛选项 | 类型 |
|:---|:---|
| 关键词 | 输入框，匹配主机名、IP、应用名、版本、路径 |
| 主机 | 主机选择器 |
| 主机分组 | 下拉选择 |
| 操作系统 | 下拉选择 |
| 分类 | 下拉选择，仅应用资产页 |
| 置信度 | 区间选择，仅应用资产页 |
| 状态 | 下拉选择 |
| 记录时间 | 时间范围 |

操作按钮：

- 刷新
- 手动采集
- 导出 CSV
- 重置筛选

---

## 6. 概览页

概览指标：

| 指标 | 说明 |
|:---|:---|
| 软件包数量 | 当前 active 软件资产总数 |
| 应用资产数量 | 当前 active 应用实例数量 |
| 数据库数量 | category=`database` |
| Web 服务数量 | category=`web_service` |
| Web 框架数量 | category=`web_framework` |
| Web 站点数量 | category=`web_site` |
| 待复核资产 | status=`needs_review` |
| 最近采集失败 | 最近 24h 失败任务数量 |

概览下方展示最近采集任务表：

- 任务 ID
- 触发方式
- 主机范围
- 状态
- 成功主机数
- 失败主机数
- 开始时间
- 结束时间
- 操作

右上角提供周期采集配置入口，点击打开配置抽屉。

配置项：

| 配置 | 默认 | 说明 |
|:---|:---|:---|
| 是否启用周期采集 | 启用 | 关闭后不再自动创建周期采集任务 |
| 采集周期 | 12 小时 | 快捷选项 6/12/24 小时，支持自定义 |
| 自定义周期 | - | 1 到 168 小时 |
| 采集范围 | 全部主机 | 第一版默认全部主机，后续扩展主机组 |
| 采集内容 | 软件 + 进程 + AI 应用分析 | 可关闭 AI 分析，仅更新软件/进程 |
| 下一次执行时间 | 自动计算 | 只读展示 |

保存规则：

- 修改周期配置后不立即触发采集。
- 用户点击“立即采集”才创建手动任务。
- 关闭周期采集不清理已有资产。
- 保存成功后刷新概览页的下一次执行时间。

---

## 7. 软件清单页

表格字段：

| 字段 | 前端字段 | 说明 |
|:---|:---|:---|
| 主机名称/AgentID | `hostname` + `host_id` | 两行展示 |
| IP 地址 | `ip_address` | 主机 IP |
| 分组名称 | `group_name` | 无分组显示默认分组 |
| 操作系统 | `os_type` | linux/windows |
| 软件名称 | `name` | 包名 |
| 安装版本 | `version` | 完整版本 |
| 应用安装路径 | `install_paths` | 多路径折叠展示 |
| 最后更新时间 | `last_modified_at` | 包安装/更新时间 |
| 记录时间 | `collected_at` | 入库时间 |

交互：

- 软件名称支持复制。
- 安装路径超过 1 条时显示首条 + 数量，点击抽屉查看全部路径样本。
- 版本为空显示 `unknown`，不显示空白。
- 包管理器用标签展示：rpm、dpkg、apk。

---

## 8. 应用资产页

应用资产页是全部分类的统一列表。

表格字段：

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | hostname + host_id |
| IP 地址 | 主机 IP |
| 分组名称 | 主机组 |
| 操作系统 | OS |
| 应用名称 | AI 识别后的名称 |
| 分类 | 数据库/Web 服务/Web 框架/Web 站点/其他 |
| 安装版本 | 版本号 |
| 监听端口 | 端口集合 |
| 启动用户 | 进程用户 |
| 启动路径 | cwd 或项目路径 |
| 置信度 | 百分比标签 |
| 状态 | active/inactive/needs_review |
| 记录时间 | 入库时间 |

详情抽屉：

- 基础信息
- 关联进程
- AI 识别证据
- 版本工具调用记录
- 关联软件包
- 配置路径和站点路径

人工复核：

- 可修改应用名称、分类、版本、安装路径、配置路径。
- 人工确认后 `review_status=confirmed`，后续自动采集不能覆盖人工字段，只更新运行状态和证据。

---

## 9. 分类页字段

### 9.1 数据库

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | hostname + host_id |
| IP 地址 | 主机 IP |
| 分组名称 | 主机组 |
| 操作系统 | OS |
| 数据库名称 | mysql/mariadb/postgresql/redis/mongodb 等 |
| 安装版本 | 版本 |
| 最后更新时间 | 版本证据或包更新时间 |
| 应用安装路径 | exe 或包路径 |
| 记录时间 | 入库时间 |

### 9.2 Web 服务

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | hostname + host_id |
| IP 地址 | 主机 IP |
| 分组名称 | 主机组 |
| 操作系统 | OS |
| Web 服务名称 | nginx/apache/tomcat/jar 等 |
| 监听端口 | 端口集合 |
| 安装版本 | 版本 |
| 启动用户 | 用户 |
| 启动路径 | 工作目录 |
| 配置文件路径 | 配置路径 |
| 记录时间 | 入库时间 |

### 9.3 Web 框架

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | hostname + host_id |
| IP 地址 | 主机 IP |
| 应用名称 | 应用或项目名 |
| 框架名称 | spring/django/flask/laravel/express 等 |
| 框架版本 | 版本 |
| 运行时 | java/python/php/node 等 |
| 项目路径 | cwd 或推断路径 |
| 置信度 | AI 输出 |
| 记录时间 | 入库时间 |

### 9.4 Web 站点

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | hostname + host_id |
| IP 地址 | 主机 IP |
| 站点名称 | server_name 或 AI 推断名 |
| 域名 | server_name/虚拟主机 |
| 监听端口 | 端口 |
| 站点根目录 | root/docroot |
| Web 服务 | 关联 Web 服务 |
| 配置文件路径 | 配置来源 |
| 记录时间 | 入库时间 |

---

## 10. 采集任务页

字段：

- 任务 ID
- 任务类型：software、process、application_analysis、full
- 触发方式：manual、schedule、agent_register
- 主机范围
- 状态
- 总主机数
- 成功主机数
- 失败主机数
- 当前阶段
- 错误摘要
- 开始时间
- 结束时间

操作：

- 查看详情
- 重试失败主机
- 取消任务
- 查看原始采集摘要

---

## 11. 漏洞扫描入口联动

智能漏洞检查页面不再展示“实时收集软件包”语义，改为引用资产数据：

- 漏洞扫描按钮文案调整为“基于资产数据扫描”。
- 扫描前展示资产数据更新时间，提醒用户数据是否过期。
- 当资产数据超过采集周期 2 倍未更新时，展示“资产数据可能过期”的提示和“立即采集”按钮。
- 扫描结果详情展示关联资产来源：软件资产或应用资产、采集时间、版本证据。
- LLM 识别出的漏洞如果缺少真实来源，前端只展示在“待复核风险”区域，不进入正式漏洞结果。

---

## 12. 空状态与错误状态

| 场景 | 展示 |
|:---|:---|
| 无资产 | 空状态 + 手动采集按钮 |
| Agent 离线 | 列表保留旧资产，任务详情展示离线原因 |
| AI 分析失败 | 应用资产不更新，任务状态为 `ai_failed` |
| 版本工具失败 | 应用保留，版本显示 `unknown`，详情展示工具错误 |
| 低置信度 | 状态标记 `待复核` |
| 周期采集关闭 | 概览页展示关闭状态和开启按钮 |
| 漏洞扫描无资产数据 | 提示先执行资产采集 |

---

## 13. 测试设计

| 测试 | 断言 |
|:---|:---|
| API 类型测试 | 软件、应用、任务接口响应可被正确解析 |
| 软件列表组件测试 | 路径折叠、版本为空、包管理器标签正确展示 |
| 应用详情抽屉测试 | 进程证据、工具记录、人工复核表单正常 |
| 任务轮询测试 | running 到 completed 后停止轮询 |
| 筛选同步测试 | URL query 与列表筛选一致 |
| 周期配置测试 | 默认 12 小时、保存后下一次执行时间刷新 |
| 漏洞扫描联动测试 | 扫描前展示资产更新时间和过期提示 |

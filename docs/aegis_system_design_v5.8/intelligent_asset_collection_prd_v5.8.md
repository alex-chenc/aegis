# V5.8 PRD: 智能资产采集

**版本**: 5.8  
**日期**: 2026-06-04  
**状态**: 设计中  

---

## 1. 背景

当前主机列表只展示 Agent 在线状态、IP、主机名、操作系统和最后心跳，无法回答安全运营中更关键的问题：

- 主机安装了哪些系统软件包，版本和安装路径是什么。
- 主机运行了哪些应用进程，这些进程实际对应数据库、Web 服务、Web 框架还是 Web 站点。
- 应用版本号是否能被识别，并作为漏洞管理、基线检查和运行时告警分析的资产上下文。

V5.8 新增 **智能资产采集** 模块，放在 **主机列表** 下，用 Agent 采集软件包和进程快照，由控制面调用大模型完成应用识别、分类和版本补全，最终形成可查询、可筛选、可用于后续安全分析的主机资产清单。

---

## 2. 产品目标

1. 软件采集支持 `rpm`、`dpkg`、`apk` 三类 Linux 包管理器，采集方式参考 Trivy 的本机软件包解析思路：优先读取包数据库和包管理器元数据，不依赖联网查询。
2. 应用采集遍历 `/proc` 下所有进程，采集 `pid`、`ppid`、`comm`、`cmdline`、`exe`、`cwd`、监听端口、启动用户、启动时间等关键上下文。
3. 控制面把进程快照交给大模型识别应用名称、应用类型和置信度；需要版本时，大模型只能调用 Agent 暴露的只读工具获取版本证据。
4. 应用识别后落库分类展示，第一版分类为：
   - 数据库
   - Web 框架
   - Web 站点
   - Web 服务
5. UI 模块放在主机列表下，提供软件、数据库、Web 服务、Web 框架、Web 站点等分类视图。

---

## 3. 用户角色

| 角色 | 核心诉求 |
|:---|:---|
| 安全运营人员 | 快速查询某台主机安装的软件、运行的服务和版本 |
| 漏洞运营人员 | 按应用名、版本、主机范围筛选受影响资产 |
| 安全分析人员 | 在告警分析时看到进程对应的业务应用和组件类型 |
| 管理员 | 配置采集周期、手动触发采集、查看采集失败原因 |

第一版沿用现有登录体系，不新增 RBAC 表；按钮权限按后端能力预留。

---

## 4. 信息架构

新增菜单位于主机列表下：

```text
主机列表
├── 主机资产态势
├── 智能资产采集
│   ├── 软件清单
│   ├── 数据库
│   ├── Web 服务
│   ├── Web 框架
│   └── Web 站点
└── 采集任务
```

建议路由：

| 页面 | 路由 | 说明 |
|:---|:---|:---|
| 智能资产概览 | `/hosts/assets` | 资产统计、最近采集状态、分类入口 |
| 软件清单 | `/hosts/assets/software` | rpm/dpkg/apk 软件包列表 |
| 应用资产 | `/hosts/assets/applications` | 全部应用资产，支持按分类过滤 |
| 数据库资产 | `/hosts/assets/databases` | 数据库类应用 |
| Web 服务资产 | `/hosts/assets/web-services` | Nginx、Apache、Tomcat、Java Jar 服务等 |
| Web 框架资产 | `/hosts/assets/web-frameworks` | Spring、Django、Flask、Laravel 等 |
| Web 站点资产 | `/hosts/assets/web-sites` | 站点根目录、域名、端口、配置路径 |
| 采集任务 | `/hosts/assets/collections` | 手动/周期采集任务和失败详情 |

---

## 5. 业务范围

### 5.1 软件采集

软件包采集范围：

| 包管理器 | 适用系统 | 采集数据 |
|:---|:---|:---|
| `rpm` | RHEL/CentOS/Rocky/Alma/openEuler 等 | name、version、release、arch、epoch、install_time、source_rpm、license、vendor、files |
| `dpkg` | Debian/Ubuntu 等 | package、version、architecture、status、source、maintainer、installed_size、conffiles |
| `apk` | Alpine 等 | package、version、arch、origin、maintainer、installed_size、files |

设计原则：

- Agent 优先直接解析包数据库，避免执行 `rpm -qa`、`dpkg-query`、`apk info` 这类命令造成性能和兼容性波动。
- 当包数据库解析失败时，允许降级调用只读命令，但必须带超时和输出大小限制。
- 支持采集安装文件路径，路径数量过大时只保留关键路径样本和完整路径摘要。

### 5.2 应用采集

应用采集以进程为原始证据：

| 字段 | 来源 |
|:---|:---|
| `pid`、`ppid` | `/proc/{pid}/stat` |
| `comm` | `/proc/{pid}/comm` |
| `cmdline` | `/proc/{pid}/cmdline` |
| `exe_path` | `/proc/{pid}/exe` |
| `cwd` | `/proc/{pid}/cwd` |
| `uid`、`username` | `/proc/{pid}/status`、系统用户库 |
| `listen_ports` | `/proc/net/*` 或 Agent 网络工具 |
| `start_time` | `/proc/{pid}/stat` + 系统启动时间 |
| `container_id` | cgroup 信息，可选 |

采集后由控制面进行去重和归并：

- 同一 `exe_path + normalized_cmdline + listen_ports` 聚合成一个应用实例。
- 多 worker 进程归并到同一服务，例如 Nginx master/worker、PostgreSQL postmaster/backend。
- 临时进程、内核线程、Agent 自身进程默认保留原始快照，但不进入应用资产列表，除非大模型识别为业务应用。

### 5.3 AI 应用识别

大模型输入：

- 主机信息：OS、架构、主机名、IP。
- 进程摘要：comm、cmdline、exe、cwd、用户、端口、父子进程。
- 关联软件包：通过 exe 路径或包文件列表匹配到的软件包。
- 可选工具结果：版本命令、配置文件路径、站点根目录证据。

大模型输出结构化 JSON：

```json
{
  "applications": [
    {
      "name": "nginx",
      "display_name": "Nginx",
      "category": "web_service",
      "version": "1.24.0",
      "confidence": 0.94,
      "evidence": ["comm=nginx", "listen=80,443", "nginx -v output"],
      "related_pids": [123, 124],
      "install_path": "/usr/sbin/nginx",
      "config_paths": ["/etc/nginx/nginx.conf"],
      "site_paths": ["/var/www/html"]
    }
  ]
}
```

AI 约束：

- 不允许大模型直接执行任意命令。
- 只允许调用 Agent 注册的只读资产工具。
- 工具调用结果必须入库为证据摘要，供人工排查。
- 置信度低于阈值的结果标记为 `needs_review`，默认不参与漏洞自动匹配。

---

## 6. 页面展示要求

### 6.1 软件清单

字段参考：

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | 两行展示 hostname 和 host_id |
| IP 地址 | 主机 IP |
| 分组名称 | 默认分组或主机组 |
| 操作系统 | linux/windows 等 |
| 软件名称 | 包名或应用名 |
| 安装版本 | 完整版本，rpm 包含 epoch/release |
| 应用安装路径 | 关键安装路径，无路径时显示 `-` |
| 最后更新时间 | 包安装或更新日期 |
| 记录时间 | 本次采集入库时间 |

### 6.2 数据库资产

字段参考：

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | hostname + host_id |
| IP 地址 | 主机 IP |
| 分组名称 | 主机组 |
| 操作系统 | OS |
| 数据库名称 | mysql、mariadb、postgresql、redis、mongodb 等 |
| 安装版本 | 识别版本 |
| 最后更新时间 | 版本证据更新时间或软件包更新时间 |
| 应用安装路径 | exe 或包安装路径 |
| 记录时间 | 入库时间 |

### 6.3 Web 服务资产

字段参考：

| 字段 | 说明 |
|:---|:---|
| 主机名称/AgentID | hostname + host_id |
| IP 地址 | 主机 IP |
| 分组名称 | 主机组 |
| 操作系统 | OS |
| Web 服务名称 | nginx、apache、tomcat、jetty、jar 等 |
| 监听端口 | 80、443、8080 等 |
| 安装版本 | 版本号 |
| 启动用户 | root、www-data、app 等 |
| 启动路径 | cwd 或服务工作目录 |
| 配置文件路径 | nginx.conf、httpd.conf、server.xml 等 |
| 记录时间 | 入库时间 |

### 6.4 Web 框架资产

字段：

- 主机名称/AgentID
- IP 地址
- 应用名称
- 框架名称
- 框架版本
- 运行时语言和版本
- 启动命令
- 项目路径
- 置信度
- 记录时间

### 6.5 Web 站点资产

字段：

- 主机名称/AgentID
- IP 地址
- 站点名称
- 域名或 server_name
- 监听端口
- 站点根目录
- Web 服务名称
- 配置文件路径
- TLS 状态
- 记录时间

---

## 7. 采集触发

| 触发方式 | 说明 |
|:---|:---|
| 手动采集 | 页面选择主机或主机组后立即触发 |
| 周期采集 | 默认每 12 小时一次，可在资产数据页面配置周期或关闭 |
| Agent 注册后采集 | 新 Agent 首次上线后延迟触发 |
| 漏洞查询前刷新 | 可选，用户在漏洞影响面查询前手动刷新资产 |

周期配置规则：

- 默认启用，周期为 12 小时。
- 允许配置为 6、12、24 小时或自定义小时数，自定义范围 1 到 168 小时。
- 修改周期后不立即触发采集，只更新下一次计划执行时间；用户可额外点击手动采集。
- 关闭周期采集后保留已有资产数据，不自动标记为失效。
- 周期任务只对在线 Agent 执行；离线主机在下次上线后进入补采队列。

采集任务状态：

```text
pending
running
agent_offline
collect_failed
ai_analyzing
ai_failed
completed
cancelled
```

---

## 8. 非目标

V5.8 第一版不做：

- Windows 软件和应用资产采集。
- 容器镜像层软件包深度解析。
- 资产自动删除。消失的应用先标记为 `inactive`。
- 对 AI 识别结果自动执行修复或阻断。
- 站点源码内容扫描。
- SaaS、云资源、Kubernetes API 资产采集。

---

## 9. 漏洞扫描策略变更

V5.8 起，漏洞扫描不再临时下发任务到 Agent 采集软件包、进程或应用信息。漏洞扫描统一读取智能资产采集模块已经入库的软件资产和应用资产，再把资产上下文传给大模型进行漏洞匹配。

策略目标：

- 漏洞扫描与资产采集解耦，避免每次扫描重复打扰 Agent。
- 漏洞影响面基于稳定的资产快照，扫描结果可追溯到采集时间和资产证据。
- 大模型只做匹配、解释和补充研判，不允许创造不存在的漏洞。

漏洞真实性约束：

- 漏洞必须来自真实漏洞来源，例如 NVD、CNVD、CNNVD、厂商公告、GitHub Security Advisory、发行版安全公告或人工录入的自定义 CVE。
- LLM 输出的每个漏洞必须包含 `cve_id` 或明确的 advisory id、来源、发布时间或公告链接。
- 如果资产可能存在风险但找不到真实漏洞编号或公告来源，结果只能标记为 `potential_risk`，不能入库为漏洞。
- 禁止基于版本号推测并创造类似 `CVE-2026-XXXX` 的漏洞编号。
- 后端必须对 LLM 输出做来源校验；校验失败的漏洞候选进入 `rejected` 或 `needs_review`，不进入正式漏洞列表。

扫描输入：

| 输入 | 来源 |
|:---|:---|
| 软件资产 | `host_software_assets` |
| 应用资产 | `host_application_assets` |
| 版本证据 | `host_application_tool_calls` 和资产 evidence |
| 采集时间 | `collected_at` / `last_seen_at` |
| 人工复核覆盖 | `manual_overrides` |

扫描输出必须引用资产：

- `host_id`
- `software_asset_id` 或 `application_asset_id`
- `asset_name`
- `asset_version`
- `asset_collected_at`
- `vulnerability_source`
- `evidence`

---

## 10. 成功指标

| 指标 | 目标 |
|:---|:---|
| Linux 软件包采集成功率 | 在线 Agent 主机 >= 95% |
| rpm/dpkg/apk 覆盖 | 三类包管理器均可采集 |
| 常见数据库识别准确率 | MySQL/MariaDB/PostgreSQL/Redis/MongoDB >= 90% |
| 常见 Web 服务识别准确率 | Nginx/Apache/Tomcat/Java Jar >= 90% |
| 单主机采集耗时 | 软件 + 进程快照 P95 <= 30s |
| AI 分析耗时 | 单主机 P95 <= 60s |
| UI 查询响应 | P95 <= 1s |
| 漏洞真实性校验 | 正式漏洞结果 100% 有真实来源或人工确认 |

---

## 11. 验收用例

| 用例 | 预期 |
|:---|:---|
| Ubuntu 主机执行采集 | 软件清单出现 dpkg 包，版本和架构正确 |
| CentOS/Rocky 主机执行采集 | 软件清单出现 rpm 包，release 和安装时间正确 |
| Alpine 主机执行采集 | 软件清单出现 apk 包，origin 字段正确 |
| Nginx 进程存在并监听 80 | Web 服务列表出现 Nginx，端口、启动用户、配置路径可见 |
| MariaDB 进程存在 | 数据库列表出现 mariadb，版本号可见 |
| Java jar 服务存在 | Web 服务列表出现 Jar 或识别出的应用名，启动路径和监听端口可见 |
| 版本识别失败 | 应用保留，版本显示 `unknown`，状态为 `needs_review` |
| Agent 离线触发采集 | 任务状态为 `agent_offline`，不产生脏数据 |
| 配置定时采集周期为 12 小时 | 下一次计划采集时间按 12 小时计算 |
| 漏洞扫描执行 | 不调用 Agent 采集接口，只读取资产表 |
| LLM 返回无来源漏洞 | 后端拒绝入正式漏洞结果 |
| LLM 编造 CVE 编号 | 后端标记 rejected 并记录原因 |

---

## 12. 相关文档

| 文档 | 说明 |
|:---|:---|
| `frontend_intelligent_asset_collection_design_v5.8.md` | 前端页面、路由、交互和接口类型 |
| `backend_intelligent_asset_collection_design_v5.8.md` | 后端服务、HTTP API、gRPC 和 LLM 编排 |
| `database_intelligent_asset_collection_design_v5.8.md` | 表结构、索引、状态枚举和保留策略 |
| `agent_intelligent_asset_collection_design_v5.8.md` | Agent 采集器、包管理器解析、进程工具和安全边界 |
| `vulnerability_asset_matching_strategy_v5.8.md` | 基于资产库的漏洞扫描策略和真实性校验 |

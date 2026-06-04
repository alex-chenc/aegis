# V5.8 Agent 设计: 智能资产采集

**版本**: 5.8  
**日期**: 2026-06-04  
**状态**: 设计中  

---

## 1. 目标

Agent 新增主机智能资产采集能力：

- 软件包采集支持 `rpm`、`dpkg`、`apk`。
- 应用采集遍历 `/proc` 进程并生成进程快照。
- 暴露只读工具给控制面和大模型，用于版本识别、配置路径确认、包归属查询。
- 采集过程不影响 eBPF、Sigma 和命令执行主流程。

---

## 2. 模块结构

建议新增目录：

```text
agent/internal/assets/
├── collector.go
├── model.go
├── package_collector.go
├── process_collector.go
├── rpm_collector.go
├── dpkg_collector.go
├── apk_collector.go
├── package_file_index.go
├── redactor.go
└── version_tools.go
```

现有 `agent/internal/asset/collector.go` 只负责注册资产信息，新模块使用复数 `assets` 表示深度资产采集，避免与旧模块混淆。

---

## 3. 采集数据模型

### 3.1 HostAssetSnapshot

```go
type HostAssetSnapshot struct {
    HostID       string             `json:"host_id"`
    Hostname     string             `json:"hostname"`
    IPAddress    string             `json:"ip_address"`
    OSType       string             `json:"os_type"`
    OSVersion    string             `json:"os_version"`
    Arch         string             `json:"arch"`
    Packages     []PackageAsset     `json:"packages"`
    Processes    []ProcessAsset     `json:"processes"`
    CollectedAt  time.Time          `json:"collected_at"`
    Errors       []CollectError     `json:"errors"`
}
```

### 3.2 PackageAsset

```go
type PackageAsset struct {
    Name           string            `json:"name"`
    Version        string            `json:"version"`
    Release        string            `json:"release,omitempty"`
    Epoch          string            `json:"epoch,omitempty"`
    Architecture   string            `json:"architecture"`
    PackageManager string            `json:"package_manager"`
    SourceName     string            `json:"source_name,omitempty"`
    Vendor         string            `json:"vendor,omitempty"`
    License        string            `json:"license,omitempty"`
    InstallTime    time.Time         `json:"install_time,omitempty"`
    InstallPaths   []string          `json:"install_paths"`
    FileCount      int               `json:"file_count"`
    Metadata       map[string]string `json:"metadata"`
}
```

### 3.3 ProcessAsset

```go
type ProcessAsset struct {
    PID           int       `json:"pid"`
    PPID          int       `json:"ppid"`
    Comm          string    `json:"comm"`
    Cmdline       string    `json:"cmdline"`
    ExePath       string    `json:"exe_path"`
    Cwd           string    `json:"cwd"`
    UID           int       `json:"uid"`
    Username      string    `json:"username"`
    ListenPorts   []int     `json:"listen_ports"`
    StartTime     time.Time `json:"start_time,omitempty"`
    ContainerID   string    `json:"container_id,omitempty"`
    PackageName   string    `json:"package_name,omitempty"`
}
```

---

## 4. 软件包采集

### 4.1 包管理器探测

探测顺序：

1. 如果存在 `/var/lib/rpm` 或 rpm 数据库文件，启用 rpm collector。
2. 如果存在 `/var/lib/dpkg/status`，启用 dpkg collector。
3. 如果存在 `/lib/apk/db/installed`，启用 apk collector。
4. 多包管理器共存时全部采集，避免容器或混合环境漏报。

### 4.2 rpm

目标：

- 采集 name、epoch、version、release、arch、install time、source rpm。
- 尽量采集文件列表，用于 exe 路径反查软件包。

实现策略：

- 第一优先：使用 Go rpmdb 解析库直接读取 rpm 数据库。
- 降级：执行只读命令 `rpm -qa --qf ...` 和 `rpm -ql`，必须加超时和输出限制。
- 文件列表过大时只保留可执行文件、配置文件和服务文件路径样本。

### 4.3 dpkg

目标：

- 解析 `/var/lib/dpkg/status`。
- 解析 `/var/lib/dpkg/info/*.list` 获取文件路径。
- 解析 `/var/lib/dpkg/info/*.conffiles` 获取配置文件。

字段映射：

| dpkg 字段 | PackageAsset 字段 |
|:---|:---|
| Package | Name |
| Version | Version |
| Architecture | Architecture |
| Source | SourceName |
| Maintainer | Metadata |
| Installed-Size | Metadata |

### 4.4 apk

目标：

- 解析 `/lib/apk/db/installed`。
- 解析包条目中的 `P`、`V`、`A`、`o`、`m`、`F`、`R` 字段。

字段映射：

| apk 字段 | PackageAsset 字段 |
|:---|:---|
| P | Name |
| V | Version |
| A | Architecture |
| o | SourceName |
| m | Metadata.maintainer |
| F/R | InstallPaths |

---

## 5. 进程采集

### 5.1 遍历规则

- 遍历 `/proc` 下数字目录。
- 跳过内核线程：`cmdline` 为空且无可执行文件时标记为 kernel，不进入应用候选。
- 跳过 Agent 自身进程，但保留到原始快照错误统计中。
- 单次最多采集 `max_process_count`，默认 2000。

### 5.2 采集字段

| 字段 | 文件 |
|:---|:---|
| comm | `/proc/{pid}/comm` |
| cmdline | `/proc/{pid}/cmdline` |
| exe_path | `/proc/{pid}/exe` |
| cwd | `/proc/{pid}/cwd` |
| uid | `/proc/{pid}/status` |
| ppid/start_time | `/proc/{pid}/stat` |
| cgroup/container | `/proc/{pid}/cgroup` |

监听端口：

- 读取 `/proc/net/tcp`、`/proc/net/tcp6`、`/proc/net/udp`、`/proc/net/udp6`。
- 建立 socket inode 到 pid 映射：扫描 `/proc/{pid}/fd` 中的 `socket:[inode]`。
- 只返回 LISTEN TCP 端口和可识别 UDP 服务端口。

### 5.3 脱敏

发送给控制面的 `cmdline` 需要脱敏：

- `--password=xxx`
- `--token=xxx`
- `--secret=xxx`
- `--access-key=xxx`
- URL 中的用户名密码。

保留参数名，值替换为 `***`。

---

## 6. Agent 只读工具

新增工具注册到 `agent/internal/tools/tool_manager.go`：

| 工具名 | 参数 | 用途 |
|:---|:---|:---|
| `AssetGetProcessVersion` | `pid`、`exe_path`、`hint` | 获取进程或二进制版本 |
| `AssetReadConfigSummary` | `path`、`max_size` | 返回配置摘要和关键字段，不返回敏感值 |
| `AssetListDirectoryHints` | `path`、`max_entries` | 返回目录关键文件名 |
| `AssetResolvePackageByFile` | `path` | 通过文件路径匹配软件包 |

### 6.1 AssetGetProcessVersion

执行策略：

1. 根据 `exe_path` 和 `comm` 匹配内置版本命令模板。
2. 不允许 LLM 提供完整 shell 命令。
3. 命令执行不经过 shell，使用 argv 数组。
4. 超时默认 3 秒，最大输出 8KB。

模板示例：

| 应用 | argv |
|:---|:---|
| nginx | `["/usr/sbin/nginx", "-v"]` |
| apache/httpd | `["/usr/sbin/httpd", "-v"]` |
| postgres | `["/usr/bin/postgres", "--version"]` |
| mysql/mariadb | `["/usr/bin/mysql", "--version"]` |
| redis-server | `["/usr/bin/redis-server", "--version"]` |
| java | `["/usr/bin/java", "-version"]` |
| node | `["/usr/bin/node", "--version"]` |
| python | `["/usr/bin/python3", "--version"]` |

### 6.2 AssetReadConfigSummary

安全限制：

- 只允许读取采集快照中已出现的配置路径，或常见配置目录下的文件。
- 最大读取 64KB。
- 脱敏 password/token/secret/key。
- 只返回关键字段摘要，例如 nginx `server_name`、`root`、`listen`。

---

## 7. gRPC 接口

建议在 `proto/agent_comm.proto` 新增：

```proto
message HostAssetCollectRequest {
  repeated string collect_types = 1; // software, process
  bool include_package_files = 2;
  bool include_listen_ports = 3;
  int32 max_process_count = 4;
}

message HostAssetCollectResponse {
  bool success = 1;
  string snapshot_json = 2;
  string error = 3;
}

service AgentService {
  rpc CollectHostAssets(HostAssetCollectRequest) returns (HostAssetCollectResponse);
}
```

兼容：

- 保留现有 `CollectSoftwareList`。
- `CollectSoftwareList` 可内部调用新 package collector，并转换为旧 `SoftwareInfo`。

---

## 8. 性能控制

| 控制项 | 默认 |
|:---|:---|
| 软件包采集超时 | 20s |
| 进程采集超时 | 10s |
| 版本工具超时 | 3s |
| 最大进程数 | 2000 |
| 最大文件路径样本 | 每包 200 条 |
| 最大 snapshot JSON | 10MB |
| 同时执行工具数 | 1 |

采集在独立 goroutine 中执行，使用 context 取消；超时后返回已采集部分和错误摘要。

---

## 9. 错误处理

| 场景 | 处理 |
|:---|:---|
| 无包数据库 | 返回空 packages + error summary |
| rpmdb 解析失败 | 降级只读命令 |
| `/proc` 进程消失 | 跳过并计数 |
| 权限不足读取 exe/cwd | 字段为空，保留错误摘要 |
| 版本命令失败 | 返回 success=false，不影响应用资产入库 |
| snapshot 过大 | 截断路径样本，保留计数 |

---

## 10. 安全边界

- 不接受控制面下发任意 shell。
- 只读工具必须注册在白名单。
- 文件读取工具必须限制路径、大小和脱敏。
- 采集不读取环境变量 `/proc/{pid}/environ`，避免凭据泄露。
- 采集结果中的 cmdline 必须脱敏后再上报。
- Agent 日志不打印完整 cmdline 和配置内容。

---

## 11. 测试设计

| 测试 | 内容 |
|:---|:---|
| rpm collector 单测 | 构造 rpmdb fixture 或命令输出 fixture |
| dpkg collector 单测 | status/list/conffiles fixture |
| apk collector 单测 | installed fixture |
| process collector 单测 | 解析 stat、status、cmdline、cgroup |
| redactor 单测 | token/password/URL 凭据脱敏 |
| version tool 单测 | 只允许模板命令，拒绝任意 shell |
| snapshot size 测试 | 路径样本截断和计数正确 |
| gRPC 测试 | CollectHostAssets 超时和部分成功 |

---

## 12. 与现有能力关系

- 不改动 eBPF runtime collector。
- 不改动 Sigma matcher。
- 复用现有 `ExecuteTool` 模式，但新增工具必须独立命名并限制为资产只读工具。
- 旧 `ListInstalledSoftware` 工具如果存在，应迁移为调用新 package collector。

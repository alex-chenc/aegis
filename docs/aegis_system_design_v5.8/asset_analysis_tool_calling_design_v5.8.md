# V5.8 设计: 资产分析 LLM 工具调用

**版本**: 5.8
**日期**: 2026-06-04
**状态**: 设计中

---

## 1. 问题描述

当前资产分析流程存在版本识别不准确的问题：

- API Server 端 `AssetAnalysisService` 只将进程快照（PID、Comm、ExePath、Cmdline、Ports）发送给 LLM
- LLM 被要求识别应用名称和版本，但大部分进程快照中**没有版本信息**
- LLM 只能"猜测"版本号，导致 `AIConfidence` 低、数据不可靠

而 Agent 端已经有完整的版本获取工具：

| 工具 | 功能 | 状态 |
|------|------|------|
| `AssetGetProcessVersion` | 执行 `nginx -v` 等命令获取版本 | ✅ 已实现，未被调用 |
| `AssetResolvePackageByFile` | 通过 `rpm -qf` / `dpkg -S` 查包 | ✅ 已实现，未被调用 |
| `AssetReadConfigSummary` | 读取配置文件摘要 | ✅ 已实现，未被调用 |
| `AssetListDirectoryHints` | 列出目录文件 | ✅ 已实现，未被调用 |

**根本原因**：资产分析流程是一次性 LLM 调用，没有工具调用能力。

---

## 2. 目标

让 LLM 在资产分析过程中能够调用 Agent 工具：

1. LLM 识别出应用后，如果无法从进程快照确定版本，自动调用 `AssetGetProcessVersion` 获取真实版本
2. LLM 可以调用 `AssetResolvePackageByFile` 通过 exe 路径查找所属软件包
3. LLM 可以调用 `AssetReadProcFile` 读取 `/proc/{pid}/` 下的只读文件（如 `maps`、`fd`、`environ` 脱敏版）获取更多信息
4. LLM 可以调用 `AssetListDirectoryHints` 查看目录结构确认安装路径

---

## 3. 方案设计

### 3.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    AssetAnalysisService                         │
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐  │
│  │ buildPrompt  │───>│  LLM Call    │───>│ parseReActOutput │  │
│  │ (进程快照)    │    │ (ReAct 格式)  │    │ (解析工具调用)    │  │
│  └──────────────┘    └──────────────┘    └──────────────────┘  │
│         ▲                                       │              │
│         │                                       ▼              │
│         │                              ┌──────────────────┐    │
│         │                              │  Tool Executor   │    │
│         │                              │  (gRPC -> Agent) │    │
│         │                              └──────────────────┘    │
│         │                                       │              │
│         └───────────── Observation ◄─────────────┘              │
│                                                                 │
│  循环直到 LLM 输出 Final Answer 或达到最大迭代次数              │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 ReAct 循环

复用现有 `ReActAgent` 的 ReAct 文本协议模式，但为资产分析定制：

```
Thought: 我识别到 nginx 进程，但无法从快照确定版本，需要调用工具获取
Action: AssetGetProcessVersion
Action Input: {"pid": 1234, "exe_path": "/usr/sbin/nginx", "hint": "nginx"}
Observation: {"success": true, "version": "1.24.0", "output": "nginx version: nginx/1.24.0"}
Thought: 获取到 nginx 版本 1.24.0，继续分析下一个进程...
...
Final Answer: {"applications": [...]}
```

### 3.3 可用工具列表

资产分析 ReAct 循环中 LLM 可调用的工具（每个进程每个工具只能调用一次）：

| 工具 | 参数 | 用途 | 限制 |
|------|------|------|------|
| `AssetGetProcessVersion` | `pid`, `exe_path`, `hint` | 获取进程版本 | 每进程 1 次 |
| `AssetResolvePackageByFile` | `path` | 通过文件路径查所属软件包 | 每进程 1 次 |
| `AssetReadConfigSummary` | `path`, `max_size` | 读取配置文件摘要 | 每进程 1 次 |
| `AssetListDirectoryHints` | `path`, `max_entries` | 列出目录文件 | 每进程 1 次 |
| `AssetReadProcFile` | `pid`, `file_name` | 读取 /proc/{pid}/ 下的文件 | 每进程 1 次，最大 10KB |

### 3.4 新增工具: AssetReadProcFile

Agent 端新增一个只读工具，允许 LLM 读取 `/proc/{pid}/` 下的任意文件：

**安全约束**：
- **只读**：不允许任何写操作
- **大小限制**：最大读取 10KB，超过则截断
- **路径限制**：只能读取 `/proc/{pid}/` 下的文件，不能跨进程读取

**禁止读取的文件**（安全风险）：
- `environ` - 包含环境变量，可能有密码/token
- `mem` - 可读取进程内存

返回值：
```json
{
  "success": true,
  "content": "文件内容（最大 10KB）",
  "file_name": "status",
  "size": 1024,
  "truncated": false
}
```

### 3.5 工具调用频率限制

**每个进程每个工具只能调用一次**。在 ReAct 循环中跟踪已调用的工具：

```go
// 调用记录 key: "pid:tool_name"
calledTools := make(map[string]bool)
```

在 prompt 中明确告知 LLM：
```
## 工具调用限制
- 每个进程的每个工具只能调用一次
- 请合理规划调用顺序，优先获取最关键的信息
- 如果工具调用失败，不要重试同一个进程的同一个工具
```

---

## 4. 代码变更

### 4.1 Agent 端

#### 4.1.1 新增 AssetReadProcFile 工具

**文件**: `agent/internal/assets/version_tools.go`

新增方法：

```go
// AssetReadProcFileResult 读取 /proc 文件结果
type AssetReadProcFileResult struct {
    Success   bool   `json:"success"`
    FileName  string `json:"file_name"`
    Content   string `json:"content,omitempty"`
    Size      int    `json:"size"`
    Truncated bool   `json:"truncated,omitempty"`
    Error     string `json:"error,omitempty"`
}

// AssetReadProcFile 读取 /proc/{pid}/ 下的文件
// 安全约束：
// - 只读，不允许写操作
// - 最大读取 10KB
// - 禁止读取 environ（环境变量）和 mem（进程内存）
// - 只能读取 /proc/{pid}/ 下的文件
func (v *VersionTool) AssetReadProcFile(ctx context.Context, pid int, fileName string) AssetReadProcFileResult
```

**文件**: `agent/internal/tools/tool_manager.go`

在 `Execute()` 的 switch-case 中新增：

```go
case "AssetReadProcFile":
    pid, err := toInt(params["pid"])
    if err != nil {
        return nil, err
    }
    fileName, _ := params["file_name"].(string)
    return m.versionTool.AssetReadProcFile(context.Background(), pid, fileName), nil
```

### 4.2 API Server 端

#### 4.2.1 重构 AssetAnalysisService

**文件**: `api-server/internal/service/asset_analysis_service.go`

核心变更：

1. 新增 `toolExecutor` 字段，用于调用 Agent 工具
2. 新增 `analyzeWithReAct()` 方法，实现 ReAct 循环
3. 修改 `completeApplicationAnalysis()` 支持多轮对话

```go
type AssetAnalysisService struct {
    repo       *repository.AssetCollectionRepository
    configRepo ConfigRepositoryInterface
    serverClient ServerClientInterface  // 新增：用于调用 Agent 工具
    logger     *zap.Logger
}

// analyzeWithReAct 使用 ReAct 循环进行应用分析
func (s *AssetAnalysisService) analyzeWithReAct(
    ctx context.Context,
    llmClient *llm.LLMClient,
    snapshot HostAssetSnapshot,
    batchIndex, batchTotal int,
) (*ApplicationAnalysisResult, error)
```

#### 4.2.2 新增资产分析工具执行器

**文件**: `api-server/internal/service/asset_analysis_service.go`

```go
// assetToolExecutor 实现 llm.ToolExecutor 接口
type assetToolExecutor struct {
    serverClient ServerClientInterface
    hostID       string
    logger       *zap.Logger
}

func (e *assetToolExecutor) Execute(ctx context.Context, tool string, args map[string]interface{}) (interface{}, error) {
    args["host_id"] = e.hostID
    argsJSON, _ := json.Marshal(args)
    resp, err := e.serverClient.ExecuteTool(ctx, uuid.New().String(), e.hostID, tool, string(argsJSON), 10)
    if err != nil {
        return nil, err
    }
    if !resp.Success {
        return nil, fmt.Errorf("tool execution failed: %s", resp.Error)
    }
    var result interface{}
    json.Unmarshal([]byte(resp.Result), &result)
    return result, nil
}
```

#### 4.2.3 更新系统提示词

**文件**: `api-server/internal/service/asset_analysis_service.go`

更新 `applicationAnalysisSystemPrompt`，加入工具调用说明：

```go
const applicationAnalysisSystemPrompt = `你是主机应用识别专家。只根据进程快照识别主机上运行的应用程序。

## 任务
1. 识别每个应用的名称、类型和版本
2. 将应用分类为：database, web_service, web_framework, web_site, other, unknown
3. 评估识别置信度（0-1）
4. 提供识别证据

## 可用工具
当无法从进程快照确定版本或需要更多信息时，可以调用以下工具：

- AssetGetProcessVersion: 获取进程版本。参数: pid (int), exe_path (string), hint (string)
- AssetResolvePackageByFile: 通过文件路径查找所属软件包。参数: path (string)
- AssetReadConfigSummary: 读取配置文件摘要。参数: path (string), max_size (int)
- AssetListDirectoryHints: 列出目录文件。参数: path (string), max_entries (int)
- AssetReadProcFile: 读取 /proc/{pid}/ 下的安全文件。参数: pid (int), file_name (string)

## 工具调用格式
Thought: [你的推理]
Action: [工具名]
Action Input: [JSON 格式参数]

## 分类规则
- database: MySQL, MariaDB, PostgreSQL, Redis, MongoDB, Elasticsearch 等
- web_service: Nginx, Apache, Tomcat, Jetty 等 Web 服务器
- web_framework: Spring Boot, Django, Flask, Laravel, Express 等框架应用
- web_site: 具体的网站站点，有域名、根目录等
- other: 其他类型应用
- unknown: 无法确定的应用

## 输出格式
当收集到足够信息后，输出 Final Answer（JSON 格式）：
Final Answer: {"applications": [...]}

每个应用包含：
{
  "name": "nginx",
  "display_name": "Nginx",
  "category": "web_service",
  "version": "1.24.0",
  "confidence": 0.95,
  "evidence": ["comm=nginx", "listen=80,443", "version_tool=1.24.0"],
  "related_pids": [123, 124],
  "install_path": "/usr/sbin/nginx",
  "start_path": "/",
  "config_paths": ["/etc/nginx/nginx.conf"],
  "site_paths": ["/var/www/html"],
  "listen_ports": [80, 443],
  "run_user": "www-data",
  "status": "active"
}

## 工具调用限制（重要）
- 每个进程的每个工具只能调用一次，请合理规划调用顺序
- 优先调用 AssetGetProcessVersion 获取版本
- 如果版本工具失败，再尝试 AssetResolvePackageByFile
- AssetReadProcFile 最大读取 10KB，禁止读取 environ 和 mem
- 总工具调用次数最多 10 次，之后必须输出 Final Answer

## 约束
- 不要编造不存在的应用
- 版本号优先来自工具调用结果，其次来自进程快照证据
- 置信度低于 0.3 的标记为 needs_review
- 如果本分片没有可识别应用，输出 Final Answer: {"applications":[]}`
```

---

## 5. 数据流

```
1. AssetAnalysisService.AnalyzeHostApplications()
   ├── 获取 LLM 配置
   ├── 按 50 个进程一批分片
   └── 对每个分片:
       ├── buildAnalysisPrompt() 构建进程快照 prompt
       ├── analyzeWithReAct() ReAct 循环:
       │   ├── LLM Call #1 (system prompt + user prompt)
       │   ├── 解析 LLM 输出:
       │   │   ├── 如果是 Action -> 执行工具 -> 拼接 Observation -> 继续循环
       │   │   └── 如果是 Final Answer -> 解析 JSON -> 返回结果
       │   ├── LLM Call #2 (含工具执行结果)
       │   └── ... 最多 10 次工具调用
       ├── parseAnalysisResult()
       └── saveApplicationAnalysisResult()
```

---

## 6. 接口变更

### 6.1 无 Proto 变更

现有 `ExecuteTool` gRPC 接口已经支持任意工具调用，无需修改 proto。

### 6.2 ServerClientInterface 变更

`AssetAnalysisService` 需要访问 `ServerClientInterface` 来调用 Agent 工具：

```go
// 在 asset_collection_service.go 中已定义
type ServerClientInterface interface {
    ListConnectedAgents(ctx context.Context) (*pb.ListConnectedAgentsResponse, error)
    ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error)
}
```

`AssetAnalysisService` 构造函数需要新增 `serverClient` 参数。

---

## 7. 安全考虑

1. **工具白名单**: 只允许调用预定义的资产只读工具，不允许任意命令执行
2. **文件读取限制**: `AssetReadProcFile` 可读取 /proc/{pid}/ 下任意文件，但：
   - 禁止读取 `environ`（环境变量，可能含密码）
   - 禁止读取 `mem`（进程内存）
   - 最大读取 10KB，超过截断
   - 只能读取当前进程的文件，不能跨进程
3. **脱敏**: `cmdline` 和配置文件内容在返回前进行脱敏处理
4. **超时**: 每个工具调用超时 10 秒
5. **迭代限制**: ReAct 循环最多 10 次工具调用
6. **频率限制**: 每个进程的每个工具只能调用一次，防止 LLM 滥用

---

## 8. 性能考虑

1. **工具调用延迟**: 每次工具调用通过 gRPC 转发，约 100-500ms
2. **最大 LLM 调用次数**: 每个分片最多 11 次（1 次初始 + 10 次工具调用）
3. **分片大小**: 保持 50 个进程一批，控制 prompt 长度
4. **超时控制**: 单个分片分析超时 120 秒
5. **文件读取限制**: `AssetReadProcFile` 最大读取 10KB，避免传输大文件
6. **频率限制**: 每个进程每个工具只能调用一次，50 个进程最多 250 次工具调用（50 进程 × 5 工具）

---

## 9. 测试用例

### 9.1 单元测试

| 测试 | 内容 |
|------|------|
| `TestAssetReadProcFile` | 验证 /proc 文件读取和白名单控制 |
| `TestAnalyzeWithReAct` | 验证 ReAct 循环正确解析工具调用 |
| `TestAssetToolExecutor` | 验证工具执行器正确转发请求 |
| `TestBuildAnalysisPromptWithTools` | 验证 prompt 包含工具说明 |

### 9.2 集成测试

| 测试 | 内容 |
|------|------|
| `TestAssetAnalysisWithVersionTool` | 端到端测试：进程快照 -> LLM 调用工具 -> 获取版本 -> 保存结果 |
| `TestAssetAnalysisToolTimeout` | 工具调用超时处理 |
| `TestAssetAnalysisMaxIterations` | 达到最大迭代次数后强制返回 |

---

## 10. 兼容性

- **向后兼容**: 如果 LLM 不调用工具，行为与原来一致
- **Agent 兼容**: 新增 `AssetReadProcFile` 工具，旧 Agent 不支持但不影响其他功能
- **配置兼容**: 不需要修改配置文件

---

## 11. 回滚计划

1. 如果工具调用导致问题，可以通过配置开关禁用
2. 回退到原始的单次 LLM 调用模式
3. 不涉及数据库 schema 变更

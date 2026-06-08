# AI 资产采集 UI 设计文档 (V6.0)

## 1. 需求概述

在"智能资产采集"模块下新增 3 个 AI 相关资产分类页面：
- **AI LLM**：LLM 服务资产（Ollama、vLLM、LiteLLM、Dify 等）
- **AI Agent**：AI Agent 框架资产（Claude Code、Cursor、Windsurf 等）
- **MCP**：MCP Server 资产

**范围约束**：仅做资产采集和展示，不做风险评估。

## 2. 设计方案

### 2.1 核心思路

**复用现有 `HostApplicationAsset` 模型**，通过扩展 `Category` 字段枚举值实现：

| 新增 Category | 含义 | 示例 |
|---------------|------|------|
| `llm_service` | LLM 推理服务 | Ollama, vLLM, LiteLLM, Dify, Open WebUI |
| `ai_agent` | AI Agent 框架 | Claude Code, Cursor, Windsurf, Gemini CLI |
| `mcp_server` | MCP Server | filesystem, github, postgres, custom servers |

**优势**：零数据库 Schema 变更，复用现有的采集、存储、查询、展示全链路。

### 2.2 数据流

```
Agent 侧采集:
  LLMServiceCollector (HTTP 端口探测)  ─┐
  AIAgentCollector (配置文件扫描)       ─┤→ HostAssetSnapshot.AIAssets
  MCPCollector (mcp.json 解析)          ─┘
       ↓
  AssetCollector.Collect() → ToolManager ("AssetCollectHostAssets")
       ↓
  gRPC ExecuteTool → Server → API-Server AssetCollectionService
       ↓
  解析 AIAssets → Upsert host_application_assets (category = llm_service/ai_agent/mcp_server)
       ↓
  前端按 category 过滤展示
```

## 3. Agent 侧采集变更

### 3.1 数据模型扩展

**文件**: `agent/internal/assets/model.go`

新增 `AIAsset` 结构体和 `HostAssetSnapshot` 新字段：

```go
// AIAsset AI 资产（LLM 服务 / AI Agent / MCP Server）
type AIAsset struct {
    Category    string   `json:"category"`     // llm_service, ai_agent, mcp_server
    Name        string   `json:"name"`         // 服务名称 (如 "ollama", "claude-code")
    DisplayName string   `json:"display_name"` // 显示名称
    Version     string   `json:"version"`      // 版本号
    Source      string   `json:"source"`       // 检测来源: probe/config/process
    Endpoint    string   `json:"endpoint"`     // 服务端点 (如 "http://localhost:11434")
    ListenPorts []int    `json:"listen_ports"` // 监听端口
    ConfigPath  string   `json:"config_path"`  // 配置文件路径
    PIDs        []int    `json:"pids"`         // 关联进程 PID
    Extra       map[string]string `json:"extra"` // 额外信息 (如 models, tools)
}

// HostAssetSnapshot 扩展
type HostAssetSnapshot struct {
    // ... 现有字段 ...
    AIAssets []AIAsset `json:"ai_assets"` // 新增
}
```

### 3.2 LLM 服务探测器

**新文件**: `agent/internal/assets/llm_service_collector.go`

参考 Julius 的 Probe 架构，对监听端口的进程发送 HTTP 探测请求：

```go
type LLMServiceCollector struct {
    logger  *zap.Logger
    client  *http.Client
    probes  []LLMProbe
}

type LLMProbe struct {
    Name        string            // 服务名 (如 "ollama")
    DisplayName string            // 显示名 (如 "Ollama")
    PortHints   []int             // 常见端口 (如 11434)
    Requests    []ProbeRequest    // 探测请求列表
    MatchMode   string            // "all" 或 "any"
}

type ProbeRequest struct {
    Path        string
    Method      string            // 默认 GET
    BodyContains []string         // body 必须包含的字符串
    StatusCodes  []int            // 允许的状态码
}
```

**检测规则**（嵌入二进制，参考 Julius 的 63 个 Probe）：

| 服务 | 端口 | 探测路径 | 匹配条件 |
|------|------|---------|---------|
| Ollama | 11434 | GET `/` + GET `/api/tags` | body.contains "Ollama is running" + "models" |
| vLLM | 8000 | GET `/v1/models` | 200 + body.contains "data" |
| LiteLLM | 4000 | GET `/health` | body.contains "litellm_metadata" |
| Dify | 5001 | GET `/apps` | body.contains "Dify" |
| Open WebUI | 3000 | GET `/api/v1/auths` | 200 响应 |
| HuggingFace TGI | 8080 | GET `/health` | body.contains "ok" |
| SGLang | 30000 | GET `/v1/models` | 200 + body.contains "data" |
| LocalAI | 8080 | GET `/v1/models` | 200 + body.contains "data" |
| llama.cpp | 8080 | GET `/health` | body.contains "ok" |
| NVIDIA NIM | 8000 | GET `/v1/models` | header.contains "nvidia" |

**采集流程**：
1. 从进程快照中筛选监听端口的进程
2. 对每个端口，按端口优先级选择 Probe 并发探测
3. 匹配成功则生成 `AIAsset{Category: "llm_service"}`
4. 使用 JQ 表达式提取可用模型列表存入 `Extra["models"]`

### 3.3 AI Agent 配置扫描器

**新文件**: `agent/internal/assets/ai_agent_collector.go`

扫描已知 Agent 配置路径：

```go
type AIAgentCollector struct {
    logger *zap.Logger
}

type AgentProfile struct {
    Name        string // "claude-code", "cursor", "windsurf"
    DisplayName string
    ConfigPaths []string // 配置文件搜索路径
}
```

**检测规则**：

| Agent | 配置路径 (Linux) | 配置路径 (macOS) |
|-------|-----------------|-----------------|
| Claude Code | `~/.claude/` | `~/.claude/` |
| Claude Desktop | `~/.config/claude/` | `~/Library/Application Support/Claude/` |
| Cursor | `~/.cursor/` | `~/Library/Application Support/Cursor/` |
| Windsurf | `~/.windsurf/` | `~/Library/Application Support/Windsurf/` |
| VS Code + AI | `~/.vscode/` | `~/Library/Application Support/Code/` |
| Gemini CLI | `~/.gemini/` | `~/.gemini/` |
| Amp | `~/.amp/` | `~/.amp/` |

**采集流程**：
1. 遍历 AgentProfile 列表
2. 检查配置路径是否存在
3. 存在则生成 `AIAsset{Category: "ai_agent", ConfigPath: path}`
4. 尝试从配置文件读取版本信息

### 3.4 MCP Server 配置解析器

**新文件**: `agent/internal/assets/mcp_collector.go`

解析 AI Agent 的 MCP 配置文件：

```go
type MCPCollector struct {
    logger *zap.Logger
}

type MCPConfig struct {
    MCPServers map[string]MCPServerDef `json:"mcpServers"`
}

type MCPServerDef struct {
    Command     string            `json:"command"`
    Args        []string          `json:"args"`
    Env         map[string]string `json:"env"`
    URL         string            `json:"url"`      // SSE/HTTP 类型
    Transport   string            `json:"transport"` // stdio, sse, http
}
```

**扫描路径**：
- `~/.claude/mcp.json`
- `~/.cursor/mcp.json`
- `~/.config/claude/mcp.json`
- `~/Library/Application Support/Claude/claude_desktop_config.json`
- 项目目录 `.mcp.json`

**采集流程**：
1. 扫描已知 MCP 配置文件路径
2. 解析 JSON 提取 `mcpServers` 定义
3. 对每个 Server 生成 `AIAsset{Category: "mcp_server", Name: serverName}`
4. 记录 command/args/url 到 Extra 字段

### 3.5 接入 AssetCollector

**文件**: `agent/internal/assets/collector.go`

```go
type AssetCollector struct {
    logger           *zap.Logger
    packageCollector *PackageCollector
    processCollector *ProcessCollector
    llmCollector     *LLMServiceCollector     // 新增
    agentCollector   *AIAgentCollector         // 新增
    mcpCollector     *MCPCollector             // 新增
}
```

在 `Collect()` 方法中，进程采集后调用 3 个新采集器，结果写入 `snapshot.AIAssets`。

## 4. API-Server 侧变更

### 4.1 解析 AIAssets 并存储

**文件**: `api-server/internal/service/asset_collection_service.go`

在 `collectHost` 方法中，解析 `HostAssetSnapshot.AIAssets`，对每个 AIAsset 调用 `repo.UpsertApplicationAsset`，映射规则：

```go
for _, ai := range snapshot.AIAssets {
    app := &model.HostApplicationAsset{
        HostID:      hostID,
        Hostname:    snapshot.Hostname,
        IPAddress:   snapshot.IPAddress,
        OSType:      snapshot.OSType,
        Category:    ai.Category,      // llm_service / ai_agent / mcp_server
        Name:        ai.Name,
        DisplayName: ai.DisplayName,
        Version:     ai.Version,
        ListenPorts: ai.ListenPorts,
        InstallPath: ai.ConfigPath,
        StartPath:   ai.Endpoint,
        AIConfidence: 0.9,  // Agent 侧直接采集，高置信度
        ReviewStatus: "auto",
        Status:       "active",
        Fingerprint:  sha256(hostID + ":" + ai.Category + ":" + ai.Name),
    }
    repo.UpsertApplicationAsset(app)
}
```

### 4.3 扩展有效分类枚举

**文件**: `api-server/internal/service/asset_analysis_service.go`

```go
// 现有
validCategories := map[string]bool{
    "database":      true,
    "web_service":   true,
    "web_framework": true,
    "web_site":      true,
    "other":         true,
    "unknown":       true,
}

// 新增
validCategories := map[string]bool{
    "database":      true,
    "web_service":   true,
    "web_framework": true,
    "web_site":      true,
    "llm_service":   true,  // 新增
    "ai_agent":      true,  // 新增
    "mcp_server":    true,  // 新增
    "other":         true,
    "unknown":       true,
}
```

### 4.5 更新 LLM 系统提示词

**文件**: `api-server/internal/service/asset_analysis_service.go` (buildAnalysisPrompt)

在分类规则中新增：
```
- `llm_service`: LLM inference services and AI platforms. Examples: Ollama, vLLM, SGLang,
  LocalAI, llama.cpp, HuggingFace TGI, NVIDIA NIM, LiteLLM, Dify, Flowise, Langflow,
  Open WebUI, LibreChat, AnythingLLM, PrivateGPT. Look for processes listening on ports
  commonly used by LLM services (11434, 8000, 4000, 3000, 5000) with model-serving
  characteristics.

- `ai_agent`: AI agent frameworks and coding assistants. Examples: Claude Code, Claude Desktop,
  Cursor, Windsurf, VS Code with AI extensions, Gemini CLI, Amp, Kiro. Look for agent
  processes, their configuration files, and MCP client connections.

- `mcp_server`: Model Context Protocol servers. These are processes launched by AI agents to
  provide tools/resources. Look for processes spawned by agent frameworks with stdio-based
  communication, or servers listening on ports configured in MCP JSON configs.
```

### 4.4 更新 AssetSummary 模型和查询

**文件**: `api-server/internal/model/asset_collection.go`

```go
type AssetSummary struct {
    // ... 现有字段 ...
    LLMServiceCount int64     `json:"llm_service_count"`
    AIAgentCount    int64     `json:"ai_agent_count"`
    MCPServerCount  int64     `json:"mcp_server_count"`
}
```

**文件**: `api-server/internal/repository/asset_collection_repo.go` (GetSummary)

```go
r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "llm_service", "deleted").Count(&summary.LLMServiceCount)
r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "ai_agent", "deleted").Count(&summary.AIAgentCount)
r.db.Model(&model.HostApplicationAsset{}).Where("category = ? AND status != ?", "mcp_server", "deleted").Count(&summary.MCPServerCount)
```

## 5. 前端变更

### 5.1 路由新增

**文件**: `frontend/src/router/index.ts`

```typescript
{
  path: '/hosts/assets/llm-services',
  name: 'AssetsLLMServices',
  component: () => import('../views/hosts/Assets/Applications.vue'),
  meta: { title: 'AI LLM 资产' },
  props: { defaultCategory: 'llm_service' }
},
{
  path: '/hosts/assets/ai-agents',
  name: 'AssetsAIAgents',
  component: () => import('../views/hosts/Assets/Applications.vue'),
  meta: { title: 'AI Agent 资产' },
  props: { defaultCategory: 'ai_agent' }
},
{
  path: '/hosts/assets/mcp-servers',
  name: 'AssetsMCPServers',
  component: () => import('../views/hosts/Assets/Applications.vue'),
  meta: { title: 'MCP 资产' },
  props: { defaultCategory: 'mcp_server' }
},
```

### 5.2 侧边栏新增

**文件**: `frontend/src/App.vue`

在"Web 站点"菜单项后新增：
```vue
<el-menu-item index="/hosts/assets/llm-services">
  <el-icon><Cpu /></el-icon>
  <span>AI LLM</span>
</el-menu-item>
<el-menu-item index="/hosts/assets/ai-agents">
  <el-icon><Avatar /></el-icon>
  <span>AI Agent</span>
</el-menu-item>
<el-menu-item index="/hosts/assets/mcp-servers">
  <el-icon><Connection /></el-icon>
  <span>MCP</span>
</el-menu-item>
```

### 5.3 Overview 页面更新

**文件**: `frontend/src/views/hosts/Assets/Overview.vue`

1. **Stats Row**: 新增 3 个统计卡片（AI LLM、AI Agent、MCP）
2. **Category Grid**: 新增 3 个分类按钮
3. **Analysis Panel**: 新增 AI 相关指标

### 5.4 TypeScript 类型更新

**文件**: `frontend/src/api/assets.ts`

```typescript
export interface AssetSummary {
  // ... 现有字段 ...
  llm_service_count: number
  ai_agent_count: number
  mcp_server_count: number
}
```

### 5.5 Applications.vue 分类标签扩展

**文件**: `frontend/src/views/hosts/Assets/Applications.vue`

分类标签颜色映射新增：
```typescript
const categoryTagType: Record<string, string> = {
  // ... 现有 ...
  llm_service: 'primary',
  ai_agent: 'success',
  mcp_server: 'warning',
}

const categoryLabel: Record<string, string> = {
  // ... 现有 ...
  llm_service: 'AI LLM',
  ai_agent: 'AI Agent',
  mcp_server: 'MCP',
}
```

## 6. 影响范围

| 组件 | 变更类型 | 文件数 |
|------|---------|--------|
| agent (model) | 新增 AIAsset 结构体 + HostAssetSnapshot 扩展 | 1 |
| agent (collector) | 新增 LLMServiceCollector | 1 |
| agent (collector) | 新增 AIAgentCollector | 1 |
| agent (collector) | 新增 MCPCollector | 1 |
| agent (collector) | 接入 AssetCollector 编排 | 1 |
| api-server (service) | 解析 AIAssets 并存储 | 1 |
| api-server (model) | 扩展 AssetSummary 字段 | 1 |
| api-server (repository) | 扩展 GetSummary 查询 | 1 |
| api-server (service) | 扩展分类枚举 + LLM 提示词 | 1 |
| frontend (router) | 新增 3 条路由 | 1 |
| frontend (App.vue) | 新增 3 个侧边栏菜单项 | 1 |
| frontend (Overview) | 新增统计卡片 + 分类按钮 | 1 |
| frontend (api) | 扩展 AssetSummary 类型 | 1 |
| frontend (Applications) | 扩展分类标签映射 | 1 |
| **合计** | | **14 个文件** |

## 7. 数据库变更

**无 Schema 变更**。`host_application_assets.category` 字段为 `varchar(32)`，已有的存储空间足以容纳新分类值。只需确保新分类值在代码层面被正确识别。

## 8. 测试用例

### 8.1 Agent 侧测试

| 编号 | 测试项 | 验证点 |
|------|--------|--------|
| T-A1 | LLMServiceCollector | 对已知端口发送探测，正确识别 Ollama/vLLM 等服务 |
| T-A2 | AIAgentCollector | 扫描配置路径，正确识别 Claude Code/Cursor 等 Agent |
| T-A3 | MCPCollector | 解析 mcp.json，正确提取 MCP Server 定义 |
| T-A4 | AgentAsset 快照 | 3 个采集器结果正确写入 HostAssetSnapshot.AIAssets |

### 8.2 后端测试

| 编号 | 测试项 | 验证点 |
|------|--------|--------|
| T-B1 | AIAssets 解析 | collectHost 正确解析 AIAssets 并 Upsert 到 host_application_assets |
| T-B2 | AssetSummary 新字段 | llm_service_count, ai_agent_count, mcp_server_count 正确返回 |
| T-B3 | 分类过滤 | category=llm_service/ai_agent/mcp_server 查询返回正确结果 |
| T-B4 | 无效分类降级 | LLM 返回未知分类时降级为 unknown |

### 8.3 前端测试

| 编号 | 测试项 | 验证点 |
|------|--------|--------|
| T-F1 | 路由导航 | 3 个新路由可正常访问，defaultCategory 正确传递 |
| T-F2 | 侧边栏显示 | 3 个新菜单项显示，点击导航正确 |
| T-F3 | Overview 统计 | 3 个新统计卡片显示正确数值 |
| T-F4 | 分类网格 | 3 个新分类按钮显示，点击跳转正确 |
| T-F5 | 列表过滤 | 进入 AI LLM/AI Agent/MCP 页面只显示对应分类数据 |
| T-F6 | 分类标签 | 新分类以正确颜色标签显示 |

## 9. 回滚方案

1. Agent：移除 3 个新采集器和 AIAsset 模型
2. API-Server：移除 AIAssets 解析逻辑、分类枚举扩展、AssetSummary 字段
3. 前端：移除新增路由、菜单项、统计卡片
4. 数据库：无需回滚（无 Schema 变更），已采集的 AI 资产数据保留但不再展示

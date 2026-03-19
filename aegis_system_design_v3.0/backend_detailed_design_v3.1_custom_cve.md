# 后端详细设计文档 - V3.1 自定义CVE功能

**版本**: 3.1
**状态**: 定稿
**作者**: 安全产品团队
**日期**: 2026-03-19

---

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 3.1 | 2026-03-19 | 安全产品团队 | **新增自定义CVE功能**。新增`CustomCVEService`服务、`HostVulnerabilityScriptService`服务、扩展VulnerabilityHandler，支持自定义CVE查询入库和多主机脚本生成。 |
| 3.0 | 2026-03-13 | 安全产品团队 | 新增漏洞管理模块后端设计。 |
| 2.2 | 2026-03-12 | Sisyphus | 任务管理与超时机制增强。 |

---

## 2. 概述

本文档描述Aegis智能主机安全系统V3.1版本新增的**自定义CVE功能**后端实现设计。

### 2.1 新增模块

| 模块 | 描述 |
|:---|:---|
| `CustomCVEService` | 自定义CVE查询服务，负责CVE查询任务管理、LLM调用、状态追踪 |
| `HostVulnerabilityScriptService` | 主机漏洞脚本服务，负责多主机脚本生成和执行状态管理 |
| `CustomCVEQueryRepo` | 自定义CVE查询数据仓储层 |
| `HostVulnerabilityScriptRepo` | 主机漏洞脚本数据仓储层 |

### 2.2 扩展模块

| 模块 | 扩展内容 |
|:---|:---|
| `VulnerabilityHandler` | 新增自定义CVE查询相关API端点 |
| `VulnerabilityService` | 扩展支持多主机脚本生成和状态查询 |

---

## 3. API设计

### 3.1 新增API端点

| 方法 | 路径 | 描述 |
|:---|:---|:---|
| POST | `/api/v1/vulnerability/custom-query` | 启动自定义CVE查询 |
| GET | `/api/v1/vulnerability/custom-query/:id/status` | 获取查询状态 |
| GET | `/api/v1/vulnerability/custom-query/current` | 获取当前进行中的查询 |
| POST | `/api/v1/vulnerability/:cve_id/scripts/generate` | 批量生成脚本 |
| GET | `/api/v1/vulnerability/:cve_id/host-scripts` | 获取各主机脚本状态 |
| POST | `/api/v1/vulnerability/:cve_id/scripts/execute` | 执行已生成的脚本 |

### 3.2 API详细设计

#### 3.2.1 启动自定义CVE查询

**Handler方法**: `StartCustomQuery`

**请求参数**:
```go
type StartCustomQueryRequest struct {
    CveID string `json:"cve_id" binding:"required"`
}
```

**响应结构**:
```go
type StartCustomQueryResponse struct {
    QueryID string `json:"query_id"`
    CveID   string `json:"cve_id"`
    Status  string `json:"status"`
}
```

**处理流程**:
```
1. 验证CVE编号格式 (正则: ^CVE-\d{4}-\d{4,}$)
2. 检查是否已有查询进行中 (查询custom_cve_queries表中status='querying'的记录)
3. 检查CVE是否已存在于vulnerabilities表
4. 创建查询记录 (status='querying')
5. 异步执行LLM查询
6. 返回query_id
```

**错误处理**:
| 场景 | HTTP状态码 | 错误消息 |
|:---|:---|:---|
| CVE格式不正确 | 400 | "CVE编号格式不正确" |
| 已有查询进行中 | 400 | "已有CVE查询任务进行中" |
| CVE已存在 | 400 | "该CVE已存在于数据库中" |
| LLM未配置 | 500 | "LLM未配置" |

#### 3.2.2 获取查询状态

**Handler方法**: `GetCustomQueryStatus`

**响应结构**:
```go
type CustomQueryStatusResponse struct {
    QueryID       string                  `json:"query_id"`
    CveID         string                  `json:"cve_id"`
    Status        string                  `json:"status"` // querying/success/failed
    Progress      int                     `json:"progress"`
    Message       string                  `json:"message"`
    Vulnerability *VulnerabilityResponse  `json:"vulnerability,omitempty"`
    Error         string                  `json:"error,omitempty"`
}
```

#### 3.2.3 批量生成脚本

**Handler方法**: `GenerateHostScripts`

**请求参数**:
```go
type GenerateHostScriptsRequest struct {
    HostIDs    []string `json:"host_ids" binding:"required"`
    ScriptType string   `json:"script_type" binding:"required,oneof=poc fix"`
}
```

**响应结构**:
```go
type GenerateHostScriptsResponse struct {
    Scripts []HostScriptStatus `json:"scripts"`
}

type HostScriptStatus struct {
    HostID   string `json:"host_id"`
    ScriptID string `json:"script_id"`
    Status   string `json:"status"` // pending/generating/generated/failed
}
```

**处理流程**:
```
1. 验证CVE是否存在
2. 验证主机ID有效性
3. 为每个主机创建脚本记录 (generation_status='pending')
4. 异步启动脚本生成 (每个主机独立goroutine)
5. 返回各主机的初始状态
```

#### 3.2.4 获取各主机脚本状态

**Handler方法**: `GetHostScriptsStatus`

**查询参数**:
- `script_type`: poc 或 fix

**响应结构**:
```go
type HostScriptsStatusResponse struct {
    CveID      string          `json:"cve_id"`
    ScriptType string          `json:"script_type"`
    Hosts      []HostScript    `json:"hosts"`
    Summary    ScriptSummary   `json:"summary"`
}

type HostScript struct {
    HostID              string  `json:"host_id"`
    HostIP              string  `json:"host_ip"`
    Hostname            string  `json:"hostname"`
    OsType              string  `json:"os_type"`
    ScriptID            string  `json:"script_id,omitempty"`
    GenerationStatus    string  `json:"generation_status"`
    GenerationProgress  int     `json:"generation_progress,omitempty"`
    GenerationMessage   string  `json:"generation_message,omitempty"`
    ScriptContent       string  `json:"script_content,omitempty"`
}

type ScriptSummary struct {
    Total     int `json:"total"`
    Generated int `json:"generated"`
    Generating int `json:"generating"`
    Pending   int `json:"pending"`
    Failed    int `json:"failed"`
}
```

---

## 4. 服务层设计

### 4.1 CustomCVEService

**文件位置**: `backend/internal/service/custom_cve_service.go`

#### 4.1.1 结构定义

```go
type CustomCVEService struct {
    vulnRepo    *repository.VulnerabilityRepo
    queryRepo   *repository.CustomCVEQueryRepo
    configRepo  *repository.ConfigRepository
    redisClient *storage.RedisClient
    
    // 查询互斥锁
    queryMutex  sync.Mutex
    queryingCVE *string
    queryingID  *uuid.UUID
}
```

#### 4.1.2 核心方法

| 方法 | 签名 | 描述 |
|:---|:---|:---|
| `StartCustomQuery` | `func(ctx context.Context, cveID string) (*model.CustomCVEQuery, error)` | 启动自定义CVE查询 |
| `GetQueryStatus` | `func(queryID string) (*model.CustomCVEQuery, error)` | 获取查询状态 |
| `GetCurrentQuery` | `func() (*model.CustomCVEQuery, bool)` | 获取当前进行中的查询 |
| `executeQuery` | `func(ctx context.Context, query *model.CustomCVEQuery)` | 异步执行LLM查询（内部方法） |
| `buildCveQueryPrompt` | `func(cveID string) string` | 构建CVE查询Prompt（内部方法） |
| `parseCveQueryResult` | `func(response string) (*model.CveQueryResult, error)` | 解析LLM返回结果（内部方法） |

#### 4.1.3 核心流程

**启动查询流程**:
```go
func (s *CustomCVEService) StartCustomQuery(ctx context.Context, cveID string) (*model.CustomCVEQuery, error) {
    // 1. 检查互斥锁
    s.queryMutex.Lock()
    if s.queryingCVE != nil {
        defer s.queryMutex.Unlock()
        return nil, fmt.Errorf("已有CVE查询进行中: %s", *s.queryingCVE)
    }
    
    // 2. 检查CVE是否已存在
    existing, _ := s.vulnRepo.FindByCveID(cveID)
    if existing != nil {
        defer s.queryMutex.Unlock()
        return nil, fmt.Errorf("CVE %s 已存在于数据库中", cveID)
    }
    
    // 3. 创建查询记录
    query := &model.CustomCVEQuery{
        CveID:     cveID,
        Status:    "querying",
        StartedAt: time.Now(),
    }
    if err := s.queryRepo.Create(query); err != nil {
        s.queryMutex.Unlock()
        return nil, fmt.Errorf("创建查询记录失败: %w", err)
    }
    
    // 4. 设置查询中状态
    s.queryingCVE = &cveID
    s.queryingID = &query.ID
    s.queryMutex.Unlock()
    
    // 5. 异步执行查询
    go s.executeQuery(context.Background(), query)
    
    return query, nil
}
```

**异步执行查询**:
```go
func (s *CustomCVEService) executeQuery(ctx context.Context, query *model.CustomCVEQuery) {
    defer func() {
        s.queryMutex.Lock()
        s.queryingCVE = nil
        s.queryingID = nil
        s.queryMutex.Unlock()
    }()
    
    // 1. 获取LLM客户端
    llmClient, err := s.getLLMClient(ctx)
    if err != nil {
        s.markQueryFailed(query.ID, "LLM配置错误: "+err.Error())
        return
    }
    
    // 2. 构建查询Prompt
    userPrompt := s.buildCveQueryPrompt(query.CveID)
    
    // 3. 调用LLM (30秒超时)
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    response, err := llmClient.ChatCompletion(ctx, llm.CVEQueryPrompt, userPrompt, 0.3)
    if err != nil {
        s.markQueryFailed(query.ID, "LLM查询失败: "+err.Error())
        return
    }
    
    // 4. 解析结果
    result, err := s.parseCveQueryResult(response)
    if err != nil || !result.Found {
        s.markQueryFailed(query.ID, "未查询到该CVE信息")
        return
    }
    
    // 5. 入库
    vuln := s.convertToVulnerability(result)
    if err := s.vulnRepo.UpsertVulnerability(vuln); err != nil {
        s.markQueryFailed(query.ID, "保存CVE数据失败: "+err.Error())
        return
    }
    
    // 6. 更新查询状态为成功
    s.queryRepo.MarkSuccess(query.ID, vuln.ID)
}
```

### 4.2 HostVulnerabilityScriptService

**文件位置**: `backend/internal/service/host_vulnerability_script_service.go`

#### 4.2.1 结构定义

```go
type HostVulnerabilityScriptService struct {
    scriptRepo  *repository.HostVulnerabilityScriptRepo
    hostRepo    *repository.HostRepository
    vulnRepo    *repository.VulnerabilityRepo
    configRepo  *repository.ConfigRepository
    grpcServer  *grpc_server.GRPCServer
    taskService *TaskService
    
    // 并发控制
    semaphore chan struct{} // 限制同时生成的脚本数
}
```

#### 4.2.2 核心方法

| 方法 | 签名 | 描述 |
|:---|:---|:---|
| `GenerateScripts` | `func(ctx context.Context, cveID string, hostIDs []string, scriptType string) ([]*model.HostVulnerabilityScript, error)` | 批量生成脚本 |
| `GetHostScriptsStatus` | `func(ctx context.Context, cveID, scriptType string) (*HostScriptsStatusResponse, error)` | 获取各主机脚本状态 |
| `ExecuteScripts` | `func(ctx context.Context, cveID, scriptType string, hostIDs []string) (*ExecuteScriptsResponse, error)` | 执行已生成的脚本 |
| `generateSingleScript` | `func(ctx context.Context, script *model.HostVulnerabilityScript)` | 生成单个主机脚本（内部方法） |

#### 4.2.3 并发控制

```go
func NewHostVulnerabilityScriptService(...) *HostVulnerabilityScriptService {
    return &HostVulnerabilityScriptService{
        // ...
        semaphore: make(chan struct{}, 5), // 最多同时生成5个脚本
    }
}

func (s *HostVulnerabilityScriptService) generateSingleScript(ctx context.Context, script *model.HostVulnerabilityScript) {
    // 获取信号量
    s.semaphore <- struct{}{}
    defer func() { <-s.semaphore }()
    
    // 实际生成逻辑...
}
```

---

## 5. 数据仓储层设计

### 5.1 CustomCVEQueryRepo

**文件位置**: `backend/internal/repository/custom_cve_query_repo.go`

#### 5.1.1 核心方法

| 方法 | 签名 | 描述 |
|:---|:---|:---|
| `Create` | `func(query *model.CustomCVEQuery) error` | 创建查询记录 |
| `FindByID` | `func(id uuid.UUID) (*model.CustomCVEQuery, error)` | 按ID查询 |
| `FindQuerying` | `func() (*model.CustomCVEQuery, error)` | 查询当前进行中的任务 |
| `MarkSuccess` | `func(id, vulnerabilityID uuid.UUID) error` | 标记查询成功 |
| `MarkFailed` | `func(id uuid.UUID, errMsg, errDetail string) error` | 标记查询失败 |

#### 5.1.2 查询互斥实现

```go
func (r *CustomCVEQueryRepo) FindQuerying() (*model.CustomCVEQuery, error) {
    var query model.CustomCVEQuery
    result := r.db.Where("status = ?", "querying").First(&query)
    if result.Error != nil {
        if errors.Is(result.Error, gorm.ErrRecordNotFound) {
            return nil, nil
        }
        return nil, result.Error
    }
    return &query, nil
}
```

### 5.2 HostVulnerabilityScriptRepo

**文件位置**: `backend/internal/repository/host_vulnerability_script_repo.go`

#### 5.2.1 核心方法

| 方法 | 签名 | 描述 |
|:---|:---|:---|
| `Create` | `func(script *model.HostVulnerabilityScript) error` | 创建脚本记录 |
| `FindByID` | `func(id uuid.UUID) (*model.HostVulnerabilityScript, error)` | 按ID查询 |
| `FindByCveAndHost` | `func(cveID string, hostID uuid.UUID, scriptType string) (*model.HostVulnerabilityScript, error)` | 按CVE和主机查询 |
| `FindByCveID` | `func(cveID, scriptType string) ([]model.HostVulnerabilityScript, error)` | 按CVE查询所有主机脚本 |
| `UpdateGenerationStatus` | `func(id uuid.UUID, status, errMsg, errDetail string) error` | 更新生成状态 |
| `UpdateScriptContent` | `func(id uuid.UUID, content string) error` | 更新脚本内容 |
| `UpdateExecutionStatus` | `func(id uuid.UUID, status string, taskID uuid.UUID) error` | 更新执行状态 |

---

## 6. LLM Prompt设计

### 6.1 CVE查询Prompt

**系统提示词** (`backend/internal/llm/prompts.go`):

```go
const CVEQueryPrompt = `你是一个专业的CVE漏洞信息查询助手。你的任务是根据用户提供的CVE编号，返回该漏洞的详细信息。

要求：
1. 准确返回CVE的官方信息，包括严重程度、CVSS评分、漏洞描述
2. 列出所有受影响的产品和版本
3. 提供官方修复版本和解决方案
4. 包含官方参考链接（NVD、供应商公告等）
5. 如果CVE不存在或信息不可用，明确返回found=false

输出格式：严格的JSON格式，不要包含任何其他文字。`
```

**用户提示词模板**:

```go
func (s *CustomCVEService) buildCveQueryPrompt(cveID string) string {
    return fmt.Sprintf(`请查询并返回以下CVE的详细信息：

CVE编号: %s

请返回以下JSON格式的信息：
{
    "found": true/false,
    "cve_id": "CVE-XXXX-XXXXX",
    "severity": "Critical/High/Medium/Low",
    "cvss_score": 数字(0.0-10.0),
    "description": "漏洞详细描述",
    "affected_products": [
        {
            "product": "产品名称",
            "vendor": "供应商",
            "versions": ["受影响版本列表"],
            "fixed_versions": ["修复版本列表"]
        }
    ],
    "solution": "修复建议",
    "references": ["参考链接"],
    "cwe_id": "CWE-XXX"
}

如果该CVE不存在或无法查询到信息，请返回 {"found": false}`, cveID)
}
```

### 6.2 POC脚本生成Prompt

**扩展现有Prompt**，增加对自定义CVE的支持：

```go
const POCVerificationPromptForCustomCVE = `你是一个专业的安全脚本编写专家。你的任务是为指定的CVE漏洞编写安全的POC（概念验证）脚本。

重要说明：
1. 此CVE为用户自定义添加的漏洞，可能没有具体的受影响主机信息
2. 脚本应该适用于指定的操作系统类型
3. POC脚本必须是安全的、非破坏性的，仅用于验证漏洞是否存在

输出格式：完整的Shell脚本，以#!/bin/bash开头。`
```

---

## 7. 路由配置

### 7.1 新增路由

**文件位置**: `backend/internal/api/router.go`

```go
// 自定义CVE查询路由
vulnerabilityGroup.POST("/custom-query", vulnerabilityHandler.StartCustomQuery)
vulnerabilityGroup.GET("/custom-query/:id/status", vulnerabilityHandler.GetCustomQueryStatus)
vulnerabilityGroup.GET("/custom-query/current", vulnerabilityHandler.GetCurrentQuery)

// 多主机脚本管理路由
vulnerabilityGroup.POST("/:cve_id/scripts/generate", vulnerabilityHandler.GenerateHostScripts)
vulnerabilityGroup.GET("/:cve_id/host-scripts", vulnerabilityHandler.GetHostScriptsStatus)
vulnerabilityGroup.POST("/:cve_id/scripts/execute", vulnerabilityHandler.ExecuteScripts)
```

---

## 8. 错误处理

### 8.1 错误码定义

**文件位置**: `backend/internal/errors/codes.go`

```go
const (
    ErrCVEQueryInProgress    = "CVE_QUERY_IN_PROGRESS"
    ErrCVENotFound           = "CVE_NOT_FOUND"
    ErrCVEAlreadyExists      = "CVE_ALREADY_EXISTS"
    ErrCVEInvalidFormat      = "CVE_INVALID_FORMAT"
    ErrScriptGenerationFailed = "SCRIPT_GENERATION_FAILED"
    ErrScriptNotGenerated    = "SCRIPT_NOT_GENERATED"
)
```

### 8.2 错误响应格式

```go
type ErrorResponse struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data,omitempty"`
}

func NewErrorResponse(code, message string, data any) *ErrorResponse {
    return &ErrorResponse{
        Code:    code,
        Message: message,
        Data:    data,
    }
}
```

---

## 9. 测试用例

### 9.1 单元测试

**测试文件**: `backend/internal/service/custom_cve_service_test.go`

```go
func TestStartCustomQuery_ValidCveID(t *testing.T) {
    // 测试正常流程
}

func TestStartCustomQuery_InvalidFormat(t *testing.T) {
    // 测试CVE格式校验
}

func TestStartCustomQuery_QueryInProgress(t *testing.T) {
    // 测试查询互斥
}

func TestStartCustomQuery_CveAlreadyExists(t *testing.T) {
    // 测试CVE已存在的情况
}
```

### 9.2 集成测试

**测试文件**: `backend/internal/api/handler/vulnerability_handler_test.go`

```go
func TestStartCustomQueryAPI(t *testing.T) {
    // 测试API端点
}

func TestGenerateHostScriptsAPI(t *testing.T) {
    // 测试多主机脚本生成
}

func TestExecuteScriptsAPI(t *testing.T) {
    // 测试脚本执行
}
```

---

## 10. 文件结构

```
backend/
├── internal/
│   ├── api/
│   │   ├── handler/
│   │   │   └── vulnerability_handler.go  (扩展)
│   │   └── router.go                     (扩展)
│   ├── model/
│   │   ├── custom_cve_query.go           (新增)
│   │   └── host_vulnerability_script.go  (新增)
│   ├── repository/
│   │   ├── custom_cve_query_repo.go      (新增)
│   │   └── host_vulnerability_script_repo.go (新增)
│   ├── service/
│   │   ├── custom_cve_service.go         (新增)
│   │   └── host_vulnerability_script_service.go (新增)
│   └── llm/
│       └── prompts.go                    (扩展)
├── scripts/
│   └── migrate_v3.1_custom_cve.sql       (新增)
└── test/
    └── ...
```

---

**文档结束**
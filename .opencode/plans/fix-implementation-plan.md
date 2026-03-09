# 修复实施计划

## 当前状态
- ✅ 任务 1.1 完成：config.yaml 超时改为 120 秒
- ⏸️ 剩余 15 项任务等待执行

## 问题1: LLM超时配置优化

### 任务 1.2: 修改 template_service.go
**文件**: `/code/ai-benchmark/backend/internal/service/template_service.go`
**行号**: 156
**修改内容**:
```go
// 修改前:
llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, 30, 3)

// 修改后:
llmClient := llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmMaxRetries)
```

**依赖**: TemplateService 结构体已有 llmTimeout 和 llmMaxRetries 字段

### 任务 1.3: 优化重试逻辑
当前退避策略: 1s, 2s, 4s
建议保持不变，因为总重试时间 (1+2+4=7s) 远小于超时时间 120s

## 问题2: API Key脱敏显示

### 任务 2.1: 修改 config_handler.go
**文件**: `/code/ai-benchmark/backend/internal/api/handler/config_handler.go`
**新增方法**:
```go
func (h *ConfigHandler) GetFullAPIKey(c *gin.Context) {
    config, err := h.configRepo.GetActive()
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "code":    500,
            "message": "failed to get config",
        })
        return
    }

    apiKey, err := h.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "code":    500,
            "message": "failed to decrypt api key",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "code":    0,
        "message": "success",
        "data": gin.H{
            "api_key": apiKey,
        },
    })
}
```

### 任务 2.2: 修改 router.go
**文件**: `/code/ai-benchmark/backend/internal/api/router.go`
**新增路由**:
```go
config.GET("/llm/full-key", r.configHandler.GetFullAPIKey)
```

### 任务 2.3: 修改前端 store
**文件**: `/code/ai-benchmark/frontend/src/store/config.ts`
**新增方法**:
```typescript
async getFullAPIKey(): Promise<{ api_key: string }> {
  const response = await fetch('/api/v1/config/llm/full-key')
  const data = await response.json()
  if (data.code !== 0) {
    throw new Error(data.message)
  }
  return data.data
}
```

### 任务 2.4: 修改 Settings.vue
**文件**: `/code/ai-benchmark/frontend/src/views/Settings.vue`
添加眼睛图标切换功能...

## 问题3: Agent日志系统

### 任务 3.1: 修改 go.mod
添加 zap 依赖

### 任务 3.2: 创建 logger.go
创建日志模块

### 任务 3.3-3.5: 替换 fmt.Println
在所有 agent 文件中使用日志库

## 待办事项
- [ ] 等待 Plan Mode 解除或权限调整
- [ ] 执行剩余 15 项任务

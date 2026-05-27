# AI生成检测包草稿功能修复设计

## 日期
2026-05-26

## 问题概述

动态检测包 → 新建检测包 → AI生成草稿功能完全失效。用户点击"AI生成草稿"后，返回的内容全部是占位符（如 `# AI generated HookPlan - to be implemented`），LLM从未被真正调用。

## 根因分析

### Bug 1: 超时单位错误（致命）

**文件**: `api-server/internal/api/handler/detection_package_handler.go:108`

```go
ctx, cancel := context.WithTimeout(c.Request.Context(), 120)
```

`time.Duration` 的默认单位是纳秒，`120` 表示120纳秒而非120秒。LLM调用在120纳秒后必定超时，导致 `ChatCompletion` 返回 `context deadline exceeded` 错误。

### Bug 2: LLM失败静默降级（严重）

**文件**: `api-server/internal/api/handler/detection_package_handler.go:111-116`

```go
if err != nil {
    logger.Warn("LLM generation failed, using placeholders", zap.Error(err))
} else {
    hookPlanYAML, ebpfSource, sigmaRulesYAML, correlationYAML = parseLLMResponse(response)
}
```

LLM调用失败时只打Warn日志，然后使用预定义的占位符字符串作为返回值。前端完全无感知，用户以为AI生成了内容但实际是空壳。

### Bug 3: 前端字段未传给LLM prompt（中等）

**文件**: `api-server/internal/api/handler/detection_package_handler.go:102-107`

前端AI对话框有 `attack_prerequisites` 字段，后端handler也接收了该字段（`req.AttackPrerequisites`），但调用 `GetDetectionPackageGenerationPrompt` 时没有传入，导致LLM缺少关键的攻击前置条件信息。

## 修复方案

### 修复1: 超时单位

```go
// Before
ctx, cancel := context.WithTimeout(c.Request.Context(), 120)

// After
ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
```

需要在文件顶部确认 `import "time"` 已存在。

### 修复2: LLM失败返回错误

将静默降级改为返回HTTP 503错误：

```go
if h.llmClient != nil {
    prompt := llm.GetDetectionPackageGenerationPrompt(
        req.CVEID,
        req.VulnerabilityDescription,
        req.AttackPrerequisites,
        req.ExploitationChain,
        req.FalsePositiveConstraints,
    )
    ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
    defer cancel()
    response, err := h.llmClient.ChatCompletion(ctx, "", prompt, 0.7)
    if err != nil {
        logger.Error("LLM generation failed", zap.Error(err))
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "code":    503,
            "message": fmt.Sprintf("AI生成失败: %s，请检查LLM配置或稍后重试", err.Error()),
        })
        return
    }
    hookPlanYAML, ebpfSource, sigmaRulesYAML, correlationYAML = parseLLMResponse(response)
}
```

移除预定义的占位符变量初始化，改为空字符串初始化。如果 `h.llmClient == nil`，直接返回503错误。

### 修复3: 传递 attack_prerequisites

更新 `GetDetectionPackageGenerationPrompt` 函数签名：

```go
// Before
func GetDetectionPackageGenerationPrompt(cveID, description, chain, constraints string) string {
    return fmt.Sprintf(DetectionPackageGenerationPrompt, cveID, description, chain, constraints)
}

// After
func GetDetectionPackageGenerationPrompt(cveID, description, prerequisites, chain, constraints string) string {
    return fmt.Sprintf(DetectionPackageGenerationPrompt, cveID, description, prerequisites, chain, constraints)
}
```

更新 `DetectionPackageGenerationPrompt` 模板，增加攻击前置条件：

```
CVE 信息：
%s

漏洞描述：
%s

攻击前置条件：
%s

利用链行为：
%s

误报约束：
%s
```

### 修复4: LLM响应解析增强

当前 `extractCodeBlock` 只匹配 `# Section Header` + ` ```code block``` ` 格式。增强为：

1. 优先匹配带section hint的代码块（现有逻辑）
2. Fallback: 按代码块语言标记顺序分配（第1个yaml→HookPlan，第1个c→eBPF，第2个yaml→Sigma，第3个yaml→Correlation）
3. 增加对 `## HookPlan` 等markdown标题的匹配

### 修复5: 前端错误反馈

`PackageEditor.vue` 的 `confirmAIGenerate` 函数中，当API返回非0 code时显示错误消息：

```ts
const draft = await generateDraft(aiForm)
if (!draft || draft.code !== undefined && draft.code !== 0) {
    ElMessage.error(draft?.message || 'AI 草稿生成失败')
    return
}
```

### 修复6: 设计文档更新

更新 `docs/aegis_system_design_v5.8/` 中相关文档，记录：
- AI生成流程的正确行为（LLM失败时返回错误而非占位符）
- 超时配置应为120秒
- AI生成输入字段完整列表

## 影响范围

| 文件 | 修改类型 |
|------|----------|
| `api-server/internal/api/handler/detection_package_handler.go` | Bug修复 + 逻辑改进 |
| `api-server/internal/llm/prompts.go` | 函数签名 + 模板更新 |
| `frontend/src/views/detection/DetectionPackages/PackageEditor.vue` | 错误反馈 |
| `frontend/src/views/detection/DetectionPackages/composables/useDetectionPackages.ts` | 错误处理 |
| `docs/aegis_system_design_v5.8/` | 文档更新 |

## 测试计划

1. 使用curl调用 `POST /api/v1/detection/packages/ai-generate` 接口，传入CVE-2026-31431信息
2. 验证LLM被正确调用，返回包含4段代码块的完整草稿
3. 验证LLM配置缺失时返回503错误而非占位符
4. 验证前端AI生成失败时显示错误提示
5. 验证 `attack_prerequisites` 字段被正确传入LLM prompt

## 安全边界

本次修复不改变AI生成的安全边界：
- AI仍然只生成草稿，不触发构建、签名、启用
- 草稿仍需人工修改、builder编译、人工审核、人工签名
- Ed25519签名验证机制不受影响

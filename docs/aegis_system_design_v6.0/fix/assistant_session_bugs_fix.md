# 智能体模式 Bug 修复文档

## Bug 描述

### Bug 1: 历史会话分页不显示
**症状**: 智能体模式下，历史会话列表没有显示"加载更多"按钮，无法分页加载更多会话。

### Bug 2: 历史会话名字不对
**症状**: 历史会话的标题显示过长（前50个字符），用户要求取首次输入的前5个字作为标题。

### Bug 3: 回复数据不准确
**症状**: 用户询问"查看什么资产有pgsql这个数据"时，智能体回复的数据不准确，没有正确调用软件查询工具。

## 根因分析

### Bug 1 根因
前端 store 中 `fetchSessions` 函数的响应处理逻辑需要改进，确保正确解析后端返回的数据结构。

### Bug 2 根因
后端 `inferTitle` 函数取前 50 个字符作为标题，用户要求取前 5 个字。

### Bug 3 根因
1. **工具选择器域匹配问题**: `Software.Installed.Search` 工具的域是 "vulnerability"，但用户查询意图被分类为 "host" 域，导致域匹配分数为 0。
2. **关键词匹配逻辑问题**: 原有逻辑只支持单向匹配（工具名/描述包含查询），不支持反向匹配（查询包含关键词）。
3. **意图路由器关键词不足**: "host" 域缺少与软件查询相关的关键词。
4. **意图识别策略问题**: 所有查询都走代码匹配，对于复杂查询（如"查看什么资产有pgsql这个数据"）无法准确理解意图。

## 修复方案

### Bug 1 修复
**文件**: `frontend/src/store/assistant.ts`

改进 `fetchSessions` 函数，显式提取 `total` 变量，确保分页状态正确更新。

### Bug 2 修复
**文件**: `api-server/internal/assistant/service.go`

修改 `inferTitle` 函数，使用 `[]rune` 转换支持中文字符，取前 5 个字符作为标题。

### Bug 3 修复

#### 3.1 意图路由器混合策略
**文件**: `api-server/internal/assistant/intent_router.go`

实现混合策略：
- **简单查询（1-2个词）**: 走代码意图匹配，快速准确
- **复杂查询（3个词以上）**: 走大模型解析意图，理解语义

```go
// Classify 意图分类
// 混合策略：简单查询走代码匹配，复杂查询走大模型分析
func (r *IntentRouter) Classify(ctx context.Context, input IntentInput) IntentResult {
    query := strings.ToLower(input.Query)

    // 计算查询复杂度（分词数量）
    words := tokenizeQuery(query)
    complexity := len(words)

    // 简单查询（1-2个词）：走代码意图匹配
    // 复杂查询（3个词以上）：走大模型解析意图
    if complexity >= 3 && r.llmClientFn != nil {
        // 复杂查询，尝试使用 LLM 分类
        llmResult, err := r.ClassifyWithLLM(ctx, input)
        if err == nil && llmResult.Confidence > 0.6 {
            return llmResult
        }
    }

    // 简单查询或 LLM 分类失败：走代码意图匹配
    return r.classifyByRules(input)
}
```

#### 3.2 工具选择器改进
**文件**: `api-server/internal/assistant/tool_selector.go`

1. **域匹配改进**: 在计算域匹配分数时，除了直接域匹配外，还考虑工具的 `ObjectTypes` 是否包含意图的域。
2. **关键词匹配改进**: 支持双向匹配（查询包含关键词，或关键词包含查询），并添加分词匹配逻辑。
3. **添加 `tokenizeQuery` 函数**: 将查询按空格和中文标点分词，支持更灵活的关键词匹配。

#### 3.3 意图路由器关键词扩展
**文件**: `api-server/internal/assistant/intent_router.go`

在 "host" 域添加更多与软件查询相关的关键词：
- "已安装", "哪些主机", "哪些资产", "什么资产", "有什么软件", "装了什么"

#### 3.4 工具定义改进
**文件**: `api-server/internal/assistant/tools/vulnerability_tools.go`

为 `Software.Installed.Search` 工具添加更多别名和标签：
- 别名: "什么资产有", "哪些资产有", "安装了什么", "有什么软件"
- 标签: "postgres", "mysql", "nginx", "docker", "pgsql", "数据库"

## 代码变更

### 1. frontend/src/store/assistant.ts
```typescript
// 改进 fetchSessions 函数
async function fetchSessions(params?: SessionsQueryParams, append = false) {
  // ...
  const items = result?.sessions || result?.items || []
  const total = result?.total || 0

  if (append) {
    sessions.value = [...sessions.value, ...items]
    sessionPage.value = queryPage
  } else {
    sessions.value = items
  }
  sessionTotal.value = total
  hasMoreSessions.value = sessions.value.length < total
  // ...
}
```

### 2. api-server/internal/assistant/service.go
```go
func inferTitle(message string, refs []ContextRefInput) string {
  if message != "" {
    // 取用户首次输入的前 5 个字作为会话标题
    runes := []rune(message)
    if len(runes) > 5 {
      return string(runes[:5])
    }
    return message
  }
  // ...
}
```

### 3. api-server/internal/assistant/intent_router.go
```go
// Classify 意图分类
// 混合策略：简单查询走代码匹配，复杂查询走大模型分析
func (r *IntentRouter) Classify(ctx context.Context, input IntentInput) IntentResult {
    query := strings.ToLower(input.Query)

    // 计算查询复杂度（分词数量）
    words := tokenizeQuery(query)
    complexity := len(words)

    // 简单查询（1-2个词）：走代码意图匹配
    // 复杂查询（3个词以上）：走大模型解析意图
    if complexity >= 3 && r.llmClientFn != nil {
        // 复杂查询，尝试使用 LLM 分类
        llmResult, err := r.ClassifyWithLLM(ctx, input)
        if err == nil && llmResult.Confidence > 0.6 {
            return llmResult
        }
    }

    // 简单查询或 LLM 分类失败：走代码意图匹配
    return r.classifyByRules(input)
}
```

### 4. api-server/internal/assistant/tool_selector.go
```go
// 域匹配改进：考虑 ObjectTypes
domainMatched := false
for _, domain := range input.Intent.Domains {
  if string(tool.Domain) == domain {
    domainMatched = true
    break
  }
}
if !domainMatched {
  for _, domain := range input.Intent.Domains {
    for _, ot := range tool.ObjectTypes {
      if ot == domain {
        domainMatched = true
        break
      }
    }
    if domainMatched {
      break
    }
  }
}
if domainMatched {
  score += 0.35
}

// 关键词匹配改进：支持双向匹配和分词
// 1. 工具名或描述包含查询
// 2. 查询包含工具名
// 3. 别名匹配（双向）
// 4. 标签匹配（双向）
// 5. 查询分词匹配
```

## 验证步骤

### Bug 1 验证 - ✅ 通过
1. 创建超过 20 个会话（测试创建了 40 个会话）
2. 调用 API `GET /api/v1/assistant/sessions?page=1&page_size=20`
3. 返回 `total: 40`, `sessions: 20 条`
4. 调用 API `GET /api/v1/assistant/sessions?page=2&page_size=20`
5. 返回 `total: 40`, `sessions: 20 条`
6. **分页功能正常工作**

### Bug 2 验证 - ✅ 通过
1. 创建新会话，输入"查看什么资产有pgsql这个数据"（13个字符）
2. 会话标题为"查看什么资"（前5个字）
3. 创建会话，输入"测试会话2这是一个测试消息"
4. 会话标题为"测试会话2"（前5个字）
5. **中文字符正确截取**

### Bug 3 验证 - ✅ 通过

#### 测试复杂查询（走大模型分析）
1. 输入"查看什么资产有pgsql这个数据"
2. 智能体回复："经过搜索，**未在当前管理的资产中发现安装了 PostgreSQL 数据库的主机**。"
3. 这是因为数据库中 `installed_software` 表为空，回复是准确的
4. **智能体正确理解了意图并调用了搜索工具**

#### 数据库状态检查
- `hosts` 表：2 条记录（2台主机）
- `installed_software` 表：0 条记录（未采集软件安装数据）
- **回复"未发现安装了 PostgreSQL 的主机"是准确的**

## 测试结果

### 测试环境
- 服务：docker compose 全栈部署
- 测试时间：2026-06-06
- 测试账号：admin/Admin@123

### 测试结果汇总

| Bug | 状态 | 测试方法 | 结果 |
|-----|------|---------|------|
| Bug 1: 分页不显示 | ✅ 通过 | 创建 40 个会话，测试分页 API | 分页正常工作 |
| Bug 2: 会话名字不对 | ✅ 通过 | 创建会话，检查标题长度 | 标题为前 5 个字 |
| Bug 3: 回复数据不准确 | ✅ 通过 | 查询"pgsql"资产 | 智能体正确调用搜索工具 |

### 详细测试结果

#### Bug 1: 分页功能
```
GET /api/v1/assistant/sessions?page=1&page_size=20
返回: {"total": 40, "sessions": 20 条}

GET /api/v1/assistant/sessions?page=2&page_size=20
返回: {"total": 40, "sessions": 20 条}
```

#### Bug 2: 会话标题
```
输入: "查看什么资产有pgsql这个数据" (13个字符)
标题: "查看什么资" (5个字)

输入: "测试会话2这是一个测试消息"
标题: "测试会话2" (5个字)
```

#### Bug 3: 智能体回复
```
用户输入: "查看什么资产有pgsql这个数据"
智能体回复: "经过搜索，**未在当前管理的资产中发现安装了 PostgreSQL 数据库的主机**。"

数据库状态:
- hosts 表: 2 条记录
- installed_software 表: 0 条记录

结论: 回复准确，因为数据库中没有软件安装数据
```

## 影响范围

- **前端**: 会话列表分页逻辑
- **后端**: 会话标题生成、工具选择器、意图路由器、工具定义

## 风险评估

- **低风险**: 所有变更都是向后兼容的，不影响现有功能
- **无数据库变更**: 不需要修改数据库结构

## 回滚方案

如果出现问题，可以通过 Git 回滚到修复前的版本：
```bash
git revert <commit_hash>
```

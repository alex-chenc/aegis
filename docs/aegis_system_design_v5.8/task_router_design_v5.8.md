# TaskRouter 智能任务路由设计文档（V2）

## 1. 问题陈述

当前 agent-runtime 的提示词是整块的（plan/react/summarize），无法灵活组合。需要：
1. 提示词拆分为独立模块（片段）
2. Router 根据任务类型选择需要的提示词片段
3. 运行时动态拼接片段，提供给 LLM
4. `enable_llm_routing` 和计划生成逻辑联动调整

### 需求
- 简单对话 → 规则直接判断，不调 LLM
- 简单工具调用 → 规则直接判断，跳过计划
- 复杂任务 → LLM 语义分析 → 选择提示词片段 → 拼接 → 执行

## 2. 提示词模块化设计

### 2.1 提示词片段（PromptFragment）

每个片段是一个独立的提示词单元，有自己的名称、描述、关键词和内容：

```go
// core/types.go

// PromptFragment 提示词片段
type PromptFragment struct {
    Name        string   `json:"name"`         // 唯一名称
    Description string   `json:"description"`  // 功能描述（供 LLM 选择时参考）
    Keywords    []string `json:"keywords"`     // 匹配关键词
    Priority    int      `json:"priority"`     // 优先级（数字越大越靠前）
    Content     string   `json:"content"`      // 提示词内容
}
```

### 2.2 片段分类

| 片段名称 | 用途 | 示例内容 |
|---------|------|---------|
| `base_assistant` | 基础身份 | "你是 Aegis 智能安全助手..." |
| `plan_decision` | 判断是否需要计划 | "评估任务复杂度，如果少于3步..." |
| `security_analysis` | 安全分析 | "分析安全事件，追溯攻击路径..." |
| `host_query` | 主机查询 | "查询主机资产信息..." |
| `vulnerability_mgmt` | 漏洞管理 | "管理漏洞扫描和修复..." |
| `alert_triage` | 告警研判 | "分析告警，判断误报..." |
| `react_format` | ReAct 输出格式 | "严格输出 JSON 格式..." |
| `tool_usage` | 工具使用规范 | "使用以下工具完成任务..." |

### 2.3 片段组合示例

```
用户："分析这台主机的攻击链"
Router 选择：base_assistant + plan_decision + security_analysis + react_format
拼接结果：base_assistant + security_analysis + plan_decision + react_format

用户："查看主机列表"
Router 选择：base_assistant + host_query + react_format
拼接结果：base_assistant + host_query + react_format（跳过 plan_decision）

用户："你好"
Router 选择：（规则匹配，不走 Router）
直接回复
```

## 3. TaskRouter 架构

### 3.1 核心结构

```go
// router/router.go

type Router struct {
    llmClient     core.LLMClient
    fragments     []PromptFragment    // 所有可用片段
    config        RouterConfig
}

type RouterConfig struct {
    EnableLLMRouting   bool          `json:"enable_llm_routing"`   // 是否启用 LLM 路由
    LLMTemperature     float64       `json:"llm_temperature"`      // LLM 分类温度
    LLMTimeout         time.Duration `json:"llm_timeout"`          // LLM 分类超时
    DirectReplyMaxLen  int           `json:"direct_reply_max_len"` // 直接回复最大消息长度
}
```

### 3.2 路由流程

```go
// Route 路由入口
func (r *Router) Route(ctx context.Context, input RouteInput) (*RouteResult, error)

type RouteInput struct {
    TaskID      string
    UserMessage string
    Tools       []core.ToolDescriptor
    MaxSteps    int
}

type RouteResult struct {
    Action          TaskAction         // 动作：direct_reply / simple_call / full_plan
    Classification  *TaskClassification // LLM 分类结果（规则匹配时为 nil）
    SelectedFragments []string         // 选中的片段名称列表
    ComposedPrompt  string             // 拼接后的完整提示词
}
```

### 3.3 第一级：规则匹配（不调 LLM）

```go
func (r *Router) matchRules(message string) (TaskAction, []string, bool) {
    // 问候/闲聊 → direct_reply，不选片段
    if isGreeting(message) {
        return ActionDirectReply, nil, true
    }

    // 简单查询 → simple_call，选基础片段
    if isSimpleQuery(message) {
        return ActionSimpleCall, []string{"base_assistant", "react_format"}, true
    }

    return "", nil, false
}
```

### 3.4 第二级：LLM 语义分析 + 片段选择

**关键：分类和片段选择在一次 LLM 调用中完成。**

```go
func (r *Router) classify(ctx context.Context, input RouteInput) (*TaskClassification, []string, error) {
    // 构建片段目录摘要（名称 + 描述 + 关键词）
    catalogSummary := r.buildCatalogSummary()

    systemPrompt := fmt.Sprintf(`你是任务分类器和提示词选择器。分析用户消息，完成两个任务：
1. 分类任务类型和复杂度
2. 从提示词目录中选择需要的片段

## 输出格式
{
  "task_type": "类型",
  "intent": "意图描述",
  "complexity": "simple/moderate/complex",
  "action": "direct_reply/simple_call/full_plan",
  "selected_fragments": ["片段名1", "片段名2"],
  "reason": "选择原因"
}

## 任务类型
- greeting: 问候、闲聊
- query: 简单数据查询
- analysis: 安全分析
- investigation: 攻击调查
- remediation: 修复操作

## 复杂度判断
- simple: 1-2步可完成 → action: simple_call
- moderate/complex: 3步以上 → action: full_plan

## 可用提示词目录
%s

## 选择规则
- 必须选择 base_assistant（基础身份）
- 根据任务类型选择对应的功能片段
- 必须选择 react_format（输出格式规范）
- 不要选择与任务无关的片段
- 选择 2-5 个片段`, catalogSummary)

    userPrompt := fmt.Sprintf("用户消息：%s\n可用工具数：%d", input.UserMessage, len(input.Tools))

    // LLM 调用...
}
```

### 3.5 第三级：片段拼接

```go
// compose 按优先级拼接选中的片段
func (r *Router) compose(selectedNames []string) string {
    var selected []PromptFragment
    for _, name := range selectedNames {
        for _, f := range r.fragments {
            if f.Name == name {
                selected = append(selected, f)
                break
            }
        }
    }

    // 按优先级排序
    sort.Slice(selected, func(i, j int) bool {
        return selected[i].Priority > selected[j].Priority
    })

    // 拼接
    var buf strings.Builder
    for i, f := range selected {
        if i > 0 {
            buf.WriteString("\n\n")
        }
        buf.WriteString(f.Content)
    }
    return buf.String()
}
```

## 4. Runtime 集成

### 4.1 流程变更

```go
// runtime.go Run() 方法

// 原来：
// assess, _ := p.Assess(ctx, planInput)
// if !assess.NeedsPlan { ... }

// 改为：
routeResult, _ := r.router.Route(ctx, router.RouteInput{...})

switch routeResult.Action {
case router.ActionDirectReply:
    // 问候：直接回复
    return r.handleDirectReply(ctx, input, taskCtx, routeResult)

case router.ActionSimpleCall:
    // 简单调用：用拼接的提示词，单步骤执行
    initialPlan = p.GenerateNoPlan(planInput)
    // 将拼接的提示词注入到 ReAct 执行器
    r.injectComposedPrompt(routeResult.ComposedPrompt)

case router.ActionFullPlan:
    // 完整流程：用拼接的提示词生成计划
    initialPlan, _ = p.Generate(ctx, planInput)
    r.injectComposedPrompt(routeResult.ComposedPrompt)
}
```

### 4.2 提示词注入

```go
// 通过 PromptProvider 注入拼接后的提示词
// AssistantPromptProvider.Build() 检查是否有注入的拼接提示词
// 如果有，直接返回拼接结果；如果没有，走原来的逻辑

type ComposedPromptProvider struct {
    base      PromptProvider       // 原始 PromptProvider
    composed  string               // Router 拼接的提示词
    mu        sync.RWMutex
}

func (p *ComposedPromptProvider) Build(ctx context.Context, req PromptRequest) (PromptBundle, error) {
    p.mu.RLock()
    composed := p.composed
    p.mu.RUnlock()

    if composed != "" && req.Purpose == PurposeReact {
        return PromptBundle{SystemPrompt: composed}, nil
    }
    return p.base.Build(ctx, req)
}
```

## 5. 片段定义（api-server 侧）

在 api-server 中定义 Aegis 相关的提示词片段：

```go
// assistant/prompt_fragments.go

var DefaultFragments = []PromptFragment{
    {
        Name:        "base_assistant",
        Description: "Aegis 智能安全助手基础身份和能力描述",
        Keywords:    []string{},
        Priority:    100,
        Content:     "你是 Aegis 智能安全助手，专注于主机安全分析和运维操作。...",
    },
    {
        Name:        "plan_decision",
        Description: "判断任务是否需要拆分为多步骤计划",
        Keywords:    []string{"分析", "调查", "修复", "评估"},
        Priority:    90,
        Content:     "## 计划判断\n如果任务需要3个以上步骤，请制定执行计划...",
    },
    {
        Name:        "security_analysis",
        Description: "安全事件分析和攻击路径追溯",
        Keywords:    []string{"攻击", "入侵", "安全", "威胁", "告警"},
        Priority:    80,
        Content:     "## 安全分析规范\n1. 收集证据 2. 分析攻击路径 3. 评估影响...",
    },
    {
        Name:        "host_query",
        Description: "主机资产查询和管理",
        Keywords:    []string{"主机", "资产", "服务器", "在线", "离线"},
        Priority:    70,
        Content:     "## 主机查询\n使用 Host.List、Host.GetDetail 等工具...",
    },
    {
        Name:        "react_format",
        Description: "ReAct 输出格式规范",
        Keywords:    []string{},
        Priority:    50,
        Content:     "## 输出格式\n严格输出 JSON：{\"action\":\"tool_call\",...}",
    },
}
```

## 6. 配置联动

`enable_llm_routing` 控制是否启用 LLM 路由：

```go
// 当 enable_llm_routing = false 时：
// - 只用规则匹配（问候、简单查询）
// - 其他任务走原有 Assess() 流程
// - 提示词走原有 PromptProvider 逻辑

// 当 enable_llm_routing = true 时：
// - 规则匹配 + LLM 分类 + 片段选择 + 拼接
// - 替代 Assess() 流程
```

## 7. 数据流

```
用户消息 "你好"
  → Router.matchRules() → isGreeting = true
  → ActionDirectReply → 直接回复

用户消息 "查看主机列表"
  → Router.matchRules() → isSimpleQuery = true
  → ActionSimpleCall + ["base_assistant", "react_format"]
  → compose() → 拼接提示词
  → 单步骤 ReAct 执行

用户消息 "分析攻击链"
  → Router.matchRules() → 未匹配
  → Router.classify(LLM)
    → TaskClassification{task_type:"investigation", action:"full_plan"}
    → selected_fragments: ["base_assistant", "plan_decision", "security_analysis", "react_format"]
  → compose() → 拼接提示词
  → Generate(拼接提示词) → 完整计划 → 执行
```

## 8. 兼容性

- **向后兼容**：Router 可选注入，不注入时走原有流程
- **AI 分析不受影响**：AegisPromptProvider 路径独立
- **PromptProvider 接口不变**：Router 通过 ComposedPromptProvider 包装

## 9. 实现范围

### agent-runtime 新增
- `router/` 包：Router, ruleMatcher, classifier, selector, composer
- `core/types.go`：PromptFragment, TaskClassification, RouteResult
- `runtime.go`：集成 Router，新增 handleDirectReply

### api-server 变更
- `assistant/prompt_fragments.go`：定义提示词片段
- `assistant/runtime_factory.go`：注入 Router 和片段
- `assistant/orchestrator.go`：移除 Assess 调用

## 10. 回滚方案

- Router 通过 `WithRouter()` 注入，不注入时走原有 `Assess()` 流程
- `enable_llm_routing=false` 禁用 LLM 路由，只用规则匹配

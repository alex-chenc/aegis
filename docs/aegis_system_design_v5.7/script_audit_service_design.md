# V5.7 统一脚本审计服务设计

**版本**: 5.7
**日期**: 2026-05-07
**状态**: 设计中

---

## 1. 背景与动机

### 1.1 当前问题

V5.6及之前版本中，脚本安全验证存在以下严重问题：

| 问题 | 影响 | 位置 |
|:---|:---|:---|
| `validateScript()` 仅6条硬编码规则 | 覆盖面极窄，大量危险命令未拦截 | script_generation_service.go:184-227 |
| 两处完全相同的验证代码 | 代码重复，维护困难 | script_generation_service.go + self_healing_service.go |
| 漏洞修复/POC脚本无任何校验 | 安全盲区，可绕过所有安全检查 | vulnerability_service.go |
| 无审计日志 | 无法追溯验证结果，无法优化规则 | 全局缺失 |
| 简单substring匹配 | 无法处理正则、上下文相关风险 | validateScript()实现 |

### 1.2 设计目标

建立**统一的脚本审计服务（ScriptAuditService）**，作为所有脚本生成管线的唯一安全审计入口，实现：

1. **统一收口**: 所有脚本类型（基线、漏洞、POC、自愈）共用同一审计服务
2. **双重审计**: 黑名单审计（确定性）+ AI审计（上下文判断）
3. **审计日志**: 所有审计结果持久化，支持追溯和规则优化
4. **可扩展**: 审计规则可配置，审计策略可插拔

---

## 2. 架构设计

### 2.1 服务位置

```
api-server/internal/service/
├── script_audit_service.go          ← 新增：统一脚本审计服务
├── script_generation_service.go     ← 改造：移除内联validateScript，调用ScriptAuditService
├── self_healing_service.go          ← 改造：移除内联validateScript，调用ScriptAuditService
├── vulnerability_service.go         ← 改造：增加ScriptAuditService调用
├── task_service.go                  ← 改造：增加下发前审计
└── ...
```

### 2.2 核心接口设计

```go
// ScriptAuditService 统一脚本审计服务
type ScriptAuditService struct {
    blacklistChecker  *BlacklistChecker      // 黑名单检查器
    llmClient         *llm.LLMClient         // LLM客户端（AI审计用）
    auditLogRepo      repository.AuditLogRepo // 审计日志仓库
    configRepo        repository.ConfigRepo   // 配置仓库
}

// AuditRequest 审计请求
type AuditRequest struct {
    ScriptContent string      // 脚本内容
    ScriptType    ScriptType  // 脚本类型：baseline_check/baseline_fix/vulnerability_fix/poc_verify/self_healing
    Context       string      // 上下文信息（规则描述、CVE信息等）
    TaskID        string      // 关联的任务ID
    RuleID        string      // 关联的规则ID
    Source        AuditSource // 审计来源：generation/dispatch/agent
}

// AuditResult 审计结果
type AuditResult struct {
    Passed        bool              // 是否通过
    RiskLevel     string            // safe/low/medium/high/critical
    BlacklistHits []BlacklistHit    // 黑名单命中详情
    AIAnalysis    *AIAnalysisResult // AI审计结果（可能为nil，当AI不可用时）
    AuditLogID    string            // 审计日志ID
    Attempt       int               // 第几次尝试
    Script        string            // 通过审计的脚本内容
}

// BlacklistHit 黑名单命中记录
type BlacklistHit struct {
    RuleID      string // 命中的规则ID
    RuleName    string // 规则名称
    Pattern     string // 匹配的模式
    MatchedText string // 实际匹配到的文本
    LineNumber  int    // 所在行号
    Severity    string // 严重等级
    RuleType    string // hard_block / soft_warn
}

// AIAnalysisResult AI审计结果
type AIAnalysisResult struct {
    Passed    bool           `json:"passed"`
    RiskLevel string         `json:"risk_level"`
    Issues    []AuditIssue   `json:"issues"`
    Summary   string         `json:"summary"`
}

// AuditIssue 审计发现的问题
type AuditIssue struct {
    Type        string `json:"type"`
    Description string `json:"description"`
    LineRange   string `json:"line_range"`
    Suggestion  string `json:"suggestion"`
}
```

### 2.3 核心方法

```go
// AuditWithRetry 带重试的审计（用于脚本生成阶段）
func (s *ScriptAuditService) AuditWithRetry(ctx context.Context, generator ScriptGenerator, req *AuditRequest) (*AuditResult, error)

// AuditForDispatch 下发前审计（仅黑名单，无重试）
func (s *ScriptAuditService) AuditForDispatch(ctx context.Context, content string, taskID string) (*AuditResult, error)

// auditBlacklist 黑名单审计（确定性检查）
func (s *ScriptAuditService) auditBlacklist(ctx context.Context, content string, scriptType ScriptType) (*BlacklistAuditResult, error)

// auditAI AI审计（上下文分析）
func (s *ScriptAuditService) auditAI(ctx context.Context, req *AuditRequest, script string, blacklistResult *BlacklistAuditResult, previousAudits []string) (*AIAnalysisResult, error)
```

---

## 3. 审计流程详细设计

### 3.1 脚本生成阶段审计流程

```
调用方（ScriptGenerationService / VulnerabilityService / SelfHealingService）
    ↓
ScriptAuditService.AuditWithRetry(ctx, generator, req)
    ↓
循环 attempt = 1..3:
    ├─ generator.Generate(req) → 脚本内容
    ├─ ParseScript(脚本内容)
    ├─ auditBlacklist(content, scriptType)
    │   ├─ 命中hard_block → 记录日志，构造重试prompt，continue
    │   ├─ 命中soft_warn → 记录警告，继续AI审计
    │   └─ 无命中 → 继续AI审计
    ├─ auditAI(req, blacklistResult)
    │   ├─ AI不可用 → 降级为仅黑名单结果
    │   ├─ AI判定不通过 → 记录日志，构造重试prompt，continue
    │   └─ AI判定通过 → 返回通过结果
    └─ 返回 AuditResult{Passed: true}
    
3次均失败 → 返回 AuditResult{Passed: false} + 完整审计日志
```

### 3.2 下发前审计流程

```
调用方（TaskService.dispatchToAgent）
    ↓
ScriptAuditService.AuditForDispatch(ctx, content, taskID)
    ↓
auditBlacklist(content, "all")
    ├─ 命中hard_block → 返回失败 + 审计日志
    └─ 通过 → 返回成功
```

下发前不做AI审计，原因：
1. 延迟要求高（P99 < 100ms）
2. 脚本在生成阶段已经过AI审计
3. 下发前校验是纵深防御，非主要审计点

### 3.3 Agent侧审计流程

Agent侧仅做黑名单检查，不做AI审计，原因：
1. Agent无LLM客户端
2. Agent资源受限（1C1G）
3. Agent侧是最后一道防线，仅做确定性检查

---

## 4. 黑名单检查器设计

### 4.1 BlacklistChecker

```go
type BlacklistChecker struct {
    rules       []CompiledRule
    regexCache  map[string]*regexp.Regexp
    mu          sync.RWMutex
    configRepo  repository.ConfigRepo
}

type CompiledRule struct {
    ID          string
    Name        string
    RuleType    string         // hard_block / soft_warn
    MatchType   string         // exact / regex
    Pattern     string
    CompiledRe  *regexp.Regexp
    Category    string
    Severity    string
    AppliesTo   []string
    IsEnabled   bool
}

type CheckResult struct {
    HasViolation bool
    Hits         []BlacklistHit
}
```

### 4.2 匹配逻辑

```go
func (c *BlacklistChecker) Check(ctx context.Context, content string, scriptType ScriptType) (*CheckResult, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    lines := strings.Split(content, "\n")
    result := &CheckResult{}

    for _, rule := range c.rules {
        if !rule.IsEnabled {
            continue
        }
        if !rule.AppliesToType(scriptType) {
            continue
        }
        for lineNum, line := range lines {
            line = strings.TrimSpace(line)
            if line == "" || strings.HasPrefix(line, "#") {
                continue
            }
            matched := false
            var matchedText string

            switch rule.MatchType {
            case "exact":
                matched = strings.Contains(line, rule.Pattern)
                matchedText = rule.Pattern
            case "regex":
                if rule.CompiledRe != nil {
                    loc := rule.CompiledRe.FindStringIndex(line)
                    if loc != nil {
                        matched = true
                        matchedText = line[loc[0]:loc[1]]
                    }
                }
            }

            if matched {
                result.HasViolation = true
                result.Hits = append(result.Hits, BlacklistHit{
                    RuleID: rule.ID, RuleName: rule.Name,
                    Pattern: rule.Pattern, MatchedText: matchedText,
                    LineNumber: lineNum + 1, Severity: rule.Severity,
                    RuleType: rule.RuleType,
                })
                if rule.RuleType == "hard_block" {
                    return result, nil
                }
            }
        }
    }
    return result, nil
}
```

### 4.3 正则安全保护

```go
const (
    MaxRegexLength    = 1000
    MaxRegexMatchTime = 10 * time.Millisecond
)

func (c *BlacklistChecker) validateRegex(pattern string) (*regexp.Regexp, error) {
    if len(pattern) > MaxRegexLength {
        return nil, fmt.Errorf("正则长度超过限制(%d)", MaxRegexLength)
    }
    re, err := regexp.Compile(pattern)
    if err != nil {
        return nil, fmt.Errorf("正则编译失败: %w", err)
    }
    start := time.Now()
    re.MatchString("test string for performance check")
    if time.Since(start) > MaxRegexMatchTime {
        return nil, fmt.Errorf("正则匹配超时，可能存在ReDoS风险")
    }
    return re, nil
}
```

---

## 5. 审计日志数据模型

```go
type ScriptAuditLog struct {
    ID            string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    TaskID        string    `gorm:"index"`
    RuleID        string    `gorm:"index"`
    ScriptType    string    `gorm:"size:50"`
    ScriptContent string    `gorm:"type:text"`
    AuditSource   string    `gorm:"size:20"`
    Attempt       int
    Passed        bool
    RiskLevel     string    `gorm:"size:20"`
    BlacklistHits JSON      `gorm:"type:jsonb"`
    AIAnalysis    JSON      `gorm:"type:jsonb"`
    ErrorMsg      string    `gorm:"type:text"`
    DurationMs    int64
    CreatedAt     time.Time `gorm:"autoCreateTime"`
}
```

---

## 6. 改造现有脚本生成管线

### 6.1 ScriptGenerationService 改造

移除 `script_generation_service.go` 中的 `validateScript()` 方法（约45行），替换为 `ScriptAuditService.AuditWithRetry()` 调用。

### 6.2 VulnerabilityService 改造

在 `generateFixScript()` 和 `generatePocScript()` 中增加 `ScriptAuditService.AuditWithRetry()` 调用（当前无任何校验）。

### 6.3 SelfHealingService 改造

移除 `self_healing_service.go` 中的 `validateScript()` 方法（约25行），替换为 `ScriptAuditService.AuditWithRetry()` 调用。

---

## 7. 配置开关

在 `system_configs` 表中：

| config_key | config_value | 说明 |
|:---|:---|:---|
| `command_audit.settings` | `{"blacklist_enabled": true, "ai_enabled": true, "max_retry": 3, "dispatch_check": true, "agent_check": true}` | 审计策略总开关 |

当 `ai_enabled=false` 或 LLM 不可用时，自动降级为仅黑名单审计模式。

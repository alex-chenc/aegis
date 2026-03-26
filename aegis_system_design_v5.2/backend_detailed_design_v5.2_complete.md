# 后端详细设计文档 - V5.2 完整版

**版本**: 5.2
**状态**: 定稿
**日期**: 2026-03-26

---

## 1. 项目结构

```
/backend
├── cmd/server/main.go           # 入口文件
├── internal/
│   ├── api/
│   │   ├── router.go            # 路由定义
│   │   └── handler/
│   │       ├── detection_handler.go  # 异常检测Handler
│   │       ├── host_handler.go
│   │       └── ...
│   ├── service/
│   │   ├── alert_service.go     # 告警服务
│   │   ├── websocket_service.go # WebSocket服务
│   │   ├── block_service.go     # 阻断服务
│   │   ├── false_positive_service.go  # 智能误报检测服务
│   │   └── ...
│   ├── repository/
│   │   ├── alert_repo.go        # 告警仓库
│   │   ├── block_policy_repo.go # 阻断策略仓库
│   │   ├── runtime_event_repo.go# 运行时事件仓库
│   │   ├── sigma_rule_repo.go   # Sigma规则仓库
│   │   └── ...
│   ├── model/
│   │   ├── alert.go             # 告警模型
│   │   ├── block_policy.go      # 阻断策略模型
│   │   ├── runtime_event.go     # 运行时事件模型
│   │   ├── sigma_rule.go        # Sigma规则模型
│   │   ├── mitre_mapping.go     # MITRE中文映射
│   │   └── ...
│   ├── grpc_server/
│   │   └── server.go            # gRPC服务
│   ├── seed/
│   │   └── block_policy.go      # 默认策略初始化
│   └── pipeline/
│       └── llm_prompt_builder.go
├── config/
│   ├── config.yaml
│   └── rules/                   # Sigma规则目录
└── migrations/
```

---

## 2. 核心数据模型

### 2.1 Alert（告警）

```go
type Alert struct {
    ID                  uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    AlertID             string     `gorm:"type:varchar(64);uniqueIndex;not null"`
    HostID              uuid.UUID  `gorm:"type:uuid;not null;index"`
    Hostname            string     `gorm:"-"` // 不存储，查询时关联
    PID                 int
    PPID                int
    CommandLine         string     `gorm:"type:text"`
    ProcessTree         string     `gorm:"type:jsonb"`
    MitreID             string     `gorm:"type:varchar(20);index"` // 大写T格式
    MitreName           string     `gorm:"type:varchar(100)"`
    Severity            string     `gorm:"type:varchar(20);index"`
    Description         string     `gorm:"type:text"`
    LLMSummary          string     `gorm:"type:text;column:llm_summary"`
    DedupeKey           string     `gorm:"type:varchar(256);not null;index"`
    HitCount            int        `gorm:"not null;default:1"`
    AutoBlocked         bool       `gorm:"not null;default:false"`
    ManualBlocked       bool       `gorm:"not null;default:false"`
    Status              string     `gorm:"type:varchar(20);not null;default:'pending';index"`
    JudgmentSource      string     `gorm:"type:varchar(20);default:'system'"`
    BlockStatus         *string    `gorm:"type:varchar(20)"`
    BlockMessage        string     `gorm:"type:text"`
    AutoDispose         bool       `gorm:"not null;default:false"`
    LLMDisposalStrategy string     `gorm:"type:text"`
    RuleID              string     `gorm:"type:varchar(128)"`
    RuleTitle           string     `gorm:"type:varchar(255)"`
    FirstSeenAt         time.Time  `gorm:"default:now()"`
    LastSeenAt          time.Time  `gorm:"default:now()"`
    CreatedAt           time.Time  `gorm:"default:now()"`
    UpdatedAt           time.Time  `gorm:"default:now()"`
}

func (Alert) TableName() string { return "alerts" }
```

### 2.2 BlockPolicy（阻断策略）

```go
type BlockPolicy struct {
    ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    MitreID     string     `gorm:"type:varchar(20);uniqueIndex;not null"` // 大写T格式，唯一约束
    MitreName   string     `gorm:"type:varchar(100)"` // 规则标题
    Enabled     bool       `gorm:"not null;default:true"`
    AutoBlock   bool       `gorm:"not null;default:false"`
    AutoDispose bool       `gorm:"not null;default:false"`
    Action      string     `gorm:"type:varchar(50);not null;default:'kill_process'"`
    CreatedAt   time.Time  `gorm:"default:now()"`
    UpdatedAt   time.Time  `gorm:"default:now()"`
}

func (BlockPolicy) TableName() string { return "block_policies" }
```

### 2.3 SigmaRule（Sigma规则）

```go
type SigmaRule struct {
    ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    RuleID      string     `gorm:"type:varchar(128);uniqueIndex;not null"`
    Title       string     `gorm:"type:varchar(255)"`
    Description string     `gorm:"type:text"`
    Content     string     `gorm:"type:text"`
    Status      string     `gorm:"type:varchar(20);default:'pending'"` // pending/experimental/active/disabled
    MitreID     string     `gorm:"type:varchar(20);uniqueIndex"` // 大写T格式，唯一约束
    Severity    string     `gorm:"type:varchar(20)"`
    GeneratedBy string     `gorm:"type:varchar(20)"` // import/llm
    Version     string     `gorm:"type:varchar(10);default:'1.0'"`
    ActivatedAt *time.Time
    CreatedAt   time.Time  `gorm:"default:now()"`
    UpdatedAt   time.Time  `gorm:"default:now()"`
}

func (SigmaRule) TableName() string { return "sigma_rules" }
```

### 2.4 RuntimeEvent（运行时事件）

```go
type RuntimeEvent struct {
    ID            uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    EventID       string     `gorm:"type:varchar(64);uniqueIndex;not null"`
    HostID        uuid.UUID  `gorm:"type:uuid;not null;index"`
    EventType     string     `gorm:"type:varchar(32);not null;index"`
    EventData     string     `gorm:"type:jsonb;not null"`
    MatchedRuleID string     `gorm:"type:varchar(128)"`
    RuleTitle     string     `gorm:"type:varchar(255)"`
    MitreID       string     `gorm:"type:varchar(20)"` // 大写T格式
    Severity      string     `gorm:"type:varchar(16)"`
    PID           int        `gorm:"column:pid"`
    CommandLine   string     `gorm:"type:text"`
    Timestamp     int64      `gorm:"not null;index"`
    CreatedAt     time.Time  `gorm:"default:now()"`
    Aggregated    bool       `gorm:"default:false;index"`
}

func (RuntimeEvent) TableName() string { return "runtime_events" }
```

### 2.5 RuleAdjustmentHistory（规则调整历史）

```go
type RuleAdjustmentHistory struct {
    ID              uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    RuleID          string     `gorm:"type:varchar(128);index"`
    TriggerCount    int        `gorm:"not null"`
    TimeWindow      string     `gorm:"type:varchar(10)"` // 10m/30m/60m
    IsFalsePositive bool       `gorm:"not null"`
    LLMReason       string     `gorm:"type:text"`
    OldContent      string     `gorm:"type:text"`
    NewContent      string     `gorm:"type:text"`
    AppliedAt       time.Time  `gorm:"default:now()"`
}

func (RuleAdjustmentHistory) TableName() string { return "rule_adjustment_histories" }
```

---

## 3. 核心服务

### 3.1 FalsePositiveDetectionService（智能误报检测服务）

```go
// backend/internal/service/false_positive_service.go
package service

type RuleTriggerStats struct {
    RuleID     string
    RuleTitle  string
    MitreID    string
    AlertCount int
    TimeWindow string
    Alerts     []model.Alert
}

type FalsePositiveAnalysisResult struct {
    IsFalsePositive bool            `json:"is_false_positive"`
    Confidence      float64         `json:"confidence"`
    Reason          string          `json:"reason"`
    RuleAdjustment  RuleAdjustment  `json:"rule_adjustments"`
}

type RuleAdjustment struct {
    RuleID          string   `json:"rule_id"`
    Action          string   `json:"action"` // tighten
    Reason          string   `json:"reason"`
    AddConditions   []string `json:"add_conditions"`
    ExcludePatterns []string `json:"exclude_patterns"`
    SeverityChange  string   `json:"severity_change"`
}

type FalsePositiveDetectionService struct {
    alertRepo      *repository.AlertRepository
    sigmaRuleRepo  *repository.SigmaRuleRepository
    configRepo     *repository.ConfigRepository
    grpcServer     *grpc_server.GRPCServer
    llmTimeout     int
    llmMaxRetries  int
    sampleSize     int
    thresholds     map[string]int // {"10m": 10, "30m": 30, "60m": 60}
    enabled        bool
    stopCh         chan struct{}
    wg             sync.WaitGroup
}

func NewFalsePositiveDetectionService(...) *FalsePositiveDetectionService

// 启动定时检测
func (s *FalsePositiveDetectionService) Start(ctx context.Context)

// 停止服务
func (s *FalsePositiveDetectionService) Stop()

// 检查时间窗口
func (s *FalsePositiveDetectionService) checkTimeWindow(window string, threshold int)

// 分析规则
func (s *FalsePositiveDetectionService) analyzeRule(ctx context.Context, stats RuleTriggerStats)

// 应用规则调整
func (s *FalsePositiveDetectionService) applyRuleAdjustment(rule *model.SigmaRule, adjustment RuleAdjustment)

// 升级实验性规则
func (s *FalsePositiveDetectionService) promoteRuleToActive(rule *model.SigmaRule) error
```

#### 检测逻辑

```go
func (s *FalsePositiveDetectionService) checkTimeWindow(window string, threshold int) {
    ctx := context.Background()
    
    var startTime time.Time
    switch window {
    case "10m":
        startTime = time.Now().Add(-10 * time.Minute)
    case "30m":
        startTime = time.Now().Add(-30 * time.Minute)
    case "60m":
        startTime = time.Now().Add(-60 * time.Minute)
    }
    
    // 查询告警统计（按规则ID分组）
    stats, err := s.alertRepo.GetRuleTriggerStats(startTime, time.Now(), threshold, s.sampleSize)
    if err != nil {
        return
    }
    
    // 对每个超过阈值的规则进行分析
    for _, stat := range stats {
        if stat.AlertCount > threshold {
            go s.analyzeRule(ctx, stat)
        }
    }
}
```

#### 规则状态流转

```
LLM判断为误报
    ↓
applyRuleAdjustment: 修改规则内容，状态设为 experimental
    ↓
保存到数据库（不下发Agent）
    ↓
1小时观察期（用户可禁用）
    ↓
promoteRuleToActive: 状态升级为 active
    ↓
广播下发到Agent
```

### 3.2 AlertService

```go
type AlertService struct {
    alertRepo       *repository.AlertRepository
    blockPolicyRepo *repository.BlockPolicyRepository
    blockRepo       *repository.BlockRepository
    grpcServer      *grpc_server.GRPCServer
}

// CheckAndAutoBlock 检查并执行自动阻断
func (s *AlertService) CheckAndAutoBlock(alert *model.Alert) error {
    policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
    if err != nil || !policy.Enabled || !policy.AutoBlock {
        return nil
    }
    
    alert.AutoBlocked = true
    blockStatus := "blocking"
    alert.BlockStatus = &blockStatus
    s.alertRepo.Update(alert)
    
    // 下发阻断指令
    record := &model.BlockRecord{
        BlockID:  "BLK-" + uuid.New().String()[:8],
        AlertID:  &alert.ID,
        HostID:   alert.HostID,
        Action:   policy.Action,
        Target:   fmt.Sprintf("%d", alert.PID),
        IssuedBy: "auto",
    }
    return s.blockRepo.Create(record)
}

// CheckAndAutoDispose 检查并执行自动处置
func (s *AlertService) CheckAndAutoDispose(alert *model.Alert) error {
    policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
    if err != nil || !policy.Enabled || !policy.AutoDispose {
        return nil
    }
    
    alert.AutoDispose = true
    alert.Status = "resolved"
    return s.alertRepo.Update(alert)
}
```

### 3.3 WebSocketService

```go
type WebSocketService struct {
    clients   map[*websocket.Conn]bool
    broadcast chan WSMessage
    mu        sync.RWMutex
}

type WSMessage struct {
    Type string      `json:"type"`
    Data interface{} `json:"data"`
}

// BroadcastAlert 广播告警更新
func (s *WebSocketService) BroadcastAlert(alert *model.Alert)

// BroadcastPolicyUpdate 广播策略更新
func (s *WebSocketService) BroadcastPolicyUpdate(policy *model.BlockPolicy)

// BroadcastRuleUpdate 广播规则更新
func (s *WebSocketService) BroadcastRuleUpdate(rule *model.SigmaRule)
```

---

## 4. API Handler

### 4.1 DetectionHandler

```go
type DetectionHandler struct {
    alertRepo          *repository.AlertRepository
    blockRepo          *repository.BlockRepository
    blockPolicyRepo    *repository.BlockPolicyRepository
    sigmaRuleRepo      *repository.SigmaRuleRepository
    runtimeEventRepo   *repository.RuntimeEventRepository
    toolCallRepo       *repository.ToolCallRepository
    alertService       *service.AlertService
    sigmaRuleService   *service.SigmaRuleService
    llmAggregationRepo *repository.LLMAggregationRepository
    configRepo         *repository.ConfigRepository
    grpcServer         *grpc_server.GRPCServer
    wsService          *service.WebSocketService
}
```

### 4.2 StartLLMAggregation（AI降噪）

```go
func (h *DetectionHandler) StartLLMAggregation(c *gin.Context) {
    var body struct {
        StartTime   string   `json:"start_time" binding:"required"`
        EndTime     string   `json:"end_time" binding:"required"`
        HostIDs     []string `json:"host_ids"`
        AutoDispose bool     `json:"auto_dispose"`
    }
    c.ShouldBindJSON(&body)
    
    startTime, _ := time.Parse(time.RFC3339, body.StartTime)
    endTime, _ := time.Parse(time.RFC3339, body.EndTime)
    
    // 创建聚合记录
    agg := &model.LLMAggregation{
        AggregationID: "AGG-" + fmt.Sprintf("%d", time.Now().UnixNano()),
        StartTime:     startTime,
        EndTime:       endTime,
        Status:        "processing",
    }
    h.llmAggregationRepo.Create(agg)
    
    // 根据时间范围直接查询告警（修复后的逻辑）
    alerts, err := h.alertRepo.FindPendingByTimeRange(startTime, endTime, body.HostIDs)
    if err != nil {
        // 错误处理
    }
    
    agg.AlertCount = len(alerts)
    h.llmAggregationRepo.Update(agg)
    
    // 调用LLM分析
    if len(alerts) > 0 {
        llmResponse, err := h.callLLMForAlerts(c.Request.Context(), alerts)
        // 处理LLM响应...
    }
    
    agg.Status = "completed"
    h.llmAggregationRepo.Update(agg)
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "data": agg})
}
```

### 4.3 GenerateSigmaRule（AI生成规则）

```go
func (h *DetectionHandler) GenerateSigmaRule(c *gin.Context) {
    var req struct {
        Event    string `json:"event" binding:"required"`
        Method   string `json:"method"`
        MitreID  string `json:"mitre_id"`
        Severity string `json:"severity"`
    }
    c.ShouldBindJSON(&req)
    
    // 构建LLM Prompt
    prompt := fmt.Sprintf(`你是安全规则专家。请根据用户描述生成一个Sigma规则。

## 用户需求
- 检测事件: %s
- 检测方式: %s
- MITRE技术ID: %s
- 严重程度: %s

## 输出要求
1. 生成符合Sigma规则格式的YAML内容
2. 规则必须包含: title, id, status, description, level, logsource, detection
3. id字段使用uuid格式
4. status设为 experimental
5. 在tags中包含MITRE技术ID

只输出YAML内容。`, req.Event, req.Method, req.MitreID, severity)
    
    // 调用LLM
    response, err := client.ChatCompletion(ctx, "", prompt, 0.7)
    
    // 解析YAML
    var rawRule struct {
        Title       string
        ID          string
        Status      string
        Description string
        Level       string
        Tags        []string
        // ...
    }
    yaml.Unmarshal([]byte(cleanResponse), &rawRule)
    
    // 统一MITRE ID格式
    mitreID := strings.ToUpper(req.MitreID)
    if !strings.HasPrefix(mitreID, "T") {
        mitreID = "T" + mitreID
    }
    
    // 检查MITRE ID是否已存在
    exists, _ := h.sigmaRuleRepo.ExistsByMitreID(mitreID)
    if exists {
        c.JSON(http.StatusBadRequest, gin.H{
            "code": 400,
            "message": fmt.Sprintf("MITRE ID %s already exists", mitreID),
        })
        return
    }
    
    // 创建规则
    rule := &model.SigmaRule{
        RuleID:      rawRule.ID,
        Title:       rawRule.Title,
        Content:     string(ruleYaml),
        Status:      "experimental",
        MitreID:     mitreID,
        Severity:    rawRule.Level,
        GeneratedBy: "llm",
    }
    h.sigmaRuleRepo.Create(rule)
    
    // 创建对应阻断策略
    policy := &model.BlockPolicy{
        MitreID:     mitreID,
        MitreName:   rule.Title,
        Enabled:     true,
        AutoBlock:   false,
        AutoDispose: false,
        Action:      "kill_process",
    }
    h.blockPolicyRepo.Create(policy)
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
        "rule_id":  rule.RuleID,
        "title":    rule.Title,
        "mitre_id": rule.MitreID,
        "severity": rule.Severity,
        "content":  rule.Content,
        "duration": int(duration),
    }})
}
```

### 4.4 DeleteRules（批量删除规则）

```go
func (h *DetectionHandler) DeleteRules(c *gin.Context) {
    var req struct {
        RuleIDs []string `json:"rule_ids" binding:"required"`
    }
    c.ShouldBindJSON(&req)
    
    deletedRules := 0
    deletedAlerts := 0
    deletedPolicies := 0
    
    for _, ruleID := range req.RuleIDs {
        rule, err := h.sigmaRuleRepo.FindByRuleID(ruleID)
        if err != nil {
            continue
        }
        
        // 删除关联告警
        alertCount, _ := h.alertRepo.DeleteByRuleID(ruleID)
        deletedAlerts += alertCount
        
        // 删除对应阻断策略
        if rule.MitreID != "" {
            policyDeleted, _ := h.blockPolicyRepo.DeleteByMitreID(rule.MitreID)
            if policyDeleted {
                deletedPolicies++
            }
        }
        
        // 删除规则
        h.sigmaRuleRepo.DeleteByRuleID(ruleID)
        deletedRules++
    }
    
    c.JSON(http.StatusOK, gin.H{
        "code": 0,
        "data": gin.H{
            "deleted_rules":    deletedRules,
            "deleted_alerts":   deletedAlerts,
            "deleted_policies": deletedPolicies,
        },
    })
}
```

---

## 5. Repository

### 5.1 AlertRepository

```go
type AlertRepository struct {
    db *gorm.DB
}

// 根据时间范围查询pending告警
func (r *AlertRepository) FindPendingByTimeRange(startTime, endTime time.Time, hostIDs []string) ([]model.Alert, error)

// 获取规则触发统计
func (r *AlertRepository) GetRuleTriggerStats(startTime, endTime time.Time, minCount int, sampleSize int) ([]RuleTriggerStats, error)

// 按规则ID统计告警数量
func (r *AlertRepository) CountByRuleID(ruleID string) (int, error)

// 按规则ID删除告警
func (r *AlertRepository) DeleteByRuleID(ruleID string) (int, error)

// 规范化MITRE ID
func (r *AlertRepository) NormalizeMitreIDs(ctx context.Context) (int, error)
```

### 5.2 BlockPolicyRepository

```go
type BlockPolicyRepository struct {
    db *gorm.DB
}

// 按MITRE ID查找
func (r *BlockPolicyRepository) FindByMitreID(mitreID string) (*model.BlockPolicy, error)

// 分页查询（带规则标题）
type BlockPolicyWithRuleTitle struct {
    model.BlockPolicy
    RuleTitle string `json:"rule_title"`
}
func (r *BlockPolicyRepository) ListPaginatedWithRuleTitle(page, pageSize int, query string) ([]BlockPolicyWithRuleTitle, int64, error)

// 更新
func (r *BlockPolicyRepository) Update(mitreID string, updates map[string]interface{}) error

// 按MITRE ID删除
func (r *BlockPolicyRepository) DeleteByMitreID(mitreID string) (bool, error)

// 规范化MITRE ID
func (r *BlockPolicyRepository) NormalizeMitreIDs(ctx context.Context) (int, error)
```

### 5.3 SigmaRuleRepository

```go
type SigmaRuleRepository struct {
    db *gorm.DB
}

// 创建规则
func (r *SigmaRuleRepository) Create(rule *model.SigmaRule) error

// 更新规则
func (r *SigmaRuleRepository) Update(rule *model.SigmaRule) error

// 按RuleID查找
func (r *SigmaRuleRepository) FindByRuleID(ruleID string) (*model.SigmaRule, error)

// 检查MITRE ID是否存在
func (r *SigmaRuleRepository) ExistsByMitreID(mitreID string) (bool, error)

// 列表查询（支持搜索）
func (r *SigmaRuleRepository) List(page, pageSize int, filters map[string]interface{}) ([]model.SigmaRule, int64, error)

// 删除规则
func (r *SigmaRuleRepository) DeleteByRuleID(ruleID string) error

// 获取实验性规则
func (r *SigmaRuleRepository) GetExperimentalRules() ([]model.SigmaRule, error)

// 规范化MITRE ID
func (r *SigmaRuleRepository) NormalizeMitreIDs(ctx context.Context) (int, error)
```

---

## 6. 默认策略初始化

```go
// backend/internal/seed/block_policy.go
package seed

// MITRE ID统一使用大写T格式
var DefaultBlockPolicies = []model.BlockPolicy{
    {MitreID: "T1003", MitreName: "OS Credential Dumping", Enabled: true, AutoBlock: false, AutoDispose: true, Action: "quarantine_file"},
    {MitreID: "T1003.001", MitreName: "LSASS Memory Dump", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
    {MitreID: "T1059.004", MitreName: "Unix Shell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
    {MitreID: "T1113", MitreName: "Screen Capture", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
    // ... 共33条，MITRE ID全部使用大写T格式
}

func SeedBlockPolicies(db *gorm.DB) {
    repo := repository.NewBlockPolicyRepository(db)
    for _, policy := range DefaultBlockPolicies {
        var existing model.BlockPolicy
        err := db.Where("mitre_id = ?", policy.MitreID).First(&existing).Error
        if err == gorm.ErrRecordNotFound {
            repo.Create(&policy)
        }
    }
}
```

---

## 7. API路由

```go
// backend/internal/api/router.go
func (r *Router) Setup(grpcServer *grpc_server.GRPCServer) {
    api := r.engine.Group("/api/v1")
    
    detection := api.Group("/detection")
    {
        // 告警
        detection.GET("/alerts", r.detectionHandler.ListAlerts)
        detection.GET("/alerts/:alert_id", r.detectionHandler.GetAlert)
        detection.POST("/alerts/:alert_id/resolve", r.detectionHandler.ResolveAlert)
        detection.POST("/alerts/:alert_id/block", r.detectionHandler.BlockAlert)
        detection.DELETE("/alerts", r.detectionHandler.DeleteAlerts)
        
        // 阻断策略
        detection.GET("/block-policies", r.detectionHandler.ListBlockPolicies)
        detection.POST("/block-policies/sync", r.detectionHandler.SyncBlockPolicies)
        detection.POST("/block-policies/normalize", r.detectionHandler.NormalizeMitreIDs)
        detection.PUT("/block-policies/:mitre_id", r.detectionHandler.UpdateBlockPolicy)
        
        // 阻断记录
        detection.GET("/blocks", r.detectionHandler.ListBlockRecords)
        
        // MITRE矩阵
        detection.GET("/attack-matrix", r.detectionHandler.GetAttackMatrix)
        
        // 规则
        detection.GET("/rules", r.detectionHandler.ListRules)
        detection.GET("/rules/:id", r.detectionHandler.GetRule)
        detection.POST("/rules/import", r.detectionHandler.ImportRules)
        detection.POST("/rules/generate", r.detectionHandler.GenerateSigmaRule)
        detection.POST("/rules/check-delete", r.detectionHandler.CheckRulesBeforeDelete)
        detection.DELETE("/rules", r.detectionHandler.DeleteRules)
        detection.PUT("/rules/:id/status", r.detectionHandler.UpdateRuleStatus)
        
        // AI降噪
        detection.POST("/llm/aggregate", r.detectionHandler.StartLLMAggregation)
        detection.GET("/llm/aggregate/:id", r.detectionHandler.GetLLMAggregationStatus)
        
        // WebSocket
        detection.GET("/runtime/ws", r.websocketHandler.HandleWebSocket)
    }
}
```

---

## 8. 构建与部署

### 8.1 构建

```bash
cd backend
make build
# 产出: backend 可执行文件
```

### 8.2 Docker镜像

```dockerfile
FROM alpine:latest
WORKDIR /root/
RUN apk add --no-cache ca-certificates
COPY backend/backend .
RUN mkdir -p config
COPY backend/config/config.yaml config/
COPY backend/config/rules/ config/rules/
EXPOSE 8080 9090
CMD ["./backend"]
```

### 8.3 部署

```bash
# 构建镜像
docker build -t aegis-system/backend:latest -f backend/Dockerfile .

# 启动服务
docker compose up -d backend
```

---

**文档结束**
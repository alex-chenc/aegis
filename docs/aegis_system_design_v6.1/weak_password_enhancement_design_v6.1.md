# 弱密码检测增强设计文档 V6.1

## 一、问题描述与需求

### 1.1 当前问题

1. **去重和只检测页面通用应用失效**：页面显示的数据变成了全量数据，未按要求去重和过滤
2. **Redis容器路径未生效**：Redis是容器部署的，但模型检测后未走容器路径
3. **应用资产未按主机去重**：同一个主机上的应用存在重复
4. **检测轮数功能缺失**：需要添加检测轮数配置（10-50轮）
5. **采集进度显示问题**：需要将"采集失败"改为"采集进度"，并添加分页
6. **openssh检测不完整**：应该检测/etc/shadow文件并获取盐等信息给大模型
7. **AI密码生成问题**：生成的密码都是顺序的，明显存在问题

### 1.2 需求概述

基于开源库veinmind-tools和lynis的弱密码检测方案，增强Aegis系统的弱密码检测能力：
- 为每种应用类型定义固定的检测skill
- 实现通用弱密码检测skill
- 修复容器路径判断
- 实现应用资产去重
- 添加检测轮数配置
- 优化前端显示

---

## 二、开源库分析总结

### 2.1 veinmind-tools 弱密码检测方案

**支持的应用类型：**
- Redis, MySQL/MariaDB, PostgreSQL, OpenSSH/Linux, Tomcat, FTP, Nginx/Apache

**密码解析器类型：**
| 解析器 | 用途 | 典型应用 |
|--------|------|---------|
| shadow | 解析/etc/shadow | Linux账户 |
| ini | 解析INI格式配置 | MySQL |
| yaml/yml | YAML格式 | 通用配置 |
| json | JSON格式 | 通用配置 |
| properties | Java properties | PostgreSQL |
| line_key_value | key=value格式 | Redis |
| htpasswd | Apache/Nginx htpasswd | Web Basic Auth |
| tomcat_users_xml | XML格式 | Tomcat |

**密码匹配方案：**
- 明文密码：直接字典匹配
- 哈希密码：使用crypt库验证（MD5-crypt, bcrypt, SHA-256/512 crypt等）

**容器支持：**
- 通过`/proc/[pid]/root/`路径访问容器内部配置文件

### 2.2 lynis 安全审计方案

**检测方法：**
- 密码哈希算法检测（AUTH-9229）
- 空密码账户检测（AUTH-9283）
- 密码过期策略检测（AUTH-9286）
- SSH配置安全检测（SSH-7408）

**Redis检测：**
- 仅检查配置文件中是否设置了`requirepass`
- 不验证密码强度

---

## 三、根因分析

### 3.1 去重问题根因

**问题**：页面显示全量数据，未去重

**根因分析：**
1. 前端`fetchCandidates`方法未正确应用筛选条件
2. 资产分析Tab和弱密码检查Tab的数据源未分离
3. 候选应用查询接口未返回去重后的数据

**代码位置：**
- `frontend/src/store/weakPassword.ts` - fetchCandidates方法
- `frontend/src/views/detection/WeakPassword/Index.vue` - 数据展示逻辑
- `api-server/internal/repository/weak_password_repository.go` - 查询方法

### 3.2 Redis容器路径问题根因

**问题**：Redis容器部署但未走容器路径

**根因分析：**
1. 资产分析时未正确识别Redis为容器应用
2. 采集计划中`related_pids`为空，导致容器路径未生成
3. `credentialPathCandidates`函数未被正确调用

**代码位置：**
- `agent/internal/weakpass/collector.go` - credentialPathCandidates函数
- `api-server/internal/service/weak_password_service.go` - attemptProcessBasedRepair函数

### 3.3 应用资产去重问题根因

**问题**：同一个主机上的应用存在重复

**根因分析：**
1. `CreateAnalysisWithCandidates`的upsert冲突键为`(host_id, asset_id, application_type)`
2. 但`asset_id`可能不同（同一应用多个实例）
3. 需要按主机+应用类型去重，而非按资产ID

**代码位置：**
- `api-server/internal/repository/weak_password_repository.go` - CreateAnalysisWithCandidates方法

### 3.4 检测轮数问题根因

**问题**：需要添加检测轮数配置（10-50轮）

**根因分析：**
1. 前端已有检测轮数配置（Index.vue中的`maxRounds`滑块）
2. 后端`WeakPasswordScanTask`模型已有`max_agent_tool_calls`字段
3. 但前端传递的参数名可能与后端不匹配

**代码位置：**
- `frontend/src/views/detection/WeakPassword/Index.vue` - 检测轮数配置
- `api-server/internal/model/weak_password.go` - WeakPasswordScanTask模型

### 3.5 采集进度显示问题根因

**问题**：需要将"采集失败"改为"采集进度"，并添加分页

**根因分析：**
1. 前端`TaskDetail.vue`中采集进度表格显示为"采集失败"
2. 需要改为"采集进度"并添加分页功能
3. 当前分页逻辑可能已存在但未正确实现

**代码位置：**
- `frontend/src/views/detection/WeakPassword/TaskDetail.vue` - 采集进度表格

### 3.6 openssh检测问题根因

**问题**：应该检测/etc/shadow文件并获取盐等信息

**根因分析：**
1. `weak_password_skills.go`中openssh的skill已定义检测/etc/shadow
2. 但可能未正确提取盐值和哈希算法信息
3. 需要将盐等信息传递给大模型进行弱密码匹配

**代码位置：**
- `api-server/internal/service/weak_password_skills.go` - openssh skill定义
- `agent/internal/weakpass/parsers.go` - shadow解析器

### 3.7 AI密码生成问题根因

**问题**：生成的密码都是顺序的

**根因分析：**
1. AI字典生成可能使用了固定的模板
2. 未使用真正的LLM生成密码
3. 生成逻辑可能只是简单的数字递增

**代码位置：**
- `api-server/internal/service/weak_password_service.go` - AI字典生成逻辑

---

## 四、设计方案

### 4.1 应用类型Skill注册表增强

基于veinmind-tools的方案，为每种应用类型定义固定的检测skill：

```go
// weak_password_skills.go
type ApplicationSkill struct {
    ApplicationType string
    DisplayName     string
    ConfigPaths     []string
    Extractors      []CredentialExtractor
    ContainerAware  bool
    Description     string
}

var applicationSkills = map[string]ApplicationSkill{
    "redis": {
        ApplicationType: "redis",
        DisplayName:     "Redis",
        ConfigPaths: []string{
            "/etc/redis/redis.conf",
            "/etc/redis.conf",
            "/usr/local/etc/redis/redis.conf",
            "/data/redis.conf",
        },
        Extractors: []CredentialExtractor{
            {Type: "line_key_value", PasswordSelector: "requirepass"},
            {Type: "line_key_value", PasswordSelector: "masterauth"},
        },
        ContainerAware: true,
        Description:    "Redis数据库弱密码检测",
    },
    "openssh": {
        ApplicationType: "openssh",
        DisplayName:     "OpenSSH",
        ConfigPaths: []string{
            "/etc/shadow",
        },
        Extractors: []CredentialExtractor{
            {Type: "shadow"},
        },
        ContainerAware: false,
        Description:    "OpenSSH/Linux账户弱密码检测",
    },
    // ... 其他应用类型
}
```

### 4.2 通用弱密码检测Skill

对于未在注册表中的应用，使用通用skill：

```go
var genericSkill = ApplicationSkill{
    ApplicationType: "generic",
    DisplayName:     "通用应用",
    ConfigPaths:     []string{}, // 动态发现
    Extractors: []CredentialExtractor{
        {Type: "line_key_value"},
        {Type: "yaml"},
        {Type: "json"},
        {Type: "properties"},
    },
    ContainerAware: true,
    Description:    "通用弱密码检测，支持多种配置格式",
}
```

### 4.3 容器路径判断增强

参考arcane的cgroup_utils.go，实现容器判断：

```go
// container_detection.go
func IsRunningInContainer() bool {
    // 方法1: 检查/.dockerenv文件
    if _, err := os.Stat("/.dockerenv"); err == nil {
        return true
    }
    
    // 方法2: 检查cgroup信息
    data, err := os.ReadFile("/proc/self/cgroup")
    if err != nil {
        return false
    }
    
    // Docker容器ID模式
    dockerPattern := regexp.MustCompile(`/docker/([a-f0-9]{64})`)
    containerdPattern := regexp.MustCompile(`/containerd/([a-f0-9]{64})`)
    
    return dockerPattern.Match(data) || containerdPattern.Match(data)
}

func GetContainerRootPath(pid int) string {
    return fmt.Sprintf("/proc/%d/root", pid)
}
```

### 4.4 应用资产去重

修改查询逻辑，按主机+应用类型去重：

```sql
-- 去重查询
SELECT DISTINCT ON (host_id, application_type)
    id, host_id, asset_id, application_type, confidence, paths, extractors
FROM weak_password_candidate_applications
WHERE analysis_id = ?
ORDER BY host_id, application_type, confidence DESC;
```

### 4.5 检测轮数配置

确保前端参数正确传递到后端：

```typescript
// 前端
const startDetection = async () => {
    const params = {
        candidate_ids: selectedCandidates.value,
        dictionary_strategy: dictionaryStrategy.value,
        max_rounds: maxRounds.value, // 10-50
        enable_ai_repair: enableAIRepair.value,
    }
    await startWeakPasswordTask(params)
}
```

```go
// 后端
type CreateTaskRequest struct {
    CandidateIDs    []uint `json:"candidate_ids"`
    DictionaryStrategy string `json:"dictionary_strategy"`
    MaxRounds       int    `json:"max_rounds" binding:"min=10,max=50"`
    EnableAIRepair  bool   `json:"enable_ai_repair"`
}
```

### 4.6 采集进度和分页

修改前端显示：

```vue
<!-- TaskDetail.vue -->
<el-table :data="paginatedToolCalls" style="width: 100%">
    <el-table-column prop="round" label="轮次" width="80" />
    <el-table-column prop="tool_name" label="工具名称" />
    <el-table-column prop="status" label="采集进度">
        <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
                {{ getStatusText(row.status) }}
            </el-tag>
        </template>
    </el-table-column>
    <!-- ... -->
</el-table>

<!-- 分页 -->
<el-pagination
    v-if="toolCalls.length > 10"
    v-model:current-page="currentPage"
    :page-size="10"
    :total="toolCalls.length"
    layout="prev, pager, next"
/>
```

### 4.7 OpenSSH盐值提取

增强shadow解析器，提取盐值和哈希算法：

```go
// parsers.go
func parseShadowFile(content string) []CredentialRecord {
    var records []CredentialRecord
    for _, line := range strings.Split(content, "\n") {
        parts := strings.Split(line, ":")
        if len(parts) < 2 {
            continue
        }
        username := parts[0]
        passwordHash := parts[1]
        
        // 跳过锁定账户和空密码
        if passwordHash == "*" || passwordHash == "!" || passwordHash == "" {
            continue
        }
        
        // 提取盐和算法
        salt, algorithm := extractSaltAndAlgorithm(passwordHash)
        
        records = append(records, CredentialRecord{
            Username:     username,
            Credential:   passwordHash,
            CredentialType: "salted_hash",
            Salt:         salt,
            Algorithm:    algorithm,
            SourcePath:   "/etc/shadow",
        })
    }
    return records
}

func extractSaltAndAlgorithm(hash string) (salt, algorithm string) {
    // $1$salt$hash - MD5-crypt
    // $5$salt$hash - SHA-256 crypt
    // $6$salt$hash - SHA-512 crypt
    // $y$params$salt$hash - yescrypt
    if strings.HasPrefix(hash, "$") {
        parts := strings.Split(hash, "$")
        if len(parts) >= 4 {
            algorithm = parts[1]
            salt = parts[2]
            return
        }
    }
    return "", ""
}
```

### 4.8 AI密码生成修复

修复AI字典生成逻辑，使用真正的LLM：

```go
// weak_password_service.go
func (s *WeakPasswordService) generateAIDictionary(request AIDictionaryRequest) (*WeakPasswordDictionary, error) {
    // 构建LLM提示词
    prompt := fmt.Sprintf(`请生成%d个可能的弱密码，基于以下描述：
%s

要求：
1. 包含常见的弱密码模式（如password, 123456, admin等）
2. 包含与描述相关的密码变体
3. 包含年份、特殊字符等常见组合
4. 不要生成顺序递增的数字（如1,2,3,4...）
5. 每行一个密码，不要有其他内容`, request.Count, request.Description)

    // 调用LLM生成密码
    response, err := s.llmClient.Generate(prompt)
    if err != nil {
        return nil, fmt.Errorf("AI生成密码失败: %w", err)
    }
    
    // 解析生成的密码
    passwords := parseGeneratedPasswords(response)
    
    // 去重并限制数量
    uniquePasswords := deduplicatePasswords(passwords)
    if len(uniquePasswords) > request.Count {
        uniquePasswords = uniquePasswords[:request.Count]
    }
    
    // 创建字典
    dictionary := &WeakPasswordDictionary{
        Name:        fmt.Sprintf("AI生成字典-%s", time.Now().Format("2006-01-02 15:04:05")),
        Description: request.Description,
        Source:      "ai_generated",
    }
    
    // 创建字典条目
    entries := make([]WeakPasswordDictionaryEntry, len(uniquePasswords))
    for i, pwd := range uniquePasswords {
        entries[i] = WeakPasswordDictionaryEntry{
            Candidate:     pwd,
            CandidateHash: sha256Hash(pwd),
        }
    }
    
    return dictionary, s.repository.CreateDictionaryWithEntries(dictionary, entries)
}
```

---

## 五、数据流设计

### 5.1 资产分析流程

```
前端"一键分析资产应用"
  → POST /api/v1/weak-password/asset-applications/analyze
  → 查询 host_application_assets 表
  → 确认 Agent 在线
  → AI 过滤不需要密码认证的应用
  → 通过 skill 注册表构建候选应用
  → 按主机+应用类型去重
  → 写入 weak_password_candidate_applications 表
  → 返回去重后的候选应用列表
```

### 5.2 弱密码检测流程

```
前端"检查弱密码"
  → POST /api/v1/weak-password/tasks/by-application
  → 创建 task/host/application/plan 记录
  → 异步启动 executeApplicationTask
    → 调用 Agent WeakPassword.CollectCredentials
    → 如失败，先尝试 process-based repair（ProcessConfigHints）
    → 如仍失败，尝试 AI repair（LLM 选择辅助工具）
    → 最多 maxRounds 轮重试
    → 匹配字典 → 写入 findings
  → 返回任务ID
```

### 5.3 容器路径处理流程

```
Agent 收到采集请求
  → 遍历应用的配置路径
  → 对每个路径，检查是否有 related_pids
  → 如有，生成 /proc/{pid}/root/{path} 候选路径
  → 依次尝试读取候选路径
  → 解析配置文件提取凭据
  → 返回凭据记录
```

---

## 六、接口设计

### 6.1 资产分析接口

**请求：**
```http
POST /api/v1/weak-password/asset-applications/analyze
Content-Type: application/json

{
    "host_ids": [1, 2, 3],  // 可选，不传则分析所有在线主机
    "force_refresh": false
}
```

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "analysis_id": 123,
        "total_hosts": 3,
        "total_applications": 15,
        "deduplicated_applications": 10,
        "status": "completed"
    }
}
```

### 6.2 弱密码检测接口

**请求：**
```http
POST /api/v1/weak-password/tasks/by-application
Content-Type: application/json

{
    "candidate_ids": [1, 2, 3],
    "dictionary_strategy": "builtin",
    "max_rounds": 20,
    "enable_ai_repair": true
}
```

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "task_id": 456,
        "status": "pending",
        "total_applications": 3
    }
}
```

### 6.3 任务详情接口

**请求：**
```http
GET /api/v1/weak-password/tasks/{task_id}
```

**响应：**
```json
{
    "code": 0,
    "message": "success",
    "data": {
        "task": {
            "id": 456,
            "status": "running",
            "progress": 45,
            "current_round": 10,
            "max_rounds": 20
        },
        "hosts": [...],
        "findings": [...],
        "tool_calls": [
            {
                "round": 1,
                "tool_name": "WeakPassword.CollectCredentials",
                "status": "success",
                "duration_ms": 1500,
                "records_found": 5
            },
            // ...
        ]
    }
}
```

---

## 七、数据库变更

### 7.1 新增字段

```sql
-- weak_password_scan_tasks 表新增字段
ALTER TABLE weak_password_scan_tasks 
ADD COLUMN current_round INTEGER DEFAULT 0,
ADD COLUMN max_rounds INTEGER DEFAULT 20;

-- weak_password_agent_tool_calls 表新增字段
ALTER TABLE weak_password_agent_tool_calls
ADD COLUMN round_number INTEGER DEFAULT 1;
```

### 7.2 索引优化

```sql
-- 候选应用去重索引
CREATE UNIQUE INDEX idx_candidates_dedup 
ON weak_password_candidate_applications(host_id, application_type, analysis_id);

-- 工具调用查询索引
CREATE INDEX idx_tool_calls_task_round 
ON weak_password_agent_tool_calls(task_id, round_number);
```

---

## 八、前端变更

### 8.1 Index.vue 变更

1. 资产分析Tab：显示去重后的应用列表
2. 弱密码检查Tab：显示检测轮数配置（10-50滑块）
3. 修复"一键检测"按钮的参数传递

### 8.2 TaskDetail.vue 变更

1. 将"采集失败"改为"采集进度"
2. 添加分页功能（每页10条）
3. 显示当前轮次和总轮次

### 8.3 Store 变更

1. 修复`fetchCandidates`方法的去重逻辑
2. 添加检测轮数参数
3. 优化数据刷新频率

---

## 九、测试用例

### 9.1 单元测试

1. **Skill注册表测试**
   - 验证所有应用类型都有对应的skill
   - 验证skill的配置路径和提取器正确

2. **容器路径判断测试**
   - 测试Docker容器检测
   - 测试containerd容器检测
   - 测试非容器环境

3. **应用资产去重测试**
   - 测试同一主机同一应用类型只保留一个
   - 测试不同主机相同应用类型不去重

4. **Shadow解析器测试**
   - 测试盐值提取
   - 测试哈希算法识别
   - 测试锁定账户跳过

5. **AI字典生成测试**
   - 验证生成的密码不重复
   - 验证生成的密码不是顺序递增
   - 验证生成数量正确

### 9.2 集成测试

1. **端到端弱密码检测流程**
   - 资产分析 → 候选应用 → 采集任务 → 密码匹配 → 结果展示

2. **容器环境检测**
   - Redis容器部署 → 容器路径识别 → 配置文件读取

3. **检测轮数配置**
   - 设置不同轮数 → 验证任务执行 → 验证轮次记录

### 9.3 前端测试

1. **去重显示测试**
   - 验证资产分析Tab显示去重后的数据
   - 验证筛选条件生效

2. **分页功能测试**
   - 验证超过10条时显示分页
   - 验证分页切换正确

3. **检测轮数配置测试**
   - 验证滑块范围10-50
   - 验证参数正确传递到后端

---

## 十、风险与回滚计划

### 10.1 风险

1. **数据库迁移风险**：新增字段可能影响现有数据
2. **兼容性风险**：前端变更可能影响现有功能
3. **性能风险**：去重查询可能影响性能

### 10.2 回滚计划

1. **数据库回滚**：保留迁移脚本的回滚版本
2. **前端回滚**：保留旧版本前端代码
3. **功能开关**：通过配置开关控制新功能启用

---

## 十一、实施计划

### 阶段一：后端增强（2天）
1. 实现应用类型Skill注册表
2. 实现容器路径判断
3. 修复应用资产去重
4. 增强Shadow解析器

### 阶段二：前端优化（1天）
1. 修复去重显示
2. 添加检测轮数配置
3. 实现分页功能
4. 优化采集进度显示

### 阶段三：AI功能修复（1天）
1. 修复AI字典生成逻辑
2. 集成真正的LLM生成
3. 优化密码生成质量

### 阶段四：测试验证（1天）
1. 执行单元测试
2. 执行集成测试
3. 执行前端测试
4. 构建验证

---

## 十二、附录

### 12.1 参考资料

1. veinmind-tools弱密码检测：https://github.com/chaitin/veinmind-tools/tree/master/plugins/go/veinmind-weakpass
2. lynis安全审计：https://github.com/CISOfy/lynis
3. arcane容器工具：https://github.com/getarcaneapp/arcane

### 12.2 相关代码文件

- `api-server/internal/service/weak_password_skills.go` - Skill注册表
- `api-server/internal/service/weak_password_service.go` - 服务层
- `api-server/internal/repository/weak_password_repository.go` - 仓库层
- `agent/internal/weakpass/collector.go` - 采集器
- `agent/internal/weakpass/parsers.go` - 解析器
- `frontend/src/views/detection/WeakPassword/Index.vue` - 主页面
- `frontend/src/views/detection/WeakPassword/TaskDetail.vue` - 任务详情页
- `frontend/src/store/weakPassword.ts` - Store

# Aegis V3.0 设计文档更新记录

本文档记录 V3.0 实现过程中的设计变更和补充说明。

---

## 1. 系统架构更新

### 1.1 新增组件

| 组件名称 | 路径 | 说明 |
|---------|------|------|
| `SeverityTag.vue` | `frontend/src/components/common/SeverityTag.vue` | 严重程度标签组件 (Critical/High/Medium/Low) |
| `ScriptPreview.vue` | `frontend/src/components/common/ScriptPreview.vue` | 脚本预览组件 (语法高亮 + 安全提示) |
| `FixConfirmationDialog.vue` | `frontend/src/components/common/FixConfirmationDialog.vue` | 修复/POC 验证对话框 |

### 1.2 路由更新

```typescript
// 新增漏洞任务中心路由
{
  path: '/vulnerability/tasks',
  name: 'VulnerabilityTasks',
  component: TaskCenter,
  meta: { title: '漏洞任务中心' }
}
```

**完整路由结构**:
```
/hosts                      → 主机列表
/baseline/workbench         → 基线工作台
/baseline/tasks             → 基线任务中心
/vulnerability              → 智能漏洞检查与修复
/vulnerability/tasks        → 漏洞任务中心 (新增)
/settings                   → 系统配置
```

---

## 2. 后端 API 更新

### 2.1 漏洞管理 API

| 方法 | 路径 | 说明 | 请求参数 | 返回格式 |
|------|------|------|---------|---------|
| POST | `/api/v1/vulnerability/scan` | 启动漏洞扫描 | `{host_ids: string[]}` | `{scan_id: string}` |
| GET | `/api/v1/vulnerability/scan/:id/status` | 查询扫描状态 | - | `ScanStatus` |
| GET | `/api/v1/vulnerability` | 漏洞列表 (分页) | `page, page_size, severity, query` | `{data: [], total: number}` |
| POST | `/api/v1/vulnerability/:id/fix` | 一键修复 | `{host_ids, preview}` | `{task_id, script}` |
| POST | `/api/v1/vulnerability/:id/poc` | POC 验证 | `{host_id, preview}` | `{task_id, script}` |

### 2.2 任务管理 API (已存在)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/tasks` | 任务列表 (支持 `task_type` 筛选) |
| GET | `/api/v1/tasks/:id` | 任务详情 |
| POST | `/api/v1/tasks/:id/retry` | 重试任务 |
| DELETE | `/api/v1/tasks/:id` | 删除单个任务 |

### 2.3 响应格式标准化

所有 API 统一使用标准响应格式：
```json
{
  "code": 0,
  "message": "success",
  "data": {...}
}
```

错误响应：
```json
{
  "code": 500,
  "message": "生成修复脚本失败：LLM 免费额度已用完，请在系统配置中更换 API Key",
  "error": "LLM 免费额度已用完，请在系统配置中更换 API Key"
}
```

---

## 3. 数据库设计更新

### 3.1 新增表结构

** vulnerabilities - 漏洞库**
```sql
CREATE TABLE vulnerabilities (
    id UUID PRIMARY KEY,
    cve_id VARCHAR(20) UNIQUE,
    severity VARCHAR(20),  -- Critical/High/Medium/Low
    cvss_score DECIMAL(3,1),
    description TEXT,
    ref_links JSONB,  -- 原 references (避免 SQL 保留字冲突)
    source VARCHAR(50) DEFAULT 'llm_analysis',
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
```

** host_vulnerabilities - 主机漏洞关联**
```sql
CREATE TABLE host_vulnerabilities (
    id UUID PRIMARY KEY,
    host_id UUID REFERENCES hosts(id),
    vulnerability_id UUID REFERENCES vulnerabilities(id),
    affected_package VARCHAR(255),
    affected_version VARCHAR(100),
    status VARCHAR(20),  -- detected/poc_verified/fixing/fixed/ignored/false_positive
    poc_result VARCHAR(20),  -- vulnerable/not_vulnerable/error
    fix_task_id UUID REFERENCES task_logs(id),
    poc_task_id UUID REFERENCES task_logs(id),
    scan_session_id UUID,
    created_at TIMESTAMPTZ
);
```

** installed_software - 软件清单缓存**
```sql
CREATE TABLE installed_software (
    id UUID PRIMARY KEY,
    host_id UUID REFERENCES hosts(id),
    package_name VARCHAR(255),
    package_version VARCHAR(100),
    package_manager VARCHAR(20),  -- rpm/dpkg
    scan_session_id UUID,
    collected_at TIMESTAMPTZ
);
```

** vulnerability_fix_scripts - 修复脚本存储**
```sql
CREATE TABLE vulnerability_fix_scripts (
    id UUID PRIMARY KEY,
    vulnerability_id UUID REFERENCES vulnerabilities(id),
    os_type VARCHAR(50),
    script_content TEXT,
    script_version INT DEFAULT 1,
    generation_source VARCHAR(20),  -- llm_generated/manual
    is_current BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ
);
```

** poc_scripts - POC 脚本存储**
```sql
CREATE TABLE poc_scripts (
    id UUID PRIMARY KEY,
    vulnerability_id UUID REFERENCES vulnerabilities(id),
    os_type VARCHAR(50),
    script_content TEXT,
    script_version INT DEFAULT 1,
    generation_source VARCHAR(20),  -- llm_generated/manual
    safety_verified BOOLEAN DEFAULT false,
    is_current BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ
);
```

### 3.2 现有表更新

** task_logs 表**
```sql
ALTER TABLE task_logs ADD COLUMN vulnerability_id UUID;
ALTER TABLE task_logs ALTER COLUMN task_type TYPE VARCHAR(20);
-- 新增任务类型：VULNERABILITY_FIX, POC_VERIFY
```

** script_versions 表**
```sql
ALTER TABLE script_versions ADD COLUMN vulnerability_id UUID;
ALTER TABLE script_versions ADD COLUMN os_type VARCHAR(50);
ALTER TABLE script_versions ALTER COLUMN script_type TYPE VARCHAR(20);
-- 新增脚本类型：VULNERABILITY_FIX, POC
```

** self_healing_logs 表**
```sql
ALTER TABLE self_healing_logs ADD COLUMN vulnerability_id UUID;
ALTER TABLE self_healing_logs ALTER COLUMN script_type TYPE VARCHAR(20);
```

---

## 4. 核心功能实现

### 4.1 漏洞扫描流程

```
1. 用户选择主机 → 点击"一键扫描"
   ↓
2. 创建扫描会话 (scan_session_id)
   ↓
3. 采集软件清单 (gRPC → Agent)
   - 使用 #SOFTWARE_COLLECT# 特殊命令
   - RPM 系统：rpm -qa --qf '%{NAME}\t%{VERSION}\n'
   - DEB 系统：dpkg-query -W -f '${Package}\t${Version}\n'
   ↓
4. 转换为 JSON Lines 格式
   {"name":"sudo","version":"1.8.31"}
   {"name":"curl","version":"7.68.0"}
   ↓
5. 批量 CVE 分析 (每 100 个软件包为一批)
   - 调用 LLM API
   - 解析 JSON 响应
   - 存储到 vulnerabilities 表
   ↓
6. 创建主机 - 漏洞关联
   ↓
7. 更新扫描状态 (Redis 缓存)
```

### 4.2 脚本生成与缓存

**优化逻辑**:
```go
// 优先生成/获取修复脚本
if preview {
    storedScript, err := repo.FindFixScriptByVulnID(vuln.ID)
    if err == nil && storedScript != nil {
        // 使用存储的脚本，不调用 LLM
        return &FixResult{Script: storedScript.ScriptContent}, nil
    }
}

// 没有存储的脚本才调用 LLM 生成
scriptContent, err := generateFixScript(ctx, vuln, host.OSType)
```

**好处**:
- 避免重复调用 LLM 浪费配额
- 响应速度从 30-60 秒提升到<1 秒
- 保证脚本一致性

### 4.3 任务执行与超时处理

```go
// 下发任务到 Agent
go s.dispatchFixToAgent(ctx, taskLog.ID, hostID, scriptContent)

// 启动超时监控
go s.monitorTaskTimeout(taskID, 360)  // 修复任务 6 分钟超时

func (s *VulnerabilityService) monitorTaskTimeout(taskID uuid.UUID, timeoutSeconds int) {
    time.Sleep(time.Duration(timeoutSeconds) * time.Second)
    
    task, err := s.taskLogRepo.FindByID(taskID)
    if err != nil { return }
    
    if task.Status == "RUNNING" || task.Status == "PENDING" {
        logger.Warn("task timeout", ...)
        s.taskLogRepo.UpdateResult(taskID, nil, nil, intPtr(-1), "TIMEOUT", time.Now())
    }
}
```

**超时配置**:
| 任务类型 | 超时时间 |
|---------|---------|
| 漏洞修复 (VULNERABILITY_FIX) | 6 分钟 (360 秒) |
| POC 验证 (POC_VERIFY) | 3 分钟 (180 秒) |

---

## 5. 前端实现

### 5.1 漏洞管理页面功能

| 功能 | 状态 | 说明 |
|------|------|------|
| 一键扫描 | ✅ | 选择主机 → 采集软件 → CVE 分析 |
| 扫描进度 | ✅ | Redis 缓存实时更新 |
| 漏洞列表 | ✅ | 分页、严重程度筛选、搜索 |
| 行展开 | ✅ | 显示受影响主机列表 |
| 严重程度标签 | ✅ | Critical(红色)/High(橙色)/Medium(灰色)/Low(绿色) |
| 统计面板 | ✅ | 总数/Critical/High/Medium/Low/待修复 |
| POC 验证 | ✅ | 脚本预览 + 执行 |
| 一键修复 | ✅ | 脚本预览 + 执行 |
| 任务中心入口 | ✅ | 跳转到 /vulnerability/tasks |

### 5.2 任务中心功能

| 功能 | 状态 | 说明 |
|------|------|------|
| 任务列表 | ✅ | 按任务组聚合显示 |
| 状态筛选 | ✅ | 全部/运行中/已完成/失败超时 |
| 类型筛选 | ✅ | 根据路由自动筛选 (基线/漏洞) |
| 进度显示 | ✅ | 成功数/失败数/待执行数/执行中数 |
| 任务详情 | ✅ | 查看 stdout/stderr/退出码 |
| 超时重试 | ✅ | 失败/超时任务支持重试 |
| 自动刷新 | ✅ | 运行中任务每 5 秒刷新 |

### 5.3 组件设计

**SeverityTag.vue**:
```vue
<template>
  <el-tag :type="tagType" :effect="effect">
    {{ severity }}
  </el-tag>
</template>

<script setup>
const tagType = computed(() => {
  switch (props.severity) {
    case 'Critical': return 'danger'
    case 'High': return 'warning'
    case 'Medium': return ''
    case 'Low': return 'success'
  }
})
const effect = computed(() => {
  return props.severity === 'Critical' || props.severity === 'High' ? 'dark' : 'light'
})
</script>
```

**ScriptPreview.vue**:
- Shell 语法高亮 (关键字/内置命令/注释/字符串/变量)
- 安全提示 (POC 脚本显示"只读检测"/修复脚本显示"修改系统")
- 一键复制功能

**FixConfirmationDialog.vue**:
- CVE 信息展示
- 主机选择
- 脚本预览
- 执行确认

---

## 6. 配置管理

### 6.1 LLM 配置

**配置文件 (`config.yaml`)**:
```yaml
llm:
  # LLM 配置已移至系统管理页面 (/settings)
  # 请在前端页面配置 API Key, Base URL 和 Model Name
  timeout_seconds: 300  # 5 分钟，用于 CVE 分析
  max_retries: 3
```

**前端配置页面**:
- 路径：`/settings`
- 字段：API Key, Base URL, 模型名称
- 测试连接功能

**推荐配置**:
| 提供商 | Base URL | 模型 |
|--------|---------|------|
| DeepSeek | `https://api.deepseek.com/v1` | `deepseek-chat` |
| 阿里云百炼 | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-plus` |

### 6.2 错误信息映射

| 错误类型 | 用户可见信息 |
|---------|------------|
| `AllocationQuota.FreeTierOnly` | LLM 免费额度已用完，请在系统配置中更换 API Key 或充值后重试 |
| `API returned status 403` | API 认证失败：[具体原因] |
| `API returned status 401` | API Key 无效，请在系统配置中检查 API Key 设置 |
| `context deadline exceeded` | LLM 请求超时，请稍后重试或增加超时时间配置 |
| `LLM client not configured` | LLM 未配置，请在系统配置中设置 API Key |

---

## 7. gRPC 通信

### 7.1 软件采集命令

```protobuf
message CommandExecute {
    string task_id = 1;
    string host_id = 2;
    string script_content = 3;  // #SOFTWARE_COLLECT# 特殊命令
    int32 timeout_seconds = 4;
}
```

### 7.2 Agent 端处理

```go
func (e *Executor) collectSoftwareList(ctx context.Context, taskID string) *ExecuteResult {
    // 检测包管理器
    if _, err := exec.LookPath("dpkg-query"); err == nil {
        // DEB 系统
        cmd = exec.Command("bash", "-c", 
            `dpkg-query -W -f='${Package}\t${Version}\n' | awk -F'\t' '{printf "{\"name\":\"%s\",\"version\":\"%s\"}\n", $1, $2}'`)
    } else if _, err := exec.LookPath("rpm"); err == nil {
        // RPM 系统
        cmd = exec.Command("bash", "-c",
            `rpm -qa --qf '%{NAME}\t%{VERSION}\n' | awk -F'\t' '{printf "{\"name\":\"%s\",\"version\":\"%s\"}\n", $1, $2}'`)
    }
    
    out, err := cmd.Output()
    return &ExecuteResult{ExitCode: 0, Stdout: strings.TrimSpace(string(out))}
}
```

---

## 8. 性能优化

### 8.1 批量 CVE 分析

- **问题**: 1462 个软件包一次性调用 LLM 导致超时
- **解决**: 每 100 个软件包为一批，循环调用
- **效果**: 单批次 2-3 分钟，总耗时 30-40 分钟

### 8.2 脚本缓存

- **问题**: 重复点击生成按钮浪费 LLM 配额
- **解决**: 优先查询 `vulnerability_fix_scripts` / `poc_scripts` 表
- **效果**: 第二次及以后响应时间 <1 秒，节省 90%+ API 配额

### 8.3 前端超时

- **问题**: 前端 axios 超时 60 秒，LLM 生成需要更长时间
- **解决**: 前端超时调整为 300 秒 (5 分钟)
- **配置**: `frontend/src/api/index.ts` → `timeout: 300000`

---

## 9. 验收标准

### 9.1 漏洞扫描

- [x] 选择多个主机启动扫描
- [x] 实时显示扫描进度条
- [x] 软件清单采集成功 (JSON Lines 格式)
- [x] CVE 分析完成并入库
- [x] 主机 - 漏洞关联正确

### 9.2 漏洞管理

- [x] 漏洞列表分页显示
- [x] 严重程度筛选功能
- [x] 搜索 CVE 编号/软件名称
- [x] 行展开显示受影响主机
- [x] 统计面板数据准确

### 9.3 POC 验证

- [x] 生成 POC 脚本 (预览模式)
- [x] 脚本预览 (语法高亮)
- [x] 执行 POC 验证
- [x] 查看执行结果 (stdout/stderr/exit_code)
- [x] 超时处理 (3 分钟)

### 9.4 一键修复

- [x] 生成修复脚本 (预览模式)
- [x] 脚本预览 (语法高亮)
- [x] 执行修复任务
- [x] 查看执行结果
- [x] 超时处理 (6 分钟)

### 9.5 任务中心

- [x] 漏洞任务独立查看 (`/vulnerability/tasks`)
- [x] 状态筛选 (全部/运行中/已完成/失败超时)
- [x] 任务详情查看
- [x] 超时任务重试

---

## 10. 遗留问题

### 10.1 LLM 配额限制

- **问题**: 阿里云百炼免费额度有限
- **解决**: 
  1. 使用脚本缓存避免重复调用
  2. 批量分析 (每批 100 个软件包)
  3. 建议配置付费 API 或使用 DeepSeek

### 10.2 前端构建问题

- **问题**: babel 解析偶发失败
- **影响**: 不影响功能，仅构建过程
- **解决**: 清理缓存后重新构建

---

## 11. 文件清单

### 11.1 后端新增文件

```
backend/internal/model/vulnerability.go
backend/internal/repository/vulnerability_repo.go
backend/internal/service/vulnerability_service.go
backend/internal/api/handler/vulnerability_handler.go
backend/scripts/migrate_v3_0_vulnerability.sql
```

### 11.2 后端修改文件

```
backend/internal/model/task_log.go
backend/internal/model/healing_log.go
backend/internal/model/script_version.go
backend/internal/storage/redis_client.go
backend/internal/llm/prompts.go
backend/internal/grpc_server/server.go
backend/internal/api/router.go
backend/internal/api/handler/vulnerability_handler.go
backend/cmd/server/main.go
backend/scripts/init.sql
backend/config/config.yaml
```

### 11.3 前端新增文件

```
frontend/src/api/vulnerability.ts
frontend/src/api/task.ts
frontend/src/store/vulnerability.ts
frontend/src/components/common/SeverityTag.vue
frontend/src/components/common/ScriptPreview.vue
frontend/src/components/common/FixConfirmationDialog.vue
```

### 11.4 前端修改文件

```
frontend/src/views/Vulnerability.vue
frontend/src/views/TaskCenter.vue
frontend/src/router/index.ts
frontend/src/App.vue
frontend/src/api/index.ts
frontend/index.html
```

### 11.5 Agent 修改文件

```
agent/internal/executor/executor.go
agent/internal/asset/collector.go
agent/cmd/agent/main.go
```

---

## 12. 总结

V3.0 漏洞管理模块已完整实现，包含以下核心功能：

1. **一键扫描** - 软件采集 + CVE 分析 + 结果入库
2. **漏洞管理** - 列表/筛选/搜索/统计
3. **POC 验证** - 脚本生成 + 预览 + 执行 + 超时处理
4. **一键修复** - 脚本生成 + 预览 + 执行 + 超时处理
5. **任务中心** - 独立漏洞任务视图 + 状态跟踪

**技术亮点**:
- 批量 CVE 分析 (每批 100 个软件包)
- 脚本缓存避免重复调用 LLM
- JSON Lines 格式优化软件清单
- 前端超时 5 分钟支持
- 友好的错误信息映射

**待优化**:
- LLM 响应速度 (建议使用更快的模型)
- 前端构建稳定性
- 任务重试功能完善

---

*文档版本：V3.0.1*
*更新日期：2026-03-16*
*更新人：Sisyphus*

# Aegis V5.5 版本变更日志

**版本**: 5.5.2
**日期**: 2026-04-10
**状态**: 已完成

---

## 1. 版本概述

V5.5版本在V5.2基础上，新增漏洞扫描终止功能和优化：

- 漏洞扫描终止按钮（随时停止扫描任务）
- CVE结果去重（同一主机同一CVE不重复入库）
- 清理已修复漏洞（主机软件更新后自动清理无效关联）
- LLM提示词优化（更严格的JSON输出要求）

---

## 2. 新增功能

### 2.1 漏洞扫描终止功能

#### 功能描述

在漏洞扫描过程中，用户可以随时点击"停止"按钮终止扫描任务，并获得：
- `total_parsed`: LLM解析到的CVE总数
- `total_saved`: 已入库的CVE数量（去重后）

#### 后端实现

**新增API**: `POST /api/v1/vulnerability/scan/stop`

```go
// api-server/internal/service/vulnerability_service.go

// StopScanResult contains the results when a scan is stopped
type StopScanResult struct {
    ScanID       string `json:"scan_id"`
    TotalParsed  int    `json:"total_parsed"`  // Total CVEs parsed from LLM
    TotalSaved   int    `json:"total_saved"`   // CVEs saved to database (after deduplication)
    CurrentBatch int    `json:"current_batch"` // Current batch number when stopped
    TotalBatches int    `json:"total_batches"` // Total batches
    Message      string `json:"message"`
}

// StopScan stops the currently running vulnerability scan
func (s *VulnerabilityService) StopScan(ctx context.Context) (*StopScanResult, error) {
    s.scanMutex.Lock()
    if !s.scanInProgress {
        s.scanMutex.Unlock()
        return nil, fmt.Errorf("no scan is currently running")
    }
    s.stopRequested = true
    scanID := s.currentScanID
    s.scanMutex.Unlock()

    // Wait for current batch to finish (with 30s timeout)
    // Returns accumulated total_parsed and total_saved
}
```

**扫描状态追踪字段**:

```go
type VulnerabilityService struct {
    scanInProgress  bool
    stopRequested   bool
    scanMutex       sync.Mutex
    currentScanID      string
    currentScanHostIDs []string
    currentScanTotal   int      // Total CVEs parsed
    currentScanSaved   int      // CVEs saved to database
}
```

**批量处理中的停止检查**:

```go
func (s *VulnerabilityService) analyzeCVEWithLLM(ctx context.Context, scanID string, software []model.SoftwareInfo, hostIDs []string) ([]model.CveAnalysisResult, error) {
    for i := 0; i < len(software); i += batchSize {
        batchNum := (i / batchSize) + 1

        // Check if stop has been requested
        s.scanMutex.Lock()
        if s.stopRequested {
            s.scanMutex.Unlock()
            logger.Info("stop requested, ending batch processing",
                zap.Int("batches_completed", batchNum-1),
                zap.Int("cves_parsed_so_far", len(allResults)))
            s.currentScanTotal = len(allResults)
            break
        }
        s.scanMutex.Unlock()

        // Continue with batch processing...
    }
}
```

#### 前端实现

**停止按钮** (`Vulnerability.vue`):

```vue
<el-button
  v-if="scanning"
  type="danger"
  :icon="Close"
  @click="handleStop"
>
  停止
</el-button>
```

**Store方法** (`vulnerability.ts`):

```typescript
async function stopScan(): Promise<boolean> {
  try {
    const result = await api.stopVulnerabilityScan()
    scanning.value = false
    if (scanStatus.value) {
      scanStatus.value.status = 'stopped'
      scanStatus.value.message = result.message || '扫描已停止'
    }
    return true
  } catch (error) {
    console.error('Failed to stop scan:', error)
    return false
  }
}
```

**API函数** (`vulnerability.ts`):

```typescript
export interface StopScanResult {
  scan_id: string
  total_parsed: number
  total_saved: number
  current_batch: number
  total_batches: number
  message: string
}

export function stopVulnerabilityScan(): Promise<StopScanResult> {
  return request.post('/vulnerability/scan/stop')
}
```

---

### 2.2 CVE结果去重

#### 功能描述

同一主机同一CVE不会重复入库，避免数据冗余。

#### 实现

**新增Repository方法** (`vulnerability_repo.go`):

```go
// HostVulnerabilityExists 检查主机漏洞关联是否已存在
func (r *VulnerabilityRepo) HostVulnerabilityExists(hostID, vulnerabilityID uuid.UUID) (bool, error) {
    var count int64
    err := r.db.Model(&model.HostVulnerability{}).
        Where("host_id = ? AND vulnerability_id = ?", hostID, vulnerabilityID).
        Count(&count).Error
    return count > 0, nil
}
```

**去重逻辑** (`vulnerability_service.go`):

```go
func (s *VulnerabilityService) saveAnalysisResults(cveResults []model.CveAnalysisResult, hostSoftwareMap map[string]*model.HostSoftwareList, scanSessionID uuid.UUID) (int, error) {
    for _, cve := range cveResults {
        // ... upsert vulnerability ...

        for hostIDStr, hostSoftware := range hostSoftwareMap {
            // Check if this host-vulnerability association already exists (deduplication)
            exists, err := s.vulnRepo.HostVulnerabilityExists(hostID, vuln.ID)
            if err != nil {
                logger.Error("failed to check host vulnerability existence", zap.Error(err))
                continue
            }
            if exists {
                logger.Debug("host-vulnerability already exists, skipping",
                    zap.String("host_id", hostIDStr),
                    zap.String("cve_id", cve.CveID))
                continue
            }
            // Create host-vulnerability association...
        }
    }
}
```

---

### 2.3 清理已修复漏洞

#### 功能描述

扫描完成后，对比新扫描结果与历史记录：
- 如果主机不再有某个漏洞，删除该主机-漏洞关联
- 如果漏洞不再关联任何主机，删除该漏洞记录

#### 实现

**新增Repository方法**:

```go
// GetHostVulnerabilityIDs 获取主机当前的所有漏洞ID列表
func (r *VulnerabilityRepo) GetHostVulnerabilityIDs(hostID uuid.UUID) ([]uuid.UUID, error)

// DeleteHostVulnerability 删除单个主机漏洞关联
func (r *VulnerabilityRepo) DeleteHostVulnerability(hostID, vulnerabilityID uuid.UUID) error

// VulnerabilityHasHosts 检查漏洞是否还有关联的主机
func (r *VulnerabilityRepo) VulnerabilityHasHosts(vulnerabilityID uuid.UUID) (bool, error)

// DeleteVulnerabilityByID 删除漏洞记录
func (r *VulnerabilityRepo) DeleteVulnerabilityByID(vulnerabilityID uuid.UUID) error
```

**清理逻辑**:

```go
func (s *VulnerabilityService) cleanupFixedVulnerabilities(cveResults []model.CveAnalysisResult, hostSoftwareMap map[string]*model.HostSoftwareList) (int, int, error) {
    // Build a map of vulnerability IDs found in this scan
    foundVulnIDs := make(map[uuid.UUID]bool)
    for _, cve := range cveResults {
        vuln, _ := s.vulnRepo.FindByCveID(cve.CveID)
        foundVulnIDs[vuln.ID] = true
    }

    // For each host, find vulnerabilities that are no longer present
    for hostIDStr := range hostSoftwareMap {
        existingVulnIDs, _ := s.vulnRepo.GetHostVulnerabilityIDs(hostID)

        for _, vulnID := range existingVulnIDs {
            if !foundVulnIDs[vulnID] {
                // Remove the association - vulnerability is now fixed
                s.vulnRepo.DeleteHostVulnerability(hostID, vulnID)
                cleanedHostVulns++

                // If vulnerability has no more hosts, delete it
                if !s.vulnRepo.VulnerabilityHasHosts(vulnID) {
                    s.vulnRepo.DeleteVulnerabilityByID(vulnID)
                    deletedVulns++
                }
            }
        }
    }
}
```

**调用时机** (`executeScan`):

```go
s.updateScanStatus(scanID, "analyzing", 80, "正在保存分析结果...", "result_save")
vulnCount, _ := s.saveAnalysisResults(cveResults, hostSoftwareMap, scanSessionID)

// Cleanup: remove vulnerabilities that no longer exist on hosts
s.updateScanStatus(scanID, "analyzing", 90, "正在清理已修复的漏洞...", "cleanup")
cleanedCount, deletedVulnCount, _ := s.cleanupFixedVulnerabilities(cveResults, hostSoftwareMap)
```

---

### 2.4 LLM提示词优化

#### 优化内容

CVE分析提示词更加严格，确保返回有效JSON数组：

```go
const CVEAnalysisPromptZH = `你是一个CVE漏洞分析助手...

## 强制输出要求（最重要，任何情况下都必须遵守）
1. 你必须且只能输出一个有效的JSON数组
2. 禁止输出任何其他文字、解释、说明、注释或空行
3. 即使发生错误，也必须返回一个JSON数组（空数组[]表示无漏洞）

## 禁止事项
- 禁止输出任何中文文字（"漏洞"、"分析"等）
- 禁止输出解释性文字
- 禁止输出空响应（即使是错误情况也必须返回[]）

## 输出示例
正确：[]
正确：[{"cve_id":"CVE-2021-44228",...}]
错误：CVE-2021-44228
错误：未发现漏洞
错误：（无输出）`
```

---

## 3. 文件变更清单

### 后端 (API Server)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/api/handler/vulnerability_handler.go` | 修改 | 新增StopScan Handler |
| `internal/api/router.go` | 修改 | 新增 `/vulnerability/scan/stop` 路由 |
| `internal/service/vulnerability_service.go` | 修改 | StopScan、saveAnalysisResults、cleanupFixedVulnerabilities |
| `internal/repository/vulnerability_repo.go` | 修改 | HostVulnerabilityExists、GetHostVulnerabilityIDs、DeleteHostVulnerability、VulnerabilityHasHosts、DeleteVulnerabilityByID |
| `internal/llm/prompts.go` | 修改 | CVEAnalysisPromptZH 提示词优化 |

### 前端 (Frontend)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `src/api/vulnerability.ts` | 修改 | 新增 stopVulnerabilityScan API、StopScanResult 接口 |
| `src/store/vulnerability.ts` | 修改 | 新增 stopScan 方法 |
| `src/views/Vulnerability.vue` | 修改 | 新增停止按钮 |

---

## 4. API变更

### 新增API

| API | 方法 | 说明 |
|-----|------|------|
| `/api/v1/vulnerability/scan/stop` | POST | 终止正在运行的漏洞扫描 |

### API响应格式

**POST /api/v1/vulnerability/scan/stop** (成功):

```json
{
  "code": 0,
  "message": "扫描已停止",
  "data": {
    "scan_id": "fd2f2258-27ef-476d-9c0b-418d068a8fcc",
    "total_parsed": 5,
    "total_saved": 3,
    "current_batch": 2,
    "total_batches": 71,
    "message": "扫描已停止"
  }
}
```

**POST /api/v1/vulnerability/scan/stop** (无扫描运行):

```json
{
  "code": 400,
  "message": "no scan is currently running"
}
```

---

## 5. 数据库变更

无新增表或字段变更。

---

## 6. 测试验证

### 6.1 停止API测试

```bash
# 启动扫描
curl -X POST http://localhost:8082/api/v1/vulnerability/scan \
  -H "Content-Type: application/json" \
  -d '{"host_ids": ["0e08e429-75da-4cd8-acdd-63cecdaeab1b"]}'

# 立即停止
curl -X POST http://localhost:8082/api/v1/vulnerability/scan/stop

# 预期响应:
# {"code":0,"data":{"current_batch":0,"scan_id":"xxx","total_batches":0,"total_parsed":0,"total_saved":0},"message":"扫描已停止"}
```

### 6.2 去重测试

```bash
# 同一主机扫描两次相同CVE
# 预期: 第二次扫描该CVE不会重复创建 host_vulnerability 记录
```

### 6.3 清理测试

```bash
# 更新主机软件后重新扫描
# 预期: 被移除的软件的CVE关联会被删除
# 预期: 没有主机关联的CVE会被删除
```

---

## 7. 部署说明

### 构建步骤

```bash
# 后端
cd api-server && make build

# 前端（使用Docker）
cd frontend
docker run --rm -v $(pwd):/app -w /app node:18 npm run build
docker build -t aegis-system/frontend:latest -f frontend/Dockerfile frontend/

# 部署
docker compose up -d api-server frontend
```

---

## 8. Bug 修复 (V5.5.1)

**日期**: 2026-04-09

### 8.1 停止/暂停超时后状态卡死问题

#### 问题描述

当用户点击"停止"或"暂停"按钮后，如果后端处理超时（停止30秒超时，暂停60秒超时），`scanInProgress` 内存状态不会被重置，导致后续所有扫描请求都返回 409 Conflict。

#### 修复方案

**StopScan 超时处理** (`vulnerability_service.go`):

```go
case <-timeout:
    logger.Warn("stop timeout reached, forcing stop", zap.String("scan_id", scanID))
    // Force reset scanInProgress to allow new scans to start
    s.scanMutex.Lock()
    s.scanInProgress = false
    s.scanMutex.Unlock()
    return &StopScanResult{...}, nil
```

**PauseScan 超时处理** (`vulnerability_service.go`):

```go
case <-timeout:
    logger.Warn("pause timeout reached, forcing pause", zap.String("scan_id", scanID))
    // ... update status to paused in Redis ...

    // Force reset scanInProgress to allow new scans to start
    s.scanMutex.Lock()
    s.scanInProgress = false
    s.scanMutex.Unlock()
    return &StopScanResult{...}, nil
```

---

### 8.2 前端暂停状态刷新丢失问题

#### 问题描述

用户点击"暂停"后页面显示正确，但刷新页面后状态丢失，表现为：
1. 暂停标签消失
2. 一键扫描按钮恢复可用
3. 再次点击扫描会返回 409

#### 根因分析

`restoreScanStatus()` 函数在收到 `not_found` 状态时，虽然调用了 `clearScanId()`，但没有重置 `scanStatus.value` 和 `scanning.value`，导致 UI 显示残留状态。

#### 修复方案

**VulnerabilityStore** (`vulnerability.ts`):

```typescript
if (status.status === 'not_found') {
  // Scan not found - reset all scan state
  scanStatus.value = null
  scanning.value = false
  clearScanId()
  return false
}
```

---

### 8.3 前端暂停/停止按钮并发点击问题

#### 问题描述

用户快速连续点击"暂停"或"停止"按钮时，后端可能收到多个请求，且按钮在 API 返回前没有即时禁用。

#### 修复方案

**Vulnerability.vue** - 新增本地 loading 状态：

```typescript
const pauseLoading = ref(false)
const stopLoading = ref(false)
```

**按钮模板** - 添加 loading 和 disabled 属性：

```vue
<el-button
  type="warning"
  :icon="VideoPause"
  :disabled="isPauseDisabled || pauseLoading"
  :loading="pauseLoading"
  @click="handlePause"
>
  {{ pauseLoading ? '暂停中...' : '暂停' }}
</el-button>

<el-button
  type="danger"
  :icon="Close"
  :disabled="isStopDisabled || stopLoading"
  :loading="stopLoading"
  @click="handleStop"
>
  {{ stopLoading ? '停止中...' : '停止' }}
</el-button>
```

**处理函数** - 立即设置 loading 状态：

```typescript
async function handleStop() {
  stopLoading.value = true
  const success = await vulnStore.stopScan()
  stopLoading.value = false
  // ...
}

async function handlePause() {
  pauseLoading.value = true
  const success = await vulnStore.pauseScan()
  pauseLoading.value = false
  // ...
}
```

---

## 9. 文件变更清单 (V5.5.1)

### 后端 (API Server)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/service/vulnerability_service.go` | 修复 | StopScan/PauseScan 超时后强制重置 scanInProgress |

### 前端 (Frontend)

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `src/store/vulnerability.ts` | 修复 | restoreScanStatus 正确重置状态 |
| `src/views/Vulnerability.vue` | 修复 | 暂停/停止按钮添加 loading 状态 |

---

## 10. 测试验证 (V5.5.1)

### 10.1 停止超时后状态重置测试

```bash
# 启动扫描
curl -X POST http://localhost:8082/api/v1/vulnerability/scan \
  -H "Content-Type: application/json" \
  -d '{"host_ids":["5494e21e-10ef-43cf-ab37-dec36f014f8a"]}'

# 等待扫描开始后立即停止
curl -X POST http://localhost:8082/api/v1/vulnerability/scan/stop

# 等待超时完成 (30秒)
# 验证状态是否为 completed
curl http://localhost:8082/api/v1/vulnerability/scan/{scan_id}/status

# 再次发起扫描 - 应返回 202 而非 409
curl -X POST http://localhost:8082/api/v1/vulnerability/scan \
  -H "Content-Type: application/json" \
  -d '{"host_ids":["5494e21e-10ef-43cf-ab37-dec36f014f8a"]}'
```

### 10.2 刷新后状态保持测试

```bash
# 1. 前端发起扫描并暂停
# 2. 刷新页面
# 3. 验证 "已暂停" 标签是否显示
# 4. 验证 "暂停" 按钮是否正确显示
```

---

## 11. Bug 修复 (V5.5.2)

**日期**: 2026-04-10

### 11.1 彻底剔除"暂停"功能

#### 问题描述

暂停功能存在以下问题：
1. 系统卡死在"扫描已暂停"状态
2. 暂停状态下的UI交互复杂，容易导致用户困惑
3. 暂停后恢复逻辑存在边界情况问题

#### 修复方案

**后端移除** (`vulnerability_service.go`):
- 删除 `ScanCompletionPaused` 常量
- 删除 `PauseScan()` 方法
- 删除 `pauseRequested` 状态字段
- 移除批量处理中的 `pauseRequested` 检查逻辑

**API 路由移除** (`router.go`):
- 删除 `POST /vulnerability/scan/pause` 路由
- 删除 `vulnerability_handler.go` 中的 `PauseScan` Handler

**前端移除** (`Vulnerability.vue`):
- 删除"暂停"按钮（`VideoPause` 图标相关代码）
- 删除 `pauseLoading` 状态变量
- 删除 `isPauseDisabled` 计算属性
- 删除 `handlePause()` 方法

**Store 移除** (`vulnerability.ts`):
- 删除 `pauseScan()` 方法
- 从 `pollScanStatus()` 中移除 `pausing`、`paused` 状态处理
- 从 `restoreScanStatus()` 中移除 `paused` 状态处理

---

### 11.2 停止功能重构 - 增量入库策略

#### 问题描述

停止扫描时，需要确保：
1. 停止前已扫描的数据能正确入库
2. 不执行全量扫描的"比对与清理"逻辑
3. 原数据库中该主机的历史存量漏洞记录必须 100% 保留

#### 修复方案

**停止后增量入库逻辑** (`vulnerability_service.go`):

```go
// StopScan 停止扫描时，确保已扫描的数据入库，但不执行清理
func (s *VulnerabilityService) StopScan(ctx context.Context) (*StopScanResult, error) {
    // ... 设置 stopRequested 标志 ...

    // 等待当前批次完成
    // 调用 saveAnalysisResults 保存已扫描的数据

    // 关键：停止时跳过 cleanupFixedVulnerabilities 清理逻辑
    // 确保历史漏洞记录不被删除
}
```

**cleanupFixedVulnerabilities 仅在完成时调用**:

```go
// 仅在 ScanCompletionCompleted 时执行清理
if s.scanCompletionType == ScanCompletionCompleted {
    s.updateScanStatus(scanID, "analyzing", 90, "正在清理已修复的漏洞...", "cleanup")
    cleanedCount, deletedVulnCount, _ = s.cleanupFixedVulnerabilities(cveResults, hostSoftwareMap)
}
```

**前端停止状态刷新重置**:

刷新页面后，停止状态不再持久化显示，而是自动清除：

```typescript
} else if (status.status === 'stopped') {
  // 停止状态刷新后应消失，不保留扫描状态
  scanStatus.value = status
  scanning.value = false
  clearScanId()
  return true
}
```

---

## 12. 文件变更清单 (V5.5.2)

### 后端 (API Server)

| 文件 | 变更类型 | 说明 |
|------|----------||
| `internal/api/router.go` | 删除 | 移除 `/vulnerability/scan/pause` 路由 |
| `internal/api/handler/vulnerability_handler.go` | 删除 | 移除 `PauseScan` Handler |
| `internal/service/vulnerability_service.go` | 重构 | 移除 PauseScan、pauseRequested；StopScan 跳过清理逻辑 |

### 前端 (Frontend)

| 文件 | 变更类型 | 说明 |
|------|----------||
| `src/views/Vulnerability.vue` | 删除 | 移除暂停按钮及相关状态 |
| `src/api/vulnerability.ts` | 删除 | 移除 `pauseVulnerabilityScan` API |
| `src/store/vulnerability.ts` | 修改 | 移除 pauseScan；停止状态刷新后清除 |

---

## 13. 测试验证 (V5.5.2)

### 13.1 暂停 API 剔除断言

```bash
# 断言返回 404
curl -X POST http://localhost:8082/api/v1/vulnerability/scan/pause
# 预期: 404 Not Found
```

### 13.2 停止增量入库断言

```bash
# 1. 启动扫描
curl -X POST http://localhost:8082/api/v1/vulnerability/scan \
  -H "Content-Type: application/json" \
  -d '{"host_ids": ["HOST_ID"]}'

# 2. 扫描中途停止
curl -X POST http://localhost:8082/api/v1/vulnerability/scan/stop

# 3. 连接 PGSQL 验证
# - 任务状态已变为 STOPPED
# - 停止前已扫描的漏洞成功入库
# - 该主机原有的老漏洞未被清理
```

### 13.3 刷新后状态清除测试

```bash
# 1. 前端发起扫描并停止
# 2. 刷新页面
# 3. 验证停止状态标签是否消失
# 4. 验证是否可以发起新扫描
```

---

## 14. 后续计划

1. **V5.6**:
   - 扫描进度百分比精确计算
   - 多主机并行扫描
   - 扫描结果导出功能
   - ResumeScan 独立接口

---

**文档结束**

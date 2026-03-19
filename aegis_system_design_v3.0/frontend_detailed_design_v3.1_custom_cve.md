# 前端详细设计文档 - V3.1 自定义CVE功能

**版本**: 3.1
**状态**: 定稿
**作者**: 安全产品团队
**日期**: 2026-03-19

---

## 1. 修订历史

| 版本 | 日期 | 作者 | 修订说明 |
|:---|:---|:---|:---|
| 3.1 | 2026-03-19 | 安全产品团队 | **新增自定义CVE功能**。新增`CustomCVEDialog`、`HostScriptStatusList`组件，扩展`Vulnerability.vue`页面，重构`FixConfirmationDialog`支持多主机状态显示。 |
| 3.0 | 2026-03-13 | 安全产品团队 | 新增漏洞管理模块前端设计。 |
| 2.2 | 2026-03-12 | Sisyphus | 任务管理与超时机制增强。 |

---

## 2. 概述

本文档描述Aegis智能主机安全系统V3.1版本新增的**自定义CVE功能**前端实现设计。

### 2.1 技术栈

- **框架**: Vue 3 + TypeScript + Vite
- **UI组件库**: Element Plus
- **状态管理**: Pinia
- **HTTP客户端**: Axios
- **路由**: Vue Router

### 2.2 新增组件

| 组件 | 路径 | 描述 |
|:---|:---|:---|
| `CustomCVEDialog.vue` | `components/vulnerability/` | 自定义CVE查询对话框 |
| `HostScriptStatusList.vue` | `components/vulnerability/` | 主机脚本状态列表组件 |

### 2.3 修改组件

| 组件 | 修改内容 |
|:---|:---|:---|
| `Vulnerability.vue` | 添加"自定义CVE"按钮，集成CustomCVEDialog |
| `FixConfirmationDialog.vue` | 重构支持多主机脚本状态显示 |
| `vulnerability.ts` (API) | 新增自定义CVE相关API接口 |
| `vulnerability.ts` (Store) | 新增自定义CVE状态管理 |

---

## 3. 组件详细设计

### 3.1 CustomCVEDialog组件

**文件位置**: `frontend/src/components/vulnerability/CustomCVEDialog.vue`

#### 3.1.1 组件功能

- 输入CVE编号并进行格式校验
- 调用后端API启动CVE查询
- 显示查询进度状态
- 查询成功后显示CVE详情预览
- 查询失败显示错误信息和重试按钮

#### 3.1.2 Props定义

```typescript
interface Props {
  visible: boolean
}

const props = defineProps<Props>()
```

#### 3.1.3 Emits定义

```typescript
interface Emits {
  (e: 'update:visible', value: boolean): void
  (e: 'success', vulnerability: Vulnerability): void
}

const emit = defineEmits<Emits>()
```

#### 3.1.4 状态管理

```typescript
// 表单数据
const formRef = ref<FormInstance>()
const form = ref({ cve_id: '' })

// 查询状态
const querying = ref(false)
const queryStatus = ref<'idle' | 'querying' | 'success' | 'failed'>('idle')
const queryProgress = ref(0)
const queryMessage = ref('')
const errorMessage = ref('')
const result = ref<Vulnerability | null>(null)
const currentQueryId = ref<string | null>(null)
```

#### 3.1.5 核心方法

| 方法 | 功能 |
|:---|:---|
| `handleQuery` | 启动CVE查询，先检查是否有进行中的查询 |
| `pollQueryStatus` | 轮询查询状态（2秒间隔） |
| `resetQuery` | 重置所有状态 |
| `handleClose` | 关闭对话框并重置状态 |

#### 3.1.6 完整模板结构

```vue
<template>
  <el-dialog
    v-model="visible"
    title="自定义CVE查询"
    width="500px"
    :close-on-click-modal="false"
    @closed="handleClose"
  >
    <!-- 输入表单 -->
    <div v-if="queryStatus === 'idle'" class="query-form">
      <el-form :model="form" :rules="rules" ref="formRef">
        <el-form-item label="CVE编号" prop="cve_id">
          <el-input
            v-model="form.cve_id"
            placeholder="请输入CVE编号，如 CVE-2021-44228"
            :disabled="querying"
          >
            <template #prepend>CVE-</template>
          </el-input>
        </el-form-item>
        
        <el-form-item>
          <el-alert type="info" :closable="false">
            <template #title>
              <el-icon><InfoFilled /></el-icon>
              系统将通过大模型查询该CVE的详细信息
            </template>
          </el-alert>
        </el-form-item>
      </el-form>
    </div>
    
    <!-- 查询中状态 -->
    <div v-else-if="queryStatus === 'querying'" class="querying-status">
      <div class="status-header">
        <el-icon class="is-loading" :size="24"><Loading /></el-icon>
        <span>正在查询 {{ form.cve_id }}...</span>
      </div>
      <el-progress :percentage="queryProgress" :stroke-width="8" />
      <p class="status-message">{{ queryMessage }}</p>
    </div>
    
    <!-- 查询成功 -->
    <div v-else-if="queryStatus === 'success'" class="query-success">
      <el-result icon="success" title="查询成功">
        <template #extra>
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="CVE编号">{{ result?.cve_id }}</el-descriptions-item>
            <el-descriptions-item label="严重程度">
              <SeverityTag :severity="result?.severity" />
            </el-descriptions-item>
            <el-descriptions-item label="CVSS评分">{{ result?.cvss_score }}</el-descriptions-item>
            <el-descriptions-item label="漏洞描述">
              <el-text line-clamp="3">{{ result?.description }}</el-text>
            </el-descriptions-item>
          </el-descriptions>
        </template>
      </el-result>
    </div>
    
    <!-- 查询失败 -->
    <div v-else-if="queryStatus === 'failed'" class="query-failed">
      <el-result icon="warning" title="查询失败" :sub-title="errorMessage">
        <template #extra>
          <el-button type="primary" @click="resetQuery">重新查询</el-button>
        </template>
      </el-result>
    </div>
    
    <!-- 底部按钮 -->
    <template #footer>
      <div v-if="queryStatus === 'idle'">
        <el-button @click="handleClose">取消</el-button>
        <el-button type="primary" :loading="querying" @click="handleQuery">
          查询CVE
        </el-button>
      </div>
      <div v-else-if="queryStatus === 'success' || queryStatus === 'failed'">
        <el-button type="primary" @click="handleClose">关闭</el-button>
      </div>
    </template>
  </el-dialog>
</template>
```

---

### 3.2 HostScriptStatusList组件

**文件位置**: `frontend/src/components/vulnerability/HostScriptStatusList.vue`

#### 3.2.1 组件功能

- 显示选中主机的脚本生成状态列表
- 支持单主机操作（查看脚本、重新生成、执行）
- 支持批量操作（批量生成、批量执行）
- 实时显示生成进度和状态统计

#### 3.2.2 Props定义

```typescript
interface Props {
  cveId: string
  scriptType: 'poc' | 'fix'  // 脚本类型
  selectedHosts: AffectedHost[]  // 选中的主机列表
}

const props = defineProps<Props>()
```

#### 3.2.3 Emits定义

```typescript
interface Emits {
  (e: 'execute', data: { taskGroupId: string; hosts: string[] }): void
}

const emit = defineEmits<Emits>()
```

#### 3.2.4 状态管理

```typescript
// 主机脚本状态列表
const hostScripts = ref<HostScript[]>([])

// 汇总统计
const summary = computed(() => ({
  total: hostScripts.value.length,
  generated: hostScripts.value.filter(h => h.generation_status === 'generated').length,
  generating: hostScripts.value.filter(h => h.generation_status === 'generating').length,
  pending: hostScripts.value.filter(h => h.generation_status === 'pending').length,
  failed: hostScripts.value.filter(h => h.generation_status === 'failed').length,
}))

// 轮询定时器
let pollTimer: number | null = null
```

#### 3.2.5 核心方法

| 方法 | 功能 |
|:---|:---|
| `fetchHostScriptsStatus` | 获取各主机脚本状态 |
| `generateSingleScript` | 生成单个主机脚本 |
| `generateAllPending` | 批量生成未生成的脚本 |
| `executeGenerated` | 批量执行已生成的脚本 |
| `viewScript` | 查看脚本内容 |
| `startPolling` | 开始轮询更新状态 |
| `stopPolling` | 停止轮询 |

#### 3.2.6 完整模板结构

```vue
<template>
  <div class="host-script-status-list">
    <!-- 标题和统计 -->
    <div class="list-header">
      <span class="title">脚本生成状态</span>
      <div class="summary">
        <el-tag type="success">已生成 {{ summary.generated }}</el-tag>
        <el-tag type="warning">生成中 {{ summary.generating }}</el-tag>
        <el-tag type="danger">失败 {{ summary.failed }}</el-tag>
        <el-tag type="info">未生成 {{ summary.pending }}</el-tag>
      </div>
    </div>
    
    <!-- 进度条 -->
    <el-progress 
      :percentage="progressPercent" 
      :status="progressStatus"
      :stroke-width="10"
    />
    
    <!-- 主机列表 -->
    <el-table :data="hostScripts" style="width: 100%">
      <!-- 主机信息列 -->
      <el-table-column label="主机" min-width="200">
        <template #default="{ row }">
          <div class="host-info">
            <span class="ip">{{ row.host_ip }}</span>
            <span class="hostname">({{ row.hostname }})</span>
            <el-tag size="small" type="info">{{ row.os_type }}</el-tag>
          </div>
        </template>
      </el-table-column>
      
      <!-- 状态列 -->
      <el-table-column label="状态" width="120">
        <template #default="{ row }">
          <div class="status-cell">
            <template v-if="row.generation_status === 'generating'">
              <el-icon class="is-loading"><Loading /></el-icon>
              <span>生成中</span>
            </template>
            <template v-else-if="row.generation_status === 'generated'">
              <el-icon color="#67c23a"><CircleCheck /></el-icon>
              <span>已生成</span>
            </template>
            <template v-else-if="row.generation_status === 'failed'">
              <el-icon color="#f56c6c"><CircleClose /></el-icon>
              <span>失败</span>
            </template>
            <template v-else>
              <el-icon color="#909399"><Clock /></el-icon>
              <span>未生成</span>
            </template>
          </div>
        </template>
      </el-table-column>
      
      <!-- 操作列 -->
      <el-table-column label="操作" width="200">
        <template #default="{ row }">
          <template v-if="row.generation_status === 'generating'">
            <el-progress 
              :percentage="row.generation_progress || 50" 
              :stroke-width="4"
              style="width: 100px"
            />
          </template>
          <template v-else-if="row.generation_status === 'generated'">
            <el-button link type="primary" size="small" @click="viewScript(row)">
              查看脚本
            </el-button>
            <el-button link type="warning" size="small" @click="regenerate(row)">
              重新生成
            </el-button>
          </template>
          <template v-else-if="row.generation_status === 'failed'">
            <el-button link type="info" size="small" @click="viewError(row)">
              查看错误
            </el-button>
            <el-button link type="primary" size="small" @click="regenerate(row)">
              重试
            </el-button>
          </template>
          <template v-else>
            <el-button link type="primary" size="small" @click="generateSingle(row)">
              生成脚本
            </el-button>
          </template>
        </template>
      </el-table-column>
    </el-table>
    
    <!-- 底部操作按钮 -->
    <div class="list-footer">
      <el-button @click="generateAllPending">
        批量生成未生成的脚本 ({{ summary.pending }})
      </el-button>
      <el-button 
        type="primary" 
        :disabled="summary.generated === 0"
        @click="executeGenerated"
      >
        批量执行已生成的脚本 ({{ summary.generated }})
      </el-button>
    </div>
  </div>
</template>
```

---

### 3.3 Vulnerability.vue修改

#### 3.3.1 新增模板内容

```vue
<template>
  <!-- 在操作区添加自定义CVE按钮 -->
  <div class="scan-header">
    <!-- 现有的主机选择器和扫描按钮 -->
    <el-select ... />
    <el-button type="primary" ... >一键扫描</el-button>
    
    <!-- 新增：自定义CVE按钮 -->
    <el-divider direction="vertical" />
    <el-button
      :icon="Plus"
      @click="openCustomCVEDialog"
      :disabled="scanning || hasQueryingCustomCVE"
    >
      自定义CVE
    </el-button>
    
    <!-- 现有的任务中心和筛选 -->
    ...
  </div>
  
  <!-- 漏洞列表 - 新增来源标签 -->
  <el-table-column prop="source" label="来源" width="80">
    <template #default="{ row }">
      <el-tag :type="row.source === 'custom_query' ? 'warning' : 'info'" size="small">
        {{ row.source === 'custom_query' ? '自定义' : '扫描' }}
      </el-tag>
    </template>
  </el-table-column>
  
  <!-- 新增：自定义CVE对话框 -->
  <CustomCVEDialog
    v-model:visible="customCVEDialogVisible"
    @success="handleCustomCVESuccess"
  />
</template>
```

#### 3.3.2 新增脚本内容

```typescript
import CustomCVEDialog from '@/components/vulnerability/CustomCVEDialog.vue'
import { Plus } from '@element-plus/icons-vue'

// 新增响应式变量
const customCVEDialogVisible = ref(false)
const hasQueryingCustomCVE = ref(false)

// 新增方法
function openCustomCVEDialog() {
  customCVEDialogVisible.value = true
}

function handleCustomCVESuccess(vulnerability: api.Vulnerability) {
  // 刷新漏洞列表
  vulnStore.fetchVulnerabilities({})
  ElMessage.success(`CVE ${vulnerability.cve_id} 已添加到漏洞列表`)
}

// 检查是否有进行中的查询
async function checkCurrentQuery() {
  try {
    const result = await api.getCurrentCustomQuery()
    hasQueryingCustomCVE.value = result.has_querying
  } catch (e) {
    // 忽略错误
  }
}

// 组件挂载时检查
onMounted(async () => {
  await checkCurrentQuery()
  // ... 其他初始化逻辑
})
```

---

### 3.4 FixConfirmationDialog.vue重构

#### 3.4.1 主要修改点

1. **主机选择逻辑修改**：
   - 自定义CVE时，从`hosts` store获取全部主机
   - 扫描CVE时，从`affectedHosts` prop获取受影响主机

2. **新增HostScriptStatusList组件集成**：
   - 用户选择主机后，显示状态列表组件
   - 状态列表替代原有的单一脚本预览区域

3. **多主机执行支持**：
   - 执行按钮改为批量执行
   - 执行前显示确认对话框，列出将执行和跳过的主机

#### 3.4.2 修改后的模板结构

```vue
<template>
  <el-dialog ...>
    <!-- CVE信息 -->
    <div class="cve-info">...</div>
    
    <!-- 主机选择 -->
    <div class="host-selection">
      <el-select
        v-model="selectedHosts"
        multiple
        filterable
        placeholder="请选择主机"
        @change="onHostSelectionChange"
      >
        <el-option
          v-for="host in availableHosts"
          :key="host.id"
          :label="`${host.ip_address} (${host.hostname})`"
          :value="host.id"
        />
      </el-select>
    </div>
    
    <!-- 主机任务状态列表 (新增) -->
    <HostScriptStatusList
      v-if="selectedHosts.length > 0"
      :cve-id="cve.cve_id"
      :script-type="mode"
      :selected-hosts="selectedHostDetails"
      @execute="handleExecute"
    />
    
    <!-- 空状态 -->
    <div v-else class="empty-state">
      <el-empty description="请先选择目标主机" />
    </div>
  </el-dialog>
</template>
```

#### 3.4.3 新增计算属性

```typescript
// 可选择的主机列表
const availableHosts = computed(() => {
  // 自定义CVE：显示全部主机
  if (props.cve.source === 'custom_query') {
    return hostStore.hosts
  }
  // 扫描CVE：只显示受影响主机
  return props.affectedHosts
})

// 选中主机的详情
const selectedHostDetails = computed(() => {
  return availableHosts.value.filter(h => selectedHosts.value.includes(h.id))
})
```

---

## 4. API接口层

### 4.1 新增接口定义

**文件位置**: `frontend/src/api/vulnerability.ts`

```typescript
// ==================== 自定义CVE查询接口 ====================

// 启动自定义CVE查询
export function startCustomCveQuery(cveId: string): Promise<{
  query_id: string
  cve_id: string
  status: string
}> {
  return request.post('/vulnerability/custom-query', { cve_id: cveId })
}

// 获取查询状态
export function getCustomQueryStatus(queryId: string): Promise<{
  query_id: string
  cve_id: string
  status: 'querying' | 'success' | 'failed'
  progress: number
  message: string
  vulnerability?: Vulnerability
  error?: string
}> {
  return request.get(`/vulnerability/custom-query/${queryId}/status`)
}

// 获取当前进行中的查询
export function getCurrentCustomQuery(): Promise<{
  has_querying: boolean
  query?: {
    query_id: string
    cve_id: string
    status: string
    started_at: string
  }
}> {
  return request.get('/vulnerability/custom-query/current')
}

// ==================== 多主机脚本生成接口 ====================

// 主机脚本状态
export interface HostScript {
  host_id: string
  host_ip: string
  hostname: string
  os_type: string
  script_id?: string
  generation_status: 'pending' | 'generating' | 'generated' | 'failed'
  generation_progress?: number
  generation_message?: string
  script_content?: string
}

// 批量生成脚本
export function generateHostScripts(
  cveId: string,
  hostIds: string[],
  scriptType: 'poc' | 'fix'
): Promise<{
  scripts: Array<{
    host_id: string
    script_id: string
    status: string
  }>
}> {
  return request.post(`/vulnerability/${cveId}/scripts/generate`, {
    host_ids: hostIds,
    script_type: scriptType
  })
}

// 获取各主机脚本状态
export function getHostScriptsStatus(
  cveId: string,
  scriptType: 'poc' | 'fix'
): Promise<{
  cve_id: string
  script_type: string
  hosts: HostScript[]
  summary: {
    total: number
    generated: number
    generating: number
    pending: number
    failed: number
  }
}> {
  return request.get(`/vulnerability/${cveId}/host-scripts`, {
    params: { script_type: scriptType }
  })
}

// 执行已生成的脚本
export function executeHostScripts(
  cveId: string,
  scriptType: 'poc' | 'fix',
  hostIds: string[]
): Promise<{
  task_group_id: string
  executed_hosts: string[]
  skipped_hosts: string[]
  skip_reasons: Record<string, string>
}> {
  return request.post(`/vulnerability/${cveId}/scripts/execute`, {
    script_type: scriptType,
    host_ids: hostIds
  })
}
```

---

## 5. 状态管理层

### 5.1 vulnerability store扩展

**文件位置**: `frontend/src/store/vulnerability.ts`

```typescript
// 新增状态
const customCVEDialogVisible = ref(false)
const currentCustomQuery = ref<CustomQuery | null>(null)

// 新增actions
async function startCustomQuery(cveId: string) {
  // ...
}

async function checkCurrentCustomQuery() {
  try {
    const result = await api.getCurrentCustomQuery()
    currentCustomQuery.value = result.has_querying ? result.query : null
  } catch (e) {
    console.error('Failed to check current custom query:', e)
  }
}

return {
  // ... 现有状态和actions
  customCVEDialogVisible,
  currentCustomQuery,
  startCustomQuery,
  checkCurrentCustomQuery,
}
```

---

## 6. 样式设计

### 6.1 状态颜色定义

```css
/* 状态颜色变量 */
--status-generating: #e6a23c;  /* 橙色 - 生成中 */
--status-generated: #67c23a;   /* 绿色 - 已生成 */
--status-failed: #f56c6c;      /* 红色 - 失败 */
--status-pending: #909399;     /* 灰色 - 未生成 */
```

### 6.2 组件样式

```css
/* CustomCVEDialog 样式 */
.query-form {
  padding: 10px 0;
}

.querying-status {
  text-align: center;
  padding: 20px;
}

.status-header {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-bottom: 20px;
}

/* HostScriptStatusList 样式 */
.host-script-status-list {
  margin-top: 20px;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 15px;
}

.summary {
  display: flex;
  gap: 8px;
}

.host-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.host-info .ip {
  font-weight: 500;
}

.host-info .hostname {
  color: #909399;
  font-size: 12px;
}

.status-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.list-footer {
  display: flex;
  justify-content: space-between;
  margin-top: 20px;
  padding-top: 15px;
  border-top: 1px solid #ebeef5;
}
```

---

## 7. 文件结构

```
frontend/src/
├── api/
│   └── vulnerability.ts                 (扩展)
├── components/
│   ├── common/
│   │   └── FixConfirmationDialog.vue   (重构)
│   └── vulnerability/
│       ├── CustomCVEDialog.vue         (新增)
│       └── HostScriptStatusList.vue    (新增)
├── store/
│   └── vulnerability.ts                 (扩展)
└── views/
    └── Vulnerability.vue               (扩展)
```

---

## 8. 测试用例

### 8.1 组件测试

**测试文件**: `frontend/src/components/vulnerability/CustomCVEDialog.test.ts`

```typescript
describe('CustomCVEDialog', () => {
  it('should validate CVE ID format', async () => {
    // 测试CVE格式校验
  })
  
  it('should show querying status when query starts', async () => {
    // 测试查询中状态显示
  })
  
  it('should show success result when query succeeds', async () => {
    // 测试成功状态显示
  })
  
  it('should show error when query fails', async () => {
    // 测试失败状态显示
  })
  
  it('should disable query button when another query is in progress', async () => {
    // 测试查询互斥
  })
})
```

### 8.2 HostScriptStatusList测试

**测试文件**: `frontend/src/components/vulnerability/HostScriptStatusList.test.ts`

```typescript
describe('HostScriptStatusList', () => {
  it('should display all selected hosts', async () => {
    // 测试主机列表显示
  })
  
  it('should show correct status for each host', async () => {
    // 测试状态显示
  })
  
  it('should call generate API when clicking generate button', async () => {
    // 测试生成按钮
  })
  
  it('should call execute API when clicking execute button', async () => {
    // 测试执行按钮
  })
  
  it('should show summary statistics correctly', async () => {
    // 测试统计显示
  })
})
```

---

## 9. 验收检查清单

### 9.1 功能验收

- [ ] CVE输入框支持格式校验
- [ ] 查询按钮在有查询进行中时禁用
- [ ] 查询成功后CVE入库并刷新列表
- [ ] 查询失败显示错误信息和重试按钮
- [ ] 主机选择支持多选
- [ ] 自定义CVE显示全部主机，扫描CVE显示受影响主机
- [ ] 主机状态列表正确显示各主机状态
- [ ] 批量生成按钮功能正常
- [ ] 批量执行按钮只执行已生成的脚本

### 9.2 UI验收

- [ ] 对话框样式与现有系统一致
- [ ] 状态图标和颜色正确
- [ ] 按钮禁用状态正确
- [ ] 表格布局正确，无溢出
- [ ] 移动端响应式布局正常

### 9.3 性能验收

- [ ] 状态轮询间隔2秒
- [ ] 组件渲染时间 < 100ms
- [ ] API请求超时设置正确

---

**文档结束**
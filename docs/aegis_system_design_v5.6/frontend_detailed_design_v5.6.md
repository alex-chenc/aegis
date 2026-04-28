# Aegis智能主机安全系统 V5.6 前端详细设计文档

**版本**: 5.6
**日期**: 2026-04-14
**状态**: 设计中

---

## 1. 概述

### 1.1 设计目标

V5.6版本前端主要新增以下功能：

| 功能 | 页面路径 | 说明 |
|------|---------|------|
| Sigma规则上传 | `/detection/rules` | 支持上传Sigma规则YAML/ZIP文件 |
| AI规则配置 | `/detection/rules` → AI配置Tab | 配置AI规则更新功能 |
| AI分析多轮对话 | `/detection/ai-analysis` | 多选告警+时间范围+多轮对话+溯源图 |
| 工具调用展示 | 嵌入在AI分析面板 | 展示AI调用的工具及结果 |
| 登录认证 | `/login`、`/force-password-change` | 首次免密进入后强制设置账号密码，后续账号密码登录 |

---

## 2.0 登录认证页面

### 2.0.1 页面与路由

```text
/login
  - 未初始化：展示“首次进入控制台”主操作
  - 已初始化：展示账号、密码登录表单

/force-password-change
  - 仅 force_password_change=true 的会话可访问
  - 设置管理员账号、新密码、确认密码
```

路由守卫规则：

| 状态 | 访问登录页 | 访问改密页 | 访问业务页 |
|------|------------|------------|------------|
| 无 token | 允许 | 重定向 `/login` | 重定向 `/login` |
| 临时 token | 重定向 `/force-password-change` | 允许 | 重定向 `/force-password-change` |
| 正常 token | 重定向 `/hosts` | 重定向 `/hosts` | 允许 |

### 2.0.2 UI 设计

- 登录页采用安全控制台风格：左侧为产品识别和系统状态，右侧为登录表单。
- 表单必须使用可见 label、密码显隐按钮、提交 loading、字段级校验错误。
- 首次进入按钮仅在 `/auth/status` 返回 `initialized=false` 时展示。
- 强制改密页不显示主导航，避免用户绕过改密流程。
- 移动端表单宽度收敛到视口内，按钮高度不低于 44px。

### 2.0.3 前端状态

认证状态保存到 `localStorage`：

```ts
{
  token: string,
  username: string,
  forcePasswordChange: boolean
}
```

Axios 请求拦截器读取 token 并注入 `Authorization`；响应遇到 `401` 时清理本地认证状态并跳转 `/login`，遇到 `403` 且本地处于强制改密状态时跳转 `/force-password-change`。

会话空闲超时规则：

- 已登录用户 5 分钟没有鼠标、键盘、触摸、滚动或页面可见性恢复等操作时，前端必须自动清理本地认证状态并跳转 `/login`。
- 登录页和强制改密页不启动空闲计时。
- 每次用户操作重置计时器；组件卸载时必须移除监听器和计时器，避免重复触发。
- 自动退出时显示明确提示，避免用户误判为页面异常。

### 2.0.4 验收测试

- 未初始化状态渲染“首次进入控制台”。
- 已初始化状态渲染账号密码表单。
- 首次进入成功后跳转 `/force-password-change`。
- 改密成功后保存新会话状态并跳转 `/hosts`。
- API 层登录、首次进入、改密调用路径和 payload 正确。
- 已登录业务页 5 分钟无操作后自动清理认证状态并跳转 `/login`。

## 2. 页面结构

### 2.1 规则管理页面

```
/detection/rules
├── RuleManagement.vue          # 主页面
├── components/
│   ├── RuleUpload.vue          # 文件上传组件
│   ├── RuleList.vue            # 规则列表
│   ├── RuleTable.vue           # 规则表格
│   ├── RuleDetail.vue          # 规则详情弹窗
│   ├── RuleFilter.vue          # 筛选组件
│   ├── BatchActions.vue        # 批量操作
│   └── tabs/
│       ├── RuleListTab.vue     # 规则列表Tab
│       └── AIConfigTab.vue     # AI配置Tab (V5.6新增)
```

### 2.2 告警中心页面

```
/detection/alerts
├── AlertCenter.vue             # 主页面
├── components/
│   ├── AlertTable.vue          # 告警表格（支持多选）
│   ├── AlertFilter.vue         # 筛选组件
│   ├── AIAnalysisPanel.vue     # AI分析面板 (V5.6增强: SSE流式+溯源图)
│   ├── ChatMessage.vue         # 聊天消息组件
│   ├── ToolCallBlock.vue       # 工具调用块
│   ├── ConclusionForm.vue      # 分析结论表单
│   ├── TimeRangePicker.vue     # 时间范围选择器
│   ├── AttackGraph.vue         # 攻击溯源图 (V5.6新增)
│   └── AttackGraphCanvas.vue   # D3.js画布 (V5.6新增)
```

---

## 3. 组件设计

### 3.1 RuleUpload.vue（规则上传组件）

```vue
<template>
  <div class="rule-upload">
    <el-upload
      ref="uploadRef"
      :action="uploadUrl"
      :headers="headers"
      :accept="acceptTypes"
      :multiple="true"
      :before-upload="handleBeforeUpload"
      :on-success="handleSuccess"
      :on-error="handleError"
      :on-progress="handleProgress"
      :file-list="fileList"
      drag
    >
      <div class="upload-content">
        <el-icon class="upload-icon"><UploadFilled /></el-icon>
        <div class="upload-text">
          <span>将Sigma规则文件拖拽到此处</span>
          <span class="upload-hint">或点击上传</span>
        </div>
        <div class="upload-formats">
          支持格式: .yaml, .yml, .zip (含多个规则)
        </div>
      </div>
    </el-upload>

    <!-- 上传结果 -->
    <div v-if="uploadResult" class="upload-result">
      <el-alert
        :type="uploadResult.success ? 'success' : 'error'"
        :title="uploadResult.message"
        :closable="false"
      >
        <template #default>
          <div v-if="uploadResult.success">
            成功解析 {{ uploadResult.parsed_count }} 条规则
            <div v-if="uploadResult.failed_count > 0">
              失败 {{ uploadResult.failed_count }} 条
            </div>
          </div>
        </template>
      </el-alert>
    </div>
  </div>
</template>

<script setup lang="ts">
interface UploadResult {
  success: boolean
  parsed_count: number
  failed_count: number
  message: string
  rules?: Array<{
    rule_id: string
    title: string
    status: string
  }>
}

const uploadUrl = '/api/v1/detection/rules/upload'
const acceptTypes = '.yaml,.yml,.zip'
const fileList = ref([])
const uploadResult = ref<UploadResult | null>(null)

const handleBeforeUpload = (file: any) => {
  // 检查文件大小
  const maxSize = 10 * 1024 * 1024 // 10MB
  if (file.size > maxSize) {
    ElMessage.error('文件大小不能超过10MB')
    return false
  }
  return true
}

const handleSuccess = (response: any) => {
  uploadResult.value = response
  ElMessage.success(`成功解析 ${response.parsed_count} 条规则`)
}

const handleError = (error: any) => {
  ElMessage.error('上传失败: ' + error.message)
}
</script>
```

> **实际实现说明 (V5.6更新 2026-04-17)**：
> - 规则上传功能集成在 `Rules.vue` 主页面中，未单独拆分组件
> - 上传组件配置：`:limit="10" :multiple="true"`，支持最多10个文件同时上传
> - 批量操作合并为下拉菜单，包含启用选中、禁用选中、删除选中
> - 严重程度 `informational` 映射为 `low`（低危）

### 3.2 AIConfigTab.vue（AI规则配置Tab）

```vue
<template>
  <div class="ai-config-tab">
    <el-form :model="config" label-width="160px">

      <!-- 功能开关 -->
      <el-form-item label="启用AI规则更新">
        <el-switch v-model="config.enabled" />
      </el-form-item>

      <!-- 模式选择 -->
      <el-form-item label="更新模式">
        <el-radio-group v-model="config.mode" :disabled="!config.enabled">
          <el-radio value="off">关闭</el-radio>
          <el-radio value="suggest">仅建议（需人工审核）</el-radio>
          <el-radio value="auto">自动（审核后自动激活）</el-radio>
        </el-radio-group>
      </el-form-item>

      <!-- 触发条件 (V5.6简化: 可配置阈值) -->
      <el-form-item label="触发条件">
        <div class="trigger-config">
          <span>同一MITRE ID在</span>
          <el-input-number
            v-model="config.thresholds.high_frequency_hours"
            :min="1"
            :max="24"
            :disabled="!config.enabled"
          />
          <span>小时内触发</span>
          <el-input-number
            v-model="config.thresholds.high_frequency_count"
            :min="10"
            :max="100"
            :disabled="!config.enabled"
          />
          <span>次，即进行AI更新规则</span>
        </div>
      </el-form-item>

      <!-- 生成策略 -->
      <el-form-item label="规则生成策略">
        <div class="strategy-slider">
          <span>保守</span>
          <el-slider
            v-model="config.conservatism"
            :min="0"
            :max="1"
            :step="0.1"
            :disabled="!config.enabled"
          />
          <span>激进</span>
        </div>
        <div class="strategy-hint">
          保守模式：更少误报，但可能漏检<br>
          激进模式：更多检测，但可能有误报
        </div>
      </el-form-item>

      <!-- 审核配置 (V5.6修改) -->
      <el-form-item label="审核配置">
        <el-checkbox v-model="config.require_approval" :disabled="!config.enabled">
          规则生成后发送审核通知
        </el-checkbox>
        <el-checkbox
          v-if="config.mode === 'suggest'"
          v-model="config.auto_activate_after_approval"
          :disabled="!config.enabled || !config.require_approval"
        >
          无人审核后24小时自动从待审核调整为实验性
        </el-checkbox>
      </el-form-item>

      <!-- 测试按钮 -->
      <el-form-item>
        <el-button type="primary" @click="saveConfig" :disabled="!config.enabled">
          保存配置
        </el-button>
        <el-button @click="testRuleGeneration">
          测试规则生成
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
interface AIConfig {
  enabled: boolean
  mode: 'off' | 'suggest' | 'auto'
  triggers: string[]
  thresholds: {
    high_frequency_count: number
    high_frequency_hours: number
  }
  conservatism: number
  require_approval: boolean
  auto_activate_after_approval: boolean
}

const config = reactive<AIConfig>({
  enabled: false,
  mode: 'suggest',
  triggers: ['high_frequency', 'critical'],
  thresholds: {
    high_frequency_count: 5,
    high_frequency_hours: 24
  },
  conservatism: 0.5,
  require_approval: true,
  auto_activate_after_approval: false
})

const saveConfig = async () => {
  await api.put('/api/v1/detection/rules/ai-config', config)
  ElMessage.success('配置已保存')
}

const testRuleGeneration = async () => {
  // TODO: 实现测试功能
}
</script>
```

### 3.3 AIAnalysisPanel.vue（AI分析面板）

```vue
<template>
  <div class="ai-analysis-panel">
    <!-- 头部 -->
    <div class="panel-header">
      <span class="title">AI安全分析助手</span>
      <el-button link @click="clearSession">
        <el-icon><Close /></el-icon>
        清除
      </el-button>
    </div>

    <!-- 选择信息 -->
    <div class="selection-info">
      <span>已选择 {{ selectedAlerts.length }} 个告警</span>
      <el-select v-model="timeRange" placeholder="时间范围" style="width: 150px">
        <el-option label="最近1小时" value="1h" />
        <el-option label="最近6小时" value="6h" />
        <el-option label="最近24小时" value="24h" />
        <el-option label="自定义" value="custom" />
      </el-select>
      <el-input-number v-model="maxIterations" :min="1" :max="100" size="small" style="width: 120px" />
      <span>最大轮数</span>
      <el-button type="primary" size="small" @click="startAnalysis">
        开始分析
      </el-button>
    </div>

    <!-- 聊天区域 -->
    <div class="chat-area" ref="chatAreaRef">
      <div v-for="msg in messages" :key="msg.id" class="message-item">
        <!-- 用户消息 -->
        <div v-if="msg.role === 'user'" class="message user">
          <div class="message-content">{{ msg.content }}</div>
        </div>

        <!-- AI消息 -->
        <div v-else-if="msg.role === 'assistant'" class="message assistant">
          <div class="message-avatar">
            <el-icon><ChatDotRound /></el-icon>
          </div>
          <div class="message-body">
            <div class="message-content">{{ msg.content }}</div>

            <!-- 工具调用 -->
            <div v-if="msg.toolCalls?.length" class="tool-calls">
              <div class="tool-call-header">
                <el-icon><Tools /></el-icon>
                <span>调用工具</span>
              </div>
              <div
                v-for="tool in msg.toolCalls"
                :key="tool.callId"
                class="tool-call-item"
              >
                <span class="tool-name">{{ tool.tool }}</span>
                <span class="tool-status">
                  <el-icon v-if="tool.result"><Check /></el-icon>
                  <el-icon v-else-if="tool.error"><Close /></el-icon>
                  <el-icon v-else class="is-loading"><Loading /></el-icon>
                </span>
              </div>
            </div>

            <!-- 工具结果 -->
            <div v-if="msg.toolResults?.length" class="tool-results">
              <div
                v-for="result in msg.toolResults"
                :key="result.callId"
                class="tool-result"
              >
                <div class="result-header">
                  <span class="tool-name">{{ result.tool }}</span>
                  <span class="execution-time">{{ result.executionTime }}ms</span>
                </div>
                <pre class="result-content">{{ formatJSON(result.data) }}</pre>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 输入区域 -->
    <div class="input-area">
      <el-input
        v-model="inputMessage"
        type="textarea"
        :rows="2"
        placeholder="输入问题或指令..."
        @keydown.enter.ctrl="sendMessage"
      />
      <el-button type="primary" @click="sendMessage" :disabled="!inputMessage.trim()">
        发送
      </el-button>
    </div>

    <!-- 结论区域 -->
    <div v-if="showConclusion" class="conclusion-area">
      <div class="conclusion-header">分析结论</div>
      <el-checkbox
        v-for="alert in selectedAlerts"
        :key="alert.id"
        v-model="alert.selected"
        class="conclusion-item"
      >
        <span :class="['severity', alert.severity.toLowerCase()]">
          {{ alert.severity }}
        </span>
        {{ alert.alertId }} - {{ alert.hostname }}
      </el-checkbox>
      <div class="conclusion-actions">
        <el-button type="primary" @click="applyConclusion">
          应用结论
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, nextTick } from 'vue'

interface Alert {
  id: string
  alertId: string
  hostname: string
  mitreId: string
  severity: string
  selected?: boolean
}

interface Message {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  thinking?: string      // AI思考过程
  action?: string        // 工具名称
  actionInput?: any      // 工具参数
  observation?: string   // 工具执行结果
  toolCalls?: ToolCall[]
  toolResults?: ToolResult[]
}

interface ToolCall {
  callId: string
  tool: string
  arguments: any
  result?: any
  error?: string
}

interface ToolResult {
  callId: string
  tool: string
  data: any
  executionTime: number
}

const selectedAlerts = ref<Alert[]>([])
const timeRange = ref('1h')
const messages = ref<Message[]>([])
const inputMessage = ref('')
const chatAreaRef = ref<HTMLElement>()
const showConclusion = ref(false)
const sessionId = ref('')

// 开始分析
const startAnalysis = async () => {
  const response = await api.post('/api/v1/detection/alerts/ai-analysis/session', {
    alert_ids: selectedAlerts.value.map(a => a.id),
    time_range: {
      start: calculateTimeRange(timeRange.value).start,
      end: calculateTimeRange(timeRange.value).end
    }
  })

  sessionId.value = response.session_id

  // 添加欢迎消息
  messages.value.push({
    id: 'welcome',
    role: 'assistant',
    content: `已选择 ${selectedAlerts.value.length} 个告警进行分析。\n` +
      `时间范围：最近${timeRange.value}\n\n` +
      `请告诉我您想了解什么？`
  })
}

// 发送消息
const sendMessage = async () => {
  if (!inputMessage.value.trim() || !sessionId.value) return

  const userMsg = inputMessage.value
  inputMessage.value = ''

  // 添加用户消息
  messages.value.push({
    id: `user-${Date.now()}`,
    role: 'user',
    content: userMsg
  })

  // 滚动到底部
  await nextTick()
  scrollToBottom()

  // 发送AI请求
  const response = await api.post(
    `/api/v1/detection/alerts/ai-analysis/${sessionId.value}/message`,
    { content: userMsg }
  )

  // 添加AI响应
  const aiMsg: Message = {
    id: response.message_id,
    role: 'assistant',
    content: response.content,
    toolCalls: response.tool_calls?.map((tc: any) => ({
      callId: tc.call_id,
      tool: tc.tool,
      arguments: JSON.parse(tc.arguments)
    }))
  }
  messages.value.push(aiMsg)

  // 如果有工具调用，异步获取结果
  if (aiMsg.toolCalls?.length) {
    for (const tool of aiMsg.toolCalls) {
      try {
        const result = await api.post(
          `/api/v1/detection/alerts/ai-analysis/${sessionId.value}/tool-result`,
          { call_id: tool.callId }
        )

        // 更新工具结果
        if (!aiMsg.toolResults) aiMsg.toolResults = []
        aiMsg.toolResults.push({
          callId: tool.callId,
          tool: tool.tool,
          data: result.data,
          executionTime: result.execution_time_ms
        })

        // 更新UI
        tool.result = result.data
      } catch (error) {
        tool.error = error.message
      }
    }
  }

  // 滚动到底部
  await nextTick()
  scrollToBottom()

  // 检查是否显示结论按钮
  if (response.show_conclusion) {
    showConclusion.value = true
  }
}

// 应用结论
const applyConclusion = async () => {
  const conclusions = selectedAlerts.value
    .filter(a => a.selected)
    .map(a => ({
      alert_id: a.id,
      action: 'confirm_threat'
    }))

  await api.post(
    `/api/v1/detection/alerts/ai-analysis/${sessionId.value}/conclusion`,
    { conclusions }
  )

  ElMessage.success('结论已应用')
  showConclusion.value = false
}

const clearSession = () => {
  sessionId.value = ''
  messages.value = []
  showConclusion.value = false
}

const scrollToBottom = () => {
  if (chatAreaRef.value) {
    chatAreaRef.value.scrollTop = chatAreaRef.value.scrollHeight
  }
}
</script>
```

### 3.4 ToolCallBlock.vue（工具调用展示组件）

```vue
<template>
  <div class="tool-call-block" :class="{ expanded }">
    <div class="tool-header" @click="toggle">
      <el-icon class="tool-icon"><Tools /></el-icon>
      <span class="tool-name">{{ toolName }}</span>
      <span class="tool-status">
        <el-tag v-if="status === 'executing'" type="warning" size="small">
          执行中
        </el-tag>
        <el-tag v-else-if="status === 'success'" type="success" size="small">
          成功
        </el-tag>
        <el-tag v-else-if="status === 'error'" type="danger" size="small">
          失败
        </el-tag>
      </span>
      <el-icon class="expand-icon">
        <ArrowDown v-if="expanded" />
        <ArrowRight v-else />
      </el-icon>
    </div>

    <div v-if="expanded" class="tool-body">
      <!-- 参数 -->
      <div class="section">
        <div class="section-title">参数</div>
        <pre class="code-block">{{ formatJSON(arguments) }}</pre>
      </div>

      <!-- 结果 -->
      <div v-if="result" class="section">
        <div class="section-title">结果</div>
        <pre class="code-block result">{{ formatJSON(result) }}</pre>
      </div>

      <!-- 错误 -->
      <div v-if="error" class="section">
        <div class="section-title">错误</div>
        <pre class="code-block error">{{ error }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

interface Props {
  toolName: string
  arguments: any
  result?: any
  error?: string
  status?: 'executing' | 'success' | 'error'
}

const props = withDefaults(defineProps<Props>(), {
  status: 'executing'
})

const expanded = ref(false)

const toggle = () => {
  expanded.value = !expanded.value
}

const formatJSON = (data: any) => {
  try {
    return JSON.stringify(data, null, 2)
  } catch {
    return String(data)
  }
}
</script>

<style scoped>
.tool-call-block {
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
  margin: 8px 0;
  background: var(--el-bg-color);
}

.tool-header {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  cursor: pointer;
  gap: 8px;
}

.tool-body {
  border-top: 1px solid var(--el-border-color);
  padding: 12px;
}

.section {
  margin-bottom: 12px;
}

.section:last-child {
  margin-bottom: 0;
}

.section-title {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}

.code-block {
  background: var(--el-fill-color-light);
  padding: 8px;
  border-radius: 4px;
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
}
</style>
```

---

## 4. API调用设计

### 4.1 规则管理API

```typescript
// api/detection.ts

// 上传Sigma规则
export interface UploadSigmaRulesResponse {
  success: boolean
  parsed_count: number
  failed_count: number
  skipped_count: number
  rules: Array<{
    rule_id: string
    title: string
    status: string
    mitre_id: string
    severity: string
  }>
  failed_files?: string[]  // 导入失败的文件列表
}

export const uploadSigmaRules = (file: File) => {
  const formData = new FormData()
  formData.append('file', file)
  return api.post('/api/v1/detection/rules/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}

// 批量启用规则
export const batchEnableRules = async (ruleIds: string[]) => {
  const promises = ruleIds.map(ruleId =>
    api.put(`/api/v1/detection/rules/${ruleId}/status`, { status: 'active' })
  )
  return Promise.all(promises)
}

// 批量禁用规则
export const batchDisableRules = async (ruleIds: string[]) => {
  const promises = ruleIds.map(ruleId =>
    api.put(`/api/v1/detection/rules/${ruleId}/status`, { status: 'disabled' })
  )
  return Promise.all(promises)
}

// 删除规则
export const deleteRules = (ruleIds: string[]) => {
  return api.delete('/api/v1/detection/rules', { data: { rule_ids: ruleIds } })
}

// 获取AI配置
export const getAIRuleConfig = () => {
  return api.get('/api/v1/detection/rules/ai-config')
}

// 更新AI配置
export const updateAIRuleConfig = (config: AIConfig) => {
  return api.put('/api/v1/detection/rules/ai-config', config)
}
```

### 4.2 AI分析API

```typescript
// api/aiAnalysis.ts

// 创建分析会话
export const createAnalysisSession = (params: {
  alert_ids: string[]
  time_range?: { start: string; end: string }
  host_filter?: string[]
  max_iterations?: number
}) => {
  return api.post('/api/v1/detection/alerts/ai-analysis/session', params)
}

// 获取会话列表（支持分页）
export const getSessionList = (page: number = 1, pageSize: number = 10, status?: string) => {
  return api.get(`/api/v1/detection/alerts/ai-analysis/sessions?page=${page}&page_size=${pageSize}${status ? `&status=${status}` : ''}`)
}

// 获取会话历史消息
export const getSessionHistory = (sessionId: string) => {
  return api.get(`/api/v1/detection/alerts/ai-analysis/${sessionId}/history`)
}

// 删除会话
export const deleteSession = (sessionId: string) => {
  return api.delete(`/api/v1/detection/alerts/ai-analysis/${sessionId}`)
}

// SSE流式发送消息（V5.6新增）
export const createAISessionStream = (
  sessionId: string,
  message: string,
  onEvent: (event: SSEEvent) => void
) => {
  const encodedMessage = encodeURIComponent(message)
  const eventSource = new EventSource(
    `/api/v1/detection/alerts/ai-analysis/${sessionId}/stream?message=${encodedMessage}`
  )

  eventSource.onmessage = (event) => {
    const data = JSON.parse(event.data) as SSEEvent
    onEvent(data)
    if (data.type === 'done' || data.type === 'error') {
      eventSource.close()
    }
  }

  return eventSource
}

// SSE事件类型
export type SSEEventType = 'thinking' | 'tool_call' | 'tool_result' | 'tool_error' | 'content' | 'done' | 'error'

export interface SSEEvent {
  type: SSEEventType
  content?: string
  tool?: string
  call_id?: string
  args?: Record<string, any>
  result?: any
  time_ms?: number
  error?: string
}

// 发送消息（非流式）
export const sendAnalysisMessage = (sessionId: string, content: string) => {
  return api.post(
    `/api/v1/detection/alerts/ai-analysis/${sessionId}/message`,
    { content }
  )
}

// 提交工具结果
export const submitToolResult = (
  sessionId: string,
  callId: string,
  result: any
) => {
  return api.post(
    `/api/v1/detection/alerts/ai-analysis/${sessionId}/tool-result`,
    { call_id: callId, result }
  )
}

// 应用结论
export const applyAnalysisConclusion = (
  sessionId: string,
  conclusions: Array<{ alert_id: string; action: string }>
) => {
  return api.post(
    `/api/v1/detection/alerts/ai-analysis/${sessionId}/conclusion`,
    { conclusions }
  )
}
```

---

## 5. 状态管理

### 5.1 Pinia Store

```typescript
// stores/aiAnalysis.ts

import { defineStore } from 'pinia'

interface AIMessage {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  toolCalls?: any[]
  toolResults?: any[]
}

interface ToolCall {
  callId: string
  tool: string
  arguments: any
  result?: any
  error?: string
  status: 'pending' | 'executing' | 'success' | 'error'
}

export const useAIAnalysisStore = defineStore('aiAnalysis', {
  state: () => ({
    sessionId: '',
    messages: [] as AIMessage[],
    activeToolCalls: new Map<string, ToolCall>(),
    selectedAlertIds: [] as string[],
    isAnalyzing: false
  }),

  actions: {
    initSession(sessionId: string, alertIds: string[]) {
      this.sessionId = sessionId
      this.selectedAlertIds = alertIds
      this.messages = []
      this.activeToolCalls.clear()
    },

    addMessage(message: AIMessage) {
      this.messages.push(message)
    },

    updateToolCall(callId: string, updates: Partial<ToolCall>) {
      const existing = this.activeToolCalls.get(callId)
      if (existing) {
        Object.assign(existing, updates)
      }
    },

    setToolResult(callId: string, result: any, executionTime: number) {
      const tool = this.activeToolCalls.get(callId)
      if (tool) {
        tool.result = result
        tool.status = 'success'
        // 更新对应消息的工具结果
        const msg = this.messages.find(m =>
          m.toolCalls?.some(tc => tc.callId === callId)
        )
        if (msg) {
          if (!msg.toolResults) msg.toolResults = []
          msg.toolResults.push({ callId, tool: tool.tool, data: result, executionTime })
        }
      }
    },

    setToolError(callId: string, error: string) {
      const tool = this.activeToolCalls.get(callId)
      if (tool) {
        tool.error = error
        tool.status = 'error'
      }
    },

    clearSession() {
      this.sessionId = ''
      this.messages = []
      this.activeToolCalls.clear()
      this.selectedAlertIds = []
      this.isAnalyzing = false
    }
  }
})
```

---

## 6. 样式设计

### 6.1 主题变量

```css
/* V5.6 新增样式变量 */

:root {
  /* AI分析面板 */
  --ai-panel-bg: #f8fafc;
  --ai-message-user-bg: #e6f0ff;
  --ai-message-assistant-bg: #ffffff;
  --ai-tool-call-bg: #fef3c7;
  --ai-tool-result-bg: #ecfdf5;

  /* 工具调用状态 */
  --tool-executing-color: #f59e0b;
  --tool-success-color: #10b981;
  --tool-error-color: #ef4444;
}
```

---

## 7. V5.6 Update (2026-04-23)

### 7.1 AI分析页面重构

V5.6版本对AI分析页面进行了以下更新：

#### 7.1.1 Bug修复

| Bug | 描述 | 修复方案 |
|-----|------|---------|
| Thought内容重复 | AI思考内容在多个对话框内重复出现 | 修改 `flushThought` 函数判断条件，避免空内容被推送 |
| 状态标签多余 | 历史会话对话框中显示"进行中"状态标签 | 移除状态标签显示 |
| 结论文本溢出 | 最终结论输出文字超出对话框 | 添加CSS样式 `word-break: break-word` |
| 历史加载不完整 | 加载历史会话时只显示用户消息，LLM回答丢失 | 修复 `response.data.messages` 访问路径 |

#### 7.1.2 新增功能

| 功能 | 描述 | 实现方式 |
|-----|------|---------|
| 会话持久化 | 页面刷新后保持当前对话 | 使用 localStorage 保存会话ID和消息内容 |
| 历史会话分页 | 历史会话列表支持分页 | 使用 `el-pagination` 组件 |
| 会话删除 | 支持删除历史会话 | 新增 `deleteSession` API |

#### 7.1.3 会话持久化实现

```typescript
// localStorage keys
const CURRENT_SESSION_KEY = 'aegis_current_session_id'

// 保存当前会话ID
function saveCurrentSessionId() {
  localStorage.setItem(CURRENT_SESSION_KEY, sessionId.value)
}

// 页面加载时恢复会话
onMounted(() => {
  const savedId = localStorage.getItem(CURRENT_SESSION_KEY)
  if (savedId) {
    savedSessionId.value = savedId
    sessionId.value = savedId
    loadConversation() // 从localStorage恢复消息
  }
})

// 自动保存消息变化
watch(messages, () => {
  saveConversation()
}, { deep: true })
```

#### 7.1.4 SSE流式交互

```typescript
// SSE事件类型
type SSEEventType = 'thinking' | 'tool_call' | 'tool_result' | 'tool_error' | 'content' | 'done' | 'error'

interface SSEEvent {
  type: SSEEventType
  content?: string
  tool?: string
  call_id?: string
  args?: Record<string, any>
  result?: any
  time_ms?: number
  error?: string
}
```

#### 7.1.5 流程图图片输出

AI分析页面必须在最终回复包含 `attack_graph` 时生成流程图图片。前端实现要求：

| 要求 | 说明 |
|------|------|
| 稳定解析 | 支持纯 JSON、`Final Answer:` 前缀、Markdown fenced code block 和前后说明文字 |
| 单视图 | 只展示交互式 `AttackGraph`，不在页面正文重复展示图片模型返回的静态图片 |
| 直接可见 | 结构化溯源图在分析完成后自动出现，不要求用户复制 JSON 或手动刷新 |
| 操作能力 | 保留下载当前溯源图能力；图片模型失败时显示明确提示并使用本地 SVG 兜底 |
| 可访问性 | 图表按钮必须有文字或可识别图标，信息面板字段统一中文展示 |

AI 分析 SSE 新增 `flowchart_image` 事件。前端可以接收并缓存 `result.url`，但页面主视图始终以 `attack_graph` 为准；如果事件携带 `error`，前端展示告警提示，并使用 `attack_graph.nodes`、`attack_graph.edges` 和 `attack_graph.timeline` 生成 SVG 兜底图。

AI 分析页面补充约束：

- 左侧告警区必须展示真实告警列表，不允许用固定示例数据替代。
- 左侧告警区展示的事件必须同时受时间范围和主机过滤约束；主机过滤支持多主机。
- 时间范围和主机过滤都为空时，左侧事件列表必须为空，不默认加载全量事件。
- 主机过滤下拉必须独立加载在线主机，只允许选择在线主机，不能依赖当前告警列表反推主机选项。
- 时间范围和主机过滤都为空时，不发起告警列表加载请求，表格也不能显示 loading 转圈。
- 时间范围或主机过滤生效后，左侧事件列表必须通过 `/api/v1/detection/alerts` 的后端筛选参数按页加载；前端不得循环拉取全部告警后再本地筛选，避免大数据量下页面一直 loading。
- 当时间范围或主机过滤变化时，已选择但不再可见的事件必须从当前选择中移除，避免提交不可见事件。
- 创建 AI 分析会话时必须冻结本次选中的事件快照，并在分析过程中继续显示该快照，不因刷新告警列表或筛选条件变化而丢失。
- 会话持久化必须保存本次分析事件快照，页面刷新恢复会话时仍能看到当次分析使用的事件列表。
- 创建会话后，首条分析消息和会话上下文必须反映本次所选告警的真实数量、主机、规则、级别和时间范围。
- 分析完成后，告警详情弹窗中的 `LLM摘要`/`处置策略` 必须能直接看到本次 AI 分析回写结果，不要求用户额外手工同步。
- 溯源图中的 `threatLevel`、节点类型、边标签、时间线事件和处置建议等所有面向用户的文案必须优先显示中文。

全局桌面布局补充约束：

- 登录后的主控制台是桌面型安全运营界面，左侧导航、顶部状态栏和主内容区不能在移动 Safari 的窄视口下被强行压缩。
- 当移动端浏览器请求桌面网站时，主控制台必须切换为 `width=1280` 的桌面 viewport，并保持不小于 1280px 的桌面画布宽度，让 Safari 按桌面宽度缩放整页，而不是只截取或挤压手机屏宽内的一部分内容。
- 控制台 viewport 必须在应用渲染前根据路径完成初始化；从登录/改密进入控制台时使用整页跳转，避免 iOS Safari 在 SPA 路由切换后不重新应用桌面 viewport。
- 登录页、强制改密页等认证页面仍按原有响应式逻辑渲染，不继承主控制台的桌面最小宽度。

### 7.3 系统配置页图片模型配置

系统配置页拆分为两个模型配置区：

| 配置区 | 用途 | 推荐默认配置 |
|--------|------|-------------------|
| 文本 LLM 配置 | 告警分析、规则生成、漏洞分析 | `MiniMax-M2.7` / `https://api.minimaxi.com/anthropic` |
| 图片模型配置 | 流程图、报告图、后续图片生成能力 | `cogview-3-flash` / `https://open.bigmodel.cn/api/paas/v4` |

图片模型配置表单字段：

| 字段 | 说明 |
|------|------|
| provider | 厂商，支持 `minimax`、`zhipu` 和 `custom` |
| api_key | 图片模型 API Key，保存后仅显示脱敏值 |
| base_url | API 根地址 |
| model_name | 图片模型名称 |

前端 API：

### 7.4 全局 UI 视觉升级方案

V5.6 前端统一采用“安全运营指挥台”视觉系统，覆盖除业务图谱外的所有页面基础体验。改造目标不是单页换色，而是通过全局设计 token 与 Element Plus 组件覆写，让主机、基线、漏洞、异常检测、策略、规则、任务和系统配置页面共享同一套视觉语言。

设计原则：

| 维度 | 规范 |
|------|------|
| 产品气质 | 冷静、专业、高信息密度的安全运营控制台 |
| 主色 | 深海军蓝 `#0b1220`、电光青 `#22d3ee`、行动蓝 `#2563eb` |
| 背景 | 浅灰蓝渐变 + 微弱网格/光斑，避免纯灰后台模板感 |
| 卡片 | 白色半透明、细边框、低阴影、12px 圆角 |
| 表格 | 去重边框，强化行 hover、表头层次和状态徽章 |
| 表单 | 明确 label、聚焦态边框、说明区与操作区分层 |
| 动效 | 150-300ms，使用 transform/opacity，不阻塞交互 |
| 可访问性 | 正文对比度不低于 4.5:1，按钮不只依赖图标 |

全局样式文件：

| 文件 | 职责 |
|------|------|
| `src/styles/aegis-theme.css` | CSS 变量、背景、Element Plus 组件覆写、通用页面类 |
| `src/App.vue` | App Shell、侧边导航、顶部状态栏、内容舞台 |

字体与截图规范：

| 要求 | 说明 |
|------|------|
| 中文字体 token | 全局必须通过 `--aegis-font-sans` 定义含 CJK 兜底的字体栈，优先覆盖 `Noto Sans CJK SC`、`Noto Sans SC`、`Source Han Sans SC`、`Microsoft YaHei`、`PingFang SC` 和 `WenQuanYi Micro Hei` |
| 等宽字体 token | 代码、JSON、日志与进程树应通过 `--aegis-font-mono` 使用等宽字体，并保留 CJK 等宽兜底 |
| 图谱字体 | `AttackGraph` 中的 SVG 文本必须显式使用 `--aegis-font-sans`，不能依赖浏览器默认 `sans-serif` |
| 截图环境 | 生成 `docs/screenshots/ui-refresh/` 截图前，浏览器所在环境必须安装中文字体，例如 `fonts-noto-cjk` 或 `fonts-wqy-microhei` |

所有业务页面应优先复用全局类：

| 类名 | 用途 |
|------|------|
| `.page-shell` | 页面根容器 |
| `.page-hero` | 页面顶部标题/说明/操作区 |
| `.aegis-card` | 业务卡片 |
| `.metric-grid` / `.metric-card` | 指标卡片 |
| `.status-pill` | 状态胶囊 |
| `.aegis-toolbar` | 搜索、筛选、批量操作工具条 |

截图归档要求：每次 UI 视觉调整完成后，将主要页面截图保存到 `docs/screenshots/ui-refresh/` 并随代码提交，用于评审和回归对比。

```typescript
getImageModelConfig(): Promise<ImageModelConfig>
saveImageModelConfig(data: ImageModelConfigRequest): Promise<void>
testImageModelConnection(data: ImageModelConfigRequest): Promise<void>
```

### 7.2 API响应格式统一

所有API响应统一使用以下格式：

```typescript
// 成功响应
{
  success: true,
  data: { ... }
}

// 错误响应
{
  success: false,
  message: "错误信息"
}
```

前端API拦截器处理：
- 提取 `data` 字段返回
- 统一错误处理和提示

---

**文档结束**

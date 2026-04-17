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
| AI降噪多轮分析 | `/detection/alerts` | 多选告警+时间范围+多轮对话 |
| 工具调用展示 | 嵌入在AI分析面板 | 展示AI调用的工具及结果 |

---

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
      <span class="title">AI降噪分析</span>
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
  time_range: { start: string; end: string }
  host_filter?: string[]
}) => {
  return api.post('/api/v1/detection/alerts/ai-analysis/session', params)
}

// 发送消息
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

**文档结束**

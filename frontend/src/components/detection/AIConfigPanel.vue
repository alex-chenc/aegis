<template>
  <div class="ai-config-panel">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>AI规则更新配置</span>
          <el-switch
            v-model="localConfig.enabled"
            active-text="启用"
            inactive-text="禁用"
            @change="handleEnabledChange"
          />
        </div>
      </template>

      <el-form :model="localConfig" label-width="140px" class="config-form">
        <!-- 模式选择 -->
        <el-form-item label="自动规则更新">
          <el-radio-group v-model="localConfig.mode" :disabled="!localConfig.enabled" @change="handleConfigChange">
            <el-radio value="suggest">仅建议（生成规则后需人工审核）</el-radio>
            <el-radio value="auto">自动（满足条件自动生成并激活）</el-radio>
          </el-radio-group>
        </el-form-item>

        <!-- 触发条件 -->
        <el-form-item label="触发条件">
          <div class="trigger-config">
            <span class="trigger-label">同一MITRE ID在</span>
            <el-input-number
              v-model="localConfig.thresholds.high_frequency_hours"
              :min="1"
              :max="24"
              :disabled="!localConfig.enabled"
              controls-position="right"
              class="threshold-input"
              @change="handleConfigChange"
            />
            <span class="trigger-label">小时内触发</span>
            <el-input-number
              v-model="localConfig.thresholds.high_frequency_count"
              :min="10"
              :max="100"
              :disabled="!localConfig.enabled"
              controls-position="right"
              class="threshold-input"
              @change="handleConfigChange"
            />
            <span class="trigger-label">次，即进行AI更新规则</span>
          </div>
        </el-form-item>

        <!-- 规则生成策略 - 滑块 -->
        <el-form-item label="规则生成策略">
          <div class="slider-container">
            <span class="slider-label">保守</span>
            <el-slider
              v-model="localConfig.conservatism"
              :min="0"
              :max="1"
              :step="0.1"
              :disabled="!localConfig.enabled"
              :format-tooltip="formatConservatism"
              @change="handleConfigChange"
            />
            <span class="slider-label">激进</span>
            <span class="slider-value">{{ (localConfig.conservatism * 100).toFixed(0) }}%</span>
          </div>
          <div class="slider-hint">
            保守模式：只检测明确的恶意行为特征，误报率低<br>
            激进模式：检测更多可能的威胁模式，覆盖率高但可能有更多误报
          </div>
        </el-form-item>

        <!-- 审核配置 -->
        <el-form-item label="审核配置">
          <el-checkbox
            v-model="localConfig.require_approval"
            :disabled="!localConfig.enabled"
            @change="handleConfigChange"
          >
            规则生成后发送审核通知
          </el-checkbox>
          <el-checkbox
            v-if="localConfig.mode === 'suggest'"
            v-model="localConfig.auto_activate_after_approval"
            :disabled="!localConfig.enabled || !localConfig.require_approval"
            @change="handleConfigChange"
          >
            无人审核后24小时自动从待审核调整为实验性
          </el-checkbox>
        </el-form-item>

        <!-- 统计信息 -->
        <el-form-item label="统计信息">
          <div class="stats-row">
            <el-statistic title="已生成规则数" :value="localConfig.rules_generated_count" />
            <el-statistic title="已审核通过数" :value="localConfig.rules_approved_count" />
          </div>
        </el-form-item>

        <!-- 操作按钮 -->
        <el-form-item>
          <el-button type="primary" :loading="saving" :disabled="!hasChanges" @click="saveConfig">
            保存配置
          </el-button>
          <el-button :loading="testing" :disabled="!localConfig.enabled" @click="testRuleGeneration">
            测试规则生成
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 测试规则生成对话框 -->
    <el-dialog v-model="testDialogVisible" title="测试规则生成" width="700px">
      <el-form :model="testForm" label-width="100px">
        <el-form-item label="MITRE技术ID" required>
          <el-input v-model="testForm.mitre_id" placeholder="例如：T1059.004" />
        </el-form-item>
        <el-form-item label="保守度">
          <el-slider
            v-model="testForm.conservatism"
            :min="0"
            :max="1"
            :step="0.1"
            :format-tooltip="formatConservatism"
          />
          <span>{{ (testForm.conservatism * 100).toFixed(0) }}%</span>
        </el-form-item>
      </el-form>

      <div v-if="testResult" class="test-result">
        <el-divider>生成结果</el-divider>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="规则ID">{{ testResult.rule_id }}</el-descriptions-item>
          <el-descriptions-item label="规则标题">{{ testResult.title }}</el-descriptions-item>
          <el-descriptions-item label="MITRE">{{ testResult.mitre_id || '-' }}</el-descriptions-item>
          <el-descriptions-item label="严重程度">{{ testResult.severity }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag type="info">{{ testResult.status }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="规则内容" :span="2">
            <pre class="content-block">{{ testResult.content }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <template #footer>
        <el-button @click="testDialogVisible = false">关闭</el-button>
        <el-button type="primary" :loading="testing" @click="executeTestGeneration">
          开始生成
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getAIConfig, updateAIConfig, generateTestRule, type AIConfig, type UpdateAIConfigRequest } from '@/api/detection'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const testDialogVisible = ref(false)

const originalConfig = ref<AIConfig | null>(null)

const localConfig = reactive<AIConfig>({
  id: '',
  name: 'default',
  enabled: false,
  mode: 'suggest',
  thresholds: {
    high_frequency_count: 10,
    high_frequency_hours: 1
  },
  conservatism: 0.5,
  require_approval: true,
  auto_activate_after_approval: false,
  activation_delay_hours: 24,
  notify_on_generation: true,
  notify_on_approval: true,
  notification_targets: [],
  rules_generated_count: 0,
  rules_approved_count: 0
})

const testForm = reactive({
  mitre_id: '',
  conservatism: 0.5
})

const testResult = ref<{
  rule_id: string
  title: string
  mitre_id: string
  severity: string
  content: string
  status: string
} | null>(null)

const hasChanges = computed(() => {
  if (!originalConfig.value) return false
  return JSON.stringify(localConfig) !== JSON.stringify(originalConfig.value)
})

function formatConservatism(val: number): string {
  return `${(val * 100).toFixed(0)}%`
}

async function loadConfig() {
  loading.value = true
  try {
    const config = await getAIConfig()
    Object.assign(localConfig, config)
    originalConfig.value = JSON.parse(JSON.stringify(config))
  } catch (error: any) {
    ElMessage.error('加载AI配置失败: ' + (error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

async function handleEnabledChange() {
  await saveConfig()
}

async function handleConfigChange() {
  // 配置变化时自动保存或标记有变化
}

async function saveConfig() {
  saving.value = true
  try {
    const request: UpdateAIConfigRequest = {
      enabled: localConfig.enabled,
      mode: localConfig.mode,
      thresholds: localConfig.thresholds,
      conservatism: localConfig.conservatism,
      require_approval: localConfig.require_approval,
      auto_activate_after_approval: localConfig.auto_activate_after_approval,
      notify_on_generation: localConfig.notify_on_generation,
      notify_on_approval: localConfig.notify_on_approval
    }

    const updated = await updateAIConfig(request)
    Object.assign(localConfig, updated)
    originalConfig.value = JSON.parse(JSON.stringify(updated))
    ElMessage.success('配置保存成功')
  } catch (error: any) {
    ElMessage.error('保存配置失败: ' + (error.message || '未知错误'))
  } finally {
    saving.value = false
  }
}

function testRuleGeneration() {
  testForm.mitre_id = ''
  testForm.conservatism = localConfig.conservatism
  testResult.value = null
  testDialogVisible.value = true
}

async function executeTestGeneration() {
  if (!testForm.mitre_id) {
    ElMessage.warning('请输入MITRE技术ID')
    return
  }

  testing.value = true
  try {
    const result = await generateTestRule({
      mitre_id: testForm.mitre_id,
      conservatism: testForm.conservatism
    })
    testResult.value = result
    ElMessage.success('测试规则生成成功')
  } catch (error: any) {
    ElMessage.error('测试规则生成失败: ' + (error.message || '未知错误'))
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  loadConfig()
})
</script>

<style scoped>
.ai-config-panel {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.config-form {
  max-width: 800px;
}

.slider-container {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.slider-label {
  font-size: 12px;
  color: #909399;
  min-width: 40px;
}

.slider-value {
  font-size: 14px;
  font-weight: 500;
  min-width: 50px;
  text-align: right;
}

.slider-hint {
  font-size: 12px;
  color: #909399;
  margin-top: 8px;
  line-height: 1.5;
}

.trigger-config {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.trigger-label {
  font-size: 14px;
  color: #606266;
}

.threshold-input {
  width: 100px;
}

.approval-delay-config {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}

.approval-label {
  font-size: 14px;
  color: #606266;
}

.approval-hint {
  font-size: 14px;
  color: #909399;
  margin-top: 8px;
}

.stats-row {
  display: flex;
  gap: 40px;
}

.test-result {
  margin-top: 16px;
}

.content-block {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 300px;
  overflow-y: auto;
  background: #f5f7fa;
  padding: 10px;
  border-radius: 4px;
  font-size: 12px;
}
</style>

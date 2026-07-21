<template>
  <div class="ai-config-panel">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.detectionAIConfigPanel_ai_rule_update_configuration_6c4170') }}</span>
          <el-switch
            v-model="localConfig.enabled"
            :active-text="$t('dynamic.activeEnabled')"
            :inactive-text="$t('dynamic.activeDisabled')"
            @change="handleEnabledChange"
          />
        </div>
      </template>

      <el-form :model="localConfig" label-width="140px" class="config-form">
        <!-- 模式选择 -->
        <el-form-item :label="$t('generated.detectionAIConfigPanel_automatic_rule_updates_2b2814')">
          <el-radio-group v-model="localConfig.mode" :disabled="!localConfig.enabled" @change="handleConfigChange">
            <el-radio value="suggest">{{ $t('generated.detectionAIConfigPanel_suggestions_only_generate_rules_to_be_12fc31') }}</el-radio>
            <el-radio value="auto">{{ $t('generated.detectionAIConfigPanel_automatic_set_as_experimental_and_released_2aaaf6') }}</el-radio>
          </el-radio-group>
        </el-form-item>

        <!-- 触发条件 -->
        <el-form-item :label="$t('generated.detectionAIConfigPanel_trigger_condition_f5c950')">
          <div class="trigger-config">
            <span class="trigger-label">{{ $t('generated.detectionAIConfigPanel_the_same_miter_id_is_in_f35d9c') }}</span>
            <el-input-number
              v-model="localConfig.thresholds.high_frequency_hours"
              :min="1"
              :max="24"
              :disabled="!localConfig.enabled"
              controls-position="right"
              class="threshold-input"
              @change="handleConfigChange"
            />
            <span class="trigger-label">{{ $t('generated.detectionAIConfigPanel_trigger_within_hours_ff704b') }}</span>
            <el-input-number
              v-model="localConfig.thresholds.high_frequency_count"
              :min="10"
              :max="100"
              :disabled="!localConfig.enabled"
              controls-position="right"
              class="threshold-input"
              @change="handleConfigChange"
            />
            <span class="trigger-label">{{ $t('generated.detectionAIConfigPanel_times_that_is_the_ai_update_a73a26') }}</span>
          </div>
        </el-form-item>

        <!-- 规则生成策略 - 滑块 -->
        <el-form-item :label="$t('generated.detectionAIConfigPanel_rule_generation_strategy_e5bea2')">
          <div class="slider-container">
            <span class="slider-label">{{ $t('generated.detectionAIConfigPanel_keep_581158') }}</span>
            <el-slider
              v-model="localConfig.conservatism"
              :min="0"
              :max="1"
              :step="0.1"
              :disabled="!localConfig.enabled"
              :format-tooltip="formatConservatism"
              @change="handleConfigChange"
            />
            <span class="slider-label">{{ $t('generated.detectionAIConfigPanel_radical_5a628f') }}</span>
            <span class="slider-value">{{ (localConfig.conservatism * 100).toFixed(0) }}%</span>
          </div>
          <div class="slider-hint">
            {{ $t('generated.detectionAIConfigPanel_conservative_mode_only_detects_clear_malicious_24922b') }}<br>
            {{ $t('generated.detectionAIConfigPanel_aggressive_mode_detects_more_possible_threat_bec1e5') }}
          </div>
        </el-form-item>

        <!-- 审核配置 -->
        <el-form-item :label="$t('generated.detectionAIConfigPanel_audit_configuration_399fde')">
          <el-checkbox
            v-model="localConfig.require_approval"
            :disabled="!localConfig.enabled"
            @change="handleConfigChange"
          >
            {{ $t('generated.detectionAIConfigPanel_send_audit_notification_after_rule_generation_4b88ad') }}
          </el-checkbox>
        </el-form-item>

        <!-- 统计信息 -->
        <el-form-item :label="$t('generated.detectionAIConfigPanel_statistics_5b00a5')">
          <div class="stats-row">
            <el-statistic :title="$t('generated.detectionAIConfigPanel_number_of_rules_generated_608d95')" :value="localConfig.rules_generated_count" />
            <el-statistic :title="$t('generated.detectionAIConfigPanel_number_of_approved_reviews_ba063a')" :value="localConfig.rules_approved_count" />
          </div>
        </el-form-item>

        <!-- 操作按钮 -->
        <el-form-item>
          <el-button type="primary" :loading="saving" :disabled="!hasChanges" @click="saveConfig">
            {{ $t('generated.common_save_configuration_817af1') }}
          </el-button>
          <el-button :loading="testing" :disabled="!localConfig.enabled" @click="testRuleGeneration">
            {{ $t('generated.detectionAIConfigPanel_test_rule_generation_687673') }}
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 测试规则生成对话框 -->
    <el-dialog v-model="testDialogVisible" :title="$t('generated.detectionAIConfigPanel_test_rule_generation_687673')" width="700px">
      <el-form :model="testForm" label-width="100px">
        <el-form-item :label="$t('generated.detectionAIConfigPanel_miter_technology_id_8d3aee')" required>
          <el-input v-model="testForm.mitre_id" :placeholder="$t('generated.detectionAIConfigPanel_for_example_t1059_004_887fa2')" />
        </el-form-item>
        <el-form-item :label="$t('generated.detectionAIConfigPanel_conservatism_aa3dd2')">
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
        <el-divider>{{ $t('generated.common_generate_results_99045f') }}</el-divider>
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('generated.common_rule_id_36c0e3')">{{ testResult.rule_id }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_rule_title_298a16')">{{ testResult.title }}</el-descriptions-item>
          <el-descriptions-item label="MITRE">{{ testResult.mitre_id || '-' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_severity_d918e4')">{{ testResult.severity }}</el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_state_62e951')">
            <el-tag type="info">{{ testResult.status }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_rule_content_3bfca1')" :span="2">
            <pre class="content-block">{{ testResult.content }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <template #footer>
        <el-button @click="testDialogVisible = false">{{ $t('generated.common_closure_6c14bd') }}</el-button>
        <el-button type="primary" :loading="testing" @click="executeTestGeneration">
          {{ $t('generated.detectionAIConfigPanel_start_generating_6793e8') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

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
    ElMessage.error(translate('generatedScript.detectionAIConfigPanel_failed_to_load_ai_configuration_851795') + (error.message || translate('generatedScript.common_unknown_error_5f76ed')))
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
      auto_activate_after_approval: false,
      notify_on_generation: localConfig.notify_on_generation,
      notify_on_approval: localConfig.notify_on_approval
    }

    const updated = await updateAIConfig(request)
    Object.assign(localConfig, updated)
    originalConfig.value = JSON.parse(JSON.stringify(updated))
    ElMessage.success(translate('generatedScript.common_configuration_saved_successfully_597832'))
  } catch (error: any) {
    ElMessage.error(translate('generatedScript.detectionAIConfigPanel_failed_to_save_configuration_0afc59') + (error.message || translate('generatedScript.common_unknown_error_5f76ed')))
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
    ElMessage.warning(translate('generatedScript.detectionAIConfigPanel_please_enter_miter_technology_id_4d1a14'))
    return
  }

  testing.value = true
  try {
    const result = await generateTestRule({
      mitre_id: testForm.mitre_id,
      conservatism: testForm.conservatism
    })
    testResult.value = result
    ElMessage.success(translate('generatedScript.detectionAIConfigPanel_test_rules_generated_successfully_8399de'))
  } catch (error: any) {
    ElMessage.error(translate('generatedScript.detectionAIConfigPanel_test_rule_generation_failed_f83ad2') + (error.message || translate('generatedScript.common_unknown_error_5f76ed')))
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

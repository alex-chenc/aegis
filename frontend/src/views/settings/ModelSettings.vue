<template>
  <div class="settings page-shell">
    <section class="page-hero settings-hero">
      <div>
        <span class="hero-kicker">Model Control Plane</span>
        <h1>{{ $t('generated.settingsModelSettings_model_configuration_4db4aa') }}</h1>
        <p>{{ $t('generated.settingsModelSettings_centrally_configure_text_reasoning_models_the_4d7b8f') }}</p>
      </div>
    </section>

    <div class="settings-grid">
      <el-card class="aegis-card settings-card">
        <template #header>
          <div class="card-header">
            <span>{{ $t('generated.settingsModelSettings_text_llm_configuration_e0beda') }}</span>
            <el-tag type="info" size="small">{{ $t('generated.settingsModelSettings_configure_via_page_only_11de86') }}</el-tag>
          </div>
        </template>

        <el-alert
          :title="$t('generated.settingsModelSettings_configuration_instructions_3315c3')"
          type="info"
          show-icon
          :closable="false"
          class="settings-alert"
        >
          <p>{{ $t('generated.settingsModelSettings_llm_configuration_is_only_set_through_5c7ce2') }}</p>
          <p>{{ $t('generated.settingsModelSettings_after_selecting_the_manufacturer_the_recommended_903801') }}</p>
        </el-alert>

        <el-form :model="form" label-width="120px">
          <el-form-item :label="$t('generated.settingsModelSettings_model_manufacturer_27c82a')">
            <el-radio-group v-model="form.provider" class="provider-selector" @change="handleProviderChange">
              <el-radio-button
                v-for="provider in providerOptions"
                :key="provider.value"
                :label="provider.value"
              >
                {{ provider.label }}
              </el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="API Key">
            <el-input
              v-model="form.api_key"
              :type="apiKeyVisible ? 'text' : 'password'"
              :placeholder="$t('generated.settingsModelSettings_please_enter_the_api_key_if_aeac98')"
            >
              <template #suffix>
                <el-icon class="cursor-pointer" @click="toggleApiKeyVisibility">
                  <View v-if="apiKeyVisible" />
                  <Hide v-else />
                </el-icon>
              </template>
            </el-input>
          </el-form-item>
          <el-form-item label="Base URL">
            <el-input v-model="form.base_url" :placeholder="selectedProvider?.baseURL || 'https://api.example.com/v1'" />
          </el-form-item>
          <el-form-item :label="$t('generated.settingsModelSettings_model_name_38719c')">
            <el-input v-model="form.model_name" :placeholder="selectedProvider?.modelName || 'model-name'" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="saving" @click="saveConfig">{{ $t('generated.common_save_configuration_817af1') }}</el-button>
            <el-button :loading="testing" @click="testConnection">{{ $t('generated.settingsModelSettings_test_connection_10b7d8') }}</el-button>
          </el-form-item>
        </el-form>
      </el-card>

    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Hide, View } from '@element-plus/icons-vue'
import { getFullAPIKey } from '@/api/config'
import { useConfigStore } from '@/store/config'

const configStore = useConfigStore()

const providerOptions = computed(() => [
  {
    value: 'deepseek',
    label: 'DeepSeek',
    baseURL: 'https://api.deepseek.com/v1',
    modelName: 'deepseek-chat'
  },
  {
    value: 'dashscope',
    label: translate('generatedScript.settingsModelSettings_alibaba_cloud_bailian_97031b'),
    baseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    modelName: 'qwen-plus'
  },
  {
    value: 'minimax',
    label: 'MiniMax',
    baseURL: 'https://api.minimaxi.com/anthropic',
    modelName: 'MiniMax-M2.7'
  },
  {
    value: 'openai',
    label: 'OpenAI',
    baseURL: 'https://api.openai.com/v1',
    modelName: 'gpt-4o-mini'
  },
  {
    value: 'custom',
    label: translate('generatedScript.common_customize_c49333'),
    baseURL: '',
    modelName: ''
  }
])

const form = ref({
  api_key: '',
  provider: 'deepseek',
  base_url: 'https://api.deepseek.com/v1',
  model_name: 'deepseek-chat'
})
const selectedProvider = computed(() => providerOptions.value.find((item) => item.value === form.value.provider))
const saving = ref(false)
const testing = ref(false)
const apiKeyVisible = ref(false)
const originalApiKey = ref('')

const inferProvider = (baseURL: string) => {
  const url = baseURL.toLowerCase()
  if (url.includes('deepseek')) return 'deepseek'
  if (url.includes('dashscope') || url.includes('aliyuncs')) return 'dashscope'
  if (url.includes('minimaxi') || url.includes('minimax')) return 'minimax'
  if (url.includes('openai')) return 'openai'
  return 'custom'
}

const handleProviderChange = () => {
  const provider = selectedProvider.value
  if (!provider || provider.value === 'custom') return

  form.value.base_url = provider.baseURL
  form.value.model_name = provider.modelName
}

const saveConfig = async () => {
  if (form.value.api_key.includes('****')) {
    ElMessage.warning(translate('generatedScript.settingsModelSettings_please_enter_the_complete_api_key_c49e60'))
    return
  }

  if (!form.value.api_key) {
    ElMessage.warning(translate('generatedScript.settingsModelSettings_please_enter_api_key_99a3ab'))
    return
  }

  saving.value = true
  try {
    await configStore.saveLLMConfig(form.value.api_key, form.value.provider, form.value.base_url, form.value.model_name)
    originalApiKey.value = form.value.api_key
    ElMessage.success(translate('generatedScript.common_configuration_saved_successfully_597832'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_save_failed_40525a'))
  } finally {
    saving.value = false
  }
}

const testConnection = async () => {
  if (!form.value.api_key) {
    ElMessage.warning(translate('generatedScript.settingsModelSettings_please_enter_api_key_99a3ab'))
    return
  }

  testing.value = true
  try {
    let apiKeyToUse = form.value.api_key
    if (apiKeyToUse.includes('****')) {
      const data = await getFullAPIKey()
      apiKeyToUse = data.api_key
      originalApiKey.value = apiKeyToUse
    }

    await configStore.testConnection(apiKeyToUse, form.value.provider, form.value.base_url, form.value.model_name)
    ElMessage.success(translate('generatedScript.settingsModelSettings_connection_test_successful_bfbf35'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsModelSettings_connection_test_failed_6ef134'))
  } finally {
    testing.value = false
  }
}

const toggleApiKeyVisibility = async () => {
  if (!apiKeyVisible.value && !originalApiKey.value) {
    try {
      const data = await getFullAPIKey()
      originalApiKey.value = data.api_key
      form.value.api_key = originalApiKey.value
    } catch {
      ElMessage.error(translate('generatedScript.settingsModelSettings_failed_to_obtain_api_key_58fb1d'))
      return
    }
  } else if (!apiKeyVisible.value) {
    form.value.api_key = originalApiKey.value
  } else {
    form.value.api_key = configStore.llmConfig?.api_key_masked || ''
  }
  apiKeyVisible.value = !apiKeyVisible.value
}

onMounted(async () => {
  try {
    await configStore.fetchLLMConfig()
    if (configStore.llmConfig) {
      form.value.provider = configStore.llmConfig.provider || inferProvider(configStore.llmConfig.base_url)
      form.value.base_url = configStore.llmConfig.base_url
      form.value.model_name = configStore.llmConfig.model_name
      form.value.api_key = configStore.llmConfig.api_key_masked || ''
    }
  } catch {
    // Keep page usable when configuration has not been initialized.
  }
})
</script>

<style scoped>
.settings-hero {
  margin-bottom: 0;
}

.hero-kicker {
  display: inline-flex;
  margin-bottom: 8px;
  color: #0891b2;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.settings-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px;
}

.settings-card {
  overflow: hidden;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.settings-alert {
  margin-bottom: 20px;
}

.cursor-pointer {
  cursor: pointer;
}

.provider-selector {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.provider-selector :deep(.el-radio-button__inner) {
  min-width: 92px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  border-radius: 999px;
  font-weight: 650;
}

.provider-selector :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  border-color: transparent;
  background: linear-gradient(135deg, #2563eb, #0891b2);
  box-shadow: 0 10px 24px rgba(37, 99, 235, 0.22);
}
</style>

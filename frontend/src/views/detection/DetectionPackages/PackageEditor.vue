<template>
  <div class="package-editor-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <div>
            <el-button link @click="$router.back()">
              <el-icon><ArrowLeft /></el-icon> {{ $t('generated.common_return_11d024') }}
            </el-button>
            <span style="margin-left: 12px; font-weight: bold;">{{ isEdit ? $t('dynamic.editDetectionPackage') : $t('dynamic.newDetectionPackage') }}</span>
          </div>
          <div>
            <el-button @click="handleAIGenerate" :loading="generating">{{ $t('generated.detectionDetectionPackagesPackageEditor_ai_generated_draft_3b6972') }}</el-button>
            <el-button type="primary" :loading="saving" @click="handleSave">{{ $t('generated.detectionDetectionPackagesPackageEditor_save_draft_4cd30e') }}</el-button>
          </div>
        </div>
      </template>

      <el-tabs v-model="activeTab">
        <el-tab-pane :label="$t('generated.common_basic_information_41654e')" name="basic">
          <el-form :model="form" label-width="120px" style="max-width: 600px;">
            <el-form-item v-if="isEdit" label="Package ID">
              <el-input v-model="form.package_id" disabled />
            </el-form-item>
            <el-form-item :label="$t('generated.common_version_989d1a')" required>
              <el-input v-model="form.target_version" :placeholder="$t('generated.detectionDetectionPackagesPackageEditor_such_as_1_0_0_949313')" />
            </el-form-item>
            <el-form-item :label="$t('generated.common_title_748d7d')" required>
              <el-input v-model="form.title" :placeholder="$t('generated.detectionDetectionPackagesPackageEditor_detection_package_title_d86270')" />
            </el-form-item>
            <el-form-item :label="$t('generated.common_describe_412f54')">
              <el-input v-model="form.description" type="textarea" :rows="3" />
            </el-form-item>
            <el-form-item label="CVE IDs">
              <el-select v-model="form.cve_ids" multiple filterable allow-create :placeholder="$t('generated.detectionDetectionPackagesPackageEditor_enter_cve_id_d51d32')">
              </el-select>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="HookPlan" name="hookplan">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            {{ $t('generated.detectionDetectionPackagesPackageEditor_define_ebpf_hook_points_and_event_cfbd3b') }}
          </el-text>
          <CodeEditorPanel v-model="form.hook_plan_yaml" language="yaml" placeholder="HookPlan YAML" />
        </el-tab-pane>

        <el-tab-pane :label="$t('generated.common_ebpf_source_code_ada0b4')" name="ebpf">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            {{ $t('generated.detectionDetectionPackagesPackageEditor_ebpf_c_source_code_draft_it_b0dd84') }}
          </el-text>
          <CodeEditorPanel v-model="form.ebpf_source" language="c" :placeholder="$t('generated.detectionDetectionPackagesPackageEditor_ebpf_c_source_code_c3a4a2')" />
        </el-tab-pane>

        <el-tab-pane :label="$t('generated.common_sigma_atomic_rules_0c85cc')" name="sigma">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            {{ $t('generated.detectionDetectionPackagesPackageEditor_single_event_matching_rules_rule_id_3c0363') }}
          </el-text>
          <CodeEditorPanel v-model="form.sigma_rules_yaml" language="yaml" placeholder="Sigma YAML" />
        </el-tab-pane>

        <el-tab-pane label="Correlation" name="correlation">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            {{ $t('generated.detectionDetectionPackagesPackageEditor_multiple_event_correlation_rules_only_ordered_d1098d') }}
          </el-text>
          <CodeEditorPanel v-model="form.correlation_yaml" language="yaml" placeholder="Correlation YAML" />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog v-model="aiDialogVisible" :title="$t('generated.detectionDetectionPackagesPackageEditor_ai_generated_draft_3b6972')" width="600">
      <el-form :model="aiForm" label-width="120px">
        <el-form-item label="CVE ID" required>
          <el-input v-model="aiForm.cve_id" :placeholder="$t('generated.detectionDetectionPackagesPackageEditor_such_as_cve_2026_31431_1c3241')" />
        </el-form-item>
        <el-form-item :label="$t('generated.common_vulnerability_description_3eb740')" required>
          <el-input v-model="aiForm.vulnerability_description" type="textarea" :rows="4" :placeholder="$t('generated.detectionDetectionPackagesPackageEditor_describe_how_the_vulnerability_works_and_941c06')" />
        </el-form-item>
        <el-form-item :label="$t('generated.detectionDetectionPackagesPackageEditor_attack_preconditions_88b9f0')">
          <el-input v-model="aiForm.attack_prerequisites" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="$t('generated.detectionDetectionPackagesPackageEditor_exploit_chain_behavior_eb89ee')">
          <el-input v-model="aiForm.exploitation_chain" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="$t('generated.detectionDetectionPackagesPackageEditor_false_positive_constraints_460bb1')">
          <el-input v-model="aiForm.false_positive_constraints" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="aiDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="primary" :loading="generating" @click="confirmAIGenerate">{{ $t('generated.detectionDetectionPackagesPackageEditor_generate_4aa230') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { useDetectionPackages } from './composables/useDetectionPackages'
import CodeEditorPanel from './components/CodeEditorPanel.vue'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const { currentDraft, loading, fetchPackage, fetchDraft, currentPackage, generateDraft, createDraft, updateDraft } = useDetectionPackages()

const isEdit = ref(!!route.params.id)
const activeTab = ref('basic')
const saving = ref(false)
const generating = ref(false)
const aiDialogVisible = ref(false)

const form = reactive({
  package_id: '',
  target_version: '1.0.0',
  title: '',
  description: '',
  cve_ids: [] as string[],
  hook_plan_yaml: '',
  ebpf_source: '',
  sigma_rules_yaml: '',
  correlation_yaml: '',
})

const aiForm = reactive({
  cve_id: '',
  vulnerability_description: '',
  attack_prerequisites: '',
  exploitation_chain: '',
  false_positive_constraints: '',
})

async function loadDraft() {
  if (isEdit.value) {
    await fetchPackage(route.params.id as string)
    if (currentPackage.value) {
      form.package_id = currentPackage.value.package_id
      form.target_version = currentPackage.value.version
      form.title = currentPackage.value.title
      form.description = currentPackage.value.description || ''
      form.cve_ids = currentPackage.value.cve_ids || []
      // Fetch draft-specific fields if package is a draft
      if (['draft', 'build_failed', 'review_rejected'].includes(currentPackage.value.status)) {
        await fetchDraft(currentPackage.value.package_id)
        if (currentDraft.value) {
          form.hook_plan_yaml = currentDraft.value.hook_plan_yaml || ''
          form.ebpf_source = currentDraft.value.ebpf_source || ''
          form.sigma_rules_yaml = currentDraft.value.sigma_rules_yaml || ''
          form.correlation_yaml = currentDraft.value.correlation_yaml || ''
        }
      }
    }
  }
}

function handleAIGenerate() {
  aiDialogVisible.value = true
}

async function confirmAIGenerate() {
  if (!aiForm.cve_id || !aiForm.vulnerability_description) {
    ElMessage.warning(translate('generatedScript.detectionDetectionPackagesPackageEditor_please_fill_in_the_cve_id_71fec4'))
    return
  }
  generating.value = true
  try {
    const draft = await generateDraft(aiForm)
    if (draft) {
      form.package_id = draft.package_id
      form.target_version = draft.target_version
      form.title = draft.title
      form.description = draft.description || ''
      form.cve_ids = draft.cve_ids || []
      form.hook_plan_yaml = draft.hook_plan_yaml || ''
      form.ebpf_source = draft.ebpf_source || ''
      form.sigma_rules_yaml = draft.sigma_rules_yaml || ''
      form.correlation_yaml = draft.correlation_yaml || ''
      aiDialogVisible.value = false
    }
  } catch (e: any) {
    const msg = e?.response?.data?.message || e?.message || translate('generatedScript.detectionDetectionPackagesPackageEditor_ai_draft_generation_failed_0583ce')
    if (e?.response?.status === 503) {
      ElMessage.error(translate('generatedScript.detectionDetectionPackagesPackageEditor_ai_service_unavailable_please_check_llm_27cb12', { p0: msg }))
    } else {
      ElMessage.error(msg)
    }
  } finally {
    generating.value = false
  }
}

async function handleSave() {
  if (!form.title || !form.target_version) {
    ElMessage.warning(translate('generatedScript.detectionDetectionPackagesPackageEditor_please_fill_in_required_fields_81265f'))
    return
  }
  saving.value = true
  try {
    if (isEdit.value && currentDraft.value) {
      await updateDraft(currentDraft.value.id, form)
    } else {
      await createDraft(form)
    }
    router.push('/detection/packages')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  if (isEdit.value) {
    loadDraft()
  }
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

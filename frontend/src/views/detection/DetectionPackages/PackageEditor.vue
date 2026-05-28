<template>
  <div class="package-editor-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <div>
            <el-button link @click="$router.back()">
              <el-icon><ArrowLeft /></el-icon> 返回
            </el-button>
            <span style="margin-left: 12px; font-weight: bold;">{{ isEdit ? '编辑检测包' : '新建检测包' }}</span>
          </div>
          <div>
            <el-button @click="handleAIGenerate" :loading="generating">AI 生成草稿</el-button>
            <el-button type="primary" :loading="saving" @click="handleSave">保存草稿</el-button>
          </div>
        </div>
      </template>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="基础信息" name="basic">
          <el-form :model="form" label-width="120px" style="max-width: 600px;">
            <el-form-item v-if="isEdit" label="Package ID">
              <el-input v-model="form.package_id" disabled />
            </el-form-item>
            <el-form-item label="版本" required>
              <el-input v-model="form.target_version" placeholder="如 1.0.0" />
            </el-form-item>
            <el-form-item label="标题" required>
              <el-input v-model="form.title" placeholder="检测包标题" />
            </el-form-item>
            <el-form-item label="描述">
              <el-input v-model="form.description" type="textarea" :rows="3" />
            </el-form-item>
            <el-form-item label="CVE IDs">
              <el-select v-model="form.cve_ids" multiple filterable allow-create placeholder="输入 CVE ID">
              </el-select>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="HookPlan" name="hookplan">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            定义 eBPF hook 点和事件提取规则。只描述采集，不包含告警逻辑。
          </el-text>
          <CodeEditorPanel v-model="form.hook_plan_yaml" language="yaml" placeholder="HookPlan YAML" />
        </el-tab-pane>

        <el-tab-pane label="eBPF 源码" name="ebpf">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            eBPF C 源码草稿。只做事件采集和轻量过滤，不做复杂检测。
          </el-text>
          <CodeEditorPanel v-model="form.ebpf_source" language="c" placeholder="eBPF C 源码" />
        </el-tab-pane>

        <el-tab-pane label="Sigma 原子规则" name="sigma">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            单事件匹配规则。rule_id 使用 package_id.stable_name 格式。
          </el-text>
          <CodeEditorPanel v-model="form.sigma_rules_yaml" language="yaml" placeholder="Sigma YAML" />
        </el-tab-pane>

        <el-tab-pane label="Correlation" name="correlation">
          <el-text type="info" size="small" style="margin-bottom: 8px; display: block;">
            多事件关联规则。只支持 ordered sequence + window + by。
          </el-text>
          <CodeEditorPanel v-model="form.correlation_yaml" language="yaml" placeholder="Correlation YAML" />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog v-model="aiDialogVisible" title="AI 生成草稿" width="600">
      <el-form :model="aiForm" label-width="120px">
        <el-form-item label="CVE ID" required>
          <el-input v-model="aiForm.cve_id" placeholder="如 CVE-2026-31431" />
        </el-form-item>
        <el-form-item label="漏洞描述" required>
          <el-input v-model="aiForm.vulnerability_description" type="textarea" :rows="4" placeholder="描述漏洞原理和利用方式" />
        </el-form-item>
        <el-form-item label="攻击前置条件">
          <el-input v-model="aiForm.attack_prerequisites" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="利用链行为">
          <el-input v-model="aiForm.exploitation_chain" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="误报约束">
          <el-input v-model="aiForm.false_positive_constraints" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="aiDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="generating" @click="confirmAIGenerate">生成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
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
      if (currentPackage.value.status === 'draft') {
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
    ElMessage.warning('请填写 CVE ID 和漏洞描述')
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
    const msg = e?.response?.data?.message || e?.message || 'AI 草稿生成失败'
    if (e?.response?.status === 503) {
      ElMessage.error(`AI 服务不可用: ${msg}，请检查 LLM 配置`)
    } else {
      ElMessage.error(msg)
    }
  } finally {
    generating.value = false
  }
}

async function handleSave() {
  if (!form.title || !form.target_version) {
    ElMessage.warning('请填写必填字段')
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

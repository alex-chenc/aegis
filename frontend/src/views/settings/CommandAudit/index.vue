<template>
  <div class="command-audit-page">
    <h2 style="margin-bottom: 16px">{{ $t('generated.settingsCommandAuditIndex_command_audit_configuration_39214f') }}</h2>

    <AuditPolicyCard :settings="settings" :llm-available="llmAvailable" @update="handleSettingsUpdate" style="margin-bottom: 16px" />

    <RuleTable
      :rules="rules"
      :loading="loading"
      :total="total"
      @add="showAddDialog"
      @edit="showEditDialog"
      @delete="handleDelete"
      @toggle="handleToggle"
      @filter="handleFilter"
      style="margin-bottom: 16px"
    />

    <RuleFormDialog
      :visible="dialogVisible"
      :rule="editingRule"
      :submitting="formSubmitting"
      @close="dialogVisible = false"
      @submit="handleFormSubmit"
    />
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import AuditPolicyCard from './components/AuditPolicyCard.vue'
import RuleTable from './components/RuleTable.vue'
import RuleFormDialog from './components/RuleFormDialog.vue'
import { useCommandAudit } from './composables/useCommandAudit'
import { useConfigStore } from '@/store/config'
import type { CommandAuditRule, CreateRulePayload, CommandAuditSettings } from '@/api/command-audit'

const {
  rules, total, loading, settings,
  fetchRules, fetchSettings, createRule, updateRule,
  deleteRule, toggleRule, updateSettings
} = useCommandAudit()

const configStore = useConfigStore()
const llmAvailable = computed(() => configStore.llmConfig !== null)

const dialogVisible = ref(false)
const editingRule = ref<CommandAuditRule | null>(null)
const formSubmitting = ref(false)

onMounted(async () => {
  await Promise.all([fetchRules(), fetchSettings(), configStore.fetchLLMConfig()])
})

function showAddDialog() {
  editingRule.value = null
  dialogVisible.value = true
}

function showEditDialog(rule: CommandAuditRule) {
  editingRule.value = rule
  dialogVisible.value = true
}

async function handleFormSubmit(data: CreateRulePayload) {
  formSubmitting.value = true
  try {
    if (editingRule.value) {
      await updateRule(editingRule.value.id, data)
      ElMessage.success(translate('generatedScript.settingsCommandAuditIndex_rules_updated_fccd13'))
    } else {
      await createRule(data)
      ElMessage.success(translate('generatedScript.settingsCommandAuditIndex_rule_created_91d42e'))
    }
    dialogVisible.value = false
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsCommandAuditIndex_operation_failed_09e424'))
  } finally {
    formSubmitting.value = false
  }
}

async function handleDelete(id: string) {
  try {
    await ElMessageBox.confirm(translate('generatedScript.settingsCommandAuditIndex_are_you_sure_you_want_to_508b11'), translate('generatedScript.common_delete_confirmation_726b6e'), { type: 'warning' })
    await deleteRule(id)
    ElMessage.success(translate('generatedScript.settingsCommandAuditIndex_rule_deleted_91ba56'))
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message || translate('generatedScript.common_delete_failed_72250c'))
  }
}

async function handleToggle(id: string) {
  try {
    await toggleRule(id)
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsCommandAuditIndex_operation_failed_09e424'))
  }
}

async function handleFilter(params: Record<string, any>) {
  await fetchRules(params)
}

async function handleSettingsUpdate(data: Partial<CommandAuditSettings>) {
  try {
    await updateSettings(data)
    ElMessage.success(translate('generatedScript.settingsCommandAuditIndex_policy_updated_ed6a75'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_update_failed_8f8818'))
  }
}
</script>

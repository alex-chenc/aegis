<template>
  <div class="command-audit-page">
    <h2 style="margin-bottom: 16px">命令审计配置</h2>

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
      ElMessage.success('规则已更新')
    } else {
      await createRule(data)
      ElMessage.success('规则已创建')
    }
    dialogVisible.value = false
  } catch (e: any) {
    ElMessage.error(e.message || '操作失败')
  } finally {
    formSubmitting.value = false
  }
}

async function handleDelete(id: string) {
  try {
    await ElMessageBox.confirm('确定要删除该规则吗？', '删除确认', { type: 'warning' })
    await deleteRule(id)
    ElMessage.success('规则已删除')
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message || '删除失败')
  }
}

async function handleToggle(id: string) {
  try {
    await toggleRule(id)
  } catch (e: any) {
    ElMessage.error(e.message || '操作失败')
  }
}

async function handleFilter(params: Record<string, any>) {
  await fetchRules(params)
}

async function handleSettingsUpdate(data: Partial<CommandAuditSettings>) {
  try {
    await updateSettings(data)
    ElMessage.success('策略已更新')
  } catch (e: any) {
    ElMessage.error(e.message || '更新失败')
  }
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('agentGuard.policy.title')"
    width="920px"
    destroy-on-close
    @close="emit('close')"
  >
    <el-alert
      type="info"
      :title="t('agentGuard.policy.monitorOnlyNotice')"
      :closable="false"
      show-icon
    />

    <div class="policy-toolbar">
      <el-button type="primary" @click="showCreate = !showCreate">
        {{ t('agentGuard.policy.create') }}
      </el-button>
      <el-button :loading="store.loading.policies" @click="store.fetchPolicies()">
        {{ t('common.actions.refresh') }}
      </el-button>
    </div>

    <el-form v-if="showCreate" label-position="top" class="policy-form" @submit.prevent="createDraft">
      <div class="policy-form-grid">
        <el-form-item :label="t('agentGuard.policy.key')" required>
          <el-input v-model.trim="form.policyKey" maxlength="128" />
        </el-form-item>
        <el-form-item :label="t('agentGuard.policy.name')" required>
          <el-input v-model.trim="form.name" maxlength="255" />
        </el-form-item>
        <el-form-item :label="t('agentGuard.policy.hostIds')" required>
          <el-input
            v-model="form.hostIDs"
            type="textarea"
            :rows="2"
            :placeholder="t('agentGuard.policy.hostIdsHint')"
          />
        </el-form-item>
        <el-form-item :label="t('agentGuard.policy.agentTypes')" required>
          <el-select v-model="form.agentTypes" multiple>
            <el-option
              v-for="agentType in AGENT_GUARD_AGENT_TYPES"
              :key="agentType"
              :value="agentType"
              :label="t(`agentGuard.agentTypes.${agentType}`)"
            />
          </el-select>
        </el-form-item>
      </div>
      <el-form-item :label="t('agentGuard.policy.toolAdapter')">
        <el-checkbox v-model="form.toolAdapterEnabled">
          {{ t('agentGuard.policy.toolAdapterHint') }}
        </el-checkbox>
      </el-form-item>
      <el-form-item :label="t('agentGuard.policy.description')">
        <el-input v-model="form.description" type="textarea" :rows="2" maxlength="1000" />
      </el-form-item>
      <div class="policy-form-actions">
        <el-button type="primary" native-type="submit" :loading="store.loading.policies">
          {{ t('agentGuard.policy.createAndValidate') }}
        </el-button>
      </div>
    </el-form>

    <el-alert
      v-if="store.errors.policies"
      type="error"
      :title="store.errors.policies"
      :closable="false"
      show-icon
    />
    <el-alert
      v-if="store.policyValidation"
      :type="store.policyValidation.valid ? 'success' : 'error'"
      :title="store.policyValidation.valid
        ? t('agentGuard.policy.validationPassed')
        : t('agentGuard.policy.validationFailed')"
      :closable="false"
      show-icon
    >
      <ul v-if="store.policyValidation.errors.length" class="validation-errors">
        <li v-for="issue in store.policyValidation.errors" :key="`${issue.field}:${issue.code}`">
          {{ issue.field }}: {{ issue.message }}
        </li>
      </ul>
    </el-alert>

    <el-table
      :data="store.policies"
      :loading="store.loading.policies"
      row-key="id"
      class="policy-table"
      @row-click="loadDeliveries"
    >
      <el-table-column prop="name" :label="t('agentGuard.policy.name')" min-width="180" />
      <el-table-column prop="policy_key" :label="t('agentGuard.policy.key')" min-width="170" />
      <el-table-column prop="version" :label="t('agentGuard.policy.version')" width="90" />
      <el-table-column prop="status" :label="t('agentGuard.policy.status')" width="120" />
      <el-table-column :label="t('agentGuard.policy.actions')" width="190" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="row.status === 'draft'"
            size="small"
            type="primary"
            :loading="store.loading.policies"
            @click.stop="publish(row)"
          >
            {{ t('agentGuard.policy.publish') }}
          </el-button>
          <el-button size="small" @click.stop="loadDeliveries(row)">
            {{ t('agentGuard.policy.deliveries') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <section v-if="store.deliveries.length" class="delivery-section">
      <h3>{{ t('agentGuard.policy.deliveryTitle') }}</h3>
      <el-table :data="store.deliveries" row-key="id" size="small">
        <el-table-column prop="host_id" :label="t('agentGuard.policy.host')" min-width="230" />
        <el-table-column prop="bundle_version" :label="t('agentGuard.policy.bundleVersion')" width="150" />
        <el-table-column prop="status" :label="t('agentGuard.policy.status')" width="130" />
        <el-table-column prop="error_code" :label="t('agentGuard.policy.errorCode')" min-width="180" />
      </el-table>
    </section>

    <template #footer>
      <el-button @click="emit('close')">{{ t('common.actions.close') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useAgentGuardStore } from '@/store/agentGuard'
import type { AgentGuardPolicy, AgentGuardPolicyDraftRequest } from '@/types/agentGuard'
import { AGENT_GUARD_AGENT_TYPES } from '../agentGuardProfiles'
import { buildAgentGuardCollectionPolicy } from '../agentGuardPolicy'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const store = useAgentGuardStore()
const showCreate = ref(false)
const form = reactive({
  policyKey: '',
  name: '',
  description: '',
  hostIDs: '',
  agentTypes: ['codex', 'openclaw', 'hermes'],
  toolAdapterEnabled: false,
})

watch(() => props.visible, visible => {
  if (visible) void store.fetchPolicies()
})

function buildPayload(): AgentGuardPolicyDraftRequest {
  return {
    policy_key: form.policyKey,
    name: form.name,
    description: form.description,
    priority: 100,
    targets: {
      host_ids: form.hostIDs.split(/[\s,]+/).map(value => value.trim()).filter(Boolean),
      host_group_ids: [],
      agent_types: [...form.agentTypes],
    },
    collection: buildAgentGuardCollectionPolicy(form.toolAdapterEnabled),
    builtin_rule_overrides: [],
    atomic_rules: [],
    correlation_rules: [],
    analysis: {
      enabled: false,
      trigger_severities: ['high', 'critical'],
      ai_only_action_ceiling: 'alert',
      evidence_window_seconds: 300,
    },
    escape_rules: [],
    freeze_timeout_seconds: 300,
  }
}

async function createDraft() {
  if (!form.policyKey || !form.name || !form.agentTypes.length || !form.hostIDs.trim()) {
    ElMessage.warning(t('agentGuard.policy.requiredFields'))
    return
  }
  const policy = await store.createPolicy(buildPayload())
  showCreate.value = false
  ElMessage.success(t('agentGuard.policy.created', { version: policy.version }))
}

async function publish(policy: AgentGuardPolicy) {
  await ElMessageBox.confirm(
    t('agentGuard.policy.publishConfirm'),
    t('agentGuard.policy.publish'),
    { type: 'warning', confirmButtonText: t('agentGuard.policy.publish') },
  )
  await store.publishPolicy(policy.id, 'monitor-only rollout approved from Agent Guard UI')
  ElMessage.success(t('agentGuard.policy.publishAccepted'))
}

async function loadDeliveries(policy: AgentGuardPolicy) {
  await store.fetchPolicyDeliveries(policy.id)
}
</script>

<style scoped>
.policy-toolbar,
.policy-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin: 16px 0;
}

.policy-form {
  padding: 16px;
  margin-bottom: 16px;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  background: #f8fbff;
}

.policy-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 18px;
}

.policy-table,
.delivery-section {
  margin-top: 16px;
}

.validation-errors {
  margin: 8px 0 0;
  padding-left: 20px;
}
</style>

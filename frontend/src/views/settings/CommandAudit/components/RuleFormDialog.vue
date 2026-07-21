<template>
  <el-dialog :model-value="visible" :title="isEdit ? $t('dynamic.editRule') : $t('dynamic.addRule')" width="600px" @close="$emit('close')">
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item :label="$t('generated.common_rule_name_1937bc')" prop="name">
        <el-input v-model="form.name" :placeholder="$t('generated.settingsCommandAuditRuleFormDialog_enter_rule_name_71c37e')" />
      </el-form-item>
      <el-form-item :label="$t('generated.common_describe_412f54')" prop="description">
        <el-input v-model="form.description" type="textarea" :rows="2" :placeholder="$t('generated.settingsCommandAuditRuleFormDialog_enter_a_rule_description_489c1d')" />
      </el-form-item>
      <el-form-item :label="$t('generated.settingsCommandAuditRuleFormDialog_rule_type_7655f4')" prop="rule_type">
        <el-radio-group v-model="form.rule_type">
          <el-radio value="hard_block">{{ $t('generated.settingsCommandAuditRuleFormDialog_hard_interception_20a8eb') }}</el-radio>
          <el-radio value="soft_warn">{{ $t('generated.settingsCommandAuditRuleFormDialog_soft_alarm_9986c5') }}</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item :label="$t('generated.common_match_type_575d0d')" prop="match_type">
        <el-radio-group v-model="form.match_type">
          <el-radio value="regex">{{ $t('generated.settingsCommandAuditRuleFormDialog_regular_expression_46e5b8') }}</el-radio>
          <el-radio value="exact">{{ $t('generated.settingsCommandAuditRuleFormDialog_exact_match_99c60b') }}</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item :label="$t('generated.common_match_pattern_a48291')" prop="pattern">
        <el-input v-model="form.pattern" type="textarea" :rows="3" :placeholder="$t('generated.settingsCommandAuditRuleFormDialog_enter_matching_pattern_a1b99c')" />
      </el-form-item>
      <el-form-item :label="$t('generated.common_classification_435c52')" prop="category">
        <el-select v-model="form.category" style="width: 100%">
          <el-option :label="$t('generated.common_file_system_42949b')" value="filesystem" />
          <el-option :label="$t('generated.common_permissions_560165')" value="permission" />
          <el-option :label="$t('generated.common_network_0cbda6')" value="network" />
          <el-option :label="$t('generated.common_system_1a1f6d')" value="system" />
          <el-option :label="$t('generated.common_privilege_escalation_b6f22d')" value="privilege" />
        </el-select>
      </el-form-item>
      <el-form-item :label="$t('generated.common_severity_level_a0681b')" prop="severity">
        <el-select v-model="form.severity" style="width: 100%">
          <el-option :label="$t('generated.common_serious_81ffc6')" value="critical" />
          <el-option :label="$t('generated.common_high_risk_e62ee8')" value="high" />
          <el-option :label="$t('generated.common_medium_risk_1098e6')" value="medium" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('close')">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
      <el-button type="primary" @click="handleSubmit" :loading="submitting">{{ $t('generated.common_sure_f526c8') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { ref, reactive, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import type { CommandAuditRule, CreateRulePayload } from '@/api/command-audit'

const props = defineProps<{
  visible: boolean
  rule: CommandAuditRule | null
  submitting?: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit', data: CreateRulePayload): void
}>()

const formRef = ref<FormInstance>()
const isEdit = ref(false)

const form = reactive<CreateRulePayload>({
  name: '',
  description: '',
  rule_type: 'hard_block',
  match_type: 'regex',
  pattern: '',
  category: 'system',
  severity: 'high',
  applies_to: ['all']
})

const rules: FormRules = {
  name: [{ required: true, message: translate('generatedScript.settingsCommandAuditRuleFormDialog_please_enter_a_rule_name_d4d9c4'), trigger: 'blur' }],
  pattern: [{ required: true, message: translate('generatedScript.settingsCommandAuditRuleFormDialog_please_enter_a_matching_pattern_9da15d'), trigger: 'blur' }],
  category: [{ required: true, message: translate('generatedScript.settingsCommandAuditRuleFormDialog_please_select_a_category_840376'), trigger: 'change' }],
  severity: [{ required: true, message: translate('generatedScript.settingsCommandAuditRuleFormDialog_please_select_severity_level_b81c2d'), trigger: 'change' }]
}

watch(() => props.visible, (val) => {
  if (val && props.rule) {
    isEdit.value = true
    Object.assign(form, {
      name: props.rule.name,
      description: props.rule.description,
      rule_type: props.rule.rule_type,
      match_type: props.rule.match_type,
      pattern: props.rule.pattern,
      category: props.rule.category,
      severity: props.rule.severity,
      applies_to: props.rule.applies_to
    })
  } else if (val) {
    isEdit.value = false
    Object.assign(form, { name: '', description: '', rule_type: 'hard_block', match_type: 'regex', pattern: '', category: 'system', severity: 'high', applies_to: ['all'] })
  }
})

async function handleSubmit() {
  if (!formRef.value) return
  await formRef.value.validate((valid) => {
    if (valid) emit('submit', { ...form })
  })
}
</script>

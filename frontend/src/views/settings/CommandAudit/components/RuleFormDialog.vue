<template>
  <el-dialog :model-value="visible" :title="isEdit ? '编辑规则' : '新增规则'" width="600px" @close="$emit('close')">
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="规则名称" prop="name">
        <el-input v-model="form.name" placeholder="输入规则名称" />
      </el-form-item>
      <el-form-item label="描述" prop="description">
        <el-input v-model="form.description" type="textarea" :rows="2" placeholder="输入规则描述" />
      </el-form-item>
      <el-form-item label="规则类型" prop="rule_type">
        <el-radio-group v-model="form.rule_type">
          <el-radio value="hard_block">硬拦截</el-radio>
          <el-radio value="soft_warn">软告警</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="匹配类型" prop="match_type">
        <el-radio-group v-model="form.match_type">
          <el-radio value="regex">正则表达式</el-radio>
          <el-radio value="exact">精确匹配</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="匹配模式" prop="pattern">
        <el-input v-model="form.pattern" type="textarea" :rows="3" placeholder="输入匹配模式" />
      </el-form-item>
      <el-form-item label="分类" prop="category">
        <el-select v-model="form.category" style="width: 100%">
          <el-option label="文件系统" value="filesystem" />
          <el-option label="权限" value="permission" />
          <el-option label="网络" value="network" />
          <el-option label="系统" value="system" />
          <el-option label="权限提升" value="privilege" />
        </el-select>
      </el-form-item>
      <el-form-item label="严重等级" prop="severity">
        <el-select v-model="form.severity" style="width: 100%">
          <el-option label="严重" value="critical" />
          <el-option label="高危" value="high" />
          <el-option label="中危" value="medium" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('close')">取消</el-button>
      <el-button type="primary" @click="handleSubmit" :loading="submitting">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
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
  name: [{ required: true, message: '请输入规则名称', trigger: 'blur' }],
  pattern: [{ required: true, message: '请输入匹配模式', trigger: 'blur' }],
  category: [{ required: true, message: '请选择分类', trigger: 'change' }],
  severity: [{ required: true, message: '请选择严重等级', trigger: 'change' }]
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

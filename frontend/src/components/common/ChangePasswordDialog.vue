<template>
  <el-dialog :model-value="visible" :title="t('auth.changePassword.title')" width="440px" append-to-body @close="$emit('close')">
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item :label="t('auth.changePassword.current')" prop="current_password">
        <el-input v-model="form.current_password" type="password" show-password :placeholder="t('auth.changePassword.currentPlaceholder')" />
      </el-form-item>
      <el-form-item :label="t('auth.changePassword.next')" prop="new_password">
        <el-input v-model="form.new_password" type="password" show-password :placeholder="t('auth.changePassword.nextPlaceholder')" />
      </el-form-item>
      <el-form-item :label="t('auth.changePassword.confirm')" prop="confirm_password">
        <el-input v-model="form.confirm_password" type="password" show-password :placeholder="t('auth.changePassword.confirmPlaceholder')" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('close')">{{ t('common.actions.cancel') }}</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">{{ t('common.actions.confirm') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { changePassword } from '@/api/auth'
import { saveAuthSession } from '@/utils/auth'

defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const formRef = ref<FormInstance>()
const { t } = useI18n()
const submitting = ref(false)

const form = reactive({
  current_password: '',
  new_password: '',
  confirm_password: ''
})

const rules = computed<FormRules>(() => ({
  current_password: [{ required: true, message: t('auth.changePassword.currentRequired'), trigger: 'blur' }],
  new_password: [
    { required: true, message: t('auth.changePassword.nextRequired'), trigger: 'blur' },
    { min: 8, message: t('auth.changePassword.nextMin'), trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (!/[a-zA-Z]/.test(value) || !/[0-9]/.test(value)) {
          callback(new Error(t('auth.changePassword.composition')))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ],
  confirm_password: [
    { required: true, message: t('auth.changePassword.confirmRequired'), trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (value !== form.new_password) {
          callback(new Error(t('auth.changePassword.mismatch')))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ]
}))

watch(() => form.new_password, () => {
  if (form.confirm_password) {
    formRef.value?.validateField('confirm_password')
  }
})

async function handleSubmit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const session = await changePassword({
      current_password: form.current_password,
      new_password: form.new_password,
      confirm_password: form.confirm_password
    })
    saveAuthSession(session)
    ElMessage.success(t('auth.changePassword.success'))
    form.current_password = ''
    form.new_password = ''
    form.confirm_password = ''
    emit('close')
    emit('success')
  } catch (err: any) {
    const msg = err?.response?.data?.message || t('auth.changePassword.failed')
    ElMessage.error(msg)
  } finally {
    submitting.value = false
  }
}
</script>

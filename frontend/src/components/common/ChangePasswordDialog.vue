<template>
  <el-dialog :model-value="visible" title="修改密码" width="440px" append-to-body @close="$emit('close')">
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="当前密码" prop="current_password">
        <el-input v-model="form.current_password" type="password" show-password placeholder="输入当前密码" />
      </el-form-item>
      <el-form-item label="新密码" prop="new_password">
        <el-input v-model="form.new_password" type="password" show-password placeholder="输入新密码（至少8位）" />
      </el-form-item>
      <el-form-item label="确认密码" prop="confirm_password">
        <el-input v-model="form.confirm_password" type="password" show-password placeholder="再次输入新密码" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('close')">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
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
const submitting = ref(false)

const form = reactive({
  current_password: '',
  new_password: '',
  confirm_password: ''
})

const rules: FormRules = {
  current_password: [{ required: true, message: '请输入当前密码', trigger: 'blur' }],
  new_password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, message: '密码长度至少8位', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (!/[a-zA-Z]/.test(value) || !/[0-9]/.test(value)) {
          callback(new Error('密码必须包含字母和数字'))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ],
  confirm_password: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (value !== form.new_password) {
          callback(new Error('两次输入的密码不一致'))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ]
}

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
    ElMessage.success('密码修改成功')
    form.current_password = ''
    form.new_password = ''
    form.confirm_password = ''
    emit('close')
    emit('success')
  } catch (err: any) {
    const msg = err?.response?.data?.message || '密码修改失败'
    ElMessage.error(msg)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="change-page">
    <section class="change-panel" aria-label="设置管理员账号密码">
      <div class="panel-heading">
        <span class="eyebrow">Required Step</span>
        <h1>设置管理员凭据</h1>
        <p>首次进入后必须完成账号密码设置，完成前无法访问控制台业务页面。</p>
      </div>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        class="change-form"
        @submit.prevent="handleSubmit"
      >
        <el-form-item label="管理员账号" prop="username">
          <el-input v-model.trim="form.username" autocomplete="username" size="large" />
        </el-form-item>
        <el-form-item label="新密码" prop="newPassword">
          <el-input
            v-model="form.newPassword"
            type="password"
            autocomplete="new-password"
            size="large"
            show-password
          />
        </el-form-item>
        <el-form-item label="确认密码" prop="confirmPassword">
          <el-input
            v-model="form.confirmPassword"
            type="password"
            autocomplete="new-password"
            size="large"
            show-password
          />
        </el-form-item>
        <el-button type="primary" size="large" class="submit-button" :loading="submitting" @click="handleSubmit">
          保存并进入控制台
        </el-button>
      </el-form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { changeCredentials } from '@/api/auth'
import { saveAuthSession } from '@/utils/auth'

const formRef = ref<FormInstance>()
const submitting = ref(false)
const form = reactive({
  username: 'admin',
  newPassword: '',
  confirmPassword: ''
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入管理员账号', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, message: '密码至少 8 位', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    {
      validator: (_, value, callback) => {
        if (value !== form.newPassword) {
          callback(new Error('两次输入的密码不一致'))
          return
        }
        callback()
      },
      trigger: 'blur'
    }
  ]
}

async function handleSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const session = await changeCredentials({
      username: form.username,
      new_password: form.newPassword,
      confirm_password: form.confirmPassword
    })
    saveAuthSession(session)
    ElMessage.success('账号密码已设置')
    window.location.assign('/hosts')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.change-page {
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background:
    linear-gradient(135deg, rgba(15, 23, 42, 0.94), rgba(30, 41, 59, 0.9)),
    #f8fafc;
}

.change-panel {
  width: min(460px, 100%);
  border-radius: 8px;
  background: #ffffff;
  border: 1px solid #dbe4ee;
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.22);
  padding: 32px;
}

.panel-heading {
  margin-bottom: 24px;
}

.eyebrow {
  color: #0891b2;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}

.panel-heading h1 {
  margin: 8px 0;
  color: #0f172a;
  font-size: 24px;
  line-height: 1.25;
  letter-spacing: 0;
}

.panel-heading p {
  margin: 0;
  color: #475569;
  line-height: 1.6;
}

.change-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.submit-button {
  width: 100%;
  min-height: 44px;
  margin-top: 4px;
}
</style>

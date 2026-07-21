<template>
  <main class="change-page">
    <section class="change-panel" :aria-label="t('auth.credentials.panelAria')">
      <div class="panel-heading">
        <span class="eyebrow">Required Step</span>
        <h1>{{ t('auth.credentials.title') }}</h1>
        <p>{{ t('auth.credentials.detailedSubtitle') }}</p>
      </div>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        class="change-form"
        @submit.prevent="handleSubmit"
      >
        <el-form-item :label="t('auth.credentials.username')" prop="username">
          <el-input v-model.trim="form.username" autocomplete="username" size="large" />
        </el-form-item>
        <el-form-item :label="t('auth.credentials.password')" prop="newPassword">
          <el-input
            v-model="form.newPassword"
            type="password"
            autocomplete="new-password"
            size="large"
            show-password
          />
        </el-form-item>
        <el-form-item :label="t('auth.credentials.confirmPassword')" prop="confirmPassword">
          <el-input
            v-model="form.confirmPassword"
            type="password"
            autocomplete="new-password"
            size="large"
            show-password
          />
        </el-form-item>
        <el-button type="primary" size="large" class="submit-button" :loading="submitting" @click="handleSubmit">
          {{ t('auth.credentials.submit') }}
        </el-button>
      </el-form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { changeCredentials } from '@/api/auth'
import { saveAuthSession } from '@/utils/auth'

const formRef = ref<FormInstance>()
const { t } = useI18n()
const router = useRouter()
const submitting = ref(false)
const form = reactive({
  username: 'admin',
  newPassword: '',
  confirmPassword: ''
})

const rules = computed<FormRules>(() => ({
  username: [{ required: true, message: t('auth.credentials.usernameRequired'), trigger: 'blur' }],
  newPassword: [
    { required: true, message: t('auth.credentials.passwordRequired'), trigger: 'blur' },
    { min: 8, message: t('auth.credentials.passwordMin'), trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: t('auth.credentials.confirmRequired'), trigger: 'blur' },
    {
      validator: (_, value, callback) => {
        if (value !== form.newPassword) {
          callback(new Error(t('auth.credentials.mismatch')))
          return
        }
        callback()
      },
      trigger: 'blur'
    }
  ]
}))

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
    ElMessage.success(t('auth.credentials.success'))
    await router.replace('/hosts')
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

<template>
  <main class="auth-page">
    <section class="auth-brand" aria-label="Aegis 控制台">
      <div class="brand-lock">
        <span class="brand-shield" aria-hidden="true"></span>
      </div>
      <h1>Aegis</h1>
      <p>主机安全指挥台</p>
      <div class="status-line">
        <span class="status-dot" />
        控制面认证保护已启用
      </div>
    </section>

    <section class="auth-panel" aria-label="登录认证">
      <div class="panel-heading">
        <span class="eyebrow">Authentication</span>
        <h2>{{ initialized ? '账号密码登录' : '首次进入控制台' }}</h2>
        <p>{{ initialized ? '请输入管理员账号和密码。' : '首次部署无需账号密码，进入后必须立即设置管理员凭据。' }}</p>
      </div>

      <el-skeleton v-if="loadingStatus" :rows="5" animated />

      <el-form
        v-else-if="initialized"
        ref="loginFormRef"
        :model="loginForm"
        :rules="loginRules"
        label-position="top"
        class="auth-form"
        @submit.prevent="handleLogin"
      >
        <el-form-item label="账号" prop="username">
          <el-input v-model.trim="loginForm.username" autocomplete="username" size="large" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="loginForm.password"
            type="password"
            autocomplete="current-password"
            size="large"
            show-password
          />
        </el-form-item>
        <el-button type="primary" size="large" class="submit-button" :loading="submitting" @click="handleLogin">
          登录
        </el-button>
      </el-form>

      <div v-else class="bootstrap-actions">
        <el-alert
          title="首次进入后只能访问账号密码设置页面。"
          type="warning"
          :closable="false"
          show-icon
        />
        <el-button type="primary" size="large" class="submit-button" :loading="submitting" @click="handleBootstrap">
          首次进入控制台
        </el-button>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { bootstrapLogin, getAuthStatus, login } from '@/api/auth'
import { saveAuthSession } from '@/utils/auth'

const router = useRouter()
const loadingStatus = ref(true)
const submitting = ref(false)
const initialized = ref(false)
const loginFormRef = ref<FormInstance>()
const loginForm = reactive({
  username: '',
  password: ''
})

const loginRules: FormRules = {
  username: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

onMounted(async () => {
  try {
    const status = await getAuthStatus()
    initialized.value = status.initialized
  } finally {
    loadingStatus.value = false
  }
})

async function handleBootstrap() {
  submitting.value = true
  try {
    const session = await bootstrapLogin()
    saveAuthSession(session)
    await router.replace('/force-password-change')
  } finally {
    submitting.value = false
  }
}

async function handleLogin() {
  if (!loginFormRef.value) return
  const valid = await loginFormRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const session = await login(loginForm.username, loginForm.password)
    saveAuthSession(session)
    if (session.force_password_change) {
      await router.replace('/force-password-change')
    } else {
      ElMessage.success('登录成功')
      await router.replace('/hosts')
    }
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.auth-page {
  min-height: 100dvh;
  display: grid;
  grid-template-columns: minmax(280px, 0.9fr) minmax(360px, 1fr);
  background:
    linear-gradient(135deg, rgba(11, 18, 32, 0.96), rgba(15, 23, 42, 0.94)),
    linear-gradient(135deg, #e6f6ff, #f8fafc);
  color: #0f172a;
}

.auth-brand {
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 56px;
  color: #f8fafc;
}

.brand-lock {
  width: 54px;
  height: 54px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #67e8f9;
  border: 1px solid rgba(103, 232, 249, 0.42);
  background: rgba(8, 145, 178, 0.2);
  margin-bottom: 24px;
}

.brand-shield {
  width: 28px;
  height: 32px;
  display: block;
  background-image: url('/aegis-brand-source.png');
  background-repeat: no-repeat;
  background-size: 2190px 919px;
  background-position: -26px -32px;
}

.auth-brand h1 {
  margin: 0;
  font-size: 44px;
  line-height: 1.1;
  letter-spacing: 0;
}

.auth-brand p {
  margin: 14px 0 28px;
  color: rgba(226, 232, 240, 0.78);
  font-size: 18px;
}

.status-line {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  color: #cbd5e1;
  font-size: 14px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #22c55e;
}

.auth-panel {
  align-self: center;
  justify-self: center;
  width: min(420px, calc(100vw - 32px));
  border-radius: 8px;
  background: #ffffff;
  border: 1px solid #dbe4ee;
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.18);
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

.panel-heading h2 {
  margin: 8px 0;
  font-size: 24px;
  line-height: 1.25;
  letter-spacing: 0;
  color: #0f172a;
}

.panel-heading p {
  margin: 0;
  color: #475569;
  line-height: 1.6;
}

.auth-form,
.bootstrap-actions {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.submit-button {
  width: 100%;
  min-height: 44px;
  margin-top: 4px;
}

@media (max-width: 860px) {
  .auth-page {
    grid-template-columns: 1fr;
    padding: 24px 0;
  }

  .auth-brand {
    padding: 32px 24px 20px;
  }

  .auth-brand h1 {
    font-size: 34px;
  }

  .auth-panel {
    align-self: start;
  }
}
</style>

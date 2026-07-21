<template>
  <main class="login-page" @mousemove="handlePointerMove">
    <canvas ref="particleCanvas" class="particle-canvas" aria-hidden="true"></canvas>
    <div class="aurora aurora-top" aria-hidden="true"></div>
    <div class="aurora aurora-bottom" aria-hidden="true"></div>
    <div class="grid-floor" aria-hidden="true"></div>
    <div class="corner-line corner-line-left" aria-hidden="true"></div>
    <div class="corner-line corner-line-right" aria-hidden="true"></div>

    <section class="brand-panel" :aria-label="t('auth.login.brandAria')">
      <div class="brand-header">
        <div class="logo-mark" aria-hidden="true">
          <svg viewBox="0 0 48 56" focusable="false">
            <path d="M24 3 43 11v15c0 13.2-7.7 23.3-19 27-11.3-3.7-19-13.8-19-27V11L24 3Z" />
            <path d="M24 11 35 16v10c0 8.1-4.3 14.6-11 17.5C17.3 40.6 13 34.1 13 26V16l11-5Z" />
          </svg>
        </div>
        <div>
          <strong>Aegis</strong>
          <span>{{ t('app.brand.subtitle') }}</span>
        </div>
      </div>

      <div class="brand-content">
        <p class="kicker">AI-NATIVE HOST SECURITY</p>
        <h1>{{ t('auth.login.heroTitle') }} · <span>{{ t('auth.login.heroAccent') }}</span></h1>
        <p class="brand-subtitle">{{ t('auth.login.slogan') }}</p>
      </div>

      <div class="hologram-wrap" aria-hidden="true">
        <div class="orbit orbit-outer"></div>
        <div class="orbit orbit-inner"></div>
        <div class="shield-core">
          <svg class="shield-svg" viewBox="0 0 180 210" focusable="false">
            <defs>
              <linearGradient id="shieldGradient" x1="20%" y1="0%" x2="80%" y2="100%">
                <stop offset="0%" stop-color="#7ACBFF" stop-opacity="0.96" />
                <stop offset="45%" stop-color="#1677FF" stop-opacity="0.72" />
                <stop offset="100%" stop-color="#00D4FF" stop-opacity="0.82" />
              </linearGradient>
            </defs>
            <path
              class="shield-shell"
              d="M90 8 158 35v52c0 52-27.5 91.6-68 109-40.5-17.4-68-57-68-109V35L90 8Z"
            />
            <path
              class="shield-panel"
              d="M90 30 134 48v38c0 34.5-16.5 61.8-44 76.5C62.5 147.8 46 120.5 46 86V48l44-18Z"
            />
            <path
              class="shield-check"
              d="m69 96 15 15 34-42"
            />
          </svg>
        </div>
        <div class="scan-platform">
          <span></span>
          <span></span>
        </div>
      </div>

      <div class="feature-list" :aria-label="t('auth.login.featureAria')">
        <article v-for="feature in features" :key="feature.title" class="feature-item">
          <span class="feature-icon" aria-hidden="true" v-html="feature.icon"></span>
          <span>
            <strong>{{ feature.title }}</strong>
            <em>{{ feature.description }}</em>
          </span>
        </article>
      </div>
    </section>

    <section class="login-shell" :aria-label="t('auth.login.panelAria')">
      <div class="auth-card">
        <div class="panel-heading">
          <span class="eyebrow">AUTHENTICATION</span>
          <h2>{{ initialized ? t('auth.login.titleInitialized') : t('auth.login.titleFirstUse') }}</h2>
          <p>{{ initialized ? t('auth.login.hintInitialized') : t('auth.login.hintFirstUse') }}</p>
        </div>

        <el-skeleton v-if="loadingStatus" :rows="5" animated class="auth-skeleton" />

        <el-form
          v-else-if="initialized"
          ref="loginFormRef"
          :model="loginForm"
          :rules="loginRules"
          label-position="top"
          class="auth-form"
          @submit.prevent="handleLogin"
        >
          <el-form-item :label="t('auth.login.username')" prop="username">
            <el-input
              v-model.trim="loginForm.username"
              autocomplete="username"
              class="aegis-input"
              :placeholder="t('auth.login.usernamePlaceholder')"
              size="large"
            >
              <template #prefix>
                <span class="input-icon" aria-hidden="true" v-html="icons.user"></span>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item :label="t('auth.login.password')" prop="password">
            <el-input
              v-model="loginForm.password"
              type="password"
              autocomplete="current-password"
              class="aegis-input"
              :placeholder="t('auth.login.passwordPlaceholder')"
              size="large"
              show-password
            >
              <template #prefix>
                <span class="input-icon" aria-hidden="true" v-html="icons.lock"></span>
              </template>
            </el-input>
          </el-form-item>

          <div class="form-row">
            <label class="remember-option">
              <input v-model="rememberMe" type="checkbox" />
              <span>{{ t('auth.login.rememberUsername') }}</span>
            </label>
            <button type="button" class="link-button">{{ t('auth.login.forgotPassword') }}</button>
          </div>

          <el-button
            type="primary"
            native-type="submit"
            size="large"
            class="submit-button"
            :loading="submitting"
            :disabled="submitting"
          >
            {{ t('auth.login.signIn') }}
          </el-button>
        </el-form>

        <div v-else class="bootstrap-actions">
          <el-alert
            :title="t('auth.login.bootstrapWarning')"
            type="warning"
            :closable="false"
            show-icon
          />
          <el-button
            type="primary"
            size="large"
            class="submit-button"
            :loading="submitting"
            :disabled="submitting"
            @click="handleBootstrap"
          >
            {{ t('auth.login.titleFirstUse') }}
          </el-button>
        </div>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { bootstrapLogin, getAuthStatus, login } from '@/api/auth'
import { saveAuthSession } from '@/utils/auth'

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  size: number
  alpha: number
}

const router = useRouter()
const { t } = useI18n()
const loadingStatus = ref(true)
const submitting = ref(false)
const initialized = ref(false)
const rememberMe = ref(false)
const loginFormRef = ref<FormInstance>()
const particleCanvas = ref<HTMLCanvasElement>()
const pointer = reactive({ x: 0, y: 0 })
const loginForm = reactive({
  username: '',
  password: ''
})

let animationFrame = 0
let removeResizeListener: (() => void) | undefined

const icons = {
  shield: '<svg viewBox="0 0 24 24"><path d="M12 3 20 6.5V12c0 5-3.2 8.6-8 10-4.8-1.4-8-5-8-10V6.5L12 3Z"/><path d="m9 12 2 2 4-5"/></svg>',
  zap: '<svg viewBox="0 0 24 24"><path d="M13 2 4 14h7l-1 8 10-13h-7l1-7Z"/></svg>',
  network: '<svg viewBox="0 0 24 24"><circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><path d="M10.5 7.6 6.5 16M13.5 7.6 17.5 16M8 19h8"/></svg>',
  user: '<svg viewBox="0 0 24 24"><circle cx="12" cy="8" r="4"/><path d="M5 21c1.4-4 4-6 7-6s5.6 2 7 6"/></svg>',
  lock: '<svg viewBox="0 0 24 24"><rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg>'
}

const features = computed(() => [
  {
    title: t('auth.login.realtimeProtection'),
    description: t('auth.login.featureRealtimeDescription'),
    icon: icons.shield
  },
  {
    title: t('auth.login.intelligentDetection'),
    description: t('auth.login.featureDetectionDescription'),
    icon: icons.zap
  },
  {
    title: t('auth.login.response'),
    description: t('auth.login.featureResponseDescription'),
    icon: icons.network
  }
])

const loginRules = computed<FormRules>(() => ({
  username: [{ required: true, message: t('auth.login.usernamePlaceholder'), trigger: 'blur' }],
  password: [{ required: true, message: t('auth.login.passwordPlaceholder'), trigger: 'blur' }]
}))

onMounted(async () => {
  restoreRememberedAccount()
  await nextTick()
  startParticleField()

  try {
    const status = await getAuthStatus()
    initialized.value = status.initialized
  } finally {
    loadingStatus.value = false
  }
})

onBeforeUnmount(() => {
  if (animationFrame) {
    cancelAnimationFrame(animationFrame)
  }
  removeResizeListener?.()
})

function handlePointerMove(event: MouseEvent) {
  pointer.x = event.clientX
  pointer.y = event.clientY
}

function restoreRememberedAccount() {
  try {
    const rememberedUsername = window.localStorage.getItem('aegis_login_username')
    if (rememberedUsername) {
      loginForm.username = rememberedUsername
      rememberMe.value = true
    }
  } catch {
    // Local storage can be unavailable in restricted browser contexts.
  }
}

function persistRememberedAccount() {
  try {
    if (rememberMe.value) {
      window.localStorage.setItem('aegis_login_username', loginForm.username)
    } else {
      window.localStorage.removeItem('aegis_login_username')
    }
  } catch {
    // Login should not fail if remember-me storage is blocked.
  }
}

function startParticleField() {
  const canvas = particleCanvas.value
  if (!canvas || typeof window === 'undefined') return
  if (import.meta.env.MODE === 'test') return

  const prefersReducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  if (prefersReducedMotion) return

  let ctx: CanvasRenderingContext2D | null = null
  try {
    ctx = canvas.getContext('2d')
  } catch {
    return
  }
  if (!ctx) return

  const particles: Particle[] = []
  let width = 0
  let height = 0
  let dpr = 1

  const resetParticle = (particle: Particle) => {
    particle.x = Math.random() * width
    particle.y = Math.random() * height
    particle.vx = (Math.random() - 0.5) * 0.28
    particle.vy = (Math.random() - 0.5) * 0.22
    particle.size = 1 + Math.random() * 1.6
    particle.alpha = 0.18 + Math.random() * 0.34
  }

  const resize = () => {
    dpr = Math.min(window.devicePixelRatio || 1, 2)
    width = window.innerWidth
    height = window.innerHeight
    canvas.width = Math.floor(width * dpr)
    canvas.height = Math.floor(height * dpr)
    canvas.style.width = `${width}px`
    canvas.style.height = `${height}px`
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

    const targetCount = Math.min(82, Math.max(36, Math.round(width / 22)))
    while (particles.length < targetCount) {
      const particle: Particle = { x: 0, y: 0, vx: 0, vy: 0, size: 1, alpha: 0.2 }
      resetParticle(particle)
      particles.push(particle)
    }
    particles.length = targetCount
  }

  const draw = () => {
    ctx.clearRect(0, 0, width, height)

    for (const particle of particles) {
      particle.x += particle.vx + (pointer.x - width / 2) * 0.000018
      particle.y += particle.vy + (pointer.y - height / 2) * 0.000012

      if (particle.x < -20 || particle.x > width + 20 || particle.y < -20 || particle.y > height + 20) {
        resetParticle(particle)
      }

      ctx.beginPath()
      ctx.fillStyle = `rgba(77, 163, 255, ${particle.alpha})`
      ctx.arc(particle.x, particle.y, particle.size, 0, Math.PI * 2)
      ctx.fill()
    }

    for (let index = 0; index < particles.length; index += 1) {
      const current = particles[index]
      for (let nextIndex = index + 1; nextIndex < particles.length; nextIndex += 1) {
        const next = particles[nextIndex]
        const distance = Math.hypot(current.x - next.x, current.y - next.y)
        if (distance < 118) {
          ctx.strokeStyle = `rgba(0, 212, 255, ${0.11 * (1 - distance / 118)})`
          ctx.lineWidth = 1
          ctx.beginPath()
          ctx.moveTo(current.x, current.y)
          ctx.lineTo(next.x, next.y)
          ctx.stroke()
        }
      }
    }

    animationFrame = requestAnimationFrame(draw)
  }

  resize()
  window.addEventListener('resize', resize)
  removeResizeListener = () => window.removeEventListener('resize', resize)
  draw()
}

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
  if (submitting.value) return
  if (!loginFormRef.value) return
  const valid = await loginFormRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const session = await login(loginForm.username, loginForm.password)
    persistRememberedAccount()
    saveAuthSession(session)
    if (session.force_password_change) {
      await router.replace('/force-password-change')
    } else {
      ElMessage.success(t('auth.login.success'))
      window.location.assign('/hosts')
    }
  } catch (err: any) {
    const msg = err?.response?.data?.message || t('auth.login.loginFailed')
    ElMessage.error(msg)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.login-page {
  position: relative;
  min-height: 100dvh;
  display: grid;
  grid-template-columns: minmax(620px, 1.28fr) minmax(430px, 0.72fr);
  overflow: hidden;
  background:
    radial-gradient(circle at 18% 8%, rgba(22, 119, 255, 0.34), transparent 28%),
    radial-gradient(circle at 82% 92%, rgba(0, 212, 255, 0.2), transparent 30%),
    linear-gradient(135deg, #050b18 0%, #07152a 48%, #0b1e3a 100%);
  color: #ffffff;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}

.login-page::before,
.login-page::after {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.login-page::before {
  opacity: 0.32;
  background-image:
    linear-gradient(rgba(77, 163, 255, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(77, 163, 255, 0.08) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: linear-gradient(to bottom, transparent 0%, black 28%, black 76%, transparent 100%);
}

.login-page::after {
  background:
    linear-gradient(115deg, transparent 0 16%, rgba(0, 212, 255, 0.08) 16.1% 16.5%, transparent 16.6%),
    linear-gradient(65deg, transparent 0 78%, rgba(47, 140, 255, 0.12) 78.1% 78.5%, transparent 78.6%);
}

.particle-canvas,
.aurora,
.grid-floor,
.corner-line {
  position: absolute;
  pointer-events: none;
}

.particle-canvas {
  inset: 0;
  z-index: 0;
  opacity: 0.72;
}

.aurora {
  z-index: 0;
  width: 54vw;
  height: 28vw;
  border-radius: 999px;
  filter: blur(44px);
  opacity: 0.32;
}

.aurora-top {
  top: -18vw;
  left: 18vw;
  background: rgba(22, 119, 255, 0.58);
}

.aurora-bottom {
  right: -18vw;
  bottom: -16vw;
  background: rgba(0, 212, 255, 0.42);
}

.grid-floor {
  z-index: 0;
  left: -10%;
  right: -10%;
  bottom: -7%;
  height: 34%;
  opacity: 0.34;
  transform: perspective(640px) rotateX(58deg);
  transform-origin: bottom;
  background-image:
    linear-gradient(rgba(0, 212, 255, 0.2) 1px, transparent 1px),
    linear-gradient(90deg, rgba(0, 212, 255, 0.16) 1px, transparent 1px);
  background-size: 58px 58px;
}

.corner-line {
  z-index: 1;
  width: 146px;
  height: 94px;
  border-color: rgba(0, 212, 255, 0.34);
}

.corner-line-left {
  left: 28px;
  bottom: 28px;
  border-left: 1px solid;
  border-bottom: 1px solid;
}

.corner-line-right {
  top: 28px;
  right: 28px;
  border-top: 1px solid;
  border-right: 1px solid;
}

.brand-panel,
.login-shell {
  position: relative;
  z-index: 2;
}

.brand-panel {
  display: grid;
  grid-template-columns: minmax(300px, 0.86fr) minmax(300px, 0.74fr);
  grid-template-rows: auto 1fr auto;
  align-items: center;
  gap: 28px 42px;
  padding: clamp(38px, 4.8vw, 72px);
}

.brand-header {
  grid-column: 1 / -1;
  align-self: start;
  display: inline-flex;
  align-items: center;
  gap: 14px;
  width: fit-content;
}

.logo-mark {
  width: 52px;
  height: 52px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(0, 212, 255, 0.36);
  border-radius: 8px;
  background: linear-gradient(145deg, rgba(22, 119, 255, 0.24), rgba(0, 212, 255, 0.1));
  box-shadow: 0 0 28px rgba(22, 119, 255, 0.28), inset 0 0 18px rgba(0, 212, 255, 0.12);
}

.logo-mark svg {
  width: 31px;
  height: 36px;
  fill: none;
  stroke: #00d4ff;
  stroke-width: 2.2;
  stroke-linecap: round;
  stroke-linejoin: round;
  filter: drop-shadow(0 0 10px rgba(0, 212, 255, 0.58));
}

.brand-header strong {
  display: block;
  font-size: 24px;
  line-height: 1.1;
  letter-spacing: 0;
}

.brand-header span {
  display: block;
  margin-top: 4px;
  color: #b8c7e0;
  font-size: 13px;
}

.brand-content {
  max-width: 680px;
}

.kicker,
.eyebrow {
  margin: 0;
  color: #00d4ff;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
}

.brand-content h1 {
  margin: 16px 0 18px;
  color: #ffffff;
  font-size: clamp(42px, 4.9vw, 76px);
  line-height: 1.06;
  letter-spacing: 0;
}

.brand-content h1 span {
  color: transparent;
  background: linear-gradient(90deg, #4da3ff, #00d4ff 58%, #bcefff);
  -webkit-background-clip: text;
  background-clip: text;
  text-shadow: 0 0 38px rgba(0, 212, 255, 0.18);
}

.brand-subtitle {
  max-width: 540px;
  margin: 0;
  color: #b8c7e0;
  font-size: 18px;
  line-height: 1.75;
}

.hologram-wrap {
  position: relative;
  grid-column: 2;
  grid-row: 2 / 4;
  width: min(34vw, 480px);
  min-width: 316px;
  aspect-ratio: 1 / 1.05;
  align-self: center;
  justify-self: center;
  display: flex;
  align-items: center;
  justify-content: center;
}

.orbit {
  position: absolute;
  border: 1px solid rgba(0, 212, 255, 0.24);
  border-radius: 50%;
  box-shadow: inset 0 0 22px rgba(22, 119, 255, 0.16), 0 0 26px rgba(0, 212, 255, 0.1);
  animation: rotateRing 18s linear infinite;
}

.orbit::before,
.orbit::after {
  content: "";
  position: absolute;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #00d4ff;
  box-shadow: 0 0 18px rgba(0, 212, 255, 0.9);
}

.orbit::before {
  top: 12%;
  left: 20%;
}

.orbit::after {
  right: 18%;
  bottom: 14%;
}

.orbit-outer {
  width: 86%;
  height: 72%;
  transform: rotate(-12deg);
}

.orbit-inner {
  width: 66%;
  height: 54%;
  border-style: dashed;
  animation-duration: 26s;
  animation-direction: reverse;
  transform: rotate(18deg);
}

.shield-core {
  position: relative;
  z-index: 2;
  width: 43%;
  animation: floatShield 5s ease-in-out infinite, glowPulse 3.2s ease-in-out infinite;
}

.shield-svg {
  width: 100%;
  height: auto;
  overflow: visible;
}

.shield-shell {
  fill: rgba(22, 119, 255, 0.14);
  stroke: url("#shieldGradient");
  stroke-width: 5;
}

.shield-panel {
  fill: rgba(0, 212, 255, 0.12);
  stroke: rgba(154, 222, 255, 0.72);
  stroke-width: 2;
}

.shield-check {
  fill: none;
  stroke: #c9f6ff;
  stroke-width: 12;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.scan-platform {
  position: absolute;
  z-index: 1;
  bottom: 17%;
  width: 70%;
  height: 22%;
  border-radius: 50%;
  background:
    radial-gradient(ellipse at center, rgba(0, 212, 255, 0.26), transparent 52%),
    linear-gradient(90deg, transparent, rgba(77, 163, 255, 0.32), transparent);
  transform: rotateX(64deg);
  box-shadow: 0 0 32px rgba(0, 183, 255, 0.24);
}

.scan-platform span {
  position: absolute;
  inset: 16%;
  border: 1px solid rgba(0, 212, 255, 0.38);
  border-radius: 50%;
  animation: scanPulse 2.8s ease-out infinite;
}

.scan-platform span:last-child {
  animation-delay: 1.35s;
}

.feature-list {
  grid-column: 1;
  display: grid;
  gap: 14px;
  max-width: 520px;
}

.feature-item {
  display: grid;
  grid-template-columns: 48px 1fr;
  gap: 14px;
  align-items: center;
  padding: 14px 16px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 8px;
  background: rgba(7, 21, 42, 0.46);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.feature-icon,
.input-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.feature-icon {
  width: 44px;
  height: 44px;
  border-radius: 8px;
  color: #00d4ff;
  background: rgba(22, 119, 255, 0.16);
  border: 1px solid rgba(0, 212, 255, 0.24);
}

.feature-icon :deep(svg),
.input-icon :deep(svg) {
  width: 20px;
  height: 20px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.feature-item strong {
  display: block;
  color: #ffffff;
  font-size: 15px;
  line-height: 1.25;
}

.feature-item em {
  display: block;
  margin-top: 5px;
  color: #b8c7e0;
  font-size: 13px;
  font-style: normal;
  line-height: 1.45;
}

.login-shell {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: clamp(24px, 4vw, 72px);
}

.auth-card {
  width: min(460px, calc(100vw - 40px));
  padding: 34px;
  border: 1px solid rgba(0, 212, 255, 0.22);
  border-radius: 8px;
  background:
    linear-gradient(180deg, rgba(11, 30, 58, 0.74), rgba(5, 11, 24, 0.76)),
    rgba(5, 11, 24, 0.68);
  box-shadow: 0 28px 80px rgba(0, 0, 0, 0.38), 0 0 42px rgba(0, 183, 255, 0.16);
  backdrop-filter: blur(18px);
  animation: cardEnter 560ms cubic-bezier(0.16, 1, 0.3, 1) both;
}

.panel-heading {
  margin-bottom: 26px;
}

.eyebrow {
  display: inline-flex;
  padding: 5px 9px;
  border: 1px solid rgba(0, 212, 255, 0.26);
  border-radius: 4px;
  background: rgba(0, 212, 255, 0.08);
}

.panel-heading h2 {
  margin: 14px 0 9px;
  color: #ffffff;
  font-size: 26px;
  line-height: 1.25;
  letter-spacing: 0;
}

.panel-heading p {
  margin: 0;
  color: #b8c7e0;
  line-height: 1.65;
}

.auth-form,
.bootstrap-actions {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.auth-form :deep(.el-form-item) {
  margin-bottom: 0;
}

.auth-form :deep(.el-form-item__label) {
  margin-bottom: 8px;
  color: #dce8ff;
  font-weight: 600;
  line-height: 1.25;
}

.auth-form :deep(.el-input__wrapper) {
  min-height: 48px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(5, 11, 24, 0.52);
  box-shadow: none;
  transition: border-color 180ms ease, box-shadow 180ms ease, background 180ms ease;
}

.auth-form :deep(.el-input__wrapper:hover) {
  border-color: rgba(77, 163, 255, 0.52);
}

.auth-form :deep(.el-input__wrapper.is-focus) {
  border-color: rgba(0, 212, 255, 0.84);
  background: rgba(7, 21, 42, 0.78);
  box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.13), 0 0 18px rgba(22, 119, 255, 0.22);
}

.auth-form :deep(.el-input__inner) {
  color: #ffffff;
}

.auth-form :deep(.el-input__inner::placeholder) {
  color: #6f819c;
}

.auth-form :deep(.el-input__prefix),
.auth-form :deep(.el-input__suffix) {
  color: #6fbfff;
}

.auth-form :deep(.el-form-item.is-error .el-input__wrapper) {
  border-color: rgba(255, 77, 79, 0.84);
  animation: inputShake 180ms ease-in-out 2;
}

.auth-form :deep(.el-form-item__error) {
  color: #ff8f91;
}

.input-icon {
  color: #6fbfff;
}

.form-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  color: #b8c7e0;
  font-size: 14px;
}

.remember-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  cursor: pointer;
}

.remember-option input {
  width: 16px;
  height: 16px;
  accent-color: #1677ff;
}

.link-button {
  border: 0;
  padding: 0;
  color: #6fbfff;
  background: transparent;
  cursor: pointer;
  font: inherit;
  transition: color 160ms ease;
}

.link-button:hover,
.link-button:focus-visible {
  color: #00d4ff;
}

.submit-button {
  position: relative;
  width: 100%;
  min-height: 48px;
  margin-top: 2px;
  overflow: hidden;
  border: 0;
  border-radius: 8px;
  background: linear-gradient(90deg, #2563eb, #0891b2);
  box-shadow: 0 12px 30px rgba(0, 119, 255, 0.28);
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
  transition: filter 180ms ease, box-shadow 180ms ease, transform 120ms ease;
}

.submit-button::before {
  content: "";
  position: absolute;
  inset: 0 auto 0 -42%;
  width: 34%;
  transform: skewX(-18deg);
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.42), transparent);
  transition: left 520ms ease;
}

.submit-button:hover {
  filter: brightness(1.08);
  box-shadow: 0 0 24px rgba(0, 183, 255, 0.45);
}

.submit-button:hover::before {
  left: 112%;
}

.submit-button:active {
  transform: scale(0.985);
}

.submit-button:disabled {
  cursor: not-allowed;
  opacity: 0.74;
}

.auth-skeleton :deep(.el-skeleton__item) {
  background: linear-gradient(90deg, rgba(255, 255, 255, 0.08), rgba(0, 212, 255, 0.14), rgba(255, 255, 255, 0.08));
}

.bootstrap-actions :deep(.el-alert) {
  border: 1px solid rgba(250, 173, 20, 0.28);
  background: rgba(250, 173, 20, 0.1);
}

@keyframes floatShield {
  0%,
  100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-14px);
  }
}

@keyframes glowPulse {
  0%,
  100% {
    filter: drop-shadow(0 0 12px rgba(0, 180, 255, 0.45));
  }
  50% {
    filter: drop-shadow(0 0 28px rgba(0, 212, 255, 0.85));
  }
}

@keyframes rotateRing {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

@keyframes scanPulse {
  0% {
    opacity: 0.72;
    transform: scale(0.68);
  }
  100% {
    opacity: 0;
    transform: scale(1.35);
  }
}

@keyframes cardEnter {
  from {
    opacity: 0;
    transform: translateX(40px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateX(0) scale(1);
  }
}

@keyframes inputShake {
  0%,
  100% {
    transform: translateX(0);
  }
  50% {
    transform: translateX(3px);
  }
}

@media (max-width: 1366px) {
  .login-page {
    grid-template-columns: minmax(570px, 1.18fr) minmax(408px, 0.82fr);
  }

  .brand-panel {
    gap: 20px 28px;
    padding: 34px 44px;
  }

  .brand-content h1 {
    font-size: 48px;
  }

  .hologram-wrap {
    min-width: 288px;
  }

  .feature-list {
    gap: 10px;
  }

  .feature-item {
    padding: 12px 14px;
  }

  .auth-card {
    padding: 28px;
  }
}

@media (max-width: 1080px) {
  .login-page {
    grid-template-columns: 1fr;
    overflow-y: auto;
  }

  .brand-panel {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto;
    padding: 32px 28px 10px;
  }

  .brand-content {
    max-width: 760px;
  }

  .brand-content h1 {
    font-size: clamp(38px, 8vw, 56px);
  }

  .hologram-wrap {
    grid-column: 1;
    grid-row: auto;
    width: min(52vw, 360px);
    min-width: 240px;
    margin-top: -16px;
  }

  .feature-list {
    grid-column: 1;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    max-width: none;
  }

  .feature-item {
    grid-template-columns: 42px 1fr;
    align-items: start;
  }

  .login-shell {
    align-items: start;
    padding-top: 20px;
  }
}

@media (max-width: 760px) {
  .brand-panel {
    gap: 18px;
    padding: 24px 20px 6px;
  }

  .brand-header span,
  .kicker,
  .feature-list,
  .corner-line {
    display: none;
  }

  .brand-content h1 {
    margin-top: 8px;
    font-size: 36px;
  }

  .brand-subtitle {
    font-size: 15px;
  }

  .hologram-wrap {
    width: 220px;
    min-width: 220px;
    margin: -24px auto -26px;
    opacity: 0.72;
  }

  .login-shell {
    padding: 18px 20px 32px;
  }

  .auth-card {
    width: 100%;
    padding: 24px;
  }
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    scroll-behavior: auto !important;
    transition-duration: 0.01ms !important;
  }
}
</style>

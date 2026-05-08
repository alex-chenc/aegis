<template>
  <router-view v-if="isAuthLayout" />
  <el-container v-else class="app-container">
    <el-aside width="220px" class="sidebar">
      <div class="logo">
        <span class="logo-mark" aria-hidden="true">
          <span class="brand-shield"></span>
        </span>
        <div class="logo-copy">
          <span class="logo-text">Aegis</span>
          <span class="logo-subtitle">主机安全指挥台</span>
        </div>
      </div>
      <el-menu
        :default-active="activeMenu"
        :router="true"
        class="sidebar-menu"
      >
        <el-menu-item index="/hosts">
          <el-icon><Monitor /></el-icon>
          <span>主机列表</span>
        </el-menu-item>

        <el-sub-menu index="baseline">
          <template #title>
            <el-icon><Document /></el-icon>
            <span>智能基线检查与修复</span>
          </template>
          <el-menu-item index="/baseline/workbench">
            <el-icon><SetUp /></el-icon>
            <span>基线工作台</span>
          </el-menu-item>
          <el-menu-item index="/baseline/tasks">
            <el-icon><List /></el-icon>
            <span>基线任务中心</span>
          </el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="vulnerability">
          <template #title>
            <el-icon><Warning /></el-icon>
            <span>智能漏洞检查与修复</span>
          </template>
          <el-menu-item index="/vulnerability">
            <el-icon><SetUp /></el-icon>
            <span>漏洞工作台</span>
          </el-menu-item>
          <el-menu-item index="/vulnerability/tasks">
            <el-icon><List /></el-icon>
            <span>漏洞任务中心</span>
          </el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="detection">
          <template #title>
            <el-icon><DataAnalysis /></el-icon>
            <span>智能异常检测</span>
          </template>
          <el-menu-item index="/detection/overview">
            <el-icon><DataAnalysis /></el-icon>
            <span>安全概览</span>
          </el-menu-item>
          <el-menu-item index="/detection/alerts">
            <el-icon><Bell /></el-icon>
            <span>告警列表</span>
          </el-menu-item>
          <el-menu-item index="/detection/ai-analysis">
            <el-icon><ChatDotRound /></el-icon>
            <span>AI 分析</span>
          </el-menu-item>
          <el-menu-item index="/detection/policies">
            <el-icon><Operation /></el-icon>
            <span>阻断策略</span>
          </el-menu-item>
          <el-menu-item index="/detection/rules">
            <el-icon><Tickets /></el-icon>
            <span>规则管理</span>
          </el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="settings">
          <template #title>
            <el-icon><Setting /></el-icon>
            <span>系统配置</span>
          </template>
          <el-menu-item index="/settings/models">
            <el-icon><Setting /></el-icon>
            <span>模型配置</span>
          </el-menu-item>
          <el-menu-item index="/settings/agent">
            <el-icon><Operation /></el-icon>
            <span>Agent 安装</span>
          </el-menu-item>
          <el-menu-item index="/settings/command-audit">
            <el-icon><Tickets /></el-icon>
            <span>命令审计配置</span>
          </el-menu-item>
          <el-menu-item index="/settings/audit-logs">
            <el-icon><Document /></el-icon>
            <span>审计日志</span>
          </el-menu-item>
        </el-sub-menu>
      </el-menu>

      <div class="sidebar-footer">
        <span class="status-dot" />
        <span class="version">控制面在线 · v3.0</span>
      </div>
    </el-aside>

    <el-container>
      <el-header class="app-header">
        <div class="header-left">
          <div class="route-kicker">Security Operations</div>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item v-for="item in breadcrumbs" :key="item.path">
              {{ item.title }}
            </el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <div class="system-chip">
            <span class="status-dot" />
            API 正常
          </div>
          <NotificationBell />
          <el-tooltip content="刷新" placement="bottom">
            <el-button :icon="Refresh" circle size="small" @click="handleRefresh" />
          </el-tooltip>
        </div>
      </el-header>

      <el-main class="app-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Monitor, Document, SetUp, List, Warning, Setting, Refresh, DataAnalysis, Bell, Operation, Tickets, ChatDotRound } from '@element-plus/icons-vue'
import NotificationBell from '@/components/notification/NotificationBell.vue'
import { clearStoredAuth, getStoredAuth } from '@/utils/auth'
import { createIdleLogout } from '@/utils/sessionTimeout'

const route = useRoute()
const router = useRouter()

const activeMenu = computed(() => {
  return route.path
})

const breadcrumbs = computed(() => {
  const matched = route.matched.filter(item => item.meta && item.meta.title)
  const crumbs = [{ path: '/', title: '首页' }]
  
  matched.forEach(item => {
    crumbs.push({
      path: item.path,
      title: item.meta.title as string
    })
  })
  
  return crumbs
})

const isAuthLayout = computed(() => {
  return Boolean(route.meta.authLayout)
})

function setViewportContent(content: string) {
  const viewport = document.querySelector<HTMLMetaElement>('meta[name="viewport"]')
  if (viewport) {
    viewport.setAttribute('content', content)
  }
}

function syncViewportForLayout() {
  if (isAuthLayout.value) {
    setViewportContent('width=device-width, initial-scale=1.0')
    return
  }

  setViewportContent('width=1280')
}

function handleRefresh() {
  router.go(0)
}

const idleLogout = createIdleLogout({
  isEnabled: () => Boolean(getStoredAuth()) && !route.meta.authLayout,
  onTimeout: () => {
    clearStoredAuth()
    ElMessage.warning('5 分钟未操作，已自动退出登录')
    router.replace('/login')
  }
})

onMounted(() => {
  syncViewportForLayout()
  idleLogout.start()
})

onBeforeUnmount(() => {
  setViewportContent('width=device-width, initial-scale=1.0')
  idleLogout.stop()
})

watch(() => route.fullPath, () => {
  syncViewportForLayout()
  idleLogout.refresh()
}, { immediate: true })
</script>

<style scoped>
.app-container {
  height: 100dvh;
  min-width: var(--aegis-desktop-min-width);
  width: max(100vw, var(--aegis-desktop-min-width));
  background:
    linear-gradient(90deg, rgba(11, 18, 32, 0.98) 0 220px, transparent 220px),
    radial-gradient(circle at 80% 8%, rgba(34, 211, 238, 0.14), transparent 25%),
    linear-gradient(135deg, #edf5ff, #f8fafc);
}

.sidebar {
  position: relative;
  background:
    radial-gradient(circle at 20% 0%, rgba(34, 211, 238, 0.18), transparent 32%),
    linear-gradient(180deg, #0b1220 0%, #111827 52%, #0f172a 100%);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid rgba(148, 163, 184, 0.14);
}

.sidebar::after {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.035) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.035) 1px, transparent 1px);
  background-size: 26px 26px;
  opacity: 0.5;
}

.logo {
  position: relative;
  z-index: 1;
  height: 72px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 18px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
}

.logo-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border: 1px solid rgba(34, 211, 238, 0.38);
  border-radius: 8px;
  background: rgba(34, 211, 238, 0.1);
  box-shadow: 0 10px 28px rgba(34, 211, 238, 0.16);
}

.brand-shield {
  width: 24px;
  height: 28px;
  display: block;
  background-image: url('/aegis-brand-source.png');
  background-repeat: no-repeat;
  background-size: 1916px 804px;
  background-position: -23px -28px;
}

.logo-copy {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.logo-text {
  font-size: 18px;
  font-weight: 800;
  color: #fff;
  letter-spacing: 0.04em;
}

.logo-subtitle {
  color: rgba(226, 232, 240, 0.62);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-menu {
  position: relative;
  z-index: 1;
  flex: 1;
  border-right: none;
  overflow-y: auto;
  background: transparent;
}

.sidebar-menu:not(.el-menu--collapse) {
  width: 220px;
}

.sidebar-footer {
  position: relative;
  z-index: 1;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-top: 1px solid rgba(148, 163, 184, 0.14);
}

.version {
  font-size: 12px;
  color: rgba(226, 232, 240, 0.72);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.14);
}

.app-header {
  height: 64px;
  background: rgba(255, 255, 255, 0.72);
  border-bottom: 1px solid rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
}

.header-left {
  display: flex;
  flex-direction: column;
  gap: 4px;
  justify-content: center;
}

.route-kicker {
  color: #64748b;
  font-size: 11px;
  font-weight: 750;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.system-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 12px;
  border: 1px solid rgba(34, 197, 94, 0.2);
  border-radius: 999px;
  color: #166534;
  background: rgba(240, 253, 244, 0.82);
  font-size: 12px;
  font-weight: 700;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.app-main {
  position: relative;
  background:
    radial-gradient(circle at 8% 12%, rgba(34, 211, 238, 0.12), transparent 24%),
    radial-gradient(circle at 92% 0%, rgba(37, 99, 235, 0.1), transparent 22%),
    linear-gradient(135deg, rgba(241, 247, 253, 0.96), rgba(248, 250, 252, 0.94));
  padding: 24px;
  overflow-y: auto;
}

:deep(.el-menu) {
  border-right: none;
}

:deep(.el-menu-item),
:deep(.el-sub-menu__title) {
  height: 46px;
  line-height: 46px;
  margin: 4px 10px;
  border-radius: 12px;
  color: rgba(226, 232, 240, 0.78);
  transition: background 180ms ease, color 180ms ease, transform 180ms ease;
}

:deep(.el-menu-item:hover),
:deep(.el-sub-menu__title:hover) {
  background: rgba(34, 211, 238, 0.08) !important;
  color: #f8fafc !important;
  transform: translateX(2px);
}

:deep(.el-menu-item.is-active) {
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.95), rgba(8, 145, 178, 0.95)) !important;
  color: #ffffff !important;
  box-shadow: 0 12px 28px rgba(37, 99, 235, 0.3);
}

:deep(.el-menu-item.is-active .el-icon) {
  color: #ffffff !important;
}

:deep(.el-menu-item.is-active span) {
  color: #ffffff !important;
}

:deep(.el-sub-menu .el-menu-item) {
  min-width: 220px;
  padding-left: 44px !important;
  background: rgba(15, 23, 42, 0.34) !important;
}

:deep(.el-sub-menu .el-menu-item:hover) {
  background: rgba(34, 211, 238, 0.08) !important;
}

:deep(.el-sub-menu .el-menu-item.is-active) {
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.95), rgba(8, 145, 178, 0.95)) !important;
  color: #ffffff !important;
}

:deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: #67e8f9 !important;
}

:deep(.el-sub-menu.is-active > .el-sub-menu__title .el-icon) {
  color: #67e8f9 !important;
}
</style>

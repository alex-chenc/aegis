<template>
  <el-container class="app-container">
    <el-aside width="220px" class="sidebar">
      <div class="logo">
        <span class="logo-icon">🛡️</span>
        <span class="logo-text">Aegis智能主机安全系统</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        :router="true"
        background-color="#304156"
        text-color="#bfcbd9"
        active-text-color="#ffffff"
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

        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <span>系统配置</span>
        </el-menu-item>
      </el-menu>

      <div class="sidebar-footer">
        <span class="version">v3.0</span>
      </div>
    </el-aside>

    <el-container>
      <el-header class="app-header">
        <div class="header-left">
          <el-breadcrumb separator="/">
            <el-breadcrumb-item v-for="item in breadcrumbs" :key="item.path">
              {{ item.title }}
            </el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
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
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Monitor, Document, SetUp, List, Warning, Setting, Refresh } from '@element-plus/icons-vue'

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

function handleRefresh() {
  router.go(0)
}
</script>

<style scoped>
.app-container {
  height: 100vh;
  width: 100vw;
}

.sidebar {
  background-color: #304156;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  background-color: #263445;
  border-bottom: 1px solid #1f2d3d;
}

.logo-icon {
  font-size: 24px;
  margin-right: 8px;
}

.logo-text {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-menu {
  flex: 1;
  border-right: none;
  overflow-y: auto;
}

.sidebar-menu:not(.el-menu--collapse) {
  width: 220px;
}

.sidebar-footer {
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #263445;
  border-top: 1px solid #1f2d3d;
}

.version {
  font-size: 12px;
  color: #909399;
}

.app-header {
  background-color: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 50px;
}

.header-left {
  display: flex;
  align-items: center;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.app-main {
  background-color: #f0f2f5;
  padding: 20px;
  overflow-y: auto;
}

:deep(.el-menu) {
  border-right: none;
}

:deep(.el-menu-item),
:deep(.el-sub-menu__title) {
  height: 50px;
  line-height: 50px;
}

:deep(.el-menu-item:hover),
:deep(.el-sub-menu__title:hover) {
  background-color: #263445 !important;
}

:deep(.el-menu-item.is-active) {
  background-color: #409EFF !important;
  color: #ffffff !important;
}

:deep(.el-menu-item.is-active .el-icon) {
  color: #ffffff !important;
}

:deep(.el-menu-item.is-active span) {
  color: #ffffff !important;
}

:deep(.el-sub-menu .el-menu-item) {
  min-width: 220px;
  padding-left: 50px !important;
  background-color: #1f2d3d !important;
}

:deep(.el-sub-menu .el-menu-item:hover) {
  background-color: #263445 !important;
}

:deep(.el-sub-menu .el-menu-item.is-active) {
  background-color: #409EFF !important;
  color: #ffffff !important;
}

:deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: #409EFF !important;
}

:deep(.el-sub-menu.is-active > .el-sub-menu__title .el-icon) {
  color: #409EFF !important;
}
</style>
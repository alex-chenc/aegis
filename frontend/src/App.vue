<template>
  <div id="app">
    <el-container>
      <el-header>
        <div class="logo">AI基线检查系统</div>
        <el-menu mode="horizontal" :router="true" :default-active="$route.path">
          <el-menu-item index="/">工作台</el-menu-item>
          <el-menu-item index="/tasks">任务中心</el-menu-item>
          <el-menu-item index="/dashboard">主机列表</el-menu-item>
          <el-menu-item index="/settings">系统配置</el-menu-item>
        </el-menu>
      </el-header>
      <el-main>
        <router-view />
      </el-main>
    </el-container>
  </div>
</template>

<script setup lang="ts">
</script>

<style scoped>
.el-header {
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  padding: 0 20px;
}

.logo {
  float: left;
  font-size: 20px;
  font-weight: bold;
  line-height: 60px;
  color: #409EFF;
}

.el-menu {
  border: none;
}

.el-main {
  background: #f0f2f5;
  padding: 20px;
}
</style>
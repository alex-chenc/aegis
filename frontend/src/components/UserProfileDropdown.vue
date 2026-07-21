<template>
  <el-dropdown trigger="click" @command="handleCommand">
    <el-button :icon="UserFilled" circle size="small" class="profile-btn" />
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="version">{{ t('app.profile.version') }}</el-dropdown-item>
        <el-dropdown-item divided command="change-password">{{ t('app.profile.changePassword') }}</el-dropdown-item>
        <el-dropdown-item command="logout">{{ t('app.profile.logout') }}</el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
  <ChangePasswordDialog v-if="showChangePassword" :visible="showChangePassword" @close="showChangePassword = false" @success="showChangePassword = false" />
  <el-dialog v-model="showVersion" :title="t('app.profile.version')" width="360px" append-to-body>
    <div class="version-detail">
      <div class="version-product">{{ t('app.brand.product') }}</div>
      <div class="version-number">V5.7</div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { UserFilled } from '@element-plus/icons-vue'
import { logout } from '@/api/auth'
import { clearStoredAuth } from '@/utils/auth'
import ChangePasswordDialog from '@/components/common/ChangePasswordDialog.vue'

const router = useRouter()
const { t } = useI18n()
const showChangePassword = ref(false)
const showVersion = ref(false)

async function handleCommand(command: string) {
  if (command === 'change-password') {
    showChangePassword.value = true
  } else if (command === 'version') {
    showVersion.value = true
  } else if (command === 'logout') {
    try {
      await logout()
    } catch {
      // ignore server errors on logout
    }
    clearStoredAuth()
    ElMessage.success(t('app.profile.logoutSuccess'))
    router.replace('/login')
  }
}
</script>

<style scoped>
.profile-btn {
  border: 1px solid rgba(148, 163, 184, 0.24);
}

.version-detail {
  text-align: center;
  padding: 12px 0;
}

.version-product {
  font-size: 14px;
  font-weight: 600;
  color: #1e293b;
  margin-bottom: 8px;
}

.version-number {
  font-size: 24px;
  font-weight: 700;
  color: #3b82f6;
}
</style>

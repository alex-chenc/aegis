<template>
  <el-tooltip :content="tooltip" placement="bottom">
    <el-button
      class="language-switcher"
      size="small"
      :aria-label="tooltip"
      @click="handleToggle"
    >
      {{ buttonLabel }}
    </el-button>
  </el-tooltip>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getCurrentLocale, toggleLocale } from '@/i18n'

const { t, locale } = useI18n()

const buttonLabel = computed(() => locale.value === 'zh-CN'
  ? t('common.language.englishShort')
  : t('common.language.chineseShort'))

const tooltip = computed(() => locale.value === 'zh-CN'
  ? t('common.language.switchToEnglish')
  : t('common.language.switchToChinese'))

function handleToggle() {
  toggleLocale()
  // Accessing the value documents that this component is driven by the global locale.
  getCurrentLocale()
}
</script>

<style scoped>
.language-switcher {
  min-width: 56px;
  border-color: rgba(148, 163, 184, 0.32);
  font-weight: 600;
}
</style>

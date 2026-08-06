<template>
  <section class="escape-policy-overview" aria-label="Agent escape protection policy">
    <div class="overview-heading">
      <div>
        <span class="eyebrow">{{ t('agentGuard.escapeOverview.eyebrow') }}</span>
        <h2>{{ t('agentGuard.escapeOverview.title') }}</h2>
        <p>{{ t('agentGuard.escapeOverview.description') }}</p>
      </div>
      <el-tag type="danger" effect="dark">{{ t('agentGuard.escapeOverview.independent') }}</el-tag>
    </div>
    <div class="strategy-grid">
      <article v-for="item in strategies" :key="item.key" class="strategy-card">
        <div>
          <h3>{{ item.title }}</h3>
          <p>{{ item.description }}</p>
          <div class="strategy-tags">
            <el-tag v-for="tag in item.tags" :key="tag" size="small" effect="plain">{{ tag }}</el-tag>
          </div>
        </div>
      </article>
    </div>
    <div class="chain-row">
      <span v-for="(stage, index) in chain" :key="stage" class="chain-stage">
        {{ stage }}<i v-if="index < chain.length - 1">→</i>
      </span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const strategies = computed(() => [
  { key: 'permission', title: t('agentGuard.escapeOverview.permissionTitle'), description: t('agentGuard.escapeOverview.permissionDescription'), tags: ['Codex', 'Claude Code', 'OpenClaw', 'Hermes', 'Zcode'] },
  { key: 'directory', title: t('agentGuard.escapeOverview.directoryTitle'), description: t('agentGuard.escapeOverview.directoryDescription'), tags: ['workspaceAccess', 'safe root', 'temp roots'] },
  { key: 'network', title: t('agentGuard.escapeOverview.networkTitle'), description: t('agentGuard.escapeOverview.networkDescription'), tags: ['curl', 'allowlist', 'elevated'] },
])
const chain = computed(() => [
  t('agentGuard.escapeOverview.permission'),
  t('agentGuard.escapeOverview.hook'),
  t('agentGuard.escapeOverview.process'),
  t('agentGuard.escapeOverview.execution'),
  t('agentGuard.escapeOverview.verdict'),
])
</script>

<style scoped>
.escape-policy-overview { margin-bottom: 18px; padding: 20px 22px; border: 1px solid rgba(239, 68, 68, .28); border-radius: 14px; background: linear-gradient(135deg, rgba(127, 29, 29, .16), var(--el-bg-color-overlay)); }
.overview-heading { display: flex; justify-content: space-between; gap: 18px; align-items: flex-start; }
.eyebrow { color: var(--el-color-danger); font-size: 11px; font-weight: 700; letter-spacing: .12em; text-transform: uppercase; }
.overview-heading h2 { margin: 5px 0 4px; font-size: 19px; }
.overview-heading p { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; }
.strategy-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin-top: 18px; }
.strategy-card { display: flex; gap: 12px; min-height: 112px; padding: 14px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-bg-color); }
.strategy-icon { flex: 0 0 auto; color: var(--el-color-danger); font: 700 13px/1.8 ui-monospace, SFMono-Regular, Menlo, monospace; }
.strategy-card h3 { margin: 0; font-size: 14px; }
.strategy-card p { margin: 6px 0 10px; color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.5; }
.strategy-tags { display: flex; flex-wrap: wrap; gap: 5px; }
.chain-row { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; padding-top: 14px; border-top: 1px dashed var(--el-border-color); }
.chain-stage { display: inline-flex; align-items: center; gap: 7px; color: var(--el-text-color-regular); font-size: 12px; }
.chain-stage strong { display: inline-flex; width: 21px; height: 21px; align-items: center; justify-content: center; border-radius: 50%; background: var(--el-color-danger); color: #fff; font-size: 11px; }
.chain-stage i { margin-left: 2px; color: var(--el-color-danger); font-style: normal; font-size: 16px; }
@media (max-width: 900px) { .strategy-grid { grid-template-columns: 1fr; } }
</style>

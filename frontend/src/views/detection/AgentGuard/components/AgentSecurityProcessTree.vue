<template>
  <article class="security-process-node" :class="{ matched: node.matched }" data-testid="security-process-node">
    <div class="process-node-header">
      <span class="tree-branch" aria-hidden="true">└─</span>
      <strong>PID {{ node.pid }}</strong>
    </div>
    <div class="process-node-meta">
      <span>PPID {{ node.ppid }}</span>
    </div>
    <div class="process-command">
      <span>{{ t('agentGuard.analysis.commandLine') }}</span>
      <code>{{ node.cmdline || '-' }}</code>
    </div>
    <div v-if="node.children?.length" class="process-tree-children">
      <AgentSecurityProcessTree
        v-for="child in node.children"
        :key="child.id"
        :node="child"
      />
    </div>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AgentSecurityFindingProcessNode } from '@/types/agentGuard'

const { t } = useI18n()

defineProps<{
  node: AgentSecurityFindingProcessNode
}>()
</script>

<style scoped>
.security-process-node {
  position: relative;
  padding: 9px 10px;
  border: 1px solid var(--aegis-border);
  border-radius: 9px;
  background: #fff;
}

.security-process-node.matched {
  border-color: var(--el-color-danger-light-5);
  background: var(--el-color-danger-light-9);
}

.process-node-header,
.process-node-meta,
.process-command {
  display: flex;
  align-items: center;
  gap: 8px;
}

.process-node-header strong {
  min-width: 0;
  overflow-wrap: anywhere;
}

.tree-branch {
  color: var(--aegis-text-muted);
}

.process-node-meta {
  flex-wrap: wrap;
  margin-top: 4px;
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.process-command {
  align-items: flex-start;
  margin-top: 7px;
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.process-command code {
  min-width: 0;
  color: var(--aegis-text);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.process-tree-children {
  display: flex;
  flex-direction: column;
  gap: 7px;
  margin: 9px 0 0 18px;
  padding-left: 12px;
  border-left: 1px dashed var(--aegis-border);
}
</style>

<template>
  <div class="panorama-explorer">
    <section class="panorama-tree" :aria-label="t('agentGuard.panorama.tree')">
      <el-empty v-if="rows.length === 0" :description="t('agentGuard.states.panoramaNotCollected')" />
      <div
        v-for="row in rows"
        v-else
        :key="row.node.id"
        role="button"
        tabindex="0"
        class="panorama-row"
        :class="{ selected: selected?.id === row.node.id }"
        :style="{ paddingLeft: `${12 + row.depth * 22}px` }"
        :data-testid="`panorama-node-${row.node.id}`"
        @click="selectNode(row.node)"
        @keydown.enter="selectNode(row.node)"
      >
        <span class="expand-slot">
          <button
            v-if="row.node.has_children"
            type="button"
            class="expand-button"
            :aria-label="t('agentGuard.panorama.expand')"
            :data-testid="`panorama-expand-${row.node.id}`"
            @click.stop="toggle(row)"
          >
            {{ expanded.has(row.node.id) ? '−' : '+' }}
          </button>
        </span>
        <span class="node-kind">{{ nodeKind(row.node.node_type) }}</span>
        <span class="node-copy">
          <strong>{{ nodeLabel(row.node) }}</strong>
          <small>
            {{ nodeMeta(row.node) }}
          </small>
        </span>
        <el-tag
          v-if="row.node.severity && row.node.severity !== 'info'"
          size="small"
          :type="severityType(row.node.severity)"
          effect="plain"
        >
          {{ row.node.severity }}
        </el-tag>
        <el-tag
          v-if="row.node.trust?.tool_semantics === 'trusted'"
          size="small"
          type="success"
          effect="plain"
        >
          {{ t('agentGuard.panorama.trustedToolSemantics') }}
        </el-tag>
        <span
          v-if="hasCompletenessGap(row.node)"
          class="evidence-gap"
          :title="t('agentGuard.panorama.incompleteEvidence')"
        >
          !
        </span>
      </div>
    </section>

    <aside class="panorama-evidence">
      <el-empty v-if="!selected" :description="t('agentGuard.panorama.selectNode')" />
      <template v-else>
        <div class="evidence-heading">
          <span class="node-kind large">{{ nodeKind(selected.node_type) }}</span>
          <div>
            <h3>{{ selected.label }}</h3>
            <p>{{ selected.node_type }} · {{ selected.occurred_at || '-' }}</p>
          </div>
        </div>
        <dl>
          <div>
            <dt>{{ t('agentGuard.panorama.nodeId') }}</dt>
            <dd>{{ selected.event_id || selected.object_id || selected.id }}</dd>
          </div>
          <div>
            <dt>{{ t('agentGuard.findings.severity') }}</dt>
            <dd>{{ selected.severity || 'info' }}</dd>
          </div>
          <div v-if="selected.collection?.visibility">
            <dt>{{ t('agentGuard.panorama.visibility') }}</dt>
            <dd>{{ selected.collection.visibility }}</dd>
          </div>
          <div v-if="selected.external_session_id">
            <dt>{{ t('agentGuard.panorama.sessionId') }}</dt>
            <dd>{{ selected.external_session_id }}</dd>
          </div>
          <div v-if="selected.cmdline">
            <dt>{{ t('agentGuard.panorama.cmdline') }}</dt>
            <dd>{{ selected.cmdline }}</dd>
          </div>
          <div v-if="selected.collection?.lost_events_since_last">
            <dt>{{ t('agentGuard.panorama.lostEvents') }}</dt>
            <dd>{{ selected.collection.lost_events_since_last }}</dd>
          </div>
          <div v-if="selected.trust?.source">
            <dt>{{ t('agentGuard.panorama.evidenceSource') }}</dt>
            <dd>{{ selected.trust.source }}</dd>
          </div>
          <div v-if="selected.trust?.correlation">
            <dt>{{ t('agentGuard.panorama.correlation') }}</dt>
            <dd>{{ selected.trust.correlation === 'matched'
              ? t('agentGuard.panorama.correlationMatched')
              : t('agentGuard.panorama.correlationUnmatched') }}</dd>
          </div>
        </dl>
        <el-alert
          v-if="hasLimitation(selected, 'tool_semantics_unobservable')"
          type="info"
          :closable="false"
          :title="t('agentGuard.panorama.toolSemanticsUnobservable')"
          show-icon
        />
        <el-alert
          v-if="hasLimitation(selected, 'remote_unobservable')"
          type="warning"
          :closable="false"
          :title="t('agentGuard.panorama.remoteUnobservable')"
          show-icon
        />
        <el-alert
          v-if="hasCompletenessGap(selected)"
          type="warning"
          :closable="false"
          :title="t('agentGuard.panorama.incompleteEvidence')"
          show-icon
        />
        <slot name="detail" :node="selected" />
      </template>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentGuardMode, PanoramaTreeNode } from '@/types/agentGuard'

interface TreeRow {
  node: PanoramaTreeNode
  depth: number
}

const props = defineProps<{
  nodes: PanoramaTreeNode[]
  mode: AgentGuardMode
  loadChildren: (nodeId: string) => Promise<PanoramaTreeNode[]>
}>()

const emit = defineEmits<{
  (event: 'select', node: PanoramaTreeNode): void
}>()

const { t } = useI18n()
const rows = ref<TreeRow[]>([])
const expanded = ref(new Set<string>())
const selected = ref<PanoramaTreeNode | null>(null)
const loading = new Set<string>()

watch(() => props.nodes, nodes => {
  rows.value = [...nodes]
    .sort(compareNodes)
    .map(node => ({ node, depth: 0 }))
  expanded.value = new Set()
  selected.value = nodes[0] || null
}, { immediate: true, deep: true })

const isEscape = computed(() => props.mode === 'escape')

async function toggle(row: TreeRow) {
  const id = row.node.id
  if (expanded.value.has(id)) {
    const index = rows.value.findIndex(item => item.node.id === id)
    let end = index + 1
    while (end < rows.value.length && rows.value[end].depth > row.depth) end++
    rows.value.splice(index + 1, end - index - 1)
    const next = new Set(expanded.value)
    next.delete(id)
    expanded.value = next
    return
  }
  if (loading.has(id)) return
  loading.add(id)
  try {
    const children = (await props.loadChildren(id))
      .slice()
      .sort(compareNodes)
      .map(node => ({ ...node, parent_id: node.parent_id || id }))
    const index = rows.value.findIndex(item => item.node.id === id)
    rows.value.splice(index + 1, 0, ...children.map(node => ({
      node,
      depth: row.depth + 1,
    })))
    expanded.value = new Set(expanded.value).add(id)
  } finally {
    loading.delete(id)
  }
}

function selectNode(node: PanoramaTreeNode) {
  selected.value = node
  emit('select', node)
}

function compareNodes(left: PanoramaTreeNode, right: PanoramaTreeNode) {
  const time = String(left.occurred_at || '').localeCompare(String(right.occurred_at || ''))
  return time || left.id.localeCompare(right.id)
}

function nodeKind(type: string) {
  const value = type.toLowerCase()
  if (value === 'agent_asset') return 'AI'
  if (value === 'instance') return 'IN'
  if (value === 'session') return 'SE'
  if (value === 'execution_unit') return isEscape.value ? 'SB' : 'UN'
  if (value === 'process') return 'PID'
  if (value === 'tool_call') return 'TL'
  if (value === 'file') return 'FI'
  if (value === 'network') return 'NW'
  if (value === 'identity' || value === 'privilege') return 'ID'
  if (value === 'isolation' || value === 'kernel') return 'ISO'
  if (value === 'rule' || value === 'finding') return 'R'
  return '·'
}

function nodeLabel(node: PanoramaTreeNode) {
  if (node.node_type === 'process' && node.pid) {
    if (node.label?.startsWith(`PID ${node.pid}`)) return node.label
    const command = node.cmdline || node.label
    return command ? `PID ${node.pid} · ${command}` : `PID ${node.pid}`
  }
  if (node.node_type === 'session' && node.external_session_id) {
    return node.external_session_id
  }
  if (node.node_type === 'session' && node.session_source === 'activity_window') {
    return t('agentGuard.panorama.inferredActivityWindow')
  }
  return node.label || node.node_type
}

function nodeMeta(node: PanoramaTreeNode) {
  if (node.pid) {
    const parts: string[] = []
    if (node.ppid !== undefined) parts.push(`PPID ${node.ppid}`)
    if (node.process_status) parts.push(t(`agentGuard.processStatus.${node.process_status}`))
    return parts.join(' · ')
  }
  if (node.node_type === 'session' && node.session_confidence === 'inferred') {
    return t('agentGuard.panorama.inferredSessionHint')
  }
  return node.occurred_at || t('agentGuard.panorama.noTimestamp')
}

function hasCompletenessGap(node: PanoramaTreeNode) {
  const collection = node.collection
  return collection?.visibility === 'partial' ||
    collection?.visibility === 'unobservable' ||
    Boolean(collection?.lost_events_since_last) ||
    Boolean(collection?.truncated_fields?.length) ||
    Boolean(collection?.limitations?.length)
}

function hasLimitation(node: PanoramaTreeNode, limitation: string) {
  return node.collection?.limitations?.includes(limitation) ||
    node.trust?.tool_semantics === limitation ||
    node.trust?.remote_visibility === limitation
}

function severityType(severity: string) {
  if (severity === 'critical' || severity === 'high') return 'danger'
  if (severity === 'medium') return 'warning'
  if (severity === 'low') return 'success'
  return 'info'
}
</script>

<style scoped>
.panorama-explorer {
  display: grid;
  grid-template-columns: minmax(360px, 3fr) minmax(280px, 2fr);
  gap: 14px;
  min-height: 420px;
}

.panorama-tree,
.panorama-evidence {
  min-width: 0;
  padding: 12px;
  overflow: auto;
  border: 1px solid var(--aegis-border);
  border-radius: 14px;
  background: var(--aegis-surface, #fff);
}

.panorama-row {
  display: grid;
  grid-template-columns: 24px 34px minmax(0, 1fr) auto auto;
  align-items: center;
  width: 100%;
  min-height: 58px;
  gap: 8px;
  border: 0;
  border-bottom: 1px solid var(--aegis-border);
  background: transparent;
  color: var(--aegis-text);
  text-align: left;
  cursor: pointer;
}

.panorama-row.selected {
  background: var(--el-color-primary-light-9);
}

.expand-button {
  width: 22px;
  height: 22px;
  border: 1px solid var(--aegis-border);
  border-radius: 50%;
  background: #fff;
  cursor: pointer;
}

.node-kind {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: #eef4ff;
  color: var(--el-color-primary);
  font-size: 11px;
  font-weight: 700;
}

.node-kind.large {
  width: 38px;
  height: 38px;
}

.node-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.node-copy strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-copy small,
.evidence-heading p {
  margin: 0;
  color: var(--aegis-text-muted);
}

.evidence-gap {
  color: var(--el-color-warning);
  font-weight: 800;
}

.evidence-heading {
  display: flex;
  align-items: center;
  gap: 10px;
}

.evidence-heading h3 {
  margin: 0 0 4px;
}

.panorama-evidence dl > div {
  display: grid;
  grid-template-columns: 112px minmax(0, 1fr);
  gap: 10px;
  padding: 9px 0;
  border-bottom: 1px solid var(--aegis-border);
}

.panorama-evidence dt {
  color: var(--aegis-text-muted);
}

.panorama-evidence dd {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
}

@media (max-width: 1100px) {
  .panorama-explorer {
    grid-template-columns: 1fr;
  }
}
</style>

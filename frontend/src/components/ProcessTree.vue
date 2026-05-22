<template>
  <div class="process-tree">
    <div v-if="!treeData" class="empty-state">
      <el-empty description="暂无进程树数据" :image-size="88" />
    </div>

    <div v-else class="tree-shell">
      <div class="tree-summary">
        <div>
          <div class="summary-eyebrow">Process Lineage</div>
          <div class="summary-title">进程链路调查视图</div>
        </div>
        <div class="summary-pills" aria-label="进程链路摘要">
          <span v-if="hasParent" class="summary-pill muted">PPID {{ treeData.ppid }}</span>
          <span class="summary-pill danger">PID {{ treeData.pid }}</span>
          <span class="summary-pill">{{ childCount }} 个子进程</span>
        </div>
      </div>

      <div class="timeline" role="list" aria-label="告警进程链路">
        <section v-if="hasParent" class="process-node" role="listitem">
          <div class="node-rail">
            <span class="rail-dot neutral">
              <el-icon><Folder /></el-icon>
            </span>
          </div>
          <article class="node-card parent-card">
            <header class="process-header">
              <div class="process-heading">
                <span class="node-kicker">父进程</span>
                <strong class="process-name">{{ parentProcessName }}</strong>
              </div>
              <el-tag size="small" type="info">PPID {{ treeData.ppid }}</el-tag>
            </header>

            <div class="meta-grid compact-grid">
              <div class="meta-item">
                <span>用户</span>
                <strong>{{ treeData.ppid_user || '-' }}</strong>
              </div>
              <div class="meta-item">
                <span>关系</span>
                <strong>启动来源</strong>
              </div>
            </div>

            <div v-if="treeData.ppid_command_line" class="command-block">
              <span class="command-label">命令行</span>
              <code :title="treeData.ppid_command_line">{{ treeData.ppid_command_line }}</code>
            </div>
          </article>
        </section>

        <section class="process-node threat-node" role="listitem">
          <div class="node-rail">
            <span class="rail-dot danger">
              <el-icon><WarningFilled /></el-icon>
            </span>
          </div>
          <article class="node-card threat-card">
            <header class="process-header">
              <div class="process-heading">
                <span class="node-kicker danger">威胁进程</span>
                <strong class="process-name">{{ processName(treeData.name, treeData.pid) }}</strong>
              </div>
              <el-tag type="danger" size="small" effect="dark">命中告警</el-tag>
            </header>

            <div class="meta-grid">
              <div class="meta-item">
                <span>PID</span>
                <strong>{{ treeData.pid }}</strong>
              </div>
              <div class="meta-item">
                <span>PPID</span>
                <strong>{{ treeData.ppid || '-' }}</strong>
              </div>
              <div class="meta-item">
                <span>用户</span>
                <strong>{{ treeData.user || '-' }}</strong>
              </div>
              <div class="meta-item">
                <span>用户组</span>
                <strong>{{ treeData.user_group || '-' }}</strong>
              </div>
            </div>

            <div v-if="treeData.exe_path" class="command-block">
              <span class="command-label">启动路径</span>
              <code :title="treeData.exe_path">{{ treeData.exe_path }}</code>
            </div>
            <div v-if="treeData.command_line" class="command-block">
              <span class="command-label">命令行</span>
              <code :title="treeData.command_line">{{ treeData.command_line }}</code>
            </div>
          </article>
        </section>

        <section v-if="childCount > 0" class="process-node children-node" role="listitem">
          <div class="node-rail">
            <span class="rail-dot accent">
              <el-icon><Document /></el-icon>
            </span>
          </div>
          <article class="node-card children-card">
            <header class="process-header">
              <div class="process-heading">
                <span class="node-kicker">派生进程</span>
                <strong class="process-name">{{ childCount }} 个子进程</strong>
              </div>
            </header>

            <div class="children-list">
              <article v-for="child in processChildren" :key="child.pid" class="child-card">
                <div class="child-header">
                  <span class="child-name">{{ processName(child.name, child.pid) }}</span>
                  <el-tag size="small">PID {{ child.pid }}</el-tag>
                </div>
                <div v-if="child.user" class="child-user">{{ child.user }}</div>
                <code
                  v-if="child.command_line"
                  class="child-command"
                  :title="child.command_line"
                >
                  {{ truncate(child.command_line, 120) }}
                </code>
              </article>
            </div>
          </article>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Document, Folder, WarningFilled } from '@element-plus/icons-vue'

interface ProcessChild {
  pid: number
  name?: string
  command_line?: string
  user?: string
}

interface ProcessTreeData {
  pid: number
  ppid: number
  name?: string
  command_line?: string
  user?: string
  user_group?: string
  exe_path?: string
  ppid_command_line?: string
  ppid_user?: string
  children?: ProcessChild[]
}

const props = defineProps<{
  processTree?: string | ProcessTreeData | null
}>()

const treeData = ref<ProcessTreeData | null>(null)

const processChildren = computed(() => treeData.value?.children || [])
const childCount = computed(() => treeData.value?.children?.length || 0)
const hasParent = computed(() => Boolean(treeData.value && treeData.value.ppid > 0))
const parentProcessName = computed(() => {
  if (!treeData.value) return ''
  if (!treeData.value.ppid_command_line) return `PID ${treeData.value.ppid}`

  return basename(firstToken(treeData.value.ppid_command_line)) || `PID ${treeData.value.ppid}`
})

const parseProcessTree = () => {
  if (!props.processTree) {
    treeData.value = null
    return
  }

  try {
    const parsed = typeof props.processTree === 'string'
      ? JSON.parse(props.processTree)
      : props.processTree

    treeData.value = isProcessTreeData(parsed) ? parsed : null
  } catch (e) {
    console.error('Failed to parse process tree:', e)
    treeData.value = null
  }
}

function isProcessTreeData(value: unknown): value is ProcessTreeData {
  return Boolean(
    value &&
    typeof value === 'object' &&
    'pid' in value &&
    typeof (value as ProcessTreeData).pid === 'number',
  )
}

function processName(name: string | undefined, pid: number): string {
  return name || `PID ${pid}`
}

function firstToken(value: string): string {
  return value.trim().split(/\s+/)[0] || ''
}

function basename(path: string): string {
  return path.split('/').filter(Boolean).pop() || path
}

function truncate(str: string, len: number): string {
  if (!str) return ''
  return str.length > len ? `${str.slice(0, len)}...` : str
}

onMounted(() => {
  parseProcessTree()
})

watch(() => props.processTree, () => {
  parseProcessTree()
})
</script>

<style scoped>
.process-tree {
  --pt-border: #d9e2ef;
  --pt-border-strong: #b9c9dd;
  --pt-surface: #ffffff;
  --pt-surface-muted: #f6f9fc;
  --pt-text: #1f2937;
  --pt-text-muted: #667085;
  --pt-blue: #2563eb;
  --pt-blue-soft: #eaf1ff;
  --pt-red: #dc2626;
  --pt-red-soft: #fff1f1;
  --pt-amber-soft: #fff8e6;
  margin-top: 12px;
  color: var(--pt-text);
}

.empty-state {
  border: 1px dashed var(--pt-border);
  border-radius: 8px;
  background: var(--pt-surface-muted);
  padding: 18px;
  text-align: center;
}

.tree-shell {
  overflow: hidden;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  background: linear-gradient(180deg, #f8fbff 0%, #ffffff 42%);
}

.tree-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--pt-border);
  padding: 14px 16px;
}

.summary-eyebrow {
  color: var(--pt-blue);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.summary-title {
  margin-top: 2px;
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
}

.summary-pills {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.summary-pill {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  border: 1px solid #c7d7fe;
  border-radius: 999px;
  background: var(--pt-blue-soft);
  color: #1d4ed8;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.2;
  white-space: nowrap;
}

.summary-pill.muted {
  border-color: var(--pt-border);
  background: var(--pt-surface);
  color: var(--pt-text-muted);
}

.summary-pill.danger {
  border-color: #fecaca;
  background: var(--pt-red-soft);
  color: var(--pt-red);
}

.timeline {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 18px 16px 20px;
}

.timeline::before {
  position: absolute;
  top: 20px;
  bottom: 22px;
  left: 32px;
  width: 2px;
  border-radius: 999px;
  background: linear-gradient(180deg, #cbd5e1 0%, #ef4444 48%, #93c5fd 100%);
  content: '';
}

.process-node {
  position: relative;
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  gap: 12px;
}

.node-rail {
  display: flex;
  justify-content: center;
  padding-top: 12px;
}

.rail-dot {
  z-index: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid var(--pt-border-strong);
  border-radius: 999px;
  background: var(--pt-surface);
  color: var(--pt-text-muted);
  box-shadow: 0 0 0 4px #f8fbff;
}

.rail-dot.danger {
  border-color: #fecaca;
  background: var(--pt-red);
  color: #ffffff;
}

.rail-dot.accent {
  border-color: #bfdbfe;
  background: var(--pt-blue-soft);
  color: var(--pt-blue);
}

.node-card {
  min-width: 0;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  background: var(--pt-surface);
  padding: 14px;
  box-shadow: 0 8px 20px rgba(31, 41, 55, 0.06);
  transition: transform 180ms ease, border-color 180ms ease, box-shadow 180ms ease;
}

.node-card:hover {
  border-color: var(--pt-border-strong);
  box-shadow: 0 12px 26px rgba(31, 41, 55, 0.1);
  transform: translateY(-1px);
}

.parent-card {
  background: linear-gradient(180deg, var(--pt-amber-soft) 0%, var(--pt-surface) 60%);
}

.threat-card {
  border-color: #fca5a5;
  background: linear-gradient(180deg, var(--pt-red-soft) 0%, var(--pt-surface) 58%);
  box-shadow: 0 12px 28px rgba(220, 38, 38, 0.14);
}

.process-header,
.child-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.process-heading {
  min-width: 0;
}

.node-kicker {
  display: block;
  margin-bottom: 2px;
  color: var(--pt-text-muted);
  font-size: 12px;
  font-weight: 700;
}

.node-kicker.danger {
  color: var(--pt-red);
}

.process-name {
  display: block;
  color: var(--pt-text);
  font-size: 16px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.meta-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-top: 14px;
}

.compact-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.meta-item {
  min-width: 0;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.72);
  padding: 9px 10px;
}

.meta-item span {
  display: block;
  color: var(--pt-text-muted);
  font-size: 12px;
  line-height: 1.3;
}

.meta-item strong {
  display: block;
  margin-top: 3px;
  color: var(--pt-text);
  font-size: 13px;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.command-block {
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  gap: 10px;
  margin-top: 12px;
  align-items: start;
}

.command-label {
  color: var(--pt-text-muted);
  font-size: 12px;
  font-weight: 700;
  line-height: 26px;
}

.command-block code,
.child-command {
  display: block;
  min-width: 0;
  border: 1px solid #dbe4ef;
  border-radius: 8px;
  background: #f7fafc;
  color: #334155;
  font-family: 'SF Mono', Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  line-height: 1.55;
  overflow-wrap: anywhere;
  padding: 6px 8px;
}

.children-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 10px;
  margin-top: 12px;
}

.child-card {
  min-width: 0;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  background: var(--pt-surface-muted);
  padding: 10px;
}

.child-name {
  min-width: 0;
  color: var(--pt-text);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.child-user {
  margin-top: 4px;
  color: var(--pt-text-muted);
  font-size: 12px;
  line-height: 1.4;
}

.child-command {
  margin-top: 8px;
  background: #ffffff;
}

@media (max-width: 720px) {
  .tree-summary {
    align-items: flex-start;
    flex-direction: column;
  }

  .summary-pills {
    justify-content: flex-start;
  }

  .meta-grid,
  .compact-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .command-block {
    grid-template-columns: 1fr;
    gap: 4px;
  }

  .command-label {
    line-height: 1.4;
  }
}

@media (max-width: 480px) {
  .timeline::before {
    left: 28px;
  }

  .timeline {
    padding-right: 12px;
    padding-left: 12px;
  }

  .process-node {
    grid-template-columns: 32px minmax(0, 1fr);
    gap: 8px;
  }

  .meta-grid,
  .compact-grid,
  .children-list {
    grid-template-columns: 1fr;
  }
}

@media (prefers-reduced-motion: reduce) {
  .node-card {
    transition: none;
  }

  .node-card:hover {
    transform: none;
  }
}
</style>

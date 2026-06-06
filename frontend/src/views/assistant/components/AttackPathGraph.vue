<template>
  <div class="attack-path">
    <div class="section-header">
      <span class="section-title">攻击路径图</span>
      <el-tag v-if="nodes.length" size="small">{{ nodes.length }} 个节点, {{ edges.length }} 条边</el-tag>
    </div>

    <div v-if="!nodes.length" class="empty-hint">
      暂无攻击路径数据
    </div>

    <div v-else class="graph-container">
      <!-- 简化图形渲染：节点列表 + 关系列表 -->
      <div class="nodes-section">
        <div class="sub-title">节点</div>
        <div class="node-list">
          <div
            v-for="node in nodes"
            :key="node.node_id"
            class="node-item"
            :class="node.risk_level"
          >
            <el-tag :type="nodeTypeTag(node.node_type)" size="small" effect="plain">
              {{ nodeTypeLabel(node.node_type) }}
            </el-tag>
            <span class="node-label">{{ node.label }}</span>
            <el-tag :type="riskTag(node.risk_level)" size="small">
              {{ node.risk_level }}
            </el-tag>
            <span v-if="node.evidence_ids?.length" class="node-evidence">
              {{ node.evidence_ids.length }} 条证据
            </span>
          </div>
        </div>
      </div>

      <div class="edges-section">
        <div class="sub-title">关系</div>
        <div class="edge-list">
          <div v-for="edge in edges" :key="`${edge.from}-${edge.to}`" class="edge-item">
            <span class="edge-from">{{ nodeLabel(edge.from) }}</span>
            <el-icon class="edge-arrow"><Right /></el-icon>
            <el-tag size="small" type="info">{{ relationLabel(edge.relation) }}</el-tag>
            <el-icon class="edge-arrow"><Right /></el-icon>
            <span class="edge-to">{{ nodeLabel(edge.to) }}</span>
            <span class="edge-confidence">{{ (edge.confidence * 100).toFixed(0) }}%</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Right } from '@element-plus/icons-vue'

interface PathNode {
  node_id: string
  node_type: string
  label: string
  risk_level: string
  evidence_ids: string[]
}

interface PathEdge {
  from: string
  to: string
  relation: string
  evidence_ids: string[]
  confidence: number
}

const props = defineProps<{
  nodes: PathNode[]
  edges: PathEdge[]
}>()

const nodeMap = computed(() => {
  const map: Record<string, PathNode> = {}
  for (const node of props.nodes) {
    map[node.node_id] = node
  }
  return map
})

function nodeLabel(nodeId: string): string {
  return nodeMap.value[nodeId]?.label || nodeId
}

function nodeTypeTag(type: string): string {
  const map: Record<string, string> = {
    host: 'primary',
    process: 'warning',
    user: 'info',
    ip: 'danger',
    file: 'warning',
    cve: 'danger',
    baseline: 'info',
    alert: 'danger',
  }
  return map[type] || 'info'
}

function nodeTypeLabel(type: string): string {
  const map: Record<string, string> = {
    host: '主机',
    process: '进程',
    user: '用户',
    ip: 'IP',
    file: '文件',
    cve: 'CVE',
    baseline: '基线',
    alert: '告警',
  }
  return map[type] || type
}

function riskTag(level: string): string {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'danger',
    medium: 'warning',
    low: 'info',
    info: 'info',
  }
  return map[level] || 'info'
}

function relationLabel(relation: string): string {
  const map: Record<string, string> = {
    spawned: '生成',
    connected_to: '连接',
    wrote_file: '写入文件',
    exploited: '利用',
    authenticated: '认证',
    triggered: '触发',
  }
  return map[relation] || relation
}
</script>

<style scoped>
.attack-path {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.empty-hint {
  text-align: center;
  color: #909399;
  font-size: 13px;
  padding: 16px;
}

.graph-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.sub-title {
  font-size: 13px;
  font-weight: 600;
  color: #606266;
  margin-bottom: 8px;
}

.node-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.node-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  background: #f5f7fa;
  border-radius: 6px;
  font-size: 13px;
}

.node-item.critical,
.node-item.high {
  border-left: 3px solid #f56c6c;
}

.node-item.medium {
  border-left: 3px solid #e6a23c;
}

.node-label {
  flex: 1;
  font-weight: 500;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-evidence {
  font-size: 12px;
  color: #909399;
}

.edge-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.edge-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: #f5f7fa;
  border-radius: 6px;
  font-size: 13px;
}

.edge-from,
.edge-to {
  font-weight: 500;
  color: #303133;
}

.edge-arrow {
  color: #909399;
  font-size: 12px;
}

.edge-confidence {
  margin-left: auto;
  font-size: 12px;
  color: #909399;
}
</style>

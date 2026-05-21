<template>
  <div class="attack-graph-wrapper">
    <!-- 头部信息 -->
    <div class="graph-header">
      <div class="header-left">
        <h3>{{ graphData.title || '攻击溯源图' }}</h3>
        <el-tag :type="threatLevelType" size="small">
          {{ threatLevelLabel }}
        </el-tag>
      </div>
      <div class="header-right">
        <el-button size="small" @click="resetZoom">
          <el-icon><Refresh /></el-icon>
          重置视图
        </el-button>
        <el-button size="small" @click="fitToScreen">
          <el-icon><FullScreen /></el-icon>
          适应屏幕
        </el-button>
      </div>
    </div>

    <!-- 摘要信息 -->
    <div class="graph-summary">{{ graphData.summary }}</div>

    <!-- D3.js 画布 -->
    <div ref="graphContainer" class="graph-container"></div>

    <!-- 底部信息面板 -->
    <el-row :gutter="16" class="info-panels">
      <!-- 时间线面板 -->
      <el-col :span="12">
        <el-card class="timeline-card">
          <template #header>
            <span>攻击时间线</span>
          </template>
          <el-timeline>
            <el-timeline-item
              v-for="(event, index) in graphData.timeline"
              :key="index"
              :timestamp="formatTime(event.timestamp)"
              placement="top"
              :color="getTimelineColor(event.nodeIds)"
            >
              {{ event.event }}
            </el-timeline-item>
          </el-timeline>
        </el-card>
      </el-col>

      <!-- 处置建议面板 -->
      <el-col :span="12">
        <el-card class="recommendations-card">
          <template #header>
            <span>处置建议</span>
          </template>
          <el-alert
            v-for="(rec, index) in graphData.recommendations"
            :key="index"
            :type="index === 0 ? 'error' : 'warning'"
            :closable="false"
            show-icon
            style="margin-bottom: 8px;"
          >
            <template #title>
              {{ index + 1 }}. {{ rec }}
            </template>
          </el-alert>
        </el-card>
      </el-col>
    </el-row>

    <!-- 节点详情弹窗 -->
    <el-dialog v-model="nodeDetailVisible" title="节点详情" width="600px">
      <el-descriptions v-if="selectedNode" :column="2" border>
        <el-descriptions-item label="节点ID">{{ selectedNode.id }}</el-descriptions-item>
        <el-descriptions-item label="类型">
          <el-tag :type="nodeTypeTag(selectedNode.type)" size="small">
            {{ nodeTypeLabel(selectedNode.type) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="标签" :span="2">{{ selectedNode.label }}</el-descriptions-item>
        <el-descriptions-item label="详情" :span="2">{{ selectedNode.detail }}</el-descriptions-item>
        <el-descriptions-item label="严重程度">
          <el-tag :type="severityTag(selectedNode.severity)" size="small">
            {{ severityLabel(selectedNode.severity) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="属性" :span="2">
          <pre style="margin: 0; font-size: 12px;">{{ JSON.stringify(selectedNode.properties, null, 2) }}</pre>
        </el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="nodeDetailVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 边详情弹窗 -->
    <el-dialog v-model="edgeDetailVisible" title="攻击链路详情" width="500px">
      <el-descriptions v-if="selectedEdge" :column="1" border>
        <el-descriptions-item label="链路ID">{{ selectedEdge.id }}</el-descriptions-item>
        <el-descriptions-item label="链路类型">
          <el-tag type="info">{{ edgeTypeLabel(selectedEdge.type) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="描述">{{ selectedEdge.label }}</el-descriptions-item>
        <el-descriptions-item label="源节点">{{ selectedEdge.source }}</el-descriptions-item>
        <el-descriptions-item label="目标节点">{{ selectedEdge.target }}</el-descriptions-item>
        <el-descriptions-item label="属性">
          <pre style="margin: 0; font-size: 12px;">{{ JSON.stringify(selectedEdge.properties, null, 2) }}</pre>
        </el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="edgeDetailVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, computed, nextTick } from 'vue'
import * as d3 from 'd3'
import { Refresh, FullScreen } from '@element-plus/icons-vue'

// Types
interface GraphNode {
  id: string
  type: string
  label: string
  detail: string
  properties: Record<string, any>
  severity: string
}

interface GraphEdge {
  id: string
  source: string
  target: string
  type: string
  label: string
  properties: Record<string, any>
}

interface TimelineEvent {
  timestamp: string
  event: string
  nodeIds: string[]
}

interface AttackGraph {
  graphId: string
  title: string
  summary: string
  threatLevel: string
  nodes: GraphNode[]
  edges: GraphEdge[]
  timeline: TimelineEvent[]
  recommendations: string[]
}

const props = defineProps<{
  graphData: AttackGraph
}>()

// Refs
const graphContainer = ref<HTMLElement | null>(null)
const nodeDetailVisible = ref(false)
const edgeDetailVisible = ref(false)
const selectedNode = ref<GraphNode | null>(null)
const selectedEdge = ref<GraphEdge | null>(null)

// D3 simulation
let simulation: d3.Simulation<any, any> | null = null
let svg: d3.Selection<SVGSVGElement, unknown, null, undefined> | null = null

// Node colors by type
const nodeColors: Record<string, string> = {
  attacker: '#ff0000',
  victim: '#ff6600',
  process: '#3399ff',
  file: '#ffcc00',
  network: '#9933ff',
  command: '#33cc33',
  malware: '#cc0000'
}

const nodeGlyphs: Record<string, string> = {
  attacker: '攻',
  victim: '靶',
  process: '进',
  file: '文',
  network: '网',
  command: '令',
  malware: '毒'
}

const threatLevelLabels: Record<string, string> = {
  critical: '严重',
  high: '高危',
  medium: '中危',
  low: '低危'
}

const nodeTypeLabels: Record<string, string> = {
  attacker: '攻击源',
  victim: '受害主机',
  process: '进程',
  file: '文件',
  network: '网络',
  command: '命令',
  malware: '恶意载荷'
}

const edgeTypeLabels: Record<string, string> = {
  spawns: '派生',
  connects: '连接',
  reads: '读取',
  writes: '写入',
  executes: '执行',
  downloads: '下载',
  encrypts: '加密',
  exfiltrates: '外传'
}

const severityLabels: Record<string, string> = {
  critical: '严重',
  high: '高危',
  medium: '中危',
  low: '低危',
  info: '信息'
}

// Threat level tag type
const threatLevelType = computed(() => {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'warning',
    medium: 'info',
    low: 'success'
  }
  return map[props.graphData.threatLevel] || 'info'
})

const threatLevelLabel = computed(() => threatLevelLabels[props.graphData.threatLevel] || props.graphData.threatLevel || '未知')

// Helper functions
function severityTag(severity: string): string {
  const map: Record<string, string> = {
    critical: 'danger',
    high: 'warning',
    medium: 'info',
    low: 'success'
  }
  return map[severity] || 'info'
}

function severityLabel(severity: string): string {
  return severityLabels[severity] || severity
}

function nodeTypeTag(type: string): string {
  const map: Record<string, string> = {
    attacker: 'danger',
    victim: 'warning',
    process: 'primary',
    file: 'warning',
    network: 'info',
    command: 'success',
    malware: 'danger'
  }
  return map[type] || 'info'
}

function nodeTypeLabel(type: string): string {
  return nodeTypeLabels[type] || type
}

function edgeTypeLabel(type: string): string {
  return edgeTypeLabels[type] || type
}

function formatTime(timestamp: string): string {
  return new Date(timestamp).toLocaleString('zh-CN')
}

function getTimelineColor(nodeIds: string[]): string {
  if (nodeIds.some(id => id.includes('attacker'))) return '#ff0000'
  if (nodeIds.some(id => id.includes('process') || id.includes('malware'))) return '#ff6600'
  return '#409eff'
}

// Initialize D3 graph
function initGraph() {
  if (!graphContainer.value || !props.graphData.nodes?.length) return

  // Clear previous graph
  d3.select(graphContainer.value).selectAll('*').remove()

  const container = graphContainer.value
  const width = container.clientWidth
  const height = 500

  // Create SVG
  svg = d3.select(container)
    .append('svg')
    .attr('width', width)
    .attr('height', height)
    .attr('viewBox', [0, 0, width, height])
    .attr('font-family', 'var(--aegis-font-sans)')

  // Add zoom behavior
  const g = svg.append('g')

  const zoom = d3.zoom<SVGSVGElement, unknown>()
    .scaleExtent([0.1, 4])
    .on('zoom', (event) => {
      g.attr('transform', event.transform)
    })

  svg.call(zoom)

  // Create arrow markers for edges
  svg.append('defs').selectAll('marker')
    .data(['arrow'])
    .join('marker')
    .attr('id', d => d)
    .attr('viewBox', '0 -5 10 10')
    .attr('refX', 25)
    .attr('refY', 0)
    .attr('markerWidth', 6)
    .attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path')
    .attr('fill', '#999')
    .attr('d', 'M0,-5L10,0L0,5')

  // Prepare data - create deep copies to avoid mutation
  // Normalize node id field (some LLM outputs use different id formats)
  const nodes = props.graphData.nodes.map(d => ({ ...d }))
  // Normalize edge source/target (LLM uses from/to, D3 needs source/target)
  const edges = props.graphData.edges.map(d => ({
    ...d,
    source: (d as any).source || (d as any).from,
    target: (d as any).target || (d as any).to,
  }))

  // Create links
  const link = g.append('g')
    .selectAll('line')
    .data(edges)
    .join('line')
    .attr('stroke', '#999')
    .attr('stroke-opacity', 0.6)
    .attr('stroke-width', 2)
    .attr('marker-end', 'url(#arrow)')
    .attr('cursor', 'pointer')
    .on('click', (event, d) => {
      selectedEdge.value = d as GraphEdge
      edgeDetailVisible.value = true
    })

  // Create edge labels
  const edgeLabel = g.append('g')
    .selectAll('text')
    .data(edges)
    .join('text')
    .attr('font-size', 10)
    .attr('fill', '#666')
    .attr('text-anchor', 'middle')
    .text(d => d.label)

  // Create node groups
  const node = g.append('g')
    .selectAll('g')
    .data(nodes)
    .join('g')
    .attr('cursor', 'pointer')
    .call(d3.drag<any, any>()
      .on('start', dragstarted)
      .on('drag', dragged)
      .on('end', dragended) as any)

  // Node circles
  node.append('circle')
    .attr('r', 20)
    .attr('fill', d => nodeColors[d.type] || '#999')
    .attr('stroke', '#fff')
    .attr('stroke-width', 2)

  // Node type glyphs avoid missing color-emoji fonts in Linux screenshot environments.
  node.append('text')
    .attr('text-anchor', 'middle')
    .attr('dy', 5)
    .attr('font-size', 14)
    .attr('font-weight', 700)
    .attr('fill', '#fff')
    .text(d => nodeGlyphs[d.type] || '?')

  // Node labels (below circle)
  node.append('text')
    .attr('dy', 40)
    .attr('text-anchor', 'middle')
    .attr('font-size', 11)
    .attr('fill', '#333')
    .text(d => d.label.length > 20 ? d.label.substring(0, 20) + '...' : d.label)

  // Node click handler
  node.on('click', (event, d) => {
    selectedNode.value = d as GraphNode
    nodeDetailVisible.value = true
  })

  // Create simulation
  simulation = d3.forceSimulation(nodes as any)
    .force('link', d3.forceLink(edges as any)
      .id((d: any) => d.id)
      .distance(150))
    .force('charge', d3.forceManyBody().strength(-400))
    .force('center', d3.forceCenter(width / 2, height / 2))
    .force('collision', d3.forceCollide().radius(50))
    .on('tick', () => {
      link
        .attr('x1', (d: any) => d.source.x)
        .attr('y1', (d: any) => d.source.y)
        .attr('x2', (d: any) => d.target.x)
        .attr('y2', (d: any) => d.target.y)

      node.attr('transform', (d: any) => `translate(${d.x},${d.y})`)

      edgeLabel
        .attr('x', (d: any) => (d.source.x + d.target.x) / 2)
        .attr('y', (d: any) => (d.source.y + d.target.y) / 2)
    })

  // Drag functions
  function dragstarted(event: any) {
    if (!event.active && simulation) simulation.alphaTarget(0.3).restart()
    event.subject.fx = event.subject.x
    event.subject.fy = event.subject.y
  }

  function dragged(event: any) {
    event.subject.fx = event.x
    event.subject.fy = event.y
  }

  function dragended(event: any) {
    if (!event.active && simulation) simulation.alphaTarget(0)
    event.subject.fx = null
    event.subject.fy = null
  }
}

// Zoom controls
function resetZoom() {
  if (svg && graphContainer.value) {
    svg.transition().duration(750).call(
      (d3.zoom() as any).transform,
      d3.zoomIdentity
    )
  }
}

function fitToScreen() {
  if (svg && graphContainer.value) {
    const width = graphContainer.value.clientWidth
    const height = 500
    svg.transition().duration(750).call(
      (d3.zoom() as any).transform,
      d3.zoomIdentity.translate(width / 2, height / 2).scale(0.8)
    )
  }
}

// Lifecycle
onMounted(() => {
  nextTick(() => {
    initGraph()
  })
})

watch(() => props.graphData, () => {
  nextTick(() => {
    initGraph()
  })
}, { deep: true })
</script>

<style scoped>
.attack-graph-wrapper {
  background: #f5f7fa;
  border-radius: 8px;
  padding: 16px;
}

.graph-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h3 {
  margin: 0;
}

.header-right {
  display: flex;
  gap: 8px;
}

.graph-summary {
  color: #666;
  margin-bottom: 16px;
  padding: 12px;
  background: white;
  border-radius: 4px;
  font-size: 14px;
}

.graph-container {
  background: white;
  border-radius: 8px;
  min-height: 500px;
  border: 1px solid #e4e7ed;
  margin-bottom: 16px;
}

.info-panels {
  margin-top: 16px;
}

.timeline-card,
.recommendations-card {
  height: 100%;
}

.timeline-card :deep(.el-card__header),
.recommendations-card :deep(.el-card__header) {
  padding: 12px 16px;
  background: #f5f7fa;
}

.timeline-card :deep(.el-card__body),
.recommendations-card :deep(.el-card__body) {
  max-height: 250px;
  overflow-y: auto;
}
</style>

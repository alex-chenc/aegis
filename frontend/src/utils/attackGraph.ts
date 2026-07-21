export interface AttackGraphNode {
  id: string
  type: string
  label: string
  detail: string
  properties: Record<string, any>
  severity: string
}

export interface AttackGraphEdge {
  id: string
  source: string
  target: string
  type: string
  label: string
  properties: Record<string, any>
}

export interface AttackGraphTimelineEvent {
  timestamp: string
  event: string
  nodeIds: string[]
}

export interface AttackGraphData {
  graphId: string
  title: string
  summary: string
  threatLevel: string
  nodes: AttackGraphNode[]
  edges: AttackGraphEdge[]
  timeline: AttackGraphTimelineEvent[]
  recommendations: string[]
}

export interface AttackGraphConclusion {
  alert_id?: string
  action?: string
  summary?: string
}

export interface AttackGraphFinalAnswer {
  graph: AttackGraphData
  conclusions: AttackGraphConclusion[]
}

function stripMarkdownFences(content: string): string {
  return content.replace(/```(?:json)?/gi, '').replace(/```/g, '')
}

function collectJSONObjectCandidates(content: string): string[] {
  const candidates: string[] = []
  let depth = 0
  let start = -1
  let inString = false
  let escaped = false

  for (let i = 0; i < content.length; i++) {
    const ch = content[i]

    if (inString) {
      if (escaped) {
        escaped = false
      } else if (ch === '\\') {
        escaped = true
      } else if (ch === '"') {
        inString = false
      }
      continue
    }

    if (ch === '"') {
      inString = true
      continue
    }

    if (ch === '{') {
      if (depth === 0) {
        start = i
      }
      depth++
    } else if (ch === '}') {
      depth--
      if (depth === 0 && start >= 0) {
        candidates.push(content.slice(start, i + 1))
        start = -1
      }
    }
  }

  return candidates
}

export function isAttackGraph(value: any): value is AttackGraphData {
  return Boolean(
    value &&
      typeof value === 'object' &&
      Array.isArray(value.nodes) &&
      value.nodes.length > 0 &&
      Array.isArray(value.edges)
  )
}

function normalizeConclusions(value: any): AttackGraphConclusion[] {
  if (!Array.isArray(value)) return []

  return value
    .filter(item => item && typeof item === 'object')
    .map(item => ({
      alert_id: typeof item.alert_id === 'string' ? item.alert_id : undefined,
      action: typeof item.action === 'string' ? item.action : undefined,
      summary: typeof item.summary === 'string' ? item.summary : undefined
    }))
}

export function extractAttackGraphFinalAnswer(content: string): AttackGraphFinalAnswer | null {
  const normalized = stripMarkdownFences(content)
  const candidates = collectJSONObjectCandidates(normalized)

  for (const candidate of candidates) {
    try {
      const parsed = JSON.parse(candidate)
      if (isAttackGraph(parsed.attack_graph)) {
        return {
          graph: parsed.attack_graph,
          conclusions: normalizeConclusions(parsed.conclusions)
        }
      }
      if (isAttackGraph(parsed)) {
        return {
          graph: parsed,
          conclusions: []
        }
      }
    } catch {
      // Keep scanning later candidates.
    }
  }

  return null
}

export function extractAttackGraph(content: string): AttackGraphData | null {
  return extractAttackGraphFinalAnswer(content)?.graph || null
}

export function isLikelyAttackGraphFinalAnswer(content: string): boolean {
  const normalized = stripMarkdownFences(content).trim()
  if (!normalized) return false
  if (normalized.includes('attack_graph')) return true
  if (normalized.startsWith('Final Answer') && normalized.includes('{') && normalized.length < 512) return true
  if (normalized.startsWith('{') && normalized.length < 256) return true
  if (normalized.startsWith('{') && normalized.includes('"nodes"') && normalized.includes('"edges"')) return true
  return false
}

const threatLevelLabelKeys: Record<string, string> = {
  critical: 'analysis.severity.critical', high: 'analysis.severity.high', medium: 'analysis.severity.medium', low: 'analysis.severity.low', info: 'analysis.severity.info',
}

const actionLabelKeys: Record<string, string> = {
  allow: 'analysis.graph.allow', monitor: 'analysis.graph.monitor', investigate: 'analysis.graph.investigate', isolate: 'analysis.graph.isolate', block: 'analysis.graph.block', close: 'analysis.graph.close',
}

function formatList(items: string[], emptyText: string): string {
  if (items.length === 0) return `- ${emptyText}`
  return items.map((item, index) => `${index + 1}. ${item}`).join('\n')
}

export function buildAttackGraphDisplayText(finalAnswer: AttackGraphFinalAnswer): string {
  const { graph, conclusions } = finalAnswer
  const threatLevel = threatLevelLabelKeys[graph.threatLevel] ? translate(threatLevelLabelKeys[graph.threatLevel]) : graph.threatLevel || translate('analysis.severity.unknown')
  const conclusionLines = conclusions.map((item) => {
    const id = item.alert_id ? `${translate('analysis.graph.alert')} ${item.alert_id}` : translate('analysis.graph.alert')
    const action = item.action ? (actionLabelKeys[item.action] ? translate(actionLabelKeys[item.action]) : item.action) : translate('analysis.graph.pendingConfirmation')
    const summary = item.summary || translate('analysis.graph.noConclusion')
    return `${id}: ${summary} (${translate('analysis.graph.disposition', { action })})`
  })

  return [
    translate('analysis.graph.completed', { title: graph.title }),
    translate('analysis.graph.riskLevel', { level: threatLevel }),
    '',
    translate('analysis.graph.summary', { summary: graph.summary || translate('analysis.graph.defaultSummary') }),
    '',
    translate('analysis.graph.alertConclusions'),
    formatList(conclusionLines, translate('analysis.graph.noAlertConclusions')),
    '',
    translate('analysis.graph.recommendations'),
    formatList(graph.recommendations || [], translate('analysis.graph.noRecommendations')),
    '',
    translate('analysis.graph.rendered')
  ].join('\n')
}

function escapeXML(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

function nodeColor(node: AttackGraphNode): string {
  const colors: Record<string, string> = {
    attacker: '#dc2626',
    victim: '#ea580c',
    process: '#2563eb',
    file: '#ca8a04',
    network: '#7c3aed',
    command: '#16a34a',
    malware: '#991b1b'
  }
  return colors[node.type] || '#475569'
}

const SVG_FONT_STACK = '"Noto Sans CJK SC", "Noto Sans SC", "Source Han Sans SC", "Microsoft YaHei", "PingFang SC", "Hiragino Sans GB", "WenQuanYi Micro Hei", "Segoe UI", sans-serif'

export function buildAttackGraphSvgDataUrl(graph: AttackGraphData): string {
  const width = 960
  const height = Math.max(360, 190 + graph.nodes.length * 82)
  const left = 96
  const gap = graph.nodes.length > 1 ? Math.min(230, (width - left * 2) / (graph.nodes.length - 1)) : 0
  const y = 190
  const positions = new Map<string, { x: number; y: number }>()

  graph.nodes.forEach((node, index) => {
    positions.set(node.id, {
      x: left + index * gap,
      y
    })
  })

  const edgeSvg = graph.edges.map((edge) => {
    const source = positions.get(edge.source)
    const target = positions.get(edge.target)
    if (!source || !target) return ''

    const midX = (source.x + target.x) / 2
    const midY = source.y - 36
    return `
      <path d="M ${source.x + 52} ${source.y} C ${midX} ${midY}, ${midX} ${midY}, ${target.x - 52} ${target.y}" fill="none" stroke="#334155" stroke-width="2.5" marker-end="url(#arrow)" />
      <text x="${midX}" y="${midY - 8}" text-anchor="middle" class="edge-label">${escapeXML(edge.label || edge.type)}</text>
    `
  }).join('')

  const nodeSvg = graph.nodes.map((node) => {
    const pos = positions.get(node.id)!
    return `
      <g transform="translate(${pos.x}, ${pos.y})">
        <circle r="44" fill="${nodeColor(node)}" />
        <text y="-5" text-anchor="middle" class="node-label">${escapeXML(node.label).slice(0, 18)}</text>
        <text y="17" text-anchor="middle" class="node-type">${escapeXML(node.type)}</text>
      </g>
    `
  }).join('')

  const timelineSvg = graph.timeline.slice(0, 5).map((item, index) => {
    const rowY = 285 + index * 38
    return `
      <g>
        <circle cx="112" cy="${rowY - 5}" r="5" fill="#0f766e" />
        <text x="132" y="${rowY}" class="timeline">${escapeXML(item.event)}</text>
      </g>
    `
  }).join('')

  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeXML(graph.title)}">
      <defs>
        <marker id="arrow" markerWidth="12" markerHeight="12" refX="9" refY="3" orient="auto" markerUnits="strokeWidth">
          <path d="M0,0 L0,6 L9,3 z" fill="#334155" />
        </marker>
        <style>
          .title { font: 700 28px ${SVG_FONT_STACK}; fill: #0f172a; }
          .summary { font: 400 15px ${SVG_FONT_STACK}; fill: #475569; }
          .node-label { font: 700 13px ${SVG_FONT_STACK}; fill: #ffffff; }
          .node-type { font: 500 11px ${SVG_FONT_STACK}; fill: rgba(255,255,255,.82); }
          .edge-label { font: 600 12px ${SVG_FONT_STACK}; fill: #1e293b; }
          .timeline-title { font: 700 16px ${SVG_FONT_STACK}; fill: #0f172a; }
          .timeline { font: 400 13px ${SVG_FONT_STACK}; fill: #334155; }
        </style>
      </defs>
      <rect width="100%" height="100%" rx="0" fill="#f8fafc" />
      <text x="48" y="52" class="title">${escapeXML(graph.title)}</text>
      <text x="48" y="82" class="summary">${escapeXML(graph.summary || '')}</text>
      <text x="48" y="118" class="timeline-title">${escapeXML(translate('analysis.graph.attackFlow'))}</text>
      ${edgeSvg}
      ${nodeSvg}
      <text x="48" y="258" class="timeline-title">${escapeXML(translate('analysis.graph.timeline'))}</text>
      ${timelineSvg}
    </svg>
  `.trim()

  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
}
import { translate } from '@/i18n'

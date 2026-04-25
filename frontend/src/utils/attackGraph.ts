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

function isAttackGraph(value: any): value is AttackGraphData {
  return Boolean(
    value &&
      typeof value === 'object' &&
      Array.isArray(value.nodes) &&
      Array.isArray(value.edges) &&
      typeof value.title === 'string'
  )
}

export function extractAttackGraph(content: string): AttackGraphData | null {
  const normalized = stripMarkdownFences(content)
  const candidates = collectJSONObjectCandidates(normalized)

  for (const candidate of candidates) {
    try {
      const parsed = JSON.parse(candidate)
      if (isAttackGraph(parsed.attack_graph)) {
        return parsed.attack_graph
      }
      if (isAttackGraph(parsed)) {
        return parsed
      }
    } catch {
      // Keep scanning later candidates.
    }
  }

  return null
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
          .title { font: 700 28px ui-sans-serif, system-ui, sans-serif; fill: #0f172a; }
          .summary { font: 400 15px ui-sans-serif, system-ui, sans-serif; fill: #475569; }
          .node-label { font: 700 13px ui-sans-serif, system-ui, sans-serif; fill: #ffffff; }
          .node-type { font: 500 11px ui-sans-serif, system-ui, sans-serif; fill: rgba(255,255,255,.82); }
          .edge-label { font: 600 12px ui-sans-serif, system-ui, sans-serif; fill: #1e293b; }
          .timeline-title { font: 700 16px ui-sans-serif, system-ui, sans-serif; fill: #0f172a; }
          .timeline { font: 400 13px ui-sans-serif, system-ui, sans-serif; fill: #334155; }
        </style>
      </defs>
      <rect width="100%" height="100%" rx="0" fill="#f8fafc" />
      <text x="48" y="52" class="title">${escapeXML(graph.title)}</text>
      <text x="48" y="82" class="summary">${escapeXML(graph.summary || '')}</text>
      <text x="48" y="118" class="timeline-title">攻击流程</text>
      ${edgeSvg}
      ${nodeSvg}
      <text x="48" y="258" class="timeline-title">关键时间线</text>
      ${timelineSvg}
    </svg>
  `.trim()

  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
}

import { describe, expect, it } from 'vitest'
import { buildAttackGraphSvgDataUrl, extractAttackGraph } from './attackGraph'

const graph = {
  graphId: 'graph-test',
  title: '反弹 Shell 攻击链路',
  summary: '攻击者通过 bash 建立外连并落地脚本',
  threatLevel: 'high',
  nodes: [
    {
      id: 'process_1',
      type: 'process',
      label: 'bash',
      detail: '/bin/bash -i',
      properties: {},
      severity: 'high'
    },
    {
      id: 'network_1',
      type: 'network',
      label: '203.0.113.10:4444',
      detail: '外连地址',
      properties: {},
      severity: 'high'
    }
  ],
  edges: [
    {
      id: 'edge_1',
      source: 'process_1',
      target: 'network_1',
      type: 'connects',
      label: '外连',
      properties: {}
    }
  ],
  timeline: [
    {
      timestamp: '2026-04-24T15:00:00Z',
      event: 'bash 发起外连',
      nodeIds: ['process_1', 'network_1']
    }
  ],
  recommendations: ['隔离主机', '终止 bash 进程']
}

describe('attack graph output', () => {
  it('extracts attack_graph from markdown fenced final answer', () => {
    const content = [
      'Final Answer:',
      '```json',
      JSON.stringify({ attack_graph: graph, conclusions: [] }, null, 2),
      '```'
    ].join('\n')

    const result = extractAttackGraph(content)

    expect(result?.graphId).toBe('graph-test')
    expect(result?.nodes).toHaveLength(2)
    expect(result?.edges[0].label).toBe('外连')
  })

  it('builds a displayable svg data url from attack_graph', () => {
    const dataUrl = buildAttackGraphSvgDataUrl(graph)

    expect(dataUrl).toMatch(/^data:image\/svg\+xml;charset=utf-8,/)
    expect(decodeURIComponent(dataUrl)).toContain('<svg')
    expect(decodeURIComponent(dataUrl)).toContain('bash')
    expect(decodeURIComponent(dataUrl)).toContain('外连')
  })

  it('embeds a CJK-safe font stack in generated svg flowcharts', () => {
    const decoded = decodeURIComponent(buildAttackGraphSvgDataUrl(graph))

    expect(decoded).toContain('Noto Sans CJK SC')
    expect(decoded).toContain('WenQuanYi Micro Hei')
  })
})

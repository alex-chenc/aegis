import { describe, expect, it } from 'vitest'
import { buildInitialAnalysisMessage, normalizeAIAnalysisErrorMessage } from './aiAnalysisView'

describe('aiAnalysisView helpers', () => {
  it('builds initial analysis message from real alert snapshots', () => {
    const message = buildInitialAnalysisMessage(
      [
        {
          id: 'internal-1',
          hostname: 'host-a',
          rule_title: '可疑进程执行',
          severity: 'high',
          description: 'bash 启动异常子进程',
          last_seen_at: '2026-04-28T01:00:00Z'
        },
        {
          id: 'internal-2',
          hostname: 'host-b',
          rule_title: '异常网络外联',
          severity: 'critical',
          description: '进程向未知外网地址发起连接',
          last_seen_at: '2026-04-28T01:05:00Z'
        }
      ],
      ['2026-04-28T01:00:00Z', '2026-04-28T02:00:00Z']
    )

    expect(message).toContain('本次需要分析以下 2 条真实告警')
    expect(message).toContain('主机：host-a')
    expect(message).toContain('规则：可疑进程执行')
    expect(message).toContain('主机：host-b')
    expect(message).toContain('规则：异常网络外联')
  })

  it('normalizes max-iteration error to a readable Chinese message', () => {
    expect(normalizeAIAnalysisErrorMessage('stream error: Maximum iterations reached without final answer')).toBe(
      'AI 已达到最大推理轮数，但仍未生成最终结论。请缩小告警范围、补充问题，或提高最大轮数后重试。'
    )
  })
})

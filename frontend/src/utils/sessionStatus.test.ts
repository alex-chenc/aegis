import { describe, expect, it } from 'vitest'

// 状态判定函数
function getDisplayStatus(session: { conclusion?: Record<string, any> | null }): 'completed' | 'active' {
  if (session.conclusion && Object.keys(session.conclusion).length > 0) {
    return 'completed'
  }
  return 'active'
}

// 判断是否为误报
function isFalsePositive(verdict: string): boolean {
  return verdict === 'benign' || verdict === 'false_positive'
}

// 获取处置建议
function getRemediationSuggestion(verdict: string): string {
  switch (verdict) {
    case 'malicious':
      return '建议立即隔离受影响主机，进行深入取证分析，并检查横向移动迹象。'
    case 'suspicious':
      return '建议进一步监控相关进程和网络活动，收集更多证据以确认威胁。'
    case 'unknown':
      return '建议人工复核分析结果，结合上下文信息进行判断。'
    default:
      return '建议根据实际情况采取相应措施。'
  }
}

// 获取结论显示类型
function getVerdictType(verdict: string): 'success' | 'danger' | 'warning' | 'info' {
  switch (verdict) {
    case 'benign':
    case 'false_positive':
      return 'success'
    case 'malicious':
      return 'danger'
    case 'suspicious':
      return 'warning'
    default:
      return 'info'
  }
}

// 获取结论显示文本
function getVerdictText(verdict: string): string {
  switch (verdict) {
    case 'benign':
    case 'false_positive':
      return '良性/误报'
    case 'malicious':
      return '恶意'
    case 'suspicious':
      return '可疑'
    default:
      return '未知'
  }
}

describe('getDisplayStatus', () => {
  it('有结论应返回completed', () => {
    const session = { conclusion: { verdict: 'benign', summary: '良性活动' } }
    expect(getDisplayStatus(session)).toBe('completed')
  })

  it('无结论应返回active', () => {
    const session = { conclusion: undefined }
    expect(getDisplayStatus(session)).toBe('active')
  })

  it('null结论应返回active', () => {
    const session = { conclusion: null }
    expect(getDisplayStatus(session)).toBe('active')
  })

  it('空结论对象应返回active', () => {
    const session = { conclusion: {} }
    expect(getDisplayStatus(session)).toBe('active')
  })

  it('有恶意结论应返回completed', () => {
    const session = { conclusion: { verdict: 'malicious', summary: '恶意活动' } }
    expect(getDisplayStatus(session)).toBe('completed')
  })

  it('有可疑结论应返回completed', () => {
    const session = { conclusion: { verdict: 'suspicious', summary: '可疑活动' } }
    expect(getDisplayStatus(session)).toBe('completed')
  })
})

describe('isFalsePositive', () => {
  it('benign应返回true', () => {
    expect(isFalsePositive('benign')).toBe(true)
  })

  it('false_positive应返回true', () => {
    expect(isFalsePositive('false_positive')).toBe(true)
  })

  it('malicious应返回false', () => {
    expect(isFalsePositive('malicious')).toBe(false)
  })

  it('suspicious应返回false', () => {
    expect(isFalsePositive('suspicious')).toBe(false)
  })

  it('unknown应返回false', () => {
    expect(isFalsePositive('unknown')).toBe(false)
  })

  it('空字符串应返回false', () => {
    expect(isFalsePositive('')).toBe(false)
  })
})

describe('getRemediationSuggestion', () => {
  it('malicious应返回隔离建议', () => {
    const suggestion = getRemediationSuggestion('malicious')
    expect(suggestion).toContain('隔离')
    expect(suggestion).toContain('取证分析')
  })

  it('suspicious应返回监控建议', () => {
    const suggestion = getRemediationSuggestion('suspicious')
    expect(suggestion).toContain('监控')
    expect(suggestion).toContain('证据')
  })

  it('unknown应返回复核建议', () => {
    const suggestion = getRemediationSuggestion('unknown')
    expect(suggestion).toContain('复核')
    expect(suggestion).toContain('上下文')
  })

  it('其他值应返回通用建议', () => {
    const suggestion = getRemediationSuggestion('other')
    expect(suggestion).toContain('实际情况')
  })
})

describe('getVerdictType', () => {
  it('benign应返回success', () => {
    expect(getVerdictType('benign')).toBe('success')
  })

  it('false_positive应返回success', () => {
    expect(getVerdictType('false_positive')).toBe('success')
  })

  it('malicious应返回danger', () => {
    expect(getVerdictType('malicious')).toBe('danger')
  })

  it('suspicious应返回warning', () => {
    expect(getVerdictType('suspicious')).toBe('warning')
  })

  it('unknown应返回info', () => {
    expect(getVerdictType('unknown')).toBe('info')
  })
})

describe('getVerdictText', () => {
  it('benign应返回良性/误报', () => {
    expect(getVerdictText('benign')).toBe('良性/误报')
  })

  it('false_positive应返回良性/误报', () => {
    expect(getVerdictText('false_positive')).toBe('良性/误报')
  })

  it('malicious应返回恶意', () => {
    expect(getVerdictText('malicious')).toBe('恶意')
  })

  it('suspicious应返回可疑', () => {
    expect(getVerdictText('suspicious')).toBe('可疑')
  })

  it('unknown应返回未知', () => {
    expect(getVerdictText('unknown')).toBe('未知')
  })
})

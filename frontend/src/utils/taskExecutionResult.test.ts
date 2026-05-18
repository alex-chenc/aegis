import { describe, expect, it } from 'vitest'
import { parseExecutionResultText, normalizeExecutionResult, normalizeVerdict } from './taskExecutionResult'

const sampleText = `Task status: completed
Exit reason: normal_completed
Completed step_1: Process 4181522 (base64 -d) has exited.
Completed step_2: 经分析，目标进程已退出且未留下任何活跃文件句柄。
Completed step_3: Benign / False Positive
Errors: open /proc/4181522/stat: no such file or directory; process 4181522 not found`

describe('task execution result helpers', () => {
  it('parses agent-runtime final text into a structured Chinese execution result', () => {
    const result = parseExecutionResultText(sampleText)

    expect(result).not.toBeNull()
    expect(result?.status).toBe('已完成')
    expect(result?.exit_reason).toBe('正常完成')
    expect(result?.steps).toHaveLength(3)
    expect(result?.steps?.[0]).toMatchObject({
      step_id: 'step_1',
      status: '已完成',
      result: 'Process 4181522 (base64 -d) has exited.'
    })
    expect(result?.steps?.[2].result).toBe('良性/误报')
    expect(result?.errors).toEqual([
      'open /proc/4181522/stat: no such file or directory',
      'process 4181522 not found'
    ])
    expect(result?.conclusion).toMatchObject({
      verdict: 'benign',
      summary: '良性/误报'
    })
  })

  it('returns null for ordinary assistant text that is not an execution result', () => {
    expect(parseExecutionResultText('这是普通分析回复，不包含任务执行结果字段。')).toBeNull()
  })

  it('normalizes raw API enum values and creates a Chinese fallback conclusion', () => {
    const result = normalizeExecutionResult({
      execution_id: 'exec-1',
      task_id: 'task-1',
      session_id: 'session-1',
      status: 'failed',
      exit_reason: 'tool_failed',
      started_at: '',
      ended_at: '',
      total_duration_ms: 0,
      steps: [],
      errors: ['tool timeout'],
      conclusion: {
        verdict: '',
        summary: '',
        reasoning: ''
      }
    })

    expect(result.status).toBe('执行失败')
    expect(result.exit_reason).toBe('工具执行失败')
    expect(result.conclusion).toMatchObject({
      verdict: 'unknown',
      summary: '执行失败，无法形成可靠安全结论'
    })
  })

  describe('normalizeVerdict severity ordering', () => {
    it('returns malicious when text contains both 可疑 and 恶意 (highest severity wins)', () => {
      // Backend uses severity ordering: malicious > suspicious > benign
      // Frontend must be consistent
      expect(normalizeVerdict('可疑文件系统路径和恶意行为')).toBe('malicious')
    })

    it('returns malicious for 恶意 keyword', () => {
      expect(normalizeVerdict('恶意')).toBe('malicious')
    })

    it('returns suspicious for 可疑 keyword alone', () => {
      expect(normalizeVerdict('可疑文件系统路径')).toBe('suspicious')
    })

    it('returns malicious when text contains threat keyword', () => {
      expect(normalizeVerdict('This is a confirmed threat')).toBe('malicious')
    })

    it('returns malicious for potentially malicious (contains malicious substring)', () => {
      expect(normalizeVerdict('potentially malicious behavior detected')).toBe('malicious')
    })

    it('returns benign for 误报 keyword', () => {
      expect(normalizeVerdict('误报')).toBe('benign')
    })

    it('returns unknown for text without verdict keywords', () => {
      expect(normalizeVerdict('普通分析文本')).toBe('unknown')
    })
  })

  it('preserves API verdict when step results contain 可疑 as substring', () => {
    // Simulates the real bug: step results contain "可疑文件系统路径" (suspicious file system path)
    // but the API verdict is "malicious". The API verdict should win.
    const result = normalizeExecutionResult({
      session_id: 'test-session',
      status: 'completed',
      conclusion: {
        verdict: 'malicious',
        summary: '恶意',
        reasoning: ''
      },
      steps: [
        {
          step_id: 'step_1',
          status: 'completed',
          result: '目标进程及其父进程未访问任何可疑文件系统路径',
          started_at: '',
          ended_at: '',
          duration_ms: 0
        },
        {
          step_id: 'step_2',
          status: 'completed',
          result: '经分析，目标主机存在显著的 SSH 暴力破解痕迹',
          started_at: '',
          ended_at: '',
          duration_ms: 0
        }
      ],
      errors: []
    })

    expect(result.conclusion.verdict).toBe('malicious')
    expect(result.conclusion.summary).toBe('恶意')
  })
})

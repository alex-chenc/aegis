import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

describe('TaskDetail automatic repair workflow', () => {
  const source = readFileSync(resolve(__dirname, 'TaskDetail.vue'), 'utf-8')

  it('removes manual repair and redispatch operations', () => {
    expect(source).not.toContain('@click="triggerScriptRepair(row)"')
    expect(source).not.toContain('@click="reExecute(row)"')
    expect(source).not.toContain('@click="openSuggestionDialog(row)"')
    expect(source).not.toContain('title="修复建议"')
    expect(source).not.toContain('triggerSelfHealing')
    expect(source).not.toContain('redispatchTask')
  })

  it('keeps the shared automatic repair process visible', () => {
    expect(source).toContain('自动修复过程')
    expect(source).toContain('task.healingStatus?.steps?.length')
    expect(source).toContain('当前轮次')
    expect(source).toContain('healingStepStatusText(step.status)')
  })
})

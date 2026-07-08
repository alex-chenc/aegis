// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { escapeCsvField, buildCsv, downloadCsv } from './csv'

describe('escapeCsvField', () => {
  it('leaves plain fields untouched', () => {
    expect(escapeCsvField('hello')).toBe('hello')
    expect(escapeCsvField(123)).toBe('123')
    expect(escapeCsvField('')).toBe('')
  })

  it('returns empty string for null/undefined', () => {
    expect(escapeCsvField(null)).toBe('')
    expect(escapeCsvField(undefined)).toBe('')
  })

  it('quotes fields containing a comma', () => {
    expect(escapeCsvField('a,b')).toBe('"a,b"')
  })

  it('quotes fields containing a double quote and doubles it', () => {
    expect(escapeCsvField('he said "hi"')).toBe('"he said ""hi"""')
  })

  it('quotes fields containing a newline', () => {
    expect(escapeCsvField('line1\nline2')).toBe('"line1\nline2"')
  })
})

describe('buildCsv', () => {
  it('joins headers and rows with CRLF and escapes special chars', () => {
    const csv = buildCsv(
      ['name', 'note'],
      [['alice', 'ok'], ['bob', 'has, comma']]
    )
    expect(csv).toBe('name,note\r\nalice,ok\r\nbob,"has, comma"')
  })

  it('handles empty rows', () => {
    expect(buildCsv(['a', 'b'], [])).toBe('a,b')
  })
})

describe('downloadCsv', () => {
  const createObjectUrlSpy = vi.fn(() => 'blob:mock')
  const revokeObjectUrlSpy = vi.fn()

  beforeEach(() => {
    // jsdom does not implement URL.createObjectURL / revokeObjectURL.
    vi.stubGlobal('URL', {
      createObjectURL: createObjectUrlSpy,
      revokeObjectURL: revokeObjectUrlSpy,
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('creates an anchor with the given filename and triggers a click', () => {
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click')
    const appendSpy = vi.spyOn(document.body, 'appendChild')
    const removeSpy = vi.spyOn(document.body, 'removeChild')

    downloadCsv('test.csv', 'a,b\r\n1,2')

    // The created <a> is appended, configured and clicked, then removed.
    expect(appendSpy).toHaveBeenCalled()
    const anchor = appendSpy.mock.calls[0][0] as HTMLAnchorElement
    expect(anchor.tagName).toBe('A')
    expect(anchor.download).toBe('test.csv')
    expect(anchor.href.startsWith('blob:')).toBe(true)
    expect(clickSpy).toHaveBeenCalled()
    expect(removeSpy).toHaveBeenCalled()
    expect(createObjectUrlSpy).toHaveBeenCalled()
    expect(revokeObjectUrlSpy).toHaveBeenCalled()

    clickSpy.mockRestore()
    appendSpy.mockRestore()
    removeSpy.mockRestore()
  })
})

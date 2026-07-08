/**
 * Minimal CSV helpers for client-side export.
 *
 * - `escapeCsvField` quotes fields containing comma, double-quote or newline,
 *   and doubles embedded quotes (RFC 4180).
 * - `buildCsv` joins headers + rows with CRLF.
 * - `downloadCsv` triggers a browser download with a UTF-8 BOM so Excel
 *   renders Chinese characters correctly.
 */

export function escapeCsvField(value: unknown): string {
  if (value === null || value === undefined) return ''
  const s = String(value)
  if (/[",\n\r]/.test(s)) {
    return '"' + s.replace(/"/g, '""') + '"'
  }
  return s
}

export function buildCsv(headers: string[], rows: (string | number)[][]): string {
  const lines = [headers.map(escapeCsvField).join(',')]
  for (const row of rows) {
    lines.push(row.map(escapeCsvField).join(','))
  }
  return lines.join('\r\n')
}

export function downloadCsv(filename: string, csv: string): void {
  // Prepend UTF-8 BOM for correct Chinese rendering in Excel.
  const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

/**
 * Vulnerability workbench shell-readiness ranking.
 * Order: 已生成(generated) > 生成中(generating) > 未生成/失败(none/pending/failed).
 */

export type ShellGenerationStatus =
  | 'generated'
  | 'generating'
  | 'none'
  | 'pending'
  | 'failed'
  | string

/** Aggregate CVE list rank: 0 generated, 1 generating, 2 none/other */
export function shellGenerationStatusRank(status?: ShellGenerationStatus | null): number {
  const s = String(status || '').toLowerCase()
  if (s === 'generated') return 0
  if (s === 'generating') return 1
  return 2
}

export function compareByShellGenerationStatus(
  a?: ShellGenerationStatus | null,
  b?: ShellGenerationStatus | null
): number {
  return shellGenerationStatusRank(a) - shellGenerationStatusRank(b)
}

/**
 * Host-row detailed rank for dialog tables:
 * generated(0) > generating(1) > failed(2) > pending/other(3)
 */
export function hostShellGenerationRank(status?: ShellGenerationStatus | null): number {
  const s = String(status || '').toLowerCase()
  if (s === 'generated') return 0
  if (s === 'generating') return 1
  if (s === 'failed') return 2
  return 3
}

export interface HostShellStatusLike {
  generation_status?: ShellGenerationStatus
  host_id?: string
}

export function compareHostShellRows(a: HostShellStatusLike, b: HostShellStatusLike): number {
  const rankDiff = hostShellGenerationRank(a.generation_status) - hostShellGenerationRank(b.generation_status)
  if (rankDiff !== 0) return rankDiff
  return String(a.host_id || '').localeCompare(String(b.host_id || ''))
}

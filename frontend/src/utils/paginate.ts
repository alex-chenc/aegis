/**
 * Slice an array into a single page view.
 *
 * Used by the baseline Workbench rule list to paginate rules per check-rule
 * set (template group) on the client side. Mirrors the existing "all view"
 * pagination behaviour so both views stay consistent.
 *
 * @param items    full (already filtered) list
 * @param page     one-based page number; values < 1 are treated as page 1
 * @param pageSize items per page; <= 0 returns the whole list unchanged
 */
export function paginate<T>(items: T[], page: number, pageSize: number): T[] {
  if (pageSize <= 0) return items
  const total = items.length
  if (total === 0) return []
  const current = page < 1 ? 1 : page
  const start = (current - 1) * pageSize
  if (start >= total) {
    // Out-of-range page: clamp to the last available page.
    const lastPage = Math.ceil(total / pageSize)
    const clampedStart = (lastPage - 1) * pageSize
    return items.slice(clampedStart)
  }
  return items.slice(start, start + pageSize)
}

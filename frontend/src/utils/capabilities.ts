// Capability snapshots are process-memory only. They are refreshed from
// /auth/login or /auth/me and never persisted with the token.
let snapshot: { values: string[]; version: number; expiresAt: number } | null = null

export function setCapabilitySnapshot(values: string[] | undefined, version = 1, ttlMs = 15 * 60 * 1000) {
  snapshot = values ? { values: [...new Set(values)], version, expiresAt: Date.now() + ttlMs } : null
}

export function getCapabilitySnapshot(): string[] | null {
  if (!snapshot || snapshot.expiresAt <= Date.now()) {
    snapshot = null
    return null
  }
  return [...snapshot.values]
}

export function canCapability(permission: string): boolean {
  const values = getCapabilitySnapshot()
  return values === null || values.includes(permission)
}

export function clearCapabilitySnapshot() {
  snapshot = null
}

import type { AuthSession } from '@/api/auth'
import { clearCapabilitySnapshot, setCapabilitySnapshot } from '@/utils/capabilities'

const AUTH_STORAGE_KEY = 'aegis-auth'

export interface StoredAuth {
  token: string
  username: string
  forcePasswordChange: boolean
  role?: string
  capabilities?: string[]
  capability_version?: string
  capability_expires_at?: string
}

export function getStoredAuth(): StoredAuth | null {
  const raw = localStorage.getItem(AUTH_STORAGE_KEY)
  if (!raw) {
    return null
  }
  try {
    const session = JSON.parse(raw) as StoredAuth
    // Rehydrate the capability snapshot after a page reload. Without this,
    // canCapability() treats the user as unrestricted until the first 403,
    // which makes the OPA editor appear writable to read-only users.
    if (session.capabilities) {
      const expiresAt = session.capability_expires_at ? Date.parse(session.capability_expires_at) : 0
      const ttlMs = expiresAt > Date.now() ? expiresAt - Date.now() : 15 * 60 * 1000
      setCapabilitySnapshot(session.capabilities, Number(session.capability_version?.replace(/\D/g, '')) || 1, ttlMs)
    }
    return session
  } catch {
    clearStoredAuth()
    return null
  }
}

export function saveAuthSession(session: AuthSession) {
  const expiresAt = session.capability_expires_at ? Date.parse(session.capability_expires_at) : 0
  const ttlMs = expiresAt > Date.now() ? expiresAt - Date.now() : 15 * 60 * 1000
  setCapabilitySnapshot(session.capabilities, Number(session.capability_version?.replace(/\D/g, '')) || 1, ttlMs)
  const auth: StoredAuth = {
    token: session.token,
    username: session.username,
    forcePasswordChange: session.force_password_change,
    role: session.role,
    capabilities: session.capabilities,
    capability_version: session.capability_version,
    capability_expires_at: session.capability_expires_at
  }
  localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(auth))
}

export function clearStoredAuth() {
  localStorage.removeItem(AUTH_STORAGE_KEY)
  clearCapabilitySnapshot()
}

export function getAuthToken() {
  return getStoredAuth()?.token || ''
}

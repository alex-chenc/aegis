import type { AuthSession } from '@/api/auth'
import { clearCapabilitySnapshot, setCapabilitySnapshot } from '@/utils/capabilities'

const AUTH_STORAGE_KEY = 'aegis-auth'

export interface StoredAuth {
  token: string
  username: string
  forcePasswordChange: boolean
  role?: string
}

export function getStoredAuth(): StoredAuth | null {
  const raw = localStorage.getItem(AUTH_STORAGE_KEY)
  if (!raw) {
    return null
  }
  try {
    return JSON.parse(raw) as StoredAuth
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
    role: session.role
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

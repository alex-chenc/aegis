import type { AuthSession } from '@/api/auth'

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
}

export function getAuthToken() {
  return getStoredAuth()?.token || ''
}

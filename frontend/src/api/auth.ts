import request from './index'

export interface AuthStatus {
  initialized: boolean
}

export interface AuthSession {
  token: string
  username: string
  force_password_change: boolean
  role?: string
}

export interface ChangeCredentialsPayload {
  username: string
  new_password: string
  confirm_password: string
}

export interface ChangePasswordPayload {
  current_password: string
  new_password: string
  confirm_password: string
}

export function getAuthStatus(): Promise<AuthStatus> {
  return request.get('/auth/status')
}

export function bootstrapLogin(): Promise<AuthSession> {
  return request.post('/auth/bootstrap-login')
}

export function login(username: string, password: string): Promise<AuthSession> {
  return request.post('/auth/login', { username, password })
}

export function changeCredentials(payload: ChangeCredentialsPayload): Promise<AuthSession> {
  return request.post('/auth/change-credentials', payload)
}

export function getCurrentUser(): Promise<Omit<AuthSession, 'token'>> {
  return request.get('/auth/me')
}

export function logout(): Promise<{ ok: boolean }> {
  return request.post('/auth/logout')
}

export function changePassword(payload: ChangePasswordPayload): Promise<AuthSession> {
  return request.post('/auth/change-password', payload)
}

import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as authApi from './auth'

const { getMock, postMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn()
}))

vi.mock('./index', () => ({
  default: {
    get: getMock,
    post: postMock
  }
}))

describe('auth APIs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('gets auth status', async () => {
    const expected = { initialized: false }
    getMock.mockResolvedValueOnce(expected)

    const result = await authApi.getAuthStatus()

    expect(getMock).toHaveBeenCalledWith('/auth/status')
    expect(result).toEqual(expected)
  })

  it('calls bootstrap login endpoint', async () => {
    const expected = { token: 'token-1', username: 'admin', force_password_change: true }
    postMock.mockResolvedValueOnce(expected)

    const result = await authApi.bootstrapLogin()

    expect(postMock).toHaveBeenCalledWith('/auth/bootstrap-login')
    expect(result).toEqual(expected)
  })

  it('logs in with username and password', async () => {
    const expected = { token: 'token-2', username: 'security-admin', force_password_change: false }
    postMock.mockResolvedValueOnce(expected)

    const result = await authApi.login('security-admin', 'StrongerPassword123!')

    expect(postMock).toHaveBeenCalledWith('/auth/login', {
      username: 'security-admin',
      password: 'StrongerPassword123!'
    })
    expect(result).toEqual(expected)
  })

  it('changes credentials with expected payload', async () => {
    const expected = { token: 'token-3', username: 'security-admin', force_password_change: false }
    postMock.mockResolvedValueOnce(expected)

    const result = await authApi.changeCredentials({
      username: 'security-admin',
      new_password: 'StrongerPassword123!',
      confirm_password: 'StrongerPassword123!'
    })

    expect(postMock).toHaveBeenCalledWith('/auth/change-credentials', {
      username: 'security-admin',
      new_password: 'StrongerPassword123!',
      confirm_password: 'StrongerPassword123!'
    })
    expect(result).toEqual(expected)
  })
})

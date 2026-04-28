// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createIdleLogout, IDLE_LOGOUT_TIMEOUT_MS } from './sessionTimeout'

describe('session idle timeout', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('logs out after five minutes without activity', () => {
    const onTimeout = vi.fn()
    const idleLogout = createIdleLogout({ onTimeout })

    idleLogout.start()
    vi.advanceTimersByTime(IDLE_LOGOUT_TIMEOUT_MS - 1)
    expect(onTimeout).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)
    expect(onTimeout).toHaveBeenCalledTimes(1)
  })

  it('resets the timeout when user activity is detected', () => {
    const onTimeout = vi.fn()
    const idleLogout = createIdleLogout({ onTimeout })

    idleLogout.start()
    vi.advanceTimersByTime(IDLE_LOGOUT_TIMEOUT_MS - 1000)
    window.dispatchEvent(new Event('mousemove'))
    vi.advanceTimersByTime(1000)
    expect(onTimeout).not.toHaveBeenCalled()

    vi.advanceTimersByTime(IDLE_LOGOUT_TIMEOUT_MS)
    expect(onTimeout).toHaveBeenCalledTimes(1)
  })

  it('does not schedule logout while disabled', () => {
    const onTimeout = vi.fn()
    const idleLogout = createIdleLogout({ onTimeout, isEnabled: () => false })

    idleLogout.start()
    vi.advanceTimersByTime(IDLE_LOGOUT_TIMEOUT_MS)

    expect(onTimeout).not.toHaveBeenCalled()
  })
})

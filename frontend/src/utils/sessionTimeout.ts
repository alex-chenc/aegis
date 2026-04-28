export const IDLE_LOGOUT_TIMEOUT_MS = 5 * 60 * 1000

const ACTIVITY_EVENTS = ['mousemove', 'mousedown', 'keydown', 'touchstart', 'scroll'] as const

interface IdleLogoutOptions {
  timeoutMs?: number
  onTimeout: () => void
  isEnabled?: () => boolean
}

export function createIdleLogout(options: IdleLogoutOptions) {
  const timeoutMs = options.timeoutMs ?? IDLE_LOGOUT_TIMEOUT_MS
  const isEnabled = options.isEnabled ?? (() => true)
  let timer: ReturnType<typeof window.setTimeout> | null = null
  let started = false

  const clearTimer = () => {
    if (timer) {
      window.clearTimeout(timer)
      timer = null
    }
  }

  const schedule = () => {
    clearTimer()
    if (!started || !isEnabled()) return

    timer = window.setTimeout(() => {
      clearTimer()
      if (isEnabled()) {
        options.onTimeout()
      }
    }, timeoutMs)
  }

  const handleActivity = () => {
    schedule()
  }

  const handleVisibilityChange = () => {
    if (document.visibilityState === 'visible') {
      schedule()
    }
  }

  return {
    start() {
      if (started) return
      started = true
      ACTIVITY_EVENTS.forEach(event => window.addEventListener(event, handleActivity, { passive: true }))
      document.addEventListener('visibilitychange', handleVisibilityChange)
      schedule()
    },
    refresh() {
      schedule()
    },
    stop() {
      if (!started) return
      started = false
      clearTimer()
      ACTIVITY_EVENTS.forEach(event => window.removeEventListener(event, handleActivity))
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }
}

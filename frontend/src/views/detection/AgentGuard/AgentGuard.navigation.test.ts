// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import router from '../../../router'
import zhCNApp from '../../../i18n/locales/zh-CN/app'
import enUSApp from '../../../i18n/locales/en-US/app'

describe('Agent Guard navigation contract', () => {
  it('provides the parent and exactly two stable menu labels in both locales', () => {
    const expectedKeys = ['agentGuard', 'agentGuardEvents', 'agentGuardEscape']
    expect(expectedKeys.filter(key => key in zhCNApp.menu)).toEqual(expectedKeys)
    expect(expectedKeys.filter(key => key in enUSApp.menu)).toEqual(expectedKeys)
  })

  it('defines the redirect and the two page routes', () => {
    const routes = router.getRoutes()
    expect(routes.find(route => route.path === '/detection/agent-guard')?.redirect)
      .toBe('/detection/agent-guard/events')
    expect(routes.filter(route => [
      '/detection/agent-guard/events',
      '/detection/agent-guard/escape',
    ].includes(route.path))).toHaveLength(2)
  })
})

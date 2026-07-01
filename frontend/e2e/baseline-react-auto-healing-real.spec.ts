import { expect, test } from '@playwright/test'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || 'http://localhost:8081'
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'

const TASK_GROUP_ID = process.env.AEGIS_BASELINE_REACT_GROUP || 'f5e2ff8c-346e-4d4f-bcf7-3f3b75450d2e'
const CHECK_TASK_ID = process.env.AEGIS_BASELINE_REACT_CHECK_TASK || '98b4d94a-a1b2-4417-8c35-e164593cbd15'

test.describe('baseline task detail controls', () => {
  test.skip(process.env.PLAYWRIGHT_REAL !== '1', 'Set PLAYWRIGHT_REAL=1 to run against a live Aegis stack')

  test('keeps non-compliant checks out of automatic repair flow', async ({ page }) => {
    await loginViaUi(page)
    await page.goto(`${BASE_URL}/baseline/tasks/${TASK_GROUP_ID}`, { waitUntil: 'domcontentloaded' })
    await expect(page.getByText(`任务详情: ${TASK_GROUP_ID}`)).toBeVisible({ timeout: 30_000 })

    await expect(page.getByRole('button', { name: '脚本修复' })).toHaveCount(0)
    await expect(page.getByText('ReAct修复')).toHaveCount(0)
    await expect(page.locator('button').filter({ hasText: /^修复$/ })).toHaveCount(0)

    const detail = await page.evaluate(async id => {
      const rawAuth = window.localStorage.getItem('aegis-auth')
      const token = rawAuth ? JSON.parse(rawAuth).token : ''
      const response = await fetch(`/api/v1/tasks/${id}`, {
        headers: { Authorization: `Bearer ${token}` }
      })
      return response.json()
    }, CHECK_TASK_ID)

    expect(detail.data.status).toBe('SUCCESS')
    expect(detail.data.exit_code).toBe(1)

    const healStatus = await page.evaluate(async id => {
      const rawAuth = window.localStorage.getItem('aegis-auth')
      const token = rawAuth ? JSON.parse(rawAuth).token : ''
      const response = await fetch(`/api/v1/tasks/${id}/heal`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` }
      })
      return response.status
    }, CHECK_TASK_ID)

    expect(healStatus).toBe(400)
  })
})

async function loginViaUi(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/login`, { waitUntil: 'domcontentloaded' })
  await page.getByPlaceholder('请输入账号').fill(USERNAME)
  await page.getByPlaceholder('请输入密码').fill(PASSWORD)
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(url => !url.pathname.startsWith('/login'), { timeout: 30_000 })
}

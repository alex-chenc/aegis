import { expect, test } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:8081'
const username = process.env.AEGIS_E2E_USERNAME || 'admin'
const password = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'

test.describe('weak password real workflow', () => {
  test.skip(process.env.PLAYWRIGHT_REAL !== '1', 'Set PLAYWRIGHT_REAL=1 to run against a live Aegis stack')

  test('logs in and verifies live weak password module, analysis, and dictionary', async ({ page }) => {
    const browserErrors: string[] = []
    page.on('pageerror', err => browserErrors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error') {
        browserErrors.push(msg.text())
      }
    })

    await page.goto(`${baseURL}/login`, { waitUntil: 'networkidle' })
    await page.getByPlaceholder('请输入账号').fill(username)
    await page.getByPlaceholder('请输入密码').fill(password)
    await page.getByRole('button', { name: '登录' }).click()
    await page.waitForFunction(() => Boolean(window.localStorage.getItem('aegis-auth')))
    await page.waitForURL(url => url.pathname === '/hosts', { timeout: 10_000 })

    await page.goto(`${baseURL}/risk/weak-password`, { waitUntil: 'networkidle' })
    await expect(page.locator('.sidebar-menu').getByText('风险管理')).toHaveCount(0)
    await expect(page.locator('.sidebar-menu').getByText('智能弱密码检测')).toBeVisible()
    await expect(page.getByText('智能弱密码检测').first()).toBeVisible()
    await expect(page.getByRole('button', { name: '一键分析资产应用' })).toBeVisible()
    await expect(page.getByText('风险说明')).toHaveCount(0)

    await page.getByRole('button', { name: '一键分析资产应用' }).click()
    const candidateButton = page.getByText('检查弱密码').first()
    let hasCandidate = false
    try {
      await expect(candidateButton).toBeVisible({ timeout: 20_000 })
      hasCandidate = true
    } catch {
      hasCandidate = false
    }
    if (hasCandidate) {
      await expect(candidateButton).toBeVisible()
    } else {
      await expect(page.getByText('暂无可分析的应用资产。')).toBeVisible()
    }

    await page.goto(`${baseURL}/risk/weak-password/dictionaries`, { waitUntil: 'networkidle' })
    await expect(page.getByRole('heading', { name: '弱密码字典' })).toBeVisible()
    await expect(page.getByText('默认弱密码字典').first()).toBeVisible()
    await expect(page.getByText('1000').first()).toBeVisible()

    expect(browserErrors).toEqual([])
  })
})

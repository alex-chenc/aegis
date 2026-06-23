import { expect, test } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:8081'
const username = process.env.AEGIS_E2E_USERNAME || 'admin'
const password = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'
const weakPassword = process.env.AEGIS_WEAK_PASSWORD_VALUE || 'Admin@123'
const targetConfigPath = process.env.AEGIS_WEAK_PASSWORD_CONFIG_PATH || '/tmp/aegis-weakpass-test/redis.conf'

test.describe('weak password real workflow', () => {
  test.skip(process.env.PLAYWRIGHT_REAL !== '1', 'Set PLAYWRIGHT_REAL=1 to run against a live Aegis stack')

  test('runs live weak password detection and reveals the confirmed finding from task and app status', async ({ page }) => {
    test.setTimeout(120_000)
    const browserErrors: string[] = []
    page.on('pageerror', err => browserErrors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error') {
        browserErrors.push(msg.text())
      }
    })

    await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' })
    await page.getByPlaceholder('请输入账号').fill(username)
    await page.getByPlaceholder('请输入密码').fill(password)
    await page.getByRole('button', { name: '登录' }).click()
    await page.waitForFunction(() => Boolean(window.localStorage.getItem('aegis-auth')))
    await page.waitForURL(url => url.pathname === '/hosts', { timeout: 15_000 })

    await page.goto(`${baseURL}/risk/weak-password`, { waitUntil: 'domcontentloaded' })
    await expect(page.locator('.sidebar-menu').getByText('智能弱密码检测')).toBeVisible()
    await expect(page.getByText('智能弱密码检测').first()).toBeVisible()
    await expect(page.getByText('检测结果')).toHaveCount(0)

    await page.getByRole('button', { name: '一键分析资产应用' }).click()
    const targetRow = page.locator('.el-table__row').filter({ hasText: targetConfigPath }).first()
    await expect(targetRow).toBeVisible({ timeout: 30_000 })
    await expect(targetRow.getByText('凭据类型')).toHaveCount(0)
    await expect(targetRow.getByText('查看分析依据')).toHaveCount(0)

    await targetRow.getByRole('button', { name: '检查弱密码' }).click()
    await expect(page.getByText(/默认弱密码字典/)).toBeVisible()
    await expect(page.getByText('混合规则')).toHaveCount(0)
    await expect(page.getByText('模糊规则')).toHaveCount(0)
    await expect(page.getByText('加密/hash LLM 匹配')).toHaveCount(0)
    await page.getByRole('button', { name: '确认检查' }).click()

    await page.waitForURL(/\/risk\/weak-password\/tasks\/[0-9a-f-]+/, { timeout: 15_000 })
    await expect(page.getByText(targetConfigPath).first()).toBeVisible({ timeout: 45_000 })
    await expect(page.getByText('*********').first()).toBeVisible({ timeout: 45_000 })
    await expect(page.getByText('confirmed').first()).toBeVisible()

    await page.getByRole('button', { name: '详情' }).first().click()
    await expect(page.getByText('请输入当前系统密码')).toBeVisible()
    await page.locator('.el-message-box input').fill(password)
    await page.getByRole('button', { name: '查看', exact: true }).click()
    await expect(page.getByText('完整密码')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText(weakPassword)).toBeVisible()
    await page.getByRole('button', { name: '关闭', exact: true }).click()

    await page.goto(`${baseURL}/risk/weak-password`, { waitUntil: 'domcontentloaded' })
    await expect(page.getByText('告警', { exact: true }).first()).toBeVisible({ timeout: 30_000 })
    await page.getByText('告警', { exact: true }).first().click()
    await expect(page.getByText('弱密码详情')).toBeVisible()
    await expect(page.getByText(targetConfigPath).first()).toBeVisible()
    await page.getByRole('button', { name: '查看明文' }).first().click()
    await page.locator('.el-message-box input').fill(password)
    await page.getByRole('button', { name: '查看', exact: true }).click()
    await expect(page.getByText(weakPassword).first()).toBeVisible()

    await page.keyboard.press('Escape')
    await page.getByRole('button', { name: '一键检测' }).click()
    await page.getByRole('button', { name: '确认检查' }).click()
    await expect(page.getByText('检查任务')).toBeVisible({ timeout: 15_000 })

    await page.goto(`${baseURL}/risk/weak-password/dictionaries`, { waitUntil: 'domcontentloaded' })
    await expect(page.getByRole('heading', { name: '弱密码字典' })).toBeVisible()
    await expect(page.getByText('内置').first()).toBeVisible()
    await expect(page.getByText('分类')).toHaveCount(0)
    await expect(page.getByText('来源')).toHaveCount(0)
    await expect(page.getByText('状态')).toHaveCount(0)
    await page.getByRole('button', { name: 'AI 一键生成字典' }).click()
    await page.getByPlaceholder(/为 Redis 管理员/).fill('为 Redis 生产环境生成弱密码字典，包含 aegis、admin 和年份')
    await page.getByRole('button', { name: '生成并保存' }).click()
    await expect(page.getByText(/已生成 \d+ 条候选/)).toBeVisible({ timeout: 15_000 })
    await page.locator('.el-drawer').getByRole('button', { name: '取消' }).click()
    await page.getByRole('button', { name: '查看条目' }).first().click()
    await expect(page.locator('.el-drawer').getByText(weakPassword).first()).toBeVisible()

    expect(browserErrors).toEqual([])
  })
})

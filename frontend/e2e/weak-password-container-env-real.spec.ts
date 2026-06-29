import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:8081'
const API_URL = `${BASE_URL}/api/v1`
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'
const TARGET_TASK_ID = process.env.AEGIS_WEAK_PASSWORD_TASK_ID || 'b9dd6dd8-b182-464c-b72a-7b2cdb96b7ee'
const REDIS_CANDIDATE_ID = process.env.AEGIS_REDIS_CANDIDATE_ID || '233f467d-e19c-4fa7-afc8-0b36f8c0f9a9'
const REDIS_DICTIONARY_ID = process.env.AEGIS_REDIS_DICTIONARY_ID || 'c3f9adfd-4cef-4e66-9826-fc76a052e6e5'
const REDIS_CONTAINER_ID = process.env.AEGIS_REDIS_CONTAINER_ID || 'eadcc18b405c55f0bc496c41deb72c3b81f1e772faaf84e33938bc42322c43a7'
const REDIS_ENV_PASSWORD = process.env.AEGIS_REDIS_ENV_PASSWORD || 'EnvRedis@123'
const REDIS_ENV_CONFIG_PATH = `/var/lib/docker/containers/${REDIS_CONTAINER_ID}/config.v2.json`

type JsonObject = Record<string, any>

test.describe.configure({ mode: 'serial' })

test.describe('weak password container Env real workflow', () => {
  test.skip(process.env.PLAYWRIGHT_REAL !== '1', 'Set PLAYWRIGHT_REAL=1 to run against a live Aegis stack')

  test('shows container Env source fields and deletes selected weak-password tasks', async ({ page, request }) => {
    test.setTimeout(5 * 60 * 1000)

    const browserErrors: string[] = []
    page.on('pageerror', err => browserErrors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error') browserErrors.push(msg.text())
    })

    await loginViaUi(page)
    const headers = authHeaders(await authToken(page))

    await test.step('真实任务详情展示容器 Env 采集路径和字段', async () => {
      const task = await apiData(request, 'GET', `/weak-password/tasks/${TARGET_TASK_ID}`, headers)
      expect(task.task.status).toBe('completed')
      expect(task.task.matched_findings).toBeGreaterThan(0)

      const progress = await apiData(request, 'GET', `/weak-password/tasks/${TARGET_TASK_ID}/collection-progress?page=1&page_size=20`, headers)
      expect(progress.items).toEqual(expect.arrayContaining([
        expect.objectContaining({
          source_path: REDIS_ENV_CONFIG_PATH,
          field_name: 'Env.REDIS_PASSWORD',
        }),
      ]))

      await page.goto(`${BASE_URL}/risk/weak-password/tasks/${TARGET_TASK_ID}`, { waitUntil: 'domcontentloaded' })
      await expect(page.getByRole('heading', { name: /弱密码检查 - Redis/ })).toBeVisible({ timeout: 30_000 })
      await expect(page.getByText('命中结果')).toBeVisible()
      await expect(page.getByText('采集进度')).toBeVisible()
      await expect(page.getByText('采集路径')).toBeVisible()
      await expect(page.getByText('采集字段')).toBeVisible()
      await expect(page.getByText(REDIS_ENV_CONFIG_PATH).first()).toBeVisible({ timeout: 30_000 })
      await expect(page.getByText('Env.REDIS_PASSWORD').first()).toBeVisible()
      await expect(page.getByText('已确认').first()).toBeVisible()

      await page.getByRole('button', { name: '详情' }).first().click()
      const passwordPrompt = page.locator('.el-message-box').filter({ hasText: '查看命中密码' })
      await passwordPrompt.locator('input').fill(PASSWORD)
      await passwordPrompt.getByRole('button', { name: '查看', exact: true }).click()
      await expect(passwordPrompt).toBeHidden({ timeout: 10_000 })
      const detailDialog = page.locator('.el-dialog').filter({ hasText: '命中密码详情' })
      await expect(detailDialog.getByText('完整密码')).toBeVisible({ timeout: 10_000 })
      await expect(detailDialog.getByText(REDIS_ENV_PASSWORD)).toBeVisible()
      await page.screenshot({ path: 'test-results/weak-password-container-env-detail.png', fullPage: true })
      await detailDialog.getByRole('button', { name: '关闭', exact: true }).click()
    })

    const disposableTaskIDs = await test.step('创建两个真实临时任务供前端批量删除', async () => {
      const ids: string[] = []
      for (let i = 0; i < 2; i += 1) {
        const task = await apiData(request, 'POST', '/weak-password/tasks/by-application', headers, {
          candidate_application_id: REDIS_CANDIDATE_ID,
          dictionary_policy: {
            use_default_1000: false,
            dictionary_ids: [REDIS_DICTIONARY_ID],
            use_ai_generated: false,
          },
          ai_policy: {
            repair_collection_errors: true,
            detection_rounds: 10,
            max_agent_tool_calls_per_app: 10,
          },
        })
        ids.push(task.task_id)
        await waitWeakPasswordTaskFinished(request, headers, task.task_id)
      }
      return ids
    })

    await test.step('页面多选删除弱密码检查任务', async () => {
      await page.goto(`${BASE_URL}/risk/weak-password`, { waitUntil: 'domcontentloaded' })
      await page.getByRole('tab', { name: '弱密码检查' }).click()
      await expect(page.getByText('检查任务')).toBeVisible()
      const rows = page.locator('.el-table__body-wrapper tbody tr').filter({ hasText: '弱密码检查 - Redis' })
      await expect(rows.nth(0)).toBeVisible({ timeout: 30_000 })
      await rows.nth(0).locator('.el-checkbox__inner').click()
      await rows.nth(1).locator('.el-checkbox__inner').click()
      await page.getByRole('button', { name: '批量删除' }).click()
      await page.locator('.el-message-box').getByRole('button', { name: '删除' }).click()
      await expect(page.getByText('已删除 2 个任务')).toBeVisible({ timeout: 30_000 })

      for (const taskID of disposableTaskIDs) {
        const response = await request.get(`${API_URL}/weak-password/tasks/${taskID}`, { headers })
        const body = await response.json().catch(() => ({}))
        expect(response.status(), `task ${taskID} should be removed: ${JSON.stringify(body)}`).toBe(404)
      }
    })

    expect(browserErrors).toEqual([])
  })
})

async function loginViaUi(page: Page) {
  await page.goto(`${BASE_URL}/login`, { waitUntil: 'domcontentloaded' })
  await page.getByPlaceholder('请输入账号').fill(USERNAME)
  await page.getByPlaceholder('请输入密码').fill(PASSWORD)
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForFunction(() => Boolean(window.localStorage.getItem('aegis-auth')), undefined, { timeout: 30_000 })
  await page.waitForURL(url => !url.pathname.startsWith('/login'), { timeout: 30_000 })
}

async function authToken(page: Page) {
  const raw = await page.evaluate(() => window.localStorage.getItem('aegis-auth'))
  expect(raw).toBeTruthy()
  return JSON.parse(raw!).token as string
}

function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` }
}

async function apiData(
  request: APIRequestContext,
  method: 'GET' | 'POST',
  path: string,
  headers: Record<string, string>,
  data?: JsonObject,
) {
  const response = await request.fetch(`${API_URL}${path}`, { method, headers, data, timeout: 300_000 })
  const text = await response.text()
  expect(response.ok(), `${method} ${path} -> ${response.status()} ${text}`).toBeTruthy()
  const json = text ? JSON.parse(text) : {}
  if (json.code !== undefined) {
    expect(json.code, `${method} ${path} -> ${text}`).toBe(0)
    return json.data
  }
  return json.data ?? json
}

async function waitWeakPasswordTaskFinished(request: APIRequestContext, headers: Record<string, string>, taskID: string) {
  const started = Date.now()
  let lastProgress: JsonObject = {}
  while (Date.now() - started < 120_000) {
    lastProgress = await apiData(request, 'GET', `/weak-password/tasks/${taskID}/progress`, headers)
    if (['completed', 'failed', 'partial_failed', 'cancelled'].includes(lastProgress.status)) {
      expect(lastProgress.status, JSON.stringify(lastProgress, null, 2)).toBe('completed')
      return lastProgress
    }
    await new Promise(resolve => setTimeout(resolve, 2_000))
  }
  throw new Error(`timed out waiting for weak password task ${taskID}: ${JSON.stringify(lastProgress, null, 2)}`)
}

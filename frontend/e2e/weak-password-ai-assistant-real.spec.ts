import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:8081'
const API_URL = `${BASE_URL}/api/v1`
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'

type JsonObject = Record<string, any>

test.describe.configure({ mode: 'serial' })

test.describe('weak password AI and assistant real workflow', () => {
  test.skip(process.env.PLAYWRIGHT_REAL !== '1', 'Set PLAYWRIGHT_REAL=1 to run against a live Aegis stack')

  test('covers AI dictionary generation, application analysis, detection progress, and assistant tools', async ({ page, request }) => {
    test.setTimeout(15 * 60 * 1000)

    const browserErrors: string[] = []
    page.on('pageerror', err => browserErrors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error') browserErrors.push(msg.text())
    })

    await loginViaUi(page)
    const token = await authToken(page)
    const headers = authHeaders(token)

    await test.step('智能体工具目录包含弱密码模块新能力', async () => {
      const tools = await apiData(request, 'GET', '/assistant/tools?keyword=WeakPassword&page=1&page_size=50', headers)
      const names = (tools.tools || []).map((item: JsonObject) => item.tool_name)
      expect(names).toEqual(expect.arrayContaining([
        'Credential.WeakPassword.GenerateDictionary',
        'Credential.WeakPassword.AnalyzeApplications',
        'Credential.WeakPassword.Scan',
        'Credential.WeakPassword.QueryProgress',
        'Credential.WeakPassword.QueryFindings',
      ]))
    })

    await test.step('UI AI 一键生成弱密码字典真实调用 AI 并保存', async () => {
      await page.goto(`${BASE_URL}/risk/weak-password/dictionaries`, { waitUntil: 'domcontentloaded' })
      await page.getByRole('button', { name: 'AI 一键生成字典' }).click()
      const drawer = page.locator('.el-drawer').filter({ hasText: 'AI 一键生成字典' })
      await drawer.getByPlaceholder(/为 Redis 管理员/).fill('为 Redis 生产环境生成弱密码字典，包含 aegis、admin 和 2026')
      await drawer.getByRole('spinbutton').fill('3')
      await drawer.getByRole('button', { name: '生成并保存' }).click()
      await expect(page.getByText(/已生成 \d+ 条候选/)).toBeVisible({ timeout: 120_000 })
      await expect(page.getByText('网络错误，请检查网络连接')).toHaveCount(0)
    })

    const analysis = await test.step('应用资产分析接口返回在线 Redis 候选', async () => {
      const data = await apiData(request, 'POST', '/weak-password/asset-applications/analyze', headers, {
        scope: {
          host_ids: [],
          host_group_ids: [],
          application_types: ['redis'],
          online_agents_only: true,
        },
      })
      expect(data.status).toBe('completed')
      expect(data.candidate_count).toBeGreaterThan(0)
      expect(data.candidates.some((item: JsonObject) => item.application_type === 'redis')).toBeTruthy()
      return data
    })

    const redisCandidate = analysis.candidates.find((item: JsonObject) => item.application_type === 'redis')
    expect(redisCandidate?.candidate_application_id).toBeTruthy()

    const taskID = await test.step('创建真实弱密码检测任务并轮询进度和命中', async () => {
      const defaultDict = await apiData(request, 'GET', '/weak-password/dictionaries/default', headers)
      const task = await apiData(request, 'POST', '/weak-password/tasks/by-application', headers, {
        candidate_application_id: redisCandidate.candidate_application_id,
        dictionary_policy: {
          use_default_1000: true,
          dictionary_ids: defaultDict?.id ? [defaultDict.id] : [],
          use_ai_generated: false,
        },
        ai_policy: {
          repair_collection_errors: true,
          detection_rounds: 10,
          max_agent_tool_calls_per_app: 10,
        },
      })
      expect(task.task_id).toBeTruthy()

      const finalProgress = await waitWeakPasswordTaskFinished(request, headers, task.task_id)
      expect(finalProgress.status, JSON.stringify(finalProgress, null, 2)).toBe('completed')
      expect(finalProgress.progress).toBe(100)

      const collection = await apiData(request, 'GET', `/weak-password/tasks/${task.task_id}/collection-progress?page=1&page_size=20`, headers)
      expect(collection.total).toBeGreaterThan(0)
      expect(collection.items.some((item: JsonObject) => item.tool_name && item.status)).toBeTruthy()

      const findings = await apiData(request, 'GET', `/weak-password/tasks/${task.task_id}/findings?page=1&page_size=20`, headers)
      expect(findings.total).toBeGreaterThan(0)
      expect(findings.items.some((item: JsonObject) => item.match_status === 'confirmed')).toBeTruthy()

      return task.task_id as string
    })

    await test.step('弱密码页面显示任务进度、采集进度和中文状态', async () => {
      await page.goto(`${BASE_URL}/risk/weak-password`, { waitUntil: 'domcontentloaded' })
      await page.getByRole('tab', { name: '弱密码检查' }).click()
      await expect(page.getByText('检查任务')).toBeVisible()
      await expect(page.getByText('已完成').first()).toBeVisible({ timeout: 30_000 })

      await page.goto(`${BASE_URL}/risk/weak-password/tasks/${taskID}`, { waitUntil: 'domcontentloaded' })
      await expect(page.getByRole('heading', { name: /弱密码检查 - Redis/ })).toBeVisible({ timeout: 30_000 })
      await expect(page.getByText('命中结果')).toBeVisible()
      await expect(page.getByText('已确认').first()).toBeVisible()
      await expect(page.getByText('采集进度')).toBeVisible()
      await expect(page.getByText('采集凭据配置').first()).toBeVisible()
      await expect(page.getByText('已完成').first()).toBeVisible()
    })

    await test.step('智能体模式真实调用弱密码进度和命中工具', async () => {
      await setPolicy(request, headers, 'full_access')
      try {
        const session = await apiData(request, 'POST', '/assistant/sessions', headers, {
          title: `Playwright 弱密码智能体 ${Date.now()}`,
          task_type: 'operations',
        })
        await apiData(request, 'POST', `/assistant/sessions/${session.session_id}/message`, headers, {
          content: [
            '请严格按顺序调用工具，不要只文字说明：',
            `1 Credential.WeakPassword.QueryProgress 参数 task_id="${taskID}",page_size=5。`,
            `2 Credential.WeakPassword.QueryFindings 参数 task_id="${taskID}"。`,
          ].join('\n'),
        })
        const calls = await waitToolCalls(request, headers, session.session_id, [
          'Credential.WeakPassword.QueryProgress',
          'Credential.WeakPassword.QueryFindings',
        ], 180_000)
        expect(calls.filter(call => call.status === 'failed'), JSON.stringify(calls, null, 2)).toHaveLength(0)
      } finally {
        await setPolicy(request, headers, 'whitelist')
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
  method: 'GET' | 'POST' | 'PUT',
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
  while (Date.now() - started < 180_000) {
    lastProgress = await apiData(request, 'GET', `/weak-password/tasks/${taskID}/progress`, headers)
    if (['completed', 'failed', 'partial_failed', 'cancelled'].includes(lastProgress.status)) {
      return lastProgress
    }
    await new Promise(resolve => setTimeout(resolve, 3_000))
  }
  throw new Error(`timed out waiting for weak password task ${taskID}: ${JSON.stringify(lastProgress, null, 2)}`)
}

async function setPolicy(request: APIRequestContext, headers: Record<string, string>, mode: string) {
  await apiData(request, 'PUT', '/assistant/tool-approval-policy', headers, { mode })
  await expect.poll(async () => (await apiData(request, 'GET', '/assistant/tool-approval-policy', headers)).mode).toBe(mode)
}

async function waitToolCalls(
  request: APIRequestContext,
  headers: Record<string, string>,
  sessionID: string,
  expectedToolNames: string[],
  timeoutMs: number,
) {
  const started = Date.now()
  let lastCalls: JsonObject[] = []
  while (Date.now() - started < timeoutMs) {
    const [session, page] = await Promise.all([
      apiData(request, 'GET', `/assistant/sessions/${sessionID}`, headers),
      apiData(request, 'GET', `/assistant/sessions/${sessionID}/tool-calls?page=1&page_size=100`, headers),
    ])
    lastCalls = page.items || []
    const failed = lastCalls.filter(call => call.status === 'failed')
    if (failed.length > 0) throw new Error(`assistant tools failed: ${JSON.stringify(failed, null, 2)}`)
    const hasAll = expectedToolNames.every(name =>
      lastCalls.some(call => call.tool_name === name && ['completed', 'success'].includes(call.status)),
    )
    if (hasAll) return lastCalls
    if (session.status === 'failed') throw new Error(`assistant session failed: ${JSON.stringify({ session, lastCalls }, null, 2)}`)
    await new Promise(resolve => setTimeout(resolve, 3_000))
  }
  throw new Error(`timed out waiting for tools ${expectedToolNames.join(', ')}: ${JSON.stringify(lastCalls, null, 2)}`)
}

import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:8081'
const API_URL = `${BASE_URL}/api/v1`
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'

type JsonObject = Record<string, any>

test.describe.configure({ mode: 'serial' })

test.describe('Kafka container weak-password collection real workflow', () => {
  test.skip(process.env.PLAYWRIGHT_REAL !== '1', 'Set PLAYWRIGHT_REAL=1 to run against a live Aegis stack')

  test('marks Kafka as a container app and shows proc-root credential paths', async ({ page, request }) => {
    test.setTimeout(18 * 60 * 1000)

    const browserErrors: string[] = []
    page.on('pageerror', err => browserErrors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error') browserErrors.push(msg.text())
    })

    await loginViaUi(page)
    const headers = authHeaders(await authToken(page))

    await test.step('真实触发当前在线主机资产采集', async () => {
      const collection = await apiData(request, 'POST', '/host-assets/collections', headers, {
        scope: 'all_hosts',
        host_ids: [],
        types: ['process', 'application_analysis'],
        force: true,
      })
      expect(collection.task_id).toBeTruthy()
      const detail = await waitCollectionFinished(request, headers, collection.task_id)
      expect(detail.task.status, JSON.stringify(detail, null, 2)).toBe('completed')
    })

    const kafkaAsset = await test.step('应用资产真实标记为容器应用', async () => {
      const asset = await waitKafkaContainerAsset(request, headers)
      expect(asset.is_container, JSON.stringify(asset, null, 2)).toBe(true)
      expect(asset.container_runtime).toBe('docker')
      expect(asset.container_id).toMatch(/^[a-f0-9]{64}$/)
      expect(asset.related_pids?.length).toBeGreaterThan(0)
      expect(asset.related_pids.some((pid: number) => pid > 0)).toBeTruthy()

      await page.goto(`${BASE_URL}/hosts/assets/applications`, { waitUntil: 'domcontentloaded' })
      await page.getByPlaceholder('搜索应用名、主机名、IP').fill('Kafka')
      await page.getByRole('button', { name: '查询' }).click()
      await expect(page.getByText('Kafka').first()).toBeVisible({ timeout: 30_000 })
      await expect(page.getByText('容器应用').first()).toBeVisible()
      return asset
    })

    const kafkaCandidate = await test.step('弱密码应用分析返回容器应用元数据', async () => {
      const analysis = await apiData(request, 'POST', '/weak-password/asset-applications/analyze', headers, {
        scope: {
          host_ids: [kafkaAsset.host_id],
          host_group_ids: [],
          application_types: ['kafka'],
          online_agents_only: true,
        },
      })
      expect(analysis.status).toBe('completed')
      expect(analysis.candidate_count).toBeGreaterThan(0)
      const candidate = (analysis.candidates || []).find((item: JsonObject) => item.application_type === 'kafka')
      expect(candidate, JSON.stringify(analysis, null, 2)).toBeTruthy()
      expect(candidate.is_container, JSON.stringify(candidate, null, 2)).toBe(true)
      expect(candidate.container_runtime).toBe('docker')
      expect(candidate.container_id).toBe(kafkaAsset.container_id)
      return candidate
    })

    const taskID = await test.step('创建真实 Kafka 弱密码任务并验证容器路径', async () => {
      const defaultDict = await apiData(request, 'GET', '/weak-password/dictionaries/default', headers)
      const task = await apiData(request, 'POST', '/weak-password/tasks/by-application', headers, {
        candidate_application_id: kafkaCandidate.candidate_application_id,
        dictionary_policy: {
          use_default_1000: true,
          dictionary_ids: defaultDict?.id ? [defaultDict.id] : [],
          use_ai_generated: false,
        },
        ai_policy: {
          repair_collection_errors: false,
          detection_rounds: 4,
          max_agent_tool_calls_per_app: 8,
        },
      })
      expect(task.task_id).toBeTruthy()

      const finalProgress = await waitWeakPasswordTaskSettled(request, headers, task.task_id)
      expect(['completed', 'failed', 'partial_failed']).toContain(finalProgress.status)

      const progress = await apiData(request, 'GET', `/weak-password/tasks/${task.task_id}/collection-progress?page=1&page_size=100`, headers)
      expect(progress.total).toBeGreaterThan(0)
      const joinedPaths = (progress.items || []).map((item: JsonObject) => item.source_path || '').join('\n')
      const joinedFields = (progress.items || []).map((item: JsonObject) => item.field_name || '').join('\n')
      const kafkaPID = Number(kafkaAsset.related_pids.find((pid: number) => pid > 0))
      const procRoot = `/proc/${kafkaPID}/root`

      expect(joinedPaths).toContain(`${procRoot}/etc/kafka/`)
      expect(joinedPaths).not.toContain(`${procRoot}\n/etc/kafka/`)
      expect(joinedPaths).not.toContain('/etc/yum.conf')
      expect(joinedFields).toContain('container_app=true')
      expect(joinedFields).toContain(`container_runtime=${kafkaAsset.container_runtime}`)
      expect(joinedFields).toContain(`container_id=${kafkaAsset.container_id}`)

      await page.goto(`${BASE_URL}/risk/weak-password/tasks/${task.task_id}`, { waitUntil: 'domcontentloaded' })
      await expect(page.getByRole('heading', { name: /弱密码检查 - .*Kafka/ })).toBeVisible({ timeout: 30_000 })
      await expect(page.getByText('采集进度')).toBeVisible()
      await expect(page.getByText('采集路径')).toBeVisible()
      await expect(page.getByText('采集字段')).toBeVisible()
      await expect(page.getByText(`${procRoot}/etc/kafka/`).first()).toBeVisible({ timeout: 30_000 })
      await expect(page.getByText('container_app=true').first()).toBeVisible()

      const clippedPath = page.locator('.collection-progress-table .clipped-multiline-cell').filter({ hasText: procRoot }).first()
      await expect(clippedPath).toBeVisible()
      await expect(clippedPath).toHaveCSS('max-height', '96px')
      await clippedPath.hover()
      await expect(page.locator('.el-popper').filter({ hasText: procRoot }).first()).toBeVisible({ timeout: 10_000 })
      await page.screenshot({ path: 'test-results/weak-password-kafka-container-path-real.png', fullPage: true })

      return task.task_id as string
    })

    expect(taskID).toBeTruthy()
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

async function waitCollectionFinished(request: APIRequestContext, headers: Record<string, string>, taskID: string) {
  const started = Date.now()
  let detail: JsonObject = {}
  while (Date.now() - started < 12 * 60_000) {
    detail = await apiData(request, 'GET', `/host-assets/collections/${taskID}`, headers)
    if (['completed', 'failed', 'cancelled'].includes(detail.task?.status)) return detail
    await new Promise(resolve => setTimeout(resolve, 5_000))
  }
  throw new Error(`asset collection ${taskID} did not finish: ${JSON.stringify(detail, null, 2)}`)
}

async function waitKafkaContainerAsset(request: APIRequestContext, headers: Record<string, string>) {
  const started = Date.now()
  let lastItems: JsonObject[] = []
  while (Date.now() - started < 120_000) {
    const page = await apiData(request, 'GET', '/host-assets/applications?keyword=Kafka&page=1&page_size=50', headers)
    lastItems = page.items || []
    const asset = lastItems.find(item =>
      String(item.name || item.display_name || '').toLowerCase().includes('kafka') &&
      item.status === 'active' &&
      item.is_container === true,
    )
    if (asset) return asset
    await new Promise(resolve => setTimeout(resolve, 3_000))
  }
  throw new Error(`Kafka container asset was not visible: ${JSON.stringify(lastItems, null, 2)}`)
}

async function waitWeakPasswordTaskSettled(request: APIRequestContext, headers: Record<string, string>, taskID: string) {
  const started = Date.now()
  let lastProgress: JsonObject = {}
  while (Date.now() - started < 180_000) {
    lastProgress = await apiData(request, 'GET', `/weak-password/tasks/${taskID}/progress`, headers)
    if (['completed', 'failed', 'partial_failed', 'cancelled'].includes(lastProgress.status)) return lastProgress
    await new Promise(resolve => setTimeout(resolve, 2_000))
  }
  throw new Error(`weak password task ${taskID} did not finish: ${JSON.stringify(lastProgress, null, 2)}`)
}

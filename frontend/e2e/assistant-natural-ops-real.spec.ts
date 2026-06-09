import { test, expect, type APIRequestContext, type Page } from '@playwright/test'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || 'http://localhost:8081'
const API_URL = `${BASE_URL}/api/v1`
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'

type JsonObject = Record<string, any>

test.describe.configure({ mode: 'serial' })
test.use({ actionTimeout: 10_000, navigationTimeout: 30_000 })

test('真实业务端口：智能体短自然语言操作指令', async ({ page, request }) => {
  test.setTimeout(15 * 60 * 1000)

  await loginViaUi(page)
  const token = await authToken(page)
  const headers = authHeaders(token)
  await expectOnlineHost(request, headers)

  await test.step('全权限：发送“进行资产采集”会直接调用资产采集工具链', async () => {
    await setPolicy(request, headers, 'full_access')
    const session = await createAssistantSession(request, headers, '短句资产采集')
    await apiData(request, 'POST', `/assistant/sessions/${session.session_id}/message`, headers, { content: '进行资产采集' })

    await waitSessionStatus(request, headers, session.session_id, ['completed'])
    const calls = await listToolCalls(request, headers, session.session_id)
    const toolNames = calls.map(call => call.tool_name)
    expect(toolNames).toContain('Asset.Collection.Trigger')
    expect(toolNames).toContain('Asset.Collection.Get')
    expect(toolNames).toContain('Asset.Application.List')
    expect(toolNames).toContain('Asset.Summary.Get')
    expect(toolNames).not.toContain('Host.List')
    expect(calls.filter(call => call.status === 'failed'), JSON.stringify(calls, null, 2)).toHaveLength(0)

    const trigger = calls.find(call => call.tool_name === 'Asset.Collection.Trigger')
    expect(trigger?.status, JSON.stringify(trigger, null, 2)).toBe('success')
    const args = asObject(trigger?.args)
    expect(args.scope).toBe('all_hosts')
    expect(args.force).toBe(true)

    const messages = await listMessages(request, headers, session.session_id)
    const answer = latestAssistantContent(messages)
    expect(answer).toContain('全部在线主机')
    expect(answer).toContain('资产采集')
  })

  await test.step('短句漏洞/基线扫描：信息不足时追问范围和执行方式', async () => {
    await setPolicy(request, headers, 'full_access')
    const vulnerability = await runPromptToCompletion(request, headers, '短句漏洞扫描', '进行漏洞扫描')
    expect(latestAssistantContent(vulnerability.messages)).toContain('扫描范围')
    expect(vulnerability.calls).toHaveLength(0)

    const baseline = await runPromptToCompletion(request, headers, '短句基线扫描', '进行基线扫描')
    expect(latestAssistantContent(baseline.messages)).toContain('基线模板')
    expect(baseline.calls).toHaveLength(0)
  })

  await test.step('请求确认：短句资产采集必须先显示审批按钮，批准后才执行工具', async () => {
    await setPolicy(request, headers, 'request_approval')
    const session = await createAssistantSession(request, headers, '短句资产采集审批')
    await apiData(request, 'POST', `/assistant/sessions/${session.session_id}/message`, headers, { content: '进行资产采集' })

    await page.goto(`${BASE_URL}/assistant?session=${session.session_id}`)
    await expect(page.getByRole('button', { name: '批准执行' }).first()).toBeVisible({ timeout: 180_000 })
    await expect.poll(async () => (await apiData(request, 'GET', `/assistant/sessions/${session.session_id}`, headers)).status, {
      timeout: 180_000,
    }).toBe('waiting_approval')

    let calls = await listToolCalls(request, headers, session.session_id)
    const pendingTrigger = calls.find(call => call.tool_name === 'Asset.Collection.Trigger')
    expect(pendingTrigger?.status, JSON.stringify(calls, null, 2)).toBe('approval_required')

    await approvePendingToolsUntilCompleted(page, request, headers, session.session_id)
    calls = await listToolCalls(request, headers, session.session_id)
    const executedTrigger = calls.find(call => call.tool_name === 'Asset.Collection.Trigger')
    expect(executedTrigger?.status, JSON.stringify(calls, null, 2)).toBe('success')
    expect(calls.filter(call => call.status === 'failed'), JSON.stringify(calls, null, 2)).toHaveLength(0)
  })

  await setPolicy(request, headers, 'whitelist')
})

async function loginViaUi(page: Page) {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('请输入账号').fill(USERNAME)
  await page.getByPlaceholder('请输入密码').fill(PASSWORD)
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(url => !url.pathname.startsWith('/login'), { timeout: 30_000 })
}

async function authToken(page: Page) {
  const raw = await page.evaluate(() => localStorage.getItem('aegis-auth'))
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
  const response = await request.fetch(`${API_URL}${path}`, {
    method,
    headers,
    data,
    timeout: 300_000,
  })
  const text = await response.text()
  expect(response.ok(), `${method} ${path} -> ${response.status()} ${text}`).toBeTruthy()
  const json = text ? JSON.parse(text) : {}
  if (json.code !== undefined) {
    expect(json.code, `${method} ${path} -> ${text}`).toBe(0)
    return json.data
  }
  return json.data ?? json
}

async function setPolicy(request: APIRequestContext, headers: Record<string, string>, mode: string) {
  await apiData(request, 'PUT', '/assistant/tool-approval-policy', headers, { mode })
  await expect.poll(async () => (await apiData(request, 'GET', '/assistant/tool-approval-policy', headers)).mode, {
    timeout: 10_000,
  }).toBe(mode)
}

async function createAssistantSession(request: APIRequestContext, headers: Record<string, string>, title: string) {
  return apiData(request, 'POST', '/assistant/sessions', headers, {
    title: `Playwright ${title}`,
    task_type: 'operations',
  })
}

async function runPromptToCompletion(request: APIRequestContext, headers: Record<string, string>, title: string, content: string) {
  const session = await createAssistantSession(request, headers, title)
  await apiData(request, 'POST', `/assistant/sessions/${session.session_id}/message`, headers, { content })
  await waitSessionStatus(request, headers, session.session_id, ['completed'])
  return {
    session,
    calls: await listToolCalls(request, headers, session.session_id),
    messages: await listMessages(request, headers, session.session_id),
  }
}

async function waitSessionStatus(request: APIRequestContext, headers: Record<string, string>, sessionId: string, statuses: string[]) {
  const started = Date.now()
  let session: JsonObject = {}
  while (Date.now() - started < 300_000) {
    session = await apiData(request, 'GET', `/assistant/sessions/${sessionId}`, headers)
    if (statuses.includes(session.status)) return session
    if (session.status === 'failed' && !statuses.includes('failed')) {
      const calls = await listToolCalls(request, headers, sessionId)
      const messages = await listMessages(request, headers, sessionId)
      throw new Error(`session ${sessionId} failed: ${JSON.stringify({ session, calls, messages }, null, 2)}`)
    }
    await new Promise(resolve => setTimeout(resolve, 2_000))
  }
  throw new Error(`session ${sessionId} did not reach ${statuses.join(',')}: ${JSON.stringify(session, null, 2)}`)
}

async function approvePendingToolsUntilCompleted(
  page: Page,
  request: APIRequestContext,
  headers: Record<string, string>,
  sessionId: string,
) {
  const started = Date.now()
  const approvedToolNames: string[] = []
  while (Date.now() - started < 300_000) {
    const session = await apiData(request, 'GET', `/assistant/sessions/${sessionId}`, headers)
    if (session.status === 'completed') {
      expect(approvedToolNames.length).toBeGreaterThanOrEqual(1)
      return
    }

    const approvalsPage = await apiData(request, 'GET', `/assistant/sessions/${sessionId}/approvals?page=1&page_size=100`, headers)
    const pending = (approvalsPage.items || []).find((approval: JsonObject) => approval.status === 'pending')
    if (!pending) {
      await new Promise(resolve => setTimeout(resolve, 1_000))
      continue
    }

    const approveButton = page.getByRole('button', { name: '批准执行' }).first()
    await expect(approveButton).toBeVisible({ timeout: 180_000 })
    await approveButton.click()
    const approveDialog = page.locator('.el-message-box').filter({ hasText: '批准执行' }).last()
    await expect(approveDialog).toBeVisible()
    await approveDialog.getByRole('button', { name: '批准' }).click()
    approvedToolNames.push(pending.tool_name)

    await expect.poll(async () => {
      const approval = await apiData(request, 'GET', `/assistant/approvals/${pending.approval_id}`, headers)
      return approval.status
    }, { timeout: 180_000 }).not.toBe('pending')
  }
  const calls = await listToolCalls(request, headers, sessionId)
  throw new Error(`pending approvals were not fully executed: ${JSON.stringify({ approvedToolNames, calls }, null, 2)}`)
}

async function listToolCalls(request: APIRequestContext, headers: Record<string, string>, sessionId: string) {
  const page = await apiData(request, 'GET', `/assistant/sessions/${sessionId}/tool-calls?page=1&page_size=100`, headers)
  return (page.items || []) as JsonObject[]
}

async function listMessages(request: APIRequestContext, headers: Record<string, string>, sessionId: string) {
  return (await apiData(request, 'GET', `/assistant/sessions/${sessionId}/messages`, headers)) as JsonObject[]
}

async function expectOnlineHost(request: APIRequestContext, headers: Record<string, string>) {
  const data = await apiData(request, 'GET', '/hosts', headers)
  const hosts: JsonObject[] = Array.isArray(data) ? data : (data.hosts || data.items || [])
  const online = hosts.find(host => host.online)
  expect(online, `需要至少 1 台在线 Agent 主机，当前 hosts=${JSON.stringify(hosts, null, 2)}`).toBeTruthy()
}

function latestAssistantContent(messages: JsonObject[]) {
  const assistantMessages = messages.filter(message => message.role === 'assistant')
  expect(assistantMessages.length, JSON.stringify(messages, null, 2)).toBeGreaterThan(0)
  return String(assistantMessages[assistantMessages.length - 1].content || '')
}

function asObject(value: unknown): JsonObject {
  if (!value) return {}
  if (typeof value === 'string') return JSON.parse(value)
  return value as JsonObject
}

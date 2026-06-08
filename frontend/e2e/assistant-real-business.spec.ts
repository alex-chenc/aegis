import { test, expect, type APIRequestContext, type Page } from '@playwright/test'
import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || 'http://localhost:8081'
const API_URL = `${BASE_URL}/api/v1`
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'
const EXACT_PDF = '/tmp/test1-2.pdf'
const FALLBACK_PDF = '/tmp/test-1-2.pdf'

type JsonObject = Record<string, any>

test.describe.configure({ mode: 'serial' })
test.use({ actionTimeout: 10_000, navigationTimeout: 30_000 })

test('真实业务端口：助手权限、基线上传生成下发、资产采集、漏洞扫描', async ({ page, request }) => {
  test.setTimeout(15 * 60 * 1000)

  const pdfPath = resolvePdfPath()
  const pdfWasExact = pdfPath === EXACT_PDF
  const pdfMd5 = md5(pdfPath)

  await loginViaUi(page)
  const token = await authToken(page)
  const headers = authHeaders(token)

  await test.step('确认三档权限和上传入口在真实助手页面可见', async () => {
    await page.goto(`${BASE_URL}/assistant`)
    const approvalMode = page.locator('.approval-mode')
    await expect(approvalMode.getByText('请求确认')).toBeVisible()
    await expect(approvalMode.getByText('白名单')).toBeVisible()
    await expect(approvalMode.getByText('全权限')).toBeVisible()
    await expect(page.getByRole('button', { name: /上传/ })).toBeVisible()

    for (const [label, mode] of [
      ['全权限', 'full_access'],
      ['白名单', 'whitelist'],
      ['请求确认', 'request_approval'],
    ] as const) {
      await approvalMode.locator('.el-radio-button__inner').filter({ hasText: label }).click()
      await expect.poll(async () => (await apiData(request, 'GET', '/assistant/tool-approval-policy', headers)).mode).toBe(mode)
    }
  })

  await test.step('验证三档权限的真实工具调用行为', async () => {
    await setPolicy(request, headers, 'full_access')
    const full = await runAssistantPrompt(request, headers, 'full_access 自动资产概览', '请只调用 Asset.Summary.Get 工具获取资产概览，不要调用其他工具。')
    expect(full.toolCalls.some(call => call.tool_name === 'Asset.Summary.Get' && isCompleted(call.status))).toBeTruthy()
    expect(full.approvals.length).toBe(0)

    await setPolicy(request, headers, 'request_approval')
    const approvalRun = await runAssistantPrompt(request, headers, 'request_approval 拦截资产概览', '请只调用 Asset.Summary.Get 工具获取资产概览，不要调用其他工具。')
    const pendingApproval = approvalRun.approvals.find(item => item.status === 'pending')
    expect(pendingApproval, JSON.stringify(approvalRun, null, 2)).toBeTruthy()

    const approved = await apiData(request, 'POST', `/assistant/approvals/${pendingApproval!.approval_id}/approve`, headers, { comment: 'Playwright real business approval' })
    expect(approved.approval.status).toBe('executed')
    expect(approved.tool_result?.success).toBe(true)

    await setPolicy(request, headers, 'whitelist')
    const whitelistRead = await runAssistantPrompt(request, headers, 'whitelist 自动只读资产概览', '请只调用 Asset.Summary.Get 工具获取资产概览，不要调用其他工具。')
    expect(whitelistRead.toolCalls.some(call => call.tool_name === 'Asset.Summary.Get' && isCompleted(call.status))).toBeTruthy()
    expect(whitelistRead.approvals.length).toBe(0)

    const whitelistWrite = await runAssistantPrompt(request, headers, 'whitelist 拦截资产采集', '请只调用 Asset.Collection.Trigger 工具，参数 scope=all_hosts, types=["process"], force=true，不要调用其他工具。')
    expect(whitelistWrite.approvals.some(item => item.tool_name === 'Asset.Collection.Trigger' && item.status === 'pending')).toBeTruthy()
  })

  let templateId = ''
  let ruleId = ''
  let hostId = ''

  await test.step('基线页面上传 PDF 并等待解析完成', async () => {
    test.info().annotations.push({ type: 'pdf-path', description: pdfPath })
    test.info().annotations.push({ type: 'pdf-exact-path-present', description: String(pdfWasExact) })

    await page.goto(`${BASE_URL}/baseline/workbench`)
    await expect(page.getByText('模板上传')).toBeVisible()
    await expect(page.getByText('将基线文档拖到此处')).toBeVisible()

    const uploadResponsePromise = page.waitForResponse(resp => resp.url().includes('/api/v1/templates/upload'), { timeout: 120_000 })
    await page.locator('input[type="file"]').first().setInputFiles(pdfPath)
    const continueButton = page.getByRole('button', { name: '继续上传' })
    if (await continueButton.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await continueButton.click()
    }
    const uploadResponse = await uploadResponsePromise
    expect(uploadResponse.ok()).toBeTruthy()
    const uploadJson = await uploadResponse.json()
    templateId = uploadJson.data?.template_id || uploadJson.data?.id

    if (!templateId) {
      const templates = await apiData(request, 'GET', '/templates', headers)
      templateId = templates.find((tpl: JsonObject) => tpl.md5 === pdfMd5 || String(tpl.display_name || tpl.name || '').includes('test'))?.id
    }
    expect(templateId, JSON.stringify(uploadJson)).toBeTruthy()

    const status = await waitTemplateCompleted(request, headers, templateId)
    expect(status.status, status.message).toBe('completed')
    expect(status.message).toContain('解析完成')

    const rules = await apiData(request, 'GET', `/templates/${templateId}/rules`, headers)
    expect(rules.length).toBeGreaterThan(0)
    ruleId = rules[0].id
  })

  await test.step('基线页面触发检测/修复脚本生成并确认 sh 内容生成', async () => {
    await page.goto(`${BASE_URL}/baseline/workbench`)
    await expect(page.getByRole('button', { name: /一键生成检测脚本/ }).first()).toBeVisible({ timeout: 30_000 })

    const checkResponsePromise = page.waitForResponse(resp => resp.url().includes(`/api/v1/templates/${templateId}/generate-scripts`), { timeout: 120_000 })
    await page.getByRole('button', { name: /一键生成检测脚本/ }).first().click()
    expect((await checkResponsePromise).ok()).toBeTruthy()

    const fixResponsePromise = page.waitForResponse(resp => resp.url().includes(`/api/v1/templates/${templateId}/generate-scripts`), { timeout: 120_000 })
    await page.getByRole('button', { name: /一键生成修复脚本/ }).first().click()
    expect((await fixResponsePromise).ok()).toBeTruthy()

    const rules = await waitScriptsGenerated(request, headers, templateId)
    const generated = rules.find(rule => rule.check_script_status === 'generated' && rule.fix_script_status === 'generated')
    expect(generated, JSON.stringify(rules, null, 2)).toBeTruthy()
    expect(String(generated!.generated_check_script || '')).toContain('#!/bin/bash')
    expect(String(generated!.generated_fix_script || '')).toContain('#!/bin/bash')
    ruleId = generated!.id
  })

  await test.step('基线检测和修复任务可下发，任务中心可看到进度', async () => {
    const hosts = await apiData(request, 'GET', '/hosts', headers)
    const host = hosts.find((item: JsonObject) => item.online) || hosts[0]
    expect(host, JSON.stringify(hosts, null, 2)).toBeTruthy()
    hostId = host.id

    const checkTask = await apiData(request, 'POST', '/tasks/run-check', headers, { rule_ids: [ruleId], host_ids: [hostId] })
    expect(checkTask.task_group_id).toBeTruthy()
    const checkStatus = await apiData(request, 'GET', `/tasks/${checkTask.task_group_id}/status`, headers)
    expect(checkStatus.total).toBeGreaterThan(0)

    const fixTask = await apiData(request, 'POST', '/tasks/run-fix', headers, { rule_ids: [ruleId], host_ids: [hostId], task_group_id: checkTask.task_group_id })
    expect(fixTask.task_group_id).toBeTruthy()

    await page.goto(`${BASE_URL}/baseline/tasks/${checkTask.task_group_id}`)
    await expect(page.getByText(/任务|检测|修复/).first()).toBeVisible({ timeout: 30_000 })
  })

  await test.step('资产采集页面可触发采集并在任务详情中显示进度', async () => {
    await page.goto(`${BASE_URL}/hosts/assets`)
    await expect(page.getByRole('button', { name: /立即采集/ })).toBeVisible()

    const collectResponsePromise = page.waitForResponse(resp => resp.url().includes('/api/v1/host-assets/collections'), { timeout: 60_000 })
    await page.getByRole('button', { name: /立即采集/ }).click()
    await page.getByRole('button', { name: '确定' }).click()
    const collectResponse = await collectResponsePromise
    expect(collectResponse.ok()).toBeTruthy()
    const collectJson = await collectResponse.json()
    const taskId = collectJson.data?.task_id
    expect(taskId, JSON.stringify(collectJson)).toBeTruthy()

    const detail = await waitCollectionVisible(request, headers, taskId)
    expect(detail.task.id).toBe(taskId)
    expect(['collecting', 'analyzing', 'completed', 'failed', 'cancelled']).toContain(detail.task.status)
  })

  await test.step('漏洞检测页面可发起扫描并读取扫描状态', async () => {
    await page.goto(`${BASE_URL}/vulnerability`)
    await expect(page.getByRole('button', { name: /一键扫描/ })).toBeVisible()

    const scan = await apiData(request, 'POST', '/vulnerability/scan', headers, { host_ids: [hostId] })
    expect(scan.scan_id).toBeTruthy()

    const status = await waitScanStarted(request, headers, scan.scan_id)
    expect(['pending', 'scanning', 'analyzing', 'completed', 'failed', 'stopping', 'stopped']).toContain(status.status)
    expect(status.total_hosts).toBeGreaterThanOrEqual(1)
  })

  await setPolicy(request, headers, 'whitelist')
})

function resolvePdfPath() {
  if (existsSync(EXACT_PDF)) return EXACT_PDF
  if (existsSync(FALLBACK_PDF)) return FALLBACK_PDF
  throw new Error(`PDF 测试文件不存在：${EXACT_PDF}；也未找到备用文件 ${FALLBACK_PDF}`)
}

function md5(filePath: string) {
  return createHash('md5').update(readFileSync(filePath)).digest('hex')
}

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

async function apiData(request: APIRequestContext, method: 'GET' | 'POST' | 'PUT', path: string, headers: Record<string, string>, data?: JsonObject) {
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

async function runAssistantPrompt(request: APIRequestContext, headers: Record<string, string>, title: string, content: string) {
  const session = await apiData(request, 'POST', '/assistant/sessions', headers, {
    title: `Playwright ${title}`,
    task_type: 'operations',
  })
  await apiData(request, 'POST', `/assistant/sessions/${session.session_id}/message`, headers, { content })
  return waitAssistantSettled(request, headers, session.session_id)
}

async function waitAssistantSettled(request: APIRequestContext, headers: Record<string, string>, sessionId: string) {
  const started = Date.now()
  let last = { session: {}, toolCalls: [], approvals: [] } as { session: JsonObject; toolCalls: JsonObject[]; approvals: JsonObject[] }
  while (Date.now() - started < 180_000) {
    const [session, toolCallsPage, approvalsPage] = await Promise.all([
      apiData(request, 'GET', `/assistant/sessions/${sessionId}`, headers),
      apiData(request, 'GET', `/assistant/sessions/${sessionId}/tool-calls?page=1&page_size=50`, headers),
      apiData(request, 'GET', `/assistant/sessions/${sessionId}/approvals?page=1&page_size=50`, headers),
    ])
    last = {
      session,
      toolCalls: toolCallsPage.items || [],
      approvals: approvalsPage.items || [],
    }
    if (last.approvals.some(item => item.status === 'pending')) return last
    if (last.toolCalls.some(call => isCompleted(call.status) || call.status === 'approval_required' || call.status === 'failed')) return last
    if (['completed', 'failed', 'waiting_approval'].includes(session.status)) return last
    await new Promise(resolve => setTimeout(resolve, 3_000))
  }
  throw new Error(`assistant run did not settle: ${JSON.stringify(last, null, 2)}`)
}

function isCompleted(status: string) {
  return ['completed', 'success'].includes(status)
}

async function waitTemplateCompleted(request: APIRequestContext, headers: Record<string, string>, templateId: string) {
  const started = Date.now()
  let last: JsonObject = {}
  while (Date.now() - started < 240_000) {
    last = await apiData(request, 'GET', `/templates/${templateId}/status`, headers)
    if (last.status === 'completed') return last
    if (last.status === 'failed') return last
    await new Promise(resolve => setTimeout(resolve, 5_000))
  }
  throw new Error(`template parse timed out: ${JSON.stringify(last, null, 2)}`)
}

async function waitScriptsGenerated(request: APIRequestContext, headers: Record<string, string>, templateId: string) {
  const started = Date.now()
  let rules: JsonObject[] = []
  while (Date.now() - started < 10 * 60_000) {
    rules = await apiData(request, 'GET', `/templates/${templateId}/rules`, headers)
    const failures = rules.filter(rule => rule.check_script_status === 'failed' || rule.fix_script_status === 'failed')
    if (failures.length > 0) {
      throw new Error(`script generation failed: ${JSON.stringify(failures, null, 2)}`)
    }
    if (rules.some(rule => rule.check_script_status === 'generated' && rule.fix_script_status === 'generated')) {
      return rules
    }
    await new Promise(resolve => setTimeout(resolve, 8_000))
  }
  throw new Error(`script generation timed out: ${JSON.stringify(rules, null, 2)}`)
}

async function waitCollectionVisible(request: APIRequestContext, headers: Record<string, string>, taskId: string) {
  const started = Date.now()
  let detail: JsonObject = {}
  while (Date.now() - started < 60_000) {
    detail = await apiData(request, 'GET', `/host-assets/collections/${taskId}`, headers)
    if (detail.task?.id === taskId) return detail
    await new Promise(resolve => setTimeout(resolve, 2_000))
  }
  throw new Error(`collection task not visible: ${JSON.stringify(detail, null, 2)}`)
}

async function waitScanStarted(request: APIRequestContext, headers: Record<string, string>, scanId: string) {
  const started = Date.now()
  let status: JsonObject = {}
  while (Date.now() - started < 90_000) {
    status = await apiData(request, 'GET', `/vulnerability/scan/${scanId}/status`, headers)
    if (status.scan_id === scanId) return status
    await new Promise(resolve => setTimeout(resolve, 3_000))
  }
  throw new Error(`scan status not visible: ${JSON.stringify(status, null, 2)}`)
}

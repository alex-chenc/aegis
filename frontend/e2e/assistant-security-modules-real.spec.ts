import { test, expect, type APIRequestContext, type Page } from '@playwright/test'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || 'http://localhost:8081'
const API_URL = `${BASE_URL}/api/v1`
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'

type JsonObject = Record<string, any>

test.describe.configure({ mode: 'serial' })
test.use({ actionTimeout: 10_000, navigationTimeout: 30_000 })

test('真实业务端口：智能体适配资产采集、漏洞、异常检测和动态检测包', async ({ page, request }) => {
  test.setTimeout(60 * 60 * 1000)

  await loginViaUi(page)
  const token = await authToken(page)
  const headers = authHeaders(token)
  await setPolicy(request, headers, 'full_access')

  const host = await findOnlineHost(request, headers)
  const vuln = await findVulnerabilityWithOnlineHost(request, headers, host.id)
  const alert = await findAlertForHost(request, headers, host.id)
  const packageID = 'b1c4300a-d050-4b12-8b0f-b41fce167b1e'

  await test.step('智能体资产采集：采集本机 AI Agent 并同步资产库', async () => {
    const prompt = [
      '请严格按顺序调用工具，不要只文字说明：',
      `1 Asset.Collection.Trigger 参数 scope=hosts, host_ids=["${host.id}"], types=["process"], force=true。`,
      '2 Asset.Collection.Get 使用 Asset.Collection.Trigger 返回的 task_id 查询采集进度。',
      '3 Asset.Application.List 参数 category=ai_agent,page=1,page_size=20。',
      '4 Asset.Summary.Get。',
      '如果返回 asset_collection_sequence_complete=true 或 all_requested_tools_success=true，请直接基于 verified_result_summary 给结论。',
    ].join('\n')
    const run = await runAssistantPrompt(request, headers, 'AI 资产采集真实链路', prompt)
    const calls = await waitToolCalls(request, headers, run.session_id, [
      'Asset.Collection.Trigger',
      'Asset.Collection.Get',
      'Asset.Application.List',
      'Asset.Summary.Get',
    ], 8 * 60_000)
    expect(calls.filter(call => call.status === 'failed'), JSON.stringify(calls, null, 2)).toHaveLength(0)

    const aiAgents = await apiData(request, 'GET', '/host-assets/applications?category=ai_agent&page=1&page_size=20', headers)
    expect(aiAgents.total).toBeGreaterThan(0)
    expect(aiAgents.items.some((item: JsonObject) => item.name === 'claude-code')).toBeTruthy()
    const summary = await apiData(request, 'GET', '/host-assets/summary', headers)
    expect(summary.ai_agent_count).toBeGreaterThan(0)
  })

  await test.step('智能体漏洞链路：生成 POC/FIX，执行 POC 和修复任务', async () => {
    const generate = await runAssistantPrompt(request, headers, '漏洞脚本生成', [
      '请严格按顺序调用工具，不要只文字说明：',
      `1 Vulnerability.Script.Generate 参数 cve_id="${vuln.cve_id}", script_type="poc", host_ids=["${host.id}"]。`,
      `2 Vulnerability.Script.Generate 参数 cve_id="${vuln.cve_id}", script_type="fix", host_ids=["${host.id}"]。`,
    ].join('\n'))
    await waitToolCalls(request, headers, generate.session_id, ['Vulnerability.Script.Generate'], 8 * 60_000, { 'Vulnerability.Script.Generate': 2 })
    await waitVulnerabilityScriptsGenerated(request, headers, vuln.cve_id, 'poc')
    await waitVulnerabilityScriptsGenerated(request, headers, vuln.cve_id, 'fix')

    const execute = await runAssistantPrompt(request, headers, '漏洞 POC 与修复下发', [
      '请严格按顺序调用工具，不要只文字说明：',
      `1 Vulnerability.List 参数 query="${vuln.cve_id}",page=1,page_size=5。`,
      `2 Vulnerability.AffectedHosts 参数 vulnerability_id="${vuln.id}"。`,
      `3 Vulnerability.Script.Status 参数 cve_id="${vuln.cve_id}", script_type="poc"。`,
      `4 Vulnerability.Script.Status 参数 cve_id="${vuln.cve_id}", script_type="fix"。`,
      `5 Vulnerability.Script.Execute 参数 cve_id="${vuln.cve_id}", script_type="poc", host_ids=["${host.id}"]。`,
      `6 Vulnerability.Script.Execute 参数 cve_id="${vuln.cve_id}", script_type="fix", host_ids=["${host.id}"]。`,
    ].join('\n'))
    const calls = await waitToolCalls(request, headers, execute.session_id, [
      'Vulnerability.AffectedHosts',
      'Vulnerability.Script.Status',
      'Vulnerability.Script.Execute',
    ], 10 * 60_000, {
      'Vulnerability.Script.Status': 2,
      'Vulnerability.Script.Execute': 2,
    })
    expect(calls.filter(call => call.status === 'failed'), JSON.stringify(calls, null, 2)).toHaveLength(0)

    const taskGroups = calls
      .filter(call => call.tool_name === 'Vulnerability.Script.Execute')
      .map(call => call.result?.task_group_id || call.result?.result?.task_group_id)
      .filter(Boolean)
    expect(taskGroups.length).toBeGreaterThanOrEqual(2)
    for (const taskGroupID of taskGroups) {
      const status = await waitTaskFinished(request, headers, taskGroupID)
      expect(status.failed, JSON.stringify(status, null, 2)).toBe(0)
      expect(status.success).toBeGreaterThan(0)
    }
  })

  await test.step('智能体异常检测：规则识别、异常事件和 AI 研判', async () => {
    const run = await runAssistantPrompt(request, headers, '异常检测真实链路', [
      '请严格按顺序调用工具，不要只文字说明：',
      '1 Detection.Alert.List 参数 page=1,page_size=10。',
      `2 Detection.Alert.Get 参数 alert_id="${alert.alert_id}"。`,
      '3 Detection.Statistics.Get。',
      '4 Detection.Trend.Get 参数 hours=24。',
      '5 SigmaRule.List 参数 page=1,page_size=10,status="active"。',
      `6 Investigation.HostAttack.Analyze 参数 host_id="${host.id}"。`,
    ].join('\n'))
    const calls = await waitToolCalls(request, headers, run.session_id, [
      'Detection.Alert.List',
      'Detection.Alert.Get',
      'Detection.Statistics.Get',
      'Detection.Trend.Get',
      'SigmaRule.List',
      'Investigation.HostAttack.Analyze',
    ], 10 * 60_000)
    expect(calls.filter(call => call.status === 'failed'), JSON.stringify(calls, null, 2)).toHaveLength(0)

    const alerts = await apiData(request, 'GET', '/detection/alerts?page=1&page_size=10', headers)
    expect(alerts.total).toBeGreaterThan(0)
    const rules = await apiData(request, 'GET', '/detection/rules?page=1&page_size=10', headers)
    expect(rules.total).toBeGreaterThan(0)
  })

  await test.step('智能体动态检测包：列表、详情和构建任务入口', async () => {
    const draftPackageID = `codex-e2e-${Date.now()}`
    await createDetectionPackageDraft(request, headers, draftPackageID)
    const run = await runAssistantPrompt(request, headers, '动态检测包真实链路', [
      '请严格按顺序调用工具，不要只文字说明：',
      '1 Package.List 参数 page=1,page_size=20。',
      `2 Package.Get 参数 package_id="${packageID}"。`,
      `3 Package.Build.Start 参数 package_id="${draftPackageID}", operator="playwright"。`,
    ].join('\n'))
    const calls = await waitToolCalls(request, headers, run.session_id, [
      'Package.List',
      'Package.Get',
      'Package.Build.Start',
    ], 8 * 60_000)
    expect(calls.filter(call => call.status === 'failed'), JSON.stringify(calls, null, 2)).toHaveLength(0)

    const latestBuild = await apiData(request, 'GET', `/detection/packages/${draftPackageID}/latest-build`, headers)
    expect(latestBuild.package_id).toBe(draftPackageID)
    expect(latestBuild.status).toBeTruthy()
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

async function apiData(request: APIRequestContext, method: 'GET' | 'POST' | 'PUT', path: string, headers: Record<string, string>, data?: JsonObject) {
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

async function setPolicy(request: APIRequestContext, headers: Record<string, string>, mode: string) {
  await apiData(request, 'PUT', '/assistant/tool-approval-policy', headers, { mode })
  await expect.poll(async () => (await apiData(request, 'GET', '/assistant/tool-approval-policy', headers)).mode).toBe(mode)
}

async function runAssistantPrompt(request: APIRequestContext, headers: Record<string, string>, title: string, content: string) {
  const session = await apiData(request, 'POST', '/assistant/sessions', headers, {
    title: `Playwright ${title}`,
    task_type: 'operations',
  })
  await apiData(request, 'POST', `/assistant/sessions/${session.session_id}/message`, headers, { content })
  return session
}

async function waitToolCalls(
  request: APIRequestContext,
  headers: Record<string, string>,
  sessionId: string,
  expectedToolNames: string[],
  timeoutMs: number,
  minCounts: Record<string, number> = {},
) {
  const started = Date.now()
  let lastCalls: JsonObject[] = []
  while (Date.now() - started < timeoutMs) {
    const [session, page] = await Promise.all([
      apiData(request, 'GET', `/assistant/sessions/${sessionId}`, headers),
      apiData(request, 'GET', `/assistant/sessions/${sessionId}/tool-calls?page=1&page_size=100`, headers),
    ])
    lastCalls = page.items || []
    const failed = lastCalls.filter(call => call.status === 'failed')
    if (failed.length > 0) throw new Error(`assistant tools failed: ${JSON.stringify(failed, null, 2)}`)
    const hasAll = expectedToolNames.every(name => {
      const count = lastCalls.filter(call => call.tool_name === name && isCompleted(call.status)).length
      return count >= (minCounts[name] || 1)
    })
    if (hasAll) return lastCalls
    if (session.status === 'failed') throw new Error(`assistant session failed: ${JSON.stringify({ session, lastCalls }, null, 2)}`)
    await new Promise(resolve => setTimeout(resolve, 3_000))
  }
  throw new Error(`timed out waiting for tools ${expectedToolNames.join(', ')}: ${JSON.stringify(lastCalls, null, 2)}`)
}

function isCompleted(status: string) {
  return ['completed', 'success'].includes(status)
}

async function findOnlineHost(request: APIRequestContext, headers: Record<string, string>) {
  const hosts = await apiData(request, 'GET', '/hosts', headers)
  const host = hosts.find((item: JsonObject) => item.online)
  expect(host, JSON.stringify(hosts, null, 2)).toBeTruthy()
  return host
}

async function findVulnerabilityWithOnlineHost(request: APIRequestContext, headers: Record<string, string>, hostId: string) {
  const page = await apiData(request, 'GET', '/vulnerability?page=1&page_size=30', headers)
  for (const vuln of page.data as JsonObject[]) {
    const hosts = await apiData(request, 'GET', `/vulnerability/${vuln.cve_id}/affected-hosts`, headers)
    if (hosts.some((host: JsonObject) => host.id === hostId && host.online)) return vuln
  }
  throw new Error('no vulnerability found for online host')
}

async function findAlertForHost(request: APIRequestContext, headers: Record<string, string>, hostId: string) {
  const alerts = await apiData(request, 'GET', '/detection/alerts?page=1&page_size=20', headers)
  const alert = (alerts.data as JsonObject[]).find(item => item.host_id === hostId) || alerts.data[0]
  expect(alert, JSON.stringify(alerts, null, 2)).toBeTruthy()
  return alert
}

async function waitVulnerabilityScriptsGenerated(request: APIRequestContext, headers: Record<string, string>, cveID: string, scriptType: 'poc' | 'fix') {
  const started = Date.now()
  let last: JsonObject = {}
  while (Date.now() - started < 10 * 60_000) {
    last = await apiData(request, 'GET', `/vulnerability/${cveID}/host-scripts?script_type=${scriptType}`, headers)
    if (last.summary?.failed > 0) throw new Error(`${scriptType} generation failed: ${JSON.stringify(last, null, 2)}`)
    if (last.summary?.generated > 0) return last
    await new Promise(resolve => setTimeout(resolve, 5_000))
  }
  throw new Error(`${scriptType} generation timed out: ${JSON.stringify(last, null, 2)}`)
}

async function waitTaskFinished(request: APIRequestContext, headers: Record<string, string>, taskGroupID: string) {
  const started = Date.now()
  let last: JsonObject = {}
  while (Date.now() - started < 15 * 60_000) {
    last = await apiData(request, 'GET', `/tasks/${taskGroupID}/status`, headers)
    const total = Number(last.total || 0)
    const done = Number(last.success || 0) + Number(last.failed || 0) + Number(last.timeout || 0)
    if (total > 0 && done >= total) return last
    await new Promise(resolve => setTimeout(resolve, 5_000))
  }
  throw new Error(`task ${taskGroupID} timed out: ${JSON.stringify(last, null, 2)}`)
}

async function createDetectionPackageDraft(request: APIRequestContext, headers: Record<string, string>, packageID: string) {
  const hookPlan = [
    'schema_version: "aegis.ebpf_plugin.v1"',
    `plugin_id: "${packageID}"`,
    `package_id: "${packageID}"`,
    'version: "1.0.0"',
    'event_schema:',
    '  events:',
    '    1001:',
    '      name: "codex_e2e_event"',
    '      fields:',
    '        1: { name: "pid", type: "int" }',
  ].join('\n')
  return apiData(request, 'POST', '/detection/packages/drafts', headers, {
    package_id: packageID,
    target_version: '1.0.0',
    title: `Codex E2E ${packageID}`,
    description: 'Playwright dynamic detection package draft',
    cve_ids: ['CVE-2026-31431'],
    hook_plan_yaml: hookPlan,
    ebpf_source: 'int main(void) { return 0; }',
    sigma_rules_yaml: 'title: Codex E2E Rule\nstatus: experimental\nlogsource:\n  product: linux\ndetection:\n  selection:\n    EventID: 1001\n  condition: selection\n',
  })
}

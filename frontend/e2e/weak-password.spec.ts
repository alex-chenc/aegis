import { expect, test } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:5176'

test('weak password workflow supports dictionary selection, app status details, batch detection, and natural language dictionary generation', async ({ page }) => {
  test.setTimeout(60_000)
  let analyzeMode: 'empty' | 'candidate' = 'empty'
  let taskCreated = false
  let batchCreated = false
  let aiDictionaryCreated = false

  const finding = {
    id: 'finding-1',
    task_id: 'task-weak-1',
    account: 'default',
    matched_password_mask: '*********',
    source_path: '/etc/redis/redis.conf',
    field_path: 'requirepass',
    process_pid: 4321,
    match_status: 'confirmed',
  }

  const candidate = () => ({
    candidate_application_id: 'cand-redis-1',
    host_id: 'host-redis-1',
    asset_id: 'asset-redis-1',
    hostname: 'redis-01',
    ip_address: '10.10.0.8',
    application_name: 'redis',
    application_type: 'redis',
    application_version: '7.2',
    profile_id: 'redis_config_v1',
    confidence: 0.96,
    candidate_paths: ['/etc/redis/redis.conf'],
    credential_types: ['plaintext'],
    ai_reason: 'redis application asset contains config path',
    status: 'candidate',
    scan_status: taskCreated ? 'alert' : 'unscanned',
    matched_findings: taskCreated ? 1 : 0,
    last_task_id: taskCreated ? 'task-weak-1' : '',
    findings: taskCreated ? [finding] : [],
  })

  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { code: 0, data: { initialized: true } } })
  )
  await page.route('**/api/v1/auth/login', async route => {
    const body = route.request().postDataJSON() as { username?: string; password?: string }
    if (body.username !== 'admin' || body.password !== 'Admin@123') {
      await route.fulfill({ status: 401, json: { code: 1, message: 'invalid credentials' } })
      return
    }
    await route.fulfill({
      json: {
        code: 0,
        data: {
          token: 'playwright-token',
          username: 'admin',
          force_password_change: false,
          role: 'admin',
        },
      },
    })
  })
  await page.route('**/api/v1/notifications**', route =>
    route.fulfill({ json: { code: 0, data: { items: [], total: 0 } } })
  )

  await page.route('**/api/v1/weak-password/asset-applications/analyze', async route => {
    if (analyzeMode === 'empty') {
      await route.fulfill({
        json: {
          code: 0,
          message: '当前范围没有应用资产，请先采集资产',
          data: {
            analysis_id: '',
            status: 'failed',
            application_asset_count: 0,
            candidate_count: 0,
            error_code: 'no_application_assets',
            message: '当前范围没有应用资产，请先采集资产',
            candidates: [],
          },
        },
      })
      return
    }
    await route.fulfill({
      json: {
        code: 0,
        data: {
          analysis_id: 'analysis-1',
          status: 'completed',
          application_asset_count: 1,
          candidate_count: 1,
          candidates: [candidate()],
        },
      },
    })
  })

  await page.route('**/api/v1/weak-password/asset-applications**', async route => {
    if (route.request().method() !== 'GET') {
      await route.fallback()
      return
    }
    await route.fulfill({
      json: {
        code: 0,
        data: {
          items: analyzeMode === 'candidate' ? [candidate()] : [],
          total: analyzeMode === 'candidate' ? 1 : 0,
        },
      },
    })
  })
  await page.route('**/api/v1/weak-password/tasks/by-application', async route => {
    const body = route.request().postDataJSON() as any
    expect(body.dictionary_policy.use_default_1000).toBe(true)
    expect(body.dictionary_policy.hybrid).toBeUndefined()
    expect(body.dictionary_policy.fuzzy).toBeUndefined()
    expect(body.ai_policy.encrypted_password_llm_match).toBeUndefined()
    taskCreated = true
    await route.fulfill({ json: { code: 0, data: { task_id: 'task-weak-1', scan_application_id: 'scan-app-1', status: 'pending' } } })
  })
  await page.route('**/api/v1/weak-password/tasks/by-applications', async route => {
    const body = route.request().postDataJSON() as any
    expect(body.candidate_application_ids).toEqual(['cand-redis-1'])
    expect(body.dictionary_policy.use_default_1000).toBe(true)
    batchCreated = true
    await route.fulfill({
      json: {
        code: 0,
        data: {
          created: [{ candidate_application_id: 'cand-redis-1', task_id: 'task-batch-1', scan_application_id: 'scan-batch-1', status: 'pending' }],
          skipped: [],
        },
      },
    })
  })
  await page.route('**/api/v1/weak-password/tasks?*', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          items: taskCreated || batchCreated ? [{
            id: 'task-weak-1',
            name: '弱密码检查 - redis',
            status: 'completed',
            progress: 100,
            current_stage: 'completed',
            matched_findings: 1,
            failed_applications: 0,
          }] : [],
          total: taskCreated || batchCreated ? 1 : 0,
        },
      },
    })
  )
  await page.route('**/api/v1/weak-password/tasks/task-weak-1/progress', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          task_id: 'task-weak-1',
          status: 'completed',
          progress: 100,
          current_stage: 'completed',
          current_host_id: 'host-redis-1',
          current_application: 'redis',
          agent_tool_call_count: 1,
          max_agent_tool_calls: 10,
          last_agent_tool: 'WeakPassword.CollectCredentials',
          last_error_code: '',
          message: 'completed',
        },
      },
    })
  )
  await page.route('**/api/v1/weak-password/tasks/task-weak-1/hosts**', route =>
    route.fulfill({ json: { code: 0, data: { items: [{ id: 'host-row', host_id: 'host-redis-1', hostname: 'redis-01', ip_address: '10.10.0.8', agent_status: 'online', status: 'completed', collected_records: 1, matched_findings: 1, error_code: '' }], total: 1 } } })
  )
  await page.route('**/api/v1/weak-password/tasks/task-weak-1/findings**', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          items: [{
            id: 'finding-1',
            task_id: 'task-weak-1',
            host_id: 'host-redis-1',
            application_name: 'redis',
            application_type: 'redis',
            account: 'default',
            credential_type: 'plaintext',
            match_status: 'confirmed',
            matched_password_mask: '*********',
            match_source: 'default_1000',
            match_rule: 'dictionary_exact',
            confidence: 1,
            source_path: '/etc/redis/redis.conf',
            field_path: 'requirepass',
            created_at: '2026-06-23T00:00:00Z',
          }],
          total: 1,
        },
      },
    })
  )
  await page.route('**/api/v1/weak-password/tasks/task-weak-1/errors**', route =>
    route.fulfill({ json: { code: 0, data: { items: [], total: 0 } } })
  )
  await page.route('**/api/v1/weak-password/tasks/task-weak-1', async route => {
    if (route.request().method() === 'DELETE') {
      taskCreated = false
      await route.fulfill({ json: { code: 0, data: { deleted: 1 } } })
      return
    }
    await route.fulfill({
      json: {
        code: 0,
        data: {
          task: { id: 'task-weak-1', name: '弱密码检查 - redis', status: 'completed', progress: 100, current_stage: 'completed' },
          errors: [],
        },
      },
    })
  })
  await page.route('**/api/v1/weak-password/findings/finding-1/reveal', async route => {
    const body = route.request().postDataJSON() as { password?: string }
    if (body.password !== 'Admin@123') {
      await route.fulfill({ status: 401, json: { code: 401, message: '系统密码错误' } })
      return
    }
    await route.fulfill({
      json: {
        code: 0,
        data: {
          finding_id: 'finding-1',
          application_name: 'redis',
          account: 'default',
          credential_type: 'plaintext',
          matched_password: 'Admin@123',
          source_path: '/etc/redis/redis.conf',
          field_path: 'requirepass',
        },
      },
    })
  })
  await page.route('**/api/v1/weak-password/dictionaries/default', route =>
    route.fulfill({ json: { code: 0, data: { id: 'dict-default', name: '默认弱密码字典', dictionary_type: 'default_1000', status: 'enabled', entry_count: 1000, source: 'built_in', categories: ['通用弱口令'] } } })
  )
  await page.route('**/api/v1/weak-password/dictionaries/dict-default/entries**', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          items: [
            { id: 'entry-1', dictionary_id: 'dict-default', candidate: 'Admin@123', category: '通用弱口令', rule_source: 'built_in', risk_level: 'high' },
            { id: 'entry-2', dictionary_id: 'dict-default', candidate: 'admin123', category: '通用弱口令', rule_source: 'built_in', risk_level: 'high' },
          ],
          total: 2,
        },
      },
    })
  )
  await page.route('**/api/v1/weak-password/dictionaries/ai-generate', async route => {
    const body = route.request().postDataJSON() as any
    expect(body.natural_language).toContain('Redis')
    aiDictionaryCreated = true
    await route.fulfill({
      json: {
        code: 0,
        data: { id: 'dict-ai', name: 'AI 生成弱密码字典', dictionary_type: 'ai_generated', status: 'enabled', entry_count: body.count, source: 'ai_generated', categories: [] },
      },
    })
  })
  await page.route('**/api/v1/weak-password/dictionaries**', async route => {
    const url = new URL(route.request().url())
    if (!url.pathname.endsWith('/api/v1/weak-password/dictionaries')) {
      await route.fallback()
      return
    }
    await route.fulfill({
      json: {
        code: 0,
        data: {
          items: [
            { id: 'dict-default', name: '默认弱密码字典', dictionary_type: 'default_1000', status: 'enabled', entry_count: 1000, source: 'built_in', categories: ['通用弱口令'] },
            ...(aiDictionaryCreated ? [{ id: 'dict-ai', name: 'AI 生成弱密码字典', dictionary_type: 'ai_generated', status: 'enabled', entry_count: 20, source: 'ai_generated', categories: [] }] : []),
          ],
          total: aiDictionaryCreated ? 2 : 1,
        },
      },
    })
  })

  await page.goto(`${baseURL}/login`)
  await page.getByPlaceholder('请输入账号').fill('admin')
  await page.getByPlaceholder('请输入密码').fill('Admin@123')
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForFunction(() => {
    const raw = window.localStorage.getItem('aegis-auth')
    return raw && JSON.parse(raw).username === 'admin'
  })

  await page.goto(`${baseURL}/risk/weak-password`)
  await expect(page.getByText('检测结果')).toHaveCount(0)
  await page.getByRole('button', { name: '一键分析资产应用' }).click()
  await expect(page.getByText('暂无可分析的应用资产。')).toBeVisible()

  analyzeMode = 'candidate'
  await page.getByRole('button', { name: '一键分析资产应用' }).click()
  await expect(page.getByText('redis-01')).toBeVisible()
  await expect(page.getByText('未扫描')).toBeVisible()
  await expect(page.getByText('凭据类型')).toHaveCount(0)
  await expect(page.getByText('查看分析依据')).toHaveCount(0)

  await page.getByRole('button', { name: '检查弱密码' }).click()
  await expect(page.getByText('默认弱密码字典（1000 条）')).toBeVisible()
  await expect(page.getByText('混合规则')).toHaveCount(0)
  await expect(page.getByText('模糊规则')).toHaveCount(0)
  await expect(page.getByText('加密/hash LLM 匹配')).toHaveCount(0)
  await page.getByRole('button', { name: '确认检查' }).click()
  await expect(page).toHaveURL(/\/risk\/weak-password\/tasks\/task-weak-1/)
  await expect(page.getByText('redis-01')).toBeVisible()
  await expect(page.getByText('*********')).toBeVisible()

  await page.goto(`${baseURL}/risk/weak-password`, { waitUntil: 'domcontentloaded' })
  await expect(page.getByText('告警', { exact: true })).toBeVisible()
  await page.getByText('告警', { exact: true }).click()
  await expect(page.getByText('弱密码详情')).toBeVisible()
  await expect(page.getByText('4321')).toBeVisible()
  await page.getByRole('button', { name: '查看明文' }).click()
  await page.locator('.el-message-box input').fill('Admin@123')
  await page.getByRole('button', { name: '查看', exact: true }).click()
  await expect(page.getByText('Admin@123')).toBeVisible()

  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '一键检测' }).click()
  await page.getByRole('button', { name: '确认检查' }).click()
  await expect(page.getByText('检查任务')).toBeVisible()

  await page.goto(`${baseURL}/risk/weak-password/dictionaries`)
  await expect(page.getByRole('heading', { name: '弱密码字典' })).toBeVisible()
  await expect(page.getByText('分类')).toHaveCount(0)
  await expect(page.getByText('来源')).toHaveCount(0)
  await expect(page.getByText('状态')).toHaveCount(0)
  await expect(page.getByText('内置').first()).toBeVisible()
  await page.getByRole('button', { name: 'AI 一键生成字典' }).click()
  await page.getByPlaceholder(/为 Redis 管理员/).fill('为 Redis 生产环境生成弱密码字典，包含 aegis、admin 和年份')
  await page.getByRole('button', { name: '生成并保存' }).click()
  await expect(page.getByText('已生成 200 条候选')).toBeVisible()
  await page.locator('.el-drawer').getByRole('button', { name: '取消' }).click()
  await page.getByRole('button', { name: '查看条目' }).first().click()
  await expect(page.getByText('Admin@123')).toBeVisible()
  await expect(page.getByText('admin123')).toBeVisible()
})

import { expect, test } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:5176'

test('weak password workflow shows asset analysis, task progress, and dictionaries', async ({ page }) => {
  let analyzeMode: 'empty' | 'candidate' = 'empty'
  let taskCreated = false

  const candidate = {
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
  }

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
          candidates: [candidate],
        },
      },
    })
  })

  await page.route('**/api/v1/weak-password/asset-applications**', async route => {
    if (route.request().method() !== 'GET') {
      await route.fallback()
      return
    }
    await route.fulfill({ json: { code: 0, data: { items: analyzeMode === 'candidate' ? [candidate] : [], total: analyzeMode === 'candidate' ? 1 : 0 } } })
  })
  await page.route('**/api/v1/weak-password/tasks/by-application', async route => {
    taskCreated = true
    await route.fulfill({ json: { code: 0, data: { task_id: 'task-weak-1', scan_application_id: 'scan-app-1', status: 'pending' } } })
  })
  await page.route('**/api/v1/weak-password/tasks?*', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          items: taskCreated ? [{
            id: 'task-weak-1',
            name: '弱密码检查 - redis',
            status: 'failed',
            progress: 100,
            current_stage: 'config_discovery_failed',
            matched_findings: 0,
            failed_applications: 1,
          }] : [],
          total: taskCreated ? 1 : 0,
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
          status: 'failed',
          progress: 100,
          current_stage: 'config_discovery_failed',
          current_host_id: 'host-redis-1',
          current_application: 'redis',
          agent_tool_call_count: 10,
          max_agent_tool_calls: 10,
          last_agent_tool: 'WeakPassword.ServiceUnitInspect',
          last_error_code: 'config_discovery_failed',
          message: 'AI 已尝试 10 次受控 Agent 工具调用',
        },
      },
    })
  )
  await page.route('**/api/v1/weak-password/tasks/task-weak-1/hosts', route =>
    route.fulfill({ json: { code: 0, data: { items: [{ id: 'host-row', host_id: 'host-redis-1', hostname: 'redis-01', ip_address: '10.10.0.8', agent_status: 'online', status: 'failed', collected_records: 1, matched_findings: 1, error_code: 'config_discovery_failed' }], total: 1 } } })
  )
  await page.route('**/api/v1/weak-password/tasks/task-weak-1/findings', route =>
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
          task: { id: 'task-weak-1', name: '弱密码检查 - redis', status: 'failed', progress: 100, current_stage: 'config_discovery_failed' },
          errors: [{ id: 'err-1', application_name: 'redis', error_code: 'config_discovery_failed', agent_tool_call_count: 10, source_path: '/etc/redis/redis.conf', error_message: '配置文件读取失败' }],
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
  await page.route('**/api/v1/weak-password/dictionaries', route =>
    route.fulfill({ json: { code: 0, data: { items: [{ id: 'dict-default', name: '默认弱密码字典', dictionary_type: 'default_1000', status: 'enabled', entry_count: 1000, source: 'built_in', categories: ['通用弱口令'] }], total: 1 } } })
  )

  await page.goto(`${baseURL}/login`)
  await page.getByPlaceholder('请输入账号').fill('admin')
  await page.getByPlaceholder('请输入密码').fill('Admin@123')
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForFunction(() => {
    const raw = window.localStorage.getItem('aegis-auth')
    return raw && JSON.parse(raw).username === 'admin'
  })

  await page.goto(`${baseURL}/risk/weak-password`)
  await expect(page.locator('.sidebar-menu').getByText('风险管理')).toHaveCount(0)
  await expect(page.locator('.sidebar-menu').getByText('智能弱密码检测')).toBeVisible()

  await page.getByRole('button', { name: '一键分析资产应用' }).click()
  await expect(page.getByText('暂无可分析的应用资产。')).toBeVisible()
  await expect(page.getByRole('button', { name: '去采集资产' })).toBeVisible()

  analyzeMode = 'candidate'
  await page.getByRole('button', { name: '一键分析资产应用' }).click()
  await expect(page.getByText('redis-01')).toBeVisible()
  await expect(page.getByText('10.10.0.8')).toBeVisible()
  await expect(page.getByText('风险说明')).toHaveCount(0)

  await page.getByRole('button', { name: '检查弱密码' }).click()
  await page.getByRole('button', { name: '确认检查' }).click()
  await expect(page).toHaveURL(/\/risk\/weak-password\/tasks\/task-weak-1/)
  await expect(page.getByText('Agent 工具')).toHaveCount(0)
  await expect(page.getByText('最近工具')).toHaveCount(0)
  await expect(page.getByText('10/10')).toHaveCount(0)
  await expect(page.getByText('redis-01')).toBeVisible()
  await expect(page.getByText('10.10.0.8')).toBeVisible()
  await expect(page.getByText('*********')).toBeVisible()
  await expect(page.getByText('config_discovery_failed').first()).toBeVisible()
  await page.getByRole('button', { name: '详情' }).click()
  await expect(page.getByText('请输入当前系统密码')).toBeVisible()
  await page.locator('.el-message-box input').fill('Admin@123')
  await page.getByRole('button', { name: '查看' }).click()
  await expect(page.getByText('完整密码')).toBeVisible()
  await expect(page.getByText('Admin@123')).toBeVisible()
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: '删除' }).click()
  await expect(page.getByText('确定删除此弱密码检测任务吗？')).toBeVisible()
  await page.locator('.el-message-box').getByRole('button', { name: '删除' }).click()
  await expect(page).toHaveURL(/\/risk\/weak-password$/)

  await page.goto(`${baseURL}/risk/weak-password/dictionaries`)
  await expect(page.getByText('默认弱密码字典').first()).toBeVisible()
  await expect(page.getByText('1000').first()).toBeVisible()
})

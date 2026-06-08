import { expect, test } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:5176'

test('assistant mode shows permissions, uploads context, and renders task progress', async ({ page }) => {
  let updatedMode = ''
  let uploadedContextVisible = false

  const session = {
    id: 'session-row-1',
    session_id: 'sess-playwright',
    title: '智能体任务进度验收',
    task_type: 'operations',
    status: 'active',
    created_by: 'tester',
    message_count: 2,
    tool_call_count: 1,
    approval_count: 0,
    metadata: {},
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }

  const toolCall = {
    id: 'tool-row-1',
    session_id: session.session_id,
    message_id: 'msg-assistant-1',
    call_id: 'call-task-1',
    tool_name: 'Task.RunCheck',
    domain: 'baseline',
    risk_level: 'medium',
    status: 'success',
    args: { rule_ids: ['rule-1'], host_ids: ['host-1', 'host-2'] },
    result: {
      task_group_id: 'tg-playwright-1',
      task_type: 'CHECK',
      task_ids: ['task-1', 'task-2'],
      task_ref: {
        kind: 'baseline_task',
        id: 'tg-playwright-1',
        task_group_id: 'tg-playwright-1',
        route_path: '/baseline',
        status_url: '/api/v1/tasks/tg-playwright-1/status',
      },
    },
    duration_ms: 842,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }

  const baseContextRefs = [{
    id: 'ctx-existing',
    session_id: session.session_id,
    object_type: 'file',
    object_id: 'file-existing',
    title: 'initial-context.txt',
    summary: '已有上下文摘要',
    created_at: new Date().toISOString(),
  }]

  await page.addInitScript(() => {
    window.localStorage.setItem('aegis-auth', JSON.stringify({
      token: 'playwright-token',
      username: 'tester',
      forcePasswordChange: false,
      role: 'admin',
    }))
  })

  await page.route('**/api/v1/assistant/tool-approval-policy', async route => {
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON() as { mode?: string }
      updatedMode = body.mode || ''
      await route.fulfill({ json: { code: 0, message: 'ok' } })
      return
    }
    await route.fulfill({ json: { code: 0, data: { mode: updatedMode || 'whitelist' } } })
  })

  await page.route('**/api/v1/assistant/sessions?page=*', route =>
    route.fulfill({ json: { code: 0, data: { sessions: [session], total: 1 } } })
  )
  await page.route(`**/api/v1/assistant/sessions/${session.session_id}`, route =>
    route.fulfill({ json: { code: 0, data: session } })
  )
  await page.route(`**/api/v1/assistant/sessions/${session.session_id}/messages`, route =>
    route.fulfill({
      json: {
        code: 0,
        data: [
          {
            id: 'msg-user-1',
            session_id: session.session_id,
            message_id: 'msg-user-1',
            role: 'user',
            content: '执行基线检查并展示进度',
            created_at: new Date().toISOString(),
          },
          {
            id: 'msg-assistant-1',
            session_id: session.session_id,
            message_id: 'msg-assistant-1',
            role: 'assistant',
            content: '已创建基线检测任务，进度如下。',
            thinking: ['正在调用工具: Task.RunCheck'],
            tool_calls: [toolCall],
            created_at: new Date().toISOString(),
          },
        ],
      },
    })
  )
  await page.route(`**/api/v1/assistant/sessions/${session.session_id}/tool-calls*`, route =>
    route.fulfill({ json: { code: 0, data: { items: [toolCall], total: 1, page: 1, page_size: 100 } } })
  )
  await page.route(`**/api/v1/assistant/sessions/${session.session_id}/approvals*`, route =>
    route.fulfill({ json: { code: 0, data: { items: [], total: 0, page: 1, page_size: 20 } } })
  )
  await page.route(`**/api/v1/assistant/sessions/${session.session_id}/context-refs`, route =>
    route.fulfill({
      json: {
        code: 0,
        data: uploadedContextVisible
          ? [
              ...baseContextRefs,
              {
                id: 'ctx-uploaded',
                session_id: session.session_id,
                object_type: 'file',
                object_id: 'file-uploaded',
                title: 'analysis.txt',
                summary: 'Playwright 上传文件摘要',
                created_at: new Date().toISOString(),
              },
            ]
          : baseContextRefs,
      },
    })
  )
  await page.route(`**/api/v1/assistant/sessions/${session.session_id}/files`, async route => {
    uploadedContextVisible = true
    await route.fulfill({
      json: {
        code: 0,
        data: {
          purpose: 'analysis',
          filename: 'analysis.txt',
          size: 18,
          context_ref: {
            id: 'ctx-uploaded',
            session_id: session.session_id,
            object_type: 'file',
            object_id: 'file-uploaded',
            title: 'analysis.txt',
            summary: 'Playwright 上传文件摘要',
            created_at: new Date().toISOString(),
          },
        },
      },
    })
  })
  await page.route('**/api/v1/tasks/tg-playwright-1/status', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          task_group_id: 'tg-playwright-1',
          status: 'running',
          total: 2,
          pending: 0,
          running: 1,
          success: 1,
          failed: 0,
          timeout: 0,
        },
      },
    })
  )
  await page.route('**/api/v1/tasks/tg-playwright-1/logs', route =>
    route.fulfill({
      json: {
        code: 0,
        data: [{
          id: 'task-2',
          task_group_id: 'tg-playwright-1',
          host_id: 'host-2',
          hostname: 'web-02',
          task_type: 'CHECK',
          status: 'RUNNING',
          created_at: new Date().toISOString(),
        }],
      },
    })
  )

  await page.goto(`${baseURL}/assistant?session=${session.session_id}`)

  await expect(page.getByText('请求确认')).toBeVisible()
  await expect(page.getByText('白名单')).toBeVisible()
  await expect(page.getByText('全权限')).toBeVisible()
  await expect(page.getByText('上传')).toBeVisible()

  await page.getByText('全权限').click()
  await expect.poll(() => updatedMode).toBe('full_access')

  await expect(page.locator('.tool-result-card .tool-name', { hasText: 'Task.RunCheck' })).toBeVisible()
  await expect(page.getByText('基线任务进度')).toBeVisible()
  await expect(page.getByText('运行中')).toBeVisible()
  await expect(page.getByText('会话上下文')).toBeVisible()
  await expect(page.getByText('initial-context.txt')).toBeVisible()

  await page.locator('input[type="file"]').setInputFiles({
    name: 'analysis.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('Playwright upload'),
  })
  await expect(page.getByText('analysis.txt')).toBeVisible()

  await page.screenshot({
    path: 'test-results/assistant-ui-playwright.png',
    fullPage: true,
  })
})

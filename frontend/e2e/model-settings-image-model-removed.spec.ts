import { expect, test } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:5176'

test('model settings only loads text LLM configuration after image model removal', async ({ page }) => {
  const imageModelRequests: string[] = []
  const failedApiRequests: string[] = []

  page.on('requestfailed', request => {
    if (request.url().includes('/api/v1/')) {
      failedApiRequests.push(request.url())
    }
  })

  await page.addInitScript(() => {
    window.localStorage.setItem('aegis-auth', JSON.stringify({
      token: 'playwright-token',
      username: 'tester',
      forcePasswordChange: false,
      role: 'admin',
    }))
  })

  await page.route('**/api/v1/config/llm', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          api_key_masked: 'sk-****test',
          provider: 'deepseek',
          base_url: 'https://api.deepseek.com/v1',
          model_name: 'deepseek-chat',
          is_active: true,
        },
      },
    })
  )

  await page.route('**/api/v1/notifications**', route =>
    route.fulfill({
      json: {
        code: 0,
        data: {
          list: [],
          total: 0,
          unread_count: 0,
        },
      },
    })
  )

  await page.route('**/api/v1/config/image-model**', route => {
    imageModelRequests.push(route.request().url())
    return route.fulfill({
      status: 410,
      json: { code: 410, message: 'image model endpoint has been removed' },
    })
  })

  await page.goto(`${baseURL}/settings/models`)

  await expect(page.getByRole('heading', { name: '模型配置' })).toBeVisible()
  await expect(page.getByText('文本 LLM 配置')).toBeVisible()
  await expect(page.getByText('模型厂商')).toBeVisible()
  await expect(page.getByText('保存配置')).toBeVisible()
  await expect(page.getByText('测试连接')).toBeVisible()

  await expect(page.getByText('图片模型配置')).toHaveCount(0)
  await expect(page.getByText('图片厂商')).toHaveCount(0)
  await expect(page.getByText('测试图片连接')).toHaveCount(0)
  await expect(page.getByText('保存图片配置')).toHaveCount(0)
  await expect(page.getByText('图片模型 API Key')).toHaveCount(0)
  expect(imageModelRequests).toEqual([])
  expect(failedApiRequests).toEqual([])
})

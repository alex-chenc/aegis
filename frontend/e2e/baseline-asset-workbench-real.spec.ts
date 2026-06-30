import { expect, test } from '@playwright/test'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

const BASE_URL = process.env.AEGIS_E2E_BASE_URL || 'http://localhost:8081'
const USERNAME = process.env.AEGIS_E2E_USERNAME || 'admin'
const PASSWORD = process.env.AEGIS_E2E_PASSWORD || 'Admin@123'
const BASELINE_PDF = process.env.AEGIS_BASELINE_PDF || '/root/test-1-2.pdf'

test.describe('baseline rule management and asset drawer real workflow', () => {
  test.skip(process.env.PLAYWRIGHT_REAL !== '1', 'Set PLAYWRIGHT_REAL=1 to run against a live Aegis stack')

  test('validates host asset drawer, sidebar collapse, parser progress, dispatch chooser, and task center', async ({ page }) => {
    test.setTimeout(360_000)
    const browserErrors: string[] = []
    let uploadPath = ''
    let uploadedTemplateId = ''

    page.on('pageerror', err => browserErrors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error' && !msg.text().includes('ResizeObserver loop')) {
        browserErrors.push(msg.text())
      }
    })

    try {
      uploadPath = createUniqueBaselineUpload()
      await loginViaUi(page)

      await page.goto(`${BASE_URL}/hosts`, { waitUntil: 'domcontentloaded' })
      await expect(page.getByRole('heading', { name: '主机资产态势' })).toBeVisible()

      await page.locator('.collapse-button').click()
      await expect(page.locator('.sidebar.collapsed')).toBeVisible()
      const collapsedBox = await page.locator('.sidebar').boundingBox()
      expect(collapsedBox?.width).toBeLessThanOrEqual(80)
      await page.locator('.collapse-button').click()
      await expect(page.locator('.sidebar:not(.collapsed)')).toBeVisible()

      const ipLink = page.locator('.el-table .el-link').filter({ hasText: /\d+\.\d+\.\d+\.\d+/ }).last()
      await expect(ipLink).toBeVisible({ timeout: 30_000 })
      await ipLink.click()
      const drawer = page.locator('.el-drawer').filter({ hasText: '软件清单' })
      await expect(drawer).toBeVisible({ timeout: 30_000 })
      for (const label of ['软件清单', '数据库', 'Web 服务', 'Web 站点', 'Web 框架', 'AI LLM', 'AI Agent', 'MCP']) {
        await expect(drawer.locator('.asset-nav button').filter({ hasText: label })).toBeVisible()
      }
      await drawer.locator('.asset-nav button').filter({ hasText: '数据库' }).click()
      await expect(drawer.locator('.asset-section h3')).toHaveText('数据库')
      await drawer.locator('.asset-nav button').filter({ hasText: '软件清单' }).click()
      const softwareSection = drawer.locator('.asset-section').filter({ hasText: '软件清单' }).first()
      await expect(softwareSection.locator('.asset-item').first()).toBeVisible({ timeout: 30_000 })
      await expect(softwareSection.locator('.el-pagination')).toBeVisible()

      await page.goto(`${BASE_URL}/baseline/workbench`, { waitUntil: 'domcontentloaded' })
      await expect(page.getByRole('heading', { name: '规则管理' })).toBeVisible()
      await expect(page.getByRole('button', { name: /文件解析/ })).toBeVisible()
      await expect(page.getByText('规则列表')).toBeVisible()

      await page.getByRole('button', { name: /文件解析/ }).click()
      await expect(page.getByRole('dialog', { name: '文件解析' })).toBeVisible()
      const uploadResponsePromise = page.waitForResponse(response =>
        response.url().includes('/api/v1/templates/upload') && response.request().method() === 'POST'
      )
      await page.locator('.el-dialog input[type="file"]').setInputFiles(uploadPath)
      const uploadResponse = await uploadResponsePromise
      const uploadPayload = await uploadResponse.json()
      uploadedTemplateId = uploadPayload.data.template_id
      const parseProgress = page.locator('.upload-progress').filter({ hasText: '解析进度' })
      await expect(parseProgress).toBeVisible({ timeout: 45_000 })
      await expect(parseProgress).toContainText(/(20|40|60|80|90|100|0)%/, { timeout: 45_000 })
      const terminalStatus = await waitForTemplateTerminalStatus(page, uploadedTemplateId)
      expect(terminalStatus.status).toBe('completed')
      expect(terminalStatus.progress).toBe(100)
      await expect(parseProgress).toContainText(/(解析完成|100%)/, { timeout: 15_000 })

      await page.keyboard.press('Escape')
      await expect(page.getByText('规则列表')).toBeVisible()
      await expect(page.getByText('文件视角')).toBeVisible()
      await expect(page.getByText('全部视角')).toBeVisible()
      await expect(page.locator('.file-rule-groups')).toBeVisible({ timeout: 60_000 })
      await page.getByRole('button', { name: /任务下发/ }).click()
      const dispatchDialog = page.getByRole('dialog', { name: '任务下发' })
      await expect(dispatchDialog).toBeVisible()
      await expect(dispatchDialog.getByPlaceholder('搜索规则')).toBeVisible()
      await expect(dispatchDialog.getByPlaceholder('搜索主机名或 IP')).toBeVisible()
      await expect(dispatchDialog.getByText('最大轮数')).toBeVisible()
      await dispatchDialog.getByRole('spinbutton').fill('2')
      await expect(dispatchDialog.getByRole('spinbutton')).toHaveValue('2')
      await page.getByRole('button', { name: '取消' }).click()

      const generatedScriptButton = page.locator('.rules-card .script-status .el-button--success').first()
      if (await generatedScriptButton.count()) {
        await generatedScriptButton.click()
        await expect(page.locator('.cm-editor')).toBeVisible({ timeout: 30_000 })
        await expect(page.getByRole('button', { name: /编辑/ })).toBeVisible()
        await page.keyboard.press('Escape')
      }

      await page.goto(`${BASE_URL}/baseline/tasks`, { waitUntil: 'domcontentloaded' })
      await expect(page.getByText('任务组', { exact: true }).first()).toBeVisible()
      await expect(page.getByText('平均通过率')).toBeVisible()
      await expect(page.locator('.live-indicator')).toContainText(/实时/)
      await expect(page.getByText('最后刷新')).toBeVisible()
      await expect(page.getByRole('button', { name: '合规报告' })).toBeVisible()
      await page.getByRole('button', { name: '合规报告' }).click()
      await expect(page.getByRole('menuitem', { name: '导出 PDF' })).toBeVisible()
      await expect(page.getByRole('menuitem', { name: '导出 Excel' })).toBeVisible()
    } finally {
      if (uploadPath) {
        fs.rmSync(uploadPath, { force: true })
      }
      if (uploadedTemplateId) {
        await deleteTemplateViaUiSession(page, uploadedTemplateId).catch(() => undefined)
      }
    }

    expect(browserErrors).toEqual([])
  })
})

async function loginViaUi(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/login`, { waitUntil: 'domcontentloaded' })
  await page.getByPlaceholder('请输入账号').fill(USERNAME)
  await page.getByPlaceholder('请输入密码').fill(PASSWORD)
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(url => !url.pathname.startsWith('/login'), { timeout: 30_000 })
}

async function waitForTemplateTerminalStatus(page: import('@playwright/test').Page, templateId: string) {
  const deadline = Date.now() + 300_000
  let lastStatus: {
    status: string
    progress: number
    message: string
    template_id: string
  } | null = null

  while (Date.now() < deadline) {
    lastStatus = await fetchTemplateStatus(page, templateId)
    if (['completed', 'failed'].includes(lastStatus.status)) {
      return lastStatus
    }
    await page.waitForTimeout(2000)
  }

  throw new Error(`Template ${templateId} did not finish parsing. Last status: ${JSON.stringify(lastStatus)}`)
}

async function deleteTemplateViaUiSession(page: import('@playwright/test').Page, templateId: string) {
  await page.evaluate(async id => {
    const rawAuth = window.localStorage.getItem('aegis-auth')
    const token = rawAuth ? JSON.parse(rawAuth).token : ''
    await fetch(`/api/v1/templates/${id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` }
    })
  }, templateId)
}

async function fetchTemplateStatus(page: import('@playwright/test').Page, templateId: string) {
  return page.evaluate(async id => {
    const rawAuth = window.localStorage.getItem('aegis-auth')
    const token = rawAuth ? JSON.parse(rawAuth).token : ''
    const response = await fetch(`/api/v1/templates/${id}/status`, {
      headers: { Authorization: `Bearer ${token}` }
    })
    const payload = await response.json()
    return payload.data as {
      status: string
      progress: number
      message: string
      template_id: string
    }
  }, templateId)
}

function createUniqueBaselineUpload() {
  if (!fs.existsSync(BASELINE_PDF)) {
    throw new Error(`Baseline PDF not found: ${BASELINE_PDF}`)
  }
  const target = path.join(os.tmpdir(), `aegis-baseline-${Date.now()}.pdf`)
  const source = fs.readFileSync(BASELINE_PDF)
  const eofMarker = Buffer.from('%%EOF')
  const eofIndex = source.lastIndexOf(eofMarker)
  if (eofIndex === -1) {
    throw new Error(`Baseline PDF is missing %%EOF: ${BASELINE_PDF}`)
  }
  const uniqueComment = Buffer.from(`\n% aegis e2e upload ${Date.now()}\n`)
  fs.writeFileSync(target, Buffer.concat([
    source.subarray(0, eofIndex),
    uniqueComment,
    source.subarray(eofIndex)
  ]))
  return target
}

import { expect, test } from '@playwright/test'

const admin = { id: 'internal:admin', username: 'admin', displayName: '系统管理员', source: 'internal', role: 'admin' }

test('internal account can sign in', async ({ page }) => {
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = null
    if (path.endsWith('/auth/internal/login')) data = { accessToken: 'internal-token', expiresAt: '2099-01-01T00:00:00Z', user: admin }
    else if (path.endsWith('/conversations') || path.endsWith('/models') || path.endsWith('/prompts')) data = []
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 'OK', message: 'success', data }) })
  })
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill('admin')
  await page.getByPlaceholder('密码').fill('admin123!')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/chat$/)
  await expect(page.getByRole('heading', { name: '今天想完成什么？' })).toBeVisible()
})

test('admin can open internal user management', async ({ page }) => {
  await page.addInitScript((user) => {
    sessionStorage.setItem('ai_workbench_access_token', 'admin-token')
    sessionStorage.setItem('ai_workbench_session', JSON.stringify({ user, expiresAt: '2099-01-01T00:00:00Z' }))
  }, admin)
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = null
    if (path.endsWith('/auth/me')) data = { user: admin }
    else if (path.endsWith('/admin/users')) data = [
      { username: 'admin', displayName: '系统管理员', role: 'admin', enabled: true, createdAt: '2026-08-24T00:00:00Z', updatedAt: '2026-08-24T00:00:00Z' },
      { username: 'alice', displayName: 'Alice', role: 'user', enabled: true, createdAt: '2026-08-24T00:00:00Z', updatedAt: '2026-08-24T00:00:00Z' },
    ]
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 'OK', message: 'success', data }) })
  })
  await page.goto('/users')
  await expect(page.getByRole('heading', { level: 2, name: '用户管理' })).toBeVisible()
  await expect(page.getByRole('cell', { name: '系统管理员 admin' })).toBeVisible()
  await expect(page.getByText('Alice', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '添加用户' })).toBeVisible()
})

test('admin can configure a Responses provider with web search', async ({ page }) => {
  await page.addInitScript((user) => {
    sessionStorage.setItem('ai_workbench_access_token', 'admin-token')
    sessionStorage.setItem('ai_workbench_session', JSON.stringify({ user, expiresAt: '2099-01-01T00:00:00Z' }))
  }, admin)
  const provider = {
    id: 'prv_sub2api', name: 'Sub2API', baseUrl: 'http://models.example', defaultModel: 'gpt-5.5',
    protocol: 'responses', webSearchEnabled: true, models: ['gpt-5.5'], enabled: true, available: true,
    lastTestedAt: '2026-08-26T12:00:00Z', lastTestLatencyMs: 3100, lastTestError: '', hasApiKey: true,
    createdAt: '2026-08-26T11:00:00Z', updatedAt: '2026-08-26T12:00:00Z',
  }
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = null
    if (path.endsWith('/auth/me')) data = { user: admin }
    else if (path.endsWith('/providers/prv_sub2api') && route.request().method() === 'PUT') data = { ...provider, ...(route.request().postDataJSON() as object) }
    else if (path.endsWith('/providers')) data = [provider]
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 'OK', message: 'success', data }) })
  })

  await page.goto('/providers')
  await expect(page.getByRole('heading', { level: 2, name: '模型连接' })).toBeVisible()
  await expect(page.getByText('Responses', { exact: true })).toBeVisible()
  await expect(page.getByText('已开启', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '编辑' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByText('Responses', { exact: true })).toBeVisible()
  const updateRequest = page.waitForRequest(request => request.method() === 'PUT' && new URL(request.url()).pathname.endsWith('/providers/prv_sub2api'))
  await dialog.getByRole('button', { name: '保存' }).click()
  expect((await updateRequest).postDataJSON()).toMatchObject({ protocol: 'responses', webSearchEnabled: true })
})

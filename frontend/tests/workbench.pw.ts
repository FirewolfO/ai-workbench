import { expect, test } from '@playwright/test'

const conversation = {
  id: 'cnv_demo', title: '发布方案评审', providerId: 'prv_demo', model: 'company-model', systemPrompt: '你是一名严谨的技术评审人。', pinned: true,
  createdAt: '2026-08-11T10:00:00Z', updatedAt: '2026-08-11T11:00:00Z', messageCount: 2, lastMessage: '建议先补齐回滚和监控方案。',
  messages: [
    { id: 'msg_1', conversationId: 'cnv_demo', role: 'user', content: '帮我评审这份发布方案。', promptTokens: 0, completionTokens: 0, latencyMs: 0, status: 'completed', createdAt: '2026-08-11T10:00:00Z' },
    { id: 'msg_2', conversationId: 'cnv_demo', role: 'assistant', content: '## 评审结论\n\n建议先补齐回滚和监控方案。', model: 'company-model', promptTokens: 42, completionTokens: 18, latencyMs: 860, status: 'completed', createdAt: '2026-08-11T10:00:03Z' },
  ],
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    sessionStorage.setItem('ai_workbench_access_token', 'visual-token')
    sessionStorage.setItem('ai_workbench_session', JSON.stringify({ user: { id: 'alice', username: 'alice', displayName: 'Alice' }, expiresAt: '2099-01-01T00:00:00Z' }))
  })
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = null
    if (path.endsWith('/auth/me')) data = { user: { id: 'alice', username: 'alice', displayName: 'Alice' } }
    else if (path.endsWith('/providers')) data = [{ id: 'prv_demo', name: '企业模型', baseUrl: 'https://model.example.com/v1', defaultModel: 'company-model', enabled: true, hasApiKey: true, createdAt: '2026-08-11T09:00:00Z', updatedAt: '2026-08-11T09:00:00Z' }]
    else if (path.endsWith('/prompts')) data = [{ id: 'pmt_demo', title: '技术评审', description: '评审技术方案', category: '研发', content: '请评审以下技术方案：', favorite: true, useCount: 8, createdAt: '2026-08-11T09:00:00Z', updatedAt: '2026-08-11T09:00:00Z' }]
    else if (path.endsWith('/conversations/cnv_demo')) data = conversation
    else if (path.endsWith('/conversations')) data = [conversation]
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 'OK', message: 'success', data }) })
  })
})

test('conversation workspace is usable without overflow', async ({ page }, testInfo) => {
  await page.goto('/chat/cnv_demo')
  await expect(page.getByRole('heading', { name: '发布方案评审' })).toBeVisible()
  await expect(page.getByPlaceholder('输入消息')).toBeVisible()
  await expect(page.getByText('评审结论')).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)
  expect(overflow).toBe(false)
  await page.screenshot({ path: `../.runtime/screenshots/chat-${testInfo.project.name}.png`, fullPage: true })
})

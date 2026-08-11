// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { renderMarkdown } from './markdown'

describe('renderMarkdown', () => {
  it('renders markdown and strips scripts', () => {
    const html = renderMarkdown('**ok**<script>alert(1)</script>')
    expect(html).toContain('<strong>ok</strong>')
    expect(html).not.toContain('<script>')
  })
})

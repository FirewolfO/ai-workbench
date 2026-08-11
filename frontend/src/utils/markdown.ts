import DOMPurify from 'dompurify'
import { marked } from 'marked'

marked.setOptions({ breaks: true, gfm: true })
export function renderMarkdown(value: string) {
  return DOMPurify.sanitize(marked.parse(value, { async: false }) as string)
}

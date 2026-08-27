// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { attachmentFileIcon, filesFromClipboard, isImagePreview } from './attachments'

describe('attachment helpers', () => {
  it('extracts clipboard files and gives generic images a useful name', () => {
    const image = new File(['image'], 'image.png', { type: 'image/png', lastModified: 10 })
    const text = new File(['notes'], 'notes.txt', { type: 'text/plain' })
    const data = { files: [image, text], items: [] } as unknown as DataTransfer
    const files = filesFromClipboard(data, new Date(2026, 7, 27, 12, 34, 56))

    expect(files).toHaveLength(2)
    expect(files[0].name).toBe('粘贴图片-20260827-123456-1.png')
    expect(files[0].type).toBe('image/png')
    expect(files[1]).toBe(text)
  })

  it('only accepts backend JPEG data URLs as previews', () => {
    expect(isImagePreview('image/jpeg', 'data:image/jpeg;base64,YWJj')).toBe(true)
    expect(isImagePreview('image/png', 'data:image/png;base64,YWJj')).toBe(true)
    expect(isImagePreview('image/png', 'https://untrusted.example/image.png')).toBe(false)
    expect(isImagePreview('text/html', 'data:image/jpeg;base64,YWJj')).toBe(false)
  })

  it.each([
    ['report.pdf', 'application/octet-stream', 'pdf', 'PDF'],
    ['proposal.docx', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'word', 'DOC'],
    ['budget.xlsx', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 'sheet', 'XLS'],
    ['slides.pptx', '', 'slides', 'PPT'],
    ['source.zip', 'application/zip', 'archive', 'ZIP'],
    ['abe_power.xml', 'text/xml', 'code', 'CODE'],
    ['recording.mp3', 'audio/mpeg', 'audio', 'AUD'],
    ['clip.mp4', 'video/mp4', 'video', 'VID'],
    ['notes.txt', 'text/plain', 'text', 'TXT'],
    ['unknown.bin', 'application/octet-stream', 'file', 'FILE'],
  ])('maps %s to its file type icon', (name, contentType, kind, label) => {
    expect(attachmentFileIcon(name, contentType)).toMatchObject({ kind, label })
  })
})

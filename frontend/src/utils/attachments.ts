function timestamp(date: Date) {
  const part = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}${part(date.getMonth() + 1)}${part(date.getDate())}-${part(date.getHours())}${part(date.getMinutes())}${part(date.getSeconds())}`
}

function imageExtension(contentType: string) {
  switch (contentType.toLowerCase()) {
    case 'image/jpeg': return 'jpg'
    case 'image/gif': return 'gif'
    case 'image/webp': return 'webp'
    case 'image/bmp': return 'bmp'
    default: return 'png'
  }
}

function normalizeClipboardFile(file: File, index: number, now: Date) {
  const genericImageName = !file.name || /^image(?:\.[a-z0-9]+)?$/i.test(file.name)
  if (!file.type.startsWith('image/') || !genericImageName) return file
  return new File([file], `粘贴图片-${timestamp(now)}-${index + 1}.${imageExtension(file.type)}`, {
    type: file.type,
    lastModified: file.lastModified,
  })
}

export type AttachmentFileKind = 'pdf' | 'word' | 'sheet' | 'slides' | 'archive' | 'code' | 'audio' | 'video' | 'text' | 'file'

export interface AttachmentFileIcon {
  kind: AttachmentFileKind
  label: string
  title: string
}

const fileIcons: Record<AttachmentFileKind, Omit<AttachmentFileIcon, 'kind'>> = {
  pdf: { label: 'PDF', title: 'PDF 文档' },
  word: { label: 'DOC', title: '文字文档' },
  sheet: { label: 'XLS', title: '电子表格' },
  slides: { label: 'PPT', title: '演示文稿' },
  archive: { label: 'ZIP', title: '压缩文件' },
  code: { label: 'CODE', title: '代码文件' },
  audio: { label: 'AUD', title: '音频文件' },
  video: { label: 'VID', title: '视频文件' },
  text: { label: 'TXT', title: '文本文件' },
  file: { label: 'FILE', title: '文件' },
}

function icon(kind: AttachmentFileKind): AttachmentFileIcon {
  return { kind, ...fileIcons[kind] }
}

export function attachmentFileIcon(name: string, contentType: string): AttachmentFileIcon {
  const type = contentType.toLowerCase().split(';', 1)[0].trim()
  const extension = name.toLowerCase().split('.').pop() || ''
  if (type === 'application/pdf' || extension === 'pdf') return icon('pdf')
  if (/wordprocessingml|msword|opendocument\.text/.test(type) || ['doc', 'docx', 'odt'].includes(extension)) return icon('word')
  if (/spreadsheetml|ms-excel|opendocument\.spreadsheet|text\/csv/.test(type) || ['xls', 'xlsx', 'ods', 'csv'].includes(extension)) return icon('sheet')
  if (/presentationml|ms-powerpoint|opendocument\.presentation/.test(type) || ['ppt', 'pptx', 'odp'].includes(extension)) return icon('slides')
  if (/zip|compressed|archive|tar|rar|7z/.test(type) || ['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz'].includes(extension)) return icon('archive')
  if (type.startsWith('audio/')) return icon('audio')
  if (type.startsWith('video/')) return icon('video')
  if (/json|javascript|xml|yaml|shellscript|sql/.test(type) || ['json', 'xml', 'yaml', 'yml', 'js', 'jsx', 'ts', 'tsx', 'vue', 'go', 'java', 'kt', 'kts', 'py', 'rb', 'php', 'sh', 'bash', 'zsh', 'sql', 'css', 'scss', 'html', 'htm'].includes(extension)) return icon('code')
  if (type.startsWith('text/') || ['txt', 'md', 'rtf', 'log'].includes(extension)) return icon('text')
  return icon('file')
}

export function filesFromClipboard(data: DataTransfer | null, now = new Date()) {
  if (!data) return []
  let files = Array.from(data.files || [])
  if (!files.length) {
    files = Array.from(data.items || []).flatMap((item) => {
      if (item.kind !== 'file') return []
      const file = item.getAsFile()
      return file ? [file] : []
    })
  }
  return files.map((file, index) => normalizeClipboardFile(file, index, now))
}

export function isImagePreview(contentType: string, previewURL: string) {
  return /^image\/(?:jpeg|png|webp|gif)$/i.test(contentType) && /^data:image\/(?:jpeg|png|webp|gif);base64,[a-z0-9+/=]+$/i.test(previewURL)
}

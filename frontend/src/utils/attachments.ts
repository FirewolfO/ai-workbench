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

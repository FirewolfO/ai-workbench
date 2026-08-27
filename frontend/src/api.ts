import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import type { Attachment, AvailableModel, ContentStatus, Conversation, CreatedUser, Dashboard, FileTool, FileToolResult, FileToolRunOptions, FrontierCategory, FrontierResult, InternalUser, Message, NewsResult, NewsSummaryResult, Prompt, PromptInput, Provider, ProviderInput, ReasoningEffort, Session, SyncState, User } from './types'

interface Envelope<T> { code: string; message: string; data: T }
interface RetryConfig extends InternalAxiosRequestConfig { _retry?: boolean }

const accessTokenKey = 'ai_workbench_access_token'
const sessionKey = 'ai_workbench_session'
const deviceIDKey = 'ai_workbench_device_id'
const api = axios.create({ baseURL: import.meta.env.VITE_AI_WORKBENCH_API_BASE_URL || '/api/v1', timeout: 120_000 })

function read(storage: Storage, key: string) {
  try { return storage.getItem(key) || '' } catch { return '' }
}
function write(storage: Storage, key: string, value: string) {
  try { storage.setItem(key, value) } catch { /* Storage can be disabled by browser policy. */ }
}
function remove(storage: Storage, key: string) {
  try { storage.removeItem(key) } catch { /* Storage can be disabled by browser policy. */ }
}
function migrateSessionStorage() {
  if (!read(localStorage, accessTokenKey)) {
    const token = read(sessionStorage, accessTokenKey)
    const session = read(sessionStorage, sessionKey)
    if (token) write(localStorage, accessTokenKey, token)
    if (session) write(localStorage, sessionKey, session)
  }
  remove(sessionStorage, accessTokenKey)
  remove(sessionStorage, sessionKey)
}
function accessToken() { return read(localStorage, accessTokenKey) }
function deviceID() {
  const saved = read(localStorage, deviceIDKey)
  if (saved) return saved
  const value = typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : Array.from(crypto.getRandomValues(new Uint8Array(24)), byte => byte.toString(16).padStart(2, '0')).join('')
  write(localStorage, deviceIDKey, value)
  return value
}

migrateSessionStorage()

api.interceptors.request.use((config) => {
  const token = accessToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  config.headers['X-AI-Workbench-Device-ID'] = deviceID()
  return config
})
api.interceptors.response.use((response) => response, (error: AxiosError<Envelope<unknown>>) => {
  const original = error.config as RetryConfig | undefined
  if (error.response?.status === 401 && accessToken() && original && !original._retry && !original.url?.includes('/auth/')) {
    clearSession()
    window.location.assign(`/login?redirect=${encodeURIComponent(location.pathname + location.search)}`)
  }
  return Promise.reject(error)
})

const unwrap = async <T>(promise: Promise<{ data: Envelope<T> }>) => (await promise).data.data
export const apiMessage = (error: unknown, fallback = '请求失败') => axios.isAxiosError<Envelope<unknown>>(error) ? error.response?.data?.message || fallback : error instanceof Error ? error.message : fallback

export function saveSession(session: Session) {
  write(localStorage, accessTokenKey, session.accessToken)
  write(localStorage, sessionKey, JSON.stringify({ user: session.user, expiresAt: session.expiresAt }))
}
export function loadSession(): { user: User; expiresAt: string } | null {
  try { return JSON.parse(read(localStorage, sessionKey) || 'null') }
  catch { clearSession(); return null }
}
export function clearSession() {
  remove(localStorage, accessTokenKey)
  remove(localStorage, sessionKey)
  remove(sessionStorage, accessTokenKey)
  remove(sessionStorage, sessionKey)
}

export const workbenchApi = {
  oauthURL: (redirectUri: string) => unwrap<{ url: string }>(api.get('/auth/oauth/url', { params: { redirect_uri: redirectUri } })),
  oauthCallback: (code: string, state: string, redirectUri: string) => unwrap<Session>(api.post('/auth/oauth/callback', { code, state, redirectUri })),
  internalLogin: (username: string, password: string) => unwrap<Session>(api.post('/auth/internal/login', { username, password })),
  me: () => unwrap<{ user: User }>(api.get('/auth/me')),
  logout: () => unwrap<{ loggedOut: boolean }>(api.post('/auth/logout')),
  users: () => unwrap<InternalUser[]>(api.get('/admin/users')),
  createUser: (input: { username: string; displayName: string; password?: string }) => unwrap<CreatedUser>(api.post('/admin/users', input)),
  updateUser: (username: string, input: { displayName?: string; password?: string; enabled?: boolean }) => unwrap<InternalUser>(api.patch(`/admin/users/${encodeURIComponent(username)}`, input)),
  deleteUser: (username: string) => unwrap<{ deleted: boolean }>(api.delete(`/admin/users/${encodeURIComponent(username)}`)),
  dashboard: () => unwrap<Dashboard>(api.get('/dashboard')),
  models: (refresh = false) => unwrap<AvailableModel[]>(api.get('/models', { params: refresh ? { refresh: true } : undefined })),
  providers: () => unwrap<Provider[]>(api.get('/providers')),
  createProvider: (input: ProviderInput) => unwrap<Provider>(api.post('/providers', input)),
  updateProvider: (id: string, input: ProviderInput) => unwrap<Provider>(api.put(`/providers/${id}`, input)),
  deleteProvider: (id: string) => unwrap<{ deleted: boolean }>(api.delete(`/providers/${id}`)),
  testProvider: (id: string) => unwrap<{ ok: boolean; latencyMs: number; modelCount: number }>(api.post(`/providers/${id}/test`)),
  prompts: (search = '') => unwrap<Prompt[]>(api.get('/prompts', { params: { search } })),
  createPrompt: (input: PromptInput) => unwrap<Prompt>(api.post('/prompts', input)),
  updatePrompt: (id: string, input: PromptInput) => unwrap<Prompt>(api.put(`/prompts/${id}`, input)),
  deletePrompt: (id: string) => unwrap<{ deleted: boolean }>(api.delete(`/prompts/${id}`)),
  usePrompt: (id: string) => unwrap<Prompt>(api.post(`/prompts/${id}/use`)),
  conversations: (search = '') => unwrap<Conversation[]>(api.get('/conversations', { params: { search } })),
  conversation: (id: string) => unwrap<Conversation>(api.get(`/conversations/${id}`)),
  createConversation: (input: { title?: string; providerId?: string; model?: string; systemPrompt?: string; reasoningEffort?: ReasoningEffort } = {}) => unwrap<Conversation>(api.post('/conversations', input)),
  updateConversation: (id: string, input: Partial<Pick<Conversation, 'title' | 'providerId' | 'model' | 'systemPrompt' | 'pinned' | 'reasoningEffort'>>) => unwrap<Conversation>(api.patch(`/conversations/${id}`, input)),
  deleteConversation: (id: string) => unwrap<{ deleted: boolean }>(api.delete(`/conversations/${id}`)),
  queueMessage: (id: string, content: string, attachmentIds: string[] = []) => unwrap<Message>(api.post(`/conversations/${id}/messages/async`, { content, attachmentIds })),
  stopGeneration: (id: string) => unwrap<{ stopped: boolean }>(api.post(`/conversations/${id}/stop`)),
  fileTools: () => unwrap<FileTool[]>(api.get('/file-tools')),
  runFileTool: (id: string, files: File[], options: FileToolRunOptions, onProgress?: (progress: number) => void) => {
    const data = new FormData()
    for (const file of files) data.append('files', file)
    if (options.pageRange != null) data.append('pageRange', options.pageRange)
    if (options.quality != null) data.append('quality', options.quality)
    if (options.imageFormat != null) data.append('imageFormat', options.imageFormat)
    if (options.maxWidth != null) data.append('maxWidth', String(options.maxWidth))
    const totalSize = files.reduce((total, file) => total + file.size, 0)
    return unwrap<FileToolResult>(api.post(`/file-tools/${encodeURIComponent(id)}`, data, {
      timeout: 5 * 60_000,
      onUploadProgress: (event) => {
        if (!onProgress) return
        const total = event.total || totalSize
        onProgress(total > 0 ? Math.min(100, Math.round((event.loaded / total) * 100)) : 0)
      },
    }))
  },
  uploadAttachment: (file: File, onProgress?: (progress: number) => void, signal?: AbortSignal) => {
    const data = new FormData()
    data.append('file', file)
    return unwrap<Attachment>(api.post('/attachments', data, {
      signal,
      onUploadProgress: (event) => {
        if (!onProgress) return
        const total = event.total || file.size
        onProgress(total > 0 ? Math.min(100, Math.round((event.loaded / total) * 100)) : 0)
      },
    }))
  },
  deleteAttachment: (id: string) => unwrap<{ deleted: boolean }>(api.delete(`/attachments/${id}`)),
  contentStatus: () => unwrap<ContentStatus>(api.get('/content/status')),
  news: (search = '', source = '', favorite = false) => unwrap<NewsResult>(api.get('/news', { params: { search, source, favorite } })),
  refreshNews: () => unwrap<SyncState>(api.post('/news/refresh')),
  summarizeNews: (articleIds: string[]) => unwrap<NewsSummaryResult>(api.post('/news/summaries', { articleIds })),
  favoriteNews: (id: string, favorite: boolean) => unwrap<{ favorite: boolean }>(api.put(`/news/${id}/favorite`, { favorite })),
  frontier: (params: { search?: string; category: FrontierCategory; language?: string; period: string; sort: string }) => unwrap<FrontierResult>(api.get('/frontier', { params })),
}

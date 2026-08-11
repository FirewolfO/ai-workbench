import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import type { Conversation, Dashboard, Message, Prompt, PromptInput, Provider, ProviderInput, Session, User } from './types'

interface Envelope<T> { code: string; message: string; data: T }
interface RetryConfig extends InternalAxiosRequestConfig { _retry?: boolean }

const accessTokenKey = 'ai_workbench_access_token'
const sessionKey = 'ai_workbench_session'
const api = axios.create({ baseURL: import.meta.env.VITE_AI_WORKBENCH_API_BASE_URL || '/api/v1', timeout: 120_000 })

api.interceptors.request.use((config) => {
  const token = sessionStorage.getItem(accessTokenKey)
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})
api.interceptors.response.use((response) => response, (error: AxiosError<Envelope<unknown>>) => {
  const original = error.config as RetryConfig | undefined
  if (error.response?.status === 401 && original && !original._retry && !original.url?.includes('/auth/oauth/')) {
    clearSession()
    window.location.assign(`/login?redirect=${encodeURIComponent(location.pathname + location.search)}`)
  }
  return Promise.reject(error)
})

const unwrap = async <T>(promise: Promise<{ data: Envelope<T> }>) => (await promise).data.data
export const apiMessage = (error: unknown, fallback = '请求失败') => axios.isAxiosError<Envelope<unknown>>(error) ? error.response?.data?.message || fallback : error instanceof Error ? error.message : fallback

export function saveSession(session: Session) {
  sessionStorage.setItem(accessTokenKey, session.accessToken)
  sessionStorage.setItem(sessionKey, JSON.stringify({ user: session.user, expiresAt: session.expiresAt }))
}
export function loadSession(): { user: User; expiresAt: string } | null {
  try { return JSON.parse(sessionStorage.getItem(sessionKey) || 'null') }
  catch { clearSession(); return null }
}
export function clearSession() { sessionStorage.removeItem(accessTokenKey); sessionStorage.removeItem(sessionKey) }

export const workbenchApi = {
  oauthURL: (redirectUri: string) => unwrap<{ url: string }>(api.get('/auth/oauth/url', { params: { redirect_uri: redirectUri } })),
  oauthCallback: (code: string, state: string, redirectUri: string) => unwrap<Session>(api.post('/auth/oauth/callback', { code, state, redirectUri })),
  me: () => unwrap<{ user: User }>(api.get('/auth/me')),
  logout: () => unwrap<{ loggedOut: boolean }>(api.post('/auth/logout')),
  dashboard: () => unwrap<Dashboard>(api.get('/dashboard')),
  providers: () => unwrap<Provider[]>(api.get('/providers')),
  createProvider: (input: ProviderInput) => unwrap<Provider>(api.post('/providers', input)),
  updateProvider: (id: string, input: ProviderInput) => unwrap<Provider>(api.put(`/providers/${id}`, input)),
  deleteProvider: (id: string) => unwrap<{ deleted: boolean }>(api.delete(`/providers/${id}`)),
  testProvider: (id: string) => unwrap<{ ok: boolean; latencyMs: number }>(api.post(`/providers/${id}/test`)),
  prompts: (search = '') => unwrap<Prompt[]>(api.get('/prompts', { params: { search } })),
  createPrompt: (input: PromptInput) => unwrap<Prompt>(api.post('/prompts', input)),
  updatePrompt: (id: string, input: PromptInput) => unwrap<Prompt>(api.put(`/prompts/${id}`, input)),
  deletePrompt: (id: string) => unwrap<{ deleted: boolean }>(api.delete(`/prompts/${id}`)),
  usePrompt: (id: string) => unwrap<Prompt>(api.post(`/prompts/${id}/use`)),
  conversations: (search = '') => unwrap<Conversation[]>(api.get('/conversations', { params: { search } })),
  conversation: (id: string) => unwrap<Conversation>(api.get(`/conversations/${id}`)),
  createConversation: (input: { title?: string; providerId: string; model?: string; systemPrompt?: string }) => unwrap<Conversation>(api.post('/conversations', input)),
  updateConversation: (id: string, input: Partial<Pick<Conversation, 'title' | 'providerId' | 'model' | 'systemPrompt' | 'pinned'>>) => unwrap<Conversation>(api.patch(`/conversations/${id}`, input)),
  deleteConversation: (id: string) => unwrap<{ deleted: boolean }>(api.delete(`/conversations/${id}`)),
  sendMessage: (id: string, content: string) => unwrap<Message>(api.post(`/conversations/${id}/messages`, { content })),
}

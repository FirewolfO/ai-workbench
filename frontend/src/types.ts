export interface User { id: string; username: string; displayName: string; source: 'internal' | 'people' | 'permission'; role: 'admin' | 'user' }
export interface Session { accessToken: string; expiresAt: string; user: User }

export interface InternalUser {
  username: string
  displayName: string
  role: 'admin' | 'user'
  enabled: boolean
  createdAt: string
  updatedAt: string
}
export interface CreatedUser { user: InternalUser; initialPassword: string }

export interface Provider {
  id: string
  name: string
  baseUrl: string
  defaultModel: string
  enabled: boolean
  available: boolean
  lastTestedAt?: string
  lastTestLatencyMs: number
  lastTestError: string
  hasApiKey: boolean
  createdAt: string
  updatedAt: string
}

export interface Prompt {
  id: string
  title: string
  description: string
  category: string
  content: string
  favorite: boolean
  useCount: number
  createdAt: string
  updatedAt: string
}

export interface Message {
  id: string
  conversationId: string
  role: 'user' | 'assistant' | 'system'
  content: string
  model?: string
  promptTokens: number
  completionTokens: number
  latencyMs: number
  status: 'completed' | 'failed' | 'stopped'
  attachments?: string[]
  createdAt: string
}

export interface Conversation {
  id: string
  title: string
  providerId: string
  model: string
  systemPrompt: string
  reasoningEffort: ReasoningEffort
  pinned: boolean
  createdAt: string
  updatedAt: string
  messageCount: number
  lastMessage?: string
  messages?: Message[]
}

export interface Dashboard {
  conversationCount: number
  messageCount: number
  promptCount: number
  providerCount: number
  totalTokens: number
  recent: Conversation[]
}

export interface ProviderInput { name: string; baseUrl: string; defaultModel: string; apiKey: string; enabled?: boolean }
export interface PromptInput { title: string; description: string; category: string; content: string; favorite?: boolean }
export type ReasoningEffort = 'fast' | 'medium' | 'high'
export interface AvailableModel { id: string; name: string; defaultModel: string }
export interface Attachment { id: string; name: string; contentType: string; size: number; expiresAt: string }

export interface NewsArticle {
  id: string
  sourceCode: string
  sourceName: string
  title: string
  summary: string
  chineseSummary: string
  url: string
  author: string
  publishedAt: string
  favorite: boolean
}
export interface ContentSource { code: string; name: string }
export interface NewsResult { items: NewsArticle[]; sources: ContentSource[]; lastSuccessAt?: string; lastError: string }
export interface NewsSummaryResult { generated: number; summaries: Record<string, string> }
export interface TrackedPerson { id: string; platform: 'x'; handle: string; displayName: string; profileImageUrl: string; enabled: boolean; lastFetchedAt?: string; lastError: string }
export interface PeopleResult { people: TrackedPerson[]; xConfigured: boolean; lastSuccessAt?: string; lastError: string }
export interface SocialPost { id: string; personId: string; handle: string; displayName: string; content: string; url: string; publishedAt: string; likeCount: number; repostCount: number; replyCount: number; favorite: boolean }
export interface SyncState { key: string; lastAttemptAt?: string; lastSuccessAt?: string; lastError: string; itemsFetched: number }
export interface ContentStatus { xConfigured: boolean; refreshHours: number; newsLastSuccessAt?: string; newsLastError: string; peopleLastSuccessAt?: string; peopleLastError: string }

export type FrontierCategory = 'project' | 'skill' | 'plugin'
export interface FrontierRepository {
  id: number
  name: string
  fullName: string
  description: string
  url: string
  homepage: string
  owner: string
  ownerAvatar: string
  category: FrontierCategory
  language: string
  license: string
  topics: string[]
  stars: number
  forks: number
  openIssues: number
  score: number
  signals: string[]
  createdAt: string
  updatedAt: string
  pushedAt: string
}
export interface FrontierResult {
  items: FrontierRepository[]
  total: number
  generatedAt: string
  githubTokenSet: boolean
  rateLimit: { limit: number; remaining: number; resetAt?: string }
  stale: boolean
  lastSuccessAt?: string
}

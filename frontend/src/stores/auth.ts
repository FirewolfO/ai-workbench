import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { clearSession, loadSession, saveSession, workbenchApi } from '@/api'
import type { User } from '@/types'

export const useAuthStore = defineStore('auth', () => {
  const cached = loadSession()
  const user = ref<User | null>(cached?.user || null)
  const initialized = ref(false)
  const authenticated = computed(() => Boolean(user.value))

  async function hydrate() {
    if (initialized.value) return
    if (!loadSession()) { initialized.value = true; return }
    try { user.value = (await workbenchApi.me()).user }
    catch { clearSession(); user.value = null }
    finally { initialized.value = true }
  }
  async function completeOAuth(code: string, state: string, redirectUri: string) {
    const session = await workbenchApi.oauthCallback(code, state, redirectUri)
    saveSession(session)
    user.value = session.user
    initialized.value = true
  }
  async function logout() {
    try { await workbenchApi.logout() } finally { clearSession(); user.value = null }
  }
  return { user, initialized, authenticated, hydrate, completeOAuth, logout }
})

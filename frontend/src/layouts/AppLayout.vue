<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChatLineRound, Close, Compass, Connection, DataAnalysis, Download, MagicStick, Menu as MenuIcon, Refresh, SwitchButton, TrendCharts, User } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

declare global {
  interface Window { AIWorkbenchNative?: { checkForUpdate: () => void; getUpdateStatus?: () => void } }
}

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const drawerOpen = ref(false)
const isNativeApp = ref(false)
const availableUpdate = ref<{ version: string; size: number } | null>(null)
const updateStatus = ref<'idle' | 'checking' | 'current' | 'available' | 'error' | 'downloading'>('idle')
const currentAppVersion = ref('')
let updateCheckTimer: number | undefined
const pageTitle = computed(() => String(route.meta.title || 'AI 工作台'))
const items = computed(() => [
  { path: '/chat', label: '对话', icon: ChatLineRound },
  { path: '/news', label: 'AI 热点', icon: TrendCharts },
  { path: '/frontier', label: '前沿项目', icon: Compass },
  { path: '/prompts', label: '提示词', icon: MagicStick },
  ...(auth.isAdmin ? [{ path: '/providers', label: '模型连接', icon: Connection }, { path: '/users', label: '用户管理', icon: User }] : []),
  { path: '/dashboard', label: '用量概览', icon: DataAnalysis },
])
const updateStatusText = computed(() => {
  if (updateStatus.value === 'checking') return '正在检查...'
  if (updateStatus.value === 'available' && availableUpdate.value) return `发现 v${availableUpdate.value.version}`
  if (updateStatus.value === 'current') return currentAppVersion.value ? `已是最新版 · v${currentAppVersion.value}` : '已是最新版'
  if (updateStatus.value === 'downloading') return '正在下载安装包'
  if (updateStatus.value === 'error') return '检查失败，点击重试'
  return currentAppVersion.value ? `当前 v${currentAppVersion.value}` : '点击检查最新版本'
})

async function logout() { await auth.logout(); await router.replace('/chat') }
function receiveAppUpdate(event: Event) {
  const detail = (event as CustomEvent<{ version?: string; size?: number }>).detail
  if (detail?.version) {
    isNativeApp.value = true
    updateStatus.value = 'available'
    availableUpdate.value = { version: detail.version, size: Number(detail.size) || 0 }
  }
}
function receiveUpdateStatus(event: Event) {
  const detail = (event as CustomEvent<{ status?: string; currentVersion?: string; latestVersion?: string; size?: number }>).detail
  if (!detail?.status) return
  isNativeApp.value = true
  if (detail.currentVersion) currentAppVersion.value = detail.currentVersion
  if (['idle', 'checking', 'current', 'available', 'error', 'downloading'].includes(detail.status)) updateStatus.value = detail.status as typeof updateStatus.value
  if (detail.status === 'available' && detail.latestVersion) availableUpdate.value = { version: detail.latestVersion, size: Number(detail.size) || 0 }
  if (detail.status === 'current') availableUpdate.value = null
  if (updateStatus.value !== 'checking' && updateCheckTimer) window.clearTimeout(updateCheckTimer)
}
function checkForAppUpdate() {
  if (availableUpdate.value) {
    window.location.href = `ai-workbench://update?version=${encodeURIComponent(availableUpdate.value.version)}`
    return
  }
  updateStatus.value = 'checking'
  window.AIWorkbenchNative?.checkForUpdate()
  if (updateCheckTimer) window.clearTimeout(updateCheckTimer)
  updateCheckTimer = window.setTimeout(() => {
    if (updateStatus.value === 'checking') updateStatus.value = 'error'
  }, 12_000)
}
onMounted(() => {
  isNativeApp.value = Boolean(window.AIWorkbenchNative) || navigator.userAgent.includes('AIWorkbenchAndroid/')
  window.addEventListener('ai-workbench-app-update', receiveAppUpdate)
  window.addEventListener('ai-workbench-app-update-status', receiveUpdateStatus)
  window.AIWorkbenchNative?.getUpdateStatus?.()
})
onBeforeUnmount(() => {
  if (updateCheckTimer) window.clearTimeout(updateCheckTimer)
  window.removeEventListener('ai-workbench-app-update', receiveAppUpdate)
  window.removeEventListener('ai-workbench-app-update-status', receiveUpdateStatus)
})
</script>

<template>
  <div class="app-shell">
    <aside class="app-sidebar desktop-sidebar">
      <router-link to="/chat" class="brand"><span class="brand-symbol">AI</span><span><strong>我的工作台</strong><small>AI WORKBENCH</small></span></router-link>
      <el-menu :default-active="route.path.startsWith('/chat') ? '/chat' : route.path" router class="nav-menu">
        <el-menu-item v-for="item in items" :key="item.path" :index="item.path"><el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span></el-menu-item>
      </el-menu>
      <button v-if="isNativeApp" class="sidebar-update" :class="{ 'has-update': availableUpdate, checking: updateStatus === 'checking' }" type="button" @click="checkForAppUpdate"><el-icon><Download v-if="availableUpdate" /><Refresh v-else /></el-icon><span><strong>检查更新</strong><small>{{ updateStatusText }}</small></span></button>
      <div class="sidebar-status"><span></span>{{ !auth.user ? '访客模式' : auth.user.source === 'internal' ? '内部账号' : 'People SSO' }}</div>
    </aside>
    <el-drawer v-model="drawerOpen" direction="ltr" :with-header="false" size="min(220px, 74vw)">
      <aside class="app-sidebar drawer-sidebar">
        <div class="drawer-mobile-header"><strong>功能</strong><el-button text :icon="Close" aria-label="关闭导航" @click="drawerOpen = false" /></div>
        <div class="brand"><span class="brand-symbol">AI</span><span><strong>我的工作台</strong><small>AI WORKBENCH</small></span></div>
        <el-menu :default-active="route.path.startsWith('/chat') ? '/chat' : route.path" router class="nav-menu" @select="drawerOpen = false">
          <el-menu-item v-for="item in items" :key="item.path" :index="item.path"><el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span></el-menu-item>
        </el-menu>
        <button v-if="isNativeApp" class="sidebar-update" :class="{ 'has-update': availableUpdate, checking: updateStatus === 'checking' }" type="button" @click="checkForAppUpdate"><el-icon><Download v-if="availableUpdate" /><Refresh v-else /></el-icon><span><strong>检查更新</strong><small>{{ updateStatusText }}</small></span></button>
        <div class="sidebar-status"><span></span>{{ !auth.user ? '访客模式' : auth.user.source === 'internal' ? '内部账号' : 'People SSO' }}</div>
      </aside>
    </el-drawer>
    <main class="app-main">
      <header class="topbar">
        <el-button class="mobile-menu" text :icon="MenuIcon" aria-label="打开功能导航" @click="drawerOpen = true" />
        <h1>{{ pageTitle }}</h1>
        <a v-if="availableUpdate" class="app-update-link" :href="`ai-workbench://update?version=${encodeURIComponent(availableUpdate.version)}`" aria-label="下载并更新新版本"><el-icon><Download /></el-icon><span>新版本</span><b>v{{ availableUpdate.version }}</b></a>
        <el-dropdown v-if="auth.authenticated" trigger="click">
          <button class="identity-button" type="button"><span>{{ auth.user?.displayName?.slice(0, 1) || 'U' }}</span><strong>{{ auth.user?.displayName || auth.user?.username }}</strong></button>
          <template #dropdown><el-dropdown-menu><el-dropdown-item disabled>{{ auth.user?.username }}</el-dropdown-item><el-dropdown-item divided :icon="SwitchButton" @click="logout">退出登录</el-dropdown-item></el-dropdown-menu></template>
        </el-dropdown>
        <el-button v-else text @click="router.push('/login')">登录</el-button>
      </header>
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChatLineRound, Close, Compass, Connection, DataAnalysis, Expand, MagicStick, Menu as MenuIcon, SwitchButton, TrendCharts, User, UserFilled } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const drawerOpen = ref(false)
const pageTitle = computed(() => String(route.meta.title || 'AI 工作台'))
const items = computed(() => [
  { path: '/chat', label: '对话', icon: ChatLineRound },
  { path: '/news', label: 'AI 热点', icon: TrendCharts },
  { path: '/frontier', label: '前沿项目', icon: Compass },
  { path: '/people', label: '大佬动态', icon: UserFilled },
  { path: '/prompts', label: '提示词', icon: MagicStick },
  ...(auth.isAdmin ? [{ path: '/providers', label: '模型连接', icon: Connection }, { path: '/users', label: '用户管理', icon: User }] : []),
  { path: '/dashboard', label: '用量概览', icon: DataAnalysis },
])

async function logout() { await auth.logout(); await router.replace('/login') }
</script>

<template>
  <div class="app-shell">
    <aside class="app-sidebar desktop-sidebar">
      <router-link to="/chat" class="brand"><span class="brand-symbol">AI</span><span><strong>我的工作台</strong><small>AI WORKBENCH</small></span></router-link>
      <el-menu :default-active="route.path.startsWith('/chat') ? '/chat' : route.path" router class="nav-menu">
        <el-menu-item v-for="item in items" :key="item.path" :index="item.path"><el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span></el-menu-item>
      </el-menu>
      <div class="sidebar-status"><span></span>{{ auth.user?.source === 'internal' ? '内部账号' : 'People SSO' }}</div>
    </aside>
    <el-drawer v-model="drawerOpen" direction="ltr" :with-header="false" size="min(220px, 74vw)">
      <aside class="app-sidebar drawer-sidebar">
        <div class="drawer-mobile-header"><strong>功能</strong><el-button text :icon="Close" aria-label="关闭导航" @click="drawerOpen = false" /></div>
        <div class="brand"><span class="brand-symbol">AI</span><span><strong>我的工作台</strong><small>AI WORKBENCH</small></span></div>
        <el-menu :default-active="route.path.startsWith('/chat') ? '/chat' : route.path" router class="nav-menu" @select="drawerOpen = false">
          <el-menu-item v-for="item in items" :key="item.path" :index="item.path"><el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span></el-menu-item>
        </el-menu>
      </aside>
    </el-drawer>
    <main class="app-main">
      <header class="topbar">
        <el-button class="mobile-menu" text :icon="Expand" aria-label="打开导航" @click="drawerOpen = true" />
        <h1>{{ pageTitle }}</h1>
        <el-dropdown trigger="click">
          <button class="identity-button" type="button"><span>{{ auth.user?.displayName?.slice(0, 1) || 'U' }}</span><strong>{{ auth.user?.displayName || auth.user?.username }}</strong><el-icon><MenuIcon /></el-icon></button>
          <template #dropdown><el-dropdown-menu><el-dropdown-item disabled>{{ auth.user?.username }}</el-dropdown-item><el-dropdown-item divided :icon="SwitchButton" @click="logout">退出登录</el-dropdown-item></el-dropdown-menu></template>
        </el-dropdown>
      </header>
      <router-view />
    </main>
  </div>
</template>

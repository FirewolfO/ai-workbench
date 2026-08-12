import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta { title?: string; public?: boolean }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { title: '登录', public: true } },
    { path: '/oauth/callback', name: 'oauth-callback', component: () => import('@/views/OAuthCallbackView.vue'), meta: { title: '登录中', public: true } },
    {
      path: '/', component: () => import('@/layouts/AppLayout.vue'),
      children: [
        { path: '', redirect: '/chat' },
        { path: 'dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: '概览' } },
        { path: 'chat/:id?', name: 'chat', component: () => import('@/views/ChatView.vue'), meta: { title: '对话' } },
        { path: 'prompts', name: 'prompts', component: () => import('@/views/PromptsView.vue'), meta: { title: '提示词' } },
        { path: 'providers', name: 'providers', component: () => import('@/views/ProvidersView.vue'), meta: { title: '模型连接' } },
        { path: 'news', name: 'news', component: () => import('@/views/NewsView.vue'), meta: { title: 'AI 热点' } },
        { path: 'people', name: 'people', component: () => import('@/views/PeopleView.vue'), meta: { title: '大佬动态' } },
        { path: 'frontier', name: 'frontier', component: () => import('@/views/FrontierView.vue'), meta: { title: '前沿项目' } },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  document.title = to.meta.title ? `${to.meta.title} - AI 工作台` : 'AI 工作台'
  const auth = useAuthStore()
  await auth.hydrate()
  if (to.meta.public) return auth.authenticated && to.name === 'login' ? '/chat' : true
  if (!auth.authenticated) return { name: 'login', query: { redirect: to.fullPath } }
  return true
})

export default router

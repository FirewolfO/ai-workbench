<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiMessage } from '@/api'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const errorMessage = ref('')

onMounted(async () => {
  const code = typeof route.query.code === 'string' ? route.query.code : ''
  const state = typeof route.query.state === 'string' ? route.query.state : ''
  if (!code || !state) { errorMessage.value = 'OAuth 回调参数不完整'; return }
  try {
    await auth.completeOAuth(code, state, `${location.origin}/oauth/callback`)
    const target = sessionStorage.getItem('ai_workbench_login_redirect') || '/chat'
    sessionStorage.removeItem('ai_workbench_login_redirect')
    await router.replace(target.startsWith('/') ? target : '/chat')
  } catch (error) { errorMessage.value = apiMessage(error, 'People 登录失败') }
})
</script>

<template><main class="callback-page"><el-result v-if="errorMessage" icon="error" title="登录失败" :sub-title="errorMessage"><template #extra><el-button type="primary" @click="router.replace('/login')">返回登录</el-button></template></el-result><div v-else class="callback-loading"><span class="brand-symbol">AI</span><el-icon class="is-loading"><Loading /></el-icon><p>正在建立安全会话</p></div></main></template>

<script lang="ts">
import { Loading } from '@element-plus/icons-vue'
export default { components: { Loading } }
</script>

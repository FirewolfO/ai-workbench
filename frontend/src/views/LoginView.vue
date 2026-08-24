<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowRight, Lock, User } from '@element-plus/icons-vue'
import { apiMessage, workbenchApi } from '@/api'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)
const mode = ref<'internal' | 'people'>('internal')
const form = reactive({ username: '', password: '' })
const destination = () => typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/chat'

async function loginPeople() {
  loading.value = true
  try {
    sessionStorage.setItem('ai_workbench_login_redirect', destination())
    const redirectUri = `${location.origin}/oauth/callback`
    location.assign((await workbenchApi.oauthURL(redirectUri)).url)
  } catch (error) { ElMessage.error(apiMessage(error, '无法发起 People 登录')); loading.value = false }
}
async function loginInternal() {
  if (!form.username.trim() || !form.password) { ElMessage.warning('请输入用户名和密码'); return }
  loading.value = true
  try { await auth.loginInternal(form.username, form.password); await router.replace(destination()) }
  catch (error) { ElMessage.error(apiMessage(error, '登录失败')) }
  finally { loading.value = false }
}
</script>

<template>
  <main class="login-page">
    <section class="login-panel">
      <div class="login-brand"><span>AI</span><strong>AI WORKBENCH</strong></div>
      <div class="login-copy"><p>企业 AI 工作空间</p><h1>我的 AI 工作台</h1><span>集中管理模型、对话与个人提示词。</span></div>
      <el-segmented v-model="mode" :options="[{ label: '内部账号', value: 'internal' }, { label: 'People', value: 'people' }]" class="login-mode" />
      <el-form v-if="mode === 'internal'" class="login-form" @submit.prevent="loginInternal">
        <el-form-item><el-input v-model="form.username" :prefix-icon="User" size="large" placeholder="用户名" autocomplete="username" /></el-form-item>
        <el-form-item><el-input v-model="form.password" :prefix-icon="Lock" size="large" type="password" show-password placeholder="密码" autocomplete="current-password" @keyup.enter="loginInternal" /></el-form-item>
        <el-button type="primary" size="large" native-type="submit" :loading="loading"><el-icon><Lock /></el-icon>登录<el-icon><ArrowRight /></el-icon></el-button>
      </el-form>
      <el-button v-else type="primary" size="large" :loading="loading" @click="loginPeople"><el-icon><Lock /></el-icon>使用 People 登录<el-icon><ArrowRight /></el-icon></el-button>
      <small>{{ mode === 'internal' ? 'AI Workbench 内部身份' : 'People 企业身份服务' }}</small>
    </section>
    <section class="login-preview" aria-hidden="true">
      <div class="preview-rail"><span></span><span></span><span></span></div>
      <div class="preview-thread"><i></i><i></i><i></i><b></b><i></i></div>
    </section>
  </main>
</template>

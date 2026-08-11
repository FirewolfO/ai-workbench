<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowRight, Lock } from '@element-plus/icons-vue'
import { apiMessage, workbenchApi } from '@/api'

const route = useRoute()
const loading = ref(false)
async function login() {
  loading.value = true
  try {
    sessionStorage.setItem('ai_workbench_login_redirect', typeof route.query.redirect === 'string' ? route.query.redirect : '/chat')
    const redirectUri = `${location.origin}/oauth/callback`
    location.assign((await workbenchApi.oauthURL(redirectUri)).url)
  } catch (error) { ElMessage.error(apiMessage(error, '无法发起 People 登录')); loading.value = false }
}
</script>

<template>
  <main class="login-page">
    <section class="login-panel">
      <div class="login-brand"><span>AI</span><strong>AI WORKBENCH</strong></div>
      <div class="login-copy"><p>企业 AI 工作空间</p><h1>我的 AI 工作台</h1><span>集中管理模型、对话与个人提示词。</span></div>
      <el-button type="primary" size="large" :loading="loading" @click="login"><el-icon><Lock /></el-icon>使用 People 登录<el-icon><ArrowRight /></el-icon></el-button>
      <small>由 People 企业身份服务保护</small>
    </section>
    <section class="login-preview" aria-hidden="true">
      <div class="preview-rail"><span></span><span></span><span></span></div>
      <div class="preview-thread"><i></i><i></i><i></i><b></b><i></i></div>
    </section>
  </main>
</template>

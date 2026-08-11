<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ChatLineRound, Connection, MagicStick, Tickets } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { Dashboard } from '@/types'

const router = useRouter()
const loading = ref(true)
const dashboard = ref<Dashboard | null>(null)
const metrics = [
  { key: 'conversationCount' as const, label: '对话', icon: ChatLineRound },
  { key: 'messageCount' as const, label: '消息', icon: Tickets },
  { key: 'promptCount' as const, label: '提示词', icon: MagicStick },
  { key: 'providerCount' as const, label: '模型连接', icon: Connection },
]
onMounted(async () => {
  try { dashboard.value = await workbenchApi.dashboard() }
  catch (error) { ElMessage.error(apiMessage(error, '概览加载失败')) }
  finally { loading.value = false }
})
const relative = (value: string) => new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' }).format(-Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 86400000)), 'day')
</script>

<template>
  <section v-loading="loading" class="page dashboard-page">
    <div class="page-heading"><div><p>PERSONAL OVERVIEW</p><h2>用量概览</h2></div><el-button type="primary" :icon="ChatLineRound" @click="router.push('/chat')">开始对话</el-button></div>
    <div v-if="dashboard" class="metric-grid">
      <div v-for="metric in metrics" :key="metric.key" class="metric-item"><el-icon><component :is="metric.icon" /></el-icon><span>{{ metric.label }}</span><strong>{{ dashboard[metric.key].toLocaleString() }}</strong></div>
      <div class="metric-item token-metric"><el-icon><Tickets /></el-icon><span>累计 Token</span><strong>{{ dashboard.totalTokens.toLocaleString() }}</strong></div>
    </div>
    <div class="section-heading"><div><h3>最近对话</h3><p>继续上次的工作</p></div><el-button text @click="router.push('/chat')">查看全部</el-button></div>
    <div v-if="dashboard?.recent.length" class="recent-list">
      <button v-for="item in dashboard.recent" :key="item.id" type="button" @click="router.push(`/chat/${item.id}`)"><span><strong>{{ item.title }}</strong><small>{{ item.lastMessage || '还没有消息' }}</small></span><span><b>{{ item.messageCount }} 条</b><small>{{ relative(item.updatedAt) }}</small></span></button>
    </div>
    <el-empty v-else description="还没有对话" />
  </section>
</template>

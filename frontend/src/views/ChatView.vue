<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChatLineRound, CopyDocument, Delete, Edit, MagicStick, MoreFilled, Plus, Promotion, Search, Setting, Star, StarFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import { renderMarkdown } from '@/utils/markdown'
import type { Conversation, Message, Prompt, Provider } from '@/types'

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const sending = ref(false)
const search = ref('')
const draft = ref('')
const conversations = ref<Conversation[]>([])
const current = ref<Conversation | null>(null)
const providers = ref<Provider[]>([])
const prompts = ref<Prompt[]>([])
const createOpen = ref(false)
const promptOpen = ref(false)
const settingsOpen = ref(false)
const thread = ref<HTMLElement | null>(null)
const createForm = reactive({ providerId: '', model: '', systemPrompt: '' })
const settings = reactive({ providerId: '', model: '', systemPrompt: '' })
const enabledProviders = computed(() => providers.value.filter((item) => item.enabled))
const activeProvider = computed(() => providers.value.find((item) => item.id === current.value?.providerId))

async function loadLists() {
  const [conversationList, providerList, promptList] = await Promise.all([workbenchApi.conversations(search.value), workbenchApi.providers(), workbenchApi.prompts()])
  conversations.value = conversationList
  providers.value = providerList
  prompts.value = promptList
}
async function loadCurrent(id: string) {
  loading.value = true
  try { current.value = await workbenchApi.conversation(id); await scrollToEnd() }
  catch (error) { ElMessage.error(apiMessage(error, '对话加载失败')); await router.replace('/chat') }
  finally { loading.value = false }
}
async function initialize() {
  loading.value = true
  try {
    await loadLists()
    const id = typeof route.params.id === 'string' ? route.params.id : ''
    if (id) await loadCurrent(id)
    if (typeof route.query.prompt === 'string') { draft.value = route.query.prompt; await router.replace({ path: route.path }) }
  } catch (error) { ElMessage.error(apiMessage(error, '工作台加载失败')) }
  finally { loading.value = false }
}
function openCreate() {
  if (!enabledProviders.value.length) { ElMessage.warning('请先添加并启用一个模型连接'); void router.push('/providers'); return }
  const provider = enabledProviders.value[0]
  Object.assign(createForm, { providerId: provider?.id || '', model: provider?.defaultModel || '', systemPrompt: '' })
  createOpen.value = true
}
function selectCreateProvider(value: string) { const provider = providers.value.find((item) => item.id === value); if (provider) createForm.model = provider.defaultModel }
async function createConversation() {
  try { const conversation = await workbenchApi.createConversation({ ...createForm }); createOpen.value = false; conversations.value.unshift(conversation); await router.push(`/chat/${conversation.id}`) }
  catch (error) { ElMessage.error(apiMessage(error, '创建对话失败')) }
}
async function send() {
  const content = draft.value.trim()
  if (!content || !current.value || sending.value) return
  const conversationID = current.value.id
  const optimistic: Message = { id: `local-${Date.now()}`, conversationId: conversationID, role: 'user', content, promptTokens: 0, completionTokens: 0, latencyMs: 0, status: 'completed', createdAt: new Date().toISOString() }
  current.value.messages ||= []
  current.value.messages.push(optimistic)
  draft.value = ''
  sending.value = true
  await scrollToEnd()
  try {
    const answer = await workbenchApi.sendMessage(conversationID, content)
    current.value.messages.push(answer)
    await loadLists()
  } catch (error) { ElMessage.error(apiMessage(error, '模型响应失败')); current.value = await workbenchApi.conversation(conversationID) }
  finally { sending.value = false; await scrollToEnd() }
}
async function scrollToEnd() { await nextTick(); if (thread.value) thread.value.scrollTop = thread.value.scrollHeight }
async function selectPrompt(item: Prompt) { await workbenchApi.usePrompt(item.id); draft.value = item.content; promptOpen.value = false; await nextTick() }
async function togglePin() { if (!current.value) return; current.value = await workbenchApi.updateConversation(current.value.id, { pinned: !current.value.pinned }); await loadLists() }
async function rename() {
  if (!current.value) return
  try { const result = await ElMessageBox.prompt('输入新的对话名称', '重命名对话', { inputValue: current.value.title, inputValidator: (value) => Boolean(value.trim()) || '名称不能为空' }); current.value = await workbenchApi.updateConversation(current.value.id, { title: result.value }); await loadLists() }
  catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error)) }
}
async function remove() {
  if (!current.value) return
  try { await ElMessageBox.confirm(`确定删除“${current.value.title}”及全部消息吗？`, '删除对话', { type: 'warning' }); await workbenchApi.deleteConversation(current.value.id); current.value = null; await loadLists(); await router.replace('/chat') }
  catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error, '删除失败')) }
}
function openSettings() { if (!current.value) return; Object.assign(settings, { providerId: current.value.providerId, model: current.value.model, systemPrompt: current.value.systemPrompt }); settingsOpen.value = true }
function selectSettingsProvider(value: string) { const provider = providers.value.find((item) => item.id === value); if (provider) settings.model = provider.defaultModel }
async function saveSettings() { if (!current.value) return; try { current.value = await workbenchApi.updateConversation(current.value.id, { ...settings }); settingsOpen.value = false; ElMessage.success('对话设置已更新') } catch (error) { ElMessage.error(apiMessage(error, '保存失败')) } }
async function copy(content: string) { try { await navigator.clipboard.writeText(content); ElMessage.success('已复制') } catch { ElMessage.error('复制失败') } }

watch(() => route.params.id, (id) => { if (typeof id === 'string') void loadCurrent(id); else current.value = null })
onMounted(initialize)
</script>

<template>
  <section class="chat-workspace">
    <aside class="conversation-panel">
      <el-button type="primary" :icon="Plus" @click="openCreate">新对话</el-button>
      <el-input v-model="search" :prefix-icon="Search" clearable placeholder="搜索对话" @keyup.enter="loadLists" @clear="loadLists" />
      <div class="conversation-list">
        <button v-for="item in conversations" :key="item.id" type="button" :class="{ active: item.id === current?.id }" @click="router.push(`/chat/${item.id}`)">
          <el-icon><StarFilled v-if="item.pinned" /><ChatLineRound v-else /></el-icon><span><strong>{{ item.title }}</strong><small>{{ item.lastMessage || '还没有消息' }}</small></span><b>{{ item.messageCount }}</b>
        </button>
      </div>
    </aside>
    <div v-loading="loading" class="chat-main">
      <template v-if="current">
        <header class="chat-header"><div><h2>{{ current.title }}</h2><span>{{ activeProvider?.name || '模型连接' }} · {{ current.model }}</span></div><div><el-button text :icon="current.pinned ? StarFilled : Star" aria-label="置顶" @click="togglePin" /><el-dropdown trigger="click"><el-button text :icon="MoreFilled" aria-label="更多操作" /><template #dropdown><el-dropdown-menu><el-dropdown-item :icon="Edit" @click="rename">重命名</el-dropdown-item><el-dropdown-item :icon="Setting" @click="openSettings">对话设置</el-dropdown-item><el-dropdown-item divided :icon="Delete" @click="remove">删除对话</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div></header>
        <div ref="thread" class="message-thread">
          <div v-if="!current.messages?.length" class="chat-empty"><span class="brand-symbol">AI</span><h3>从一个问题开始</h3><p>{{ current.systemPrompt || '选择提示词，或直接输入你想处理的事情。' }}</p></div>
          <article v-for="message in current.messages" :key="message.id" class="message-row" :class="message.role">
            <span class="message-avatar">{{ message.role === 'user' ? '我' : 'AI' }}</span>
            <div class="message-body"><header><strong>{{ message.role === 'user' ? '我' : message.model || '助手' }}</strong><el-button v-if="message.role === 'assistant' && message.status === 'completed'" text :icon="CopyDocument" aria-label="复制回答" @click="copy(message.content)" /></header><div v-if="message.role === 'assistant'" class="markdown-body" :class="{ failed: message.status === 'failed' }" v-html="renderMarkdown(message.content)"></div><p v-else>{{ message.content }}</p><small v-if="message.role === 'assistant' && message.status === 'completed'">{{ message.latencyMs }} ms · {{ message.promptTokens + message.completionTokens }} tokens</small></div>
          </article>
          <article v-if="sending" class="message-row assistant"><span class="message-avatar">AI</span><div class="message-body thinking"><i></i><i></i><i></i></div></article>
        </div>
        <footer class="composer"><div class="composer-tools"><el-button text :icon="MagicStick" @click="promptOpen = true">提示词</el-button><span>{{ draft.length }}/20000</span></div><el-input v-model="draft" type="textarea" resize="none" :autosize="{ minRows: 2, maxRows: 7 }" maxlength="20000" placeholder="输入消息" @keydown.ctrl.enter.prevent="send" /><el-button class="send-button" type="primary" :icon="Promotion" circle :loading="sending" aria-label="发送消息" @click="send" /></footer>
      </template>
      <div v-else class="workspace-empty"><span class="brand-symbol">AI</span><h2>今天想完成什么？</h2><p>选择左侧对话继续，或者创建一个新对话。</p><el-button type="primary" :icon="Plus" @click="openCreate">新建对话</el-button></div>
    </div>
    <el-dialog v-model="createOpen" title="新建对话" width="min(520px, 94vw)"><el-form label-position="top"><el-form-item label="模型连接"><el-select v-model="createForm.providerId" style="width: 100%" @change="selectCreateProvider"><el-option v-for="item in enabledProviders" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item><el-form-item label="模型"><el-input v-model="createForm.model" /></el-form-item><el-form-item label="系统提示词"><el-input v-model="createForm.systemPrompt" type="textarea" :rows="4" placeholder="可选，用于约束本次对话的角色和输出方式" /></el-form-item></el-form><template #footer><el-button @click="createOpen = false">取消</el-button><el-button type="primary" @click="createConversation">创建</el-button></template></el-dialog>
    <el-drawer v-model="promptOpen" title="选择提示词" size="min(460px, 94vw)"><div class="prompt-drawer-list"><button v-for="item in prompts" :key="item.id" type="button" @click="selectPrompt(item)"><span><strong>{{ item.title }}</strong><el-tag v-if="item.category" size="small" effect="plain">{{ item.category }}</el-tag></span><p>{{ item.content }}</p></button><el-empty v-if="!prompts.length" description="还没有提示词" /></div></el-drawer>
    <el-drawer v-model="settingsOpen" title="对话设置" size="min(460px, 94vw)"><el-form label-position="top"><el-form-item label="模型连接"><el-select v-model="settings.providerId" style="width: 100%" @change="selectSettingsProvider"><el-option v-for="item in enabledProviders" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item><el-form-item label="模型"><el-input v-model="settings.model" /></el-form-item><el-form-item label="系统提示词"><el-input v-model="settings.systemPrompt" type="textarea" :rows="8" maxlength="10000" show-word-limit /></el-form-item><el-button type="primary" style="width: 100%" @click="saveSettings">保存设置</el-button></el-form></el-drawer>
  </section>
</template>

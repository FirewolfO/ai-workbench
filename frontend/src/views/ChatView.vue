<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChatLineRound, CircleCheck, Close, CopyDocument, Delete, Edit, Expand, Loading, MagicStick, MoreFilled, Paperclip, Plus, Promotion, Refresh, Search, Setting, Star, StarFilled, VideoPause, WarningFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import { attachmentFileIcon, filesFromClipboard, isImagePreview } from '@/utils/attachments'
import { renderMarkdown } from '@/utils/markdown'
import { useAuthStore } from '@/stores/auth'
import type { AttachmentFileIcon } from '@/utils/attachments'
import type { Attachment, AvailableModel, Conversation, Message, MessageAttachment, Prompt, ReasoningEffort } from '@/types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const loading = ref(true)
const sending = ref(false)
const stopping = ref(false)
const switchingModel = ref(false)
const refreshingModels = ref(false)
const search = ref('')
const draft = ref('')
const conversations = ref<Conversation[]>([])
const current = ref<Conversation | null>(null)
const models = ref<AvailableModel[]>([])
const prompts = ref<Prompt[]>([])
type UploadStatus = 'queued' | 'uploading' | 'ready' | 'failed'
interface AttachmentUpload {
  localId: string
  file: File
  name: string
  progress: number
  status: UploadStatus
  error: string
  retryable: boolean
  previewUrl: string
  fileIcon: AttachmentFileIcon
  attachment?: Attachment
  controller?: AbortController
}
const attachmentUploads = ref<AttachmentUpload[]>([])
const pendingProviderId = ref('')
const pendingModel = ref('')
const promptOpen = ref(false)
const settingsOpen = ref(false)
const mobileConversationsOpen = ref(false)
const thread = ref<HTMLElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const settings = reactive({ providerId: '', model: '', systemPrompt: '', reasoningEffort: 'medium' as ReasoningEffort })
const selectedProviderId = computed(() => current.value?.providerId || pendingProviderId.value)
const selectedModel = computed(() => current.value?.model || pendingModel.value)
const activeModel = computed(() => models.value.find((item) => item.id === selectedProviderId.value))
const displayMessages = computed(() => current.value?.messages?.filter((message) => message.status !== 'generating') || [])
const readyAttachments = computed(() => attachmentUploads.value.flatMap((item) => item.attachment ? [item.attachment] : []))
const activeUploadCount = computed(() => attachmentUploads.value.filter((item) => item.status === 'uploading').length)
const uploadingCount = computed(() => attachmentUploads.value.filter((item) => item.status === 'queued' || item.status === 'uploading').length)
const failedUploadCount = computed(() => attachmentUploads.value.filter((item) => item.status === 'failed').length)
const uploadButtonState = computed(() => {
  if (failedUploadCount.value) return 'failed'
  if (uploadingCount.value) return 'uploading'
  if (attachmentUploads.value.length && readyAttachments.value.length === attachmentUploads.value.length) return 'ready'
  return 'idle'
})
const canSend = computed(() => !sending.value && uploadingCount.value === 0 && failedUploadCount.value === 0 && Boolean(draft.value.trim() || readyAttachments.value.length))
const activeModelNames = computed(() => {
  const result = [...(activeModel.value?.models || [])]
  const selected = selectedModel.value
  if (selected && !result.includes(selected)) result.unshift(selected)
  return result
})
const settingsModelNames = computed(() => {
  const provider = models.value.find((item) => item.id === settings.providerId)
  const result = [...(provider?.models || [])]
  if (settings.model && !result.includes(settings.model)) result.unshift(settings.model)
  return result
})
const effortOptions = [{ label: '快速', value: 'fast' }, { label: '中等', value: 'medium' }, { label: '高', value: 'high' }, { label: '极高', value: 'xhigh' }]
let modelRefreshTimer: number | undefined
let reasoningTimer: number | undefined
let reasoningSaving = false
let generationPoll = 0
let pendingReasoning: { conversationId: string; value: ReasoningEffort } | null = null
const persistedReasoning = new Map<string, ReasoningEffort>()
const MAX_ATTACHMENT_SIZE = 8 * 1024 * 1024
const MAX_CONCURRENT_UPLOADS = 2

async function loadLists() {
  const [conversationList, promptList] = await Promise.all([workbenchApi.conversations(search.value), workbenchApi.prompts()])
  conversations.value = conversationList
  prompts.value = promptList
}
async function refreshModels(refresh = false, reportError = false) {
  if (refreshingModels.value) return
  refreshingModels.value = true
  try { models.value = await workbenchApi.models(refresh); syncPendingModelSelection() }
  catch (error) { if (reportError) ElMessage.error(apiMessage(error, '模型列表加载失败')) }
  finally { refreshingModels.value = false }
}
function syncPendingModelSelection() {
  if (current.value) return
  const provider = models.value.find((item) => item.id === pendingProviderId.value) || models.value[0]
  pendingProviderId.value = provider?.id || ''
  if (!provider) { pendingModel.value = ''; return }
  if (!provider.models.includes(pendingModel.value)) pendingModel.value = provider.defaultModel
}
function refreshModelsWhenVisible() {
  if (!document.hidden) {
    void refreshModels(true)
    void workbenchApi.prompts().then(items => { prompts.value = items }).catch(() => undefined)
  }
}
async function loadCurrent(id: string) {
  generationPoll += 1
  sending.value = false
  stopping.value = false
  loading.value = true
  try {
    current.value = await workbenchApi.conversation(id)
    persistedReasoning.set(id, current.value.reasoningEffort)
    await scrollToEnd()
    const pending = [...(current.value.messages || [])].reverse().find((message) => message.role === 'assistant' && message.status === 'generating')
    if (pending) void trackGeneration(id, pending.id)
  }
  catch (error) { ElMessage.error(apiMessage(error, '对话加载失败')); await router.replace('/chat') }
  finally { loading.value = false }
}
async function initialize() {
  loading.value = true
  try {
    const initialPrompt = typeof route.query.prompt === 'string' ? route.query.prompt : ''
    await Promise.all([loadLists(), refreshModels(true, true)])
    const id = typeof route.params.id === 'string' ? route.params.id : ''
    if (id) await loadCurrent(id)
    else await openLatestOrCreate()
    if (initialPrompt) { draft.value = initialPrompt; await router.replace({ path: route.path }) }
  } catch (error) { ElMessage.error(apiMessage(error, '工作台加载失败')) }
  finally { loading.value = false }
}
async function createConversation() {
  if (!models.value.length) {
    ElMessage.warning(auth.isAdmin ? '请先添加并启用一个模型连接' : '管理员尚未配置可用模型')
    if (auth.isAdmin) await router.push('/providers')
    return
  }
  try {
    const conversation = await workbenchApi.createConversation({ providerId: selectedProviderId.value, model: selectedModel.value })
    conversations.value.unshift(conversation)
    mobileConversationsOpen.value = false
    await router.push(`/chat/${conversation.id}`)
  }
  catch (error) { ElMessage.error(apiMessage(error, '创建对话失败')) }
}
async function openLatestOrCreate() {
  if (typeof route.params.id === 'string' && route.params.id) return
  const latest = conversations.value.reduce<Conversation | undefined>((result, item) => !result || new Date(item.updatedAt).getTime() > new Date(result.updatedAt).getTime() ? item : result, undefined)
  if (latest) {
    await router.replace({ path: `/chat/${latest.id}`, query: route.query })
  }
}
async function send() {
  const content = draft.value.trim()
  if (!canSend.value || !current.value) return
  const conversationID = current.value.id
  const pendingUploads = attachmentUploads.value.filter((item) => item.attachment)
  const pendingAttachments = pendingUploads.flatMap((item) => item.attachment ? [item.attachment] : [])
  const sentPreviewURLs = pendingUploads.flatMap((item) => item.previewUrl ? [item.previewUrl] : [])
  const optimisticContent = content || `请处理附件：${pendingAttachments.map((item) => item.name).join('、')}`
  const optimisticAttachments: MessageAttachment[] = pendingUploads.map((item) => ({ name: item.name, contentType: item.file.type, previewUrl: item.previewUrl }))
  const optimistic: Message = { id: `local-${Date.now()}`, conversationId: conversationID, role: 'user', content: optimisticContent, attachments: optimisticAttachments, promptTokens: 0, completionTokens: 0, latencyMs: 0, status: 'completed', createdAt: new Date().toISOString() }
  current.value.messages ||= []
  current.value.messages.push(optimistic)
  draft.value = ''
  attachmentUploads.value = []
  sending.value = true
  await scrollToEnd()
  try {
    const queued = await workbenchApi.queueMessage(conversationID, content, pendingAttachments.map((item) => item.id))
    current.value.messages.push(queued)
    await trackGeneration(conversationID, queued.id)
  } catch (error) {
    generationPoll += 1
    sending.value = false
    stopping.value = false
    ElMessage.error(apiMessage(error, '模型响应失败'))
    current.value = await workbenchApi.conversation(conversationID)
  } finally {
    for (const previewURL of sentPreviewURLs) URL.revokeObjectURL(previewURL)
    await scrollToEnd()
  }
}

async function trackGeneration(conversationID: string, messageID: string) {
  const poll = ++generationPoll
  sending.value = true
  let failures = 0
  try {
    while (poll === generationPoll && current.value?.id === conversationID) {
      await new Promise((resolve) => window.setTimeout(resolve, 1_000))
      if (poll !== generationPoll || current.value?.id !== conversationID) return
      let updated: Conversation
      try {
        updated = await workbenchApi.conversation(conversationID)
        failures = 0
      } catch (error) {
        failures += 1
        if (failures >= 3) throw error
        continue
      }
      current.value = updated
      await scrollToEnd()
      const answer = updated.messages?.find((message) => message.id === messageID)
      if (!answer || answer.status === 'generating') continue
      if (answer.status === 'failed') ElMessage.error('模型响应失败，请查看错误信息')
      try { await loadLists() } catch { /* 对话结果已经加载，列表稍后会自动刷新。 */ }
      return
    }
  } catch (error) {
    ElMessage.error(apiMessage(error, '生成状态读取失败，请刷新页面查看结果'))
  } finally {
    if (poll === generationPoll) {
      sending.value = false
      stopping.value = false
    }
  }
}
async function stop() {
  if (!current.value || !sending.value || stopping.value) return
  stopping.value = true
  try {
    const result = await workbenchApi.stopGeneration(current.value.id)
    if (!result.stopped) { stopping.value = false; ElMessage.info('当前没有正在生成的回答') }
  } catch (error) { stopping.value = false; ElMessage.error(apiMessage(error, '停止失败')) }
}
async function changeProvider(providerId: string) {
  if (sending.value || switchingModel.value) return
  const selected = models.value.find((item) => item.id === providerId)
  if (!selected) return
  if (!current.value) {
    pendingProviderId.value = providerId
    pendingModel.value = selected.defaultModel
    return
  }
  switchingModel.value = true
  try { applyConversationUpdate(await workbenchApi.updateConversation(current.value.id, { providerId, model: selected.defaultModel })) }
  catch (error) { ElMessage.error(apiMessage(error, '模型切换失败')) }
  finally { switchingModel.value = false }
}
async function changeSpecificModel(model: string) {
  if (sending.value || switchingModel.value || !model) return
  if (!current.value) { pendingModel.value = model; return }
  switchingModel.value = true
  try { applyConversationUpdate(await workbenchApi.updateConversation(current.value.id, { model })) }
  catch (error) { ElMessage.error(apiMessage(error, '模型切换失败')) }
  finally { switchingModel.value = false }
}
function changeReasoning(value: string | number | boolean) {
  if (!current.value || sending.value) return
  const effort = value as ReasoningEffort
  current.value.reasoningEffort = effort
  pendingReasoning = { conversationId: current.value.id, value: effort }
  if (reasoningTimer) window.clearTimeout(reasoningTimer)
  reasoningTimer = window.setTimeout(() => void flushReasoning(), 180)
}
async function flushReasoning() {
  if (reasoningSaving || !pendingReasoning) return
  const update = pendingReasoning
  pendingReasoning = null
  reasoningSaving = true
  try {
    await workbenchApi.updateConversation(update.conversationId, { reasoningEffort: update.value })
    persistedReasoning.set(update.conversationId, update.value)
  } catch (error) {
    if (current.value?.id === update.conversationId && !pendingReasoning) {
      current.value.reasoningEffort = persistedReasoning.get(update.conversationId) || 'medium'
      ElMessage.error(apiMessage(error, '推理档位更新失败'))
    }
  } finally {
    reasoningSaving = false
    if (pendingReasoning) void flushReasoning()
  }
}
function localUploadID() {
  return typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `upload-${Date.now()}-${Math.random().toString(16).slice(2)}`
}
function selectFiles(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  addFiles(files)
}
function pasteAttachments(event: ClipboardEvent) {
  const files = filesFromClipboard(event.clipboardData)
  if (!files.length) return
  event.preventDefault()
  addFiles(files)
}
function addFiles(files: File[]) {
  const availableSlots = 4 - attachmentUploads.value.length
  if (availableSlots <= 0) { ElMessage.warning('每次最多上传 4 个附件'); return }
  const accepted = files.slice(0, availableSlots)
  if (files.length > accepted.length) ElMessage.warning('每次最多上传 4 个附件，多余文件未加入')
  for (const file of accepted) {
    const oversized = file.size > MAX_ATTACHMENT_SIZE
    const item = reactive<AttachmentUpload>({
      localId: localUploadID(),
      file,
      name: file.name,
      progress: 0,
      status: oversized ? 'failed' : 'queued',
      error: oversized ? '文件超过 8 MiB' : '',
      retryable: !oversized,
      previewUrl: file.type.startsWith('image/') ? URL.createObjectURL(file) : '',
      fileIcon: attachmentFileIcon(file.name, file.type),
    })
    attachmentUploads.value.push(item)
  }
  pumpUploads()
}
function pumpUploads() {
  while (activeUploadCount.value < MAX_CONCURRENT_UPLOADS) {
    const next = attachmentUploads.value.find((item) => item.status === 'queued')
    if (!next) return
    void uploadAttachment(next)
  }
}
async function uploadAttachment(item: AttachmentUpload) {
  const controller = new AbortController()
  item.controller = controller
  item.progress = 0
  item.status = 'uploading'
  item.error = ''
  item.retryable = true
  try {
    const attachment = await workbenchApi.uploadAttachment(item.file, (progress) => { item.progress = progress }, controller.signal)
    if (!attachmentUploads.value.some((candidate) => candidate.localId === item.localId)) {
      void workbenchApi.deleteAttachment(attachment.id).catch(() => undefined)
      return
    }
    item.attachment = attachment
    item.progress = 100
    item.status = 'ready'
  } catch (error) {
    if (controller.signal.aborted) return
    item.status = 'failed'
    item.error = apiMessage(error, '上传失败')
  } finally {
    if (item.controller === controller) item.controller = undefined
    pumpUploads()
  }
}
function retryAttachment(item: AttachmentUpload) {
  if (item.status !== 'failed' || !item.retryable) return
  item.status = 'queued'
  item.progress = 0
  item.error = ''
  pumpUploads()
}
function uploadStatusLabel(item: AttachmentUpload) {
  if (item.status === 'ready') return '完成'
  if (item.status === 'failed') return item.retryable ? '失败' : '超过 8 MiB'
  if (item.status === 'queued') return '等待上传'
  if (item.progress >= 100) return '服务器确认中'
  return item.progress > 0 ? `${item.progress}%` : '正在上传'
}
function openFilePicker() {
  if (attachmentUploads.value.length >= 4) {
    ElMessage.warning('每次最多上传 4 个附件')
    return
  }
  fileInput.value?.click()
}
function removeAttachment(item: AttachmentUpload) {
  item.controller?.abort()
  attachmentUploads.value = attachmentUploads.value.filter((candidate) => candidate.localId !== item.localId)
  if (item.previewUrl) URL.revokeObjectURL(item.previewUrl)
  if (item.attachment) void workbenchApi.deleteAttachment(item.attachment.id).catch((error) => ElMessage.error(apiMessage(error, '附件删除失败')))
  pumpUploads()
}
function clearUnusedAttachments() {
  const unused = attachmentUploads.value
  attachmentUploads.value = []
  for (const item of unused) {
    item.controller?.abort()
    if (item.previewUrl) URL.revokeObjectURL(item.previewUrl)
    if (item.attachment) void workbenchApi.deleteAttachment(item.attachment.id).catch(() => undefined)
  }
}
function canPreviewAttachment(attachment: MessageAttachment) {
  return isImagePreview(attachment.contentType, attachment.previewUrl || '')
}
async function scrollToEnd() { await nextTick(); if (thread.value) thread.value.scrollTop = thread.value.scrollHeight }
async function selectPrompt(item: Prompt) { await workbenchApi.usePrompt(item.id); draft.value = item.content; promptOpen.value = false; await nextTick() }
function applyConversationUpdate(updated: Conversation) {
  if (current.value?.id === updated.id) updated.reasoningEffort = current.value.reasoningEffort
  current.value = updated
}
async function togglePin() { if (!current.value) return; applyConversationUpdate(await workbenchApi.updateConversation(current.value.id, { pinned: !current.value.pinned })); await loadLists() }
async function rename() {
  if (!current.value) return
  try { const result = await ElMessageBox.prompt('输入新的对话名称', '重命名对话', { inputValue: current.value.title, inputValidator: (value) => Boolean(value.trim()) || '名称不能为空' }); applyConversationUpdate(await workbenchApi.updateConversation(current.value.id, { title: result.value })); await loadLists() }
  catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error)) }
}
async function remove() {
  if (!current.value) return
  try { await ElMessageBox.confirm(`确定删除“${current.value.title}”及全部消息吗？`, '删除对话', { type: 'warning' }); await workbenchApi.deleteConversation(current.value.id); current.value = null; await loadLists(); await router.replace('/chat') }
  catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error, '删除失败')) }
}
function openSettings() { if (!current.value) return; Object.assign(settings, { providerId: current.value.providerId, model: current.value.model, systemPrompt: current.value.systemPrompt, reasoningEffort: current.value.reasoningEffort }); settingsOpen.value = true }
function selectSettingsModel(value: string) { const selected = models.value.find((item) => item.id === value); if (selected) settings.model = selected.defaultModel }
async function saveSettings() { if (!current.value) return; try { current.value = await workbenchApi.updateConversation(current.value.id, { ...settings }); settingsOpen.value = false; ElMessage.success('对话设置已更新') } catch (error) { ElMessage.error(apiMessage(error, '保存失败')) } }
async function copy(content: string) { try { await navigator.clipboard.writeText(content); ElMessage.success('已复制') } catch { ElMessage.error('复制失败') } }
async function openConversation(id: string) { mobileConversationsOpen.value = false; if (pendingReasoning) void flushReasoning(); await router.push(`/chat/${id}`) }

watch(() => route.params.id, (id) => {
  mobileConversationsOpen.value = false
  clearUnusedAttachments()
  if (typeof id === 'string' && id) void loadCurrent(id)
  else {
    generationPoll += 1
    sending.value = false
    stopping.value = false
    current.value = null
    void openLatestOrCreate()
  }
})
onMounted(() => {
  void initialize()
  modelRefreshTimer = window.setInterval(refreshModelsWhenVisible, 60_000)
  document.addEventListener('visibilitychange', refreshModelsWhenVisible)
  window.addEventListener('focus', refreshModelsWhenVisible)
})
onBeforeUnmount(() => {
  generationPoll += 1
  clearUnusedAttachments()
  if (modelRefreshTimer) window.clearInterval(modelRefreshTimer)
  if (reasoningTimer) window.clearTimeout(reasoningTimer)
  if (pendingReasoning) void flushReasoning()
  document.removeEventListener('visibilitychange', refreshModelsWhenVisible)
  window.removeEventListener('focus', refreshModelsWhenVisible)
})
</script>

<template>
  <section class="chat-workspace">
    <button v-if="mobileConversationsOpen" class="conversation-scrim" type="button" aria-label="关闭对话列表" @click="mobileConversationsOpen = false" />
    <aside class="conversation-panel" :class="{ 'mobile-open': mobileConversationsOpen }">
      <header class="conversation-mobile-header"><strong>对话</strong><el-button text :icon="Close" aria-label="关闭对话列表" @click="mobileConversationsOpen = false" /></header>
      <el-button type="primary" :icon="Plus" @click="createConversation">新对话</el-button>
      <el-input v-model="search" :prefix-icon="Search" clearable placeholder="搜索对话" @keyup.enter="loadLists" @clear="loadLists" />
      <div class="conversation-list">
        <button v-for="item in conversations" :key="item.id" type="button" :class="{ active: item.id === current?.id }" @click="openConversation(item.id)">
          <el-icon><StarFilled v-if="item.pinned" /><ChatLineRound v-else /></el-icon><span><strong>{{ item.title }}</strong><small>{{ item.lastMessage || '还没有消息' }}</small></span><b>{{ item.messageCount }}</b>
        </button>
      </div>
    </aside>
    <div v-loading="loading" class="chat-main">
      <div class="chat-model-bar">
        <label><span>供应商</span><el-select :model-value="selectedProviderId" filterable :disabled="!models.length || sending || switchingModel" aria-label="供应商" @change="changeProvider"><el-option v-for="item in models" :key="item.id" :label="item.name" :value="item.id" /></el-select></label>
        <label><span>模型</span><el-select :model-value="selectedModel" filterable :disabled="!activeModelNames.length || sending || switchingModel" aria-label="模型" @change="changeSpecificModel"><el-option v-for="item in activeModelNames" :key="item" :label="item" :value="item" /></el-select></label>
        <el-tooltip content="刷新模型列表"><el-button text :icon="Refresh" :loading="refreshingModels" aria-label="刷新模型列表" @click="refreshModels(true, true)" /></el-tooltip>
      </div>
      <template v-if="current">
        <header class="chat-header"><div class="chat-title"><el-button class="mobile-conversation-toggle" text :icon="Expand" aria-label="打开对话列表" @click="mobileConversationsOpen = true" /><div><h2>{{ current.title }}</h2><span>{{ activeModel?.name || '模型' }} · {{ current.model }}<template v-if="activeModel?.webSearchEnabled"> · 可联网</template></span></div></div><div><el-button text :icon="current.pinned ? StarFilled : Star" aria-label="置顶" @click="togglePin" /><el-dropdown trigger="click"><el-button text :icon="MoreFilled" aria-label="更多操作" /><template #dropdown><el-dropdown-menu><el-dropdown-item :icon="Edit" @click="rename">重命名</el-dropdown-item><el-dropdown-item :icon="Setting" @click="openSettings">对话设置</el-dropdown-item><el-dropdown-item divided :icon="Delete" @click="remove">删除对话</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div></header>
        <div ref="thread" class="message-thread">
          <div v-if="!displayMessages.length" class="chat-empty"><span class="brand-symbol">AI</span><h3>从一个问题开始</h3><p>{{ current.systemPrompt || '选择提示词，或直接输入你想处理的事情。' }}</p></div>
          <article v-for="message in displayMessages" :key="message.id" class="message-row" :class="message.role">
            <span class="message-avatar">{{ message.role === 'user' ? '我' : 'AI' }}</span>
            <div class="message-body"><header><strong>{{ message.role === 'user' ? '我' : message.model || '助手' }}</strong><el-button v-if="message.role === 'assistant' && message.status === 'completed'" text :icon="CopyDocument" aria-label="复制回答" @click="copy(message.content)" /></header><div v-if="message.role === 'assistant'" class="markdown-body" :class="{ failed: message.status !== 'completed' }" v-html="renderMarkdown(message.content)"></div><template v-else><p>{{ message.content }}</p><div v-if="message.attachments?.length" class="message-attachments"><figure v-for="(attachment, index) in message.attachments" :key="`${attachment.name}-${index}`" :class="{ 'is-image': canPreviewAttachment(attachment) }"><el-image v-if="canPreviewAttachment(attachment)" :src="attachment.previewUrl" :preview-src-list="[attachment.previewUrl || '']" :alt="attachment.name" fit="cover" preview-teleported /><span><el-icon><Paperclip /></el-icon><b>{{ attachment.name }}</b></span></figure></div></template><small v-if="message.role === 'assistant' && message.status === 'completed'">{{ message.latencyMs }} ms · {{ message.promptTokens + message.completionTokens }} tokens</small></div>
          </article>
          <article v-if="sending" class="message-row assistant"><span class="message-avatar">AI</span><div class="message-body thinking"><i></i><i></i><i></i><span>{{ stopping ? '正在停止' : '正在生成' }}</span></div></article>
        </div>
        <footer class="composer">
          <div v-if="attachmentUploads.length" class="attachment-list" aria-live="polite">
            <span v-for="item in attachmentUploads" :key="item.localId" class="attachment-chip" :class="[`is-${item.status}`, { 'has-preview': item.previewUrl }]" :style="{ '--upload-progress': `${item.progress}%` }" :title="item.error || item.name">
              <img v-if="item.previewUrl" class="attachment-upload-preview" :src="item.previewUrl" alt="" />
              <span v-else class="attachment-type-icon" :class="`is-${item.fileIcon.kind}`" :title="item.fileIcon.title" aria-hidden="true"><small>{{ item.fileIcon.label }}</small></span>
              <b class="attachment-name">{{ item.name }}</b>
              <span class="attachment-status-group">
                <el-icon v-if="item.status === 'ready'" class="attachment-status"><CircleCheck /></el-icon>
                <el-icon v-else-if="item.status === 'failed'" class="attachment-status"><WarningFilled /></el-icon>
                <el-icon v-else class="attachment-status spinning"><Loading /></el-icon>
                <small class="attachment-status">{{ uploadStatusLabel(item) }}</small>
                <button v-if="item.status === 'failed' && item.retryable" type="button" :aria-label="`重试 ${item.name}`" @click="retryAttachment(item)"><el-icon><Refresh /></el-icon></button>
                <button type="button" :aria-label="`移除 ${item.name}`" @click="removeAttachment(item)"><el-icon><Close /></el-icon></button>
              </span>
            </span>
          </div>
          <el-input v-model="draft" type="textarea" resize="none" :autosize="{ minRows: 2, maxRows: 7 }" maxlength="20000" placeholder="输入消息" @paste="pasteAttachments" @keydown.enter.exact.prevent="send" />
          <div class="composer-tools">
            <div class="composer-actions">
              <el-button class="attachment-trigger" :class="`is-${uploadButtonState}`" text aria-label="选择附件" @click="openFilePicker">
                <el-icon :class="{ spinning: uploadButtonState === 'uploading' }"><Loading v-if="uploadButtonState === 'uploading'" /><CircleCheck v-else-if="uploadButtonState === 'ready'" /><WarningFilled v-else-if="uploadButtonState === 'failed'" /><Paperclip v-else /></el-icon>
              </el-button>
              <input ref="fileInput" class="file-input" type="file" multiple @change="selectFiles" />
              <el-tooltip content="选择提示词"><el-button text :icon="MagicStick" aria-label="选择提示词" @click="promptOpen = true" /></el-tooltip>
              <el-select :model-value="current.reasoningEffort" class="composer-effort" size="small" :disabled="sending" aria-label="推理档位" @change="changeReasoning"><el-option v-for="item in effortOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select>
            </div>
            <span>{{ draft.length }}/20000</span>
          </div>
          <el-tooltip :content="sending ? '停止生成' : uploadingCount ? '等待附件上传完成' : failedUploadCount ? '请重试或移除失败附件' : '发送'"><el-button class="send-button" :type="sending ? 'danger' : 'primary'" :icon="sending ? VideoPause : Promotion" circle :disabled="!sending && !canSend" :aria-label="sending ? '停止生成' : '发送消息'" @click="sending ? stop() : send()" /></el-tooltip>
        </footer>
      </template>
      <div v-else class="workspace-empty"><span class="brand-symbol">AI</span><h2>今天想完成什么？</h2><p>打开已有对话继续，或者创建一个新对话。</p><div class="workspace-empty-actions"><el-button class="mobile-conversation-toggle" :icon="Expand" @click="mobileConversationsOpen = true">对话列表</el-button><el-button type="primary" :icon="Plus" @click="createConversation">新建对话</el-button></div></div>
    </div>
    <el-drawer v-model="promptOpen" title="选择提示词" size="min(320px, 78vw)"><div class="prompt-drawer-list"><button v-for="item in prompts" :key="item.id" type="button" @click="selectPrompt(item)"><span><strong>{{ item.title }}</strong><el-tag v-if="item.shared" type="success" size="small" effect="plain">共享</el-tag><el-tag v-if="item.category" size="small" effect="plain">{{ item.category }}</el-tag></span><p>{{ item.content }}</p></button><el-empty v-if="!prompts.length" description="还没有提示词" /></div></el-drawer>
    <el-drawer v-model="settingsOpen" title="对话设置" size="min(460px, 94vw)"><el-form label-position="top"><el-form-item label="供应商"><el-select v-model="settings.providerId" filterable style="width: 100%" @change="selectSettingsModel"><el-option v-for="item in models" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item><el-form-item label="模型"><el-select v-model="settings.model" filterable style="width: 100%"><el-option v-for="item in settingsModelNames" :key="item" :label="item" :value="item" /></el-select></el-form-item><el-form-item label="推理档位"><el-segmented v-model="settings.reasoningEffort" :options="effortOptions" /></el-form-item><el-form-item label="系统提示词"><el-input v-model="settings.systemPrompt" type="textarea" :rows="8" maxlength="10000" show-word-limit /></el-form-item><el-button type="primary" style="width: 100%" @click="saveSettings">保存设置</el-button></el-form></el-drawer>
  </section>
</template>

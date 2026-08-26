<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Connection, Delete, Edit, Key, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { Provider, ProviderInput } from '@/types'

const loading = ref(true)
const saving = ref(false)
const testingID = ref('')
const dialogOpen = ref(false)
const editingID = ref('')
const providers = ref<Provider[]>([])
const form = reactive<ProviderInput>({ name: '', baseUrl: 'https://api.openai.com/v1', defaultModel: '', apiKey: '', enabled: true })

async function load() {
  loading.value = true
  try { providers.value = await workbenchApi.providers() }
  catch (error) { ElMessage.error(apiMessage(error, '模型连接加载失败')) }
  finally { loading.value = false }
}
function openCreate() { editingID.value = ''; Object.assign(form, { name: '', baseUrl: 'https://api.openai.com/v1', defaultModel: '', apiKey: '', enabled: true }); dialogOpen.value = true }
function openEdit(item: Provider) { editingID.value = item.id; Object.assign(form, { name: item.name, baseUrl: item.baseUrl, defaultModel: item.defaultModel, apiKey: '', enabled: item.enabled }); dialogOpen.value = true }
function status(item: Provider): { label: string; type: 'success' | 'danger' | 'warning' | 'info' } {
  if (!item.enabled) return { label: '已停用', type: 'info' }
  if (item.available) return { label: '可使用', type: 'success' }
  if (item.lastTestedAt) return { label: '连接失败', type: 'danger' }
  return { label: '未测试', type: 'warning' }
}
function formatTestTime(value?: string) {
  if (!value) return '尚未测试'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value))
}
async function save() {
  if (!form.name.trim() || !form.baseUrl.trim() || !form.defaultModel.trim()) { ElMessage.warning('请填写完整的模型连接信息'); return }
  saving.value = true
  try {
    if (editingID.value) await workbenchApi.updateProvider(editingID.value, { ...form })
    else await workbenchApi.createProvider({ ...form })
    ElMessage.success(editingID.value ? '模型连接已更新' : '模型连接已创建')
    dialogOpen.value = false
    await load()
  } catch (error) { ElMessage.error(apiMessage(error, '保存失败')) }
  finally { saving.value = false }
}
async function test(item: Provider) {
  testingID.value = item.id
  const close = ElMessage.info({ message: '正在测试模型连接', duration: 0 })
  try { const result = await workbenchApi.testProvider(item.id); close.close(); ElMessage.success(`连接正常，发现 ${result.modelCount} 个模型，延迟 ${result.latencyMs} ms`) }
  catch (error) { close.close(); ElMessage.error(apiMessage(error, '连接测试失败')) }
  finally { testingID.value = ''; await load() }
}
async function remove(item: Provider) {
  try { await ElMessageBox.confirm(`确定删除“${item.name}”吗？`, '删除模型连接', { type: 'warning' }); await workbenchApi.deleteProvider(item.id); ElMessage.success('已删除'); await load() }
  catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error, '删除失败')) }
}
onMounted(load)
</script>

<template>
  <section class="page providers-page">
    <div class="page-heading"><div><p>MODEL CONNECTIONS</p><h2>模型连接</h2><span>连接 OpenAI Compatible 模型服务</span></div><el-button type="primary" :icon="Plus" @click="openCreate">添加连接</el-button></div>
    <div v-loading="loading" class="provider-grid">
      <article v-for="item in providers" :key="item.id" class="provider-card">
        <header><span class="provider-icon"><el-icon><Connection /></el-icon></span><el-tag :type="status(item).type" effect="plain">{{ status(item).label }}</el-tag></header>
        <h3>{{ item.name }}</h3><p>{{ item.baseUrl }}</p>
        <dl><div><dt>默认模型</dt><dd>{{ item.defaultModel }}</dd></div><div><dt>可用模型</dt><dd>{{ item.models.length }} 个</dd></div><div><dt>访问密钥</dt><dd><el-icon><Key /></el-icon>{{ item.hasApiKey ? '已加密配置' : '未填写' }}</dd></div><div><dt>最近检测</dt><dd>{{ formatTestTime(item.lastTestedAt) }}<template v-if="item.available"> · {{ item.lastTestLatencyMs }} ms</template></dd></div></dl>
        <p class="provider-error" :title="item.lastTestError">{{ item.lastTestError || ' ' }}</p>
        <footer><el-button :loading="testingID === item.id" :disabled="Boolean(testingID)" @click="test(item)">测试连接</el-button><span><el-button text :icon="Edit" aria-label="编辑" @click="openEdit(item)" /><el-button text type="danger" :icon="Delete" aria-label="删除" @click="remove(item)" /></span></footer>
      </article>
      <button v-if="!loading && !providers.length" class="provider-empty" type="button" @click="openCreate"><el-icon><Plus /></el-icon><strong>添加第一个模型连接</strong><span>支持 OpenAI、Azure OpenAI、Ollama 等兼容接口</span></button>
    </div>
    <el-dialog v-model="dialogOpen" :title="editingID ? '编辑模型连接' : '添加模型连接'" width="min(520px, 94vw)" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="连接名称"><el-input v-model="form.name" maxlength="100" placeholder="例如：公司 OpenAI" /></el-form-item>
        <el-form-item label="API Base URL"><el-input v-model="form.baseUrl" placeholder="https://api.openai.com/v1" /></el-form-item>
        <el-form-item label="默认模型"><el-input v-model="form.defaultModel" placeholder="例如：gpt-4.1" /></el-form-item>
        <el-form-item label="API Key"><el-input v-model="form.apiKey" type="password" show-password :placeholder="editingID ? '留空则保留原密钥' : '本地模型可留空'" autocomplete="new-password" /></el-form-item>
        <el-form-item label="状态"><el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="dialogOpen = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>
  </section>
</template>

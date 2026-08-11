<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ChatLineRound, Delete, Edit, Plus, Search, Star, StarFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { Prompt, PromptInput } from '@/types'

const router = useRouter()
const loading = ref(true)
const saving = ref(false)
const search = ref('')
const prompts = ref<Prompt[]>([])
const dialogOpen = ref(false)
const editingID = ref('')
const form = reactive<PromptInput>({ title: '', description: '', category: '', content: '', favorite: false })

async function load() { loading.value = true; try { prompts.value = await workbenchApi.prompts(search.value) } catch (error) { ElMessage.error(apiMessage(error, '提示词加载失败')) } finally { loading.value = false } }
function openCreate() { editingID.value = ''; Object.assign(form, { title: '', description: '', category: '', content: '', favorite: false }); dialogOpen.value = true }
function openEdit(item: Prompt) { editingID.value = item.id; Object.assign(form, { title: item.title, description: item.description, category: item.category, content: item.content, favorite: item.favorite }); dialogOpen.value = true }
async function save() { if (!form.title.trim() || !form.content.trim()) { ElMessage.warning('请填写标题和提示词内容'); return }; saving.value = true; try { if (editingID.value) await workbenchApi.updatePrompt(editingID.value, { ...form }); else await workbenchApi.createPrompt({ ...form }); dialogOpen.value = false; ElMessage.success('提示词已保存'); await load() } catch (error) { ElMessage.error(apiMessage(error, '保存失败')) } finally { saving.value = false } }
async function toggleFavorite(item: Prompt) { try { await workbenchApi.updatePrompt(item.id, { title: item.title, description: item.description, category: item.category, content: item.content, favorite: !item.favorite }); await load() } catch (error) { ElMessage.error(apiMessage(error)) } }
async function use(item: Prompt) { await workbenchApi.usePrompt(item.id); await router.push({ path: '/chat', query: { prompt: item.content } }) }
async function remove(item: Prompt) { try { await ElMessageBox.confirm(`确定删除“${item.title}”吗？`, '删除提示词', { type: 'warning' }); await workbenchApi.deletePrompt(item.id); await load() } catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error(apiMessage(error, '删除失败')) } }
onMounted(load)
</script>

<template>
  <section class="page prompts-page">
    <div class="page-heading"><div><p>PROMPT LIBRARY</p><h2>我的提示词</h2><span>沉淀可复用的工作方法</span></div><el-button type="primary" :icon="Plus" @click="openCreate">新建提示词</el-button></div>
    <div class="filter-line"><el-input v-model="search" :prefix-icon="Search" clearable placeholder="搜索标题、描述或内容" @keyup.enter="load" @clear="load" /><el-button @click="load">搜索</el-button></div>
    <div v-loading="loading" class="prompt-grid">
      <article v-for="item in prompts" :key="item.id" class="prompt-card">
        <header><el-tag v-if="item.category" effect="plain">{{ item.category }}</el-tag><el-button text :icon="item.favorite ? StarFilled : Star" :class="{ favorite: item.favorite }" aria-label="收藏" @click="toggleFavorite(item)" /></header>
        <h3>{{ item.title }}</h3><p>{{ item.description || item.content }}</p><pre>{{ item.content }}</pre>
        <footer><span>使用 {{ item.useCount }} 次</span><div><el-button text :icon="Edit" aria-label="编辑" @click="openEdit(item)" /><el-button text type="danger" :icon="Delete" aria-label="删除" @click="remove(item)" /><el-button type="primary" plain :icon="ChatLineRound" @click="use(item)">用于对话</el-button></div></footer>
      </article>
    </div>
    <el-empty v-if="!loading && !prompts.length" description="还没有提示词"><el-button type="primary" @click="openCreate">新建提示词</el-button></el-empty>
    <el-dialog v-model="dialogOpen" :title="editingID ? '编辑提示词' : '新建提示词'" width="min(620px, 94vw)" destroy-on-close>
      <el-form label-position="top"><div class="form-grid"><el-form-item label="标题"><el-input v-model="form.title" maxlength="120" /></el-form-item><el-form-item label="分类"><el-input v-model="form.category" maxlength="60" placeholder="例如：研发、写作" /></el-form-item></div><el-form-item label="描述"><el-input v-model="form.description" maxlength="300" /></el-form-item><el-form-item label="提示词内容"><el-input v-model="form.content" type="textarea" :rows="9" maxlength="20000" show-word-limit /></el-form-item><el-checkbox v-model="form.favorite">加入收藏</el-checkbox></el-form>
      <template #footer><el-button @click="dialogOpen = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>
  </section>
</template>
